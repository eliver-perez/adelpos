package inventory

import (
	"github.com/shopspring/decimal"
)

// Presentaciones de compra de un insumo («Caja con 24», «Costal 25 kg»). La equivalencia se
// captura como cantidad en cualquier unidad de la misma dimensión que la unidad base (por omisión,
// la unidad base) y Go la guarda convertida a la unidad base. Una sola presentación activa puede
// ser la de compra predeterminada: al marcar otra, la anterior deja de serlo en la misma
// transacción. Las respuestas devuelven el insumo completo con todas sus presentaciones.

type presentationInput struct {
	OperationID     string  `json:"operation_id"`
	Revision        int     `json:"revision"`
	Name            string  `json:"name"`
	Quantity        string  `json:"quantity"`
	UnitID          *string `json:"unit_id"`
	Barcode         *string `json:"barcode"`
	Active          bool    `json:"active"`
	PurchaseDefault bool    `json:"purchase_default"`
}

// presentationValues son los datos validados de una presentación.
type presentationValues struct {
	baseQuantity decimal.Decimal
	barcode      *string
}

// validatePresentation valida nombre y código de barras y convierte la equivalencia a la unidad
// base del insumo.
func (handler *service) validatePresentation(input *presentationInput, item *Item) (presentationValues, error) {
	values := presentationValues{}
	if err := validateName(&input.Name, "El nombre de la presentación", 60); err != nil {
		return values, err
	}
	if input.PurchaseDefault && !input.Active {
		return values, invalid("La presentación de compra predeterminada debe estar activa.")
	}
	barcode, err := normalizeBarcode(input.Barcode)
	if err != nil {
		return values, err
	}
	values.barcode = barcode
	quantity, err := parseQuantity(input.Quantity, "La equivalencia de la presentación")
	if err != nil {
		return values, err
	}
	unitID, err := validateOptionalID(input.UnitID)
	if err != nil {
		return values, err
	}
	values.baseQuantity, err = handler.toBaseQuantity(item, quantity, unitID)
	if err != nil {
		return values, err
	}
	return values, nil
}

// toBaseQuantity convierte una cantidad capturada en una unidad (por omisión, la unidad base) a la
// unidad base del insumo. Solo se admiten unidades de la misma dimensión.
func (handler *service) toBaseQuantity(item *Item, quantity decimal.Decimal, unitID *string) (decimal.Decimal, error) {
	base, _, err := handler.unitMeasure(item.BaseUnit.ID, true)
	if err != nil {
		return zero, err
	}
	if unitID == nil || *unitID == item.BaseUnit.ID {
		return convertQuantity(quantity, base, base)
	}
	captured, _, err := handler.unitMeasure(*unitID, false)
	if err != nil {
		return zero, err
	}
	return convertQuantity(quantity, captured, base)
}

func presentationConflict(err error) error {
	if !uniqueViolation(err) {
		return err
	}
	if constraintName(err) == "inventory_presentations_barcode_key" {
		return conflict("DUPLICATE_BARCODE", "El código de barras ya pertenece a otro insumo o presentación.")
	}
	return conflict("DUPLICATE_NAME", "El insumo ya tiene una presentación con ese nombre.")
}

// clearOtherDefaults quita la marca de predeterminada a las demás presentaciones del insumo (con
// revisión nueva, para que otra terminal detecte el cambio).
func (handler *service) clearOtherDefaults(itemID, presentationID string) error {
	_, err := handler.tx.Exec(handler.ctx, `UPDATE app.inventory_presentations SET purchase_default=false,revision=revision+1,
        updated_at=now() WHERE item_id=$1 AND id<>$2 AND purchase_default`, itemID, presentationID)
	return err
}

func (handler *service) readPresentation(id string) (*Presentation, error) {
	presentation, err := scanPresentation(handler.tx.QueryRow(handler.ctx, presentationColumns+" WHERE id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La presentación no existe.")
	}
	return presentation, err
}

// createPresentation agrega una presentación al insumo de la ruta.
func (handler *service) createPresentation(parts []string) (any, error) {
	var input presentationInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	item, err := handler.readItem(parts[1])
	if err != nil {
		return nil, err
	}
	values, err := handler.validatePresentation(&input, item)
	if err != nil {
		return nil, err
	}
	if err := handler.barcodeTaken(values.barcode, "", ""); err != nil {
		return nil, err
	}
	id := newID()
	if input.PurchaseDefault {
		if err := handler.clearOtherDefaults(item.ID, id); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_presentations(id,item_id,name,base_quantity,barcode,active,
            purchase_default)
        VALUES ($1,$2,$3,$4::numeric,$5,$6,$7)`, id, item.ID, input.Name, values.baseQuantity.String(), values.barcode, input.Active,
		input.PurchaseDefault)
	if err != nil {
		return nil, presentationConflict(err)
	}
	saved, err := handler.readPresentation(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &item.ID); err != nil {
		return nil, err
	}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.presentation_created", "inventory_presentation", &id, nil, saved); err != nil {
		return nil, err
	}
	return handler.refreshedItem(item.ID)
}

// updatePresentation guarda una presentación con revisión optimista.
func (handler *service) updatePresentation(parts []string) (any, error) {
	var input presentationInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.readPresentation(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La presentación")
	}
	item, err := handler.readItem(previous.ItemID)
	if err != nil {
		return nil, err
	}
	values, err := handler.validatePresentation(&input, item)
	if err != nil {
		return nil, err
	}
	if err := handler.barcodeTaken(values.barcode, "", previous.ID); err != nil {
		return nil, err
	}
	if input.PurchaseDefault {
		if err := handler.clearOtherDefaults(item.ID, previous.ID); err != nil {
			return nil, err
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_presentations SET name=$2,base_quantity=$3::numeric,barcode=$4,
            active=$5,purchase_default=$6,revision=revision+1,updated_at=now()
        WHERE id=$1`, previous.ID, input.Name, values.baseQuantity.String(), values.barcode, input.Active, input.PurchaseDefault)
	if err != nil {
		return nil, presentationConflict(err)
	}
	saved, err := handler.readPresentation(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &item.ID); err != nil {
		return nil, err
	}
	err = handler.audit(handler.ctx, handler.tx, "inventory.presentation_updated", "inventory_presentation", &saved.ID, previous, saved)
	if err != nil {
		return nil, err
	}
	return handler.refreshedItem(item.ID)
}

// refreshedItem devuelve el insumo con sus presentaciones vigentes, sin costos si no corresponde.
func (handler *service) refreshedItem(itemID string) (any, error) {
	item, err := handler.readItem(itemID)
	if err != nil {
		return nil, err
	}
	return handler.present(item), nil
}
