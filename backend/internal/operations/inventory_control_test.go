//go:build integration

package operations_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/kitchen"
)

// Etapa 8, fase D: ajustes, mermas, transferencias, conteos físicos, niveles por almacén, alertas y
// sugerencias de compra (0014). Se encadena al final de la fase C con sus propios insumos, almacén
// («Barra D»), estación, catálogo, proveedor y usuarios. Las cifras de cada caso están calculadas a
// mano en los comentarios (motor de costos: salidas al promedio V/Q, entradas sin costo propio al
// promedio o al provisional del faltante).

type controlFixture struct {
	*inventoryFixture
	sales *recipesFixture
	// recorder registra ajustes sin autorizarlos; promoted registra y después recibe el permiso de
	// autorizar (para probar que nadie autoriza lo que registró); authorizer solo autoriza; wasteClerk
	// solo registra mermas; transferClerk solo transfiere; counter cuenta a ciegas; approver aprueba y
	// aplica conteos; buyer crea órdenes de compra; reader solo consulta inventario y eventos.
	recorder, promoted, authorizer, wasteClerk, transferClerk, counter, approver, buyer, reader testTerminal
	promotedRole                                                                                string
	items                                                                                       map[string]inventory.Item
	presentations                                                                               map[string]string
	barra                                                                                       inventory.Warehouse
	category                                                                                    string
	products                                                                                    map[string]catalog.Product
	supplier                                                                                    string
	order                                                                                       inventory.PurchaseOrder
}

// controlUser crea un rol a medida, un usuario y su terminal con sesión; devuelve también el rol.
func controlUser(t *testing.T, fixture *restaurantFixture, username string, permissions []string) (testTerminal, string) {
	t.Helper()
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Rol " + username, "permissions": permissions}, 200))
	password := "Clave temporal de " + username
	fixture.adminCall(t, "POST", "users", map[string]any{
		"username": username, "display_name": "Prueba " + username, "role_id": role.ID, "active": true, "password": password,
	}, 200)
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": "Terminal " + username}, 200))
	terminal := testTerminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200))
	terminal.sessionToken = login.Token
	return terminal, role.ID
}

func testInventoryControl(t *testing.T, base *inventoryFixture) {
	fixture := &controlFixture{
		inventoryFixture: base, sales: &recipesFixture{inventoryFixture: base}, items: map[string]inventory.Item{},
		presentations: map[string]string{}, products: map[string]catalog.Product{},
	}
	cursor := eventCursor(t, base.restaurantFixture)
	t.Run("inventory_d_setup", func(t *testing.T) {
		fixture.setup(t)
	})
	t.Run("inventory_d_adjustments_and_waste", func(t *testing.T) {
		testControlAdjustments(t, fixture)
	})
	t.Run("inventory_d_transfers", func(t *testing.T) {
		testControlTransfers(t, fixture)
	})
	t.Run("inventory_d_counts_with_concurrent_sales", func(t *testing.T) {
		testControlCounts(t, fixture)
	})
	t.Run("inventory_d_count_workflow", func(t *testing.T) {
		testControlCountWorkflow(t, fixture)
	})
	t.Run("inventory_d_stock_levels", func(t *testing.T) {
		testControlStockLevels(t, fixture)
	})
	t.Run("inventory_d_alerts", func(t *testing.T) {
		testControlAlerts(t, fixture)
	})
	t.Run("inventory_d_purchase_suggestions", func(t *testing.T) {
		testControlSuggestions(t, fixture)
	})
	t.Run("inventory_d_entries_without_cost", func(t *testing.T) {
		testControlEntriesWithoutCost(t, fixture)
	})
	t.Run("inventory_d_events_audit_and_ledger", func(t *testing.T) {
		testControlEventsAndAudit(t, fixture, cursor)
	})
}

func (fixture *controlFixture) setup(t *testing.T) {
	base := fixture.restaurantFixture
	units := fixture.units
	user := func(username string, permissions ...string) testTerminal {
		terminal, _ := controlUser(t, base, username, permissions)
		return terminal
	}
	fixture.recorder = user("control_ajustes", "events.read", "inventory.read", "inventory.adjustments.record", "inventory.stock.read")
	fixture.promoted, fixture.promotedRole = controlUser(t, base, "control_promovido", []string{"inventory.read",
		"inventory.adjustments.record"})
	fixture.authorizer = user("control_autoriza", "inventory.read", "inventory.adjustments.authorize")
	fixture.wasteClerk = user("control_mermas", "inventory.read", "inventory.waste.record")
	fixture.transferClerk = user("control_traslados", "inventory.read", "inventory.transfers.record")
	fixture.counter = user("control_cuenta", "inventory.read", "inventory.counts.perform")
	fixture.approver = user("control_aprueba", "inventory.read", "inventory.counts.approve")
	fixture.buyer = user("control_compras", "inventory.read", "inventory.purchases.create")
	fixture.reader = user("control_lector", "events.read", "inventory.read")
	fixture.category = decodeResponse[inventory.Category](t, fixture.adminWrite(t, "POST", "inventory/categories",
		map[string]any{"name": "Bebidas D", "active": true}, 200)).ID
	for _, entry := range []struct {
		key, code, name, unit string
		category              bool
		minimum, reorder      any
	}{
		{"aceite", "D-ACE", "Aceite D", "ml", false, nil, nil},
		{"vino", "D-VIN", "Vino tinto D", "pza", true, "60", "70"},
		{"limon", "D-LIM", "Limón D", "pza", true, nil, nil},
		{"azucar", "D-AZU", "Azúcar D", "g", false, nil, nil},
		{"hielo", "D-HIE", "Hielo D", "pza", false, nil, nil},
	} {
		body := map[string]any{"code": entry.code, "name": entry.name, "base_unit_id": units[entry.unit].ID, "kind": "purchased",
			"active": true, "min_stock": entry.minimum, "reorder_point": entry.reorder}
		if entry.category {
			body["category_id"] = fixture.category
		}
		fixture.items[entry.key] = fixture.createItem(t, body)
	}
	for _, entry := range []struct{ key, item, name, quantity string }{
		{"caja", "vino", "Caja con 6", "6"}, {"bolsa", "limon", "Bolsa con 12", "12"},
	} {
		item := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items/"+fixture.items[entry.item].ID+"/presentations",
			map[string]any{"name": entry.name, "quantity": entry.quantity, "unit_id": units["pza"].ID, "active": true,
				"purchase_default": true}, 200))
		for _, presentation := range item.Presentations {
			if presentation.Name == entry.name {
				fixture.presentations[entry.key] = presentation.ID
			}
		}
		fixture.items[entry.item] = item
	}
	fixture.barra = decodeResponse[inventory.Warehouse](t, fixture.adminWrite(t, "POST", "inventory/warehouses",
		map[string]any{"name": "Barra D", "active": true}, 200))
	// Estación de barra (pantalla de cocina, sin impresora) que consume de «Barra D».
	station := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Inv Barra D", Active: true, Reception: "kds", TargetMinutes: 5, WarningMinutes: 8, OverdueMinutes: 12,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.adminWrite(t, "PUT", "inventory/station-warehouses/"+station.ID, map[string]any{"revision": 0, "warehouse_id": fixture.barra.ID},
		200)
	category := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories",
		catalog.Category{Metadata: catalog.Metadata{Name: "Inventario D", Active: true}}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &category.ID, Mode: "prepare", StationIDs: []string{station.ID}}, 200)
	for key, entry := range map[string]struct{ name, code string }{"limonada": {"Limonada D", "INV-LIMD"}, "agua": {"Agua fresca D", "INV-AGUAD"}} {
		fixture.products[key] = decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
			Metadata: catalog.Metadata{Name: entry.name, Active: true}, CategoryID: category.ID, Code: entry.code, Kind: "product",
			BasePrice: "30",
		}, 200))
	}
	// Cada limonada consume 3 limones de la barra; el agua fresca no tiene receta (alerta).
	fixture.adminWrite(t, "POST", "inventory/recipes", map[string]any{
		"owner_kind": "product", "product_id": fixture.products["limonada"].ID, "active": true,
		"lines": []map[string]any{recipeItemLine(fixture.items["limon"], "3", units["pza"], false)},
	}, 200)
	// Existencias iniciales: aceite 10,000 ml a $0.05 ($500) y vino 50 botellas a $120 ($6,000) en el
	// almacén general; limón 100 piezas a $2 ($200) y azúcar 5,000 g a $0.02 ($100) en la barra.
	line := func(key, quantity, cost string) map[string]any {
		return map[string]any{"item_id": fixture.items[key].ID, "quantity": quantity, "unit_cost": cost}
	}
	fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
		"warehouse_id": fixture.general.ID, "lines": []map[string]any{line("aceite", "10000", "0.05"), line("vino", "50", "120")},
	}, 200)
	fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
		"warehouse_id": fixture.barra.ID, "lines": []map[string]any{line("limon", "100", "2"), line("azucar", "5000", "0.02")},
	}, 200)
	fixture.sales.setFeature(t, true)
}

// stock devuelve la existencia de un insumo en un almacén («0» si no tiene fila).
func (fixture *controlFixture) stock(t *testing.T, key, warehouseID string) string {
	t.Helper()
	var quantity string
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce((SELECT quantity FROM app.inventory_stock WHERE item_id=$1 AND warehouse_id=$2),0)::text`,
		fixture.items[key].ID, warehouseID).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return decimal.RequireFromString(quantity).String()
}

// valuation devuelve «cantidad/valor» de la valuación de un insumo.
func (fixture *controlFixture) valuation(t *testing.T, key string) string {
	t.Helper()
	var quantity, value string
	err := fixture.pool.QueryRow(fixture.ctx, "SELECT quantity::text,value::text FROM app.inventory_valuations WHERE item_id=$1",
		fixture.items[key].ID).Scan(&quantity, &value)
	if err != nil {
		t.Fatal(err)
	}
	return decimal.RequireFromString(quantity).String() + "/" + decimal.RequireFromString(value).String()
}

func (fixture *controlFixture) movements(t *testing.T, sourceID string) int {
	t.Helper()
	return rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements WHERE source_id=$1", sourceID)
}

func (fixture *controlFixture) adjust(t *testing.T, terminal testTerminal, body map[string]any, status int) inventory.Adjustment {
	t.Helper()
	response := fixture.write(t, terminal, "POST", "inventory/adjustments", body, status)
	if status != 200 {
		return inventory.Adjustment{}
	}
	return decodeResponse[inventory.Adjustment](t, response)
}

// adjustmentBody arma un ajuste del almacén general con un renglón de aceite.
func (fixture *controlFixture) adjustmentBody(kind, reason, quantity string, extra map[string]any) map[string]any {
	line := map[string]any{"item_id": fixture.items["aceite"].ID, "quantity": quantity}
	for key, value := range extra {
		line[key] = value
	}
	return map[string]any{"kind": kind, "reason": reason, "warehouse_id": fixture.general.ID, "lines": []map[string]any{line}}
}

func warningCodes(warnings []inventory.Warning) []string {
	codes := []string{}
	for _, warning := range warnings {
		codes = append(codes, warning.Code)
	}
	return codes
}

func testControlAdjustments(t *testing.T, fixture *controlFixture) {
	general := fixture.general.ID
	// Sobrante de 200 ml registrado por quien puede autorizar: se aplica al registrarlo, al promedio
	// 500 ÷ 10,000 = 0.05 → +$10 (10,200 ml, $510).
	operation := randomID()
	surplus := fixture.adjustmentBody("discrepancy", "surplus", "200", nil)
	surplus["operation_id"] = operation
	raw := fixture.adminCall(t, "POST", "inventory/adjustments", surplus, 200)
	applied := decodeResponse[inventory.Adjustment](t, raw)
	if applied.State != "applied" || applied.Kind != "discrepancy" || applied.ReasonLabel != "Sobrante" || applied.AppliedAt == nil ||
		applied.AuthorizedByName != nil || len(applied.Lines) != 1 || text(applied.Lines[0].MovementKind) != "adjustment_in" ||
		applied.Lines[0].Direction != "in" || text(applied.Value) != "10.00" || text(applied.Lines[0].StockAfter) != "10200" ||
		fixture.valuation(t, "aceite") != "10200/510" {
		t.Fatalf("Sobrante aplicado: %+v %s", applied, fixture.valuation(t, "aceite"))
	}
	// Repetir el mismo operation_id devuelve el mismo ajuste sin movimientos nuevos; otro cuerpo, 409.
	replayed := decodeResponse[inventory.Adjustment](t, fixture.adminCall(t, "POST", "inventory/adjustments", surplus, 200))
	if replayed.ID != applied.ID || fixture.movements(t, applied.ID) != 1 {
		t.Fatalf("Reintento del ajuste: %s %s %d", replayed.ID, applied.ID, fixture.movements(t, applied.ID))
	}
	surplus["notes"] = "Otra nota"
	if errorCode(t, fixture.adminCall(t, "POST", "inventory/adjustments", surplus, 409)) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("Un cuerpo distinto con el mismo operation_id debe rechazarse")
	}
	// Rotura de 1 l (1,000 ml) capturada en litros: salida al promedio 510 ÷ 10,200 → −$50 exactos.
	breakage := fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "breakage", "-1",
		map[string]any{"unit_id": fixture.units["l"].ID}), 200)
	if text(breakage.Lines[0].MovementKind) != "adjustment_out" || breakage.Lines[0].Quantity != "-1000" ||
		breakage.Lines[0].CapturedQuantity != "-1" || breakage.Lines[0].CaptureLabel != "Litro" || text(breakage.Value) != "-50.00" ||
		fixture.valuation(t, "aceite") != "9200/460" {
		t.Fatalf("Rotura: %+v %s", breakage, fixture.valuation(t, "aceite"))
	}
	// Faltante físico de 300 ml por quien no puede autorizar: queda pendiente, sin movimientos.
	pending := fixture.adjust(t, fixture.recorder, fixture.adjustmentBody("discrepancy", "physical_shortage", "-300", nil), 200)
	if pending.State != "pending_authorization" || pending.AppliedAt != nil || pending.Lines[0].MovementID != nil ||
		pending.Authorizable || fixture.movements(t, pending.ID) != 0 || fixture.stock(t, "aceite", general) != "9200" ||
		pending.Value != nil {
		t.Fatalf("Ajuste pendiente: %+v", pending)
	}
	visible := decodeResponse[inventory.Adjustment](t, fixture.call(t, fixture.authorizer, "GET", "inventory/adjustments/"+pending.ID, nil, 200))
	if !visible.Authorizable {
		t.Fatal("Otra persona con el permiso debe poder autorizarlo")
	}
	route := "inventory/adjustments/" + pending.ID
	fixture.write(t, fixture.recorder, "POST", route+"/authorize", map[string]any{"revision": pending.Revision}, 403)
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", route+"/authorize", map[string]any{"revision": 7}, 409)) !=
		"REVISION_CONFLICT" {
		t.Fatal("La revisión obsoleta debe rechazarse")
	}
	// Autorizado por otra persona: salida de 300 ml al promedio 460 ÷ 9,200 → −$15.
	authorized := decodeResponse[inventory.Adjustment](t, fixture.write(t, fixture.authorizer, "POST", route+"/authorize",
		map[string]any{"revision": pending.Revision}, 200))
	if authorized.State != "applied" || text(authorized.AuthorizedByName) != "Prueba control_autoriza" || authorized.AppliedAt == nil ||
		text(authorized.Lines[0].MovementKind) != "adjustment_out" || authorized.Value != nil || fixture.movements(t, pending.ID) != 1 ||
		fixture.valuation(t, "aceite") != "8900/445" {
		t.Fatalf("Ajuste autorizado: %+v %s", authorized, fixture.valuation(t, "aceite"))
	}
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", route+"/authorize", map[string]any{"revision": authorized.Revision},
		409)) != "ADJUSTMENT_NOT_PENDING" {
		t.Fatal("Un ajuste aplicado no se autoriza otra vez")
	}
	// Sin autoautorización: quien registró el ajuste no lo autoriza ni lo rechaza aunque reciba el permiso.
	own := fixture.adjust(t, fixture.promoted, fixture.adjustmentBody("discrepancy", "spill", "-100", nil), 200)
	fixture.adminCall(t, "PATCH", "roles/"+fixture.promotedRole, map[string]any{"name": "Rol control_promovido", "permissions": []string{
		"inventory.read", "inventory.adjustments.record", "inventory.adjustments.authorize",
	}}, 200)
	ownRoute := "inventory/adjustments/" + own.ID
	for _, action := range []string{"/authorize", "/reject"} {
		body := map[string]any{"revision": own.Revision}
		if action == "/reject" {
			body["reason"] = "Me equivoqué al registrarlo"
		}
		if errorCode(t, fixture.write(t, fixture.promoted, "POST", ownRoute+action, body, 403)) != "SELF_AUTHORIZATION_FORBIDDEN" {
			t.Fatalf("Autodecisión %s aceptada", action)
		}
	}
	fixture.write(t, fixture.authorizer, "POST", ownRoute+"/reject", map[string]any{"revision": own.Revision, "reason": "No"}, 400)
	rejected := decodeResponse[inventory.Adjustment](t, fixture.write(t, fixture.authorizer, "POST", ownRoute+"/reject",
		map[string]any{"revision": own.Revision, "reason": "No se encontró el derrame"}, 200))
	if rejected.State != "rejected" || text(rejected.RejectionReason) != "No se encontró el derrame" || fixture.movements(t, own.ID) != 0 ||
		text(rejected.RejectedByName) != "Prueba control_autoriza" {
		t.Fatalf("Ajuste rechazado: %+v", rejected)
	}
	// Con el permiso, sus ajustes nuevos se aplican al registrarlos: +100 ml al promedio 445 ÷ 8,900 = +$5.
	direct := fixture.adjust(t, fixture.promoted, fixture.adjustmentBody("discrepancy", "entry_error", "100", nil), 200)
	if direct.State != "applied" || fixture.valuation(t, "aceite") != "9000/450" {
		t.Fatalf("Ajuste directo del promovido: %+v %s", direct, fixture.valuation(t, "aceite"))
	}
	// Merma operativa: se aplica directamente como waste (500 ml al promedio 450 ÷ 9,000 = −$25).
	waste := fixture.adjust(t, fixture.wasteClerk, fixture.adjustmentBody("waste", "breakage", "-500", nil), 200)
	if waste.State != "applied" || waste.KindLabel != "Merma operativa" || text(waste.Lines[0].MovementKind) != "waste" ||
		fixture.valuation(t, "aceite") != "8500/425" {
		t.Fatalf("Merma: %+v %s", waste, fixture.valuation(t, "aceite"))
	}
	// La merma solo admite salidas y motivos de pérdida; cada tipo exige su permiso; el ajuste negativo
	// sigue siendo adjustment_out (no se convierte en merma).
	fixture.adjust(t, fixture.wasteClerk, fixture.adjustmentBody("waste", "breakage", "5", nil), 400)
	fixture.adjust(t, fixture.wasteClerk, fixture.adjustmentBody("waste", "surplus", "-5", nil), 400)
	fixture.adjust(t, fixture.wasteClerk, fixture.adjustmentBody("discrepancy", "breakage", "-5", nil), 403)
	fixture.adjust(t, fixture.recorder, fixture.adjustmentBody("waste", "breakage", "-5", nil), 403)
	fixture.adjust(t, fixture.reader, fixture.adjustmentBody("discrepancy", "breakage", "-5", nil), 403)
	// Merma que deja la existencia negativa: aviso sin bloqueo. 8,500 ml por $425 y salen 9,000: $425
	// exactos + 500 ml al último costo válido 0.05 = −$450; quedan −500 ml y −$25.
	negative := fixture.adjust(t, fixture.wasteClerk, fixture.adjustmentBody("waste", "expiry", "-9000", nil), 200)
	if !slices.Contains(warningCodes(negative.Warnings), "NEGATIVE_STOCK") || fixture.stock(t, "aceite", general) != "-500" ||
		fixture.valuation(t, "aceite") != "-500/-25" {
		t.Fatalf("Merma con existencia negativa: %+v %s", negative.Warnings, fixture.valuation(t, "aceite"))
	}
	// Validaciones: «Otro» sin texto, sentido contrario al motivo, insumo repetido, sin renglones, tipo y
	// motivo desconocidos.
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "other", "5", nil), 400)
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "breakage", "5", nil), 400)
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "surplus", "-5", nil), 400)
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "entry_error", "0", nil), 400)
	repeated := fixture.adjustmentBody("discrepancy", "entry_error", "5", nil)
	repeated["lines"] = []map[string]any{{"item_id": fixture.items["aceite"].ID, "quantity": "5"}, {"item_id": fixture.items["aceite"].ID,
		"quantity": "-5"}}
	fixture.adjust(t, fixture.admin, repeated, 400)
	empty := fixture.adjustmentBody("discrepancy", "entry_error", "5", nil)
	empty["lines"] = []map[string]any{}
	fixture.adjust(t, fixture.admin, empty, 400)
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("loss", "breakage", "-5", nil), 400)
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "theft", "-5", nil), 400)
	described := fixture.adjustmentBody("discrepancy", "other", "-1", nil)
	described["reason_detail"] = "Muestra para cata"
	sample := fixture.adjust(t, fixture.recorder, described, 200)
	if sample.State != "pending_authorization" || sample.ReasonDetail != "Muestra para cata" {
		t.Fatalf("Ajuste con otro motivo: %+v", sample)
	}
	fixture.write(t, fixture.authorizer, "POST", "inventory/adjustments/"+sample.ID+"/reject", map[string]any{"revision": sample.Revision,
		"reason": "Se registrará como merma"}, 200)
	// Consultas: filtros, costos solo con inventory.costs.read y lectura solo con permisos de ajustes.
	wastes := decodeResponse[inventory.AdjustmentPage](t, fixture.adminCall(t, "GET", "inventory/adjustments?kind=waste&item_id="+
		fixture.items["aceite"].ID, nil, 200))
	if wastes.Total != 2 || !wastes.CostsVisible || text(wastes.Items[0].Value) != "-450.00" || text(wastes.Items[1].Value) != "-25.00" {
		t.Fatalf("Mermas listadas: %+v", wastes)
	}
	hidden := decodeResponse[inventory.AdjustmentPage](t, fixture.call(t, fixture.recorder, "GET", "inventory/adjustments?state=rejected",
		nil, 200))
	if hidden.Total != 2 || hidden.CostsVisible || hidden.Items[0].Value != nil {
		t.Fatalf("Ajustes rechazados sin costos: %+v", hidden)
	}
	fixture.call(t, fixture.transferClerk, "GET", "inventory/adjustments", nil, 403)
	fixture.call(t, fixture.recorder, "GET", "inventory/adjustments?kind=loss", nil, 400)
	// Libro: tipos y orígenes del ajuste y de la merma.
	ledger := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?source_id="+waste.ID, nil, 200))
	if ledger.Total != 1 || ledger.Items[0].Kind != "waste" || ledger.Items[0].SourceKindLabel != "Merma operativa" ||
		text(ledger.Items[0].Reason) != "Rotura" {
		t.Fatalf("Movimiento de la merma: %+v", ledger.Items)
	}
}

func testControlTransfers(t *testing.T, fixture *controlFixture) {
	general, barra := fixture.general.ID, fixture.barra.ID
	transfer := func(terminal testTerminal, from, to, quantity string, status int) inventory.TransferDocument {
		t.Helper()
		response := fixture.write(t, terminal, "POST", "inventory/transfers", map[string]any{
			"from_warehouse_id": from, "to_warehouse_id": to, "lines": []map[string]any{{"item_id": fixture.items["vino"].ID, "quantity": quantity}},
		}, status)
		if status != 200 {
			return inventory.TransferDocument{}
		}
		return decodeResponse[inventory.TransferDocument](t, response)
	}
	// General 50 → Barra 20: General 30 y Barra 20, salida y entrada con el mismo valor (20 × $120) y el
	// costo del insumo sin cambio (50 botellas, $6,000).
	operation := randomID()
	body := map[string]any{"operation_id": operation, "from_warehouse_id": general, "to_warehouse_id": barra, "notes": "Surtido de barra",
		"lines": []map[string]any{{"item_id": fixture.items["vino"].ID, "quantity": "20"}}}
	first := decodeResponse[inventory.TransferDocument](t, fixture.call(t, fixture.transferClerk, "POST", "inventory/transfers", body, 200))
	if fixture.stock(t, "vino", general) != "30" || fixture.stock(t, "vino", barra) != "20" || fixture.valuation(t, "vino") != "50/6000" ||
		len(first.Lines) != 1 || first.Lines[0].FromStockAfter != "30" || first.Lines[0].ToStockAfter != "20" || first.Value != nil ||
		len(first.Warnings) != 0 || first.Notes != "Surtido de barra" {
		t.Fatalf("Transferencia: %+v %s", first, fixture.valuation(t, "vino"))
	}
	var outValue, inValue string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT (SELECT value::text FROM app.inventory_movements WHERE id=$1),
        (SELECT value::text FROM app.inventory_movements WHERE id=$2)`, first.Lines[0].OutMovementID, first.Lines[0].InMovementID).
		Scan(&outValue, &inValue); err != nil {
		t.Fatal(err)
	}
	if decimal.RequireFromString(outValue).String() != "-2400" || decimal.RequireFromString(inValue).String() != "2400" {
		t.Fatalf("Valores de la transferencia: %s y %s", outValue, inValue)
	}
	// Idempotente: el mismo operation_id devuelve la misma transferencia sin movimientos nuevos.
	again := decodeResponse[inventory.TransferDocument](t, fixture.call(t, fixture.transferClerk, "POST", "inventory/transfers", body, 200))
	if again.ID != first.ID || fixture.movements(t, first.ID) != 2 || fixture.stock(t, "vino", barra) != "20" {
		t.Fatal("La transferencia repetida duplicó movimientos")
	}
	// Existencia negativa en el origen: aviso sin bloqueo (General −5, Barra 55); el costo sigue igual.
	negative := transfer(fixture.transferClerk, general, barra, "35", 200)
	if !slices.Contains(warningCodes(negative.Warnings), "NEGATIVE_STOCK") || fixture.stock(t, "vino", general) != "-5" ||
		fixture.stock(t, "vino", barra) != "55" || fixture.valuation(t, "vino") != "50/6000" {
		t.Fatalf("Transferencia con faltante: %+v", negative.Warnings)
	}
	transfer(fixture.transferClerk, barra, general, "5", 200)
	if fixture.stock(t, "vino", general) != "0" || fixture.stock(t, "vino", barra) != "50" {
		t.Fatal("La devolución a general no cuadra")
	}
	// Validaciones y permisos; ninguna crea compras ni ventas.
	transfer(fixture.transferClerk, general, general, "1", 400)
	transfer(fixture.transferClerk, general, barra, "0", 400)
	transfer(fixture.transferClerk, general, barra, "-1", 400)
	transfer(fixture.reader, general, barra, "1", 403)
	transfer(fixture.wasteClerk, general, barra, "1", 403)
	listed := decodeResponse[inventory.TransferPage](t, fixture.call(t, fixture.transferClerk, "GET", "inventory/transfers?warehouse_id="+barra,
		nil, 200))
	withCosts := decodeResponse[inventory.TransferDocument](t, fixture.adminCall(t, "GET", "inventory/transfers/"+first.ID, nil, 200))
	if listed.Total != 3 || listed.CostsVisible || text(withCosts.Value) != "2400.00" || text(withCosts.Lines[0].Value) != "2400.00" {
		t.Fatalf("Transferencias listadas: %+v / %+v", listed, withCosts)
	}
	fixture.call(t, fixture.recorder, "GET", "inventory/transfers", nil, 403)
	stock := decodeResponse[inventory.StockPage](t, fixture.adminCall(t, "GET", "inventory/stock?search=D-VIN", nil, 200))
	if len(stock.Items) != 1 || text(stock.Items[0].AverageCost) != "120.00" {
		t.Fatalf("El costo del vino cambió con las transferencias: %+v", stock.Items)
	}
}

// sell confirma una cuenta con limonadas (la confirmación libera a cocina y registra la solicitud de
// consumo) y devuelve la unidad de la primera.
func (fixture *controlFixture) sell(t *testing.T, name string, quantity int) string {
	t.Helper()
	bill := fixture.sales.order(t, name, "dine_in", "", catalog.LineRequest{ProductID: fixture.products["limonada"].ID, Quantity: quantity})
	return fixture.sales.accountUnits(t, bill.Account.ID)[0]
}

func (fixture *controlFixture) requestState(t *testing.T, unitID string) string {
	t.Helper()
	var state string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT state FROM app.inventory_consumption_requests WHERE restaurant_unit_id=$1 AND kind='release'",
		unitID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (fixture *controlFixture) count(t *testing.T, terminal testTerminal, id string) inventory.Count {
	t.Helper()
	return decodeResponse[inventory.Count](t, fixture.call(t, terminal, "GET", "inventory/counts/"+id, nil, 200))
}

func countLineOf(t *testing.T, count inventory.Count, itemID string) inventory.CountLine {
	t.Helper()
	for _, line := range count.Lines {
		if line.Item.ID == itemID {
			return line
		}
	}
	t.Fatalf("El conteo no tiene el insumo %s", itemID)
	return inventory.CountLine{}
}

func testControlCounts(t *testing.T, fixture *controlFixture) {
	barra := fixture.barra.ID
	limon, azucar, vino := fixture.items["limon"].ID, fixture.items["azucar"].ID, fixture.items["vino"].ID
	// S0: una limonada liberada antes del conteo cuya solicitud falla (disparador de prueba): alerta de
	// consumo sin calcular.
	failing := fixture.sell(t, "Conteo D falla", 1)
	if _, err := fixture.pool.Exec(fixture.ctx, `CREATE FUNCTION app.inventory_test_control_failure() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN RAISE EXCEPTION 'falla de prueba'; END; $$;
        CREATE TRIGGER inventory_test_control_failure BEFORE INSERT ON app.inventory_unit_consumptions FOR EACH ROW
        WHEN (NEW.restaurant_unit_id='`+failing+`') EXECUTE FUNCTION app.inventory_test_control_failure()`); err != nil {
		t.Fatal(err)
	}
	fixture.sales.process(t)
	if fixture.requestState(t, failing) != "failed" {
		t.Fatal("La solicitud de prueba debía fallar")
	}
	alerts := decodeResponse[inventory.AlertPage](t, fixture.call(t, fixture.reader, "GET", "inventory/alerts?type=consumption_failed", nil, 200))
	if alerts.Total != 1 || alerts.Items[0].Severity != "critical" || alerts.Items[0].Reference.Kind != "consumption_requests" ||
		alerts.Items[0].Count != 1 || !strings.Contains(alerts.Items[0].Message, "1 consumo de ventas no se pudo calcular") {
		t.Fatalf("Alerta de consumo fallido: %+v", alerts)
	}
	// S1: otra limonada liberada antes del conteo, sin procesar.
	pending := fixture.sell(t, "Conteo D pendiente", 1)
	// Conteo de la barra: limón, azúcar y vino.
	count := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": barra, "scope": "items", "item_ids": []string{limon, azucar, vino}, "notes": "Conteo de cierre",
	}, 200))
	if count.State != "open" || count.LineCount != 3 || count.TheoreticalVisible || !count.Editable || count.Folio == 0 {
		t.Fatalf("Conteo creado: %+v", count)
	}
	// Un insumo no puede estar en dos conteos sin terminar del mismo almacén.
	if errorCode(t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": barra, "scope": "items", "item_ids": []string{limon},
	}, 409)) != "COUNT_ITEM_IN_PROGRESS" {
		t.Fatal("Se aceptó un segundo conteo del mismo insumo")
	}
	fixture.write(t, fixture.approver, "POST", "inventory/counts", map[string]any{"warehouse_id": barra, "scope": "all"}, 403)
	// Captura a ciegas (quien solo cuenta no ve la existencia teórica): 93 limones (físicamente hay 94:
	// 100 − 3 − 3; falta uno), 5.2 kg de azúcar (sobran 200 g) y 8 cajas de 6 botellas (faltan 2).
	route := "inventory/counts/" + count.ID
	captured := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{
		"lines": []map[string]any{
			{"item_id": limon, "counted_quantity": "93"},
			{"item_id": azucar, "counted_quantity": "5.2", "unit_id": fixture.units["kg"].ID},
			{"item_id": vino, "counted_quantity": "8", "presentation_id": fixture.presentations["caja"]},
		},
	}, 200))
	line := countLineOf(t, captured, azucar)
	if captured.CountedLines != 3 || line.TheoreticalQuantity != nil || line.Difference != nil || text(line.CountedQuantity) != "5200" ||
		text(line.CapturedQuantity) != "5.2" || text(line.CaptureLabel) != "Kilogramo" || line.CapturedAt == nil ||
		text(countLineOf(t, captured, vino).CountedQuantity) != "48" {
		t.Fatalf("Captura a ciegas: %+v", captured)
	}
	fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{"lines": []map[string]any{
		{"item_id": fixture.items["aceite"].ID, "counted_quantity": "1"},
	}}, 400)
	fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{"lines": []map[string]any{
		{"item_id": limon, "counted_quantity": "-1"},
	}}, 400)
	// S2: una venta después de capturar el limón (sin procesar todavía).
	later := fixture.sell(t, "Conteo D posterior", 1)
	// Quien aprueba ve la existencia teórica a la hora de cada renglón: sin calcular los consumos
	// anteriores todavía es 100 limones, con el aviso de los 2 consumos pendientes.
	review := fixture.count(t, fixture.approver, count.ID)
	lemon := countLineOf(t, review, limon)
	if !review.TheoreticalVisible || text(lemon.TheoreticalQuantity) != "100" || text(lemon.Difference) != "-7" ||
		!slices.ContainsFunc(review.Warnings, func(warning inventory.Warning) bool {
			return warning.Code == "COUNT_PENDING_CONSUMPTION" && strings.HasPrefix(warning.Message, "2 consumos")
		}) {
		t.Fatalf("Revisión del conteo: %+v", review)
	}
	// Flujo: enviar (quien cuenta), aprobar y aplicar (solo quien aprueba).
	submitted := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", route+"/submit",
		map[string]any{"revision": captured.Revision}, 200))
	fixture.write(t, fixture.counter, "POST", route+"/approve", map[string]any{"revision": submitted.Revision}, 403)
	fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{"lines": []map[string]any{
		{"item_id": limon, "counted_quantity": "90"},
	}}, 409)
	approved := decodeResponse[inventory.Count](t, fixture.write(t, fixture.approver, "POST", route+"/approve",
		map[string]any{"revision": submitted.Revision}, 200))
	if approved.State != "approved" || approved.ApprovedByName == nil || !approved.Applicable {
		t.Fatalf("Conteo aprobado: %+v", approved)
	}
	fixture.write(t, fixture.counter, "POST", route+"/apply", map[string]any{"revision": approved.Revision}, 403)
	// Con un consumo anterior que no se puede calcular, el conteo no se aplica (y nada queda registrado).
	if errorCode(t, fixture.write(t, fixture.approver, "POST", route+"/apply", map[string]any{"revision": approved.Revision}, 409)) !=
		"COUNT_CONSUMPTION_PENDING" || fixture.requestState(t, pending) != "pending" || fixture.movements(t, count.ID) != 0 {
		t.Fatal("El conteo se aplicó con un consumo anterior sin calcular")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `DROP TRIGGER inventory_test_control_failure ON app.inventory_unit_consumptions;
        DROP FUNCTION app.inventory_test_control_failure()`); err != nil {
		t.Fatal(err)
	}
	// Aplicar: primero se calculan los consumos liberados antes de la hora del conteo (S0 y S1, no S2);
	// limón teórico 94 → faltante 1 (a $2: −$2); azúcar 5,000 → sobrante 200 g (al promedio 0.02:
	// +$4); vino 50 en la barra → faltante 2 (a $120: −$240). Cada ajuste con la hora de su renglón.
	operation := randomID()
	body := map[string]any{"operation_id": operation, "revision": approved.Revision}
	raw := fixture.call(t, fixture.approver, "POST", route+"/apply", body, 200)
	result := decodeResponse[inventory.Count](t, raw)
	if result.State != "applied" || result.ReferenceAt == nil || result.DifferenceLines == nil || *result.DifferenceLines != 3 ||
		fixture.requestState(t, failing) != "processed" || fixture.requestState(t, pending) != "processed" ||
		fixture.requestState(t, later) != "pending" {
		t.Fatalf("Conteo aplicado: %+v", result)
	}
	for _, expected := range []struct{ item, theoretical, difference, kind, movement string }{
		{limon, "94", "-1", "shortage", "count_out"}, {azucar, "5000", "200", "surplus", "count_in"}, {vino, "50", "-2", "shortage", "count_out"},
	} {
		line := countLineOf(t, result, expected.item)
		if text(line.TheoreticalQuantity) != expected.theoretical || text(line.Difference) != expected.difference ||
			text(line.DifferenceKind) != expected.kind || text(line.MovementKind) != expected.movement || line.Value != nil {
			t.Fatalf("Renglón aplicado de %s: %+v", expected.item, line)
		}
	}
	var mismatched int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM app.inventory_count_lines l JOIN app.inventory_movements m ON m.id=l.movement_id
        WHERE l.count_id=$1 AND (m.occurred_at<>l.captured_at OR m.entry_key<>'count:'||l.count_id||':'||l.item_id OR m.source_kind<>'count')`,
		count.ID).Scan(&mismatched); err != nil {
		t.Fatal(err)
	}
	if mismatched != 0 || fixture.valuation(t, "limon") != "93/186" || fixture.valuation(t, "azucar") != "5200/104" ||
		fixture.valuation(t, "vino") != "48/5760" {
		t.Fatalf("Movimientos del conteo: %d %s %s %s", mismatched, fixture.valuation(t, "limon"), fixture.valuation(t, "azucar"),
			fixture.valuation(t, "vino"))
	}
	costs := fixture.count(t, fixture.admin, count.ID)
	if text(costs.Value) != "-238.00" || text(countLineOf(t, costs, azucar).Value) != "4.00" {
		t.Fatalf("Valores del conteo con costos: %s %+v", text(costs.Value), costs.Lines)
	}
	// Aplicar dos veces no genera otro ajuste: el mismo operation_id devuelve la respuesta guardada y uno
	// nuevo responde COUNT_ALREADY_APPLIED.
	replayed := decodeResponse[inventory.Count](t, fixture.call(t, fixture.approver, "POST", route+"/apply", body, 200))
	if replayed.ID != result.ID || fixture.movements(t, count.ID) != 3 {
		t.Fatal("El reintento de la aplicación duplicó movimientos")
	}
	if errorCode(t, fixture.write(t, fixture.approver, "POST", route+"/apply", map[string]any{"revision": result.Revision}, 409)) !=
		"COUNT_ALREADY_APPLIED" || fixture.movements(t, count.ID) != 3 {
		t.Fatal("La segunda aplicación generó otro ajuste")
	}
	// El consumo posterior a la captura se conserva: 93 contados − 3 de S2 = 90.
	fixture.sales.process(t)
	if fixture.requestState(t, later) != "processed" || fixture.stock(t, "limon", barra) != "90" {
		t.Fatalf("Consumo posterior al conteo: %s %s", fixture.requestState(t, later), fixture.stock(t, "limon", barra))
	}
	// Quien solo cuenta sigue sin ver la existencia teórica de un conteo aplicado.
	// Tampoco ve el movimiento de cada renglón: su tipo (sobrante o faltante) revelaría el sentido de
	// la diferencia y su existencia, qué renglones tuvieron diferencia.
	blind := fixture.count(t, fixture.counter, count.ID)
	if countLineOf(t, blind, limon).TheoreticalQuantity != nil || blind.DifferenceLines != nil || blind.Value != nil {
		t.Fatalf("Conteo aplicado a ciegas: %+v", blind)
	}
	for _, line := range blind.Lines {
		if line.MovementID != nil || line.MovementKind != nil || line.MovementLabel != nil {
			t.Fatalf("Renglón aplicado a ciegas con su movimiento: %+v", line)
		}
	}
	fixture.call(t, fixture.recorder, "GET", "inventory/counts/"+count.ID, nil, 403)
}

func testControlCountWorkflow(t *testing.T, fixture *controlFixture) {
	barra := fixture.barra.ID
	limon, azucar, vino := fixture.items["limon"].ID, fixture.items["azucar"].ID, fixture.items["vino"].ID
	// Cancelar un conteo abierto (con motivo); ya no se captura ni se cancela otra vez.
	cancelled := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": barra, "scope": "items", "item_ids": []string{limon},
	}, 200))
	route := "inventory/counts/" + cancelled.ID
	fixture.write(t, fixture.counter, "POST", route+"/cancel", map[string]any{"revision": cancelled.Revision, "reason": "No"}, 400)
	cancelled = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", route+"/cancel",
		map[string]any{"revision": cancelled.Revision, "reason": "Se contará mañana"}, 200))
	if cancelled.State != "cancelled" || text(cancelled.CancelReason) != "Se contará mañana" || cancelled.Cancellable {
		t.Fatalf("Conteo cancelado: %+v", cancelled)
	}
	if errorCode(t, fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{"lines": []map[string]any{
		{"item_id": limon, "counted_quantity": "1"},
	}}, 409)) != "COUNT_NOT_OPEN" {
		t.Fatal("Se capturó un conteo cancelado")
	}
	if errorCode(t, fixture.write(t, fixture.counter, "POST", route+"/cancel", map[string]any{"revision": cancelled.Revision,
		"reason": "Otra vez"}, 409)) != "COUNT_NOT_CANCELLABLE" {
		t.Fatal("Se canceló dos veces")
	}
	// Por categoría (vino y limón de «Bebidas D»), se agrega el azúcar y se quita el limón.
	byCategory := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": barra, "scope": "category", "category_id": fixture.category,
	}, 200))
	if byCategory.LineCount != 2 || byCategory.Category == nil || byCategory.Category.Name != "Bebidas D" {
		t.Fatalf("Conteo por categoría: %+v", byCategory)
	}
	route = "inventory/counts/" + byCategory.ID
	byCategory = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PATCH", route, map[string]any{
		"revision": byCategory.Revision, "add_item_ids": []string{azucar},
	}, 200))
	byCategory = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PATCH", route, map[string]any{
		"revision": byCategory.Revision, "remove_item_ids": []string{limon}, "notes": "Sin limón",
	}, 200))
	if byCategory.LineCount != 2 || byCategory.Notes != "Sin limón" {
		t.Fatalf("Conteo editado: %+v", byCategory)
	}
	fixture.write(t, fixture.counter, "PATCH", route, map[string]any{"revision": byCategory.Revision, "add_item_ids": []string{vino}}, 400)
	fixture.write(t, fixture.counter, "PATCH", route, map[string]any{"revision": 1, "notes": "Revisión vieja"}, 409)
	// Nada contado: no se envía.
	if errorCode(t, fixture.write(t, fixture.counter, "POST", route+"/submit", map[string]any{"revision": byCategory.Revision}, 409)) !=
		"COUNT_NOTHING_COUNTED" {
		t.Fatal("Se envió un conteo sin capturas")
	}
	// Solo el vino (8 cajas = 48, sin diferencia); el azúcar queda sin contar y no se ajusta.
	byCategory = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{
		"lines": []map[string]any{{"item_id": vino, "counted_quantity": "8", "presentation_id": fixture.presentations["caja"]}},
	}, 200))
	byCategory = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", route+"/submit",
		map[string]any{"revision": byCategory.Revision}, 200))
	byCategory = decodeResponse[inventory.Count](t, fixture.write(t, fixture.approver, "POST", route+"/approve",
		map[string]any{"revision": byCategory.Revision}, 200))
	if !slices.Contains(warningCodes(fixture.count(t, fixture.approver, byCategory.ID).Warnings), "COUNT_UNCOUNTED_LINES") {
		t.Fatal("Falta el aviso de insumos sin contar")
	}
	movementsBefore := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements")
	applied := decodeResponse[inventory.Count](t, fixture.write(t, fixture.approver, "POST", route+"/apply",
		map[string]any{"revision": byCategory.Revision}, 200))
	wine := countLineOf(t, applied, vino)
	sugar := countLineOf(t, applied, azucar)
	if applied.State != "applied" || text(wine.Difference) != "0" || text(wine.DifferenceKind) != "none" || wine.MovementID != nil ||
		sugar.Counted || sugar.MovementID != nil || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements") !=
		movementsBefore {
		t.Fatalf("Conteo sin diferencias: %+v", applied)
	}
	// Conteo en cero del azúcar (faltan 5,200 g): se captura, se borra y se vuelve a capturar; queda
	// enviado para la alerta de diferencias pendientes. Quien cuenta no cancela uno enviado.
	zero := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": barra, "scope": "items", "item_ids": []string{azucar},
	}, 200))
	route = "inventory/counts/" + zero.ID
	for _, quantity := range []any{"0", nil, "0"} {
		zero = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{
			"lines": []map[string]any{{"item_id": azucar, "counted_quantity": quantity}},
		}, 200))
	}
	zero = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", route+"/submit", map[string]any{"revision": zero.Revision}, 200))
	review := countLineOf(t, fixture.count(t, fixture.approver, zero.ID), azucar)
	if text(review.CountedQuantity) != "0" || text(review.Difference) != "-5200" {
		t.Fatalf("Conteo en cero: %+v", review)
	}
	fixture.write(t, fixture.counter, "POST", route+"/cancel", map[string]any{"revision": zero.Revision, "reason": "No se terminó"}, 403)
	listed := decodeResponse[inventory.CountPage](t, fixture.call(t, fixture.counter, "GET", "inventory/counts?warehouse_id="+barra+
		"&state=active", nil, 200))
	if listed.Total != 1 || listed.Items[0].ID != zero.ID {
		t.Fatalf("Conteos activos: %+v", listed)
	}
	fixture.call(t, fixture.counter, "GET", "inventory/counts?state=paused", nil, 400)
}

func testControlStockLevels(t *testing.T, fixture *controlFixture) {
	barra := fixture.barra.ID
	limon := fixture.items["limon"].ID
	save := func(terminal testTerminal, revision int, minimum, reorder any, status int) inventory.StockLevel {
		t.Helper()
		response := fixture.write(t, terminal, "PUT", "inventory/stock-levels", map[string]any{
			"item_id": limon, "warehouse_id": barra, "revision": revision, "min_stock": minimum, "reorder_point": reorder,
		}, status)
		if status != 200 {
			return inventory.StockLevel{}
		}
		return decodeResponse[inventory.StockLevel](t, response)
	}
	save(fixture.buyer, 0, "105", "120", 403)
	level := save(fixture.admin, 0, "105", "120", 200)
	if !level.Configured || level.Revision != 1 || text(level.MinStock) != "105" || text(level.Quantity) != "90" || level.BelowMinimum == nil ||
		!*level.BelowMinimum {
		t.Fatalf("Nivel por almacén: %+v", level)
	}
	if errorCode(t, fixture.write(t, fixture.admin, "PUT", "inventory/stock-levels", map[string]any{
		"item_id": limon, "warehouse_id": barra, "revision": 0, "min_stock": "1",
	}, 409)) != "REVISION_CONFLICT" {
		t.Fatal("Se aceptó una revisión obsoleta del nivel")
	}
	level = save(fixture.admin, 1, "95", nil, 200)
	level = save(fixture.admin, level.Revision, nil, nil, 200)
	if level.Configured || level.Revision != 0 || level.MinStock != nil {
		t.Fatalf("Nivel quitado: %+v", level)
	}
	save(fixture.admin, 0, "-1", nil, 400)
	level = save(fixture.admin, 0, "105", "120", 200)
	listed := decodeResponse[inventory.StockLevelPage](t, fixture.call(t, fixture.reader, "GET", "inventory/stock-levels?warehouse_id="+barra,
		nil, 200))
	if listed.Total != 1 || listed.Items[0].Quantity != nil || listed.Items[0].StockVisible || text(listed.Items[0].ReorderPoint) != "120" {
		t.Fatalf("Niveles sin existencias: %+v", listed)
	}
	// La consulta de existencias filtrada por la barra usa su nivel (90 < 105); sin filtro, los del insumo.
	filtered := decodeResponse[inventory.StockPage](t, fixture.adminCall(t, "GET", "inventory/stock?warehouse_id="+barra+"&state=below_minimum",
		nil, 200))
	found := false
	for _, row := range filtered.Items {
		if row.ItemID == limon {
			found = row.LevelScope == "warehouse" && text(row.MinStock) == "105" && row.BelowMinimum && row.AtReorderPoint
		}
	}
	total := decodeResponse[inventory.StockPage](t, fixture.adminCall(t, "GET", "inventory/stock?search=D-LIM", nil, 200))
	if !found || len(total.Items) != 1 || total.Items[0].LevelScope != "item" || total.Items[0].MinStock != nil || total.Items[0].BelowMinimum {
		t.Fatalf("Existencias con niveles: %+v / %+v", filtered.Items, total.Items)
	}
	// Proveedor de la barra: surte limón (bolsa de 12, preferido) y vino (caja de 6, preferido). Orden de 2
	// bolsas de limón a $30 sin IVA, emitida y recibida a medias (1 bolsa = 12 piezas): parcialmente recibida.
	supplier := decodeResponse[inventory.SupplierDetail](t, fixture.adminWrite(t, "POST", "inventory/suppliers",
		map[string]any{"trade_name": "Proveedor de barra D"}, 200))
	fixture.supplier = supplier.ID
	for _, entry := range []struct{ item, presentation string }{{"limon", "bolsa"}, {"vino", "caja"}} {
		fixture.adminWrite(t, "PUT", "inventory/suppliers/"+supplier.ID+"/items", map[string]any{
			"revision": 0, "item_id": fixture.items[entry.item].ID, "presentation_id": fixture.presentations[entry.presentation],
			"preferred": true,
		}, 200)
	}
	draft := decodeResponse[inventory.PurchaseOrder](t, fixture.adminWrite(t, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": supplier.ID, "expected_warehouse_id": barra, "lines": []map[string]any{{
			"item_id": limon, "presentation_id": fixture.presentations["bolsa"], "quantity": "2", "unit_price": "30", "tax_rate": 0,
		}},
	}, 200))
	issued := decodeResponse[inventory.PurchaseOrder](t, fixture.adminWrite(t, "POST", "inventory/purchase-orders/"+draft.ID+"/issue",
		map[string]any{"revision": draft.Revision}, 200))
	fixture.adminWrite(t, "POST", "inventory/receipts", map[string]any{
		"purchase_order_id": issued.ID, "lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}},
	}, 200)
	fixture.order = decodeResponse[inventory.PurchaseOrder](t, fixture.adminCall(t, "GET", "inventory/purchase-orders/"+issued.ID, nil, 200))
	if fixture.order.State != "partially_received" || fixture.stock(t, "limon", barra) != "102" {
		t.Fatalf("Orden parcial: %s %s", fixture.order.State, fixture.stock(t, "limon", barra))
	}
}

func alertsOf(page inventory.AlertPage, kind string) []inventory.Alert {
	alerts := []inventory.Alert{}
	for _, alert := range page.Items {
		if alert.Type == kind {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// alertFor busca la alerta de un tipo cuyo destino tiene ese nombre (y almacén, si se indica).
func alertFor(page inventory.AlertPage, kind, label, warehouse string) *inventory.Alert {
	for _, alert := range alertsOf(page, kind) {
		if alert.Reference.Label != label {
			continue
		}
		if warehouse == "" && alert.Warehouse == nil || alert.Warehouse != nil && alert.Warehouse.Name == warehouse {
			copied := alert
			return &copied
		}
	}
	return nil
}

func testControlAlerts(t *testing.T, fixture *controlFixture) {
	// Costo pendiente: 2 piezas de hielo sin existencia ni costo salen en cero.
	fixture.write(t, fixture.wasteClerk, "POST", "inventory/adjustments", map[string]any{"kind": "waste", "reason": "spill",
		"warehouse_id": fixture.general.ID, "lines": []map[string]any{{"item_id": fixture.items["hielo"].ID, "quantity": "-2"}}}, 200)
	// Cancelación sin clasificar: una limonada liberada, consumida (3 limones: la barra queda en 99) y
	// cancelada.
	cancelled := fixture.sales.order(t, "Alerta D cancelación", "dine_in", "", catalog.LineRequest{ProductID: fixture.products["limonada"].ID,
		Quantity: 1})
	fixture.sales.process(t)
	fixture.sales.cancelOne(t, cancelled)
	fixture.sales.process(t)
	// Frecuencia diaria: el almacén general (con existencias, nunca contado) vence; la barra se contó hoy.
	settings := decodeResponse[inventory.Settings](t, fixture.adminCall(t, "GET", "inventory/settings", nil, 200))
	settingsBody := func(frequency string, warn bool, revision int) map[string]any {
		return map[string]any{"revision": revision, "purchase_tax_in_cost": settings.PurchaseTaxInCost,
			"operating_day_cutoff": settings.OperatingDayCutoff, "warn_products_without_recipe": warn, "count_frequency": frequency}
	}
	settings = decodeResponse[inventory.Settings](t, fixture.adminWrite(t, "PUT", "inventory/settings",
		settingsBody("daily", true, settings.Revision), 200))
	page := decodeResponse[inventory.AlertPage](t, fixture.adminCall(t, "GET", "inventory/alerts?page_size=100", nil, 200))
	all := []inventory.Alert{}
	for current := 1; ; current++ {
		next := decodeResponse[inventory.AlertPage](t, fixture.adminCall(t, "GET", "inventory/alerts?page_size=100&page="+strconv.Itoa(current), nil, 200))
		all = append(all, next.Items...)
		if len(all) >= next.Total || len(next.Items) == 0 {
			break
		}
	}
	page.Items = all
	vino, limon := fixture.items["vino"], fixture.items["limon"]
	checks := []struct {
		label string
		alert *inventory.Alert
		want  func(*inventory.Alert) bool
	}{
		{"existencia negativa del aceite", alertFor(page, "negative_stock", "Aceite D", "Almacén general"), func(alert *inventory.Alert) bool {
			return alert.Severity == "critical" && text(alert.Quantity) == "-500" && strings.Contains(alert.Message, "-500 ml")
		}},
		{"limón bajo el mínimo de la barra", alertFor(page, "below_minimum", limon.Name, "Barra D"), func(alert *inventory.Alert) bool {
			return text(alert.Quantity) == "99" && text(alert.Threshold) == "105" && alert.Reference.Kind == "stock"
		}},
		{"vino bajo el mínimo total", alertFor(page, "below_minimum", vino.Name, ""), func(alert *inventory.Alert) bool {
			return text(alert.Quantity) == "48" && text(alert.Threshold) == "60"
		}},
		{"reabastecer limón en la barra", alertFor(page, "reorder_point", limon.Name, "Barra D"), func(alert *inventory.Alert) bool {
			return text(alert.Threshold) == "120" && alert.Severity == "info"
		}},
		{"reabastecer vino en total", alertFor(page, "reorder_point", vino.Name, ""), func(alert *inventory.Alert) bool {
			return text(alert.Threshold) == "70"
		}},
		{"aceite sin proveedor", alertFor(page, "item_without_supplier", "Aceite D", ""), func(alert *inventory.Alert) bool {
			return alert.Reference.Kind == "item" && alert.Reference.ID != nil
		}},
		{"agua sin receta", alertFor(page, "product_without_recipe", "Agua fresca D", ""), func(alert *inventory.Alert) bool {
			return alert.Reference.Kind == "product" && strings.Contains(alert.Message, "no tiene receta")
		}},
		{"costo pendiente del hielo", alertFor(page, "cost_pending", "Hielo D", ""), func(alert *inventory.Alert) bool {
			return text(alert.Quantity) == "2" && alert.Severity == "warning"
		}},
		{"conteo pendiente del general", alertFor(page, "count_due", "Almacén general", "Almacén general"), func(alert *inventory.Alert) bool {
			return alert.OccurredAt == nil && strings.Contains(alert.Message, "frecuencia diaria")
		}},
		{"orden parcialmente recibida", alertFor(page, "purchase_order_partial", "Orden de compra "+strconv.FormatInt(fixture.order.Folio, 10), ""),
			func(alert *inventory.Alert) bool {
				return alert.Reference.Kind == "purchase_order" && *alert.Reference.ID == fixture.order.ID &&
					strings.Contains(alert.Message, "Proveedor de barra D")
			}},
		{"cancelaciones sin clasificar", alertFor(page, "cancellations_unclassified", "Cancelaciones pendientes", ""),
			func(alert *inventory.Alert) bool {
				return alert.Count >= 1 && alert.Reference.ID == nil && alert.OccurredAt != nil
			}},
	}
	for _, check := range checks {
		if check.alert == nil || !check.want(check.alert) {
			t.Fatalf("Alerta «%s»: %+v", check.label, check.alert)
		}
	}
	for _, absent := range []struct{ kind, label, warehouse string }{
		{"item_without_supplier", limon.Name, ""}, {"product_without_recipe", "Limonada D", ""}, {"count_due", "Barra D", "Barra D"},
		{"consumption_failed", "Solicitudes de consumo", ""},
	} {
		if alertFor(page, absent.kind, absent.label, absent.warehouse) != nil {
			t.Fatalf("Alerta que no corresponde: %+v", absent)
		}
	}
	// Diferencia de conteo pendiente: el conteo del azúcar en cero sigue enviado con 1 diferencia.
	differences := alertsOf(page, "count_difference_pending")
	if len(differences) != 1 || differences[0].Warehouse == nil || differences[0].Warehouse.Name != "Barra D" ||
		!strings.Contains(differences[0].Message, "pendiente de aprobar con 1 diferencia") {
		t.Fatalf("Alerta de diferencia de conteo: %+v", differences)
	}
	// El número de diferencias solo lo ve quien ve la existencia teórica del conteo (aprueba conteos o
	// consulta existencias); quien solo cuenta o solo consulta inventario sabe que está pendiente.
	for _, viewer := range []struct {
		terminal testTerminal
		visible  bool
	}{{fixture.approver, true}, {fixture.recorder, true}, {fixture.counter, false}, {fixture.reader, false}} {
		seen := alertsOf(decodeResponse[inventory.AlertPage](t, fixture.call(t, viewer.terminal, "GET",
			"inventory/alerts?type=count_difference_pending", nil, 200)), "count_difference_pending")
		if len(seen) != 1 || strings.Contains(seen[0].Message, "diferencia") == !viewer.visible ||
			(!viewer.visible && (seen[0].Count != 1 || !strings.HasSuffix(seen[0].Message, "está pendiente de aprobar."))) {
			t.Fatalf("Alerta de diferencia de conteo (visible %v): %+v", viewer.visible, seen)
		}
	}
	// Gasto vinculado corregido después (vínculos que dejó la fase C): una alerta por recepción.
	corrected := rowCount(t, fixture.restaurantFixture, `SELECT count(DISTINCT cl.receipt_id) FROM app.inventory_receipt_cash_links cl
        JOIN app.cash_operations fix ON fix.corrects_operation_id=cl.cash_operation_id WHERE cl.unlinked_at IS NULL`)
	if corrected == 0 || len(alertsOf(page, "cash_link_expense_corrected")) != corrected ||
		alertsOf(page, "cash_link_expense_corrected")[0].Reference.Kind != "receipt" {
		t.Fatalf("Alertas de gastos vinculados corregidos: %d %+v", corrected, alertsOf(page, "cash_link_expense_corrected"))
	}
	// Orden: primero las críticas; los conteos coinciden con los de todas las alertas.
	if page.Items[0].Severity != "critical" || page.Counts.Total != len(page.Items) || page.Counts.ByType["negative_stock"] < 1 ||
		page.Counts.Critical+page.Counts.Warning+page.Counts.Info != page.Counts.Total {
		t.Fatalf("Orden y conteos de alertas: %+v", page.Counts)
	}
	summary := decodeResponse[inventory.Summary](t, fixture.adminCall(t, "GET", "inventory/summary", nil, 200))
	if summary.Alerts.Total != page.Counts.Total || summary.Alerts.Critical != page.Counts.Critical ||
		summary.Alerts.ByType["count_due"] != page.Counts.ByType["count_due"] {
		t.Fatalf("Resumen con alertas: %+v / %+v", summary.Alerts, page.Counts)
	}
	// Sin costos ni compras ni existencias: no ve el costo pendiente ni las alertas de compras, y las
	// alertas no llevan cantidades.
	reader := decodeResponse[inventory.AlertPage](t, fixture.call(t, fixture.reader, "GET", "inventory/alerts?type=negative_stock", nil, 200))
	limited := decodeResponse[inventory.AlertPage](t, fixture.call(t, fixture.reader, "GET", "inventory/alerts", nil, 200))
	if reader.CostsVisible || reader.StockVisible || reader.PurchasingVisible || reader.Total == 0 || reader.Items[0].Quantity != nil ||
		strings.Contains(reader.Items[0].Message, " ml") || limited.Counts.ByType["cost_pending"] != 0 ||
		limited.Counts.ByType["purchase_order_partial"] != 0 || limited.Counts.ByType["cash_link_expense_corrected"] != 0 ||
		limited.Counts.ByType["negative_stock"] != page.Counts.ByType["negative_stock"] {
		t.Fatalf("Alertas sin permisos adicionales: %+v / %+v", reader, limited.Counts)
	}
	critical := decodeResponse[inventory.AlertPage](t, fixture.adminCall(t, "GET", "inventory/alerts?severity=critical", nil, 200))
	if critical.Total != page.Counts.Critical {
		t.Fatalf("Filtro de gravedad: %d de %d", critical.Total, page.Counts.Critical)
	}
	fixture.adminCall(t, "GET", "inventory/alerts?type=unknown", nil, 400)
	fixture.adminCall(t, "GET", "inventory/alerts?severity=low", nil, 400)
	// Sin advertencia de productos sin receta no hay alertas de ese tipo; sin frecuencia, ningún conteo vence.
	settings = decodeResponse[inventory.Settings](t, fixture.adminWrite(t, "PUT", "inventory/settings",
		settingsBody("none", false, settings.Revision), 200))
	quiet := decodeResponse[inventory.AlertPage](t, fixture.adminCall(t, "GET", "inventory/alerts", nil, 200))
	if quiet.Counts.ByType["product_without_recipe"] != 0 || quiet.Counts.ByType["count_due"] != 0 {
		t.Fatalf("Alertas con la configuración desactivada: %+v", quiet.Counts)
	}
	fixture.adminWrite(t, "PUT", "inventory/settings", settingsBody("none", true, settings.Revision), 200)
	// Se cancela el conteo enviado (quien aprueba).
	zero := decodeResponse[inventory.CountPage](t, fixture.adminCall(t, "GET", "inventory/counts?state=submitted&warehouse_id="+fixture.barra.ID,
		nil, 200)).Items[0]
	fixture.write(t, fixture.approver, "POST", "inventory/counts/"+zero.ID+"/cancel", map[string]any{"revision": zero.Revision,
		"reason": "Conteo incompleto"}, 200)
}

func suggestionOf(t *testing.T, page inventory.PurchaseSuggestionPage, itemID string) inventory.PurchaseSuggestion {
	t.Helper()
	for _, suggestion := range page.Items {
		if suggestion.Item.ID == itemID {
			return suggestion
		}
	}
	t.Fatalf("No hay sugerencia para %s: %+v", itemID, page.Items)
	return inventory.PurchaseSuggestion{}
}

func testControlSuggestions(t *testing.T, fixture *controlFixture) {
	limon, vino := fixture.items["limon"].ID, fixture.items["vino"].ID
	// Limón: sin punto propio, usa la suma de los de sus almacenes (120); existencia 99 (102 recibidos −
	// 3 de la venta cancelada de las alertas) + pendiente de la orden emitida 12 → faltan 9 → 1 bolsa de
	// 12 (último precio $30 por bolsa). Vino: punto 70,
	// existencia 48, nada pendiente → faltan 22 → 4 cajas de 6 (24), sin precio anterior.
	page := decodeResponse[inventory.PurchaseSuggestionPage](t, fixture.adminCall(t, "GET",
		"inventory/purchase-suggestions?supplier_id="+fixture.supplier, nil, 200))
	lemon := suggestionOf(t, page, limon)
	wine := suggestionOf(t, page, vino)
	if page.Total != 2 || lemon.ReorderSource != "warehouses" || text(lemon.ReorderPoint) != "120" || text(lemon.Stock) != "99" ||
		text(lemon.PendingQuantity) != "12" || text(lemon.NeedQuantity) != "9" || text(lemon.SuggestedQuantity) != "1" ||
		text(lemon.SuggestedBaseQuantity) != "12" || lemon.Presentation == nil || lemon.CaptureLabel != "Bolsa con 12" ||
		text(lemon.LastUnitPrice) != "30.00" || lemon.Supplier == nil || lemon.Supplier.ID != fixture.supplier {
		t.Fatalf("Sugerencia del limón: %+v", lemon)
	}
	if wine.ReorderSource != "item" || text(wine.NeedQuantity) != "22" || text(wine.SuggestedQuantity) != "4" ||
		text(wine.SuggestedBaseQuantity) != "24" ||
		wine.LastUnitPrice != nil || wine.BelowMinimum == nil || !*wine.BelowMinimum {
		t.Fatalf("Sugerencia del vino: %+v", wine)
	}
	// Quien solo consulta inventario ve qué insumos reabastecer, sin proveedor, precios ni existencias,
	// y tampoco el punto de reabastecimiento ni lo sugerido (con ambos se deduciría la existencia).
	// Antes recibía la cantidad sugerida («1»).
	blind := decodeResponse[inventory.PurchaseSuggestionPage](t, fixture.call(t, fixture.reader, "GET", "inventory/purchase-suggestions",
		nil, 200))
	lemonBlind := suggestionOf(t, blind, limon)
	if blind.PurchasingVisible || blind.StockVisible || lemonBlind.Supplier != nil || lemonBlind.LastUnitPrice != nil ||
		lemonBlind.Stock != nil || lemonBlind.ReorderPoint != nil || lemonBlind.SuggestedQuantity != nil ||
		lemonBlind.SuggestedBaseQuantity != nil || lemonBlind.NeedQuantity != nil || lemonBlind.CaptureLabel != "Bolsa con 12" {
		t.Fatalf("Sugerencias sin permisos de compras: %+v", lemonBlind)
	}
	// Con inventory.purchases.create (quien arma el borrador) sí las ve.
	buyerLemon := suggestionOf(t, decodeResponse[inventory.PurchaseSuggestionPage](t, fixture.call(t, fixture.buyer, "GET",
		"inventory/purchase-suggestions", nil, 200)), limon)
	if !buyerLemon.StockVisible || text(buyerLemon.ReorderPoint) != "120" || text(buyerLemon.SuggestedQuantity) != "1" {
		t.Fatalf("Sugerencias de quien compra: %+v", buyerLemon)
	}
	fixture.call(t, fixture.reader, "GET", "inventory/purchase-suggestions?supplier_id="+fixture.supplier, nil, 403)
	// El borrador es una acción explícita con inventory.purchases.create; nunca se emite.
	ordersBefore := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_purchase_orders")
	cashBefore := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.cash_operations")
	fixture.write(t, fixture.reader, "POST", "inventory/purchase-suggestions/draft-order", map[string]any{
		"supplier_id": fixture.supplier, "item_ids": []string{limon},
	}, 403)
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-suggestions/draft-order", map[string]any{
		"supplier_id": fixture.supplier, "item_ids": []string{limon, fixture.items["aceite"].ID},
	}, 409)) != "SUGGESTION_CHANGED" || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_purchase_orders") != ordersBefore {
		t.Fatal("Se creó un borrador con un insumo sin sugerencia")
	}
	operation := randomID()
	body := map[string]any{"operation_id": operation, "supplier_id": fixture.supplier, "item_ids": []string{limon, vino},
		"expected_warehouse_id": fixture.barra.ID, "notes": "Desde sugerencias"}
	draft := decodeResponse[inventory.PurchaseOrder](t, fixture.call(t, fixture.buyer, "POST", "inventory/purchase-suggestions/draft-order",
		body, 200))
	if draft.State != "draft" || draft.IssuedAt != nil || len(draft.Lines) != 2 || draft.Lines[0].Item.ID != limon ||
		draft.Lines[0].Quantity != "1" || draft.Lines[0].UnitPrice != "30.00" || draft.Lines[0].TaxRate != 0 ||
		draft.Lines[0].Presentation == nil || draft.Lines[1].Quantity != "4" || draft.Lines[1].UnitPrice != "0.00" ||
		draft.Lines[1].TaxRate != 16 || draft.ExpectedWarehouse.ID != fixture.barra.ID || draft.Total != "30.00" {
		t.Fatalf("Borrador desde sugerencias: %+v", draft)
	}
	again := decodeResponse[inventory.PurchaseOrder](t, fixture.call(t, fixture.buyer, "POST", "inventory/purchase-suggestions/draft-order",
		body, 200))
	if again.ID != draft.ID || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_purchase_orders") != ordersBefore+1 ||
		rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.cash_operations") != cashBefore ||
		fixture.stock(t, "limon", fixture.barra.ID) != "99" {
		t.Fatal("El borrador repetido duplicó la orden o tocó existencias o caja")
	}
	// El borrador no cuenta como pendiente: la sugerencia sigue e informa lo que ya está en borradores.
	after := suggestionOf(t, decodeResponse[inventory.PurchaseSuggestionPage](t, fixture.adminCall(t, "GET", "inventory/purchase-suggestions",
		nil, 200)), limon)
	if text(after.SuggestedQuantity) != "1" || text(after.DraftQuantity) != "12" {
		t.Fatalf("Sugerencia después del borrador: %+v", after)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='inventory.purchase_order_drafted_from_suggestions'") != 1 {
		t.Fatal("Falta la auditoría del borrador desde sugerencias")
	}
}

// errorMessage devuelve el mensaje de un rechazo de la API.
func errorMessage(t *testing.T, body []byte) string {
	t.Helper()
	return decodeResponse[struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}](t, body).Error.Message
}

// testControlEntriesWithoutCost: una entrada sin documento (ajuste positivo o sobrante de conteo)
// que dejaría existencia positiva sin ningún costo disponible se rechaza con COST_REQUIRED, sin
// registrar nada (§0-B.6: nunca a un costo inventado); cubrir solo el déficit sigue permitido. El
// hielo salió 2 piezas sin existencia ni costo en las alertas: −2 piezas, $0, 2 pendientes de costo.
func testControlEntriesWithoutCost(t *testing.T, fixture *controlFixture) {
	general, hielo := fixture.general.ID, fixture.items["hielo"].ID
	required := "El insumo «Hielo D» no tiene costo: configura su costo de referencia antes de registrar una entrada sin documento"
	expectRejected := func(label string, body []byte) {
		t.Helper()
		if errorCode(t, body) != "COST_REQUIRED" || !strings.HasPrefix(errorMessage(t, body), required) {
			t.Fatalf("%s: %s", label, body)
		}
	}
	if fixture.valuation(t, "hielo") != "-2/0" {
		t.Fatalf("Hielo antes de las entradas: %s", fixture.valuation(t, "hielo"))
	}
	iceLine := func(quantity string) map[string]any {
		return map[string]any{"kind": "discrepancy", "reason": "surplus", "warehouse_id": general,
			"lines": []map[string]any{{"item_id": hielo, "quantity": quantity}}}
	}
	// Conteo del hielo en el almacén general: se cuentan 4 piezas con teórico −2 → sobrante 6, que
	// cubriría el déficit de 2 y dejaría 4 piezas sin costo. Aplicar falla sin registrar nada: el
	// conteo sigue aprobado, sin movimientos ni consumos procesados.
	count := decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", "inventory/counts", map[string]any{
		"warehouse_id": general, "scope": "items", "item_ids": []string{hielo},
	}, 200))
	route := "inventory/counts/" + count.ID
	count = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "PUT", route+"/lines", map[string]any{
		"lines": []map[string]any{{"item_id": hielo, "counted_quantity": "4"}},
	}, 200))
	count = decodeResponse[inventory.Count](t, fixture.write(t, fixture.counter, "POST", route+"/submit", map[string]any{"revision": count.Revision}, 200))
	approved := decodeResponse[inventory.Count](t, fixture.write(t, fixture.approver, "POST", route+"/approve",
		map[string]any{"revision": count.Revision}, 200))
	movementsBefore := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements")
	expectRejected("Conteo con sobrante sin costo", fixture.write(t, fixture.approver, "POST", route+"/apply",
		map[string]any{"revision": approved.Revision}, 409))
	if current := fixture.count(t, fixture.approver, count.ID); current.State != "approved" || current.Revision != approved.Revision ||
		fixture.movements(t, count.ID) != 0 || rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements") !=
		movementsBefore || fixture.valuation(t, "hielo") != "-2/0" {
		t.Fatalf("El conteo rechazado registró algo: %+v %s", current, fixture.valuation(t, "hielo"))
	}
	// Ajuste pendiente de +5 (quien no autoriza): registrarlo no aplica nada; autorizarlo falla igual y
	// sigue pendiente.
	pending := fixture.adjust(t, fixture.recorder, iceLine("5"), 200)
	if pending.State != "pending_authorization" {
		t.Fatalf("Ajuste pendiente del hielo: %+v", pending)
	}
	expectRejected("Autorización sin costo", fixture.write(t, fixture.authorizer, "POST", "inventory/adjustments/"+pending.ID+"/authorize",
		map[string]any{"revision": pending.Revision}, 409))
	stillPending := decodeResponse[inventory.Adjustment](t, fixture.call(t, fixture.authorizer, "GET", "inventory/adjustments/"+pending.ID, nil, 200))
	if stillPending.State != "pending_authorization" || stillPending.Revision != pending.Revision || fixture.movements(t, pending.ID) != 0 {
		t.Fatalf("El ajuste rechazado cambió: %+v", stillPending)
	}
	// Ajuste aplicado al registrarlo (+3): se rechaza sin guardar el ajuste.
	adjustments := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_adjustments")
	expectRejected("Ajuste directo sin costo", fixture.write(t, fixture.admin, "POST", "inventory/adjustments", iceLine("3"), 409))
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_adjustments") != adjustments ||
		fixture.valuation(t, "hielo") != "-2/0" {
		t.Fatal("El ajuste rechazado se guardó")
	}
	// +2 solo cubre el déficit: entra a su valor provisional ($0) y deja 0 piezas, $0, sin pendiente.
	covered := fixture.adjust(t, fixture.admin, iceLine("2"), 200)
	if covered.State != "applied" || fixture.valuation(t, "hielo") != "0/0" ||
		rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_valuations WHERE item_id=$1 AND pending_cost_quantity>0",
			hielo) != 0 {
		t.Fatalf("Ajuste que cubre el déficit: %+v %s", covered, fixture.valuation(t, "hielo"))
	}
	// Con el costo de referencia configurado ($1.50) el ajuste pendiente se autoriza (5 × 1.50 = $7.50)
	// y el conteo se aplica: su teórico a la hora del renglón sigue en −2 (las entradas posteriores se
	// conservan), sobrante 6 al promedio 1.50 = $9 → 11 piezas por $16.50.
	ice := decodeResponse[inventory.Item](t, fixture.adminCall(t, "GET", "inventory/items/"+hielo, nil, 200))
	fixture.adminWrite(t, "PATCH", "inventory/items/"+hielo, map[string]any{"revision": ice.Revision, "code": ice.Code, "name": ice.Name,
		"base_unit_id": ice.BaseUnit.ID, "kind": ice.Kind, "active": true, "reference_cost": "1.5"}, 200)
	authorized := decodeResponse[inventory.Adjustment](t, fixture.write(t, fixture.authorizer, "POST",
		"inventory/adjustments/"+pending.ID+"/authorize", map[string]any{"revision": pending.Revision}, 200))
	if authorized.State != "applied" || fixture.valuation(t, "hielo") != "5/7.5" {
		t.Fatalf("Ajuste autorizado con costo de referencia: %+v %s", authorized, fixture.valuation(t, "hielo"))
	}
	applied := decodeResponse[inventory.Count](t, fixture.write(t, fixture.approver, "POST", route+"/apply",
		map[string]any{"revision": approved.Revision}, 200))
	if applied.State != "applied" || text(countLineOf(t, applied, hielo).Difference) != "6" || fixture.valuation(t, "hielo") != "11/16.5" ||
		fixture.stock(t, "hielo", general) != "11" {
		t.Fatalf("Conteo aplicado con costo de referencia: %+v %s", applied, fixture.valuation(t, "hielo"))
	}
}

func testControlEventsAndAudit(t *testing.T, fixture *controlFixture, cursor int64) {
	// El faltante del aceite se cubre: 500 ml al provisional 500 × (−25 ÷ −500) = $25 → 0 ml, $0.
	fixture.adjust(t, fixture.admin, fixture.adjustmentBody("discrepancy", "entry_error", "500", nil), 200)
	if fixture.valuation(t, "aceite") != "0/0" {
		t.Fatalf("Faltante cubierto: %s", fixture.valuation(t, "aceite"))
	}
	kinds := eventKinds(fixture.inventoryEvents(t, fixture.reader, cursor))
	for _, kind := range []string{"inventory.adjustments.changed", "inventory.transfers.changed", "inventory.counts.changed",
		"inventory.alerts.changed", "inventory.stock.changed", "inventory.movements.recorded", "inventory.items.changed"} {
		if !slices.Contains(kinds, kind) {
			t.Fatalf("Falta el evento %s: %v", kind, kinds)
		}
	}
	for action, minimum := range map[string]int{
		"inventory.adjustment_recorded": 4, "inventory.adjustment_requested": 3, "inventory.adjustment_authorized": 1,
		"inventory.adjustment_rejected": 2, "inventory.waste_recorded": 3, "inventory.transfer_recorded": 3, "inventory.count_created": 4,
		"inventory.count_updated": 2, "inventory.count_lines_captured": 5, "inventory.count_submitted": 3, "inventory.count_approved": 2,
		"inventory.count_applied": 2, "inventory.count_cancelled": 2, "inventory.stock_level_saved": 4,
	} {
		if got := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action=$1", action); got < minimum {
			t.Fatalf("Auditoría %s: %d, se esperaban al menos %d", action, got, minimum)
		}
	}
	checkLedgerConsistency(t, fixture.inventoryFixture)
	// Documentos de la fase D de solo inserción o solo con transiciones válidas también en PostgreSQL.
	for _, statement := range []string{
		"DELETE FROM app.inventory_adjustments", "UPDATE app.inventory_transfers SET notes='x'", "DELETE FROM app.inventory_counts",
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s", statement)
		}
	}
}
