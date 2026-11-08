//go:build integration

package operations_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
)

type recordingTransport struct {
	mutex sync.Mutex
	jobs  []kitchen.PrintJob
	state string
}

func (transport *recordingTransport) Submit(_ context.Context, job kitchen.PrintJob) kitchen.Outcome {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	transport.jobs = append(transport.jobs, job)
	return kitchen.Outcome{State: transport.state, Accepted: transport.state == "submitted", Reference: "adaptador-controlado", Message: "Transporte de aceptación; no impresión física."}
}

func kitchenCount(t *testing.T, fixture *restaurantFixture, table string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app."+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func kitchenTasks(t *testing.T, fixture *restaurantFixture) []kitchen.Task {
	t.Helper()
	var result struct {
		Tasks []kitchen.Task `json:"tasks"`
	}
	result = decodeResponse[struct {
		Tasks []kitchen.Task `json:"tasks"`
	}](t, fixture.adminCall(t, "GET", "kitchen/board?state=all", nil, 200))
	return result.Tasks
}
func kitchenReleases(t *testing.T, fixture *restaurantFixture, bill operations.Bill) []kitchen.Release {
	t.Helper()
	return decodeResponse[[]kitchen.Release](t, fixture.adminCall(t, "GET", "kitchen/releases?account_id="+bill.Account.ID, nil, 200))
}
func kitchenState(t *testing.T, fixture *restaurantFixture, tasks []kitchen.Task, stationID, state string) {
	t.Helper()
	references := []map[string]any{}
	for _, task := range tasks {
		if task.StationID == stationID && task.State != "cancelled" {
			references = append(references, map[string]any{"id": task.ID, "revision": task.Revision})
		}
	}
	fixture.mutate(t, "POST", "kitchen/states", map[string]any{"state": state, "tasks": references}, 200)
}

func testKitchenAcceptance(t *testing.T, fixture *restaurantFixture) {
	var printer kitchen.Printer
	var first, second kitchen.Station
	var product catalog.Product
	var bill operations.Bill
	var release kitchen.Release
	t.Run("cocina estaciones compartidas rutas y destinos válidos", func(t *testing.T) {
		printer = decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
			Name: "Impresora virtual de aceptación", Transport: "cups", Destination: "ADEL_TEST_ONLY", Active: true, PaperWidth: 80, Copies: 1, Port: 9100,
		}, 200))
		first = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
			Name: "Cocina caliente", Active: true, Reception: "printer", PrinterID: &printer.ID, TargetMinutes: 12, WarningMinutes: 15,
			OverdueMinutes: 25, Grouping: "inherit", Tracking: "inherit",
		}, 200))
		second = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
			Name: "Freidoras", Active: true, Reception: "both", PrinterID: &printer.ID, TargetMinutes: 8, WarningMinutes: 10,
			OverdueMinutes: 20, Grouping: "inherit", Tracking: "inherit",
		}, 200))
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{first.ID}}, 200)
		product = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: "Platillo de cocina", Active: true}, CategoryID: fixture.category.ID, Code: "COCINA", Kind: "product", BasePrice: "100",
			Ingredients: fixture.product.Ingredients, Groups: fixture.product.Groups,
		}, 200))
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &product.ID, Mode: "prepare", StationIDs: []string{first.ID, first.ID}}, 400)
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{ProductID: &product.ID, Mode: "prepare", StationIDs: []string{first.ID, second.ID}}, 200)
		routes := decodeResponse[[]kitchen.Route](t, fixture.adminCall(t, "GET", "kitchen/routes", nil, 200))
		if len(routes) != 2 {
			t.Fatal("Se duplicaron rutas o se perdió la precedencia por producto")
		}
	})
	t.Run("cocina confirmación durable unidades únicas y tickets independientes", func(t *testing.T) {
		bill = newAccount(t, fixture, "Aceptación cocina")
		jobsBefore := kitchenCount(t, fixture, "kitchen_print_jobs")
		items := []operations.DraftItem{{ID: randomID(), LineRequest: catalog.LineRequest{ProductID: product.ID, Quantity: 2,
			Customization: catalog.Customization{Notes: "Café y ñ; sin sal"}}}}
		route := "restaurant/accounts/" + bill.Account.ID
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", route+"/draft", map[string]any{"revision": bill.Account.Revision, "items": items}, 200))
		if kitchenCount(t, fixture, "kitchen_print_jobs") != jobsBefore {
			t.Fatal("Guardar borrador produjo papel")
		}
		quote := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
		body := map[string]any{"operation_id": randomID(), "revision": bill.Account.Revision, "expected_fingerprint": quote.Fingerprint}
		bill = decodeResponse[operations.Bill](t, fixture.adminCall(t, "POST", route+"/confirm", body, 200))
		fixture.adminCall(t, "POST", route+"/confirm", body, 200)
		if bill.Account.Totals.Total != "200.00" || bill.Account.PreparationState != "released" {
			t.Fatalf("No se integró preparación: %+v", bill.Account)
		}
		if kitchenCount(t, fixture, "kitchen_unit_releases") != 2 || len(kitchenTasks(t, fixture)) != 4 || kitchenCount(t, fixture, "kitchen_print_jobs") != jobsBefore+2 {
			t.Fatal("Se duplicó la venta/liberación o no se separaron las estaciones que comparten impresora")
		}
		release = kitchenReleases(t, fixture, bill)[0]
		var documentText string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT document::text FROM app.kitchen_tickets WHERE release_id=$1 LIMIT 1", release.ID).Scan(&documentText); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(documentText, "Guarniciones") || !strings.Contains(documentText, "Café y ñ; sin sal") {
			t.Fatal("La comanda no conservó los complementos o las observaciones históricas")
		}
		fixture.mutate(t, "POST", "kitchen/releases/"+release.ID+"/release", map[string]any{"revision": release.Revision}, 200)
		if kitchenCount(t, fixture, "kitchen_print_jobs") != jobsBefore+2 {
			t.Fatal("Liberar otra vez duplicó comandas")
		}
	})
	t.Run("cocina consolidación por estación y pago independiente", func(t *testing.T) {
		kitchenState(t, fixture, kitchenTasks(t, fixture), first.ID, "ready")
		current := fixture.refresh(t, bill.Account.ID)
		if current.Account.PreparationState == "ready" {
			t.Fatal("Una estación ocultó pendientes de la otra")
		}
		kitchenState(t, fixture, kitchenTasks(t, fixture), second.ID, "ready")
		bill = fixture.refresh(t, bill.Account.ID)
		if bill.Account.PreparationState != "ready" || bill.Account.PaymentState != "not_recorded" || bill.Account.DeliveryState != "not_delivered" {
			t.Fatal("No se consolidó preparación o se simuló pago/entrega")
		}
	})
	t.Run("cocina modificación y cancelación conservan snapshots e identidad", func(t *testing.T) {
		line := bill.Lines[0]
		line.Request.Customization.Notes = "Nueva indicación: empacar separado"
		edit := map[string]any{"revision": bill.Account.Revision, "version_id": line.VersionID, "line": line.Request, "reason": "Solicitud del cliente"}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit-preview", edit, 200))
		edit["expected_fingerprint"] = preview.Fingerprint
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit", edit, 200))
		var modified int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.kitchen_tickets WHERE kind='modification'").Scan(&modified); err != nil {
			t.Fatal(err)
		}
		if modified != 2 || bill.Account.PreparationState == "ready" {
			t.Fatal("No se avisó la modificación o permaneció lista la preparación anterior")
		}
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/cancel", map[string]any{
			"revision": bill.Account.Revision, "version_id": bill.Lines[0].VersionID, "quantity": 1, "reason": "Cliente canceló una unidad",
		}, 200))
		if kitchenCount(t, fixture, "kitchen_unit_releases") != 2 {
			t.Fatal("Cancelar perdió el punto histórico de liberación")
		}
		var cancelled int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.kitchen_tickets WHERE kind='cancellation'").Scan(&cancelled); err != nil {
			t.Fatal(err)
		}
		if cancelled != 2 {
			t.Fatal("No se notificó la cancelación a las estaciones afectadas")
		}
	})
	t.Run("cocina colas persistentes recuperación y reimpresión normal", func(t *testing.T) {
		fixture.adminCall(t, "GET", "kitchen/jobs", nil, 200)
		adapter := &recordingTransport{state: "submitted"}
		worker := kitchen.Worker{Pool: fixture.pool, Transport: adapter}
		for index := 0; index < 10; index++ {
			worked, err := worker.PrintNext(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !worked {
				break
			}
		}
		if len(adapter.jobs) != 6 {
			t.Fatalf("Se perdieron o duplicaron trabajos: %d", len(adapter.jobs))
		}
		for _, job := range adapter.jobs {
			if strings.Contains(job.Content, "ADICIÓN") || strings.Contains(job.Content, "REIMPRESIÓN") || strings.Contains(job.Content, "$200") {
				t.Fatal("La comanda incluye etiquetas o importes no configurados")
			}
		}
		original := adapter.jobs[0]
		copyJob := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": original.ID, "reason": "Copia controlada de aceptación"}, 200))
		if _, err := worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if adapter.jobs[len(adapter.jobs)-1].Content != original.Content || copyJob.ID == original.ID {
			t.Fatal("La copia alteró la presentación o reutilizó el trabajo físico")
		}
		if kitchenCount(t, fixture, "kitchen_unit_releases") != 2 {
			t.Fatal("Reimprimir creó otra liberación")
		}
		// Preparar otra copia y simular que el proceso cayó después de iniciar transporte.
		interrupted := decodeResponse[kitchen.PrintJob](t, fixture.mutate(t, "POST", "kitchen/reprint", map[string]any{"job_id": original.ID, "reason": "Ensayo de interrupción"}, 200))
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='processing',lease_until=now()-interval '1 second' WHERE id=$1", interrupted.ID); err != nil {
			t.Fatal(err)
		}
		before := len(adapter.jobs)
		if _, err := worker.PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		current := decodeResponse[struct {
			Job kitchen.PrintJob `json:"job"`
		}](t, fixture.adminCall(t, "GET", "kitchen/jobs/"+interrupted.ID, nil, 200)).Job
		if current.State != "uncertain" || len(adapter.jobs) != before {
			t.Fatal("La recuperación reenvió un resultado desconocido")
		}
		fixture.mutate(t, "POST", "kitchen/jobs/"+current.ID+"/retry", map[string]any{"revision": current.Revision, "reason": "No repetir sin revisar"}, 400)
		fixture.mutate(t, "POST", "kitchen/jobs/"+current.ID+"/resolve", map[string]any{"revision": current.Revision, "decision": "cancel", "reason": "Operador revisó la cola y cancela"}, 200)
	})
	testKitchenQueueSafety(t, fixture)
	testKitchenFrozenChanges(t, fixture, bill, first, second)
	testKitchenSchedulingAndGrouping(t, fixture, printer, first, second, product)
	testKitchenRealtime(t, fixture)
}
