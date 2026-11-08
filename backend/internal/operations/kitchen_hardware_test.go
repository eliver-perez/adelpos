//go:build integration

package operations_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/kitchen"
)

// Solo se ejecuta con un destino local explícito; las suites normales no imprimen.
func TestControlledCUPSPrinter(t *testing.T) {
	queue := os.Getenv("ADEL_HARDWARE_QUEUE")
	if queue == "" {
		t.Skip("Impresión física opt-in; requiere destino explícito")
	}
	fixture := newFixture(t)
	printer := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Prueba controlada de impresora", Transport: "cups", Destination: queue, PaperWidth: 80, Copies: 1, Port: 9100, Active: true,
	}, 200))
	job := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/test-print", map[string]string{"printer_id": printer.ID}, 200))
	var content string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", job.ID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if filename := os.Getenv("ADEL_HARDWARE_PREVIEW"); filename != "" {
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report := map[string]any{"queue": queue, "copies": 1, "job_id": job.ID, "physical_confirmed": false, "sent": false}
	if os.Getenv("ADEL_HARDWARE_SEND") == "1" {
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: kitchen.HostTransport{}}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		current := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+job.ID, nil, 200)).Job
		report["sent"], report["state"], report["transport_reference"], report["message"] = true, current.State, current.TransportReference, current.LastError
		if current.TransportReference != "" {
			time.Sleep(time.Second)
			// Limpiar únicamente este trabajo de prueba si el spooler aún lo conserva.
			command := exec.CommandContext(fixture.ctx, "/usr/bin/cancel", current.TransportReference)
			cleanupError := command.Run()
			report["spooler_cleanup_requested"] = cleanupError == nil
		}
	}
	if filename := os.Getenv("ADEL_HARDWARE_REPORT"); filename != "" {
		encoded, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(filename, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("Ensayo CUPS controlado registrado; no se afirma salida física de papel.")
}
