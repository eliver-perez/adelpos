//go:build integration

package administration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/backupschedule"
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
)

// scheduledTarget sustituye al respaldo de la instalación en las rutas de respaldos: escribe un archivo pequeño, lo
// «verifica» por su contenido y puede fallar o quedarse esperando a pedido.
type scheduledTarget struct {
	mutex      sync.Mutex
	directory  string
	server     string
	failVerify bool
	block      chan struct{}
	started    chan struct{}
	dumps      int
}

func (target *scheduledTarget) Backup(ctx context.Context) (string, error) {
	return "", errors.New("no se usa")
}

func (target *scheduledTarget) DefaultDirectory() string {
	return target.directory
}

func (target *scheduledTarget) ProtectedDirectories() []string {
	return []string{target.server}
}

func (target *scheduledTarget) DatabaseName() string {
	return "adel_pos_test_rutas"
}

func (target *scheduledTarget) Dump(ctx context.Context, filename string) error {
	target.mutex.Lock()
	target.dumps++
	block, started := target.block, target.started
	target.mutex.Unlock()
	if started != nil {
		started <- struct{}{}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return os.WriteFile(filename, []byte("PGDMP"), 0600)
}

func (target *scheduledTarget) Verify(ctx context.Context, filename string) error {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	if target.failVerify {
		return errors.New("fallo simulado de la verificación")
	}
	return nil
}

func (target *scheduledTarget) dumpCount() int {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	return target.dumps
}

func (target *scheduledTarget) set(change func(*scheduledTarget)) {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	change(target)
}

type backupRunBody struct {
	Run struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		Status        string `json:"status"`
		FileName      string `json:"file_name"`
		Location      string `json:"location"`
		FolderName    string `json:"folder_name"`
		Verified      bool   `json:"verified"`
		FailureReason string `json:"failure_reason"`
		Available     *bool  `json:"available"`
	} `json:"run"`
}

type backupStateBody struct {
	Settings struct {
		Enabled       bool    `json:"enabled"`
		Time          *string `json:"time"`
		EffectiveTime string  `json:"effective_time"`
		Keep          int     `json:"keep"`
		Directory     struct {
			Custom     bool   `json:"custom"`
			Path       string `json:"path"`
			FolderName string `json:"folder_name"`
			Available  bool   `json:"available"`
		} `json:"directory"`
		DefaultDirectory struct {
			Path string `json:"path"`
		} `json:"default_directory"`
	} `json:"settings"`
	NextRunAt                *time.Time        `json:"next_run_at"`
	Pending                  bool              `json:"pending"`
	Running                  bool              `json:"running"`
	Alert                    *json.RawMessage  `json:"alert"`
	Runs                     []json.RawMessage `json:"runs"`
	DirectoryChangeAvailable bool              `json:"directory_change_available"`
}

// TestStage9GBackupRoutes cubre las rutas de respaldos de la etapa 9G, fase F: solo con settings.manage; la
// configuración validada y auditada; la carpeta solo desde la computadora principal, validada por Go y auditada;
// «Respaldar ahora» idempotente y auditado al terminar; respaldo en curso; fallo de verificación → aviso al
// administrador (estado de respaldos y del servicio) hasta un respaldo correcto posterior. Usa su propia base: la
// deja vacía al empezar y al terminar (cada prueba de este paquete migra la suya).
func TestStage9GBackupRoutes(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	// La carpeta de datos del servidor (protegida) y, dentro, la predeterminada de los respaldos, como en el servicio.
	server := filepath.Join(root, "datos-servidor")
	for _, folder := range []string{filepath.Join(server, "logs"), filepath.Join(server, "postgres", "pg_wal")} {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(server, "postgres", "PG_VERSION"), []byte("18"), 0600); err != nil {
		t.Fatal(err)
	}
	target := &scheduledTarget{directory: filepath.Join(server, "backups"), server: server}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	service := &administration.Service{Pool: pool, ProvisioningFile: tokenFile, Logger: logger}
	service.ScheduledBackups = backupschedule.NewRunner(pool, target, logger)
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, logger, service)

	send := func(requestContext context.Context, actor terminal, method, route string, body any) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		if body == nil {
			encoded = nil
		}
		request := httptest.NewRequestWithContext(requestContext, method, "/api/v1/"+route, bytes.NewReader(encoded))
		request.RemoteAddr = "127.0.0.1:50505"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Device-ID", actor.id)
		request.Header.Set("X-Device-Token", actor.secret)
		request.Header.Set("Authorization", "Bearer "+actor.session)
		request.Header.Set("X-Provisioning-Token", provisioning)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	call := func(actor terminal, method, route string, body any, expected int) json.RawMessage {
		t.Helper()
		response := send(context.Background(), actor, method, route, body)
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
	scalar := func(query string, args ...any) int {
		t.Helper()
		var value int
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	eventually := func(description string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal(description)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	state := func(actor terminal) backupStateBody {
		t.Helper()
		var value backupStateBody
		if err := json.Unmarshal(call(actor, "GET", "maintenance/backups", nil, 200), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	run := func(actor terminal, operationID string, expected int) backupRunBody {
		t.Helper()
		var value backupRunBody
		body := call(actor, "POST", "maintenance/backups/run", map[string]string{"operation_id": operationID}, expected)
		if expected == 200 {
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
		}
		return value
	}

	setup := call(terminal{}, "POST", "setup", map[string]any{"name": "Respaldos de prueba", "timezone": "America/Monterrey",
		"currency": "MXN", "theme": "light", "terminal_name": "Computadora principal"}, 200)
	admin := terminal{id: field(setup, "device_id"), secret: field(setup, "device_token")}
	admin.session = field(call(admin, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200), "session_token")
	password := administration.NewToken()
	role := field(call(admin, "POST", "roles", map[string]any{"name": "Solo usuarios", "permissions": []string{"users.read"}}, 200), "id")
	call(admin, "POST", "users", map[string]any{"username": "cajero", "display_name": "Cajero", "role_id": role, "active": true, "password": password}, 200)
	managerRole := field(call(admin, "POST", "roles", map[string]any{"name": "Gerente", "permissions": []string{"settings.manage"}}, 200), "id")
	call(admin, "POST", "users", map[string]any{"username": "gerente", "display_name": "Gerente", "role_id": managerRole, "active": true, "password": password}, 200)
	// Cada usuario en su propia terminal (una sesión por usuario y terminal).
	invited := func() terminal {
		invitation := field(call(admin, "POST", "devices/invitations", map[string]any{}, 200), "code")
		enrolled := call(terminal{}, "POST", "devices/enroll", map[string]string{"name": "Terminal " + invitation[:6], "code": invitation}, 200)
		return terminal{id: field(enrolled, "device_id"), secret: field(enrolled, "device_token")}
	}
	cashier := invited()
	cashier.session = field(call(cashier, "POST", "auth/login", map[string]string{"username": "cajero", "password": password}, 200), "session_token")
	manager := invited()
	manager.session = field(call(manager, "POST", "auth/login", map[string]string{"username": "gerente", "password": password}, 200), "session_token")

	t.Run("solo_con_settings_manage", func(t *testing.T) {
		for _, route := range []string{"maintenance/backups", "maintenance/backups/status"} {
			if body := call(cashier, "GET", route, nil, 403); code(body) != "PERMISSION_DENIED" {
				t.Fatalf("%s sin permiso: %s", route, body)
			}
		}
		call(cashier, "POST", "maintenance/backups/run", map[string]string{"operation_id": "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c01"}, 403)
		call(cashier, "PUT", "maintenance/backups/settings", map[string]any{"enabled": false, "time": nil, "keep": 5}, 403)
	})

	t.Run("estado_inicial", func(t *testing.T) {
		value := state(admin)
		if !value.Settings.Enabled || value.Settings.Time != nil || value.Settings.EffectiveTime != "05:00" || value.Settings.Keep != 30 {
			t.Fatalf("configuración inicial: %+v", value.Settings)
		}
		if value.Settings.Directory.Custom || value.Settings.DefaultDirectory.Path != target.directory {
			t.Fatalf("carpeta inicial: %+v", value.Settings)
		}
		if !value.Pending || value.NextRunAt == nil || value.Alert != nil || len(value.Runs) != 0 || !value.DirectoryChangeAvailable {
			t.Fatalf("estado inicial: %+v", value)
		}
	})

	t.Run("configuracion_validada_y_auditada", func(t *testing.T) {
		for _, body := range []map[string]any{
			{"enabled": true, "time": nil, "keep": 0},
			{"enabled": true, "time": nil, "keep": 366},
			{"enabled": true, "time": "25:00", "keep": 30},
			{"enabled": true, "time": "", "keep": 30},
			{"enabled": true, "time": "5:00", "keep": 30},
			{"time": nil, "keep": 30},
			{"enabled": true, "time": nil},
			{"enabled": true, "time": nil, "keep": 30, "directory": "/tmp"},
		} {
			if response := call(admin, "PUT", "maintenance/backups/settings", body, 400); code(response) != "VALIDATION_ERROR" {
				t.Fatalf("configuración inválida aceptada: %v", body)
			}
		}
		var value backupStateBody
		body := call(admin, "PUT", "maintenance/backups/settings", map[string]any{"enabled": false, "time": "03:30", "keep": 10}, 200)
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if value.Settings.Enabled || value.Settings.Time == nil || *value.Settings.Time != "03:30" || value.Settings.Keep != 10 ||
			value.NextRunAt != nil || value.Pending {
			t.Fatalf("configuración guardada: %+v", value)
		}
		audits := `SELECT count(*) FROM app.audit_events WHERE action = 'maintenance.backup_settings_changed'`
		if scalar(audits) != 1 {
			t.Fatal("no se auditó el cambio de configuración")
		}
		if scalar(audits+` AND before_state->>'keep' = '30' AND after_state->>'time' = '03:30' AND after_state->>'enabled' = 'false'`) != 1 {
			t.Fatal("la auditoría no tiene el antes y el después")
		}
		call(admin, "PUT", "maintenance/backups/settings", map[string]any{"enabled": false, "time": "03:30", "keep": 10}, 200)
		if scalar(audits) != 1 {
			t.Fatal("se auditó un guardado sin cambios")
		}
		call(admin, "PUT", "maintenance/backups/settings", map[string]any{"enabled": true, "time": nil, "keep": 30}, 200)
	})

	chosen := filepath.Join(root, "usb", "Respaldos")
	if err := os.MkdirAll(chosen, 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("carpeta_solo_desde_la_computadora_principal", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"directory": chosen})
		_, err := service.Execute(ctx, administration.Request{
			Method: "PUT", Path: "/api/v1/maintenance/backups/directory", Body: body,
			DeviceID: admin.id, DeviceToken: admin.secret, SessionToken: admin.session, Local: false,
		})
		var fault *administration.Fault
		if !errors.As(err, &fault) || fault.Code != "LOCAL_REQUIRED" || fault.Status != 403 {
			t.Fatalf("carpeta desde otra terminal: %v", err)
		}
		// Por la red, el estado dice que la carpeta no se puede cambiar desde ahí.
		result, err := service.Execute(ctx, administration.Request{
			Method: "GET", Path: "/api/v1/maintenance/backups",
			DeviceID: admin.id, DeviceToken: admin.secret, SessionToken: admin.session, Local: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if remote, ok := result.(backupschedule.State); !ok || remote.DirectoryChangeAvailable {
			t.Fatalf("estado por la red: %+v", result)
		}
	})

	t.Run("carpeta_validada_y_auditada", func(t *testing.T) {
		file := filepath.Join(root, "archivo.txt")
		if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		// También se rechazan las carpetas de los datos del servidor y de la base (directas o por un enlace).
		toLogs := filepath.Join(root, "atajo")
		if err := os.Symlink(filepath.Join(server, "logs"), toLogs); err != nil {
			t.Fatal(err)
		}
		for _, directory := range []string{"", "Respaldos", filepath.Join(root, "no-existe"), file, chosen + "\n", server,
			filepath.Join(server, "logs"), filepath.Join(server, "postgres", "pg_wal"), toLogs} {
			response := call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": directory}, 400)
			if code(response) != "BACKUP_DIRECTORY_INVALID" {
				t.Fatalf("carpeta %q: %s", directory, response)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "no-existe")); err == nil {
			t.Fatal("se creó la carpeta inexistente")
		}
		var value backupStateBody
		if err := json.Unmarshal(call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": chosen}, 200), &value); err != nil {
			t.Fatal(err)
		}
		if !value.Settings.Directory.Custom || value.Settings.Directory.Path != chosen || value.Settings.Directory.FolderName != "Respaldos" ||
			!value.Settings.Directory.Available {
			t.Fatalf("carpeta elegida: %+v", value.Settings.Directory)
		}
		audits := `SELECT count(*) FROM app.audit_events WHERE action = 'maintenance.backup_directory_changed'`
		if scalar(audits+` AND before_state->>'directory' IS NULL AND after_state->>'directory' = $1`, chosen) != 1 {
			t.Fatal("no se auditó el cambio de carpeta")
		}
		// Un enlace a la misma carpeta se guarda resuelto: no cambia nada ni se audita otra vez.
		toChosen := filepath.Join(root, "atajo-usb")
		if err := os.Symlink(chosen, toChosen); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": toChosen}, 200), &value); err != nil {
			t.Fatal(err)
		}
		if value.Settings.Directory.Path != chosen || scalar(audits) != 1 {
			t.Fatalf("carpeta por un enlace: %+v", value.Settings.Directory)
		}
		// Elegir la predeterminada es volver a ella.
		if err := json.Unmarshal(call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": target.directory}, 200), &value); err != nil {
			t.Fatal(err)
		}
		if value.Settings.Directory.Custom {
			t.Fatal("la carpeta predeterminada quedó como elegida")
		}
		call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": chosen}, 200)
		if err := json.Unmarshal(call(admin, "POST", "maintenance/backups/directory/default", map[string]any{}, 200), &value); err != nil {
			t.Fatal(err)
		}
		if value.Settings.Directory.Custom || scalar(audits) != 4 {
			t.Fatalf("volver a la predeterminada: %+v, %d auditorías", value.Settings.Directory, scalar(audits))
		}
		call(admin, "POST", "maintenance/backups/directory/default", nil, 200)
		if scalar(audits) != 4 {
			t.Fatal("se auditó un cambio sin efecto")
		}
		call(admin, "PUT", "maintenance/backups/directory", map[string]string{"directory": chosen}, 200)
	})

	created := `SELECT count(*) FROM app.audit_events WHERE action = 'maintenance.backup_created'`
	var firstRun string

	t.Run("respaldar_ahora_idempotente", func(t *testing.T) {
		if response := call(admin, "POST", "maintenance/backups/run", map[string]string{"operation_id": "no-es-uuid"}, 400); code(response) != "VALIDATION_ERROR" {
			t.Fatal("clave inválida aceptada")
		}
		operation := "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c11"
		first := run(admin, operation, 200)
		if first.Run.Status != "succeeded" || first.Run.Kind != "manual" || !first.Run.Verified || first.Run.Location != "custom" ||
			first.Run.FolderName != "Respaldos" || first.Run.Available == nil || !*first.Run.Available {
			t.Fatalf("respaldo manual: %+v", first.Run)
		}
		firstRun = first.Run.ID
		if _, err := os.Stat(filepath.Join(chosen, first.Run.FileName)); err != nil || !strings.HasSuffix(first.Run.FileName, "-manual.dump") {
			t.Fatalf("archivo del respaldo manual: %s %v", first.Run.FileName, err)
		}
		eventually("no se auditó el respaldo manual", func() bool {
			return scalar(created+` AND success AND entity_id = $1`, first.Run.ID) == 1
		})
		again := run(admin, operation, 200)
		if again.Run.ID != first.Run.ID || again.Run.Status != "succeeded" || target.dumpCount() != 1 {
			t.Fatalf("reenvío: %+v (%d copias)", again.Run, target.dumpCount())
		}
		if body := call(manager, "POST", "maintenance/backups/run", map[string]string{"operation_id": operation}, 409); code(body) != "IDEMPOTENCY_CONFLICT" {
			t.Fatalf("misma clave de otro usuario: %s", body)
		}
		if scalar(created) != 1 || scalar(`SELECT count(*) FROM app.backup_runs`) != 1 {
			t.Fatal("la misma clave respaldó o auditó otra vez")
		}
	})

	t.Run("fallo_de_verificacion_avisa_al_administrador", func(t *testing.T) {
		target.set(func(target *scheduledTarget) {
			target.failVerify = true
		})
		failed := run(admin, "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c21", 200)
		if failed.Run.Status != "failed" || failed.Run.FailureReason != "verify_failed" || failed.Run.Verified || failed.Run.FileName != "" {
			t.Fatalf("respaldo fallido: %+v", failed.Run)
		}
		eventually("no se auditó el respaldo fallido", func() bool {
			return scalar(created+` AND NOT success AND after_state->>'failure_reason' = 'verify_failed'`) == 1
		})
		var summary struct {
			Alert *struct {
				Kind     string `json:"kind"`
				Reason   string `json:"reason"`
				Failures int    `json:"failures"`
			} `json:"alert"`
			LastBackup *struct {
				Status string `json:"status"`
			} `json:"last_backup"`
		}
		if err := json.Unmarshal(call(manager, "GET", "maintenance/backups/status", nil, 200), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Alert == nil || summary.Alert.Kind != "failed" || summary.Alert.Reason != "verify_failed" || summary.Alert.Failures != 1 ||
			summary.LastBackup == nil || summary.LastBackup.Status != "failed" {
			t.Fatalf("aviso al administrador: %+v", summary)
		}
		var status serviceStatusBody
		if err := json.Unmarshal(call(admin, "GET", "maintenance/service", nil, 200), &status); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(status.LastBackup), `"status":"failed"`) {
			t.Fatalf("último respaldo en el estado del servicio: %s", status.LastBackup)
		}
		if files, _ := os.ReadDir(chosen); len(files) != 1 {
			t.Fatalf("quedó el respaldo que no se pudo verificar: %d archivos", len(files))
		}
		target.set(func(target *scheduledTarget) {
			target.failVerify = false
		})
	})

	t.Run("respaldo_en_curso", func(t *testing.T) {
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		target.set(func(target *scheduledTarget) {
			target.block = release
			target.started = started
		})
		slow := "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c31"
		// La petición tiene 7 s: el respaldo sigue después de responder «en curso».
		shortContext, cancelShort := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancelShort()
		answered := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			answered <- send(shortContext, admin, "POST", "maintenance/backups/run", map[string]string{"operation_id": slow})
		}()
		<-started
		if body := call(admin, "POST", "maintenance/backups/run", map[string]string{"operation_id": "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c41"}, 409); code(body) != "BACKUP_IN_PROGRESS" {
			t.Fatalf("otro respaldo con uno en curso: %s", body)
		}
		if again := run(admin, slow, 200); again.Run.Status != "running" {
			t.Fatalf("reenvío de un respaldo en curso: %+v", again.Run)
		}
		response := <-answered
		var pending backupRunBody
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &pending) != nil || pending.Run.Status != "running" {
			t.Fatalf("respuesta al vencer el plazo: %d %s", response.Code, response.Body.String())
		}
		if value := state(admin); !value.Running {
			t.Fatal("el estado no dice que hay un respaldo en curso")
		}
		close(release)
		eventually("el respaldo en curso no terminó", func() bool {
			return scalar(`SELECT count(*) FROM app.backup_runs WHERE status = 'succeeded'`) == 2
		})
		eventually("no se auditó el respaldo que terminó después de responder", func() bool {
			return scalar(created+` AND success`) == 2
		})
		target.set(func(target *scheduledTarget) {
			target.block = nil
			target.started = nil
		})
		value := state(admin)
		if value.Running || value.Alert != nil || len(value.Runs) != 3 {
			t.Fatalf("un respaldo correcto posterior quita el aviso: %+v", value)
		}
		if final := run(admin, slow, 200); final.Run.Status != "succeeded" || final.Run.ID == firstRun {
			t.Fatalf("resultado del respaldo lento: %+v", final.Run)
		}
	})
	service.ScheduledBackups.Wait()
}
