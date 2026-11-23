package licensing

import (
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Installation es la identidad de la computadora que verifica (contrato §2): su `server_id` y la llave
// pública de licencias, en base64url sin relleno.
type Installation struct {
	ServerID  string
	PublicKey string
}

// DatabaseProbe es lo que el gestor consulta de la base, sin tomar locks del negocio (datapresence.go).
type DatabaseProbe interface {
	// HasOperations indica si la base tiene operaciones: distingue «Sin activar» de «Sin activar con datos».
	HasOperations(ctx context.Context) (bool, error)
	// LatestActivity es la hora de la última actividad registrada (max de audit_events.occurred_at), con
	// falso si no hay ninguna. Solo inicia la última hora confiable cuando todavía no existe.
	LatestActivity(ctx context.Context) (time.Time, bool, error)
}

// Options es lo que necesita el gestor. Todos los campos son opcionales salvo Directory.
type Options struct {
	// Directory es la carpeta del estado de la licencia, identity/licencia (L1-D9).
	Directory string
	// Installation es la identidad de esta computadora. Sin ella (identidad dañada o sin llave de
	// licencias) no se verifica ni se acepta ninguna licencia y el estado es «Sin activar».
	Installation *Installation
	// PrivateKey es la llave privada de licencias de la instalación (localsetup.EnsureLicenseKey). La usa el
	// cliente para la prueba de posesión y los `.licreq` (Fase B). Nunca se registra ni sale del equipo.
	PrivateKey ed25519.PrivateKey
	// IdentityProblem explica, para la bitácora, por qué no hay Installation.
	IdentityProblem string
	// Trust son las llaves de confianza. Sin valor, las del entorno compilado (CompiledTrust).
	Trust *TrustSet
	// Now es el reloj local. Sin valor, time.Now. Las pruebas inyectan uno simulado (L1-D16).
	Now func() time.Time
	// MachineID lee el identificador del equipo (IOPlatformUUID o MachineGuid). Sin valor, el del sistema
	// (machine_*.go). Las pruebas inyectan uno simulado.
	MachineID func() (string, error)
	// MachineRetryDelay es la espera entre los tres intentos de lectura de la huella. Sin valor, 200 ms.
	MachineRetryDelay time.Duration
	// Database es la consulta de la base. Sin valor, la base se considera sin operaciones y la última hora
	// confiable no se inicia con la auditoría.
	Database DatabaseProbe
	// Logger recibe los avisos técnicos. Nunca recibe una licencia completa ni una llave.
	Logger *slog.Logger
}

// Errores de Accept que no vienen del verificador.
var (
	// ErrIdentityUnavailable: sin identidad o sin llave de licencias no se acepta ninguna licencia.
	ErrIdentityUnavailable = errors.New("la identidad del servidor o su llave de licencias no están disponibles")
	// ErrCopiedInstallation: la carpeta de datos se copió de otra computadora; antes de activar hay que crear
	// una identidad nueva (L1-D11).
	ErrCopiedInstallation = errors.New("esta instalación se copió de otra computadora; crea una identidad nueva antes de activar")
	// ErrMachineUnverified: no se pudo leer la huella del equipo (falla cerrada, L1-D10).
	ErrMachineUnverified = errors.New("no se pudo comprobar la huella de esta computadora")
	// ErrStateNotSaved: la licencia era válida pero no se pudo guardar; no se aceptó (§10.5 del contrato).
	ErrStateNotSaved = errors.New("no se pudo guardar el estado de la licencia")
)

// Manager es el gestor de la licencia del servidor Go principal: verifica (§10 del contrato), guarda en
// identity/licencia (store.go) y calcula el estado (§5.6 y §6) en memoria. Es seguro para uso concurrente.
//
// Interfaz para las demás fases (publicada en la Fase A; no se cambia sin avisar):
//
//   - Status(): el estado vigente, con los cupos.
//   - TrustedTime(): la última hora confiable.
//   - ObserveTime(t, fuente): sube la última hora confiable; nunca la baja.
//   - ReplaceTrustedTime(t, motivo): la reemplaza, también hacia atrás. Solo §6.3 y §6.4.
//   - Accept(jws): verifica y guarda una licencia (respuesta en línea o archivo .lic).
//   - LocalClock(): el reloj local inyectado, para que la puerta y los cupos usen el mismo.
type Manager struct {
	options Options
	mutex   sync.Mutex
}

// NewManager prepara el gestor en memoria. Load lee el disco y la base.
func NewManager(options Options) *Manager {
	return &Manager{options: options}
}

// Load lee el estado guardado, comprueba la huella del equipo y la presencia de operaciones en la base, y
// vuelve a verificar la licencia guardada (L1-D20). El gestor queda utilizable aunque devuelva un error
// (por ejemplo, sin poder escribir la carpeta): el error es para la bitácora.
func (manager *Manager) Load(ctx context.Context) error {
	return errors.New("sin implementar")
}

// Status es el estado de la licencia en este instante, calculado en memoria.
func (manager *Manager) Status() Status {
	return Status{State: StateNotActivated}
}

// TrustedTime es la última hora confiable (cero si todavía no hay ninguna).
func (manager *Manager) TrustedTime() time.Time {
	return time.Time{}
}

// ObserveTime alimenta la última hora confiable: solo sube.
func (manager *Manager) ObserveTime(instant time.Time, source TimeSource) {
}

// ReplaceTrustedTime reemplaza la última hora confiable, también hacia atrás, y la guarda de inmediato.
// Devuelve el valor anterior para la auditoría. Solo la usan la §6.3 y la §6.4.
func (manager *Manager) ReplaceTrustedTime(instant time.Time, reason ReplaceReason) (time.Time, error) {
	return time.Time{}, errors.New("sin implementar")
}

// Accept verifica una licencia con la §10 del contrato y, si es válida, la guarda y pasa a ser la vigente.
func (manager *Manager) Accept(jws string) (Status, error) {
	return Status{}, errors.New("sin implementar")
}

// LocalClock es el reloj local del gestor (Options.Now o time.Now).
func (manager *Manager) LocalClock() time.Time {
	return time.Now()
}
