package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"local/ad-el-pos/server/internal/events"
)

// Trabajador de consumo de inventario (§0-B «Consumo asíncrono» y §6 del diseño). Arranca con el
// servidor como los trabajadores de cocina (contexto cancelable, apagado ordenado) y sondea cada
// segundo, sin LISTEN/NOTIFY:
//   - ProcessPending procesa solicitudes de consumo: cada una en su propia transacción con el lock
//     7820148308 exclusivo, en orden de seq por unidad comercial (no se procesa una solicitud si su
//     unidad tiene otra anterior sin procesar), tomándola con FOR UPDATE SKIP LOCKED. Es idempotente
//     por estado (processed) y por entry_key del libro: reprocesar nunca duplica movimientos. Un
//     fallo revierte esa solicitud, se registra con un mensaje genérico y espera creciente, y no
//     detiene las demás unidades.
//   - ReconcileCancellations aplica «No preparado» automático a las cancelaciones pendientes cuyo
//     corte ya pasó (UPDATE … WHERE state='pending', idempotente).
// Al iniciar procesa todo lo pendiente (también los días que Go estuvo apagado).

// workerProcess identifica al trabajador como responsable de movimientos y auditoría.
const workerProcess = "inventory_worker"

// failureMessage es el único mensaje de error que se guarda en una solicitud: sin detalles
// internos ni costos.
const failureMessage = "No se pudo calcular el consumo; se reintentará automáticamente."

// batchLimit acota cada vuelta del trabajador para no monopolizar el lock de inventario.
const batchLimit = 50

// Worker procesa solicitudes de consumo y reconcilia cancelaciones vencidas.
type Worker struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	// Now permite simular la hora en pruebas; nil usa time.Now.
	Now func() time.Time
}

// Run sondea hasta que se cancela el contexto. Al iniciar procesa todo lo pendiente.
func (worker *Worker) Run(ctx context.Context) {
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			worker.tick(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	wait.Wait()
}

// tick procesa lo pendiente en lotes hasta vaciarlo y luego reconcilia las cancelaciones vencidas.
func (worker *Worker) tick(ctx context.Context) {
	for ctx.Err() == nil {
		processed, err := ProcessPending(ctx, worker.Pool, batchLimit)
		if err != nil {
			if ctx.Err() == nil && worker.Logger != nil {
				worker.Logger.Warn("inventory_consumption_unavailable")
			}
			return
		}
		if processed < batchLimit {
			break
		}
	}
	now := time.Now()
	if worker.Now != nil {
		now = worker.Now()
	}
	if _, err := ReconcileCancellations(ctx, worker.Pool, now); err != nil && ctx.Err() == nil && worker.Logger != nil {
		worker.Logger.Warn("inventory_reconciliation_unavailable")
	}
}

// consumptionRequest es una solicitud de consumo tomada para procesar.
type consumptionRequest struct {
	ID          string
	Kind        string
	UnitID      string
	SourceID    string
	VersionID   string
	ReleasedAt  time.Time
	OccurredAt  time.Time
	ServiceMode string
	State       string
	Attempts    int
}

// ProcessPending procesa hasta limit solicitudes pendientes y devuelve cuántas tomó (procesadas o
// fallidas). Solo devuelve error si no puede consultar la base; el fallo de una solicitud queda
// registrado en ella.
func ProcessPending(ctx context.Context, pool *pgxpool.Pool, limit int) (int, error) {
	taken := 0
	for taken < limit {
		claimed, err := processNext(ctx, pool)
		if err != nil {
			return taken, err
		}
		if !claimed {
			break
		}
		taken++
	}
	return taken, nil
}

// errNothingPending indica que no hay solicitudes disponibles.
var errNothingPending = errors.New("sin solicitudes de consumo pendientes")

// processNext toma y procesa una solicitud. Devuelve false si no había ninguna disponible.
func processNext(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	if err := ensureLedgerLock(ctx, tx); err != nil {
		return false, err
	}
	request, err := claimRequest(ctx, tx)
	if errors.Is(err, errNothingPending) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := processRequest(ctx, tx, request); err != nil {
		_ = tx.Rollback(context.Background())
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, markFailed(ctx, pool, request)
	}
	if err := tx.Commit(ctx); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, markFailed(ctx, pool, request)
	}
	return true, nil
}

// claimRequest bloquea la siguiente solicitud procesable: la de menor seq cuya unidad no tiene
// otra anterior sin procesar y cuya espera ya terminó.
func claimRequest(ctx context.Context, tx pgx.Tx) (*consumptionRequest, error) {
	var request consumptionRequest
	err := tx.QueryRow(ctx, `SELECT r.id,r.kind,r.restaurant_unit_id,r.source_id,r.version_id,r.released_at,r.occurred_at,
            r.service_mode,r.state,r.attempts
        FROM app.inventory_consumption_requests r
        WHERE r.state<>'processed' AND r.next_attempt_at<=clock_timestamp()
            AND NOT EXISTS(SELECT FROM app.inventory_consumption_requests p
                WHERE p.restaurant_unit_id=r.restaurant_unit_id AND p.state<>'processed' AND p.seq<r.seq)
        ORDER BY r.seq LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&request.ID, &request.Kind, &request.UnitID, &request.SourceID,
		&request.VersionID, &request.ReleasedAt, &request.OccurredAt, &request.ServiceMode, &request.State, &request.Attempts)
	if isNoRows(err) {
		return nil, errNothingPending
	}
	if err != nil {
		return nil, err
	}
	return &request, nil
}

// markFailed registra el fallo con espera creciente (2, 4, 8 … hasta 300 segundos) en una
// transacción propia. La primera vez publica inventory.alerts.changed (solicitudes fallidas).
func markFailed(ctx context.Context, pool *pgxpool.Pool, request *consumptionRequest) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	delay := 2 << min(request.Attempts, 7)
	if delay > 300 {
		delay = 300
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_consumption_requests SET state='failed',attempts=attempts+1,last_error=$2,
            next_attempt_at=clock_timestamp()+make_interval(secs=>$3)
        WHERE id=$1 AND state<>'processed'`, request.ID, failureMessage, delay)
	if err != nil {
		return err
	}
	if request.State != "failed" {
		if err := events.Append(ctx, tx, "inventory.alerts.changed", Scope, &request.ID, nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// processRequest aplica una solicitud según su tipo y la marca procesada con su diagnóstico.
func processRequest(ctx context.Context, tx pgx.Tx, request *consumptionRequest) error {
	var outcome requestOutcome
	var err error
	switch request.Kind {
	case "release":
		outcome, err = processRelease(ctx, tx, request)
	case "amend":
		outcome, err = processAmend(ctx, tx, request)
	case "cancel":
		outcome, err = processCancel(ctx, tx, request)
	default:
		err = errors.New("inventory: tipo de solicitud desconocido")
	}
	if err != nil {
		return err
	}
	diagnostics := outcome.diagnostics
	if diagnostics == nil {
		diagnostics = []Diagnostic{}
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_consumption_requests SET state='processed',processed_at=clock_timestamp(),
            last_error=NULL,diagnostics=$2 WHERE id=$1`, request.ID, jsonValue(diagnostics))
	if err != nil {
		return err
	}
	if outcome.movements > 0 {
		if err := events.Append(ctx, tx, "inventory.stock.changed", Scope, &request.UnitID, nil); err != nil {
			return err
		}
		if err := events.Append(ctx, tx, "inventory.movements.recorded", Scope, &request.ID, nil); err != nil {
			return err
		}
	}
	if outcome.alerts || request.State == "failed" {
		return events.Append(ctx, tx, "inventory.alerts.changed", Scope, &request.UnitID, nil)
	}
	return nil
}

// requestOutcome resume lo que produjo una solicitud.
type requestOutcome struct {
	movements   int
	diagnostics []Diagnostic
	alerts      bool
}

// unitConsumption es el estado de consumo de una unidad.
type unitConsumption struct {
	ReleasedAt  time.Time
	ServiceMode string
	State       string
	VersionID   string
}

// lockUnitConsumption lee (con FOR UPDATE) el estado de consumo de una unidad; nil si no existe.
func lockUnitConsumption(ctx context.Context, tx pgx.Tx, unitID string) (*unitConsumption, error) {
	var state unitConsumption
	err := tx.QueryRow(ctx, `SELECT released_at,service_mode,state,last_version_id FROM app.inventory_unit_consumptions
        WHERE restaurant_unit_id=$1 FOR UPDATE`, unitID).Scan(&state.ReleasedAt, &state.ServiceMode, &state.State, &state.VersionID)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// processRelease registra el consumo de una unidad liberada: resuelve la versión liberada a la
// fecha de la liberación y escribe un movimiento consumption por (insumo, almacén).
func processRelease(ctx context.Context, tx pgx.Tx, request *consumptionRequest) (requestOutcome, error) {
	existing, err := lockUnitConsumption(ctx, tx, request.UnitID)
	if err != nil {
		return requestOutcome{}, err
	}
	if existing != nil {
		return requestOutcome{diagnostics: []Diagnostic{{Code: diagnosticUnitAlreadyRegistered, Path: "root"}}}, nil
	}
	resolved, err := resolveRequest(ctx, tx, request.VersionID, request.UnitID, request.ReleasedAt, request.ServiceMode)
	if err != nil {
		return requestOutcome{}, err
	}
	movements, err := adjustUnitConsumption(ctx, tx, consumptionTarget{
		unitID: request.UnitID, entryPrefix: "consumption:" + request.ID, sourceKind: "consumption", sourceID: request.ID,
		occurredAt: request.ReleasedAt, reason: "Liberación a cocina", target: resolved.entries(), stations: resolved.Stations,
		process: workerProcess,
	})
	if err != nil {
		return requestOutcome{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.inventory_unit_consumptions(restaurant_unit_id,release_request_id,last_request_id,
            last_version_id,released_at,service_mode)
        VALUES ($1,$2,$2,$3,$4,$5)`, request.UnitID, request.ID, request.VersionID, request.ReleasedAt, request.ServiceMode)
	if err != nil {
		return requestOutcome{}, err
	}
	return requestOutcome{movements: movements, diagnostics: resolved.Diagnostics}, nil
}

// processAmend registra solo la diferencia entre el consumo de la versión nueva (resuelta a la
// fecha de la liberación de la unidad) y el consumo neto ya registrado de la unidad.
func processAmend(ctx context.Context, tx pgx.Tx, request *consumptionRequest) (requestOutcome, error) {
	state, err := lockUnitConsumption(ctx, tx, request.UnitID)
	if err != nil {
		return requestOutcome{}, err
	}
	if state == nil {
		return requestOutcome{diagnostics: []Diagnostic{{Code: diagnosticNotConsumed, Path: "root"}}}, nil
	}
	if state.State == "reversed" {
		return requestOutcome{diagnostics: []Diagnostic{{Code: diagnosticAlreadyReversed, Path: "root"}}}, nil
	}
	resolved, err := resolveRequest(ctx, tx, request.VersionID, request.UnitID, state.ReleasedAt, state.ServiceMode)
	if err != nil {
		return requestOutcome{}, err
	}
	movements, err := adjustUnitConsumption(ctx, tx, consumptionTarget{
		unitID: request.UnitID, entryPrefix: "amend:" + request.ID, sourceKind: "amendment", sourceID: request.ID,
		occurredAt: request.OccurredAt, reason: "Modificación de partida liberada", target: resolved.entries(),
		stations: resolved.Stations, process: workerProcess,
	})
	if err != nil {
		return requestOutcome{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_unit_consumptions SET last_version_id=$2,last_request_id=$3,updated_at=clock_timestamp()
        WHERE restaurant_unit_id=$1`, request.UnitID, request.VersionID, request.ID)
	if err != nil {
		return requestOutcome{}, err
	}
	return requestOutcome{movements: movements, diagnostics: resolved.Diagnostics}, nil
}

// processCancel crea la clasificación pendiente de una unidad consumida y cancelada, con el día
// operativo de la cancelación. Sin consumo neto no hay nada que clasificar.
func processCancel(ctx context.Context, tx pgx.Tx, request *consumptionRequest) (requestOutcome, error) {
	state, err := lockUnitConsumption(ctx, tx, request.UnitID)
	if err != nil {
		return requestOutcome{}, err
	}
	if state == nil {
		return requestOutcome{diagnostics: []Diagnostic{{Code: diagnosticNotConsumed, Path: "root"}}}, nil
	}
	net, err := unitNet(ctx, tx, request.UnitID, nil)
	if err != nil {
		return requestOutcome{}, err
	}
	consumed := false
	for _, entry := range net {
		if entry.quantity.IsPositive() {
			consumed = true
		}
	}
	if !consumed || state.State == "reversed" {
		return requestOutcome{diagnostics: []Diagnostic{{Code: diagnosticNothingConsumed, Path: "root"}}}, nil
	}
	day, err := operatingDayFor(ctx, tx, request.OccurredAt)
	if err != nil {
		return requestOutcome{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.inventory_cancellation_classifications(restaurant_unit_id,request_id,cancelled_at,
            operating_date,deadline_at)
        VALUES ($1,$2,$3,$4,$5) ON CONFLICT ON CONSTRAINT inventory_cancellation_classifications_unit_key DO NOTHING`,
		request.UnitID, request.ID, request.OccurredAt, day.Date, day.Deadline)
	if err != nil {
		return requestOutcome{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_unit_consumptions SET last_request_id=$2,updated_at=clock_timestamp()
        WHERE restaurant_unit_id=$1`, request.UnitID, request.ID)
	return requestOutcome{alerts: true}, err
}

// operatingDayFor calcula el día operativo de un instante con la zona horaria y la hora de corte
// vigentes.
func operatingDayFor(ctx context.Context, tx pgx.Tx, instant time.Time) (OperatingDay, error) {
	var timezone string
	var hour, minute int
	err := tx.QueryRow(ctx, `SELECT e.timezone,extract(hour FROM s.operating_day_cutoff)::int,extract(minute FROM s.operating_day_cutoff)::int
        FROM app.inventory_settings s CROSS JOIN app.establishment e`).Scan(&timezone, &hour, &minute)
	if isNoRows(err) {
		timezone, hour, minute = "UTC", 5, 0
	} else if err != nil {
		return OperatingDay{}, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	return operatingDayOf(instant, location, hour, minute), nil
}

// resolveRequest carga la versión, su catálogo congelado, el enrutamiento de cocina de la unidad
// y la configuración vigente a releasedAt, y resuelve el consumo.
func resolveRequest(ctx context.Context, tx pgx.Tx, versionID, unitID string, releasedAt time.Time, mode string) (*resolution, error) {
	var quoteLine, snapshotJSON []byte
	err := tx.QueryRow(ctx, `SELECT v.quote_line,o.pricing_snapshot FROM app.restaurant_item_versions v
        JOIN app.restaurant_orders o ON o.id=v.order_id WHERE v.id=$1`, versionID).Scan(&quoteLine, &snapshotJSON)
	if err != nil {
		return nil, err
	}
	breakdown, err := decodeBreakdown(quoteLine)
	if err != nil {
		return nil, err
	}
	snapshot, err := decodeSnapshot(snapshotJSON)
	if err != nil {
		return nil, err
	}
	routing, err := loadUnitRouting(ctx, tx, unitID, releasedAt)
	if err != nil {
		return nil, err
	}
	return resolveUnit(breakdown, snapshot, mode, newRecipeStore(ctx, tx, releasedAt), routing)
}

// loadUnitRouting lee las estaciones que recibieron cada parte de la unidad (kitchen_tasks, también
// las canceladas: conservan sus instrucciones) y la asignación estación → almacén a la fecha.
func loadUnitRouting(ctx context.Context, tx pgx.Tx, unitID string, at time.Time) (unitRouting, error) {
	routing := unitRouting{StationsByPath: map[string][]stationRef{}, Assignments: map[string]string{}}
	if err := tx.QueryRow(ctx, "SELECT id FROM app.inventory_warehouses WHERE is_default").Scan(&routing.DefaultWarehouse); err != nil {
		return routing, err
	}
	rows, err := tx.Query(ctx, `SELECT t.station_id,s.name,coalesce(t.instructions->'parts','[]'::jsonb)
        FROM app.kitchen_tasks t JOIN app.kitchen_stations s ON s.id=t.station_id WHERE t.unit_id=$1`, unitID)
	if err != nil {
		return routing, err
	}
	stations := []string{}
	for rows.Next() {
		var station stationRef
		var encoded []byte
		if err := rows.Scan(&station.ID, &station.Name, &encoded); err != nil {
			rows.Close()
			return routing, err
		}
		var parts []struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(encoded, &parts); err != nil {
			rows.Close()
			return routing, err
		}
		for _, part := range parts {
			routing.StationsByPath[part.Path] = append(routing.StationsByPath[part.Path], station)
		}
		stations = append(stations, station.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return routing, err
	}
	if len(stations) == 0 {
		return routing, nil
	}
	assignments, err := tx.Query(ctx, `SELECT DISTINCT ON (a.station_id) a.station_id,a.warehouse_id
        FROM app.inventory_station_warehouses a WHERE a.station_id=ANY($1::uuid[]) AND a.effective_from<=$2
        ORDER BY a.station_id, a.effective_from DESC, a.seq DESC`, stations, at)
	if err != nil {
		return routing, err
	}
	defer assignments.Close()
	for assignments.Next() {
		var station string
		var warehouse *string
		if err := assignments.Scan(&station, &warehouse); err != nil {
			return routing, err
		}
		if warehouse != nil {
			routing.Assignments[station] = *warehouse
		}
	}
	return routing, assignments.Err()
}

// netEntry es el consumo neto registrado de una unidad en un (insumo, almacén): cantidad y valor
// positivos (consumos menos reversiones).
type netEntry struct {
	quantity decimal.Decimal
	value    decimal.Decimal
}

// unitNet suma los consumos y reversiones de una unidad por (insumo, almacén). sourceKinds limita
// los orígenes (nil = todos).
func unitNet(ctx context.Context, tx pgx.Tx, unitID string, sourceKinds []string) (map[consumptionKey]netEntry, error) {
	statement := `SELECT item_id,warehouse_id,(-sum(quantity))::text,(-sum(value))::text FROM app.inventory_movements
        WHERE restaurant_unit_id=$1 AND kind IN ('consumption','consumption_reversal')`
	arguments := []any{unitID}
	if sourceKinds != nil {
		statement += " AND source_kind=ANY($2::text[])"
		arguments = append(arguments, sourceKinds)
	}
	rows, err := tx.Query(ctx, statement+" GROUP BY item_id,warehouse_id", arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	net := map[consumptionKey]netEntry{}
	for rows.Next() {
		var key consumptionKey
		var quantity, value string
		if err := rows.Scan(&key.ItemID, &key.WarehouseID, &quantity, &value); err != nil {
			return nil, err
		}
		net[key] = netEntry{quantity: parseDatabaseDecimal(quantity), value: parseDatabaseDecimal(value)}
	}
	return net, rows.Err()
}

// consumptionTarget describe cómo llevar el consumo neto de una unidad a un objetivo.
type consumptionTarget struct {
	unitID      string
	entryPrefix string
	sourceKind  string
	sourceID    string
	occurredAt  time.Time
	reason      string
	target      []resolvedEntry
	stations    map[string][]string
	actorID     *string
	process     string
	operationID *string
	// net es el consumo neto de partida; nil lo lee del libro.
	net map[consumptionKey]netEntry
}

// adjustUnitConsumption escribe la diferencia entre el objetivo y el consumo neto registrado de la
// unidad por (insumo, almacén): consumption si falta consumir y consumption_reversal (al valor
// proporcional registrado, exacto si revierte todo) si sobra. Devuelve cuántos movimientos
// escribió. Idempotente por entry_key.
func adjustUnitConsumption(ctx context.Context, tx pgx.Tx, plan consumptionTarget) (int, error) {
	net := plan.net
	if net == nil {
		var err error
		net, err = unitNet(ctx, tx, plan.unitID, nil)
		if err != nil {
			return 0, err
		}
	}
	targets := map[consumptionKey]decimal.Decimal{}
	keys := []consumptionKey{}
	for _, entry := range plan.target {
		if _, seen := targets[entry.Key]; !seen {
			keys = append(keys, entry.Key)
		}
		targets[entry.Key] = targets[entry.Key].Add(entry.Quantity)
	}
	for key := range net {
		if _, seen := targets[key]; !seen {
			keys = append(keys, key)
			targets[key] = zero
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].ItemID != keys[right].ItemID {
			return keys[left].ItemID < keys[right].ItemID
		}
		return keys[left].WarehouseID < keys[right].WarehouseID
	})
	written := 0
	for _, key := range keys {
		current := net[key]
		difference := targets[key].Sub(current.quantity)
		if difference.IsZero() {
			continue
		}
		input := MovementInput{
			EntryKey: plan.entryPrefix + ":" + key.ItemID + ":" + key.WarehouseID, ItemID: key.ItemID, WarehouseID: key.WarehouseID,
			OccurredAt: plan.occurredAt, SourceKind: plan.sourceKind, SourceID: reference(plan.sourceID),
			RestaurantUnitID: reference(plan.unitID), Reason: stationReason(plan.reason, plan.stations[key.WarehouseID]),
			ActorID: plan.actorID, OperationID: plan.operationID,
		}
		if plan.actorID == nil {
			input.Process = reference(plan.process)
		}
		if difference.IsPositive() {
			input.Kind = KindConsumption
			input.Quantity = difference
		} else {
			reversed := difference.Neg()
			value := current.value
			if !reversed.Equal(current.quantity) && current.quantity.IsPositive() {
				value = reversed.Mul(current.value).DivRound(current.quantity, valueScale)
			}
			input.Kind = KindConsumptionReversal
			input.Quantity = reversed
			input.Value = &value
		}
		if _, err := Apply(ctx, tx, input); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// stationReason agrega al motivo las estaciones que resolvieron el almacén (para verificar la
// asignación), o indica que se usó el almacén predeterminado.
func stationReason(reason string, stations []string) *string {
	text := reason
	if len(stations) > 0 {
		text += " · Estación: " + strings.Join(stations, ", ")
	}
	return &text
}
