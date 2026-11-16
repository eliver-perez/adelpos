package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacyread"
	"local/ad-el-pos/server/internal/legacysynth"
)

// Etapa 9H: subcomandos de la migración del sistema anterior. Tienen sus propios argumentos y su propia salida (líneas
// JSON), así que se atienden antes que los demás comandos y sin la bitácora del servidor.
//
//   - legacy-prepare: el lector del respaldo (internal/legacyread, con Docker). Es el único lugar del servidor que lo
//     usa: serve y host nunca dependen de Docker ni de SQL Server.
//   - legacy-synthetic: solo pruebas (exige ADEL_TEST_MODE=1). Escribe el paquete sintético de internal/legacysynth,
//     sin Docker, para el asistente en Playwright.
func runLegacyCommand(arguments []string) (int, bool) {
	if len(arguments) == 0 {
		return 0, false
	}
	switch arguments[0] {
	case "legacy-prepare":
		return legacyread.Main(arguments[1:]), true
	case "legacy-synthetic":
		return runLegacySynthetic(arguments[1:], os.Stdin, os.Stdout), true
	}
	return 0, false
}

// syntheticResult es la única línea que escribe legacy-synthetic.
type syntheticResult struct {
	Event              string `json:"event"`
	Output             string `json:"output,omitempty"`
	PackageBytes       int64  `json:"package_bytes,omitempty"`
	ContentFingerprint string `json:"content_fingerprint,omitempty"`
	Message            string `json:"message,omitempty"`
}

func runLegacySynthetic(arguments []string, stdin io.Reader, stdout io.Writer) int {
	emit := func(result syntheticResult) {
		encoded, _ := json.Marshal(result)
		_, _ = stdout.Write(append(encoded, '\n'))
	}
	failed := func(message string) int {
		emit(syntheticResult{Event: "error", Message: message})
		return 1
	}
	if os.Getenv("ADEL_TEST_MODE") != "1" {
		return failed("legacy-synthetic solo corre en modo de pruebas (ADEL_TEST_MODE=1).")
	}
	flags := flag.NewFlagSet("legacy-synthetic", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "paquete sintético .adelmig que se crea")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *output == "" {
		return failed("Uso: ad-el-pos-server legacy-synthetic --output <paquete.adelmig> (frase de paso por la entrada estándar).")
	}
	line, err := bufio.NewReader(io.LimitReader(stdin, legacypkg.MaxPassphraseBytes+2)).ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return failed("No llegó la frase de paso por la entrada estándar.")
	}
	passphrase := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if err := legacypkg.ValidatePassphrase(passphrase); err != nil {
		return failed(err.Error())
	}
	file, err := legacypkg.CreateOutput(*output)
	if err != nil {
		return failed("No se pudo crear el paquete: " + err.Error())
	}
	result, err := legacysynth.Build(file, passphrase, legacypkg.WriterOptions{})
	if err != nil {
		file.Abort()
		return failed("No se pudo escribir el paquete sintético.")
	}
	if err := file.Commit(); err != nil {
		return failed("No se pudo guardar el paquete: " + err.Error())
	}
	emit(syntheticResult{
		Event:              "result",
		Output:             file.Path(),
		PackageBytes:       result.Bytes,
		ContentFingerprint: result.Manifest.ContentFingerprint,
	})
	return 0
}
