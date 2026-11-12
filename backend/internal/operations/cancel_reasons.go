package operations

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"local/ad-el-pos/server/internal/ordering"
)

// Motivos de cancelación de cuentas (etapa 9D, §5.2). Catálogo configurable sembrado en la 0021 con
// «Cliente se retiró sin pagar», «Cliente canceló el pedido», «Error de captura» y «Otro»; solo «Otro»
// exige descripción y solo el primero marca lo ya enviado a cocina como merma. Se administra con el
// mismo permiso que los motivos de descuento (payments.settings.manage), para que ambos catálogos de
// motivos vivan juntos en Administración, y cada cambio se audita en la misma transacción. Renombrar o
// desactivar un motivo nunca modifica las cancelaciones registradas: conservan su reason_id, el
// nombre y la marca de merma que tenía el motivo al cancelar.

// cancelReasonsPermission administra el catálogo de motivos de cancelación.
const cancelReasonsPermission = "payments.settings.manage"

// CancelReason es un motivo del catálogo.
type CancelReason struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	RequiresDescription bool      `json:"requires_description"`
	PreparedIsWaste     bool      `json:"prepared_is_waste"`
	Active              bool      `json:"active"`
	SortOrder           int       `json:"sort_order"`
	Revision            int       `json:"revision"`
	UpdatedAt           time.Time `json:"updated_at"`
}

const cancelReasonColumns = `SELECT id,name,requires_description,prepared_is_waste,active,sort_order,revision,updated_at
    FROM app.account_cancel_reasons`

func scanCancelReason(row interface{ Scan(...any) error }) (*CancelReason, error) {
	var reason CancelReason
	err := row.Scan(&reason.ID, &reason.Name, &reason.RequiresDescription, &reason.PreparedIsWaste, &reason.Active,
		&reason.SortOrder, &reason.Revision, &reason.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &reason, nil
}

func (handler *service) readCancelReason(id string) (*CancelReason, error) {
	reason, err := scanCancelReason(handler.tx.QueryRow(handler.ctx, cancelReasonColumns+" WHERE id=$1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "El motivo de cancelación no existe."}
	}
	return reason, err
}

// cancelReasons atiende GET y POST restaurant/cancel-reasons y PATCH restaurant/cancel-reasons/{id}.
// Los permisos ya se comprobaron en routePermission antes de cualquier escritura.
func (handler *service) cancelReasons(parts []string) (any, error) {
	switch {
	case len(parts) == 1 && handler.request.Method == "GET":
		return handler.listCancelReasons()
	case len(parts) == 1 && handler.request.Method == "POST":
		return handler.createCancelReason()
	case len(parts) == 2 && handler.request.Method == "PATCH":
		return handler.updateCancelReason(parts[1])
	}
	return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La ruta operativa no existe."}
}

// listCancelReasons devuelve todos los motivos en su orden visual, activos e inactivos: «Cancelar
// cuenta» ofrece solo los activos y la administración los muestra todos.
func (handler *service) listCancelReasons() (any, error) {
	rows, err := handler.tx.Query(handler.ctx, cancelReasonColumns+" ORDER BY sort_order,name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reasons := []CancelReason{}
	for rows.Next() {
		reason, err := scanCancelReason(rows)
		if err != nil {
			return nil, err
		}
		reasons = append(reasons, *reason)
	}
	return reasons, rows.Err()
}

type cancelReasonInput struct {
	OperationID         string             `json:"operation_id"`
	Revision            int                `json:"revision"`
	Name                string             `json:"name"`
	RequiresDescription bool               `json:"requires_description"`
	PreparedIsWaste     bool               `json:"prepared_is_waste"`
	Active              bool               `json:"active"`
	Position            *ordering.Position `json:"position"`
}

// validate exige un nombre de 1 a 60 caracteres sin saltos de línea ni caracteres de control: el
// motivo se muestra en reportes, en el historial de la cuenta y en el aviso de cancelación de cocina.
func (input *cancelReasonInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	length := utf8.RuneCountInString(input.Name)
	if length < 1 || length > 60 {
		return invalid("El nombre del motivo debe tener entre 1 y 60 caracteres.")
	}
	controls := strings.IndexFunc(input.Name, func(character rune) bool {
		return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
	})
	if controls >= 0 {
		return invalid("El nombre del motivo no admite saltos de línea ni caracteres de control.")
	}
	return nil
}

func duplicateCancelReason() error {
	return &Error{Status: 409, Code: "DUPLICATE_NAME", Message: "Ya existe un motivo de cancelación con ese nombre."}
}

// requireUniqueCancelReason comprueba el nombre antes de escribir, para responder un conflicto claro
// sin abortar la transacción con la restricción única.
func (handler *service) requireUniqueCancelReason(name, id string) error {
	var taken bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.account_cancel_reasons
        WHERE name=$1 AND id::text<>$2)`, name, id).Scan(&taken)
	if err != nil {
		return err
	}
	if taken {
		return duplicateCancelReason()
	}
	return nil
}

func uniqueViolation(err error) bool {
	var problem *pgconn.PgError
	return errors.As(err, &problem) && problem.Code == "23505"
}

// placeCancelReason aplica la posición elegida en la misma transacción y audita la renumeración de
// los demás motivos.
func (handler *service) placeCancelReason(input cancelReasonInput, id string, joining bool) error {
	if input.Position == nil {
		return nil
	}
	after, err := ordering.Validate(input.Position, id)
	if err != nil {
		return invalid(err.Error())
	}
	result, err := ordering.Place(handler.ctx, handler.tx, ordering.AccountCancelReasons, "", id, after, joining)
	var problem *ordering.Error
	if errors.As(err, &problem) {
		return invalid(problem.Message)
	}
	if err != nil {
		return err
	}
	if len(result.Changes) == 0 {
		return nil
	}
	return handler.audit(handler.ctx, handler.tx, "restaurant.cancel_reasons_reordered", "account_cancel_reason", &id, nil, map[string]any{
		"placed_id": id, "after": after, "sort_order": result.SortOrder, "changes": result.Changes,
	})
}

// requireActiveCancelReason impide quedarse sin motivos activos: «Cancelar cuenta» exige elegir uno.
func (handler *service) requireActiveCancelReason() error {
	var active int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.account_cancel_reasons WHERE active").Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return &Error{Status: 409, Code: "LAST_ACTIVE_REASON", Message: "Debe quedar al menos un motivo de cancelación activo."}
	}
	return nil
}

// createCancelReason agrega un motivo al final de la lista (o en la posición indicada).
func (handler *service) createCancelReason() (any, error) {
	var input cancelReasonInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	if input.Position != nil {
		if _, err := ordering.Validate(input.Position, ""); err != nil {
			return nil, invalid(err.Error())
		}
	}
	if err := handler.requireUniqueCancelReason(input.Name, ""); err != nil {
		return nil, err
	}
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.account_cancel_reasons(id,name,requires_description,prepared_is_waste,
            active,sort_order)
        VALUES ($1,$2,$3,$4,$5,(SELECT coalesce(max(sort_order),0)+10 FROM app.account_cancel_reasons))`,
		id, input.Name, input.RequiresDescription, input.PreparedIsWaste, input.Active)
	if uniqueViolation(err) {
		return nil, duplicateCancelReason()
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeCancelReason(input, id, true); err != nil {
		return nil, err
	}
	if err := handler.requireActiveCancelReason(); err != nil {
		return nil, err
	}
	saved, err := handler.readCancelReason(id)
	if err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "restaurant.cancel_reason_created", "account_cancel_reason", &id, nil, saved)
}

// updateCancelReason renombra, activa o desactiva un motivo, cambia si exige descripción, si lo
// enviado a cocina cuenta como merma o su posición. Las cancelaciones registradas no cambian.
func (handler *service) updateCancelReason(id string) (any, error) {
	var input cancelReasonInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := handler.readCancelReason(id)
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, &Error{Status: 409, Code: "REVISION_CONFLICT", Message: "El motivo cambió en otra terminal. Actualiza y vuelve a guardar."}
	}
	if err := handler.requireUniqueCancelReason(input.Name, previous.ID); err != nil {
		return nil, err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.account_cancel_reasons SET name=$2,requires_description=$3,
        prepared_is_waste=$4,active=$5,revision=revision+1,updated_at=now() WHERE id=$1`,
		previous.ID, input.Name, input.RequiresDescription, input.PreparedIsWaste, input.Active)
	if uniqueViolation(err) {
		return nil, duplicateCancelReason()
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeCancelReason(input, previous.ID, false); err != nil {
		return nil, err
	}
	if err := handler.requireActiveCancelReason(); err != nil {
		return nil, err
	}
	saved, err := handler.readCancelReason(previous.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "restaurant.cancel_reason_updated", "account_cancel_reason", &saved.ID, previous, saved)
}
