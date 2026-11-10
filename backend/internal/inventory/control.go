package inventory

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Control de inventario (fase D, §21–§24 de la especificación): ajustes de discrepancia, mermas
// operativas, transferencias entre almacenes, conteos físicos, niveles por almacén, alertas y
// sugerencias de compra. Reglas comunes:
//   - ningún documento edita existencias: cada efecto es un movimiento del libro (Apply o
//     ApplyTransfer) con el motor de costos, idempotente por entry_key;
//   - una existencia negativa nunca bloquea (ajustes, mermas, transferencias y conteos la avisan en
//     warnings con NEGATIVE_STOCK), igual que las ventas;
//   - los valores de los movimientos solo se entregan con inventory.costs.read;
//   - cada escritura publica sus eventos (sin contenido) y se audita en la misma transacción.

// Lecturas de los documentos de la fase D: las ve quien los registra o autoriza, o quien consulta
// el libro de movimientos.
var (
	adjustmentReadPermissions = []string{
		permissionAdjustmentsRecord, permissionAdjustmentsAuthorize, permissionWasteRecord, permissionMovementsRead,
	}
	transferReadPermissions = []string{permissionTransfersRecord, permissionMovementsRead}
	countReadPermissions    = []string{permissionCountsPerform, permissionCountsApprove}
)

// maximumControlLines limita los renglones de un ajuste o de una transferencia.
const maximumControlLines = 200

// readAny es una lectura que exige al menos uno de los permisos indicados.
func readAny(action func(handler *service, parts []string) (any, error), permissions []string) *route {
	return &route{anyPermission: permissions, readOnly: true, action: action}
}

// resolveControl enumera las rutas de la fase D. Un solo identificador por ruta (segunda posición);
// las sugerencias de compra tienen una acción con nombre fijo.
func resolveControl(method string, parts []string) *route {
	length := len(parts)
	resource := parts[0]
	switch {
	case length == 1:
		switch resource + " " + method {
		case "adjustments GET":
			return readAny((*service).listAdjustments, adjustmentReadPermissions)
		case "adjustments POST":
			// Discrepancia: inventory.adjustments.record; merma: inventory.waste.record. Se comprueba
			// al decodificar, antes de escribir.
			return write((*service).recordAdjustment)
		case "transfers GET":
			return readAny((*service).listTransfers, transferReadPermissions)
		case "transfers POST":
			return write((*service).recordTransfer, permissionTransfersRecord)
		case "counts GET":
			return readAny((*service).listCounts, countReadPermissions)
		case "counts POST":
			return write((*service).createCount, permissionCountsPerform)
		case "stock-levels GET":
			return read((*service).listStockLevels)
		case "stock-levels PUT":
			return write((*service).saveStockLevel, permissionItemsManage)
		case "alerts GET":
			return read((*service).listAlerts)
		case "purchase-suggestions GET":
			return read((*service).listPurchaseSuggestions)
		}
	case length == 2 && resource == "purchase-suggestions" && parts[1] == "draft-order" && method == "POST":
		selected := write((*service).draftOrderFromSuggestions, permissionPurchasesCreate)
		selected.staticPath = true
		return selected
	case length == 2:
		switch resource + " " + method {
		case "adjustments GET":
			return readAny((*service).readAdjustmentRoute, adjustmentReadPermissions)
		case "transfers GET":
			return readAny((*service).readTransferRoute, transferReadPermissions)
		case "counts GET":
			return readAny((*service).readCountRoute, countReadPermissions)
		case "counts PATCH":
			return write((*service).updateCount, permissionCountsPerform)
		}
	case length == 3:
		switch resource + " " + parts[2] + " " + method {
		case "adjustments authorize POST":
			return write((*service).authorizeAdjustment, permissionAdjustmentsAuthorize)
		case "adjustments reject POST":
			return write((*service).rejectAdjustment, permissionAdjustmentsAuthorize)
		case "counts lines PUT":
			return write((*service).captureCountLines, permissionCountsPerform)
		case "counts submit POST":
			return write((*service).submitCount, permissionCountsPerform)
		case "counts approve POST":
			return write((*service).approveCount, permissionCountsApprove)
		case "counts apply POST":
			return write((*service).applyCount, permissionCountsApprove)
		case "counts cancel POST":
			// Un conteo abierto lo cancela quien cuenta o quien aprueba; uno enviado o aprobado, solo
			// quien aprueba (se comprueba en la acción, antes de escribir).
			selected := write((*service).cancelCount)
			selected.anyPermission = countReadPermissions
			return selected
		}
	}
	return nil
}

// controlLineInput es un renglón capturado de un ajuste, una merma o una transferencia: insumo y
// cantidad en una presentación, en una unidad de su dimensión o, por omisión, en su unidad base.
type controlLineInput struct {
	ItemID         string  `json:"item_id"`
	Quantity       string  `json:"quantity"`
	PresentationID *string `json:"presentation_id"`
	UnitID         *string `json:"unit_id"`
}

// controlCapture valida un renglón y convierte su cantidad a la unidad base. allowZero admite una
// cantidad cero (conteos físicos); signed admite el signo «-» (ajustes) y devuelve la cantidad y
// la cantidad base con su signo. Los insumos, presentaciones y unidades desactivados se admiten:
// la existencia que les quede también se debe poder ajustar, transferir o contar.
func (handler *service) controlCapture(itemID string, presentationID, unitID *string, text, prefix string, allowZero, signed bool) (
	purchaseCapture, error) {
	trimmed := strings.TrimSpace(text)
	negative := false
	if signed && strings.HasPrefix(trimmed, "-") {
		negative = true
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
	}
	isZero := false
	if allowZero {
		value, err := parseNonNegativeQuantity(trimmed, prefix+"la cantidad")
		if err != nil {
			return purchaseCapture{}, err
		}
		if value.IsZero() {
			// Se valida la unidad o la presentación con una cantidad unitaria y después se fija el cero.
			isZero = true
			trimmed = "1"
		}
	}
	capture, err := handler.resolveCapture(itemID, presentationID, unitID, trimmed, prefix, true)
	if err != nil {
		return purchaseCapture{}, err
	}
	if isZero {
		capture.quantity, capture.base = zero, zero
	}
	if negative {
		capture.quantity, capture.base = capture.quantity.Neg(), capture.base.Neg()
	}
	return capture, nil
}

// captureColumns devuelve la presentación y la unidad de captura que se guardan (exactamente una).
func captureColumns(capture purchaseCapture) (*string, *string) {
	if capture.presentationID != nil {
		return capture.presentationID, nil
	}
	return nil, capture.unitID
}

// controlNotes recorta las observaciones opcionales de un documento de la fase D.
func controlNotes(text string, maximum int) (string, error) {
	trimmed := strings.TrimSpace(text)
	if textLength(trimmed) > maximum {
		return "", invalid("Las observaciones admiten hasta " + itoa(maximum) + " caracteres.")
	}
	return trimmed, nil
}

// activeWarehouse lee un almacén activo por su identificador.
func (handler *service) activeWarehouse(id, subject string) (*Warehouse, error) {
	if err := validateID(id); err != nil {
		return nil, invalid("Elige " + subject + ".")
	}
	warehouse, err := handler.readWarehouse(strings.ToLower(id))
	if err != nil {
		return nil, err
	}
	if !warehouse.Active {
		return nil, conflict("WAREHOUSE_INACTIVE", "El almacén «"+warehouse.Name+"» está desactivado.")
	}
	return warehouse, nil
}

// transactionTime es la hora de la transacción (now()), la misma para todos los renglones.
func (handler *service) transactionTime() (time.Time, error) {
	var now time.Time
	err := handler.tx.QueryRow(handler.ctx, "SELECT now()").Scan(&now)
	return now, err
}

// negativeStockWarning es el aviso de existencia negativa de un renglón (nunca bloquea).
func negativeStockWarning(item ItemReference, warehouseName string) Warning {
	id := item.ID
	return Warning{Code: "NEGATIVE_STOCK", ItemID: &id,
		Message: "La existencia de «" + item.Name + "» en «" + warehouseName + "» quedó negativa."}
}

// itemReferenceOf resume un insumo leído completo.
func itemReferenceOf(item *Item) ItemReference {
	return ItemReference{ID: item.ID, Code: item.Code, Name: item.Name, Kind: item.Kind, Active: item.Active, Unit: item.BaseUnit}
}

// signedQuantityText es la cadena JSON de una cantidad con signo.
func signedQuantityText(value decimal.Decimal) string {
	return quantityText(value)
}

// captureReferences lee la presentación o la unidad de captura de un renglón guardado.
func captureReferences(presentationID, presentationName, presentationBase *string, presentationActive *bool, unitID, unitCode,
	unitName, unitDimension *string) (*PresentationReference, *UnitReference, string) {
	if presentationID != nil {
		reference := &PresentationReference{ID: *presentationID, Name: *presentationName,
			BaseQuantity: quantityText(parseDatabaseDecimal(*presentationBase)), Active: *presentationActive}
		return reference, nil, *presentationName
	}
	if unitID != nil {
		reference := &UnitReference{ID: *unitID, Code: *unitCode, Name: *unitName, Dimension: *unitDimension}
		return nil, reference, *unitName
	}
	return nil, nil, ""
}

// controlLineJoins agrega a un renglón (alias l) su insumo, unidad base, presentación y unidad de
// captura; controlLineColumns son las columnas correspondientes.
const controlLineJoins = ` JOIN app.inventory_items i ON i.id=l.item_id
        JOIN app.inventory_units bu ON bu.id=i.base_unit_id
        LEFT JOIN app.inventory_presentations p ON p.id=l.presentation_id
        LEFT JOIN app.inventory_units cu ON cu.id=l.unit_id`

const controlLineColumns = `i.id,i.code,i.name,i.kind,i.active,bu.id,bu.code,bu.name,bu.dimension,
        p.id,p.name,p.base_quantity::text,p.active,cu.id,cu.code,cu.name,cu.dimension`

// controlLineTargets devuelve los destinos de Scan de controlLineColumns y la función que arma la
// presentación o la unidad de captura.
func controlLineTargets(item *ItemReference) ([]any, func() (*PresentationReference, *UnitReference, string)) {
	var presentationID, presentationName, presentationBase *string
	var presentationActive *bool
	var unitID, unitCode, unitName, unitDimension *string
	targets := []any{&item.ID, &item.Code, &item.Name, &item.Kind, &item.Active, &item.Unit.ID, &item.Unit.Code, &item.Unit.Name,
		&item.Unit.Dimension, &presentationID, &presentationName, &presentationBase, &presentationActive, &unitID, &unitCode,
		&unitName, &unitDimension}
	finish := func() (*PresentationReference, *UnitReference, string) {
		return captureReferences(presentationID, presentationName, presentationBase, presentationActive, unitID, unitCode, unitName,
			unitDimension)
	}
	return targets, finish
}

// movementValues suma el valor de los movimientos de un documento (incluidos los ajustes de costo y
// residuos que los acompañan) por renglón; la clave 0 es el total.
func (handler *service) movementValues(sourceKinds []string, sourceID string) (map[int]decimal.Decimal, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT coalesce(source_line,0),sum(value)::text FROM app.inventory_movements
        WHERE source_kind = ANY($1) AND source_id=$2 GROUP BY 1`, sourceKinds, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[int]decimal.Decimal{0: zero}
	for rows.Next() {
		var line int
		var value string
		if err := rows.Scan(&line, &value); err != nil {
			return nil, err
		}
		amount := parseDatabaseDecimal(value)
		values[line] = values[line].Add(amount)
		if line != 0 {
			values[0] = values[0].Add(amount)
		}
	}
	return values, rows.Err()
}

// optionalMoneyPointers devuelve el valor exacto y a centavos solo si los costos son visibles.
func (handler *service) optionalMoneyPointers(value decimal.Decimal) (*string, *string) {
	if !handler.costsVisible() {
		return nil, nil
	}
	return moneyPointers(value)
}
