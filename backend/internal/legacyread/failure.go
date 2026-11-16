// Package legacyread es el lector del respaldo del sistema anterior (etapa 9H, §3.1): restaura el .bak en un SQL Server
// temporal de Docker, sin red y solo para leerlo, extrae todas las tablas y escribe el paquete de migración cifrado con
// internal/legacypkg.
//
// Solo lo usa el subcomando legacy-prepare de ad-el-pos-server: serve, host y los módulos del servidor nunca lo importan
// (lo comprueba imports_test.go), así que el servidor normal no depende de Docker ni de SQL Server.
//
// Reglas (docs/etapas/ETAPA_9H.md, «Datos reales» y §3.1):
//   - Imagen oficial de Microsoft (SQL Server 2022, edición Express gratuita), para linux/amd64 (Apple Silicon la
//     corre con la emulación de Docker Desktop).
//   - El contenedor no tiene red (--network none): SQL Server solo escucha en el loopback del propio contenedor y no se
//     publica ningún puerto. Las consultas van por docker exec con sqlcmd dentro del contenedor. Nunca se conecta a
//     otro SQL Server.
//   - La contraseña de sa se genera al momento, viaja solo en el entorno del proceso docker (nunca en sus argumentos)
//     y no se guarda en ningún lado.
//   - El respaldo se monta de solo lectura. El contenedor y su volumen se eliminan siempre al terminar (también con
//     error, cancelación o pánico) y, al empezar, se eliminan los que haya dejado una ejecución interrumpida (por su
//     etiqueta local.adel.pos.legacy-reader=1).
//   - Las contraseñas del sistema anterior nunca salen de SQL Server (columna <nombre>_present) y los binarios y las
//     tablas excluidas no viajan (legacypkg.TreatmentFor y ExcludedTableReason).
//   - Los mensajes son en español sencillo y nunca llevan datos del respaldo.
package legacyread

import (
	"errors"
	"fmt"
)

// Códigos de error del subcomando (evento «error» de la salida). La Fase D los muestra con su mensaje.
const (
	CodeUsage             = "USAGE"
	CodePassphraseInvalid = "PASSPHRASE_INVALID"
	CodeBackupUnreadable  = "BACKUP_UNREADABLE"
	CodeBackupInvalid     = "BACKUP_INVALID"
	CodeOutputExists      = "OUTPUT_EXISTS"
	CodeOutputInvalid     = "OUTPUT_INVALID"
	CodeDockerMissing     = "DOCKER_MISSING"
	CodeDockerNotRunning  = "DOCKER_NOT_RUNNING"
	CodeDockerUnsupported = "DOCKER_UNSUPPORTED"
	CodeImageUnavailable  = "IMAGE_UNAVAILABLE"
	CodeReaderStartFailed = "READER_START_FAILED"
	CodeReaderMemory      = "READER_MEMORY"
	CodeRestoreFailed     = "RESTORE_FAILED"
	CodeReadFailed        = "READ_FAILED"
	CodeWriteFailed       = "WRITE_FAILED"
	CodeCancelled         = "CANCELLED"
	CodeInternal          = "INTERNAL"
)

// messages son los mensajes para el propietario, sin tecnicismos.
var messages = map[string]string{
	CodeUsage:             "Uso: ad-el-pos-server legacy-prepare --backup <respaldo.bak> --output <paquete.adelmig> (la frase de paso, en la primera línea de la entrada estándar).",
	CodePassphraseInvalid: "La frase de paso debe tener al menos 8 caracteres.",
	CodeBackupUnreadable:  "No se pudo leer el respaldo. Comprueba que el archivo exista y que se pueda abrir.",
	CodeBackupInvalid:     "El archivo no es un respaldo de SQL Server del sistema anterior, o está dañado.",
	CodeOutputExists:      "Ya existe un archivo con ese nombre. Elige otro nombre para el paquete.",
	CodeOutputInvalid:     "El paquete debe terminar en .adelmig y guardarse en una carpeta en la que se pueda escribir.",
	CodeDockerMissing:     "Para preparar el paquete hace falta Docker Desktop. Instálalo desde docker.com (es gratuito), ábrelo y vuelve a intentarlo.",
	CodeDockerNotRunning:  "Docker Desktop está instalado pero no está en marcha. Ábrelo, espera a que diga que está listo y vuelve a intentarlo.",
	CodeDockerUnsupported: "Docker Desktop está usando contenedores de Windows. Cámbialo a contenedores de Linux desde su menú y vuelve a intentarlo.",
	CodeImageUnavailable:  "No se pudo descargar el lector del sistema anterior. Revisa la conexión a Internet y vuelve a intentarlo.",
	CodeReaderStartFailed: "No se pudo iniciar el lector temporal. Reinicia Docker Desktop y vuelve a intentarlo.",
	CodeReaderMemory:      "Docker Desktop no tiene memoria suficiente para el lector (necesita al menos 2 GB). Auméntala en la configuración de Docker Desktop (Resources) y vuelve a intentarlo.",
	CodeRestoreFailed:     "No se pudo abrir el respaldo en el lector temporal. Puede estar dañado o incompleto; prueba con otro respaldo.",
	CodeReadFailed:        "No se pudieron leer los datos del respaldo.",
	CodeWriteFailed:       "No se pudo guardar el paquete. Revisa que haya espacio en el disco o en la USB y vuelve a intentarlo.",
	CodeCancelled:         "Se canceló la preparación del paquete. No quedó nada a medias.",
	CodeInternal:          "Ocurrió un error inesperado al preparar el paquete.",
}

// Failure es un error del lector con su código, el mensaje para el propietario y un detalle técnico (sin datos del
// respaldo ni contraseñas: nunca el texto de un error de SQL Server, que puede citar valores).
type Failure struct {
	Code    string
	Message string
	Detail  string
	cause   error
}

func (failure *Failure) Error() string {
	if failure.Detail == "" {
		return failure.Code + ": " + failure.Message
	}
	return failure.Code + ": " + failure.Message + " (" + failure.Detail + ")"
}

func (failure *Failure) Unwrap() error {
	return failure.cause
}

func fail(code, detail string, cause error) *Failure {
	return &Failure{Code: code, Message: messages[code], Detail: detail, cause: cause}
}

func failf(code string, cause error, format string, values ...any) *Failure {
	return fail(code, fmt.Sprintf(format, values...), cause)
}

// AsFailure convierte cualquier error en un Failure (los que no lo son, en INTERNAL).
func AsFailure(err error) *Failure {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure
	}
	return fail(CodeInternal, "", err)
}
