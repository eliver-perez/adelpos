//go:build integration

package administration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/backupschedule"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
	"local/ad-el-pos/server/internal/legacypkg"
	"local/ad-el-pos/server/internal/legacysynth"
)

const legacyPassphrase = "frase de paso de las rutas 9H"

// TestStage9HLegacyMigrationRoutes cubre las rutas de la migración del sistema anterior (etapa 9H, fase C) por HTTP:
// solo con settings.manage y desde la computadora principal; subida por partes con su huella; análisis e importación
// en segundo plano con el respaldo verificado previo; el informe en PDF; «Asignar contraseña» a un usuario importado;
// que la operación de AD-EL POS no ve las ventas importadas; y que la frase de paso no queda en ningún registro.
func TestStage9HLegacyMigrationRoutes(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	reset := func() {
		if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations"); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	provisioning := administration.NewToken()
	tokenFile := filepath.Join(t.TempDir(), "provisioning-token")
	if err := os.WriteFile(tokenFile, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(root, "datos-servidor")
	target := &scheduledTarget{directory: filepath.Join(server, "backups"), server: server}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	service := &administration.Service{Pool: pool, ProvisioningFile: tokenFile, Logger: logger}
	service.ScheduledBackups = backupschedule.NewRunner(pool, target, logger)
	service.EnableLegacyMigration(filepath.Join(server, "legacy-import"))
	workers, stopWorkers := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		service.Legacy.Run(workers)
	}()
	defer func() {
		stopWorkers()
		<-workerDone
	}()
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, logger, service)
	call := func(actor terminal, method, route string, body any, expected int) json.RawMessage {
		t.Helper()
		encoded, _ := json.Marshal(body)
		if body == nil {
			encoded = nil
		}
		request := httptest.NewRequestWithContext(context.Background(), method, "/api/v1/"+route, bytes.NewReader(encoded))
		request.RemoteAddr = "127.0.0.1:50505"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Device-ID", actor.id)
		request.Header.Set("X-Device-Token", actor.secret)
		request.Header.Set("Authorization", "Bearer "+actor.session)
		request.Header.Set("X-Provisioning-Token", provisioning)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("%s %s: esperado %d, recibido %d (%s)", method, route, expected, response.Code, response.Body.String())
		}
		return append(json.RawMessage{}, response.Body.Bytes()...)
	}
	code := func(body json.RawMessage) string {
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		return failure.Error.Code
	}
	field := func(body json.RawMessage, key string) string {
		var fields map[string]any
		_ = json.Unmarshal(body, &fields)
		value, _ := fields[key].(string)
		return value
	}
	text := func(query string, args ...any) string {
		t.Helper()
		var value string
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	setup := call(terminal{}, "POST", "setup", map[string]any{"name": "Migración de prueba 9H", "timezone": "America/Mexico_City",
		"currency": "MXN", "theme": "light", "terminal_name": "Computadora principal"}, 200)
	admin := terminal{id: field(setup, "device_id"), secret: field(setup, "device_token")}
	admin.session = field(call(admin, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200), "session_token")

	// El paquete sintético de la Fase B.
	var content bytes.Buffer
	if _, err := legacysynth.Build(&content, legacyPassphrase, legacypkg.WriterOptions{SpoolDir: t.TempDir(),
		KDF: legacypkg.KDFParams{Time: 1, MemoryKiB: 8 << 10, Threads: 1}}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content.Bytes())
	upload := func() string {
		t.Helper()
		created := call(admin, "POST", "maintenance/legacy/uploads", map[string]any{"size": content.Len(),
			"sha256": hex.EncodeToString(digest[:])}, 200)
		id := field(created, "id")
		part := content.Bytes()
		partDigest := sha256.Sum256(part)
		body := call(admin, "POST", "maintenance/legacy/uploads/"+id+"/parts", map[string]any{"index": 0,
			"data": base64.StdEncoding.EncodeToString(part), "sha256": hex.EncodeToString(partDigest[:])}, 200)
		if field(body, "status") != "complete" {
			t.Fatalf("subida: %s", body)
		}
		return id
	}
	waitRun := func(id string) map[string]any {
		t.Helper()
		deadline := time.Now().Add(2 * time.Minute)
		for {
			var run map[string]any
			if err := json.Unmarshal(call(admin, "GET", "maintenance/legacy/runs/"+id, nil, 200), &run); err != nil {
				t.Fatal(err)
			}
			if run["status"] != "queued" && run["status"] != "running" {
				return run
			}
			if time.Now().After(deadline) {
				t.Fatalf("la corrida no terminó: %v", run)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	t.Run("solo_con_settings_manage_y_en_la_computadora_principal", func(t *testing.T) {
		password := administration.NewToken()
		role := field(call(admin, "POST", "roles", map[string]any{"name": "Usuarios 9H-C", "permissions": []string{"users.read"}}, 200), "id")
		call(admin, "POST", "users", map[string]any{"username": "lectora9hc", "display_name": "Lectora", "role_id": role,
			"active": true, "password": password}, 200)
		invitation := field(call(admin, "POST", "devices/invitations", map[string]any{}, 200), "code")
		enrolled := call(terminal{}, "POST", "devices/enroll", map[string]string{"name": "Terminal 9H-C", "code": invitation}, 200)
		reader := terminal{id: field(enrolled, "device_id"), secret: field(enrolled, "device_token")}
		reader.session = field(call(reader, "POST", "auth/login", map[string]string{"username": "lectora9hc", "password": password}, 200), "session_token")
		for _, route := range []string{"maintenance/legacy", "maintenance/legacy/runs/6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c01"} {
			if body := call(reader, "GET", route, nil, 403); code(body) != "PERMISSION_DENIED" {
				t.Fatalf("%s sin permiso: %s", route, body)
			}
		}
		// Por la escucha de red (no local) se rechaza aunque tenga el permiso.
		_, err := service.Execute(ctx, administration.Request{Method: "GET", Path: "/api/v1/maintenance/legacy",
			DeviceID: admin.id, DeviceToken: admin.secret, SessionToken: admin.session, Local: false})
		var fault *administration.Fault
		if !errors.As(err, &fault) || fault.Code != "LOCAL_REQUIRED" {
			t.Fatalf("migración por la red: %v", err)
		}
		// El usuario de prueba no debe impedir importar: se borra (no tiene operación).
		if _, err := pool.Exec(ctx, `DELETE FROM app.sessions WHERE user_id = (SELECT id FROM app.users WHERE username = 'lectora9hc');
            DELETE FROM app.user_preferences WHERE user_id = (SELECT id FROM app.users WHERE username = 'lectora9hc')`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM app.audit_events WHERE actor_id = (SELECT id FROM app.users WHERE username = 'lectora9hc')`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM app.users WHERE username = 'lectora9hc'`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("subida_por_partes", func(t *testing.T) {
		if body := call(admin, "POST", "maintenance/legacy/uploads", map[string]any{"size": int64(2) << 30,
			"sha256": strings.Repeat("a", 64)}, 413); code(body) != "LEGACY_UPLOAD_TOO_LARGE" {
			t.Fatalf("tope de tamaño: %s", body)
		}
		created := call(admin, "POST", "maintenance/legacy/uploads", map[string]any{"size": content.Len(),
			"sha256": hex.EncodeToString(digest[:])}, 200)
		id := field(created, "id")
		if body := call(admin, "POST", "maintenance/legacy/uploads/"+id+"/parts", map[string]any{"index": 0,
			"data": base64.StdEncoding.EncodeToString(content.Bytes()), "sha256": strings.Repeat("0", 64)}, 400); code(body) != "LEGACY_UPLOAD_HASH_MISMATCH" {
			t.Fatalf("parte con huella mala: %s", body)
		}
		if body := call(admin, "POST", "maintenance/legacy/uploads/"+id+"/parts", map[string]any{"index": 3,
			"data": "", "sha256": hex.EncodeToString(sha256.New().Sum(nil))}, 409); code(body) != "LEGACY_UPLOAD_OUT_OF_ORDER" {
			t.Fatalf("parte fuera de orden: %s", body)
		}
		call(admin, "POST", "maintenance/legacy/uploads/"+id+"/cancel", nil, 200)
		call(admin, "GET", "maintenance/legacy/uploads/"+id, nil, 404)
	})

	id := upload()
	if body := call(admin, "POST", "maintenance/legacy/analyze", map[string]any{"upload_id": id, "passphrase": "corta"}, 400); code(body) != "LEGACY_PASSPHRASE_INVALID" {
		t.Fatalf("frase corta: %s", body)
	}
	wrong := waitRun(field(call(admin, "POST", "maintenance/legacy/analyze", map[string]any{"upload_id": id,
		"passphrase": "otra frase de paso"}, 200), "id"))
	if wrong["status"] != "failed" || wrong["error_code"] != "LEGACY_PASSPHRASE_INVALID" {
		t.Fatalf("frase incorrecta: %v", wrong)
	}
	analysis := waitRun(field(call(admin, "POST", "maintenance/legacy/analyze", map[string]any{"upload_id": id,
		"passphrase": legacyPassphrase}, 200), "id"))
	summary, _ := analysis["summary"].(map[string]any)
	if analysis["status"] != "succeeded" || summary["clean"] != true || summary["will_replace"] != false {
		t.Fatalf("análisis: %v", analysis)
	}
	if body := call(admin, "POST", "maintenance/legacy/import", map[string]any{"upload_id": id, "passphrase": legacyPassphrase,
		"analysis_run_id": analysis["id"], "confirm": false}, 400); code(body) != "LEGACY_UPLOAD_INVALID" {
		t.Fatalf("importar sin confirmar: %s", body)
	}
	imported := waitRun(field(call(admin, "POST", "maintenance/legacy/import", map[string]any{"upload_id": id,
		"passphrase": legacyPassphrase, "analysis_run_id": analysis["id"], "confirm": true, "replace_batch_id": nil}, 200), "id"))
	if imported["status"] != "succeeded" || imported["with_differences"] != false || target.dumpCount() != 1 {
		t.Fatalf("importación: %v (respaldos %d)", imported, target.dumpCount())
	}
	batch, _ := imported["batch_id"].(string)
	detail := call(admin, "GET", "maintenance/legacy/batches/"+batch, nil, 200)
	if !strings.Contains(string(detail), `"ok":true`) {
		t.Fatalf("lote: %s", detail)
	}
	var status struct {
		CurrentBatch *struct {
			ID string `json:"id"`
		} `json:"current_batch"`
		ActiveRun *json.RawMessage `json:"active_run"`
	}
	if err := json.Unmarshal(call(admin, "GET", "maintenance/legacy", nil, 200), &status); err != nil || status.CurrentBatch == nil ||
		status.CurrentBatch.ID != batch || status.ActiveRun != nil {
		t.Fatalf("estado de la migración: %+v %v", status, err)
	}

	t.Run("informe_en_pdf", func(t *testing.T) {
		request := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/maintenance/legacy/batches/"+batch+"/report.pdf", nil)
		request.RemoteAddr = "127.0.0.1:50505"
		request.Header.Set("X-Device-ID", admin.id)
		request.Header.Set("X-Device-Token", admin.secret)
		request.Header.Set("Authorization", "Bearer "+admin.session)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 || response.Header().Get("Content-Type") != "application/pdf" ||
			!bytes.HasPrefix(response.Body.Bytes(), []byte("%PDF")) {
			t.Fatalf("PDF: %d %s", response.Code, response.Header().Get("Content-Type"))
		}
	})

	t.Run("la_operacion_no_ve_el_historial", func(t *testing.T) {
		// Las rutas de cuentas, del estado del restaurante y de cobros no ven ninguna venta importada.
		for _, route := range []string{"restaurant/accounts", "restaurant/accounts?state=closed", "restaurant/state", "restaurant/overview"} {
			if body := call(admin, "GET", route, nil, 200); strings.Contains(string(body), `"folio"`) {
				t.Fatalf("%s muestra cuentas después de importar: %s", route, body)
			}
		}
		for _, table := range []string{"restaurant_accounts", "payments", "cash_shifts", "account_cancellations", "sales_recognitions",
			"inventory_movements"} {
			if got := text("SELECT count(*)::text FROM app." + table); got != "0" {
				t.Fatalf("la migración escribió en %s: %s", table, got)
			}
		}
	})

	t.Run("asignar_contrasena", func(t *testing.T) {
		var users []struct {
			ID              string `json:"id"`
			Username        string `json:"username"`
			HasPassword     bool   `json:"has_password"`
			InitialPassword bool   `json:"initial_password"`
			Active          bool   `json:"active"`
		}
		if err := json.Unmarshal(call(admin, "GET", "users", nil, 200), &users); err != nil {
			t.Fatal(err)
		}
		var imported, adminID string
		for _, user := range users {
			if user.Username == "admin" {
				adminID = user.ID
				if !user.HasPassword {
					t.Fatal("el administrador tiene contraseña")
				}
			} else if !user.HasPassword && user.Active && imported == "" {
				imported = user.ID
			}
		}
		if imported == "" {
			t.Fatalf("no hay usuarios importados sin contraseña: %+v", users)
		}
		if body := call(admin, "POST", "users/"+adminID+"/password", map[string]string{"password": "nueva-contrasena"}, 409); code(body) != "PASSWORD_ALREADY_SET" {
			t.Fatalf("asignar a quien ya tiene: %s", body)
		}
		call(admin, "POST", "users/"+imported+"/password", map[string]string{"password": "corta"}, 400)
		assigned := call(admin, "POST", "users/"+imported+"/password", map[string]string{"password": "contrasena-inicial-9h"}, 200)
		if !strings.Contains(string(assigned), `"initial_password": true`) && !strings.Contains(string(assigned), `"initial_password":true`) {
			t.Fatalf("contraseña inicial: %s", assigned)
		}
		username := field(assigned, "username")
		login := call(terminal{id: admin.id, secret: admin.secret}, "POST", "auth/login",
			map[string]string{"username": username, "password": "contrasena-inicial-9h"}, 200)
		if field(login, "session_token") == "" {
			t.Fatalf("inicio de sesión del usuario importado: %s", login)
		}
		if got := text(`SELECT count(*)::text FROM app.audit_events WHERE action = 'user.password_assigned' AND entity_id = $1`, imported); got != "1" {
			t.Fatalf("auditoría de «Asignar contraseña»: %s", got)
		}
	})

	t.Run("auditoria_sin_frase_de_paso", func(t *testing.T) {
		for _, action := range []string{"legacy.upload_created", "legacy.upload_completed", "legacy.analysis_requested",
			"legacy.analysis_finished", "legacy.import_requested", "legacy.import_finished", "legacy.report_downloaded",
			"maintenance.backup_created"} {
			if got := text(`SELECT count(*)::text FROM app.audit_events WHERE action = $1`, action); got == "0" {
				t.Fatalf("falta la auditoría %s", action)
			}
		}
		if got := text(`SELECT count(*)::text FROM app.audit_events WHERE before_state::text LIKE '%' || $1 || '%'
            OR after_state::text LIKE '%' || $1 || '%'`, legacyPassphrase); got != "0" {
			t.Fatal("la frase de paso quedó en el Registro de movimientos")
		}
		call(admin, "POST", "maintenance/legacy/preparations", map[string]any{"succeeded": true, "backup_taken_at": "2026-11-14T22:10:05",
			"sales_first": "2019-10", "sales_last": "2026-11", "package_bytes": content.Len(), "format_version": 1,
			"tables": map[string]int{"ventas": 240}, "timings_ms": map[string]int{"read": 1200}}, 200)
		if body := call(admin, "POST", "maintenance/legacy/preparations", map[string]any{"succeeded": true,
			"backup_taken_at": "C:\\Users\\dueño\\possys.bak"}, 400); code(body) != "VALIDATION_ERROR" {
			t.Fatalf("una preparación con una ruta: %s", body)
		}
	})

	t.Run("reemplazo_confirmado", func(t *testing.T) {
		second := upload()
		again := waitRun(field(call(admin, "POST", "maintenance/legacy/analyze", map[string]any{"upload_id": second,
			"passphrase": legacyPassphrase}, 200), "id"))
		preview, _ := again["summary"].(map[string]any)
		if preview["will_replace"] != true || preview["clean"] != true {
			t.Fatalf("segundo análisis: %v", preview)
		}
		if body := call(admin, "POST", "maintenance/legacy/import", map[string]any{"upload_id": second, "passphrase": legacyPassphrase,
			"analysis_run_id": again["id"], "confirm": true, "replace_batch_id": nil}, 409); code(body) != "LEGACY_BATCH_CHANGED" {
			t.Fatalf("reemplazar sin confirmar el lote: %s", body)
		}
		replaced := waitRun(field(call(admin, "POST", "maintenance/legacy/import", map[string]any{"upload_id": second,
			"passphrase": legacyPassphrase, "analysis_run_id": again["id"], "confirm": true, "replace_batch_id": batch}, 200), "id"))
		if replaced["status"] != "succeeded" {
			t.Fatalf("reemplazo: %v", replaced)
		}
		// La contraseña asignada al usuario importado se conserva al reemplazar el lote.
		if got := text(`SELECT count(*)::text FROM app.users WHERE password_hash IS NOT NULL AND initial_password`); got != "2" {
			t.Fatalf("usuarios con contraseña inicial después del reemplazo: %s", got)
		}
		if got := text(`SELECT count(*)::text FROM app.audit_events WHERE action = 'legacy.batch_replaced'`); got != "1" {
			t.Fatalf("auditoría del reemplazo: %s", got)
		}
	})
}
