//go:build integration

package legacyimport

import (
	"context"
	"errors"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/database"
)

// testDatabase abre la base temporal (exige un nombre adel_pos_test_*), la deja vacía y la migra.
func testDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// installation imita una instalación nueva después del asistente inicial: establecimiento, roles, el administrador
// (creado en la misma transacción que el establecimiento), sa, «Caja 1» y la terminal. Devuelve el administrador y la
// terminal.
func installation(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`INSERT INTO app.establishment(name, timezone) VALUES ('Fonda de prueba 9H', 'America/Mexico_City')`,
		`INSERT INTO app.roles(code, name, reserved) VALUES ('administrator','Administrador',false), ('supervisor','Supervisor',false),
            ('cashier','Cajero',false), ('waiter','Mesero',false), ('kitchen','Cocina',false), ('inventory','Inventarios',false),
            ('finance','Finanzas',false), ('support','Soporte reservado',true) ON CONFLICT DO NOTHING`,
		`INSERT INTO app.users(username, display_name, role_id, support) SELECT 'sa', 'Soporte', id, true FROM app.roles WHERE code = 'support'
            ON CONFLICT DO NOTHING`,
		`INSERT INTO app.users(username, display_name, password_hash, role_id, initial_password)
            SELECT 'admin', 'Administrador', 'hash-de-prueba', id, true FROM app.roles WHERE code = 'administrator'`,
		`INSERT INTO app.registers(name) VALUES ('Caja 1')`,
		`INSERT INTO app.devices(name, secret_hash, register_id) SELECT 'Caja principal', 'hash-9h-c', id FROM app.registers`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var admin, device string
	if err := tx.QueryRow(ctx, `SELECT (SELECT id::text FROM app.users WHERE username = 'admin'),
            (SELECT id::text FROM app.devices LIMIT 1)`).Scan(&admin, &device); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return admin, device
}

// importSynthetic lee un paquete en memoria, hace la correspondencia e importa.
func importSynthetic(t *testing.T, pool *pgxpool.Pool, pkg *syntheticPackage, admin, device, expectedBatch string, hooks *Hooks) (*Report, error) {
	t.Helper()
	ctx := context.Background()
	environment, err := ReadEnvironment(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	model, err := Load(ctx, pkg.source(), environment.Config.Location, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, _ := Analyze(model, environment, 1234, strings.Repeat("cd", 32))
	return Import(ctx, pool, plan, environment.Config, ImportRequest{
		UserID: admin, Username: "admin", DeviceID: device, ExpectedBatchID: expectedBatch,
		PackageSHA256: strings.Repeat("cd", 32), PackageBytes: 1234, BackupFile: "respaldo-de-prueba.dump", Hooks: hooks,
	}, nil)
}

func queryText(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

// snapshot resume el lote vigente y todo el historial, para comprobar que un intento fallido no cambió nada.
func snapshot(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	parts := []string{queryText(t, pool, `SELECT coalesce(string_agg(id::text || ':' || status, ',' ORDER BY seq), '') FROM app.legacy_import_batches`)}
	for _, table := range append([]string{"legacy_id_map"}, historyTables...) {
		parts = append(parts, table+"="+queryText(t, pool, "SELECT count(*)::text || '/' || coalesce(count(DISTINCT batch_id), 0)::text FROM app."+table))
	}
	parts = append(parts, queryText(t, pool, `SELECT coalesce(sum(net), 0)::text FROM app.legacy_sales_recognitions`))
	parts = append(parts, queryText(t, pool, `SELECT count(*)::text || ':' || coalesce(string_agg(name || '/' || base_price, ',' ORDER BY code), '')
        FROM app.catalog_products`))
	parts = append(parts, queryText(t, pool, `SELECT count(*)::text FROM app.restaurant_tables`))
	parts = append(parts, queryText(t, pool, `SELECT string_agg(username || '/' || active, ',' ORDER BY username) FROM app.users`))
	return strings.Join(parts, " | ")
}

func TestLegacyImportMapsHistoryExactly(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	folioSequence := queryText(t, pool, `SELECT pg_get_serial_sequence('app.restaurant_accounts', 'folio')`)
	sequenceBefore := queryText(t, pool, "SELECT last_value::text || '/' || is_called::text FROM "+folioSequence)
	environment, err := ReadEnvironment(ctx, pool)
	if err != nil || len(environment.Blockers) != 0 || environment.CurrentBatch != nil {
		t.Fatalf("una instalación nueva es limpia: %+v %v", environment.Blockers, err)
	}
	report, err := importSynthetic(t, pool, standardSynthetic(), admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validation.OK || report.Validation.HasMoneyDifferences {
		for _, check := range report.Validation.Checks {
			if !check.OK {
				t.Errorf("validación: %+v", check)
			}
		}
		t.Fatal("la validación de un paquete correcto debía cuadrar")
	}
	// Importes exactos, sin recalcular.
	expectations := map[string]string{
		`SELECT status || '/' || (recognized_at AT TIME ZONE 'UTC')::text || '/' || tax::text || '/' || tax_estimated::text FROM app.legacy_sales WHERE legacy_id = 9`: "paid/2026-01-04 16:00:00/4.1379/true",
		`SELECT amount_due::text || '/' || lines_amount::text || '/' || array_to_string(flags, ',') FROM app.legacy_sales WHERE legacy_id = 5`:                         "100.0000/95.5000/total_mismatch,subtotal_mismatch",
		`SELECT status || '/' || coalesce(recognized_at::text, '-') || '/' || (cancelled_at AT TIME ZONE 'UTC')::text FROM app.legacy_sales WHERE legacy_id = 3`:       "cancelled/-/2024-03-15 21:20:00",
		`SELECT amount::text || '/' || array_to_string(flags, ',') FROM app.legacy_payments WHERE legacy_id = 8`:                                                       "-1.0000/negative_amount",
		`SELECT method || '/' || method_name FROM app.legacy_payments WHERE legacy_id = 2`:                                                                             "card/Tarjeta (sistema anterior)",
		`SELECT array_to_string(flags, ',') FROM app.legacy_payments WHERE legacy_id = 6`:                                                                              "possible_duplicate",
		`SELECT total::text || '/' || array_to_string(flags, ',') FROM app.legacy_line_complements WHERE position = 1`:                                                 "-20.0000/negative_amount",
		`SELECT complements_total::text || '/' || product_code || '/' || category_name FROM app.legacy_sale_lines WHERE legacy_id = 3`:                                 "5.0000/SA-4/Platillos",
		`SELECT notes FROM app.legacy_sale_lines WHERE legacy_id = 2`:                                                                                                  "Sin \"crema\"\tbien | caliente",
		`SELECT amount::text || '/' || array_to_string(flags, ',') FROM app.legacy_cash_withdrawals WHERE legacy_id = 3`:                                               "150000.0000/doubtful_amount",
		`SELECT status || '/' || array_to_string(flags, ',') FROM app.legacy_cash_shifts WHERE legacy_id = 2`:                                                          "closed/sales_mismatch,withdrawals_mismatch",
		`SELECT original_quantity::text || '>' || new_quantity::text || '/' || legacy_sale_id FROM app.legacy_quantity_changes WHERE position = 1`:                     "1.000>0.850/11",
		`SELECT type_name || '/' || amount::text FROM app.legacy_discounts WHERE legacy_sale_id = 6`:                                                                   "Atencion/5.0000",
		`SELECT count(*)::text FROM app.legacy_price_history`:                                                                                                          "11",
		// El libro: la diferencia de la venta 5, el cargo y el descuento de la 6; nada de la cancelada ni de la abierta.
		`SELECT net::text || '/' || concept FROM app.legacy_sales_recognitions WHERE entry_kind = 'difference'`:                                      "4.5000/Diferencia del sistema anterior",
		`SELECT string_agg(entry_kind || '=' || net::text, ',' ORDER BY sequence) FROM app.legacy_sales_recognitions WHERE legacy_sale_id = 6`:       "line=60.0000,discount=-5.0000,charge=12.5000",
		`SELECT count(*)::text FROM app.legacy_sales_recognitions WHERE legacy_sale_id IN (3, 4)`:                                                    "0",
		`SELECT sum(net)::text FROM app.legacy_sales_recognitions`:                                                                                   "583.5000",
		`SELECT gross::text || '/' || tax::text || '/' || base::text || '/' || tax_rate FROM app.legacy_sales_recognitions WHERE legacy_line_id = 8`: "30.0000/4.1379/25.8621/16",
	}
	for query, expected := range expectations {
		if got := queryText(t, pool, query); got != expected {
			t.Errorf("%s\n  = %q\n  se esperaba %q", query, got, expected)
		}
	}
	// Por venta: Σ net del libro = lo que se debía y Σ tax = el IVA de la venta.
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.legacy_sales s JOIN (SELECT legacy_sale_id, sum(net) n, sum(tax) x
        FROM app.legacy_sales_recognitions GROUP BY 1) l ON l.legacy_sale_id = s.legacy_id WHERE l.n <> s.amount_due OR l.x <> s.tax`); got != "0" {
		t.Fatalf("ventas cuyo libro no suma lo guardado: %s", got)
	}
	// Lo vivo: catálogo, mesas y usuarios.
	live := map[string]string{
		`SELECT string_agg(code || ':' || base_price || ':' || tax_rate || ':' || price_can_decrease || ':' || active, ',' ORDER BY code) FROM app.catalog_products`:                                                                         "SA-1:30.00:0:false:true,SA-2:30.00:16:false:true,SA-3:95.50:0:false:true,SA-4:150.00:0:true:true,SA-5:110.01:8:false:true,SA-6:0.00:0:false:false",
		`SELECT string_agg(name, ',' ORDER BY sort_order) FROM app.catalog_categories`:                                                                                                                                                       "Bebidas,Platillos,Postres",
		`SELECT string_agg(g.name || '=' || o.reference_value || '+' || o.extra_price, ',' ORDER BY g.name, o.reference_value) FROM app.catalog_group_options o JOIN app.catalog_option_groups g ON g.id = o.group_id`:                       "Complementos=15.00+0.00,Complementos=20.00+0.00,Complementos=25.00+0.00,Complementos: Papas=25.00+0.00",
		`SELECT count(*)::text FROM app.catalog_group_defaults WHERE quantity = 1`:                                                                                                                                                           "2",
		`SELECT string_agg(a.name || '/' || t.name || '/' || t.seats, ',' ORDER BY t.x, t.y) FROM app.restaurant_tables t JOIN app.restaurant_areas a ON a.id = t.area_id`:                                                                   "Salón/Mesa 1/4,Salón/Mesa 2/4,Salón/Terraza grande/40",
		`SELECT string_agg(u.username || '/' || r.code || '/' || u.active || '/' || (u.password_hash IS NULL), ',' ORDER BY u.username) FROM app.users u JOIN app.roles r ON r.id = u.role_id WHERE NOT u.support AND u.username <> 'admin'`: "admin-2/administrator/true/true,jose/waiter/false/true,u1beto/waiter/true/true",
		`SELECT count(*)::text FROM app.user_preferences`: "3",
	}
	for query, expected := range live {
		if got := queryText(t, pool, query); got != expected {
			t.Errorf("%s\n  = %q\n  se esperaba %q", query, got, expected)
		}
	}
	// La operación, la caja, los turnos y el inventario no cambian; la numeración de folios sigue igual.
	for _, table := range []string{"restaurant_accounts", "payments", "cash_shifts", "cash_movements", "inventory_movements",
		"sales_recognitions", "account_cancellations"} {
		if got := queryText(t, pool, "SELECT count(*)::text FROM app."+table); got != "0" {
			t.Fatalf("la importación escribió en %s: %s", table, got)
		}
	}
	if after := queryText(t, pool, "SELECT last_value::text || '/' || is_called::text FROM "+folioSequence); after != sequenceBefore {
		t.Fatalf("los folios de AD-EL POS cambiaron: %s → %s", sequenceBefore, after)
	}
	checkReadOnly(t, pool)
	checkQuotesLikeLegacy(t, pool)
}

// checkReadOnly: fuera de la importación, el historial rechaza INSERT, UPDATE, DELETE y TRUNCATE (también con una
// marca de importación que no corresponde a un lote en curso).
func checkReadOnly(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		"UPDATE app.legacy_payments SET amount = amount + 1",
		"DELETE FROM app.legacy_sales",
		"INSERT INTO app.legacy_discounts(batch_id, position, legacy_sale_id, amount) SELECT id, 99, 1, 1 FROM app.legacy_import_batches",
		"TRUNCATE app.legacy_sales_recognitions",
		"UPDATE app.legacy_import_batches SET has_differences = true",
		"DELETE FROM app.legacy_id_map",
	}
	for _, guarded := range []bool{false, true} {
		for _, statement := range statements {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if guarded {
				// Un identificador válido, pero de un lote que no está en curso: tampoco basta.
				if _, err := tx.Exec(ctx, `SELECT set_config('adel.legacy_import', id::text, true) FROM app.legacy_import_batches`); err != nil {
					t.Fatal(err)
				}
			}
			_, err = tx.Exec(ctx, statement)
			_ = tx.Rollback(ctx)
			var problem *pgconn.PgError
			if !errors.As(err, &problem) || problem.Code != "55000" {
				t.Fatalf("el historial aceptó %q (marca %v): %v", statement, guarded, err)
			}
		}
	}
}

// checkQuotesLikeLegacy: el motor real de AD-EL POS cobra un producto importado como el anterior,
// (pedido − incluido) × precio por complemento, también con la bonificación al quitar uno incluido.
func checkQuotesLikeLegacy(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var product, group string
	err := pool.QueryRow(ctx, `SELECT p.id::text, g.id::text FROM app.catalog_products p
        JOIN app.catalog_product_groups pg ON pg.product_id = p.id JOIN app.catalog_option_groups g ON g.id = pg.group_id
        WHERE p.code = 'SA-4'`).Scan(&product, &group)
	if err != nil {
		t.Fatal(err)
	}
	options := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT c.name, o.id::text FROM app.catalog_group_options o JOIN app.catalog_complements c ON c.id = o.complement_id
        WHERE o.group_id = $1`, group)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			t.Fatal(err)
		}
		options[name] = id
	}
	rows.Close()
	cases := []struct {
		label                 string
		arroz, ensalada, papa string
		expected              string
	}{
		{"incluidos", "1", "1", "0", "150.00"},
		{"quitar el arroz (bonificación de $20)", "0", "1", "0", "130.00"},
		{"quitar el arroz y agregar papas (como la venta 2)", "0", "1", "1", "155.00"},
		{"agregar papas", "1", "1", "1", "175.00"},
		{"dos ensaladas", "1", "2", "0", "165.00"},
	}
	for _, item := range cases {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		quote, _, err := catalog.QuoteForOrder(ctx, tx, catalog.PreviewRequest{Lines: []catalog.LineRequest{{
			ProductID: product, Quantity: 1, Customization: catalog.Customization{Groups: []catalog.GroupSelection{{
				GroupID: group, Options: []catalog.IncludedOption{
					{OptionID: options["Arroz"], Quantity: item.arroz},
					{OptionID: options["Ensalada"], Quantity: item.ensalada},
					{OptionID: options["Papas"], Quantity: item.papa},
				}}}},
		}}})
		_ = tx.Rollback(ctx)
		if err != nil {
			t.Fatalf("%s: %v", item.label, err)
		}
		if quote.Lines[0].UnitPrice != item.expected {
			t.Fatalf("%s: AD-EL POS cobra %s y el anterior %s", item.label, quote.Lines[0].UnitPrice, item.expected)
		}
	}
}

func TestLegacyImportReplacesTheBatchCompletely(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	first, err := importSynthetic(t, pool, standardSynthetic(), admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Entre importaciones: el administrador asigna una contraseña a un usuario importado y le agrega una receta…
	// aquí basta con la contraseña (el reemplazo la conserva) y con los identificadores de lo importado.
	if _, err := pool.Exec(ctx, `UPDATE app.users SET password_hash = 'asignada', initial_password = true WHERE username = 'u1beto'`); err != nil {
		t.Fatal(err)
	}
	ids := queryText(t, pool, `SELECT string_agg(code || '=' || id, ',' ORDER BY code) FROM app.catalog_products`)
	userID := queryText(t, pool, `SELECT id::text FROM app.users WHERE username = 'u1beto'`)
	// El respaldo nuevo: el agua cambió de nombre y de precio, la «Terraza grande» ya no existe, hay una venta más y
	// la cajera «José» dejó de existir.
	updated := standardSynthetic()
	updated.table("productos").rows[0][2] = "Agua de jamaica"
	updated.table("productos_precios").add(map[string]any{"producto": 1, "fecha_inicio": "2026-11-01T08:00:00",
		"subtotal": money("35.0000"), "iva": money("0.0000"), "total": money("35.0000")})
	updated.table("mesas").rows = updated.table("mesas").rows[:3]
	updated.table("usuarios").rows = updated.table("usuarios").rows[:2]
	updated.table("ventas").add(map[string]any{"id": 12, "caja": 1, "mesa": 2, "folio": "12", "fecha": "2026-11-14T19:00:00",
		"cajero": 1, "subtotal": money("35.0000"), "iva": money("0.0000"), "total": money("35.0000"), "pagado": 1, "corte": 3})
	updated.table("ventas_productos").add(map[string]any{"id": 13, "venta": 12, "producto": 1, "cantidad": money("1.000"),
		"subtotal": money("35.0000"), "iva": money("0.0000"), "total": money("35.0000")})
	updated.table("pagos").add(map[string]any{"id": 12, "venta": 12, "tipo": 1, "monto": money("35.0000"), "recibio": 1,
		"fecha": "2026-11-14T19:30:00", "corte": 3})
	// La confirmación de reemplazo debe nombrar el lote vigente.
	if _, err := importSynthetic(t, pool, updated, admin, device, "", nil); CodeOf(err) != CodeBatchChanged {
		t.Fatalf("reemplazar sin confirmar el lote vigente: %v", err)
	}
	second, err := importSynthetic(t, pool, updated, admin, device, first.BatchID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Validation.OK || second.ReplacedBatchID != first.BatchID {
		t.Fatalf("segundo lote: %+v", second.Validation)
	}
	// El informe del reemplazo (y no el de la primera importación) avisa qué pasa al repetir la migración.
	for _, warning := range replaceWarnings() {
		if !slices.Contains(second.Warnings, warning) || slices.Contains(first.Warnings, warning) {
			t.Fatalf("aviso de reemplazo: primera %q, segunda %q", first.Warnings, second.Warnings)
		}
	}
	environment, err := ReadEnvironment(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, preview := Analyze(loadSynthetic(t, updated), environment, 1234, strings.Repeat("cd", 32)); !preview.WillReplace ||
		!slices.Contains(preview.Warnings, replaceWarnings()[0]) {
		t.Fatalf("el resumen previo de un reemplazo debía avisarlo: %+v", preview.Warnings)
	}
	if got := queryText(t, pool, `SELECT string_agg(status, ',' ORDER BY seq) FROM app.legacy_import_batches`); got != "replaced,completed" {
		t.Fatalf("estados de los lotes: %s", got)
	}
	for _, table := range append([]string{"legacy_id_map"}, historyTables...) {
		if got := queryText(t, pool, "SELECT count(*)::text FROM app."+table+" WHERE batch_id = $1", first.BatchID); got != "0" {
			t.Fatalf("quedaron filas del lote reemplazado en %s: %s", table, got)
		}
	}
	if got := queryText(t, pool, `SELECT count(*)::text || '/' || sum(net)::text FROM app.legacy_sales_recognitions`); got != "14/618.5000" {
		t.Fatalf("libro del lote nuevo: %s", got)
	}
	// Los mismos identificadores en su lugar, con los datos nuevos; la contraseña asignada se conserva.
	if after := queryText(t, pool, `SELECT string_agg(code || '=' || id, ',' ORDER BY code) FROM app.catalog_products`); after != ids {
		t.Fatalf("los productos cambiaron de identificador:\n%s\n%s", ids, after)
	}
	if got := queryText(t, pool, `SELECT name || '/' || base_price FROM app.catalog_products WHERE code = 'SA-1'`); got != "Agua de jamaica/35.00" {
		t.Fatalf("producto actualizado: %s", got)
	}
	if got := queryText(t, pool, `SELECT id::text || '/' || password_hash || '/' || initial_password FROM app.users WHERE username = 'u1beto'`); got != userID+"/asignada/true" {
		t.Fatalf("usuario actualizado en su lugar: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.restaurant_tables`); got != "2" {
		t.Fatalf("la mesa que ya no existe debía borrarse: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.users WHERE username = 'jose'`); got != "0" {
		t.Fatalf("el usuario que ya no existe debía borrarse: %s", got)
	}
	environment, err = ReadEnvironment(ctx, pool)
	if err != nil || len(environment.Blockers) != 0 || environment.CurrentBatch == nil || environment.CurrentBatch.ID != second.BatchID {
		t.Fatalf("después del reemplazo la instalación sigue limpia: %+v %v", environment.Blockers, err)
	}
	// Un usuario importado que inició sesión no se puede borrar: al retirarlo queda desactivado y en la
	// correspondencia como «retired».
	if _, err := pool.Exec(ctx, `INSERT INTO app.sessions(user_id, device_id, token_hash, expires_at)
        SELECT id, $1, 'sesion-9h-c', now() + interval '1 hour' FROM app.users WHERE username = 'u1beto'`, device); err != nil {
		t.Fatal(err)
	}
	third := standardSynthetic()
	third.table("usuarios").rows = third.table("usuarios").rows[:1]
	report, err := importSynthetic(t, pool, third, admin, device, second.BatchID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Retired != 1 {
		t.Fatalf("retirados: %d", report.Retired)
	}
	if got := queryText(t, pool, `SELECT username || '/' || active FROM app.users WHERE id = $1`, userID); !strings.HasPrefix(got, "u1beto.retirado-") ||
		!strings.HasSuffix(got, "/false") {
		t.Fatalf("usuario retirado: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text FROM app.sessions WHERE token_hash = 'sesion-9h-c' AND revoked_at IS NULL`); got != "0" {
		t.Fatal("el usuario retirado conservó su sesión")
	}
	environment, err = ReadEnvironment(ctx, pool)
	if err != nil || len(environment.Blockers) != 0 {
		t.Fatalf("lo retirado sigue siendo del lote: %+v %v", environment.Blockers, err)
	}
	// Otro reemplazo sin ese usuario lo vuelve a retirar con una sola terminación en el nombre (revisión de la Fase C,
	// M-4: antes quedaba «u1beto.retirado-x.retirado-x»).
	retiredName := queryText(t, pool, `SELECT username FROM app.users WHERE id = $1`, userID)
	fourth, err := importSynthetic(t, pool, third, admin, device, report.BatchID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fourth.Retired != 1 || !fourth.Validation.OK {
		t.Fatalf("cuarto lote: retirados %d, validación %+v", fourth.Retired, fourth.Validation)
	}
	if got := queryText(t, pool, `SELECT username || '/' || active FROM app.users WHERE id = $1`, userID); got != retiredName+"/false" ||
		strings.Count(got, ".retirado-") != 1 {
		t.Fatalf("el usuario retirado otra vez: %s (antes %s)", got, retiredName)
	}
	if got := queryText(t, pool, `SELECT state FROM app.legacy_id_map WHERE batch_id = $1 AND new_id = $2`, fourth.BatchID, userID); got != "retired" {
		t.Fatalf("el usuario retirado en la correspondencia nueva: %s", got)
	}
}

func TestLegacyImportRefusesOwnData(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	// Operación propia: un turno de caja (dentro de una transacción que no se confirma).
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id, actor_id, device_id, route, fingerprint, response)
        VALUES ('00000000-0000-4000-8000-0000000009c1', $1, $2, 'payments/shifts/open', 'x', '{}')`, admin, device)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.cash_shifts(id, register_id, opening_float, opened_by, opened_device_id, operation_id)
        SELECT '00000000-0000-4000-8000-0000000009c2', id, 500, $1, $2, '00000000-0000-4000-8000-0000000009c1' FROM app.registers`,
		admin, device)
	if err != nil {
		t.Fatal(err)
	}
	blockers, err := CheckInstallation(ctx, tx)
	_ = tx.Rollback(ctx)
	if err != nil || len(blockers) != 1 || blockers[0].Code != "own_shifts" || blockers[0].Message != "Hay 1 turno de caja." {
		t.Fatalf("turno propio: %+v %v", blockers, err)
	}
	// Catálogo, mesas y usuarios propios (confirmados): la importación se niega y no escribe nada.
	_, err = pool.Exec(ctx, `INSERT INTO app.catalog_categories(name) VALUES ('Antojitos propios 9H-C');
        INSERT INTO app.restaurant_areas(name) VALUES ('Patio propio 9H-C');
        INSERT INTO app.users(username, display_name, role_id) SELECT 'cajera9hc', 'Cajera', id FROM app.roles WHERE code = 'cashier'`)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, pool)
	_, err = importSynthetic(t, pool, standardSynthetic(), admin, device, "", nil)
	var problem *Error
	if !errors.As(err, &problem) || problem.Code != CodeNotClean || len(problem.Blockers) != 3 {
		t.Fatalf("instalación con datos propios: %v", err)
	}
	codes := []string{}
	for _, blocker := range problem.Blockers {
		codes = append(codes, blocker.Code+": "+blocker.Message)
	}
	if strings.Join(codes, "; ") != "own_categories: Hay 1 categoría creada en AD-EL POS.; own_areas: Hay 1 área de mesas creada en AD-EL POS.; own_users: Hay 1 usuario creado en AD-EL POS." {
		t.Fatalf("lo que impide importar: %v", codes)
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("un rechazo escribió algo:\n%s\n%s", before, after)
	}
}

func TestLegacyImportInterruptedLeavesNothing(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	first, err := importSynthetic(t, pool, standardSynthetic(), admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, pool)
	failing := errors.New("falla provocada a la mitad")
	// Un error con el historial ya escrito (antes de tocar lo vivo).
	_, err = importSynthetic(t, pool, standardSynthetic(), admin, device, first.BatchID, &Hooks{
		AfterHistory: func(context.Context, pgx.Tx) error {
			return failing
		},
	})
	if !errors.Is(err, failing) {
		t.Fatalf("interrupción después del historial: %v", err)
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("una importación interrumpida dejó datos a medias:\n%s\n%s", before, after)
	}
	// El contexto cancelado con el catálogo ya reemplazado (como un apagado del servidor).
	ctx, cancel := context.WithCancel(context.Background())
	environment, err := ReadEnvironment(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	model, err := Load(ctx, standardSynthetic().source(), environment.Config.Location, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, _ := Analyze(model, environment, 1, strings.Repeat("ef", 32))
	_, err = Import(ctx, pool, plan, environment.Config, ImportRequest{UserID: admin, DeviceID: device,
		ExpectedBatchID: first.BatchID, Hooks: &Hooks{AfterLive: func(context.Context, pgx.Tx) error {
			cancel()
			return nil
		}}}, nil)
	if err == nil {
		t.Fatal("una importación cancelada a la mitad no debía confirmarse")
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("una importación cancelada dejó datos a medias:\n%s\n%s", before, after)
	}
}

func TestLegacyImportShowsMoneyDifferences(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	// Una diferencia provocada a propósito dentro de la transacción: un pago en efectivo de 2019 con un peso de más.
	report, err := importSynthetic(t, pool, standardSynthetic(), admin, device, "", &Hooks{
		AfterHistory: func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE app.legacy_payments SET amount = amount + 1 WHERE legacy_id = 1")
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Validation.OK || !report.Validation.HasMoneyDifferences {
		t.Fatal("la validación debía mostrar la diferencia")
	}
	found := false
	for _, check := range report.Validation.Checks {
		if !check.OK {
			if check.Key != "payments_cash_2019" || check.Expected != "145.5000" || check.Actual != "146.5000" || check.Difference != "1.0000" {
				t.Fatalf("diferencia equivocada: %+v", check)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no apareció la diferencia")
	}
	if got := queryText(t, pool, `SELECT status || '/' || has_differences FROM app.legacy_import_batches`); got != "completed/true" {
		t.Fatalf("un lote con diferencias se completa marcado: %s", got)
	}
}
