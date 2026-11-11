package kitchen

import "local/ad-el-pos/server/internal/catalog"

func preparationIngredients(source []catalog.IngredientBreakdown) []catalog.IngredientBreakdown {
	result := append([]catalog.IngredientBreakdown{}, source...)
	for index := range result {
		result[index].Extra = ""
	}
	return result
}

// preparationGroups copia los grupos de una parte sin importes: la cocina ve opciones y porciones,
// nunca valores, diferencias (con o sin signo, etapa 9A) ni extras. Los importes del producto
// (precio unitario, ajuste por complementos) nunca se copian a una parte.
func preparationGroups(source []catalog.GroupBreakdown, snapshot catalog.Snapshot) []catalog.GroupBreakdown {
	result := append([]catalog.GroupBreakdown{}, source...)
	for index := range result {
		group := &result[index]
		group.OriginalValue, group.RequestedValue, group.Difference, group.Extras = "", "", "", ""
		group.Options = append([]catalog.OptionBreakdown{}, group.Options...)
		for optionIndex := range group.Options {
			option := &group.Options[optionIndex]
			if option.PortionID != nil {
				if portion, exists := snapshot.Portions[*option.PortionID]; exists {
					option.Name += " (" + portion.Name + ")"
				}
			}
		}
	}
	return result
}
