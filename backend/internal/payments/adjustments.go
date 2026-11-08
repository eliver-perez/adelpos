package payments

import (
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

type DiscountInput struct {
	OperationID     string              `json:"operation_id"`
	Scope           string              `json:"scope"`
	Method          string              `json:"method"`
	Value           string              `json:"value"`
	Courtesy        bool                `json:"courtesy"`
	Concept         string              `json:"concept"`
	Reason          string              `json:"reason"`
	Selections      []Selection         `json:"selections"`
	ExpectedBalance string              `json:"expected_balance"`
	Authorization   *AuthorizationInput `json:"authorization"`
}

type DiscountUnit struct {
	UnitID string `json:"unit_id"`
	Amount string `json:"amount"`
}

// DiscountPlan es el cálculo de Go para un descuento: base, importe, porcentaje efectivo y si el
// límite del usuario exige autorización.
type DiscountPlan struct {
	Scope                 string         `json:"scope"`
	Method                string         `json:"method"`
	Value                 string         `json:"value"`
	Base                  string         `json:"base"`
	Amount                string         `json:"amount"`
	Percent               string         `json:"percent"`
	Courtesy              bool           `json:"courtesy"`
	ActorLimit            string         `json:"actor_limit"`
	RequiresAuthorization bool           `json:"requires_authorization"`
	BalanceAfter          string         `json:"balance_after"`
	Settles               bool           `json:"settles"`
	Units                 []DiscountUnit `json:"units"`

	amount   decimal.Decimal
	percent  decimal.Decimal
	value    decimal.Decimal
	selected []unitState
	shares   []decimal.Decimal
}

// selectDiscountUnits elige unidades sin pago ni descuento manual vigente.
func selectDiscountUnits(finance *Finance, selections []Selection) ([]unitState, error) {
	if len(selections) == 0 || len(selections) > 100 {
		return nil, invalid("Selecciona entre 1 y 100 partidas para el descuento por partida.")
	}
	seen := map[string]bool{}
	selected := []unitState{}
	for _, selection := range selections {
		if err := validateID(selection.VersionID); err != nil {
			return nil, err
		}
		if seen[selection.VersionID] {
			return nil, invalid("Una partida está repetida en el descuento.")
		}
		seen[selection.VersionID] = true
		candidates := []unitState{}
		for _, unit := range finance.units {
			if unit.VersionID == selection.VersionID && unit.Paid.IsZero() && unit.Discount.IsZero() {
				candidates = append(candidates, unit)
			}
		}
		if selection.Quantity < 1 || selection.Quantity > len(candidates) {
			return nil, conflict("UNIT_NOT_DISCOUNTABLE", "La cantidad excede las unidades sin pago ni descuento de esa partida. Las unidades pagadas requieren primero su devolución.")
		}
		selected = append(selected, candidates[:selection.Quantity]...)
	}
	return selected, nil
}

func (handler *service) planDiscount(finance *Finance, input DiscountInput) (*DiscountPlan, error) {
	if finance.State != "open" {
		return nil, conflict("ACCOUNT_NOT_OPEN", "La cuenta no está abierta; no admite descuentos.")
	}
	if input.Scope != "account" && input.Scope != "items" {
		return nil, invalid("Indica si el descuento es general o por partida.")
	}
	if input.Method != "percent" && input.Method != "amount" {
		return nil, invalid("Indica si el descuento es por porcentaje o por importe.")
	}
	value, err := parseAmount(input.Value, "descuento")
	if err != nil {
		return nil, err
	}
	if value.Sign() <= 0 {
		return nil, invalid("El descuento debe ser mayor que cero.")
	}
	if input.Method == "percent" && value.GreaterThan(hundred) {
		return nil, invalid("Un descuento no puede superar el 100 %.")
	}
	// La cortesía es un descuento total autorizado: liquida sin inventar un pago de $0.
	if input.Courtesy && (input.Method != "percent" || !value.Equal(hundred)) {
		return nil, invalid("Una cortesía corresponde a un descuento del 100 %.")
	}
	plan := &DiscountPlan{Scope: input.Scope, Method: input.Method, Value: money(value), Courtesy: input.Courtesy, value: value, Units: []DiscountUnit{}}
	base := finance.total
	if input.Scope == "items" {
		selected, err := selectDiscountUnits(finance, input.Selections)
		if err != nil {
			return nil, err
		}
		base = zero
		for _, unit := range selected {
			base = base.Add(unit.Due())
		}
		plan.selected = selected
	} else if len(input.Selections) > 0 {
		return nil, invalid("Un descuento general no selecciona partidas.")
	}
	if base.Sign() <= 0 {
		return nil, invalid("No hay importe sobre el cual aplicar el descuento.")
	}
	amount := value
	if input.Method == "percent" {
		amount = roundMoney(base.Mul(value).Div(hundred))
	}
	if amount.GreaterThan(base) {
		return nil, invalid("El descuento excede el importe de lo seleccionado.")
	}
	if amount.Sign() <= 0 {
		return nil, invalid("El descuento resulta menor que un centavo.")
	}
	// Ningún descuento puede dejar la cuenta con un total menor que lo ya pagado.
	if amount.GreaterThan(finance.balance) {
		return nil, conflict("ACCOUNT_BELOW_PAYMENTS", "El descuento excede el saldo pendiente: la cuenta ya tiene pagos. Registra primero la devolución correspondiente.")
	}
	plan.amount = amount
	plan.percent = percentOf(amount, base)
	plan.Base = money(base)
	plan.Amount = money(amount)
	plan.Percent = money(plan.percent)
	after := finance.balance.Sub(amount)
	plan.BalanceAfter = money(after)
	plan.Settles = after.IsZero()
	if input.Scope == "items" {
		weights := make([]decimal.Decimal, len(plan.selected))
		for index, unit := range plan.selected {
			weights[index] = unit.Due()
		}
		plan.shares = allocate(amount, weights)
		for index, unit := range plan.selected {
			plan.Units = append(plan.Units, DiscountUnit{UnitID: unit.ID, Amount: money(plan.shares[index])})
		}
	}
	limit, err := handler.discountLimit(handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	plan.ActorLimit = money(limit)
	actor := handler.request.Actor
	if input.Courtesy {
		plan.RequiresAuthorization = !actor.Can("payments.courtesy")
	} else {
		plan.RequiresAuthorization = !actor.Can("payments.discounts.apply") || plan.percent.GreaterThan(limit)
	}
	return plan, nil
}

func (handler *service) previewDiscount(parts []string) (any, error) {
	var input DiscountInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	return handler.planDiscount(finance, input)
}

func cleanConcept(concept, fallback string) (string, error) {
	concept = strings.TrimSpace(concept)
	if concept == "" {
		concept = fallback
	}
	if textLength(concept) > 120 {
		return "", invalid("El concepto admite hasta 120 caracteres.")
	}
	return concept, nil
}

// applyDiscount registra un descuento manual o una cortesía. Si supera el límite del usuario,
// exige la autorización puntual de un supervisor con permiso y límite suficientes.
func (handler *service) applyDiscount(parts []string) (any, error) {
	var input DiscountInput
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
	if input.ExpectedBalance != finance.Balance {
		return nil, conflict("BALANCE_CHANGED", "El saldo de la cuenta cambió en otra terminal. Revisa el saldo actualizado.")
	}
	plan, err := handler.planDiscount(finance, input)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo del descuento (5–500 caracteres).")
	}
	fallback := "Descuento manual"
	scope := "discount"
	if input.Courtesy {
		fallback = "Cortesía"
		scope = "courtesy"
	}
	concept, err := cleanConcept(input.Concept, fallback)
	if err != nil {
		return nil, err
	}
	message := "El descuento supera tu límite de " + plan.ActorLimit + " %. Solicita la autorización de un supervisor."
	if input.Courtesy {
		message = "Otorgar una cortesía requiere el permiso correspondiente o la autorización de un supervisor."
	}
	approved, err := handler.approve(scope, !plan.RequiresAuthorization, input.Authorization, message,
		func(supervisor *Supervisor) (bool, error) {
			if input.Courtesy {
				return supervisor.Can("payments.courtesy"), nil
			}
			if !supervisor.Can("payments.discounts.authorize") {
				return false, nil
			}
			limit, err := handler.discountLimit(supervisor.UserID)
			if err != nil {
				return false, err
			}
			return !plan.percent.GreaterThan(limit), nil
		})
	if err != nil {
		return nil, err
	}
	// A partir de aquí se escribe: la autorización ya se verificó.
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.account_adjustments(id,account_id,kind,scope,method,value,amount,courtesy,
        concept,reason,actor_id,device_id,authorized_by,operation_id) VALUES ($1,$2,'discount',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		id, accountID, input.Scope, input.Method, money(plan.value), plan.Amount, input.Courtesy, concept, reason,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, approved.authorizedBy(), handler.operationID)
	if err != nil {
		return nil, err
	}
	for index, unit := range plan.selected {
		if plan.shares[index].Sign() <= 0 {
			continue
		}
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.account_adjustment_allocations(adjustment_id,unit_id,amount) VALUES ($1,$2,$3)",
			id, unit.ID, money(plan.shares[index])); err != nil {
			return nil, err
		}
	}
	if err := handler.recordApproval(approved, accountID, plan.amount); err != nil {
		return nil, err
	}
	return handler.finishAdjustment(accountID, id, "payments.discount_applied", plan)
}

func (handler *service) finishAdjustment(accountID, adjustmentID, action string, detail any) (any, error) {
	closed, err := handler.closeIfSettled(accountID)
	if err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, accountID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.adjusted", handler.scope(finance), &accountID, nil); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, action, "account", &accountID, nil, map[string]any{
		"adjustment_id": adjustmentID, "detail": detail, "balance": finance.Balance,
	}); err != nil {
		return nil, err
	}
	return map[string]any{"adjustment_id": adjustmentID, "finance": finance, "closed": closed}, nil
}

// addCharge registra un cargo extra general, distinto de los extras de productos. Forma parte de
// la venta y lleva la tasa de IVA vigente al registrarse.
func (handler *service) addCharge(parts []string) (any, error) {
	var input struct {
		OperationID     string `json:"operation_id"`
		Concept         string `json:"concept"`
		Amount          string `json:"amount"`
		Reason          string `json:"reason"`
		ExpectedBalance string `json:"expected_balance"`
	}
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
	if finance.State != "open" {
		return nil, conflict("ACCOUNT_NOT_OPEN", "La cuenta no está abierta; no admite cargos.")
	}
	if input.ExpectedBalance != finance.Balance {
		return nil, conflict("BALANCE_CHANGED", "El saldo de la cuenta cambió en otra terminal. Revisa el saldo actualizado.")
	}
	// Un cargo nunca es negativo: los descuentos tienen su propia operación.
	amount, err := parseAmount(input.Amount, "cargo")
	if err != nil {
		return nil, err
	}
	if amount.Sign() <= 0 {
		return nil, invalid("El cargo debe ser mayor que cero.")
	}
	concept := strings.TrimSpace(input.Concept)
	if textLength(concept) < 1 || textLength(concept) > 120 {
		return nil, invalid("Indica el concepto del cargo (hasta 120 caracteres).")
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) > 500 {
		return nil, invalid("El motivo admite hasta 500 caracteres.")
	}
	rate, err := CurrentTaxRate(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.account_adjustments(id,account_id,kind,scope,method,value,amount,tax_rate,
        concept,reason,actor_id,device_id,operation_id) VALUES ($1,$2,'charge','account','amount',$3,$3,$4,$5,$6,$7,$8,$9)`,
		id, accountID, money(amount), rate, concept, reason, handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
	if err != nil {
		return nil, err
	}
	return handler.finishAdjustment(accountID, id, "payments.charge_added", map[string]string{"concept": concept, "amount": money(amount)})
}

// voidAdjustment cancela un descuento o cargo con un registro compensatorio; el original se conserva.
func (handler *service) voidAdjustment(parts []string) (any, error) {
	var input struct {
		OperationID   string              `json:"operation_id"`
		AdjustmentID  string              `json:"adjustment_id"`
		Reason        string              `json:"reason"`
		Authorization *AuthorizationInput `json:"authorization"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.AdjustmentID); err != nil {
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
	if finance.State != "open" {
		return nil, conflict("ACCOUNT_NOT_OPEN", "La cuenta no está abierta; no admite cambios de ajustes.")
	}
	var target *Adjustment
	for index := range finance.Adjustments {
		if finance.Adjustments[index].ID == input.AdjustmentID {
			target = &finance.Adjustments[index]
		}
	}
	if target == nil {
		return nil, notFound("El ajuste no pertenece a esta cuenta.")
	}
	if target.Voided {
		return nil, conflict("ADJUSTMENT_VOIDED", "El ajuste ya fue cancelado.")
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) < 5 || textLength(reason) > 500 {
		return nil, invalid("Indica el motivo de la cancelación (5–500 caracteres).")
	}
	amount := decimal.RequireFromString(target.Amount)
	// Cancelar un cargo reduce el total: no puede dejarlo por debajo de lo pagado.
	if target.Kind == "charge" && amount.GreaterThan(finance.balance) {
		return nil, conflict("ACCOUNT_BELOW_PAYMENTS", "Cancelar el cargo dejaría la cuenta con un total menor que sus pagos. Registra primero la devolución correspondiente.")
	}
	permission := "payments.discounts.apply"
	supervisorPermission := "payments.discounts.authorize"
	if target.Kind == "charge" {
		permission = "payments.charges.manage"
		supervisorPermission = "payments.charges.manage"
	}
	approved, err := handler.approve("adjustment_void", handler.request.Actor.Can(permission), input.Authorization,
		"Cancelar este ajuste requiere la autorización de un supervisor.",
		func(supervisor *Supervisor) (bool, error) {
			return supervisor.Can(supervisorPermission), nil
		})
	if err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.account_adjustment_voids(adjustment_id,reason,actor_id,device_id,authorized_by,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6)`, target.ID, reason, handler.request.Actor.UserID, handler.request.Actor.DeviceID, approved.authorizedBy(), handler.operationID)
	if err != nil {
		return nil, err
	}
	if err := handler.recordApproval(approved, accountID, amount); err != nil {
		return nil, err
	}
	return handler.finishAdjustment(accountID, target.ID, "payments.adjustment_voided", map[string]string{"kind": target.Kind, "reason": reason})
}
