package catalog

import "time"

type IngredientSelection struct {
	IngredientID string `json:"ingredient_id"`
	Quantity     string `json:"quantity"`
}

type GroupSelection struct {
	GroupID string           `json:"group_id"`
	Options []IncludedOption `json:"options"`
}

type ComponentSelection struct {
	SlotID        string        `json:"slot_id"`
	ProductID     string        `json:"product_id"`
	VariantID     *string       `json:"variant_id"`
	Quantity      int           `json:"quantity"`
	Customization Customization `json:"customization"`
}

// QuickNoteSelection es una indicación rápida elegida en la captura (etapa 8): su identificador y
// el texto que vio el mesero. El texto sigue también en Notes, como en la etapa 7.1, para que la
// comanda y los documentos funcionen igual.
type QuickNoteSelection struct {
	NoteID string `json:"note_id"`
	Text   string `json:"text"`
}

type Customization struct {
	Ingredients []IngredientSelection `json:"ingredients"`
	Groups      []GroupSelection      `json:"groups"`
	Components  []ComponentSelection  `json:"components"`
	Notes       string                `json:"notes"`
	// QuickNotes se omite cuando está vacío: los borradores, solicitudes y huellas anteriores a la
	// etapa 8 (solo texto, sin efecto) se serializan exactamente igual que antes.
	QuickNotes []QuickNoteSelection `json:"quick_notes,omitempty"`
}

type LineRequest struct {
	ProductID     string        `json:"product_id"`
	VariantID     *string       `json:"variant_id"`
	Quantity      int           `json:"quantity"`
	Customization Customization `json:"customization"`
}

type PreviewRequest struct {
	PriceListID          *string       `json:"price_list_id"`
	Lines                []LineRequest `json:"lines"`
	OverrideAvailability bool          `json:"override_availability"`
	OverrideReason       string        `json:"override_reason"`
}

// Snapshot conserva la configuración exacta del cálculo; pedidos puede persistirlo al confirmar.
// Por sí mismo no representa un cobro ni reserva o consume existencias.
type Snapshot struct {
	Products    map[string]Product
	Categories  map[string]Category
	Ingredients map[string]Ingredient
	Complements map[string]Complement
	Portions    map[string]Portion
	Groups      map[string]OptionGroup
	PriceList   PriceList
	Promotions  []Promotion
	// QuickNotes guarda solo las indicaciones rápidas que usa el cálculo (etapa 8), tal como
	// estaban al confirmar. Se omite sin indicaciones para no cambiar los snapshots anteriores.
	QuickNotes map[string]CategoryQuickNote `json:"QuickNotes,omitempty"`
}

type IngredientBreakdown struct {
	IngredientID   string `json:"ingredient_id"`
	Name           string `json:"name"`
	Included       string `json:"included"`
	Requested      string `json:"requested"`
	Extra          string `json:"extra"`
	RecipeQuantity string `json:"recipe_quantity"`
}

type OptionBreakdown struct {
	OptionID     string  `json:"option_id"`
	ComplementID string  `json:"complement_id"`
	PortionID    *string `json:"portion_id"`
	Name         string  `json:"name"`
	Included     string  `json:"included"`
	Requested    string  `json:"requested"`
}

type GroupBreakdown struct {
	GroupID        string `json:"group_id"`
	Revision       int    `json:"revision"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	OriginalValue  string `json:"original_value"`
	RequestedValue string `json:"requested_value"`
	Difference     string `json:"difference"`
	// AppliedDifference (etapa 9A, §3.1) solo aparece cuando se aplicó la opción «El precio puede
	// bajar al cambiar complementos»: es el efecto real del grupo en el precio por unidad (la
	// diferencia con la regla con piso más la parte de la reducción que de verdad se aplicó). Con
	// los límites puede ser menor que Difference en valor absoluto; nunca promete más de lo
	// aplicado. Sin la opción se omite.
	AppliedDifference string            `json:"applied_difference,omitempty"`
	Extras            string            `json:"extras"`
	Options           []OptionBreakdown `json:"options"`
}

// QuickNoteBreakdown es una indicación rápida resuelta por Go al cotizar: congela su texto, su
// efecto y su ingrediente para que editar la indicación después no cambie la partida confirmada.
type QuickNoteBreakdown struct {
	NoteID       string  `json:"note_id"`
	Text         string  `json:"text"`
	Effect       string  `json:"effect"`
	IngredientID *string `json:"ingredient_id"`
}

type ComponentBreakdown struct {
	SlotID   string            `json:"slot_id"`
	Name     string            `json:"name"`
	Quantity int               `json:"quantity"`
	Charge   string            `json:"charge"`
	Product  *ProductBreakdown `json:"product"`
}

type ProductBreakdown struct {
	ProductID        string  `json:"product_id"`
	Revision         int     `json:"revision"`
	CategoryID       string  `json:"category_id"`
	VariantID        *string `json:"variant_id"`
	Name             string  `json:"name"`
	BasePrice        string  `json:"base_price"`
	PriceOrigin      string  `json:"price_origin"`
	IngredientExtras string  `json:"ingredient_extras"`
	GroupExtras      string  `json:"group_extras"`
	ComponentExtras  string  `json:"component_extras"`
	UnitPrice        string  `json:"unit_price"`
	// PriceCanDecrease y ComplementAdjustment (etapa 9A, §3.1) solo aparecen cuando la opción «El
	// precio puede bajar al cambiar complementos» se aplicó a este cálculo (el producto la tiene y,
	// en un componente, también el combo). Sin la opción se omiten, así que las cotizaciones, los
	// borradores y las huellas anteriores se serializan exactamente igual que antes.
	// ComplementAdjustment es la reducción efectiva respecto de la regla con piso: una cadena con
	// signo («-10.00», o «0.00» si no bajó). En la partida ya está incluida en GroupExtras y en
	// UnitPrice. En el producto de un componente de combo es la parte que ese componente aportó
	// al precio del combo, por unidad del componente, y no se incluye en su propio precio (que
	// sigue la regla con piso, porque el combo lo cubre): los límites se aplican una sola vez, en
	// el núcleo del combo.
	PriceCanDecrease     bool                  `json:"price_can_decrease,omitempty"`
	ComplementAdjustment string                `json:"complement_adjustment,omitempty"`
	Ingredients          []IngredientBreakdown `json:"ingredients"`
	Groups               []GroupBreakdown      `json:"groups"`
	Components           []ComponentBreakdown  `json:"components"`
	Notes                string                `json:"notes"`
	// QuickNotes se omite sin indicaciones, así que las cotizaciones y huellas anteriores no
	// cambian.
	QuickNotes []QuickNoteBreakdown `json:"quick_notes,omitempty"`
}

type QuoteLine struct {
	ProductBreakdown
	Quantity int    `json:"quantity"`
	Subtotal string `json:"subtotal"`
	Discount string `json:"discount"`
	Total    string `json:"total"`
}

type PromotionResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision int    `json:"revision"`
	Applied  bool   `json:"applied"`
	Reason   string `json:"reason"`
	Discount string `json:"discount"`
}

type Quote struct {
	CalculatedAt         time.Time         `json:"calculated_at"`
	Timezone             string            `json:"timezone"`
	PriceListID          string            `json:"price_list_id"`
	PriceListRevision    int               `json:"price_list_revision"`
	PriceListName        string            `json:"price_list_name"`
	AvailabilityOverride bool              `json:"availability_override"`
	Lines                []QuoteLine       `json:"lines"`
	Promotions           []PromotionResult `json:"promotions"`
	Subtotal             string            `json:"subtotal"`
	Discount             string            `json:"discount"`
	Total                string            `json:"total"`
}
