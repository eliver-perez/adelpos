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

type Customization struct {
	Ingredients []IngredientSelection `json:"ingredients"`
	Groups      []GroupSelection      `json:"groups"`
	Components  []ComponentSelection  `json:"components"`
	Notes       string                `json:"notes"`
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

// Snapshot solo vive durante el cálculo; no representa una venta ni reserva existencias.
type Snapshot struct {
	Products    map[string]Product
	Categories  map[string]Category
	Ingredients map[string]Ingredient
	Complements map[string]Complement
	Portions    map[string]Portion
	Groups      map[string]OptionGroup
	PriceList   PriceList
	Promotions  []Promotion
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
	GroupID        string            `json:"group_id"`
	Revision       int               `json:"revision"`
	Name           string            `json:"name"`
	Mode           string            `json:"mode"`
	OriginalValue  string            `json:"original_value"`
	RequestedValue string            `json:"requested_value"`
	Difference     string            `json:"difference"`
	Extras         string            `json:"extras"`
	Options        []OptionBreakdown `json:"options"`
}

type ComponentBreakdown struct {
	SlotID   string            `json:"slot_id"`
	Name     string            `json:"name"`
	Quantity int               `json:"quantity"`
	Charge   string            `json:"charge"`
	Product  *ProductBreakdown `json:"product"`
}

type ProductBreakdown struct {
	ProductID        string                `json:"product_id"`
	Revision         int                   `json:"revision"`
	CategoryID       string                `json:"category_id"`
	VariantID        *string               `json:"variant_id"`
	Name             string                `json:"name"`
	BasePrice        string                `json:"base_price"`
	PriceOrigin      string                `json:"price_origin"`
	IngredientExtras string                `json:"ingredient_extras"`
	GroupExtras      string                `json:"group_extras"`
	ComponentExtras  string                `json:"component_extras"`
	UnitPrice        string                `json:"unit_price"`
	Ingredients      []IngredientBreakdown `json:"ingredients"`
	Groups           []GroupBreakdown      `json:"groups"`
	Components       []ComponentBreakdown  `json:"components"`
	Notes            string                `json:"notes"`
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
