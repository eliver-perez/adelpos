package kitchen

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/stationscope"
)

type Error struct {
	Status  int
	Code    string
	Message string
	// Details son datos públicos opcionales del rechazo (etapa 9F: la impresora que no puede imprimir).
	Details any
}

func (problem *Error) Error() string {
	return problem.Code
}
func invalid(message string) error {
	return &Error{Status: 400, Code: "KITCHEN_VALIDATION", Message: message}
}
func conflict() error {
	return &Error{Status: 409, Code: "REVISION_CONFLICT", Message: "La información cambió. Actualiza y revisa antes de volver a guardar."}
}

type Actor struct {
	UserID      string
	DeviceID    string
	Permissions []string
}

func (actor Actor) Can(key string) bool {
	return slices.Contains(actor.Permissions, key)
}
func (actor Actor) Require(keys ...string) error {
	for _, key := range keys {
		if !actor.Can(key) {
			return &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación de cocina."}
		}
	}
	return nil
}

type Audit func(context.Context, pgx.Tx, string, string, *string, any, any) error
type Request struct {
	Method string
	Route  string
	Query  url.Values
	Body   json.RawMessage
	Actor  Actor
}
type service struct {
	ctx         context.Context
	tx          pgx.Tx
	request     Request
	audit       Audit
	operationID string
	// scope son las estaciones visibles en la terminal para el usuario (etapa 9F); se lee una sola
	// vez por operación.
	scope *stationscope.Scope
}

var identifierPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(id string) error {
	if !identifierPattern.MatchString(id) {
		return invalid("El identificador no es válido.")
	}
	return nil
}
func encoded(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}
func decode(body []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("Revisa los campos y tipos del documento de cocina.")
	}
	return nil
}
func one[T any](ctx context.Context, tx pgx.Tx, query string, args ...any) (*T, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func many[T any](ctx context.Context, tx pgx.Tx, query string, args ...any) ([]T, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func (handler *service) history(kind string, id *string, before, after any) error {
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_history
        (id,operation_id,actor_id,device_id,entity_id,kind,before_value,after_value)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, events.NewID(), handler.operationID,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, id, kind, encoded(before), encoded(after))
	if err != nil {
		return err
	}
	if handler.audit != nil {
		return handler.audit(handler.ctx, handler.tx, "kitchen."+kind, "kitchen", id, before, after)
	}
	return nil
}

// stationScope devuelve las estaciones que se ven en esta terminal (etapa 9F, decisión D6): las que
// muestra la terminal intersectadas con las que atiende el usuario (con kitchen.stations.all, todas las
// de la terminal). Rige la preparación, sus cambios de estado y los acuses de recepción.
func (handler *service) stationScope() (*stationscope.Scope, error) {
	if handler.scope != nil {
		return handler.scope, nil
	}
	actor := handler.request.Actor
	scope, err := stationscope.Load(handler.ctx, handler.tx, actor.DeviceID, actor.UserID, actor.Can("kitchen.stations.all"))
	if err != nil {
		return nil, err
	}
	handler.scope = scope
	return scope, nil
}

// documentScope es el alcance de las comandas guardadas (lista, detalle y vista previa de la cola): con
// kitchen.stations.all todas, porque la cola, la reimpresión y el historial son administrativos y no
// dependen de la terminal; sin él, las estaciones visibles.
func (handler *service) documentScope() (*stationscope.Scope, error) {
	if handler.request.Actor.Can("kitchen.stations.all") {
		return stationscope.Everything(), nil
	}
	return handler.stationScope()
}

// stationAccess exige que la estación sea visible en esta terminal para el usuario.
func (handler *service) stationAccess(id string) error {
	if err := validID(id); err != nil {
		return err
	}
	scope, err := handler.stationScope()
	if err != nil {
		return err
	}
	if !scope.Allows(id) {
		return stationDenied()
	}
	return nil
}

// documentStationAccess exige que la sección de una comanda pertenezca al alcance de las comandas.
func (handler *service) documentStationAccess(id string) error {
	if err := validID(id); err != nil {
		return err
	}
	scope, err := handler.documentScope()
	if err != nil {
		return err
	}
	if !scope.Allows(id) {
		return stationDenied()
	}
	return nil
}

// stationDenied es el rechazo de una estación que no se ve en esta terminal o que el usuario no atiende.
func stationDenied() error {
	return &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "Esta estación no se muestra en esta terminal o tu usuario no la atiende."}
}
