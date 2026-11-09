package payments

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

type RefundInput struct {
	OperationID   string              `json:"operation_id"`
	Mode          string              `json:"mode"`
	PaymentID     string              `json:"payment_id"`
	Amount        string              `json:"amount"`
	Tip           string              `json:"tip"`
	Selections    []Selection         `json:"selections"`
	MethodID      string              `json:"method_id"`
	Reference     string              `json:"reference"`
	Reason        string              `json:"reason"`
	Authorization *AuthorizationInput `json:"authorization"`
	// ShiftID es el turno que mostraba la terminal al preparar la devolución (opcional). Un
	// reintento sobre un turno pausado o cerrado se rechaza en lugar de caer en otro turno.
	ShiftID string `json:"shift_id"`
}

type RefundUnit struct {
	UnitID string `json:"unit_id"`
	Amount string `json:"amount"`
}

// RefundPart es la porción de una devolución que revierte un pago original concreto.
type RefundPart struct {
	PaymentID string       `json:"payment_id"`
	Amount    string       `json:"amount"`
	Tip       string       `json:"tip"`
	Units     []RefundUnit `json:"units"`
	Tax       []TaxLine    `json:"tax"`

	amount decimal.Decimal
	tip    decimal.Decimal
	units  map[string]decimal.Decimal
}

type RefundPlan struct {
	Mode                  string       `json:"mode"`
	Amount                string       `json:"amount"`
	Tip                   string       `json:"tip"`
	Parts                 []RefundPart `json:"parts"`
	RequiresAuthorization bool         `json:"requires_authorization"`
	BalanceAfter          string       `json:"balance_after"`

	total decimal.Decimal
}

// paymentLedger describe lo que todavía puede devolverse de un pago: venta, propina y la parte
// asignada a cada unidad.
type paymentLedger struct {
	record  PaymentRecord
	amount  decimal.Decimal
	tip     decimal.Decimal
	general decimal.Decimal
	units   map[string]decimal.Decimal
	order   []string
}

func (handler *service) readLedgers(finance *Finance) (map[string]*paymentLedger, []string, error) {
	ledgers := map[string]*paymentLedger{}
	order := []string{}
	for _, payment := range finance.Payments {
		ledger := &paymentLedger{
			record: payment,
			amount: decimal.RequireFromString(payment.Amount).Sub(decimal.RequireFromString(payment.RefundedAmount)),
			tip:    decimal.RequireFromString(payment.Tip).Sub(decimal.RequireFromString(payment.RefundedTip)),
			units:  map[string]decimal.Decimal{},
		}
		ledgers[payment.ID] = ledger
		order = append(order, payment.ID)
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT a.payment_id,a.unit_id,(a.amount - coalesce((SELECT sum(r.amount)
            FROM app.payment_refund_allocations r JOIN app.payment_refunds f ON f.id=r.refund_id
            WHERE f.payment_id=a.payment_id AND r.unit_id=a.unit_id),0))::numeric(18,2)::text
        FROM app.payment_allocations a JOIN app.payments p ON p.id=a.payment_id WHERE p.account_id=$1
        ORDER BY a.payment_id,a.unit_id`, finance.AccountID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var paymentID, unitID, remaining string
		if err := rows.Scan(&paymentID, &unitID, &remaining); err != nil {
			return nil, nil, err
		}
		value := decimal.RequireFromString(remaining)
		if value.Sign() > 0 {
			ledger := ledgers[paymentID]
			ledger.units[unitID] = value
			ledger.order = append(ledger.order, unitID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for _, ledger := range ledgers {
		allocated := zero
		for _, value := range ledger.units {
			allocated = allocated.Add(value)
		}
		ledger.general = ledger.amount.Sub(allocated)
	}
	return ledgers, order, nil
}

// grossByRate devuelve el bruto vigente de la cuenta por tasa, usado para repartir una devolución
// general entre tasas con la proporción de la venta original.
func grossByRate(finance *Finance) map[int]decimal.Decimal {
	result := map[int]decimal.Decimal{}
	for _, line := range finance.Tax {
		result[line.Rate] = decimal.RequireFromString(line.Total)
	}
	return result
}

// refundTax calcula el IVA devuelto: las unidades con su propia tasa y la parte general repartida
// en proporción a las tasas de la cuenta. El resultado se congela en la devolución.
func refundTax(finance *Finance, part *RefundPart) []TaxLine {
	rates := map[string]int{}
	for _, unit := range finance.units {
		rates[unit.ID] = unit.TaxRate
	}
	gross := map[int]decimal.Decimal{}
	general := part.amount
	for unitID, value := range part.units {
		gross[rates[unitID]] = gross[rates[unitID]].Add(value)
		general = general.Sub(value)
	}
	if general.Sign() > 0 {
		base := grossByRate(finance)
		keys := []int{}
		weights := []decimal.Decimal{}
		for _, rate := range allowedTaxRates {
			if value, found := base[rate]; found {
				keys = append(keys, rate)
				weights = append(weights, value)
			}
		}
		if len(keys) == 0 {
			gross[16] = gross[16].Add(general)
		} else {
			for index, share := range allocate(general, weights) {
				gross[keys[index]] = gross[keys[index]].Add(share)
			}
		}
	}
	return taxBreakdown(gross)
}

func (handler *service) planRefund(finance *Finance, input RefundInput) (*RefundPlan, error) {
	ledgers, order, err := handler.readLedgers(finance)
	if err != nil {
		return nil, err
	}
	parts := []*RefundPart{}
	partFor := map[string]*RefundPart{}
	part := func(paymentID string) *RefundPart {
		if existing, found := partFor[paymentID]; found {
			return existing
		}
		created := &RefundPart{PaymentID: paymentID, units: map[string]decimal.Decimal{}}
		partFor[paymentID] = created
		parts = append(parts, created)
		return created
	}
	switch input.Mode {
	case "total":
		// Devolución total: todo lo neto de cada pago, incluidas sus propinas y asignaciones.
		for _, paymentID := range order {
			ledger := ledgers[paymentID]
			if ledger.amount.Sign() <= 0 && ledger.tip.Sign() <= 0 {
				continue
			}
			target := part(paymentID)
			target.amount = ledger.amount
			target.tip = ledger.tip
			for _, unitID := range ledger.order {
				target.units[unitID] = ledger.units[unitID]
			}
		}
	case "payment":
		if err := validateID(input.PaymentID); err != nil {
			return nil, err
		}
		ledger, found := ledgers[input.PaymentID]
		if !found {
			return nil, notFound("El pago no pertenece a esta cuenta.")
		}
		amount, err := parseOptionalAmount(input.Amount, "devolución")
		if err != nil {
			return nil, err
		}
		tip, err := parseOptionalAmount(input.Tip, "propina devuelta")
		if err != nil {
			return nil, err
		}
		// La parte asignada a productos se devuelve seleccionando esos productos.
		if amount.GreaterThan(ledger.general) {
			return nil, invalid("El importe excede lo devolvible de ese pago (" + formatPesos(ledger.general) + "). Lo asignado a productos se devuelve seleccionándolos.")
		}
		if tip.GreaterThan(ledger.tip) {
			return nil, invalid("La propina devuelta excede la propina neta de ese pago.")
		}
		if amount.Add(tip).Sign() <= 0 {
			return nil, invalid("Indica el importe o la propina que se devuelve de ese pago.")
		}
		target := part(input.PaymentID)
		target.amount = amount
		target.tip = tip
	case "items":
		selected, err := selectRefundUnits(finance, input.Selections)
		if err != nil {
			return nil, err
		}
		for _, unit := range selected {
			for _, paymentID := range order {
				ledger := ledgers[paymentID]
				value, found := ledger.units[unit.ID]
				if !found {
					continue
				}
				target := part(paymentID)
				target.amount = target.amount.Add(value)
				target.units[unit.ID] = value
			}
		}
	default:
		return nil, invalid("Indica si la devolución es total, de un pago o de productos.")
	}
	plan := &RefundPlan{Mode: input.Mode, Parts: []RefundPart{}}
	total, tips := zero, zero
	for _, item := range parts {
		if item.amount.Add(item.tip).Sign() <= 0 {
			continue
		}
		item.Amount = money(item.amount)
		item.Tip = money(item.tip)
		unitIDs := []string{}
		for unitID := range item.units {
			unitIDs = append(unitIDs, unitID)
		}
		sort.Strings(unitIDs)
		item.Units = []RefundUnit{}
		for _, unitID := range unitIDs {
			item.Units = append(item.Units, RefundUnit{UnitID: unitID, Amount: money(item.units[unitID])})
		}
		item.Tax = refundTax(finance, item)
		total = total.Add(item.amount)
		tips = tips.Add(item.tip)
		plan.Parts = append(plan.Parts, *item)
	}
	if len(plan.Parts) == 0 {
		return nil, conflict("NOTHING_TO_REFUND", "No hay importe devolvible con esa selección: ya se devolvió o no se pagó.")
	}
	plan.total = total.Add(tips)
	plan.Amount = money(total)
	plan.Tip = money(tips)
	plan.BalanceAfter = money(finance.balance.Add(total))
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	overThreshold := false
	if settings.RefundAuthorizationThreshold != nil {
		overThreshold = plan.total.GreaterThan(decimal.RequireFromString(*settings.RefundAuthorizationThreshold))
	}
	plan.RequiresAuthorization = !handler.request.Actor.Can("payments.refunds.execute") || overThreshold
	return plan, nil
}

// selectRefundUnits elige unidades de la cuenta con pago asignado, por ordinal.
func selectRefundUnits(finance *Finance, selections []Selection) ([]unitState, error) {
	if len(selections) == 0 || len(selections) > 100 {
		return nil, invalid("Selecciona entre 1 y 100 partidas para devolver.")
	}
	seen := map[string]bool{}
	selected := []unitState{}
	for _, selection := range selections {
		if err := validateID(selection.VersionID); err != nil {
			return nil, err
		}
		if seen[selection.VersionID] {
			return nil, invalid("Una partida está repetida en la devolución.")
		}
		seen[selection.VersionID] = true
		candidates := []unitState{}
		for _, unit := range finance.units {
			if unit.VersionID == selection.VersionID && unit.Paid.Sign() > 0 {
				candidates = append(candidates, unit)
			}
		}
		if selection.Quantity < 1 || selection.Quantity > len(candidates) {
			return nil, invalid("La cantidad excede las unidades pagadas de esa partida que aún pueden devolverse.")
		}
		selected = append(selected, candidates[:selection.Quantity]...)
	}
	return selected, nil
}

func (handler *service) previewRefund(parts []string) (any, error) {
	var input RefundInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	return handler.planRefund(finance, input)
}

// refundOutlet valida el método real por el que sale el dinero, que puede diferir del original.
func (handler *service) refundOutlet(methodID, reference string) (*Method, string, error) {
	if err := validateID(methodID); err != nil {
		return nil, "", invalid("Selecciona el método por el que se devuelve el dinero.")
	}
	method, err := handler.readMethod(methodID)
	if err != nil {
		return nil, "", err
	}
	if !method.Active || !method.AllowsRefund {
		return nil, "", invalid("El método " + method.Name + " no admite devoluciones.")
	}
	reference = strings.TrimSpace(reference)
	if method.ReferenceMode == "none" && reference != "" {
		return nil, "", invalid("El método " + method.Name + " no admite referencia.")
	}
	if method.ReferenceMode == "required" && reference == "" {
		return nil, "", invalid("Registra la referencia de la devolución confirmada externamente.")
	}
	if textLength(reference) > 120 {
		return nil, "", invalid("La referencia admite hasta 120 caracteres.")
	}
	return method, reference, nil
}

// refund registra devoluciones compensatorias sin modificar los pagos originales. El dinero sale
// del turno abierto de la terminal y, si la cuenta estaba cerrada y queda saldo, se reabre.
func (handler *service) refund(parts []string) (any, error) {
	var input RefundInput
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
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo de la devolución (5–500 caracteres).")
	}
	plan, err := handler.planRefund(finance, input)
	if err != nil {
		return nil, err
	}
	method, reference, err := handler.refundOutlet(input.MethodID, input.Reference)
	if err != nil {
		return nil, err
	}
	// El turno se verifica antes que la autorización: con la caja en arqueo o cerrada no se
	// consume un intento de la contraseña del supervisor.
	shift, err := handler.usableShiftFor(input.ShiftID)
	if err != nil {
		return nil, err
	}
	approved, err := handler.approve("refund", !plan.RequiresAuthorization, input.Authorization,
		"Esta devolución requiere la autorización de un supervisor.",
		func(supervisor *Supervisor) (bool, error) {
			return supervisor.Can("payments.refunds.authorize"), nil
		})
	if err != nil {
		return nil, err
	}
	// A partir de aquí se escribe: permisos, autorización y turno ya se verificaron.
	refundIDs := []string{}
	for index := range plan.Parts {
		item := &plan.Parts[index]
		id := newID()
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_refunds(id,payment_id,account_id,kind,shift_id,register_id,method_id,
            method_name,is_cash,amount,tip,reason,reference,tax_breakdown,actor_id,device_id,authorized_by,operation_id)
            VALUES ($1,$2,$3,'refund',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			id, item.PaymentID, accountID, shift.ID, shift.RegisterID, method.ID, method.Name, method.IsCash, item.Amount, item.Tip,
			reason, reference, jsonValue(item.Tax), handler.request.Actor.UserID, handler.request.Actor.DeviceID, approved.authorizedBy(), handler.operationID)
		if err != nil {
			return nil, err
		}
		for _, unit := range item.Units {
			if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.payment_refund_allocations(refund_id,unit_id,amount) VALUES ($1,$2,$3)",
				id, unit.UnitID, unit.Amount); err != nil {
				return nil, err
			}
		}
		sale := decimal.RequireFromString(item.Amount)
		tip := decimal.RequireFromString(item.Tip)
		if err := handler.insertMovement(shift, movementInput{
			Kind: "refund", Method: method, Sale: sale.Neg(), Tip: tip.Neg(), AccountID: accountID, RefundID: &id,
		}); err != nil {
			return nil, err
		}
		refundIDs = append(refundIDs, id)
	}
	if err := handler.recordApproval(approved, accountID, plan.total); err != nil {
		return nil, err
	}
	// Una devolución en efectivo saca dinero del cajón: con el cajón activado se abre una vez por
	// devolución, aunque abarque varios pagos. Un fallo del comando no revierte la devolución.
	drawer := DrawerOutcome{Requested: false}
	if method.IsCash {
		drawer = handler.openDrawerAfter(shift, drawerRefund)
	}
	if err := handler.cashMovementEvent(shift); err != nil {
		return nil, err
	}
	reopened, err := handler.reopenIfPending(accountID)
	if err != nil {
		return nil, err
	}
	saved, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.refunded", handler.scope(saved), &accountID, nil); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "payments.refund_recorded", "account", &accountID, nil, map[string]any{
		"refund_ids": refundIDs, "plan": plan, "method": method.Name, "reason": reason,
	}); err != nil {
		return nil, err
	}
	return map[string]any{"refund_ids": refundIDs, "plan": plan, "finance": saved, "reopened": reopened, "drawer": drawer}, nil
}

// correct corrige el método de un pago capturado por error mediante un reverso compensatorio y un
// pago nuevo con el método correcto. Solo procede mientras el turno original permanece abierto.
func (handler *service) correct(parts []string) (any, error) {
	var input struct {
		OperationID   string              `json:"operation_id"`
		PaymentID     string              `json:"payment_id"`
		MethodID      string              `json:"method_id"`
		Reference     string              `json:"reference"`
		Reason        string              `json:"reason"`
		Authorization *AuthorizationInput `json:"authorization"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.PaymentID); err != nil {
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
	var original *PaymentRecord
	for index := range finance.Payments {
		if finance.Payments[index].ID == input.PaymentID {
			original = &finance.Payments[index]
		}
	}
	if original == nil {
		return nil, notFound("El pago no pertenece a esta cuenta.")
	}
	if original.Corrected || original.RefundedAmount != "0.00" || original.RefundedTip != "0.00" {
		return nil, conflict("PAYMENT_NOT_CORRECTABLE", "El pago ya fue corregido o tiene devoluciones; registra una devolución en su lugar.")
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo de la corrección (5–500 caracteres).")
	}
	if input.MethodID == original.MethodID {
		return nil, invalid("Selecciona un método distinto del registrado.")
	}
	if err := validateID(input.MethodID); err != nil {
		return nil, invalid("Selecciona el método correcto.")
	}
	replacement, err := handler.readMethod(input.MethodID)
	if err != nil {
		return nil, err
	}
	if !replacement.Active {
		return nil, invalid("El método " + replacement.Name + " está desactivado.")
	}
	reference := strings.TrimSpace(input.Reference)
	if replacement.ReferenceMode == "none" && reference != "" {
		return nil, invalid("El método " + replacement.Name + " no admite referencia.")
	}
	if replacement.ReferenceMode == "required" && reference == "" {
		return nil, invalid("El método " + replacement.Name + " requiere la referencia de la operación confirmada.")
	}
	if decimal.RequireFromString(original.Tip).Sign() > 0 && !replacement.AllowsTip {
		return nil, invalid("El método " + replacement.Name + " no admite la propina registrada en el pago.")
	}
	shift, err := handler.readShift("s.id=$1", original.ShiftID)
	if err != nil {
		return nil, err
	}
	// Invariante del modelo: una vez cerrado el turno, sus movimientos no se modifican; durante el
	// arqueo de cierre la caja está pausada y tampoco admite correcciones.
	if shift == nil || shift.State == shiftClosed {
		return nil, conflict("SHIFT_CLOSED", "El turno del pago ya está cerrado. Las regularizaciones posteriores usan un procedimiento separado.")
	}
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	approved, err := handler.approve("correction", handler.request.Actor.Can("payments.corrections"), input.Authorization,
		"Corregir un pago requiere la autorización de un supervisor.",
		func(supervisor *Supervisor) (bool, error) {
			return supervisor.Can("payments.corrections"), nil
		})
	if err != nil {
		return nil, err
	}
	previousMethod, err := handler.readMethod(original.MethodID)
	if err != nil {
		return nil, err
	}
	amount := decimal.RequireFromString(original.Amount)
	tip := decimal.RequireFromString(original.Tip)
	ledgers, _, err := handler.readLedgers(finance)
	if err != nil {
		return nil, err
	}
	ledger := ledgers[original.ID]
	reversalID := newID()
	reversalTax := refundTax(finance, &RefundPart{amount: amount, units: ledger.units})
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_refunds(id,payment_id,account_id,kind,shift_id,register_id,method_id,
        method_name,is_cash,amount,tip,reason,reference,tax_breakdown,actor_id,device_id,authorized_by,operation_id)
        VALUES ($1,$2,$3,'correction',$4,$5,$6,$7,$8,$9,$10,$11,'',$12,$13,$14,$15,$16)`,
		reversalID, original.ID, accountID, shift.ID, shift.RegisterID, previousMethod.ID, original.MethodName, original.IsCash,
		original.Amount, original.Tip, reason, jsonValue(reversalTax), handler.request.Actor.UserID, handler.request.Actor.DeviceID,
		approved.authorizedBy(), handler.operationID)
	if err != nil {
		return nil, err
	}
	for _, unitID := range ledger.order {
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.payment_refund_allocations(refund_id,unit_id,amount) VALUES ($1,$2,$3)",
			reversalID, unitID, money(ledger.units[unitID])); err != nil {
			return nil, err
		}
	}
	// El reverso invierte lo que el pago original movió físicamente por moneda (pesos y dólares
	// recibidos, cambio entregado), de modo que el cajón queda como antes del pago erróneo.
	var reversal *cashPhysical
	if effect := original.physicalEffect(); effect != nil {
		inverted := effect.reversed()
		reversal = &inverted
	}
	if err := handler.insertMovement(shift, movementInput{
		Kind: "correction_reversal", Method: previousMethod, Sale: amount.Neg(), Tip: tip.Neg(), AccountID: accountID, RefundID: &reversalID,
		Physical: reversal,
	}); err != nil {
		return nil, err
	}
	paymentID := newID()
	var received *string
	var receivedValue *decimal.Decimal
	change := zero
	currency := paymentCurrency{}
	var replacementPhysical *cashPhysical
	if replacement.IsCash {
		charged := amount.Add(tip)
		formatted := money(charged)
		received = &formatted
		receivedValue = &charged
		// Entre dos métodos de efectivo, un pago con dólares conserva lo que físicamente se recibió
		// y entregó: el dinero sigue en el cajón aunque cambie el método. Un efectivo solo en pesos
		// conserva el comportamiento de la etapa 6 (recibido igual al importe, sin cambio).
		if original.IsCash && original.ReceivedUSD != nil {
			received = original.Received
			value := decimal.RequireFromString(*original.Received)
			receivedValue = &value
			change = decimal.RequireFromString(original.Change)
			currency = paymentCurrency{
				receivedMXN: original.ReceivedMXN, receivedUSD: original.ReceivedUSD, rate: original.USDRate,
				changeCurrency: original.ChangeCurrency, changeUSD: original.ChangeUSD,
			}
			replacementPhysical = original.physicalEffect()
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.payments(id,account_id,shift_id,register_id,method_id,method_name,is_cash,
        kind,amount,tip,received,change_given,reference,corrects_payment_id,actor_id,device_id,operation_id,
        received_mxn,received_usd,usd_rate,change_currency,change_usd)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::numeric,$12,$13,$14,$15,$16,$17,$18::numeric,$19::numeric,$20::numeric,$21,$22::numeric)`,
		paymentID, accountID, shift.ID, shift.RegisterID, replacement.ID, replacement.Name, replacement.IsCash, original.Kind,
		original.Amount, original.Tip, received, money(change), reference, original.ID, handler.request.Actor.UserID,
		handler.request.Actor.DeviceID, handler.operationID,
		currency.receivedMXN, currency.receivedUSD, currency.rate, currency.changeCurrency, currency.changeUSD)
	if err != nil {
		return nil, err
	}
	for _, unitID := range ledger.order {
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.payment_allocations(payment_id,unit_id,amount) VALUES ($1,$2,$3)",
			paymentID, unitID, money(ledger.units[unitID])); err != nil {
			return nil, err
		}
	}
	if err := handler.insertMovement(shift, movementInput{
		Kind: "payment", Method: replacement, Sale: amount, Tip: tip, Received: receivedValue, Change: change, AccountID: accountID,
		PaymentID: &paymentID, Physical: replacementPhysical,
	}); err != nil {
		return nil, err
	}
	if err := handler.recordApproval(approved, accountID, amount.Add(tip)); err != nil {
		return nil, err
	}
	// La corrección del método no mueve dinero físico nuevo: no abre el cajón, pero sí avisa a Caja.
	if err := handler.cashMovementEvent(shift); err != nil {
		return nil, err
	}
	saved, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.corrected", handler.scope(saved), &accountID, nil); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "payments.payment_corrected", "account", &accountID,
		map[string]string{"payment_id": original.ID, "method": original.MethodName},
		map[string]string{"payment_id": paymentID, "method": replacement.Name, "reason": reason}); err != nil {
		return nil, err
	}
	return map[string]any{"reversal_id": reversalID, "payment_id": paymentID, "finance": saved}, nil
}
