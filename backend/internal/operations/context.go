package operations

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Error struct {
	Status  int
	Code    string
	Message string
	// Details acompaña al error con datos públicos que la terminal necesita para decidir (por
	// ejemplo, las cuentas que otra terminal abrió en la mesa). Nunca lleva secretos.
	Details any
}

func (problem *Error) Error() string {
	return problem.Code
}

func invalid(message string) error {
	return &Error{Status: 400, Code: "OPERATION_VALIDATION", Message: message}
}

func conflict() error {
	return &Error{Status: 409, Code: "ACCOUNT_CONFLICT", Message: "La cuenta o distribución cambió en otra terminal. Recarga y compara los cambios antes de continuar."}
}

type Permission struct {
	Key         string
	Description string
}

var Permissions = []Permission{
	{"restaurant.areas.read", "Consultar áreas del restaurante"},
	{"restaurant.areas.manage", "Administrar áreas del restaurante"},
	{"restaurant.tables.read", "Consultar mesas y ocupación real"},
	{"restaurant.tables.manage", "Administrar mesas y distribución"},
	{"restaurant.settings.manage", "Configurar umbrales del tablero para llevar"},
	{"accounts.read", "Consultar cuentas e importes"},
	{"accounts.open", "Abrir cuentas"},
	{"accounts.manage", "Editar referencias y anular cuentas sin partidas activas"},
	{"accounts.split", "Dividir cuentas"},
	{"accounts.merge", "Combinar cuentas compatibles"},
	{"accounts.transfer", "Transferir la responsabilidad de cuentas"},
	{"accounts.move", "Mover cuentas entre mesas"},
	{"accounts.transfer_items", "Transferir partidas y cantidades entre cuentas"},
	{"orders.capture", "Capturar y editar borradores de pedidos"},
	{"orders.confirm", "Confirmar pedidos"},
	{"orders.edit", "Modificar partidas confirmadas sin aprobación adicional"},
	{"orders.cancel", "Cancelar partidas o cantidades confirmadas"},
	{"orders.price_list.manage", "Seleccionar listas de precios para nuevas capturas"},
	{"orders.history.read", "Consultar historial de pedidos y cuentas"},
	{"takeaway.read", "Consultar tablero de pedidos para llevar y ventanilla"},
	{"takeaway.manage", "Administrar datos de pedidos para llevar"},
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
			return &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación del restaurante."}
		}
	}
	return nil
}

type Request struct {
	Method string
	Route  string
	Query  url.Values
	Body   json.RawMessage
	Actor  Actor
}

type Audit func(context.Context, pgx.Tx, string, string, *string, any, any) error

type service struct {
	ctx         context.Context
	tx          pgx.Tx
	request     Request
	audit       Audit
	operationID string
}

var identifierPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validateID(id string) error {
	if !identifierPattern.MatchString(id) {
		return invalid("El identificador de la operación o del registro no es válido.")
	}
	return nil
}

func newID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6] = (value[6] & 15) | 64
	value[8] = (value[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func decode(body []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalid("Los campos de la operación no tienen un formato válido.")
	}
	return nil
}

func jsonValue(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
