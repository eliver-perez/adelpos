package administration

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/licensing"
)

// Vigilante de la licencia y de la hora (etapa L1, Fase C; §6.1 y §6.2 de la especificación).
//
// Revisa al arrancar, cada licenseMonitorInterval y cuando la puerta ve un bloqueo, en el servidor Go principal:
//
//   - **La hora**: cada bloqueo («Hora del equipo incorrecta») y cada desbloqueo (el reloj se corrigió, o la última hora
//     confiable se reemplazó con la §6.3 o la §6.4) queda en la auditoría con las dos fechas: license.clock_blocked y
//     license.clock_unblocked.
//   - **El estado de la licencia**: un cambio de estado (por ejemplo, de Vigente a Tolerancia porque pasó el
//     vencimiento) queda como license.state_changed.
//   - **Las terminales**: cualquier cambio de lo que ven (estado, días de tolerancia, vencimiento o bloqueo por hora)
//     publica license.changed en el canal de eventos. El evento no lleva datos: las terminales vuelven a leer auth/me o
//     restaurant/state y muestran el aviso fijo.
//
// Todo se escribe en una transacción propia y corta, nunca dentro de la de una petición: así la puerta no toma una
// segunda conexión de la base mientras tiene la suya. Si no se puede escribir, se reintenta en la siguiente revisión.

// licenseMonitorInterval es cada cuánto revisa el vigilante: más seguido que el minuto de la §6.1, para que el aviso de
// las terminales aparezca y se quite pronto.
const licenseMonitorInterval = 15 * time.Second

// licenseMonitorProcess es el autor de la auditoría del vigilante (actor_kind «process»).
const licenseMonitorProcess = "license_monitor"

// Acciones de la auditoría del vigilante.
const (
	clockBlockedAction   = "license.clock_blocked"
	clockUnblockedAction = "license.clock_unblocked"
	stateChangedAction   = "license.state_changed"
)

// licenseSnapshot es lo que ven las terminales de la licencia y de la hora: si cambia, se avisa.
type licenseSnapshot struct {
	State              licensing.State
	Copied             bool
	MachineUnverified  bool
	RevocationReason   string
	LicenseID          string
	Revision           int64
	ExpiresAt          string
	DaysUntilExpiry    int
	GraceDaysRemaining int
	ClockBlocked       bool
}

// snapshotOf es lo que ven las terminales de un estado y una revisión de la hora.
func snapshotOf(status licensing.Status, clock licensing.ClockStatus) licenseSnapshot {
	snapshot := licenseSnapshot{
		State:             status.State,
		Copied:            status.Copied,
		MachineUnverified: status.MachineUnverified,
		RevocationReason:  status.RevocationReason,
		LicenseID:         status.LicenseID,
		Revision:          status.Revision,
		ClockBlocked:      clock.Blocked,
	}
	if status.ExpiresAt != nil {
		snapshot.ExpiresAt = status.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	switch {
	case status.State == licensing.StateActive && status.ExpiresAt != nil:
		snapshot.DaysUntilExpiry = daysUntil(*status.ExpiresAt, status.EffectiveNow)
	case status.State == licensing.StateGrace && status.GraceEndsAt != nil:
		snapshot.GraceDaysRemaining = daysUntil(*status.GraceEndsAt, status.EffectiveNow)
	}
	return snapshot
}

type licenseMonitor struct {
	mutex    sync.Mutex
	recorded *licenseSnapshot
	wake     chan struct{}
}

// monitor es el vigilante del servicio (se crea al primer uso).
func (service *Service) monitor() *licenseMonitor {
	service.licenseMonitorOnce.Do(func() {
		service.licenseMonitorState = &licenseMonitor{wake: make(chan struct{}, 1)}
	})
	return service.licenseMonitorState
}

// wakeLicenseMonitor pide una revisión sin esperarla (la puerta, al ver un bloqueo, y las rutas de la hora).
func (service *Service) wakeLicenseMonitor() {
	select {
	case service.monitor().wake <- struct{}{}:
	default:
	}
}

// RunLicenseMonitor revisa al arrancar y después cada licenseMonitorInterval o cuando se le pide, hasta que se cancela
// el contexto. Sin gestor de licencias no hace nada.
func (service *Service) RunLicenseMonitor(ctx context.Context) {
	if service.License == nil {
		return
	}
	monitor := service.monitor()
	service.checkLicenseLogged(ctx)
	ticker := time.NewTicker(licenseMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-monitor.wake:
		}
		service.checkLicenseLogged(ctx)
	}
}

func (service *Service) checkLicenseLogged(ctx context.Context) {
	if err := service.CheckLicense(ctx); err != nil && service.Logger != nil && ctx.Err() == nil {
		service.Logger.Warn("license_monitor_failed", "reason", err.Error())
	}
}

// CheckLicense revisa la licencia y la hora y registra lo que cambió desde la revisión anterior (auditoría y evento
// license.changed). Lo usa el vigilante; las rutas de la licencia y las pruebas lo llaman para avisar en el momento.
func (service *Service) CheckLicense(ctx context.Context) error {
	manager := service.License
	if manager == nil {
		return nil
	}
	monitor := service.monitor()
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	status := manager.Status()
	clock := manager.Clock()
	snapshot := snapshotOf(status, clock)
	previous := monitor.recorded
	if previous != nil && *previous == snapshot {
		return nil
	}
	// Al arrancar sin bloqueo solo se toma la referencia: no hay nada que avisar.
	if previous == nil && !snapshot.ClockBlocked {
		monitor.recorded = &snapshot
		return nil
	}
	writeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := service.Pool.Begin(writeContext)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	location, timezone, err := restaurantLocation(writeContext, tx)
	if err != nil {
		return err
	}
	if previous == nil || previous.ClockBlocked != snapshot.ClockBlocked {
		action := clockUnblockedAction
		if snapshot.ClockBlocked {
			action = clockBlockedAction
		}
		if err := auditLicenseProcess(writeContext, tx, action, clockDetails(clock, status, location, timezone)); err != nil {
			return err
		}
	}
	if previous != nil && previous.State != snapshot.State {
		if err := auditLicenseProcess(writeContext, tx, stateChangedAction, map[string]any{
			"from": previous.State, "to": snapshot.State, "copied": snapshot.Copied, "reason": snapshot.RevocationReason,
		}); err != nil {
			return err
		}
	}
	if err := events.AppendLicenseChanged(writeContext, tx); err != nil {
		return err
	}
	if err := tx.Commit(writeContext); err != nil {
		return err
	}
	monitor.recorded = &snapshot
	return nil
}

// auditLicenseProcess escribe un registro de auditoría del vigilante, sin usuario ni terminal (como el trabajador de
// «Agotado»).
func auditLicenseProcess(ctx context.Context, tx pgx.Tx, action string, after any) error {
	encoded, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_id,actor_username,actor_kind,device_id,action,entity_type,
            entity_id,success,before_state,after_state)
        VALUES (NULL,$1,'process',NULL,$2,'license',NULL,true,NULL,$3)`, licenseMonitorProcess, action, encoded)
	return err
}
