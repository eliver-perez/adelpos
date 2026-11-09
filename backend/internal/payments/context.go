// Package payments es la autoridad financiera de las cuentas: saldos, pagos, asignaciones a
// unidades, cambio, descuentos manuales, cortesías, cargos, propinas, IVA, devoluciones,
// correcciones, turno mínimo, movimientos de caja, documentos comerciales y entregas.
//
// catalog sigue siendo la única autoridad de precios: este paquete nunca recalcula importes de
// productos; reutiliza los importes históricos de restaurant_item_units.
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

type Permission struct {
	Key         string
	Description string
}

// Permissions enumera las operaciones financieras reales. Cada clave también se inserta en la
// migración que la introdujo (0006, 0007 u 0008) y se concede inicialmente solo al rol administrador.
var Permissions = []Permission{
	{"payments.read", "Consultar información financiera y pagos de cuentas"},
	{"payments.collect", "Registrar cobros"},
	{"payments.collect_partial", "Registrar pagos parciales"},
	{"payments.collect_selection", "Cobrar productos y cantidades seleccionados"},
	{"payments.methods.manage", "Administrar métodos de pago"},
	{"payments.settings.manage", "Configurar políticas de cobro e impresión de tickets"},
	{"payments.tax.manage", "Configurar la tasa de IVA del establecimiento"},
	{"payments.discounts.apply", "Aplicar descuentos manuales dentro del límite del rol"},
	{"payments.discounts.authorize", "Autorizar descuentos superiores al límite"},
	{"payments.courtesy", "Otorgar cortesías"},
	{"payments.charges.create", "Registrar cargos extras generales"},
	{"payments.charges.manage", "Modificar o cancelar cargos extras"},
	{"payments.tips.record", "Registrar propinas"},
	{"payments.corrections", "Anular o corregir pagos"},
	{"payments.refunds.execute", "Ejecutar devoluciones"},
	{"payments.refunds.authorize", "Autorizar devoluciones"},
	{"payments.tickets.print", "Imprimir tickets y precuentas"},
	{"payments.tickets.reprint", "Reimprimir tickets"},
	{"cash.shifts.open", "Abrir turno de caja"},
	{"cash.shifts.read", "Consultar turnos y movimientos de caja"},
	{"payments.usd_rate.manage", "Capturar el tipo de cambio del dólar del turno"},
	{"takeaway.deliver", "Registrar entrega de pedidos para llevar"},
	// Etapa 7 (migración 0008): administración de caja, arqueos y cortes.
	{permissionShiftsReadAll, "Consultar turnos de otras cajas"},
	{permissionIncome, "Registrar ingresos de efectivo"},
	{permissionWithdrawals, "Registrar retiros de efectivo"},
	{permissionExpenses, "Registrar gastos pagados desde caja"},
	{permissionOutflowsAuthorize, "Autorizar retiros y gastos de efectivo"},
	{permissionCorrections, "Corregir y regularizar movimientos de efectivo"},
	{permissionCorrectionsAuthorize, "Autorizar correcciones y regularizaciones de efectivo"},
	{permissionMovementsRead, "Consultar movimientos de efectivo"},
	{permissionPartialCuts, "Realizar cortes parciales"},
	{permissionCounts, "Realizar arqueos"},
	{permissionClose, "Cerrar turnos de caja"},
	{permissionCancelAnyClosing, "Cancelar el arqueo de cierre iniciado por otro usuario"},
	{permissionHistoryRead, "Consultar turnos y cortes históricos"},
	{permissionExpectedRead, "Consultar importes esperados y diferencias"},
	{permissionClosuresReview, "Comentar cierres de caja"},
	{"cash.reports.print", "Imprimir cortes y comprobantes de caja"},
	{"cash.reports.reprint", "Reimprimir cortes y comprobantes de caja"},
	{"cash.drawer.open", "Abrir el cajón de dinero manualmente"},
	{permissionCashSettings, "Administrar denominaciones, categorías de gasto, tope de caja y cajón"},
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

// Supervisor es la identidad validada de una autorización puntual. No es una sesión.
type Supervisor struct {
	UserID      string
	Username    string
	Permissions []string
}

func (supervisor Supervisor) Can(permission string) bool {
	return slices.Contains(supervisor.Permissions, permission)
}

// Authorizer valida en la misma transacción usuario y contraseña de un supervisor, con el mismo
// límite de intentos del inicio de sesión. Lo implementa administración, dueña de las credenciales.
type Authorizer func(ctx context.Context, tx pgx.Tx, username, password string) (*Supervisor, error)

type Request struct {
	Method    string
	Route     string
	Query     url.Values
	Body      json.RawMessage
	Actor     Actor
	Authorize Authorizer
}

type Audit func(ctx context.Context, tx pgx.Tx, action, entity string, entityID *string, before, after any) error

type Error struct {
	Status  int
	Code    string
	Message string
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: 400, Code: "PAYMENT_VALIDATION", Message: message}
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

var identifierPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateID(value string) error {
	if !identifierPattern.MatchString(value) {
		return invalid("El identificador no es válido.")
	}
	return nil
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
		panic("valor financiero no serializable")
	}
	return encoded
}

func textLength(value string) int {
	return len([]rune(value))
}

// service agrupa el contexto de una operación financiera dentro de su transacción.
type service struct {
	ctx         context.Context
	tx          pgx.Tx
	request     Request
	audit       Audit
	operationID string
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
