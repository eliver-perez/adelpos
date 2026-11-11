//go:build integration

package reports_test

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/reports"
)

// Medición opt-in (ADEL_REPORTS_MEASURE=1, después de TestReportsAcceptance en la misma base):
// genera un año de volumen sintético sobre registros reales (200 000 renglones de reconocimiento, cada
// uno con su unidad, y 100 000 cobros), mide los reportes de un año por HTTP e imprime el plan (EXPLAIN ANALYZE) de las
// consultas principales para comprobar que usan los índices de fecha de la 0017. No es una prueba
// de aceptación: solo registra tiempos.
func TestReportsVolumeMeasurement(t *testing.T) {
	if os.Getenv("ADEL_REPORTS_MEASURE") != "1" {
		t.Skip("Medición opt-in: ADEL_REPORTS_MEASURE=1")
	}
	ctx, pool := openPool(t)
	var configured bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.sales_recognitions)").Scan(&configured); err != nil || !configured {
		t.Skip("Requiere los datos de TestReportsAcceptance")
	}
	started := time.Now()
	// Cada renglón sintético tiene su propia unidad (como en la operación real, donde una unidad tiene
	// uno o pocos renglones), en una cuenta anulada propia de la medición: así la consulta de
	// diferencias de un cierre se mide con una distribución realista.
	if _, err := pool.Exec(ctx, `
        CREATE TEMP TABLE volume_source AS SELECT * FROM app.sales_recognitions WHERE source_kind='unit' ORDER BY sequence LIMIT 1;
        INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,origin,state,opened_by,assigned_user_id,device_id)
        SELECT '00000000-0000-4000-8000-00000000f001',a.mode,'Volumen de medición',a.price_list_id,a.origin,'voided',a.opened_by,
            a.assigned_user_id,a.device_id
        FROM app.restaurant_accounts a JOIN volume_source v ON v.account_id=a.id;
        INSERT INTO app.restaurant_item_units(id,item_id,version_id,ordinal,account_id,state,subtotal,discount,total,cancelled_at)
        SELECT md5('unidad'||n)::uuid,u.item_id,u.version_id,100000+n,'00000000-0000-4000-8000-00000000f001','cancelled',0,0,0,now()
        FROM generate_series(1,200000) n
        CROSS JOIN LATERAL (SELECT u.* FROM app.restaurant_item_units u JOIN volume_source v ON v.unit_id=u.id) u;
        INSERT INTO app.sales_recognitions(closing_id,recognition_kind,recognized_at,account_id,account_folio,mode,origin,table_id,
            table_name,area_id,area_name,shift_id,register_id,source_kind,unit_id,version_id,product_id,product_name,product_code,
            category_id,category_name,price_list_id,price_list_name,tax_rate,quantity,gross,promotion,discount,courtesy,charge,net,base,tax)
        SELECT gen_random_uuid(),'resettlement',date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900)),
            '00000000-0000-4000-8000-00000000f001',r.account_folio,r.mode,r.origin,r.table_id,r.table_name,r.area_id,r.area_name,r.shift_id,
            r.register_id,'unit',md5('unidad'||n)::uuid,r.version_id,r.product_id,r.product_name,r.product_code,r.category_id,r.category_name,
            r.price_list_id,r.price_list_name,16,1,10,0,0,0,0,10,8.62,1.38
        FROM generate_series(1,200000) n CROSS JOIN volume_source r;
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT md5('volumen'||n)::uuid,p.actor_id,p.device_id,'POST medición','huella','{}'
        FROM generate_series(1,100000) n CROSS JOIN LATERAL (SELECT actor_id,device_id FROM app.payments LIMIT 1) p;
        INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,kind,amount,tip,actor_id,device_id,
            operation_id,created_at)
        SELECT gen_random_uuid(),p.account_id,p.shift_id,p.register_id,p.method_id,p.method_name,false,'general',1,0,p.actor_id,
            p.device_id,md5('volumen'||n)::uuid,date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => (n % 900))
        FROM generate_series(1,100000) n CROSS JOIN LATERAL (SELECT * FROM app.payments WHERE NOT is_cash LIMIT 1) p;
        ANALYZE app.sales_recognitions; ANALYZE app.payments; ANALYZE app.payment_refunds`); err != nil {
		t.Fatal(err)
	}
	t.Logf("Volumen sintético generado en %s", time.Since(started).Round(time.Millisecond))
	// Reportes de un año completos (consulta, totales, orden y página) en el ejecutor de solo lectura.
	runner := reports.NewRunner(pool, reports.Slots)
	actor := reports.Actor{Permissions: []string{reports.PermissionRead, reports.PermissionSales, reports.PermissionPayments,
		reports.PermissionDiscounts, reports.PermissionCashExpected, "cash.shifts.read", "cash.shifts.read_all", "cash.history.read"}}
	for _, id := range []string{"sales.by_day", "sales.by_product", "sales.by_month", "payments.by_method", "payments.by_day",
		"discounts.summary", "sales.evolution"} {
		begin := time.Now()
		result, err := reports.Run(ctx, runner, actor, id, url.Values{"period": {"current_year"}})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s (año actual): %d renglones en %s", id, result.Page.TotalRows, time.Since(begin).Round(time.Millisecond))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := reports.Handle(ctx, tx, reports.Request{Method: "GET", Route: "reports/dashboard",
		Query: url.Values{"period": {"current_year"}}, Actor: actor})
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	if _, err := plan.Execute(ctx, runner); err != nil {
		t.Fatal(err)
	}
	t.Logf("dashboard (año actual contra el anterior): %s", time.Since(begin).Round(time.Millisecond))
	var closedAccount string
	if err := pool.QueryRow(ctx, `SELECT account_id::text FROM app.sales_recognitions WHERE recognition_kind='initial'
        ORDER BY sequence DESC LIMIT 1`).Scan(&closedAccount); err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{
		"ventas por día (un año)": `SELECT ((r.recognized_at AT TIME ZONE 'America/Monterrey') - interval '05:00')::date,sum(r.net),sum(r.tax)
            FROM app.sales_recognitions r WHERE r.recognized_at >= now() - interval '366 days' AND r.recognized_at < now() + interval '1 day'
            GROUP BY 1`,
		"ventas por producto (un año)": `SELECT r.product_id,(array_agg(r.product_name ORDER BY r.sequence DESC))[1],sum(r.quantity),sum(r.net)
            FROM app.sales_recognitions r WHERE r.recognized_at >= now() - interval '366 days' AND r.recognized_at < now() + interval '1 day'
                AND r.product_id IS NOT NULL GROUP BY r.product_id`,
		"cobros por método (un año)": `SELECT p.method_id,sum(p.amount),sum(p.tip) FROM app.payments p
            JOIN app.cash_shifts s ON s.id=p.shift_id AND s.state <> 'closing'
            WHERE p.created_at >= now() - interval '366 days' AND p.created_at < now() + interval '1 day' GROUP BY p.method_id`,
		"ventas de un día": `SELECT sum(r.net) FROM app.sales_recognitions r
            WHERE r.recognized_at >= date_trunc('day',now()) AND r.recognized_at < date_trunc('day',now()) + interval '1 day'`,
		"cobros de un día": `SELECT sum(p.amount) FROM app.payments p
            WHERE p.created_at >= date_trunc('day',now()) AND p.created_at < date_trunc('day',now()) + interval '1 day'`,
		// Lo que consulta un cierre para calcular sus diferencias (payments/recognition.go).
		"diferencias al cerrar una cuenta": `WITH sources AS (
                SELECT DISTINCT source_kind,unit_id,adjustment_id,tax_rate FROM app.sales_recognitions
                WHERE account_id='` + closedAccount + `'
            )
            SELECT r.* FROM app.sales_recognitions r WHERE r.unit_id IN (SELECT unit_id FROM sources WHERE unit_id IS NOT NULL)
            UNION
            SELECT r.* FROM app.sales_recognitions r WHERE r.adjustment_id IN (SELECT adjustment_id FROM sources WHERE adjustment_id IS NOT NULL)`,
	}
	for name, query := range queries {
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query)
		if err != nil {
			t.Fatal(err)
		}
		plan := []string{}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		rows.Close()
		t.Logf("== %s\n%s", name, strings.Join(plan, "\n"))
	}
}
