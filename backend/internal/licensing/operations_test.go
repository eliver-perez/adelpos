package licensing

import "testing"

// La tabla de operaciones de la §6 del contrato, celda por celda (avance §0.5.7; las celdas «—» de Sin activar son «No»,
// salvo reportes, historial y respaldos, que ningún estado bloquea).
func TestOperationTableOfTheContract(t *testing.T) {
	const (
		yes     = Allow
		no      = Deny
		pending = AllowWithPendingAccounts
	)
	states := []State{StateActive, StateGrace, StateExpired, StateRevoked, StateNotActivatedWithData, StateNotActivated}
	table := map[Row][]Decision{
		//                   Vigente Tolerancia Vencida Revocada  Sin activar con datos  Sin activar
		RowCapture:        {yes, yes, no, no, no, no},
		RowCaptured:       {yes, yes, yes, yes, no, no},
		RowOpenShift:      {yes, yes, pending, pending, no, no},
		RowCloseShift:     {yes, yes, yes, yes, no, no},
		RowKitchen:        {yes, yes, yes, yes, no, no},
		RowRecords:        {yes, yes, yes, yes, yes, yes},
		RowAdministration: {yes, yes, yes, yes, no, no},
		RowLicense:        {yes, yes, yes, yes, yes, yes},
		RowAlways:         {yes, yes, yes, yes, yes, yes},
		RowStartup:        {yes, yes, yes, yes, yes, yes},
		RowUnknownWrite:   {yes, yes, no, no, no, no},
	}
	if len(table) != len(Rows) {
		t.Fatalf("la prueba no cubre todas las filas: %d de %d", len(table), len(Rows))
	}
	for _, row := range Rows {
		expected, listed := table[row]
		if !listed {
			t.Fatalf("falta la fila %s", row)
		}
		for index, state := range states {
			if got := row.Decide(state); got != expected[index] {
				t.Errorf("%s en %s: %s, se esperaba %s", row, state, got, expected[index])
			}
		}
	}
}

// Las reglas firmes del contrato y lo que sigue disponible con la hora incorrecta (L1-D14), por operación.
func TestClassifyOperation(t *testing.T) {
	id := "11111111-2222-4333-8444-555555555555"
	cases := []struct {
		method, route string
		row           Row
		dated         bool
	}{
		// F1: crear pedidos y agregar productos (con fecha).
		{"POST", "restaurant/accounts", RowCapture, true},
		{"PUT", "restaurant/accounts/" + id + "/draft", RowCapture, true},
		{"POST", "restaurant/accounts/" + id + "/confirm", RowCapture, true},
		{"POST", "restaurant/accounts/" + id + "/edit", RowCapture, true},
		{"POST", "/api/v1/restaurant/offline-orders", RowCapture, true},
		// Las previas solo calculan.
		{"POST", "restaurant/accounts/" + id + "/draft-preview", RowRecords, false},
		{"POST", "restaurant/accounts/" + id + "/edit-preview", RowRecords, false},
		// F2: cobrar, cancelar, descuentos y reorganizar lo capturado (con fecha: la hora también bloquea cobrar, L1-D1).
		{"POST", "payments/accounts/" + id + "/payments", RowCaptured, true},
		{"POST", "payments/accounts/" + id + "/discounts", RowCaptured, true},
		{"POST", "payments/accounts/" + id + "/refunds", RowCaptured, true},
		{"POST", "payments/accounts/" + id + "/precheck/print", RowCaptured, true},
		{"POST", "payments/accounts/" + id + "/deliver", RowCaptured, true},
		{"POST", "restaurant/accounts/" + id + "/cancel", RowCaptured, true},
		{"POST", "restaurant/accounts/" + id + "/cancel-account", RowCaptured, true},
		{"POST", "restaurant/accounts/" + id + "/split", RowCaptured, true},
		{"POST", "restaurant/accounts/" + id + "/merge", RowCaptured, true},
		{"PATCH", "restaurant/accounts/" + id, RowCaptured, true},
		{"POST", "payments/accounts/" + id + "/preview", RowRecords, false},
		// F3 y F4: turnos y efectivo.
		{"POST", "payments/shifts", RowOpenShift, true},
		{"POST", "payments/shifts/" + id + "/closing/confirm", RowCloseShift, true},
		{"PUT", "payments/shifts/" + id + "/closing/count", RowCloseShift, true},
		{"POST", "payments/shifts/" + id + "/withdrawals", RowCloseShift, true},
		{"POST", "payments/cash/open-drawer", RowCloseShift, true},
		{"POST", "payments/cash/count-preview", RowRecords, false},
		// F5: cocina.
		{"GET", "kitchen/board", RowKitchen, false},
		{"POST", "kitchen/states", RowKitchen, true},
		{"POST", "kitchen/display-mode", RowKitchen, false},
		{"POST", "kitchen/releases/" + id + "/release", RowKitchen, true},
		{"POST", "kitchen/jobs/" + id + "/retry", RowKitchen, true},
		{"POST", "kitchen/print-jobs/rerender", RowKitchen, true},
		// F6: reportes, historial y respaldos, siempre y sin fecha.
		{"GET", "reports/run/sales.by_day", RowRecords, false},
		{"GET", "reports/export/sales.by_day?format=csv", RowRecords, false},
		{"POST", "maintenance/backups/run", RowRecords, false},
		{"PUT", "maintenance/backups/settings", RowRecords, false},
		{"GET", "restaurant/accounts/" + id + "/history", RowRecords, false},
		{"POST", "payments/cash-reports/" + id + "/print", RowRecords, false},
		{"GET", "audit", RowRecords, false},
		// F7: administración, también inventario y emparejar (con fecha).
		{"POST", "catalog/products", RowAdministration, true},
		{"POST", "catalog/products/" + id + "/sold-out", RowAdministration, true},
		{"POST", "catalog/preview", RowRecords, false},
		{"POST", "inventory/adjustments", RowAdministration, true},
		{"PATCH", "users/" + id, RowAdministration, true},
		{"PUT", "payments/settings", RowAdministration, true},
		{"POST", "payments/cash/drawers/" + id + "/test", RowAdministration, true},
		{"PUT", "kitchen/settings", RowAdministration, true},
		{"POST", "kitchen/printers/" + id + "/width-test", RowAdministration, true},
		{"POST", "pairing/requests", RowAdministration, true},
		{"GET", "pairing/requests/" + id, RowRecords, false},
		{"POST", "devices/pairing-requests/" + id + "/approve", RowAdministration, true},
		{"POST", "devices/" + id + "/revoke", RowAdministration, true},
		{"POST", "maintenance/test-data/remove", RowAdministration, true},
		{"POST", "documents/preview", RowRecords, false},
		{"PUT", "documents/identity", RowAdministration, true},
		// F8: la licencia, siempre y sin fecha.
		{"POST", "license/activate", RowLicense, false},
		{"POST", "license/import", RowLicense, false},
		{"POST", "license/deactivate", RowLicense, false},
		{"POST", "license/time-check", RowLicense, false},
		{"POST", "license/support-unlock", RowLicense, false},
		// Lo que la tabla no lista y siempre se permite (L1-D6).
		{"POST", "auth/login", RowAlways, false},
		{"POST", "auth/pin-login", RowAlways, false},
		{"POST", "auth/password", RowAlways, false},
		{"PUT", "preferences", RowAlways, false},
		{"PUT", "terminal/preferences", RowAlways, false},
		{"POST", "maintenance/service/restart", RowAlways, false},
		{"POST", "devices/" + id + "/seat/release", RowAlways, false},
		{"POST", "sessions/" + id + "/revoke", RowAlways, false},
		{"GET", "events/snapshot", RowAlways, false},
		{"GET", "ready-alerts", RowAlways, false},
		{"POST", "print-agent/jobs/claim", RowAlways, false},
		// El arranque.
		{"GET", "setup/status", RowStartup, false},
		{"POST", "setup", RowStartup, false},
		{"POST", "devices/enroll", RowStartup, false},
	}
	for _, item := range cases {
		operation := ClassifyOperation(item.method, item.route)
		if operation.Row != item.row || operation.Dated != item.dated || !operation.Known {
			t.Errorf("%s %s: %+v, se esperaba %s con fecha=%v", item.method, item.route, operation, item.row, item.dated)
		}
	}
}

// Una escritura que no está en la tabla toma la clase más restrictiva (L1-D12); una lectura desconocida no registra
// nada.
func TestUnknownOperationsTakeTheMostRestrictiveClass(t *testing.T) {
	for _, item := range []struct{ method, route string }{
		{"POST", "restaurant/accounts/11111111-2222-4333-8444-555555555555/teleport"},
		{"POST", "payments/accounts/11111111-2222-4333-8444-555555555555/magic"},
		{"DELETE", "kitchen/states"},
		{"POST", "nueva-ruta"},
		{"POST", "sessions"},
	} {
		operation := ClassifyOperation(item.method, item.route)
		if operation.Row != RowUnknownWrite || !operation.Dated || operation.Known {
			t.Errorf("%s %s: %+v, se esperaba la clase más restrictiva", item.method, item.route, operation)
		}
	}
	read := ClassifyOperation("GET", "nueva-ruta")
	if read.Row != RowRecords || read.Dated || read.Known {
		t.Errorf("una lectura desconocida: %+v", read)
	}
}
