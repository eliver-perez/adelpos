//go:build windows

package standalone

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/fileacl"
)

// Cluster administra el clúster de PostgreSQL de la carpeta de datos.
type Cluster struct {
	layout Layout
	logger *slog.Logger
}

func NewCluster(layout Layout, logger *slog.Logger) *Cluster {
	return &Cluster{layout: layout, logger: logger}
}

// owner es el marcador de propiedad del clúster, igual que .local/cluster-owner.json en macOS: sin él (o si
// no coincide) nunca se toca una carpeta postgres que no se creó aquí.
type owner struct {
	DataDirectory string `json:"data_directory"`
	Port          int    `json:"port"`
}

// prepareFolders crea la carpeta de datos con su ACL privada y las de bitácoras y respaldos.
func (cluster *Cluster) prepareFolders() error {
	if err := os.MkdirAll(cluster.layout.DataDir, 0700); err != nil {
		return errors.New("no se pudo crear la carpeta de datos de AD-EL POS")
	}
	// La ACL privada de la carpeta se hereda a todo lo que se cree dentro (secretos, clúster, respaldos).
	if err := fileacl.Protect(cluster.layout.DataDir); err != nil {
		return err
	}
	for _, folder := range []string{cluster.layout.Logs(), cluster.layout.Backups()} {
		if err := os.MkdirAll(folder, 0700); err != nil {
			return errors.New("no se pudo crear la carpeta " + filepath.Base(folder))
		}
	}
	return nil
}

func (cluster *Cluster) checkOwner() (bool, error) {
	text, err := os.ReadFile(cluster.layout.Marker())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("no se pudo leer el marcador del clúster")
	}
	var marker owner
	if err := json.Unmarshal(text, &marker); err != nil || !samePath(marker.DataDirectory, cluster.layout.Cluster()) || marker.Port != cluster.layout.Port {
		return false, errors.New("el marcador no coincide con este clúster; revisión manual necesaria")
	}
	return true, nil
}

// Ensure crea el clúster la primera vez. Una inicialización que no terminó (sin PG_VERSION) se borra y se
// repite solo si el marcador demuestra que la carpeta es de esta instalación: todavía no puede contener
// datos de operación.
func (cluster *Cluster) Ensure(ctx context.Context) error {
	if err := cluster.prepareFolders(); err != nil {
		return err
	}
	if _, err := os.Stat(cluster.layout.tool("initdb")); err != nil {
		return errors.New("faltan los binarios de PostgreSQL de la instalación; reinstala AD-EL POS")
	}
	// Sin nombres cortos de Windows, una carpeta del programa con acentos impide crear la base (ver tool).
	if !isASCII(cluster.layout.tool("initdb")) {
		return errors.New("la carpeta donde está instalado AD-EL POS tiene acentos u otros caracteres que la base de datos no admite al crearse; instálalo en una carpeta sin ellos")
	}
	owned, err := cluster.checkOwner()
	if err != nil {
		return err
	}
	_, clusterErr := os.Stat(cluster.layout.Cluster())
	clusterExists := clusterErr == nil
	if clusterExists && !owned {
		return errors.New("existe una carpeta postgres que no creó AD-EL POS; no se modificará")
	}
	if _, err := os.Stat(filepath.Join(cluster.layout.Cluster(), "PG_VERSION")); err == nil {
		return nil
	}
	if clusterExists {
		cluster.logger.Warn("cluster_incomplete_removed")
		if err := os.RemoveAll(cluster.layout.Cluster()); err != nil {
			return errors.New("no se pudo limpiar una inicialización incompleta del clúster")
		}
	}
	if _, err := secretFile(cluster.layout.AdminPassword()); err != nil {
		return err
	}
	marker, _ := json.Marshal(owner{DataDirectory: cluster.layout.Cluster(), Port: cluster.layout.Port})
	if err := os.WriteFile(cluster.layout.Marker(), marker, 0600); err != nil {
		return errors.New("no se pudo escribir el marcador del clúster")
	}
	started := time.Now()
	output, err := runCaptured(ctx, cluster.layout.tool("initdb"), []string{
		"-D", cluster.layout.Cluster(),
		"--username=" + superuser,
		"--auth=scram-sha-256",
		"--encoding=UTF8",
		"--locale=C",
		"--pwfile=" + cluster.layout.AdminPassword(),
	}, []string{"LC_MESSAGES=C"})
	if err != nil {
		cluster.logger.Error("initdb_failed", "output", tail(output))
		return errors.New("no se pudo crear la base de datos de AD-EL POS")
	}
	// Igual que en macOS: solo TCP de loopback, UTC y SCRAM; en Windows sin socket de Unix.
	settings := fmt.Sprintf("\nlisten_addresses = '127.0.0.1'\nport = %d\nunix_socket_directories = ''\n"+
		"timezone = 'UTC'\npassword_encryption = 'scram-sha-256'\n", cluster.layout.Port)
	file, err := os.OpenFile(filepath.Join(cluster.layout.Cluster(), "postgresql.conf"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return errors.New("no se pudo configurar el clúster")
	}
	_, writeErr := file.WriteString(settings)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("no se pudo configurar el clúster")
	}
	cluster.logger.Info("cluster_initialized", "duration_ms", time.Since(started).Milliseconds())
	return nil
}

// Running indica si el postmaster de este clúster está activo, según postmaster.pid y el ejecutable del
// proceso: un PID reutilizado por otro programa no cuenta.
func (cluster *Cluster) Running() bool {
	pid, ok := cluster.postmasterPID()
	if !ok {
		return false
	}
	return samePath(processImage(pid), cluster.layout.tool("postgres"))
}

func (cluster *Cluster) pidFile() string {
	return filepath.Join(cluster.layout.Cluster(), "postmaster.pid")
}

func (cluster *Cluster) postmasterPID() (int, bool) {
	text, err := os.ReadFile(cluster.pidFile())
	if err != nil {
		return 0, false
	}
	first, _, _ := strings.Cut(string(text), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// Start arranca el clúster. Si ya está activo (por ejemplo, porque la aplicación se cerró mal y PostgreSQL
// siguió corriendo) se adopta. Un postmaster.pid huérfano, de un PostgreSQL que ya no existe, se borra
// antes de arrancar: PostgreSQL se negaría si el PID se reutilizó en otro proceso.
func (cluster *Cluster) Start(ctx context.Context) error {
	if cluster.Running() {
		cluster.logger.Info("cluster_adopted")
		return nil
	}
	if _, err := os.Stat(cluster.pidFile()); err == nil {
		cluster.logger.Warn("stale_postmaster_pid_removed")
		if err := os.Remove(cluster.pidFile()); err != nil {
			return errors.New("no se pudo limpiar el estado de un cierre anterior de PostgreSQL")
		}
	}
	started := time.Now()
	err := runLogged(ctx, cluster.layout.ControlLog(), cluster.layout.tool("pg_ctl"), []string{
		"start", "-w", "-t", "120",
		"-D", cluster.layout.Cluster(),
		"-l", cluster.layout.PostgresLog(),
	})
	if err != nil {
		return errors.New("PostgreSQL no pudo iniciar; revisa logs\\postgres.log en la carpeta de datos")
	}
	cluster.logger.Info("cluster_started", "duration_ms", time.Since(started).Milliseconds())
	return nil
}

// Stop detiene el clúster en orden (modo fast: cierra las conexiones y escribe un punto de control).
func (cluster *Cluster) Stop(ctx context.Context) error {
	if !cluster.Running() {
		return nil
	}
	err := runLogged(ctx, cluster.layout.ControlLog(), cluster.layout.tool("pg_ctl"), []string{
		"stop", "-w", "-t", "60", "-m", "fast",
		"-D", cluster.layout.Cluster(),
	})
	if err != nil {
		cluster.logger.Error("cluster_stop_failed")
		return errors.New("PostgreSQL no se detuvo a tiempo")
	}
	cluster.logger.Info("cluster_stopped")
	return nil
}

// secretFile devuelve el secreto de un archivo privado y lo crea con 24 bytes aleatorios si falta.
func secretFile(filename string) (string, error) {
	if text, err := os.ReadFile(filename); err == nil {
		value := strings.TrimSpace(string(text))
		if value == "" {
			return "", errors.New("un archivo de secreto está vacío; revisión manual necesaria")
		}
		return value, nil
	}
	value, err := randomHex(24)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filename, []byte(value), 0600); err != nil {
		return "", errors.New("no se pudo guardar un secreto local")
	}
	if err := fileacl.Protect(filename); err != nil {
		return "", err
	}
	return value, nil
}

func randomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("no se pudo generar un secreto aleatorio")
	}
	return hex.EncodeToString(bytes), nil
}

// tail son las últimas líneas de la salida de un programa, para la bitácora.
func tail(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 2000 {
		text = text[len(text)-2000:]
	}
	return text
}
