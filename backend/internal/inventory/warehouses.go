package inventory

import (
	"time"
)

// Almacenes y asignación de estaciones de preparación (§5 de la especificación).
//   - «Almacén general» (sembrado en la 0010) es el predeterminado: no se desactiva y es el destino
//     de las estaciones sin asignación. No se crea ningún otro almacén automáticamente.
//   - Un almacén con existencias distintas de cero o asignado a una estación no se desactiva.
//   - La asignación estación → almacén es un historial de solo inserción; la vigente es la última.
//     Asignar null vuelve al almacén predeterminado. Su revisión es el número de asignaciones
//     registradas para la estación (0 si nunca se asignó).

type Warehouse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Active      bool               `json:"active"`
	IsDefault   bool               `json:"is_default"`
	Stations    []StationReference `json:"stations"`
	Revision    int                `json:"revision"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// StationReference es una estación asignada a un almacén.
type StationReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// currentAssignments es la asignación vigente de cada estación (la de effective_from más reciente).
const currentAssignments = `SELECT DISTINCT ON (a.station_id) a.station_id,a.warehouse_id,a.effective_from,a.actor_id
        FROM app.inventory_station_warehouses a
        ORDER BY a.station_id, a.effective_from DESC, a.seq DESC`

const warehouseColumns = `SELECT w.id,w.name,w.description,w.active,w.is_default,w.revision,w.updated_at
        FROM app.inventory_warehouses w`

func scanWarehouse(row interface{ Scan(...any) error }) (*Warehouse, error) {
	var warehouse Warehouse
	err := row.Scan(&warehouse.ID, &warehouse.Name, &warehouse.Description, &warehouse.Active, &warehouse.IsDefault,
		&warehouse.Revision, &warehouse.UpdatedAt)
	if err != nil {
		return nil, err
	}
	warehouse.Stations = []StationReference{}
	return &warehouse, nil
}

// attachStations agrega a cada almacén las estaciones que lo tienen asignado hoy. Las estaciones
// sin asignación (o con null) consumen del predeterminado.
func (handler *service) attachStations(warehouses []*Warehouse) error {
	byID := map[string]*Warehouse{}
	var fallback *Warehouse
	for _, warehouse := range warehouses {
		byID[warehouse.ID] = warehouse
		if warehouse.IsDefault {
			fallback = warehouse
		}
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT s.id,s.name,current.warehouse_id
        FROM app.kitchen_stations s LEFT JOIN (`+currentAssignments+`) current ON current.station_id=s.id
        WHERE s.active ORDER BY s.name, s.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var station StationReference
		var warehouseID *string
		if err := rows.Scan(&station.ID, &station.Name, &warehouseID); err != nil {
			return err
		}
		target := fallback
		if warehouseID != nil {
			target = byID[*warehouseID]
		}
		if target != nil {
			target.Stations = append(target.Stations, station)
		}
	}
	return rows.Err()
}

func (handler *service) readWarehouse(id string) (*Warehouse, error) {
	warehouse, err := scanWarehouse(handler.tx.QueryRow(handler.ctx, warehouseColumns+" WHERE w.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El almacén no existe.")
	}
	return warehouse, err
}

// defaultWarehouse devuelve el almacén predeterminado («Almacén general»).
func (handler *service) defaultWarehouse() (*Warehouse, error) {
	warehouse, err := scanWarehouse(handler.tx.QueryRow(handler.ctx, warehouseColumns+" WHERE w.is_default"))
	if isNoRows(err) {
		return nil, conflict("DEFAULT_WAREHOUSE_MISSING", "No existe el almacén predeterminado.")
	}
	return warehouse, err
}

// listWarehouses devuelve los almacenes (el predeterminado primero) con sus estaciones asignadas.
func (handler *service) listWarehouses(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, warehouseColumns+" ORDER BY w.is_default DESC, w.active DESC, w.name, w.id")
	if err != nil {
		return nil, err
	}
	warehouses := []*Warehouse{}
	for rows.Next() {
		warehouse, err := scanWarehouse(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		warehouses = append(warehouses, warehouse)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := handler.attachStations(warehouses); err != nil {
		return nil, err
	}
	return warehouses, nil
}

type warehouseInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

func (input *warehouseInput) validate() error {
	if err := validateName(&input.Name, "El nombre del almacén", 60); err != nil {
		return err
	}
	return validateDescription(&input.Description, 500)
}

func warehouseConflict(err error) error {
	if uniqueViolation(err) {
		return conflict("DUPLICATE_NAME", "Ya existe un almacén con ese nombre.")
	}
	return err
}

// savedWarehouse relee el almacén con sus estaciones para la respuesta.
func (handler *service) savedWarehouse(id string) (*Warehouse, error) {
	all, err := handler.listWarehouses(nil)
	if err != nil {
		return nil, err
	}
	for _, warehouse := range all.([]*Warehouse) {
		if warehouse.ID == id {
			return warehouse, nil
		}
	}
	return nil, notFound("El almacén no existe.")
}

// createWarehouse agrega un almacén (nunca predeterminado: el predeterminado es el sembrado).
func (handler *service) createWarehouse(_ []string) (any, error) {
	var input warehouseInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_warehouses(id,name,description,active)
        VALUES ($1,$2,$3,$4)`, id, input.Name, input.Description, input.Active)
	if err != nil {
		return nil, warehouseConflict(err)
	}
	saved, err := handler.savedWarehouse(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.settings.changed", &id); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.warehouse_created", "inventory_warehouse", &id, nil, saved)
}

// updateWarehouse renombra, describe, activa o desactiva un almacén con revisión optimista.
func (handler *service) updateWarehouse(parts []string) (any, error) {
	var input warehouseInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := handler.savedWarehouse(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("El almacén")
	}
	if previous.Active && !input.Active {
		if err := handler.checkDeactivation(previous); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_warehouses SET name=$2,description=$3,active=$4,
        revision=revision+1,updated_at=now() WHERE id=$1`, previous.ID, input.Name, input.Description, input.Active)
	if err != nil {
		return nil, warehouseConflict(err)
	}
	saved, err := handler.savedWarehouse(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.settings.changed", &saved.ID); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.warehouse_updated", "inventory_warehouse", &saved.ID, previous, saved)
}

// checkDeactivation impide desactivar el predeterminado, un almacén con existencias o uno asignado a
// una estación (activa o no).
func (handler *service) checkDeactivation(warehouse *Warehouse) error {
	if warehouse.IsDefault {
		return conflict("DEFAULT_WAREHOUSE", "El almacén predeterminado no se puede desactivar.")
	}
	var stocked bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.inventory_stock WHERE warehouse_id=$1 AND quantity<>0)",
		warehouse.ID).Scan(&stocked)
	if err != nil {
		return err
	}
	if stocked {
		return conflict("WAREHOUSE_HAS_STOCK", "El almacén tiene existencias: transfiérelas o ajústalas antes de desactivarlo.")
	}
	// Cuenta también las estaciones inactivas: al reactivarse volverían a consumir de este almacén.
	var assigned bool
	err = handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM ("+currentAssignments+") current WHERE current.warehouse_id=$1)",
		warehouse.ID).Scan(&assigned)
	if err != nil {
		return err
	}
	if assigned {
		return conflict("WAREHOUSE_ASSIGNED",
			"El almacén está asignado a estaciones de preparación: cambia su asignación antes de desactivarlo.")
	}
	return nil
}

// StationWarehouse es la asignación vigente de una estación con su historial reciente.
type StationWarehouse struct {
	StationID      string              `json:"station_id"`
	StationName    string              `json:"station_name"`
	StationActive  bool                `json:"station_active"`
	WarehouseID    *string             `json:"warehouse_id"`
	WarehouseName  string              `json:"warehouse_name"`
	UsesDefault    bool                `json:"uses_default"`
	EffectiveFrom  *time.Time          `json:"effective_from"`
	AssignedByName *string             `json:"assigned_by_name"`
	Revision       int                 `json:"revision"`
	History        []StationAssignment `json:"history"`
}

// StationAssignment es una fila del historial de asignaciones (la más reciente primero).
type StationAssignment struct {
	WarehouseID   *string   `json:"warehouse_id"`
	WarehouseName string    `json:"warehouse_name"`
	EffectiveFrom time.Time `json:"effective_from"`
	ActorName     *string   `json:"actor_name"`
}

// stationHistoryLimit acota el historial que acompaña a cada estación en la respuesta.
const stationHistoryLimit = 20

// stationWarehouses devuelve las estaciones (todas o una) con su almacén de consumo vigente.
func (handler *service) stationWarehouses(stationID *string) ([]*StationWarehouse, error) {
	fallback, err := handler.defaultWarehouse()
	if err != nil {
		return nil, err
	}
	statement := `SELECT s.id,s.name,s.active FROM app.kitchen_stations s`
	arguments := []any{}
	if stationID != nil {
		statement += " WHERE s.id=$1"
		arguments = append(arguments, *stationID)
	}
	rows, err := handler.tx.Query(handler.ctx, statement+" ORDER BY s.active DESC, s.sort_order, s.name, s.id", arguments...)
	if err != nil {
		return nil, err
	}
	stations := []*StationWarehouse{}
	byID := map[string]*StationWarehouse{}
	for rows.Next() {
		station := &StationWarehouse{History: []StationAssignment{}, WarehouseName: fallback.Name, UsesDefault: true}
		if err := rows.Scan(&station.StationID, &station.StationName, &station.StationActive); err != nil {
			rows.Close()
			return nil, err
		}
		stations = append(stations, station)
		byID[station.StationID] = station
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	history, err := handler.tx.Query(handler.ctx, `SELECT a.station_id,a.warehouse_id,w.name,a.effective_from,u.display_name,
            count(*) OVER (PARTITION BY a.station_id),
            row_number() OVER (PARTITION BY a.station_id ORDER BY a.effective_from DESC, a.seq DESC)
        FROM app.inventory_station_warehouses a
        LEFT JOIN app.inventory_warehouses w ON w.id=a.warehouse_id
        LEFT JOIN app.users u ON u.id=a.actor_id
        ORDER BY a.station_id, a.effective_from DESC, a.seq DESC`)
	if err != nil {
		return nil, err
	}
	defer history.Close()
	for history.Next() {
		var assignment StationAssignment
		var station string
		var warehouseName *string
		var total, position int
		err := history.Scan(&station, &assignment.WarehouseID, &warehouseName, &assignment.EffectiveFrom, &assignment.ActorName,
			&total, &position)
		if err != nil {
			return nil, err
		}
		target := byID[station]
		if target == nil {
			continue
		}
		assignment.WarehouseName = fallback.Name
		if warehouseName != nil {
			assignment.WarehouseName = *warehouseName
		}
		if position == 1 {
			target.Revision = total
			target.WarehouseID = assignment.WarehouseID
			target.WarehouseName = assignment.WarehouseName
			target.UsesDefault = assignment.WarehouseID == nil
			effective := assignment.EffectiveFrom
			target.EffectiveFrom = &effective
			target.AssignedByName = assignment.ActorName
		}
		if position <= stationHistoryLimit {
			target.History = append(target.History, assignment)
		}
	}
	return stations, history.Err()
}

func (handler *service) listStationWarehouses(_ []string) (any, error) {
	return handler.stationWarehouses(nil)
}

type stationWarehouseInput struct {
	OperationID string  `json:"operation_id"`
	Revision    int     `json:"revision"`
	WarehouseID *string `json:"warehouse_id"`
}

// assignStationWarehouse registra la asignación de la estación de la ruta. Asignar el mismo
// almacén vigente no agrega historial.
func (handler *service) assignStationWarehouse(parts []string) (any, error) {
	var input stationWarehouseInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	warehouseID, err := validateOptionalID(input.WarehouseID)
	if err != nil {
		return nil, err
	}
	current, err := handler.stationWarehouses(&parts[1])
	if err != nil {
		return nil, err
	}
	if len(current) == 0 {
		return nil, notFound("La estación de preparación no existe.")
	}
	previous := current[0]
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La asignación de la estación")
	}
	if warehouseID != nil {
		warehouse, err := handler.readWarehouse(*warehouseID)
		if err != nil {
			return nil, err
		}
		if !warehouse.Active {
			return nil, conflict("WAREHOUSE_INACTIVE", "El almacén «"+warehouse.Name+"» está desactivado.")
		}
	}
	same := (warehouseID == nil && previous.WarehouseID == nil) ||
		(warehouseID != nil && previous.WarehouseID != nil && *warehouseID == *previous.WarehouseID)
	if same {
		return previous, nil
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_station_warehouses(station_id,warehouse_id,actor_id,operation_id)
        VALUES ($1,$2,$3,$4)`, previous.StationID, warehouseID, handler.actorReference(), handler.operationReference())
	if err != nil {
		return nil, err
	}
	updated, err := handler.stationWarehouses(&parts[1])
	if err != nil {
		return nil, err
	}
	saved := updated[0]
	if err := handler.publish("inventory.settings.changed", &saved.StationID); err != nil {
		return nil, err
	}
	before := map[string]any{"warehouse_id": previous.WarehouseID, "warehouse_name": previous.WarehouseName}
	after := map[string]any{"warehouse_id": saved.WarehouseID, "warehouse_name": saved.WarehouseName, "effective_from": saved.EffectiveFrom}
	err = handler.audit(handler.ctx, handler.tx, "inventory.station_warehouse_assigned", "kitchen_station", &saved.StationID, before, after)
	return saved, err
}
