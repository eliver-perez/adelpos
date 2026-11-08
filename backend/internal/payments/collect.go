package payments

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

type Selection struct {
	VersionID string `json:"version_id"`
	Quantity  int    `json:"quantity"`
}

type PaymentInput struct {
	OperationID     string      `json:"operation_id"`
	MethodID        string      `json:"method_id"`
	Mode            string      `json:"mode"`
	Amount          string      `json:"amount"`
	Received        string      `json:"received"`
	Tip             string      `json:"tip"`
	Reference       string      `json:"reference"`
	Selections      []Selection `json:"selections"`
	ExpectedBalance string      `json:"expected_balance"`
	Print           *bool       `json:"print"`
	Deliver         bool        `json:"deliver"`
}

type SelectedUnit struct {
	UnitID    string `json:"unit_id"`
	VersionID string `json:"version_id"`
	Remaining string `json:"remaining"`
	Amount    string `json:"amount"`
}

// PaymentPlan es el cálculo autoritativo de Go para una captura: lo usan la previsualización y
// el registro, de modo que la interfaz nunca calcula importes, cambio ni saldos.
type PaymentPlan struct {
	Mode           string         `json:"mode"`
	Target         string         `json:"target"`
	Amount         string         `json:"amount"`
	Tip            string         `json:"tip"`
	Received       *string        `json:"received"`
	Change         string         `json:"change"`
	BalanceBefore  string         `json:"balance_before"`
	BalanceAfter   string         `json:"balance_after"`
	Settles        bool           `json:"settles"`
	SelectionTotal *string        `json:"selection_total"`
	Units          []SelectedUnit `json:"units"`

	method   *Method
	amount   decimal.Decimal
	tip      decimal.Decimal
	received *decimal.Decimal
	change   decimal.Decimal
	shares   []decimal.Decimal
	selected []unitState
	kind     string
}

// selectUnits elige, por cada partida, unidades con importe pendiente: primero las que ya tienen
// pago parcial y después por ordinal, para no dejar fracciones dispersas.
func selectUnits(finance *Finance, selections []Selection) ([]unitState, error) {
	if len(selections) > 100 {
		return nil, invalid("Selecciona hasta 100 partidas por cobro.")
	}
	seen := map[string]bool{}
	selected := []unitState{}
	for _, selection := range selections {
		if err := validateID(selection.VersionID); err != nil {
			return nil, err
		}
		if seen[selection.VersionID] {
			return nil, invalid("Una partida está repetida en la selección.")
		}
		seen[selection.VersionID] = true
		candidates := []unitState{}
		for _, unit := range finance.units {
			if unit.VersionID == selection.VersionID && unit.Remaining().Sign() > 0 {
				candidates = append(candidates, unit)
			}
		}
		sort.SliceStable(candidates, func(left, right int) bool {
			leftPartial := candidates[left].Paid.Sign() > 0
			rightPartial := candidates[right].Paid.Sign() > 0
			if leftPartial != rightPartial {
				return leftPartial
			}
			return candidates[left].Ordinal < candidates[right].Ordinal
		})
		if selection.Quantity < 1 || selection.Quantity > len(candidates) {
			return nil, invalid("La cantidad seleccionada excede las unidades pendientes de pago de esa partida.")
		}
		selected = append(selected, candidates[:selection.Quantity]...)
	}
	return selected, nil
}

func (handler *service) planPayment(finance *Finance, input PaymentInput) (*PaymentPlan, error) {
	if finance.State != "open" {
		return nil, conflict("ACCOUNT_NOT_OPEN", "La cuenta no está abierta; no admite cobros.")
	}
	if finance.balance.Sign() <= 0 {
		return nil, conflict("NOTHING_TO_COLLECT", "La cuenta no tiene saldo pendiente.")
	}
	if input.Mode != "total" && input.Mode != "partial" {
		return nil, invalid("Indica si el pago es total o parcial.")
	}
	if err := validateID(input.MethodID); err != nil {
		return nil, invalid("Selecciona un método de pago.")
	}
	method, err := handler.readMethod(input.MethodID)
	if err != nil {
		return nil, err
	}
	if !method.Active {
		return nil, invalid("El método de pago " + method.Name + " está desactivado.")
	}
	plan := &PaymentPlan{Mode: input.Mode, method: method, kind: "general", BalanceBefore: money(finance.balance), Units: []SelectedUnit{}}
	target := finance.balance
	if len(input.Selections) > 0 {
		selected, err := selectUnits(finance, input.Selections)
		if err != nil {
			return nil, err
		}
		selectionTotal := zero
		for _, unit := range selected {
			selectionTotal = selectionTotal.Add(unit.Remaining())
		}
		// Un pago por selección nunca supera el saldo: los abonos generales ya cubren parte de la cuenta.
		if selectionTotal.GreaterThan(finance.balance) {
			return nil, conflict("SELECTION_EXCEEDS_BALANCE", "La selección suma más que el saldo pendiente porque la cuenta tiene abonos generales. Cobra el saldo sin seleccionar productos.")
		}
		formatted := money(selectionTotal)
		plan.SelectionTotal = &formatted
		plan.selected = selected
		plan.kind = "selection"
		target = selectionTotal
	}
	plan.Target = money(target)
	amount := target
	if input.Mode == "partial" {
		amount, err = parseAmount(input.Amount, "abono")
		if err != nil {
			return nil, err
		}
		if amount.Sign() <= 0 {
			return nil, invalid("El abono debe ser mayor que cero.")
		}
		// No se permite sobrepago: el excedente nunca se interpreta como propina.
		if amount.GreaterThan(target) {
			return nil, invalid("El abono excede el importe pendiente. Para efectivo, captura el excedente como efectivo recibido.")
		}
	} else if input.Amount != "" {
		requested, err := parseAmount(input.Amount, "pago total")
		if err != nil {
			return nil, err
		}
		if !requested.Equal(target) {
			return nil, conflict("BALANCE_CHANGED", "El importe total cambió. Actualiza la cuenta para ver el saldo real.")
		}
	}
	tip, err := parseOptionalAmount(input.Tip, "propina")
	if err != nil {
		return nil, err
	}
	if tip.Sign() > 0 && !method.AllowsTip {
		return nil, invalid("El método " + method.Name + " no admite propina.")
	}
	plan.amount, plan.tip = amount, tip
	charged := amount.Add(tip)
	if method.IsCash {
		received := charged
		if strings.TrimSpace(input.Received) != "" {
			received, err = parseAmount(input.Received, "efectivo recibido")
			if err != nil {
				return nil, err
			}
			if received.LessThan(charged) {
				return nil, invalid("El efectivo recibido no cubre el importe y la propina.")
			}
		}
		plan.received = &received
		formatted := money(received)
		plan.Received = &formatted
		plan.change = received.Sub(charged)
	} else if strings.TrimSpace(input.Received) != "" {
		return nil, invalid("Solo el efectivo admite importe recibido y cambio.")
	}
	reference := strings.TrimSpace(input.Reference)
	switch method.ReferenceMode {
	case "none":
		if reference != "" {
			return nil, invalid("El método " + method.Name + " no admite referencia.")
		}
	case "required":
		if reference == "" {
			return nil, invalid("El método " + method.Name + " requiere la referencia de la operación confirmada.")
		}
	}
	if textLength(reference) > 120 {
		return nil, invalid("La referencia admite hasta 120 caracteres.")
	}
	plan.Amount = money(amount)
	plan.Tip = money(tip)
	plan.Change = money(plan.change)
	after := finance.balance.Sub(amount)
	plan.BalanceAfter = money(after)
	plan.Settles = after.Sign() == 0
	if plan.kind == "selection" {
		weights := make([]decimal.Decimal, len(plan.selected))
		for index, unit := range plan.selected {
			weights[index] = unit.Remaining()
		}
		plan.shares = allocate(amount, weights)
		for index, unit := range plan.selected {
			plan.Units = append(plan.Units, SelectedUnit{
				UnitID: unit.ID, VersionID: unit.VersionID, Remaining: money(unit.Remaining()), Amount: money(plan.shares[index]),
			})
		}
	}
	return plan, nil
}

// requireCollectPermissions valida, antes de cualquier escritura, los permisos de la modalidad.
func (handler *service) requireCollectPermissions(plan *PaymentPlan) error {
	actor := handler.request.Actor
	if plan.kind == "selection" {
		if err := actor.Require("payments.collect_selection"); err != nil {
			return err
		}
	}
	if plan.Mode == "partial" && plan.amount.LessThan(decimal.RequireFromString(plan.Target)) {
		if err := actor.Require("payments.collect_partial"); err != nil {
			return err
		}
	}
	if plan.tip.Sign() > 0 {
		return actor.Require("payments.tips.record")
	}
	return nil
}

func (handler *service) previewPayment(parts []string) (any, error) {
	var input PaymentInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	return handler.planPayment(finance, input)
}

func (handler *service) readAccountFinance(parts []string) (any, error) {
	return loadFinance(handler.ctx, handler.tx, parts[1])
}

func (handler *service) scope(finance *Finance) string {
	if finance.Mode == "takeaway" {
		return "takeaway"
	}
	return "restaurant"
}

// registerPayment guarda un pago, sus asignaciones y su movimiento de caja en la misma transacción.
// Los pagos mixtos son pagos sucesivos: cada uno conserva método, usuario, terminal, caja y turno.
func (handler *service) registerPayment(parts []string) (any, error) {
	var input PaymentInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	accountID := parts[1]
	if _, err := readAccountRecord(handler.ctx, handler.tx, accountID, true); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	// Una terminal con información obsoleta no sobrescribe el saldo real: debe actualizarse.
	if input.ExpectedBalance == "" || input.ExpectedBalance != finance.Balance {
		return nil, conflict("BALANCE_CHANGED", "El saldo de la cuenta cambió en otra terminal. Revisa el saldo actualizado antes de cobrar.")
	}
	plan, err := handler.planPayment(finance, input)
	if err != nil {
		return nil, err
	}
	if err := handler.requireCollectPermissions(plan); err != nil {
		return nil, err
	}
	if input.Deliver {
		if finance.Mode != "takeaway" {
			return nil, invalid("Solo los pedidos para llevar se marcan como entregados.")
		}
		if err := handler.request.Actor.Require("takeaway.deliver"); err != nil {
			return nil, err
		}
	}
	if err := handler.requirePrintPermission(input.Print); err != nil {
		return nil, err
	}
	shift, err := handler.usableShift()
	if err != nil {
		return nil, err
	}
	// A partir de aquí se escribe: toda autorización ya se verificó.
	paymentID := newID()
	var received *string
	if plan.received != nil {
		formatted := money(*plan.received)
		received = &formatted
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,
        kind,amount,tip,received,change_given,reference,actor_id,device_id,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::numeric,$12,$13,$14,$15,$16)`,
		paymentID, accountID, shift.ID, shift.RegisterID, plan.method.ID, plan.method.Name, plan.method.IsCash, plan.kind,
		plan.Amount, plan.Tip, received, plan.Change, strings.TrimSpace(input.Reference),
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
	if err != nil {
		return nil, err
	}
	for index, unit := range plan.selected {
		if plan.shares[index].Sign() <= 0 {
			continue
		}
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.payment_allocations(payment_id,unit_id,amount) VALUES ($1,$2,$3)",
			paymentID, unit.ID, money(plan.shares[index])); err != nil {
			return nil, err
		}
	}
	if err := handler.insertMovement(shift, movementInput{
		Kind: "payment", Method: plan.method, Sale: plan.amount, Tip: plan.tip, Received: plan.received, Change: plan.change,
		AccountID: accountID, PaymentID: &paymentID,
	}); err != nil {
		return nil, err
	}
	return handler.completeCollection(accountID, paymentID, input)
}

// completeCollection cierra la cuenta liquidada cuando corresponde, registra la entrega
// solicitada, encola el ticket y devuelve el resultado con el estado financiero actualizado.
func (handler *service) completeCollection(accountID, paymentID string, input PaymentInput) (any, error) {
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	delivered := false
	if input.Deliver && finance.Mode == "takeaway" && finance.Settled && finance.Delivery == nil {
		if err := handler.insertDelivery(accountID); err != nil {
			return nil, err
		}
		delivered = true
	}
	closed, err := handler.closeIfSettled(accountID)
	if err != nil {
		return nil, err
	}
	finance, err = loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	var payment *PaymentRecord
	for index := range finance.Payments {
		if finance.Payments[index].ID == paymentID {
			payment = &finance.Payments[index]
		}
	}
	printing, err := handler.printReceiptIfRequested(finance, payment, input.Print)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.recorded", handler.scope(finance), &accountID, nil); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "payments.payment_recorded", "account", &accountID, nil, map[string]any{
		"payment": payment, "balance": finance.Balance, "closed": closed, "delivered": delivered,
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"payment": payment, "finance": finance, "closed": closed, "delivered": delivered, "printing": printing,
	}, nil
}

// closeIfSettled cierra una cuenta liquidada: las de salón al quedar en saldo cero sin borrador,
// las de para llevar solo cuando además se entregaron. Cerrar libera la mesa.
func (handler *service) closeIfSettled(accountID string) (bool, error) {
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return false, err
	}
	if finance.State != "open" || !finance.Settled || finance.record.DraftQuantity > 0 {
		return false, nil
	}
	if finance.Mode == "takeaway" && finance.Delivery == nil {
		return false, nil
	}
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET state='closed',closed_at=now(),
        revision=revision+1,updated_at=now() WHERE id=$1 AND state='open'`, accountID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if err := events.Append(handler.ctx, handler.tx, "restaurant.account.closed", handler.scope(finance), &accountID, nil); err != nil {
		return false, err
	}
	return true, handler.audit(handler.ctx, handler.tx, "payments.account_closed", "account", &accountID, nil, map[string]string{"state": "closed"})
}

// reopenIfPending reabre una cuenta cerrada cuando una devolución o corrección deja saldo pendiente.
func (handler *service) reopenIfPending(accountID string) (bool, error) {
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return false, err
	}
	if finance.State != "closed" || finance.balance.Sign() <= 0 {
		return false, nil
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET state='open',closed_at=NULL,
        revision=revision+1,updated_at=now() WHERE id=$1 AND state='closed'`, accountID); err != nil {
		return false, err
	}
	if err := events.Append(handler.ctx, handler.tx, "restaurant.account.reopened", handler.scope(finance), &accountID, nil); err != nil {
		return false, err
	}
	return true, handler.audit(handler.ctx, handler.tx, "payments.account_reopened", "account", &accountID, nil, map[string]string{"state": "open"})
}

type movementInput struct {
	Kind      string
	Method    *Method
	Sale      decimal.Decimal
	Tip       decimal.Decimal
	Received  *decimal.Decimal
	Change    decimal.Decimal
	AccountID string
	PaymentID *string
	RefundID  *string
}

// insertMovement registra el efecto del cobro o devolución en el turno. El efectivo recibido y el
// cambio se guardan para el arqueo, pero el neto (venta + propina) es lo único que suma en caja.
func (handler *service) insertMovement(shift *Shift, input movementInput) error {
	received := zero
	if input.Received != nil {
		received = *input.Received
	}
	net := input.Sale.Add(input.Tip)
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_movements(id,shift_id,register_id,kind,method_id,method_name,is_cash,
        sale_amount,tip_amount,net_amount,cash_received,change_given,account_id,payment_id,refund_id,actor_id,device_id,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		newID(), shift.ID, shift.RegisterID, input.Kind, input.Method.ID, input.Method.Name, input.Method.IsCash,
		money(input.Sale), money(input.Tip), money(net), money(received), money(input.Change), input.AccountID,
		input.PaymentID, input.RefundID, handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
	return err
}
