package standalone

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Modalidad de la computadora (etapa 9G, §2.3) del lado del servidor, en la carpeta de datos:
//
//   - installation-mode.json: {"installation_mode": "server-terminal" | "server-only" | "terminal-only"}.
//     Lo escriben el instalador de macOS (postinstall) y la aplicación al cambiar la modalidad (Rust,
//     desktop/src-tauri/src/installation.rs, con la misma lista cerrada). La terminal guarda además su
//     propia modalidad en connection.json.
//   - service-enabled: archivo de habilitación (decisión D3). Mientras existe, launchd mantiene vivo el
//     servicio (KeepAlive → PathState del plist) y el servidor sigue sirviendo; si se quita, el servidor
//     deja de servir, detiene PostgreSQL y conserva los datos, y launchd ya no lo relanza. Crearlo o
//     quitarlo no pide la contraseña de administrador porque la carpeta es de la cuenta que instaló.
//
// En «Solo terminal» el servidor no inicia PostgreSQL ni Go aunque launchd lo lance al arrancar la
// computadora (RunAtLoad): termina enseguida sin tocar nada.
const (
	ModeFileName   = "installation-mode.json"
	EnableFileName = "service-enabled"

	ModeServerTerminal = "server-terminal"
	ModeServerOnly     = "server-only"
	ModeTerminalOnly   = "terminal-only"
)

// ValidMode indica si una modalidad está en la lista cerrada.
func ValidMode(mode string) bool {
	return mode == ModeServerTerminal || mode == ModeServerOnly || mode == ModeTerminalOnly
}

type modeFile struct {
	InstallationMode string `json:"installation_mode"`
}

// ReadMode devuelve la modalidad guardada; vacía (sin error) si todavía no hay archivo.
func ReadMode(layout Layout) (string, error) {
	file, err := os.Open(layout.ModeFile())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("no se pudo leer la modalidad de esta computadora")
	}
	defer file.Close()
	var stored modeFile
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil || !ValidMode(stored.InstallationMode) {
		return "", errors.New("la modalidad guardada de esta computadora no es válida; revisión manual necesaria")
	}
	return stored.InstallationMode, nil
}

// WriteMode guarda la modalidad (archivo privado, reemplazo atómico).
func WriteMode(layout Layout, mode string) error {
	if !ValidMode(mode) {
		return errors.New("modalidad desconocida")
	}
	body, err := json.Marshal(modeFile{InstallationMode: mode})
	if err != nil {
		return err
	}
	return writePrivateFile(layout.ModeFile(), append(body, '\n'))
}

// ServiceEnabled indica si existe el archivo de habilitación (un archivo normal, no un enlace).
func ServiceEnabled(layout Layout) bool {
	info, err := os.Lstat(layout.EnableFile())
	return err == nil && info.Mode().IsRegular()
}

// SetServiceEnabled crea o quita el archivo de habilitación.
func SetServiceEnabled(layout Layout, enabled bool) error {
	if !enabled {
		if err := os.Remove(layout.EnableFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("no se pudo deshabilitar el servicio del servidor")
		}
		return nil
	}
	if ServiceEnabled(layout) {
		return nil
	}
	if err := writePrivateFile(layout.EnableFile(), []byte("1\n")); err != nil {
		return errors.New("no se pudo habilitar el servicio del servidor")
	}
	return nil
}

// ServiceWanted indica si el servidor debe servir: archivo de habilitación presente y una modalidad con
// servidor. Sin archivo de modalidad (por ejemplo, una carpeta preparada a mano) cuenta como «Servidor y
// terminal». Devuelve también la modalidad efectiva.
func ServiceWanted(layout Layout) (bool, string, error) {
	mode, err := ReadMode(layout)
	if err != nil {
		return false, "", err
	}
	if mode == "" {
		mode = ModeServerTerminal
	}
	return ServiceEnabled(layout) && mode != ModeTerminalOnly, mode, nil
}

// writePrivateFile escribe un archivo 0600 con reemplazo atómico (archivo temporal y rename en la misma
// carpeta): quien lo lee nunca ve uno a medias.
func writePrivateFile(filename string, body []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(filename), "."+filepath.Base(filename)+".*")
	if err != nil {
		return errors.New("no se pudo escribir en la carpeta de datos")
	}
	name := temporary.Name()
	_, writeErr := temporary.Write(body)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(name)
		return errors.New("no se pudo escribir en la carpeta de datos")
	}
	if err := os.Chmod(name, 0600); err != nil {
		_ = os.Remove(name)
		return errors.New("no se pudo escribir en la carpeta de datos")
	}
	if err := os.Rename(name, filename); err != nil {
		_ = os.Remove(name)
		return errors.New("no se pudo escribir en la carpeta de datos")
	}
	return nil
}
