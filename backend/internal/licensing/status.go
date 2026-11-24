package licensing

import "time"

// State es uno de los seis estados de la licencia en AD-EL POS (contrato, sección 6). Los valores son los
// mismos que usan los archivos de prueba del contrato (`temporal_states`) y los códigos de error de la API
// (`details.state` de LICENSE_NOT_ACTIVATED, avance §0.6.2).
type State string

const (
	// StateActive es «Vigente»: licencia válida, `active`, antes de su vencimiento (o perpetua).
	StateActive State = "active"
	// StateGrace es «Tolerancia»: licencia válida, `active`, dentro de sus `grace_days`.
	StateGrace State = "grace"
	// StateExpired es «Vencida»: licencia válida, `active`, con la tolerancia agotada.
	StateExpired State = "expired"
	// StateRevoked es «Revocada»: licencia válida con `license_status: revoked`. El motivo va en
	// Status.RevocationReason.
	StateRevoked State = "revoked"
	// StateNotActivated es «Sin activar»: ninguna licencia válida y la base sin operaciones.
	StateNotActivated State = "not_activated"
	// StateNotActivatedWithData es «Sin activar con datos»: ninguna licencia válida, pero la base tiene
	// operaciones (por ejemplo, un respaldo restaurado o una instalación copiada de otra computadora).
	StateNotActivatedWithData State = "not_activated_with_data"
)

// Motivos de una licencia revocada (`status_reason`, contrato §4). El mensaje al usuario depende de él.
const (
	ReasonDeactivated = "deactivated" // la computadora liberó la licencia
	ReasonTransferred = "transferred" // se transfirió a otra computadora, o el propietario la retiró de esta
	ReasonRevoked     = "revoked"     // el propietario revocó la licencia
)

// Plan es informativo (contrato §4): los derechos reales son Limits.
type Plan struct {
	PlanID string
	Name   string
	Custom bool
}

// Limits son los cupos de sesiones simultáneas de la licencia (contrato §7): enteros de 0 a 999. En V1 no
// existe «sin límite»: sin licencia válida, los tres valen 0.
type Limits struct {
	CashRegisters   int
	OrderTakers     int
	KitchenDisplays int
}

// Status es la fotografía del estado de la licencia que entrega el gestor (Manager.Status). Se calcula en
// memoria, sin internet y sin la base, con la licencia ya verificada al aceptarla o al arrancar (L1-D20).
//
// Sin licencia válida (los dos estados «Sin activar»), los campos de la licencia quedan vacíos y Limits en
// cero. Una licencia guardada en otra computadora (Copied) o sin poder comprobar la huella del equipo
// (MachineUnverified) se ignora: el estado es «Sin activar» o «Sin activar con datos».
type Status struct {
	State State

	// Copied indica que la huella del equipo no coincide con la guardada: la carpeta de datos se copió de
	// otra computadora (§1.1 de la especificación, L1-D11). Mensaje: «Esta instalación se copió de otra
	// computadora. Actívala o transfiere la licencia.». Para activar hay que crear antes una identidad nueva.
	Copied bool
	// MachineUnverified indica que la huella del equipo no se pudo leer tras tres intentos (L1-D10): se trata
	// como distinta (falla cerrada), con su propio mensaje: «No se pudo comprobar esta computadora. Reinicia el
	// servidor; si sigue igual, comunícate con soporte.».
	MachineUnverified bool
	// IdentityUnavailable indica que falta la identidad del servidor o su llave de licencias: no se puede
	// verificar ni activar ninguna licencia (IDENTITY_UNAVAILABLE en las rutas de la licencia).
	IdentityUnavailable bool
	// RevocationReason es el `status_reason` de una licencia revocada (ReasonDeactivated, ReasonTransferred o
	// ReasonRevoked); vacío en los demás estados.
	RevocationReason string

	// Environment es el entorno de firma en que confía esta compilación: «test» en las versiones de
	// desarrollo y «production» en las de producción (§1.2 de la especificación). Se informa siempre.
	Environment string

	// Datos de la licencia vigente (vacíos sin licencia válida).
	LicenseID      string
	ActivationID   string
	LicenseeName   string
	LicenseType    string // perpetual, subscription o trial
	BillingPeriod  string // monthly, quarterly, semiannual o annual; vacío en perpetua y prueba
	Revision       int64
	IssuedAt       time.Time
	PeriodStartsAt time.Time
	UpdatesUntil   time.Time
	Plan           Plan
	Limits         Limits
	// Features son los módulos habilitados (`features`, clave → true). Es una copia: quien la recibe puede
	// guardarla sin afectar al gestor. Las claves desconocidas no habilitan nada.
	Features map[string]bool

	// Perpetual indica una licencia perpetua: no vence y ExpiresAt y GraceEndsAt quedan en nil.
	Perpetual bool
	// ExpiresAt es el vencimiento (instante exclusivo, `expires_at`).
	ExpiresAt *time.Time
	// GraceDays son los días de tolerancia (`grace_days`): 5 en suscripción, 0 en prueba y perpetua.
	GraceDays int
	// GraceEndsAt es el fin de la tolerancia: ExpiresAt + GraceDays × 86 400 s (instante exclusivo). Desde
	// ahí la licencia está vencida. En una prueba coincide con ExpiresAt.
	GraceEndsAt *time.Time

	// EffectiveNow es máximo(reloj local, última hora confiable) con que se calculó el estado (§5.6).
	EffectiveNow time.Time
	// TrustedTime es la última hora confiable guardada (cero si todavía no hay ninguna).
	TrustedTime time.Time
}

// HasLicense indica si hay una licencia válida aceptada en esta computadora (Vigente, Tolerancia, Vencida o
// Revocada). En los dos estados «Sin activar» es falso y los cupos valen cero.
func (status Status) HasLicense() bool {
	switch status.State {
	case StateActive, StateGrace, StateExpired, StateRevoked:
		return true
	default:
		return false
	}
}

// TimeSource es la fuente de una hora observada (Manager.ObserveTime). Con cualquier fuente, la última
// hora confiable solo sube (§5.6 del contrato y L1-D15).
type TimeSource string

const (
	// TimeFromServer es el `server_time` de una respuesta del servidor de licencias.
	TimeFromServer TimeSource = "server_time"
	// TimeFromLicense es el `issued_at` de una licencia aceptada.
	TimeFromLicense TimeSource = "issued_at"
	// TimeFromClock es el reloj local mientras el servidor funciona (cada minuto).
	TimeFromClock TimeSource = "local_clock"
	// TimeFromOperation es la hora de una operación aceptada por el backend.
	TimeFromOperation TimeSource = "operation"
	// TimeFromAudit es la última actividad registrada en la base (`max(audit_events.occurred_at)`), que
	// solo inicia la última hora confiable cuando todavía no existe (Duda 16, L1-D15).
	TimeFromAudit TimeSource = "audit"
)

// ReplaceReason es el motivo por el que la última hora confiable se reemplaza (y puede bajar). Solo hay
// dos (L1-D15): la corrección con internet (§6.3) y el desbloqueo de soporte (§6.4).
type ReplaceReason string

const (
	// ReplaceByInternetCheck es «Comprobar la hora con internet» (§6.3): el reloj coincide con el
	// `server_time` y la última hora confiable estaba en el futuro.
	ReplaceByInternetCheck ReplaceReason = "internet_check"
	// ReplaceBySupportUnlock es la respuesta firmada del desbloqueo de soporte (§6.4).
	ReplaceBySupportUnlock ReplaceReason = "support_unlock"
)
