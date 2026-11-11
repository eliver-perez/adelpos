package catalog

import (
	"github.com/shopspring/decimal"
)

// Opción «El precio puede bajar al cambiar complementos» (etapa 9A, §3.1). Cada producto que la
// aplica calcula su precio con la regla con piso y, aparte, lo que sus complementos permitirían
// bajar (reductionTree). Solo la partida que se cobra (la raíz) aplica los límites, una sola vez:
// en un combo, las reducciones de sus componentes se acumulan con signo, sin acotarlas al precio
// suelto de cada componente (el combo lo cubre y ese precio no se cobra).

// reductionTree es lo que la opción permite bajar en un desglose, antes de los límites, con el
// mismo orden que el desglose para repartir después la reducción efectiva.
type reductionTree struct {
	// groups es la reducción de cada grupo respecto de la regla con piso (cero o negativa).
	groups []decimal.Decimal
	// components son los componentes que también aplican la opción.
	components []componentReduction
	// total es la suma de todo lo anterior por unidad del producto (cero o negativa).
	total decimal.Decimal
}

// componentReduction enlaza la reducción de un componente con su posición en el desglose del
// combo y con sus unidades.
type componentReduction struct {
	index int
	units decimal.Decimal
	tree  *reductionTree
}

func newReductionTree(groups groupTotals, components componentTotals) *reductionTree {
	tree := &reductionTree{groups: groups.reductions, components: components.reductions, total: decimal.Zero}
	for _, reduction := range groups.reductions {
		tree.total = tree.total.Add(reduction)
	}
	for _, component := range components.reductions {
		tree.total = tree.total.Add(component.tree.total.Mul(component.units))
	}
	return tree
}

// limitedAdjustment aplica los límites de §3.1 al núcleo del precio de la partida y devuelve la
// reducción efectiva respecto de la regla con piso: cero o negativa, nunca positiva. floorCore es
// base + diferencias con la regla con piso; el núcleo con la opción es floorCore + reducción y
// nunca queda:
//   - por debajo de cero;
//   - por debajo del valor de referencia de los complementos que permanecen seleccionados (los
//     propios y, en un combo, los de sus componentes);
//   - por encima del núcleo con la regla con piso: la opción solo permite bajar el precio.
//
// Los extras explícitos, los ingredientes adicionales y los cargos de los componentes se suman
// después del núcleo, así que estos cargos independientes nunca se descuentan.
func limitedAdjustment(floorCore, reduction, remaining decimal.Decimal) decimal.Decimal {
	core := decimal.Max(floorCore.Add(reduction), remaining, decimal.Zero)
	core = decimal.Min(core, floorCore)
	return core.Sub(floorCore)
}

// remainingComplementValue es el valor de referencia de los complementos que permanecen
// seleccionados en un desglose: el valor solicitado de sus grupos y, en un combo, el de sus
// componentes por cada unidad.
func remainingComplementValue(breakdown *ProductBreakdown) decimal.Decimal {
	total := decimal.Zero
	for _, group := range breakdown.Groups {
		total = total.Add(amount(group.RequestedValue))
	}
	for _, component := range breakdown.Components {
		if component.Product == nil {
			continue
		}
		units := decimal.NewFromInt(int64(component.Quantity))
		total = total.Add(remainingComplementValue(component.Product).Mul(units))
	}
	return total
}

// allocateReduction reparte la reducción efectiva (por unidad del producto, cero o negativa)
// entre los grupos y los componentes del desglose, en su orden y sin pasar de lo que cada uno
// permite: así cada grupo muestra lo que de verdad bajó el precio (applied_difference) y no una
// diferencia que el límite ya no aplicó. Cada desglose con la opción registra su parte en
// complement_adjustment; en un componente, por unidad del componente y sin incluirla en su propio
// precio, porque se aplica en el del combo.
//
// Si una parte de un componente con varias unidades no se divide exacta en centavos, la parte por
// unidad se trunca hacia cero: lo mostrado nunca promete más de lo aplicado.
func allocateReduction(breakdown *ProductBreakdown, tree *reductionTree, effective decimal.Decimal) {
	breakdown.ComplementAdjustment = money(effective)
	remaining := effective
	for index, reduction := range tree.groups {
		share := decimal.Max(reduction, remaining)
		remaining = remaining.Sub(share)
		group := &breakdown.Groups[index]
		// La diferencia con signo es piso + reducción: lo aplicado es piso + la parte asignada.
		applied := amount(group.Difference).Sub(reduction).Add(share)
		group.AppliedDifference = money(applied)
	}
	for _, component := range tree.components {
		available := component.tree.total.Mul(component.units)
		share := decimal.Max(available, remaining)
		perUnit := component.tree.total
		if !share.Equal(available) {
			perUnit = share.Div(component.units).Truncate(2)
		}
		remaining = remaining.Sub(perUnit.Mul(component.units))
		allocateReduction(breakdown.Components[component.index].Product, component.tree, perUnit)
	}
}
