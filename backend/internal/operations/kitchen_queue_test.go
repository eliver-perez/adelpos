//go:build integration

package operations_test

import (
	"context"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/kitchen"
)

type gatedTransport struct {
	started chan struct{}
	finish  chan struct{}
}

func (transport *gatedTransport) Submit(ctx context.Context, job kitchen.PrintJob) kitchen.Outcome {
	close(transport.started)
	select {
	case <-transport.finish:
		return kitchen.Outcome{State: "submitted", Accepted: true}
	case <-ctx.Done():
		return kitchen.Outcome{State: "uncertain"}
	}
}

func testKitchenQueueSafety(t *testing.T, fixture *restaurantFixture) {
	backup := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name:        "Respaldo exclusivo de aceptación",
		Transport:   "cups",
		Destination: "ADEL_TEST_BACKUP",
		Active:      true,
		PaperWidth:  80,
		Copies:      1,
	}, 200))
	primary := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name:        "Cola exclusiva de aceptación",
		Transport:   "cups",
		Destination: "ADEL_TEST_SERIAL",
		Active:      true,
		PaperWidth:  80,
		Copies:      1,
		FallbackID:  &backup.ID,
	}, 200))
	request := func() kitchen.PrintJob {
		return decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/test-print", map[string]string{
			"printer_id": primary.ID,
		}, 200))
	}
	read := func(id string) kitchen.PrintJob {
		return decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+id, nil, 200)).Job
	}
	t.Run("cola serializa el transporte entre trabajadores independientes", func(t *testing.T) {
		request()
		request()
		gate := &gatedTransport{started: make(chan struct{}), finish: make(chan struct{})}
		worker := kitchen.Worker{Pool: fixture.pool, Transport: gate}
		result := make(chan error, 1)
		go func() {
			_, err := worker.PrintNext(fixture.ctx)
			result <- err
		}()
		select {
		case <-gate.started:
		case <-time.After(5 * time.Second):
			t.Fatal("El primer transporte no comenzó")
		}
		adapter := &recordingTransport{state: "submitted"}
		other := kitchen.Worker{Pool: fixture.pool, Transport: adapter}
		worked, err := other.PrintNext(fixture.ctx)
		if err != nil || worked || len(adapter.jobs) != 0 {
			t.Fatalf("Se intercalaron trabajos de la misma impresora: %v", err)
		}
		close(gate.finish)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if worked, err := other.PrintNext(fixture.ctx); err != nil || !worked || len(adapter.jobs) != 1 {
			t.Fatalf("El segundo trabajo no pudo continuar: %v", err)
		}
	})
	t.Run("respaldo explícito solo tras errores confirmados sin envío", func(t *testing.T) {
		job := request()
		adapter := &recordingTransport{state: "error"}
		worker := kitchen.Worker{Pool: fixture.pool, Transport: adapter}
		for attempt := 0; attempt < 3; attempt++ {
			if _, err := worker.PrintNext(fixture.ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET next_attempt_at=now() WHERE id=$1", job.ID); err != nil {
				t.Fatal(err)
			}
		}
		redirected := read(job.ID)
		if redirected.State != "pending" || redirected.PrinterID != backup.ID || redirected.PrinterSnapshot.FallbackID != nil {
			t.Fatal("No se aplicó un único respaldo explícito")
		}
		adapter.state = "submitted"
		if _, err := worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if read(job.ID).State != "submitted" {
			t.Fatal("El respaldo no conservó el mismo trabajo durable")
		}
	})
	t.Run("incertidumbre no activa reintentos ni respaldo y bloquea copias pendientes", func(t *testing.T) {
		job := request()
		adapter := &recordingTransport{state: "uncertain"}
		worker := kitchen.Worker{Pool: fixture.pool, Transport: adapter}
		if _, err := worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if worked, err := worker.PrintNext(fixture.ctx); err != nil || worked || len(adapter.jobs) != 1 {
			t.Fatal("Se reenvió automáticamente un resultado desconocido")
		}
		current := read(job.ID)
		if current.PrinterID != primary.ID || current.State != "uncertain" {
			t.Fatal("Un resultado incierto fue redirigido")
		}
		fixture.mutate(t, "POST", "kitchen/reprint", map[string]string{
			"job_id": job.ID, "reason": "No debe copiar un resultado incierto",
		}, 400)
		fixture.mutate(t, "POST", "kitchen/jobs/"+job.ID+"/resolve", map[string]any{
			"revision": current.Revision, "decision": "cancel", "reason": "Ensayo de resolución explícita",
		}, 200)
		copy := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]string{
			"job_id": job.ID, "reason": "Una sola copia bajo supervisión",
		}, 200))
		fixture.mutate(t, "POST", "kitchen/reprint", map[string]string{
			"job_id": job.ID, "reason": "No debe generar otra copia pendiente",
		}, 409)
		fixture.mutate(t, "POST", "kitchen/jobs/"+copy.ID+"/resolve", map[string]any{
			"revision": copy.Revision, "decision": "cancel", "reason": "Finaliza ensayo sin transporte físico",
		}, 200)
	})
}
