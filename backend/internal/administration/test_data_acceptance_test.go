//go:build integration

package administration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/maintenance"
)

// recordingBackup sustituye al respaldo real: cuenta las llamadas y falla a pedido.
type recordingBackup struct {
	calls int
	fail  bool
}

func (backup *recordingBackup) Backup(context.Context) (string, error) {
	backup.calls++
	if backup.fail {
		return "", errors.New("disco lleno (simulado)")
	}
	return fmt.Sprintf("adel_pos_test_respaldo-20261113T000000Z-%06d.dump", backup.calls), nil
}

// labels resume una lista de la vista previa como «tipo:nombre».
func labels(entries []maintenance.Entry) []string {
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry.Kind+":"+entry.Name)
	}
	return result
}

// operationalCounts cuenta lo que «Quitar datos de prueba» nunca toca.
func operationalCounts(t *testing.T, env *delegationEnvironment) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{
		"restaurant_accounts", "restaurant_orders", "restaurant_items", "restaurant_item_versions",
		"restaurant_item_units", "restaurant_events", "kitchen_releases", "kitchen_tasks", "kitchen_tickets",
		"kitchen_print_jobs", "payments", "payment_documents", "cash_shifts", "cash_operations",
		"sales_recognitions", "inventory_movements",
	} {
		counts[table] = env.scalar(t, "SELECT count(*) FROM app."+table)
	}
	return counts
}

// testTestDataRemoval cubre la etapa 9F, fase A (§3 y §8): la vista previa exacta, el permiso, el
// respaldo previo (si falla no se borra nada), la idempotencia, los datos con operaciones que se
// desactivan sin tocar ventas, la configuración que se desvincula, un dato real con el mismo nombre
// genérico que no se toca y la categoría de prueba que sí se elimina.
func testTestDataRemoval(t *testing.T, env *delegationEnvironment) {
	backups := &recordingBackup{}
	env.service.Backups = backups
	defer func() {
		env.service.Backups = nil
	}()
	preview := func(actor terminal) maintenance.Preview {
		t.Helper()
		var value maintenance.Preview
		if err := json.Unmarshal(env.call(actor, "GET", "maintenance/test-data", nil, 200), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	remove := func(operationID, planID string, expected int) json.RawMessage {
		t.Helper()
		return env.call(env.admin, "POST", "maintenance/test-data/remove",
			map[string]any{"operation_id": operationID, "plan_id": planID}, expected)
	}
	removed := func(body json.RawMessage) maintenance.Result {
		t.Helper()
		var value maintenance.Result
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	kitchen := func(method, route string, body map[string]any) string {
		t.Helper()
		body["operation_id"] = events.NewID()
		return env.text(env.call(env.admin, method, route, body, 200), "id")
	}
	printer := func(name, destination string) string {
		return kitchen("POST", "kitchen/printers", map[string]any{
			"name": name, "transport": "agent", "destination": destination, "port": 0, "paper_width": 80,
			"copies": 1, "supports_cut": false, "cut": false, "cutter_option": "", "active": true, "fallback_id": nil,
		})
	}
	station := func(name, reception, printerID string) string {
		return kitchen("POST", "kitchen/stations", map[string]any{
			"name": name, "description": "", "active": true, "sort_order": 0, "reception": reception, "printer_id": printerID,
		})
	}
	catalog := func(route string, body map[string]any) string {
		t.Helper()
		return env.text(env.call(env.admin, "POST", route, body, 200), "id")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := env.pool.Exec(env.ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	eventCount := func(kind string) int {
		return env.scalar(t, "SELECT count(*) FROM app.realtime_events WHERE kind=$1", kind)
	}

	t.Run("9F_mantenimiento_exige_settings_manage", func(t *testing.T) {
		_, reader := env.createUser(t, "lector_mantenimiento_9f", env.createRole(t, "Lector de configuración 9F", "settings.read"))
		denied := env.deniedCount(t)
		env.call(reader, "GET", "maintenance/test-data", nil, 403)
		env.call(reader, "POST", "maintenance/test-data/remove",
			map[string]any{"operation_id": events.NewID(), "plan_id": strings.Repeat("a", 64)}, 403)
		if env.deniedCount(t) != denied+2 {
			t.Fatal("las denegaciones de mantenimiento no quedaron auditadas")
		}
		// Sin plan revisado no se quita nada.
		env.call(env.admin, "POST", "maintenance/test-data/remove", map[string]any{"operation_id": events.NewID(), "plan_id": "x"}, 400)
		env.call(env.admin, "POST", "maintenance/test-data/remove", map[string]any{"plan_id": strings.Repeat("a", 64)}, 400)
		if backups.calls != 0 {
			t.Fatal("se respaldó sin una operación válida")
		}
	})

	t.Run("9F_sin_datos_de_prueba", func(t *testing.T) {
		value := preview(env.admin)
		if !value.Empty() || len(value.Keep) != 0 || len(value.PlanID) != 64 {
			t.Fatalf("vista previa inicial: %+v", value)
		}
		result := removed(remove(events.NewID(), value.PlanID, 200))
		if result.Backup != "" || len(result.Removed)+len(result.Deactivated)+len(result.Unlinked) != 0 || backups.calls != 0 {
			t.Fatalf("sin datos de prueba no se respalda ni se quita nada: %+v (%d respaldos)", result, backups.calls)
		}
	})

	var realPrinter, realStation, category, tacos string
	t.Run("9F_dato_real_intacto_configuracion_desvinculada_y_respaldo_previo", func(t *testing.T) {
		// Datos del restaurante con nombres genéricos de las pruebas: no se tocan.
		realPrinter = printer("Impresora de cocina 9F", "Agente de cocina del restaurante")
		realStation = station("Cocina caliente", "printer", realPrinter)
		category = catalog("catalog/categories", map[string]any{"name": "Platillos", "active": true, "color": "#d64545"})
		tacos = catalog("catalog/products", map[string]any{
			"name": "Tacos de la casa", "active": true, "category_id": category, "code": "TACOS-9F", "kind": "product", "base_price": "95",
		})
		// La impresora de aceptación, como la dejó la prueba de humo de Windows: impresora de tickets,
		// cajón de «Caja 1» y respaldo de otra impresora.
		testPrinter := printer("Destino controlado E2E", "Agente de aceptación no disponible")
		exec("UPDATE app.payment_settings SET receipt_printer_id=$1", testPrinter)
		exec("UPDATE app.kitchen_printers SET fallback_id=$1 WHERE id=$2", testPrinter, realPrinter)
		exec(`INSERT INTO app.cash_drawer_settings(register_id, printer_id, enabled)
            SELECT id, $1, true FROM app.registers WHERE name='Caja 1'
            ON CONFLICT (register_id) DO UPDATE SET printer_id=excluded.printer_id, enabled=true`, testPrinter)

		value := preview(env.admin)
		if !slices.Equal(labels(value.Remove), []string{"printer:Destino controlado E2E"}) || len(value.Deactivate) != 0 || len(value.Keep) != 0 {
			t.Fatalf("vista previa: %+v", value)
		}
		if !slices.Equal(labels(value.Unlink), []string{
			"setting:Impresora de tickets de Cobros",
			"setting:Cajón de dinero de «Caja 1»",
			"setting:Respaldo de la impresora «Impresora de cocina 9F»",
		}) {
			t.Fatalf("configuración que se desvincula: %v", labels(value.Unlink))
		}
		for _, entry := range slices.Concat(value.Remove, value.Unlink) {
			if entry.Reason == "" || strings.Contains(entry.Reason, "_") {
				t.Fatalf("motivo vacío o técnico: %+v", entry)
			}
		}

		// Si el respaldo falla, no se borra nada y queda el intento en el registro de movimientos.
		backups.fail = true
		failed := env.call(env.admin, "POST", "maintenance/test-data/remove",
			map[string]any{"operation_id": events.NewID(), "plan_id": value.PlanID}, 503)
		if !strings.Contains(string(failed), "BACKUP_FAILED") || strings.Contains(string(failed), "disco lleno") {
			t.Fatalf("respuesta del respaldo fallido: %s", failed)
		}
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_printers WHERE id=$1", testPrinter) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.payment_settings WHERE receipt_printer_id=$1", testPrinter) != 1 {
			t.Fatal("se quitaron datos aunque el respaldo falló")
		}
		if env.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='maintenance.test_data_removed' AND NOT success
            AND after_state->>'backup'='failed'`) != 1 {
			t.Fatal("el respaldo fallido no quedó en el registro de movimientos")
		}
		backups.fail = false

		before := map[string]int{}
		for _, kind := range []string{"kitchen.printers.changed", "payments.settings.changed", "cash.settings.changed", "maintenance.test_data.removed"} {
			before[kind] = eventCount(kind)
		}
		operationID := events.NewID()
		body := remove(operationID, value.PlanID, 200)
		result := removed(body)
		if backups.calls != 2 || result.Backup != "adel_pos_test_respaldo-20261113T000000Z-000002.dump" {
			t.Fatalf("respaldo previo: %d llamadas, %q", backups.calls, result.Backup)
		}
		if !slices.Equal(labels(result.Removed), labels(value.Remove)) || !slices.Equal(labels(result.Unlinked), labels(value.Unlink)) {
			t.Fatalf("resultado distinto de la vista previa: %+v", result)
		}
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_printers WHERE id=$1", testPrinter) != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.payment_settings WHERE receipt_printer_id IS NOT NULL") != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.cash_drawer_settings WHERE printer_id IS NOT NULL OR enabled") != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_printers WHERE id=$1 AND fallback_id IS NULL AND active", realPrinter) != 1 {
			t.Fatal("la impresora de prueba o su configuración siguen ahí")
		}
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_stations WHERE id=$1 AND name='Cocina caliente' AND active", realStation) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.catalog_categories WHERE id=$1 AND active", category) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.catalog_products WHERE id=$1 AND active", tacos) != 1 {
			t.Fatal("se tocó un dato del restaurante con un nombre genérico de las pruebas")
		}
		if env.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='maintenance.test_data_removed' AND success
            AND after_state->>'backup'=$1 AND jsonb_array_length(after_state->'removed')=1`, result.Backup) != 1 {
			t.Fatal("la operación no quedó auditada con la lista y el respaldo")
		}
		for kind, count := range before {
			if eventCount(kind) <= count {
				t.Fatalf("falta el evento %s para actualizar las pantallas", kind)
			}
		}
		// Idempotencia: la misma operación devuelve lo guardado sin otro respaldo; con otro plan, conflicto.
		again := remove(operationID, value.PlanID, 200)
		if removed(again).Backup != result.Backup || backups.calls != 2 {
			t.Fatal("la repetición volvió a respaldar o devolvió otro resultado")
		}
		env.call(env.admin, "POST", "maintenance/test-data/remove",
			map[string]any{"operation_id": operationID, "plan_id": strings.Repeat("f", 64)}, 409)
		if !preview(env.admin).Empty() {
			t.Fatal("después de quitar todavía hay datos de prueba")
		}
		// Un plan que ya no coincide con los datos no quita nada.
		env.call(env.admin, "POST", "maintenance/test-data/remove",
			map[string]any{"operation_id": events.NewID(), "plan_id": value.PlanID}, 409)
	})

	t.Run("9F_datos_con_operaciones_se_desactivan_sin_tocar_ventas", func(t *testing.T) {
		exec("UPDATE app.kitchen_stations SET name='Parrilla 9F' WHERE id=$1", realStation)
		testPrinter := printer("Destino controlado E2E", "Agente de aceptación no disponible")
		hot := station("Cocina caliente", "printer", testPrinter)
		fryers := station("Freidoras", "both", testPrinter)
		kitchen("POST", "kitchen/routes", map[string]any{"category_id": category, "mode": "prepare", "station_ids": []string{hot, fryers}})
		tacosRoute := kitchen("POST", "kitchen/routes", map[string]any{"product_id": tacos, "mode": "prepare", "station_ids": []string{fryers, realStation}})
		kitchen("PUT", "kitchen/access", map[string]any{"device_id": env.device.id, "station_ids": []string{hot}, "expected_station_ids": []string{}})
		steak := catalog("catalog/products", map[string]any{
			"name": "Filete de prueba", "active": true, "category_id": category, "code": "FILETE", "kind": "product", "base_price": "150",
		})
		// Una venta enviada a cocina: tareas en ambas estaciones y un trabajo para la impresora de prueba.
		opened := env.call(env.admin, "POST", "restaurant/accounts", map[string]any{"operation_id": events.NewID(), "mode": "takeaway", "name": "Cliente 9F"}, 200)
		var account struct {
			Account struct {
				ID       string `json:"id"`
				Revision int    `json:"revision"`
			} `json:"account"`
		}
		if err := json.Unmarshal(opened, &account); err != nil {
			t.Fatal(err)
		}
		route := "restaurant/accounts/" + account.Account.ID
		drafted := env.call(env.admin, "PUT", route+"/draft", map[string]any{
			"operation_id": events.NewID(), "revision": account.Account.Revision,
			"items": []map[string]any{{"id": events.NewID(), "product_id": steak, "variant_id": nil, "quantity": 2}},
		}, 200)
		if err := json.Unmarshal(drafted, &account); err != nil {
			t.Fatal(err)
		}
		fingerprint := env.text(env.call(env.admin, "GET", route+"/preview", nil, 200), "fingerprint")
		env.call(env.admin, "POST", route+"/confirm", map[string]any{
			"operation_id": events.NewID(), "revision": account.Account.Revision, "expected_fingerprint": fingerprint,
		}, 200)
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_tasks WHERE station_id=$1", hot) == 0 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_tasks WHERE station_id=$1", fryers) == 0 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_print_jobs WHERE printer_id=$1", testPrinter) == 0 {
			t.Fatal("la venta no llegó a las estaciones ni a la impresora de prueba")
		}

		var terminalName string
		if err := env.pool.QueryRow(env.ctx, "SELECT name FROM app.devices WHERE id=$1", env.device.id).Scan(&terminalName); err != nil {
			t.Fatal(err)
		}
		value := preview(env.admin)
		if !slices.Equal(labels(value.Remove), []string{"route:Ruta de la categoría «Platillos»"}) {
			t.Fatalf("se elimina: %v", labels(value.Remove))
		}
		if !slices.Equal(labels(value.Deactivate), []string{
			"station:Cocina caliente", "station:Freidoras", "product:Filete de prueba", "printer:Destino controlado E2E",
		}) {
			t.Fatalf("se desactiva: %v", labels(value.Deactivate))
		}
		for _, entry := range value.Deactivate {
			if !strings.HasPrefix(entry.Reason, "Ya tiene registros: ") {
				t.Fatalf("motivo de desactivar: %+v", entry)
			}
		}
		if !strings.Contains(value.Deactivate[2].Reason, "ventas") || !strings.Contains(value.Deactivate[3].Reason, "trabajos de impresión") {
			t.Fatalf("motivos: %+v", value.Deactivate)
		}
		if !slices.Equal(labels(value.Unlink), []string{
			"route:Ruta del producto «Tacos de la casa»", "setting:Estaciones de la terminal «" + terminalName + "»",
		}) {
			t.Fatalf("se desvincula: %v", labels(value.Unlink))
		}
		if !slices.Equal(labels(value.Keep), []string{"category:Platillos"}) ||
			value.Keep[0].Reason != "Tiene otros productos del restaurante, como «Tacos de la casa»." {
			t.Fatalf("se conserva: %+v", value.Keep)
		}

		counts := operationalCounts(t, env)
		result := removed(remove(events.NewID(), value.PlanID, 200))
		if result.Backup == "" || backups.calls != 3 {
			t.Fatal("no hubo respaldo previo")
		}
		if after := operationalCounts(t, env); !maps.Equal(counts, after) {
			t.Fatalf("se tocaron registros de operación: %v → %v", counts, after)
		}
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_printers WHERE id=$1 AND NOT active", testPrinter) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_stations WHERE id = ANY($1::uuid[]) AND NOT active", []string{hot, fryers}) != 2 ||
			env.scalar(t, "SELECT count(*) FROM app.catalog_products WHERE id=$1 AND NOT active AND archived", steak) != 1 {
			t.Fatal("los datos de prueba con operaciones debían quedar desactivados")
		}
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_routes WHERE category_id=$1", category) != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_route_stations WHERE route_id=$1", tacosRoute) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_route_stations WHERE route_id=$1 AND station_id=$2", tacosRoute, realStation) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_device_stations WHERE station_id = ANY($1::uuid[])", []string{hot, fryers}) != 0 {
			t.Fatal("las rutas o las estaciones por terminal siguen apuntando a estaciones de prueba")
		}
		if env.scalar(t, "SELECT count(*) FROM app.catalog_categories WHERE id=$1 AND active", category) != 1 {
			t.Fatal("se tocó la categoría con productos del restaurante")
		}
		if !preview(env.admin).Empty() {
			t.Fatal("lo desactivado volvió a aparecer: quitar debe ser idempotente")
		}
	})

	t.Run("9F_categoria_de_prueba_se_elimina_con_su_ruta", func(t *testing.T) {
		// La venta anterior conserva su producto (desactivado); se aparta para repetir la firma.
		exec("UPDATE app.catalog_products SET name='Filete vendido 9F', code='FILETE-VENDIDO-9F' WHERE code='FILETE'")
		exec("UPDATE app.catalog_categories SET name='Platillos del restaurante' WHERE id=$1", category)
		testCategory := catalog("catalog/categories", map[string]any{"name": "Platillos", "active": true, "color": "#d64545"})
		steak := catalog("catalog/products", map[string]any{
			"name": "Filete de prueba", "active": true, "category_id": testCategory, "code": "FILETE", "kind": "product", "base_price": "150",
		})
		kitchen("POST", "kitchen/routes", map[string]any{"category_id": testCategory, "mode": "none", "station_ids": []string{}})
		value := preview(env.admin)
		if !slices.Equal(labels(value.Remove), []string{
			"product:Filete de prueba", "category:Platillos", "route:Ruta de la categoría «Platillos»",
		}) || len(value.Deactivate) != 0 || len(value.Unlink) != 0 {
			t.Fatalf("vista previa: %+v", value)
		}
		removed(remove(events.NewID(), value.PlanID, 200))
		if env.scalar(t, "SELECT count(*) FROM app.catalog_products WHERE id=$1", steak) != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.catalog_categories WHERE id=$1", testCategory) != 0 ||
			env.scalar(t, "SELECT count(*) FROM app.kitchen_routes WHERE category_id=$1", testCategory) != 0 {
			t.Fatal("la categoría de prueba, su producto o su ruta siguen ahí")
		}
		if env.scalar(t, "SELECT count(*) FROM app.catalog_categories WHERE id=$1", category) != 1 ||
			env.scalar(t, "SELECT count(*) FROM app.catalog_products WHERE code='FILETE-VENDIDO-9F'") != 1 {
			t.Fatal("se tocó la categoría del restaurante o el producto vendido")
		}
	})

	t.Run("9F_respaldo_real_con_pg_dump", func(t *testing.T) {
		bin := os.Getenv("ADEL_PG_BIN")
		if bin == "" {
			bin = "/usr/local/opt/postgresql@18/bin"
			if runtime.GOARCH == "arm64" {
				bin = "/opt/homebrew/opt/postgresql@18/bin"
			}
		}
		tool := "pg_dump"
		if runtime.GOOS == "windows" {
			tool += ".exe"
		}
		if _, err := os.Stat(filepath.Join(bin, tool)); err != nil {
			t.Skip("sin pg_dump: define ADEL_PG_BIN para comprobar el respaldo real")
		}
		dsn, err := url.Parse(os.Getenv("ADEL_TEST_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(dsn.Port())
		password, _ := dsn.User.Password()
		database := config.Database{
			Host: dsn.Hostname(), Port: port, User: dsn.User.Username(), Password: password,
			Name: strings.TrimPrefix(dsn.Path, "/"), SSLMode: "disable",
		}
		directory := filepath.Join(t.TempDir(), "backups")
		backup := maintenance.NewDumpBackup(config.Config{Database: database, PostgresBin: bin, BackupDirectory: directory}, "")
		name, err := backup.Backup(env.ctx)
		if err != nil {
			t.Fatal(err)
		}
		// El mismo nombre que scripts/database.py backup: <base>-<AAAAMMDDTHHMMSSZ>-<6 hex>.dump.
		if !regexp.MustCompile(`^` + regexp.QuoteMeta(database.Name) + `-\d{8}T\d{6}Z-[0-9a-f]{6}\.dump$`).MatchString(name) {
			t.Fatalf("nombre del respaldo: %s", name)
		}
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("respaldo vacío o ausente: %v", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("permisos del respaldo: %v", info.Mode().Perm())
		}
	})
}
