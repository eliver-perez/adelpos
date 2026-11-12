//go:build integration

package administration_test

import (
	"encoding/json"
	"testing"
)

// testLongOpenHours cubre la etapa 9D en administración: el umbral de cuenta abierta por mucho tiempo
// (§3.4, restaurant_settings.long_open_hours de la 0021) se lee con cualquier sesión, se cambia solo
// con el permiso de «Funciones del establecimiento» (settings.features.manage) y Go lo valida de 1 a 72
// horas con revisión optimista, auditoría y aviso en tiempo real; los umbrales del tablero para llevar
// (PUT restaurant/settings) no lo cambian y el mapa lo entrega. También comprueba el permiso nuevo
// «Cancelar cuentas» en el registro de Go y en el administrador.
func testLongOpenHours(t *testing.T, env *delegationEnvironment) {
	type threshold struct {
		LongOpenHours int `json:"long_open_hours"`
		MinimumHours  int `json:"minimum_hours"`
		MaximumHours  int `json:"maximum_hours"`
		Revision      int `json:"revision"`
	}
	read := func(actor terminal) threshold {
		var value threshold
		if err := json.Unmarshal(env.call(actor, "GET", "settings/long-open-hours", nil, 200), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	t.Run("9D_umbral_lectura_y_permiso", func(t *testing.T) {
		initial := read(env.admin)
		if initial.LongOpenHours != 12 || initial.MinimumHours != 1 || initial.MaximumHours != 72 {
			t.Fatalf("umbral inicial: %+v", initial)
		}
		_, reader := env.createUser(t, "lector_umbral_9d", env.createRole(t, "Lector umbral 9D", "settings.read"))
		if value := read(reader); value.LongOpenHours != 12 {
			t.Fatalf("lectura sin permiso de funciones: %+v", value)
		}
		denied := env.deniedCount(t)
		env.call(reader, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": 6, "revision": initial.Revision}, 403)
		if env.deniedCount(t) != denied+1 || read(env.admin).LongOpenHours != 12 {
			t.Fatal("un usuario sin «Funciones del establecimiento» cambió el umbral o no se auditó la denegación")
		}
	})

	t.Run("9D_umbral_validacion_revision_y_auditoria", func(t *testing.T) {
		initial := read(env.admin)
		for _, hours := range []any{0, 73, -1, "12", nil} {
			env.call(env.admin, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": hours, "revision": initial.Revision},
				map[bool]int{true: 400, false: 422}[hours == "12"])
		}
		env.call(env.admin, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": 6, "revision": initial.Revision + 5}, 409)
		events := env.scalar(t, "SELECT count(*) FROM app.realtime_events WHERE kind='restaurant.settings.changed'")
		var saved threshold
		if err := json.Unmarshal(env.call(env.admin, "PUT", "settings/long-open-hours",
			map[string]any{"long_open_hours": 6, "revision": initial.Revision}, 200), &saved); err != nil {
			t.Fatal(err)
		}
		if saved.LongOpenHours != 6 || saved.Revision != initial.Revision+1 {
			t.Fatalf("umbral guardado: %+v", saved)
		}
		if env.scalar(t, "SELECT count(*) FROM app.realtime_events WHERE kind='restaurant.settings.changed'") != events+2 {
			t.Fatal("el cambio debía avisar al mapa, a las cuentas y al tablero para llevar")
		}
		if env.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='settings.long_open_hours_changed' AND success
            AND before_state->>'long_open_hours'='12' AND after_state->>'long_open_hours'='6'`) != 1 {
			t.Fatal("el cambio del umbral no quedó auditado")
		}
		// Repetir el mismo valor no escribe.
		env.call(env.admin, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": 6, "revision": saved.Revision}, 200)
		if read(env.admin).Revision != saved.Revision {
			t.Fatal("guardar el mismo umbral cambió la revisión")
		}
	})

	t.Run("9D_umbral_en_el_mapa_y_tablero_para_llevar", func(t *testing.T) {
		current := read(env.admin)
		var settings map[string]any
		if err := json.Unmarshal(env.call(env.admin, "GET", "restaurant/settings", nil, 200), &settings); err != nil {
			t.Fatal(err)
		}
		if settings["long_open_hours"] != float64(current.LongOpenHours) {
			t.Fatalf("restaurant/settings no entrega el umbral: %v", settings)
		}
		// Los umbrales del tablero para llevar no cambian el de cuenta abierta aunque el cliente lo envíe.
		env.call(env.admin, "PUT", "restaurant/settings", map[string]any{"operation_id": "00000000-0000-4000-8000-00000000d901",
			"warning_minutes": 20, "overdue_minutes": 40, "long_open_hours": 48, "revision": current.Revision}, 200)
		after := read(env.admin)
		if after.LongOpenHours != current.LongOpenHours || after.Revision != current.Revision+1 {
			t.Fatalf("restaurant/settings cambió el umbral: %+v", after)
		}
		env.call(env.admin, "PUT", "settings/long-open-hours", map[string]any{"long_open_hours": 12, "revision": after.Revision}, 200)
	})

	t.Run("9D_permiso_cancelar_cuentas", func(t *testing.T) {
		var permissions []struct {
			Key         string `json:"key"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(env.call(env.admin, "GET", "permissions", nil, 200), &permissions); err != nil {
			t.Fatal(err)
		}
		descriptions := map[string]string{}
		for _, permission := range permissions {
			descriptions[permission.Key] = permission.Description
		}
		if descriptions["accounts.cancel"] != "Cancelar cuentas" ||
			descriptions["accounts.manage"] != "Editar datos de cuentas y cancelar cuentas vacías con motivo libre" {
			t.Fatalf("descripciones visibles: %q / %q", descriptions["accounts.cancel"], descriptions["accounts.manage"])
		}
		if env.scalar(t, `SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
            WHERE p.permission_key='accounts.cancel' AND r.code='administrator'`) != 1 ||
			env.scalar(t, `SELECT count(*) FROM app.role_permissions p JOIN app.roles r ON r.id=p.role_id
            WHERE p.permission_key='accounts.cancel' AND r.code<>'administrator'`) != 0 {
			t.Fatal("«Cancelar cuentas» debía concederse solo al administrador")
		}
	})
}
