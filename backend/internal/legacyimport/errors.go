package legacyimport

import (
	"errors"
	"fmt"
)

// Códigos de error de la migración (los ve el asistente en error_code de la corrida y en las respuestas de las
// rutas). Los mensajes van en español sencillo y nunca llevan datos del paquete ni la frase de paso.
const (
	CodeNotPackage         = "LEGACY_NOT_PACKAGE"
	CodeUnsupportedVersion = "LEGACY_PACKAGE_VERSION"
	CodeWrongPassphrase    = "LEGACY_PASSPHRASE_INVALID"
	CodeDamaged            = "LEGACY_PACKAGE_DAMAGED"
	CodeLimit              = "LEGACY_PACKAGE_LIMIT"
	CodeIncompatible       = "LEGACY_PACKAGE_INCOMPATIBLE"
	CodeNotClean           = "LEGACY_INSTALLATION_NOT_CLEAN"
	CodeBackupFailed       = "LEGACY_BACKUP_FAILED"
	CodeBusy               = "LEGACY_BUSY"
	CodeBatchChanged       = "LEGACY_BATCH_CHANGED"
	CodeInterrupted        = "LEGACY_INTERRUPTED"
	CodeLocksBusy          = "LEGACY_LOCKS_BUSY"
	CodeUploadMissing      = "LEGACY_UPLOAD_MISSING"
	CodeFailed             = "LEGACY_FAILED"
)

// Error es un error de la migración con su código y su mensaje para el propietario. Detail es técnico (bitácora) y
// nunca lleva datos del restaurante. Blockers lista lo que impide importar (instalación no limpia).
type Error struct {
	Code     string
	Message  string
	Detail   string
	Blockers []Blocker
	cause    error
}

func (problem *Error) Error() string {
	if problem.Detail == "" {
		return problem.Code + ": " + problem.Message
	}
	return problem.Code + ": " + problem.Message + " (" + problem.Detail + ")"
}

func (problem *Error) Unwrap() error {
	return problem.cause
}

// CodeOf es el código de un error de la migración o CodeFailed.
func CodeOf(err error) string {
	var problem *Error
	if errors.As(err, &problem) {
		return problem.Code
	}
	return CodeFailed
}

// MessageOf es el mensaje para el propietario de un error de la migración o uno genérico.
func MessageOf(err error) string {
	var problem *Error
	if errors.As(err, &problem) {
		return problem.Message
	}
	return "La migración no se pudo completar. Revisa que el servidor esté disponible e inténtalo de nuevo; no se importó nada."
}

func incompatible(table, column, detail string) *Error {
	message := "El paquete no tiene la estructura esperada del sistema anterior."
	if column != "" {
		message = fmt.Sprintf("El paquete no tiene la estructura esperada del sistema anterior (tabla %s, columna %s).", table, column)
	} else if table != "" {
		message = fmt.Sprintf("El paquete no tiene la estructura esperada del sistema anterior (tabla %s).", table)
	}
	return &Error{Code: CodeIncompatible, Message: message, Detail: detail}
}

func failure(message string, cause error) *Error {
	detail := ""
	if cause != nil {
		detail = cause.Error()
	}
	return &Error{Code: CodeFailed, Message: message, Detail: detail, cause: cause}
}
