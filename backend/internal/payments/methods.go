package payments

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/events"
)

type Method struct {
	ID            string    `json:"id"`
	Code          *string   `json:"code"`
	Name          string    `json:"name"`
	Icon          string    `json:"icon"`
	Active        bool      `json:"active"`
	SortOrder     int       `json:"sort_order"`
	IsCash        bool      `json:"is_cash"`
	ReferenceMode string    `json:"reference_mode"`
	AllowsTip     bool      `json:"allows_tip"`
	AllowsRefund  bool      `json:"allows_refund"`
	Revision      int       `json:"revision"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const methodColumns = "id,code,name,icon,active,sort_order,is_cash,reference_mode,allows_tip,allows_refund,revision,updated_at"

var methodIcons = []string{"cash", "card", "transfer", "platform", "voucher", "other"}

func (handler *service) readMethod(id string) (*Method, error) {
	var method Method
	err := handler.tx.QueryRow(handler.ctx, "SELECT "+methodColumns+" FROM app.payment_methods WHERE id=$1", id).Scan(
		&method.ID, &method.Code, &method.Name, &method.Icon, &method.Active, &method.SortOrder, &method.IsCash,
		&method.ReferenceMode, &method.AllowsTip, &method.AllowsRefund, &method.Revision, &method.UpdatedAt)
	if isNoRows(err) {
		return nil, notFound("El método de pago no existe.")
	}
	return &method, err
}

func (handler *service) listMethods(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, "SELECT "+methodColumns+" FROM app.payment_methods ORDER BY sort_order,name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	methods := []Method{}
	for rows.Next() {
		var method Method
		if err := rows.Scan(&method.ID, &method.Code, &method.Name, &method.Icon, &method.Active, &method.SortOrder, &method.IsCash,
			&method.ReferenceMode, &method.AllowsTip, &method.AllowsRefund, &method.Revision, &method.UpdatedAt); err != nil {
			return nil, err
		}
		methods = append(methods, method)
	}
	return methods, rows.Err()
}

type methodInput struct {
	OperationID   string `json:"operation_id"`
	Revision      int    `json:"revision"`
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	Active        bool   `json:"active"`
	SortOrder     int    `json:"sort_order"`
	IsCash        bool   `json:"is_cash"`
	ReferenceMode string `json:"reference_mode"`
	AllowsTip     bool   `json:"allows_tip"`
	AllowsRefund  bool   `json:"allows_refund"`
}

func (input *methodInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	if textLength(input.Name) < 1 || textLength(input.Name) > 60 {
		return invalid("El nombre del método debe tener entre 1 y 60 caracteres.")
	}
	if !slices.Contains(methodIcons, input.Icon) {
		return invalid("Selecciona un icono válido para el método.")
	}
	if input.ReferenceMode != "none" && input.ReferenceMode != "optional" && input.ReferenceMode != "required" {
		return invalid("La referencia debe ser no admitida, opcional u obligatoria.")
	}
	// El efectivo entrega cambio y no requiere referencia externa.
	if input.IsCash && input.ReferenceMode != "none" {
		return invalid("Un método de efectivo no admite referencia electrónica.")
	}
	if input.SortOrder < 0 || input.SortOrder > 10000 {
		return invalid("El orden debe estar entre 0 y 10000.")
	}
	return nil
}

func duplicateName(err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == "23505"
}

func (handler *service) createMethod(_ []string) (any, error) {
	var input methodInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_methods(id,name,icon,active,sort_order,is_cash,reference_mode,allows_tip,allows_refund)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, input.Name, input.Icon, input.Active, input.SortOrder, input.IsCash,
		input.ReferenceMode, input.AllowsTip, input.AllowsRefund)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe un método de pago con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	saved, err := handler.readMethod(id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.methods.changed", "configuration", &id, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.method_created", "payment_method", &id, nil, saved)
}

// updateMethod conserva la identidad del método; los pagos históricos guardan su propio nombre,
// por lo que renombrarlo no altera registros anteriores.
func (handler *service) updateMethod(parts []string) (any, error) {
	var input methodInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := handler.readMethod(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "El método cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.payment_methods SET name=$1,icon=$2,active=$3,sort_order=$4,is_cash=$5,
        reference_mode=$6,allows_tip=$7,allows_refund=$8,revision=revision+1,updated_at=now() WHERE id=$9`,
		input.Name, input.Icon, input.Active, input.SortOrder, input.IsCash, input.ReferenceMode, input.AllowsTip, input.AllowsRefund, previous.ID)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe un método de pago con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	saved, err := handler.readMethod(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.methods.changed", "configuration", &previous.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.method_updated", "payment_method", &previous.ID, previous, saved)
}
