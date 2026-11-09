package payments

import (
	"strings"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

// Operaciones de efectivo del turno distintas de un cobro: ingresos, retiros y gastos. Son
// registros inmutables con su usuario, terminal, caja y turno. Ningún ingreso se registra como
// venta y ningún gasto crea una compra de inventario.
//
// Autorizaciones (decisión del propietario):
//   - Ingreso: basta el permiso de registrarlo; no se pide segunda contraseña.
//   - Retiro y gasto: siempre autorizados. Si quien registra tiene cash.outflows.authorize, su
//     propio permiso basta ('self'); si no, la autorización puntual de otra persona con ese
//     permiso ('supervisor'), con el mismo autorizador y límite de intentos de la etapa 6.
//   - Una salida que excede el efectivo esperado de su moneda se rechaza sin revelar cifras, salvo
//     que se solicite como excepcional y la autorice alguien que además pueda consultar el
//     efectivo esperado.

// CashOperationInput es la captura de un ingreso, retiro o gasto.
type CashOperationInput struct {
	OperationID string `json:"operation_id"`
	Currency    string `json:"currency"`
	Amount      string `json:"amount"`
	Concept     string `json:"concept"`
	// Type es el tipo de ingreso o el motivo del retiro (lista cerrada); no aplica a gastos.
	Type string `json:"type"`
	// CategoryID es la categoría del gasto; solo aplica a gastos.
	CategoryID string `json:"category_id"`
	Reference  string `json:"reference"`
	Notes      string `json:"notes"`
	// Exceptional solicita registrar una salida que excede el efectivo esperado; exige motivo.
	Exceptional   bool                `json:"exceptional"`
	Reason        string              `json:"reason"`
	Authorization *AuthorizationInput `json:"authorization"`
}

// operationDraft es una operación validada antes de escribir.
type operationDraft struct {
	kind        string
	currency    string
	amount      decimal.Decimal
	direction   string
	concept     string
	subtype     *string
	categoryID  *string
	reference   string
	notes       string
	reason      string
	exceptional bool
}

// cashCurrency valida la moneda de una operación (pesos por omisión).
func cashCurrency(text string) (string, error) {
	currency := strings.TrimSpace(text)
	if currency == "" {
		return currencyMXN, nil
	}
	if currency != currencyMXN && currency != currencyUSD {
		return "", invalid("La moneda debe ser pesos (MXN) o dólares (USD).")
	}
	return currency, nil
}

// validateOperationInput comprueba la forma completa de la captura según su tipo.
func (handler *service) validateOperationInput(kind string, input CashOperationInput) (*operationDraft, error) {
	currency, err := cashCurrency(input.Currency)
	if err != nil {
		return nil, err
	}
	amount, err := parseAmount(strings.TrimSpace(input.Amount), "la operación")
	if err != nil {
		return nil, err
	}
	// No se admiten importes negativos ni cero.
	if amount.Sign() <= 0 {
		return nil, invalid("El importe debe ser mayor que cero.")
	}
	draft := &operationDraft{
		kind:      kind,
		currency:  currency,
		amount:    amount,
		direction: directionOut,
		reference: strings.TrimSpace(input.Reference),
		notes:     strings.TrimSpace(input.Notes),
		reason:    strings.TrimSpace(input.Reason),
	}
	if kind == operationIncome {
		draft.direction = directionIn
		if input.Authorization != nil || input.Exceptional {
			return nil, invalid("Los ingresos de efectivo no requieren autorización de supervisor ni son excepcionales.")
		}
	}
	if textLength(draft.reference) > 120 {
		return nil, invalid("La referencia admite hasta 120 caracteres.")
	}
	if textLength(draft.notes) > 2000 {
		return nil, invalid("Las observaciones admiten hasta 2000 caracteres.")
	}
	if textLength(draft.reason) > 500 {
		return nil, invalid("El motivo admite hasta 500 caracteres.")
	}
	if input.Exceptional && textLength(draft.reason) < 5 {
		return nil, invalid("Indica el motivo de la salida excepcional (5–500 caracteres).")
	}
	draft.exceptional = input.Exceptional
	fallback, err := handler.classifyOperation(kind, input, draft)
	if err != nil {
		return nil, err
	}
	concept, err := cleanConcept(input.Concept, fallback)
	if err != nil {
		return nil, err
	}
	draft.concept = concept
	return draft, nil
}

// classifyOperation valida el tipo (ingreso y retiro) o la categoría (gasto) y devuelve el
// concepto predeterminado cuando no se captura uno.
func (handler *service) classifyOperation(kind string, input CashOperationInput, draft *operationDraft) (string, error) {
	if kind == operationExpense {
		if strings.TrimSpace(input.Type) != "" {
			return "", invalid("Los gastos se clasifican por categoría, no por tipo.")
		}
		if err := validateID(input.CategoryID); err != nil {
			return "", invalid("Selecciona la categoría del gasto.")
		}
		category, err := handler.readExpenseCategory(input.CategoryID)
		if err != nil {
			return "", err
		}
		if !category.Active {
			return "", invalid("La categoría de gasto " + category.Name + " está desactivada.")
		}
		draft.categoryID = &category.ID
		return category.Name, nil
	}
	if strings.TrimSpace(input.CategoryID) != "" {
		return "", invalid("Solo los gastos llevan categoría.")
	}
	code := strings.TrimSpace(input.Type)
	label := subtypeLabel(kind, code)
	if label == "" {
		if kind == operationIncome {
			return "", invalid("Selecciona el tipo de ingreso.")
		}
		return "", invalid("Selecciona el motivo del retiro.")
	}
	draft.subtype = &code
	return label, nil
}

// operationRate devuelve el tipo de cambio del turno para una operación en dólares. Los dólares
// exigen la función activa y un tipo de cambio capturado en el turno.
func (handler *service) operationRate(shift *Shift, currency string) (*string, error) {
	if currency != currencyUSD {
		return nil, nil
	}
	if err := handler.requireUSDCash(); err != nil {
		return nil, err
	}
	if shift.USDRate == nil {
		return nil, conflict("USD_RATE_REQUIRED", "El turno no tiene tipo de cambio: no acepta operaciones en dólares hasta que alguien con permiso lo capture.")
	}
	return shift.USDRate, nil
}

// authorizeOutflow resuelve la autorización obligatoria de un retiro o gasto antes de escribir.
// Una salida excepcional exige además que quien autoriza pueda consultar el efectivo esperado.
func (handler *service) authorizeOutflow(input *AuthorizationInput, exceptional bool) (*approval, error) {
	required := []string{permissionOutflowsAuthorize}
	if exceptional {
		required = append(required, permissionExpectedRead)
	}
	allowed := handler.request.Actor.Require(required...) == nil
	return handler.approve("cash_outflow", allowed, input, "Este retiro o gasto requiere la autorización de un supervisor.",
		func(supervisor *Supervisor) (bool, error) {
			for _, permission := range required {
				if !supervisor.Can(permission) {
					return false, nil
				}
			}
			return true, nil
		})
}

// cashInsufficient rechaza una salida que excede el efectivo disponible sin revelar cuánto hay.
func cashInsufficient() error {
	return conflict("CASH_INSUFFICIENT", "El importe no puede autorizarse con el efectivo de la caja; pide a un supervisor que lo revise.")
}

// approvalColumns traduce la autorización a las columnas de la operación: por permiso propio el
// autorizador es el mismo actor; por supervisor, otra persona.
func (handler *service) approvalColumns(approved *approval) (*string, *string) {
	if approved == nil {
		return nil, nil
	}
	mode := "self"
	authorizedBy := handler.request.Actor.UserID
	if approved.supervisor != nil {
		mode = "supervisor"
		authorizedBy = approved.supervisor.UserID
	}
	return &authorizedBy, &mode
}

func (handler *service) recordIncome(parts []string) (any, error) {
	return handler.recordCashOperation(parts[1], operationIncome)
}

func (handler *service) recordWithdrawal(parts []string) (any, error) {
	return handler.recordCashOperation(parts[1], operationWithdrawal)
}

func (handler *service) recordExpense(parts []string) (any, error) {
	return handler.recordCashOperation(parts[1], operationExpense)
}

// recordCashOperation valida, autoriza y registra un ingreso, retiro o gasto en el turno indicado
// en la ruta. Todo se comprueba antes de la primera escritura.
func (handler *service) recordCashOperation(shiftID, kind string) (any, error) {
	var input CashOperationInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	draft, err := handler.validateOperationInput(kind, input)
	if err != nil {
		return nil, err
	}
	shift, err := handler.operableShift(shiftID)
	if err != nil {
		return nil, err
	}
	rate, err := handler.operationRate(shift, draft.currency)
	if err != nil {
		return nil, err
	}
	var approved *approval
	if kind != operationIncome {
		// La autorización se valida antes de comparar con el efectivo disponible: así un rechazo
		// por exceso nunca puede provocarse sin una autorización válida.
		approved, err = handler.authorizeOutflow(input.Authorization, draft.exceptional)
		if err != nil {
			return nil, err
		}
		if err := handler.checkAvailable(shift, draft); err != nil {
			return nil, err
		}
	}
	id, err := handler.insertCashOperation(shift, draft, rate, approved, nil, false)
	if err != nil {
		return nil, err
	}
	operation, err := handler.readCashOperation(id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.operation.recorded", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return operation, handler.audit(handler.ctx, handler.tx, "cash.operation_recorded", "cash_operation", &id, nil, operation)
}

// checkAvailable compara una salida con el efectivo esperado de su moneda. Sin solicitud
// excepcional, el exceso se rechaza con un mensaje sin cifras; con ella (ya autorizada por
// alguien que puede consultar el esperado) la salida queda marcada como excepcional. Si no
// excede, se registra como ordinaria aunque se haya solicitado como excepcional.
func (handler *service) checkAvailable(shift *Shift, draft *operationDraft) error {
	expected, err := handler.expectedCash(shift)
	if err != nil {
		return err
	}
	exceeds := draft.amount.GreaterThan(expected.amount(draft.currency))
	if exceeds && !draft.exceptional {
		return cashInsufficient()
	}
	draft.exceptional = exceeds
	return nil
}

// insertCashOperation guarda la operación inmutable con su autorización.
func (handler *service) insertCashOperation(shift *Shift, draft *operationDraft, rate *string, approved *approval,
	corrects *string, regularization bool) (string, error) {
	id := newID()
	authorizedBy, mode := handler.approvalColumns(approved)
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_operations(id,shift_id,register_id,kind,currency,amount,direction,
        concept,subtype,category_id,reference,notes,reason,corrects_operation_id,regularization,exceptional,usd_rate,actor_id,device_id,
        authorized_by,authorization_mode,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::numeric,$18,$19,$20,$21,$22)`,
		id, shift.ID, shift.RegisterID, draft.kind, draft.currency, money(draft.amount), draft.direction, draft.concept, draft.subtype,
		draft.categoryID, draft.reference, draft.notes, draft.reason, corrects, regularization, draft.exceptional, rate,
		handler.request.Actor.UserID, handler.request.Actor.DeviceID, authorizedBy, mode, handler.operationID)
	return id, err
}

const cashOperationQuery = `SELECT o.id,o.shift_id,o.register_id,r.name,o.kind,o.currency,o.amount::text,o.direction,o.concept,o.subtype,
    o.category_id,c.name,o.reference,o.notes,o.reason,o.corrects_operation_id,fix.id,o.regularization,o.exceptional,o.usd_rate::text,
    actor.display_name,d.name,authorizer.display_name,o.authorization_mode,o.created_at
    FROM app.cash_operations o JOIN app.registers r ON r.id=o.register_id
    JOIN app.users actor ON actor.id=o.actor_id JOIN app.devices d ON d.id=o.device_id
    LEFT JOIN app.users authorizer ON authorizer.id=o.authorized_by
    LEFT JOIN app.cash_expense_categories c ON c.id=o.category_id
    LEFT JOIN app.cash_operations fix ON fix.corrects_operation_id=o.id`

// scanCashOperation lee una fila de cashOperationQuery y completa sus datos derivados.
func scanCashOperation(scan func(...any) error) (*CashOperation, error) {
	var item CashOperation
	if err := scan(&item.ID, &item.ShiftID, &item.RegisterID, &item.RegisterName, &item.Kind, &item.Currency, &item.Amount,
		&item.Direction, &item.Concept, &item.Type, &item.CategoryID, &item.CategoryName, &item.Reference, &item.Notes, &item.Reason,
		&item.CorrectsOperationID, &item.CorrectedByID, &item.Regularization, &item.Exceptional, &item.USDRate, &item.ActorName, &item.DeviceName, &item.AuthorizedByName, &item.AuthorizationMode, &item.CreatedAt); err != nil {
		return nil, err
	}
	if item.Type != nil {
		label := subtypeLabel(item.Kind, *item.Type)
		item.TypeLabel = &label
	}
	item.EquivalentMXN = item.Amount
	if item.Currency == currencyUSD && item.USDRate != nil {
		item.EquivalentMXN = money(usdToMXN(decimal.RequireFromString(item.Amount), decimal.RequireFromString(*item.USDRate)))
	}
	item.Status = operationStatus(&item)
	return &item, nil
}

// operationStatus resume el estado visible de una operación sin mostrar identificadores.
func operationStatus(item *CashOperation) string {
	switch {
	case item.Regularization:
		return "regularization"
	case item.Kind == operationCorrection:
		return "correction"
	case item.CorrectedByID != nil:
		return "corrected"
	}
	return "recorded"
}

func (handler *service) readCashOperation(id string) (*CashOperation, error) {
	operation, err := scanCashOperation(handler.tx.QueryRow(handler.ctx, cashOperationQuery+" WHERE o.id=$1", id).Scan)
	if isNoRows(err) {
		return nil, notFound("La operación de efectivo no existe.")
	}
	return operation, err
}

// listCashOperations devuelve las operaciones que cumplen la condición, en orden cronológico.
func (handler *service) listCashOperations(where string, arguments ...any) ([]CashOperation, error) {
	rows, err := handler.tx.Query(handler.ctx, cashOperationQuery+" WHERE "+where+" ORDER BY o.created_at,o.id", arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CashOperation{}
	for rows.Next() {
		operation, err := scanCashOperation(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, *operation)
	}
	return result, rows.Err()
}

// shiftOperations atiende la consulta de operaciones de efectivo de un turno (cash.movements.read).
// Son registros individuales; ningún total del que se deduzca el esperado viaja aquí.
func (handler *service) shiftOperations(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	operations, err := handler.listCashOperations("o.shift_id=$1", shift.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"shift": shift, "operations": operations}, nil
}
