//go:build integration

package operations_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa 8, fase C: compras (0013). Se encadena al final de la fase B con sus propios insumos,
// proveedores, usuarios y caja («Caja de compras C»), para no depender del estado de pruebas
// anteriores. Las cifras de cada caso están calculadas a mano en los comentarios.

type purchasingFixture struct {
	*inventoryFixture
	// buyer crea órdenes y recibe (incluida la recepción directa) sin autorizar ni ver costos;
	// authorizer solo emite, cancela y cierra; receiver solo recibe; returnsClerk solo devuelve;
	// supplierClerk solo administra proveedores; reader solo consulta inventario.
	buyer, authorizer, receiver, returnsClerk, supplierClerk, reader testTerminal
	// cashier opera la caja de compras; localLinker vincula desde una terminal de esa caja (sin
	// otras cajas ni turnos cerrados); historyLinker además consulta turnos cerrados; remoteLinker
	// vincula desde una terminal sin caja; noCashReader no lee movimientos de caja y cashOnly no
	// vincula.
	cashier, localLinker, historyLinker, remoteLinker, noCashReader, cashOnly testTerminal
	registerID                                                                string
	category                                                                  string
	items                                                                     map[string]inventory.Item
	presentations                                                             map[string]string
	suppliers                                                                 map[string]inventory.SupplierDetail
	orders                                                                    map[string]inventory.PurchaseOrder
	receipts                                                                  map[string]inventory.Receipt
	expenses                                                                  map[string]string
	shifts                                                                    map[string]string
}

// purchasingUser crea un rol, un usuario y una terminal (asociada a la caja indicada, si la hay)
// con sesión iniciada.
func purchasingUser(t *testing.T, fixture *restaurantFixture, username string, permissions []string, registerID string) testTerminal {
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
	if registerID != "" {
		fixture.adminCall(t, "PATCH", "devices/"+device.ID, map[string]any{
			"name": "Terminal " + username, "register_id": registerID, "keyboard_mode": "auto",
		}, 200)
	}
	terminal := testTerminal{deviceID: device.ID, deviceToken: device.Token}
	login := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": username, "password": password}, 200))
	terminal.sessionToken = login.Token
	return terminal
}

func testInventoryPurchasing(t *testing.T, base *inventoryFixture) {
	fixture := &purchasingFixture{
		inventoryFixture: base, items: map[string]inventory.Item{}, presentations: map[string]string{},
		suppliers: map[string]inventory.SupplierDetail{}, orders: map[string]inventory.PurchaseOrder{},
		receipts: map[string]inventory.Receipt{}, expenses: map[string]string{}, shifts: map[string]string{},
	}
	t.Run("inventory_c_setup", func(t *testing.T) {
		fixture.setup(t)
	})
	t.Run("inventory_c_suppliers", func(t *testing.T) {
		testPurchasingSuppliers(t, fixture)
	})
	t.Run("inventory_c_purchase_orders", func(t *testing.T) {
		testPurchaseOrders(t, fixture)
	})
	t.Run("inventory_c_receipts_total_partial_and_repeated", func(t *testing.T) {
		testPartialReceipts(t, fixture)
	})
	t.Run("inventory_c_excess_close_and_cancel_with_receipts", func(t *testing.T) {
		testReceiptExcessAndClosing(t, fixture)
	})
	t.Run("inventory_c_direct_receipts_and_tax_in_cost", func(t *testing.T) {
		testDirectReceiptsAndTax(t, fixture)
	})
	t.Run("inventory_c_receipt_with_negative_stock", func(t *testing.T) {
		testReceiptWithNegativeStock(t, fixture)
	})
	t.Run("inventory_c_supplier_returns", func(t *testing.T) {
		testSupplierReturns(t, fixture)
	})
	t.Run("inventory_c_cash_links", func(t *testing.T) {
		testCashLinks(t, fixture)
	})
	t.Run("inventory_c_supplier_deactivation_events_and_audit", func(t *testing.T) {
		testSupplierDeactivationAndAudit(t, fixture)
	})
	// Etapa 9A (0016): datos adicionales de proveedores, «Compras de mostrador» y código sugerido.
	testInventoryStage9A(t, fixture)
}

func (fixture *purchasingFixture) setup(t *testing.T) {
	units := fixture.units
	for _, entry := range []struct{ key, code, name, unit string }{
		{"refresco", "C-REF", "Refresco 600 ml C", "pza"},
		{"harina", "C-HAR", "Harina de trigo C", "g"},
		{"aceite", "C-ACE", "Aceite C", "ml"},
		{"queso", "C-QUE", "Queso fresco C", "pza"},
		{"vaso", "C-VAS", "Vaso desechable C", "pza"},
	} {
		fixture.items[entry.key] = fixture.createItem(t, map[string]any{
			"code": entry.code, "name": entry.name, "base_unit_id": units[entry.unit].ID, "kind": "purchased", "active": true,
		})
	}
	for _, entry := range []struct{ key, item, name, quantity, unit string }{
		{"caja", "refresco", "Caja con 24", "24", "pza"},
		{"costal", "harina", "Costal 25 kg", "25", "kg"},
	} {
		item := decodeResponse[inventory.Item](t, fixture.adminWrite(t, "POST", "inventory/items/"+fixture.items[entry.item].ID+"/presentations",
			map[string]any{"name": entry.name, "quantity": entry.quantity, "unit_id": units[entry.unit].ID, "active": true}, 200))
		for _, presentation := range item.Presentations {
			if presentation.Name == entry.name {
				fixture.presentations[entry.key] = presentation.ID
			}
		}
		fixture.items[entry.item] = item
	}
	base := fixture.restaurantFixture
	fixture.buyer = purchasingUser(t, base, "compras_comprador", []string{
		"events.read", "inventory.read", "inventory.purchases.create", "inventory.purchases.receive",
	}, "")
	fixture.authorizer = purchasingUser(t, base, "compras_autoriza", []string{"inventory.read", "inventory.purchases.authorize"}, "")
	fixture.receiver = purchasingUser(t, base, "compras_recibe", []string{"inventory.read", "inventory.purchases.receive"}, "")
	fixture.returnsClerk = purchasingUser(t, base, "compras_devoluciones", []string{"inventory.read", "inventory.supplier_returns.record"}, "")
	fixture.supplierClerk = purchasingUser(t, base, "compras_proveedores", []string{"inventory.read", "inventory.suppliers.manage"}, "")
	fixture.reader = purchasingUser(t, base, "compras_lector", []string{"inventory.read", "inventory.stock.read"}, "")
	fixture.registerID = decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": "Caja de compras C", "description": "", "active": true}, 200)).ID
	cashier := append([]string{"cash.outflows.authorize", "cash.corrections.authorize"}, cashierPermissions...)
	fixture.cashier = purchasingUser(t, base, "compras_cajero", cashier, fixture.registerID)
	linker := []string{"inventory.read", "inventory.purchases.link_cash", "cash.movements.read"}
	fixture.localLinker = purchasingUser(t, base, "compras_vincula", linker, fixture.registerID)
	fixture.historyLinker = purchasingUser(t, base, "compras_vincula_historial", append([]string{"cash.history.read"}, linker...),
		fixture.registerID)
	fixture.remoteLinker = purchasingUser(t, base, "compras_vincula_remoto", linker, "")
	fixture.noCashReader = purchasingUser(t, base, "compras_sin_caja", []string{"inventory.read", "inventory.purchases.link_cash"},
		fixture.registerID)
	fixture.cashOnly = purchasingUser(t, base, "compras_solo_caja", []string{"inventory.read", "cash.movements.read"}, fixture.registerID)
	catalog := decodeResponse[struct {
		Categories []payments.ExpenseCategory `json:"expense_categories"`
	}](t, fixture.adminCall(t, "GET", "payments/cash/catalog", nil, 200))
	for _, category := range catalog.Categories {
		if category.Name == "Insumos" {
			fixture.category = category.ID
		}
	}
	if fixture.category == "" {
		t.Fatal("No existe la categoría de gasto «Insumos»")
	}
	// La configuración de la fase A puede haber quedado con el IVA fuera del costo: estas pruebas
	// parten del valor predeterminado (IVA incluido en el costo).
	fixture.setTaxInCost(t, true)
}

// setTaxInCost guarda «El IVA de las compras forma parte del costo» con la revisión vigente.
func (fixture *purchasingFixture) setTaxInCost(t *testing.T, taxInCost bool) {
	t.Helper()
	settings := decodeResponse[inventory.Settings](t, fixture.adminCall(t, "GET", "inventory/settings", nil, 200))
	if settings.PurchaseTaxInCost == taxInCost {
		return
	}
	fixture.adminWrite(t, "PUT", "inventory/settings", map[string]any{
		"revision": settings.Revision, "purchase_tax_in_cost": taxInCost, "operating_day_cutoff": settings.OperatingDayCutoff,
		"warn_products_without_recipe": settings.WarnProductsWithoutRecipe, "count_frequency": settings.CountFrequency,
		"count_interval_days": settings.CountIntervalDays,
	}, 200)
}

// count devuelve un conteo de PostgreSQL.
func (fixture *purchasingFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	return rowCount(t, fixture.restaurantFixture, query, args...)
}

func testPurchasingSuppliers(t *testing.T, fixture *purchasingFixture) {
	body := map[string]any{
		"trade_name": "Abarrotes del Norte C", "legal_name": "Abarrotes del Norte S.A. de C.V.", "rfc": " adn010203ab1 ",
		"phone": "81 1234 5678", "email": "ventas@abarrotes.example", "address": "Av. Juárez 100\nCentro", "contact": "Laura Pérez",
		"notes": "Entrega martes y viernes",
	}
	// Sin permiso de proveedores no se da de alta ni se consulta sin ningún permiso de compras.
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/suppliers", body, 403)) != "PERMISSION_DENIED" {
		t.Fatal("Un comprador sin inventory.suppliers.manage dio de alta un proveedor")
	}
	fixture.call(t, fixture.reader, "GET", "inventory/suppliers", nil, 403)
	// Solo existe el proveedor especial «Compras de mostrador» que siembra la 0016 (etapa 9A).
	if fixture.count(t, "SELECT count(*) FROM app.inventory_suppliers WHERE kind='formal'") != 0 {
		t.Fatal("Un rechazo dejó un proveedor registrado")
	}
	main := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers", body, 200))
	if main.TradeName != "Abarrotes del Norte C" || text(main.RFC) != "ADN010203AB1" || !main.Active || main.Revision != 1 ||
		main.HasPurchases || main.Address != "Av. Juárez 100\nCentro" || len(main.Items) != 0 || main.History.OrderCount != 0 {
		t.Fatalf("Proveedor registrado: %+v", main.Supplier)
	}
	// Nombre comercial único sin distinguir mayúsculas; RFC no único; formatos validados.
	duplicate := map[string]any{"trade_name": "ABARROTES DEL NORTE c"}
	if errorCode(t, fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers", duplicate, 409)) != "DUPLICATE_NAME" {
		t.Fatal("Se aceptó un nombre comercial repetido")
	}
	for _, rejected := range []map[string]any{
		{"trade_name": "RFC inválido", "rfc": "ADN011303AB1"},
		{"trade_name": "RFC corto", "rfc": "AD010203AB1"},
		{"trade_name": "Correo inválido", "email": "ventas@"},
		{"trade_name": " "},
	} {
		fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers", rejected, 400)
	}
	second := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "POST", "inventory/suppliers",
		map[string]any{"trade_name": "Lácteos La Vaca C", "rfc": "ADN010203AB1"}, 200))
	// Edición con revisión: la obsoleta se rechaza.
	edited := map[string]any{
		"revision": 1, "trade_name": "Abarrotes del Norte C", "legal_name": "Abarrotes del Norte S.A. de C.V.", "rfc": "ADN010203AB1",
		"phone": "81 1234 5678", "email": "compras@abarrotes.example", "address": "Av. Juárez 100", "contact": "Laura Pérez",
		"notes": "Entrega martes y viernes",
	}
	main = decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+main.ID, edited, 200))
	if main.Revision != 2 || main.Email != "compras@abarrotes.example" || !main.Active {
		t.Fatalf("Edición del proveedor: %+v", main.Supplier)
	}
	if errorCode(t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+main.ID, edited, 409)) != "REVISION_CONFLICT" {
		t.Fatal("Se aceptó una revisión obsoleta del proveedor")
	}
	// Insumos que surte: un preferido por insumo; el segundo preferido quita la marca al primero.
	item := map[string]any{
		"revision": 0, "item_id": fixture.items["refresco"].ID, "presentation_id": fixture.presentations["caja"], "supplier_code": "R600",
		"preferred": true,
	}
	main = decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+main.ID+"/items", item, 200))
	if len(main.Items) != 1 || !main.Items[0].Preferred || main.Items[0].SupplierCode != "R600" || main.Items[0].Presentation == nil ||
		main.Items[0].Presentation.Name != "Caja con 24" || main.Items[0].Revision != 1 || main.ItemCount != 1 {
		t.Fatalf("Insumo del proveedor: %+v", main.Items)
	}
	if errorCode(t, fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+main.ID+"/items", item, 409)) != "REVISION_CONFLICT" {
		t.Fatal("Se aceptó una revisión obsoleta del insumo del proveedor")
	}
	// Una presentación de otro insumo se rechaza.
	foreign := map[string]any{"revision": 0, "item_id": fixture.items["harina"].ID, "presentation_id": fixture.presentations["caja"]}
	fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+main.ID+"/items", foreign, 400)
	fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+main.ID+"/items", map[string]any{
		"revision": 0, "item_id": fixture.items["harina"].ID, "presentation_id": fixture.presentations["costal"], "supplier_code": "H25",
	}, 200)
	second = decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PUT", "inventory/suppliers/"+second.ID+"/items",
		map[string]any{"revision": 0, "item_id": fixture.items["refresco"].ID, "preferred": true}, 200))
	main = decodeResponse[inventory.SupplierDetail](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers/"+main.ID, nil, 200))
	for _, supplied := range main.Items {
		if supplied.Item.ID == fixture.items["refresco"].ID && supplied.Preferred {
			t.Fatal("El refresco quedó con dos proveedores preferidos")
		}
	}
	if fixture.count(t, "SELECT count(*) FROM app.inventory_supplier_items WHERE item_id=$1 AND preferred", fixture.items["refresco"].ID) != 1 {
		t.Fatal("Debe existir un solo proveedor preferido por insumo")
	}
	// Quitar un insumo del segundo proveedor (configuración, no historia).
	second = decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "POST",
		"inventory/suppliers/"+second.ID+"/items/remove", map[string]any{"revision": 1, "item_id": fixture.items["refresco"].ID}, 200))
	if len(second.Items) != 0 {
		t.Fatalf("El insumo no se quitó del proveedor: %+v", second.Items)
	}
	// Búsqueda y filtros: por RFC (no único, ambos lo comparten antes de quitarlo), por insumo y por estado.
	page := decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?search=ADN010203", nil, 200))
	if page.Total != 2 {
		t.Fatalf("Búsqueda por RFC: %d", page.Total)
	}
	page = decodeResponse[inventory.SupplierPage](t, fixture.call(t, fixture.buyer, "GET",
		"inventory/suppliers?item_id="+fixture.items["harina"].ID+"&active=true", nil, 200))
	if page.Total != 1 || page.Items[0].ID != main.ID {
		t.Fatalf("Filtro por insumo: %+v", page.Items)
	}
	fixture.call(t, fixture.buyer, "GET", "inventory/suppliers?order_by=name", nil, 400)
	fixture.suppliers["main"] = main
	fixture.suppliers["second"] = second
	if fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action IN ('inventory.supplier_created','inventory.supplier_updated',"+
		"'inventory.supplier_item_saved','inventory.supplier_item_removed')") != 7 {
		t.Fatal("Las altas, ediciones e insumos de proveedores deben auditarse")
	}
}

// orderLine arma un renglón de orden.
func orderLine(item inventory.Item, quantity, price, discount string, rate int, presentation, unit string) map[string]any {
	line := map[string]any{"item_id": item.ID, "quantity": quantity, "unit_price": price, "discount": discount, "tax_rate": rate}
	if presentation != "" {
		line["presentation_id"] = presentation
	}
	if unit != "" {
		line["unit_id"] = unit
	}
	return line
}

func (fixture *purchasingFixture) order(t *testing.T, id string) inventory.PurchaseOrder {
	t.Helper()
	return decodeResponse[inventory.PurchaseOrder](t, fixture.call(t, fixture.buyer, "GET", "inventory/purchase-orders/"+id, nil, 200))
}

// issueOrder crea y emite una orden del proveedor principal.
func (fixture *purchasingFixture) issueOrder(t *testing.T, lines ...map[string]any) inventory.PurchaseOrder {
	t.Helper()
	draft := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": fixture.suppliers["main"].ID, "lines": lines,
	}, 200))
	return decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.authorizer, "POST",
		"inventory/purchase-orders/"+draft.ID+"/issue", map[string]any{"revision": draft.Revision}, 200))
}

func testPurchaseOrders(t *testing.T, fixture *purchasingFixture) {
	movementsBefore := fixture.count(t, "SELECT count(*) FROM app.inventory_movements")
	cashBefore := fixture.count(t, "SELECT count(*) FROM app.cash_operations")
	box, sack, liter := fixture.presentations["caja"], fixture.presentations["costal"], fixture.units["l"].ID
	lines := []map[string]any{
		// 20 cajas × $250 − $50 = $4,950.00; IVA 16 % = $792.00; total $5,742.00 (480 piezas).
		orderLine(fixture.items["refresco"], "20", "250", "50", 16, box, ""),
		// 2 costales × $430.50 = $861.00 exento (50,000 g).
		orderLine(fixture.items["harina"], "2", "430.50", "", 0, sack, ""),
		// 4 l × $45 = $180.00; IVA 8 % = $14.40 (se corrige a 5 l en el borrador). Total 5,742 + 861 + 194.40.
		orderLine(fixture.items["aceite"], "4", "45", "0", 8, "", liter),
	}
	body := map[string]any{"supplier_id": fixture.suppliers["main"].ID, "notes": "Pedido semanal", "lines": lines}
	// Sin inventory.purchases.create no se crea la orden.
	fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders", body, 403)
	draft := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", body, 200))
	if draft.State != "draft" || draft.Folio <= 0 || draft.LineCount != 3 || draft.Total != "6797.40" || draft.Tax != "806.40" ||
		draft.ExpectedWarehouse.ID != fixture.general.ID || !draft.Editable || draft.Receivable || draft.CreatedByName == "" {
		t.Fatalf("Borrador de orden: %+v", draft.PurchaseOrderSummary)
	}
	// El borrador se edita con revisión: 5 l × $45 = $225.00; IVA $18.00.
	lines[2] = orderLine(fixture.items["aceite"], "5", "45", "0", 8, "", liter)
	body["revision"] = draft.Revision
	body["lines"] = lines
	draft = decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.buyer, "PATCH", "inventory/purchase-orders/"+draft.ID, body, 200))
	// Documento: subtotal 4,950 + 861 + 225 = $6,036.00; descuento $50.00; IVA 792 + 0 + 18 = $810.00; total $6,846.00.
	if draft.Revision != 2 || draft.Subtotal != "6036.00" || draft.Discount != "50.00" || draft.Tax != "810.00" || draft.Total != "6846.00" {
		t.Fatalf("Totales de la orden: %+v", draft.PurchaseOrderSummary)
	}
	first := draft.Lines[0]
	if first.Gross != "5000.00" || first.Discount != "50.00" || first.Subtotal != "4950.00" || first.Tax != "792.00" ||
		first.Total != "5742.00" || first.BaseQuantity != "480" || first.CaptureLabel != "Caja con 24" || first.PendingQuantity != "20" {
		t.Fatalf("Renglón de refresco: %+v", first)
	}
	if draft.Lines[1].Tax != "0.00" || draft.Lines[1].BaseQuantity != "50000" || draft.Lines[2].BaseQuantity != "5000" ||
		draft.Lines[2].Tax != "18.00" || draft.Lines[2].Unit == nil || draft.Lines[2].Unit.Code != "l" {
		t.Fatalf("Renglones de harina y aceite: %+v %+v", draft.Lines[1], draft.Lines[2])
	}
	// Tasa fuera de la lista cerrada, descuento mayor que el importe e insumo repetido.
	for _, rejected := range [][]map[string]any{
		{orderLine(fixture.items["refresco"], "1", "250", "0", 15, box, "")},
		{orderLine(fixture.items["refresco"], "1", "250", "250.01", 16, box, "")},
		{orderLine(fixture.items["refresco"], "1", "250", "0", 16, box, ""), orderLine(fixture.items["refresco"], "24", "10", "0", 16, "", "")},
		{orderLine(fixture.items["harina"], "1", "10", "0", 16, box, "")},
	} {
		fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
			"supplier_id": fixture.suppliers["main"].ID, "lines": rejected,
		}, 400)
	}
	fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": fixture.suppliers["main"].ID, "order_date": "2999-01-01", "lines": lines,
	}, 400)
	// Emitir: el comprador no autoriza; el autorizador sí. Crear y emitir no tocan existencias ni caja.
	fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders/"+draft.ID+"/issue", map[string]any{"revision": draft.Revision}, 403)
	issued := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.authorizer, "POST",
		"inventory/purchase-orders/"+draft.ID+"/issue", map[string]any{"revision": draft.Revision}, 200))
	if issued.State != "issued" || issued.IssuedAt == nil || text(issued.IssuedByName) == "" || issued.Editable || !issued.Receivable {
		t.Fatalf("Orden emitida: %+v", issued.PurchaseOrderSummary)
	}
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+draft.ID+"/issue",
		map[string]any{"revision": issued.Revision}, 409)) != "ORDER_NOT_DRAFT" {
		t.Fatal("Se emitió dos veces la misma orden")
	}
	body["revision"] = issued.Revision
	if errorCode(t, fixture.write(t, fixture.buyer, "PATCH", "inventory/purchase-orders/"+draft.ID, body, 409)) != "ORDER_NOT_EDITABLE" {
		t.Fatal("Se editó una orden emitida")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.inventory_purchase_order_lines SET quantity=1 WHERE order_id=$1", draft.ID); err == nil {
		t.Fatal("PostgreSQL permitió cambiar los renglones de una orden emitida")
	}
	if fixture.count(t, "SELECT count(*) FROM app.inventory_movements") != movementsBefore ||
		fixture.count(t, "SELECT count(*) FROM app.cash_operations") != cashBefore {
		t.Fatal("Crear o emitir una orden tocó existencias o caja")
	}
	fixture.orders["weekly"] = issued
	// Una orden vacía no se emite; un borrador se cancela con motivo y no admite recepciones.
	empty := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": fixture.suppliers["main"].ID, "lines": []map[string]any{},
	}, 200))
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+empty.ID+"/issue",
		map[string]any{"revision": empty.Revision}, 409)) != "ORDER_EMPTY" {
		t.Fatal("Se emitió una orden sin renglones")
	}
	fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+empty.ID+"/cancel",
		map[string]any{"revision": empty.Revision, "reason": "No"}, 400)
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+empty.ID+"/close",
		map[string]any{"revision": empty.Revision, "reason": "Cierre de un borrador"}, 409)) != "ORDER_NOT_CLOSABLE" {
		t.Fatal("Se cerró administrativamente un borrador")
	}
	cancelled := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.authorizer, "POST",
		"inventory/purchase-orders/"+empty.ID+"/cancel", map[string]any{"revision": empty.Revision, "reason": "Pedido duplicado"}, 200))
	if cancelled.State != "cancelled" || text(cancelled.CancelReason) != "Pedido duplicado" || cancelled.IssuedAt != nil || cancelled.Cancellable {
		t.Fatalf("Orden cancelada: %+v", cancelled)
	}
	if errorCode(t, fixture.write(t, fixture.receiver, "POST", "inventory/receipts", map[string]any{
		"purchase_order_id": empty.ID, "lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}},
	}, 409)) != "ORDER_NOT_RECEIVABLE" {
		t.Fatal("Se recibió una orden cancelada")
	}
	// Listado y filtros.
	list := decodeResponse[inventory.PurchaseOrderPage](t, fixture.call(t, fixture.receiver, "GET",
		"inventory/purchase-orders?state=open&supplier_id="+fixture.suppliers["main"].ID, nil, 200))
	if list.Total != 1 || list.Items[0].ID != issued.ID {
		t.Fatalf("Órdenes abiertas: %+v", list.Items)
	}
	fixture.call(t, fixture.receiver, "GET", "inventory/purchase-orders?state=pagada", nil, 400)
	if len(decodeResponse[inventory.PurchaseOrderPage](t, fixture.call(t, fixture.buyer, "GET",
		"inventory/purchase-orders?search="+itoaFolio(issued.Folio), nil, 200)).Items) != 1 {
		t.Fatal("La búsqueda por folio no encontró la orden")
	}
}

func itoaFolio(folio int64) string {
	return decimal.NewFromInt(folio).String()
}

// receive registra una recepción con la terminal indicada.
func (fixture *purchasingFixture) receive(t *testing.T, terminal testTerminal, body map[string]any, status int) inventory.Receipt {
	t.Helper()
	return decodeResponse[inventory.Receipt](t, fixture.write(t, terminal, "POST", "inventory/receipts", body, status))
}

func (fixture *purchasingFixture) receipt(t *testing.T, terminal testTerminal, id string) inventory.Receipt {
	t.Helper()
	return decodeResponse[inventory.Receipt](t, fixture.call(t, terminal, "GET", "inventory/receipts/"+id, nil, 200))
}

// valuation devuelve cantidad y valor del insumo (todos sus almacenes).
func (fixture *purchasingFixture) valuation(t *testing.T, item string) (string, string) {
	t.Helper()
	var quantity, value string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT quantity::text,value::text FROM app.inventory_valuations WHERE item_id=$1",
		fixture.items[item].ID).Scan(&quantity, &value); err != nil {
		t.Fatal(err)
	}
	return quantity, value
}

func testPartialReceipts(t *testing.T, fixture *purchasingFixture) {
	order := fixture.orders["weekly"]
	cashBefore := fixture.count(t, "SELECT count(*) FROM app.cash_operations")
	// Primera recepción: 12 de 20 cajas y 1 de 2 costales, por quien solo recibe.
	operation := randomID()
	body := map[string]any{
		"operation_id": operation, "purchase_order_id": order.ID, "supplier_reference": "Remisión 4471",
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "12"}, {"order_line_no": 2, "quantity": "1"}},
	}
	raw := fixture.call(t, fixture.receiver, "POST", "inventory/receipts", body, 200)
	first := decodeResponse[inventory.Receipt](t, raw)
	// 12 × $250 = $3,000.00 − 60 % del descuento ($30.00) = $2,970.00; IVA $475.20; total $3,445.20.
	// Costal: $430.50. Documento: subtotal $3,400.50, descuento $30.00, IVA $475.20, total $3,875.70.
	if first.Direct || first.PurchaseOrderID == nil || *first.PurchaseOrderID != order.ID || !first.TaxInCost || first.Total != "3875.70" ||
		first.Subtotal != "3400.50" || first.Discount != "30.00" || first.Tax != "475.20" || first.SupplierReference != "Remisión 4471" ||
		first.Warehouse.ID != fixture.general.ID || first.CostsVisible || first.InventoryValue != nil || first.LinkableAmount != "3875.70" {
		t.Fatalf("Primera recepción: %+v", first.ReceiptSummary)
	}
	soda := first.Lines[0]
	if soda.Discount != "30.00" || soda.Total != "3445.20" || soda.BaseQuantity != "288" || text(soda.OrderedQuantity) != "20" ||
		text(soda.ReceivedBeforeQuantity) != "0" || text(soda.PendingAfterQuantity) != "8" || soda.StockAfter != "288" ||
		soda.UnitCost != nil || soda.ReturnableQuantity != "12" {
		t.Fatalf("Renglón recibido de refresco: %+v", soda)
	}
	// Con costos: valor de inventario con IVA (configuración vigente) y costo por pieza 3,445.20 ÷ 288 = 11.9625.
	costed := fixture.receipt(t, fixture.admin, first.ID)
	if text(costed.InventoryValue) != "3875.70" || text(costed.Lines[0].UnitCost) != "11.9625" ||
		text(costed.Lines[1].UnitCost) != "0.01722" {
		t.Fatalf("Costos de la recepción: %v %v %v", text(costed.InventoryValue), text(costed.Lines[0].UnitCost), text(costed.Lines[1].UnitCost))
	}
	current := fixture.order(t, order.ID)
	if current.State != "partially_received" || current.Revision != order.Revision+1 || current.Lines[0].ReceivedQuantity != "12" ||
		current.Lines[0].PendingQuantity != "8" || current.Lines[0].PendingBaseQuantity != "192" || current.Lines[1].PendingQuantity != "1" ||
		current.Lines[2].ReceivedQuantity != "0" || current.Lines[2].PendingQuantity != "5" || len(current.Receipts) != 1 {
		t.Fatalf("Orden tras la primera recepción: %+v", current)
	}
	// La misma operación no recibe dos veces; otro cuerpo con el mismo identificador es un conflicto.
	movements := fixture.count(t, "SELECT count(*) FROM app.inventory_movements")
	replayed := fixture.call(t, fixture.receiver, "POST", "inventory/receipts", body, 200)
	if decodeResponse[inventory.Receipt](t, replayed).ID != first.ID ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_receipts WHERE purchase_order_id=$1", order.ID) != 1 ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_movements") != movements {
		t.Fatal("La recepción repetida se aplicó otra vez")
	}
	changed := map[string]any{"operation_id": operation, "purchase_order_id": order.ID,
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "13"}}}
	if errorCode(t, fixture.call(t, fixture.receiver, "POST", "inventory/receipts", changed, 409)) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("Un cuerpo distinto con el mismo operation_id no se rechazó")
	}
	// Recibir más de lo pendiente sin confirmarlo se rechaza sin escribir nada.
	if errorCode(t, fixture.write(t, fixture.receiver, "POST", "inventory/receipts", map[string]any{
		"purchase_order_id": order.ID, "lines": []map[string]any{{"order_line_no": 1, "quantity": "9"}},
	}, 409)) != "RECEIPT_EXCEEDS_PENDING" {
		t.Fatal("Se recibió más de lo pendiente sin allow_excess")
	}
	// Un renglón con insumo propio no pertenece a la recepción de una orden.
	fixture.write(t, fixture.receiver, "POST", "inventory/receipts", map[string]any{
		"purchase_order_id": order.ID, "lines": []map[string]any{{"order_line_no": 1, "quantity": "1", "item_id": fixture.items["vaso"].ID}},
	}, 400)
	// Segunda recepción: las 8 cajas restantes (40 % del descuento: $20.00), el otro costal y los 5 l.
	second := fixture.receive(t, fixture.receiver, map[string]any{
		"purchase_order_id": order.ID,
		"lines": []map[string]any{
			{"order_line_no": 1, "quantity": "8"}, {"order_line_no": 2, "quantity": "1"}, {"order_line_no": 3, "quantity": "5"},
		},
	}, 200)
	// 8 × 250 = 2,000 − 20 = $1,980.00 + IVA $316.80 = $2,296.80; costal $430.50; aceite $243.00.
	// Documento: subtotal $2,635.50, descuento $20.00, IVA $334.80, total $2,970.30. Las dos suman la orden: $6,846.00.
	if second.Total != "2970.30" || second.Discount != "20.00" || second.Tax != "334.80" || second.Lines[0].Total != "2296.80" ||
		text(second.Lines[0].ReceivedBeforeQuantity) != "12" || text(second.Lines[0].PendingAfterQuantity) != "0" ||
		second.Lines[2].BaseQuantity != "5000" {
		t.Fatalf("Segunda recepción: %+v %+v", second.ReceiptSummary, second.Lines[0])
	}
	current = fixture.order(t, order.ID)
	if current.State != "received" || current.Receivable || !current.Lines[0].FullyReceived || current.Lines[0].PendingQuantity != "0" ||
		current.Lines[0].ReceivedQuantity != "20" || len(current.Receipts) != 2 {
		t.Fatalf("Orden recibida: %+v", current)
	}
	// 480 piezas por 3,445.20 + 2,296.80 = $5,742.00 (promedio 11.9625).
	if quantity, value := fixture.valuation(t, "refresco"); quantity != "480.000000" || value != "5742.000000" {
		t.Fatalf("Valuación del refresco: %s %s", quantity, value)
	}
	if errorCode(t, fixture.write(t, fixture.receiver, "POST", "inventory/receipts", map[string]any{
		"purchase_order_id": order.ID, "lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}},
	}, 409)) != "ORDER_NOT_RECEIVABLE" {
		t.Fatal("Se recibió una orden ya recibida")
	}
	// Movimientos receipt con su origen; último precio del proveedor actualizado.
	if fixture.count(t, "SELECT count(*) FROM app.inventory_movements WHERE source_kind='receipt' AND kind='receipt' AND source_id=$1",
		second.ID) != 3 {
		t.Fatal("Cada renglón recibido debe registrar un movimiento receipt")
	}
	supplier := decodeResponse[inventory.SupplierDetail](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers/"+fixture.suppliers["main"].ID, nil, 200))
	for _, item := range supplier.Items {
		if item.Item.ID == fixture.items["refresco"].ID && (text(item.LastUnitPrice) != "250.00" || text(item.LastPriceUnitLabel) != "Caja con 24") {
			t.Fatalf("Último precio del refresco: %+v", item)
		}
	}
	if !supplier.HasPurchases || supplier.History.ReceiptCount != 2 || supplier.History.ReceiptsTotal != "6846.00" {
		t.Fatalf("Historial del proveedor: %+v", supplier.History)
	}
	if fixture.count(t, "SELECT count(*) FROM app.cash_operations") != cashBefore {
		t.Fatal("Una recepción creó un movimiento de caja")
	}
	fixture.receipts["first"] = first
	fixture.receipts["second"] = second
	checkLedgerConsistency(t, fixture.inventoryFixture)
}

func testReceiptExcessAndClosing(t *testing.T, fixture *purchasingFixture) {
	sack, box := fixture.presentations["costal"], fixture.presentations["caja"]
	// Exceso: 3 costales de 2 ordenados solo con allow_excess (aviso).
	extra := fixture.issueOrder(t, orderLine(fixture.items["harina"], "2", "430.50", "", 0, sack, ""))
	lines := []map[string]any{{"order_line_no": 1, "quantity": "3"}}
	fixture.receive(t, fixture.receiver, map[string]any{"purchase_order_id": extra.ID, "lines": lines}, 409)
	excess := fixture.receive(t, fixture.receiver, map[string]any{"purchase_order_id": extra.ID, "lines": lines, "allow_excess": true}, 200)
	if !excess.Excess || len(excess.Warnings) != 1 || excess.Warnings[0].Code != "RECEIPT_EXCESS" || excess.Total != "1291.50" {
		t.Fatalf("Recepción con excedente: %+v %+v", excess.ReceiptSummary, excess.Warnings)
	}
	current := fixture.order(t, extra.ID)
	if current.State != "received" || current.Lines[0].ExcessQuantity != "1" || current.Lines[0].PendingQuantity != "0" {
		t.Fatalf("Orden con excedente: %+v", current.Lines[0])
	}
	// Cierre administrativo: 4 de 10 cajas recibidas; lo pendiente ya no se espera y lo recibido queda.
	partial := fixture.issueOrder(t, orderLine(fixture.items["refresco"], "10", "250", "", 16, box, ""))
	received := fixture.receive(t, fixture.receiver, map[string]any{"purchase_order_id": partial.ID,
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "4"}}}, 200)
	current = fixture.order(t, partial.ID)
	fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders/"+partial.ID+"/close",
		map[string]any{"revision": current.Revision, "reason": "El proveedor ya no surtirá el resto"}, 403)
	closed := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+partial.ID+"/close",
		map[string]any{"revision": current.Revision, "reason": "El proveedor ya no surtirá el resto"}, 200))
	if closed.State != "closed" || text(closed.CloseReason) != "El proveedor ya no surtirá el resto" || closed.Receivable ||
		len(closed.Receipts) != 1 || closed.Lines[0].ReceivedQuantity != "4" {
		t.Fatalf("Orden cerrada: %+v", closed)
	}
	if errorCode(t, fixture.write(t, fixture.receiver, "POST", "inventory/receipts", map[string]any{"purchase_order_id": partial.ID,
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}}}, 409)) != "ORDER_NOT_RECEIVABLE" {
		t.Fatal("Se recibió una orden cerrada")
	}
	// Cancelar una orden con recepciones conserva las entradas.
	oil := fixture.issueOrder(t, orderLine(fixture.items["aceite"], "2", "45", "", 8, "", fixture.units["l"].ID))
	oilReceipt := fixture.receive(t, fixture.receiver, map[string]any{"purchase_order_id": oil.ID,
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}}}, 200)
	current = fixture.order(t, oil.ID)
	// Con la revisión anterior a la recepción, la cancelación exige actualizar.
	if errorCode(t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+oil.ID+"/cancel",
		map[string]any{"revision": oil.Revision, "reason": "Se canceló el pedido"}, 409)) != "REVISION_CONFLICT" {
		t.Fatal("Se canceló con una revisión anterior a la recepción")
	}
	movements := fixture.count(t, "SELECT count(*) FROM app.inventory_movements WHERE source_id=$1", oilReceipt.ID)
	cancelled := decodeResponse[inventory.PurchaseOrder](t, fixture.write(t, fixture.authorizer, "POST", "inventory/purchase-orders/"+oil.ID+"/cancel",
		map[string]any{"revision": current.Revision, "reason": "Se canceló el pedido"}, 200))
	if cancelled.State != "cancelled" || len(cancelled.Receipts) != 1 || cancelled.Lines[0].ReceivedQuantity != "1" ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_movements WHERE source_id=$1", oilReceipt.ID) != movements || movements != 1 {
		t.Fatalf("La cancelación borró entradas: %+v", cancelled)
	}
	fixture.receipts["partial"] = received
}

func testDirectReceiptsAndTax(t *testing.T, fixture *purchasingFixture) {
	supplier := fixture.suppliers["main"].ID
	liter := fixture.units["l"].ID
	direct := func(item inventory.Item, quantity, price string, rate int, presentation, unit string) map[string]any {
		return map[string]any{"supplier_id": supplier, "lines": []map[string]any{orderLine(item, quantity, price, "", rate, presentation, unit)}}
	}
	receipts := fixture.count(t, "SELECT count(*) FROM app.inventory_receipts")
	// Quien solo recibe no registra una recepción directa (exige crear compras); nada se escribe.
	fixture.receive(t, fixture.receiver, direct(fixture.items["aceite"], "2", "50", 16, "", liter), 403)
	if fixture.count(t, "SELECT count(*) FROM app.inventory_receipts") != receipts {
		t.Fatal("Una recepción directa rechazada dejó registros")
	}
	// La directa exige proveedor.
	fixture.receive(t, fixture.buyer, map[string]any{"lines": []map[string]any{
		orderLine(fixture.items["aceite"], "2", "50", "", 16, "", liter)}}, 400)
	// IVA incluido en el costo (configuración vigente): 2 l × $50 = $100 + IVA $16 = $116; costo por ml 0.058.
	included := fixture.receive(t, fixture.buyer, direct(fixture.items["aceite"], "2", "50", 16, "", liter), 200)
	if !included.Direct || !included.TaxInCost || included.Total != "116.00" {
		t.Fatalf("Recepción directa con IVA en el costo: %+v", included.ReceiptSummary)
	}
	fixture.setTaxInCost(t, false)
	// IVA fuera del costo: el mismo documento ($116.00) vale $100 en inventario (0.05 por ml).
	excluded := fixture.receive(t, fixture.buyer, direct(fixture.items["aceite"], "2", "50", 16, "", liter), 200)
	if excluded.TaxInCost || excluded.Total != "116.00" || excluded.Tax != "16.00" || excluded.Subtotal != "100.00" {
		t.Fatalf("Recepción con IVA fuera del costo: %+v", excluded.ReceiptSummary)
	}
	fixture.setTaxInCost(t, true)
	before := fixture.receipt(t, fixture.admin, included.ID)
	after := fixture.receipt(t, fixture.admin, excluded.ID)
	if !before.TaxInCost || text(before.InventoryValue) != "116.00" || text(before.Lines[0].UnitCost) != "0.058" ||
		after.TaxInCost || text(after.InventoryValue) != "100.00" || text(after.Lines[0].UnitCost) != "0.05" {
		t.Fatalf("El cambio de configuración alteró recepciones: %v %v %v %v", text(before.InventoryValue), text(before.Lines[0].UnitCost),
			text(after.InventoryValue), text(after.Lines[0].UnitCost))
	}
	var storedValue string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT value::text FROM app.inventory_movements WHERE source_id=$1 AND kind='receipt'",
		excluded.ID).Scan(&storedValue); err != nil || storedValue != "100.000000" {
		t.Fatalf("Valor del movimiento sin IVA: %s %v", storedValue, err)
	}
	// Recepción directa de $850.00 exactos (un costal exento) para vincular con un gasto.
	fixture.receipts["r850"] = fixture.receive(t, fixture.buyer, direct(fixture.items["harina"], "1", "850", 0, fixture.presentations["costal"], ""), 200)
	if fixture.receipts["r850"].Total != "850.00" {
		t.Fatalf("Recepción de $850: %s", fixture.receipts["r850"].Total)
	}
	list := decodeResponse[inventory.ReceiptPage](t, fixture.call(t, fixture.receiver, "GET",
		"inventory/receipts?direct=true&supplier_id="+supplier, nil, 200))
	if list.Total != 3 {
		t.Fatalf("Recepciones directas del proveedor: %d", list.Total)
	}
}

// applyAsWorker registra un consumo como lo haría el trabajador de Go, con el lock de inventario.
func (fixture *purchasingFixture) applyAsWorker(t *testing.T, key, item, quantity string) inventory.LedgerResult {
	t.Helper()
	process := "inventory_worker"
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.ctx)
	if _, err := tx.Exec(fixture.ctx, "SELECT pg_advisory_xact_lock(7820148308)"); err != nil {
		t.Fatal(err)
	}
	result, err := inventory.Apply(fixture.ctx, tx, inventory.MovementInput{
		EntryKey: key, Kind: inventory.KindConsumption, ItemID: fixture.items[item].ID, WarehouseID: fixture.general.ID,
		Quantity: decimal.RequireFromString(quantity), OccurredAt: time.Now(), SourceKind: "acceptance", Process: &process,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	return result
}

func testReceiptWithNegativeStock(t *testing.T, fixture *purchasingFixture) {
	// 10 piezas a $20 ($200, L = 20) y un consumo de 15: 200 + 5 × 20 = $300 → −5 piezas, −$100.
	fixture.adminWrite(t, "POST", "inventory/initial-balances", map[string]any{
		"warehouse_id": fixture.general.ID, "lines": []map[string]any{{"item_id": fixture.items["queso"].ID, "quantity": "10", "unit_cost": "20"}},
	}, 200)
	fixture.applyAsWorker(t, "acceptance:purchasing:queso:1", "queso", "15")
	if quantity, value := fixture.valuation(t, "queso"); quantity != "-5.000000" || value != "-100.000000" {
		t.Fatalf("Existencia negativa del queso: %s %s", quantity, value)
	}
	// Recepción de 8 piezas a $30 ($240): el déficit de 5 valía 100 provisional y 150 real; el ajuste de
	// costo separado es 100 − 150 = −$50. Resultado: 3 piezas por $90 (promedio $30).
	cheese := fixture.receive(t, fixture.buyer, map[string]any{"supplier_id": fixture.suppliers["main"].ID,
		"lines": []map[string]any{orderLine(fixture.items["queso"], "8", "30", "", 0, "", "")}}, 200)
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT kind,quantity::text,value::text,coalesce(stock_after::text,'-'),
            related_movement_id IS NOT NULL FROM app.inventory_movements WHERE source_id=$1 ORDER BY seq`, cheese.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for rows.Next() {
		var kind, quantity, value, stock string
		var related bool
		if err := rows.Scan(&kind, &quantity, &value, &stock, &related); err != nil {
			t.Fatal(err)
		}
		got = append(got, kind+" "+quantity+" "+value+" "+stock+" "+map[bool]string{true: "ligado", false: "-"}[related])
	}
	rows.Close()
	expected := []string{"receipt 8.000000 240.000000 3.000000 -", "cost_adjustment 0.000000 -50.000000 - ligado"}
	if len(got) != 2 || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("Movimientos de la recepción con existencia negativa: %v", got)
	}
	if quantity, value := fixture.valuation(t, "queso"); quantity != "3.000000" || value != "90.000000" {
		t.Fatalf("Valuación del queso tras la recepción: %s %s", quantity, value)
	}
	// El documento conserva el valor de la recepción ($240) y el costo del documento (30 por pieza).
	costed := fixture.receipt(t, fixture.admin, cheese.ID)
	if text(costed.InventoryValue) != "240.00" || text(costed.Lines[0].UnitCost) != "30.00" {
		t.Fatalf("Costos de la recepción del queso: %v %v", text(costed.InventoryValue), text(costed.Lines[0].UnitCost))
	}
	checkLedgerConsistency(t, fixture.inventoryFixture)
}

func testSupplierReturns(t *testing.T, fixture *purchasingFixture) {
	supplier := fixture.suppliers["main"].ID
	cashBefore := fixture.count(t, "SELECT count(*) FROM app.cash_operations")
	cups := func(price string) inventory.Receipt {
		return fixture.receive(t, fixture.buyer, map[string]any{"supplier_id": supplier,
			"lines": []map[string]any{orderLine(fixture.items["vaso"], "100", price, "", 16, "", "")}}, 200)
	}
	// 100 vasos a $2 + IVA = $232.00 y 100 a $3 + IVA = $348.00: 200 vasos por $580.00 (promedio 2.90).
	cheap := cups("2")
	expensive := cups("3")
	if quantity, value := fixture.valuation(t, "vaso"); quantity != "200.000000" || value != "580.000000" {
		t.Fatalf("Valuación de vasos: %s %s", quantity, value)
	}
	operation := randomID()
	body := map[string]any{
		"operation_id": operation, "receipt_id": expensive.ID, "reason": "Vasos defectuosos",
		"lines": []map[string]any{{"receipt_line_no": 1, "quantity": "10"}},
	}
	// Sin el permiso de devoluciones no se registra.
	fixture.call(t, fixture.receiver, "POST", "inventory/supplier-returns", map[string]any{
		"operation_id": randomID(), "receipt_id": expensive.ID, "reason": "Vasos defectuosos",
		"lines": []map[string]any{{"receipt_line_no": 1, "quantity": "10"}},
	}, 403)
	// Devolución con diferencia absorbida: salen 10 al promedio ($29.00); a costo original valen 10 × 3.48
	// = $34.80; la existencia restante (190) absorbe −$5.80. Quedan 190 vasos por $545.20.
	first := decodeResponse[inventory.SupplierReturn](t, fixture.call(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", body, 200))
	if first.Value != "34.80" || first.Reason != "Vasos defectuosos" || first.ReceiptID != expensive.ID || first.CostsVisible ||
		first.Lines[0].OutflowValue != nil || first.Lines[0].BaseQuantity != "10" || first.Lines[0].StockAfter != "190" {
		t.Fatalf("Devolución con diferencia absorbida: %+v %+v", first.SupplierReturnSummary, first.Lines)
	}
	costed := decodeResponse[inventory.SupplierReturn](t, fixture.adminCall(t, "GET", "inventory/supplier-returns/"+first.ID, nil, 200))
	line := costed.Lines[0]
	if text(line.OutflowValue) != "29.00" || text(line.CostAdjustment) != "-5.80" || text(line.Unabsorbed) != "0.00" ||
		text(line.OriginalUnitCost) != "3.48" || line.CostAdjustmentMovement == nil || text(costed.Unabsorbed) != "0.00" {
		t.Fatalf("Costos de la devolución absorbida: %+v", line)
	}
	if quantity, value := fixture.valuation(t, "vaso"); quantity != "190.000000" || value != "545.200000" {
		t.Fatalf("Valuación tras la devolución absorbida: %s %s", quantity, value)
	}
	// Devolución duplicada: la misma operación devuelve el mismo documento sin otro movimiento.
	movements := fixture.count(t, "SELECT count(*) FROM app.inventory_movements")
	if decodeResponse[inventory.SupplierReturn](t, fixture.call(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", body, 200)).ID != first.ID ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_movements") != movements ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_supplier_returns WHERE receipt_id=$1", expensive.ID) != 1 {
		t.Fatal("La devolución repetida se registró otra vez")
	}
	body["lines"] = []map[string]any{{"receipt_line_no": 1, "quantity": "11"}}
	if errorCode(t, fixture.call(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", body, 409)) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal("Otra devolución con el mismo operation_id no se rechazó")
	}
	// Nunca más de lo recibido menos lo devuelto.
	for _, rejected := range []struct {
		receipt  string
		quantity string
	}{{cheap.ID, "101"}, {expensive.ID, "91"}} {
		if errorCode(t, fixture.write(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", map[string]any{
			"receipt_id": rejected.receipt, "reason": "Devolución excesiva", "lines": []map[string]any{{"receipt_line_no": 1, "quantity": rejected.quantity}},
		}, 409)) != "RETURN_EXCEEDS_RECEIVED" {
			t.Fatalf("Se devolvió más de lo recibido: %+v", rejected)
		}
	}
	fixture.write(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", map[string]any{
		"receipt_id": expensive.ID, "reason": "No", "lines": []map[string]any{{"receipt_line_no": 1, "quantity": "1"}},
	}, 400)
	// Consumo de 185: 185 × 545.20 ÷ 190 = 530.852632; quedan 5 vasos por $14.347368.
	fixture.applyAsWorker(t, "acceptance:purchasing:vaso:1", "vaso", "185")
	// Devolución con diferencia no absorbida: salen los 5 restantes por $14.347368 exactos; a costo
	// original valen 5 × 3.48 = $17.40; sin existencia restante, 17.40 − 14.347368 = 3.052632 queda sin
	// absorber en el documento (sin movimiento).
	second := decodeResponse[inventory.SupplierReturn](t, fixture.write(t, fixture.returnsClerk, "POST", "inventory/supplier-returns", map[string]any{
		"receipt_id": expensive.ID, "reason": "Fin de temporada", "lines": []map[string]any{{"receipt_line_no": 1, "quantity": "5"}},
	}, 200))
	costed = decodeResponse[inventory.SupplierReturn](t, fixture.adminCall(t, "GET", "inventory/supplier-returns/"+second.ID, nil, 200))
	line = costed.Lines[0]
	if second.Value != "17.40" || text(line.OutflowValue) != "14.347368" || text(line.CostAdjustment) != "0.00" ||
		text(line.Unabsorbed) != "3.052632" || line.CostAdjustmentMovement != nil || text(costed.Unabsorbed) != "3.052632" ||
		line.StockAfter != "0" {
		t.Fatalf("Devolución con diferencia no absorbida: %+v %+v", costed.SupplierReturnSummary, line)
	}
	if quantity, value := fixture.valuation(t, "vaso"); quantity != "0.000000" || value != "0.000000" {
		t.Fatalf("Valuación tras la devolución no absorbida: %s %s", quantity, value)
	}
	receipt := fixture.receipt(t, fixture.buyer, expensive.ID)
	if receipt.Lines[0].ReturnedQuantity != "15" || receipt.Lines[0].ReturnableQuantity != "85" || len(receipt.Returns) != 2 ||
		receipt.ReturnedValue != "52.20" || receipt.Total != "348.00" {
		t.Fatalf("Recepción con devoluciones: %+v %+v", receipt.ReceiptSummary, receipt.Lines[0])
	}
	list := decodeResponse[inventory.SupplierReturnPage](t, fixture.call(t, fixture.buyer, "GET",
		"inventory/supplier-returns?receipt_id="+expensive.ID, nil, 200))
	if list.Total != 2 {
		t.Fatalf("Devoluciones de la recepción: %d", list.Total)
	}
	if fixture.count(t, "SELECT count(*) FROM app.cash_operations") != cashBefore {
		t.Fatal("Una devolución a proveedor creó un movimiento de caja")
	}
	for _, statement := range []string{
		"UPDATE app.inventory_receipts SET total=0",
		"DELETE FROM app.inventory_receipt_lines",
		"UPDATE app.inventory_supplier_returns SET value=0",
		"DELETE FROM app.inventory_supplier_return_lines",
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s", statement)
		}
	}
	checkLedgerConsistency(t, fixture.inventoryFixture)
}

// shiftAction ejecuta una acción de caja del cajero de compras.
func (fixture *purchasingFixture) shiftAction(t *testing.T, method, shiftID, action string, body map[string]any, status int) []byte {
	t.Helper()
	route := "payments/shifts/" + shiftID
	if action != "" {
		route += "/" + action
	}
	if method != "GET" {
		body["operation_id"] = randomID()
	}
	return fixture.call(t, fixture.cashier, method, route, body, status)
}

func (fixture *purchasingFixture) expense(t *testing.T, shift, key, amount string) {
	t.Helper()
	operation := decodeResponse[payments.CashOperation](t, fixture.shiftAction(t, "POST", shift, "expenses", map[string]any{
		"category_id": fixture.category, "amount": amount, "reference": "Compra " + key,
	}, 200))
	fixture.expenses[key] = operation.ID
}

func (fixture *purchasingFixture) correct(t *testing.T, shift, key string) payments.CashOperation {
	t.Helper()
	return decodeResponse[correctionResult](t, fixture.shiftAction(t, "POST", shift, "corrections", map[string]any{
		"corrects_operation_id": fixture.expenses[key], "reason": "Gasto capturado por error",
	}, 200)).Correction
}

func (fixture *purchasingFixture) link(t *testing.T, terminal testTerminal, receipt, expense, amount string, status int) []byte {
	t.Helper()
	return fixture.write(t, terminal, "POST", "inventory/receipts/"+receipt+"/cash-links", map[string]any{
		"cash_operation_id": expense, "amount": amount,
	}, status)
}

func linkOf(t *testing.T, receipt inventory.Receipt, expense string, active bool) inventory.CashLink {
	t.Helper()
	for _, link := range receipt.CashLinks {
		if link.CashOperationID == expense && link.Active == active {
			return link
		}
	}
	t.Fatalf("La recepción no tiene el vínculo con el gasto %s (vigente %v): %+v", expense, active, receipt.CashLinks)
	return inventory.CashLink{}
}

func testCashLinks(t *testing.T, fixture *purchasingFixture) {
	r850, first := fixture.receipts["r850"].ID, fixture.receipts["first"].ID
	cashBefore := fixture.count(t, "SELECT count(*) FROM app.cash_operations")
	receiptsBefore := fixture.count(t, "SELECT count(*) FROM app.inventory_receipts")
	ordersBefore := fixture.count(t, "SELECT count(*) FROM app.inventory_purchase_orders")
	shift := decodeResponse[payments.Shift](t, fixture.call(t, fixture.cashier, "POST", "payments/shifts", map[string]any{
		"operation_id": randomID(), "opening_float": "5000",
	}, 200))
	fixture.shifts["first"] = shift.ID
	for key, amount := range map[string]string{"e850": "850", "e500": "500", "e300": "300", "e200": "200", "e150": "150"} {
		fixture.expense(t, shift.ID, key, amount)
	}
	// Gasto sin compra: registrar gastos no crea compras ni recepciones.
	if fixture.count(t, "SELECT count(*) FROM app.inventory_receipts") != receiptsBefore ||
		fixture.count(t, "SELECT count(*) FROM app.inventory_purchase_orders") != ordersBefore {
		t.Fatal("Un gasto de caja creó una compra")
	}
	e300Correction := fixture.correct(t, shift.ID, "e300")
	// Permisos: sin leer movimientos de caja, sin vincular o desde otra caja sin permiso, nada se escribe.
	fixture.link(t, fixture.noCashReader, r850, fixture.expenses["e850"], "850", 403)
	fixture.link(t, fixture.cashOnly, r850, fixture.expenses["e850"], "850", 403)
	fixture.link(t, fixture.remoteLinker, r850, fixture.expenses["e850"], "850", 403)
	if fixture.count(t, "SELECT count(*) FROM app.inventory_receipt_cash_links") != 0 {
		t.Fatal("Un vínculo rechazado quedó registrado")
	}
	if len(decodeResponse[inventory.CashExpensePage](t, fixture.call(t, fixture.remoteLinker, "GET", "inventory/cash-expenses", nil, 200)).Items) != 0 {
		t.Fatal("Una terminal sin caja ni permiso de otras cajas vio gastos")
	}
	fixture.call(t, fixture.remoteLinker, "GET", "inventory/cash-expenses?register_id="+fixture.registerID, nil, 403)
	// Vinculación total: la recepción de $850 con el gasto de $850.
	linked := decodeResponse[inventory.Receipt](t, fixture.link(t, fixture.localLinker, r850, fixture.expenses["e850"], "850", 200))
	full := linkOf(t, linked, fixture.expenses["e850"], true)
	if linked.LinkedAmount != "850.00" || linked.LinkableAmount != "0.00" || full.Status != "active" || !full.ExpenseVisible ||
		full.Expense == nil || full.Expense.Amount != "850.00" || full.Expense.RegisterName != "Caja de compras C" {
		t.Fatalf("Vinculación total: %+v %+v", linked.ReceiptSummary, full)
	}
	// Vinculación parcial: $500 de la recepción de $3,875.70.
	partial := decodeResponse[inventory.Receipt](t, fixture.link(t, fixture.localLinker, first, fixture.expenses["e500"], "500", 200))
	if partial.LinkedAmount != "500.00" || partial.LinkableAmount != "3375.70" {
		t.Fatalf("Vinculación parcial: %+v", partial.ReceiptSummary)
	}
	// Sin egresos duplicados: el gasto de $850 ya está vinculado por completo (también el de $500) y la
	// recepción de $850 también, aunque el gasto de $200 todavía tenga importe disponible.
	if errorCode(t, fixture.link(t, fixture.localLinker, first, fixture.expenses["e850"], "1", 409)) != "CASH_EXPENSE_OVERLINKED" {
		t.Fatal("Se vinculó un gasto por más de su importe")
	}
	if errorCode(t, fixture.link(t, fixture.localLinker, r850, fixture.expenses["e500"], "1", 409)) != "CASH_EXPENSE_OVERLINKED" {
		t.Fatal("Se vinculó el gasto de $500 por más de su importe")
	}
	if errorCode(t, fixture.link(t, fixture.localLinker, r850, fixture.expenses["e200"], "1", 409)) != "RECEIPT_OVERLINKED" {
		t.Fatal("Se vinculó una recepción por más de su total")
	}
	if errorCode(t, fixture.link(t, fixture.localLinker, first, fixture.expenses["e500"], "1", 409)) != "CASH_LINK_EXISTS" {
		t.Fatal("Se duplicó el vínculo entre la misma recepción y el mismo gasto")
	}
	// Un gasto corregido no se vincula; una corrección tampoco (no es gasto).
	if errorCode(t, fixture.link(t, fixture.localLinker, first, fixture.expenses["e300"], "300", 409)) != "CASH_EXPENSE_CORRECTED" {
		t.Fatal("Se vinculó un gasto corregido")
	}
	if errorCode(t, fixture.link(t, fixture.localLinker, first, e300Correction.ID, "300", 409)) != "CASH_OPERATION_NOT_EXPENSE" {
		t.Fatal("Se vinculó una corrección de caja")
	}
	fixture.link(t, fixture.localLinker, first, fixture.expenses["e200"], "0", 400)
	// Corrección posterior detectada: el gasto de $200 vinculado y después corregido.
	fixture.link(t, fixture.localLinker, first, fixture.expenses["e200"], "200", 200)
	fixture.correct(t, shift.ID, "e200")
	view := fixture.receipt(t, fixture.localLinker, first)
	corrected := linkOf(t, view, fixture.expenses["e200"], true)
	if corrected.Status != "expense_corrected" || !view.CashLinkAttention || corrected.Expense.Status != "corrected" {
		t.Fatalf("La corrección posterior no se detectó: %+v", corrected)
	}
	var amount, kind string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT amount::text,kind FROM app.cash_operations WHERE id=$1", fixture.expenses["e200"]).
		Scan(&amount, &kind); err != nil || amount != "200.00" || kind != "expense" {
		t.Fatal("El vínculo modificó el gasto")
	}
	// Gastos candidatos: por importe y con lo vinculado a la recepción indicada.
	candidates := decodeResponse[inventory.CashExpensePage](t, fixture.call(t, fixture.localLinker, "GET",
		"inventory/cash-expenses?min_amount=850&max_amount=850", nil, 200))
	if candidates.Total != 1 || candidates.Items[0].ID != fixture.expenses["e850"] || candidates.Items[0].Linkable ||
		text(candidates.Items[0].UnavailableReason) != "fully_linked" || candidates.Items[0].AvailableAmount != "0.00" {
		t.Fatalf("Gasto candidato de $850: %+v", candidates.Items)
	}
	candidates = decodeResponse[inventory.CashExpensePage](t, fixture.call(t, fixture.localLinker, "GET",
		"inventory/cash-expenses?receipt_id="+first+"&register_id="+fixture.registerID, nil, 200))
	byID := map[string]inventory.CashExpense{}
	for _, expense := range candidates.Items {
		byID[expense.ID] = expense
	}
	if candidates.Total != 5 || text(byID[fixture.expenses["e500"]].LinkedToReceipt) != "500.00" ||
		text(byID[fixture.expenses["e300"]].UnavailableReason) != "corrected" || !byID[fixture.expenses["e150"]].Linkable ||
		byID[fixture.expenses["e150"]].AvailableAmount != "150.00" {
		t.Fatalf("Gastos candidatos de la caja: %+v", candidates.Items)
	}
	// Turno cerrado: el gasto de $150 queda en un turno cerrado; vincularlo exige cash.history.read y
	// no modifica el turno ni su cierre.
	fixture.shiftAction(t, "POST", shift.ID, "closing", map[string]any{}, 200)
	closing := decodeResponse[payments.ClosingView](t, fixture.shiftAction(t, "GET", shift.ID, "closing", nil, 200))
	fixture.shiftAction(t, "PUT", shift.ID, "closing/count", map[string]any{
		"revision": closing.Count.Revision, "lines": []map[string]any{}, "extra_mxn": "3500", "extra_note": "Conteo de la caja de compras",
		"notes": "",
	}, 200)
	closing = decodeResponse[payments.ClosingView](t, fixture.shiftAction(t, "GET", shift.ID, "closing", nil, 200))
	fixture.shiftAction(t, "POST", shift.ID, "closing/confirm", map[string]any{"count_revision": closing.Count.Revision}, 200)
	snapshot := func() string {
		var state string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT (SELECT to_jsonb(s)::text FROM app.cash_shifts s WHERE s.id=$1)||
            (SELECT to_jsonb(c)::text FROM app.cash_shift_closures c WHERE c.shift_id=$1)||
            (SELECT string_agg(to_jsonb(o)::text,'|' ORDER BY o.id) FROM app.cash_operations o WHERE o.shift_id=$1)`, shift.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	closedBefore := snapshot()
	fixture.link(t, fixture.localLinker, first, fixture.expenses["e150"], "150", 403)
	visible := decodeResponse[inventory.CashExpensePage](t, fixture.call(t, fixture.localLinker, "GET", "inventory/cash-expenses", nil, 200))
	for _, expense := range visible.Items {
		if expense.ShiftID == shift.ID {
			t.Fatal("Sin cash.history.read se listaron gastos de un turno cerrado")
		}
	}
	if decodeResponse[inventory.CashExpensePage](t, fixture.call(t, fixture.historyLinker, "GET", "inventory/cash-expenses", nil, 200)).Total != 5 {
		t.Fatal("Con cash.history.read deben verse los gastos del turno cerrado")
	}
	closedLink := decodeResponse[inventory.Receipt](t, fixture.link(t, fixture.historyLinker, first, fixture.expenses["e150"], "150", 200))
	if closedLink.LinkedAmount != "850.00" || linkOf(t, closedLink, fixture.expenses["e150"], true).Status != "active" {
		t.Fatalf("Vínculo con un gasto de un turno cerrado: %+v", closedLink.ReceiptSummary)
	}
	// Sin cash.history.read el vínculo con el turno cerrado se ve, pero sin los datos del gasto.
	hidden := linkOf(t, fixture.receipt(t, fixture.localLinker, first), fixture.expenses["e150"], true)
	if hidden.ExpenseVisible || hidden.Expense != nil || hidden.Amount != "150.00" {
		t.Fatalf("Datos de un gasto de turno cerrado sin permiso: %+v", hidden)
	}
	if snapshot() != closedBefore {
		t.Fatal("Vincular modificó el turno cerrado, su cierre o sus operaciones")
	}
	// Regularización posterior en el turno vigente de la misma caja: el vínculo la detecta y el cierre no cambia.
	reopened := decodeResponse[payments.Shift](t, fixture.call(t, fixture.cashier, "POST", "payments/shifts", map[string]any{
		"operation_id": randomID(), "opening_float": "0",
	}, 200))
	fixture.shifts["second"] = reopened.ID
	regularization := fixture.correct(t, reopened.ID, "e150")
	if !regularization.Regularization {
		t.Fatalf("La corrección de un turno cerrado debía ser regularización: %+v", regularization)
	}
	regularized := linkOf(t, fixture.receipt(t, fixture.historyLinker, first), fixture.expenses["e150"], true)
	if regularized.Status != "expense_regularized" || regularized.Expense.Status != "regularized" {
		t.Fatalf("La regularización posterior no se detectó: %+v", regularized)
	}
	var closure string
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT to_jsonb(c)::text FROM app.cash_shift_closures c WHERE c.shift_id=$1", shift.ID).
		Scan(&closure); err != nil || !containsAll(closedBefore, closure) {
		t.Fatal("La regularización o el vínculo cambiaron el cierre del turno")
	}
	// Un gasto en dólares se rechaza con un mensaje claro (se inserta directamente: el pago en dólares
	// es una función activable ajena a esta prueba).
	usd := randomID()
	operation := randomID()
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT $1,u.id,$2,'POST payments/shifts/expenses','prueba','{}' FROM app.users u WHERE u.username='compras_cajero'`,
		operation, fixture.cashier.deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO app.cash_operations(id,shift_id,register_id,kind,currency,amount,direction,concept,
            category_id,usd_rate,actor_id,device_id,authorized_by,authorization_mode,operation_id)
        SELECT $1,s.id,s.register_id,'expense','USD',20,'out','Insumos',$2,18.50,u.id,$3,u.id,'self',$4
        FROM app.cash_shifts s, app.users u WHERE s.id=$5 AND u.username='compras_cajero'`,
		usd, fixture.category, fixture.cashier.deviceID, operation, reopened.ID); err != nil {
		t.Fatal(err)
	}
	if errorCode(t, fixture.link(t, fixture.localLinker, first, usd, "20", 409)) != "CASH_EXPENSE_CURRENCY" {
		t.Fatal("Se vinculó un gasto en dólares")
	}
	// Desvincular es un registro con motivo, no un borrado; después se puede vincular otra vez. El gasto
	// de $500 quedó en el turno cerrado: desvincularlo exige, como vincularlo, poder leerlo
	// (cash.history.read).
	link := linkOf(t, fixture.receipt(t, fixture.historyLinker, first), fixture.expenses["e500"], true)
	reason := map[string]any{"reason": "Se vinculó a la recepción equivocada"}
	fixture.write(t, fixture.historyLinker, "POST", "inventory/cash-links/"+link.ID+"/unlink", map[string]any{"reason": "No"}, 400)
	fixture.write(t, fixture.noCashReader, "POST", "inventory/cash-links/"+link.ID+"/unlink", reason, 403)
	fixture.write(t, fixture.localLinker, "POST", "inventory/cash-links/"+link.ID+"/unlink", reason, 403)
	unlinked := decodeResponse[inventory.Receipt](t, fixture.write(t, fixture.historyLinker, "POST",
		"inventory/cash-links/"+link.ID+"/unlink", reason, 200))
	removed := linkOf(t, unlinked, fixture.expenses["e500"], false)
	if removed.Status != "unlinked" || text(removed.UnlinkReason) != "Se vinculó a la recepción equivocada" || removed.UnlinkedAt == nil ||
		unlinked.LinkedAmount != "350.00" {
		t.Fatalf("Desvinculación: %+v %s", removed, unlinked.LinkedAmount)
	}
	if errorCode(t, fixture.write(t, fixture.historyLinker, "POST", "inventory/cash-links/"+link.ID+"/unlink", reason, 409)) !=
		"CASH_LINK_INACTIVE" {
		t.Fatal("Se desvinculó dos veces")
	}
	relinked := decodeResponse[inventory.Receipt](t, fixture.link(t, fixture.historyLinker, first, fixture.expenses["e500"], "500", 200))
	if relinked.LinkedAmount != "850.00" || fixture.count(t, "SELECT count(*) FROM app.inventory_receipt_cash_links") != 5 {
		t.Fatalf("Vincular de nuevo: %s", relinked.LinkedAmount)
	}
	for _, statement := range []string{
		"UPDATE app.inventory_receipt_cash_links SET amount=1",
		"DELETE FROM app.inventory_receipt_cash_links",
		"UPDATE app.inventory_receipt_cash_links SET unlinked_at=NULL,unlinked_by=NULL,unlink_reason=NULL,unlink_operation_id=NULL " +
			"WHERE unlinked_at IS NOT NULL",
	} {
		if _, err := fixture.pool.Exec(fixture.ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s", statement)
		}
	}
	// Las únicas operaciones de caja nuevas son las de la caja (5 gastos, 3 correcciones y el gasto en
	// dólares): el inventario nunca crea gastos.
	if fixture.count(t, "SELECT count(*) FROM app.cash_operations") != cashBefore+9 {
		t.Fatal("El inventario creó o borró operaciones de caja")
	}
	if fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action='inventory.cash_link_created'") != 5 ||
		fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action='inventory.cash_link_removed'") != 1 {
		t.Fatal("Los vínculos y desvinculaciones deben auditarse")
	}
}

func testSupplierDeactivationAndAudit(t *testing.T, fixture *purchasingFixture) {
	cursor := eventCursor(t, fixture.restaurantFixture)
	pending := fixture.issueOrder(t, orderLine(fixture.items["refresco"], "1", "250", "", 16, fixture.presentations["caja"], ""))
	main := decodeResponse[inventory.SupplierDetail](t, fixture.call(t, fixture.buyer, "GET", "inventory/suppliers/"+fixture.suppliers["main"].ID, nil, 200))
	// Desactivar un proveedor con compras: se conserva con su historial; no admite compras nuevas, pero
	// la mercancía de una orden ya emitida se puede recibir.
	deactivated := decodeResponse[inventory.SupplierDetail](t, fixture.write(t, fixture.supplierClerk, "PATCH", "inventory/suppliers/"+main.ID,
		map[string]any{
			"revision": main.Revision, "trade_name": main.TradeName, "legal_name": main.LegalName, "rfc": main.RFC, "phone": main.Phone,
			"email": main.Email, "address": main.Address, "contact": main.Contact, "notes": main.Notes, "active": false,
		}, 200))
	if deactivated.Active || !deactivated.HasPurchases || deactivated.History.OrderCount < 5 || len(deactivated.History.Receipts) == 0 {
		t.Fatalf("Proveedor desactivado: %+v %+v", deactivated.Supplier, deactivated.History)
	}
	if errorCode(t, fixture.write(t, fixture.buyer, "POST", "inventory/purchase-orders", map[string]any{
		"supplier_id": main.ID, "lines": []map[string]any{},
	}, 409)) != "SUPPLIER_INACTIVE" {
		t.Fatal("Se creó una orden con un proveedor desactivado")
	}
	fixture.receive(t, fixture.buyer, map[string]any{"supplier_id": main.ID,
		"lines": []map[string]any{orderLine(fixture.items["vaso"], "1", "2", "", 16, "", "")}}, 409)
	fixture.receive(t, fixture.receiver, map[string]any{"purchase_order_id": pending.ID,
		"lines": []map[string]any{{"order_line_no": 1, "quantity": "1"}}}, 200)
	fixture.call(t, fixture.supplierClerk, "DELETE", "inventory/suppliers/"+main.ID, map[string]any{"operation_id": randomID()}, 404)
	if fixture.count(t, "SELECT count(*) FROM app.inventory_suppliers WHERE id=$1", main.ID) != 1 {
		t.Fatal("El proveedor con compras debe conservarse")
	}
	// Eventos sin contenido en el ámbito de inventario para quien lo consulta.
	kinds := eventKinds(fixture.inventoryEvents(t, fixture.buyer, cursor))
	for _, kind := range []string{"inventory.purchases.changed", "inventory.receipts.changed", "inventory.stock.changed", "inventory.movements.recorded"} {
		found := false
		for _, received := range kinds {
			found = found || received == kind
		}
		if !found {
			t.Fatalf("Falta el evento %s: %v", kind, kinds)
		}
	}
	for action, minimum := range map[string]int{
		"inventory.purchase_order_created": 6, "inventory.purchase_order_updated": 1, "inventory.purchase_order_issued": 5,
		"inventory.purchase_order_cancelled": 2, "inventory.purchase_order_closed": 1, "inventory.receipt_recorded": 12,
		"inventory.supplier_return_recorded": 2, "inventory.supplier_deactivated": 1,
	} {
		if count := fixture.count(t, "SELECT count(*) FROM app.audit_events WHERE action=$1", action); count < minimum {
			t.Fatalf("Auditoría %s: %d (mínimo %d)", action, count, minimum)
		}
	}
	checkLedgerConsistency(t, fixture.inventoryFixture)
}
