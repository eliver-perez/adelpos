//go:build integration

package operations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// testStage9A cubre la parte A1 de la etapa 9A, encadenada al final de TestRestaurantAcceptance:
// el precio de complementos configurable por producto (§3.1) en cotización, confirmación,
// historial, promociones, descuentos manuales e IVA, y la vigencia de la precuenta por revisión de
// la cuenta (§33), con pruebas concurrentes que controlan el lock 7820148305.
func testStage9A(t *testing.T, fixture *restaurantFixture) {
	base := &paymentsFixture{restaurantFixture: fixture, methods: map[string]payments.Method{}, registers: map[string]string{}}
	testStage9APricing(t, fixture, base)
	testStage9APrecheck(t, fixture, base)
}

// stage9ASides es la selección de guarniciones del filete con las opciones indicadas (una porción
// de cada una): Arroz $20, Ensalada $15, Papas $25 y Frijoles $10; incluidos Arroz y Ensalada.
func stage9ASides(fixture *restaurantFixture, names ...string) catalog.Customization {
	options := []catalog.IncludedOption{}
	for _, name := range names {
		for _, option := range fixture.group.Options {
			if option.Name == name {
				options = append(options, catalog.IncludedOption{OptionID: option.ID, Quantity: "1"})
			}
		}
	}
	return catalog.Customization{Groups: []catalog.GroupSelection{{GroupID: fixture.group.ID, Options: options}}}
}

// stage9AVersion lee del pedido confirmado la cotización congelada de la partida, su tasa de IVA y
// la opción congelada en el snapshot del pedido.
type stage9AVersion struct {
	Adjustment *string
	Decrease   *string
	Difference string
	TaxRate    int
	Snapshot   *string
}

func readStage9AVersion(t *testing.T, fixture *restaurantFixture, versionID, productID string) stage9AVersion {
	t.Helper()
	var version stage9AVersion
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT v.quote_line->>'complement_adjustment',v.quote_line->>'price_can_decrease',
        v.quote_line->'groups'->0->>'difference',v.tax_rate,o.pricing_snapshot->'Products'->$2->>'price_can_decrease'
        FROM app.restaurant_item_versions v JOIN app.restaurant_orders o ON o.id=v.order_id WHERE v.id=$1`, versionID, productID).
		Scan(&version.Adjustment, &version.Decrease, &version.Difference, &version.TaxRate, &version.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func testStage9APricing(t *testing.T, fixture *restaurantFixture, base *paymentsFixture) {
	eight := 8
	flexible := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: "Filete flexible 9A", Active: true}, CategoryID: fixture.category.ID, Code: "FILETE-9A",
		Kind: "product", BasePrice: "150", TaxRate: &eight, PriceCanDecrease: true,
		Groups: []catalog.ProductGroup{{
			GroupID: fixture.group.ID,
			Included: []catalog.IncludedOption{
				{OptionID: fixture.group.Options[0].ID, Quantity: "1"},
				{OptionID: fixture.group.Options[1].ID, Quantity: "1"},
			},
		}},
	}, 200))
	cheaper := stage9ASides(fixture, "Ensalada", "Frijoles")
	line := catalog.LineRequest{ProductID: flexible.ID, Quantity: 1, Customization: cheaper}
	// confirmed es la cuenta con la partida confirmada con la opción activada.
	var confirmed operations.Bill

	t.Run("9A_31_cotizacion_y_confirmacion_con_la_opcion", func(t *testing.T) {
		if !flexible.PriceCanDecrease {
			t.Fatal("El producto no guardó la opción")
		}
		// La cotización de la captura ya muestra el precio reducido: $150 − ($35 − $25) = $140.
		bill := newAccount(t, fixture, "Precio que baja 9A")
		route := "restaurant/accounts/" + bill.Account.ID
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", route+"/draft-preview", map[string]any{
			"revision": bill.Account.Revision, "items": draftOf(line),
		}, 200))
		quoted := preview.Quote.Lines[0]
		if preview.Quote.Total != "140.00" || quoted.Groups[0].Difference != "-10.00" || quoted.ComplementAdjustment != "-10.00" || !quoted.PriceCanDecrease {
			t.Fatalf("Cotización con la opción: %s %+v", preview.Quote.Total, quoted.ProductBreakdown)
		}
		bill = fixture.capture(t, bill, []catalog.LineRequest{line})
		if bill.Account.Totals.Total != "140.00" {
			t.Fatalf("Confirmación con la opción: %s", bill.Account.Totals.Total)
		}
		version := readStage9AVersion(t, fixture, bill.Lines[0].VersionID, flexible.ID)
		if version.Adjustment == nil || *version.Adjustment != "-10.00" || version.Decrease == nil || *version.Decrease != "true" ||
			version.Difference != "-10.00" || version.Snapshot == nil || *version.Snapshot != "true" {
			t.Fatalf("La partida no conservó la regla y su efecto: %+v", version)
		}
		// IVA por producto sin cambio de regla: la tasa propia (8 %) se copia a la versión y el
		// desglose se hace sobre el importe reducido (140 / 1.08 = 129.63 + 10.37).
		finance := base.finance(t, bill.Account.ID)
		if version.TaxRate != 8 || len(finance.Tax) != 1 || finance.Tax[0].Rate != 8 || finance.Tax[0].Total != "140.00" ||
			finance.Tax[0].Base != "129.63" || finance.Tax[0].Tax != "10.37" {
			t.Fatalf("IVA de la partida reducida: tasa %d, desglose %+v", version.TaxRate, finance.Tax)
		}
		confirmed = bill
	})

	t.Run("9A_31_historial_al_cambiar_la_opcion", func(t *testing.T) {
		bill := confirmed
		current := decodeResponse[catalog.Product](t, fixture.adminCall(t, "GET", "catalog/products/"+flexible.ID, nil, 200))
		current.PriceCanDecrease = false
		flexible = decodeResponse[catalog.Product](t, fixture.adminCall(t, "PATCH", "catalog/products/"+flexible.ID, current, 200))
		if flexible.PriceCanDecrease {
			t.Fatal("La opción no se desactivó")
		}
		// La partida confirmada conserva su precio y su cotización.
		bill = fixture.refresh(t, bill.Account.ID)
		version := readStage9AVersion(t, fixture, bill.Lines[0].VersionID, flexible.ID)
		if bill.Account.Totals.Total != "140.00" || version.Adjustment == nil || *version.Adjustment != "-10.00" {
			t.Fatalf("Cambiar la opción alteró la partida confirmada: %s %+v", bill.Account.Totals.Total, version)
		}
		// Cambiar solo las observaciones conserva los importes históricos (camino «Preserve»).
		request := bill.Lines[0].Request
		request.Quantity = bill.Lines[0].Quantity
		request.Customization.Notes = "Término medio"
		edit := operations.EditInput{
			Mutation: operations.Mutation{Revision: bill.Account.Revision}, VersionID: bill.Lines[0].VersionID, Line: request,
			Reason: "Indicación del cliente",
		}
		preview := decodeResponse[operations.QuoteResult](t, fixture.adminCall(t, "POST", "restaurant/accounts/"+bill.Account.ID+"/edit-preview", edit, 200))
		if preview.Quote.Total != "140.00" {
			t.Fatalf("Cambiar observaciones volvió a cotizar la partida: %s", preview.Quote.Total)
		}
		// Una partida nueva con la misma selección usa la regla vigente: el piso.
		other := fixture.capture(t, newAccount(t, fixture, "Opción desactivada 9A"), []catalog.LineRequest{line})
		if other.Account.Totals.Total != "150.00" {
			t.Fatalf("Sin la opción la sustitución no debe bajar el precio: %s", other.Account.Totals.Total)
		}
		if version := readStage9AVersion(t, fixture, other.Lines[0].VersionID, flexible.ID); version.Adjustment != nil || version.Decrease != nil {
			t.Fatalf("Sin la opción la partida no debe registrar los campos nuevos: %+v", version)
		}
		flexible.PriceCanDecrease = true
		flexible = decodeResponse[catalog.Product](t, fixture.adminCall(t, "PATCH", "catalog/products/"+flexible.ID, flexible, 200))
	})

	t.Run("9A_31_promocion_y_descuento_manual_sin_doble_descuento", func(t *testing.T) {
		promotion := decodeResponse[catalog.Promotion](t, fixture.adminCall(t, "POST", "catalog/promotions", catalog.Promotion{
			Metadata: catalog.Metadata{Name: "10 % filete flexible", Active: true}, Kind: "percent", Percent: "10", SpecialPrice: "0",
			BuyQuantity: 2, PayQuantity: 1, MinimumQuantity: 1, ProductIDs: []string{flexible.ID},
		}, 200))
		// Orden: complementos (−$10), promoción sobre $140 (−$14.00, no −$15.00) y descuento manual
		// sobre lo que se debe ($126.00 → −$12.60).
		bill := fixture.capture(t, newAccount(t, fixture, "Promoción 9A"), []catalog.LineRequest{line})
		if bill.Account.Totals.Subtotal != "140.00" || bill.Account.Totals.Discount != "14.00" || bill.Account.Totals.Total != "126.00" {
			t.Fatalf("Promoción sobre el precio reducido: %+v", bill.Account.Totals)
		}
		finance := base.finance(t, bill.Account.ID)
		discounted := decodeResponse[adjustmentResult](t, base.post(t, fixture.admin, bill.Account.ID, "discounts", map[string]any{
			"scope": "items", "method": "percent", "value": "10", "reason": "Cliente frecuente",
			"selections":       []map[string]any{{"version_id": bill.Lines[0].VersionID, "quantity": 1}},
			"expected_balance": finance.Balance,
		}, 200))
		if discounted.Finance.ItemDiscounts != "12.60" || discounted.Finance.Total != "113.40" {
			t.Fatalf("Descuento manual después de la promoción: %s %s", discounted.Finance.ItemDiscounts, discounted.Finance.Total)
		}
		var subtotal, discount, total string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT subtotal::text,discount::text,total::text FROM app.restaurant_item_units
            WHERE version_id=$1`, bill.Lines[0].VersionID).Scan(&subtotal, &discount, &total); err != nil {
			t.Fatal(err)
		}
		if subtotal != "140.00" || discount != "14.00" || total != "126.00" {
			t.Fatalf("La unidad no conserva complemento → promoción: %s %s %s", subtotal, discount, total)
		}
		if tax := discounted.Finance.Tax; len(tax) != 1 || tax[0].Rate != 8 || tax[0].Total != "113.40" || tax[0].Base != "105.00" || tax[0].Tax != "8.40" {
			t.Fatalf("IVA después de promoción y descuento: %+v", tax)
		}
		promotion.Active = false
		fixture.adminCall(t, "PATCH", "catalog/promotions/"+promotion.ID, promotion, 200)
	})
}

// waitingOperationLock cuenta las transacciones que esperan el lock 7820148305 (clave de 64 bits:
// classid con los 32 bits altos y objid con los bajos).
const waitingOperationLock = `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted
    AND ((classid::bigint << 32) | objid::bigint)=7820148305 AND objsubid=1`

// waitForCount espera hasta 15 s a que la consulta devuelva el número indicado.
func waitForCount(t *testing.T, fixture *restaurantFixture, description, query string, want int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var count int
		if err := fixture.pool.QueryRow(fixture.ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("No se alcanzó el estado esperado (%s): %d de %d", description, count, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type stage9AResponse struct {
	status int
	body   string
}

func sendAsync(fixture *restaurantFixture, terminal testTerminal, method, route string, body any) chan stage9AResponse {
	encoded, _ := json.Marshal(body)
	result := make(chan stage9AResponse, 1)
	go func() {
		request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
		request.RemoteAddr = "127.0.0.1:54000"
		request.Header.Set("X-Device-ID", terminal.deviceID)
		request.Header.Set("X-Device-Token", terminal.deviceToken)
		request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
		response := httptest.NewRecorder()
		fixture.service.ServeHTTP(response, request)
		result <- stage9AResponse{status: response.Code, body: response.Body.String()}
	}()
	return result
}

func receiveResponse(t *testing.T, description string, channel chan stage9AResponse) stage9AResponse {
	t.Helper()
	select {
	case response := <-channel:
		if response.status != 200 {
			t.Fatalf("%s: %d %s", description, response.status, response.body)
		}
		return response
	case <-time.After(30 * time.Second):
		t.Fatalf("%s no terminó", description)
	}
	return stage9AResponse{}
}

// precheckRace imprime la precuenta de una cuenta mientras otra terminal la modifica, en el orden
// que engañaba a la regla por marcas de tiempo (etapa 7.1, §33): la modificación inicia su
// transacción antes que la impresión (queda detenida al autenticarse por un bloqueo de la fila de
// su terminal que retiene la prueba) y después espera el lock 7820148305, que la prueba también
// retiene, en la fila detrás de la impresión. Al soltar el lock, la impresión registra la precuenta
// con la revisión anterior y la modificación se confirma después, con updated_at (o created_at del
// ajuste) igual al inicio de su transacción: anterior a la precuenta.
func precheckRace(t *testing.T, fixture *restaurantFixture, accountID string, modifier testTerminal, method, route string, body any) {
	t.Helper()
	ctx := fixture.ctx
	rowLock, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rowLock.Rollback(context.Background())
	if _, err := rowLock.Exec(ctx, "SELECT id FROM app.devices WHERE id=$1 FOR UPDATE", modifier.deviceID); err != nil {
		t.Fatal(err)
	}
	operationLock, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer operationLock.Rollback(context.Background())
	if _, err := operationLock.Exec(ctx, "SELECT pg_advisory_xact_lock(7820148305)"); err != nil {
		t.Fatal(err)
	}
	modification := sendAsync(fixture, modifier, method, route, body)
	waitForCount(t, fixture, "modificación detenida al autenticarse", `SELECT count(*) FROM pg_stat_activity
        WHERE wait_event_type='Lock' AND query LIKE 'UPDATE app.devices SET last_seen_at%'`, 1)
	precheck := sendAsync(fixture, fixture.admin, "POST", "payments/accounts/"+accountID+"/precheck/print", map[string]any{"operation_id": randomID()})
	waitForCount(t, fixture, "impresión esperando el lock", waitingOperationLock, 1)
	if err := rowLock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	waitForCount(t, fixture, "modificación esperando el lock detrás de la impresión", waitingOperationLock, 2)
	if err := operationLock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	receiveResponse(t, "La impresión de la precuenta", precheck)
	receiveResponse(t, "La modificación concurrente", modification)
}

// timestampRuleWouldShowCurrent evalúa la regla anterior por marcas de tiempo sobre la cuenta:
// precuenta posterior al último cambio de la cuenta y al último ajuste.
func timestampRuleWouldShowCurrent(t *testing.T, fixture *restaurantFixture, accountID string) bool {
	t.Helper()
	var current bool
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce((SELECT max(d.created_at) FROM app.payment_documents d
            WHERE d.account_id=a.id AND d.kind='precheck') > greatest(a.updated_at,
            (SELECT max(j.created_at) FROM app.account_adjustments j WHERE j.account_id=a.id),
            (SELECT max(v.created_at) FROM app.account_adjustment_voids v JOIN app.account_adjustments j ON j.id=v.adjustment_id
                WHERE j.account_id=a.id)),false)
        FROM app.restaurant_accounts a WHERE a.id=$1`, accountID).Scan(&current)
	if err != nil {
		t.Fatal(err)
	}
	return current
}

// lastPrecheck devuelve la revisión y el número de ajustes registrados con la última precuenta.
func lastPrecheck(t *testing.T, fixture *restaurantFixture, accountID string) (*int, *int) {
	t.Helper()
	var revision, adjustments *int
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT account_revision,adjustment_count FROM app.payment_documents
        WHERE account_id=$1 AND kind='precheck' ORDER BY created_at DESC LIMIT 1`, accountID).Scan(&revision, &adjustments)
	if err != nil {
		t.Fatal(err)
	}
	return revision, adjustments
}

func accountRevision(t *testing.T, fixture *restaurantFixture, accountID string) int {
	t.Helper()
	var revision int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT revision FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

func testStage9APrecheck(t *testing.T, fixture *restaurantFixture, base *paymentsFixture) {
	fries := catalog.LineRequest{ProductID: fixture.fries.ID, Quantity: 1}
	// Impresora de tickets sin hardware (una cola del equipo que no existe; estas pruebas no ejecutan el
	// trabajador): la precuenta queda en la cola, nunca se envía. Desde la etapa 9F una impresora de otra
	// terminal («agent») ya no es válida para precuentas.
	printer := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Precuentas 9A", Transport: "cups", Destination: "ADEL_TEST_PRECHECK_9A", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))
	base.receiptSettings(t, printer.ID, false)
	// Terminal propia que modifica las cuentas en las carreras: captura, datos y cargos.
	modifier := newCashier(t, base, "terminal_9a", []string{
		"accounts.read", "accounts.open", "accounts.manage", "orders.capture", "orders.confirm", "restaurant.tables.read",
		"restaurant.areas.read", "catalog.read", "catalog.prices.read", "payments.read", "payments.charges.create",
	}, "")
	printPrecheck := func(t *testing.T, accountID string) {
		t.Helper()
		base.post(t, fixture.admin, accountID, "precheck/print", map[string]any{}, 200)
	}
	openTable := func(t *testing.T, name string) (operations.Table, operations.Bill) {
		t.Helper()
		table := quickTable(t, fixture, name, 120)
		bill := confirmDraft(t, fixture, decodeResponse[operations.Bill](t, openWithItems(t, fixture, fixture.admin, table.ID, []string{}, draftOf(fries), 200)))
		return table, bill
	}

	t.Run("9A_33_vigencia_por_revision_y_conteo_de_ajustes", func(t *testing.T) {
		table, bill := openTable(t, "Mesa 9A revisión")
		printPrecheck(t, bill.Account.ID)
		revision, adjustments := lastPrecheck(t, fixture, bill.Account.ID)
		if revision == nil || *revision != accountRevision(t, fixture, bill.Account.ID) || adjustments == nil || *adjustments != 0 {
			t.Fatalf("La precuenta no guardó la revisión y los ajustes: %v %v", revision, adjustments)
		}
		expectTableState(t, fixture, table.ID, "awaiting_payment", "40.00")
		// Un cargo posterior la invalida, aunque no cambie la revisión de la cuenta.
		charge := decodeResponse[adjustmentResult](t, base.post(t, fixture.admin, bill.Account.ID, "charges", map[string]any{
			"concept": "Servicio", "amount": "5", "expected_balance": "40.00",
		}, 200))
		if accountRevision(t, fixture, bill.Account.ID) != *revision {
			t.Fatal("Un ajuste no debe cambiar la revisión de la cuenta (provocaría conflictos en otras capturas)")
		}
		expectTableState(t, fixture, table.ID, "occupied", "45.00")
		// Su cancelación tampoco devuelve la vigencia a la precuenta anterior.
		base.post(t, fixture.admin, bill.Account.ID, "adjustments/void", map[string]any{
			"adjustment_id": charge.AdjustmentID, "reason": "Cargo capturado por error",
		}, 200)
		expectTableState(t, fixture, table.ID, "occupied", "40.00")
		// Una precuenta nueva con los dos ajustes vuelve a estar vigente.
		printPrecheck(t, bill.Account.ID)
		if _, adjustments := lastPrecheck(t, fixture, bill.Account.ID); adjustments == nil || *adjustments != 2 {
			t.Fatalf("La precuenta nueva no contó el cargo y su cancelación: %v", adjustments)
		}
		expectTableState(t, fixture, table.ID, "awaiting_payment", "40.00")
		// Un ticket de pago no guarda revisión ni ajustes (solo las precuentas).
		if base.countRows(t, `SELECT count(*) FROM app.payment_documents WHERE kind='receipt' AND
            (account_revision IS NOT NULL OR adjustment_count IS NOT NULL)`) != 0 {
			t.Fatal("Un ticket de pago guardó la revisión de la cuenta")
		}
	})

	t.Run("9A_33_carrera_con_una_modificacion_que_espera_el_lock", func(t *testing.T) {
		table, bill := openTable(t, "Mesa 9A carrera")
		bill = fixture.refresh(t, bill.Account.ID)
		before := bill.Account.Revision
		// La terminal guarda un producto nuevo en el borrador mientras se imprime la precuenta.
		precheckRace(t, fixture, bill.Account.ID, modifier, "PUT", "restaurant/accounts/"+bill.Account.ID+"/draft", map[string]any{
			"operation_id": randomID(), "revision": before, "items": draftOf(fries),
		})
		revision, _ := lastPrecheck(t, fixture, bill.Account.ID)
		if revision == nil || *revision != before || accountRevision(t, fixture, bill.Account.ID) != before+1 {
			t.Fatalf("La carrera no ocurrió en el orden controlado: precuenta %v, cuenta %d", revision, accountRevision(t, fixture, bill.Account.ID))
		}
		// La regla anterior por marcas de tiempo la habría mostrado vigente (precuenta posterior al
		// inicio de la transacción de la modificación); la revisión la marca obsoleta.
		if !timestampRuleWouldShowCurrent(t, fixture, bill.Account.ID) {
			t.Fatal("La prueba no reprodujo la carrera: las marcas de tiempo ya invalidaban la precuenta")
		}
		expectTableState(t, fixture, table.ID, "occupied", "40.00")
	})

	t.Run("9A_33_carrera_con_un_cargo_que_espera_el_lock", func(t *testing.T) {
		table, bill := openTable(t, "Mesa 9A cargo")
		precheckRace(t, fixture, bill.Account.ID, modifier, "POST", "payments/accounts/"+bill.Account.ID+"/charges", map[string]any{
			"operation_id": randomID(), "concept": "Servicio", "amount": "5", "expected_balance": "40.00",
		})
		if _, adjustments := lastPrecheck(t, fixture, bill.Account.ID); adjustments == nil || *adjustments != 0 {
			t.Fatalf("La precuenta debía registrarse antes del cargo: %v", adjustments)
		}
		if !timestampRuleWouldShowCurrent(t, fixture, bill.Account.ID) {
			t.Fatal("La prueba no reprodujo la carrera del cargo")
		}
		expectTableState(t, fixture, table.ID, "occupied", "45.00")
	})

	t.Run("9A_33_precuentas_historicas_con_la_regla_anterior", func(t *testing.T) {
		table, bill := openTable(t, "Mesa 9A histórica")
		printPrecheck(t, bill.Account.ID)
		// Una precuenta anterior a la 0015 no tiene revisión: se simula vaciando sus columnas.
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.payment_documents SET account_revision=NULL,adjustment_count=NULL
            WHERE account_id=$1 AND kind='precheck'`, bill.Account.ID); err != nil {
			t.Fatal(err)
		}
		expectTableState(t, fixture, table.ID, "awaiting_payment", "40.00")
		// Un cambio de contenido posterior la invalida, como antes.
		bill = fixture.refresh(t, bill.Account.ID)
		bill = decodeResponse[operations.Bill](t, fixture.mutate(t, "PUT", "restaurant/accounts/"+bill.Account.ID+"/draft", map[string]any{
			"revision": bill.Account.Revision, "items": draftOf(fries),
		}, 200))
		expectTableState(t, fixture, table.ID, "occupied", "40.00")
		// Una precuenta histórica impresa antes del último ajuste tampoco es vigente.
		other, account := openTable(t, "Mesa 9A histórica con ajuste")
		printPrecheck(t, account.Account.ID)
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE app.payment_documents SET account_revision=NULL,adjustment_count=NULL
            WHERE account_id=$1 AND kind='precheck'`, account.Account.ID); err != nil {
			t.Fatal(err)
		}
		base.post(t, fixture.admin, account.Account.ID, "charges", map[string]any{
			"concept": "Servicio", "amount": "5", "expected_balance": "40.00",
		}, 200)
		expectTableState(t, fixture, other.ID, "occupied", "45.00")
	})
}
