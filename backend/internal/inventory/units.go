package inventory

import (
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Unidades de medida y conversiones (§4 de la especificación). Cada unidad pertenece a una
// dimensión (masa, volumen, piezas o porciones) y tiene un factor respecto de la unidad base de esa
// dimensión (gramo, mililitro, pieza y porción valen 1). Solo se convierte dentro de la misma
// dimensión; paquetes y cajas no son unidades sino presentaciones de cada insumo. Las seis unidades
// del sistema (0010) son de referencia e inmutables. Go es la autoridad de toda conversión.

var dimensionLabels = map[string]string{
	"mass":    "Masa",
	"volume":  "Volumen",
	"count":   "Piezas",
	"portion": "Porciones",
}

var unitCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,15}$`)

// maximumFactor coincide con inventory_units_factor_check.
var maximumFactor = decimal.NewFromInt(1000000)

// Unit es una unidad de medida tal como la entrega la API.
type Unit struct {
	ID             string    `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Dimension      string    `json:"dimension"`
	DimensionLabel string    `json:"dimension_label"`
	Factor         string    `json:"factor"`
	System         bool      `json:"system"`
	Active         bool      `json:"active"`
	InUse          bool      `json:"in_use"`
	Revision       int       `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UnitReference es la unidad resumida que acompaña a insumos, existencias y movimientos.
type UnitReference struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Dimension string `json:"dimension"`
}

// measure es lo que necesita una conversión: dimensión y factor.
type measure struct {
	Dimension string
	Factor    decimal.Decimal
}

// convertQuantity convierte una cantidad entre dos unidades de la misma dimensión:
// cantidad × factor de origen ÷ factor de destino, redondeada a 6 decimales (mitad alejándose de
// cero). Rechaza dimensiones distintas y resultados que se redondean a cero.
func convertQuantity(quantity decimal.Decimal, from, to measure) (decimal.Decimal, error) {
	if from.Dimension != to.Dimension {
		return zero, invalid("No se puede convertir entre unidades de dimensiones distintas (" +
			dimensionLabels[from.Dimension] + " y " + dimensionLabels[to.Dimension] + ").")
	}
	if !from.Factor.IsPositive() || !to.Factor.IsPositive() {
		return zero, invalid("La unidad no tiene una equivalencia válida.")
	}
	converted := quantity.Mul(from.Factor).DivRound(to.Factor, quantityScale)
	if quantity.IsPositive() && !converted.IsPositive() {
		return zero, invalid("La cantidad convertida es demasiado pequeña para la unidad base del insumo.")
	}
	if converted.GreaterThan(maximumQuantity) {
		return zero, invalid("La cantidad convertida excede el máximo admitido.")
	}
	return converted, nil
}

// presentationQuantity convierte un número de presentaciones (cajas, paquetes) a la unidad base
// del insumo con la equivalencia propia de la presentación.
func presentationQuantity(count, baseQuantity decimal.Decimal) (decimal.Decimal, error) {
	if !baseQuantity.IsPositive() {
		return zero, invalid("La presentación no tiene una equivalencia válida.")
	}
	converted := count.Mul(baseQuantity).Round(quantityScale)
	if converted.GreaterThan(maximumQuantity) {
		return zero, invalid("La cantidad convertida excede el máximo admitido.")
	}
	return converted, nil
}

const unitColumns = `SELECT u.id,u.code,u.name,u.dimension,u.factor::text,u.system,u.active,u.revision,u.updated_at,
        EXISTS(SELECT FROM app.inventory_items i WHERE i.base_unit_id=u.id) AS in_use
        FROM app.inventory_units u`

func scanUnit(row interface{ Scan(...any) error }) (*Unit, error) {
	var unit Unit
	var factor string
	err := row.Scan(&unit.ID, &unit.Code, &unit.Name, &unit.Dimension, &factor, &unit.System, &unit.Active,
		&unit.Revision, &unit.UpdatedAt, &unit.InUse)
	if err != nil {
		return nil, err
	}
	unit.Factor = parseDatabaseDecimal(factor).String()
	unit.DimensionLabel = dimensionLabels[unit.Dimension]
	return &unit, nil
}

func (handler *service) readUnit(id string) (*Unit, error) {
	unit, err := scanUnit(handler.tx.QueryRow(handler.ctx, unitColumns+" WHERE u.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La unidad no existe.")
	}
	return unit, err
}

// unitMeasure lee la dimensión y el factor de una unidad para convertir. inactiveAllowed admite
// una unidad desactivada (por ejemplo, la unidad base de un insumo existente).
func (handler *service) unitMeasure(id string, inactiveAllowed bool) (measure, string, error) {
	var dimension, factor, name string
	var active bool
	err := handler.tx.QueryRow(handler.ctx, "SELECT dimension,factor::text,name,active FROM app.inventory_units WHERE id=$1", id).
		Scan(&dimension, &factor, &name, &active)
	if isNoRows(err) {
		return measure{}, "", notFound("La unidad no existe.")
	}
	if err != nil {
		return measure{}, "", err
	}
	if !active && !inactiveAllowed {
		return measure{}, "", conflict("UNIT_INACTIVE", "La unidad «"+name+"» está desactivada.")
	}
	return measure{Dimension: dimension, Factor: parseDatabaseDecimal(factor)}, name, nil
}

// listUnits devuelve todas las unidades, activas e inactivas, agrupadas por dimensión y de la
// menor a la mayor.
func (handler *service) listUnits(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, unitColumns+` ORDER BY
        array_position(ARRAY['mass','volume','count','portion'], u.dimension), u.factor, u.name, u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []Unit{}
	for rows.Next() {
		unit, err := scanUnit(rows)
		if err != nil {
			return nil, err
		}
		units = append(units, *unit)
	}
	return units, rows.Err()
}

type unitInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Dimension   string `json:"dimension"`
	Factor      string `json:"factor"`
	Active      bool   `json:"active"`
}

// parseFactor acepta la equivalencia de una unidad propia: positiva, hasta diez decimales y no
// mayor que un millón de unidades base.
func parseFactor(text string) (decimal.Decimal, error) {
	factor, err := parseCost(text, "La equivalencia de la unidad")
	if err != nil {
		return zero, err
	}
	if !factor.IsPositive() || factor.GreaterThan(maximumFactor) {
		return zero, invalid("La equivalencia de la unidad debe ser mayor que cero y hasta 1,000,000.")
	}
	return factor, nil
}

// unitConflict traduce los índices únicos de unidades a un rechazo claro.
func unitConflict(err error) error {
	if !uniqueViolation(err) {
		return err
	}
	if strings.Contains(constraintName(err), "code") {
		return conflict("DUPLICATE_CODE", "Ya existe una unidad con esa abreviatura.")
	}
	return conflict("DUPLICATE_NAME", "Ya existe una unidad con ese nombre.")
}

// createUnit agrega una unidad propia de una dimensión existente (por ejemplo «oz», masa,
// 28.349523125 g). La abreviatura y la dimensión no cambian después.
func (handler *service) createUnit(_ []string) (any, error) {
	var input unitInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	input.Code = strings.TrimSpace(input.Code)
	if !unitCodePattern.MatchString(input.Code) {
		return nil, invalid("La abreviatura de la unidad debe iniciar con una letra minúscula y tener hasta 16 letras, " +
			"dígitos o guiones bajos.")
	}
	if err := validateName(&input.Name, "El nombre de la unidad", 40); err != nil {
		return nil, err
	}
	if _, known := dimensionLabels[input.Dimension]; !known {
		return nil, invalid("La dimensión de la unidad no es válida.")
	}
	factor, err := parseFactor(input.Factor)
	if err != nil {
		return nil, err
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_units(id,code,name,dimension,factor,active)
        VALUES ($1,$2,$3,$4,$5::numeric,$6)`, id, input.Code, input.Name, input.Dimension, factor.String(), input.Active)
	if err != nil {
		return nil, unitConflict(err)
	}
	saved, err := handler.readUnit(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &id); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.unit_created", "inventory_unit", &id, nil, saved)
}

// updateUnit renombra, activa o desactiva una unidad propia o cambia su equivalencia. Las
// unidades del sistema no se modifican; la equivalencia de una unidad usada no cambia (las
// cantidades registradas dejarían de significar lo mismo) y no se desactiva la unidad base de un
// insumo activo.
func (handler *service) updateUnit(parts []string) (any, error) {
	var input unitInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateName(&input.Name, "El nombre de la unidad", 40); err != nil {
		return nil, err
	}
	factor, err := parseFactor(input.Factor)
	if err != nil {
		return nil, err
	}
	previous, err := handler.readUnit(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La unidad")
	}
	if previous.System {
		return nil, conflict("UNIT_SYSTEM", "Las unidades del sistema no se modifican.")
	}
	if (input.Code != "" && input.Code != previous.Code) || (input.Dimension != "" && input.Dimension != previous.Dimension) {
		return nil, conflict("UNIT_IMMUTABLE", "La abreviatura y la dimensión de una unidad no cambian. Crea otra unidad.")
	}
	if previous.InUse && !factor.Equal(parseDatabaseDecimal(previous.Factor)) {
		return nil, conflict("UNIT_IN_USE", "La unidad ya se usa en insumos: su equivalencia no puede cambiar.")
	}
	if previous.Active && !input.Active {
		var activeItems int
		err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_items WHERE base_unit_id=$1 AND active", previous.ID).
			Scan(&activeItems)
		if err != nil {
			return nil, err
		}
		if activeItems > 0 {
			return nil, conflict("UNIT_IN_USE", "La unidad es la unidad base de insumos activos: no se puede desactivar.")
		}
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_units SET name=$2,factor=$3::numeric,active=$4,
        revision=revision+1,updated_at=now() WHERE id=$1`, previous.ID, input.Name, factor.String(), input.Active)
	if err != nil {
		return nil, unitConflict(err)
	}
	saved, err := handler.readUnit(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &saved.ID); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.unit_updated", "inventory_unit", &saved.ID, previous, saved)
}
