//go:build integration

package operations_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/inventory"
)

// Existencia inicial, libro de movimientos, visibilidad de costos, configuración, función activable
// y eventos de la fase A de la etapa 8.

func (fixture *inventoryFixture) warehouseByName(t *testing.T, name string) inventory.Warehouse {
	t.Helper()
	for _, warehouse := range decodeResponse[[]inventory.Warehouse](t, fixture.adminCall(t, "GET", "inventory/warehouses", nil, 200)) {
		if warehouse.Name == name {
			return warehouse
		}
	}
	t.Fatalf("No existe el almacén %s", name)
	return inventory.Warehouse{}
}

func testInventoryInitialBalances(t *testing.T, fixture *inventoryFixture) {
	rice := fixture.itemByCode(t, fixture.admin, "ARZ-01")
	soda := fixture.itemByCode(t, fixture.admin, "REF-355")
	oil := fixture.itemByCode(t, fixture.admin, "ACE-01")
	onion := fixture.itemByCode(t, fixture.admin, "CEB-01")
	salt := fixture.itemByCode(t, fixture.admin, "SAL-01")
	var box string
	for _, presentation := range soda.Presentations {
		if presentation.Name == "Caja con 24" {
			box = presentation.ID
		}
	}
	cursor := eventCursor(t, fixture.restaurantFixture)
	operationID := randomID()
	body := map[string]any{
		"operation_id": operationID,
		"warehouse_id": fixture.general.ID,
		"reason":       "Inventario de arranque",
		"lines": []map[string]any{
			// 10 kg a $20/kg = $200.
			{"item_id": rice.ID, "quantity": "10", "unit_cost": "20"},
			// 2 cajas de 24 a $240 la caja = 48 piezas por $480 ($10 por pieza).
			{"item_id": soda.ID, "quantity": "2", "presentation_id": box, "unit_cost": "240"},
			// 2,500 ml = 2.5 l al costo de referencia confirmado ($45/l) = $112.50.
			{"item_id": oil.ID, "quantity": "2500", "unit_id": fixture.units["ml"].ID, "use_reference_cost": true},
			// 1.5 kg = 1,500 g a $12/kg = $18 ($0.012 por gramo).
			{"item_id": onion.ID, "quantity": "1.5", "unit_id": fixture.units["kg"].ID, "unit_cost": "12"},
		},
	}
	first := fixture.adminCall(t, "POST", "inventory/initial-balances", body, 200)
	result := decodeResponse[inventory.InitialBalanceResult](t, first)
	if result.OperationID != operationID || result.Warehouse.Name != "Almacén general" || len(result.Lines) != 4 {
		t.Fatalf("Existencia inicial: %+v", result)
	}
	expected := []struct{ quantity, captured, label, value, cost string }{
		{"10", "10", "Kilogramo", "200.00", "20.00"},
		{"48", "2", "Caja con 24", "480.00", "10.00"},
		{"2.5", "2500", "Mililitro", "112.50", "45.00"},
		{"1500", "1.5", "Kilogramo", "18.00", "0.012"},
	}
	for index, line := range result.Lines {
		want := expected[index]
		if line.Quantity != want.quantity || line.CapturedQuantity != want.captured || line.CapturedUnitLabel != want.label ||
			line.Value == nil || *line.Value != want.value || line.UnitCost == nil || *line.UnitCost != want.cost ||
			line.StockAfter != want.quantity {
			t.Fatalf("Renglón %d: %+v", index+1, line)
		}
	}
	if !result.Lines[2].UseReferenceCost {
		t.Fatal("El renglón con costo de referencia debe indicarlo")
	}
	movements := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements")
	if movements != 4 {
		t.Fatalf("Movimientos de la existencia inicial: %d", movements)
	}
	// Reintento idempotente: mismo operation_id y cuerpo → misma respuesta, sin duplicar.
	// La respuesta guardada sale de jsonb (otro orden de claves): se compara su contenido.
	again := fixture.adminCall(t, "POST", "inventory/initial-balances", body, 200)
	if !reflect.DeepEqual(decodeResponse[map[string]any](t, again), decodeResponse[map[string]any](t, first)) ||
		rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements") != 4 {
		t.Fatal("El reintento con el mismo operation_id no devolvió la respuesta guardada")
	}
	changed := map[string]any{"operation_id": operationID, "warehouse_id": fixture.general.ID, "lines": []map[string]any{
		{"item_id": rice.ID, "quantity": "11", "unit_cost": "20"},
	}}
	if code := errorCode(t, fixture.adminCall(t, "POST", "inventory/initial-balances", changed, 409)); code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("Mismo operation_id con otro cuerpo: %s", code)
	}
	// Reglas de costo y de existencia previa.
	rejections := []struct {
		line   map[string]any
		status int
		code   string
	}{
		{map[string]any{"item_id": rice.ID, "quantity": "1", "unit_cost": "20"}, 409, "INITIAL_BALANCE_EXISTS"},
		{map[string]any{"item_id": salt.ID, "quantity": "100", "use_reference_cost": true}, 409, "REFERENCE_COST_MISSING"},
		{map[string]any{"item_id": salt.ID, "quantity": "100"}, 400, "INVENTORY_VALIDATION"},
		{map[string]any{"item_id": salt.ID, "quantity": "100", "unit_cost": "1", "use_reference_cost": true}, 400, "INVENTORY_VALIDATION"},
		{map[string]any{"item_id": salt.ID, "quantity": "-1", "unit_cost": "1"}, 400, "INVENTORY_VALIDATION"},
		{map[string]any{"item_id": salt.ID, "quantity": "1", "unit_id": fixture.units["l"].ID, "unit_cost": "1"}, 400, "INVENTORY_VALIDATION"},
	}
	for _, rejection := range rejections {
		response := fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
			"warehouse_id": fixture.general.ID, "lines": []map[string]any{rejection.line},
		}, rejection.status)
		if code := errorCode(t, response); code != rejection.code {
			t.Fatalf("%v: esperado %s, obtenido %s", rejection.line, rejection.code, code)
		}
	}
	// Permisos: autorizar ajustes y administrar costos, ambos.
	for _, terminal := range []testTerminal{fixture.stock, fixture.costs, fixture.clerk} {
		fixture.write(t, terminal, "POST", "inventory/initial-balances", map[string]any{
			"warehouse_id": fixture.general.ID, "lines": []map[string]any{{"item_id": salt.ID, "quantity": "1", "unit_cost": "1"}},
		}, 403)
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_movements") != 4 {
		t.Fatal("Una existencia inicial rechazada dejó movimientos")
	}
	// El mismo insumo puede recibir existencia inicial en otro almacén: 1 caja a $300 en Barra.
	bar := fixture.warehouseByName(t, "Barra")
	fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
		"warehouse_id": bar.ID, "lines": []map[string]any{{"item_id": soda.ID, "quantity": "1", "presentation_id": box, "unit_cost": "300"}},
	}, 200)
	// Auditoría y eventos en la misma transacción, sin costos en el evento.
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='inventory.initial_balance_recorded'") != 2 {
		t.Fatal("Cada existencia inicial debe auditarse una vez")
	}
	kinds := eventKinds(fixture.inventoryEvents(t, fixture.stock, cursor))
	if !slices.Contains(kinds, "inventory.stock.changed") || !slices.Contains(kinds, "inventory.movements.recorded") {
		t.Fatalf("Eventos de la existencia inicial: %v", kinds)
	}
}

// testInventoryLedger aplica entradas del libro como lo hará un proceso de Go (fase B en adelante):
// en su propia transacción, con el lock de inventario exclusivo.
func testInventoryLedger(t *testing.T, fixture *inventoryFixture) {
	rice := fixture.itemByCode(t, fixture.admin, "ARZ-01")
	bar := fixture.warehouseByName(t, "Barra")
	process := "inventory_worker"
	occurred := time.Now()
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, "SELECT pg_advisory_xact_lock(7820148308)"); err != nil {
		t.Fatal(err)
	}
	entry := func(key, kind string, quantity string, value *decimal.Decimal) inventory.MovementInput {
		return inventory.MovementInput{
			EntryKey: key, Kind: kind, ItemID: rice.ID, WarehouseID: fixture.general.ID, Quantity: decimal.RequireFromString(quantity),
			Value: value, OccurredAt: occurred, SourceKind: "acceptance", Process: &process,
		}
	}
	// Consumo de 12 kg con 10 kg por $200 (L = 20): cruza a negativo, 200 + 2 × 20 = 240; −2 kg.
	consumption, err := inventory.Apply(fixture.ctx, tx, entry("acceptance:consumption:1", inventory.KindConsumption, "12", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !consumption.Movement.Value.Equal(decimal.RequireFromString("-240")) || !consumption.NegativeStock ||
		!consumption.StockAfter.Equal(decimal.RequireFromString("-2")) || consumption.Movement.CostBasis != "reference" {
		t.Fatalf("Consumo que cruza a negativo: %+v", consumption.Movement)
	}
	// La misma clave no vuelve a aplicarse.
	replayed, err := inventory.Apply(fixture.ctx, tx, entry("acceptance:consumption:1", inventory.KindConsumption, "12", nil))
	if err != nil || !replayed.Replayed || replayed.Movement.ID != consumption.Movement.ID {
		t.Fatalf("La entrada repetida se aplicó otra vez: %+v %v", replayed, err)
	}
	// Recepción de 5 kg por $125 con −2 kg por −40: provisional 40, real 125 × 2 / 5 = 50, ajuste −10.
	// V = −40 + 125 − 10 = 75 con 3 kg (L = 25).
	value := decimal.RequireFromString("125")
	receipt, err := inventory.Apply(fixture.ctx, tx, entry("acceptance:receipt:1", inventory.KindReceipt, "5", &value))
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Adjustments) != 1 || !receipt.Adjustments[0].Value.Equal(decimal.RequireFromString("-10")) ||
		receipt.Adjustments[0].WarehouseID != nil || receipt.Adjustments[0].RelatedMovementID == nil ||
		*receipt.Adjustments[0].RelatedMovementID != receipt.Movement.ID {
		t.Fatalf("Ajuste de costo de la recepción: %+v", receipt.Adjustments)
	}
	if !receipt.Valuation.Value.Equal(decimal.RequireFromString("75")) || !receipt.Valuation.Quantity.Equal(decimal.RequireFromString("3")) {
		t.Fatalf("Valuación tras la recepción: %+v", receipt.Valuation)
	}
	// Transferencia de 1 kg a Barra: mismo valor de salida y entrada (25), sin cambiar Q ni V.
	transfer, err := inventory.ApplyTransfer(fixture.ctx, tx, inventory.TransferInput{
		EntryKey: "acceptance:transfer:1", ItemID: rice.ID, FromWarehouseID: fixture.general.ID, ToWarehouseID: bar.ID,
		Quantity: decimal.RequireFromString("1"), OccurredAt: occurred, SourceKind: "acceptance", Process: &process,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !transfer.Out.Value.Equal(decimal.RequireFromString("-25")) || !transfer.In.Value.Equal(decimal.RequireFromString("25")) ||
		*transfer.In.RelatedMovementID != transfer.Out.ID || !transfer.FromStock.Equal(decimal.RequireFromString("2")) {
		t.Fatalf("Transferencia: %+v %+v", transfer.Out, transfer.In)
	}
	repeated, err := inventory.ApplyTransfer(fixture.ctx, tx, inventory.TransferInput{
		EntryKey: "acceptance:transfer:1", ItemID: rice.ID, FromWarehouseID: fixture.general.ID, ToWarehouseID: bar.ID,
		Quantity: decimal.RequireFromString("1"), OccurredAt: occurred, SourceKind: "acceptance", Process: &process,
	})
	if err != nil || !repeated.Replayed {
		t.Fatalf("La transferencia repetida se aplicó otra vez: %v", err)
	}
	// Una entrada sin responsable o con un tipo que solo produce el motor se rechaza.
	invalid := entry("acceptance:invalid:1", inventory.KindCostAdjustment, "1", nil)
	if _, err := inventory.Apply(fixture.ctx, tx, invalid); err == nil {
		t.Fatal("Se aceptó un ajuste de costo directo")
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	checkLedgerConsistency(t, fixture)
	// El libro es de solo inserción también en PostgreSQL.
	for _, statement := range []string{
		"UPDATE app.inventory_movements SET value=0",
		"DELETE FROM app.inventory_movements",
		"TRUNCATE app.inventory_movements",
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s", statement)
		}
	}
	// El invariante cantidad 0 ⇒ valor 0 también lo hace cumplir la base.
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_valuations SET quantity=0,value=1 WHERE item_id=$1", rice.ID); err == nil {
		t.Fatal("PostgreSQL aceptó una valuación con cantidad cero y valor")
	}
	testInventoryMovementQueries(t, fixture, rice, bar)
}

// checkLedgerConsistency compara existencias, valuaciones y libro para todos los insumos:
// Σ existencias = cantidad de la valuación, Σ valores de movimientos = valor de la valuación,
// Σ cantidades por almacén = existencia del almacén y el último estado registrado = la valuación.
func checkLedgerConsistency(t *testing.T, fixture *inventoryFixture) {
	t.Helper()
	checks := map[string]string{
		"existencias y valuación": `SELECT count(*) FROM app.inventory_valuations v
            WHERE v.quantity <> (SELECT coalesce(sum(s.quantity),0) FROM app.inventory_stock s WHERE s.item_id=v.item_id)`,
		"valores del libro": `SELECT count(*) FROM app.inventory_valuations v
            WHERE v.value <> (SELECT coalesce(sum(m.value),0) FROM app.inventory_movements m WHERE m.item_id=v.item_id)`,
		"cantidades por almacén": `SELECT count(*) FROM app.inventory_stock s
            WHERE s.quantity <> (SELECT coalesce(sum(m.quantity),0) FROM app.inventory_movements m
                WHERE m.item_id=s.item_id AND m.warehouse_id=s.warehouse_id)`,
		"último estado registrado": `SELECT count(*) FROM app.inventory_valuations v
            JOIN LATERAL (SELECT m.valuation_quantity_after,m.valuation_value_after FROM app.inventory_movements m
                WHERE m.item_id=v.item_id ORDER BY m.seq DESC LIMIT 1) last ON true
            WHERE last.valuation_quantity_after<>v.quantity OR last.valuation_value_after<>v.value`,
	}
	for label, query := range checks {
		if count := rowCount(t, fixture.restaurantFixture, query); count != 0 {
			t.Fatalf("El libro no coincide (%s): %d insumos", label, count)
		}
	}
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.inventory_valuations") !=
		rowCount(t, fixture.restaurantFixture, "SELECT count(DISTINCT item_id) FROM app.inventory_movements") {
		t.Fatal("Cada insumo con movimientos debe tener su valuación")
	}
}

func testInventoryMovementQueries(t *testing.T, fixture *inventoryFixture, rice inventory.Item, bar inventory.Warehouse) {
	all := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?item_id="+rice.ID, nil, 200))
	// Existencia inicial, consumo, recepción, ajuste de costo, salida y entrada de la transferencia.
	if all.Total != 6 || !all.CostsVisible {
		t.Fatalf("Movimientos del arroz: %d", all.Total)
	}
	kinds := []string{}
	for _, movement := range all.Items {
		kinds = append(kinds, movement.Kind)
		if movement.ItemName != "Arroz blanco" || movement.Unit.Code != "kg" || movement.KindLabel == "" {
			t.Fatalf("Movimiento sin nombres legibles: %+v", movement)
		}
	}
	for _, kind := range []string{"initial", "consumption", "receipt", "cost_adjustment", "transfer_out", "transfer_in"} {
		if !slices.Contains(kinds, kind) {
			t.Fatalf("Falta el movimiento %s: %v", kind, kinds)
		}
	}
	hidden := decodeResponse[inventory.MovementPage](t, fixture.call(t, fixture.stock, "GET", "inventory/movements?item_id="+rice.ID, nil, 200))
	if hidden.Total != 5 || hidden.CostsVisible {
		t.Fatalf("Sin costos no se listan los ajustes de costo: %d", hidden.Total)
	}
	for _, movement := range hidden.Items {
		if movement.Value != nil || movement.UnitCost != nil || movement.CostBasis != nil || movement.ValuationValueAfter != nil ||
			movement.CostPending != nil {
			t.Fatalf("Un movimiento reveló costos sin permiso: %+v", movement)
		}
		if movement.Kind == "consumption" && (movement.ProcessLabel == nil || movement.ActorName != nil) {
			t.Fatalf("El consumo debe nombrar su proceso: %+v", movement)
		}
	}
	initial := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?kind=initial&page_size=2", nil, 200))
	if initial.Total != 5 || len(initial.Items) != 2 {
		t.Fatalf("Filtro por tipo y paginación: %d %d", initial.Total, len(initial.Items))
	}
	inBar := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?warehouse_id="+bar.ID, nil, 200))
	if inBar.Total != 2 {
		t.Fatalf("Movimientos de Barra: %d", inBar.Total)
	}
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location).Format("2006-01-02")
	yesterday := time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	dated := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?from="+today+"&until="+today, nil, 200))
	if dated.Total != 10 {
		t.Fatalf("Movimientos de hoy: %d", dated.Total)
	}
	past := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?until="+yesterday, nil, 200))
	if past.Total != 0 {
		t.Fatalf("Movimientos de ayer: %d", past.Total)
	}
	bySource := decodeResponse[inventory.MovementPage](t, fixture.adminCall(t, "GET", "inventory/movements?source_kind=acceptance", nil, 200))
	if bySource.Total != 5 {
		t.Fatalf("Filtro por origen: %d", bySource.Total)
	}
	fixture.adminCall(t, "GET", "inventory/movements?kind=robo", nil, 400)
	fixture.adminCall(t, "GET", "inventory/movements?from=10/11/2026", nil, 400)
	fixture.call(t, fixture.clerk, "GET", "inventory/movements", nil, 403)
}

func testInventoryCostVisibility(t *testing.T, fixture *inventoryFixture) {
	rice := fixture.itemByCode(t, fixture.admin, "ARZ-01")
	// Sin inventory.costs.read: ni costos de referencia, ni promedios, ni valores, ni valuación total.
	hiddenItem := decodeResponse[inventory.Item](t, fixture.call(t, fixture.stock, "GET", "inventory/items/"+rice.ID, nil, 200))
	if hiddenItem.CostsVisible || hiddenItem.ReferenceCost != nil || hiddenItem.ReferenceCostDisplay != nil {
		t.Fatalf("El insumo reveló su costo: %+v", hiddenItem)
	}
	hiddenStock := decodeResponse[inventory.StockPage](t, fixture.call(t, fixture.stock, "GET", "inventory/stock?search=ARZ", nil, 200))
	if hiddenStock.CostsVisible || hiddenStock.ValuationVisible || hiddenStock.TotalValue != nil || len(hiddenStock.Items) != 1 {
		t.Fatalf("Existencias sin costos: %+v", hiddenStock)
	}
	row := hiddenStock.Items[0]
	if row.AverageCost != nil || row.Value != nil || row.CostPending != nil || row.TotalQuantity != "3" || !row.BelowMinimum ||
		!row.AtReorderPoint || len(row.Warehouses) != 2 {
		t.Fatalf("Fila de existencias sin costos: %+v", row)
	}
	// Con costos y valuación: promedio $25, valor $75 y valor total del inventario.
	visible := decodeResponse[inventory.StockPage](t, fixture.call(t, fixture.costs, "GET", "inventory/stock?search=ARZ", nil, 200))
	costRow := visible.Items[0]
	if costRow.AverageCost == nil || *costRow.AverageCost != "25.00" || *costRow.Value != "75.00" || visible.TotalValue == nil {
		t.Fatalf("Existencias con costos: %+v", costRow)
	}
	// Valor total: arroz 75 + refrescos 480 + 300 + aceite 112.50 + cebolla 18 = 985.50.
	if *visible.TotalValue != "985.50" || *visible.TotalValueDisplay != "985.50" {
		t.Fatalf("Valuación total: %s", *visible.TotalValue)
	}
	// Por almacén: el arroz tiene 2 kg en el general y 1 kg en Barra.
	general := decodeResponse[inventory.StockPage](t, fixture.call(t, fixture.costs, "GET",
		"inventory/stock?search=ARZ&warehouse_id="+fixture.general.ID, nil, 200))
	if general.Warehouse == nil || general.Items[0].Quantity != "2" || general.Items[0].TotalQuantity != "3" {
		t.Fatalf("Existencia por almacén: %+v", general.Items)
	}
	below := decodeResponse[inventory.StockPage](t, fixture.call(t, fixture.stock, "GET", "inventory/stock?state=below_minimum", nil, 200))
	if below.Total != 1 || below.Items[0].ItemID != rice.ID {
		t.Fatalf("Filtro bajo mínimo: %+v", below.Items)
	}
	fixture.call(t, fixture.stock, "GET", "inventory/stock?state=robado", nil, 400)
	fixture.call(t, fixture.clerk, "GET", "inventory/stock", nil, 403)
	// Resumen: conteos para todos; costos pendientes y valuación solo con permiso.
	summary := decodeResponse[inventory.Summary](t, fixture.call(t, fixture.stock, "GET", "inventory/summary", nil, 200))
	if summary.ActiveItems != 5 || summary.ActiveWarehouses != 3 || summary.BelowMinimum != 1 || summary.AtReorderPoint != 1 ||
		summary.NegativeStock != 0 || summary.PendingCostItems != nil || summary.TotalValue != nil || summary.Feature.Enabled {
		t.Fatalf("Resumen sin costos: %+v", summary)
	}
	full := decodeResponse[inventory.Summary](t, fixture.call(t, fixture.costs, "GET", "inventory/summary", nil, 200))
	if full.PendingCostItems == nil || *full.PendingCostItems != 0 || full.TotalValue == nil || *full.TotalValue != "985.50" {
		t.Fatalf("Resumen con costos: %+v", full)
	}
	// Un almacén con existencias no se desactiva.
	bar := fixture.warehouseByName(t, "Barra")
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/warehouses/"+bar.ID, map[string]any{
		"revision": bar.Revision, "name": "Barra", "active": false,
	}, 409)); code != "WAREHOUSE_HAS_STOCK" {
		t.Fatalf("Almacén con existencias desactivado: %s", code)
	}
	// La unidad base de un insumo con movimientos no cambia.
	if code := errorCode(t, fixture.adminWrite(t, "PATCH", "inventory/items/"+rice.ID, map[string]any{
		"revision": rice.Revision, "code": "ARZ-01", "name": "Arroz blanco", "base_unit_id": fixture.units["g"].ID, "active": true,
	}, 409)); code != "BASE_UNIT_LOCKED" {
		t.Fatalf("Unidad base con movimientos: %s", code)
	}
}

func testInventorySettingsAndFeature(t *testing.T, fixture *inventoryFixture) {
	cursor := eventCursor(t, fixture.restaurantFixture)
	settings := decodeResponse[inventory.Settings](t, fixture.adminCall(t, "GET", "inventory/settings", nil, 200))
	saved := decodeResponse[inventory.Settings](t, fixture.adminWrite(t, "PUT", "inventory/settings", map[string]any{
		"revision": settings.Revision, "purchase_tax_in_cost": false, "operating_day_cutoff": "04:30",
		"warn_products_without_recipe": true, "count_frequency": "custom", "count_interval_days": 7,
	}, 200))
	if saved.PurchaseTaxInCost || saved.OperatingDayCutoff != "04:30" || saved.CountIntervalDays == nil || *saved.CountIntervalDays != 7 ||
		saved.Revision != settings.Revision+1 || saved.UpdatedByName == nil {
		t.Fatalf("Configuración guardada: %+v", saved)
	}
	for _, body := range []map[string]any{
		{"revision": saved.Revision, "purchase_tax_in_cost": true, "operating_day_cutoff": "05:00", "warn_products_without_recipe": true, "count_frequency": "custom"},
		{"revision": saved.Revision, "purchase_tax_in_cost": true, "operating_day_cutoff": "25:00", "warn_products_without_recipe": true, "count_frequency": "none"},
		{"revision": saved.Revision, "purchase_tax_in_cost": true, "operating_day_cutoff": "05:00", "warn_products_without_recipe": true, "count_frequency": "weekly", "count_interval_days": 3},
	} {
		fixture.adminWrite(t, "PUT", "inventory/settings", body, 400)
	}
	fixture.adminWrite(t, "PUT", "inventory/settings", map[string]any{
		"revision": settings.Revision, "purchase_tax_in_cost": true, "operating_day_cutoff": "05:00",
		"warn_products_without_recipe": true, "count_frequency": "none",
	}, 409)
	fixture.write(t, fixture.clerk, "PUT", "inventory/settings", map[string]any{
		"revision": saved.Revision, "purchase_tax_in_cost": true, "operating_day_cutoff": "05:00",
		"warn_products_without_recipe": true, "count_frequency": "none",
	}, 403)
	if rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.audit_events WHERE action='inventory.settings_changed'") != 1 {
		t.Fatal("El cambio de configuración debe auditarse una vez")
	}
	// Función activable: enabled_at se fija al activar y se conserva al desactivar.
	var current features.Feature
	for _, feature := range decodeResponse[[]features.Feature](t, fixture.adminCall(t, "GET", "settings/features", nil, 200)) {
		if feature.Code == features.InventoryControl {
			current = feature
		}
	}
	if current.Name != "Control de inventario" || current.Enabled || current.EnabledAt != nil {
		t.Fatalf("Función de inventario inicial: %+v", current)
	}
	enabled := decodeResponse[features.Feature](t, fixture.adminCall(t, "PUT", "settings/features", map[string]any{
		"code": features.InventoryControl, "enabled": true, "revision": current.Revision,
	}, 200))
	if !enabled.Enabled || enabled.EnabledAt == nil {
		t.Fatalf("Al activar se guarda la fecha: %+v", enabled)
	}
	summary := decodeResponse[inventory.Summary](t, fixture.call(t, fixture.stock, "GET", "inventory/summary", nil, 200))
	if !summary.Feature.Enabled || summary.Feature.EnabledAt == nil || !summary.Feature.EnabledAt.Equal(*enabled.EnabledAt) {
		t.Fatalf("El resumen informa la activación: %+v", summary.Feature)
	}
	disabled := decodeResponse[features.Feature](t, fixture.adminCall(t, "PUT", "settings/features", map[string]any{
		"code": features.InventoryControl, "enabled": false, "revision": enabled.Revision,
	}, 200))
	if disabled.Enabled || disabled.EnabledAt == nil || !disabled.EnabledAt.Equal(*enabled.EnabledAt) {
		t.Fatalf("Desactivar conserva la fecha de activación: %+v", disabled)
	}
	again := decodeResponse[features.Feature](t, fixture.adminCall(t, "PUT", "settings/features", map[string]any{
		"code": features.InventoryControl, "enabled": true, "revision": disabled.Revision,
	}, 200))
	if again.EnabledAt == nil || !again.EnabledAt.After(*enabled.EnabledAt) {
		t.Fatalf("Reactivar fija una fecha nueva: %v → %v", enabled.EnabledAt, again.EnabledAt)
	}
	// Dejarla desactivada para el resto de la aceptación (en la fase A todavía no hay consumo).
	fixture.adminCall(t, "PUT", "settings/features", map[string]any{
		"code": features.InventoryControl, "enabled": false, "revision": again.Revision,
	}, 200)
	// Eventos del ámbito inventory: los recibe quien tiene inventory.read; quien no, ninguno.
	received := fixture.inventoryEvents(t, fixture.stock, cursor)
	kinds := eventKinds(received)
	if !slices.Contains(kinds, "inventory.settings.changed") {
		t.Fatalf("Eventos de configuración de inventario: %v", kinds)
	}
	if outsider := fixture.inventoryEvents(t, fixture.outsider, cursor); len(outsider) != 0 {
		t.Fatalf("Un usuario sin inventory.read recibió eventos de inventario: %v", eventKinds(outsider))
	}
	if count := rowCount(t, fixture.restaurantFixture, "SELECT count(*) FROM app.realtime_events WHERE scope='inventory' AND sequence>$1",
		cursor); count != len(received) {
		t.Fatalf("Eventos de inventario registrados %d, recibidos %d", count, len(received))
	}
}
