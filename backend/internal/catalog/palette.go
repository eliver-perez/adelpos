package catalog

import "strings"

// PaletteColor es un color elegible para una categoría.
type PaletteColor struct {
	Hex  string `json:"hex"`
	Name string `json:"name"`
}

// Palette es la lista cerrada de colores de categoría. Los tonos son de saturación y luminosidad
// medias: cada uno contrasta al menos 3:1 con el panel claro (#ffffff) y con el oscuro (#19232e),
// de modo que la marca de color se distingue en ambos temas. La interfaz recibe esta misma lista
// en los lookups del catálogo y no la duplica.
var Palette = []PaletteColor{
	{Hex: "#d64545", Name: "Rojo"},
	{Hex: "#dd6b26", Name: "Naranja"},
	{Hex: "#c07f0f", Name: "Ámbar"},
	{Hex: "#a68f14", Name: "Amarillo"},
	{Hex: "#719a26", Name: "Lima"},
	{Hex: "#3f9a4a", Name: "Verde"},
	{Hex: "#1f9a78", Name: "Esmeralda"},
	{Hex: "#1a96a3", Name: "Turquesa"},
	{Hex: "#3591d4", Name: "Celeste"},
	{Hex: "#4370d6", Name: "Azul"},
	{Hex: "#6862d4", Name: "Índigo"},
	{Hex: "#8a5bd3", Name: "Violeta"},
	{Hex: "#a64fba", Name: "Morado"},
	{Hex: "#d0508c", Name: "Rosa"},
	{Hex: "#9c6b46", Name: "Café"},
	{Hex: "#77818c", Name: "Gris"},
}

// validateCategoryColor acepta sin color, un color de la paleta (normalizado a su forma
// canónica en minúsculas) o, solo al editar, exactamente el color que el registro ya tenía
// guardado aunque no pertenezca a la paleta: los colores capturados antes de la paleta se
// conservan y la interfaz los muestra como «Color personalizado». Un color nuevo fuera de la
// paleta se rechaza.
func validateCategoryColor(category *Category, previous *Category) error {
	if category.Color == "" {
		return nil
	}
	if previous != nil && previous.Color != "" && category.Color == previous.Color {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(category.Color))
	for _, color := range Palette {
		if color.Hex == normalized {
			category.Color = color.Hex
			return nil
		}
	}
	return invalid("Elige un color de la paleta o deja la categoría sin color.")
}
