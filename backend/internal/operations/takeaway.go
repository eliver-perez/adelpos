package operations

import "strings"

// Para llevar en la aplicación de meseros (etapa A2, §3 de la especificación y decisión A2-D1).

// TakeawayOrigin es el origen con que «Nuevo pedido» abre un pedido para llevar: el mismo que manda la caja
// (desktop/src/services/restaurant.ts, defaultTakeawayOrigin) y el del pedido para llevar capturado sin conexión.
const TakeawayOrigin = "Para llevar"

// Límites del nombre del cliente y el teléfono de una cuenta: los mismos al abrirla o editarla (validateAccountInput),
// en el pedido para llevar sin conexión y en «Datos del cliente» (updateTakeawayCustomer). Se cuentan como siempre en
// bytes del texto ya recortado.
const (
	maxCustomerNameLength = 100
	maxPhoneLength        = 40
)

const customerFieldsMessage = "Revisa el nombre del cliente (100) y el teléfono (40 caracteres)."

// validCustomerFields dice si el nombre del cliente y el teléfono (ya recortados) caben en sus límites.
func validCustomerFields(customerName, phone string) bool {
	return len(customerName) <= maxCustomerNameLength && len(phone) <= maxPhoneLength
}

// takeawayCustomerInput es el cuerpo de POST restaurant/accounts/{id}/customer. Los dos campos son obligatorios (vacío
// quita el dato), para que un cliente que solo manda uno no borre el otro sin querer.
type takeawayCustomerInput struct {
	OperationID  string  `json:"operation_id"`
	CustomerName *string `json:"customer_name"`
	Phone        *string `json:"phone"`
}

// updateTakeawayCustomer cambia el nombre del cliente y el teléfono de un pedido para llevar abierto (A2-D1). Es una
// ruta estrecha en lugar de PATCH restaurant/accounts/{id}: exige takeaway.manage (routePermission y authorizeAccount)
// y no accounts.manage, que permitiría además renombrar cualquier cuenta y cambiar observaciones y programación.
//
// No pide la revisión ni la sube: solo cambia esos dos datos, que no intervienen en nada de lo que la revisión protege
// (partidas, borrador, importes, mesa ni programación). Así el mesero los edita en cualquier momento sin chocar con el
// borrador, que sube la revisión en cada guardado, y una edición hecha sin conexión se envía después sin quedar
// obsoleta: la última edición gana. El lock exclusivo de la operación (7820148305, Handle) ordena esta escritura con
// las demás de la cuenta.
//
// La idempotencia es la de toda escritura de cuentas (Handle, restaurant_operations). En la misma transacción quedan la
// historia de la cuenta, el Registro de movimientos y el evento restaurant.account.updated en el ámbito takeaway
// (handler.event); sin cambios no se registra nada.
func (handler *service) updateTakeawayCustomer(id string) (any, error) {
	var input takeawayCustomerInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.CustomerName == nil || input.Phone == nil {
		return nil, invalid("Indica el nombre del cliente y el teléfono (vacíos para quitarlos).")
	}
	customerName := strings.TrimSpace(*input.CustomerName)
	phone := strings.TrimSpace(*input.Phone)
	if !validCustomerFields(customerName, phone) {
		return nil, invalid(customerFieldsMessage)
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if account.Mode != "takeaway" {
		return nil, invalid("El nombre del cliente y el teléfono se cambian aquí solo en los pedidos para llevar.")
	}
	if account.State != "open" {
		return nil, accountNotOpen(account)
	}
	if account.CustomerName == customerName && account.Phone == phone {
		return handler.readBill(id)
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET customer_name=$1,phone=$2,updated_at=now()
        WHERE id=$3 AND state='open'`, customerName, phone, id)
	if err != nil {
		return nil, err
	}
	saved, err := handler.readBill(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.event(id, nil, "account.updated", account, saved.Account)
}
