package legacypkg

import (
	"errors"
	"fmt"
)

// ErrorKind clasifica los errores de lectura del paquete para que el importador y el asistente den un mensaje claro
// (y, en la API, un código propio).
type ErrorKind string

const (
	// KindNotPackage: el archivo no empieza con la firma de un paquete de migración.
	KindNotPackage ErrorKind = "not_package"
	// KindUnsupportedVersion: el paquete es de una versión del formato más nueva que esta versión de AD-EL POS.
	KindUnsupportedVersion ErrorKind = "unsupported_version"
	// KindWrongPassphrase: el valor de comprobación de la llave no coincide con la frase de paso.
	KindWrongPassphrase ErrorKind = "wrong_passphrase"
	// KindDamaged: bloque alterado, archivo truncado, huella o conteo que no coincide, JSON o tar inválido.
	KindDamaged ErrorKind = "damaged"
	// KindLimit: el paquete pide más recursos de los permitidos (memoria de Argon2, bloque, tamaño descomprimido,
	// número de tablas o tamaño de una fila). Un paquete legítimo nunca llega a esos límites.
	KindLimit ErrorKind = "limit"
	// KindInvalid: un uso incorrecto del escritor o del lector (por ejemplo, una frase de paso demasiado corta).
	KindInvalid ErrorKind = "invalid"
	// KindUnreadable: no se pudo leer el archivo (error de entrada y salida: una USB desconectada, una subida
	// interrumpida). No dice nada del paquete: puede estar bien y bastar con volver a intentarlo.
	KindUnreadable ErrorKind = "unreadable"
)

// Error es un error tipificado del paquete. Message está en español sencillo para el propietario; Detail es técnico
// (para la bitácora) y nunca lleva datos del paquete ni la frase de paso.
type Error struct {
	Kind    ErrorKind
	Message string
	Detail  string
	cause   error
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Message
	}
	return e.Message + " (" + e.Detail + ")"
}

func (e *Error) Unwrap() error {
	return e.cause
}

// Is permite comparar por tipo con los errores de referencia (errors.Is(err, legacypkg.ErrDamaged)).
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok {
		return false
	}
	return other.Kind == e.Kind && other.Message == "" && other.Detail == ""
}

// Errores de referencia para errors.Is; no llevan mensaje propio.
var (
	ErrNotPackage         = &Error{Kind: KindNotPackage}
	ErrUnsupportedVersion = &Error{Kind: KindUnsupportedVersion}
	ErrWrongPassphrase    = &Error{Kind: KindWrongPassphrase}
	ErrDamaged            = &Error{Kind: KindDamaged}
	ErrLimit              = &Error{Kind: KindLimit}
	ErrInvalid            = &Error{Kind: KindInvalid}
	ErrUnreadable         = &Error{Kind: KindUnreadable}
)

// KindOf devuelve el tipo de un error del paquete o "" si no es uno.
func KindOf(err error) ErrorKind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return ""
}

func notPackage() *Error {
	return &Error{
		Kind:    KindNotPackage,
		Message: "El archivo no es un paquete de migración de AD-EL POS.",
	}
}

func unsupportedVersion(version uint16) *Error {
	return &Error{
		Kind: KindUnsupportedVersion,
		Message: "El paquete se preparó con una versión más nueva de AD-EL POS. " +
			"Actualiza AD-EL POS en esta computadora y vuelve a intentarlo.",
		Detail: fmt.Sprintf("versión del formato %d", version),
	}
}

func wrongPassphrase() *Error {
	return &Error{
		Kind:    KindWrongPassphrase,
		Message: "La frase de paso no es correcta.",
	}
}

func damaged(detail string, cause error) *Error {
	return &Error{
		Kind:    KindDamaged,
		Message: "El paquete está dañado o fue modificado. Vuelve a copiarlo o prepáralo de nuevo.",
		Detail:  detail,
		cause:   cause,
	}
}

// unreadable es un error al leer el archivo (no un paquete dañado): el texto técnico del sistema va como causa.
func unreadable(detail string, cause error) *Error {
	return &Error{
		Kind: KindUnreadable,
		Message: "No se pudo leer el archivo del paquete. " +
			"Si está en una USB, comprueba que siga conectada; si se copió o se subió, vuelve a intentarlo.",
		Detail: detail,
		cause:  cause,
	}
}

func limitExceeded(detail string) *Error {
	return &Error{
		Kind:    KindLimit,
		Message: "El paquete excede los límites de seguridad y no se puede abrir.",
		Detail:  detail,
	}
}

func invalid(message string) *Error {
	return &Error{
		Kind:    KindInvalid,
		Message: message,
	}
}
