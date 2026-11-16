//go:build integration

package legacyimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
)

const syntheticPassphrase = "frase de paso sintética 9H"

// syntheticPackageBytes es el paquete sintético de la Fase B (legacysynth) cifrado con parámetros de prueba.
func syntheticPackageBytes(t *testing.T, tables []*legacysynth.Table) []byte {
	t.Helper()
	options := legacypkg.WriterOptions{SpoolDir: t.TempDir(), KDF: legacypkg.KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}}
	var output bytes.Buffer
	if tables == nil {
		if _, err := legacysynth.Build(&output, syntheticPassphrase, options); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	writer, err := legacypkg.NewWriter(options)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	summary, err := legacysynth.WritePackage(writer, tables)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Finish(&output, syntheticPassphrase, legacypkg.PackageInfo{CreatedAt: time.Now(),
		Creator: legacypkg.Creator{Product: "AD-EL POS", Version: "0.10.0", Tool: "prueba-9h-c"},
		Source:  legacypkg.Source{Kind: legacypkg.SourceSynthetic}, Summary: summary}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

// auditRecorder guarda las acciones auditadas por el trabajador.
type auditRecorder struct {
	mutex   sync.Mutex
	actions []string
	details []string
}

func (recorder *auditRecorder) audit(_ context.Context, _ pgx.Tx, _ Actor, action, _ string, details any, success bool) error {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	encoded, _ := json.Marshal(details)
	recorder.actions = append(recorder.actions, action+map[bool]string{true: ":ok", false: ":error"}[success])
	recorder.details = append(recorder.details, string(encoded))
	return nil
}

func (recorder *auditRecorder) all() string {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return strings.Join(recorder.actions, ",") + " " + strings.Join(recorder.details, " ")
}

// harness es un trabajador de la migración sobre la base de la prueba, con un respaldo simulado.
type harness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	manager *Manager
	actor   Actor
	backups *int
	audits  *auditRecorder
}

func newHarness(t *testing.T, pool *pgxpool.Pool, admin, device string, backupFails bool) *harness {
	t.Helper()
	backups := 0
	audits := &auditRecorder{}
	manager := &Manager{Pool: pool, WorkDir: t.TempDir(), Audit: audits.audit,
		Backup: func(context.Context, Actor, string) (BackupResult, error) {
			backups++
			if backupFails {
				return BackupResult{}, errors.New("fallo simulado del respaldo")
			}
			return BackupResult{RunID: "respaldo", FileName: "adel_pos_test-previo.dump"}, nil
		}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	// El trabajador ya atiende (Run marcó su estado antes de recuperar las corridas).
	deadline := time.Now().Add(5 * time.Second)
	for {
		manager.mutex.Lock()
		running := manager.running
		manager.mutex.Unlock()
		if running || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return &harness{t: t, pool: pool, manager: manager, actor: Actor{UserID: admin, Username: "admin", DeviceID: device},
		backups: &backups, audits: audits}
}

// upload sube un paquete en partes de 1 MiB.
func (h *harness) upload(content []byte) UploadView {
	h.t.Helper()
	digest := sha256.Sum256(content)
	view, err := h.manager.CreateUpload(h.actor, int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil {
		h.t.Fatal(err)
	}
	for index := 0; index < view.Parts; index++ {
		end := (index + 1) * PartSize
		if end > len(content) {
			end = len(content)
		}
		part := content[index*PartSize : end]
		partDigest := sha256.Sum256(part)
		view, err = h.manager.AddPart(h.actor, view.ID, index, part, hex.EncodeToString(partDigest[:]))
		if err != nil {
			h.t.Fatal(err)
		}
	}
	if view.Status != UploadComplete {
		h.t.Fatalf("subida incompleta: %+v", view)
	}
	return view
}

// start registra una corrida en una transacción como la de la petición y la entrega al trabajador al confirmar.
func (h *harness) start(register func(tx pgx.Tx) (*Pending, error)) (*Pending, error) {
	h.t.Helper()
	ctx := context.Background()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	pending, err := register(tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		h.t.Fatal(err)
	}
	h.manager.Dispatch(pending)
	return pending, nil
}

// wait espera el resultado de una corrida.
func (h *harness) wait(id string) RunView {
	h.t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		run, err := FindRun(context.Background(), h.pool, id)
		if err != nil {
			h.t.Fatal(err)
		}
		if run.Status != StatusQueued && run.Status != StatusRunning {
			return *run
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("la corrida no terminó: %+v", run)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (h *harness) analyze(view UploadView, passphrase string) RunView {
	h.t.Helper()
	pending, err := h.start(func(tx pgx.Tx) (*Pending, error) {
		return h.manager.StartAnalysis(context.Background(), tx, h.actor, view.ID, passphrase)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.wait(pending.Run.ID)
}

func (h *harness) importRun(view UploadView, analysis RunView, replace string) RunView {
	h.t.Helper()
	pending, err := h.start(func(tx pgx.Tx) (*Pending, error) {
		return h.manager.StartImport(context.Background(), tx, h.actor, ImportInput{UploadID: view.ID, Passphrase: syntheticPassphrase,
			AnalysisRunID: analysis.ID, Confirm: true, ReplaceBatchID: replace})
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.wait(pending.Run.ID)
}

func errorCode(run RunView) string {
	if run.ErrorCode == nil {
		return ""
	}
	return *run.ErrorCode
}

func TestLegacyManagerAnalyzesAndImportsPackages(t *testing.T) {
	pool := testDatabase(t)
	admin, device := installation(t, pool)
	h := newHarness(t, pool, admin, device, false)
	content := syntheticPackageBytes(t, nil)
	view := h.upload(content)

	t.Run("frase_incorrecta_paquete_danado_version_y_no_paquete", func(t *testing.T) {
		if run := h.analyze(view, "otra frase de paso"); run.Status != StatusFailed || errorCode(run) != CodeWrongPassphrase {
			t.Fatalf("frase incorrecta: %+v", run)
		}
		// Un paquete coherente consigo mismo pero sin una tabla que la importación necesita: error claro, no una
		// importación parcial.
		tables := []*legacysynth.Table{}
		for _, table := range legacysynth.Dataset() {
			if table.Name != "cortes" {
				tables = append(tables, table)
			}
		}
		withoutShifts := h.upload(syntheticPackageBytes(t, tables))
		incomplete := h.analyze(withoutShifts, syntheticPassphrase)
		if incomplete.Status != StatusFailed || errorCode(incomplete) != CodeIncompatible || incomplete.ErrorMessage == nil ||
			!strings.Contains(*incomplete.ErrorMessage, "cortes") {
			t.Fatalf("paquete sin la tabla cortes: %+v", incomplete)
		}
		if err := h.manager.CancelUpload(h.actor, withoutShifts.ID); err != nil {
			t.Fatal(err)
		}
		damaged := append([]byte{}, content...)
		damaged[len(damaged)/2] ^= 0x40
		newer := append([]byte{}, content...)
		newer[len(legacypkg.Signature)+1] = 9
		cases := []struct {
			label   string
			content []byte
			code    string
		}{
			{"paquete dañado", damaged, CodeDamaged},
			{"versión desconocida", newer, CodeUnsupportedVersion},
			{"no es un paquete", []byte("esto no es un paquete de migración de AD-EL POS"), CodeNotPackage},
		}
		for _, item := range cases {
			bad := h.upload(item.content)
			if run := h.analyze(bad, syntheticPassphrase); run.Status != StatusFailed || errorCode(run) != item.code {
				t.Fatalf("%s: %+v", item.label, run)
			}
			if err := h.manager.CancelUpload(h.actor, bad.ID); err != nil {
				t.Fatal(err)
			}
		}
	})

	analysis := h.analyze(view, syntheticPassphrase)
	if analysis.Status != StatusSucceeded {
		t.Fatalf("análisis: %+v", analysis)
	}
	var preview Preview
	if err := json.Unmarshal(analysis.Summary, &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.Clean || preview.WillReplace || preview.Counts[CountSales] == 0 || preview.SalesPeriod.Label == "" ||
		preview.Package.Fingerprint == "" || len(preview.Flags) == 0 {
		t.Fatalf("resumen previo: %+v", preview)
	}
	if count := queryText(t, pool, `SELECT count(*)::text FROM app.legacy_import_batches`); count != "0" {
		t.Fatal("el análisis escribió un lote")
	}

	t.Run("dos_importaciones_a_la_vez", func(t *testing.T) {
		ctx := context.Background()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		input := ImportInput{UploadID: view.ID, Passphrase: syntheticPassphrase, AnalysisRunID: analysis.ID, Confirm: true}
		if _, err := h.manager.StartImport(ctx, tx, h.actor, input); err != nil {
			t.Fatal(err)
		}
		other, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Rollback(ctx)
		// La segunda espera a la primera transacción (el índice único de corridas activas) y se rechaza al confirmar
		// la primera.
		result := make(chan error, 1)
		go func() {
			_, err := h.manager.StartImport(ctx, other, h.actor, input)
			result <- err
		}()
		time.Sleep(200 * time.Millisecond)
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-result; CodeOf(err) != CodeBusy {
			t.Fatalf("la segunda importación a la vez: %v", err)
		}
		// La primera, sin entregar al trabajador, queda en cola: el trabajador la marca interrumpida al «arrancar».
		if err := h.manager.RecoverInterrupted(ctx); err != nil {
			t.Fatal(err)
		}
		if active, err := ActiveRun(ctx, pool); err != nil || active != nil {
			t.Fatalf("corrida activa después de recuperar: %+v %v", active, err)
		}
	})

	imported := h.importRun(view, analysis, "")
	if imported.Status != StatusSucceeded || imported.BatchID == nil || imported.WithDifferences || *h.backups != 1 {
		t.Fatalf("importación: %+v (respaldos %d)", imported, *h.backups)
	}
	var report Report
	if err := json.Unmarshal(imported.Summary, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Validation.OK || report.BackupFile != "adel_pos_test-previo.dump" {
		for _, check := range report.Validation.Checks {
			if !check.OK {
				t.Errorf("validación: %+v", check)
			}
		}
		t.Fatalf("informe: %+v", report.Validation)
	}
	// El paquete subido se borra al importar.
	if _, err := h.manager.GetUpload(h.actor, view.ID); CodeOf(err) != CodeUploadMissing {
		t.Fatalf("la subida debía borrarse al importar: %v", err)
	}
	// El PDF del informe.
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	file, err := RenderReport(context.Background(), tx, *imported.BatchID)
	_ = tx.Rollback(context.Background())
	if err != nil || file.ContentType != "application/pdf" || !bytes.HasPrefix(file.Content, []byte("%PDF")) || len(file.Content) < 2000 {
		t.Fatalf("PDF del informe: %v", err)
	}
	audits := h.audits.all()
	for _, action := range []string{"legacy.analysis_requested:ok", "legacy.analysis_finished:error", "legacy.analysis_finished:ok",
		"legacy.import_requested:ok", "legacy.import_finished:ok"} {
		if !strings.Contains(audits, action) {
			t.Fatalf("falta la auditoría %s: %s", action, audits)
		}
	}
	if strings.Contains(audits, syntheticPassphrase) {
		t.Fatal("la frase de paso quedó en la auditoría")
	}
	if strings.Contains(queryText(t, pool, `SELECT coalesce(string_agg(summary::text || coalesce(error_message, ''), ' '), '') FROM app.legacy_import_runs`),
		syntheticPassphrase) {
		t.Fatal("la frase de paso quedó en las corridas")
	}

	t.Run("reemplazo_y_respaldo_fallido", func(t *testing.T) {
		second := h.upload(content)
		again := h.analyze(second, syntheticPassphrase)
		if err := json.Unmarshal(again.Summary, &preview); err != nil {
			t.Fatal(err)
		}
		if !preview.WillReplace || preview.CurrentBatch == nil || preview.CurrentBatch.ID != *imported.BatchID || !preview.Clean {
			t.Fatalf("el segundo análisis debía ofrecer reemplazar el lote: %+v", preview)
		}
		// Sin un respaldo verificado no se importa nada (otro trabajador, con un respaldo que falla).
		failing := newHarness(t, pool, admin, device, true)
		failingUpload := failing.upload(content)
		failingAnalysis := failing.analyze(failingUpload, syntheticPassphrase)
		before := snapshot(t, pool)
		run := failing.importRun(failingUpload, failingAnalysis, *imported.BatchID)
		if run.Status != StatusFailed || errorCode(run) != CodeBackupFailed {
			t.Fatalf("respaldo fallido: %+v", run)
		}
		if after := snapshot(t, pool); after != before {
			t.Fatal("un respaldo fallido no debía importar nada")
		}
		replaced := h.importRun(second, again, *imported.BatchID)
		if replaced.Status != StatusSucceeded || replaced.WithDifferences {
			t.Fatalf("reemplazo: %+v", replaced)
		}
		if got := queryText(t, pool, `SELECT string_agg(status, ',' ORDER BY seq) FROM app.legacy_import_batches`); got != "replaced,completed" {
			t.Fatalf("lotes: %s", got)
		}
	})
}
