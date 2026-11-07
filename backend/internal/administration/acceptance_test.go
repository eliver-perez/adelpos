//go:build integration

package administration_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"local/ad-el-pos/server/internal/database"
	"local/ad-el-pos/server/internal/httpapi"
)

type terminal struct {
	id      string
	secret  string
	session string
}

func TestStageTwoAcceptance(t *testing.T) {
	dsn := os.Getenv("ADEL_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/adel_pos_test_") {
		t.Fatal("Se requiere una base temporal exclusiva")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("No se pudo crear el pool de prueba")
	}
	defer pool.Close()
	if _, err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	provisioning := administration.NewToken()
	filename := filepath.Join(t.TempDir(), "provisioning-token")
	if err := os.WriteFile(filename, []byte(provisioning), 0600); err != nil {
		t.Fatal(err)
	}
	service := &administration.Service{Pool: pool, ProvisioningFile: filename}
	handler := httpapi.New(func(ctx context.Context) database.Health {
		return database.Probe(ctx, pool)
	}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), service)

	call := func(actor terminal, method, route string, body any, expected int) json.RawMessage {
		t.Helper()
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequest(method, "/api/v1/"+route, bytes.NewReader(encoded))
		request.RemoteAddr = "127.0.0.1:50505"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Device-ID", actor.id)
		request.Header.Set("X-Device-Token", actor.secret)
		request.Header.Set("Authorization", "Bearer "+actor.session)
		request.Header.Set("X-Provisioning-Token", provisioning)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			var failure struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(response.Body.Bytes(), &failure)
			t.Fatalf("%s %s: esperado %d, recibido %d (%s)", method, route, expected, response.Code, failure.Error.Code)
		}
		return append(json.RawMessage{}, response.Body.Bytes()...)
	}
	text := func(body json.RawMessage, key string) string {
		t.Helper()
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		var result string
		_ = json.Unmarshal(fields[key], &result)
		return result
	}
	var primary terminal
	setup := map[string]any{"name": "Restaurante de prueba temporal", "timezone": "America/Monterrey", "currency": "MXN", "theme": "dark", "terminal_name": "Terminal de prueba"}

	t.Run("01_initial_setup_and_02_idempotence", func(t *testing.T) {
		response := call(terminal{}, "POST", "setup", setup, 200)
		primary.id, primary.secret = text(response, "device_id"), text(response, "device_token")
		call(terminal{}, "POST", "setup", setup, 200)
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.establishment").Scan(&count); err != nil || count != 1 {
			t.Fatal("La inicialización no fue idempotente")
		}
		wrong := httptest.NewRequest("POST", "/api/v1/setup", bytes.NewReader(response))
		wrong.RemoteAddr = "192.168.1.5:5000"
		rejected := httptest.NewRecorder()
		handler.ServeHTTP(rejected, wrong)
		if rejected.Code != 403 {
			t.Fatal("Se permitió aprovisionamiento remoto")
		}
	})
	t.Run("03_register_without_economic_movements", func(t *testing.T) {
		var name string
		if err := pool.QueryRow(ctx, "SELECT name FROM app.registers").Scan(&name); err != nil || name != "Caja 1" {
			t.Fatal("No se creó Caja 1")
		}
	})
	t.Run("04_default_admin_and_05_nonblocking_warning", func(t *testing.T) {
		response := call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 200)
		primary.session = text(response, "session_token")
		identity := call(primary, "GET", "auth/me", nil, 200)
		if !strings.Contains(string(identity), `"initial_password":true`) {
			t.Fatal("No se informó la contraseña inicial")
		}
		call(primary, "GET", "users", nil, 200)
	})
	password := administration.NewToken()
	t.Run("06_password_change_survives_service_restart", func(t *testing.T) {
		response := call(primary, "POST", "auth/password", map[string]string{"current_password": "admin", "new_password": password}, 200)
		oldSession := primary
		primary.session = text(response, "session_token")
		call(oldSession, "GET", "users", nil, 401)
		handler = httpapi.New(func(ctx context.Context) database.Health {
			return database.Probe(ctx, pool)
		}, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)), &administration.Service{Pool: pool, ProvisioningFile: filename})
		call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": "admin"}, 401)
		response = call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": password}, 200)
		primary.session = text(response, "session_token")
	})
	var supportID string
	supportPassword := administration.NewToken()
	t.Run("07_local_sa_provisioning_08_hidden_09_protected", func(t *testing.T) {
		if err := service.ProvisionSupport(ctx, supportPassword, false); err != nil {
			t.Fatal(err)
		}
		if err := service.ProvisionSupport(ctx, administration.NewToken(), false); err == nil {
			t.Fatal("Se reemplazó una credencial de soporte existente")
		}
		if err := pool.QueryRow(ctx, "SELECT id FROM app.users WHERE username='sa'").Scan(&supportID); err != nil {
			t.Fatal(err)
		}
		users := call(primary, "GET", "users", nil, 200)
		if strings.Contains(string(users), supportID) {
			t.Fatal("sa apareció en usuarios ordinarios")
		}
		call(primary, "PATCH", "users/"+supportID, map[string]string{"username": "sa", "display_name": "Soporte", "role_id": supportID}, 400)
		call(primary, "PUT", "users/"+supportID+"/permissions", map[string]any{"overrides": map[string]string{"audit.read": "deny"}}, 403)
	})
	var employeeID, roleID string
	employeePassword := administration.NewToken()
	t.Run("10_users_and_11_custom_roles", func(t *testing.T) {
		response := call(primary, "POST", "roles", map[string]any{"name": "Rol temporal", "permissions": []string{"registers.read"}}, 200)
		roleID = text(response, "id")
		call(primary, "PATCH", "roles/"+roleID, map[string]any{"name": "Rol editado", "permissions": []string{"registers.read"}}, 200)
		response = call(primary, "POST", "users", map[string]any{"username": "prueba", "display_name": "Empleado de prueba", "role_id": roleID, "active": true, "password": employeePassword}, 200)
		employeeID = text(response, "id")
		call(primary, "PATCH", "users/"+employeeID, map[string]any{"username": "prueba", "display_name": "Nombre actualizado", "role_id": roleID, "active": true}, 200)
	})
	secondResponse := call(primary, "POST", "devices/invitations", map[string]any{}, 200)
	code := text(secondResponse, "code")
	enrolled := call(terminal{}, "POST", "devices/enroll", map[string]string{"name": "Segunda terminal", "code": code}, 200)
	second := terminal{id: text(enrolled, "device_id"), secret: text(enrolled, "device_token")}
	call(terminal{}, "POST", "devices/enroll", map[string]string{"name": "No debe crearse", "code": code}, 401)
	employee := second
	employee.session = text(call(employee, "POST", "auth/login", map[string]string{"username": "prueba", "password": employeePassword}, 200), "session_token")
	t.Run("12_permission_precedence_and_13_actual_denial", func(t *testing.T) {
		call(employee, "GET", "registers", nil, 200)
		call(employee, "GET", "users", nil, 403)

		call(primary, "PATCH", "roles/"+roleID, map[string]any{"name": "Rol editado", "permissions": []string{}}, 200)
		call(employee, "GET", "registers", nil, 403)
		call(primary, "PATCH", "roles/"+roleID, map[string]any{"name": "Rol editado", "permissions": []string{"registers.read"}}, 200)
		call(employee, "GET", "registers", nil, 200)
		call(primary, "PUT", "users/"+employeeID+"/permissions", map[string]any{"overrides": map[string]string{"registers.read": "deny", "users.read": "allow"}}, 200)
		call(employee, "GET", "registers", nil, 403)
		call(employee, "GET", "users", nil, 200)
		call(primary, "PUT", "users/"+employeeID+"/permissions", map[string]any{"overrides": map[string]string{"registers.read": "inherit", "users.read": "inherit"}}, 200)
		call(employee, "GET", "registers", nil, 200)
		call(employee, "GET", "users", nil, 403)
	})
	t.Run("17_preferences_and_18_terminal_keyboard", func(t *testing.T) {
		call(employee, "PUT", "preferences", map[string]string{"theme": "dark", "home_screen": "users"}, 200)
		response := call(employee, "GET", "preferences", nil, 200)
		if text(response, "theme") != "dark" || text(response, "home_screen") != "status" {
			t.Fatal("No persistieron preferencias o se permitió una pantalla sin permiso")
		}
		call(employee, "PUT", "terminal/preferences", map[string]string{"keyboard_mode": "always"}, 200)
		var mode string
		if err := pool.QueryRow(ctx, "SELECT keyboard_mode FROM app.devices WHERE id=$1", second.id).Scan(&mode); err != nil || mode != "always" {
			t.Fatal("No persistió el modo de teclado")
		}
	})
	t.Run("14_new_login_invalidates_previous_session", func(t *testing.T) {
		latest := primary
		latest.session = text(call(latest, "POST", "auth/login", map[string]string{"username": "prueba", "password": employeePassword}, 200), "session_token")
		response := call(employee, "GET", "auth/me", nil, 401)
		if !strings.Contains(string(response), "SESSION_REPLACED") {
			t.Fatal("No se informó el nuevo acceso")
		}
		call(latest, "GET", "auth/me", nil, 200)
	})
	t.Run("15_device_revocation_and_16_enforcement", func(t *testing.T) {
		call(primary, "POST", "devices/"+second.id+"/revoke", map[string]any{}, 200)
		call(employee, "GET", "auth/me", nil, 401)
		forged := primary
		forged.id = second.id
		call(forged, "GET", "users", nil, 401)
	})
	t.Run("19_authorized_audit_and_20_preserved_history", func(t *testing.T) {
		call(primary, "PATCH", "users/"+employeeID, map[string]any{"username": "prueba", "display_name": "Empleado inactivo", "role_id": roleID, "active": false}, 200)
		response := call(primary, "GET", "audit?action=user.created&page_size=1", nil, 200)
		if !strings.Contains(string(response), employeeID) {
			t.Fatal("Se perdió el historial al desactivar el usuario")
		}
		var history string
		if err := pool.QueryRow(ctx, "SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]')::text FROM app.audit_events a").Scan(&history); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{primary.secret, primary.session, password, supportPassword, employeePassword, "password_hash", "session_token", "device_token"} {
			if strings.Contains(history, secret) {
				t.Fatal("La auditoría contiene un secreto")
			}
		}
	})
	t.Run("settings_registers_and_administrative_session_revocation", func(t *testing.T) {
		updatedSettings := map[string]any{
			"name": "Configuración temporal actualizada", "timezone": "America/Tijuana",
			"currency": "MXN", "phone": "", "email": "", "address": "", "theme": "light",
		}
		call(primary, "PUT", "settings", updatedSettings, 200)
		savedSettings := call(primary, "GET", "settings", nil, 200)
		if text(savedSettings, "timezone") != "America/Tijuana" {
			t.Fatal("La zona horaria no se persistió")
		}
		response := call(primary, "POST", "registers", map[string]any{"name": "Caja adicional", "description": "Solo prueba temporal", "active": true}, 200)
		registerID := text(response, "id")
		call(primary, "PATCH", "registers/"+registerID, map[string]any{"name": "Caja renombrada", "description": "Editada", "active": true}, 200)
		call(primary, "PATCH", "devices/"+primary.id, map[string]any{"name": "Terminal editada", "register_id": registerID, "keyboard_mode": "disabled"}, 200)
		call(primary, "PATCH", "registers/"+registerID, map[string]any{"name": "Caja renombrada", "description": "Editada", "active": false}, 409)
		call(primary, "PATCH", "devices/"+primary.id, map[string]any{"name": "Terminal editada", "register_id": nil, "keyboard_mode": "auto"}, 200)
		call(primary, "PATCH", "registers/"+registerID, map[string]any{"name": "Caja renombrada", "description": "Editada", "active": false}, 200)

		localCode, err := service.CreateLocalEnrollment(ctx)
		if err != nil {
			t.Fatal(err)
		}
		local := call(terminal{}, "POST", "devices/enroll", map[string]string{"name": "Recuperación local temporal", "code": localCode}, 200)
		recovered := terminal{id: text(local, "device_id"), secret: text(local, "device_token")}
		recovered.session = text(call(recovered, "POST", "auth/login", map[string]string{"username": "admin", "password": password}, 200), "session_token")
		var current struct {
			User struct {
				SessionID string `json:"session_id"`
			} `json:"user"`
		}
		if err := json.Unmarshal(call(recovered, "GET", "auth/me", nil, 200), &current); err != nil {
			t.Fatal(err)
		}
		primary.session = text(call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": password}, 200), "session_token")
		call(primary, "POST", "sessions/"+current.User.SessionID+"/revoke", map[string]any{}, 200)
		call(recovered, "GET", "auth/me", nil, 401)
		call(primary, "GET", "sessions", nil, 200)

		var adminRoleID string
		if err := pool.QueryRow(ctx, "SELECT role_id FROM app.users WHERE username='admin'").Scan(&adminRoleID); err != nil {
			t.Fatal(err)
		}
		call(primary, "PATCH", "users/"+supportID, map[string]any{"username": "support_attempt", "display_name": "Cambio no autorizado", "role_id": adminRoleID, "active": false}, 403)
	})
	t.Run("expired_sessions_and_anonymous_requests", func(t *testing.T) {
		call(terminal{}, "GET", "users", nil, 401)
		if _, err := pool.Exec(ctx, "UPDATE app.sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", administration.TokenHash(primary.session)); err != nil {
			t.Fatal(err)
		}
		call(primary, "GET", "users", nil, 401)
		primary.session = text(call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": password}, 200), "session_token")
	})
	t.Run("sa_normal_login_and_privileged_audit", func(t *testing.T) {
		support := primary
		support.session = text(call(support, "POST", "auth/login", map[string]string{"username": "sa", "password": supportPassword}, 200), "session_token")
		call(support, "GET", "users", nil, 200)
		var events int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.audit_events WHERE actor_id=$1 AND action='support.read'", supportID).Scan(&events); err != nil || events < 1 {
			t.Fatal("La actividad de soporte no quedó auditada")
		}
	})
	t.Run("failed_login_throttling", func(t *testing.T) {
		for attempt := 0; attempt < 5; attempt++ {
			call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": "incorrecta"}, 401)
		}
		call(primary, "POST", "auth/login", map[string]string{"username": "admin", "password": password}, 429)
	})
}
