package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
)

// Actor es quien hace la petición, con sus permisos efectivos leídos de PostgreSQL.
type Actor struct {
	UserID      string
	DeviceID    string
	Permissions []string
}

// Can indica si el actor tiene el permiso.
func (actor Actor) Can(permission string) bool {
	return slices.Contains(actor.Permissions, permission)
}

// Request es una petición documents/identity* ya autenticada por la administración.
type Request struct {
	Method string
	Route  string
	Body   []byte
	Actor  Actor
}

// Audit escribe un registro de auditoría en la misma transacción.
type Audit func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error

// route describe una ruta: qué permisos exige y si escribe (operation_id, lock, idempotencia).
type route struct {
	// anyPermission exige al menos uno de estos permisos.
	anyPermission []string
	write         bool
	action        func(handler *service) (any, error)
}

// service es el contexto de una petición.
type service struct {
	ctx     context.Context
	tx      pgx.Tx
	request Request
	audit   Audit
	// current es la fila de la identidad bloqueada para escribir (solo en escrituras).
	current *identityRow
}

var identifierPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Routes son las rutas que atiende Handle (la administración las delega por prefijo).
var Routes = []string{
	"documents/identity",
	"documents/identity/logo",
	"documents/identity/logo/remove",
	"documents/identity/logo/preview",
}

// Owns indica si una ruta pertenece a la identidad documental.
func Owns(path string) bool {
	return slices.Contains(Routes, strings.Trim(path, "/"))
}

// resolve enumera las rutas de la identidad documental.
func resolve(method, path string) *route {
	readers := []string{PermissionTemplatesRead, PermissionSettingsRead}
	managers := []string{PermissionManage}
	switch method + " " + strings.Trim(path, "/") {
	case "GET documents/identity":
		return &route{anyPermission: readers, action: (*service).read}
	case "PUT documents/identity":
		return &route{anyPermission: managers, write: true, action: (*service).save}
	case "POST documents/identity/logo":
		return &route{anyPermission: managers, write: true, action: (*service).uploadLogo}
	case "POST documents/identity/logo/remove":
		return &route{anyPermission: managers, write: true, action: (*service).removeLogo}
	case "POST documents/identity/logo/preview":
		// Convierte con otra presentación sin guardar nada: no es una escritura, pero solo la usa
		// quien puede cambiar la presentación.
		return &route{anyPermission: managers, action: (*service).previewLogo}
	}
	return nil
}

// Handle atiende documents/identity* dentro de la transacción autenticada de la administración.
//   - Los permisos se comprueban antes de leer el cuerpo y de cualquier escritura.
//   - Las escrituras toman primero el lock de fila de la identidad: así dos peticiones con el
//     mismo operation_id se serializan y la segunda recibe la respuesta guardada de la primera.
//   - Toda escritura exige operation_id y se registra en restaurant_operations con su huella: un
//     reintento con el mismo cuerpo devuelve la respuesta guardada sin repetir efectos y uno con otro
//     cuerpo responde 409 IDEMPOTENCY_CONFLICT. Además exige la revisión vigente (409
//     REVISION_CONFLICT si otra persona guardó antes).
func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	selected := resolve(request.Method, request.Route)
	if selected == nil {
		return nil, notFound("La ruta de identidad documental no existe.")
	}
	if !canAny(request.Actor, selected.anyPermission) {
		return nil, denied()
	}
	handler := &service{ctx: ctx, tx: tx, request: request, audit: audit}
	if !selected.write {
		return selected.action(handler)
	}
	operationID, err := readOperationID(request.Body)
	if err != nil {
		return nil, err
	}
	handler.current, err = lockIdentity(ctx, tx)
	if err != nil {
		return nil, err
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return nil, err
	}
	cached, found, err := handler.replay(operationID, fingerprint)
	if err != nil || found {
		return cached, err
	}
	result, err := selected.action(handler)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id, actor_id, device_id, route, fingerprint, response)
        VALUES ($1, $2, $3, $4, $5, $6)`,
		operationID,
		request.Actor.UserID,
		request.Actor.DeviceID,
		request.Method+" "+strings.Trim(request.Route, "/"),
		fingerprint,
		encoded,
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func canAny(actor Actor, permissions []string) bool {
	for _, permission := range permissions {
		if actor.Can(permission) {
			return true
		}
	}
	return false
}

// readOperationID exige un operation_id UUID en el cuerpo de una escritura.
func readOperationID(body []byte) (string, error) {
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(body, &mutation); err != nil {
		return "", invalid("La operación requiere un cuerpo JSON válido.")
	}
	if !identifierPattern.MatchString(mutation.OperationID) {
		return "", invalid("La operación requiere un operation_id válido.")
	}
	return strings.ToLower(mutation.OperationID), nil
}

// requestFingerprint identifica el contenido de la operación: método, ruta y JSON canónico del
// cuerpo (los números se conservan como texto). El logotipo forma parte de la huella, de modo que
// reintentar con otro archivo y el mismo operation_id se rechaza.
func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(request.Body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	route := request.Method + " " + strings.Trim(request.Route, "/")
	digest := sha256.Sum256([]byte(route + "\n" + string(canonical)))
	return fmt.Sprintf("%x", digest), nil
}

// replay devuelve la respuesta guardada de una operación ya confirmada por el mismo actor, desde la
// misma terminal y con la misma huella.
func (handler *service) replay(operationID, fingerprint string) (any, bool, error) {
	var actorID, deviceID, previous string
	var response json.RawMessage
	err := handler.tx.QueryRow(handler.ctx, `SELECT actor_id, device_id, fingerprint, response
        FROM app.restaurant_operations WHERE id = $1`, operationID).Scan(&actorID, &deviceID, &previous, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	actor := handler.request.Actor
	if actorID != actor.UserID || deviceID != actor.DeviceID || previous != fingerprint {
		return nil, false, conflict(
			"IDEMPOTENCY_CONFLICT",
			"El identificador de operación ya corresponde a otra solicitud. No se repitieron sus cambios.",
		)
	}
	return response, true, nil
}

// decodeStrict decodifica el cuerpo de una ruta rechazando claves desconocidas.
func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return invalid("Los datos de la identidad no tienen el formato esperado.")
	}
	return nil
}

// changed audita y publica un cambio de la identidad en la misma transacción.
func (handler *service) changed(action, entity string, id *string, before, after any) error {
	if err := handler.audit(handler.ctx, handler.tx, action, entity, id, before, after); err != nil {
		return err
	}
	return events.Append(handler.ctx, handler.tx, EventChanged, Scope, nil, nil)
}

// Scope es el ámbito SSE de los eventos de la identidad: el de los documentos (visible con
// documents.templates.read).
const Scope = "documents"

func newID() string {
	return events.NewID()
}
