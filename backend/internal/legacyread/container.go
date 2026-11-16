package legacyread

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// Imagen y contenedor del lector.
const (
	// Image es la imagen oficial de Microsoft (SQL Server 2022): restaura respaldos de SQL Server 2012.
	Image    = "mcr.microsoft.com/mssql/server:2022-latest"
	Platform = "linux/amd64"
	// ReaderLabel marca los contenedores y volúmenes del lector: los que queden de una ejecución interrumpida se
	// eliminan al empezar (y solo esos).
	ReaderLabel  = "local.adel.pos.legacy-reader=1"
	namePrefix   = "adel-legacy-"
	backupTarget = "/backup/respaldo.bak"
	dataTarget   = "/var/opt/mssql"
	databaseName = "adel_legacy"
)

// Plazos de las órdenes de docker.
const (
	shortTimeout   = 60 * time.Second
	runTimeout     = 3 * time.Minute
	cleanupTimeout = 2 * time.Minute
	probeTimeout   = 30 * time.Second
	pullTimeout    = 2 * time.Hour
)

// imageInfo es la imagen que se usó (va al manifiesto).
type imageInfo struct {
	ID     string
	Digest string
}

// container es el SQL Server temporal de una preparación.
type container struct {
	runner   Runner
	events   Reporter
	name     string
	volume   string
	copyDir  string
	tempDir  string
	sleep    func(context.Context, time.Duration) error
	created  bool
	readyFor time.Duration
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic("legacyread: sin números aleatorios")
	}
	return hex.EncodeToString(buffer)
}

// newPassword es la contraseña de sa: aleatoria, con mayúscula, minúscula, número y símbolo (la política de SQL
// Server). Solo existe en el entorno del proceso docker run y del contenedor.
func newPassword() string {
	return "Aa1!" + randomHex(24)
}

func newContainer(
	runner Runner,
	events Reporter,
	tempDir string,
	sleep func(context.Context, time.Duration) error,
	ready time.Duration,
) *container {
	name := namePrefix + randomHex(6)
	return &container{
		runner:   runner,
		events:   events,
		name:     name,
		volume:   name + "-data",
		tempDir:  tempDir,
		sleep:    sleep,
		readyFor: ready,
	}
}

// run ejecuta una orden corta y devuelve sus salidas (acotadas).
func (box *container) run(ctx context.Context, timeout time.Duration, arguments ...string) (string, string, error) {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := newBoundedBuffer(1 << 20)
	stderr := newBoundedBuffer(64 << 10)
	err := box.runner.Run(commandContext, Command{Args: arguments, Stdout: stdout, Stderr: stderr})
	return stdout.String(), stderr.String(), err
}

// checkDocker comprueba que Docker esté en marcha y use contenedores de Linux.
func (box *container) checkDocker(ctx context.Context) error {
	stdout, _, err := box.run(ctx, shortTimeout, "info", "--format", "{{.OSType}}")
	if ctx.Err() != nil {
		return fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return fail(CodeDockerNotRunning, "docker info falló", err)
	}
	if strings.TrimSpace(stdout) != "linux" {
		return failf(CodeDockerUnsupported, nil, "Docker usa contenedores %q", strings.TrimSpace(stdout))
	}
	return nil
}

// ensureImage usa la imagen si ya está (para linux/amd64) o la descarga con avance.
func (box *container) ensureImage(ctx context.Context) (imageInfo, error) {
	info, ok := box.inspectImage(ctx)
	if ok {
		return info, nil
	}
	pullContext, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	layers := map[string]bool{}
	complete := map[string]bool{}
	output := newLineWriter(func(line string) {
		layer, status, found := strings.Cut(line, ": ")
		if !found || strings.Contains(layer, " ") {
			return
		}
		layers[layer] = true
		if strings.HasPrefix(status, "Pull complete") || strings.HasPrefix(status, "Already exists") {
			complete[layer] = true
		}
		box.events.Emit(ProgressEvent{
			Event:   "progress",
			Step:    StepImage,
			Message: "Descargando el lector (solo la primera vez; puede tardar varios minutos)",
			Current: int64(len(complete)),
			Total:   int64(len(layers)),
		})
	})
	stderr := newBoundedBuffer(64 << 10)
	err := box.runner.Run(pullContext, Command{Args: []string{"pull", "--platform", Platform, Image}, Stdout: output, Stderr: stderr})
	if ctx.Err() != nil {
		return imageInfo{}, fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return imageInfo{}, fail(CodeImageUnavailable, "docker pull falló", err)
	}
	info, ok = box.inspectImage(ctx)
	if !ok {
		return imageInfo{}, fail(CodeImageUnavailable, "la imagen no quedó disponible", nil)
	}
	return info, nil
}

func (box *container) inspectImage(ctx context.Context) (imageInfo, bool) {
	stdout, _, err := box.run(ctx, shortTimeout, "image", "inspect", "--format", "{{.Id}}|{{.Architecture}}|{{json .RepoDigests}}", Image)
	if err != nil {
		return imageInfo{}, false
	}
	parts := strings.SplitN(strings.TrimSpace(stdout), "|", 3)
	if len(parts) != 3 || parts[1] != "amd64" || parts[0] == "" {
		return imageInfo{}, false
	}
	info := imageInfo{ID: parts[0]}
	var digests []string
	if json.Unmarshal([]byte(parts[2]), &digests) == nil {
		for _, digest := range digests {
			if strings.HasPrefix(digest, "mcr.microsoft.com/mssql/server@") {
				info.Digest = digest
			}
		}
	}
	return info, true
}

// removeLeftovers elimina los contenedores y volúmenes del lector que haya dejado una ejecución interrumpida (solo los
// que llevan la etiqueta del lector).
func (box *container) removeLeftovers(ctx context.Context) error {
	stdout, _, err := box.run(ctx, shortTimeout, "ps", "-a", "-q", "--filter", "label="+ReaderLabel)
	if err != nil {
		return fail(CodeDockerNotRunning, "docker ps falló", err)
	}
	if ids := strings.Fields(stdout); len(ids) > 0 {
		if _, _, err := box.run(ctx, runTimeout, append([]string{"rm", "-f", "-v"}, ids...)...); err != nil {
			return fail(CodeReaderStartFailed, "no se pudo eliminar un lector anterior", err)
		}
	}
	stdout, _, err = box.run(ctx, shortTimeout, "volume", "ls", "-q", "--filter", "label="+ReaderLabel)
	if err != nil {
		return fail(CodeDockerNotRunning, "docker volume ls falló", err)
	}
	if names := strings.Fields(stdout); len(names) > 0 {
		if _, _, err := box.run(ctx, runTimeout, append([]string{"volume", "rm", "-f"}, names...)...); err != nil {
			return fail(CodeReaderStartFailed, "no se pudo eliminar un volumen anterior", err)
		}
	}
	return nil
}

// mountOption arma el valor de --mount (CSV: una ruta con comas o comillas va entre comillas).
func mountOption(fields ...string) string {
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	_ = writer.Write(fields)
	writer.Flush()
	return strings.TrimRight(builder.String(), "\r\n")
}

// runArguments son los argumentos exactos de docker run (las pruebas los comprueban): sin red, con la etiqueta del
// lector, el respaldo de solo lectura, el volumen propio y la contraseña solo por el entorno (-e sin valor).
func (box *container) runArguments(image, backup string) []string {
	return []string{
		"run", "-d",
		"--name", box.name,
		"--label", ReaderLabel,
		"--platform", Platform,
		"--pull", "never",
		"--network", "none",
		"-e", "ACCEPT_EULA=Y",
		"-e", "MSSQL_PID=Express",
		"-e", "MSSQL_SA_PASSWORD",
		"--mount", mountOption("type=bind", "source="+backup, "target="+backupTarget, "readonly"),
		"--mount", mountOption("type=volume", "source="+box.volume, "target="+dataTarget),
		image,
	}
}

// start crea el volumen y el contenedor. Si Docker no puede montar la carpeta del respaldo (por ejemplo una USB fuera
// de las carpetas compartidas de Docker Desktop), copia el respaldo a una carpeta temporal privada y monta la copia.
func (box *container) start(ctx context.Context, image, backup string, size int64) error {
	if _, _, err := box.run(ctx, shortTimeout, "volume", "create", "--label", ReaderLabel, box.volume); err != nil {
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		return fail(CodeReaderStartFailed, "docker volume create falló", err)
	}
	box.created = true
	stderr, err := box.runContainer(ctx, image, backup)
	if err == nil && !box.backupVisible(ctx, size) {
		err = errors.New("el respaldo no se ve completo dentro del contenedor")
		stderr = "mount"
		box.removeContainer(ctx)
	}
	if err != nil && ctx.Err() == nil && strings.Contains(strings.ToLower(stderr), "mount") {
		copied, copyErr := box.copyBackup(ctx, backup)
		if copyErr != nil {
			return copyErr
		}
		box.removeContainer(ctx)
		stderr, err = box.runContainer(ctx, image, copied)
		if err == nil && !box.backupVisible(ctx, size) {
			return fail(CodeReaderStartFailed, "la copia del respaldo no se ve dentro del contenedor", nil)
		}
	}
	if ctx.Err() != nil {
		return fail(CodeCancelled, "", ctx.Err())
	}
	if err != nil {
		return fail(CodeReaderStartFailed, "docker run falló", err)
	}
	return nil
}

func (box *container) runContainer(ctx context.Context, image, backup string) (string, error) {
	commandContext, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	stderr := newBoundedBuffer(64 << 10)
	err := box.runner.Run(commandContext, Command{
		Args:   box.runArguments(image, backup),
		Env:    []string{"MSSQL_SA_PASSWORD=" + newPassword()},
		Stdout: io.Discard,
		Stderr: stderr,
	})
	return stderr.String(), err
}

// backupVisible comprueba que el respaldo montado tiene, dentro del contenedor, el mismo tamaño que el archivo.
func (box *container) backupVisible(ctx context.Context, size int64) bool {
	stdout, _, err := box.run(ctx, shortTimeout, "exec", box.name, "stat", "-c", "%s", backupTarget)
	if err != nil {
		return false
	}
	seen, err := strconv.ParseInt(strings.TrimSpace(stdout), 10, 64)
	return err == nil && seen == size
}

// copyBackup copia el respaldo a una carpeta temporal privada (0700; en Windows con la ACL privada). Se borra al
// terminar.
func (box *container) copyBackup(ctx context.Context, backup string) (string, error) {
	directory, err := os.MkdirTemp(box.tempDir, "adel-respaldo-")
	if err != nil {
		return "", fail(CodeReaderStartFailed, "no se pudo crear la carpeta para la copia del respaldo", err)
	}
	box.copyDir = directory
	if err := fileacl.Protect(directory); err != nil {
		return "", fail(CodeReaderStartFailed, "no se pudo proteger la carpeta de la copia", err)
	}
	target := filepath.Join(directory, "respaldo.bak")
	source, err := os.Open(backup)
	if err != nil {
		return "", fail(CodeBackupUnreadable, "", err)
	}
	defer source.Close()
	output, err := fileacl.CreatePrivate(target)
	if err != nil {
		return "", fail(CodeReaderStartFailed, "no se pudo crear la copia del respaldo", err)
	}
	_, copyErr := io.Copy(output, contextReader{ctx: ctx, source: source})
	closeErr := output.Close()
	if ctx.Err() != nil {
		return "", fail(CodeCancelled, "", ctx.Err())
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", fail(CodeWriteFailed, "no se pudo copiar el respaldo a la carpeta temporal", err)
	}
	return target, nil
}

// contextReader corta una copia larga al cancelar.
type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.source.Read(buffer)
}

// waitReady espera a que SQL Server acepte consultas. Si el contenedor se detiene, distingue la falta de memoria.
func (box *container) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(box.readyFor)
	for {
		probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
		err := box.runner.Run(probeContext, Command{
			Args:   execArguments(box.name, "", jsonArguments),
			Stdin:  strings.NewReader("SET NOCOUNT ON; SELECT 1;"),
			Stdout: io.Discard,
			Stderr: io.Discard,
		})
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fail(CodeCancelled, "", ctx.Err())
		}
		if !box.running(ctx) {
			return box.startFailure(ctx)
		}
		if time.Now().After(deadline) {
			return fail(CodeReaderStartFailed, "SQL Server no respondió a tiempo", nil)
		}
		if err := box.sleep(ctx, 2*time.Second); err != nil {
			return fail(CodeCancelled, "", err)
		}
	}
}

func (box *container) running(ctx context.Context) bool {
	stdout, _, err := box.run(ctx, shortTimeout, "inspect", "--format", "{{.State.Running}}", box.name)
	return err == nil && strings.TrimSpace(stdout) == "true"
}

// startFailure examina la bitácora del contenedor detenido (solo para reconocer la falta de memoria; nunca se muestra).
func (box *container) startFailure(ctx context.Context) error {
	stdout, stderr, _ := box.run(ctx, shortTimeout, "logs", "--tail", "200", box.name)
	if strings.Contains(stdout+stderr, "2000 megabytes") {
		return fail(CodeReaderMemory, "SQL Server pide al menos 2 GB de memoria", nil)
	}
	return fail(CodeReaderStartFailed, "el contenedor se detuvo al arrancar", nil)
}

func (box *container) removeContainer(ctx context.Context) {
	_, _, _ = box.run(ctx, cleanupTimeout, "rm", "-f", "-v", box.name)
}

// cleanup elimina el contenedor, su volumen y la copia del respaldo. Usa su propio plazo: también corre al cancelar.
func (box *container) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var failures []error
	if box.created {
		_, stderr, err := box.run(ctx, cleanupTimeout, "rm", "-f", "-v", box.name)
		if err != nil && !strings.Contains(stderr, "No such container") {
			failures = append(failures, errors.New("no se pudo eliminar el contenedor"))
		}
		_, stderr, err = box.run(ctx, cleanupTimeout, "volume", "rm", "-f", box.volume)
		if err != nil && !strings.Contains(stderr, "no such volume") {
			failures = append(failures, errors.New("no se pudo eliminar el volumen"))
		}
		box.created = false
	}
	if box.copyDir != "" {
		if err := os.RemoveAll(box.copyDir); err != nil {
			failures = append(failures, errors.New("no se pudo borrar la copia del respaldo"))
		}
		box.copyDir = ""
	}
	return errors.Join(failures...)
}
