//go:build integration

package database

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/product"
	"local/ad-el-pos/server/migrations"
)

// Etapa K1.1 (Fase A, K11-D3 y K11-D4): actualización desde el esquema 29 con lo que dejó la 0.16.0 en «Pendientes de
// liberación y avisos»: envíos bloqueados de cuentas cerradas, canceladas, combinadas y abiertas, un pedido programado
// de un pedido para llevar cobrado y sin entregar, un envío ya liberado y avisos pendientes de una cuenta cerrada, de
// una abierta y de una combinada en una abierta. La sección «Fase A» de la 0030 agrega «Productos sin ruta» (NULL, «No
// necesitan preparación») y dismissed_at/dismissed_reason en los avisos, y da por terminados solo los pendientes de
// cuentas que ya no están abiertas (por la pertenencia actual de las unidades), con su fila en el historial de cocina.
// Ninguna otra tabla cambia: ventas, pagos, cuentas, partidas, unidades, eventos, turnos, inventario,
// kitchen_unit_releases, tareas, comandas, trabajos ni auditoría. Deja la base vacía.
func TestStageK11Upgrade(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(u.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("pool creation failed")
	}
	defer pool.Close()
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()
	expected, err := ReadMigrations(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if last := expected[len(expected)-1].Version; last < 30 || last != product.Read().SchemaVersion {
		t.Fatalf("la última migración (%d) no coincide con el esquema del producto (%d)", last, product.Read().SchemaVersion)
	}
	schema29 := fstest.MapFS{}
	for _, migration := range expected {
		if migration.Version <= 29 {
			schema29[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if n, err := Apply(ctx, pool, schema29); err != nil || n != 29 {
		t.Fatalf("no se preparó el esquema 29: %d %v", n, err)
	}
	if _, err := pool.Exec(ctx, stageK11ExistingData); err != nil {
		t.Fatal(err)
	}
	tables := stageA1Tables(ctx, t, pool)
	before := map[string]string{}
	for table, columns := range tables {
		before[table] = stageA1Snapshot(ctx, t, pool, table, columns)
	}
	releasesBefore := stageK11Releases(ctx, t, pool)
	if n, err := Migrate(ctx, pool); err != nil || n != len(expected)-29 {
		t.Fatalf("la 0030 no se aplicó sobre el esquema 29: %d %v", n, err)
	}
	if n, err := Migrate(ctx, pool); err != nil || n != 0 {
		t.Fatal("la 0030 no es repetible")
	}
	// Solo cambian las liberaciones y los avisos de cuentas que ya no están abiertas, y el historial de cocina gana sus
	// filas; las columnas nuevas no cuentan (la comparación usa las columnas del esquema 29).
	changed := map[string]bool{"kitchen_releases": true, "kitchen_pending_changes": true, "kitchen_history": true}
	for table, columns := range tables {
		if changed[table] {
			continue
		}
		if after := stageA1Snapshot(ctx, t, pool, table, columns); after != before[table] {
			t.Fatalf("la 0030 cambió filas existentes de %s:\nantes   %s\ndespués %s", table, before[table], after)
		}
	}
	checkStageK11Releases(ctx, t, pool, releasesBefore)
	checkStageK11Changes(ctx, t, pool)
	var unrouted *string
	if err := pool.QueryRow(ctx, "SELECT unrouted_station_id::text FROM app.kitchen_settings").Scan(&unrouted); err != nil || unrouted != nil {
		t.Fatalf("«Productos sin ruta» al actualizar: %v %v", unrouted, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app.kitchen_pending_changes SET dismissed_at=now()
        WHERE event_id='00000000-0000-4000-8000-000000000ec2'`); !stageK1Violates("23514", err) {
		t.Fatalf("un aviso terminado sin motivo: %v", err)
	}
}

// stageK11Release es el estado de una liberación que la 0030 puede cambiar.
type stageK11Release struct {
	State    string
	Revision int
	Warnings string
}

func stageK11Releases(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]stageK11Release {
	t.Helper()
	rows, err := pool.Query(ctx, "SELECT id::text,state,revision,warnings::text FROM app.kitchen_releases")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]stageK11Release{}
	for rows.Next() {
		var id string
		var release stageK11Release
		if err := rows.Scan(&id, &release.State, &release.Revision, &release.Warnings); err != nil {
			t.Fatal(err)
		}
		result[id] = release
	}
	return result
}

// checkStageK11Releases: los bloqueados de la cuenta cerrada y de la cancelada pasan a «cancelled» (revisión siguiente,
// advertencias intactas) con su release.dismissed sin actor; los de la combinada (sus unidades están en una abierta), de
// la abierta, el programado del pedido para llevar sin entregar y el ya liberado no cambian.
func checkStageK11Releases(ctx context.Context, t *testing.T, pool *pgxpool.Pool, before map[string]stageK11Release) {
	t.Helper()
	after := stageK11Releases(ctx, t, pool)
	dismissed := map[string]bool{
		"00000000-0000-4000-8000-000000000e01": true,
		"00000000-0000-4000-8000-000000000e02": true,
	}
	for id, previous := range before {
		current := after[id]
		if dismissed[id] {
			if current.State != "cancelled" || current.Revision != previous.Revision+1 || current.Warnings != previous.Warnings {
				t.Fatalf("la liberación %s de una cuenta que ya no está abierta: %+v → %+v", id, previous, current)
			}
			var beforeValue, afterValue []byte
			var actor *string
			if err := pool.QueryRow(ctx, `SELECT before_value,after_value,actor_id::text FROM app.kitchen_history
                WHERE kind='release.dismissed' AND entity_id=$1`, id).Scan(&beforeValue, &afterValue, &actor); err != nil {
				t.Fatalf("historial de %s: %v", id, err)
			}
			var recorded struct {
				State    string   `json:"state"`
				Warnings []string `json:"warnings"`
			}
			var result map[string]string
			if json.Unmarshal(beforeValue, &recorded) != nil || json.Unmarshal(afterValue, &result) != nil ||
				recorded.State != previous.State || len(recorded.Warnings) == 0 || actor != nil ||
				result["state"] != "cancelled" || result["reason"] != "account_not_open" || result["migration"] != "0030" {
				t.Fatalf("historial de %s: %s → %s (actor %v)", id, beforeValue, afterValue, actor)
			}
			continue
		}
		if current != previous {
			t.Fatalf("la 0030 cambió la liberación %s de una cuenta abierta: %+v → %+v", id, previous, current)
		}
	}
	var history int
	if err := pool.QueryRow(ctx, "SELECT count(*)::int FROM app.kitchen_history WHERE kind='release.dismissed'").Scan(&history); err != nil ||
		history != len(dismissed) {
		t.Fatalf("filas release.dismissed: %d %v", history, err)
	}
}

// checkStageK11Changes: el aviso de la cuenta cerrada recibe dismissed_at con account_not_open y su change.dismissed;
// el de la abierta y el de la combinada en una abierta siguen pendientes.
func checkStageK11Changes(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT event_id::text,dismissed_at IS NOT NULL,coalesce(dismissed_reason,'')
        FROM app.kitchen_pending_changes ORDER BY event_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := []string{}
	for rows.Next() {
		var id, reason string
		var dismissed bool
		if err := rows.Scan(&id, &dismissed, &reason); err != nil {
			t.Fatal(err)
		}
		if dismissed {
			found = append(found, id[len(id)-3:]+":"+reason)
		} else {
			found = append(found, id[len(id)-3:]+":pendiente")
		}
	}
	if strings.Join(found, ",") != "ec1:account_not_open,ec2:pendiente,ec3:pendiente" {
		t.Fatalf("avisos pendientes después de la 0030: %v", found)
	}
	var history int
	if err := pool.QueryRow(ctx, `SELECT count(*)::int FROM app.kitchen_history WHERE kind='change.dismissed'
        AND entity_id='00000000-0000-4000-8000-000000000ec1' AND actor_id IS NULL AND after_value->>'reason'='account_not_open'
        AND after_value->>'migration'='0030' AND before_value->>'kind'='item.modified'`).Scan(&history); err != nil || history != 1 {
		t.Fatalf("historial change.dismissed: %d %v", history, err)
	}
}

// Una instalación en el esquema 29 con lo que deja la 0.16.0 (identificadores fijos):
//   - a1 cerrada (e01 bloqueado y ec1, el aviso de una modificación de su unidad liberada u11);
//   - a2 cancelada (e02 bloqueado, su unidad u21 cancelada);
//   - a3 combinada en a4 (e03 bloqueado con la unidad u31, que ahora está en a4, y ec3, el aviso de la cancelación de
//     u32, que se quedó en a3);
//   - a4 abierta (e04 bloqueado con u41 y ec2, el aviso de una modificación de u42, liberada);
//   - a5 para llevar, cobrada y sin entregar (sigue abierta; e05 programado);
//   - a6 cerrada con e06 ya liberado (u61 en kitchen_unit_releases).
const stageK11ExistingData = `
INSERT INTO app.establishment(name,timezone) VALUES ('Restaurante K1.1','America/Monterrey');
INSERT INTO app.roles(id,code,name) VALUES ('00000000-0000-4000-8000-000000000001','administrator','Administrador');
INSERT INTO app.users(id,username,display_name,role_id) VALUES
    ('00000000-0000-4000-8000-000000000011','cajera','Cajera','00000000-0000-4000-8000-000000000001');
INSERT INTO app.registers(id,name) VALUES ('00000000-0000-4000-8000-000000000021','Caja K1.1');
INSERT INTO app.devices(id,name,secret_hash,register_id) VALUES
    ('00000000-0000-4000-8000-000000000022','Terminal 1','hash-k11','00000000-0000-4000-8000-000000000021');
INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
SELECT op::uuid,'00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022','POST prueba','huella','{}'
FROM unnest(ARRAY['00000000-0000-4000-8000-000000000d01','00000000-0000-4000-8000-000000000d02','00000000-0000-4000-8000-000000000d03',
    '00000000-0000-4000-8000-000000000d04','00000000-0000-4000-8000-000000000d05','00000000-0000-4000-8000-000000000d06',
    '00000000-0000-4000-8000-000000000d07','00000000-0000-4000-8000-000000000d08']) op;
INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id) VALUES
    ('00000000-0000-4000-8000-0000000000f1','00000000-0000-4000-8000-000000000021',0,'00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000d07');
INSERT INTO app.catalog_price_lists(id,name,fallback) VALUES ('00000000-0000-4000-8000-000000000031','Lista K1.1','base');
INSERT INTO app.catalog_categories(id,name) VALUES ('00000000-0000-4000-8000-000000000041','Desayunos'),
    ('00000000-0000-4000-8000-000000000042','Refrescos');
INSERT INTO app.catalog_products(id,category_id,name,code,kind,base_price) VALUES
    ('00000000-0000-4000-8000-000000000051','00000000-0000-4000-8000-000000000041','Machacado Natural','MACHACADO','product',95),
    ('00000000-0000-4000-8000-000000000052','00000000-0000-4000-8000-000000000042','Coca Cola 600ml','COCA-600','product',35);
INSERT INTO app.kitchen_stations(id,name,reception) VALUES ('00000000-0000-4000-8000-000000000061','Cocina','kds');
INSERT INTO app.restaurant_accounts(id,mode,name,price_list_id,origin,state,opened_by,assigned_user_id,device_id,closed_at,
    merged_into_id,scheduled_at,schedule_kind) VALUES
    ('00000000-0000-4000-8000-0000000000a4','dine_in','Abierta','00000000-0000-4000-8000-000000000031','Mostrador','open',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     NULL,NULL,NULL,NULL),
    ('00000000-0000-4000-8000-0000000000a1','dine_in','Cobrada','00000000-0000-4000-8000-000000000031','Mostrador','closed',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     '2026-11-25T20:00:00Z',NULL,NULL,NULL),
    ('00000000-0000-4000-8000-0000000000a2','dine_in','Cancelada','00000000-0000-4000-8000-000000000031','Mostrador','voided',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     NULL,NULL,NULL,NULL),
    ('00000000-0000-4000-8000-0000000000a3','dine_in','Combinada','00000000-0000-4000-8000-000000000031','Mostrador','merged',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     NULL,'00000000-0000-4000-8000-0000000000a4',NULL,NULL),
    ('00000000-0000-4000-8000-0000000000a5','takeaway','Para llevar sin entregar','00000000-0000-4000-8000-000000000031',
     'Ventanilla','open','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011',
     '00000000-0000-4000-8000-000000000022',NULL,NULL,'2026-11-27T02:00:00Z','delivery'),
    ('00000000-0000-4000-8000-0000000000a6','dine_in','Cobrada y enviada','00000000-0000-4000-8000-000000000031','Mostrador','closed',
     '00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000011','00000000-0000-4000-8000-000000000022',
     '2026-11-25T21:00:00Z',NULL,NULL,NULL);
INSERT INTO app.restaurant_orders(id,original_account_id,operation_id,kind,author_id,device_id,price_list_id,pricing_snapshot,quote)
SELECT o.id::uuid,o.account::uuid,o.operation::uuid,'confirmation','00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000031','{}','{}'
FROM (VALUES ('00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d01'),
    ('00000000-0000-4000-8000-000000000202','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000d02'),
    ('00000000-0000-4000-8000-000000000203','00000000-0000-4000-8000-0000000000a3','00000000-0000-4000-8000-000000000d03'),
    ('00000000-0000-4000-8000-000000000204','00000000-0000-4000-8000-0000000000a4','00000000-0000-4000-8000-000000000d04'),
    ('00000000-0000-4000-8000-000000000205','00000000-0000-4000-8000-0000000000a5','00000000-0000-4000-8000-000000000d05'),
    ('00000000-0000-4000-8000-000000000206','00000000-0000-4000-8000-0000000000a6','00000000-0000-4000-8000-000000000d06')) o(id,account,operation);
INSERT INTO app.restaurant_items(id,original_order_id,product_id)
SELECT i.id::uuid,i.order_id::uuid,i.product::uuid FROM (VALUES
    ('00000000-0000-4000-8000-000000000281','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000282','00000000-0000-4000-8000-000000000202','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000283','00000000-0000-4000-8000-000000000203','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000284','00000000-0000-4000-8000-000000000204','00000000-0000-4000-8000-000000000052'),
    ('00000000-0000-4000-8000-000000000285','00000000-0000-4000-8000-000000000205','00000000-0000-4000-8000-000000000051'),
    ('00000000-0000-4000-8000-000000000286','00000000-0000-4000-8000-000000000206','00000000-0000-4000-8000-000000000051')
) i(id,order_id,product);
INSERT INTO app.restaurant_item_versions(id,item_id,order_id,version,request,quote_line,author_id,device_id,tax_rate)
SELECT ('00000000-0000-4000-8000-000000000' || (290 + n))::uuid,('00000000-0000-4000-8000-000000000' || (280 + n))::uuid,
    ('00000000-0000-4000-8000-000000000' || (200 + n))::uuid,1,'{}','{}','00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022',16
FROM generate_series(1,6) n;
INSERT INTO app.restaurant_item_units(id,item_id,version_id,ordinal,account_id,state,subtotal,discount,total,cancelled_at)
SELECT u.id::uuid,('00000000-0000-4000-8000-000000000' || u.item)::uuid,('00000000-0000-4000-8000-000000000' || (u.item::int + 10))::uuid,
    u.ordinal,u.account::uuid,u.state,35,0,35,CASE WHEN u.state='cancelled' THEN '2026-11-25T19:30:00Z'::timestamptz END
FROM (VALUES
    ('00000000-0000-4000-8000-000000000311','281',1,'00000000-0000-4000-8000-0000000000a1','active'),
    ('00000000-0000-4000-8000-000000000312','281',2,'00000000-0000-4000-8000-0000000000a1','active'),
    ('00000000-0000-4000-8000-000000000321','282',1,'00000000-0000-4000-8000-0000000000a2','cancelled'),
    ('00000000-0000-4000-8000-000000000331','283',1,'00000000-0000-4000-8000-0000000000a4','active'),
    ('00000000-0000-4000-8000-000000000332','283',2,'00000000-0000-4000-8000-0000000000a3','cancelled'),
    ('00000000-0000-4000-8000-000000000341','284',1,'00000000-0000-4000-8000-0000000000a4','active'),
    ('00000000-0000-4000-8000-000000000342','284',2,'00000000-0000-4000-8000-0000000000a4','active'),
    ('00000000-0000-4000-8000-000000000351','285',1,'00000000-0000-4000-8000-0000000000a5','active'),
    ('00000000-0000-4000-8000-000000000361','286',1,'00000000-0000-4000-8000-0000000000a6','active')
) u(id,item,ordinal,account,state);
INSERT INTO app.restaurant_events(id,account_id,operation_id,kind,payload,author_id,device_id)
SELECT e.id::uuid,e.account::uuid,e.operation::uuid,e.kind,e.payload::jsonb,'00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022'
FROM (VALUES
    ('00000000-0000-4000-8000-000000000e01','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d01','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000e02','00000000-0000-4000-8000-0000000000a2','00000000-0000-4000-8000-000000000d02','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000e03','00000000-0000-4000-8000-0000000000a3','00000000-0000-4000-8000-000000000d03','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000e04','00000000-0000-4000-8000-0000000000a4','00000000-0000-4000-8000-000000000d04','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000e05','00000000-0000-4000-8000-0000000000a5','00000000-0000-4000-8000-000000000d05','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000e06','00000000-0000-4000-8000-0000000000a6','00000000-0000-4000-8000-000000000d06','order.confirmed','{}'),
    ('00000000-0000-4000-8000-000000000ec1','00000000-0000-4000-8000-0000000000a1','00000000-0000-4000-8000-000000000d01','item.modified','{}'),
    ('00000000-0000-4000-8000-000000000ec2','00000000-0000-4000-8000-0000000000a4','00000000-0000-4000-8000-000000000d04','item.modified','{}'),
    ('00000000-0000-4000-8000-000000000ec3','00000000-0000-4000-8000-0000000000a3','00000000-0000-4000-8000-000000000d03','item.cancelled','{}')
) e(id,account,operation,kind,payload);
INSERT INTO app.kitchen_releases(id,source_event_id,order_id,account_id,unit_ids,state,scheduled_at,due_at,warnings,released_at)
SELECT r.id::uuid,r.id::uuid,r.order_id::uuid,r.account::uuid,r.units::uuid[],r.state,r.scheduled::timestamptz,r.scheduled::timestamptz,
    r.warnings::jsonb,r.released::timestamptz
FROM (VALUES
    ('00000000-0000-4000-8000-000000000e01','00000000-0000-4000-8000-000000000201','00000000-0000-4000-8000-0000000000a1',
     '{00000000-0000-4000-8000-000000000312}','blocked',NULL,'["Sin ruta de preparación: Coca Cola 600ml"]',NULL),
    ('00000000-0000-4000-8000-000000000e02','00000000-0000-4000-8000-000000000202','00000000-0000-4000-8000-0000000000a2',
     '{00000000-0000-4000-8000-000000000321}','blocked',NULL,'["Sin ruta de preparación: Coca Cola 600ml"]',NULL),
    ('00000000-0000-4000-8000-000000000e03','00000000-0000-4000-8000-000000000203','00000000-0000-4000-8000-0000000000a3',
     '{00000000-0000-4000-8000-000000000331}','blocked',NULL,'["Sin ruta de preparación: Coca Cola 600ml"]',NULL),
    ('00000000-0000-4000-8000-000000000e04','00000000-0000-4000-8000-000000000204','00000000-0000-4000-8000-0000000000a4',
     '{00000000-0000-4000-8000-000000000341}','blocked',NULL,'["Estación inactiva: Barra"]',NULL),
    ('00000000-0000-4000-8000-000000000e05','00000000-0000-4000-8000-000000000205','00000000-0000-4000-8000-0000000000a5',
     '{00000000-0000-4000-8000-000000000351}','scheduled','2026-11-27T02:00:00Z','[]',NULL),
    ('00000000-0000-4000-8000-000000000e06','00000000-0000-4000-8000-000000000206','00000000-0000-4000-8000-0000000000a6',
     '{00000000-0000-4000-8000-000000000361}','released',NULL,'[]','2026-11-25T20:30:00Z')
) r(id,order_id,account,units,state,scheduled,warnings,released);
INSERT INTO app.kitchen_unit_releases(unit_id,release_id,version_id) VALUES
    ('00000000-0000-4000-8000-000000000361','00000000-0000-4000-8000-000000000e06','00000000-0000-4000-8000-000000000296');
INSERT INTO app.kitchen_pending_changes(event_id,account_id,kind,payload,warnings) VALUES
    ('00000000-0000-4000-8000-000000000ec1','00000000-0000-4000-8000-0000000000a1','item.modified',
     '{"modified_unit_ids":["00000000-0000-4000-8000-000000000311"],"added_unit_ids":null,"cancelled_unit_ids":[]}',
     '["Aviso pendiente para estación inactiva: Cocina"]'),
    ('00000000-0000-4000-8000-000000000ec2','00000000-0000-4000-8000-0000000000a4','item.modified',
     '{"modified_unit_ids":["00000000-0000-4000-8000-000000000342"]}','["Aviso pendiente para estación inactiva: Cocina"]'),
    ('00000000-0000-4000-8000-000000000ec3','00000000-0000-4000-8000-0000000000a3','item.cancelled',
     '{"unit_ids":["00000000-0000-4000-8000-000000000332"],"reason":"Ya no lo quiso"}',
     '["Aviso pendiente para estación inactiva: Cocina"]');
INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,kind,amount,actor_id,device_id,operation_id)
SELECT '00000000-0000-4000-8000-0000000000b1','00000000-0000-4000-8000-0000000000a5','00000000-0000-4000-8000-0000000000f1',
    '00000000-0000-4000-8000-000000000021',id,name,true,'general',95,'00000000-0000-4000-8000-000000000011',
    '00000000-0000-4000-8000-000000000022','00000000-0000-4000-8000-000000000d08'
FROM app.payment_methods WHERE code='cash';
`
