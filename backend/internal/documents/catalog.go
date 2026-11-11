package documents

import "slices"

// Catálogo cerrado de bloques: qué tipos de documento acepta cada bloque, qué propiedades comunes
// le aplican, sus opciones con valores permitidos y predeterminados, cuántas veces puede aparecer y
// por qué puede no imprimir nada. Es la única fuente de verdad del editor (GET documents/catalog) y
// de la validación de Go.

// Propiedades comunes de un bloque.
const (
	PropertyVisible       = "visible"
	PropertyAlign         = "align"
	PropertySize          = "size"
	PropertyBold          = "bold"
	PropertyUppercase     = "uppercase"
	PropertySpacingBefore = "spacing_before"
	PropertySpacingAfter  = "spacing_after"
)

var allProperties = []string{
	PropertyVisible, PropertyAlign, PropertySize, PropertyBold, PropertyUppercase, PropertySpacingBefore, PropertySpacingAfter,
}

// Grupos del panel de elementos.
const (
	GroupCommon     = "common"
	GroupCommercial = "commercial"
	GroupKitchen    = "kitchen"
	GroupCash       = "cash"
)

// OptionValue es un valor permitido de una opción enumerada.
type OptionValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// OptionSpec describe una opción de un bloque para el editor. Type es enum, boolean, text, integer
// o field_items.
type OptionSpec struct {
	Key       string        `json:"key"`
	Label     string        `json:"label"`
	Type      string        `json:"type"`
	Values    []OptionValue `json:"values,omitempty"`
	Default   any           `json:"default"`
	MaxLength int           `json:"max_length,omitempty"`
	MaxLines  int           `json:"max_lines,omitempty"`
	Min       *int          `json:"min,omitempty"`
	Max       *int          `json:"max,omitempty"`
	Required  bool          `json:"required,omitempty"`
	// Help es una explicación breve que el editor muestra junto al control.
	Help string `json:"help,omitempty"`
}

// blockDefinition describe un tipo de bloque.
type blockDefinition struct {
	Type        string
	Name        string
	Group       string
	Description string
	Properties  []string
	// Max limita las apariciones en una plantilla (0 = sin límite propio, solo el de 60 bloques).
	Max int
	// Required exige exactamente uno, visible (aviso fiscal).
	Required bool
	// Protected marca el esperado y las diferencias de caja: solo cortes y solo con permiso.
	Protected bool
	accepts   func(kind Kind) bool
	// defaults devuelve las opciones predeterminadas del tipo para la clase de documento.
	defaults    func(kind Kind) any
	options     func(kind Kind) []OptionSpec
	EmptyReason string
}

func anyKind(Kind) bool {
	return true
}

func familyIs(family Family) func(Kind) bool {
	return func(kind Kind) bool {
		return kind.Family() == family
	}
}

func kindsAre(kinds ...Kind) func(Kind) bool {
	return func(kind Kind) bool {
		return slices.Contains(kinds, kind)
	}
}

func noOptions(Kind) any {
	return &EmptyOptions{}
}

func noOptionSpecs(Kind) []OptionSpec {
	return []OptionSpec{}
}

func integer(value int) *int {
	return &value
}

func booleanOption(key, label string, value bool) OptionSpec {
	return OptionSpec{Key: key, Label: label, Type: "boolean", Default: value}
}

var inheritValues = []OptionValue{
	{Inherit, "Según la configuración de Cocina"},
	{OptionYes, "Sí"},
	{OptionNo, "No"},
}

// priceValues son los valores de «Importes» de la lista de cocina. No existe «Sí»: la comanda solo
// guarda los precios cuando Cocina tiene activado «Mostrar precios» (kitchen/documents.go), así
// que una plantilla no puede imprimir importes que el documento no conserva (revisión de la 9B).
var priceValues = []OptionValue{
	{Inherit, "Según la configuración de Cocina"},
	{OptionNo, "Ocultar"},
}

// priceHelp explica cómo se imprimen los importes de la lista de cocina, con el nombre de la opción
// de Cocina que muestra los precios (KitchenSettingsForm.tsx).
const priceHelp = "Para imprimir los importes, activa «Incluir importes comerciales en comandas» en Cocina → " +
	"Comandas y tiempos; con «Ocultar», esta plantilla no los imprime aunque Cocina los muestre."

// Límites de las plantillas (§2 del diseño).
const (
	MaxBlocks         = 60
	MaxTemplateBytes  = 32 * 1024
	MaxFixedTextChars = 300
	MaxFixedTextLines = 8
	MaxLabelChars     = 40
	MaxFieldItems     = 4
	MaxSpacing        = 3
	MaxSpacerLines    = 5
)

// identityBlock describe un bloque que imprime un dato de la identidad del establecimiento.
func identityBlock(blockType, name, description, empty string) blockDefinition {
	return blockDefinition{
		Type: blockType, Name: name, Group: GroupCommon, Description: description, Properties: allProperties,
		accepts: anyKind, defaults: noOptions, options: noOptionSpecs, EmptyReason: empty,
	}
}

var definitions = []blockDefinition{
	{
		Type: BlockLogo, Name: "Logotipo", Group: GroupCommon,
		Description: "Imagen monocromática del logotipo del restaurante; su alineación y ancho se configuran en la identidad.",
		Properties:  []string{PropertyVisible, PropertySpacingBefore, PropertySpacingAfter},
		Max:         3, accepts: anyKind, defaults: noOptions, options: noOptionSpecs,
		EmptyReason: "No hay logotipo cargado: el bloque no imprime nada.",
	},
	{
		Type: BlockEstablishmentName, Name: "Nombre del establecimiento", Group: GroupCommon,
		Description: "Nombre comercial del establecimiento (Configuración general).",
		Properties:  allProperties, accepts: anyKind,
		defaults: func(kind Kind) any {
			if kind.Family() == FamilyKitchen {
				return &EstablishmentNameOptions{When: WhenKitchenSetting}
			}
			return &EstablishmentNameOptions{}
		},
		options: func(kind Kind) []OptionSpec {
			if kind.Family() != FamilyKitchen {
				return []OptionSpec{}
			}
			return []OptionSpec{{
				Key: "when", Label: "Cuándo se imprime", Type: "enum", Default: WhenKitchenSetting,
				Values: []OptionValue{
					{WhenKitchenSetting, "Según «Mostrar nombre del establecimiento» de Cocina"},
					{WhenAlways, "Siempre"},
				},
			}}
		},
		EmptyReason: "La configuración de Cocina no imprime el nombre del establecimiento.",
	},
	identityBlock(BlockEstablishmentAddress, "Dirección", "Dirección del establecimiento (Configuración general).",
		"El establecimiento no tiene dirección registrada."),
	identityBlock(BlockEstablishmentPhone, "Teléfono", "Teléfono del establecimiento (Configuración general).",
		"El establecimiento no tiene teléfono registrado."),
	identityBlock(BlockEstablishmentEmail, "Correo", "Correo del establecimiento (Configuración general).",
		"El establecimiento no tiene correo registrado."),
	identityBlock(BlockEstablishmentExtra, "Información adicional", "Información adicional autorizada de la identidad del establecimiento.",
		"La identidad del establecimiento no tiene información adicional."),
	identityBlock(BlockHeaderText, "Texto de encabezado", "Texto de encabezado de la identidad del establecimiento.",
		"La identidad del establecimiento no tiene texto de encabezado."),
	identityBlock(BlockFooterText, "Texto de pie", "Texto de pie de la identidad del establecimiento.",
		"La identidad del establecimiento no tiene texto de pie."),
	{
		Type: BlockFixedText, Name: "Texto fijo", Group: GroupCommon, Description: "Texto escrito en la plantilla.",
		Properties: allProperties, accepts: anyKind,
		defaults: func(Kind) any {
			return &FixedTextOptions{}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{{
				Key: "text", Label: "Texto", Type: "text", Default: "", MaxLength: MaxFixedTextChars, MaxLines: MaxFixedTextLines, Required: true,
			}}
		},
	},
	{
		Type: BlockDocumentTitle, Name: "Título del documento", Group: GroupCommon,
		Description: "Título del tipo de documento; el del corte parcial lleva su número.",
		Properties:  allProperties, accepts: anyKind,
		defaults: func(Kind) any {
			return &TitleOptions{}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{{Key: "text", Label: "Título personalizado (vacío: el del tipo)", Type: "text", Default: "", MaxLength: MaxLabelChars, MaxLines: 1}}
		},
	},
	{
		Type: BlockDivider, Name: "Separador", Group: GroupCommon, Description: "Línea repetida a todo el ancho.",
		Properties: allProperties, accepts: anyKind,
		defaults: func(Kind) any {
			return &DividerOptions{Character: "-"}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{{
				Key: "character", Label: "Carácter", Type: "enum", Default: "-",
				Values: []OptionValue{{"-", "Guiones"}, {"=", "Doble línea"}, {"*", "Asteriscos"}, {".", "Puntos"}},
			}}
		},
	},
	{
		Type: BlockSpacer, Name: "Espacio", Group: GroupCommon, Description: "Líneas en blanco.",
		Properties: []string{PropertyVisible}, accepts: anyKind,
		defaults: func(Kind) any {
			return &SpacerOptions{Lines: 1}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{{Key: "lines", Label: "Líneas en blanco", Type: "integer", Default: 1, Min: integer(1), Max: integer(MaxSpacerLines)}}
		},
	},
	{
		Type: BlockField, Name: "Campo de datos", Group: GroupCommon,
		Description: "Uno o varios datos del documento (fecha, folio, mesa, cajero…) en un renglón.",
		Properties:  allProperties, accepts: anyKind,
		defaults: func(Kind) any {
			return &FieldOptions{Items: []FieldItem{}, Layout: FieldInline, OmitEmpty: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{
				{Key: "items", Label: "Datos", Type: "field_items", Default: []FieldItem{}, MaxLength: MaxLabelChars, Min: integer(1), Max: integer(MaxFieldItems), Required: true},
				{
					Key: "layout", Label: "Disposición", Type: "enum", Default: FieldInline,
					Values: []OptionValue{{FieldInline, "En línea, unidos con «·»"}, {FieldPair, "Etiqueta a la izquierda y valor a la derecha (un solo dato)"}},
				},
				booleanOption("omit_empty", "Omitir el renglón si no hay datos", true),
			}
		},
		EmptyReason: "Los datos del campo están vacíos en este documento.",
	},
	{
		Type: BlockProducts, Name: "Lista de productos", Group: GroupCommercial,
		Description: "Partidas con cantidad «N x», nombre e importe.", Properties: allProperties, Max: 1,
		accepts: familyIs(FamilyCommercial),
		defaults: func(Kind) any {
			return &NotesOptions{ShowNotes: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("show_notes", "Imprimir observaciones", true)}
		},
		EmptyReason: "El documento no tiene partidas.",
	},
	{
		Type: BlockAdjustments, Name: "Descuentos y cargos", Group: GroupCommercial,
		Description: "Descuentos, cortesías y cargos con su motivo.", Properties: allProperties, Max: 1,
		accepts: familyIs(FamilyCommercial),
		defaults: func(Kind) any {
			return &ReasonOptions{ShowReason: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("show_reason", "Mostrar el motivo del descuento o cortesía", true)}
		},
		EmptyReason: "El documento no tiene descuentos ni cargos.",
	},
	{
		Type: BlockTaxes, Name: "Impuestos", Group: GroupCommercial,
		Description: "Subtotal sin IVA e IVA por tasa.", Properties: allProperties, Max: 1,
		accepts: familyIs(FamilyCommercial),
		defaults: func(Kind) any {
			return &TaxesOptions{ShowBase: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("show_base", "Imprimir el subtotal sin IVA", true)}
		},
		EmptyReason: "El documento no tiene desglose de IVA.",
	},
	{
		Type: BlockTotal, Name: "Total", Group: GroupCommercial, Description: "Total del documento.",
		Properties: allProperties, Max: 1, accepts: familyIs(FamilyCommercial), defaults: noOptions, options: noOptionSpecs,
	},
	{
		Type: BlockUSDTotal, Name: "Total en dólares", Group: GroupCommercial,
		Description: "Total equivalente en dólares con el tipo de cambio del turno (solo si existe).",
		Properties:  allProperties, Max: 1, accepts: familyIs(FamilyCommercial), defaults: noOptions, options: noOptionSpecs,
		EmptyReason: "Sin total en dólares: el pago en dólares está desactivado o el turno no tiene tipo de cambio.",
	},
	{
		Type: BlockThisPayment, Name: "Este pago", Group: GroupCommercial,
		Description: "El pago del ticket: método, efectivo recibido, dólares, cambio, referencia y propina.",
		Properties:  allProperties, Max: 1, accepts: kindsAre(KindReceipt),
		defaults: func(Kind) any {
			return &SectionOptions{Divider: true, Heading: true}
		},
		options:     sectionOptionSpecs("ESTE PAGO"),
		EmptyReason: "El documento no tiene un pago propio.",
	},
	{
		Type: BlockPayments, Name: "Pagos registrados", Group: GroupCommercial,
		Description: "Pagos de la cuenta con lo pagado y las propinas.", Properties: allProperties, Max: 1,
		accepts: familyIs(FamilyCommercial),
		defaults: func(Kind) any {
			return &SectionOptions{Divider: true, Heading: true}
		},
		options:     sectionOptionSpecs("PAGOS REGISTRADOS"),
		EmptyReason: "La cuenta no tiene pagos registrados.",
	},
	{
		Type: BlockBalance, Name: "Saldo pendiente", Group: GroupCommercial, Description: "Saldo pendiente de la cuenta (solo si existe).",
		Properties: allProperties, Max: 1, accepts: familyIs(FamilyCommercial), defaults: noOptions, options: noOptionSpecs,
		EmptyReason: "La cuenta no tiene saldo pendiente.",
	},
	{
		Type: BlockKitchenLocation, Name: "Ubicación", Group: GroupKitchen,
		Description: "Para llevar con su origen y cliente, o área y mesa.", Properties: allProperties,
		accepts: familyIs(FamilyKitchen),
		defaults: func(Kind) any {
			return &KitchenLocationOptions{ShowCustomer: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("show_customer", "Imprimir el nombre del cliente (para llevar)", true)}
		},
	},
	{
		Type: BlockKitchenItems, Name: "Lista de cocina", Group: GroupKitchen,
		Description: "Estaciones y partidas con cantidades, complementos, observaciones, antes/ahora y motivo.",
		Properties:  allProperties, Max: 1, accepts: familyIs(FamilyKitchen),
		defaults: func(Kind) any {
			return &KitchenItemsOptions{
				ShowNotes: Inherit, ShowPrices: Inherit, QuantityStyle: Inherit, LineSpacing: SpacingInherit,
				StationDivider: true, StationHeadings: true, ItemReference: true,
			}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{
				{Key: "show_notes", Label: "Observaciones", Type: "enum", Default: Inherit, Values: inheritValues},
				{Key: "show_prices", Label: "Importes", Type: "enum", Default: Inherit, Values: priceValues, Help: priceHelp},
				{
					Key: "quantity_style", Label: "Cantidades", Type: "enum", Default: Inherit,
					Values: []OptionValue{{Inherit, "Según la configuración de Cocina"}, {QuantityPrefix, "«2 x Producto»"}, {QuantityMulti, "«Producto × 2»"}},
				},
				{
					Key: "line_spacing", Label: "Espacio entre partidas", Type: "enum", Default: SpacingInherit,
					Values: []OptionValue{{SpacingInherit, "Según la configuración de Cocina"}, {"0", "Sin espacio"}, {"1", "1 línea"}, {"2", "2 líneas"}, {"3", "3 líneas"}},
				},
				booleanOption("station_divider", "Separador antes de cada estación", true),
				booleanOption("show_station_headings", "Nombre de cada estación", true),
				booleanOption("show_item_reference", "Referencia «Partida: …»", true),
			}
		},
		EmptyReason: "El documento no tiene partidas.",
	},
	{
		Type: BlockVoucherDetail, Name: "Detalle del comprobante", Group: GroupCash,
		Description: "Categoría, tipo o motivo, concepto, importe, tipo de cambio, referencia, observaciones y salida excepcional.",
		Properties:  allProperties, Max: 1, accepts: func(kind Kind) bool { return kind.IsVoucher() },
		defaults: func(Kind) any {
			return &VoucherDetailOptions{ShowReference: true, ShowNotes: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{
				booleanOption("show_reference", "Imprimir la referencia", true),
				booleanOption("show_notes", "Imprimir las observaciones", true),
			}
		},
	},
	{
		Type: BlockSignatures, Name: "Firmas", Group: GroupCash,
		Description: "Quién registró y quién autorizó, estado y línea de firma.", Properties: allProperties, Max: 1,
		accepts: func(kind Kind) bool { return kind.IsVoucher() },
		defaults: func(Kind) any {
			return &SignaturesOptions{ShowStatus: true, SignatureLine: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{
				booleanOption("show_status", "Indicar si fue corregido", true),
				booleanOption("signature_line", "Línea de firma", true),
			}
		},
	},
	{
		Type: BlockOpeningFloat, Name: "Fondo inicial", Group: GroupCash, Description: "Fondo inicial en pesos y dólares y tipo de cambio del turno.",
		Properties: allProperties, Max: 1, accepts: func(kind Kind) bool { return kind.IsCut() }, defaults: noOptions, options: noOptionSpecs,
	},
	{
		Type: BlockSalesByMethod, Name: "Ventas y pagos por método", Group: GroupCash,
		Description: "Ventas, propinas y devoluciones de cada método de pago.", Properties: allProperties, Max: 1,
		accepts: func(kind Kind) bool { return kind.IsCut() },
		defaults: func(Kind) any {
			return &HeadingOptions{Heading: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("heading", "Encabezado «VENTAS Y PAGOS POR MÉTODO»", true)}
		},
	},
	{
		Type: BlockShiftTotals, Name: "Totales del turno", Group: GroupCash,
		Description: "Ventas, devoluciones, propinas, descuentos, cortesías y cobros registrados.", Properties: allProperties, Max: 1,
		accepts: func(kind Kind) bool { return kind.IsCut() }, defaults: noOptions, options: noOptionSpecs,
	},
	{
		Type: BlockCashMovements, Name: "Ingresos, retiros y gastos", Group: GroupCash,
		Description: "Operaciones de efectivo por moneda con correcciones y regularizaciones.", Properties: allProperties, Max: 1,
		accepts: func(kind Kind) bool { return kind.IsCut() },
		defaults: func(Kind) any {
			return &HeadingOptions{Heading: true}
		},
		options: func(Kind) []OptionSpec {
			return []OptionSpec{booleanOption("heading", "Encabezado «INGRESOS, RETIROS Y GASTOS»", true)}
		},
	},
	{
		Type: BlockDeclaredCash, Name: "Efectivo declarado", Group: GroupCash, Description: "Lo que el cajero contó en el arqueo.",
		Properties: allProperties, Max: 1, accepts: kindsAre(KindCashFinalCut),
		defaults: func(Kind) any {
			return &SectionOptions{Divider: true, Heading: true}
		},
		options:     sectionOptionSpecs("EFECTIVO DECLARADO"),
		EmptyReason: "El corte no tiene efectivo declarado.",
	},
	{
		Type: BlockExpectedDifferences, Name: "Esperado y diferencias", Group: GroupCash,
		Description: "Efectivo esperado y diferencias del arqueo. Solo se imprime para quien tiene permiso de consultarlos y nunca durante un arqueo; si no, imprime «Esperado y diferencias: no incluidos.».",
		Properties:  allProperties, Max: 1, Protected: true, accepts: func(kind Kind) bool { return kind.IsCut() },
		defaults: noOptions, options: noOptionSpecs,
	},
	{
		Type: BlockObservations, Name: "Observaciones", Group: GroupCash, Description: "Observaciones del cajero en el arqueo.",
		Properties: allProperties, Max: 1, accepts: kindsAre(KindCashFinalCut),
		defaults: func(Kind) any {
			return &SectionOptions{Divider: true, Heading: true}
		},
		options:     sectionOptionSpecs("OBSERVACIONES"),
		EmptyReason: "El corte no tiene observaciones.",
	},
	{
		Type: BlockFiscalNotice, Name: "Aviso fiscal", Group: GroupCommon,
		Description: "Aviso obligatorio de que el documento no es un comprobante fiscal (CFDI). Se puede mover y cambiar su alineación, tamaño y énfasis; no se puede quitar ni ocultar.",
		Properties:  allProperties, Max: 1, Required: true,
		accepts: func(kind Kind) bool { return kind.Family() != FamilyKitchen }, defaults: noOptions, options: noOptionSpecs,
	},
}

func sectionOptionSpecs(heading string) func(Kind) []OptionSpec {
	return func(Kind) []OptionSpec {
		return []OptionSpec{
			booleanOption("divider", "Separador antes del bloque (solo si imprime algo)", true),
			booleanOption("heading", "Encabezado «"+heading+"»", true),
		}
	}
}

// definition devuelve la descripción de un tipo de bloque.
func definition(blockType string) (*blockDefinition, bool) {
	for index := range definitions {
		if definitions[index].Type == blockType {
			return &definitions[index], true
		}
	}
	return nil, false
}

// FiscalNotice es el texto obligatorio que identifica el documento como no fiscal.
func FiscalNotice(kind Kind) string {
	if kind.Family() == FamilyCash {
		return "Documento interno de caja. No es un comprobante fiscal (CFDI)."
	}
	return "Documento comercial. No es un comprobante fiscal (CFDI)."
}

// Respuesta del catálogo para el editor.

// CatalogBlock es un tipo de bloque compatible con un tipo de documento.
type CatalogBlock struct {
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	Group       string       `json:"group"`
	Description string       `json:"description"`
	Properties  []string     `json:"properties"`
	Options     []OptionSpec `json:"options"`
	Max         int          `json:"max,omitempty"`
	Required    bool         `json:"required"`
	Protected   bool         `json:"protected"`
}

// CatalogKind es un tipo de documento con sus bloques, sus datos de campo y los atajos del editor.
type CatalogKind struct {
	Kind      Kind            `json:"kind"`
	Name      string          `json:"name"`
	Family    Family          `json:"family"`
	TextSizes []string        `json:"text_sizes"`
	Blocks    []CatalogBlock  `json:"blocks"`
	FieldKeys []FieldKeyInfo  `json:"field_keys"`
	Shortcuts []FieldShortcut `json:"shortcuts"`
}

// PaperOption es un ancho de papel con sus columnas en tamaño normal.
type PaperOption struct {
	PaperWidth int `json:"paper_width"`
	Columns    int `json:"columns"`
}

// CatalogLimits son los límites que valida Go al guardar.
type CatalogLimits struct {
	Blocks         int `json:"blocks"`
	Bytes          int `json:"bytes"`
	FixedTextChars int `json:"fixed_text_chars"`
	FixedTextLines int `json:"fixed_text_lines"`
	LabelChars     int `json:"label_chars"`
	FieldItems     int `json:"field_items"`
	Spacing        int `json:"spacing"`
	SpacerLines    int `json:"spacer_lines"`
}

// CatalogProperties son los valores admitidos de las propiedades comunes.
type CatalogProperties struct {
	Align   []OptionValue `json:"align"`
	Size    []OptionValue `json:"size"`
	Spacing IntRange      `json:"spacing"`
}

// IntRange es un intervalo entero cerrado.
type IntRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Catalog es la respuesta de GET documents/catalog.
type Catalog struct {
	Kinds       []CatalogKind     `json:"kinds"`
	PaperWidths []PaperOption     `json:"paper_widths"`
	Properties  CatalogProperties `json:"properties"`
	Limits      CatalogLimits     `json:"limits"`
}

// BuildCatalog arma el catálogo completo del editor.
func BuildCatalog() Catalog {
	catalog := Catalog{
		Kinds:       []CatalogKind{},
		PaperWidths: []PaperOption{{PaperWidth: 58, Columns: 30}, {PaperWidth: 80, Columns: 42}},
		Properties: CatalogProperties{
			Align: []OptionValue{{AlignLeft, "Izquierda"}, {AlignCenter, "Centro"}, {AlignRight, "Derecha"}},
			Size: []OptionValue{
				{SizeNormal, "Normal"}, {SizeDoubleHeight, "Doble alto"}, {SizeDoubleWidth, "Doble ancho"}, {SizeDouble, "Doble"},
			},
			Spacing: IntRange{Min: 0, Max: MaxSpacing},
		},
		Limits: CatalogLimits{
			Blocks: MaxBlocks, Bytes: MaxTemplateBytes, FixedTextChars: MaxFixedTextChars, FixedTextLines: MaxFixedTextLines,
			LabelChars: MaxLabelChars, FieldItems: MaxFieldItems, Spacing: MaxSpacing, SpacerLines: MaxSpacerLines,
		},
	}
	for _, info := range Kinds {
		entry := CatalogKind{
			Kind: info.Kind, Name: info.Name, Family: info.Family, TextSizes: textSizesFor(info.Kind),
			Blocks: []CatalogBlock{}, FieldKeys: fieldKeysFor(info.Kind), Shortcuts: shortcutsFor(info.Kind),
		}
		for _, item := range definitions {
			if !item.accepts(info.Kind) {
				continue
			}
			entry.Blocks = append(entry.Blocks, CatalogBlock{
				Type: item.Type, Name: item.Name, Group: item.Group, Description: item.Description, Properties: item.Properties,
				Options: item.options(info.Kind), Max: item.Max, Required: item.Required, Protected: item.Protected,
			})
		}
		catalog.Kinds = append(catalog.Kinds, entry)
	}
	return catalog
}

// textSizesFor son los tamaños de documento admitidos por el tipo.
func textSizesFor(kind Kind) []string {
	if kind.Family() == FamilyKitchen {
		return []string{TextSizeInherit, TextSizeNormal, TextSizeLarge}
	}
	return []string{TextSizeNormal}
}
