//go:build integration

package administration_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/events"
)

// userStations es la forma de GET users/{id}/kitchen-stations (etapa 9F).
type userStations struct {
	UserID          string   `json:"user_id"`
	AllStations     bool     `json:"all_stations"`
	StationIDs      []string `json:"station_ids"`
	AllByPermission bool     `json:"all_by_permission"`
	Stations        []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	} `json:"stations"`
}

// testKitchenUserStations cubre la etapa 9F, fase C (§5.2): «Estaciones que puede atender» en el usuario,
// con «todas» por omisión o una selección, validada en Go, auditada en la misma transacción, con
// conflicto ante una edición concurrente, sin editar las propias ni las de un usuario con más permisos,
// y con kitchen.stations.all como «siempre todas».
func testKitchenUserStations(t *testing.T, env *delegationEnvironment) {
	read := func(actor terminal, userID string) userStations {
		t.Helper()
		var value userStations
		if err := json.Unmarshal(env.call(actor, "GET", "users/"+userID+"/kitchen-stations", nil, 200), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	save := func(actor terminal, userID string, body map[string]any, expected int) json.RawMessage {
		t.Helper()
		return env.call(actor, "PUT", "users/"+userID+"/kitchen-stations", body, expected)
	}
	station := func(name string) string {
		t.Helper()
		return env.text(env.call(env.admin, "POST", "kitchen/stations", map[string]any{
			"operation_id": events.NewID(), "name": name, "description": "", "active": true, "sort_order": 0,
			"reception": "kds", "printer_id": nil,
		}, 200), "id")
	}
	var bar, desserts, cookID, adminID string
	var manager terminal

	t.Run("9F_estaciones_del_usuario_todas_por_omision", func(t *testing.T) {
		bar = station("Barra usuarios 9F")
		desserts = station("Postres usuarios 9F")
		cookID, _ = env.createUser(t, "cocina_usuarios_9f", env.createRole(t, "Cocina usuarios 9F", "kitchen.states.read"))
		value := read(env.admin, cookID)
		if value.UserID != cookID || !value.AllStations || len(value.StationIDs) != 0 || value.AllByPermission {
			t.Fatalf("usuario nuevo: %+v", value)
		}
		listed := []string{}
		for _, entry := range value.Stations {
			listed = append(listed, entry.Name)
		}
		if !slices.Contains(listed, "Barra usuarios 9F") || !slices.Contains(listed, "Postres usuarios 9F") {
			t.Fatalf("estaciones para elegir: %v", listed)
		}
		if err := env.pool.QueryRow(env.ctx, "SELECT id::text FROM app.users WHERE username='admin'").Scan(&adminID); err != nil {
			t.Fatal(err)
		}
		// El administrador tiene kitchen.stations.all: siempre atiende todas.
		if admin := read(env.admin, adminID); !admin.AllStations || !admin.AllByPermission {
			t.Fatalf("administrador: %+v", admin)
		}
	})

	t.Run("9F_seleccion_validada_auditada_y_con_conflicto", func(t *testing.T) {
		invalid := []map[string]any{
			{"all_stations": true, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{}},
			{"all_stations": false, "station_ids": []string{}, "expected_all_stations": true, "expected_station_ids": []string{}},
			{"all_stations": false, "station_ids": []string{bar, bar}, "expected_all_stations": true, "expected_station_ids": []string{}},
			{"all_stations": false, "station_ids": []string{"no-es-una-estacion"}, "expected_all_stations": true, "expected_station_ids": []string{}},
			{"all_stations": false, "station_ids": []string{events.NewID()}, "expected_all_stations": true, "expected_station_ids": []string{}},
			{"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{}, "extra": 1},
		}
		for _, body := range invalid {
			save(env.admin, cookID, body, 400)
		}
		// La selección esperada no coincide con la vigente: conflicto sin escribir.
		conflict := save(env.admin, cookID, map[string]any{
			"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": false, "expected_station_ids": []string{},
		}, 409)
		if !strings.Contains(string(conflict), "REVISION_CONFLICT") {
			t.Fatalf("conflicto: %s", conflict)
		}
		before := env.scalar(t, "SELECT count(*) FROM app.realtime_events WHERE kind='kitchen.access.changed'")
		var saved userStations
		if err := json.Unmarshal(save(env.admin, cookID, map[string]any{
			"all_stations": false, "station_ids": []string{desserts, bar}, "expected_all_stations": true, "expected_station_ids": []string{},
		}, 200), &saved); err != nil {
			t.Fatal(err)
		}
		expected := []string{bar, desserts}
		slices.Sort(expected)
		if saved.AllStations || !slices.Equal(saved.StationIDs, expected) {
			t.Fatalf("selección guardada: %+v", saved)
		}
		if value := read(env.admin, cookID); value.AllStations || !slices.Equal(value.StationIDs, expected) {
			t.Fatalf("selección leída: %+v", value)
		}
		if env.scalar(t, `SELECT count(*) FROM app.audit_events WHERE action='user.kitchen_stations_changed' AND entity_id=$1
            AND (before_state->>'all_stations')::boolean AND NOT (after_state->>'all_stations')::boolean
            AND jsonb_array_length(after_state->'station_ids')=2`, cookID) != 1 {
			t.Fatal("el cambio no quedó auditado con la selección anterior y la nueva")
		}
		if env.scalar(t, "SELECT count(*) FROM app.realtime_events WHERE kind='kitchen.access.changed'") != before+1 {
			t.Fatal("las pantallas de Cocina no recibieron el aviso del cambio")
		}
		// De vuelta a «todas».
		save(env.admin, cookID, map[string]any{
			"all_stations": true, "station_ids": []string{}, "expected_all_stations": false, "expected_station_ids": []string{bar, desserts},
		}, 200)
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_user_stations WHERE user_id=$1", cookID) != 0 {
			t.Fatal("con «todas» no se conserva una selección")
		}
		save(env.admin, cookID, map[string]any{
			"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{},
		}, 200)
	})

	t.Run("9F_estaciones_del_usuario_con_permisos_y_delegacion", func(t *testing.T) {
		// Consultar exige users.read y cambiar exige users.manage; las denegaciones se auditan y no escriben.
		_, reader := env.createUser(t, "lector_usuarios_9f", env.createRole(t, "Lector de usuarios 9F", "users.read"))
		_, outsider := env.createUser(t, "sin_usuarios_9f", env.createRole(t, "Sin usuarios 9F", "settings.read"))
		denied := env.deniedCount(t)
		read(reader, cookID)
		env.call(outsider, "GET", "users/"+cookID+"/kitchen-stations", nil, 403)
		save(reader, cookID, map[string]any{
			"all_stations": true, "station_ids": []string{}, "expected_all_stations": false, "expected_station_ids": []string{bar},
		}, 403)
		if env.deniedCount(t) != denied+2 {
			t.Fatal("las denegaciones no quedaron auditadas")
		}
		// Un encargado con users.manage no cambia sus propias estaciones ni las de un usuario con más
		// permisos (el administrador), y sí las del usuario de cocina.
		var managerID string
		managerID, manager = env.createUser(t, "encargado_usuarios_9f", env.createRole(t, "Encargado de usuarios 9F",
			"users.read", "users.manage", "roles.read", "kitchen.states.read"))
		save(manager, managerID, map[string]any{
			"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{},
		}, 403)
		save(manager, adminID, map[string]any{
			"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{},
		}, 403)
		save(env.admin, adminID, map[string]any{
			"all_stations": false, "station_ids": []string{bar}, "expected_all_stations": true, "expected_station_ids": []string{},
		}, 403)
		save(manager, cookID, map[string]any{
			"all_stations": false, "station_ids": []string{desserts}, "expected_all_stations": false, "expected_station_ids": []string{bar},
		}, 200)
		if env.scalar(t, "SELECT count(*) FROM app.kitchen_user_stations WHERE user_id=$1", managerID)+
			env.scalar(t, "SELECT count(*) FROM app.kitchen_user_stations WHERE user_id=$1", adminID) != 0 {
			t.Fatal("una edición rechazada escribió estaciones")
		}
		// La identidad reservada de soporte no se administra desde aquí.
		var supportID string
		if err := env.pool.QueryRow(env.ctx, "SELECT id::text FROM app.users WHERE username='sa'").Scan(&supportID); err != nil {
			t.Fatal(err)
		}
		env.call(env.admin, "GET", "users/"+supportID+"/kitchen-stations", nil, 403)
	})
}
