//go:build windows

package standalone

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"local/ad-el-pos/server/internal/config"
)

func TestNewLayoutUsesLocalAppDataAndProgramFolder(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	layout, err := NewLayout("", "", "AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(layout.DataDir, filepath.Join(base, "AD-EL POS")) {
		t.Fatalf("carpeta de datos inesperada: %s", layout.DataDir)
	}
	executable, _ := os.Executable()
	if !samePath(layout.PGBin, filepath.Join(filepath.Dir(executable), "postgresql", "bin")) {
		t.Fatalf("carpeta de PostgreSQL inesperada: %s", layout.PGBin)
	}
	if layout.Port != ClusterPort || !strings.HasSuffix(layout.tool("pg_ctl"), `\pg_ctl.exe`) {
		t.Fatalf("puerto o herramienta inesperados: %d %s", layout.Port, layout.tool("pg_ctl"))
	}
	// Los datos nunca viven junto al programa (0-B de la 9E).
	if samePath(layout.DataDir, filepath.Dir(executable)) {
		t.Fatal("la carpeta de datos no debe ser la del programa")
	}
}

func TestSamePathAcceptsShortAndLongForms(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "Datos José Ñandú con espacios")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	long := canonicalPath(directory)
	source, _ := windows.UTF16PtrFromString(long)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size, err := windows.GetShortPathName(source, &buffer[0], uint32(len(buffer)))
	if err != nil || size == 0 {
		t.Skip("el volumen no genera nombres cortos")
	}
	short := windows.UTF16ToString(buffer[:size])
	if !samePath(short, long) || !samePath(strings.ToUpper(long), long) {
		t.Fatalf("formas de la misma carpeta no coinciden: %s y %s", short, long)
	}
	// Una ruta que aún no existe se resuelve desde su parte existente.
	if !samePath(filepath.Join(short, "nueva", "carpeta"), filepath.Join(long, "nueva", "carpeta")) {
		t.Fatal("una ruta inexistente bajo una carpeta corta no coincide con la larga")
	}
	if samePath(long, long+"-otra") {
		t.Fatal("dos carpetas distintas no deben coincidir")
	}
}

func TestServerConfigIsValidLoopbackDevelopment(t *testing.T) {
	cfg := serverConfig("adel_pos", "secreto-de-prueba", ClusterPort)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != "127.0.0.1:8181" || cfg.AllowLAN || cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 55432 {
		t.Fatalf("la instalación solo debe escuchar en loopback: %+v", cfg)
	}
	if cfg.Environment != "development" || cfg.LicenseMode != "development" {
		t.Fatal("la instalación de prueba no usa licencias comerciales")
	}
}

func TestEnsureConfigWritesOnceAndKeepsExisting(t *testing.T) {
	layout := Layout{DataDir: t.TempDir(), Port: ClusterPort}
	if err := ensureConfig(layout, "adel_pos"); err != nil {
		t.Fatal(err)
	}
	first, err := config.Load(layout.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Database.Password) != 48 {
		t.Fatal("la contraseña del rol debe ser aleatoria de 24 bytes")
	}
	if err := ensureConfig(layout, "adel_pos"); err != nil {
		t.Fatal(err)
	}
	second, _ := config.Load(layout.ConfigFile())
	if second.Database.Password != first.Database.Password {
		t.Fatal("un segundo arranque no debe reemplazar la configuración")
	}
}

func TestCheckBackupFileRejectsFilesOutsideBackups(t *testing.T) {
	layout := Layout{DataDir: t.TempDir(), Port: ClusterPort}
	if err := os.MkdirAll(layout.Backups(), 0700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(layout.Backups(), "adel_pos-prueba.dump")
	outside := filepath.Join(layout.DataDir, "server.json")
	for _, filename := range []string{inside, outside} {
		if err := os.WriteFile(filename, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := checkBackupFile(layout, inside); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{outside, filepath.Join(layout.Backups(), "no-existe.dump"), layout.Backups()} {
		if _, err := checkBackupFile(layout, filename); err == nil {
			t.Fatalf("se aceptó %s", filename)
		}
	}
}

func TestLockIsExclusivePerDataFolder(t *testing.T) {
	layout := Layout{DataDir: t.TempDir(), Port: ClusterPort}
	first, err := AcquireLock(layout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(layout); !errors.Is(err, ErrLocked) {
		t.Fatalf("un segundo servidor debe recibir ErrLocked: %v", err)
	}
	first.Release()
	again, err := AcquireLock(layout)
	if err != nil {
		t.Fatalf("al liberar, el bloqueo debe poder tomarse otra vez: %v", err)
	}
	again.Release()
}

func TestProcessImageOfMissingProcessIsEmpty(t *testing.T) {
	if processImage(0) != "" || processImage(-5) != "" {
		t.Fatal("un PID inválido no tiene ejecutable")
	}
	own := processImage(os.Getpid())
	executable, _ := os.Executable()
	if !samePath(own, executable) {
		t.Fatalf("el ejecutable propio no coincide: %s", own)
	}
}
