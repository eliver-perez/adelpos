package kitchen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

// Agente de impresión de las terminales (etapa 9G, fase E, §6 y decisión D13). Las impresoras de red y las del
// equipo servidor las sigue usando el trabajador de Go (worker.go). Una impresora USB conectada a otra terminal
// (transporte «agent» con su terminal) la imprime la aplicación de esa terminal mientras está abierta:
//
//   - informa las impresoras de su computadora (solo lecturas de su sistema de impresión): PUT print-agent/printers;
//   - reclama los trabajos de sus impresoras de la misma cola durable, con el mismo intento y lease de 60 s
//     confirmados antes de imprimir y los bytes ESC/POS ya preparados por Go: POST print-agent/jobs/claim;
//   - los imprime por la cola de su computadora (CUPS en macOS con lp -o raw y argumentos separados; Windows en
//     la Fase W) y confirma el resultado con las reglas de siempre (submitted = aceptado por la cola, nunca papel;
//     uncertain exige decisión humana y nunca se reenvía solo): POST print-agent/jobs/{id}/result.
//
// Estas rutas se autentican solo con la credencial de la terminal (sin sesión de usuario: el agente trabaja
// aunque nadie haya iniciado sesión en esa caja) y cada terminal solo ve, reclama y confirma los trabajos de sus
// propias impresoras. Si la terminal está apagada o sin red, sus trabajos esperan en la cola y el problema
// agent_offline lo dice (print_problems.go); al volver, su agente imprime lo pendiente.

// printerListQuery es una impresora con el nombre de la terminal que la tiene conectada (vacío si no es de una
// terminal), para la interfaz.
const printerListQuery = `SELECT to_jsonb(p)||jsonb_build_object('terminal_name',coalesce(agent_terminal.name,''))
        FROM app.kitchen_printers p LEFT JOIN app.devices agent_terminal ON agent_terminal.id=p.agent_device_id`

// agentLeaseSeconds es la lease de un trabajo reclamado por un agente: la misma de los trabajos del servidor. Si
// el agente no confirma el resultado antes de que venza, el trabajo pasa a «Resultado incierto» y nunca se
// reenvía solo.
const agentLeaseSeconds = 60

// maxLocalPrinters limita las impresoras que informa una terminal en cada informe.
const maxLocalPrinters = 64

var connectionPattern = regexp.MustCompile(`^[a-z0-9+.-]{0,20}$`)

// AgentRequest es una petición del agente de impresión de una terminal: la ruta sin el prefijo de la API, el
// cuerpo, la terminal autenticada por su credencial y la fuente de los bits del logotipo para preparar los
// documentos con imagen.
type AgentRequest struct {
	Method   string
	Route    string
	Body     json.RawMessage
	DeviceID string
	Rasters  documents.RasterSource
}

// HandleAgent atiende las rutas print-agent/* de la terminal ya autenticada. audit escribe en el Registro de
// movimientos con la terminal como autor.
func HandleAgent(ctx context.Context, tx pgx.Tx, request AgentRequest, audit Audit) (any, error) {
	if err := validID(request.DeviceID); err != nil {
		return nil, err
	}
	agent := agentService{ctx: ctx, tx: tx, request: request, audit: audit}
	parts := strings.Split(strings.TrimPrefix(request.Route, "print-agent/"), "/")
	switch {
	case request.Method == "PUT" && len(parts) == 1 && parts[0] == "printers":
		return agent.reportPrinters()
	case request.Method == "POST" && len(parts) == 2 && parts[0] == "jobs" && parts[1] == "claim":
		return agent.claim()
	case request.Method == "POST" && len(parts) == 3 && parts[0] == "jobs" && parts[2] == "result":
		if err := validID(parts[1]); err != nil {
			return nil, err
		}
		return agent.result(parts[1])
	}
	return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La ruta del agente de impresión no existe."}
}

type agentService struct {
	ctx     context.Context
	tx      pgx.Tx
	request AgentRequest
	audit   Audit
}

// LocalPrinter es una impresora como la informa la terminal: el nombre de su cola, el modelo, el tipo de conexión
// (solo el esquema de device-uri: usb, socket, ipp…; nunca la dirección) y el tipo de impresora de CUPS.
type LocalPrinter struct {
	Queue       string `json:"queue"`
	Model       string `json:"model"`
	Connection  string `json:"connection"`
	PrinterType *int64 `json:"printer_type"`
}

// reportPrinters reemplaza la lista de impresoras de la terminal. Solo se audita cuando la lista cambia: el agente
// la informa al abrir la aplicación y cada pocos minutos.
func (agent agentService) reportPrinters() (any, error) {
	var input struct {
		Printers []LocalPrinter `json:"printers"`
	}
	if err := decode(agent.request.Body, &input); err != nil {
		return nil, err
	}
	if input.Printers == nil || len(input.Printers) > maxLocalPrinters {
		return nil, invalid("La lista de impresoras de la terminal no es válida.")
	}
	seen := map[string]bool{}
	for index := range input.Printers {
		printer := &input.Printers[index]
		printer.Model = strings.TrimSpace(cleanText(printer.Model))
		if len([]rune(printer.Model)) > 120 {
			printer.Model = string([]rune(printer.Model)[:120])
		}
		printer.Connection = strings.ToLower(strings.TrimSpace(printer.Connection))
		if !queuePattern.MatchString(printer.Queue) || !connectionPattern.MatchString(printer.Connection) || seen[printer.Queue] {
			return nil, invalid("La lista de impresoras de la terminal no es válida.")
		}
		seen[printer.Queue] = true
	}
	slices.SortFunc(input.Printers, func(left, right LocalPrinter) int {
		return strings.Compare(left.Queue, right.Queue)
	})
	previous, err := many[LocalPrinter](agent.ctx, agent.tx, `SELECT jsonb_build_object('queue',queue,'model',model,
        'connection',connection,'printer_type',printer_type) FROM app.device_local_printers WHERE device_id=$1 ORDER BY queue`,
		agent.request.DeviceID)
	if err != nil {
		return nil, err
	}
	if _, err := agent.tx.Exec(agent.ctx, "DELETE FROM app.device_local_printers WHERE device_id=$1", agent.request.DeviceID); err != nil {
		return nil, err
	}
	for _, printer := range input.Printers {
		if _, err := agent.tx.Exec(agent.ctx, `INSERT INTO app.device_local_printers(device_id,queue,model,connection,printer_type)
            VALUES ($1,$2,$3,$4,$5)`, agent.request.DeviceID, printer.Queue, printer.Model, printer.Connection, printer.PrinterType); err != nil {
			return nil, err
		}
	}
	if _, err := agent.tx.Exec(agent.ctx, "UPDATE app.devices SET local_printers_reported_at=now() WHERE id=$1", agent.request.DeviceID); err != nil {
		return nil, err
	}
	if !sameLocalPrinters(previous, input.Printers) {
		// Sistema › Terminales y Cocina › Impresoras vuelven a consultar la lista.
		if err := events.Append(agent.ctx, agent.tx, "devices.printers.changed", "configuration", &agent.request.DeviceID, nil); err != nil {
			return nil, err
		}
		if err := agent.audit(agent.ctx, agent.tx, "device.local_printers_reported", "device", &agent.request.DeviceID,
			map[string]any{"printers": queueNames(previous)}, map[string]any{"printers": queueNames(input.Printers)}); err != nil {
			return nil, err
		}
	}
	queues := []TerminalQueue{}
	for _, printer := range input.Printers {
		queues = append(queues, EvaluateTerminalQueue(printer.Queue, printer.Connection, printer.Model, printer.PrinterType))
	}
	return map[string]any{"printers": queues}, nil
}

func sameLocalPrinters(left, right []LocalPrinter) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		sameType := (a.PrinterType == nil && b.PrinterType == nil) ||
			(a.PrinterType != nil && b.PrinterType != nil && *a.PrinterType == *b.PrinterType)
		if a.Queue != b.Queue || a.Model != b.Model || a.Connection != b.Connection || !sameType {
			return false
		}
	}
	return true
}

func queueNames(printers []LocalPrinter) []string {
	names := []string{}
	for _, printer := range printers {
		names = append(names, printer.Queue)
	}
	return names
}

// AgentJob es un trabajo reclamado por el agente: la cola de su computadora, el título técnico del trabajo y los
// bytes ESC/POS exactos que debe entregar en modo raw (todas las copias), que Go ya preparó.
type AgentJob struct {
	ID         string    `json:"id"`
	AttemptID  string    `json:"attempt_id"`
	Queue      string    `json:"queue"`
	Title      string    `json:"title"`
	Data       string    `json:"data"`
	Size       int       `json:"size"`
	Drawer     bool      `json:"drawer"`
	LeaseUntil time.Time `json:"lease_until"`
}

// agentJobColumns es un trabajo con lo que necesita su envío: el tipo de ticket, el tamaño de texto y el pulso del
// cajón, como los lee el trabajador del servidor.
const agentJobColumns = `SELECT to_jsonb(j)||jsonb_build_object(
		'text_size',coalesce(t.document->>'text_size',t.document->'settings'->>'text_size'),
		'ticket_kind',t.kind,
		'drawer_pulse',CASE WHEN t.kind='cash_drawer' THEN t.document->'drawer_pulse' END)
		FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id`

// claim reclama el siguiente trabajo de las impresoras de la terminal. Repetir la misma claim_id (el agente no
// recibió la respuesta) devuelve el mismo trabajo y el mismo intento mientras su lease siga vigente: nunca se
// crean dos intentos por una respuesta perdida. Un trabajo por impresora a la vez, en orden de llegada.
func (agent agentService) claim() (any, error) {
	var input struct {
		ClaimID string `json:"claim_id"`
	}
	if err := decode(agent.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validID(input.ClaimID); err != nil {
		return nil, err
	}
	device := agent.request.DeviceID
	// La última consulta del agente es la señal de que la terminal está conectada (print_problems.go).
	if _, err := agent.tx.Exec(agent.ctx, "UPDATE app.devices SET print_agent_seen_at=now() WHERE id=$1", device); err != nil {
		return nil, err
	}
	repeated, err := many[PrintJob](agent.ctx, agent.tx, agentJobColumns+` JOIN app.kitchen_print_attempts a ON a.job_id=j.id
        WHERE j.state='processing' AND j.lease_until>now() AND a.finished_at IS NULL AND a.result->>'claim_id'=$1
        AND a.result->>'agent_device_id'=$2 FOR UPDATE OF j`, input.ClaimID, device)
	if err != nil {
		return nil, err
	}
	if len(repeated) > 0 {
		var attemptID string
		if err := agent.tx.QueryRow(agent.ctx, `SELECT id FROM app.kitchen_print_attempts WHERE job_id=$1 AND finished_at IS NULL
            ORDER BY attempt DESC LIMIT 1`, repeated[0].ID).Scan(&attemptID); err != nil {
			return nil, err
		}
		return agent.deliver(repeated[0], attemptID)
	}
	// Un trabajo de esta terminal cuyo intento venció sin resultado pasa a «Resultado incierto» antes de reclamar
	// otro de la misma impresora: nunca se reenvía solo.
	expired, err := many[PrintJob](agent.ctx, agent.tx, agentJobColumns+` WHERE j.state='processing' AND j.lease_until<now()
        AND `+agentDeviceSQL("j")+`=$1 FOR UPDATE OF j SKIP LOCKED`, device)
	if err != nil {
		return nil, err
	}
	for _, job := range expired {
		if err := expireAgentLease(agent.ctx, agent.tx, job.ID); err != nil {
			return nil, err
		}
	}
	// Mismas reglas de reintento que el trabajador del servidor: un error confirmado se reintenta solo hasta tres
	// veces con espera creciente, salvo el comando del cajón.
	candidates, err := many[PrintJob](agent.ctx, agent.tx, agentJobColumns+` WHERE `+agentDeviceSQL("j")+`=$1
        AND (j.state='pending' OR (j.state='error' AND j.attempts<3 AND j.next_attempt_at<=now() AND t.kind<>'cash_drawer'))
        AND NOT EXISTS(SELECT FROM app.kitchen_print_jobs busy WHERE busy.printer_id=j.printer_id AND busy.state='processing')
        ORDER BY j.created_at,j.id LIMIT 20 FOR UPDATE OF j SKIP LOCKED`, device)
	if err != nil {
		return nil, err
	}
	busy := map[string]bool{}
	for _, job := range candidates {
		if busy[job.PrinterID] {
			continue
		}
		// El mismo lock por impresora de los trabajadores (7820148307), solo durante esta transacción: dos reclamos
		// simultáneos de la misma terminal no toman dos trabajos de una impresora.
		var locked bool
		if err := agent.tx.QueryRow(agent.ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,7820148307))", job.PrinterID).Scan(&locked); err != nil {
			return nil, err
		}
		if !locked {
			busy[job.PrinterID] = true
			continue
		}
		attemptID := events.NewID()
		job.Attempts++
		if _, err := agent.tx.Exec(agent.ctx, `UPDATE app.kitchen_print_jobs SET state='processing',attempts=attempts+1,
            last_attempt_at=now(),lease_until=now()+make_interval(secs=>$2),revision=revision+1,updated_at=now() WHERE id=$1`,
			job.ID, agentLeaseSeconds); err != nil {
			return nil, err
		}
		if _, err := agent.tx.Exec(agent.ctx, `INSERT INTO app.kitchen_print_attempts(id,job_id,attempt,result) VALUES ($1,$2,$3,$4)`,
			attemptID, job.ID, job.Attempts, encoded(map[string]string{"claim_id": input.ClaimID, "agent_device_id": device})); err != nil {
			return nil, err
		}
		// Como el trabajador del servidor: una impresora o una estación desactivada, o una fotografía que ya no es
		// válida, no se envía; el trabajo queda en intervención con su motivo.
		if outcome, ok, err := agent.blocked(job); err != nil {
			return nil, err
		} else if !ok {
			if err := storeOutcome(agent.ctx, agent.tx, job, attemptID, outcome, device); err != nil {
				return nil, err
			}
			busy[job.PrinterID] = true
			continue
		}
		if err := events.Append(agent.ctx, agent.tx, "printing.processing", "printing", &job.ID, nil); err != nil {
			return nil, err
		}
		if err := deviceHistory(agent.ctx, agent.tx, "print.agent_claimed", job.ID, device, map[string]any{
			"attempt_id": attemptID, "attempt": job.Attempts}); err != nil {
			return nil, err
		}
		return agent.deliver(job, attemptID)
	}
	return map[string]any{"job": nil}, nil
}

// blocked revisa, antes de entregar el trabajo, lo mismo que el trabajador del servidor: la impresora y las
// estaciones del documento siguen activas y la fotografía de la impresora es válida.
func (agent agentService) blocked(job PrintJob) (Outcome, bool, error) {
	var active bool
	err := agent.tx.QueryRow(agent.ctx, `SELECT p.active AND NOT EXISTS(
		SELECT FROM app.kitchen_tickets t JOIN app.kitchen_stations s ON s.id=ANY(t.station_ids)
		WHERE t.id=$2 AND NOT s.active) FROM app.kitchen_printers p WHERE p.id=$1`, job.PrinterID, job.TicketID).Scan(&active)
	if err != nil {
		return Outcome{}, false, err
	}
	if !active {
		return Outcome{State: "intervention", Code: ProblemInactive, Message: "La impresora o una estación fue desactivada; el documento permanece en cola."}, false, nil
	}
	snapshot := job.PrinterSnapshot
	if err := validatePrinter(&snapshot); err != nil || !snapshot.AgentPrinter() {
		return Outcome{State: "intervention", Code: ProblemSetup, Message: "El destino guardado requiere revisión."}, false, nil
	}
	return Outcome{}, true, nil
}

// deliver prepara la respuesta del reclamo con los bytes exactos que el agente debe entregar.
func (agent agentService) deliver(job PrintJob, attemptID string) (any, error) {
	var leaseUntil time.Time
	if err := agent.tx.QueryRow(agent.ctx, "SELECT lease_until FROM app.kitchen_print_jobs WHERE id=$1", job.ID).Scan(&leaseUntil); err != nil {
		return nil, err
	}
	data := AgentJobBytes(agent.ctx, job, agent.request.Rasters)
	title := "ADEL-" + job.ID
	if job.DrawerPulse != nil {
		title = "ADEL-cajon-" + job.ID
	}
	return map[string]any{"job": AgentJob{
		ID: job.ID, AttemptID: attemptID, Queue: job.PrinterSnapshot.Destination, Title: title,
		Data: base64.StdEncoding.EncodeToString(data), Size: len(data), Drawer: job.DrawerPulse != nil, LeaseUntil: leaseUntil,
	}}, nil
}

// AgentJobBytes son los bytes ESC/POS de un trabajo para la impresora de una terminal, los mismos que recibe una
// impresora ESC/POS por red o la cola de Windows (etapa 9E): el pulso del cajón (una sola vez, sin texto ni
// corte), el documento con formato o logotipo de la IR o el texto con el codificador CP850 de siempre, con el
// final de papel de la impresora. Las copias van repetidas en los mismos bytes: el agente envía un solo trabajo.
func AgentJobBytes(ctx context.Context, job PrintJob, source documents.RasterSource) []byte {
	if job.DrawerPulse != nil {
		return DrawerPulseBytes(job.DrawerPulse.Pin)
	}
	printer := job.PrinterSnapshot
	var data []byte
	if job.Layout != nil && !job.Layout.Plain() {
		job.Rasters = loadLayoutRasters(ctx, source, job.Layout)
		rasters, _ := printableRasters(job)
		data = documents.EncodeESCPos(job.Layout, rasters, printer.PaperCut())
	} else {
		data = ESCPosBytes(job.Content, printer, job.TextSize)
	}
	copies := max(1, min(printer.Copies, 3))
	result := make([]byte, 0, len(data)*copies)
	for range copies {
		result = append(result, data...)
	}
	return result
}

// loadLayoutRasters carga los bits de las imágenes de un documento; una imagen que no se puede cargar se omite.
func loadLayoutRasters(ctx context.Context, source documents.RasterSource, layout *documents.Layout) map[string]*documents.Raster {
	rasters := map[string]*documents.Raster{}
	if layout == nil || source == nil {
		return rasters
	}
	for _, line := range layout.Lines {
		if line.Image == nil || rasters[line.Image.RasterID] != nil {
			continue
		}
		raster, err := source.Raster(ctx, line.Image.RasterID)
		if err == nil && raster != nil {
			rasters[line.Image.RasterID] = raster
		}
	}
	return rasters
}

// Resultados que informa el agente: aceptado por la cola de su computadora, error confirmado sin enviar nada o
// incierto (lp empezó y no devolvió un resultado inequívoco).
var agentErrorCodes = []string{ProblemNotFound, ProblemUnreachable, ProblemSetup}

// result guarda el resultado de un intento con las reglas de siempre. Repetir el mismo resultado (el agente no
// recibió la respuesta) responde lo guardado sin cambiar nada; un resultado que llega con la lease vencida no se
// acepta: el trabajo queda en «Resultado incierto» para que una persona revise el papel, y el informe tardío queda
// en el historial del trabajo.
func (agent agentService) result(jobID string) (any, error) {
	var input struct {
		AttemptID string `json:"attempt_id"`
		Outcome   string `json:"outcome"`
		Code      string `json:"code"`
		Reference string `json:"reference"`
		Detail    string `json:"detail"`
	}
	if err := decode(agent.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validID(input.AttemptID); err != nil {
		return nil, err
	}
	device := agent.request.DeviceID
	if _, err := agent.tx.Exec(agent.ctx, "UPDATE app.devices SET print_agent_seen_at=now() WHERE id=$1", device); err != nil {
		return nil, err
	}
	job, err := one[PrintJob](agent.ctx, agent.tx, agentJobColumns+` WHERE j.id=$1 AND `+agentDeviceSQL("j")+`=$2 FOR UPDATE OF j`, jobID, device)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nunca se confirma un trabajo de otra terminal (ni se dice si existe).
		return nil, &Error{Status: 404, Code: "PRINT_JOB_NOT_FOUND", Message: "El trabajo no pertenece a las impresoras de esta terminal."}
	}
	if err != nil {
		return nil, err
	}
	var finished bool
	var stored string
	var leaseValid bool
	err = agent.tx.QueryRow(agent.ctx, `SELECT a.finished_at IS NOT NULL, a.outcome,
        j.state='processing' AND j.lease_until>=now() AND a.finished_at IS NULL
        FROM app.kitchen_print_attempts a JOIN app.kitchen_print_jobs j ON j.id=a.job_id
        WHERE a.id=$1 AND a.job_id=$2 AND a.result->>'agent_device_id'=$3`, input.AttemptID, jobID, device).Scan(&finished, &stored, &leaseValid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Status: 404, Code: "PRINT_JOB_NOT_FOUND", Message: "El intento no pertenece a las impresoras de esta terminal."}
	}
	if err != nil {
		return nil, err
	}
	terminal, err := agent.terminalName()
	if err != nil {
		return nil, err
	}
	outcome, err := agentOutcome(input.Outcome, input.Code, input.Reference, job, terminal)
	if err != nil {
		return nil, err
	}
	if finished {
		// El mismo resultado repetido es la respuesta perdida de un envío anterior. Si el intento quedó incierto
		// porque venció su lease, se responde lo mismo que al informe tardío. Otro resultado distinto es un error
		// del agente y no cambia nada.
		if stored == outcome.State {
			return map[string]any{"state": job.State, "recorded": true}, nil
		}
		if stored == "uncertain" {
			return map[string]any{"state": job.State, "recorded": false, "reason": "lease_expired"}, nil
		}
		return nil, &Error{Status: 409, Code: "PRINT_RESULT_CONFLICT", Message: "Ese intento ya tiene otro resultado guardado."}
	}
	if !leaseValid {
		if job.State == "processing" {
			if err := expireAgentLease(agent.ctx, agent.tx, job.ID); err != nil {
				return nil, err
			}
		}
		if err := deviceHistory(agent.ctx, agent.tx, "print.agent_late_result", job.ID, device, map[string]any{
			"attempt_id": input.AttemptID, "outcome": outcome, "detail": shortDetail(input.Detail)}); err != nil {
			return nil, err
		}
		return map[string]any{"state": "uncertain", "recorded": false, "reason": "lease_expired"}, nil
	}
	if err := storeOutcome(agent.ctx, agent.tx, *job, input.AttemptID, outcome, device); err != nil {
		return nil, err
	}
	if detail := shortDetail(input.Detail); detail != "" {
		if err := deviceHistory(agent.ctx, agent.tx, "print.agent_detail", job.ID, device, map[string]any{
			"attempt_id": input.AttemptID, "detail": detail}); err != nil {
			return nil, err
		}
	}
	var state string
	if err := agent.tx.QueryRow(agent.ctx, "SELECT state FROM app.kitchen_print_jobs WHERE id=$1", job.ID).Scan(&state); err != nil {
		return nil, err
	}
	return map[string]any{"state": state, "recorded": true}, nil
}

func (agent agentService) terminalName() (string, error) {
	var name string
	err := agent.tx.QueryRow(agent.ctx, "SELECT name FROM app.devices WHERE id=$1", agent.request.DeviceID).Scan(&name)
	return name, err
}

// shortDetail es el detalle técnico que el agente agrega para soporte, sin controles y acotado.
func shortDetail(detail string) string {
	detail = strings.TrimSpace(cleanText(detail))
	if len([]rune(detail)) > 300 {
		detail = string([]rune(detail)[:300])
	}
	return detail
}

// agentOutcome valida el resultado informado y lo traduce al de la cola con los textos de Go.
func agentOutcome(state, code, reference string, job *PrintJob, terminal string) (Outcome, error) {
	where := "de " + terminal
	switch state {
	case "submitted":
		queue := job.PrinterSnapshot.Destination
		if reference == "" || reference != cupsJobReference(queue, []byte(reference)) {
			return Outcome{}, invalid("El resultado del agente no es válido.")
		}
		message := "Aceptado por la cola de impresión " + where + "; no confirma salida física de papel."
		if job.DrawerPulse != nil {
			message = "Comando de apertura aceptado por la cola de impresión " + where + "; no confirma que el cajón se abrió."
		}
		return Outcome{State: "submitted", Reference: reference, Accepted: true, Message: message}, nil
	case "error":
		if !slices.Contains(agentErrorCodes, code) || reference != "" {
			return Outcome{}, invalid("El resultado del agente no es válido.")
		}
		message := "La cola de impresión " + where + " no respondió; no se envió nada."
		switch code {
		case ProblemNotFound:
			message = "No se encontró la impresora en la computadora " + where + "; no se envió nada."
		case ProblemSetup:
			message = "La impresora " + where + " no recibe los datos sin convertir; no se envió nada."
		}
		return Outcome{State: "error", Code: code, Message: message}, nil
	case "uncertain":
		if code != "" || reference != "" {
			return Outcome{}, invalid("El resultado del agente no es válido.")
		}
		return Outcome{State: "uncertain", Code: ProblemUncertain,
			Message: "La cola de impresión " + where + " no devolvió un resultado inequívoco. Revisa el papel antes de autorizar otra copia."}, nil
	}
	return Outcome{}, invalid("El resultado del agente no es válido.")
}

// expireAgentLease deja en «Resultado incierto» un trabajo de una terminal cuya lease venció sin resultado: el
// agente pudo haberlo enviado a su cola antes de perder la conexión o cerrarse, así que nunca se reenvía solo.
func expireAgentLease(ctx context.Context, tx pgx.Tx, jobID string) error {
	if _, err := tx.Exec(ctx, `UPDATE app.kitchen_print_jobs SET state='uncertain',lease_until=NULL,
        last_error='La terminal no informó el resultado a tiempo; revisar el papel antes de autorizar otra copia.',
        result=jsonb_build_object('state','uncertain','code','uncertain'),revision=revision+1,updated_at=now()
        WHERE id=$1 AND state='processing'`, jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.kitchen_print_attempts SET outcome='uncertain',finished_at=now(),
        detail='La terminal no informó el resultado antes de vencer el plazo.',
        result=result||jsonb_build_object('state','uncertain','code','uncertain') WHERE job_id=$1 AND finished_at IS NULL`, jobID); err != nil {
		return err
	}
	if err := systemHistory(ctx, tx, "print.recovered_uncertain", jobID, map[string]string{"reason": "agent_lease_expired"}); err != nil {
		return err
	}
	return events.Append(ctx, tx, "printing.uncertain", "printing", &jobID, nil)
}

// deviceHistory escribe en el historial de cocina un paso del agente con la terminal como autor.
func deviceHistory(ctx context.Context, tx pgx.Tx, kind, id, deviceID string, after any) error {
	_, err := tx.Exec(ctx, `INSERT INTO app.kitchen_history(id,device_id,entity_id,kind,after_value)
        VALUES ($1,$2,$3,$4,$5)`, events.NewID(), deviceID, id, kind, encoded(after))
	return err
}

// TerminalPrinterSet son las impresoras de una terminal: las que informó su computadora (con si sirven) y las
// impresoras de AD-EL POS que tiene asignadas, con su agente de impresión en línea o no.
type TerminalPrinterSet struct {
	DeviceID    string           `json:"device_id"`
	DeviceName  string           `json:"device_name"`
	AgentOnline bool             `json:"agent_online"`
	AgentSeenAt *time.Time       `json:"agent_seen_at"`
	ReportedAt  *time.Time       `json:"reported_at"`
	Queues      []TerminalQueue  `json:"queues"`
	Printers    []AssignedOutput `json:"printers"`
}

// AssignedOutput es una impresora de AD-EL POS conectada a la terminal.
type AssignedOutput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Active      bool   `json:"active"`
}

// TerminalPrinters lee las impresoras de las terminales no revocadas (o solo de una, con deviceID).
func TerminalPrinters(ctx context.Context, tx pgx.Tx, deviceID string) ([]TerminalPrinterSet, error) {
	rows, err := tx.Query(ctx, `SELECT d.id::text,d.name,d.print_agent_seen_at,d.local_printers_reported_at,
        coalesce(d.print_agent_seen_at > now() - make_interval(secs => $2),false)
        FROM app.devices d WHERE d.revoked_at IS NULL AND ($1='' OR d.id::text=$1) ORDER BY d.name,d.id`, deviceID, AgentOfflineSeconds)
	if err != nil {
		return nil, err
	}
	sets := []TerminalPrinterSet{}
	for rows.Next() {
		set := TerminalPrinterSet{Queues: []TerminalQueue{}, Printers: []AssignedOutput{}}
		if err := rows.Scan(&set.DeviceID, &set.DeviceName, &set.AgentSeenAt, &set.ReportedAt, &set.AgentOnline); err != nil {
			rows.Close()
			return nil, err
		}
		sets = append(sets, set)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range sets {
		set := &sets[index]
		reported, err := many[LocalPrinter](ctx, tx, `SELECT jsonb_build_object('queue',queue,'model',model,'connection',connection,
            'printer_type',printer_type) FROM app.device_local_printers WHERE device_id=$1 ORDER BY queue`, set.DeviceID)
		if err != nil {
			return nil, err
		}
		for _, printer := range reported {
			set.Queues = append(set.Queues, EvaluateTerminalQueue(printer.Queue, printer.Connection, printer.Model, printer.PrinterType))
		}
		set.Printers, err = many[AssignedOutput](ctx, tx, `SELECT jsonb_build_object('id',id,'name',name,'destination',destination,
            'active',active) FROM app.kitchen_printers WHERE agent_device_id=$1 ORDER BY name`, set.DeviceID)
		if err != nil {
			return nil, err
		}
	}
	return sets, nil
}

// checkAgentPrinter valida la terminal de una impresora «agent»: existe y no está revocada y, al elegirla o cambiar
// de cola, la cola es una de las que informó esa terminal y sirve para tickets (EvaluateTerminalQueue). Cambiar
// solo el nombre o el papel de una impresora ya guardada no exige que la terminal esté conectada.
func (handler *service) checkAgentPrinter(printer *Printer, before *Printer) error {
	if !printer.AgentPrinter() {
		return nil
	}
	var revoked bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT revoked_at IS NOT NULL FROM app.devices WHERE id=$1", *printer.AgentDeviceID).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("La terminal elegida no existe.")
	}
	if err != nil {
		return err
	}
	if revoked {
		return invalid("La terminal elegida ya no tiene autorización; elige otra.")
	}
	unchanged := before != nil && before.AgentPrinter() && *before.AgentDeviceID == *printer.AgentDeviceID &&
		before.Destination == printer.Destination
	if unchanged {
		return nil
	}
	reported, err := many[LocalPrinter](handler.ctx, handler.tx, `SELECT jsonb_build_object('queue',queue,'model',model,
        'connection',connection,'printer_type',printer_type) FROM app.device_local_printers WHERE device_id=$1 AND queue=$2`,
		*printer.AgentDeviceID, printer.Destination)
	if err != nil {
		return err
	}
	if len(reported) == 0 {
		return invalid("Esa terminal no informó esa impresora. Abre AD-EL POS en la terminal con la impresora conectada y vuelve a elegirla.")
	}
	queue := EvaluateTerminalQueue(reported[0].Queue, reported[0].Connection, reported[0].Model, reported[0].PrinterType)
	if !queue.Usable {
		return invalid("La impresora " + queue.Queue + " no sirve para tickets: " + queue.Reason)
	}
	return nil
}
