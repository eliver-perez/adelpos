package backupschedule

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// Nombres de los archivos que crean los respaldos programados: <base>-<instalación>-<AAAAMMDDTHHMMSSZ>-<6 hex>-
// <etiqueta>.dump. <instalación> son 8 caracteres hexadecimales del identificador de esta instalación (el server_id de
// su identidad o, sin ella, el installation_id de backup_settings): en una carpeta compartida (una USB o una carpeta de
// red que usan la Mac y la PC, o dos sucursales) cada quien reconoce los suyos a simple vista. Mientras se crea y se
// verifica, el archivo lleva además la terminación .partial: un respaldo a medias nunca tiene el nombre definitivo.
//
// Qué archivos son «propios» no se decide por el nombre: la retención y la limpieza de restos solo tocan los archivos
// registrados en app.backup_runs por esta instalación (runner.go).
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
	ReasonServerData   = "server_data"
	ReasonInvalid      = "invalid"
)

// maxDirectoryLength es el largo máximo de la ruta de la carpeta (la misma restricción de backup_settings).
const maxDirectoryLength = 1024

// installationTagLength es cuántos caracteres del identificador de la instalación lleva el nombre.
const installationTagLength = 8

// InstallationTag convierte el identificador de la instalación (un UUID) en los 8 caracteres hexadecimales del nombre.
// Un identificador sin suficientes caracteres hexadecimales da "" (el nombre no puede llevar otra cosa).
func InstallationTag(identifier string) string {
	tag := make([]byte, 0, installationTagLength)
	for _, character := range strings.ToLower(identifier) {
		if character == '-' {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", character) {
			return ""
		}
		tag = append(tag, byte(character))
		if len(tag) == installationTagLength {
			return string(tag)
		}
	}
	return ""
}

// FileName arma el nombre de un respaldo programado de la base database hecho por la instalación tag.
func FileName(database, tag, kind string, instant time.Time, suffix string) string {
	label := automaticLabel
	if kind == KindManual {
		label = manualLabel
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s.dump", database, tag, instant.UTC().Format("20060102T150405Z"), suffix, label)
}

// removeRegular borra directory/name solo si es un archivo regular (nunca sigue un enlace ni borra una carpeta).
// Devuelve true si el archivo ya no está (borrado o inexistente).
func removeRegular(directory, name string) (bool, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return false, errors.New("nombre de archivo no válido")
	}
	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("no es un archivo regular; no se borra")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return true, nil
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
// caracteres de control, existente, carpeta y con permiso de escritura para el servidor. Se resuelven los enlaces y se
// devuelve la ruta real (la que se guarda), que no puede estar dentro de una carpeta de datos de PostgreSQL (ella o
// alguna superior tiene PG_VERSION: base, pg_wal…) ni dentro de las carpetas protegidas del servidor (la carpeta de
// datos del servicio con su identidad y bitácoras, o la de la configuración con serve).
func ValidateDirectory(directory string, protected []string) (string, error) {
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
	resolved, err := filepath.EvalSymlinks(filepath.Clean(directory))
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", &DirectoryError{Reason: ReasonNotWritable}
		}
		return "", &DirectoryError{Reason: ReasonMissing}
	}
	if len(resolved) > maxDirectoryLength {
		return "", &DirectoryError{Reason: ReasonInvalid}
	}
	if reason := CheckDirectory(resolved); reason != "" {
		return "", &DirectoryError{Reason: reason}
	}
	if insidePostgresData(resolved) {
		return "", &DirectoryError{Reason: ReasonPostgresData}
	}
	for _, folder := range protected {
		if folder == "" {
			continue
		}
		real := filepath.Clean(folder)
		if evaluated, err := filepath.EvalSymlinks(real); err == nil {
			real = evaluated
		}
		if Inside(resolved, real) {
			return "", &DirectoryError{Reason: ReasonServerData}
		}
	}
	return resolved, nil
}

// insidePostgresData indica si la carpeta o alguna superior es una carpeta de datos de PostgreSQL (tiene PG_VERSION).
func insidePostgresData(directory string) bool {
	current := directory
	for {
		if info, err := os.Stat(filepath.Join(current, "PG_VERSION")); err == nil && info.Mode().IsRegular() {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// Inside indica si path es folder o está dentro de él (rutas limpias; sin distinguir mayúsculas en macOS y Windows,
// cuyos sistemas de archivos habituales no las distinguen).
func Inside(path, folder string) bool {
	path = filepath.Clean(path)
	folder = filepath.Clean(folder)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		path = strings.ToLower(path)
		folder = strings.ToLower(folder)
	}
	relative, err := filepath.Rel(folder, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative))
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
