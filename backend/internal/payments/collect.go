package payments

import (
	"errors"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
)

type Selection struct {
	VersionID string `json:"version_id"`
	Quantity  int    `json:"quantity"`
}

type PaymentInput struct {
	OperationID string `json:"operation_id"`
	MethodID    string `json:"method_id"`
	Mode        string `json:"mode"`
	Amount      string `json:"amount"`
	// Received son los pesos recibidos en efectivo. Sin dólares, vacío significa importe exacto;
	// con dólares, vacío significa cero pesos.
	Received string `json:"received"`
	// ReceivedUSD son los dólares recibidos y ChangeCurrency la moneda en que sale el cambio
	// (MXN predeterminado). Solo se admiten con la función usd_cash activa y método de efectivo.
	ReceivedUSD     string      `json:"received_usd"`
	ChangeCurrency  string      `json:"change_currency"`
	Tip             string      `json:"tip"`
	Reference       string      `json:"reference"`
	Selections      []Selection `json:"selections"`
	ExpectedBalance string      `json:"expected_balance"`
	Print           *bool       `json:"print"`
	Deliver         bool        `json:"deliver"`
	// ShiftID es el turno que mostraba la terminal al preparar el cobro (opcional). Forma parte de
	// la huella: un reintento incierto sobre un turno pausado o cerrado se rechaza con un error
	// claro en lugar de registrarse en otro turno.
	ShiftID string `json:"shift_id"`
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
	// USD solo aparece con la función usd_cash activa, método de efectivo y turno con tipo de cambio.
	USD *USDPlan `json:"usd,omitempty"`

	method   *Method
	amount   decimal.Decimal
	tip      decimal.Decimal
	received *decimal.Decimal
	change   decimal.Decimal
	shares   []decimal.Decimal
	selected []unitState
	kind     string
	// dollars conserva la captura en dólares validada; settlement, su liquidación con el tipo de
	// cambio del turno cuando intervienen dólares.
	dollars       *dollarCapture
	settlement    *cashSettlement
	rate          *decimal.Decimal
	inputReceived string
}

// USDPlan expresa en pesos y en dólares el cobro en efectivo. Todas las cifras las calcula Go con
// el tipo de cambio fijo del turno; la interfaz solo las muestra.
type USDPlan struct {
	Rate           string `json:"rate"`
	BalanceUSD     string `json:"balance_usd"`
	ChargedUSD     string `json:"charged_usd"`
	ReceivedMXN    string `json:"received_mxn"`
	ReceivedUSD    string `json:"received_usd"`
	ReceivedUSDMXN string `json:"received_usd_mxn"`
	ReceivedTotal  string `json:"received_total"`
	ChangeMXN      string `json:"change_mxn"`
	ChangeUSD      string `json:"change_usd"`
	ChangeCurrency string `json:"change_currency"`
	Covered        bool   `json:"covered"`
	MissingMXN     string `json:"missing_mxn"`
	MissingUSD     string `json:"missing_usd"`
}

// dollarCapture es la parte en dólares de una captura de efectivo.
type dollarCapture struct {
	receivedUSD    decimal.Decimal
	changeCurrency string
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
	dollars, err := handler.parseDollars(input, method)
	if err != nil {
		return nil, err
	}
	plan.dollars = dollars
	plan.inputReceived = strings.TrimSpace(input.Received)
	if method.IsCash && dollars != nil {
		// Con dólares, lo recibido y el cambio se liquidan con el tipo de cambio del turno
		// (applyDollars); aquí solo se valida la forma de los pesos recibidos.
		if strings.TrimSpace(input.Received) != "" {
			if _, err := parseAmount(input.Received, "pesos recibidos"); err != nil {
				return nil, err
			}
		}
	} else if method.IsCash {
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
	plan, err := handler.planPayment(finance, input)
	if err != nil {
		return nil, err
	}
	if err := handler.previewDollars(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// previewDollars agrega a la vista previa de un cobro en efectivo sus cifras en dólares cuando la
// función está activa y el turno tiene tipo de cambio. Sin dólares capturados, un turno no
// utilizable no impide la vista previa en pesos; con dólares, su error se informa.
func (handler *service) previewDollars(plan *PaymentPlan) error {
	if !plan.method.IsCash {
		return nil
	}
	if plan.dollars == nil {
		enabled, err := features.Enabled(handler.ctx, handler.tx, features.USDCash)
		if err != nil || !enabled {
			return err
		}
	}
	shift, err := handler.usableShift()
	if err != nil {
		var problem *Error
		if plan.dollars == nil && errors.As(err, &problem) {
			return nil
		}
		return err
	}
	return handler.applyDollars(plan, shift, true)
}

// parseDollars valida la parte en dólares de la captura. Cualquier dato en dólares con la función
// desactivada se rechaza (409 FEATURE_DISABLED) aunque se llame a la API directamente; solo el
// efectivo admite dólares.
func (handler *service) parseDollars(input PaymentInput, method *Method) (*dollarCapture, error) {
	currency := strings.TrimSpace(input.ChangeCurrency)
	if currency != "" && currency != currencyMXN && currency != currencyUSD {
		return nil, invalid("La moneda del cambio debe ser pesos (MXN) o dólares (USD).")
	}
	receivedText := strings.TrimSpace(input.ReceivedUSD)
	if receivedText == "" && currency != currencyUSD {
		return nil, nil
	}
	if err := handler.requireUSDCash(); err != nil {
		return nil, err
	}
	if !method.IsCash {
		return nil, invalid("Solo el efectivo admite dólares. " + method.Name + " se registra en pesos.")
	}
	received := zero
	if receivedText != "" {
		value, err := parseAmount(receivedText, "dólares recibidos")
		if err != nil {
			return nil, err
		}
		received = value
	}
	if currency == "" {
		currency = currencyMXN
	}
	return &dollarCapture{receivedUSD: received, changeCurrency: currency}, nil
}

// applyDollars liquida el efectivo con el tipo de cambio fijo del turno. Con dólares capturados,
// un turno sin tipo de cambio no los acepta (409 USD_RATE_REQUIRED) y un efectivo insuficiente se
// rechaza al registrar; en la vista previa (preview) se informa lo que falta en ambas monedas.
func (handler *service) applyDollars(plan *PaymentPlan, shift *Shift, preview bool) error {
	if shift.USDRate == nil {
		if plan.dollars != nil {
			return conflict("USD_RATE_REQUIRED", "El turno no tiene tipo de cambio: no acepta dólares hasta que alguien con permiso lo capture.")
		}
		return nil
	}
	rate := decimal.RequireFromString(*shift.USDRate)
	charged := plan.amount.Add(plan.tip)
	usd := &USDPlan{
		Rate:       money(rate),
		BalanceUSD: money(mxnToUSD(decimal.RequireFromString(plan.BalanceBefore), rate)),
		ChargedUSD: money(mxnToUSD(charged, rate)),
		MissingMXN: "0.00",
		MissingUSD: "0.00",
	}
	if plan.dollars == nil {
		// Solo pesos: las cifras en dólares son informativas y el pago se registra como en la etapa 6.
		usd.ReceivedMXN = money(*plan.received)
		usd.ReceivedUSD = "0.00"
		usd.ReceivedUSDMXN = "0.00"
		usd.ReceivedTotal = money(*plan.received)
		usd.ChangeMXN = money(plan.change)
		usd.ChangeUSD = money(mxnToUSD(plan.change, rate))
		usd.ChangeCurrency = currencyMXN
		usd.Covered = true
		plan.USD = usd
		return nil
	}
	pesos := zero
	if strings.TrimSpace(plan.inputReceived) != "" {
		pesos = decimal.RequireFromString(plan.inputReceived)
	}
	settlement := settleCashWithUSD(charged, pesos, plan.dollars.receivedUSD, rate, plan.dollars.changeCurrency)
	if !settlement.covered && !preview {
		return invalid("El efectivo recibido no cubre el importe y la propina: faltan " + formatPesos(settlement.missing) +
			" (US$" + money(usdToCover(settlement.missing, rate)) + ").")
	}
	usd.ReceivedMXN = money(settlement.receivedMXN)
	usd.ReceivedUSD = money(settlement.receivedUSD)
	usd.ReceivedUSDMXN = money(settlement.receivedUSDMXN)
	usd.ReceivedTotal = money(settlement.receivedTotal)
	usd.ChangeMXN = money(settlement.change)
	usd.ChangeUSD = money(settlement.changeUSD)
	usd.ChangeCurrency = settlement.changeCurrency
	usd.Covered = settlement.covered
	usd.MissingMXN = money(settlement.missing)
	usd.MissingUSD = money(usdToCover(settlement.missing, rate))
	received := settlement.receivedTotal
	plan.received = &received
	formatted := money(received)
	plan.Received = &formatted
	plan.change = settlement.change
	plan.Change = money(settlement.change)
	plan.settlement = &settlement
	plan.rate = &rate
	plan.USD = usd
	return nil
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
	shift, err := handler.usableShiftFor(input.ShiftID)
	if err != nil {
		return nil, err
	}
	if plan.dollars != nil {
		if err := handler.applyDollars(plan, shift, false); err != nil {
			return nil, err
		}
	}
	// A partir de aquí se escribe: toda autorización y validación ya se verificó.
	paymentID := newID()
	var received *string
	if plan.received != nil {
		formatted := money(*plan.received)
		received = &formatted
	}
	currency := paymentCurrency{}
	var physical *cashPhysical
	if plan.settlement != nil && plan.settlement.usesDollars() {
		currency = currencyColumns(*plan.settlement, *plan.rate)
		effect := plan.settlement.physical(*plan.rate)
		physical = &effect
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,
        kind,amount,tip,received,change_given,reference,actor_id,device_id,operation_id,
        received_mxn,received_usd,usd_rate,change_currency,change_usd)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::numeric,$12,$13,$14,$15,$16,$17::numeric,$18::numeric,$19::numeric,$20,$21::numeric)`,
		paymentID, accountID, shift.ID, shift.RegisterID, plan.method.ID, plan.method.Name, plan.method.IsCash, plan.kind,
		plan.Amount, plan.Tip, received, plan.Change, strings.TrimSpace(input.Reference),
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID,
		currency.receivedMXN, currency.receivedUSD, currency.rate, currency.changeCurrency, currency.changeUSD)
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
		AccountID: accountID, PaymentID: &paymentID, Physical: physical,
	}); err != nil {
		return nil, err
	}
	// El cajón se abre solo con efectivo físico; un pago electrónico nunca lo abre (§18).
	drawer := DrawerOutcome{Requested: false}
	if plan.method.IsCash {
		drawer = handler.openDrawerAfter(shift, drawerPayment)
	}
	if err := handler.cashMovementEvent(shift); err != nil {
		return nil, err
	}
	result, err := handler.completeCollection(accountID, paymentID, input)
	if err != nil {
		return nil, err
	}
	result["drawer"] = drawer
	return result, nil
}

// cashMovementEvent avisa en el ámbito de caja que el turno registró un cobro, una devolución o
// una corrección de pago, para que Caja y el cobro de otras terminales se actualicen sin esperar
// el sondeo. Solo lleva el identificador del turno, nunca importes.
func (handler *service) cashMovementEvent(shift *Shift) error {
	return events.Append(handler.ctx, handler.tx, "cash.movement.recorded", cashScope, &shift.ID, nil)
}

// paymentCurrency son las columnas por moneda de un pago; todas nil en un pago solo en pesos.
type paymentCurrency struct {
	receivedMXN    *string
	receivedUSD    *string
	rate           *string
	changeCurrency *string
	changeUSD      *string
}

func optionalMoney(value decimal.Decimal) *string {
	formatted := money(value)
	return &formatted
}

func currencyColumns(settlement cashSettlement, rate decimal.Decimal) paymentCurrency {
	currency := settlement.changeCurrency
	columns := paymentCurrency{
		receivedMXN:    optionalMoney(settlement.receivedMXN),
		receivedUSD:    optionalMoney(settlement.receivedUSD),
		rate:           optionalMoney(rate),
		changeCurrency: &currency,
	}
	if currency == currencyUSD {
		columns.changeUSD = optionalMoney(settlement.changeUSD)
	}
	return columns
}

// completeCollection cierra la cuenta liquidada cuando corresponde, registra la entrega
// solicitada, encola el ticket y devuelve el resultado con el estado financiero actualizado.
func (handler *service) completeCollection(accountID, paymentID string, input PaymentInput) (map[string]any, error) {
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
	// Physical es el efecto por moneda cuando intervienen dólares o se invierte otro movimiento;
	// si es nil, un movimiento de efectivo se registra solo en pesos.
	Physical *cashPhysical
}

// pesosOnly deduce el efecto físico de un efectivo solo en pesos: entra lo recibido y sale el
// cambio; sin importe recibido, el neto positivo entra y el negativo (devoluciones) sale.
func pesosOnly(input movementInput, net decimal.Decimal) cashPhysical {
	if input.Received != nil {
		return cashPhysical{mxnIn: *input.Received, mxnOut: input.Change}
	}
	if net.Sign() >= 0 {
		return cashPhysical{mxnIn: net}
	}
	return cashPhysical{mxnOut: net.Neg()}
}

// insertMovement registra el efecto del cobro o devolución en el turno. El efectivo recibido y el
// cambio se guardan para el arqueo, pero el neto (venta + propina) es lo único que suma en caja.
// Los movimientos de efectivo registran además lo que entra y sale por moneda (pesos y dólares
// físicos) para que el corte de la etapa 7 cuente cada moneda por separado.
func (handler *service) insertMovement(shift *Shift, input movementInput) error {
	received := zero
	if input.Received != nil {
		received = *input.Received
	}
	net := input.Sale.Add(input.Tip)
	var mxnIn, mxnOut, usdIn, usdOut, rate *string
	if input.Method.IsCash {
		physical := pesosOnly(input, net)
		if input.Physical != nil {
			physical = *input.Physical
		}
		mxnIn, mxnOut = optionalMoney(physical.mxnIn), optionalMoney(physical.mxnOut)
		usdIn, usdOut = optionalMoney(physical.usdIn), optionalMoney(physical.usdOut)
		if physical.rate != nil {
			rate = optionalMoney(*physical.rate)
		}
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_movements(id,shift_id,register_id,kind,method_id,method_name,is_cash,
        sale_amount,tip_amount,net_amount,cash_received,change_given,account_id,payment_id,refund_id,actor_id,device_id,operation_id,
        mxn_in,mxn_out,usd_in,usd_out,usd_rate)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::numeric,$20::numeric,$21::numeric,$22::numeric,$23::numeric)`,
		newID(), shift.ID, shift.RegisterID, input.Kind, input.Method.ID, input.Method.Name, input.Method.IsCash,
		money(input.Sale), money(input.Tip), money(net), money(received), money(input.Change), input.AccountID,
		input.PaymentID, input.RefundID, handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID,
		mxnIn, mxnOut, usdIn, usdOut, rate)
	return err
}
