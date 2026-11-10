package inventory

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// Renglones de compra: la cantidad se captura en una presentación del insumo («20 Caja con 24») o
// en una unidad de su dimensión («5 kg» de un insumo en gramos; por omisión, su unidad base) y se
// guarda también convertida a la unidad base al registrarse, de modo que cambiar después una
// presentación o una unidad no altera la historia.

// purchaseLineInput es un renglón capturado de una orden o de una recepción directa.
type purchaseLineInput struct {
	ItemID         string  `json:"item_id"`
	PresentationID *string `json:"presentation_id"`
	UnitID         *string `json:"unit_id"`
	Quantity       string  `json:"quantity"`
	UnitPrice      string  `json:"unit_price"`
	Discount       *string `json:"discount"`
	TaxRate        *int    `json:"tax_rate"`
}

// purchaseCapture es una cantidad capturada con su conversión a la unidad base.
type purchaseCapture struct {
	item           *Item
	presentationID *string
	unitID         *string
	label          string
	quantity       decimal.Decimal
	base           decimal.Decimal
}

// preparedPurchaseLine es un renglón validado con sus importes.
type preparedPurchaseLine struct {
	capture   purchaseCapture
	unitPrice decimal.Decimal
	taxRate   int
	amounts   PurchaseAmounts
	// orderLineNo es el renglón de la orden que recibe (solo en recepciones de una orden).
	orderLineNo *int
	// excess es la parte recibida por encima de lo pendiente de la orden (con allow_excess).
	excess decimal.Decimal
}

// resolveCapture valida insumo, presentación o unidad y cantidad, y convierte a la unidad base.
// allowInactive admite un insumo, una presentación o una unidad desactivados después de que una
// orden emitida los comprometió (la mercancía en camino se debe poder recibir).
func (handler *service) resolveCapture(itemID string, presentationID, unitID *string, quantityText, prefix string,
	allowInactive bool) (purchaseCapture, error) {
	if err := validateID(itemID); err != nil {
		return purchaseCapture{}, invalid(prefix + "elige el insumo.")
	}
	item, err := handler.readItem(strings.ToLower(itemID))
	if err != nil {
		return purchaseCapture{}, err
	}
	if !item.Active && !allowInactive {
		return purchaseCapture{}, conflict("ITEM_INACTIVE", "El insumo «"+item.Name+"» está desactivado.")
	}
	quantity, err := parseQuantity(quantityText, prefix+"la cantidad")
	if err != nil {
		return purchaseCapture{}, err
	}
	capture := purchaseCapture{item: item, quantity: quantity}
	if presentationID != nil && *presentationID != "" && unitID != nil && *unitID != "" {
		return purchaseCapture{}, invalid(prefix + "captura la cantidad en una unidad o en una presentación, no en ambas.")
	}
	if presentationID != nil && *presentationID != "" {
		normalized, err := validateOptionalID(presentationID)
		if err != nil || normalized == nil {
			return purchaseCapture{}, invalid(prefix + "la presentación no es válida.")
		}
		for _, presentation := range item.Presentations {
			if presentation.ID != *normalized {
				continue
			}
			if !presentation.Active && !allowInactive {
				return purchaseCapture{}, conflict("PRESENTATION_INACTIVE", "La presentación «"+presentation.Name+"» está desactivada.")
			}
			base, err := presentationQuantity(quantity, parseDatabaseDecimal(presentation.BaseQuantity))
			if err != nil {
				return purchaseCapture{}, err
			}
			capture.presentationID, capture.label, capture.base = normalized, presentation.Name, base
			return capture, nil
		}
		return purchaseCapture{}, invalid(prefix + "la presentación no pertenece al insumo «" + item.Name + "».")
	}
	normalized, err := validateOptionalID(unitID)
	if err != nil {
		return purchaseCapture{}, invalid(prefix + "la unidad no es válida.")
	}
	if normalized == nil || *normalized == item.BaseUnit.ID {
		base := item.BaseUnit.ID
		capture.unitID, capture.label, capture.base = &base, item.BaseUnit.Name, quantity
		return capture, nil
	}
	target, _, err := handler.unitMeasure(item.BaseUnit.ID, true)
	if err != nil {
		return purchaseCapture{}, err
	}
	captured, name, err := handler.unitMeasure(*normalized, allowInactive)
	if err != nil {
		return purchaseCapture{}, err
	}
	converted, err := convertQuantity(quantity, captured, target)
	if err != nil {
		return purchaseCapture{}, err
	}
	capture.unitID, capture.label, capture.base = normalized, name, converted
	return capture, nil
}

// preparePurchaseLine valida un renglón capturado (orden o recepción directa) y calcula sus importes.
func (handler *service) preparePurchaseLine(input purchaseLineInput, number int) (preparedPurchaseLine, error) {
	prefix := "Renglón " + itoa(number) + ": "
	capture, err := handler.resolveCapture(input.ItemID, input.PresentationID, input.UnitID, input.Quantity, prefix, false)
	if err != nil {
		return preparedPurchaseLine{}, err
	}
	price, err := parsePurchasePrice(input.UnitPrice, prefix+"el precio unitario sin IVA")
	if err != nil {
		return preparedPurchaseLine{}, err
	}
	discount := zero
	if input.Discount != nil && strings.TrimSpace(*input.Discount) != "" {
		if discount, err = parsePurchaseAmount(*input.Discount, prefix+"el descuento"); err != nil {
			return preparedPurchaseLine{}, err
		}
	}
	rate, err := validTaxRate(input.TaxRate, "Renglón "+itoa(number))
	if err != nil {
		return preparedPurchaseLine{}, err
	}
	amounts, err := purchaseLineAmounts(capture.quantity, price, discount, rate)
	if err != nil {
		return preparedPurchaseLine{}, prefixed(prefix, err)
	}
	return preparedPurchaseLine{capture: capture, unitPrice: price, taxRate: rate, amounts: amounts}, nil
}

// preparePurchaseLines valida los renglones de una orden o de una recepción directa: un insumo una
// sola vez por documento.
func (handler *service) preparePurchaseLines(inputs []purchaseLineInput) ([]preparedPurchaseLine, PurchaseAmounts, error) {
	prepared := make([]preparedPurchaseLine, 0, len(inputs))
	totals := PurchaseAmounts{}
	seen := map[string]bool{}
	for index, input := range inputs {
		line, err := handler.preparePurchaseLine(input, index+1)
		if err != nil {
			return nil, totals, err
		}
		if seen[line.capture.item.ID] {
			return nil, totals, invalid("El insumo «" + line.capture.item.Name + "» aparece en más de un renglón.")
		}
		seen[line.capture.item.ID] = true
		totals.add(line.amounts)
		prepared = append(prepared, line)
	}
	return prepared, totals, nil
}

// PurchaseLine son los datos comunes de un renglón de orden o de recepción tal como los entrega la
// API: insumo, captura (presentación o unidad), cantidades e importes.
type PurchaseLine struct {
	LineNo       int                    `json:"line_no"`
	Item         ItemReference          `json:"item"`
	Presentation *PresentationReference `json:"presentation"`
	// Unit es la unidad de captura (nil si se capturó en una presentación).
	Unit         *UnitReference `json:"unit"`
	CaptureLabel string         `json:"capture_label"`
	Quantity     string         `json:"quantity"`
	BaseQuantity string         `json:"base_quantity"`
	UnitPrice    string         `json:"unit_price"`
	Gross        string         `json:"gross"`
	Discount     string         `json:"discount"`
	Subtotal     string         `json:"subtotal"`
	TaxRate      int            `json:"tax_rate"`
	Tax          string         `json:"tax"`
	Total        string         `json:"total"`
}

// purchaseLineColumns son las columnas comunes de un renglón (alias l) con su insumo, unidad base,
// presentación y unidad de captura. Requiere los JOIN de purchaseLineJoins.
const purchaseLineColumns = `l.line_no,i.id,i.code,i.name,i.kind,i.active,bu.id,bu.code,bu.name,bu.dimension,
        p.id,p.name,p.base_quantity::text,p.active,cu.id,cu.code,cu.name,cu.dimension,
        l.quantity::text,l.base_quantity::text,l.unit_price::text,l.discount::text,l.tax_rate,l.subtotal::text,l.tax::text,l.total::text`

const purchaseLineJoins = ` JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units bu ON bu.id=i.base_unit_id
        LEFT JOIN app.inventory_presentations p ON p.id=l.presentation_id
        LEFT JOIN app.inventory_units cu ON cu.id=l.unit_id`

// purchaseLineTargets devuelve los destinos de Scan de purchaseLineColumns y una función que completa
// el renglón después de leerlo.
func purchaseLineTargets(line *PurchaseLine) ([]any, func()) {
	var presentationID, presentationName, presentationBase *string
	var presentationActive *bool
	var unitID, unitCode, unitName, unitDimension *string
	var quantity, base, price, discount, subtotal, tax, total string
	targets := []any{&line.LineNo, &line.Item.ID, &line.Item.Code, &line.Item.Name, &line.Item.Kind, &line.Item.Active,
		&line.Item.Unit.ID, &line.Item.Unit.Code, &line.Item.Unit.Name, &line.Item.Unit.Dimension, &presentationID, &presentationName,
		&presentationBase, &presentationActive, &unitID, &unitCode, &unitName, &unitDimension, &quantity, &base, &price, &discount,
		&line.TaxRate, &subtotal, &tax, &total}
	finish := func() {
		if presentationID != nil {
			line.Presentation = &PresentationReference{
				ID: *presentationID, Name: *presentationName, BaseQuantity: quantityText(parseDatabaseDecimal(*presentationBase)),
				Active: *presentationActive,
			}
			line.CaptureLabel = *presentationName
		}
		if unitID != nil {
			line.Unit = &UnitReference{ID: *unitID, Code: *unitCode, Name: *unitName, Dimension: *unitDimension}
			line.CaptureLabel = *unitName
		}
		line.Quantity = quantityText(parseDatabaseDecimal(quantity))
		line.BaseQuantity = quantityText(parseDatabaseDecimal(base))
		line.UnitPrice = exactMoneyText(parseDatabaseDecimal(price))
		discountValue := parseDatabaseDecimal(discount)
		subtotalValue := parseDatabaseDecimal(subtotal)
		line.Discount = money(discountValue)
		line.Subtotal = money(subtotalValue)
		line.Gross = money(subtotalValue.Add(discountValue))
		line.Tax = money(parseDatabaseDecimal(tax))
		line.Total = money(parseDatabaseDecimal(total))
	}
	return targets, finish
}

// prefixed antepone el renglón al mensaje de un rechazo público («Renglón 2: el descuento…»).
func prefixed(prefix string, err error) error {
	var problem *Error
	if !errors.As(err, &problem) {
		return err
	}
	first, size := utf8.DecodeRuneInString(problem.Message)
	return &Error{Status: problem.Status, Code: problem.Code, Message: prefix + string(unicode.ToLower(first)) + problem.Message[size:]}
}
