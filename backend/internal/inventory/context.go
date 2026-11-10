// Package inventory es la autoridad de cantidades, existencias y costos de inventario (etapa 8):
// unidades y conversiones, insumos y presentaciones, almacenes, existencias por almacén, valuación
// por insumo, libro de movimientos de solo inserción y motor de costo promedio ponderado.
//
// Reglas de arquitectura:
//   - no importa operations, kitchen, payments ni administration; las tablas de otros módulos
//     (estaciones de cocina, usuarios) se leen por SQL en solo lectura;
//   - toda escritura del libro toma el lock 7820148308 exclusivo (siempre después de 7820148305 y
//     7820148304 cuando concurren) y es idempotente por operation_id (restaurant_operations) y por
//     entry_key (inventory_movements);
//   - auditoría y evento en la misma transacción; los eventos nunca llevan costos;
//   - los costos y valores solo se entregan con inventory.costs.read y la valuación total solo con
//     inventory.valuation.read; inventory.stock.read nunca implica costos.
package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/events"
)

// lockKey es el lock consultivo de inventario: libro de movimientos, existencias y documentos de
// inventario. Exclusivo en escrituras y compartido en lecturas que deben ser consistentes.
const lockKey = 7820148308

// cashLockKey es el lock de restaurante, cocina, cobros y caja (payments). Las rutas que leen gastos
// de caja lo toman compartido antes que lockKey; inventory nunca lo toma exclusivo.
const cashLockKey = 7820148305

// Scope es el ámbito SSE de inventario; lo reciben quienes tienen inventory.read.
const Scope = "inventory"

// Claves de permisos de la fase A.
const (
	permissionRead                 = "inventory.read"
	permissionItemsManage          = "inventory.items.manage"
	permissionCostsRead            = "inventory.costs.read"
	permissionCostsManage          = "inventory.costs.manage"
	permissionStockRead            = "inventory.stock.read"
	permissionWarehousesManage     = "inventory.warehouses.manage"
	permissionMovementsRead        = "inventory.movements.read"
	permissionAdjustmentsAuthorize = "inventory.adjustments.authorize"
	permissionValuationRead        = "inventory.valuation.read"
	permissionSettingsManage       = "inventory.settings.manage"
	// Fase B (0011).
	permissionRecipesManage    = "inventory.recipes.manage"
	permissionSubrecipesManage = "inventory.subrecipes.manage"
	permissionProductionRecord = "inventory.production.record"
	permissionWasteRecord      = "inventory.waste.record"
	// Fase C (0013): compras.
	permissionSuppliersManage    = "inventory.suppliers.manage"
	permissionPurchasesCreate    = "inventory.purchases.create"
	permissionPurchasesAuthorize = "inventory.purchases.authorize"
	permissionPurchasesReceive   = "inventory.purchases.receive"
	permissionSupplierReturns    = "inventory.supplier_returns.record"
	permissionPurchasesLinkCash  = "inventory.purchases.link_cash"
	// Fase D (0014): ajustes, conteos y transferencias.
	permissionAdjustmentsRecord = "inventory.adjustments.record"
	permissionCountsPerform     = "inventory.counts.perform"
	permissionCountsApprove     = "inventory.counts.approve"
	permissionTransfersRecord   = "inventory.transfers.record"
)

type Permission struct {
	Key         string
	Description string
}

// Permissions enumera los permisos de inventario. Cada clave también se inserta en la migración de
// su fase (0010 la fase A, 0011 la fase B, 0013 la fase C, 0014 la fase D) con la misma descripción y
// se concede inicialmente solo al rol administrador.
var Permissions = []Permission{
	{permissionRead, "Consultar inventario (sección y resumen)"},
	{permissionItemsManage, "Administrar insumos, unidades y presentaciones"},
	{permissionCostsRead, "Consultar costos (unitarios, valores, costo teórico)"},
	{permissionCostsManage, "Administrar costos de referencia y costos de existencias iniciales"},
	{permissionStockRead, "Consultar existencias (cantidades, sin costos)"},
	{permissionWarehousesManage, "Administrar almacenes y su asignación a estaciones"},
	{permissionMovementsRead, "Consultar movimientos (cantidades; valores solo con consulta de costos)"},
	{permissionAdjustmentsAuthorize, "Autorizar ajustes y corregir clasificaciones vencidas"},
	{permissionValuationRead, "Consultar valuaciones (valor total del inventario por insumo/almacén)"},
	{permissionSettingsManage, "Administrar parámetros de inventario"},
	{permissionRecipesManage, "Administrar recetas y porciones habituales"},
	{permissionSubrecipesManage, "Administrar subrecetas (preparaciones)"},
	{permissionProductionRecord, "Registrar elaboraciones de subrecetas almacenables"},
	{permissionWasteRecord, "Registrar mermas y clasificar cancelaciones"},
	{permissionSuppliersManage, "Administrar proveedores"},
	{permissionPurchasesCreate, "Crear órdenes de compra"},
	{permissionPurchasesAuthorize, "Emitir (autorizar), cancelar y cerrar órdenes de compra"},
	{permissionPurchasesReceive, "Recibir mercancía"},
	{permissionSupplierReturns, "Registrar devoluciones a proveedores"},
	{permissionPurchasesLinkCash, "Vincular compras con gastos de caja"},
	{permissionAdjustmentsRecord, "Registrar ajustes de inventario"},
	{permissionCountsPerform, "Realizar conteos físicos"},
	{permissionCountsApprove, "Aprobar y aplicar conteos"},
	{permissionTransfersRecord, "Transferir mercancía entre almacenes"},
}

type Actor struct {
	UserID      string
	DeviceID    string
	Permissions []string
}

func (actor Actor) Can(permission string) bool {
	return slices.Contains(actor.Permissions, permission)
}

func (actor Actor) Require(permissions ...string) error {
	for _, permission := range permissions {
		if !actor.Can(permission) {
			return denied()
		}
	}
	return nil
}

// RequireAny exige al menos uno de los permisos; una lista vacía no exige nada.
func (actor Actor) RequireAny(permissions ...string) error {
	if len(permissions) == 0 {
		return nil
	}
	for _, permission := range permissions {
		if actor.Can(permission) {
			return nil
		}
	}
	return denied()
}

type Request struct {
	Method string
	Route  string
	Query  url.Values
	Body   json.RawMessage
	Actor  Actor
}

// Audit escribe un registro de auditoría en la misma transacción de la operación.
type Audit func(ctx context.Context, tx pgx.Tx, action, entity string, entityID *string, before, after any) error

// Error es un rechazo público con código estable y mensaje en español. Ningún mensaje contiene
// costos ni valores de inventario.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: 400, Code: "INVENTORY_VALIDATION", Message: message}
}

func denied() error {
	return &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación."}
}

func conflict(code, message string) error {
	return &Error{Status: 409, Code: code, Message: message}
}

func notFound(message string) error {
	return &Error{Status: 404, Code: "NOT_FOUND", Message: message}
}

func revisionConflict(subject string) error {
	return conflict("REVISION_CONFLICT", subject+" cambió en otra terminal. Actualiza y vuelve a guardar.")
}

var identifierPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validID(value string) bool {
	return identifierPattern.MatchString(value)
}

func validateID(value string) error {
	if !validID(value) {
		return invalid("El identificador no es válido.")
	}
	return nil
}

// validateOptionalID acepta nil o un UUID; una cadena vacía equivale a nil.
func validateOptionalID(value *string) (*string, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if !validID(*value) {
		return nil, invalid("El identificador no es válido.")
	}
	normalized := strings.ToLower(*value)
	return &normalized, nil
}

func newID() string {
	return events.NewID()
}

// decode rechaza campos desconocidos y contenido adicional, como el resto de los módulos.
func decode(body json.RawMessage, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("La solicitud contiene campos o valores inválidos.")
	}
	if decoder.More() {
		return invalid("La solicitud contiene contenido adicional.")
	}
	return nil
}

func jsonValue(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic("valor de inventario no serializable")
	}
	return encoded
}

func textLength(value string) int {
	return len([]rune(value))
}

// hasControlCharacters detecta saltos de línea y caracteres invisibles en nombres que se muestran
// en listas y documentos.
func hasControlCharacters(value string) bool {
	return strings.IndexFunc(value, func(character rune) bool {
		return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
	}) >= 0
}

// validateName recorta y valida un nombre visible de 1 a maximum caracteres.
func validateName(value *string, subject string, maximum int) error {
	*value = strings.TrimSpace(*value)
	if textLength(*value) < 1 || textLength(*value) > maximum {
		return invalid(subject + " debe tener entre 1 y " + itoa(maximum) + " caracteres.")
	}
	if hasControlCharacters(*value) {
		return invalid(subject + " no admite saltos de línea ni caracteres de control.")
	}
	return nil
}

// validateDescription recorta una descripción opcional de hasta maximum caracteres.
func validateDescription(value *string, maximum int) error {
	*value = strings.TrimSpace(*value)
	if textLength(*value) > maximum {
		return invalid("La descripción admite hasta " + itoa(maximum) + " caracteres.")
	}
	return nil
}

var barcodePattern = regexp.MustCompile(`^[!-~]{1,64}$`)

// normalizeBarcode recorta un código de barras opcional: vacío equivale a sin código. Solo admite
// caracteres ASCII visibles, sin espacios (lo que entrega un lector de códigos).
func normalizeBarcode(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if !barcodePattern.MatchString(trimmed) {
		return nil, invalid("El código de barras admite de 1 a 64 caracteres visibles, sin espacios.")
	}
	return &trimmed, nil
}

func uniqueViolation(err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == "23505"
}

// constraintName devuelve la restricción o el índice que rechazó una escritura.
func constraintName(err error) string {
	var problem *pgconn.PgError
	if errors.As(err, &problem) {
		return problem.ConstraintName
	}
	return ""
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// service agrupa el contexto de una operación de inventario dentro de su transacción.
type service struct {
	ctx         context.Context
	tx          pgx.Tx
	request     Request
	audit       Audit
	operationID string
}

// costsVisible indica si la respuesta puede incluir costos unitarios y valores.
func (handler *service) costsVisible() bool {
	return handler.request.Actor.Can(permissionCostsRead)
}

// valuationVisible indica si la respuesta puede incluir la valuación total del inventario.
func (handler *service) valuationVisible() bool {
	return handler.request.Actor.Can(permissionValuationRead)
}

// publish agrega un evento del ámbito de inventario en la misma transacción. Solo lleva la
// referencia del agregado: ningún costo viaja por SSE.
func (handler *service) publish(kind string, aggregateID *string) error {
	return events.Append(handler.ctx, handler.tx, kind, Scope, aggregateID, nil)
}
