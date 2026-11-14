package backupschedule

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Nombres de los archivos que crean los respaldos programados: <base>-<AAAAMMDDTHHMMSSZ>-<6 hex>-<etiqueta>.dump, el
// mismo formato de los demás respaldos con una etiqueta propia. Mientras se crea y se verifica, el archivo lleva además
// la terminación .partial: un respaldo a medias nunca tiene el nombre definitivo, así que la retención nunca lo cuenta.
const (
	automaticLabel = "automatico"
	manualLabel    = "manual"
	partialSuffix  = ".partial"
)

// Kinds de respaldo.
const (
	KindAutomatic = "automatic"
	KindManual    = "manual"
)

// Motivos por los que la carpeta elegida no se usó (fallback_reason) o no se acepta al elegirla.
const (
	ReasonMissing      = "missing"
	ReasonNotDirectory = "not_directory"
	ReasonNotWritable  = "not_writable"
	// Solo al elegirla.
	ReasonRelative     = "relative"
	ReasonPostgresData = "postgres_data"
	ReasonInvalid      = "invalid"
)

// maxDirectoryLength es el largo máximo de la ruta de la carpeta (la misma restricción de backup_settings).
const maxDirectoryLength = 1024

// FileName arma el nombre de un respaldo programado de la base database.
func FileName(database, kind string, instant time.Time, suffix string) string {
	label := automaticLabel
	if kind == KindManual {
		label = manualLabel
	}
	return fmt.Sprintf("%s-%s-%s-%s.dump", database, instant.UTC().Format("20060102T150405Z"), suffix, label)
}

// automaticPattern reconoce exactamente los respaldos automáticos de la base: los únicos que borra la retención.
func automaticPattern(database string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(database) + `-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}-` + automaticLabel + `\.dump$`)
}

// partialPattern reconoce un respaldo programado que quedó a medias (el servidor se detuvo mientras respaldaba).
func partialPattern(database string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(database) + `-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}-(` + automaticLabel + `|` +
		manualLabel + `)\.dump\.partial$`)
}

// regularFiles devuelve los archivos regulares de la carpeta cuyo nombre cumple el patrón, en orden de nombre (que es
// el orden en que se crearon: la fecha va en el nombre, en UTC). Los enlaces y las carpetas nunca cuentan.
func regularFiles(directory string, pattern *regexp.Regexp) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && pattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Retain borra los respaldos automáticos más viejos de la carpeta y conserva los keep más recientes. Solo toca
// archivos regulares cuyo nombre es exactamente el de un respaldo automático de esta base: nunca los de «Respaldar
// ahora», los de la línea de comandos, los de antes de migrar ni otros archivos. Devuelve los nombres que borró; si
// uno no se pudo borrar, sigue con los demás y devuelve el primer error.
func Retain(directory, database string, keep int) ([]string, error) {
	if keep < 1 {
		return nil, errors.New("hay que conservar al menos un respaldo")
	}
	names, err := regularFiles(directory, automaticPattern(database))
	if err != nil {
		return nil, err
	}
	if len(names) <= keep {
		return nil, nil
	}
	removed := []string{}
	var first error
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		removed = append(removed, name)
	}
	return removed, first
}

// CleanPartials borra los respaldos programados que quedaron a medias en la carpeta. Solo se llama con el lock de los
// respaldos tomado: entonces ningún respaldo está en curso y un .partial con el formato propio es un resto.
func CleanPartials(directory, database string) {
	names, err := regularFiles(directory, partialPattern(database))
	if err != nil {
		return
	}
	for _, name := range names {
		_ = os.Remove(filepath.Join(directory, name))
	}
}

// CheckDirectory comprueba que el servidor pueda guardar respaldos en la carpeta: que exista (un disco o una USB
// desconectados no existen), que sea una carpeta y que acepte un archivo (se crea uno de prueba y se borra). Devuelve
// "" si está disponible o el motivo. Nunca crea la carpeta: si la USB no está conectada, no se crea una carpeta con
// su nombre en el disco de la computadora.
func CheckDirectory(directory string) string {
	info, err := os.Stat(directory)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return ReasonNotWritable
		}
		return ReasonMissing
	}
	if !info.IsDir() {
		return ReasonNotDirectory
	}
	probe, err := os.CreateTemp(directory, ".adel-pos-prueba-*")
	if err != nil {
		return ReasonNotWritable
	}
	name := probe.Name()
	_, writeError := probe.Write([]byte("AD-EL POS"))
	closeError := probe.Close()
	removeError := os.Remove(name)
	if writeError != nil || closeError != nil || removeError != nil {
		return ReasonNotWritable
	}
	return ""
}

// CheckExists es la comprobación sin escribir de la pantalla: que la carpeta exista y sea una carpeta. Devuelve "" o
// el motivo.
func CheckExists(directory string) string {
	info, err := os.Stat(directory)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return ReasonNotWritable
		}
		return ReasonMissing
	}
	if !info.IsDir() {
		return ReasonNotDirectory
	}
	return ""
}

// DirectoryError explica por qué no se acepta una carpeta elegida.
type DirectoryError struct {
	Reason string
}

func (problem *DirectoryError) Error() string {
	return "carpeta de respaldos no aceptada: " + problem.Reason
}

// ValidateDirectory valida la carpeta que llega del diálogo nativo de la computadora principal: ruta absoluta sin
// caracteres de control, existente, carpeta, con permiso de escritura para el servidor y que no sea la carpeta de
// datos de PostgreSQL. Devuelve la ruta limpia.
func ValidateDirectory(directory string) (string, error) {
	if directory == "" || len(directory) > maxDirectoryLength {
		return "", &DirectoryError{Reason: ReasonInvalid}
	}
	for _, character := range directory {
		if unicode.IsControl(character) {
			return "", &DirectoryError{Reason: ReasonInvalid}
		}
	}
	if !filepath.IsAbs(directory) {
		return "", &DirectoryError{Reason: ReasonRelative}
	}
	clean := filepath.Clean(directory)
	if reason := CheckDirectory(clean); reason != "" {
		return "", &DirectoryError{Reason: reason}
	}
	// La carpeta de datos de PostgreSQL (tiene PG_VERSION) no es lugar para respaldos.
	if _, err := os.Stat(filepath.Join(clean, "PG_VERSION")); err == nil {
		return "", &DirectoryError{Reason: ReasonPostgresData}
	}
	return clean, nil
}

// Describe da el nombre visible de una carpeta: su último componente y, si está en otro disco (en macOS, bajo
// /Volumes; en Windows, la unidad), el nombre del disco. La pantalla muestra eso en lugar de la ruta completa, que
// solo aparece en «Información para soporte».
func Describe(directory string) (string, string) {
	if directory == "" {
		return "", ""
	}
	folder := filepath.Base(directory)
	volume := ""
	switch runtime.GOOS {
	case "darwin":
		if rest, found := strings.CutPrefix(directory, "/Volumes/"); found {
			volume, _, _ = strings.Cut(rest, "/")
		}
	case "windows":
		volume = filepath.VolumeName(directory)
	}
	if folder == volume || folder == string(filepath.Separator) {
		folder = ""
	}
	return folder, volume
}

// SameDirectory compara dos carpetas por su ruta limpia (y sin distinguir mayúsculas en Windows y macOS, cuyos
// sistemas de archivos habituales no las distinguen).
func SameDirectory(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(first, second)
	}
	return first == second
}
