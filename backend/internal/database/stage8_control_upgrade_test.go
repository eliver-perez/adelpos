//go:build integration

package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// controlConstraints son las restricciones con nombre explícito de la 0014 (etapa 8, fase D).
var controlConstraints = []string{
	"inventory_adjustments_kind_check", "inventory_adjustments_reason_check", "inventory_adjustments_reason_detail_check",
	"inventory_adjustments_warehouse_id_fkey", "inventory_adjustments_state_check", "inventory_adjustments_notes_check",
	"inventory_adjustments_line_count_check", "inventory_adjustments_requested_by_fkey",
	"inventory_adjustments_authorized_by_fkey", "inventory_adjustments_rejected_by_fkey", "inventory_adjustments_revision_check",
	"inventory_adjustments_operation_id_fkey", "inventory_adjustments_decision_operation_id_fkey",
	"inventory_adjustments_folio_key", "inventory_adjustments_operation_id_key",
	"inventory_adjustments_decision_operation_id_key", "inventory_adjustments_other_check",
	"inventory_adjustments_waste_reason_check", "inventory_adjustments_state_shape_check",
	"inventory_adjustment_lines_adjustment_id_fkey", "inventory_adjustment_lines_line_no_check",
	"inventory_adjustment_lines_item_id_fkey", "inventory_adjustment_lines_presentation_id_fkey",
	"inventory_adjustment_lines_unit_id_fkey", "inventory_adjustment_lines_captured_quantity_check",
	"inventory_adjustment_lines_quantity_check", "inventory_adjustment_lines_movement_id_fkey", "inventory_adjustment_lines_pkey",
	"inventory_adjustment_lines_item_key", "inventory_adjustment_lines_movement_id_key",
	"inventory_adjustment_lines_capture_check", "inventory_adjustment_lines_sign_check",
	"inventory_transfers_from_warehouse_id_fkey", "inventory_transfers_to_warehouse_id_fkey", "inventory_transfers_actor_id_fkey",
	"inventory_transfers_notes_check", "inventory_transfers_line_count_check", "inventory_transfers_operation_id_fkey",
	"inventory_transfers_folio_key", "inventory_transfers_operation_id_key", "inventory_transfers_warehouses_check",
	"inventory_transfer_lines_transfer_id_fkey", "inventory_transfer_lines_line_no_check",
	"inventory_transfer_lines_item_id_fkey", "inventory_transfer_lines_presentation_id_fkey",
	"inventory_transfer_lines_unit_id_fkey", "inventory_transfer_lines_captured_quantity_check",
	"inventory_transfer_lines_quantity_check", "inventory_transfer_lines_out_movement_id_fkey",
	"inventory_transfer_lines_in_movement_id_fkey", "inventory_transfer_lines_pkey", "inventory_transfer_lines_item_key",
	"inventory_transfer_lines_out_movement_id_key", "inventory_transfer_lines_in_movement_id_key",
	"inventory_transfer_lines_capture_check", "inventory_transfer_lines_movements_check", "inventory_counts_warehouse_id_fkey",
	"inventory_counts_scope_check", "inventory_counts_category_id_fkey", "inventory_counts_state_check",
	"inventory_counts_notes_check", "inventory_counts_line_count_check", "inventory_counts_created_by_fkey",
	"inventory_counts_updated_by_fkey", "inventory_counts_submitted_by_fkey", "inventory_counts_approved_by_fkey",
	"inventory_counts_applied_by_fkey", "inventory_counts_cancelled_by_fkey", "inventory_counts_revision_check",
	"inventory_counts_operation_id_fkey", "inventory_counts_folio_key", "inventory_counts_operation_id_key",
	"inventory_counts_scope_category_check", "inventory_counts_submit_check", "inventory_counts_approve_check",
	"inventory_counts_apply_check", "inventory_counts_cancel_check", "inventory_count_lines_count_id_fkey",
	"inventory_count_lines_line_no_check", "inventory_count_lines_item_id_fkey", "inventory_count_lines_counted_quantity_check",
	"inventory_count_lines_captured_quantity_check", "inventory_count_lines_presentation_id_fkey",
	"inventory_count_lines_unit_id_fkey", "inventory_count_lines_captured_by_fkey", "inventory_count_lines_movement_id_fkey",
	"inventory_count_lines_pkey", "inventory_count_lines_item_key", "inventory_count_lines_movement_id_key",
	"inventory_count_lines_capture_check", "inventory_count_lines_difference_check", "inventory_count_lines_movement_check",
	"inventory_stock_levels_item_id_fkey", "inventory_stock_levels_warehouse_id_fkey", "inventory_stock_levels_min_stock_check",
	"inventory_stock_levels_reorder_point_check", "inventory_stock_levels_revision_check",
	"inventory_stock_levels_updated_by_fkey", "inventory_stock_levels_pkey", "inventory_stock_levels_values_check",
}

// checkStage8PhaseD comprueba la 0014 sobre la instalación simulada en esquema 9: sin siembras,
// permisos de la fase D solo del administrador, restricciones con nombre, CHECK seguros ante NULL
// (con tablas temporales) y disparadores de los documentos (ajustes que solo se autorizan o
// rechazan pendientes, mermas solo de salida, transferencias de solo inserción y conteos que solo se
// capturan abiertos).
func checkStage8PhaseD(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var named, rows, granted, others int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_constraint WHERE conname = ANY($1)", controlConstraints).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != len(controlConstraints) || len(controlConstraints) != 100 {
		t.Fatalf("Faltan restricciones con nombre explícito en la 0014: %d de %d", named, len(controlConstraints))
	}
	keys := []string{
		"inventory.adjustments.record", "inventory.counts.perform", "inventory.counts.approve", "inventory.transfers.record",
	}
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.inventory_adjustments)+(SELECT count(*) FROM app.inventory_adjustment_lines)
            +(SELECT count(*) FROM app.inventory_transfers)+(SELECT count(*) FROM app.inventory_transfer_lines)
            +(SELECT count(*) FROM app.inventory_counts)+(SELECT count(*) FROM app.inventory_count_lines)
            +(SELECT count(*) FROM app.inventory_stock_levels),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code='administrator'
            AND p.permission_key = ANY($1)),
        (SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id WHERE r.code<>'administrator'
            AND p.permission_key = ANY($1))`, keys).Scan(&rows, &granted, &others)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 || granted != 4 || others != 0 {
		t.Fatalf("La 0014 no debe sembrar datos y solo concede al administrador: filas %d, administrador %d, otros %d", rows, granted, others)
	}
	checkControlChecks(ctx, t, pool)
	checkControlTriggers(ctx, t, pool)
}

// checkControlChecks prueba los CHECK de la 0014 con copias temporales de las tablas.
func checkControlChecks(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	copies := map[string]string{
		"adjustments_check": "inventory_adjustments", "adjustment_lines_check": "inventory_adjustment_lines",
		"transfers_check": "inventory_transfers", "transfer_lines_check": "inventory_transfer_lines",
		"counts_check": "inventory_counts", "count_lines_check": "inventory_count_lines", "levels_check": "inventory_stock_levels",
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
	// Quien registra el ajuste (requester) y otra persona (other).
	requester := "'00000000-0000-4000-8000-0000000000d1'"
	other := "'00000000-0000-4000-8000-0000000000d2'"
	adjustment := "INSERT INTO adjustments_check(id,warehouse_id,line_count,requested_by,operation_id,kind,reason,reason_detail,state," +
		"authorized_by,authorized_at,applied_at,rejected_by,rejected_at,rejection_reason,decision_operation_id) VALUES (gen_random_uuid()," +
		"gen_random_uuid(),1," + requester + ",gen_random_uuid(),%s)"
	adjustmentLine := "INSERT INTO adjustment_lines_check(adjustment_id,line_no,item_id,presentation_id,unit_id,captured_quantity," +
		"quantity) VALUES (gen_random_uuid(),%s)"
	transfer := "INSERT INTO transfers_check(id,occurred_at,actor_id,operation_id,from_warehouse_id,to_warehouse_id,line_count) " +
		"VALUES (gen_random_uuid(),now(),gen_random_uuid(),gen_random_uuid(),%s)"
	transferLine := "INSERT INTO transfer_lines_check(transfer_id,item_id,line_no,presentation_id,unit_id,captured_quantity,quantity," +
		"out_movement_id,in_movement_id) VALUES (gen_random_uuid(),gen_random_uuid(),%s)"
	count := "INSERT INTO counts_check(id,warehouse_id,created_by,updated_by,operation_id,scope,category_id,state,line_count,submitted_by," +
		"submitted_at,approved_by,approved_at,applied_by,applied_at,reference_at,cancelled_by,cancelled_at,cancel_reason) VALUES " +
		"(gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),%s)"
	countLine := "INSERT INTO count_lines_check(count_id,line_no,item_id,counted_quantity,captured_quantity,presentation_id,unit_id," +
		"captured_at,captured_by,theoretical_quantity,difference,movement_id) VALUES (gen_random_uuid(),1,gen_random_uuid(),%s)"
	level := "INSERT INTO levels_check(item_id,warehouse_id,updated_by,min_stock,reorder_point) VALUES (gen_random_uuid(),gen_random_uuid()," +
		"gen_random_uuid(),%s)"
	none := "NULL,NULL,NULL,NULL,NULL,NULL,NULL"
	// captured es la hora y el autor de la captura de un renglón; actorAt, el autor y la hora de un
	// cambio de estado del conteo.
	captured := "now()," + uuid
	actorAt := uuid + ",now()"
	accepted := []string{
		// Pendiente, aplicado directamente, autorizado por otra persona, rechazado por otra persona, merma y «Otro» con texto.
		fill(adjustment, "'discrepancy','breakage','','pending_authorization',"+none),
		fill(adjustment, "'discrepancy','surplus','','applied',NULL,NULL,now(),NULL,NULL,NULL,NULL"),
		fill(adjustment, "'discrepancy','entry_error','','applied',"+other+",now(),now(),NULL,NULL,NULL,"+uuid),
		fill(adjustment, "'discrepancy','spill','','rejected',NULL,NULL,NULL,"+other+",now(),'No procede',"+uuid),
		fill(adjustment, "'waste','expiry','','applied',NULL,NULL,now(),NULL,NULL,NULL,NULL"),
		fill(adjustment, "'discrepancy','other','Cata de vinos','pending_authorization',"+none),
		fill(adjustmentLine, "1,"+uuid+",NULL,"+uuid+",-2,-2000"), fill(adjustmentLine, "1,"+uuid+","+uuid+",NULL,3,72"),
		fill(transfer, uuid+","+uuid+",1"),
		fill(transferLine, "1,NULL,"+uuid+",20,20,"+uuid+","+uuid), fill(transferLine, "1,"+uuid+",NULL,2,48,"+uuid+","+uuid),
		fill(count, "'all',NULL,'open',3,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'category',"+uuid+",'open',0,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'submitted',2,"+actorAt+",NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'approved',2,"+actorAt+","+actorAt+",NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'applied',2,"+actorAt+","+actorAt+","+actorAt+",now(),NULL,NULL,NULL"),
		fill(count, "'all',NULL,'cancelled',2,NULL,NULL,NULL,NULL,NULL,NULL,NULL,"+actorAt+",'Conteo duplicado'"),
		fill(count, "'all',NULL,'cancelled',2,"+actorAt+","+actorAt+",NULL,NULL,NULL,"+actorAt+",'Conteo duplicado'"),
		fill(countLine, "NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",NULL,NULL,NULL"),
		fill(countLine, "12,0.5,"+uuid+",NULL,"+captured+",15,-3,NULL"),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",15,-3,"+uuid),
		fill(countLine, "0,0,NULL,"+uuid+","+captured+",2,-2,"+uuid),
		fill(level, "5,NULL"), fill(level, "NULL,10"), fill(level, "5,10"),
	}
	rejected := []string{
		// «Otro» sin texto, motivo de merma no admitido, merma pendiente, autoautorización, decisión sin su
		// operación, aplicado sin hora, rechazo propio, motivo de rechazo corto o ausente, merma autorizada
		// por separado, pendiente con hora de aplicación, tipo desconocido y texto con espacios.
		fill(adjustment, "'discrepancy','other','','pending_authorization',"+none),
		fill(adjustment, "'waste','surplus','','applied',NULL,NULL,now(),NULL,NULL,NULL,NULL"),
		fill(adjustment, "'waste','spill','','pending_authorization',"+none),
		fill(adjustment, "'discrepancy','entry_error','','applied',"+requester+",now(),now(),NULL,NULL,NULL,"+uuid),
		fill(adjustment, "'discrepancy','entry_error','','applied',"+other+",now(),now(),NULL,NULL,NULL,NULL"),
		fill(adjustment, "'discrepancy','surplus','','applied',NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(adjustment, "'discrepancy','spill','','rejected',NULL,NULL,NULL,"+requester+",now(),'No procede',"+uuid),
		fill(adjustment, "'discrepancy','spill','','rejected',NULL,NULL,NULL,"+other+",now(),'No',"+uuid),
		fill(adjustment, "'discrepancy','spill','','rejected',NULL,NULL,NULL,"+other+",now(),NULL,"+uuid),
		fill(adjustment, "'waste','spill','','applied',"+other+",now(),now(),NULL,NULL,NULL,"+uuid),
		fill(adjustment, "'discrepancy','breakage','','pending_authorization',NULL,NULL,now(),NULL,NULL,NULL,NULL"),
		fill(adjustment, "'loss','breakage','','pending_authorization',"+none),
		fill(adjustment, "'discrepancy','breakage',' x','pending_authorization',"+none),
		// Renglón sin unidad de captura o con dos, cantidad cero, signos distintos y renglón fuera de rango.
		fill(adjustmentLine, "1,"+uuid+",NULL,NULL,-2,-2000"), fill(adjustmentLine, "1,"+uuid+","+uuid+","+uuid+",-2,-2000"),
		fill(adjustmentLine, "1,"+uuid+",NULL,"+uuid+",0,0"), fill(adjustmentLine, "1,"+uuid+",NULL,"+uuid+",2,-2000"),
		fill(adjustmentLine, "201,"+uuid+",NULL,"+uuid+",-2,-2000"),
		// Mismo almacén de origen y destino; sin renglones.
		fill(transfer, "'00000000-0000-4000-8000-0000000000d3','00000000-0000-4000-8000-0000000000d3',1"),
		fill(transfer, uuid+","+uuid+",0"),
		// La misma salida y entrada; cantidad cero o negativa; sin unidad de captura.
		fill(transferLine, "1,NULL,"+uuid+",20,20,'00000000-0000-4000-8000-0000000000d4','00000000-0000-4000-8000-0000000000d4'"),
		fill(transferLine, "1,NULL,"+uuid+",0,0,"+uuid+","+uuid), fill(transferLine, "1,NULL,"+uuid+",-1,-1,"+uuid+","+uuid),
		fill(transferLine, "1,NULL,NULL,20,20,"+uuid+","+uuid),
		// Alcance y categoría incoherentes, estados incompletos (incluidas las ramas con NULL) y tamaño.
		fill(count, "'category',NULL,'open',3,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'all',"+uuid+",'open',3,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'submitted',2,"+uuid+",NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'open',2,"+actorAt+",NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'approved',2,"+actorAt+",NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'applied',2,"+actorAt+","+actorAt+","+actorAt+",NULL,NULL,NULL,NULL"),
		fill(count, "'items',NULL,'open',2,NULL,NULL,NULL,NULL,NULL,NULL,now(),NULL,NULL,NULL"),
		fill(count, "'all',NULL,'cancelled',2,NULL,NULL,NULL,NULL,NULL,NULL,NULL,"+actorAt+",NULL"),
		fill(count, "'all',NULL,'cancelled',2,NULL,NULL,NULL,NULL,NULL,NULL,NULL,"+actorAt+",'No'"),
		fill(count, "'all',NULL,'cancelled',2,NULL,NULL,NULL,NULL,NULL,NULL,NULL,"+uuid+",NULL,'Conteo duplicado'"),
		fill(count, "'all',NULL,'cancelled',2,"+uuid+",NULL,NULL,NULL,NULL,NULL,NULL,"+actorAt+",'Conteo duplicado'"),
		fill(count, "'all',NULL,'open',2001,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		fill(count, "'all',NULL,'closed',2,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL,NULL"),
		// Captura a medias, con dos o ninguna unidad, diferencia que no cuadra o sin teórico, movimiento sin
		// diferencia, cantidad negativa y teórico sin captura.
		fill(countLine, "12,12,NULL,"+uuid+",NULL,"+uuid+",NULL,NULL,NULL"),
		fill(countLine, "12,NULL,NULL,"+uuid+","+captured+",NULL,NULL,NULL"),
		fill(countLine, "12,12,"+uuid+","+uuid+","+captured+",NULL,NULL,NULL"),
		fill(countLine, "12,12,NULL,NULL,"+captured+",NULL,NULL,NULL"),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",15,-2,NULL"),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",15,NULL,NULL"),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",12,0,"+uuid),
		fill(countLine, "12,12,NULL,"+uuid+","+captured+",NULL,NULL,"+uuid),
		fill(countLine, "-1,-1,NULL,"+uuid+","+captured+",NULL,NULL,NULL"),
		fill(countLine, "NULL,NULL,NULL,NULL,NULL,NULL,5,-5,NULL"),
		// Nivel sin valores, negativo o fuera de rango.
		fill(level, "NULL,NULL"), fill(level, "-1,NULL"), fill(level, "NULL,1000000001"),
	}
	for _, statement := range accepted {
		if _, err := connection.Exec(ctx, statement); err != nil {
			t.Fatalf("PostgreSQL rechazó un registro válido de la 0014 (%s): %v", statement, err)
		}
	}
	for _, statement := range rejected {
		if _, err := connection.Exec(ctx, statement); err == nil {
			t.Fatalf("PostgreSQL aceptó un registro incoherente de la 0014: %s", statement)
		}
	}
	for copy := range copies {
		if _, err := connection.Exec(ctx, "DROP TABLE "+copy); err != nil {
			t.Fatal(err)
		}
	}
}

// checkControlTriggers prueba los disparadores de la 0014 sobre las tablas reales con datos mínimos
// (el insumo de prueba de la 0010, la cajera y las operaciones de la instalación simulada).
func checkControlTriggers(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	setup := `INSERT INTO app.users(username,display_name,role_id) SELECT 'supervisora','Supervisora',id FROM app.roles WHERE code='administrator';
        INSERT INTO app.inventory_warehouses(id,name) VALUES ('00000000-0000-4000-8000-0000000000e0','Barra de prueba');
        INSERT INTO app.inventory_movements(id,item_id,warehouse_id,kind,quantity,value,unit_cost,cost_basis,occurred_at,source_kind,
            process,entry_key,stock_after,valuation_quantity_after,valuation_value_after)
        SELECT m.id::uuid,'00000000-0000-4000-8000-0000000000a0',w.id,'adjustment_in',1,20,20,'average',now(),'adjustment',
            'migration_check',m.key,11,11,220
        FROM app.inventory_warehouses w,
            (VALUES ('00000000-0000-4000-8000-0000000000e1','prueba:d1'),('00000000-0000-4000-8000-0000000000e2','prueba:d2'),
                    ('00000000-0000-4000-8000-0000000000e3','prueba:d3')) AS m(id,key)
        WHERE w.is_default;
        INSERT INTO app.inventory_adjustments(id,kind,reason,warehouse_id,state,line_count,requested_by,operation_id)
        SELECT '00000000-0000-4000-8000-0000000000e4','discrepancy','breakage',w.id,'pending_authorization',1,u.id,
            '00000000-0000-4000-8000-000000000091'
        FROM app.inventory_warehouses w, app.users u WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_adjustment_lines(adjustment_id,line_no,item_id,unit_id,captured_quantity,quantity)
        SELECT '00000000-0000-4000-8000-0000000000e4',1,'00000000-0000-4000-8000-0000000000a0',id,-1,-1000
        FROM app.inventory_units WHERE code='kg';
        INSERT INTO app.inventory_adjustments(id,kind,reason,warehouse_id,state,line_count,requested_by,applied_at,operation_id)
        SELECT '00000000-0000-4000-8000-0000000000e5','waste','spill',w.id,'applied',1,u.id,now(),'00000000-0000-4000-8000-000000000093'
        FROM app.inventory_warehouses w, app.users u WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_transfers(id,from_warehouse_id,to_warehouse_id,occurred_at,actor_id,line_count,operation_id)
        SELECT '00000000-0000-4000-8000-0000000000e6',w.id,'00000000-0000-4000-8000-0000000000e0',now(),u.id,1,
            '00000000-0000-4000-8000-000000000091'
        FROM app.inventory_warehouses w, app.users u WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_transfer_lines(transfer_id,line_no,item_id,unit_id,captured_quantity,quantity,out_movement_id,
            in_movement_id)
        SELECT '00000000-0000-4000-8000-0000000000e6',1,'00000000-0000-4000-8000-0000000000a0',id,1,1000,
            '00000000-0000-4000-8000-0000000000e1','00000000-0000-4000-8000-0000000000e2'
        FROM app.inventory_units WHERE code='kg';
        INSERT INTO app.inventory_counts(id,warehouse_id,scope,line_count,created_by,updated_by,operation_id)
        SELECT c.id::uuid,w.id,'items',1,u.id,u.id,c.operation::uuid
        FROM app.inventory_warehouses w, app.users u,
            (VALUES ('00000000-0000-4000-8000-0000000000e7','00000000-0000-4000-8000-000000000091'),
                    ('00000000-0000-4000-8000-0000000000e8','00000000-0000-4000-8000-000000000092')) AS c(id,operation)
        WHERE w.is_default AND u.username='cajera';
        INSERT INTO app.inventory_count_lines(count_id,line_no,item_id) VALUES
            ('00000000-0000-4000-8000-0000000000e7',1,'00000000-0000-4000-8000-0000000000a0'),
            ('00000000-0000-4000-8000-0000000000e8',1,'00000000-0000-4000-8000-0000000000a0')`
	if _, err := pool.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}
	reject := func(statements ...string) {
		t.Helper()
		for _, statement := range statements {
			if _, err := pool.Exec(ctx, statement); err == nil {
				t.Fatalf("PostgreSQL aceptó %s", statement)
			}
		}
	}
	accept := func(statements ...string) {
		t.Helper()
		for _, statement := range statements {
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatalf("PostgreSQL rechazó %s: %v", statement, err)
			}
		}
	}
	pending := "'00000000-0000-4000-8000-0000000000e4'"
	waste := "'00000000-0000-4000-8000-0000000000e5'"
	// Una merma no admite entradas; un ajuste no cambia lo registrado ni se borra; sus renglones solo
	// reciben su movimiento una vez.
	reject(
		`INSERT INTO app.inventory_adjustment_lines(adjustment_id,line_no,item_id,unit_id,captured_quantity,quantity)
            SELECT `+waste+`,1,'00000000-0000-4000-8000-0000000000a0',id,1,1000 FROM app.inventory_units WHERE code='kg'`,
		"UPDATE app.inventory_adjustments SET notes='Otra nota' WHERE id="+pending,
		"UPDATE app.inventory_adjustments SET warehouse_id='00000000-0000-4000-8000-0000000000e0' WHERE id="+pending,
		"DELETE FROM app.inventory_adjustments WHERE id="+pending,
		"UPDATE app.inventory_adjustment_lines SET quantity=-2000",
		"DELETE FROM app.inventory_adjustment_lines",
		"TRUNCATE app.inventory_adjustment_lines",
		"UPDATE app.inventory_adjustments SET revision=2 WHERE id="+waste,
	)
	accept(
		"UPDATE app.inventory_adjustment_lines SET movement_id='00000000-0000-4000-8000-0000000000e3' WHERE adjustment_id="+pending,
		`UPDATE app.inventory_adjustments SET state='rejected',rejected_by=(SELECT id FROM app.users WHERE username='supervisora'),
            rejected_at=now(),rejection_reason='No procede el ajuste',decision_operation_id='00000000-0000-4000-8000-000000000092',
            revision=2 WHERE id=`+pending,
	)
	reject(
		"UPDATE app.inventory_adjustment_lines SET movement_id=NULL",
		"UPDATE app.inventory_adjustments SET revision=3 WHERE id="+pending,
		// Transferencias de solo inserción.
		"UPDATE app.inventory_transfers SET notes='Otra nota'",
		"DELETE FROM app.inventory_transfers",
		"UPDATE app.inventory_transfer_lines SET quantity=2000",
		"DELETE FROM app.inventory_transfer_lines",
		"TRUNCATE app.inventory_transfer_lines",
		// Un conteo no cambia de almacén ni se borra.
		"UPDATE app.inventory_counts SET warehouse_id='00000000-0000-4000-8000-0000000000e0'",
		"DELETE FROM app.inventory_counts",
		"UPDATE app.inventory_count_lines SET line_no=2 WHERE count_id='00000000-0000-4000-8000-0000000000e7'",
		"UPDATE app.inventory_count_lines SET movement_id='00000000-0000-4000-8000-0000000000e1'",
	)
	open := "'00000000-0000-4000-8000-0000000000e7'"
	approved := "'00000000-0000-4000-8000-0000000000e8'"
	capture := `UPDATE app.inventory_count_lines SET counted_quantity=%s,captured_quantity=%s,unit_id=(SELECT id FROM app.inventory_units
        WHERE code='g'),captured_at=now(),captured_by=(SELECT id FROM app.users WHERE username='cajera') WHERE count_id=`
	accept(
		// Se captura mientras está abierto; se cancela con motivo.
		strings.Replace(strings.Replace(capture, "%s", "900", 1), "%s", "900", 1)+open,
		`UPDATE app.inventory_counts SET state='cancelled',cancelled_by=created_by,cancelled_at=now(),cancel_reason='Conteo de prueba',
            revision=2 WHERE id=`+open,
		// El otro se captura, se envía y se aprueba; al aplicarse registra una vez su teórico, diferencia y movimiento.
		strings.Replace(strings.Replace(capture, "%s", "1100", 1), "%s", "1100", 1)+approved,
		`UPDATE app.inventory_counts SET state='approved',submitted_by=created_by,submitted_at=now(),approved_by=created_by,
            approved_at=now(),revision=2 WHERE id=`+approved,
		`UPDATE app.inventory_count_lines SET theoretical_quantity=1000,difference=100,movement_id='00000000-0000-4000-8000-0000000000e1'
            WHERE count_id=`+approved,
	)
	reject(
		// Cancelado: ni capturas ni cambios; aprobado: la captura no cambia y no se quitan renglones.
		strings.Replace(strings.Replace(capture, "%s", "800", 1), "%s", "800", 1)+open,
		"UPDATE app.inventory_counts SET notes='Otra nota' WHERE id="+open,
		strings.Replace(strings.Replace(capture, "%s", "1200", 1), "%s", "1200", 1)+approved,
		"UPDATE app.inventory_count_lines SET movement_id='00000000-0000-4000-8000-0000000000e2' WHERE count_id="+approved,
		"DELETE FROM app.inventory_count_lines WHERE count_id="+approved,
		"TRUNCATE app.inventory_count_lines",
	)
	accept(
		`UPDATE app.inventory_counts SET state='applied',applied_by=created_by,applied_at=now(),reference_at=now(),revision=3
            WHERE id=` + approved,
	)
	reject("UPDATE app.inventory_counts SET notes='Otra nota' WHERE id=" + approved)
}
