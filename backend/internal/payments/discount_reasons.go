package payments

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/ordering"
)

// Motivos de descuento (etapa 7.1, decisión 15). Catálogo configurable sembrado en la 0009 con
// Atención, Promoción, Demora, Cortesía de la casa y Otro; solo Otro exige descripción. Se
// administra con payments.settings.manage y cada cambio se audita en la misma transacción.
// Desactivar o renombrar un motivo nunca modifica los descuentos registrados: conservan su
// reason_id y el texto de su motivo.

type DiscountReason struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	RequiresDescription bool      `json:"requires_description"`
	Active              bool      `json:"active"`
	SortOrder           int       `json:"sort_order"`
	Revision            int       `json:"revision"`
	UpdatedAt           time.Time `json:"updated_at"`
}

const discountReasonColumns = `SELECT id,name,requires_description,active,sort_order,revision,updated_at FROM app.discount_reasons`

func scanDiscountReason(row interface{ Scan(...any) error }) (*DiscountReason, error) {
	var reason DiscountReason
	err := row.Scan(&reason.ID, &reason.Name, &reason.RequiresDescription, &reason.Active, &reason.SortOrder,
		&reason.Revision, &reason.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &reason, nil
}

func (handler *service) readDiscountReason(id string) (*DiscountReason, error) {
	reason, err := scanDiscountReason(handler.tx.QueryRow(handler.ctx, discountReasonColumns+" WHERE id=$1", id))
	if isNoRows(err) {
		return nil, notFound("El motivo de descuento no existe.")
	}
	return reason, err
}

// listDiscountReasons devuelve todos los motivos en su orden visual, activos e inactivos: el
// formulario de descuento ofrece solo los activos y la administración los muestra todos.
func (handler *service) listDiscountReasons(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, discountReasonColumns+" ORDER BY sort_order,name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reasons := []DiscountReason{}
	for rows.Next() {
		reason, err := scanDiscountReason(rows)
		if err != nil {
			return nil, err
		}
		reasons = append(reasons, *reason)
	}
	return reasons, rows.Err()
}

type discountReasonInput struct {
	OperationID         string             `json:"operation_id"`
	Revision            int                `json:"revision"`
	Name                string             `json:"name"`
	RequiresDescription bool               `json:"requires_description"`
	Active              bool               `json:"active"`
	Position            *ordering.Position `json:"position"`
}

// hasControlCharacters detecta saltos de línea y caracteres invisibles: el nombre se imprime en
// el ticket y la precuenta.
func hasControlCharacters(value string) bool {
	return strings.IndexFunc(value, func(character rune) bool {
		return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
	}) >= 0
}

func (input *discountReasonInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	if textLength(input.Name) < 1 || textLength(input.Name) > 60 {
		return invalid("El nombre del motivo debe tener entre 1 y 60 caracteres.")
	}
	if hasControlCharacters(input.Name) {
		return invalid("El nombre del motivo no admite saltos de línea ni caracteres de control.")
	}
	return nil
}

// placeDiscountReason aplica la posición elegida en la misma transacción y audita la
// renumeración de los demás motivos.
func (handler *service) placeDiscountReason(input discountReasonInput, id string, joining bool) error {
	if input.Position == nil {
		return nil
	}
	after, err := ordering.Validate(input.Position, id)
	if err != nil {
		return invalid(err.Error())
	}
	result, err := ordering.Place(handler.ctx, handler.tx, ordering.DiscountReasons, "", id, after, joining)
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
	return handler.audit(handler.ctx, handler.tx, "payments.discount_reasons_reordered", "discount_reason", &id, nil, map[string]any{
		"placed_id": id, "after": after, "sort_order": result.SortOrder, "changes": result.Changes,
	})
}

// requireActiveDiscountReason impide quedarse sin motivos activos: el formulario de descuento
// exige elegir uno.
func (handler *service) requireActiveDiscountReason() error {
	var active int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.discount_reasons WHERE active").Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return conflict("LAST_ACTIVE_REASON", "Debe quedar al menos un motivo de descuento activo.")
	}
	return nil
}

// createDiscountReason agrega un motivo al final de la lista (o en la posición indicada).
func (handler *service) createDiscountReason(_ []string) (any, error) {
	var input discountReasonInput
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
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.discount_reasons(id,name,requires_description,active,sort_order)
        VALUES ($1,$2,$3,$4,(SELECT coalesce(max(sort_order),0)+10 FROM app.discount_reasons))`,
		id, input.Name, input.RequiresDescription, input.Active)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe un motivo de descuento con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeDiscountReason(input, id, true); err != nil {
		return nil, err
	}
	if err := handler.requireActiveDiscountReason(); err != nil {
		return nil, err
	}
	saved, err := handler.readDiscountReason(id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.discount_reasons.changed", "configuration", &id, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.discount_reason_created", "discount_reason", &id, nil, saved)
}

// updateDiscountReason renombra, activa o desactiva un motivo, cambia si exige descripción o su
// posición. Los descuentos registrados con él no cambian.
func (handler *service) updateDiscountReason(parts []string) (any, error) {
	var input discountReasonInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := handler.readDiscountReason(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "El motivo cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.discount_reasons SET name=$2,requires_description=$3,active=$4,
        revision=revision+1,updated_at=now() WHERE id=$1`, previous.ID, input.Name, input.RequiresDescription, input.Active)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe un motivo de descuento con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeDiscountReason(input, previous.ID, false); err != nil {
		return nil, err
	}
	if err := handler.requireActiveDiscountReason(); err != nil {
		return nil, err
	}
	saved, err := handler.readDiscountReason(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.discount_reasons.changed", "configuration", &saved.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "payments.discount_reason_updated", "discount_reason", &saved.ID, previous, saved)
}

// discountReasonChoice es el motivo elegido para un descuento ya validado.
type discountReasonChoice struct {
	id     *string
	name   string
	reason string
}

// snapshotName es el nombre del motivo que se guarda con el descuento (reason_name): nil sin motivo
// del catálogo. Lo registrado conserva el nombre aunque el motivo se renombre después.
func (choice discountReasonChoice) snapshotName() *string {
	if choice.id == nil {
		return nil
	}
	name := choice.name
	return &name
}

// resolveDiscountReason aplica las reglas de la decisión 15 antes de escribir:
//   - sin reason_id (compatibilidad con la etapa 6), el motivo escrito es obligatorio de 5 a 500
//     caracteres y el descuento se guarda sin motivo del catálogo, como los registros anteriores;
//   - con reason_id, el motivo debe existir y estar activo; la descripción es obligatoria (5–500)
//     si el motivo lo exige (Otro) y opcional (hasta 500) en otro caso.
func (handler *service) resolveDiscountReason(reasonID *string, text string) (discountReasonChoice, error) {
	description := strings.TrimSpace(text)
	if reasonID == nil || *reasonID == "" {
		if textLength(description) < 5 || textLength(description) > 500 {
			return discountReasonChoice{}, invalid("Indica el motivo del descuento (5–500 caracteres).")
		}
		return discountReasonChoice{reason: description}, nil
	}
	if err := validateID(*reasonID); err != nil {
		return discountReasonChoice{}, err
	}
	reason, err := handler.readDiscountReason(*reasonID)
	if err != nil {
		return discountReasonChoice{}, err
	}
	if !reason.Active {
		return discountReasonChoice{}, conflict("DISCOUNT_REASON_INACTIVE", "El motivo «"+reason.Name+"» está desactivado. Elige otro motivo.")
	}
	if reason.RequiresDescription {
		if textLength(description) < 5 || textLength(description) > 500 {
			return discountReasonChoice{}, invalid("El motivo «" + reason.Name + "» requiere una descripción de 5 a 500 caracteres.")
		}
	} else if textLength(description) > 500 {
		return discountReasonChoice{}, invalid("La descripción del motivo admite hasta 500 caracteres.")
	}
	id := reason.ID
	return discountReasonChoice{id: &id, name: reason.Name, reason: description}, nil
}
