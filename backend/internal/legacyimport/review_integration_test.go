//go:build integration

package legacyimport

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Correcciones de la revisión independiente de la Fase C con la base real: la fecha de reconocimiento con un pago
// repetido en otro año (I-1), las ventas pagadas que no quedan en el libro (M-7) y los cortes sin fecha (M-8). Todo con
// datos sintéticos.
func TestLegacyImportRecognitionShiftDatesAndNotes(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	pkg := standardSynthetic()
	// Venta 20: abierta el 31/12/2024 a las 20:00 y pagada a las 20:30; el pago se repitió el 02/01/2025 y después hubo
	// un pago en cero. Se reconoce el 31/12/2024 (el pago con el que se alcanzó lo que se debía), no en 2025.
	pkg.table("ventas").add(map[string]any{"id": 20, "caja": 1, "mesa": 2, "folio": "20", "fecha": "2024-12-31T20:00:00",
		"cajero": 1, "subtotal": money("50.0000"), "iva": money("0.0000"), "total": money("50.0000"),
		"montoPagado": money("100.0000"), "pagado": 1, "corte": nil, "cancelado": 0})
	pkg.table("ventas_productos").add(map[string]any{"id": 20, "venta": 20, "producto": 1, "cantidad": money("1.000"),
		"cantidadOriginal": money("1.000"), "subtotal": money("50.0000"), "iva": money("0.0000"), "total": money("50.0000")})
	for _, payment := range []struct {
		id     int
		amount string
		at     string
	}{{20, "50.0000", "2024-12-31T20:30:00"}, {21, "50.0000", "2025-01-02T10:00:00"}, {22, "0.0000", "2025-01-03T10:00:00"}} {
		pkg.table("pagos").add(map[string]any{"id": payment.id, "venta": 20, "folio": "", "tipo": 1, "monto": money(payment.amount),
			"recibio": 1, "fecha": payment.at, "corte": nil})
	}
	// Corte 9: cerrado sin fecha_inicio ni fecha_fin, con una venta y su pago. Corte 10: sin fechas, ventas ni pagos.
	pkg.table("cortes").add(map[string]any{"id": 9, "cajero": 1, "inicio_caja": money("100.0000"), "fin_caja": money("130.0000"),
		"venta": money("30.0000"), "caja": 1, "status": "C", "autorizacion": 1, "retiros": money("0.0000"),
		"diferencia": money("0.0000")})
	pkg.table("cortes").add(map[string]any{"id": 10, "cajero": 2, "inicio_caja": money("50.0000"), "caja": 1, "status": "C"})
	pkg.table("ventas").add(map[string]any{"id": 21, "caja": 1, "mesa": 2, "folio": "21", "fecha": "2023-05-10T13:00:00",
		"cajero": 1, "subtotal": money("30.0000"), "iva": money("0.0000"), "total": money("30.0000"),
		"montoPagado": money("30.0000"), "pagado": 1, "corte": 9, "cancelado": 0})
	pkg.table("ventas_productos").add(map[string]any{"id": 21, "venta": 21, "producto": 1, "cantidad": money("1.000"),
		"cantidadOriginal": money("1.000"), "subtotal": money("30.0000"), "iva": money("0.0000"), "total": money("30.0000")})
	pkg.table("pagos").add(map[string]any{"id": 23, "venta": 21, "folio": "", "tipo": 1, "monto": money("30.0000"), "recibio": 1,
		"fecha": "2023-05-10T13:40:00", "corte": 9})

	report, err := importSynthetic(t, pool, pkg, admin, device, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// La validación §4.4 sigue cuadrando: la pasada independiente usa la misma regla y no hay diferencias falsas.
	if !report.Validation.OK || report.Validation.HasMoneyDifferences {
		for _, check := range report.Validation.Checks {
			if !check.OK {
				t.Errorf("validación: %+v", check)
			}
		}
		t.Fatal("la validación debía cuadrar")
	}
	checks := map[string]Check{}
	for _, check := range report.Validation.Checks {
		checks[check.Key] = check
	}
	// 2024: 222.5 del paquete estándar más los 50 de la venta 20; 2025 no cambia (130), aunque el cobro repetido sí
	// cuenta en los cobros de 2025 (160 en efectivo del paquete estándar más el pago repetido de 50).
	for key, amount := range map[string]string{"sales_2024": "272.5000", "sales_2025": "130.0000", "payments_cash_2025": "210.0000"} {
		if checks[key].Expected != amount || checks[key].Actual != amount {
			t.Fatalf("%s: %+v; se esperaba %s", key, checks[key], amount)
		}
	}
	expression := func(column string) string {
		return "to_char(" + column + " AT TIME ZONE 'America/Mexico_City', 'YYYY-MM-DD HH24:MI')"
	}
	if got := queryText(t, pool, "SELECT "+expression("recognized_at")+" FROM app.legacy_sales WHERE legacy_id = 20"); got != "2024-12-31 20:30" {
		t.Fatalf("reconocimiento de la venta 20 en app.legacy_sales: %s", got)
	}
	if got := queryText(t, pool, `SELECT count(*)::text || '/' || count(*) FILTER (WHERE l.recognized_at = s.recognized_at)::text
        FROM app.legacy_sales_recognitions l JOIN app.legacy_sales s ON s.batch_id = l.batch_id AND s.legacy_id = l.legacy_sale_id
        WHERE l.legacy_sale_id = 20`); got != "1/1" {
		t.Fatalf("el libro de la venta 20 debía reconocerse con la venta: %s", got)
	}
	// Los pagos repetido y en cero se importan con su marca y cuentan en Cobros.
	if got := queryText(t, pool, `SELECT string_agg(legacy_id || ':' || amount || ':' || array_to_string(flags, ','), ' ' ORDER BY legacy_id)
        FROM app.legacy_payments WHERE legacy_sale_id = 20`); got != "20:50.0000:no_shift 21:50.0000:no_shift,possible_duplicate 22:0.0000:zero_amount,no_shift" {
		t.Fatalf("pagos de la venta 20: %s", got)
	}

	// M-7: la venta 10 del paquete estándar (pagada, sin renglones y con total 0) no queda en el libro; la validación lo
	// explica con su conteo, que es justo lo que le falta al conteo de cuentas de los reportes.
	paidWithoutLedger := queryText(t, pool, `SELECT (count(*) FILTER (WHERE status = 'paid') -
            (SELECT count(DISTINCT legacy_sale_id) FROM app.legacy_sales_recognitions))::text FROM app.legacy_sales`)
	if paidWithoutLedger != "1" {
		t.Fatalf("ventas pagadas sin libro: %s", paidWithoutLedger)
	}
	if !slices.ContainsFunc(report.Validation.Notes, func(note string) bool {
		return strings.HasPrefix(note, "1 venta pagada no tiene renglones ni importes (total 0)") &&
			strings.Contains(note, "dan 1 menos que «Ventas pagadas»")
	}) {
		t.Fatalf("falta la nota de las ventas pagadas sin libro: %q", report.Validation.Notes)
	}

	// M-8: el corte 9 toma las fechas de su venta y su pago; el 10 queda sin fecha. El resumen lo cuenta.
	if got := queryText(t, pool, "SELECT "+expression("opened_at")+" || ' a ' || "+expression("closed_at")+
		" || ' ' || array_to_string(flags, ',') FROM app.legacy_cash_shifts WHERE legacy_id = 9"); got != "2023-05-10 13:00 a 2023-05-10 13:40 start_estimated,end_estimated" {
		t.Fatalf("corte 9: %s", got)
	}
	if got := queryText(t, pool, `SELECT coalesce(opened_at::text, 'sin inicio') || ' ' || coalesce(closed_at::text, 'sin fin') || ' ' ||
        array_to_string(flags, ',') FROM app.legacy_cash_shifts WHERE legacy_id = 10`); got != "sin inicio sin fin no_activity,no_start" {
		t.Fatalf("corte 10: %s", got)
	}
	// El periodo de «Turnos de caja» (el mismo filtro de reports/cash.go) ya encuentra el corte 9.
	if got := queryText(t, pool, `SELECT coalesce(string_agg(legacy_id::text, ','), '') FROM app.legacy_cash_shifts
        WHERE (opened_at AT TIME ZONE 'America/Mexico_City')::date BETWEEN '2023-05-01' AND '2023-05-31'`); got != "9" {
		t.Fatalf("cortes de mayo de 2023: %s", got)
	}
	for _, warning := range []string{
		"1 corte del sistema anterior no tenía fecha de inicio: se usa la de su primera venta o pago para que aparezca en «Turnos de caja».",
		"1 corte cerrado del sistema anterior no tenía fecha de fin: se usa la de su última venta o pago.",
		"1 corte del sistema anterior sin fecha de inicio ni ventas o pagos queda sin fecha y no aparece en «Turnos de caja».",
	} {
		if !slices.Contains(report.Warnings, warning) {
			t.Fatalf("falta el aviso «%s»: %q", warning, report.Warnings)
		}
	}
	// Sin lote vigente, ningún aviso de reemplazo.
	for _, warning := range replaceWarnings() {
		if slices.Contains(report.Warnings, warning) {
			t.Fatalf("la primera importación no reemplaza nada: %q", report.Warnings)
		}
	}
}

// importWithRun importa un paquete sintético como lo haría el trabajador para la corrida runID (el lote queda con su
// run_id), sin registrar el resultado de la corrida: como si el proceso hubiera muerto justo después de confirmar.
func importWithRun(t *testing.T, pool *pgxpool.Pool, pkg *syntheticPackage, admin, device, runID, expectedBatch string) *Report {
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
	report, err := Import(ctx, pool, plan, environment.Config, ImportRequest{RunID: runID, UserID: admin, Username: "admin",
		DeviceID: device, ExpectedBatchID: expectedBatch, PackageSHA256: strings.Repeat("cd", 32), PackageBytes: 1234,
		BackupFile: "respaldo-de-prueba.dump"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

// Recuperación de corridas (revisión de la Fase C, M-1): una importación en curso cuyo lote sí se confirmó queda
// «succeeded» con el informe del lote; una corrida vieja sin lote queda «interrupted»; una reciente o con el lock de la
// migración tomado no se toca.
func TestLegacyManagerClosesLeftoverRuns(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	ctx := context.Background()
	audits := &auditRecorder{}
	manager := &Manager{Pool: pool, WorkDir: t.TempDir(), Audit: audits.audit}
	start := func(kind string, running bool, replace *string) string {
		t.Helper()
		run, err := insertRun(ctx, pool, RunView{Kind: kind, UploadID: newUploadID(), packageSHA256: strings.Repeat("cd", 32),
			PackageBytes: 1234, ReplaceBatchID: replace, RequestedBy: admin, requestedDevice: device})
		if err != nil {
			t.Fatal(err)
		}
		if running {
			if _, err := pool.Exec(ctx, `UPDATE app.legacy_import_runs SET status = 'running', phase = 'committing',
                started_at = now(), updated_at = now() WHERE id = $1`, run.ID); err != nil {
				t.Fatal(err)
			}
		}
		return run.ID
	}
	age := func(id string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE app.legacy_import_runs SET updated_at = now() - interval '10 minutes' WHERE id = $1`,
			id); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id string) string {
		t.Helper()
		return queryText(t, pool, `SELECT status || '/' || coalesce(error_code, '-') || '/' || coalesce(batch_id::text, '-')
            FROM app.legacy_import_runs WHERE id = $1`, id)
	}

	// Una importación «en curso» cuyo lote se confirmó (el proceso murió antes de registrar el resultado).
	first := start(KindImport, true, nil)
	report := importWithRun(t, pool, standardSynthetic(), admin, device, first, "")
	// Reciente: la recuperación periódica no la toca.
	if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := state(first); got != "running/-/-" {
		t.Fatalf("una corrida reciente no se toca: %s", got)
	}
	// Vieja, pero con el lock de la migración tomado (algo está en curso en otro servidor): tampoco.
	age(first)
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(MigrationLock)); err != nil {
		t.Fatal(err)
	}
	if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := state(first); got != "running/-/-" {
		t.Fatalf("con el lock tomado no se toca: %s", got)
	}
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", int64(MigrationLock)); err != nil {
		t.Fatal(err)
	}
	holder.Release()
	// Con el lock libre: correcta, con el informe y la validación del lote.
	if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := state(first); got != "succeeded/-/"+report.BatchID {
		t.Fatalf("la corrida con su lote confirmado debía quedar correcta: %s", got)
	}
	run, err := FindRun(ctx, pool, first)
	if err != nil {
		t.Fatal(err)
	}
	var recovered Report
	if err := json.Unmarshal(run.Summary, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.BatchID != report.BatchID || !recovered.Validation.OK || run.WithDifferences || run.BackupFile == nil ||
		*run.BackupFile != "respaldo-de-prueba.dump" || run.FinishedAt == nil || run.ErrorMessage != nil {
		t.Fatalf("corrida recuperada: %+v", run)
	}
	if audits := audits.all(); !strings.Contains(audits, "legacy.import_finished:ok") || !strings.Contains(audits, `"recovered":true`) {
		t.Fatalf("auditoría de la corrida recuperada: %s", audits)
	}

	// Un análisis que quedó en cola sin que nadie lo atienda: reciente no se toca; viejo queda interrumpido.
	second := start(KindAnalyze, false, nil)
	if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := state(second); got != "queued/-/-" {
		t.Fatalf("un análisis recién registrado no se toca: %s", got)
	}
	age(second)
	if err := manager.closeLeftoverRuns(ctx, staleAfter); err != nil {
		t.Fatal(err)
	}
	if got := state(second); got != "interrupted/"+CodeInterrupted+"/-" {
		t.Fatalf("el análisis viejo debía quedar interrumpido: %s", got)
	}
	if got := queryText(t, pool, "SELECT error_message FROM app.legacy_import_runs WHERE id = $1", second); got != leftoverMessage {
		t.Fatalf("mensaje del análisis interrumpido: %s", got)
	}

	// Al arrancar (RecoverInterrupted) se cierran todas, aunque sean recientes: un reemplazo confirmado queda correcto
	// (con su auditoría de reemplazo) y una importación en curso sin lote, interrumpida.
	third := start(KindImport, true, &report.BatchID)
	replaced := importWithRun(t, pool, standardSynthetic(), admin, device, third, report.BatchID)
	if err := manager.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	if got := state(third); got != "succeeded/-/"+replaced.BatchID {
		t.Fatalf("el reemplazo confirmado debía quedar correcto al arrancar: %s", got)
	}
	if audits := audits.all(); !strings.Contains(audits, "legacy.batch_replaced:ok") {
		t.Fatalf("auditoría del reemplazo recuperado: %s", audits)
	}
	fourth := start(KindImport, true, &replaced.BatchID)
	if err := manager.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	if got := state(fourth); got != "interrupted/"+CodeInterrupted+"/-" {
		t.Fatalf("una importación sin lote debía quedar interrumpida al arrancar: %s", got)
	}
	if got := queryText(t, pool, "SELECT error_message FROM app.legacy_import_runs WHERE id = $1", fourth); got != interruptedMessage {
		t.Fatalf("mensaje de la importación interrumpida: %s", got)
	}
	if active, err := ActiveRun(ctx, pool); err != nil || active != nil {
		t.Fatalf("corrida activa después de recuperar: %+v %v", active, err)
	}
}
