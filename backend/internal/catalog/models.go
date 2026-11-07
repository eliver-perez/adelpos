// Package catalog administra el catálogo y centraliza sus cálculos comerciales exactos.
package catalog

type Metadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Revision    int    `json:"revision"`
}

type TimeWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type Schedule struct {
	StartsOn string       `json:"starts_on,omitempty"`
	EndsOn   string       `json:"ends_on,omitempty"`
	Days     []int        `json:"days"`
	Windows  []TimeWindow `json:"windows"`
}

type Category struct {
	Metadata
	SortOrder    int      `json:"sort_order"`
	Color        string   `json:"color"`
	ImageID      *string  `json:"image_id"`
	Availability Schedule `json:"availability"`
}

type Ingredient struct {
	Metadata
	Unit string `json:"unit"`
}

type Complement struct {
	Metadata
	ProductID    *string `json:"product_id"`
	IngredientID *string `json:"ingredient_id"`
}

type Portion struct {
	Metadata
	Factor string `json:"factor"`
}

// Los tramos son incrementales: desde esa unidad se utiliza su precio hasta el siguiente tramo.
type QuantityTier struct {
	From      int    `json:"from"`
	UnitPrice string `json:"unit_price"`
}

type GroupOption struct {
	ID             string         `json:"id"`
	ComplementID   string         `json:"complement_id"`
	PortionID      *string        `json:"portion_id"`
	ReferenceValue string         `json:"reference_value"`
	ExtraPrice     string         `json:"extra_price"`
	MaxQuantity    *string        `json:"max_quantity"`
	Active         bool           `json:"active"`
	Tiers          []QuantityTier `json:"tiers"`
	Name           string         `json:"name,omitempty"`
	PortionName    string         `json:"portion_name,omitempty"`
	PortionFactor  string         `json:"portion_factor,omitempty"`
}

type OptionGroup struct {
	Metadata
	PricingMode string        `json:"pricing_mode"`
	MinQuantity string        `json:"min_quantity"`
	MaxQuantity *string       `json:"max_quantity"`
	Options     []GroupOption `json:"options"`
}

type Variant struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Code         string   `json:"code"`
	Barcode      *string  `json:"barcode"`
	Price        string   `json:"price"`
	Active       bool     `json:"active"`
	Availability Schedule `json:"availability"`
}

type ProductIngredient struct {
	IngredientID     string `json:"ingredient_id"`
	IncludedQuantity string `json:"included_quantity"`
	Removable        bool   `json:"removable"`
	Addable          bool   `json:"addable"`
	MaxQuantity      string `json:"max_quantity"`
	ExtraPrice       string `json:"extra_price"`
	RecipeQuantity   string `json:"recipe_quantity"`
	Name             string `json:"name,omitempty"`
}

type IncludedOption struct {
	OptionID string `json:"option_id"`
	Quantity string `json:"quantity"`
}

type ProductGroup struct {
	GroupID     string           `json:"group_id"`
	MinQuantity *string          `json:"min_quantity"`
	MaxQuantity *string          `json:"max_quantity"`
	Included    []IncludedOption `json:"included"`
}

type ComboOption struct {
	ProductID         string         `json:"product_id"`
	SubstitutionPrice string         `json:"substitution_price"`
	Tiers             []QuantityTier `json:"tiers"`
}

type ComboSlot struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	MinQuantity      int           `json:"min_quantity"`
	MaxQuantity      int           `json:"max_quantity"`
	IncludedQuantity int           `json:"included_quantity"`
	DefaultQuantity  int           `json:"default_quantity"`
	DefaultProductID string        `json:"default_product_id"`
	Options          []ComboOption `json:"options"`
}

type Product struct {
	Metadata
	CategoryID           string              `json:"category_id"`
	Code                 string              `json:"code"`
	Barcode              *string             `json:"barcode"`
	Kind                 string              `json:"kind"`
	BasePrice            string              `json:"base_price"`
	ImageID              *string             `json:"image_id"`
	SortOrder            int                 `json:"sort_order"`
	Archived             bool                `json:"archived"`
	TemporaryUnavailable bool                `json:"temporary_unavailable"`
	Availability         Schedule            `json:"availability"`
	Variants             []Variant           `json:"variants"`
	Ingredients          []ProductIngredient `json:"ingredients"`
	Groups               []ProductGroup      `json:"groups"`
	Slots                []ComboSlot         `json:"slots"`
}

type PriceEntry struct {
	ProductID string  `json:"product_id"`
	VariantID *string `json:"variant_id"`
	Price     string  `json:"price"`
}

type PriceList struct {
	Metadata
	Default         bool         `json:"is_default"`
	Fallback        string       `json:"fallback"`
	AllowPromotions bool         `json:"allow_promotions"`
	Entries         []PriceEntry `json:"entries"`
}

type Promotion struct {
	Metadata
	Kind            string   `json:"kind"`
	Percent         string   `json:"percent"`
	SpecialPrice    string   `json:"special_price"`
	BuyQuantity     int      `json:"buy_quantity"`
	PayQuantity     int      `json:"pay_quantity"`
	MinimumQuantity int      `json:"minimum_quantity"`
	Priority        int      `json:"priority"`
	Stackable       bool     `json:"stackable"`
	IncludeExtras   bool     `json:"include_extras"`
	ProductIDs      []string `json:"product_ids"`
	CategoryIDs     []string `json:"category_ids"`
	PriceListIDs    []string `json:"price_list_ids"`
	Availability    Schedule `json:"availability"`
}
