//go:build integration

package operations_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/payments"
)

// cashFixture reúne los datos de la aceptación de caja de la etapa 7. Usa cajas y terminales
// propias para no depender del turno principal de la aceptación de cobros.
type cashFixture struct {
	*paymentsFixture
	registerID string
	// cashier registra y cuenta sin permisos de autorizar ni de consultar el esperado; partner es
	// un segundo cajero de la misma caja; manager autoriza salidas y correcciones con su propio
	// permiso, pero no consulta el esperado; supervisor autoriza y consulta el esperado desde una
	// terminal sin caja; observer solo consulta turnos de su caja.
	cashier    testTerminal
	partner    testTerminal
	manager    testTerminal
	supervisor testTerminal
	observer   testTerminal
	cashierID  string
	// denominations identifica el catálogo por «MONEDA VALOR TIPO», por ejemplo «MXN 500.00 bill».
	denominations map[string]string
	categories    map[string]string
	shifts        map[string]string
	operations    map[string]string
	accounts      map[string]string
	payments      map[string]string
	reports       map[string]string
}

// Permisos de un cajero de caja: cobra, devuelve y registra movimientos, cortes y arqueos.
var cashierPermissions = []string{
	"accounts.read", "takeaway.read", "payments.read", "payments.collect", "payments.collect_partial", "payments.refunds.execute",
	"cash.shifts.open", "cash.shifts.read", "cash.income.record", "cash.withdrawals.record", "cash.expenses.record",
	"cash.corrections.record", "cash.movements.read", "cash.partial_cuts.create", "cash.counts.perform", "cash.shifts.close",
}

func authorizationOf(username string) map[string]any {
	return map[string]any{"username": username, "password": "Clave temporal de " + username}
}

// newCashFixture prepara la caja de arqueo, sus usuarios y el catálogo de caja.
func newCashFixture(t *testing.T, base *paymentsFixture) *cashFixture {
	t.Helper()
	fixture := &cashFixture{
		paymentsFixture: base, denominations: map[string]string{}, categories: map[string]string{}, shifts: map[string]string{},
		operations: map[string]string{}, accounts: map[string]string{}, payments: map[string]string{}, reports: map[string]string{},
	}
	fixture.registerID = fixture.createRegister(t, "Caja de arqueo")
	fixture.cashier = newCashier(t, base, "cajero_caja", cashierPermissions, fixture.registerID)
	fixture.partner = newCashier(t, base, "cajero_dos", []string{
		"accounts.read", "payments.read", "cash.shifts.read", "cash.income.record", "cash.counts.perform", "cash.movements.read",
	}, fixture.registerID)
	fixture.manager = newCashier(t, base, "encargado_caja", []string{
		"payments.read", "cash.shifts.read", "cash.withdrawals.record", "cash.expenses.record", "cash.outflows.authorize",
		"cash.corrections.record", "cash.corrections.authorize", "cash.movements.read",
	}, fixture.registerID)
	fixture.supervisor = newCashier(t, base, "supervisor_caja", []string{
		"payments.read", "cash.shifts.read", "cash.shifts.read_all", "cash.outflows.authorize", "cash.corrections.authorize",
		"cash.expected.read", "cash.history.read", "cash.closures.review",
	}, "")
	fixture.observer = newCashier(t, base, "observador_caja", []string{"payments.read", "cash.shifts.read"}, fixture.registerID)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.users WHERE username='cajero_caja'").Scan(&fixture.cashierID); err != nil {
		t.Fatal(err)
	}
	catalog := decodeResponse[struct {
		Denominations []payments.Denomination    `json:"denominations"`
		Categories    []payments.ExpenseCategory `json:"expense_categories"`
	}](t, fixture.adminCall(t, "GET", "payments/cash/catalog", nil, 200))
	for _, denomination := range catalog.Denominations {
		fixture.denominations[denomination.Currency+" "+denomination.Value+" "+denomination.Kind] = denomination.ID
	}
	for _, category := range catalog.Categories {
		fixture.categories[category.Name] = category.ID
	}
	return fixture
}

func (fixture *cashFixture) createRegister(t *testing.T, name string) string {
	t.Helper()
	return decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": name, "description": "", "active": true}, 200)).ID
}

// shiftCall envía una acción sobre un turno; las escrituras reciben un operation_id nuevo si no
// lo traen.
func (fixture *cashFixture) shiftCall(t *testing.T, terminal testTerminal, method, shiftID, action string, body map[string]any, status int) []byte {
	t.Helper()
	route := "payments/shifts/" + shiftID
	if action != "" {
		route += "/" + action
	}
	if method != "GET" {
		if body == nil {
			body = map[string]any{}
		}
		if _, found := body["operation_id"]; !found {
			body["operation_id"] = randomID()
		}
	}
	return fixture.call(t, terminal, method, route, body, status)
}

// openShift abre el turno de la caja de la terminal.
func (fixture *cashFixture) openShift(t *testing.T, terminal testTerminal, body map[string]any, status int) []byte {
	t.Helper()
	if _, found := body["operation_id"]; !found {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, terminal, "POST", "payments/shifts", body, status)
}

// raw envía una petición y devuelve código y cuerpo, para carreras entre terminales.
func (fixture *cashFixture) raw(terminal testTerminal, method, route string, body any) (int, []byte) {
	encoded, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:54000"
	request.Header.Set("X-Device-ID", terminal.deviceID)
	request.Header.Set("X-Device-Token", terminal.deviceToken)
	request.Header.Set("Authorization", "Bearer "+terminal.sessionToken)
	response := httptest.NewRecorder()
	fixture.service.ServeHTTP(response, request)
	return response.Code, append([]byte{}, response.Body.Bytes()...)
}

func (fixture *cashFixture) detail(t *testing.T, terminal testTerminal, shiftID string) payments.ShiftDetail {
	t.Helper()
	return decodeResponse[payments.ShiftDetail](t, fixture.shiftCall(t, terminal, "GET", shiftID, "", nil, 200))
}

// expected consulta como administrador el efectivo esperado vigente de un turno abierto.
func (fixture *cashFixture) expected(t *testing.T, shiftID string) payments.CashExpectation {
	t.Helper()
	detail := fixture.detail(t, fixture.admin, shiftID)
	if !detail.ExpectedVisible || detail.Expected == nil {
		t.Fatal("El administrador no recibió el efectivo esperado de un turno abierto")
	}
	return *detail.Expected
}

func (fixture *cashFixture) operation(t *testing.T, body []byte) payments.CashOperation {
	t.Helper()
	return decodeResponse[payments.CashOperation](t, body)
}

// line arma una línea de conteo por denominación.
func (fixture *cashFixture) line(t *testing.T, key string, quantity int) map[string]any {
	t.Helper()
	id, found := fixture.denominations[key]
	if !found {
		t.Fatalf("La denominación %s no está en el catálogo", key)
	}
	return map[string]any{"denomination_id": id, "quantity": quantity}
}

// pesosCount expresa un importe en pesos con billetes de $500 y $100, monedas de $10 y el resto
// como importe adicional identificado. El conteo lo valida y totaliza Go.
func (fixture *cashFixture) pesosCount(t *testing.T, value string) map[string]any {
	t.Helper()
	remaining := amountOf(t, value)
	if remaining.Sign() < 0 {
		t.Fatalf("No se puede contar un importe negativo: %s", value)
	}
	lines := []map[string]any{}
	for _, denomination := range []struct {
		key   string
		value int64
	}{{"MXN 500.00 bill", 500}, {"MXN 100.00 bill", 100}, {"MXN 10.00 coin", 10}} {
		size := decimal.NewFromInt(denomination.value)
		quantity := remaining.Div(size).Floor()
		lines = append(lines, fixture.line(t, denomination.key, int(quantity.IntPart())))
		remaining = remaining.Sub(quantity.Mul(size))
	}
	count := map[string]any{"lines": lines}
	if remaining.Sign() > 0 {
		count["extra_mxn"] = remaining.StringFixed(2)
		count["extra_note"] = "Monedas sueltas sin desglosar"
	}
	return count
}

// readClosing devuelve el arqueo en curso visto por la terminal indicada.
func (fixture *cashFixture) readClosing(t *testing.T, terminal testTerminal, shiftID string) payments.ClosingView {
	t.Helper()
	return decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, terminal, "GET", shiftID, "closing", nil, 200))
}

// saveCount guarda el borrador del arqueo con la revisión vigente.
func (fixture *cashFixture) saveCount(t *testing.T, terminal testTerminal, shiftID string, count map[string]any, notes string) payments.ClosingView {
	t.Helper()
	current := fixture.readClosing(t, terminal, shiftID)
	body := map[string]any{"revision": current.Count.Revision, "notes": notes}
	for key, value := range count {
		body[key] = value
	}
	return decodeResponse[payments.ClosingView](t, fixture.shiftCall(t, terminal, "PUT", shiftID, "closing/count", body, 200))
}

// blind verifica que una respuesta no contenga el efectivo esperado ni diferencias.
func blind(t *testing.T, body []byte, context string) {
	t.Helper()
	text := string(body)
	for _, fragment := range []string{`"expected":`, `"expected_cash"`, `"by_currency"`, `"sensitive"`, `"differences"`, `"expected_mxn"`, `"difference_mxn"`} {
		if strings.Contains(text, fragment) {
			t.Fatalf("%s reveló %s: %s", context, fragment, text)
		}
	}
}

func (fixture *cashFixture) userID(t *testing.T, username string) string {
	t.Helper()
	var id string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id FROM app.users WHERE username=$1", username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// testCashShifts encadena la aceptación de la etapa 7 después de la de cobros y dólares.
func testCashShifts(t *testing.T, base *paymentsFixture) {
	fixture := newCashFixture(t, base)
	testCashOpening(t, fixture)
	testCashOperations(t, fixture)
	testCashCorrections(t, fixture)
	testCashPartialCuts(t, fixture)
	testCashClosing(t, fixture)
	testCashRegularization(t, fixture)
	testCashUSD(t, fixture)
	testCashHistory(t, fixture)
	testCashViews(t, fixture)
	testCashPrinting(t, fixture)
}
