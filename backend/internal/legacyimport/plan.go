package legacyimport

import (
	"fmt"
	"strings"
)

// Plan es la correspondencia completa de un paquete, en seco: lo que se importará a lo vivo y el historial derivado.
// La misma función la usan el análisis (resumen previo) y la importación.
type Plan struct {
	Model   *Model
	Catalog CatalogPlan
	History *HistoryPlan
}

// Ledger entries del libro del historial.
const (
	LedgerLine       = "line"
	LedgerDiscount   = "discount"
	LedgerCharge     = "charge"
	LedgerDifference = "difference"
)

// BuildPlan hace la correspondencia del paquete. reservedUsernames son los nombres de usuario de AD-EL POS que no
// pertenecen al lote vigente (los del asistente inicial).
func BuildPlan(model *Model, reservedUsernames map[string]bool) *Plan {
	return &Plan{
		Model:   model,
		Catalog: buildCatalog(model, reservedUsernames),
		History: buildHistory(model),
	}
}

// LedgerEntry es un renglón del libro del historial (app.legacy_sales_recognitions).
type LedgerEntry struct {
	Kind          string
	SaleIndex     int
	LineIndex     int
	DiscountIndex int
	Quantity      Quantity
	Gross         Money
	Discount      Money
	Charge        Money
	Net           Money
	Base          Money
	Tax           Money
	TaxRate       int
	HasRate       bool
	TaxEstimated  bool
	Concept       string
}

// Ledger entrega los renglones del libro del historial de las ventas pagadas no canceladas, en este orden: renglones
// (con sus complementos sumados), descuentos, cargos extra y diferencias. Por construcción, la suma de net por venta
// es lo que se debía según el anterior (total + montoExtra − descuentos) y la de tax, el IVA de la venta (guardado o,
// si no estaba, estimado): los productos usan sus renglones y lo que no cuadra queda en «Diferencia del sistema
// anterior» (decisión 2 del propietario).
func (plan *Plan) Ledger(emit func(LedgerEntry) error) error {
	iterator := plan.LedgerIterator()
	for {
		entry, ok := iterator.Next()
		if !ok {
			return nil
		}
		if err := emit(entry); err != nil {
			return err
		}
	}
}

// LedgerIterator recorre el libro del historial sin guardarlo en memoria (el COPY lo pide renglón por renglón).
type LedgerIterator struct {
	plan  *Plan
	phase int
	index int
}

// LedgerIterator empieza el recorrido del libro.
func (plan *Plan) LedgerIterator() *LedgerIterator {
	return &LedgerIterator{plan: plan}
}

// Next entrega el renglón siguiente del libro; ok es falso al terminar.
func (iterator *LedgerIterator) Next() (LedgerEntry, bool) {
	model, history := iterator.plan.Model, iterator.plan.History
	for iterator.phase < 4 {
		switch iterator.phase {
		case 0:
			for iterator.index < len(model.Lines) {
				index := iterator.index
				iterator.index++
				if entry, ok := lineEntry(model, history, index); ok {
					return entry, true
				}
			}
		case 1:
			for iterator.index < len(model.Discounts) {
				index := iterator.index
				iterator.index++
				if entry, ok := discountEntry(model, history, index); ok {
					return entry, true
				}
			}
		case 2:
			for iterator.index < len(model.Sales) {
				index := iterator.index
				iterator.index++
				if entry, ok := chargeEntry(model, history, index); ok {
					return entry, true
				}
			}
		case 3:
			for iterator.index < len(model.Sales) {
				index := iterator.index
				iterator.index++
				if entry, ok := differenceEntry(model, history, index); ok {
					return entry, true
				}
			}
		}
		iterator.phase++
		iterator.index = 0
	}
	return LedgerEntry{}, false
}

// lineEntry es el renglón del libro de un renglón vendido, con sus complementos sumados.
func lineEntry(model *Model, history *HistoryPlan, index int) (LedgerEntry, bool) {
	line := model.Lines[index]
	state := history.Lines[index]
	if state.SaleIndex < 0 || history.Sales[state.SaleIndex].Status != SalePaid {
		return LedgerEntry{}, false
	}
	gross := line.Total + state.ComplementsTotal
	tax := state.Tax + state.ComplementsTax
	return LedgerEntry{
		Kind: LedgerLine, SaleIndex: state.SaleIndex, LineIndex: index, DiscountIndex: -1,
		Quantity: line.Quantity, Gross: gross, Net: gross, Tax: tax, Base: gross - tax,
		TaxRate: state.TaxRate, HasRate: state.HasRate, TaxEstimated: state.TaxEstimated || state.ComplementEstimated,
	}, true
}

// discountEntry es el renglón del libro de un descuento (sin IVA, como en el anterior).
func discountEntry(model *Model, history *HistoryPlan, index int) (LedgerEntry, bool) {
	discount := model.Discounts[index]
	state := history.Discounts[index]
	if state.SaleIndex < 0 || history.Sales[state.SaleIndex].Status != SalePaid || discount.Amount == 0 {
		return LedgerEntry{}, false
	}
	return LedgerEntry{
		Kind: LedgerDiscount, SaleIndex: state.SaleIndex, LineIndex: -1, DiscountIndex: index,
		Discount: discount.Amount, Net: -discount.Amount, Base: -discount.Amount,
		Concept: discountConcept(state.TypeName, discount.Description),
	}, true
}

// chargeEntry es el renglón del libro del cargo extra de una venta (sin IVA).
func chargeEntry(model *Model, history *HistoryPlan, index int) (LedgerEntry, bool) {
	sale := model.Sales[index]
	charge := sale.ChargeTotal.Or(0)
	if history.Sales[index].Status != SalePaid || charge == 0 {
		return LedgerEntry{}, false
	}
	return LedgerEntry{
		Kind: LedgerCharge, SaleIndex: index, LineIndex: -1, DiscountIndex: -1,
		Charge: charge, Net: charge, Base: charge, Concept: chargeConcept(sale.Charge),
	}, true
}

// differenceEntry es la «Diferencia del sistema anterior» de una venta cuyo total o IVA guardados no cuadran con sus
// renglones.
func differenceEntry(model *Model, history *HistoryPlan, index int) (LedgerEntry, bool) {
	sale := model.Sales[index]
	state := history.Sales[index]
	if state.Status != SalePaid {
		return LedgerEntry{}, false
	}
	difference := sale.Total - state.LinesAmount
	taxDifference := state.Tax - state.LinesTax
	if difference == 0 && taxDifference == 0 {
		return LedgerEntry{}, false
	}
	return LedgerEntry{
		Kind: LedgerDifference, SaleIndex: index, LineIndex: -1, DiscountIndex: -1,
		Gross: difference, Net: difference, Tax: taxDifference, Base: difference - taxDifference,
		Concept: differenceConcept, TaxEstimated: state.TaxEstimated,
	}, true
}

// discountConcept es el concepto de un descuento en el libro: su tipo y su descripción.
func discountConcept(typeName, description string) string {
	parts := []string{}
	if name := cleanName(typeName, 60, ""); name != "" {
		parts = append(parts, name)
	}
	if text := cleanName(description, 200, ""); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return "Descuento (sistema anterior)"
	}
	return "Descuento: " + strings.Join(parts, " · ")
}

// chargeConcept es el concepto de un cargo extra en el libro.
func chargeConcept(text string) string {
	if concept := cleanName(text, 250, ""); concept != "" {
		return "Cargo: " + concept
	}
	return "Cargo extra (sistema anterior)"
}

// tableName es el nombre de la mesa importada de una venta (o vacío para llevar).
func (plan *Plan) tableFor(legacyTable NullInt) (PlannedTable, bool) {
	if !legacyTable.Valid || legacyTable.Value == takeawayTableID {
		return PlannedTable{}, false
	}
	for _, table := range plan.Catalog.Tables {
		if table.LegacyID == legacyTable.Value {
			return table, true
		}
	}
	return PlannedTable{}, false
}

// SalesPeriod es el rango de fechas de apertura de las ventas como «03/2018 a 11/2026».
func (plan *Plan) SalesPeriod() (first, last string, label string) {
	model := plan.Model
	if len(model.Sales) == 0 {
		return "", "", "Sin ventas"
	}
	minimum, maximum := model.Sales[0].OpenedAt, model.Sales[0].OpenedAt
	for _, sale := range model.Sales {
		if sale.OpenedAt.Before(minimum) {
			minimum = sale.OpenedAt
		}
		if sale.OpenedAt.After(maximum) {
			maximum = sale.OpenedAt
		}
	}
	first = minimum.In(model.Location).Format("2006-01")
	last = maximum.In(model.Location).Format("2006-01")
	label = fmt.Sprintf("%s a %s", minimum.In(model.Location).Format("01/2006"), maximum.In(model.Location).Format("01/2006"))
	return first, last, label
}
