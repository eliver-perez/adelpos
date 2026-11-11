//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/inventory"
)

// Etapa 9A, fase A (A2, migración 0016): datos adicionales de proveedores (§3.3), proveedor especial
// «Compras de mostrador» (§3.2) y comprobación del código interno de insumos (§3.4). Se encadena al
// final de las compras de la etapa 8 con su propio proveedor formal, sin depender del principal (que
// esa fase deja desactivado).
func testInventoryStage9A(t *testing.T, fixture *purchasingFixture) {
	counter := counterSupplier(t, fixture)
	t.Run("inventory_9a_supplier_contact_fields", func(t *testing.T) {
		testSupplierContactFields(t, fixture)
	})
	t.Run("inventory_9a_counter_supplier_rules", func(t *testing.T) {
		testCounterSupplierRules(t, fixture, counter)
	})
	t.Run("inventory_9a_counter_direct_receipts", func(t *testing.T) {
		testCounterDirectReceipts(t, fixture, counter)
	})
	t.Run("inventory_9a_item_code_check", func(t *testing.T) {
		testItemCodeCheck(t, fixture)
	})
}

// counterSupplier comprueba la siembra de la 0016: un solo proveedor de mostrador, activo, sin RFC,
// razón social, insumos ni compras, y sin usuario que lo creara.
func counterSupplier(t *testing.T, fixture *purchasingFixture) inventory.Supplier {
	t.Helper()
	page := decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?kind=counter", nil, 200))
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("Debe existir exactamente un proveedor de mostrador: %+v", page.Items)
	}
	counter := *page.Items[0]
	if counter.Kind != inventory.SupplierKindCounter || counter.TradeName != "Compras de mostrador" || !counter.Active ||
		counter.RFC != nil || counter.LegalName != "" || counter.ItemCount != 0 || counter.HasPurchases || counter.UpdatedByName != nil {
		t.Fatalf("Proveedor de mostrador sembrado: %+v", counter)
	}
	if fixture.count(t, "SELECT count(*) FROM app.inventory_suppliers WHERE kind='counter' AND created_by IS NULL") != 1 {
		t.Fatal("El proveedor de mostrador sembrado no debe atribuirse a un usuario")
	}
	formal := decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?kind=formal&page_size=100",
		nil, 200))
	for _, supplier := range formal.Items {
		if supplier.Kind != inventory.SupplierKindFormal {
			t.Fatalf("El filtro de proveedores formales incluyó %+v", supplier)
		}
	}
	all := decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?active=true", nil, 200))
	if len(all.Items) == 0 || all.Items[0].ID != counter.ID {
		t.Fatal("El proveedor de mostrador debe aparecer primero entre los activos")
	}
	fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?kind=mostrador", nil, 400)
	return counter
}

// testSupplierContactFields: segundo teléfono, celular y página web opcionales y validados; un
// proveedor sin ellos (o con un teléfono principal libre, como los de la etapa 8) se sigue guardando.
func testSupplierContactFields(t *testing.T, fixture *purchasingFixture) {
	body := map[string]any{
		"trade_name": "Verduras Hermanos Ruiz 9A", "phone": "Pedir a Juan", "phone_secondary": " (33) 3615-0000 ext. 12 ",
		"mobile_phone": "+52 1 33 1234 5678", "website": "https://www.verdurasruiz.example.mx/pedidos", "contact": "Juan Ruiz",
	}
	created := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers", body, 200))
	if created.Kind != inventory.SupplierKindFormal || created.Phone != "Pedir a Juan" || created.PhoneSecondary != "(33) 3615-0000 ext. 12" ||
		created.MobilePhone != "+52 1 33 1234 5678" || created.Website != "https://www.verdurasruiz.example.mx/pedidos" {
		t.Fatalf("Datos adicionales del proveedor: %+v", created.Supplier)
	}
	for _, rejected := range []map[string]any{
		{"trade_name": "Celular corto 9A", "mobile_phone": "12345"},
		{"trade_name": "Teléfono con texto 9A", "phone_secondary": "llamar en la tarde"},
		{"trade_name": "Teléfono con salto 9A", "phone_secondary": "33 3615\n0000"},
		{"trade_name": "Web con esquema 9A", "website": "javascript:alert(1)"},
		{"trade_name": "Web con espacio 9A", "website": "www.proveedor .mx"},
		{"trade_name": "Web larga 9A", "website": "https://" + strings.Repeat("a", 200) + ".mx"},
		{"trade_name": "Modalidad 9A", "kind": "counter"},
	} {
		fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers", rejected, 400)
	}
	if fixture.count(t, "SELECT count(*) FROM app.inventory_suppliers WHERE trade_name LIKE '% 9A' AND trade_name<>$1", created.TradeName) != 0 {
		t.Fatal("Un proveedor rechazado quedó registrado")
	}
	// La búsqueda incluye el celular y la página web.
	for _, search := range []string{"52 1 33", "verdurasruiz"} {
		page := decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?search="+
			strings.ReplaceAll(search, " ", "%20"), nil, 200))
		if page.Total != 1 || page.Items[0].ID != created.ID {
			t.Fatalf("Búsqueda %q: %+v", search, page.Items)
		}
	}
	// Borrar los datos nuevos al editar (vacíos son «sin dato») y conservar el resto.
	edited := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+created.ID,
		map[string]any{"revision": created.Revision, "trade_name": created.TradeName, "phone": created.Phone, "contact": created.Contact,
			"website": "verdurasruiz.example.mx"}, 200))
	if edited.PhoneSecondary != "" || edited.MobilePhone != "" || edited.Website != "verdurasruiz.example.mx" || edited.Revision != 2 {
		t.Fatalf("Edición de los datos adicionales: %+v", edited.Supplier)
	}
	// Un proveedor de la etapa 8 (sin los campos nuevos en el cuerpo) se sigue guardando.
	second := fixture.suppliers["second"]
	current := decodeResponse[inventory.SupplierDetail](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers/"+second.ID, nil, 200))
	saved := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+second.ID,
		map[string]any{"revision": current.Revision, "trade_name": current.TradeName, "rfc": current.RFC, "notes": "Etapa 9A"}, 200))
	if saved.Notes != "Etapa 9A" || saved.PhoneSecondary != "" || saved.Kind != inventory.SupplierKindFormal {
		t.Fatalf("Proveedor existente sin los campos nuevos: %+v", saved.Supplier)
	}
	fixture.suppliers["contact"] = edited
}

// testCounterSupplierRules: el proveedor de mostrador es único, no lleva RFC ni razón social, no
// surte insumos y se rechaza en órdenes de compra y en borradores desde sugerencias.
func testCounterSupplierRules(t *testing.T, fixture *purchasingFixture, counter inventory.Supplier) {
	// Único también en PostgreSQL (índice parcial) y sin RFC ni razón social (CHECK).
	for _, statement := range []string{
		"INSERT INTO app.inventory_suppliers(id,trade_name,kind) VALUES (gen_random_uuid(),'Otro mostrador 9A','counter')",
		"UPDATE app.inventory_suppliers SET rfc='ADN010203AB1' WHERE kind='counter'",
		"UPDATE app.inventory_suppliers SET legal_name='Mercado S.A.' WHERE kind='counter'",
		"UPDATE app.inventory_suppliers SET kind='otro' WHERE kind='counter'",
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó: %s", statement)
		}
	}
	base := map[string]any{"revision": counter.Revision, "trade_name": counter.TradeName, "notes": counter.Notes}
	withRFC := map[string]any{"rfc": "ADN010203AB1"}
	withLegal := map[string]any{"legal_name": "Mercado Municipal S.A."}
	for _, extra := range []map[string]any{withRFC, withLegal} {
		body := map[string]any{}
		for key, value := range base {
			body[key] = value
		}
		for key, value := range extra {
			body[key] = value
		}
		fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+counter.ID, body, 400)
	}
	// Se puede renombrar y describir; la modalidad no cambia.
	renamed := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+counter.ID,
		map[string]any{"revision": counter.Revision, "trade_name": "Compras de mostrador", "notes": "Mercados y centrales"}, 200))
	if renamed.Kind != inventory.SupplierKindCounter || renamed.Notes != "Mercados y centrales" || renamed.Revision != counter.Revision+1 {
		t.Fatalf("Edición del proveedor de mostrador: %+v", renamed.Supplier)
	}
	// No surte insumos: ni preferido ni último precio.
	if errorCode(t, fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+counter.ID+"/items", map[string]any{
		"revision": 0, "item_id": fixture.items["harina"].ID, "preferred": true,
	}, 409)) != "COUNTER_SUPPLIER" {
		t.Fatal("El proveedor de mostrador registró un insumo surtido")
	}
	// Órdenes de compra: ni nuevas ni cambiando el proveedor de un borrador.
	orders := fixture.count(t, "SELECT count(*) FROM app.inventory_purchase_orders")
	line := orderLine(fixture.items["vaso"], "10", "2", "", 16, "", "")
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": counter.ID, "lines": []map[string]any{line},
	}, 409)) != "COUNTER_SUPPLIER" {
		t.Fatal("Se creó una orden de compra con el proveedor de mostrador")
	}
	formal := fixture.suppliers["contact"]
	draft := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": formal.ID, "lines": []map[string]any{line},
	}, 200))
	if errorCode(t, fixture.write(t, fixture.buyer, "PATCH", "inventory/purchase-orders/"+draft.ID, map[string]any{
		"revision": draft.Revision, "supplier_id": counter.ID, "lines": []map[string]any{line},
	}, 409)) != "COUNTER_SUPPLIER" {
		t.Fatal("Un borrador cambió al proveedor de mostrador")
	}
	// Sugerencias: el borrador desde sugerencias tampoco lo admite.
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-suggestions/draft-order", map[string]any{
		"supplier_id": counter.ID, "item_ids": []string{fixture.items["vaso"].ID},
	}, 409)) != "COUNTER_SUPPLIER" {
		t.Fatal("Se creó un borrador desde sugerencias con el proveedor de mostrador")
	}
	if fixture.count(t, "SELECT count(*) FROM app.inventory_purchase_orders") != orders+1 ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_purchase_orders WHERE supplier_id=$1", counter.ID) != 0 {
		t.Fatal("Los rechazos del proveedor de mostrador dejaron órdenes")
	}
}

// testCounterDirectReceipts: la recepción directa con «Compras de mostrador» y el comercio concreto;
// sin último precio; distinguible en listas; las recepciones históricas quedan intactas.
func testCounterDirectReceipts(t *testing.T, fixture *purchasingFixture, counter inventory.Supplier) {
	historic := fixture.count(t, "SELECT count(*) FROM app.inventory_receipts")
	historicFormal := fixture.count(t, "SELECT count(*) FROM app.inventory_receipts r JOIN app.inventory_suppliers s ON s.id=r.supplier_id "+
		"WHERE s.kind='formal' AND r.counter_store=''")
	if historic == 0 || historicFormal != historic {
		t.Fatalf("Las recepciones de la etapa 8 deben quedar formales y sin comercio: %d de %d", historicFormal, historic)
	}
	liter := fixture.units["l"].ID
	direct := func(supplier, store string) map[string]any {
		return map[string]any{
			"supplier_id": supplier, "counter_store": store, "supplier_reference": "Nota 55",
			"lines": []map[string]any{orderLine(fixture.items["aceite"], "3", "40", "", 0, "", liter)},
		}
	}
	// El comercio solo describe compras de mostrador; una línea de hasta 100 caracteres.
	formal := fixture.suppliers["contact"].ID
	fixture.receive(t, fixture.buyer, direct(formal, "Central de abastos"), 400)
	fixture.receive(t, fixture.buyer, direct(counter.ID, "Central\nde abastos"), 400)
	fixture.receive(t, fixture.buyer, direct(counter.ID, strings.Repeat("m", 101)), 400)
	// Quien solo recibe tampoco registra una compra de mostrador (es una recepción directa).
	fixture.receive(t, fixture.receiver, direct(counter.ID, "Central de abastos"), 403)
	if fixture.count(t, "SELECT count(*) FROM app.inventory_receipts") != historic {
		t.Fatal("Una compra de mostrador rechazada dejó registros")
	}
	// 3 l × $40 = $120.00 exento, en la Central de abastos.
	market := fixture.receive(t, fixture.buyer, direct(counter.ID, "  Central de abastos, bodega 12 "), 200)
	if !market.Direct || market.Supplier.ID != counter.ID || market.Supplier.Kind != inventory.SupplierKindCounter ||
		market.CounterStore != "Central de abastos, bodega 12" || market.SupplierReference != "Nota 55" || market.Total != "120.00" {
		t.Fatalf("Compra de mostrador: %+v", market.ReceiptSummary)
	}
	// Sin comercio también procede.
	plain := fixture.receive(t, fixture.buyer, direct(counter.ID, ""), 200)
	if plain.CounterStore != "" || plain.Supplier.Kind != inventory.SupplierKindCounter {
		t.Fatalf("Compra de mostrador sin comercio: %+v", plain.ReceiptSummary)
	}
	// Sin último precio ni insumos surtidos: el proveedor de mostrador nunca alimenta sugerencias.
	if fixture.count(t, "SELECT count(*) FROM app.inventory_supplier_items WHERE supplier_id=$1", counter.ID) != 0 {
		t.Fatal("La compra de mostrador registró el último precio")
	}
	var reason string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT reason FROM app.inventory_movements WHERE source_id=$1 AND kind='receipt'",
		market.ID).Scan(&reason); err != nil || !strings.Contains(reason, "Compras de mostrador (Central de abastos, bodega 12)") {
		t.Fatalf("Motivo del movimiento de la compra de mostrador: %q %v", reason, err)
	}
	var audited string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT after_state->>'counter_store' FROM app.audit_events
        WHERE action='inventory.receipt_recorded' AND entity_id=$1 AND after_state->>'supplier_kind'='counter'`, market.ID).Scan(&audited); err != nil ||
		audited != "Central de abastos, bodega 12" {
		t.Fatalf("Auditoría de la compra de mostrador: %q %v", audited, err)
	}
	// Distinguible en listas: filtro por modalidad y búsqueda por comercio.
	counterList := decodeResponse[inventory.ReceiptPage](t, fixture.call(t, fixture.receiver, "GET", "inventory/receipts?supplier_kind=counter",
		nil, 200))
	if counterList.Total != 2 {
		t.Fatalf("Recepciones de mostrador: %d", counterList.Total)
	}
	formalList := decodeResponse[inventory.ReceiptPage](t, fixture.call(t, fixture.receiver, "GET", "inventory/receipts?supplier_kind=formal",
		nil, 200))
	if formalList.Total != historic {
		t.Fatalf("Recepciones formales: %d de %d", formalList.Total, historic)
	}
	found := decodeResponse[inventory.ReceiptPage](t, fixture.call(t, fixture.receiver, "GET", "inventory/receipts?search=bodega%2012", nil, 200))
	if found.Total != 1 || found.Items[0].ID != market.ID {
		t.Fatalf("Búsqueda por comercio: %+v", found.Items)
	}
	fixture.call(t, fixture.receiver, "GET", "inventory/receipts?supplier_kind=mercado", nil, 400)
	// El historial del proveedor de mostrador muestra sus recepciones.
	detail := decodeResponse[inventory.SupplierDetail](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers/"+counter.ID, nil, 200))
	if detail.History.ReceiptCount != 2 || !detail.HasPurchases || len(detail.Items) != 0 || detail.History.ReceiptsTotal != "240.00" {
		t.Fatalf("Historial del proveedor de mostrador: %+v", detail.History)
	}
	// Desactivado no admite compras nuevas; reactivado vuelve a admitirlas.
	deactivated := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+counter.ID,
		map[string]any{"revision": detail.Revision, "trade_name": detail.TradeName, "notes": detail.Notes, "active": false}, 200))
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/receipts", direct(counter.ID, ""), 409)) != "SUPPLIER_INACTIVE" {
		t.Fatal("Se recibió con el proveedor de mostrador desactivado")
	}
	fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+counter.ID, map[string]any{
		"revision": deactivated.Revision, "trade_name": deactivated.TradeName, "notes": deactivated.Notes, "active": true,
	}, 200)
	// Las recepciones anteriores conservan su proveedor, sus importes y su comercio vacío.
	if fixture.count(t, "SELECT count(*) FROM app.inventory_receipts r JOIN app.inventory_suppliers s ON s.id=r.supplier_id "+
		"WHERE s.kind='formal' AND r.counter_store=''") != historicFormal {
		t.Fatal("Las compras de mostrador alteraron recepciones históricas")
	}
	checkLedgerConsistency(t, fixture.inventoryFixture)
}

// testItemCodeCheck: Go indica si un código interno está libre (sin distinguir mayúsculas,
// excluido el insumo que se edita) y sugiere el primero libre con la regla de la etapa 6.1.
func testItemCodeCheck(t *testing.T, fixture *purchasingFixture) {
	check := func(query string) inventory.ItemCodeCheck {
		t.Helper()
		return decodeResponse[inventory.ItemCodeCheck](t, fixture.call(t, fixture.reader, "GET", "inventory/item-codes?"+query, nil, 200))
	}
	harina := fixture.items["harina"]
	taken := check("code=c-har")
	if taken.Available || taken.Suggestion != "c-har-2" || text(taken.TakenBy) != harina.Name || taken.Code != "c-har" {
		t.Fatalf("Código tomado: %+v", taken)
	}
	fixture.createItem(t, map[string]any{
		"code": "C-HAR-2", "name": "Harina integral 9A", "base_unit_id": fixture.units["g"].ID, "kind": "purchased", "active": true,
	})
	if next := check("code=c-har"); next.Suggestion != "c-har-3" {
		t.Fatalf("Sugerencia con -2 ocupado: %+v", next)
	}
	// Al editar el propio insumo su código cuenta como libre.
	if own := check("code=C-HAR&item_id=" + harina.ID); !own.Available || own.Suggestion != "C-HAR" || own.TakenBy != nil {
		t.Fatalf("Código del propio insumo: %+v", own)
	}
	if free := check("code=harina-de-centeno"); !free.Available || free.Suggestion != "harina-de-centeno" {
		t.Fatalf("Código libre: %+v", free)
	}
	// Los comodines de LIKE no se interpretan.
	if wildcard := check("code=c%25"); !wildcard.Available || wildcard.Suggestion != "c%" {
		t.Fatalf("Comodín: %+v", wildcard)
	}
	// Go sigue siendo la autoridad al guardar.
	if errorCode(t, fixture.adminWrite(t, "POST", "inventory/items", map[string]any{
		"code": "c-har", "name": "Harina repetida 9A", "base_unit_id": fixture.units["g"].ID, "kind": "purchased", "active": true,
	}, 409)) != "DUPLICATE_CODE" {
		t.Fatal("Se aceptó un código interno repetido")
	}
	for _, rejected := range []string{"", "code=", "code=" + strings.Repeat("x", 41), "code=a&item_id=otro", "code=a&page=1"} {
		fixture.call(t, fixture.reader, "GET", "inventory/item-codes?"+rejected, nil, 400)
	}
	fixture.call(t, fixture.outsider, "GET", "inventory/item-codes?code=c-har", nil, 403)
}
