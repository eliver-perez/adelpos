package kitchen

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
)

type Worker struct {
	Pool      *pgxpool.Pool
	Transport Transport
	Logger    *slog.Logger
	// Rasters carga los bits del logotipo de los documentos que lo llevan (etapa 9B; la tabla y su
	// implementación son de la fase B). Sin valor, la imagen se omite con aviso y el documento se
	// imprime igual.
	Rasters documents.RasterSource
}

func (worker *Worker) Run(ctx context.Context) {
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := worker.ReleaseDue(ctx); err != nil && ctx.Err() == nil && worker.Logger != nil {
				worker.Logger.Warn("scheduled_release_unavailable")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	for index := 0; index < 2; index++ {
		go func() {
			defer wait.Done()
			ticker := time.NewTicker(300 * time.Millisecond)
			defer ticker.Stop()
			for {
				if _, err := worker.PrintNext(ctx); err != nil && ctx.Err() == nil && worker.Logger != nil {
					worker.Logger.Warn("print_queue_unavailable")
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	wait.Wait()
}

func (worker *Worker) ReleaseDue(ctx context.Context) error {
	tx, err := worker.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
		return err
	}
	releases, err := many[Release](ctx, tx, `SELECT to_jsonb(r) FROM app.kitchen_releases r
        WHERE state='scheduled' AND automatic AND due_at<=now() ORDER BY due_at,id LIMIT 20 FOR UPDATE`)
	if err != nil {
		return err
	}
	handler := service{ctx: ctx, tx: tx}
	for index := range releases {
		release := &releases[index]
		if err := handler.prepareRelease(release); err != nil {
			return err
		}
		if err := systemHistory(ctx, tx, "release.automatic", release.ID, map[string]any{"scheduled_at": release.ScheduledAt}); err != nil {
			return err
		}
		if err := events.Append(ctx, tx, "kitchen.schedule.executed", "kitchen", &release.ID, nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func systemHistory(ctx context.Context, tx pgx.Tx, kind, id string, after any) error {
	_, err := tx.Exec(ctx, `INSERT INTO app.kitchen_history(id,entity_id,kind,after_value)
        VALUES ($1,$2,$3,$4)`, events.NewID(), id, kind, encoded(after))
	return err
}

type ownedJob struct {
	Connection  *pgxpool.Conn
	Job         PrintJob
	AttemptID   string
	PrinterLock string
}

func releasePrinterLock(connection *pgxpool.Conn, printerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,7820148307))", printerID); err != nil {
		// Un lock de sesión nunca debe regresar al pool si no se pudo liberar.
		_ = connection.Conn().Close(ctx)
	}
}

func (worker *Worker) claim(ctx context.Context) (*ownedJob, error) {
	connection, err := worker.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	keep := false
	heldPrinter := ""
	defer func() {
		if !keep {
			if heldPrinter != "" {
				releasePrinterLock(connection, heldPrinter)
			}
			connection.Release()
		}
	}()
	tx, err := connection.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	// Un comando del cajón con error confirmado no se reintenta solo: un cajón que se abre minutos
	// después, sin nadie que lo atienda, es un riesgo físico. Se resuelve desde la cola o se vuelve
	// a abrir manualmente desde Caja, con su permiso y motivo.
	candidates, err := many[PrintJob](ctx, tx, `SELECT to_jsonb(j)||jsonb_build_object(
		'text_size',coalesce(t.document->>'text_size',t.document->'settings'->>'text_size'),
		'ticket_kind',t.kind,
		'drawer_pulse',CASE WHEN t.kind='cash_drawer' THEN t.document->'drawer_pulse' END)
		FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
        WHERE (j.state='pending' OR (j.state='error' AND j.attempts<3 AND j.next_attempt_at<=now() AND t.kind<>'cash_drawer')
        OR (j.state='processing' AND j.lease_until<now())) ORDER BY j.created_at,j.id LIMIT 20 FOR UPDATE OF j SKIP LOCKED`)
	if err != nil {
		return nil, err
	}
	for _, job := range candidates {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,7820148307))", job.PrinterID).Scan(&locked); err != nil {
			return nil, err
		}
		if !locked {
			continue
		}
		heldPrinter = job.PrinterID
		if job.State == "processing" {
			if _, err := tx.Exec(ctx, `UPDATE app.kitchen_print_jobs SET state='uncertain',lease_until=NULL,
                last_error='El trabajador se interrumpió sin guardar resultado; revisar antes de autorizar otra copia.',
                revision=revision+1,updated_at=now() WHERE id=$1`, job.ID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE app.kitchen_print_attempts SET outcome='uncertain',finished_at=now(),
                detail='Resultado desconocido tras interrupción.' WHERE job_id=$1 AND finished_at IS NULL`, job.ID); err != nil {
				return nil, err
			}
			if err := systemHistory(ctx, tx, "print.recovered_uncertain", job.ID, nil); err != nil {
				return nil, err
			}
			if err := events.Append(ctx, tx, "printing.uncertain", "printing", &job.ID, nil); err != nil {
				return nil, err
			}
			err := tx.Commit(ctx)
			return nil, err
		}
		attemptID := events.NewID()
		job.Attempts++
		if _, err := tx.Exec(ctx, `UPDATE app.kitchen_print_jobs SET state='processing',attempts=attempts+1,
            last_attempt_at=now(),lease_until=now()+interval '60 seconds',revision=revision+1,updated_at=now() WHERE id=$1`, job.ID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO app.kitchen_print_attempts(id,job_id,attempt) VALUES ($1,$2,$3)`, attemptID, job.ID, job.Attempts); err != nil {
			return nil, err
		}
		if err := events.Append(ctx, tx, "printing.processing", "printing", &job.ID, nil); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		keep = true
		return &ownedJob{Connection: connection, Job: job, AttemptID: attemptID, PrinterLock: job.PrinterID}, nil
	}
	return nil, nil
}

// loadRasters carga los bits de las imágenes de un documento con atributos. Una imagen que no se
// puede cargar se omite: el transporte imprime el resto del documento y lo avisa.
func (worker *Worker) loadRasters(ctx context.Context, layout *documents.Layout) map[string]*documents.Raster {
	rasters := map[string]*documents.Raster{}
	if layout == nil || worker.Rasters == nil {
		return rasters
	}
	for _, line := range layout.Lines {
		if line.Image == nil || rasters[line.Image.RasterID] != nil {
			continue
		}
		raster, err := worker.Rasters.Raster(ctx, line.Image.RasterID)
		if err == nil && raster != nil {
			rasters[line.Image.RasterID] = raster
		}
	}
	return rasters
}

func (worker *Worker) PrintNext(ctx context.Context) (bool, error) {
	owned, err := worker.claim(ctx)
	if err != nil || owned == nil {
		return false, err
	}
	defer owned.Connection.Release()
	defer releasePrinterLock(owned.Connection, owned.PrinterLock)
	transport := worker.Transport
	if transport == nil {
		transport = HostTransport{}
	}
	attemptContext, cancel := context.WithTimeout(ctx, 12*time.Second)
	var active bool
	checkError := owned.Connection.QueryRow(attemptContext, `SELECT p.active AND NOT EXISTS(
		SELECT FROM app.kitchen_tickets t JOIN app.kitchen_stations s ON s.id=ANY(t.station_ids)
		WHERE t.id=$2 AND NOT s.active) FROM app.kitchen_printers p WHERE p.id=$1`, owned.Job.PrinterID, owned.Job.TicketID).Scan(&active)
	outcome := Outcome{State: "intervention", Message: "La impresora o una estación fue desactivada; el documento permanece en cola."}
	if checkError != nil {
		outcome = Outcome{State: "error", Message: "No se pudo validar el destino; no se envió el documento."}
	}
	if checkError == nil && active {
		owned.Job.Rasters = worker.loadRasters(attemptContext, owned.Job.Layout)
		outcome = transport.Submit(attemptContext, owned.Job)
	}
	cancel()
	// Se intenta guardar el resultado aun durante SIGTERM; si PostgreSQL falla, la lease
	// queda para reconciliación conservadora y nunca para reenvío automático ciego.
	storeContext, done := context.WithTimeout(context.Background(), 4*time.Second)
	defer done()
	tx, err := owned.Connection.Begin(storeContext)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(context.Background())
	state := outcome.State
	if state != "submitted" && state != "error" && state != "uncertain" && state != "intervention" {
		state = "uncertain"
	}
	_, err = tx.Exec(storeContext, `UPDATE app.kitchen_print_jobs SET state=$1,transport_reference=$2,last_error=$3,
        result=$4,lease_until=NULL,next_attempt_at=now()+make_interval(secs=>$5),revision=revision+1,updated_at=now()
        WHERE id=$6 AND state='processing'`, state, outcome.Reference, outcome.Message, encoded(outcome), float64(int64(1)<<uint(min(owned.Job.Attempts, 3))), owned.Job.ID)
	if err != nil {
		return true, err
	}
	if _, err := tx.Exec(storeContext, `UPDATE app.kitchen_print_attempts SET finished_at=now(),outcome=$1,detail=$2,result=$3
        WHERE id=$4`, state, outcome.Message, encoded(outcome), owned.AttemptID); err != nil {
		return true, err
	}
	confirmedFailures := false
	// El comando del cajón nunca pasa a una impresora de respaldo: el cajón está conectado a una
	// impresora concreta y otra no lo abriría.
	if state == "error" && owned.Job.Attempts >= 3 && owned.Job.PrinterSnapshot.FallbackID != nil && owned.Job.DrawerPulse == nil {
		// El respaldo exige tres fallos confirmados consecutivos sin envío. Un intento incierto o
		// en intervención entre ellos (por ejemplo, antes de un envío autorizado manualmente)
		// interrumpe la cuenta: el total de intentos no basta.
		var trailingErrors int
		err := tx.QueryRow(storeContext, `SELECT count(*) FILTER (WHERE outcome='error') FROM (
            SELECT outcome FROM app.kitchen_print_attempts WHERE job_id=$1 ORDER BY attempt DESC LIMIT 3) latest`,
			owned.Job.ID).Scan(&trailingErrors)
		if err != nil {
			return true, err
		}
		confirmedFailures = trailingErrors == 3
	}
	if confirmedFailures {
		fallback, err := one[Printer](storeContext, tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", owned.Job.PrinterSnapshot.FallbackID)
		if err != nil {
			return true, err
		}
		if fallback.Active {
			// Solo un respaldo automático explícito y únicamente tras fallos previos al envío.
			fallback.FallbackID = nil
			if _, err := tx.Exec(storeContext, `UPDATE app.kitchen_print_jobs SET printer_id=$1,printer_snapshot=$2,
                state='pending',next_attempt_at=now(),revision=revision+1 WHERE id=$3`, fallback.ID, encoded(fallback), owned.Job.ID); err != nil {
				return true, err
			}
			if err := systemHistory(storeContext, tx, "print.redirected", owned.Job.ID, map[string]string{
				"from_printer": owned.Job.PrinterID, "to_printer": fallback.ID, "reason": "Respaldo configurado tras tres fallos confirmados sin envío."}); err != nil {
				return true, err
			}
		}
	}
	if err := systemHistory(storeContext, tx, "print."+state, owned.Job.ID, outcome); err != nil {
		return true, err
	}
	if err := events.Append(storeContext, tx, "printing."+state, "printing", &owned.Job.ID, nil); err != nil {
		return true, err
	}
	// Los documentos y comandos de caja avisan también en el ámbito de caja, que reciben los
	// cajeros sin acceso a la cola de cocina: así la pantalla de Caja refleja el estado del envío.
	if isCashKind(owned.Job.TicketKind) {
		if err := events.Append(storeContext, tx, "cash.print.updated", "cash", &owned.Job.ID, nil); err != nil {
			return true, err
		}
	}
	return true, tx.Commit(storeContext)
}
