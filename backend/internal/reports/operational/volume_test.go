//go:build integration

package operational_test

import (
	"math"
	"net/url"
	"os"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/reports"
)

// Medición opt-in (ADEL_REPORTS_MEASURE=1, después de TestOperationalReports en la misma base):
// genera un año de volumen sintético sobre los registros reales de la aceptación (120 000 unidades
// vendidas y reconocidas, 100 000 de ellas con su solicitud de consumo procesada y 2 movimientos
// de consumo cada una —200 000 movimientos— y 60 000 cobros con su movimiento de caja), mide los
// reportes principales de la fase B2 y el dashboard (media y p95 de varias ejecuciones) y comprueba
// que un cobro concurrente no espera mientras un reporte de un año mantiene abierta su
// transacción de solo lectura. No es una prueba de aceptación de importes: registra tiempos.
func TestOperationalVolume(t *testing.T) {
	if os.Getenv("ADEL_REPORTS_MEASURE") != "1" {
		t.Skip("Medición opt-in: ADEL_REPORTS_MEASURE=1")
	}
	fx := reopenFixture(t)
	started := time.Now()
	fx.exec(t, `
        CREATE TEMP TABLE volume_unit AS
            SELECT r.* FROM app.sales_recognitions r WHERE r.source_kind='unit' AND r.product_name='Hamburguesa' AND r.shift_id IS NOT NULL
            ORDER BY r.sequence LIMIT 1;
        CREATE TEMP TABLE volume_payment AS SELECT * FROM app.payments WHERE NOT is_cash ORDER BY created_at LIMIT 1;
        INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,origin,state,opened_by,assigned_user_id,device_id)
        SELECT '00000000-0000-4000-8000-0000000b2001',a.mode,'Volumen de medición B2',a.price_list_id,a.origin,'voided',a.opened_by,
            a.assigned_user_id,a.device_id
        FROM app.restaurant_accounts a JOIN volume_unit v ON v.account_id=a.id;
        INSERT INTO app.restaurant_item_units(id,item_id,version_id,ordinal,account_id,state,subtotal,discount,total,cancelled_at)
        SELECT md5('unidad b2 '||n)::uuid,u.item_id,u.version_id,200000+n,'00000000-0000-4000-8000-0000000b2001','cancelled',0,0,0,now()
        FROM generate_series(1,120000) n CROSS JOIN LATERAL (SELECT u.* FROM app.restaurant_item_units u JOIN volume_unit v ON v.unit_id=u.id) u;
        INSERT INTO app.sales_recognitions(closing_id,recognition_kind,recognized_at,account_id,account_folio,mode,origin,shift_id,
            register_id,responsible_user_id,actor_id,source_kind,unit_id,version_id,product_id,product_name,product_code,category_id,
            category_name,price_list_id,price_list_name,tax_rate,quantity,gross,promotion,discount,courtesy,charge,net,base,tax)
        SELECT md5('cierre b2 '||(n/3))::uuid,'resettlement',
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => 900 + (n % 600)),
            '00000000-0000-4000-8000-0000000b2001',r.account_folio,r.mode,r.origin,r.shift_id,r.register_id,r.responsible_user_id,r.actor_id,
            'unit',md5('unidad b2 '||n)::uuid,r.version_id,r.product_id,r.product_name,r.product_code,r.category_id,r.category_name,
            r.price_list_id,r.price_list_name,16,1,120,0,0,0,0,120,103.45,16.55
        FROM generate_series(1,120000) n CROSS JOIN volume_unit r;
        INSERT INTO app.inventory_consumption_requests(id,kind,restaurant_unit_id,source_id,version_id,released_at,occurred_at,service_mode,
            state,attempts,processed_at)
        SELECT md5('solicitud b2 '||n)::uuid,'release',md5('unidad b2 '||n)::uuid,md5('liberación b2 '||n)::uuid,r.version_id,
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => 840 + (n % 600)),
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => 840 + (n % 600)),
            'dine_in','processed',1,now()
        FROM generate_series(1,100000) n CROSS JOIN volume_unit r;
        INSERT INTO app.inventory_unit_consumptions(restaurant_unit_id,release_request_id,last_request_id,last_version_id,released_at,service_mode)
        SELECT q.restaurant_unit_id,q.id,q.id,q.version_id,q.released_at,'dine_in' FROM app.inventory_consumption_requests q
        WHERE q.id IN (SELECT md5('solicitud b2 '||n)::uuid FROM generate_series(1,100000) n);
        INSERT INTO app.inventory_movements(id,item_id,warehouse_id,kind,quantity,value,unit_cost,cost_basis,cost_pending,occurred_at,
            source_kind,source_id,restaurant_unit_id,process,entry_key,stock_after,valuation_quantity_after,valuation_value_after)
        SELECT gen_random_uuid(),i.id,w.id,'consumption',i.quantity,i.value,i.cost,'average',false,q.released_at,'consumption',q.source_id,
            q.restaurant_unit_id,'inventory_worker','volumen b2:'||q.id||':'||i.code,0,0,0
        FROM app.inventory_consumption_requests q
        CROSS JOIN (SELECT id,code,CASE code WHEN 'PAN' THEN -1 ELSE -150 END AS quantity,CASE code WHEN 'PAN' THEN -3 ELSE -30 END AS value,
                CASE code WHEN 'PAN' THEN 3 ELSE 0.2 END AS cost
            FROM app.inventory_items WHERE code IN ('PAN','CARNE')) i
        CROSS JOIN (SELECT id FROM app.inventory_warehouses WHERE is_default) w
        WHERE q.id IN (SELECT md5('solicitud b2 '||n)::uuid FROM generate_series(1,100000) n);
        INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        SELECT md5('volumen b2 '||n)::uuid,p.actor_id,p.device_id,'POST medición B2','huella','{}'
        FROM generate_series(1,60000) n CROSS JOIN volume_payment p;
        INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,kind,amount,tip,actor_id,device_id,
            operation_id,created_at)
        SELECT md5('cobro b2 '||n)::uuid,'00000000-0000-4000-8000-0000000b2001',p.shift_id,p.register_id,p.method_id,p.method_name,false,
            'general',120,0,p.actor_id,p.device_id,md5('volumen b2 '||n)::uuid,
            date_trunc('day',now()) - make_interval(days => (n % 365)) + make_interval(mins => 900 + (n % 600))
        FROM generate_series(1,60000) n CROSS JOIN volume_payment p;
        INSERT INTO app.cash_movements(id,shift_id,register_id,kind,method_id,method_name,is_cash,sale_amount,tip_amount,net_amount,
            account_id,payment_id,actor_id,device_id,operation_id,created_at)
        SELECT gen_random_uuid(),p.shift_id,p.register_id,'payment',p.method_id,p.method_name,false,120,0,120,p.account_id,p.id,p.actor_id,
            p.device_id,p.operation_id,p.created_at
        FROM app.payments p WHERE p.id IN (SELECT md5('cobro b2 '||n)::uuid FROM generate_series(1,60000) n);
        ANALYZE app.sales_recognitions; ANALYZE app.inventory_movements; ANALYZE app.inventory_consumption_requests;
        ANALYZE app.inventory_unit_consumptions; ANALYZE app.payments; ANALYZE app.cash_movements; ANALYZE app.restaurant_item_units`)
	t.Logf("Volumen sintético generado en %s: %s unidades reconocidas, %s movimientos de inventario, %s cobros",
		time.Since(started).Round(time.Millisecond), fx.scalar(t, "SELECT count(*)::text FROM app.sales_recognitions WHERE source_kind='unit'"),
		fx.scalar(t, "SELECT count(*)::text FROM app.inventory_movements"), fx.scalar(t, "SELECT count(*)::text FROM app.payments"))
	first := fx.scalar(t, "SELECT ((now() AT TIME ZONE 'America/Monterrey') - interval '05:00')::date - 364 || ''")
	last := fx.scalar(t, "SELECT ((now() AT TIME ZONE 'America/Monterrey') - interval '05:00')::date || ''")
	year := url.Values{"period": {"range"}, "from": {first}, "until": {last}}
	runner := reports.NewRunner(fx.pool, reports.Slots)
	actor := reports.Actor{UserID: fx.adminID, DeviceID: fx.admin.deviceID, Permissions: allPermissions()}
	measure := func(name string, run func() (int, error)) {
		samples := []time.Duration{}
		rows := 0
		for index := 0; index < 12; index++ {
			begin := time.Now()
			count, err := run()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			samples = append(samples, time.Since(begin))
			rows = count
		}
		sort.Slice(samples, func(left, right int) bool { return samples[left] < samples[right] })
		var total time.Duration
		for _, sample := range samples {
			total += sample
		}
		p95 := samples[int(math.Ceil(0.95*float64(len(samples))))-1]
		t.Logf("| `%s` | %d | %s | %s |", name, rows, (total / time.Duration(len(samples))).Round(time.Millisecond), p95.Round(time.Millisecond))
	}
	t.Log("| Reporte (un año, 12 ejecuciones) | Renglones | Media | p95 |")
	for _, id := range []string{"cash.shifts", "cash.by_day", "cash.sales_collections_by_shift", "inventory.consumption_by_item",
		"inventory.consumption_by_day", "inventory.consumption_by_product", "inventory.consumption_by_kitchen", "costs.margin_by_product",
		"costs.margin_by_category", "costs.margin_by_period", "costs.cost_status", "purchases.received", "purchases.by_supplier",
		"employees.activity", "employees.responsible_sales"} {
		measure(id, func() (int, error) {
			result, err := reports.Run(fx.ctx, runner, actor, id, year)
			if err != nil {
				return 0, err
			}
			return result.Page.TotalRows, nil
		})
	}
	// Un día comercial con el volumen de un año: las consultas usan los índices de fecha y de unidad.
	for _, id := range []string{"costs.margin_by_product", "inventory.consumption_by_item", "cash.by_day"} {
		measure(id+" (hoy)", func() (int, error) {
			result, err := reports.Run(fx.ctx, runner, actor, id, url.Values{"period": {"today"}})
			if err != nil {
				return 0, err
			}
			return result.Page.TotalRows, nil
		})
	}
	measure("inventory.kardex (un mes, un insumo)", func() (int, error) {
		values := url.Values{"period": {"month"}, "month": {last[:7]}, "item_id": {fx.scalar(t, "SELECT id::text FROM app.inventory_items WHERE code='PAN'")}}
		result, err := reports.Run(fx.ctx, runner, actor, "inventory.kardex", values)
		if err != nil {
			return 0, err
		}
		return result.Page.TotalRows, nil
	})
	measure("dashboard (un año contra el anterior)", func() (int, error) {
		tx, err := fx.pool.Begin(fx.ctx)
		if err != nil {
			return 0, err
		}
		plan, err := reports.Handle(fx.ctx, tx, reports.Request{Method: "GET", Route: "reports/dashboard", Query: year, Actor: actor})
		_ = tx.Rollback(fx.ctx)
		if err != nil {
			return 0, err
		}
		_, err = plan.Execute(fx.ctx, runner)
		return 0, err
	})

	// Cobro concurrente: un reporte de margen de un año mantiene abierta su transacción de solo lectura
	// mientras se registra un cobro por la API; el cobro no espera al reporte.
	shift := decode[map[string]any](t, fx.mutate(t, "POST", "payments/shifts", map[string]any{"opening_float": "100"}, 200))
	bill := fx.account(t, "Cobro concurrente", line(fx.soda, 1))
	other := fx.account(t, "Cobro de referencia", line(fx.soda, 1))
	baseline := time.Now()
	fx.pay(t, fx.admin, other.Account.ID, map[string]any{"method_id": fx.methods["debit"], "mode": "total"})
	reference := time.Since(baseline)
	inside := make(chan struct{})
	paid := make(chan struct{})
	var finished atomic.Bool
	var reportTime time.Duration
	errors := make(chan error, 1)
	go func() {
		begin := time.Now()
		err := runner.Read(fx.ctx, func(tx pgx.Tx) error {
			close(inside)
			definition, raw, err := reports.Prepare(actor, "costs.margin_by_product", year)
			if err != nil {
				return err
			}
			if _, err := reports.Execute(fx.ctx, tx, actor, definition, raw); err != nil {
				return err
			}
			reportTime = time.Since(begin)
			<-paid
			return nil
		})
		finished.Store(true)
		errors <- err
	}()
	<-inside
	time.Sleep(20 * time.Millisecond)
	begin := time.Now()
	fx.pay(t, fx.admin, bill.Account.ID, map[string]any{"method_id": fx.methods["debit"], "mode": "total"})
	payment := time.Since(begin)
	stillOpen := !finished.Load()
	close(paid)
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	t.Logf("Cobro concurrente (consulta del saldo y pago por la API) con el reporte de margen de un año en curso: %s "+
		"(el mismo cobro sin reporte: %s); reporte de margen de un año: %s; transacción del reporte aún abierta al terminar el cobro: %v; turno %s",
		payment.Round(time.Millisecond), reference.Round(time.Millisecond), reportTime.Round(time.Millisecond), stillOpen, shift["id"])
	if !stillOpen || payment > 3*time.Second {
		t.Fatalf("El cobro esperó al reporte: %s (transacción abierta: %v)", payment, stillOpen)
	}
	for name, query := range map[string]string{
		"margen por producto (un año)": `EXPLAIN (ANALYZE, BUFFERS) WITH recognized AS (SELECT r.* FROM app.sales_recognitions r
                WHERE r.unit_id IS NOT NULL AND r.recognized_at >= now() - interval '366 days'),
            units AS (SELECT DISTINCT unit_id FROM recognized)
            SELECT count(*),-sum(m.value) FROM app.inventory_movements m JOIN units u ON u.unit_id=m.restaurant_unit_id
            WHERE m.kind IN ('consumption','consumption_reversal')`,
		"consumo por día (un año)": `EXPLAIN (ANALYZE, BUFFERS) SELECT ((m.occurred_at AT TIME ZONE 'America/Monterrey') - interval '05:00')::date,
                count(DISTINCT m.restaurant_unit_id),-sum(m.value) FROM app.inventory_movements m
            WHERE m.kind IN ('consumption','consumption_reversal') AND m.restaurant_unit_id IS NOT NULL
                AND m.occurred_at >= now() - interval '366 days' GROUP BY 1`,
	} {
		rows, err := fx.pool.Query(fx.ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		plan := ""
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				t.Fatal(err)
			}
			plan += text + "\n"
		}
		rows.Close()
		t.Logf("== %s\n%s", name, plan)
	}
}

// allPermissions son los permisos de los reportes y de origen que exige la medición.
func allPermissions() []string {
	return []string{reports.PermissionRead, reports.PermissionSales, reports.PermissionPayments, reports.PermissionDiscounts,
		reports.PermissionCash, reports.PermissionInventory, reports.PermissionPurchases, reports.PermissionEmployees,
		reports.PermissionCashExpected, reports.PermissionCostsRead, reports.PermissionValuationRead, reports.PermissionStockRead,
		"inventory.movements.read", "cash.shifts.read", "cash.shifts.read_all", "cash.history.read", "cash.movements.read"}
}
