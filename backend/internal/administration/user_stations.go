package administration

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

// «Estaciones que puede atender» de cada usuario (etapa 9F, decisión D6): todas por omisión o una
// selección. En Cocina se ve la intersección de las estaciones que muestra la terminal y las que atiende
// el usuario (paquete stationscope); quien tiene kitchen.stations.all siempre atiende todas y su selección
// guardada no tiene efecto.

// userStationsView es la configuración de un usuario con las estaciones para elegir.
type userStationsView struct {
	UserID      string   `json:"user_id"`
	AllStations bool     `json:"all_stations"`
	StationIDs  []string `json:"station_ids"`
	// AllByPermission: el usuario tiene kitchen.stations.all y atiende todas las estaciones aunque tenga
	// una selección guardada (la interfaz lo explica).
	AllByPermission bool               `json:"all_by_permission"`
	Stations        []userStationEntry `json:"stations"`
}

// userStationEntry es una estación que se puede elegir (también las inactivas, que se marcan).
type userStationEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// userStationsAudit es lo que se audita al cambiar la selección (sin nombres de estaciones: solo
// identificadores y la marca).
type userStationsAudit struct {
	AllStations bool     `json:"all_stations"`
	StationIDs  []string `json:"station_ids"`
}

func userKitchenStations(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string) (any, error) {
	if request.Method == "GET" {
		if err := principal.require("users.read"); err != nil {
			return nil, err
		}
		if err := editableUser(ctx, tx, userID); err != nil {
			return nil, err
		}
		return readUserStations(ctx, tx, userID)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para las estaciones del usuario.")
	}
	if err := principal.require("users.manage"); err != nil {
		return nil, err
	}
	var input struct {
		AllStations         bool     `json:"all_stations"`
		StationIDs          []string `json:"station_ids"`
		ExpectedAllStations bool     `json:"expected_all_stations"`
		ExpectedStationIDs  []string `json:"expected_station_ids"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateUserStations(input.AllStations, input.StationIDs); err != nil {
		return nil, err
	}
	// Serializa los cambios administrativos y bloquea al usuario, como al editarlo.
	if err := lockAdministration(ctx, tx); err != nil {
		return nil, err
	}
	if err := editableUser(ctx, tx, userID); err != nil {
		return nil, err
	}
	// Como con las excepciones de permisos, nadie cambia su propio alcance ni el de un usuario con
	// permisos que no tiene.
	if userID == principal.UserID {
		return nil, delegationDenied("No puedes cambiar tus propias estaciones de cocina.")
	}
	if err := requireManageableUser(ctx, tx, principal, userID); err != nil {
		return nil, err
	}
	before, err := userStationsSelection(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	// Revisión 9F (parte 3, B4): la ruta no lleva operation_id; si lo pedido ya es lo guardado (por ejemplo, el
	// reenvío de un guardado cuya respuesta se perdió), responde con lo vigente sin escribir, auditar ni
	// publicar, en lugar de un 409 «cambiaron en otra terminal» por su propio cambio.
	requested := append([]string{}, input.StationIDs...)
	slices.Sort(requested)
	if before.AllStations == input.AllStations && slices.Equal(before.StationIDs, requested) {
		return readUserStations(ctx, tx, userID)
	}
	expected := append([]string{}, input.ExpectedStationIDs...)
	slices.Sort(expected)
	if before.AllStations != input.ExpectedAllStations || !slices.Equal(before.StationIDs, expected) {
		return nil, &Fault{Status: 409, Code: "REVISION_CONFLICT",
			Message: "Las estaciones de este usuario cambiaron en otra terminal. Revisa la selección vigente y vuelve a guardar."}
	}
	var existing int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM app.kitchen_stations WHERE id::text = ANY($1::text[])", input.StationIDs).Scan(&existing); err != nil {
		return nil, err
	}
	if existing != len(input.StationIDs) {
		return nil, invalid("Elige estaciones existentes.")
	}
	after := userStationsAudit{AllStations: input.AllStations, StationIDs: append([]string{}, input.StationIDs...)}
	slices.Sort(after.StationIDs)
	if _, err := tx.Exec(ctx, "UPDATE app.users SET kitchen_all_stations=$1,updated_at=now() WHERE id=$2", after.AllStations, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app.kitchen_user_stations WHERE user_id=$1", userID); err != nil {
		return nil, err
	}
	for _, stationID := range after.StationIDs {
		if _, err := tx.Exec(ctx, "INSERT INTO app.kitchen_user_stations(user_id,station_id) VALUES ($1,$2)", userID, stationID); err != nil {
			return nil, err
		}
	}
	if err := audit(ctx, tx, principal, "user.kitchen_stations_changed", "user", &userID, before, after, true); err != nil {
		return nil, err
	}
	// Las pantallas de Cocina abiertas vuelven a consultar su alcance (sin contenido en el evento).
	if err := events.Append(ctx, tx, "kitchen.access.changed", "configuration", nil, nil); err != nil {
		return nil, err
	}
	return readUserStations(ctx, tx, userID)
}

// validateUserStations aplica las reglas de la selección: «todas» sin lista, o de 1 a 100 estaciones
// válidas y sin repetir.
func validateUserStations(allStations bool, stationIDs []string) error {
	if allStations && len(stationIDs) > 0 {
		return invalid("Con «Todas las estaciones» no se elige una selección.")
	}
	if !allStations && len(stationIDs) == 0 {
		return invalid("Elige al menos una estación o «Todas las estaciones».")
	}
	if len(stationIDs) > 100 {
		return invalid("Un usuario admite hasta 100 estaciones.")
	}
	seen := map[string]bool{}
	for _, id := range stationIDs {
		if !identifierPattern.MatchString(id) {
			return invalid("El identificador de la estación no es válido.")
		}
		if seen[id] {
			return invalid("La estación está repetida.")
		}
		seen[id] = true
	}
	return nil
}

// userStationsSelection lee la marca y la selección guardadas del usuario.
func userStationsSelection(ctx context.Context, tx pgx.Tx, userID string) (userStationsAudit, error) {
	selection := userStationsAudit{StationIDs: []string{}}
	if err := tx.QueryRow(ctx, "SELECT kitchen_all_stations FROM app.users WHERE id=$1", userID).Scan(&selection.AllStations); err != nil {
		return selection, err
	}
	rows, err := tx.Query(ctx, "SELECT station_id::text FROM app.kitchen_user_stations WHERE user_id=$1 ORDER BY station_id", userID)
	if err != nil {
		return selection, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return selection, err
	}
	selection.StationIDs = append(selection.StationIDs, ids...)
	return selection, nil
}

// readUserStations arma la vista del usuario: su selección, si atiende todas por permiso y las estaciones
// del establecimiento en el orden de Cocina.
func readUserStations(ctx context.Context, tx pgx.Tx, userID string) (*userStationsView, error) {
	selection, err := userStationsSelection(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	permissions, err := effectivePermissions(ctx, tx, userID, false)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'name',name,'active',active)
        ORDER BY sort_order,name,id),'[]') FROM app.kitchen_stations`).Scan(&raw); err != nil {
		return nil, err
	}
	stations := []userStationEntry{}
	if err := json.Unmarshal(raw, &stations); err != nil {
		return nil, err
	}
	return &userStationsView{
		UserID:          userID,
		AllStations:     selection.AllStations,
		StationIDs:      selection.StationIDs,
		AllByPermission: slices.Contains(permissions, "kitchen.stations.all"),
		Stations:        stations,
	}, nil
}
