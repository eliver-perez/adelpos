package legacyread

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"local/ad-el-pos/server/internal/legacypkg"
)

// CLI es el subcomando legacy-prepare:
//
//	ad-el-pos-server legacy-prepare --backup <respaldo.bak> --output <paquete.adelmig> [--watch-stdin]
//
// La frase de paso llega solo por la entrada estándar (primera línea; nunca por argumentos ni variables). Si la
// entrada es una terminal, se pide dos veces sin mostrarla. Con --watch-stdin, cerrar la entrada estándar cancela la
// preparación (así la aplicación de escritorio la cancela también en Windows, donde no hay SIGTERM). El avance va a la
// salida estándar como líneas JSON (events.go). Sale con 0 si el paquete quedó listo y con 1 si no.
type CLI struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Terminal lee la frase de paso de una terminal sin mostrarla (nil: la entrada no es una terminal).
	Terminal func(prompt string) (string, error)
	// FindDocker y NewRunner permiten probar sin el docker real.
	FindDocker func() (string, error)
	NewRunner  func(docker string) Runner
	TempDir    string
	// Adjust permite a las pruebas cambiar las opciones (Argon2 rápido, páginas chicas).
	Adjust func(options *Options)
}

// Main ejecuta el subcomando con la entrada y la salida del proceso, y se detiene en orden con Ctrl-C o SIGTERM.
func Main(arguments []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Si la aplicación que lanzó el subcomando cierra su salida, escribir no debe terminar el proceso sin limpiar (en
	// macOS SIGPIPE lo mataría): la escritura falla y la preparación se cancela.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)
	cli := CLI{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		FindDocker: SystemLocator().Find,
		NewRunner: func(docker string) Runner {
			return ExecRunner{Docker: docker}
		},
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		cli.Terminal = func(prompt string) (string, error) {
			fmt.Fprint(os.Stderr, prompt)
			secret, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			return string(secret), err
		}
	}
	return cli.Run(ctx, arguments)
}

// Run interpreta los argumentos, lee la frase de paso y prepara el paquete.
func (cli CLI) Run(parent context.Context, arguments []string) int {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	reporter := NewJSONReporter(cli.Stdout, cancel)
	report := func(err error) int {
		failure := AsFailure(err)
		if parent.Err() != nil || (ctx.Err() != nil && failure.Code != CodeOutputExists) {
			failure = fail(CodeCancelled, "", err)
		}
		reporter.Emit(errorEvent(failure))
		return 1
	}
	flags := flag.NewFlagSet("legacy-prepare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	backup := flags.String("backup", "", "respaldo .bak del sistema anterior")
	output := flags.String("output", "", "paquete .adelmig que se crea (nunca se reemplaza uno existente)")
	watch := flags.Bool("watch-stdin", false, "cancelar si se cierra la entrada estándar")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *backup == "" || *output == "" {
		return report(fail(CodeUsage, "", err))
	}
	input := bufio.NewReaderSize(cli.Stdin, 4096)
	passphrase, err := cli.readPassphrase(input)
	if err != nil {
		return report(err)
	}
	if *watch {
		go func() {
			_, _ = io.Copy(io.Discard, input)
			cancel()
		}()
	}
	docker, err := cli.FindDocker()
	if err != nil {
		return report(err)
	}
	options := Options{Backup: *backup, Output: *output, Passphrase: passphrase}
	if cli.Adjust != nil {
		cli.Adjust(&options)
	}
	tempDir := cli.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	preparer := &Preparer{Runner: cli.NewRunner(docker), Events: reporter, TempDir: tempDir}
	summary, err := preparer.Prepare(ctx, options)
	if err != nil {
		return report(err)
	}
	reporter.Emit(ResultEvent{Event: "result", Summary: summary})
	return 0
}

// readPassphrase lee la frase de paso: de la terminal (dos veces) o de la primera línea de la entrada estándar.
func (cli CLI) readPassphrase(input *bufio.Reader) (string, error) {
	if cli.Terminal != nil {
		first, err := cli.Terminal("Frase de paso del paquete: ")
		if err != nil {
			return "", fail(CodePassphraseInvalid, "", err)
		}
		second, err := cli.Terminal("Escríbela otra vez: ")
		if err != nil {
			return "", fail(CodePassphraseInvalid, "", err)
		}
		if first != second {
			return "", fail(CodePassphraseInvalid, "las dos frases no coinciden", nil)
		}
		return checkPassphrase(first)
	}
	line := []byte{}
	for {
		character, err := input.ReadByte()
		if err == io.EOF && len(line) > 0 {
			break
		}
		if err != nil {
			return "", fail(CodePassphraseInvalid, "no llegó la frase de paso por la entrada estándar", err)
		}
		if character == '\n' {
			break
		}
		line = append(line, character)
		if len(line) > legacypkg.MaxPassphraseBytes+1 {
			return "", fail(CodePassphraseInvalid, "frase de paso demasiado larga", nil)
		}
	}
	return checkPassphrase(strings.TrimSuffix(string(line), "\r"))
}

func checkPassphrase(passphrase string) (string, error) {
	if err := legacypkg.ValidatePassphrase(passphrase); err != nil {
		var typed *legacypkg.Error
		if errors.As(err, &typed) {
			return "", &Failure{Code: CodePassphraseInvalid, Message: typed.Message, cause: err}
		}
		return "", fail(CodePassphraseInvalid, "", err)
	}
	return passphrase, nil
}
