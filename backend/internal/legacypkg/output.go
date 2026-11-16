package legacypkg

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"local/ad-el-pos/server/internal/fileacl"
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
// con la ACL privada).
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
	file, err := fileacl.CreatePrivate(temporary)
	if err != nil {
		return nil, err
	}
	return &Output{File: file, temporary: temporary, final: final}, nil
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
