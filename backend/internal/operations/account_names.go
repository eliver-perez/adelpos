package operations

import (
	"reflect"
	"strings"
)

// Referencias neutras de una cuenta sin nombre capturado (etapa 6.2). La columna
// restaurant_accounts.name es obligatoria, así que Go asigna una referencia cuando la cuenta se
// abre o se edita sin nombre: en salón, el nombre de su mesa (o «Cuenta» si no tiene mesa); en
// para llevar, «Pedido para llevar». La interfaz identifica estas cuentas por su folio y no
// presenta la referencia como si fuera un nombre capturado.
const (
	neutralDineInName   = "Cuenta"
	neutralTakeawayName = "Pedido para llevar"
)

// neutralAccountName devuelve la referencia neutra para la modalidad y la mesa indicadas. Una mesa
// con identificador inválido o inexistente usa «Cuenta»: la validación de la mesa de destino
// (validateTableTarget) es la que rechaza la operación después.
func (handler *service) neutralAccountName(mode string, tableID *string) (string, error) {
	if mode == "takeaway" {
		return neutralTakeawayName, nil
	}
	if tableID == nil {
		return neutralDineInName, nil
	}
	if err := validateID(*tableID); err != nil {
		return neutralDineInName, nil
	}
	var name string
	err := handler.tx.QueryRow(handler.ctx, "SELECT coalesce((SELECT name FROM app.restaurant_tables WHERE id=$1),'')", *tableID).Scan(&name)
	if err != nil {
		return "", err
	}
	if name == "" {
		return neutralDineInName, nil
	}
	return name, nil
}

// fillAccountName asigna la referencia neutra cuando el nombre llega vacío. Solo la usan abrir una
// cuenta y editar sus datos: dividir conserva su propia validación (exige el nombre de la cuenta
// resultante).
func (handler *service) fillAccountName(input *AccountInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name != "" {
		return nil
	}
	name, err := handler.neutralAccountName(input.Mode, input.TableID)
	if err != nil {
		return err
	}
	input.Name = name
	return nil
}

// movedAccountName conserva la identificación de una cuenta que cambia de mesa: si su nombre es la
// referencia neutra de la mesa actual, pasa a ser la de la mesa nueva; un nombre capturado se
// conserva sin cambios.
func (handler *service) movedAccountName(account *Account, tableID *string) (string, error) {
	if reflect.DeepEqual(account.TableID, tableID) {
		return account.Name, nil
	}
	current, err := handler.neutralAccountName(account.Mode, account.TableID)
	if err != nil {
		return "", err
	}
	if account.Name != current {
		return account.Name, nil
	}
	return handler.neutralAccountName(account.Mode, tableID)
}
