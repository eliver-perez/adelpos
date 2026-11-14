//go:build !windows

package standalone

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/config"
)

// Etapa 9G, fase A: rutas del servicio de macOS. Sin indicaciones, los datos van a la carpeta del
// servicio y PostgreSQL es el de Homebrew; --data-dir y ADEL_DATA_DIR cambian la carpeta (pruebas) y
// --pg-bin, postgres_bin de la configuración existente y ADEL_PG_BIN cambian PostgreSQL, en ese orden.
func TestNewLayoutUsesTheServiceFolderAndPostgreSQL(t *testing.T) {
	t.Setenv("ADEL_DATA_DIR", "")
	t.Setenv("ADEL_PG_BIN", "")
	layout, err := NewLayout("", "", "AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" && !samePath(layout.DataDir, "/Library/Application Support/AD-EL POS") {
		t.Fatalf("carpeta del servicio inesperada: %s", layout.DataDir)
	}
	if layout.PGBin != homebrewPostgresBin() || layout.Port != ClusterPort || layout.serverAddress() != ServerAddress {
		t.Fatalf("PostgreSQL, puerto o dirección inesperados: %+v", layout)
	}
	if !strings.HasSuffix(layout.tool("pg_ctl"), "/bin/pg_ctl") {
		t.Fatalf("herramienta inesperada: %s", layout.tool("pg_ctl"))
	}

	temporary := filepath.Join(t.TempDir(), "AD-EL POS")
	t.Setenv("ADEL_DATA_DIR", temporary)
	t.Setenv("ADEL_PG_BIN", "/opt/otro/postgresql/bin")
	layout, err = NewLayout("", "", "AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(layout.DataDir, temporary) || layout.PGBin != "/opt/otro/postgresql/bin" {
		t.Fatalf("las variables deben cambiar carpeta y PostgreSQL: %+v", layout)
	}
	// postgres_bin de una configuración existente gana a ADEL_PG_BIN; --pg-bin gana a ambos.
	if err := os.MkdirAll(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "server.json"), []byte(`{"postgres_bin": "/opt/configurado/bin"}`), 0600); err != nil {
		t.Fatal(err)
	}
	layout, _ = NewLayout("", "", "AD-EL POS")
	if layout.PGBin != "/opt/configurado/bin" {
		t.Fatalf("postgres_bin de la configuración: %s", layout.PGBin)
	}
	explicit := filepath.Join(t.TempDir(), "indicada")
	layout, _ = NewLayout(explicit, "/opt/indicado/bin", "AD-EL POS")
	if !samePath(layout.DataDir, explicit) || layout.PGBin != "/opt/indicado/bin" {
		t.Fatalf("las banderas ganan: %+v", layout)
	}
}

// El modo de pruebas rechaza la carpeta real del servicio aunque ADEL_DATA_DIR apunte a otra parte, y
// también escrita con otras mayúsculas o con «..»; acepta cualquier otra.
func TestRefuseRealDataDirOnMacOS(t *testing.T) {
	t.Setenv("ADEL_DATA_DIR", t.TempDir())
	real, err := RealDataDir("AD-EL POS")
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(real, SystemDataDir("AD-EL POS")) {
		t.Fatalf("la carpeta real no depende de ADEL_DATA_DIR: %s", real)
	}
	for _, path := range []string{real, real + "/", "/Library/Application Support/x/../AD-EL POS"} {
		if err := RefuseRealDataDir(Layout{DataDir: path}, "AD-EL POS"); err == nil {
			t.Fatalf("el modo de pruebas aceptó la carpeta real: %s", path)
		}
	}
	if runtime.GOOS == "darwin" {
		if err := RefuseRealDataDir(Layout{DataDir: strings.ToLower(real)}, "AD-EL POS"); err == nil {
			t.Fatal("el modo de pruebas aceptó la carpeta real escrita con otras mayúsculas")
		}
	}
	if err := RefuseRealDataDir(Layout{DataDir: filepath.Join(t.TempDir(), "AD-EL POS")}, "AD-EL POS"); err != nil {
		t.Fatalf("el modo de pruebas rechazó una carpeta temporal: %v", err)
	}
}

func TestLockIsExclusivePerDataFolderOnUnix(t *testing.T) {
	layout := Layout{DataDir: filepath.Join(t.TempDir(), "datos"), Port: ClusterPort}
	first, err := AcquireLock(layout)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(layout.DataDir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("la carpeta de datos se crea privada: %v %v", info.Mode(), err)
	}
	// flock pertenece al archivo abierto: otro intento, aun en el mismo proceso, queda bloqueado.
	if _, err := AcquireLock(layout); !errors.Is(err, ErrLocked) {
		t.Fatalf("un segundo servidor debe recibir ErrLocked: %v", err)
	}
	first.Release()
	again, err := AcquireLock(layout)
	if err != nil {
		t.Fatalf("al liberar, el bloqueo debe poder tomarse otra vez: %v", err)
	}
	again.Release()
	again.Release()
}

func TestInstallationModeAndEnableFile(t *testing.T) {
	layout := Layout{DataDir: t.TempDir()}
	mode, err := ReadMode(layout)
	if err != nil || mode != "" {
		t.Fatalf("sin archivo no hay modalidad: %q %v", mode, err)
	}
	// Sin modalidad cuenta como «Servidor y terminal», pero sin archivo de habilitación no sirve.
	wanted, effective, err := ServiceWanted(layout)
	if err != nil || wanted || effective != ModeServerTerminal {
		t.Fatalf("sin habilitación no se sirve: %v %q %v", wanted, effective, err)
	}
	if err := SetServiceEnabled(layout, true); err != nil {
		t.Fatal(err)
	}
	if err := SetServiceEnabled(layout, true); err != nil {
		t.Fatal("habilitar dos veces no es un error")
	}
	if info, err := os.Stat(layout.EnableFile()); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("el archivo de habilitación es privado: %v", err)
	}
	for _, candidate := range []string{ModeServerTerminal, ModeServerOnly, ModeTerminalOnly} {
		if err := WriteMode(layout, candidate); err != nil {
			t.Fatal(err)
		}
		mode, err := ReadMode(layout)
		if err != nil || mode != candidate {
			t.Fatalf("modalidad guardada: %q %v", mode, err)
		}
		wanted, _, _ := ServiceWanted(layout)
		if wanted != (candidate != ModeTerminalOnly) {
			t.Fatalf("«%s» con habilitación: sirve=%v", candidate, wanted)
		}
	}
	if info, err := os.Stat(layout.ModeFile()); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("la modalidad es privada: %v", err)
	}
	if err := WriteMode(layout, "servidor"); err == nil {
		t.Fatal("una modalidad fuera de la lista cerrada se rechaza")
	}
	// Lo que escribe Rust (installation.rs) y el instalador tiene la misma forma.
	for _, text := range []string{`{"installation_mode":"server-only"}`, "{\"installation_mode\": \"server-only\"}\n"} {
		if err := os.WriteFile(layout.ModeFile(), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if mode, err := ReadMode(layout); err != nil || mode != ModeServerOnly {
			t.Fatalf("formato compartido: %q %v", mode, err)
		}
	}
	for _, text := range []string{`{"installation_mode":"todo"}`, `{"installation_mode":"server-only","extra":1}`, `nada`} {
		if err := os.WriteFile(layout.ModeFile(), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadMode(layout); err == nil {
			t.Fatalf("modalidad inválida aceptada: %s", text)
		}
		if _, _, err := ServiceWanted(layout); err == nil {
			t.Fatalf("una modalidad ilegible no se adivina: %s", text)
		}
	}
	if err := SetServiceEnabled(layout, false); err != nil || ServiceEnabled(layout) {
		t.Fatalf("deshabilitar quita el archivo: %v", err)
	}
	if err := SetServiceEnabled(layout, false); err != nil {
		t.Fatal("deshabilitar dos veces no es un error")
	}
	// Un enlace simbólico no habilita el servicio.
	target := filepath.Join(t.TempDir(), "otro")
	if err := os.WriteFile(target, []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, layout.EnableFile()); err != nil {
		t.Fatal(err)
	}
	if ServiceEnabled(layout) {
		t.Fatal("un enlace no cuenta como archivo de habilitación")
	}
}

func TestEnsureConfigUsesTheTestAddressOnlyForANewFolder(t *testing.T) {
	layout := Layout{DataDir: t.TempDir(), Port: 55511, Listen: "127.0.0.1:8192"}
	if err := ensureConfig(layout, "adel_pos"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(layout.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != "127.0.0.1:8192" || cfg.Database.Port != 55511 || cfg.AllowLAN || cfg.InstallationMode != "server-terminal" {
		t.Fatalf("configuración inesperada: %+v", cfg)
	}
	if info, err := os.Stat(layout.ConfigFile()); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("la configuración es privada: %v", err)
	}
	layout.Listen = "127.0.0.1:8193"
	if err := ensureConfig(layout, "adel_pos"); err != nil {
		t.Fatal(err)
	}
	again, _ := config.Load(layout.ConfigFile())
	if again.ListenAddress != "127.0.0.1:8192" || again.Database.Password != cfg.Database.Password {
		t.Fatal("una configuración existente nunca se reescribe")
	}
}

func TestCheckBackupFileRejectsFilesOutsideBackupsOnUnix(t *testing.T) {
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
	for _, filename := range []string{outside, filepath.Join(layout.Backups(), "no-existe.dump"), layout.Backups(), filepath.Join(layout.Backups(), "..", "server.json")} {
		if _, err := checkBackupFile(layout, filename); err == nil {
			t.Fatalf("se aceptó %s", filename)
		}
	}
}

// Un postmaster.pid huérfano (su proceso ya no existe o es otro programa) no cuenta como PostgreSQL vivo.
func TestPostmasterAliveRecognizesOnlyPostgreSQL(t *testing.T) {
	if postmasterAlive(0) || postmasterAlive(-3) {
		t.Fatal("un PID inválido no es PostgreSQL")
	}
	if postmasterAlive(os.Getpid()) {
		t.Fatal("el proceso de la prueba no es PostgreSQL")
	}
	if postmasterAlive(999999) {
		t.Fatal("un proceso inexistente no es PostgreSQL")
	}
	if processName(os.Getpid()) == "" {
		t.Fatal("el nombre del propio proceso debe poder consultarse")
	}
}

func TestHasDataDetectsAnExistingInstallation(t *testing.T) {
	layout := Layout{DataDir: t.TempDir()}
	if err := os.MkdirAll(layout.Logs(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteMode(layout, ModeTerminalOnly); err != nil {
		t.Fatal(err)
	}
	if HasData(layout) {
		t.Fatal("bitácoras y modalidad no son datos del restaurante")
	}
	for _, path := range []string{layout.Cluster(), layout.ConfigFile(), layout.Marker(), layout.AdoptionMarker()} {
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		if !HasData(layout) {
			t.Fatalf("%s indica una instalación", filepath.Base(path))
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFreePortSkipsBusyPorts(t *testing.T) {
	port, err := freePort(55514)
	if err != nil || port <= 55514 || port > 55554 {
		t.Fatalf("puerto temporal inesperado: %d %v", port, err)
	}
}
