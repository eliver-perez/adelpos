package legacypkg

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Extension es la terminación de los paquetes de migración.
const Extension = ".adelmig"

// Errores al crear el archivo del paquete.
var (
	ErrOutputExists    = errors.New("ya existe un archivo con ese nombre")
	ErrOutputExtension = errors.New("el paquete debe terminar en .adelmig")
)

// Output es el archivo de un paquete mientras se escribe: un archivo temporal privado junto al destino que solo recibe
// el nombre definitivo al terminar (Commit) y nunca reemplaza un archivo existente. Abort lo borra.
type Output struct {
	*os.File
	temporary string
	final     string
	done      bool
}

// CreateOutput prepara el archivo del paquete. El destino no debe existir y debe terminar en .adelmig; el temporal
// («.<nombre>.<aleatorio>.partial» en la misma carpeta) nace 0600, sin seguir enlaces ni reemplazar nada (en Windows,
// con la ACL privada si el disco la admite: createPartial).
func CreateOutput(path string) (*Output, error) {
	if !strings.EqualFold(filepath.Ext(path), Extension) {
		return nil, ErrOutputExtension
	}
	final, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(final); err == nil {
		return nil, ErrOutputExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	temporary := filepath.Join(filepath.Dir(final), "."+filepath.Base(final)+"."+hex.EncodeToString(suffix)+".partial")
	file, err := createPartial(temporary)
	if err != nil {
		return nil, err
	}
	return &Output{File: file, temporary: temporary, final: final}, nil
}

// TemporaryPath es la ruta del temporal mientras se escribe (para comprobarlo antes de darle su nombre o para borrarlo
// si el proceso termina a la fuerza).
func (output *Output) TemporaryPath() string {
	return output.temporary
}

// partialPattern es el nombre de un temporal de CreateOutput.
var partialPattern = regexp.MustCompile(`^\..+\.adelmig\.[0-9a-f]{12}\.partial$`)

// RemoveStalePartial borra el temporal de un paquete que quedó a medias (el proceso terminó a la fuerza), solo si la ruta
// tiene la forma exacta de un temporal de CreateOutput y es un archivo regular (nunca un enlace ni otra cosa).
func RemoveStalePartial(path string) (bool, error) {
	if !filepath.IsAbs(path) || !partialPattern.MatchString(strings.ToLower(filepath.Base(path))) {
		return false, nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	return true, os.Remove(path)
}

// Path es la ruta definitiva.
func (output *Output) Path() string {
	return output.final
}

// Commit escribe a disco, cierra y da el nombre definitivo sin reemplazar un archivo que haya aparecido mientras tanto.
// Si falla, borra el temporal.
func (output *Output) Commit() error {
	if output.done {
		return errors.New("el paquete ya se cerró")
	}
	output.done = true
	syncErr := output.File.Sync()
	closeErr := output.File.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		_ = os.Remove(output.temporary)
		return err
	}
	if err := renameNoReplace(output.temporary, output.final); err != nil {
		_ = os.Remove(output.temporary)
		return err
	}
	return nil
}

// Abort cierra y borra el temporal (también después de un Commit fallido; no hace nada después de uno correcto).
func (output *Output) Abort() {
	if output.done {
		return
	}
	output.done = true
	_ = output.File.Close()
	_ = os.Remove(output.temporary)
}

// renameLinkOrCheck es el último recurso de renameNoReplace: un enlace duro (falla si el destino existe) y, en
// sistemas de archivos sin enlaces (una USB en exFAT), comprobar y renombrar.
func renameLinkOrCheck(temporary, final string) error {
	if err := os.Link(temporary, final); err == nil {
		_ = os.Remove(temporary)
		return nil
	} else if errors.Is(err, fs.ErrExist) {
		return ErrOutputExists
	}
	if _, err := os.Lstat(final); err == nil {
		return ErrOutputExists
	}
	return os.Rename(temporary, final)
}
