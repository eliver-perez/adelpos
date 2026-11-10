//go:build integration

package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// purchasingConstraints son las restricciones con nombre explícito de la 0013 (etapa 8, fase C).
var purchasingConstraints = []string{
	"inventory_suppliers_trade_name_check", "inventory_suppliers_legal_name_check", "inventory_suppliers_rfc_check",
	"inventory_suppliers_phone_check", "inventory_suppliers_email_check", "inventory_suppliers_address_check",
	"inventory_suppliers_contact_check", "inventory_suppliers_notes_check", "inventory_suppliers_revision_check",
	"inventory_suppliers_created_by_fkey", "inventory_suppliers_updated_by_fkey",
	"inventory_supplier_items_supplier_id_fkey", "inventory_supplier_items_item_id_fkey",
	"inventory_supplier_items_presentation_id_fkey", "inventory_supplier_items_supplier_code_check",
	"inventory_supplier_items_last_unit_price_check", "inventory_supplier_items_last_price_presentation_id_fkey",
	"inventory_supplier_items_last_price_unit_id_fkey", "inventory_supplier_items_revision_check",
	"inventory_supplier_items_updated_by_fkey", "inventory_supplier_items_pkey",
	"inventory_supplier_items_last_price_check", "inventory_purchase_orders_supplier_id_fkey",
	"inventory_purchase_orders_state_check", "inventory_purchase_orders_expected_warehouse_id_fkey",
	"inventory_purchase_orders_notes_check", "inventory_purchase_orders_subtotal_check",
	"inventory_purchase_orders_discount_check", "inventory_purchase_orders_tax_check",
	"inventory_purchase_orders_line_count_check", "inventory_purchase_orders_created_by_fkey",
	"inventory_purchase_orders_updated_by_fkey", "inventory_purchase_orders_issued_by_fkey",
	"inventory_purchase_orders_cancelled_by_fkey", "inventory_purchase_orders_closed_by_fkey",
	"inventory_purchase_orders_revision_check", "inventory_purchase_orders_operation_id_fkey",
	"inventory_purchase_orders_folio_key", "inventory_purchase_orders_operation_id_key",
	"inventory_purchase_orders_total_check", "inventory_purchase_orders_issue_check",
	"inventory_purchase_orders_cancel_check", "inventory_purchase_orders_close_check",
	"inventory_purchase_order_lines_order_id_fkey", "inventory_purchase_order_lines_line_no_check",
	"inventory_purchase_order_lines_item_id_fkey", "inventory_purchase_order_lines_presentation_id_fkey",
	"inventory_purchase_order_lines_unit_id_fkey", "inventory_purchase_order_lines_quantity_check",
	"inventory_purchase_order_lines_base_quantity_check", "inventory_purchase_order_lines_unit_price_check",
	"inventory_purchase_order_lines_discount_check", "inventory_purchase_order_lines_tax_rate_check",
	"inventory_purchase_order_lines_subtotal_check", "inventory_purchase_order_lines_tax_check",
	"inventory_purchase_order_lines_pkey", "inventory_purchase_order_lines_capture_check",
	"inventory_purchase_order_lines_total_check", "inventory_receipts_supplier_id_fkey",
	"inventory_receipts_purchase_order_id_fkey", "inventory_receipts_warehouse_id_fkey",
	"inventory_receipts_received_by_fkey", "inventory_receipts_supplier_reference_check",
	"inventory_receipts_subtotal_check", "inventory_receipts_discount_check", "inventory_receipts_tax_check",
	"inventory_receipts_inventory_value_check", "inventory_receipts_line_count_check", "inventory_receipts_notes_check",
	"inventory_receipts_operation_id_fkey", "inventory_receipts_folio_key", "inventory_receipts_operation_id_key",
	"inventory_receipts_order_key", "inventory_receipts_total_check", "inventory_receipts_value_check",
	"inventory_receipts_excess_check", "inventory_supplier_items_last_receipt_id_fkey",
	"inventory_receipt_lines_receipt_id_fkey", "inventory_receipt_lines_line_no_check",
	"inventory_receipt_lines_item_id_fkey", "inventory_receipt_lines_presentation_id_fkey",
	"inventory_receipt_lines_unit_id_fkey", "inventory_receipt_lines_quantity_check",
	"inventory_receipt_lines_base_quantity_check", "inventory_receipt_lines_unit_price_check",
	"inventory_receipt_lines_discount_check", "inventory_receipt_lines_tax_rate_check",
	"inventory_receipt_lines_subtotal_check", "inventory_receipt_lines_tax_check",
	"inventory_receipt_lines_inventory_value_check", "inventory_receipt_lines_unit_cost_check",
	"inventory_receipt_lines_movement_id_fkey", "inventory_receipt_lines_pkey",
	"inventory_receipt_lines_movement_id_key", "inventory_receipt_lines_receipt_order_fkey",
	"inventory_receipt_lines_order_line_fkey", "inventory_receipt_lines_order_check",
	"inventory_receipt_lines_capture_check", "inventory_receipt_lines_total_check",
	"inventory_supplier_returns_receipt_id_fkey", "inventory_supplier_returns_supplier_id_fkey",
	"inventory_supplier_returns_warehouse_id_fkey", "inventory_supplier_returns_reason_check",
	"inventory_supplier_returns_actor_id_fkey", "inventory_supplier_returns_value_check",
	"inventory_supplier_returns_line_count_check", "inventory_supplier_returns_operation_id_fkey",
	"inventory_supplier_returns_folio_key", "inventory_supplier_returns_operation_id_key",
	"inventory_supplier_returns_receipt_key", "inventory_supplier_return_lines_return_id_fkey",
	"inventory_supplier_return_lines_line_no_check", "inventory_supplier_return_lines_item_id_fkey",
	"inventory_supplier_return_lines_quantity_check", "inventory_supplier_return_lines_base_quantity_check",
	"inventory_supplier_return_lines_original_unit_cost_check", "inventory_supplier_return_lines_value_check",
	"inventory_supplier_return_lines_outflow_value_check", "inventory_supplier_return_lines_movement_id_fkey",
	"inventory_supplier_return_lines_adjustment_movement_id_fkey", "inventory_supplier_return_lines_pkey",
	"inventory_supplier_return_lines_movement_id_key", "inventory_supplier_return_lines_receipt_line_key",
	"inventory_supplier_return_lines_return_receipt_fkey", "inventory_supplier_return_lines_receipt_line_fkey",
	"inventory_supplier_return_lines_adjustment_check", "inventory_supplier_return_lines_difference_check",
	"inventory_receipt_cash_links_receipt_id_fkey", "inventory_receipt_cash_links_cash_operation_id_fkey",
	"inventory_receipt_cash_links_amount_check", "inventory_receipt_cash_links_notes_check",
	"inventory_receipt_cash_links_linked_by_fkey", "inventory_receipt_cash_links_operation_id_fkey",
	"inventory_receipt_cash_links_unlinked_by_fkey", "inventory_receipt_cash_links_unlink_operation_id_fkey",
	"inventory_receipt_cash_links_operation_id_key", "inventory_receipt_cash_links_unlink_operation_id_key",
	"inventory_receipt_cash_links_unlink_check",
}

// checkStage8PhaseC comprueba la 0013 sobre la instalación simulada en esquema 9: sin siembras,
// permisos de la fase C solo del administrador, restricciones con nombre, CHECK seguros ante NULL
// (con tablas temporales) y disparadores de los documentos (renglones de una orden emitida,
// recepciones de solo inserción y vínculos con gastos que solo se desvinculan).
func checkStage8PhaseC(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named, rows, granted, others int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname = ANY($1)", purchasingConstraints).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != len(purchasingConstraints) {
		t.Fatalf("Faltan restricciones con nombre explícito en la 0013: %d de %d", named, len(purchasingConstraints))
	}
	keys := []string{
		"inventory.suppliers.manage", "inventory.purchases.create", "inventory.purchases.authorize", "inventory.purchases.receive",
		"inventory.supplier_returns.record", "inventory.purchases.link_cash",
	}
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.inventory_suppliers)+(SELECT count(*) FROM app.inventory_supplier_items)
            +(SELECT count(*) FROM app.inventory_purchase_orders)+(SELECT count(*) FROM app.inventory_purchase_order_lines)
            +(SELECT count(*) FROM app.inventory_receipts)+(SELECT count(*) FROM app.inventory_receipt_lines)
            +(SELECT count(*) FROM app.inventory_supplier_returns)+(SELECT count(*) FROM app.inventory_supplier_return_lines)
            +(SELECT count(*) FROM app.inventory_receipt_cash_links),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='administrator'
            AND p.permission_key = ANY($1)),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code<>'administrator'
            AND p.permission_key = ANY($1))`, keys).Scan(&rows, &granted, &others)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 || granted != 6 || others != 0 {
		t.Fatalf("La 0013 no debe sembrar datos y solo concede al administrador: filas %d, administrador %d, otros %d", rows, granted, others)
	}
	checkPurchasingChecks(ctx, t, pool)
	checkPurchasingTriggers(ctx, t, pool)
}

// checkPurchasingChecks prueba los CHECK de la 0013 con copias temporales de las tablas.
func checkPurchasingChecks(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	copies := map[string]string{
		"suppliers_check": "inventory_suppliers", "supplier_items_check": "inventory_supplier_items",
		"orders_check": "inventory_purchase_orders", "order_lines_check": "inventory_purchase_order_lines",
		"receipts_check": "inventory_receipts", "return_lines_check": "inventory_supplier_return_lines",
		"links_check": "inventory_receipt_cash_links",
	}
	for copy, table := range copies {
		statement := "CREATE TEMP TABLE " + copy + " (LIKE app." + table + " INCLUDING CONSTRAINTS INCLUDING DEFAULTS INCLUDING IDENTITY)"
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	fill := func(template, values string) string {
		return strings.Replace(template, "%s", values, 1)
	}
	uuid := "gen_random_uuid()"
	supplier := "INSERT INTO suppliers_check(id,trade_name,rfc) VALUES (gen_random_uuid(),%s)"
	item := "INSERT INTO supplier_items_check(supplier_id,item_id,last_unit_price,last_price_at,last_receipt_id,last_price_presentation_id," +
		"last_price_unit_id) VALUES (gen_random_uuid(),gen_random_uuid(),%s)"
	order := "INSERT INTO orders_check(id,supplier_id,order_date,expected_warehouse_id,created_by,updated_by,operation_id,state,issued_by," +
		"issued_at,cancelled_by,cancelled_at,cancel_reason,closed_by,closed_at,close_reason,subtotal,tax,total) VALUES (gen_random_uuid()," +
		"gen_random_uuid(),current_date,gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s)"
	line := "INSERT INTO order_lines_check(order_id,line_no,item_id,presentation_id,unit_id,quantity,base_quantity,unit_price,discount," +
		"tax_rate,subtotal,tax,total) VALUES (gen_random_uuid(),1,gen_random_uuid(),%s)"
	receipt := "INSERT INTO receipts_check(id,supplier_id,purchase_order_id,warehouse_id,occurred_at,received_by,tax_in_cost,subtotal," +
		"discount,tax,total,inventory_value,line_count,excess,operation_id) VALUES (gen_random_uuid(),gen_random_uuid(),%s," +
		"gen_random_uuid())"
	returned := "INSERT INTO return_lines_check(return_id,line_no,receipt_id,receipt_line_no,item_id,quantity,base_quantity," +
		"original_unit_cost,value,outflow_value,cost_adjustment,unabsorbed,movement_id,cost_adjustment_movement_id) VALUES " +
		"(gen_random_uuid(),1,gen_random_uuid(),1,gen_random_uuid(),10,10,%s)"
	link := "INSERT INTO links_check(id,receipt_id,cash_operation_id,amount,linked_by,operation_id,unlinked_by,unlinked_at,unlink_reason," +
		"unlink_operation_id) VALUES (gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s)"
	accepted := []string{
		fill(supplier, "'Abarrotes',NULL"), fill(supplier, "'Abarrotes','ABC010203XY9'"), fill(supplier, "'Abarrotes','GODE561231GR8'"),
		fill(item, "NULL,NULL,NULL,NULL,NULL"), fill(item, "250,now(),"+uuid+","+uuid+",NULL"), fill(item, "0.05,now(),"+uuid+",NULL,"+uuid),
		fill(order, "'draft',NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,0,0"),
		fill(order, "'issued',"+uuid+",now(),NULL,NULL,NULL,NULL,NULL,NULL,100,16,116"),
		fill(order, "'cancelled',NULL,NULL,"+uuid+",now(),'Pedido duplicado',NULL,NULL,NULL,0,0,0"),
		fill(order, "'cancelled',"+uuid+",now(),"+uuid+",now(),'Ya no se necesita',NULL,NULL,NULL,0,0,0"),
		fill(order, "'closed',"+uuid+",now(),NULL,NULL,NULL,"+uuid+",now(),'El proveedor no surtirá',100,16,116"),
		fill(line, uuid+",NULL,20,480,250,50,16,4950,792,5742"), fill(line, "NULL,"+uuid+",5,5000,45,0,8,225,18,243"),
		fill(receipt, "NULL,gen_random_uuid(),now(),gen_random_uuid(),true,100,0,16,116,116,1,false"),
		fill(receipt, uuid+",gen_random_uuid(),now(),gen_random_uuid(),false,100,0,16,116,100,1,true"),
		fill(returned, "3.48,34.80,29.00,-5.80,0,"+uuid+","+uuid), fill(returned, "3.48,17.40,14.347368,0,3.052632,"+uuid+",NULL"),
		fill(link, "850,"+uuid+","+uuid+",NULL,NULL,NULL,NULL"), fill(link, "850,"+uuid+","+uuid+","+uuid+",now(),'Recepción equivocada',"+uuid),
	}
	rejected := []string{
		// RFC en minúsculas o sin homoclave; nombre con espacios o vacío.
		fill(supplier, "'Abarrotes','abc010203xy9'"), fill(supplier, "'Abarrotes','ABC010203'"), fill(supplier, "' Abarrotes',NULL"),
		fill(supplier, "'',NULL"),
		// Último precio a medias, con dos unidades de captura o negativo.
		fill(item, "250,NULL,NULL,NULL,"+uuid), fill(item, "250,now(),"+uuid+","+uuid+","+uuid), fill(item, "250,now(),"+uuid+",NULL,NULL"),
		fill(item, "-1,now(),"+uuid+",NULL,"+uuid),
		// Estados incoherentes (incluidos los NULL en la rama que valida) y total que no cuadra.
		fill(order, "'draft',"+uuid+",now(),NULL,NULL,NULL,NULL,NULL,NULL,0,0,0"),
		fill(order, "'issued',"+uuid+",NULL,NULL,NULL,NULL,NULL,NULL,NULL,0,0,0"),
		fill(order, "'cancelled',NULL,NULL,"+uuid+",now(),NULL,NULL,NULL,NULL,0,0,0"),
		fill(order, "'cancelled',NULL,NULL,"+uuid+",now(),'No',NULL,NULL,NULL,0,0,0"),
		fill(order, "'cancelled',"+uuid+",NULL,"+uuid+",now(),'Pedido duplicado',NULL,NULL,NULL,0,0,0"),
		fill(order, "'closed',NULL,NULL,NULL,NULL,NULL,"+uuid+",now(),'El proveedor no surtirá',0,0,0"),
		fill(order, "'closed',"+uuid+",now(),NULL,NULL,NULL,"+uuid+",NULL,'El proveedor no surtirá',0,0,0"),
		fill(order, "'received',"+uuid+",now(),"+uuid+",now(),'Pedido duplicado',NULL,NULL,NULL,0,0,0"),
		fill(order, "'issued',"+uuid+",now(),NULL,NULL,NULL,NULL,NULL,NULL,100,16,117"),
		fill(order, "'paid',"+uuid+",now(),NULL,NULL,NULL,NULL,NULL,NULL,0,0,0"),
		// Renglón con dos o ninguna unidad de captura, tasa fuera de la lista, total que no cuadra,
		// cantidad cero o descuento negativo.
		fill(line, uuid+","+uuid+",20,480,250,50,16,4950,792,5742"), fill(line, "NULL,NULL,20,480,250,50,16,4950,792,5742"),
		fill(line, uuid+",NULL,20,480,250,50,15,4950,742.50,5692.50"), fill(line, uuid+",NULL,20,480,250,50,16,4950,792,5743"),
		fill(line, uuid+",NULL,0,0,250,0,16,0,0,0"), fill(line, uuid+",NULL,20,480,250,-1,16,5001,800.16,5801.16"),
		// Valor de inventario distinto del que fija tax_in_cost, exceso en una recepción directa, total y renglones.
		fill(receipt, "NULL,gen_random_uuid(),now(),gen_random_uuid(),true,100,0,16,116,100,1,false"),
		fill(receipt, "NULL,gen_random_uuid(),now(),gen_random_uuid(),false,100,0,16,116,116,1,false"),
		fill(receipt, "NULL,gen_random_uuid(),now(),gen_random_uuid(),true,100,0,16,116,116,1,true"),
		fill(receipt, uuid+",gen_random_uuid(),now(),gen_random_uuid(),true,100,0,16,117,117,1,false"),
		fill(receipt, uuid+",gen_random_uuid(),now(),gen_random_uuid(),true,100,0,16,116,116,0,false"),
		// Ajuste de costo sin movimiento, movimiento sin ajuste y diferencia que no cuadra.
		fill(returned, "3.48,34.80,29.00,-5.80,0,"+uuid+",NULL"), fill(returned, "3.48,17.40,14.347368,0,3.052632,"+uuid+","+uuid),
		fill(returned, "3.48,34.80,29.00,-5.00,0,"+uuid+","+uuid),
		// Vínculo sin importe o desvinculación a medias o sin motivo suficiente.
		fill(link, "0,"+uuid+","+uuid+",NULL,NULL,NULL,NULL"), fill(link, "850,"+uuid+","+uuid+","+uuid+",now(),NULL,"+uuid),
		fill(link, "850,"+uuid+","+uuid+","+uuid+",now(),'No',"+uuid),
		fill(link, "850,"+uuid+","+uuid+","+uuid+",NULL,'Recepción equivocada',"+uuid),
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido de la 0013 (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente de la 0013: %s", statement)
		}
	}
	for copy := range copies {
		if _, err := connection.Exec(ctx, "DROP TABLE "+copy); err != nil {
			t.Fatal(err)
		}
	}
}

// checkPurchasingTriggers prueba los disparadores de la 0013 sobre las tablas reales con datos
// mínimos (el insumo de prueba de la 0010 y las operaciones de la instalación simulada).
func checkPurchasingTriggers(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	setup := `INSERT INTO app.inventory_suppliers(id,trade_name) VALUES ('00000000-0000-4000-8000-0000000000c1','Proveedor de prueba');
        INSERT INTO app.inventory_purchase_orders(id,supplier_id,order_date,expected_warehouse_id,created_by,updated_by,operation_id)
            SELECT '00000000-0000-4000-8000-0000000000c2','00000000-0000-4000-8000-0000000000c1',current_date,w.id,u.id,u.id,
                '00000000-0000-4000-8000-000000000091'
            FROM app.inventory_warehouses w, app.users u WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_purchase_order_lines(order_id,line_no,item_id,unit_id,quantity,base_quantity,unit_price,tax_rate,subtotal,
            tax,total)
            SELECT '00000000-0000-4000-8000-0000000000c2',1,'00000000-0000-4000-8000-0000000000a0',id,2,2,10,16,20,3.20,23.20
            FROM app.inventory_units WHERE code='kg';
        UPDATE app.inventory_purchase_orders SET state='issued',issued_by=created_by,issued_at=now()
            WHERE id='00000000-0000-4000-8000-0000000000c2';
        INSERT INTO app.inventory_receipts(id,supplier_id,warehouse_id,occurred_at,received_by,tax_in_cost,subtotal,discount,tax,total,
            inventory_value,line_count,operation_id)
            SELECT '00000000-0000-4000-8000-0000000000c3','00000000-0000-4000-8000-0000000000c1',w.id,now(),u.id,true,20,0,3.20,23.20,
                23.20,1,'00000000-0000-4000-8000-000000000092'
            FROM app.inventory_warehouses w, app.users u WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_receipt_cash_links(id,receipt_id,cash_operation_id,amount,linked_by,operation_id)
            SELECT '00000000-0000-4000-8000-0000000000c4','00000000-0000-4000-8000-0000000000c3','00000000-0000-4000-8000-000000000097',
                23.20,id,'00000000-0000-4000-8000-000000000093' FROM app.users WHERE username='cajera'`
	if _, err := pool.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		// Renglones de una orden emitida.
		"UPDATE app.inventory_purchase_order_lines SET quantity=3",
		"DELETE FROM app.inventory_purchase_order_lines",
		`INSERT INTO app.inventory_purchase_order_lines(order_id,line_no,item_id,unit_id,quantity,base_quantity,unit_price,tax_rate,subtotal,
            tax,total) SELECT '00000000-0000-4000-8000-0000000000c2',2,'00000000-0000-4000-8000-0000000000a0',id,1,1,1,0,1,0,1
            FROM app.inventory_units WHERE code='kg'`,
		"TRUNCATE app.inventory_purchase_order_lines",
		// Recepciones de solo inserción.
		"UPDATE app.inventory_receipts SET total=0,subtotal=0,tax=0,inventory_value=0",
		"DELETE FROM app.inventory_receipts",
		// Un vínculo no se modifica ni se borra.
		"UPDATE app.inventory_receipt_cash_links SET amount=1",
		"DELETE FROM app.inventory_receipt_cash_links",
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó %s", statement)
		}
	}
	// Desvincular es la única actualización admitida, y solo una vez.
	unlink := `UPDATE app.inventory_receipt_cash_links SET unlinked_by=linked_by,unlinked_at=now(),unlink_reason='Vínculo de prueba',
        unlink_operation_id='00000000-0000-4000-8000-000000000094' WHERE id='00000000-0000-4000-8000-0000000000c4'`
	if _, err := pool.Exec(ctx, unlink); err != nil {
		t.Fatalf("PostgreSQL rechazó la desvinculación: %v", err)
	}
	if _, err := pool.Exec(ctx, unlink); err == nil {
		t.Fatal("PostgreSQL aceptó desvincular dos veces")
	}
	// Los gastos de caja no cambiaron.
	var operations string
	if err := pool.QueryRow(ctx, `SELECT string_agg(kind||':'||amount::text,'|' ORDER BY id) FROM app.cash_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != "expense:250.00|expense:180.50|correction:250.00" {
		t.Fatalf("Los vínculos alteraron los gastos de caja: %s", operations)
	}
}
