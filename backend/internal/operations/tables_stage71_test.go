//go:build integration

package operations_test

import (
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 7.1, fase B (mesas): cuenta creada con el primer producto, TABLE_HAS_ACCOUNT, liberar cuentas
// vacías, estados de mesa calculados por Go y fuera de servicio desde la operación.

type stage71Problem struct {
	Error struct {
		Code    string `json:"code"`
		Details struct {
			Accounts []operations.TableAccountRef `json:"accounts"`
		} `json:"details"`
	} `json:"error"`
}

func decodeProblem(t *testing.T, body []byte) stage71Problem {
	t.Helper()
	return decodeResponse[stage71Problem](t, body)
}

// recordCounts cuenta las filas que deja cualquier escritura operativa: si no cambian, la acción no
// dejó registro alguno.
func recordCounts(t *testing.T, fixture *restaurantFixture) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{
		"restaurant_accounts", "restaurant_draft_items", "restaurant_operations", "restaurant_events",
		"realtime_events", "audit_events", "restaurant_orders",
	} {
		var count int
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app."+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

func sameCounts(t *testing.T, before, after map[string]int, context string) {
	t.Helper()
	for table, count := range before {
		if after[table] != count {
			t.Fatalf("%s: %s pasó de %d a %d filas", context, table, count, after[table])
		}
	}
}

func draftOf(lines ...catalog.LineRequest) []operations.DraftItem {
	items := make([]operations.DraftItem, 0, len(lines))
	for _, line := range lines {
		items = append(items, operations.DraftItem{ID: randomID(), LineRequest: line})
	}
	return items
}

// openWithItems abre una cuenta de mesa con su borrador inicial, como el primer guardado de la captura.
func openWithItems(t *testing.T, fixture *restaurantFixture, terminal testTerminal, tableID string, seen []string, items []operations.DraftItem, status int) []byte {
	t.Helper()
	return fixture.call(t, terminal, "POST", "restaurant/accounts", map[string]any{
		"operation_id": randomID(), "mode": "dine_in", "table_id": tableID, "seen_account_ids": seen, "items": items,
	}, status)
}

func tableByID(t *testing.T, fixture *restaurantFixture, id string) operations.Table {
	t.Helper()
	overview := decodeResponse[struct {
		Tables []operations.Table `json:"tables"`
	}](t, fixture.adminCall(t, "GET", "restaurant/overview", nil, 200))
	for _, table := range overview.Tables {
		if table.ID == id {
			return table
		}
	}
	t.Fatalf("La mesa %s no aparece en el mapa", id)
	return operations.Table{}
}

func expectTableState(t *testing.T, fixture *restaurantFixture, id, state, balance string) operations.Table {
	t.Helper()
	table := tableByID(t, fixture, id)
	if table.State != state || table.Balance != balance {
		t.Fatalf("Estado de mesa: esperado %s con saldo %s, recibido %s con saldo %s", state, balance, table.State, table.Balance)
	}
	// La lista de mesas y el mapa usan la misma consulta.
	tables := decodeResponse[[]operations.Table](t, fixture.adminCall(t, "GET", "restaurant/tables", nil, 200))
	for _, entry := range tables {
		if entry.ID == id && (entry.State != table.State || entry.Balance != table.Balance) {
			t.Fatal("La lista de mesas y el mapa calcularon estados distintos")
		}
	}
	return table
}

func confirmDraft(t *testing.T, fixture *restaurantFixture, bill operations.Bill) operations.Bill {
	t.Helper()
	route := "restaurant/accounts/" + bill.Account.ID
	preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "GET", route+"/preview", nil, 200))
	return decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", route+"/confirm", map[string]any{
		"revision": bill.Account.Revision, "expected_fingerprint": preview.Fingerprint,
	}, 200))
}

func release(t *testing.T, fixture *restaurantFixture, terminal testTerminal, bill operations.Bill, status int) []byte {
	t.Helper()
	return fixture.call(t, terminal, "POST", "restaurant/accounts/"+bill.Account.ID+"/release", map[string]any{
		"operation_id": randomID(), "revision": bill.Account.Revision,
	}, status)
}

func tableService(t *testing.T, fixture *restaurantFixture, terminal testTerminal, table operations.Table, outOfService bool, status int) []byte {
	t.Helper()
	return fixture.call(t, terminal, "POST", "restaurant/tables/"+table.ID+"/service", map[string]any{
		"operation_id": randomID(), "revision": table.Revision, "out_of_service": outOfService,
	}, status)
}

func auditCount(t *testing.T, fixture *restaurantFixture, action, entityID string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM app.audit_events WHERE action=$1 AND entity_id=$2", action, entityID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func testTablesStage71(t *testing.T, fixture *restaurantFixture) {
	base := &paymentsFixture{restaurantFixture: fixture, methods: map[string]payments.Method{}, registers: map[string]string{}}
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil {
			base.methods[*method.Code] = method
		}
	}
	debit := base.methods["debit"].ID
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}
	// Cajero con caja y turno propios para registrar pagos sin depender de las pruebas anteriores.
	register := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja de mesas 7.1", "description": "", "active": true}, 200)).ID
	cashier := newCashier(t, base, "cajero_mesas", []string{
		"accounts.read", "payments.read", "payments.collect", "payments.collect_partial", "cash.shifts.open", "cash.shifts.read",
	}, register)
	fixture.call(t, cashier, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)
	// Segunda terminal con los permisos del mesero (la del mesero original cerró su sesión en las
	// pruebas de seguridad): captura y abre cuentas, pero no administra mesas.
	fixture.waiter = newCashier(t, base, "mesero_mesas", []string{
		"accounts.read", "accounts.open", "accounts.manage", "orders.capture", "orders.confirm", "orders.cancel",
		"restaurant.tables.read", "restaurant.areas.read", "catalog.read", "catalog.prices.read",
	}, "")
	// Impresora de tickets sin hardware: la precuenta queda en la cola en intervención, nunca se envía.
	printer := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Precuentas de mesas 7.1", Transport: "agent", Destination: "Agente de aceptación no disponible", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	base.receiptSettings(t, printer.ID, false)
	printPrecheck := func(t *testing.T, accountID string) {
		t.Helper()
		base.post(t, fixture.admin, accountID, "precheck/print", map[string]any{}, 200)
	}

	t.Run("71B_salir_sin_capturar_no_deja_registro", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 libre", 300)
		before := recordCounts(t, fixture)
		// Lo que consulta la captura de una mesa sin cuenta: mapa, cuentas de la mesa, productos con la
		// lista predeterminada y la cotización tentativa del catálogo. Ninguna escribe.
		fixture.adminCall(t, "GET", "restaurant/overview", nil, 200)
		fixture.adminCall(t, "GET", "restaurant/accounts?mode=dine_in&state=open&table_id="+table.ID, nil, 200)
		fixture.adminCall(t, "GET", "catalog/products?active=true&archived=false&category_id="+fixture.category.ID, nil, 200)
		fixture.adminCall(t, "POST", "catalog/preview", map[string]any{"price_list_id": nil, "lines": []catalog.LineRequest{fries}}, 200)
		sameCounts(t, before, recordCounts(t, fixture), "Salir sin capturar")
		if openAccounts(t, fixture, table.ID) != 0 {
			t.Fatal("La mesa dejó de estar libre sin capturar productos")
		}
		state := expectTableState(t, fixture, table.ID, "free", "0.00")
		if state.OccupiedSince != nil || state.OccupiedSeconds != 0 || state.AccountCount != 0 {
			t.Fatal("Una mesa libre informó tiempo de ocupación")
		}
	})

	t.Run("71B_cuenta_creada_con_el_primer_producto_idempotente", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 primer producto", 440)
		items := draftOf(fries)
		body := map[string]any{
			"operation_id": randomID(), "mode": "dine_in", "table_id": table.ID, "seen_account_ids": []string{}, "items": items,
		}
		before := recordCounts(t, fixture)
		first := fixture.adminCall(t, "POST", "restaurant/accounts", body, 200)
		bill := decodeResponse[operations.Bill](t, first)
		if len(bill.Draft) != 1 || bill.Draft[0].ID != items[0].ID || bill.Account.DraftQuantity != 1 || bill.Account.Revision != 1 || len(bill.Lines) != 0 {
			t.Fatalf("La cuenta no se creó con su borrador inicial: %+v", bill.Account)
		}
		if bill.Account.Name != "Mesa 71 primer producto" || bill.Account.Releasable {
			t.Fatal("La cuenta con borrador no conservó la referencia neutra o se marcó liberable")
		}
		after := recordCounts(t, fixture)
		if after["restaurant_accounts"] != before["restaurant_accounts"]+1 || after["restaurant_operations"] != before["restaurant_operations"]+1 ||
			after["restaurant_draft_items"] != before["restaurant_draft_items"]+1 {
			t.Fatal("La cuenta y su borrador no se registraron en una sola operación")
		}
		var kinds []string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT array_agg(kind ORDER BY sequence) FROM app.restaurant_events
            WHERE operation_id=$1`, body["operation_id"]).Scan(&kinds); err != nil {
			t.Fatal(err)
		}
		if len(kinds) != 2 || kinds[0] != "account.opened" || kinds[1] != "draft.saved" {
			t.Fatalf("Eventos de la apertura con producto: %v", kinds)
		}
		// Reintento incierto: el mismo operation_id y cuerpo devuelven la cuenta original sin otra.
		replay := fixture.adminCall(t, "POST", "restaurant/accounts", body, 200)
		if decodeResponse[operations.Bill](t, replay).Account.ID != bill.Account.ID {
			t.Fatal("El reintento con el mismo operation_id abrió otra cuenta")
		}
		sameCounts(t, after, recordCounts(t, fixture), "Reintento idempotente")
		if openAccounts(t, fixture, table.ID) != 1 {
			t.Fatal("El reintento duplicó la cuenta")
		}
		// El mismo operation_id con otro contenido no repite nada.
		changed := map[string]any{}
		for key, value := range body {
			changed[key] = value
		}
		changed["items"] = draftOf(fries)
		fixture.adminCall(t, "POST", "restaurant/accounts", changed, 409)
		// Los guardados siguientes usan PUT …/draft con la revisión de la cuenta creada.
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+bill.Account.ID+"/draft", map[string]any{
			"revision": bill.Account.Revision, "items": append(bill.Draft, draftOf(fries)...),
		}, 200))
		if len(bill.Draft) != 2 || bill.Account.Revision != 2 {
			t.Fatal("El segundo guardado no continuó sobre la cuenta creada")
		}
		// Enviar a cocina sigue el flujo de siempre.
		bill = confirmDraft(t, fixture, bill)
		if bill.Account.ConfirmedQuantity != 2 || len(bill.Draft) != 0 {
			t.Fatal("La cuenta creada con su primer producto no se pudo enviar a cocina")
		}
		// Las mismas validaciones del borrador: sin escribir nada si fallan.
		before = recordCounts(t, fixture)
		invalid := draftOf(catalog.LineRequest{ProductID: randomID(), Quantity: 1})
		openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID}, invalid, 400)
		tooMany := []operations.DraftItem{}
		for range 21 {
			tooMany = append(tooMany, draftOf(fries)...)
		}
		openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID}, tooMany, 400)
		zero := draftOf(catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 0})
		openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID}, zero, 400)
		sameCounts(t, before, recordCounts(t, fixture), "Borrador inicial inválido")
		// Una partida que pertenece al borrador de otra cuenta tampoco se acepta.
		other := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID}, draftOf(fries), 200))
		before = recordCounts(t, fixture)
		foreign := []operations.DraftItem{{ID: other.Draft[0].ID, LineRequest: fries}}
		openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID, other.Account.ID}, foreign, 400)
		sameCounts(t, before, recordCounts(t, fixture), "Partida de otra cuenta")
	})

	testTablesStage71Concurrency(t, fixture)
	testTablesStage71Release(t, fixture, base, cashier, debit, printPrecheck)
	testTablesStage71States(t, fixture, base, cashier, debit, printPrecheck)
	testTablesStage71Service(t, fixture, base)
}

func testTablesStage71Concurrency(t *testing.T, fixture *restaurantFixture) {
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}
	pair := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 2}

	t.Run("71B_dos_terminales_en_la_misma_mesa_libre", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 compartida", 580)
		// La primera terminal guarda su primer producto: crea la cuenta.
		first := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200))
		// La segunda terminal también la veía libre: no se abre otra cuenta en silencio.
		before := recordCounts(t, fixture)
		waiterItems := draftOf(pair)
		problem := decodeProblem(t, openWithItems(t, fixture, fixture.waiter, table.ID, []string{}, waiterItems, 409))
		if problem.Error.Code != "TABLE_HAS_ACCOUNT" || len(problem.Error.Details.Accounts) != 1 {
			t.Fatalf("Rechazo esperado TABLE_HAS_ACCOUNT con la cuenta nueva: %+v", problem)
		}
		reference := problem.Error.Details.Accounts[0]
		if reference.ID != first.Account.ID || reference.Folio != first.Account.Folio || reference.Name != first.Account.Name {
			t.Fatalf("El rechazo no identificó la cuenta de la otra terminal: %+v", reference)
		}
		sameCounts(t, before, recordCounts(t, fixture), "TABLE_HAS_ACCOUNT")
		if openAccounts(t, fixture, table.ID) != 1 {
			t.Fatal("El rechazo dejó una segunda cuenta")
		}
		// «Agregar a esa cuenta»: sus productos se suman al borrador de la cuenta existente, que
		// conserva el suyo.
		current := fixture.refresh(t, first.Account.ID)
		merged := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "PUT", "restaurant/accounts/"+first.Account.ID+"/draft", map[string]any{
			"operation_id": randomID(), "revision": current.Account.Revision, "items": append(current.Draft, waiterItems...),
		}, 200))
		if len(merged.Draft) != 2 || merged.Draft[0].ID != first.Draft[0].ID || merged.Draft[1].ID != waiterItems[0].ID || merged.Account.DraftQuantity != 3 {
			t.Fatal("Agregar a la cuenta existente perdió su borrador o los productos nuevos")
		}
		// «Abrir una cuenta nueva»: se reenvía con la cuenta ya vista y otro operation_id.
		second := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.waiter, table.ID, []string{first.Account.ID}, draftOf(fries), 200))
		if second.Account.ID == first.Account.ID || openAccounts(t, fixture, table.ID) != 2 {
			t.Fatal("Abrir una cuenta nueva a sabiendas no creó la segunda cuenta")
		}
		// Sin el campo seen_account_ids se conserva el comportamiento anterior (sin comprobación).
		legacy := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		release(t, fixture, fixture.admin, legacy, 200)
		// Identificadores vistos inválidos se rechazan antes de escribir.
		openWithItems(t, fixture, fixture.admin, table.ID, []string{"no-es-uuid"}, draftOf(fries), 400)
	})

	t.Run("71B_nueva_cuenta_en_mesa_ocupada", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 ocupada", 720)
		first := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200))
		// «+ Nueva cuenta» conoce la cuenta abierta de la mesa: se crea la segunda con su producto.
		second := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{first.Account.ID}, draftOf(fries), 200))
		if second.Account.DraftQuantity != 1 || openAccounts(t, fixture, table.ID) != 2 {
			t.Fatal("«Nueva cuenta» en mesa ocupada no se creó con su primer producto")
		}
		// Otra terminal que solo conocía la primera recibe la segunda como cuenta nueva.
		problem := decodeProblem(t, openWithItems(t, fixture, fixture.waiter, table.ID, []string{first.Account.ID}, draftOf(fries), 409))
		if problem.Error.Code != "TABLE_HAS_ACCOUNT" || len(problem.Error.Details.Accounts) != 1 || problem.Error.Details.Accounts[0].ID != second.Account.ID {
			t.Fatalf("La terminal no supo de la cuenta que no conocía: %+v", problem)
		}
		// Dos terminales al mismo tiempo sobre una mesa libre: una sola cuenta.
		race := quickTable(t, fixture, "Mesa 71 carrera", 860)
		statuses := concurrentStatuses(fixture, "POST", "restaurant/accounts",
			map[string]any{"operation_id": randomID(), "mode": "dine_in", "table_id": race.ID, "seen_account_ids": []string{}, "items": draftOf(fries)},
			map[string]any{"operation_id": randomID(), "mode": "dine_in", "table_id": race.ID, "seen_account_ids": []string{}, "items": draftOf(fries)})
		created, rejected := 0, 0
		for _, status := range statuses {
			switch status {
			case 200:
				created++
			case 409:
				rejected++
			}
		}
		if created != 1 || rejected != 1 || openAccounts(t, fixture, race.ID) != 1 {
			t.Fatalf("Dos aperturas simultáneas en una mesa libre: %v", statuses)
		}
	})
}

func testTablesStage71Release(t *testing.T, fixture *restaurantFixture, base *paymentsFixture, cashier testTerminal, debit string, printPrecheck func(*testing.T, string)) {
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}

	t.Run("71B_liberar_cuenta_vacia_permitido_y_auditado", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 liberar", 20)
		empty := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		if !empty.Account.Releasable {
			t.Fatal("Una cuenta abierta sin contenido no se marcó liberable")
		}
		expectTableState(t, fixture, table.ID, "occupied", "0.00")
		body := map[string]any{"operation_id": randomID(), "revision": empty.Account.Revision}
		// Sin motivo ni autorización: basta accounts.open (el mesero lo tiene).
		released := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", "restaurant/accounts/"+empty.Account.ID+"/release", body, 200))
		if released.Account.State != "voided" || released.Account.Releasable {
			t.Fatal("La cuenta liberada no quedó anulada")
		}
		replay := decodeResponse[operations.Bill](t, fixture.call(t, fixture.waiter, "POST", "restaurant/accounts/"+empty.Account.ID+"/release", body, 200))
		if replay.Account.Revision != released.Account.Revision {
			t.Fatal("El reintento de liberar no devolvió el resultado original")
		}
		if auditCount(t, fixture, "restaurant.account.released", empty.Account.ID) != 1 {
			t.Fatal("Liberar la mesa no quedó auditado una sola vez")
		}
		var reason string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT payload->'after'->>'reason' FROM app.restaurant_events
            WHERE account_id=$1 AND kind='account.released'`, empty.Account.ID).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if reason != "Cuenta vacía liberada" {
			t.Fatalf("Motivo fijo de la liberación: %q", reason)
		}
		if base.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='restaurant.account.released' AND aggregate_id=$1 AND scope='restaurant'", empty.Account.ID) != 1 {
			t.Fatal("Liberar la mesa no publicó su evento para el mapa")
		}
		expectTableState(t, fixture, table.ID, "free", "0.00")
		// Ya no está abierta: no se libera otra vez.
		release(t, fixture, fixture.admin, released, 400)
	})

	t.Run("71B_liberar_rechaza_cuentas_con_contenido", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 con contenido", 160)
		expectNotEmpty := func(t *testing.T, bill operations.Bill, context string) {
			t.Helper()
			current := fixture.refresh(t, bill.Account.ID)
			if current.Account.Releasable {
				t.Fatalf("%s: la cuenta se marcó liberable", context)
			}
			before := recordCounts(t, fixture)
			if code := decodeProblem(t, release(t, fixture, fixture.admin, current, 409)).Error.Code; code != "ACCOUNT_NOT_EMPTY" {
				t.Fatalf("%s: rechazo esperado ACCOUNT_NOT_EMPTY, recibido %s", context, code)
			}
			sameCounts(t, before, recordCounts(t, fixture), context)
			if fixture.refresh(t, bill.Account.ID).Account.State != "open" {
				t.Fatalf("%s: la cuenta dejó de estar abierta", context)
			}
		}
		// Borrador: no se libera; al vaciarlo vuelve a ser liberable.
		withDraft := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, nil, draftOf(fries), 200))
		expectNotEmpty(t, withDraft, "Con borrador")
		cleared := decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+withDraft.Account.ID+"/draft", map[string]any{
			"revision": withDraft.Account.Revision, "items": []operations.DraftItem{},
		}, 200))
		if !cleared.Account.Releasable {
			t.Fatal("Un borrador vaciado sin otros registros debe poder liberarse")
		}
		release(t, fixture, fixture.admin, cleared, 200)
		// Unidades canceladas: conservan historial, así que no es una cuenta vacía.
		cancelled := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, nil, draftOf(fries), 200)))
		cancelled = decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts/"+cancelled.Account.ID+"/cancel", map[string]any{
			"revision": cancelled.Account.Revision, "version_id": cancelled.Lines[0].VersionID, "quantity": 1, "reason": "Producto no disponible",
		}, 200))
		if cancelled.Account.ConfirmedQuantity != 0 || cancelled.Account.DraftQuantity != 0 {
			t.Fatal("La cancelación no dejó la cuenta sin unidades activas")
		}
		expectNotEmpty(t, cancelled, "Con unidades canceladas")
		// Pago: la cuenta tiene un abono registrado.
		paid := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, nil, draftOf(fries), 200)))
		base.pay(t, cashier, paid.Account.ID, map[string]any{"method_id": debit, "mode": "partial", "amount": "10"}, 200)
		expectNotEmpty(t, paid, "Con pago")
		// Ajuste sobre una cuenta sin productos: un cargo cancelado también es un registro financiero.
		adjusted := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		charge := decodeResponse[struct {
			Finance payments.Finance `json:"finance"`
		}](t, base.post(t, fixture.admin, adjusted.Account.ID, "charges", map[string]any{
			"concept": "Servicio", "amount": "15", "expected_balance": "0.00",
		}, 200))
		if len(charge.Finance.Adjustments) != 1 {
			t.Fatal("El cargo no quedó registrado en la cuenta")
		}
		base.post(t, fixture.admin, adjusted.Account.ID, "adjustments/void", map[string]any{
			"adjustment_id": charge.Finance.Adjustments[0].ID, "reason": "Cargo capturado por error",
		}, 200)
		expectNotEmpty(t, adjusted, "Con ajuste cancelado")
		// Precuenta impresa sobre una cuenta sin productos.
		printed := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		printPrecheck(t, printed.Account.ID)
		expectNotEmpty(t, printed, "Con precuenta")
		// Cocina: un cambio pendiente que la configuración no pudo enrutar (registro de cocina).
		kitchenless := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.kitchen_pending_changes(event_id,account_id,kind,payload,warnings)
            SELECT id,account_id,'item.modified','{}','["Sin ruta de cocina"]' FROM app.restaurant_events
            WHERE account_id=$1 AND kind='account.opened'`, kitchenless.Account.ID); err != nil {
			t.Fatal(err)
		}
		expectNotEmpty(t, kitchenless, "Con cambio de cocina pendiente")
		// Combinación recibida: la cuenta destino recibió las unidades de otra cuenta.
		source := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, nil, draftOf(fries), 200)))
		target := decodeResponse[operations.Bill](t, fixture.mutate(t, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in", "table_id": table.ID}, 200))
		fixture.mutate(t, "POST", "restaurant/accounts/"+source.Account.ID+"/merge", map[string]any{
			"revision": source.Account.Revision, "target_account_id": target.Account.ID, "target_revision": target.Account.Revision,
		}, 200)
		expectNotEmpty(t, target, "Con combinación recibida")
	})
}

func testTablesStage71States(t *testing.T, fixture *restaurantFixture, base *paymentsFixture, cashier testTerminal, debit string, printPrecheck func(*testing.T, string)) {
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}

	t.Run("71B_estados_de_mesa_calculados_por_go", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 estados", 300)
		expectTableState(t, fixture, table.ID, "free", "0.00")
		// Ocupada: una cuenta con producto, sin precuenta. El tiempo es el de la cuenta más antigua.
		bill := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200)))
		occupied := expectTableState(t, fixture, table.ID, "occupied", "40.00")
		if occupied.OccupiedSince == nil || !occupied.OccupiedSince.Equal(bill.Account.CreatedAt) || occupied.OccupiedSeconds < 0 || occupied.AccountCount != 1 {
			t.Fatalf("Tiempo de ocupación incorrecto: %+v", occupied)
		}
		// Esperando pago: precuenta impresa con saldo.
		printPrecheck(t, bill.Account.ID)
		expectTableState(t, fixture, table.ID, "awaiting_payment", "40.00")
		if base.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='payments.document.printed' AND aggregate_id=$1 AND scope='restaurant'", bill.Account.ID) != 1 {
			t.Fatal("La precuenta impresa no llega al ámbito que recibe el mapa")
		}
		// Un cambio posterior de contenido la invalida.
		bill = fixture.refresh(t, bill.Account.ID)
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+bill.Account.ID+"/draft", map[string]any{
			"revision": bill.Account.Revision, "items": draftOf(fries),
		}, 200))
		expectTableState(t, fixture, table.ID, "occupied", "40.00")
		bill = confirmDraft(t, fixture, bill)
		printPrecheck(t, bill.Account.ID)
		expectTableState(t, fixture, table.ID, "awaiting_payment", "80.00")
		// Un pago parcial posterior a la precuenta no la invalida.
		base.pay(t, cashier, bill.Account.ID, map[string]any{"method_id": debit, "mode": "partial", "amount": "30"}, 200)
		expectTableState(t, fixture, table.ID, "awaiting_payment", "50.00")
		// Un ajuste posterior sí: queda como pago parcial con su saldo.
		base.post(t, fixture.admin, bill.Account.ID, "charges", map[string]any{
			"concept": "Servicio", "amount": "5", "expected_balance": "50.00",
		}, 200)
		expectTableState(t, fixture, table.ID, "partial_payment", "55.00")
		// Prioridad con varias cuentas: una ocupada no rebaja el pago parcial…
		other := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{bill.Account.ID}, draftOf(fries), 200)))
		expectTableState(t, fixture, table.ID, "partial_payment", "95.00")
		// …y una cuenta esperando pago gana sobre las demás; el saldo suma todas las cuentas abiertas.
		printPrecheck(t, other.Account.ID)
		state := expectTableState(t, fixture, table.ID, "awaiting_payment", "95.00")
		if state.AccountCount != 2 || state.OccupiedSince == nil || !state.OccupiedSince.Equal(bill.Account.CreatedAt) {
			t.Fatal("Con varias cuentas el tiempo debe ser el de la apertura más antigua")
		}
		// El saldo de la mesa coincide con el que payments informa en cada cuenta.
		accounts := decodeResponse[struct {
			Items []operations.Account `json:"items"`
		}](t, fixture.adminCall(t, "GET", "restaurant/accounts?mode=dine_in&state=open&table_id="+table.ID, nil, 200))
		balances := map[string]string{}
		for _, account := range accounts.Items {
			balances[account.ID] = account.Finance.Balance
		}
		if len(balances) != 2 || balances[bill.Account.ID] != "55.00" || balances[other.Account.ID] != "40.00" {
			t.Fatalf("Saldos de las cuentas de la mesa: %v", balances)
		}
		// Liquidar las cuentas libera la mesa: un pago total cierra la cuenta de salón.
		base.pay(t, cashier, other.Account.ID, map[string]any{"method_id": debit, "mode": "total"}, 200)
		expectTableState(t, fixture, table.ID, "partial_payment", "55.00")
		base.pay(t, cashier, bill.Account.ID, map[string]any{"method_id": debit, "mode": "total"}, 200)
		expectTableState(t, fixture, table.ID, "free", "0.00")
		// Pago parcial sin precuenta.
		partial := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200)))
		base.pay(t, cashier, partial.Account.ID, map[string]any{"method_id": debit, "mode": "partial", "amount": "15"}, 200)
		expectTableState(t, fixture, table.ID, "partial_payment", "25.00")
	})
}

func testTablesStage71Service(t *testing.T, fixture *restaurantFixture, base *paymentsFixture) {
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}

	t.Run("71B_fuera_de_servicio_desde_la_operacion", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 servicio", 440)
		// Sin permiso de administrar mesas: rechazo sin cambios.
		tableService(t, fixture, fixture.waiter, table, true, 403)
		if tableByID(t, fixture, table.ID).OutOfService {
			t.Fatal("Un usuario sin permiso puso la mesa fuera de servicio")
		}
		events := base.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='restaurant.tables.changed' AND scope='restaurant'")
		body := map[string]any{"operation_id": randomID(), "revision": table.Revision, "out_of_service": true}
		disabled := decodeResponse[operations.Table](t, fixture.adminCall(t, "POST", "restaurant/tables/"+table.ID+"/service", body, 200))
		if !disabled.OutOfService || disabled.State != "out_of_service" || disabled.Revision != table.Revision+1 {
			t.Fatalf("La mesa no quedó fuera de servicio: %+v", disabled)
		}
		replay := decodeResponse[operations.Table](t, fixture.adminCall(t, "POST", "restaurant/tables/"+table.ID+"/service", body, 200))
		if replay.Revision != disabled.Revision {
			t.Fatal("El reintento cambió otra vez la mesa")
		}
		if auditCount(t, fixture, "restaurant.table.out_of_service", table.ID) != 1 {
			t.Fatal("Poner fuera de servicio no quedó auditado una sola vez")
		}
		if base.countRows(t, "SELECT count(*) FROM app.realtime_events WHERE kind='restaurant.tables.changed' AND scope='restaurant'") != events+1 {
			t.Fatal("El cambio de servicio no publicó su evento")
		}
		expectTableState(t, fixture, table.ID, "out_of_service", "0.00")
		// No admite cuentas nuevas mientras está fuera de servicio.
		openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 400)
		// Revisión obsoleta y estado repetido.
		tableService(t, fixture, fixture.admin, table, false, 409)
		tableService(t, fixture, fixture.admin, disabled, true, 400)
		enabled := decodeResponse[operations.Table](t, tableService(t, fixture, fixture.admin, disabled, false, 200))
		if enabled.OutOfService || enabled.State != "free" || auditCount(t, fixture, "restaurant.table.enabled", table.ID) != 1 {
			t.Fatal("Habilitar la mesa no la dejó libre y auditada")
		}
		// Con una cuenta abierta no se pone fuera de servicio, ni por la operación ni por el PATCH.
		bill := decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200))
		current := tableByID(t, fixture, table.ID)
		if code := decodeProblem(t, tableService(t, fixture, fixture.admin, current, true, 409)).Error.Code; code != "TABLE_HAS_OPEN_ACCOUNTS" {
			t.Fatalf("Rechazo esperado TABLE_HAS_OPEN_ACCOUNTS: %s", code)
		}
		current.OutOfService = true
		if code := decodeProblem(t, fixture.mutate(t, "PATCH", "restaurant/tables/"+table.ID, current, 409)).Error.Code; code != "TABLE_HAS_OPEN_ACCOUNTS" {
			t.Fatalf("El PATCH debe rechazar fuera de servicio con cuentas abiertas: %s", code)
		}
		if tableByID(t, fixture, table.ID).OutOfService {
			t.Fatal("La mesa con cuenta abierta quedó fuera de servicio")
		}
		// Sin cuentas abiertas, el PATCH vuelve a admitirlo.
		cleared := decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+bill.Account.ID+"/draft", map[string]any{
			"revision": bill.Account.Revision, "items": []operations.DraftItem{},
		}, 200))
		release(t, fixture, fixture.admin, cleared, 200)
		current = tableByID(t, fixture, table.ID)
		current.OutOfService = true
		patched := decodeResponse[operations.Table](t, fixture.mutate(t, "PATCH", "restaurant/tables/"+table.ID, current, 200))
		if patched.State != "out_of_service" {
			t.Fatal("El PATCH no puso fuera de servicio una mesa libre")
		}
	})

	t.Run("71B_permisos_antes_de_escribir", func(t *testing.T) {
		table := quickTable(t, fixture, "Mesa 71 permisos", 580)
		host := newCashier(t, base, "anfitrion_mesas", []string{"accounts.read", "accounts.open", "restaurant.tables.read", "restaurant.areas.read"}, "")
		reader := newCashier(t, base, "lector_mesas", []string{"accounts.read", "restaurant.tables.read", "restaurant.areas.read"}, "")
		before := recordCounts(t, fixture)
		// Con productos exige además los permisos del borrador.
		openWithItems(t, fixture, host, table.ID, []string{}, draftOf(fries), 403)
		after := recordCounts(t, fixture)
		for _, table := range []string{"restaurant_accounts", "restaurant_operations", "restaurant_draft_items", "restaurant_events"} {
			if after[table] != before[table] {
				t.Fatalf("Una apertura denegada dejó registros en %s", table)
			}
		}
		var denied int
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM app.audit_events WHERE action='access.denied'
            AND actor_id=(SELECT id FROM app.users WHERE username='anfitrion_mesas')`).Scan(&denied); err != nil {
			t.Fatal(err)
		}
		if denied != 1 {
			t.Fatal("La denegación no quedó auditada")
		}
		// Sin productos, abrir sigue exigiendo solo accounts.open.
		empty := decodeResponse[operations.Bill](t, fixture.call(t, host, "POST", "restaurant/accounts", map[string]any{
			"operation_id": randomID(), "mode": "dine_in", "table_id": table.ID, "seen_account_ids": []string{},
		}, 200))
		// Liberar exige accounts.open; cambiar el servicio, restaurant.tables.manage.
		release(t, fixture, reader, empty, 403)
		tableService(t, fixture, host, tableByID(t, fixture, table.ID), false, 403)
		release(t, fixture, host, empty, 200)
	})
}
