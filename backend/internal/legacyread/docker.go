package legacyread

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Command es una orden de docker: sus argumentos (sin el programa), variables de entorno adicionales (la contraseña
// generada solo viaja así, nunca en Args) y sus flujos.
type Command struct {
	Args   []string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Runner ejecuta órdenes de docker. ExecRunner es el real; las pruebas usan uno simulado.
type Runner interface {
	Run(ctx context.Context, command Command) error
}

// ExitError es una orden que terminó con un código distinto de cero.
type ExitError struct {
	Code int
}

func (exit *ExitError) Error() string {
	return "docker terminó con un código distinto de cero"
}

// ExecRunner ejecuta el docker del sistema con argumentos separados (nunca un intérprete de órdenes en este equipo).
type ExecRunner struct {
	Docker string
}

func (runner ExecRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, runner.Docker, command.Args...)
	process.Env = append(dockerEnvironment(runner.Docker), command.Env...)
	process.Stdin = command.Stdin
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	// Al cancelar, docker recibe la señal y, si no termina, se le da un plazo antes de abandonar sus flujos.
	process.WaitDelay = 10 * time.Second
	err := process.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && ctx.Err() == nil {
		return &ExitError{Code: exit.ExitCode()}
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// dockerEnvironment es el entorno de docker: el del proceso, con la carpeta de docker (y la de su destino, si es un
// enlace) al principio del PATH, porque docker busca ahí sus ayudantes de credenciales (docker-credential-desktop)
// y la aplicación de escritorio lanza este subcomando con un PATH mínimo. Nunca hereda una MSSQL_SA_PASSWORD ajena.
func dockerEnvironment(docker string) []string {
	directories := []string{filepath.Dir(docker)}
	if resolved, err := filepath.EvalSymlinks(docker); err == nil && filepath.Dir(resolved) != directories[0] {
		directories = append(directories, filepath.Dir(resolved))
	}
	environment := []string{}
	path := ""
	for _, variable := range os.Environ() {
		name, value, _ := strings.Cut(variable, "=")
		switch {
		case strings.EqualFold(name, "PATH"):
			path = value
		case name == "MSSQL_SA_PASSWORD" || name == "SQLCMDPASSWORD":
		default:
			environment = append(environment, variable)
		}
	}
	prefix := strings.Join(directories, string(os.PathListSeparator))
	if path != "" {
		prefix += string(os.PathListSeparator) + path
	}
	// Sin sugerencias ni mensajes de «What's next» de docker en la salida.
	return append(environment, "PATH="+prefix, "DOCKER_CLI_HINTS=false")
}

// Locator busca el docker del sistema. Sus campos permiten probar la búsqueda sin depender del equipo.
type Locator struct {
	GOOS     string
	Home     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
}

// SystemLocator usa el sistema operativo real.
func SystemLocator() Locator {
	home, _ := os.UserHomeDir()
	return Locator{GOOS: runtime.GOOS, Home: home, Getenv: os.Getenv, LookPath: exec.LookPath, Stat: os.Stat}
}

// Candidates son las rutas donde se busca docker después del PATH: la carpeta de usuario de Docker Desktop, Homebrew y
// la aplicación en macOS, y Program Files en Windows.
func (locator Locator) Candidates() []string {
	if locator.GOOS == "windows" {
		programFiles := locator.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		return []string{filepath.Join(programFiles, "Docker", "Docker", "resources", "bin", "docker.exe")}
	}
	candidates := []string{}
	if locator.Home != "" {
		candidates = append(candidates, filepath.Join(locator.Home, ".docker", "bin", "docker"))
	}
	return append(candidates,
		"/usr/local/bin/docker",
		"/opt/homebrew/bin/docker",
		"/Applications/Docker.app/Contents/Resources/bin/docker",
	)
}

// Find devuelve la ruta de docker o DOCKER_MISSING.
func (locator Locator) Find() (string, error) {
	if path, err := locator.LookPath("docker"); err == nil {
		if absolute, err := filepath.Abs(path); err == nil {
			return absolute, nil
		}
	}
	for _, candidate := range locator.Candidates() {
		info, err := locator.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if locator.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fail(CodeDockerMissing, "no se encontró docker", nil)
}

// boundedBuffer guarda como máximo limit bytes (la salida de una orden que solo se examina, nunca entera en memoria).
type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (bounded *boundedBuffer) Write(data []byte) (int, error) {
	room := bounded.limit - bounded.buffer.Len()
	if room > 0 {
		if len(data) > room {
			bounded.buffer.Write(data[:room])
		} else {
			bounded.buffer.Write(data)
		}
	}
	return len(data), nil
}

func (bounded *boundedBuffer) String() string {
	return bounded.buffer.String()
}
