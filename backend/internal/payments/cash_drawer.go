package payments

import (
	"errors"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Cajón de dinero (etapa 7, §18). Cada caja física puede tener un cajón conectado a una impresora
// ESC/POS: Go encola un pulso (ESC p) en la cola durable de impresión y registra el comando en
// cash_drawer_events ligado a su trabajo.
//
//   - Configuración por caja (cash.settings.manage, auditada). Solo se admiten impresoras cuyo
//     transporte entregue los bytes ESC/POS sin alterar (ESC/POS por red); con CUPS o el agente
//     futuro se rechaza: no se asume que todos los dispositivos admiten el mismo protocolo.
//   - Apertura automática, con el cajón activado: después de cobros y devoluciones con efectivo
//     físico y de ingresos, retiros y gastos registrados. Nunca por pagos exclusivamente
//     electrónicos. Si el comando no puede encolarse, la operación ya válida se confirma igual.
//   - Apertura manual con cash.drawer.open, motivo, idempotencia y auditoría, desde una terminal
//     de la caja.
//   - Un resultado incierto nunca se reintenta solo (semántica uncertain de la cola) y un error
//     confirmado tampoco: un cajón que se abre más tarde sin nadie presente es un riesgo físico.
//   - Las respuestas nunca afirman que el cajón se abrió: solo que el comando se envió a la cola o
//     que la impresora aceptó los bytes.

// Motivos de un comando de apertura (cash_drawer_events.reason).
const (
	drawerPayment   = "payment"
	drawerRefund    = "refund"
	drawerOperation = "operation"
	drawerManual    = "manual"
)

// DrawerOutcome es el resultado de solicitar la apertura del cajón. Status es el estado del
// trabajo en la cola o disabled (cajón no activado), unavailable (impresora inactiva o
// incompatible) o failed (no se pudo encolar).
type DrawerOutcome struct {
	Requested bool   `json:"requested"`
	Status    string `json:"status,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

// DrawerSettings es la configuración del cajón de una caja física.
type DrawerSettings struct {
	RegisterID   string     `json:"register_id"`
	RegisterName string     `json:"register_name"`
	Active       bool       `json:"register_active"`
	PrinterID    *string    `json:"printer_id"`
	PrinterName  *string    `json:"printer_name"`
	PulsePin     int        `json:"pulse_pin"`
	Enabled      bool       `json:"enabled"`
	Revision     int        `json:"revision"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

// DrawerPrinter es una impresora candidata con su compatibilidad para el pulso del cajón.
type DrawerPrinter struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Transport  string `json:"transport"`
	Active     bool   `json:"active"`
	Compatible bool   `json:"compatible"`
}

// drawerConfig es la configuración vigente del cajón de una caja con su impresora.
type drawerConfig struct {
	registerName string
	printerID    *string
	printerName  *string
	transport    *string
	active       *bool
	pin          int
	enabled      bool
	revision     int
}

// readDrawerConfig lee la configuración del cajón de una caja; sin fila, revisión 0 y desactivado.
func (handler *service) readDrawerConfig(registerID string) (*drawerConfig, error) {
	config := &drawerConfig{}
	err := handler.tx.QueryRow(handler.ctx, `SELECT r.name,c.printer_id,p.name,p.transport,p.active,coalesce(c.pulse_pin,0),
        coalesce(c.enabled,false),coalesce(c.revision,0)
        FROM app.registers r LEFT JOIN app.cash_drawer_settings c ON c.register_id=r.id
        LEFT JOIN app.kitchen_printers p ON p.id=c.printer_id WHERE r.id=$1`, registerID).Scan(&config.registerName,
		&config.printerID, &config.printerName, &config.transport, &config.active, &config.pin, &config.enabled, &config.revision)
	if isNoRows(err) {
		return nil, notFound("La caja no existe.")
	}
	return config, err
}

// drawerUnavailable explica por qué un cajón activado no puede recibir el comando.
func drawerUnavailable(config *drawerConfig) string {
	if config.printerID == nil || config.transport == nil {
		return "El cajón de " + config.registerName + " no tiene impresora asociada."
	}
	if config.active == nil || !*config.active {
		return "La impresora del cajón de " + config.registerName + " está desactivada; no se envió el comando de apertura."
	}
	if !kitchen.CompatibleDrawerTransport(*config.transport) {
		return "La impresora del cajón de " + config.registerName + " no usa ESC/POS por red; no se envió el comando de apertura."
	}
	return ""
}

// queueDrawerCommand encola el pulso y registra el comando con la transacción del servicio.
func (handler *service) queueDrawerCommand(registerID string, shiftID *string, config *drawerConfig, reason, notes string) (DrawerOutcome, error) {
	job, _, err := kitchen.EnqueueDrawerPulse(handler.ctx, handler.tx, kitchen.DrawerCommand{
		PrinterID: *config.printerID, Pin: config.pin, RegisterName: config.registerName, Reason: reason,
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return DrawerOutcome{}, err
	}
	eventID := newID()
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_drawer_events(id,register_id,shift_id,reason,notes,actor_id,device_id,
        job_id,operation_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, eventID, registerID, shiftID, reason, notes,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, job.ID, handler.operationID); err != nil {
		return DrawerOutcome{}, err
	}
	aggregate := registerID
	if shiftID != nil {
		aggregate = *shiftID
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.drawer.command_queued", cashScope, &aggregate, nil); err != nil {
		return DrawerOutcome{}, err
	}
	return DrawerOutcome{Requested: true, Status: job.State, JobID: job.ID, EventID: eventID, Message: job.LastError}, nil
}

// openDrawerAfter solicita la apertura automática del cajón de la caja del turno tras una operación
// con efectivo físico. Sin cajón activado no hace nada. Se ejecuta en un punto de guardado: un
// fallo al encolar nunca revierte la operación ya válida.
func (handler *service) openDrawerAfter(shift *Shift, reason string) DrawerOutcome {
	// La consulta de la configuración también va dentro del punto de guardado: un error de
	// PostgreSQL fuera de él abortaría la transacción completa y, con ella, la operación.
	outcome, err := handler.withSavepoint(func(scoped *service) (any, error) {
		config, err := scoped.readDrawerConfig(shift.RegisterID)
		if err != nil {
			return nil, err
		}
		if !config.enabled {
			return DrawerOutcome{Requested: false, Status: "disabled"}, nil
		}
		if message := drawerUnavailable(config); message != "" {
			return DrawerOutcome{Requested: true, Status: "unavailable", Message: message + " La operación quedó registrada."}, nil
		}
		return scoped.queueDrawerCommand(shift.RegisterID, &shift.ID, config, reason, "")
	})
	if err != nil {
		return DrawerOutcome{Requested: true, Status: "failed", Message: "No se pudo enviar el comando de apertura del cajón; la operación quedó registrada."}
	}
	return outcome.(DrawerOutcome)
}

// openDrawerManually atiende POST payments/cash/open-drawer (cash.drawer.open): abre el cajón de
// la caja de la terminal con un motivo auditado. Es idempotente por operation_id: el reintento
// devuelve el mismo resultado sin emitir otro pulso.
func (handler *service) openDrawerManually(_ []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Reason      string `json:"reason"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo de la apertura del cajón (5–500 caracteres).")
	}
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	if register == nil {
		return nil, conflict("REGISTER_REQUIRED", "Esta terminal no tiene una caja física asignada; el cajón se abre desde una terminal de su caja.")
	}
	config, err := handler.readDrawerConfig(register.ID)
	if err != nil {
		return nil, err
	}
	if !config.enabled {
		return nil, conflict("DRAWER_DISABLED", "El cajón de "+register.Name+" no está activado. Configúralo en Caja → Configuración.")
	}
	if message := drawerUnavailable(config); message != "" {
		return nil, conflict("DRAWER_UNAVAILABLE", message)
	}
	shift, err := handler.activeShift(register.ID)
	if err != nil {
		return nil, err
	}
	var shiftID *string
	if shift != nil {
		shiftID = &shift.ID
	}
	outcome, err := handler.queueDrawerCommand(register.ID, shiftID, config, drawerManual, reason)
	if err != nil {
		return nil, err
	}
	return outcome, handler.audit(handler.ctx, handler.tx, "cash.drawer_opened_manually", "register", &register.ID, nil, map[string]any{
		"reason": reason, "job_id": outcome.JobID, "event_id": outcome.EventID, "shift_id": shiftID,
	})
}

// listDrawerSettings atiende GET payments/cash/drawers (cash.settings.manage): la configuración
// del cajón de cada caja y las impresoras con su compatibilidad.
func (handler *service) listDrawerSettings(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT r.id,r.name,r.active,c.printer_id,p.name,coalesce(c.pulse_pin,0),
        coalesce(c.enabled,false),coalesce(c.revision,0),c.updated_at
        FROM app.registers r LEFT JOIN app.cash_drawer_settings c ON c.register_id=r.id
        LEFT JOIN app.kitchen_printers p ON p.id=c.printer_id
        WHERE r.active OR c.register_id IS NOT NULL ORDER BY r.name,r.id`)
	if err != nil {
		return nil, err
	}
	drawers := []DrawerSettings{}
	for rows.Next() {
		var item DrawerSettings
		if err := rows.Scan(&item.RegisterID, &item.RegisterName, &item.Active, &item.PrinterID, &item.PrinterName, &item.PulsePin,
			&item.Enabled, &item.Revision, &item.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		drawers = append(drawers, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	printerRows, err := handler.tx.Query(handler.ctx, "SELECT id,name,transport,active FROM app.kitchen_printers ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer printerRows.Close()
	printers := []DrawerPrinter{}
	for printerRows.Next() {
		var item DrawerPrinter
		if err := printerRows.Scan(&item.ID, &item.Name, &item.Transport, &item.Active); err != nil {
			return nil, err
		}
		item.Compatible = kitchen.CompatibleDrawerTransport(item.Transport)
		printers = append(printers, item)
	}
	if err := printerRows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"drawers": drawers, "printers": printers}, nil
}

// drawerProtocolUnsupported rechaza una impresora cuyo transporte no entrega el pulso sin alterar.
func drawerProtocolUnsupported(name, transport string) error {
	labels := map[string]string{"cups": "CUPS", "agent": "un agente de impresión"}
	label := labels[transport]
	if label == "" {
		label = transport
	}
	return &Error{
		Status: 400,
		Code:   "DRAWER_PROTOCOL_UNSUPPORTED",
		Message: "La impresora " + name + " usa " + label + ", que no entrega el comando ESC/POS de apertura sin alterar. " +
			"El cajón solo se abre con una impresora ESC/POS conectada por red.",
	}
}

// saveDrawerSettings atiende PUT payments/cash/drawers/{caja} (cash.settings.manage). Valida la
// caja, la impresora y su protocolo antes de escribir; revisión optimista y auditoría.
func (handler *service) saveDrawerSettings(parts []string) (any, error) {
	var input struct {
		OperationID string  `json:"operation_id"`
		Revision    int     `json:"revision"`
		PrinterID   *string `json:"printer_id"`
		PulsePin    int     `json:"pulse_pin"`
		Enabled     bool    `json:"enabled"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	registerID := parts[2]
	if input.PulsePin != 0 && input.PulsePin != 1 {
		return nil, invalid("El pin del cajón debe ser 0 (conector 2) o 1 (conector 5).")
	}
	if input.PrinterID != nil && strings.TrimSpace(*input.PrinterID) == "" {
		input.PrinterID = nil
	}
	if input.Enabled && input.PrinterID == nil {
		return nil, invalid("Selecciona la impresora a la que está conectado el cajón.")
	}
	if input.PrinterID != nil {
		if err := validateID(*input.PrinterID); err != nil {
			return nil, err
		}
		if err := handler.requireDrawerPrinter(*input.PrinterID); err != nil {
			return nil, err
		}
	}
	previous, err := handler.readDrawerConfig(registerID)
	if err != nil {
		return nil, err
	}
	if previous.revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La configuración del cajón cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_drawer_settings(register_id,printer_id,pulse_pin,enabled)
        VALUES ($1,$2,$3,$4) ON CONFLICT (register_id) DO UPDATE SET printer_id=EXCLUDED.printer_id,pulse_pin=EXCLUDED.pulse_pin,
        enabled=EXCLUDED.enabled,revision=app.cash_drawer_settings.revision+1,updated_at=now()`,
		registerID, input.PrinterID, input.PulsePin, input.Enabled)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readDrawerConfig(registerID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, &registerID, nil); err != nil {
		return nil, err
	}
	result := map[string]any{
		"register_id": registerID, "register_name": saved.registerName, "printer_id": saved.printerID, "printer_name": saved.printerName,
		"pulse_pin": saved.pin, "enabled": saved.enabled, "revision": saved.revision,
	}
	return result, handler.audit(handler.ctx, handler.tx, "cash.drawer_settings_changed", "register", &registerID,
		map[string]any{"printer_id": previous.printerID, "pulse_pin": previous.pin, "enabled": previous.enabled},
		map[string]any{"printer_id": saved.printerID, "pulse_pin": saved.pin, "enabled": saved.enabled})
}

// requireDrawerPrinter exige una impresora existente, activa y con un transporte compatible.
func (handler *service) requireDrawerPrinter(printerID string) error {
	var name, transport string
	var active bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT name,transport,active FROM app.kitchen_printers WHERE id=$1", printerID).Scan(
		&name, &transport, &active)
	if isNoRows(err) {
		return invalid("La impresora seleccionada no existe.")
	}
	if err != nil {
		return err
	}
	if !kitchen.CompatibleDrawerTransport(transport) {
		return drawerProtocolUnsupported(name, transport)
	}
	if !active {
		return invalid("La impresora " + name + " está desactivada.")
	}
	return nil
}

// drawerEnabled indica si la caja tiene un cajón activado (sin datos de su impresora).
func (handler *service) drawerEnabled(registerID string) (bool, error) {
	config, err := handler.readDrawerConfig(registerID)
	var problem *Error
	if errors.As(err, &problem) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return config.enabled, nil
}
