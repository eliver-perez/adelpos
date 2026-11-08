package payments

import (
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
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
}

const shiftQuery = `SELECT s.id,s.register_id,r.name,s.state,s.opening_float::text,s.opened_by,u.display_name,d.name,
    s.opened_at,s.closed_at,s.revision
    FROM app.cash_shifts s JOIN app.registers r ON r.id=s.register_id
    JOIN app.users u ON u.id=s.opened_by JOIN app.devices d ON d.id=s.opened_device_id`

func (handler *service) readShift(where string, argument any) (*Shift, error) {
	var shift Shift
	err := handler.tx.QueryRow(handler.ctx, shiftQuery+" WHERE "+where, argument).Scan(&shift.ID, &shift.RegisterID, &shift.RegisterName,
		&shift.State, &shift.OpeningFloat, &shift.OpenedBy, &shift.OpenedByName, &shift.OpenedDevice, &shift.OpenedAt, &shift.ClosedAt, &shift.Revision)
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
	result := map[string]any{"register": register, "shift": nil, "shared_shifts": settings.SharedShifts, "usable": false, "reason": ""}
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
		OperationID  string `json:"operation_id"`
		OpeningFloat string `json:"opening_float"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	// El fondo inicial es obligatorio, aunque puede ser $0.
	opening, err := parseAmount(input.OpeningFloat, "fondo inicial")
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
	// El lock operativo serializa las aperturas; el índice único parcial impide duplicados aunque falle esa serialización.
	existing, err := handler.readShift("s.register_id=$1 AND s.state='open'", register.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, conflict("SHIFT_ALREADY_OPEN", "La caja "+register.Name+" ya tiene un turno abierto por "+existing.OpenedByName+".")
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_shifts(id,register_id,opening_float,opened_by,opened_device_id,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6)`, id, register.ID, money(opening), handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
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

type Movement struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	MethodName   string    `json:"method_name"`
	IsCash       bool      `json:"is_cash"`
	SaleAmount   string    `json:"sale_amount"`
	TipAmount    string    `json:"tip_amount"`
	NetAmount    string    `json:"net_amount"`
	CashReceived string    `json:"cash_received"`
	ChangeGiven  string    `json:"change_given"`
	AccountID    string    `json:"account_id"`
	Folio        int64     `json:"folio"`
	PaymentID    *string   `json:"payment_id"`
	RefundID     *string   `json:"refund_id"`
	ActorName    string    `json:"actor_name"`
	CreatedAt    time.Time `json:"created_at"`
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
        m.net_amount::text,m.cash_received::text,m.change_given::text,m.account_id,a.folio,m.payment_id,m.refund_id,u.display_name,m.created_at
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
	for rows.Next() {
		var item Movement
		if err := rows.Scan(&item.ID, &item.Kind, &item.MethodName, &item.IsCash, &item.SaleAmount, &item.TipAmount, &item.NetAmount,
			&item.CashReceived, &item.ChangeGiven, &item.AccountID, &item.Folio, &item.PaymentID, &item.RefundID, &item.ActorName, &item.CreatedAt); err != nil {
			return nil, err
		}
		movements = append(movements, item)
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
	}, nil
}
