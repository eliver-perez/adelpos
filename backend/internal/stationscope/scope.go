// Package stationscope decide qué estaciones de cocina se ven en una terminal (etapa 9F, decisión D6 del
// coordinador): las estaciones que muestra la terminal (todas o una selección) intersectadas con las que
// puede atender el usuario (todas, una selección o, con kitchen.stations.all, siempre todas).
//
// Es un paquete neutral, sin dependencias de los módulos de negocio, para que lo usen cocina, el canal de
// eventos y la administración con la misma regla. Solo lee; nunca escribe.
package stationscope

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Scope son las estaciones visibles para una persona en una terminal.
type Scope struct {
	// All indica que no se filtra: la terminal muestra todas y la persona atiende todas (también las
	// estaciones que se creen después).
	All bool
	// visible son las estaciones de la intersección cuando All es falso.
	visible map[string]bool
	// summary explica de dónde sale la intersección (para el aviso sin estaciones en común).
	summary Summary
}

// Summary describe la intersección para la interfaz: si hay estaciones en común y, para el aviso, qué
// estaciones muestra la terminal y cuáles atiende el usuario (por nombre, en el orden de Cocina).
type Summary struct {
	Common              bool     `json:"common"`
	All                 bool     `json:"all"`
	TerminalAll         bool     `json:"terminal_all"`
	TerminalStations    []string `json:"terminal_stations"`
	UserAll             bool     `json:"user_all"`
	UserAllByPermission bool     `json:"user_all_by_permission"`
	UserStations        []string `json:"user_stations"`
}

// Station es una estación con su pertenencia a la selección de la terminal y a la del usuario.
type Station struct {
	ID         string
	Name       string
	InTerminal bool
	InUser     bool
}

// Everything es el alcance sin filtro (por ejemplo, las consultas administrativas de quien tiene
// kitchen.stations.all).
func Everything() *Scope {
	return &Scope{
		All:     true,
		visible: map[string]bool{},
		summary: Summary{
			Common:           true,
			All:              true,
			TerminalAll:      true,
			TerminalStations: []string{},
			UserAll:          true,
			UserStations:     []string{},
		},
	}
}

// Load lee la marca y la selección de la terminal y del usuario y calcula la intersección. Una terminal o
// un usuario inexistentes no muestran ninguna estación (nunca se amplía el alcance por falta de datos).
func Load(ctx context.Context, tx pgx.Tx, deviceID, userID string, allByPermission bool) (*Scope, error) {
	var terminalAll, userAll bool
	err := tx.QueryRow(ctx, `SELECT
            coalesce((SELECT kitchen_all_stations FROM app.devices WHERE id::text = $1), false),
            coalesce((SELECT kitchen_all_stations FROM app.users WHERE id::text = $2), false)`,
		deviceID, userID).Scan(&terminalAll, &userAll)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id::text, s.name,
            EXISTS(SELECT FROM app.kitchen_device_stations d WHERE d.device_id::text = $1 AND d.station_id = s.id),
            EXISTS(SELECT FROM app.kitchen_user_stations u WHERE u.user_id::text = $2 AND u.station_id = s.id)
        FROM app.kitchen_stations s
        ORDER BY s.sort_order, s.name, s.id`, deviceID, userID)
	if err != nil {
		return nil, err
	}
	stations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Station, error) {
		var station Station
		err := row.Scan(&station.ID, &station.Name, &station.InTerminal, &station.InUser)
		return station, err
	})
	if err != nil {
		return nil, err
	}
	return Compute(terminalAll, userAll, allByPermission, stations), nil
}

// Compute calcula la intersección a partir de las marcas y de las estaciones existentes. Con el permiso
// kitchen.stations.all el usuario atiende todas aunque tenga una selección guardada.
func Compute(terminalAll, userAll, allByPermission bool, stations []Station) *Scope {
	userAll = userAll || allByPermission
	scope := &Scope{
		All:     terminalAll && userAll,
		visible: map[string]bool{},
		summary: Summary{
			TerminalAll:         terminalAll,
			TerminalStations:    []string{},
			UserAll:             userAll,
			UserAllByPermission: allByPermission,
			UserStations:        []string{},
		},
	}
	for _, station := range stations {
		if !terminalAll && station.InTerminal {
			scope.summary.TerminalStations = append(scope.summary.TerminalStations, station.Name)
		}
		if !userAll && station.InUser {
			scope.summary.UserStations = append(scope.summary.UserStations, station.Name)
		}
		shownByTerminal := terminalAll || station.InTerminal
		attendedByUser := userAll || station.InUser
		if shownByTerminal && attendedByUser {
			scope.visible[station.ID] = true
		}
	}
	scope.summary.All = scope.All
	scope.summary.Common = scope.All || len(scope.visible) > 0
	return scope
}

// Allows indica si la estación es visible.
func (scope *Scope) Allows(stationID string) bool {
	return scope.All || scope.visible[stationID]
}

// Common indica si hay al menos una estación visible (o todas).
func (scope *Scope) Common() bool {
	return scope.summary.Common
}

// IDs devuelve las estaciones visibles en orden estable (vacío si All: no se filtra).
func (scope *Scope) IDs() []string {
	if scope.All {
		return []string{}
	}
	ids := make([]string, 0, len(scope.visible))
	for id := range scope.visible {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Summary devuelve la explicación de la intersección para la interfaz.
func (scope *Scope) Summary() Summary {
	return scope.summary
}
