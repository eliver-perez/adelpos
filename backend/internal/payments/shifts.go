package payments

import (
	"strings"
	"time"

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
	ID            string     `json:"id"`
	RegisterID    string     `json:"register_id"`
	RegisterName  string     `json:"register_name"`
	State         string     `json:"state"`
	OpeningFloat  string     `json:"opening_float"`
	OpenedBy      string     `json:"opened_by"`
	OpenedByName  string     `json:"opened_by_name"`
	OpenedDevice  string     `json:"opened_device_name"`
	OpenedAt      time.Time  `json:"opened_at"`
	ClosedAt      *time.Time `json:"closed_at"`
	Revision      int        `json:"revision"`
	SharedAllowed bool       `json:"shared_allowed"`
	// Tipo de cambio fijo del turno (MXN por USD); nil mientras no se captura.
	USDRate          *string    `json:"usd_rate"`
	USDRateSetByName *string    `json:"usd_rate_set_by_name"`
	USDRateSetAt     *time.Time `json:"usd_rate_set_at"`
}

const shiftQuery = `SELECT s.id,s.register_id,r.name,s.state,s.opening_float::text,s.opened_by,u.display_name,d.name,
    s.opened_at,s.closed_at,s.revision,s.usd_rate::text,setter.display_name,s.usd_rate_set_at
    FROM app.cash_shifts s JOIN app.registers r ON r.id=s.register_id
    JOIN app.users u ON u.id=s.opened_by JOIN app.devices d ON d.id=s.opened_device_id
    LEFT JOIN app.users setter ON setter.id=s.usd_rate_set_by`

func (handler *service) readShift(where string, argument any) (*Shift, error) {
	var shift Shift
	err := handler.tx.QueryRow(handler.ctx, shiftQuery+" WHERE "+where, argument).Scan(&shift.ID, &shift.RegisterID, &shift.RegisterName,
		&shift.State, &shift.OpeningFloat, &shift.OpenedBy, &shift.OpenedByName, &shift.OpenedDevice, &shift.OpenedAt, &shift.ClosedAt, &shift.Revision,
		&shift.USDRate, &shift.USDRateSetByName, &shift.USDRateSetAt)
	if isNoRows(err) {
		return nil, nil
	}
	return &shift, err
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
	shift, err := handler.readShift("s.register_id=$1 AND s.state='open'", register.ID)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		result["reason"] = "No hay un turno abierto en " + register.Name + ". Ábrelo con su fondo inicial antes de cobrar."
		return result, nil
	}
	shift.SharedAllowed = settings.SharedShifts
	result["shift"] = shift
	result["usd_rate_required"] = usdCash && shift.USDRate == nil
	if !settings.SharedShifts && shift.OpenedBy != handler.request.Actor.UserID {
		result["reason"] = "El turno abierto pertenece a " + shift.OpenedByName + " y la configuración no permite compartirlo."
		return result, nil
	}
	result["usable"] = true
	return result, nil
}

// usableShift exige caja asociada y turno abierto antes de registrar cualquier cobro o devolución.
// Nunca se abre un turno automáticamente con un fondo inventado.
func (handler *service) usableShift() (*Shift, error) {
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	if register == nil {
		return nil, conflict("REGISTER_REQUIRED", "Esta terminal no tiene una caja física asignada. Asígnala desde Terminales antes de cobrar.")
	}
	shift, err := handler.readShift("s.register_id=$1 AND s.state='open'", register.ID)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, conflict("SHIFT_REQUIRED", "No hay un turno abierto en "+register.Name+". Abre el turno con su fondo inicial antes de cobrar.")
	}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if !settings.SharedShifts && shift.OpenedBy != handler.request.Actor.UserID {
		return nil, conflict("SHIFT_NOT_SHARED", "El turno abierto pertenece a "+shift.OpenedByName+" y la configuración del establecimiento no permite compartirlo.")
	}
	return shift, nil
}

func (handler *service) openShift(_ []string) (any, error) {
	var input struct {
		OperationID           string `json:"operation_id"`
		OpeningFloat          string `json:"opening_float"`
		USDRate               string `json:"usd_rate"`
		ConfirmRateDifference bool   `json:"confirm_rate_difference"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	// El fondo inicial es obligatorio, aunque puede ser $0.
	opening, err := parseAmount(input.OpeningFloat, "fondo inicial")
	if err != nil {
		return nil, err
	}
	// El tipo de cambio es opcional al abrir; si viene, exige la función activa y su permiso, y se
	// valida por completo antes de escribir.
	var usdRate *string
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
		usdRate = &formatted
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
	// El lock operativo serializa las aperturas; el índice único parcial impide duplicados aunque falle esa serialización.
	existing, err := handler.readShift("s.register_id=$1 AND s.state='open'", register.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, conflict("SHIFT_ALREADY_OPEN", "La caja "+register.Name+" ya tiene un turno abierto por "+existing.OpenedByName+".")
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id,
        usd_rate,usd_rate_set_by,usd_rate_set_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,CASE WHEN $7::numeric IS NULL THEN NULL ELSE $4::uuid END,
        CASE WHEN $7::numeric IS NULL THEN NULL ELSE now() END)`,
		id, register.ID, money(opening), handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID, usdRate)
	if err != nil {
		return nil, err
	}
	shift, err := handler.readShift("s.id=$1", id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.opened", "configuration", &id, nil); err != nil {
		return nil, err
	}
	return shift, handler.audit(handler.ctx, handler.tx, "cash.shift_opened", "cash_shift", &id, nil, shift)
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

func formatPesos(value decimal.Decimal) string {
	return "$" + money(value)
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
	if shift.State != "open" {
		return nil, conflict("SHIFT_CLOSED", "El turno ya está cerrado; su tipo de cambio no puede capturarse.")
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
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.usd_rate_set", "configuration", &shift.ID, nil); err != nil {
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

// shiftMovements expone los movimientos persistidos del turno, base de los cortes de la etapa 7.
func (handler *service) shiftMovements(parts []string) (any, error) {
	shift, err := handler.readShift("s.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
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
	// Efectivo esperado = fondo inicial + efectivo neto cobrado: el cambio nunca suma dos veces.
	expectedCash := decimal.RequireFromString(shift.OpeningFloat).Add(cashNet)
	return map[string]any{
		"shift": shift, "movements": movements, "by_method": byMethod,
		"cash_net": money(cashNet), "expected_cash": money(expectedCash),
		"by_currency": currency.summary(decimal.RequireFromString(shift.OpeningFloat)),
	}, nil
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

// summary expresa lo físico por moneda. El efectivo esperado en pesos incluye el fondo inicial;
// los dólares se cuentan aparte, sin convertir. No es un arqueo: el cierre pertenece a la etapa 7.
func (totals *currencyTotals) summary(opening decimal.Decimal) map[string]string {
	return map[string]string{
		"mxn_in":       money(totals.mxnIn),
		"mxn_out":      money(totals.mxnOut),
		"usd_in":       money(totals.usdIn),
		"usd_out":      money(totals.usdOut),
		"expected_mxn": money(opening.Add(totals.mxnIn).Sub(totals.mxnOut)),
		"expected_usd": money(totals.usdIn.Sub(totals.usdOut)),
	}
}
