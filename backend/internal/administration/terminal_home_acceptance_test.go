//go:build integration

package administration_test

import (
	"encoding/json"
	"testing"
)

// terminalHomeOf lee la pantalla de inicio que informa la identidad (`auth/me`) de un actor.
func terminalHomeOf(t *testing.T, env *delegationEnvironment, actor terminal) string {
	t.Helper()
	var identity struct {
		Terminal struct {
			HomeScreen string `json:"home_screen"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.call(actor, "GET", "auth/me", nil, 200), &identity); err != nil {
		t.Fatal(err)
	}
	return identity.Terminal.HomeScreen
}

// storedTerminalHome lee la columna devices.home_screen directamente de PostgreSQL.
func storedTerminalHome(t *testing.T, env *delegationEnvironment, deviceID string) string {
	t.Helper()
	var screen string
	if err := env.pool.QueryRow(env.ctx, "SELECT home_screen FROM app.devices WHERE id=$1", deviceID).Scan(&screen); err != nil {
		t.Fatal(err)
	}
	return screen
}

// testTerminalHome cubre la pantalla de inicio por terminal de la etapa 9C (§1.5, migración 0020):
// Mesas por omisión, lectura en la identidad y en la lista de terminales, edición en el recurso de
// terminales con el permiso existente devices.manage, lista cerrada validada en Go y en el CHECK de
// la base, conservación del valor cuando un cliente anterior no lo envía y auditoría del cambio.
func testTerminalHome(t *testing.T, env *delegationEnvironment) {
	deviceID := env.admin.id
	var name string
	var registerID *string
	var keyboard string
	if err := env.pool.QueryRow(env.ctx, "SELECT name,register_id,keyboard_mode FROM app.devices WHERE id=$1", deviceID).Scan(&name, &registerID, &keyboard); err != nil {
		t.Fatal(err)
	}
	body := func(home any) map[string]any {
		values := map[string]any{"name": name, "register_id": registerID, "keyboard_mode": keyboard}
		if home != nil {
			values["home_screen"] = home
		}
		return values
	}

	t.Run("9C_lectura_mesas_por_omision", func(t *testing.T) {
		if home := terminalHomeOf(t, env, env.admin); home != "tables" {
			t.Fatalf("la terminal debía iniciar en Mesas, informó %q", home)
		}
		var devices []map[string]any
		if err := json.Unmarshal(env.call(env.admin, "GET", "devices", nil, 200), &devices); err != nil {
			t.Fatal(err)
		}
		if len(devices) == 0 {
			t.Fatal("no se listaron terminales")
		}
		for _, device := range devices {
			if device["home_screen"] != "tables" {
				t.Fatalf("la lista de terminales no informó la pantalla de inicio: %v", device["home_screen"])
			}
		}
	})

	t.Run("9C_edicion_con_permiso_y_auditoria", func(t *testing.T) {
		before := env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action='device.updated' AND entity_id=$1", deviceID)
		var saved map[string]any
		if err := json.Unmarshal(env.call(env.admin, "PATCH", "devices/"+deviceID, body("kitchen"), 200), &saved); err != nil {
			t.Fatal(err)
		}
		if saved["home_screen"] != "kitchen" || storedTerminalHome(t, env, deviceID) != "kitchen" {
			t.Fatal("no se guardó la pantalla de inicio de la terminal")
		}
		if home := terminalHomeOf(t, env, env.admin); home != "kitchen" {
			t.Fatalf("la identidad no informó la pantalla nueva: %q", home)
		}
		audited := env.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='device.updated' AND entity_id=$1
            AND before_state->>'home_screen'='tables' AND after_state->>'home_screen'='kitchen' AND success`, deviceID)
		if audited != 1 || env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action='device.updated' AND entity_id=$1", deviceID) != before+1 {
			t.Fatal("el cambio de la pantalla de inicio no quedó auditado con el valor anterior y el nuevo")
		}
		// Un cliente anterior que no envía el campo edita la terminal sin cambiar su inicio.
		env.call(env.admin, "PATCH", "devices/"+deviceID, body(nil), 200)
		if storedTerminalHome(t, env, deviceID) != "kitchen" {
			t.Fatal("una edición sin home_screen cambió la pantalla de inicio")
		}
	})

	t.Run("9C_lista_cerrada_en_go_y_en_la_base", func(t *testing.T) {
		for _, rejected := range []any{"status", "administration", "catalog", "TABLES", "", 3} {
			env.call(env.admin, "PATCH", "devices/"+deviceID, body(rejected), 400)
		}
		if storedTerminalHome(t, env, deviceID) != "kitchen" {
			t.Fatal("un valor fuera de la lista cerrada cambió la pantalla de inicio")
		}
		for _, accepted := range []string{"takeaway", "cash", "tables"} {
			env.call(env.admin, "PATCH", "devices/"+deviceID, body(accepted), 200)
			if storedTerminalHome(t, env, deviceID) != accepted {
				t.Fatalf("no se aceptó %q de la lista cerrada", accepted)
			}
		}
		// El CHECK con nombre de la 0020 respalda la validación de Go.
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.devices SET home_screen='status' WHERE id=$1", deviceID); err == nil {
			t.Fatal("la base aceptó una pantalla de inicio fuera de la lista cerrada")
		}
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.devices SET home_screen=NULL WHERE id=$1", deviceID); err == nil {
			t.Fatal("la base aceptó una pantalla de inicio nula")
		}
	})

	t.Run("9C_sin_devices_manage_no_se_edita", func(t *testing.T) {
		readerRole := env.createRole(t, "Inicio por terminal: solo lectura", "devices.read")
		_, reader := env.createUser(t, "lector_terminales", readerRole)
		denials := env.deniedCount(t)
		env.call(reader, "PATCH", "devices/"+deviceID, body("cash"), 403)
		if storedTerminalHome(t, env, deviceID) != "tables" || env.deniedCount(t) != denials+1 {
			t.Fatal("un usuario sin devices.manage cambió la pantalla de inicio o no se auditó la denegación")
		}
		// La pantalla de inicio es de la terminal: cualquier usuario que entre en ella la recibe.
		if home := terminalHomeOf(t, env, reader); home != "tables" {
			t.Fatalf("el usuario de la terminal no recibió su pantalla de inicio: %q", home)
		}
	})
}
