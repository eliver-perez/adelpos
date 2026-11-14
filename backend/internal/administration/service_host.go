package administration

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/backupschedule"
	"local/ad-el-pos/server/internal/product"
)

// Estado y reinicio del servicio del servidor (etapa 9G, fase A; decisión D3), en el Mantenimiento del
// administrador (settings.manage):
//
//   - GET maintenance/service: si el servidor corre como servicio del sistema, su modalidad, desde cuándo
//     está activo, las terminales conectadas (vistas en los últimos minutos) y el último respaldo (fase F:
//     si fue correcto y dónde está).
//   - POST maintenance/service/restart: responde y después termina el servidor en orden (Go y
//     PostgreSQL); launchd lo vuelve a lanzar porque el archivo de habilitación sigue existiendo. No pide
//     la contraseña de administrador del sistema. Solo existe cuando el servidor es un servicio
//     administrado: con «serve» (desarrollo) o en Windows antes de la Fase W responde 409.

// HostControl informa y controla el servicio del sistema que ejecuta este servidor. Sin valor (serve en
// desarrollo y pruebas) el servidor no es un servicio administrado.
type HostControl interface {
	HostStatus() HostStatus
	// Restart programa el reinicio (después de responder) y devuelve false si no es posible.
	Restart() bool
}

// HostStatus es el estado del servicio que conoce el proceso del servidor.
type HostStatus struct {
	// Managed: lo lanzó y lo mantiene vivo el sistema (launchd), así que puede reiniciarse.
	Managed          bool
	InstallationMode string
	ServiceEnabled   bool
	StartedAt        time.Time
}

// La ventana de «conectada» es de 2 minutos desde la última petición de la terminal: cada terminal
// consulta su sesión cada 5 s y el tiempo real cada 750 ms, así que una terminal abierta nunca la pasa.

// processStarted aproxima el arranque del servidor cuando no hay HostControl.
var processStarted = time.Now()

// serviceRestart es el reinicio autorizado y auditado que se completa después de confirmar la transacción
// de autenticación (completeDeferred).
type serviceRestart struct{}

type serviceTerminals struct {
	Connected  int `json:"connected"`
	Registered int `json:"registered"`
}

type serviceStatusResponse struct {
	Managed          bool             `json:"managed"`
	RestartAvailable bool             `json:"restart_available"`
	InstallationMode string           `json:"installation_mode"`
	ServiceEnabled   bool             `json:"service_enabled"`
	StartedAt        time.Time        `json:"started_at"`
	UptimeSeconds    int64            `json:"uptime_seconds"`
	ServerVersion    string           `json:"server_version"`
	Terminals        serviceTerminals `json:"terminals"`
	// LastBackup es el último respaldo que hizo el servidor (automático o «Respaldar ahora»; etapa 9G, fase F): si
	// fue correcto y dónde está. null si todavía no hay ninguno.
	LastBackup *backupschedule.RunView `json:"last_backup"`
}

func (service *Service) serviceRoutes(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if err := principal.require("settings.manage"); err != nil {
		return nil, err
	}
	switch request.Method + " " + route {
	case "GET maintenance/service":
		return service.serviceStatus(ctx, tx, principal)
	case "POST maintenance/service/restart":
		return service.prepareRestart(ctx, tx, request, principal)
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

func (service *Service) hostStatus() HostStatus {
	if service.Host == nil {
		return HostStatus{StartedAt: processStarted}
	}
	return service.Host.HostStatus()
}

func (service *Service) serviceStatus(ctx context.Context, tx pgx.Tx, principal Principal) (any, error) {
	status := service.hostStatus()
	// Las terminales se cuentan sin la que pregunta (la pantalla del propio servidor) y sin las revocadas.
	var terminals serviceTerminals
	err := tx.QueryRow(ctx, `SELECT
            count(*) FILTER (WHERE last_seen_at > now() - interval '2 minutes'),
            count(*)
        FROM app.devices
        WHERE revoked_at IS NULL AND id IS DISTINCT FROM $1::uuid`,
		nullableID(principal.DeviceID)).Scan(&terminals.Connected, &terminals.Registered)
	if err != nil {
		return nil, err
	}
	lastBackup, err := backupschedule.LastBackup(ctx, tx, time.Now())
	if err != nil {
		return nil, err
	}
	return serviceStatusResponse{
		Managed:          status.Managed,
		RestartAvailable: status.Managed && status.ServiceEnabled,
		InstallationMode: status.InstallationMode,
		ServiceEnabled:   status.ServiceEnabled,
		StartedAt:        status.StartedAt.UTC(),
		UptimeSeconds:    int64(time.Since(status.StartedAt).Seconds()),
		ServerVersion:    product.Read().Version,
		Terminals:        terminals,
		LastBackup:       lastBackup,
	}, nil
}

// prepareRestart autoriza y audita el reinicio en la transacción de autenticación; el reinicio se programa
// después de confirmarla (completeDeferred).
func (service *Service) prepareRestart(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if len(request.Body) > 0 {
		var input struct{}
		if err := decode(request.Body, &input); err != nil {
			return nil, err
		}
	}
	// Solo se reinicia lo que launchd volverá a lanzar: un servicio administrado y habilitado.
	status := service.hostStatus()
	if !status.Managed || !status.ServiceEnabled {
		return nil, &Fault{
			Status:  409,
			Code:    "SERVICE_NOT_MANAGED",
			Message: "Este servidor no está instalado como servicio de la computadora; no se puede reiniciar desde aquí.",
		}
	}
	if err := audit(ctx, tx, principal, "maintenance.service_restarted", "maintenance", nil, nil, nil, true); err != nil {
		return nil, err
	}
	return &serviceRestart{}, nil
}

// completeRestart programa el reinicio ya confirmada la auditoría. El servidor termina en orden después
// de entregar esta respuesta (el cierre ordenado espera a las peticiones en curso).
func (service *Service) completeRestart() (any, error) {
	if service.Host == nil || !service.Host.Restart() {
		return nil, &Fault{
			Status:  409,
			Code:    "SERVICE_NOT_MANAGED",
			Message: "Este servidor no está instalado como servicio de la computadora; no se puede reiniciar desde aquí.",
		}
	}
	return map[string]bool{"restarting": true}, nil
}

// nullableID convierte un identificador vacío en NULL para las consultas.
func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}
