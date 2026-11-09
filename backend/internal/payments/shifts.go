package payments

import (
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
)

type Register struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type Shift struct {
	ID              string  `json:"id"`
	RegisterID      string  `json:"register_id"`
	RegisterName    string  `json:"register_name"`
	State           string  `json:"state"`
	OpeningFloat    string  `json:"opening_float"`
	OpeningFloatUSD *string `json:"opening_float_usd"`
	OpeningNotes    string  `json:"opening_notes"`
	// Fecha comercial: fecha de apertura en la zona horaria del establecimiento.
	BusinessDate string    `json:"business_date"`
	OpenedBy     string    `json:"opened_by"`
	OpenedByName string    `json:"opened_by_name"`
	OpenedDevice string    `json:"opened_device_name"`
	OpenedAt     time.Time `json:"opened_at"`
	// Arqueo de cierre en curso (o el que cerró el turno): quién lo inició, cuándo y desde dónde.
	ClosingStartedBy     *string    `json:"closing_started_by"`
	ClosingStartedByName *string    `json:"closing_started_by_name"`
	ClosingStartedAt     *time.Time `json:"closing_started_at"`
	ClosingDeviceName    *string    `json:"closing_device_name"`
	ClosedAt             *time.Time `json:"closed_at"`
	ClosedBy             *string    `json:"closed_by"`
	ClosedByName         *string    `json:"closed_by_name"`
	ClosedDeviceName     *string    `json:"closed_device_name"`
	Revision             int        `json:"revision"`
	SharedAllowed        bool       `json:"shared_allowed"`
	// Tipo de cambio fijo del turno (MXN por USD); nil mientras no se captura.
	USDRate          *string    `json:"usd_rate"`
	USDRateSetByName *string    `json:"usd_rate_set_by_name"`
	USDRateSetAt     *time.Time `json:"usd_rate_set_at"`

	// Conteos de registros al iniciar el arqueo; la confirmación los compara.
	closingMovements  *int
	closingOperations *int
}

// businessDateExpression calcula la fecha comercial de un turno: la guardada al abrir o, en
// turnos anteriores a la etapa 7, la fecha de apertura en la zona horaria vigente.
const businessDateExpression = `coalesce(s.business_date,(s.opened_at AT TIME ZONE coalesce(e.timezone,'UTC'))::date)`

const shiftQuery = `SELECT s.id,s.register_id,r.name,s.state,s.opening_float::text,s.opening_float_usd::text,s.opening_notes,
    ` + businessDateExpression + `::text,s.opened_by,u.display_name,d.name,s.opened_at,
    s.closing_started_by,starter.display_name,s.closing_started_at,closing_device.name,
    s.closed_at,s.closed_by,closer.display_name,closed_device.name,
    s.revision,s.usd_rate::text,setter.display_name,s.usd_rate_set_at,s.closing_movement_count,s.closing_operation_count
    FROM app.cash_shifts s JOIN app.registers r ON r.id=s.register_id
    JOIN app.users u ON u.id=s.opened_by JOIN app.devices d ON d.id=s.opened_device_id
    LEFT JOIN app.users setter ON setter.id=s.usd_rate_set_by
    LEFT JOIN app.users starter ON starter.id=s.closing_started_by
    LEFT JOIN app.devices closing_device ON closing_device.id=s.closing_device_id
    LEFT JOIN app.users closer ON closer.id=s.closed_by
    LEFT JOIN app.devices closed_device ON closed_device.id=s.closed_device_id
    LEFT JOIN app.establishment e ON true`

// scanShift lee una fila de shiftQuery.
func scanShift(row pgx.Row) (*Shift, error) {
	var shift Shift
	err := row.Scan(&shift.ID, &shift.RegisterID, &shift.RegisterName, &shift.State, &shift.OpeningFloat, &shift.OpeningFloatUSD,
		&shift.OpeningNotes, &shift.BusinessDate, &shift.OpenedBy, &shift.OpenedByName, &shift.OpenedDevice, &shift.OpenedAt,
		&shift.ClosingStartedBy, &shift.ClosingStartedByName, &shift.ClosingStartedAt, &shift.ClosingDeviceName,
		&shift.ClosedAt, &shift.ClosedBy, &shift.ClosedByName, &shift.ClosedDeviceName,
		&shift.Revision, &shift.USDRate, &shift.USDRateSetByName, &shift.USDRateSetAt,
		&shift.closingMovements, &shift.closingOperations)
	if err != nil {
		return nil, err
	}
	return &shift, nil
}

func (handler *service) readShift(where string, argument any) (*Shift, error) {
	shift, err := scanShift(handler.tx.QueryRow(handler.ctx, shiftQuery+" WHERE "+where, argument))
	if isNoRows(err) {
		return nil, nil
	}
	return shift, err
}

// activeShift devuelve el turno activo de una caja: abierto o en arqueo de cierre.
func (handler *service) activeShift(registerID string) (*Shift, error) {
	return handler.readShift("s.register_id=$1 AND s.state IN ('open','closing')", registerID)
}

// deviceRegister obtiene la caja física asociada a la terminal actual.
func (handler *service) deviceRegister() (*Register, error) {
	var id, name *string
	var active *bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT r.id,r.name,r.active FROM app.devices d
        LEFT JOIN app.registers r ON r.id=d.register_id WHERE d.id=$1`, handler.request.Actor.DeviceID).Scan(&id, &name, &active)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	if id == nil {
		return nil, nil
	}
	return &Register{ID: *id, Name: *name, Active: *active}, nil
}

func (handler *service) currentShift(_ []string) (any, error) {
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	// La interfaz del cobro solo muestra dólares con la función activa; usd_rate_required indica
	// que el turno abierto aún no tiene tipo de cambio y por eso no acepta dólares.
	usdCash, err := features.Enabled(handler.ctx, handler.tx, features.USDCash)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"register": register, "shift": nil, "shared_shifts": settings.SharedShifts, "usable": false, "reason": "",
		"usd_cash": usdCash, "usd_rate_required": false,
	}
	if register == nil {
		result["reason"] = "Esta terminal no tiene una caja física asignada. Asígnala desde Terminales."
		return result, nil
	}
	shift, err := handler.activeShift(register.ID)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		result["reason"] = "No hay un turno abierto en " + register.Name + ". Ábrelo con su fondo inicial antes de cobrar."
		return result, nil
	}
	shift.SharedAllowed = settings.SharedShifts
	result["shift"] = shift
	// Un turno en arqueo de cierre está pausado: no acepta cobros, devoluciones ni movimientos.
	if shift.State == shiftClosing {
		result["reason"] = closingMessage(shift)
		return result, nil
	}
	result["usd_rate_required"] = usdCash && shift.USDRate == nil
	// Aviso del tope de caja (función cash_limit): solo para quien ve el esperado del turno. Nunca
	// impide cobrar.
	warning, err := handler.cashLimitWarning(shift)
	if err != nil {
		return nil, err
	}
	if warning != nil {
		result["cash_limit_warning"] = warning
	}
	if !settings.SharedShifts && shift.OpenedBy != handler.request.Actor.UserID {
		result["reason"] = "El turno abierto pertenece a " + shift.OpenedByName + " y la configuración no permite compartirlo."
		return result, nil
	}
	result["usable"] = true
	return result, nil
}

// usableShift exige caja asociada y turno abierto antes de registrar cualquier cobro o devolución.
// Nunca se abre un turno automáticamente con un fondo inventado. Un turno en arqueo de cierre
// responde 409 SHIFT_CLOSING.
func (handler *service) usableShift() (*Shift, error) {
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	if register == nil {
		return nil, conflict("REGISTER_REQUIRED", "Esta terminal no tiene una caja física asignada. Asígnala desde Terminales antes de cobrar.")
	}
	shift, err := handler.activeShift(register.ID)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, conflict("SHIFT_REQUIRED", "No hay un turno abierto en "+register.Name+". Abre el turno con su fondo inicial antes de cobrar.")
	}
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	if err := handler.requireSharedShift(shift); err != nil {
		return nil, err
	}
	return shift, nil
}

// requireSharedShift respeta la política de turno compartido: si está desactivada, solo quien
// abrió el turno registra cobros y movimientos en él.
func (handler *service) requireSharedShift(shift *Shift) error {
	settings, err := handler.settings()
	if err != nil {
		return err
	}
	if !settings.SharedShifts && shift.OpenedBy != handler.request.Actor.UserID {
		return conflict("SHIFT_NOT_SHARED", "El turno abierto pertenece a "+shift.OpenedByName+" y la configuración del establecimiento no permite compartirlo.")
	}
	return nil
}

// usableShiftFor valida además el turno que la terminal mostraba al preparar la operación
// (shift_id opcional del cuerpo, parte de la huella idempotente). Así, el reintento de una
// operación incierta sobre un turno pausado o cerrado se rechaza con un error claro en lugar de
// registrarse en otro turno; si sigue haciendo falta, se registra como operación nueva.
func (handler *service) usableShiftFor(expectedID string) (*Shift, error) {
	expectedID = strings.TrimSpace(expectedID)
	if expectedID != "" {
		if err := validateID(expectedID); err != nil {
			return nil, err
		}
		expected, err := handler.readShift("s.id=$1", expectedID)
		if err != nil {
			return nil, err
		}
		if expected == nil {
			return nil, notFound("El turno indicado no existe.")
		}
		if err := shiftStateError(expected); err != nil {
			return nil, err
		}
	}
	shift, err := handler.usableShift()
	if err != nil {
		return nil, err
	}
	if expectedID != "" && shift.ID != expectedID {
		return nil, conflict("SHIFT_CHANGED", "El turno de la caja cambió desde que se preparó la operación. Actualiza la pantalla y revisa el turno vigente.")
	}
	return shift, nil
}

// OpeningInput es la apertura ampliada del turno. El fondo en pesos es obligatorio (puede ser
// $0); el fondo en dólares y el tipo de cambio solo se aceptan con la función usd_cash activa. El
// desglose por denominaciones es opcional: si viene, su total por moneda debe coincidir con el
// fondo declarado.
type OpeningInput struct {
	OperationID           string      `json:"operation_id"`
	OpeningFloat          string      `json:"opening_float"`
	OpeningFloatUSD       string      `json:"opening_float_usd"`
	OpeningNotes          string      `json:"opening_notes"`
	USDRate               string      `json:"usd_rate"`
	ConfirmRateDifference bool        `json:"confirm_rate_difference"`
	Count                 *CountInput `json:"count"`
}

// openingPlan es la apertura validada por completo antes de escribir.
type openingPlan struct {
	float    decimal.Decimal
	floatUSD *decimal.Decimal
	notes    string
	rate     *string
	count    *countTally
}

func (handler *service) openShift(_ []string) (any, error) {
	var input OpeningInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	plan, err := handler.planOpening(input)
	if err != nil {
		return nil, err
	}
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	if register == nil {
		return nil, conflict("REGISTER_REQUIRED", "Esta terminal no tiene una caja física asignada. Asígnala desde Terminales.")
	}
	if !register.Active {
		return nil, conflict("REGISTER_INACTIVE", "La caja "+register.Name+" está desactivada.")
	}
	// El lock operativo serializa las aperturas; el índice único parcial impide duplicados aunque
	// falle esa serialización. Un turno en arqueo de cierre sigue activo: no se abre otro.
	existing, err := handler.activeShift(register.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, shiftAlreadyOpen(register.Name, existing)
	}
	id := newID()
	if err := handler.insertShift(id, register, plan); err != nil {
		return nil, err
	}
	if plan.count != nil {
		if _, err := handler.insertCount(id, "opening", "final", *plan.count, ""); err != nil {
			return nil, err
		}
	}
	shift, err := handler.readShift("s.id=$1", id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.opened", cashScope, &id, nil); err != nil {
		return nil, err
	}
	return shift, handler.audit(handler.ctx, handler.tx, "cash.shift_opened", "cash_shift", &id, nil, map[string]any{
		"shift": shift, "counted_by_denominations": plan.count != nil,
	})
}

// shiftAlreadyOpen explica por qué una caja no admite otra apertura.
func shiftAlreadyOpen(registerName string, existing *Shift) error {
	if existing.State == shiftClosing {
		return conflict("SHIFT_ALREADY_OPEN", "La caja "+registerName+" está en arqueo de cierre. Confirma o cancela el arqueo antes de abrir otro turno.")
	}
	return conflict("SHIFT_ALREADY_OPEN", "La caja "+registerName+" ya tiene un turno abierto por "+existing.OpenedByName+".")
}

// planOpening valida la apertura completa (fondos, observaciones, tipo de cambio y desglose)
// antes de cualquier escritura.
func (handler *service) planOpening(input OpeningInput) (*openingPlan, error) {
	// El fondo inicial es obligatorio, aunque puede ser $0.
	opening, err := parseAmount(input.OpeningFloat, "fondo inicial")
	if err != nil {
		return nil, err
	}
	plan := &openingPlan{float: opening, notes: strings.TrimSpace(input.OpeningNotes)}
	if textLength(plan.notes) > 2000 {
		return nil, invalid("Las observaciones de la apertura admiten hasta 2000 caracteres.")
	}
	// El fondo en dólares es un dato en dólares: con la función desactivada se rechaza.
	if strings.TrimSpace(input.OpeningFloatUSD) != "" {
		if err := handler.requireUSDCash(); err != nil {
			return nil, err
		}
		floatUSD, err := parseAmount(strings.TrimSpace(input.OpeningFloatUSD), "fondo inicial en dólares")
		if err != nil {
			return nil, err
		}
		plan.floatUSD = &floatUSD
	}
	// El tipo de cambio es opcional al abrir; si viene, exige la función activa y su permiso, y se
	// valida por completo antes de escribir.
	if strings.TrimSpace(input.USDRate) != "" || input.ConfirmRateDifference {
		if err := handler.requireUSDCash(); err != nil {
			return nil, err
		}
		if err := handler.request.Actor.Require("payments.usd_rate.manage"); err != nil {
			return nil, err
		}
		rate, err := handler.validateShiftRate(input.USDRate, input.ConfirmRateDifference, nil)
		if err != nil {
			return nil, err
		}
		formatted := money(rate)
		plan.rate = &formatted
	}
	if input.Count != nil {
		tally, err := handler.evaluateCount(*input.Count, plan.floatUSD != nil)
		if err != nil {
			return nil, err
		}
		if err := matchOpeningCount(tally, plan); err != nil {
			return nil, err
		}
		plan.count = &tally
	}
	return plan, nil
}

// matchOpeningCount exige que el desglose sume exactamente el fondo declarado en cada moneda.
func matchOpeningCount(tally countTally, plan *openingPlan) error {
	floatUSD := zero
	if plan.floatUSD != nil {
		floatUSD = *plan.floatUSD
	}
	if !tally.mxn.Equal(plan.float) {
		return &Error{
			Status:  400,
			Code:    "COUNT_TOTAL_MISMATCH",
			Message: "El desglose suma " + formatPesos(tally.mxn) + " y el fondo inicial declarado es " + formatPesos(plan.float) + ". Revisa el conteo.",
		}
	}
	if !tally.usd.Equal(floatUSD) {
		return &Error{
			Status:  400,
			Code:    "COUNT_TOTAL_MISMATCH",
			Message: "El desglose en dólares suma " + formatDollars(tally.usd) + " y el fondo en dólares declarado es " + formatDollars(floatUSD) + ". Revisa el conteo.",
		}
	}
	return nil
}

// insertShift registra el turno con su fecha comercial, calculada con la misma hora de apertura
// en la zona horaria del establecimiento.
func (handler *service) insertShift(id string, register *Register, plan *openingPlan) error {
	var floatUSD *string
	if plan.floatUSD != nil {
		floatUSD = optionalMoney(*plan.floatUSD)
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_shifts(id,register_id,opening_float,opening_float_usd,opening_notes,
        business_date,opened_by,opened_device_id,operation_id,usd_rate,usd_rate_set_by,usd_rate_set_at)
        VALUES ($1,$2,$3,$4::numeric,$5,(now() AT TIME ZONE coalesce((SELECT timezone FROM app.establishment),'UTC'))::date,
        $6,$7,$8,$9::numeric,CASE WHEN $9::numeric IS NULL THEN NULL ELSE $6::uuid END,
        CASE WHEN $9::numeric IS NULL THEN NULL ELSE now() END)`,
		id, register.ID, money(plan.float), floatUSD, plan.notes, handler.request.Actor.UserID, handler.request.Actor.DeviceID,
		handler.operationID, plan.rate)
	// Dos aperturas que eludieran el lock chocarían con el índice único de turno activo.
	var problem *pgconn.PgError
	if errors.As(err, &problem) && problem.Code == "23505" && problem.ConstraintName == "cash_shifts_one_active_idx" {
		return conflict("SHIFT_ALREADY_OPEN", "La caja "+register.Name+" ya tiene un turno activo.")
	}
	return err
}

// validateShiftRate valida un tipo de cambio: formato, rango configurado y, si difiere más del
// porcentaje de alerta del último tipo capturado en otro turno, confirmación explícita. Así se
// evitan capturas como 1.95 en lugar de 19.50.
func (handler *service) validateShiftRate(text string, confirmed bool, currentShift *string) (decimal.Decimal, error) {
	rate, err := parseRate(strings.TrimSpace(text))
	if err != nil {
		return zero, err
	}
	settings, err := handler.settings()
	if err != nil {
		return zero, err
	}
	minimum := decimal.RequireFromString(settings.USDRateMin)
	maximum := decimal.RequireFromString(settings.USDRateMax)
	if rate.LessThan(minimum) || rate.GreaterThan(maximum) {
		return zero, &Error{
			Status: 400,
			Code:   "USD_RATE_OUT_OF_RANGE",
			Message: "El tipo de cambio debe estar entre " + formatPesos(minimum) + " y " + formatPesos(maximum) +
				" por dólar. Revisa que no falte ni sobre un dígito.",
		}
	}
	// El turno anterior es el último que capturó tipo de cambio en cualquier caja del establecimiento.
	var previousText *string
	err = handler.tx.QueryRow(handler.ctx, `SELECT usd_rate::text FROM app.cash_shifts
        WHERE usd_rate IS NOT NULL AND ($1::uuid IS NULL OR id<>$1::uuid)
        ORDER BY usd_rate_set_at DESC,opened_at DESC,id LIMIT 1`, currentShift).Scan(&previousText)
	if err != nil && !isNoRows(err) {
		return zero, err
	}
	if previousText != nil && !confirmed {
		previous := decimal.RequireFromString(*previousText)
		difference := rateDifference(rate, previous)
		if difference.GreaterThan(decimal.RequireFromString(settings.USDRateAlertPercent)) {
			return zero, conflict("USD_RATE_CONFIRMATION_REQUIRED", "El tipo de cambio "+formatPesos(rate)+" difiere "+
				difference.StringFixed(2)+" % del turno anterior ("+formatPesos(previous)+"). Confirma que es correcto.")
		}
	}
	return rate, nil
}

// setShiftRate captura una sola vez el tipo de cambio de un turno abierto que no lo tiene, por
// ejemplo cuando la función se activó con el turno ya abierto. Queda fijo durante el turno.
func (handler *service) setShiftRate(parts []string) (any, error) {
	var input struct {
		OperationID           string `json:"operation_id"`
		USDRate               string `json:"usd_rate"`
		ConfirmRateDifference bool   `json:"confirm_rate_difference"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := handler.requireUSDCash(); err != nil {
		return nil, err
	}
	shift, err := handler.readShift("s.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
	}
	// En arqueo de cierre o cerrado, el tipo de cambio ya no puede capturarse.
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	if shift.USDRate != nil {
		return nil, conflict("USD_RATE_ALREADY_SET", "El turno ya tiene tipo de cambio ("+formatPesos(decimal.RequireFromString(*shift.USDRate))+
			"). Queda fijo hasta el cierre del turno.")
	}
	rate, err := handler.validateShiftRate(input.USDRate, input.ConfirmRateDifference, &shift.ID)
	if err != nil {
		return nil, err
	}
	// La condición usd_rate IS NULL impide sobrescribir un tipo capturado por otra terminal.
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_shifts SET usd_rate=$2,usd_rate_set_by=$3,usd_rate_set_at=now(),
        revision=revision+1 WHERE id=$1 AND state='open' AND usd_rate IS NULL`, shift.ID, money(rate), handler.request.Actor.UserID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, conflict("USD_RATE_ALREADY_SET", "El turno ya tiene tipo de cambio. Queda fijo hasta el cierre del turno.")
	}
	saved, err := handler.readShift("s.id=$1", shift.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.usd_rate_set", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.shift_usd_rate_set", "cash_shift", &shift.ID,
		map[string]any{"usd_rate": nil}, map[string]any{"usd_rate": saved.USDRate, "confirmed_difference": input.ConfirmRateDifference})
}

type Movement struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	MethodName   string `json:"method_name"`
	IsCash       bool   `json:"is_cash"`
	SaleAmount   string `json:"sale_amount"`
	TipAmount    string `json:"tip_amount"`
	NetAmount    string `json:"net_amount"`
	CashReceived string `json:"cash_received"`
	ChangeGiven  string `json:"change_given"`
	// Efecto físico por moneda; nil en movimientos anteriores a la 0007 o no efectivo.
	MXNIn     *string   `json:"mxn_in"`
	MXNOut    *string   `json:"mxn_out"`
	USDIn     *string   `json:"usd_in"`
	USDOut    *string   `json:"usd_out"`
	USDRate   *string   `json:"usd_rate"`
	AccountID string    `json:"account_id"`
	Folio     int64     `json:"folio"`
	PaymentID *string   `json:"payment_id"`
	RefundID  *string   `json:"refund_id"`
	ActorName string    `json:"actor_name"`
	CreatedAt time.Time `json:"created_at"`
}

type MethodTotal struct {
	MethodName string `json:"method_name"`
	IsCash     bool   `json:"is_cash"`
	Sales      string `json:"sales"`
	Tips       string `json:"tips"`
	Net        string `json:"net"`
}

// shiftMovements expone los movimientos de cobro persistidos del turno con sus totales por
// método. El efectivo esperado y los totales físicos por moneda (de los que se deduce) solo se
// incluyen con cash.expected.read y nunca durante el arqueo de cierre.
func (handler *service) shiftMovements(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT m.id,m.kind,m.method_name,m.is_cash,m.sale_amount::text,m.tip_amount::text,
        m.net_amount::text,m.cash_received::text,m.change_given::text,m.mxn_in::text,m.mxn_out::text,m.usd_in::text,m.usd_out::text,
        m.usd_rate::text,m.account_id,a.folio,m.payment_id,m.refund_id,u.display_name,m.created_at
        FROM app.cash_movements m JOIN app.restaurant_accounts a ON a.id=m.account_id JOIN app.users u ON u.id=m.actor_id
        WHERE m.shift_id=$1 ORDER BY m.created_at,m.id`, shift.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	movements := []Movement{}
	totals := map[string]*MethodTotal{}
	order := []string{}
	cashNet := zero
	currency := &currencyTotals{}
	for rows.Next() {
		var item Movement
		if err := rows.Scan(&item.ID, &item.Kind, &item.MethodName, &item.IsCash, &item.SaleAmount, &item.TipAmount, &item.NetAmount,
			&item.CashReceived, &item.ChangeGiven, &item.MXNIn, &item.MXNOut, &item.USDIn, &item.USDOut, &item.USDRate,
			&item.AccountID, &item.Folio, &item.PaymentID, &item.RefundID, &item.ActorName, &item.CreatedAt); err != nil {
			return nil, err
		}
		movements = append(movements, item)
		if item.IsCash {
			currency.add(item)
		}
		total, found := totals[item.MethodName]
		if !found {
			total = &MethodTotal{MethodName: item.MethodName, IsCash: item.IsCash, Sales: "0.00", Tips: "0.00", Net: "0.00"}
			totals[item.MethodName] = total
			order = append(order, item.MethodName)
		}
		total.Sales = money(decimal.RequireFromString(total.Sales).Add(decimal.RequireFromString(item.SaleAmount)))
		total.Tips = money(decimal.RequireFromString(total.Tips).Add(decimal.RequireFromString(item.TipAmount)))
		total.Net = money(decimal.RequireFromString(total.Net).Add(decimal.RequireFromString(item.NetAmount)))
		if item.IsCash {
			cashNet = cashNet.Add(decimal.RequireFromString(item.NetAmount))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byMethod := []MethodTotal{}
	for _, name := range order {
		byMethod = append(byMethod, *totals[name])
	}
	result := map[string]any{
		"shift": shift, "movements": movements, "by_method": byMethod, "cash_net": money(cashNet),
		"expected_visible": false,
	}
	if !handler.expectedVisible(shift) {
		return result, nil
	}
	// El esperado es el mismo que usan cortes y cierre: fondos, cobros en efectivo por moneda y
	// operaciones de efectivo del turno. expected_cash conserva el nombre de la etapa 6 y expresa
	// los pesos físicos esperados.
	expected, err := handler.expectedCash(shift)
	if err != nil {
		return nil, err
	}
	result["expected_visible"] = true
	result["expected"] = expected.public()
	result["expected_cash"] = money(expected.mxn)
	result["by_currency"] = currency.summary(expected)
	return result, nil
}

// currencyTotals acumula el efectivo físico del turno por moneda para el corte de la etapa 7.
type currencyTotals struct {
	mxnIn, mxnOut, usdIn, usdOut decimal.Decimal
}

// add suma el efecto físico de un movimiento de efectivo. Un movimiento sin columnas por moneda
// (anterior a la 0007) fue solo en pesos: entró lo recibido y salió lo recibido menos el neto
// (el cambio de un cobro o el importe de una devolución).
func (totals *currencyTotals) add(item Movement) {
	if item.MXNIn == nil {
		received := decimal.RequireFromString(item.CashReceived)
		totals.mxnIn = totals.mxnIn.Add(received)
		totals.mxnOut = totals.mxnOut.Add(received.Sub(decimal.RequireFromString(item.NetAmount)))
		return
	}
	totals.mxnIn = totals.mxnIn.Add(decimal.RequireFromString(*item.MXNIn))
	totals.mxnOut = totals.mxnOut.Add(decimal.RequireFromString(*item.MXNOut))
	totals.usdIn = totals.usdIn.Add(decimal.RequireFromString(*item.USDIn))
	totals.usdOut = totals.usdOut.Add(decimal.RequireFromString(*item.USDOut))
}

// summary expresa lo físico de los cobros por moneda y el efectivo esperado del turno en cada
// moneda (fondos y operaciones de efectivo incluidos), sin convertir dólares a pesos.
func (totals *currencyTotals) summary(expected cashExpectation) map[string]string {
	return map[string]string{
		"mxn_in":       money(totals.mxnIn),
		"mxn_out":      money(totals.mxnOut),
		"usd_in":       money(totals.usdIn),
		"usd_out":      money(totals.usdOut),
		"expected_mxn": money(expected.mxn),
		"expected_usd": money(expected.usd),
	}
}
