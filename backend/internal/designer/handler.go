// Package designer atiende las rutas documents/* del diseñador de documentos (etapa 9B, fase A):
// catálogo de bloques, plantillas y versiones, vista previa con datos de muestra o con un documento
// real, documentos recientes, impresión de muestras e impresoras compatibles.
//
// Reglas:
//   - todos los permisos de la ruta se comprueban antes de cualquier lectura con bloqueo o escritura;
//     la vista previa con un documento real exige además los permisos de su módulo de origen, que
//     aplican kitchen y payments;
//   - toda escritura exige operation_id y se registra en restaurant_operations con su huella (patrón
//     de inventory/handler.go): un reintento con el mismo cuerpo devuelve la respuesta guardada y
//     uno con otro cuerpo responde 409 IDEMPOTENCY_CONFLICT;
//   - las plantillas usan revisión optimista (409 REVISION_CONFLICT) y bloqueo de fila; las
//     versiones son inmutables (disparador de la 0018);
//   - auditoría y evento documents.templates.changed (ámbito documents) en la misma transacción;
//   - las muestras se encolan con el lock 7820148305 exclusivo, como la prueba de impresora; las
//     lecturas de documentos reales lo toman compartido, como Cobros y Caja.
package designer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
)

// operationsLock es el lock de restaurante, cocina, cobros y caja (CLAUDE.md §3).
const operationsLock = 7820148305

// Actor es el usuario autenticado con sus permisos efectivos.
type Actor struct {
	UserID      string
	DeviceID    string
	Permissions []string
}

// Can indica si el actor tiene un permiso.
func (actor Actor) Can(permission string) bool {
	return slices.Contains(actor.Permissions, permission)
}

// Require exige todos los permisos indicados.
func (actor Actor) Require(permissions ...string) error {
	for _, permission := range permissions {
		if !actor.Can(permission) {
			return &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación de documentos."}
		}
	}
	return nil
}

// Request es la petición ya autenticada. Queues consulta las colas CUPS (compatibilidad de las
// impresoras); Rasters entrega la vista previa del logotipo (fase B). Sin valor, se usan las
// consultas de solo lectura del equipo y la vista previa no lleva imagen.
type Request struct {
	Method  string
	Route   string
	Query   url.Values
	Body    json.RawMessage
	Actor   Actor
	Queues  kitchen.QueueInspector
	Rasters documents.RasterSource
}

// Audit registra la auditoría en la misma transacción.
type Audit func(ctx context.Context, tx pgx.Tx, action, entity string, id *string, before, after any) error

// Error es un rechazo público del diseñador. Details lleva datos públicos (el bloque afectado).
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: 400, Code: "DOCUMENTS_VALIDATION", Message: message}
}

func conflict(code, message string) error {
	return &Error{Status: 409, Code: code, Message: message}
}

func notFound(message string) error {
	return &Error{Status: 404, Code: "NOT_FOUND", Message: message}
}

// templateInvalid traduce el rechazo de validación de una plantilla a 422 con el bloque afectado.
func templateInvalid(err error) error {
	var problem *documents.ValidationError
	if errors.As(err, &problem) {
		return &Error{Status: 422, Code: "TEMPLATE_INVALID", Message: problem.Message, Details: map[string]string{"block_id": problem.BlockID}}
	}
	return err
}

var identifierPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(id string) error {
	if !identifierPattern.MatchString(id) {
		return invalid("El identificador no es válido.")
	}
	return nil
}

// decode decodifica el cuerpo rechazando campos desconocidos.
func decode(body []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("Revisa los campos y tipos de la solicitud.")
	}
	return nil
}

func encoded(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}

type service struct {
	ctx         context.Context
	tx          pgx.Tx
	request     Request
	audit       Audit
	operationID string
}

// route describe una ruta: permisos exigidos antes de todo, si escribe, si toma el lock operativo
// (compartido en lecturas de documentos reales y exclusivo al encolar muestras) y su acción.
type route struct {
	permissions []string
	write       bool
	lock        string
	action      func(handler *service, parts []string) (any, error)
}

const (
	lockNone      = ""
	lockShared    = "shared"
	lockExclusive = "exclusive"
)

// resolve enumera las rutas documents/*. Un solo identificador por ruta, siempre en la segunda
// posición, como exige el puente nativo.
func resolve(method string, parts []string) *route {
	read := documents.PermissionTemplatesRead
	manage := documents.PermissionTemplatesManage
	length := len(parts)
	resource := parts[0]
	switch {
	case length == 1 && resource == "catalog" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).catalog}
	case length == 1 && resource == "templates" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).listTemplates}
	case length == 1 && resource == "templates" && method == "POST":
		return &route{permissions: []string{read, manage}, write: true, action: (*service).createTemplate}
	case length == 2 && resource == "templates" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).readTemplate}
	case length == 2 && resource == "templates" && method == "PUT":
		return &route{permissions: []string{read, manage}, write: true, action: (*service).saveTemplate}
	case length == 3 && resource == "templates" && parts[2] == "rename" && method == "POST":
		return &route{permissions: []string{read, manage}, write: true, action: (*service).renameTemplate}
	case length == 3 && resource == "templates" && parts[2] == "activate" && method == "POST":
		return &route{permissions: []string{read, manage}, write: true, action: (*service).activateTemplate}
	case length == 3 && resource == "templates" && parts[2] == "versions" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).listVersions}
	case length == 2 && resource == "versions" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).readVersion}
	case length == 1 && resource == "restore-default" && method == "POST":
		return &route{permissions: []string{read, manage}, write: true, action: (*service).restoreDefault}
	case length == 1 && resource == "preview" && method == "POST":
		// Solo calcula; con un documento real exige además documents.preview.real y su origen.
		return &route{permissions: []string{read}, lock: lockShared, action: (*service).preview}
	case length == 1 && resource == "recent" && method == "GET":
		return &route{permissions: []string{read, documents.PermissionPreviewReal}, lock: lockShared, action: (*service).recent}
	case length == 1 && resource == "samples" && method == "POST":
		return &route{permissions: []string{documents.PermissionSamplesPrint}, write: true, lock: lockExclusive, action: (*service).printSample}
	case length == 1 && resource == "printers" && method == "GET":
		return &route{permissions: []string{read}, action: (*service).printers}
	}
	return nil
}

// Handle atiende documents/* dentro de la transacción autenticada de administración.
func Handle(ctx context.Context, tx pgx.Tx, request Request, audit Audit) (any, error) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(request.Route, "documents/"), "/"), "/")
	selected := resolve(request.Method, parts)
	if selected == nil {
		return nil, notFound("La ruta de documentos no existe.")
	}
	if err := request.Actor.Require(selected.permissions...); err != nil {
		return nil, err
	}
	if len(parts) > 1 {
		if err := validID(parts[1]); err != nil {
			return nil, err
		}
	}
	switch selected.lock {
	case lockShared:
		if _, err := tx.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_xact_lock_shared(%d)", operationsLock)); err != nil {
			return nil, err
		}
	case lockExclusive:
		if _, err := tx.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_xact_lock(%d)", operationsLock)); err != nil {
			return nil, err
		}
	}
	handler := &service{ctx: ctx, tx: tx, request: request, audit: audit}
	if !selected.write {
		return selected.action(handler, parts)
	}
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal(request.Body, &mutation); err != nil {
		return nil, invalid("La operación requiere un cuerpo JSON válido.")
	}
	if err := validID(strings.ToLower(mutation.OperationID)); err != nil {
		return nil, invalid("La operación requiere un operation_id válido.")
	}
	handler.operationID = strings.ToLower(mutation.OperationID)
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return nil, err
	}
	cached, found, err := handler.replay(fingerprint)
	if err != nil || found {
		return cached, err
	}
	result, err := selected.action(handler, parts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.restaurant_operations(id,actor_id,device_id,route,fingerprint,response)
        VALUES ($1,$2,$3,$4,$5,$6)`, handler.operationID, request.Actor.UserID, request.Actor.DeviceID,
		request.Method+" "+request.Route, fingerprint, encoded(result))
	if err != nil {
		return nil, err
	}
	return result, nil
}

// requestFingerprint identifica el contenido de la operación: método, ruta y JSON canónico del
// cuerpo (los números se conservan como texto).
func requestFingerprint(request Request) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(request.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", invalid("La operación no contiene JSON válido.")
	}
	digest := sha256.Sum256([]byte(request.Method + " " + request.Route + "\n" + string(encoded(value))))
	return fmt.Sprintf("%x", digest), nil
}

// replay devuelve la respuesta guardada de una operación ya confirmada por el mismo actor, desde la
// misma terminal y con la misma huella.
func (handler *service) replay(fingerprint string) (any, bool, error) {
	var actorID, deviceID, previousFingerprint string
	var result json.RawMessage
	err := handler.tx.QueryRow(handler.ctx, `SELECT actor_id,device_id,fingerprint,response
        FROM app.restaurant_operations WHERE id=$1`, handler.operationID).Scan(&actorID, &deviceID, &previousFingerprint, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if actorID != handler.request.Actor.UserID || deviceID != handler.request.Actor.DeviceID || previousFingerprint != fingerprint {
		return nil, false, conflict("IDEMPOTENCY_CONFLICT", "Esta operación ya se había enviado con otros datos y no se repitió. Consulta el resultado antes de intentarlo de nuevo.")
	}
	return result, true, nil
}

// changed publica el evento de plantillas en el ámbito documents, en la misma transacción.
func (handler *service) changed(templateID string) error {
	return appendEvent(handler.ctx, handler.tx, documents.EventTemplatesChanged, &templateID)
}

func (handler *service) catalog(_ []string) (any, error) {
	return documents.BuildCatalog(), nil
}
