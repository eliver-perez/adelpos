package documents

import "encoding/json"

// Template es el contenido de una versión de plantilla: opciones del documento y una lista
// ordenada de bloques de la lista cerrada. Se guarda como JSON canónico (Encode) en
// app.document_template_versions.layout.
type Template struct {
	Options TemplateOptions `json:"options"`
	Blocks  []Block         `json:"blocks"`
}

// TemplateOptions son las opciones del documento completo. TextSize solo admite inherit, normal o
// large en las familias de cocina (inherit = la opción «Tamaño de texto» de Cocina); en las demás
// familias es normal fijo.
type TemplateOptions struct {
	TextSize string `json:"text_size"`
}

// Block es un elemento de la plantilla con las propiedades comunes de todo bloque y sus opciones
// propias (esquema cerrado por tipo, decodificación estricta).
type Block struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Visible       bool            `json:"visible"`
	Align         string          `json:"align"`
	Size          string          `json:"size"`
	Bold          bool            `json:"bold"`
	Uppercase     bool            `json:"uppercase"`
	SpacingBefore int             `json:"spacing_before"`
	SpacingAfter  int             `json:"spacing_after"`
	Options       json.RawMessage `json:"options"`
	// parsed son las opciones ya decodificadas y validadas del tipo; el renderizador las usa.
	parsed any
}

// Alineaciones de un bloque: relleno con espacios dentro del ancho efectivo de la línea.
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
)

// Tipos de bloque de la lista cerrada (catalog.go describe cada uno para el editor).
const (
	// Comunes a los diez tipos.
	BlockLogo                 = "logo"
	BlockEstablishmentName    = "establishment_name"
	BlockEstablishmentAddress = "establishment_address"
	BlockEstablishmentPhone   = "establishment_phone"
	BlockEstablishmentEmail   = "establishment_email"
	BlockEstablishmentExtra   = "establishment_extra"
	BlockHeaderText           = "header_text"
	BlockFooterText           = "footer_text"
	BlockFixedText            = "fixed_text"
	BlockDocumentTitle        = "document_title"
	BlockDivider              = "divider"
	BlockSpacer               = "spacer"
	BlockField                = "field"
	// Precuenta y ticket.
	BlockProducts    = "products"
	BlockAdjustments = "adjustments"
	BlockTaxes       = "taxes"
	BlockTotal       = "total"
	BlockUSDTotal    = "usd_total"
	BlockThisPayment = "this_payment"
	BlockPayments    = "payments"
	BlockBalance     = "balance"
	// Comandas y avisos de cocina.
	BlockKitchenLocation = "kitchen_location"
	BlockKitchenItems    = "kitchen_items"
	// Comprobantes de caja.
	BlockVoucherDetail = "voucher_detail"
	BlockSignatures    = "signatures"
	// Cortes de caja.
	BlockOpeningFloat        = "opening_float"
	BlockSalesByMethod       = "sales_by_method"
	BlockShiftTotals         = "shift_totals"
	BlockCashMovements       = "cash_movements"
	BlockDeclaredCash        = "declared_cash"
	BlockExpectedDifferences = "expected_differences"
	BlockObservations        = "observations"
	// Aviso fiscal obligatorio (precuenta, ticket y documentos de caja).
	BlockFiscalNotice = "fiscal_notice"
)

// Opciones propias de cada tipo de bloque. Al decodificar, un bloque parte de las opciones
// predeterminadas de su tipo y solo sustituye las claves presentes; una clave desconocida se
// rechaza. La versión guardada lleva siempre todas las opciones (JSON canónico).

// EmptyOptions es el esquema de los bloques sin opciones.
type EmptyOptions struct{}

// EstablishmentNameOptions: en cocina, When decide si el nombre sigue la opción «Mostrar nombre del
// establecimiento» de Cocina (kitchen_setting) o se imprime siempre (always). En las demás
// familias no lleva opciones.
type EstablishmentNameOptions struct {
	When string `json:"when,omitempty"`
}

// Valores de EstablishmentNameOptions.When.
const (
	WhenKitchenSetting = "kitchen_setting"
	WhenAlways         = "always"
)

// FixedTextOptions es un texto fijo de hasta 300 caracteres y 8 líneas.
type FixedTextOptions struct {
	Text string `json:"text"`
}

// TitleOptions sustituye el título del tipo por Text (hasta 40 caracteres); vacío conserva el
// título del tipo. El del corte parcial siempre lleva su número.
type TitleOptions struct {
	Text string `json:"text"`
}

// DividerOptions es el carácter que repite el separador a todo el ancho.
type DividerOptions struct {
	Character string `json:"character"`
}

// SpacerOptions es el número de líneas en blanco (1–5).
type SpacerOptions struct {
	Lines int `json:"lines"`
}

// FieldItem es un dato del renglón: Key de la lista del tipo y Label impreso antes del valor
// (separado por un espacio; vacío imprime solo el valor).
type FieldItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// FieldOptions es un campo de datos: Items unidos con « · » (inline) o un solo dato con la etiqueta
// a la izquierda y el valor a la derecha (pair). OmitEmpty omite el renglón si todos los valores
// están vacíos.
type FieldOptions struct {
	Items     []FieldItem `json:"items"`
	Layout    string      `json:"layout"`
	OmitEmpty bool        `json:"omit_empty"`
}

// Disposiciones de un campo de datos.
const (
	FieldInline = "inline"
	FieldPair   = "pair"
)

// NotesOptions decide si la lista de productos imprime las observaciones de cada partida.
type NotesOptions struct {
	ShowNotes bool `json:"show_notes"`
}

// ReasonOptions decide si los descuentos y cortesías muestran su motivo («Descuento · Atención»).
type ReasonOptions struct {
	ShowReason bool `json:"show_reason"`
}

// TaxesOptions decide si se imprime el subtotal sin IVA de cada tasa además del IVA.
type TaxesOptions struct {
	ShowBase bool `json:"show_base"`
}

// SectionOptions controla el separador y el encabezado de un bloque que solo imprime algo cuando
// hay datos (este pago, pagos registrados, efectivo declarado, observaciones): el separador se
// imprime solo si el bloque imprime algo.
type SectionOptions struct {
	Divider bool `json:"divider"`
	Heading bool `json:"heading"`
}

// HeadingOptions controla el encabezado de un bloque de caja («VENTAS Y PAGOS POR MÉTODO»).
type HeadingOptions struct {
	Heading bool `json:"heading"`
}

// KitchenLocationOptions decide si la comanda para llevar imprime el nombre del cliente.
type KitchenLocationOptions struct {
	ShowCustomer bool `json:"show_customer"`
}

// Valores de las opciones heredables de la lista de cocina.
const (
	Inherit           = "inherit"
	OptionYes         = "yes"
	OptionNo          = "no"
	QuantityPrefix    = "prefix"
	QuantityMulti     = "multiplier"
	SpacingInherit    = "inherit"
	quickNoteNoEffect = "none"
)

// KitchenItemsOptions son las opciones de la lista de cocina: las cuatro primeras admiten inherit
// (la configuración de Cocina copiada en la comanda).
type KitchenItemsOptions struct {
	ShowNotes       string `json:"show_notes"`
	ShowPrices      string `json:"show_prices"`
	QuantityStyle   string `json:"quantity_style"`
	LineSpacing     string `json:"line_spacing"`
	StationDivider  bool   `json:"station_divider"`
	StationHeadings bool   `json:"show_station_headings"`
	ItemReference   bool   `json:"show_item_reference"`
}

// VoucherDetailOptions decide si el comprobante imprime la referencia y las observaciones.
type VoucherDetailOptions struct {
	ShowReference bool `json:"show_reference"`
	ShowNotes     bool `json:"show_notes"`
}

// SignaturesOptions decide si se imprime el estado «corregido» y la línea de firma.
type SignaturesOptions struct {
	ShowStatus    bool `json:"show_status"`
	SignatureLine bool `json:"signature_line"`
}
