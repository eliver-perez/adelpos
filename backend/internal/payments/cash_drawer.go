package payments

import (
	"context"
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
//     transporte entregue los bytes ESC/POS sin alterar: ESC/POS por red o, desde la etapa 7.1
//     (modo mixto), una cola CUPS que admita datos raw según lpstat y lpoptions (USB, serie o
//     AppSocket). Una cola sin controlador, IPP, una clase o el agente futuro se rechazan con su
//     motivo: no se asume que todos los dispositivos admiten el mismo protocolo.
//   - «Probar apertura» desde la configuración (etapa 7.1): mismo permiso, auditada y registrada
//     con el motivo test.
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
	// drawerTest es la prueba de apertura desde la configuración (etapa 7.1, migración 0009).
	drawerTest = "test"
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

// DrawerPrinter es una impresora candidata con su compatibilidad para el pulso del cajón. Para una
// cola CUPS, Connection es la conexión que informa CUPS (usb, socket, ipp…) y Reason explica por
// qué no admite datos raw; nunca se imprime nada para averiguarlo. TerminalName es la terminal que
// tiene conectada una impresora USB de otra terminal (etapa 9G, fase E).
type DrawerPrinter struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Transport    string `json:"transport"`
	Active       bool   `json:"active"`
	Compatible   bool   `json:"compatible"`
	Connection   string `json:"connection,omitempty"`
	Reason       string `json:"reason,omitempty"`
	TerminalName string `json:"terminal_name,omitempty"`
}

// drawerConfig es la configuración vigente del cajón de una caja con su impresora (desde la etapa 9G, fase E, con
// la terminal de una impresora USB conectada a otra terminal).
type drawerConfig struct {
	registerName string
	printerID    *string
	printerName  *string
	transport    *string
	destination  *string
	active       *bool
	agentDevice  *string
	pin          int
	enabled      bool
	revision     int
}

// readDrawerConfig lee la configuración del cajón de una caja; sin fila, revisión 0 y desactivado.
func (handler *service) readDrawerConfig(registerID string) (*drawerConfig, error) {
	config := &drawerConfig{}
	err := handler.tx.QueryRow(handler.ctx, `SELECT r.name,c.printer_id,p.name,p.transport,p.destination,p.active,p.agent_device_id::text,
        coalesce(c.pulse_pin,0),coalesce(c.enabled,false),coalesce(c.revision,0)
        FROM app.registers r LEFT JOIN app.cash_drawer_settings c ON c.register_id=r.id
        LEFT JOIN app.kitchen_printers p ON p.id=c.printer_id WHERE r.id=$1`, registerID).Scan(&config.registerName,
		&config.printerID, &config.printerName, &config.transport, &config.destination, &config.active, &config.agentDevice,
		&config.pin, &config.enabled, &config.revision)
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
	if !kitchen.CompatibleDrawerPrinter(*config.transport, config.agentDevice) {
		return "La impresora del cajón de " + config.registerName + " no usa ESC/POS por red, una cola CUPS ni la impresora de una terminal; no se envió el comando de apertura."
	}
	return ""
}

// queueInspector devuelve quien consulta las colas CUPS: el de la petición o, sin él, las
// consultas de solo lectura del equipo donde corre Go.
func (handler *service) queueInspector() kitchen.QueueInspector {
	if handler.request.Queues != nil {
		return handler.request.Queues
	}
	return kitchen.HostQueueInspector{}
}

// drawerProbeBudget limita el tiempo total de las consultas a CUPS de una petición: se ejecutan
// dentro de la transacción y con el lock operativo, que no deben retenerse por una cola que no
// responde.
const drawerProbeBudget = 4 * time.Second

// drawerPrinterCompatibility completa la compatibilidad de una impresora para el pulso del cajón.
// Una cola CUPS activa se consulta (solo lecturas); una desactivada no se consulta. La impresora de una terminal
// (etapa 9G, fase E) es compatible: su cola se eligió entre las que informó la terminal con la misma regla de
// datos raw y su agente envía el pulso tal cual.
func drawerPrinterCompatibility(ctx context.Context, inspector kitchen.QueueInspector, item *DrawerPrinter, destination string) {
	if item.Transport == "agent" && item.TerminalName != "" {
		item.Compatible = true
		item.Connection = "terminal"
		return
	}
	switch item.Transport {
	case "escpos_tcp":
		item.Compatible = true
		item.Connection = "network"
	case "cups":
		if !item.Active {
			item.Reason = "Impresora desactivada; su cola CUPS no se consultó."
			// En Windows (etapa 9E) la cola del equipo es la de la impresora instalada en Windows.
			if kitchen.SystemQueueOf(inspector) == kitchen.SystemQueueWindows {
				item.Reason = "Impresora desactivada; no se consultó en Windows."
			}
			return
		}
		capability := inspector.InspectQueue(ctx, destination)
		item.Compatible = capability.Checked && capability.Raw
		item.Connection = capability.Connection
		item.Reason = capability.Reason
	default:
		item.Reason = "Esta impresora no tiene una terminal asignada: no puede entregar el comando del cajón."
	}
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

// testDrawer atiende POST payments/cash/drawers/{caja}/test (cash.settings.manage, etapa 7.1):
// «Probar apertura» envía un pulso con la configuración guardada de esa caja, aunque el cajón aún
// no esté activado, para comprobar la conexión antes de activarlo. Se registra con el motivo test,
// ligado al turno activo si lo hay, se audita y es idempotente por operation_id: el reintento
// devuelve el mismo comando sin otro pulso. Como toda apertura, la respuesta solo informa que el
// comando se envió.
func (handler *service) testDrawer(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	registerID := parts[2]
	config, err := handler.readDrawerConfig(registerID)
	if err != nil {
		return nil, err
	}
	if config.printerID == nil {
		return nil, conflict("DRAWER_NOT_CONFIGURED", "Guarda primero la impresora a la que está conectado el cajón de "+config.registerName+".")
	}
	if message := drawerUnavailable(config); message != "" {
		return nil, conflict("DRAWER_UNAVAILABLE", message)
	}
	if *config.transport == "cups" {
		if err := handler.requireRawQueue(*config.printerName, *config.destination); err != nil {
			return nil, err
		}
	}
	shift, err := handler.activeShift(registerID)
	if err != nil {
		return nil, err
	}
	var shiftID *string
	if shift != nil {
		shiftID = &shift.ID
	}
	outcome, err := handler.queueDrawerCommand(registerID, shiftID, config, drawerTest, "Prueba de apertura desde la configuración del cajón")
	if err != nil {
		return nil, err
	}
	return outcome, handler.audit(handler.ctx, handler.tx, "cash.drawer_tested", "register", &registerID, nil, map[string]any{
		"reason": drawerTest, "printer_id": config.printerID, "job_id": outcome.JobID, "event_id": outcome.EventID, "shift_id": shiftID,
		"enabled": config.enabled,
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
	printerRows, err := handler.tx.Query(handler.ctx, `SELECT p.id,p.name,p.transport,p.destination,p.active,coalesce(d.name,'')
        FROM app.kitchen_printers p LEFT JOIN app.devices d ON d.id=p.agent_device_id ORDER BY p.name,p.id`)
	if err != nil {
		return nil, err
	}
	printers := []DrawerPrinter{}
	destinations := []string{}
	for printerRows.Next() {
		var item DrawerPrinter
		var destination string
		if err := printerRows.Scan(&item.ID, &item.Name, &item.Transport, &destination, &item.Active, &item.TerminalName); err != nil {
			printerRows.Close()
			return nil, err
		}
		printers = append(printers, item)
		destinations = append(destinations, destination)
	}
	printerRows.Close()
	if err := printerRows.Err(); err != nil {
		return nil, err
	}
	// Las colas CUPS se consultan después de leer las filas, con un tiempo total acotado.
	probe, cancel := context.WithTimeout(handler.ctx, drawerProbeBudget)
	defer cancel()
	inspector := handler.queueInspector()
	for index := range printers {
		drawerPrinterCompatibility(probe, inspector, &printers[index], destinations[index])
	}
	return map[string]any{"drawers": drawers, "printers": printers}, nil
}

// drawerProtocolUnsupported rechaza una impresora cuyo transporte no entrega el pulso sin alterar.
func drawerProtocolUnsupported(name, transport string) error {
	// Etapa 9G (fase E): la impresora de otra terminal sí abre el cajón cuando tiene su terminal elegida.
	if transport == "agent" {
		return &Error{
			Status: 400,
			Code:   "DRAWER_PROTOCOL_UNSUPPORTED",
			Message: "La impresora " + name + " no tiene una terminal asignada, así que no puede entregar el comando ESC/POS de apertura. " +
				"Elige en Cocina › Impresoras en qué terminal está conectada.",
		}
	}
	return &Error{
		Status: 400,
		Code:   "DRAWER_PROTOCOL_UNSUPPORTED",
		Message: "La impresora " + name + " usa " + transport + ", que no entrega el comando ESC/POS de apertura sin alterar. " +
			"El cajón se abre con una impresora ESC/POS por red, con una cola CUPS que admita datos raw o con la impresora USB de una terminal.",
	}
}

// drawerQueueRejected explica por qué una cola CUPS no sirve para el cajón: no se pudo consultar
// (DRAWER_QUEUE_UNAVAILABLE) o no admite datos raw (DRAWER_QUEUE_NOT_RAW).
func drawerQueueRejected(name string, capability kitchen.QueueCapability) error {
	if !capability.Checked {
		return &Error{Status: 409, Code: "DRAWER_QUEUE_UNAVAILABLE", Message: capability.Reason +
			" No se guardó la impresora " + name + " para el cajón."}
	}
	return &Error{Status: 400, Code: "DRAWER_QUEUE_NOT_RAW", Message: capability.Reason +
		" Elige otra cola o una impresora ESC/POS por red para el cajón."}
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
		// La cola CUPS se consulta solo si el cajón queda activado: desactivarlo nunca depende de
		// que CUPS responda; al volver a activarlo se consulta otra vez.
		if err := handler.requireDrawerPrinter(*input.PrinterID, input.Enabled); err != nil {
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

// requireDrawerPrinter exige una impresora existente, activa y con un transporte compatible. Con
// probeQueue, una cola CUPS se consulta además con lpstat y lpoptions (solo lecturas): si no
// admite datos raw, se rechaza con su motivo en lugar de fallar en silencio al primer cobro.
func (handler *service) requireDrawerPrinter(printerID string, probeQueue bool) error {
	var name, transport, destination string
	var active bool
	var agentDevice *string
	err := handler.tx.QueryRow(handler.ctx, "SELECT name,transport,destination,active,agent_device_id::text FROM app.kitchen_printers WHERE id=$1", printerID).Scan(
		&name, &transport, &destination, &active, &agentDevice)
	if isNoRows(err) {
		return invalid("La impresora seleccionada no existe.")
	}
	if err != nil {
		return err
	}
	if !kitchen.CompatibleDrawerPrinter(transport, agentDevice) {
		return drawerProtocolUnsupported(name, transport)
	}
	if !active {
		return invalid("La impresora " + name + " está desactivada.")
	}
	if transport == "cups" && probeQueue {
		return handler.requireRawQueue(name, destination)
	}
	return nil
}

// requireRawQueue consulta la cola CUPS del cajón con un tiempo acotado y la rechaza si no admite
// datos raw o no responde.
func (handler *service) requireRawQueue(name, queue string) error {
	probe, cancel := context.WithTimeout(handler.ctx, drawerProbeBudget)
	defer cancel()
	capability := handler.queueInspector().InspectQueue(probe, queue)
	if capability.Checked && capability.Raw {
		return nil
	}
	return drawerQueueRejected(name, capability)
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
