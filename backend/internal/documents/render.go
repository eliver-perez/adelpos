package documents

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
)

// Renderizador único: convierte una plantilla y los datos de un documento en la IR. La vista previa
// del editor, la impresión, la reimpresión desde la cola y la impresión con la plantilla vigente
// llaman a esta misma función; solo cambian los datos, el ancho y si se incluye lo protegido.

// RenderOptions son las condiciones del documento que no dependen de la plantilla.
type RenderOptions struct {
	// PaperWidth es el ancho del papel en milímetros (58 u 80).
	PaperWidth int
	// Columns sustituye el ancho en caracteres calculado del papel; cero usa el del papel (30 o 42).
	// Solo lo usan las envolturas de compatibilidad de las pruebas anteriores a la 9B.
	Columns int
	// IncludeExpected permite el bloque protegido de los cortes: solo cuando quien imprime tiene
	// cash.expected.read y el turno no está en arqueo (lo decide el módulo de caja).
	IncludeExpected bool
}

// ColumnsFor es el ancho en caracteres de una línea normal para un ancho de papel.
func ColumnsFor(paperWidth int) int {
	if paperWidth == 58 {
		return 30
	}
	return 42
}

// pendingLine es una línea producida por un bloque antes de aplicar alineación y atributos.
// justified marca los renglones «concepto … importe» y los separadores, que ocupan todo el ancho.
type pendingLine struct {
	text      string
	justified bool
	image     *Image
}

// writer acumula las líneas de un bloque con su ancho efectivo y la regla de corte de su familia.
type writer struct {
	family Family
	width  int
	upper  bool
	lines  []pendingLine
}

func (w *writer) transform(value string) string {
	if w.upper {
		return strings.ToUpper(value)
	}
	return value
}

// wrap corta un texto con la regla de la familia (cocina por carácter conservando saltos,
// comerciales por carácter y caja por palabras).
func (w *writer) wrap(value string) []string {
	switch w.family {
	case FamilyKitchen:
		return wrapKitchen(value, w.width)
	case FamilyCash:
		return wrapWords(value, w.width)
	}
	return fit(value, w.width)
}

// text agrega un texto cortado al ancho del bloque.
func (w *writer) text(value string) {
	for _, line := range w.wrap(w.transform(value)) {
		w.lines = append(w.lines, pendingLine{text: line})
	}
}

// freeText agrega un texto de varias líneas (texto fijo o de la identidad): cada línea escrita se
// corta por separado.
func (w *writer) freeText(value string) {
	for _, line := range strings.Split(value, "\n") {
		w.text(line)
	}
}

// row agrega un renglón «concepto … importe» justificado.
func (w *writer) row(left, right string) {
	for _, line := range row(w.transform(left), w.transform(right), w.width) {
		w.lines = append(w.lines, pendingLine{text: line, justified: true})
	}
}

// rule agrega un separador a todo el ancho.
func (w *writer) rule(character string) {
	w.lines = append(w.lines, pendingLine{text: strings.Repeat(character, w.width), justified: true})
}

func (w *writer) image(image Image) {
	w.lines = append(w.lines, pendingLine{image: &image})
}

// Render produce la IR de un documento con la plantilla indicada.
func Render(kind Kind, template *Template, data Data, options RenderOptions) (*Layout, error) {
	family := kind.Family()
	if family == "" {
		return nil, errors.New("tipo de documento desconocido")
	}
	if !dataMatches(family, data) {
		return nil, errors.New("los datos no corresponden al tipo de documento")
	}
	if template == nil {
		template = Default(kind)
	}
	paper := options.PaperWidth
	if paper != 58 {
		paper = 80
	}
	columns := ColumnsFor(paper)
	if options.Columns > 0 {
		columns = options.Columns
	}
	textSize := resolveTextSize(template, data)
	if textSize == TextSizeLarge {
		columns /= 2
	}
	layout := &Layout{
		PaperWidth: paper, Columns: columns, TextSize: textSize, TrailingNewline: family == FamilyKitchen, Lines: []Line{},
	}
	for _, block := range template.Blocks {
		if !block.Visible {
			continue
		}
		renderOne(kind, block, data, options, layout)
	}
	return layout, nil
}

func dataMatches(family Family, data Data) bool {
	switch family {
	case FamilyKitchen:
		return data.Kitchen != nil
	case FamilyCommercial:
		return data.Commercial != nil
	case FamilyCash:
		return data.Cash != nil
	}
	return false
}

// resolveTextSize decide el tamaño del documento: en cocina, inherit toma la opción de Cocina
// copiada en la comanda; las demás familias siempre son de tamaño normal.
func resolveTextSize(template *Template, data Data) string {
	if data.Kitchen == nil {
		return TextSizeNormal
	}
	size := template.Options.TextSize
	if size == "" || size == TextSizeInherit {
		size = data.Kitchen.Settings.TextSize
	}
	if size == TextSizeLarge {
		return TextSizeLarge
	}
	return TextSizeNormal
}

// EffectiveTextSize es el tamaño del documento completo que imprimirá una comanda con la plantilla:
// la cola lo necesita para el camino de texto (cpi de CUPS y GS ! de ESC/POS).
func EffectiveTextSize(template *Template, settingsTextSize string) string {
	return resolveTextSize(template, Data{Kitchen: &KitchenData{Settings: KitchenSettings{TextSize: settingsTextSize}}})
}

// renderOne aplica un bloque: produce sus líneas con su ancho efectivo y después la alineación, el
// énfasis, el tamaño y el espaciado. Un bloque visible sin líneas se informa en EmptyBlocks.
func renderOne(kind Kind, block Block, data Data, options RenderOptions, layout *Layout) {
	item, found := definition(block.Type)
	if !found {
		return
	}
	// Un bloque armado sin ParseTemplate (por ejemplo, decodificado directamente) parte de las
	// opciones predeterminadas de su tipo.
	if block.parsed == nil {
		options := item.defaults(kind)
		if len(block.Options) > 0 {
			_ = json.Unmarshal(block.Options, options)
		}
		block.parsed = options
	}
	applies := func(property string) bool {
		return slices.Contains(item.Properties, property)
	}
	size := SizeNormal
	// Con el texto grande de Cocina todo el documento ya va a doble tamaño: los tamaños por bloque
	// se ignoran, como antes de la 9B.
	if layout.TextSize != TextSizeLarge && applies(PropertySize) && block.Size != "" {
		size = block.Size
	}
	width := layout.Columns
	if size == SizeDoubleWidth || size == SizeDouble {
		width = max(1, layout.Columns/2)
	}
	w := &writer{family: kind.Family(), width: width, upper: block.Uppercase && applies(PropertyUppercase)}
	renderBlockContent(kind, block, data, options, w, layout)
	if len(w.lines) == 0 {
		reason := item.EmptyReason
		if reason == "" {
			reason = "El elemento no tiene datos para imprimir en este documento."
		}
		layout.EmptyBlocks = append(layout.EmptyBlocks, EmptyBlock{BlockID: block.ID, Reason: reason})
		return
	}
	spacing := func(count int) {
		for index := 0; index < count; index++ {
			layout.Lines = append(layout.Lines, Line{Text: "", Size: SizeNormal, BlockID: block.ID})
		}
	}
	if applies(PropertySpacingBefore) {
		spacing(block.SpacingBefore)
	}
	bold := block.Bold && applies(PropertyBold)
	for _, pending := range w.lines {
		if pending.image != nil {
			image := *pending.image
			layout.Lines = append(layout.Lines, Line{Size: SizeNormal, Image: &image, BlockID: block.ID})
			continue
		}
		text := pending.text
		if !pending.justified && applies(PropertyAlign) {
			text = alignText(text, block.Align, width)
		}
		layout.Lines = append(layout.Lines, Line{Text: text, Bold: bold, Size: size, BlockID: block.ID})
	}
	if applies(PropertySpacingAfter) {
		spacing(block.SpacingAfter)
	}
}

// alignText rellena con espacios a la izquierda para centrar o alinear a la derecha dentro del
// ancho efectivo; la alineación izquierda deja la línea como está.
func alignText(text, align string, width int) string {
	free := width - runeCount(text)
	if free <= 0 || text == "" {
		return text
	}
	switch align {
	case AlignCenter:
		return strings.Repeat(" ", free/2) + text
	case AlignRight:
		return strings.Repeat(" ", free) + text
	}
	return text
}

// renderBlockContent produce las líneas propias de cada tipo de bloque.
func renderBlockContent(kind Kind, block Block, data Data, options RenderOptions, w *writer, layout *Layout) {
	identity := identityOf(data)
	switch block.Type {
	case BlockLogo:
		renderLogo(identity, layout, w)
	case BlockEstablishmentName:
		renderEstablishmentName(block, data, identity, w)
	case BlockEstablishmentAddress:
		w.freeTextIfAny(identity.Address)
	case BlockEstablishmentPhone:
		w.freeTextIfAny(identity.Phone)
	case BlockEstablishmentEmail:
		w.freeTextIfAny(identity.Email)
	case BlockEstablishmentExtra:
		w.freeTextIfAny(identity.Extra)
	case BlockHeaderText:
		w.freeTextIfAny(identity.Header)
	case BlockFooterText:
		w.freeTextIfAny(identity.Footer)
	case BlockFixedText:
		if parsed, ok := block.parsed.(*FixedTextOptions); ok {
			w.freeText(parsed.Text)
		}
	case BlockDocumentTitle:
		w.text(documentTitle(kind, block, data))
	case BlockDivider:
		character := "-"
		if parsed, ok := block.parsed.(*DividerOptions); ok {
			character = parsed.Character
		}
		w.rule(character)
	case BlockSpacer:
		lines := 1
		if parsed, ok := block.parsed.(*SpacerOptions); ok {
			lines = parsed.Lines
		}
		for index := 0; index < lines; index++ {
			w.lines = append(w.lines, pendingLine{text: ""})
		}
	case BlockField:
		if parsed, ok := block.parsed.(*FieldOptions); ok {
			renderField(kind, parsed, data, w)
		}
	case BlockFiscalNotice:
		w.text(FiscalNotice(kind))
	default:
		switch kind.Family() {
		case FamilyKitchen:
			renderKitchenBlock(block, data.Kitchen, w)
		case FamilyCommercial:
			renderCommercialBlock(block, data.Commercial, w)
		case FamilyCash:
			renderCashBlock(block, data.Cash, options, w)
		}
	}
}

// freeTextIfAny imprime un dato de la identidad solo si tiene contenido.
func (w *writer) freeTextIfAny(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	w.freeText(value)
}

func identityOf(data Data) Identity {
	switch {
	case data.Kitchen != nil:
		return data.Kitchen.Identity
	case data.Commercial != nil:
		return data.Commercial.Identity
	case data.Cash != nil:
		return data.Cash.Identity
	}
	return Identity{}
}

// renderLogo agrega la línea de imagen del logotipo del ancho del papel; sin logotipo no imprime
// nada y deja un aviso para la configuración.
func renderLogo(identity Identity, layout *Layout, w *writer) {
	logo := identity.logoFor(layout.PaperWidth)
	if logo == nil {
		warning := "No hay logotipo cargado: el bloque no imprime nada."
		if !slices.Contains(layout.Warnings, warning) {
			layout.Warnings = append(layout.Warnings, warning)
		}
		return
	}
	w.image(Image{RasterID: logo.RasterID, WidthDots: logo.WidthDots, HeightDots: logo.HeightDots})
}

// renderEstablishmentName imprime el nombre comercial. En cocina, con «según Cocina», solo si la
// configuración copiada en la comanda lo muestra. El nombre siempre se imprime aunque esté vacío,
// como en las plantillas anteriores.
func renderEstablishmentName(block Block, data Data, identity Identity, w *writer) {
	if data.Kitchen != nil {
		options, _ := block.parsed.(*EstablishmentNameOptions)
		when := WhenKitchenSetting
		if options != nil && options.When != "" {
			when = options.When
		}
		if when == WhenKitchenSetting && !data.Kitchen.Settings.ShowEstablishment {
			return
		}
	}
	w.text(identity.Name)
}

// documentTitle es el título del tipo o el texto que lo sustituye.
func documentTitle(kind Kind, block Block, data Data) string {
	custom := ""
	if parsed, ok := block.parsed.(*TitleOptions); ok {
		custom = parsed.Text
	}
	switch kind.Family() {
	case FamilyKitchen:
		if custom != "" {
			return custom
		}
		switch data.Kitchen.TicketKind {
		case "modification":
			return "MODIFICACIÓN"
		case "cancellation":
			return "CANCELACIÓN"
		case "test":
			return "PRUEBA DE IMPRESIÓN"
		}
		return "COMANDA"
	case FamilyCommercial:
		if custom != "" {
			return custom
		}
		if kind == KindPrecheck {
			return "PRECUENTA"
		}
		return "TICKET DE PAGO"
	}
	sequence := 0
	if data.Cash.Cut != nil {
		sequence = data.Cash.Cut.Sequence
	}
	if custom != "" {
		if kind == KindCashPartialCut {
			return custom + " #" + strconv.Itoa(sequence)
		}
		return custom
	}
	return CashTitle(kind, sequence)
}

// CashTitle es el título impreso de un documento de caja.
func CashTitle(kind Kind, sequence int) string {
	switch kind {
	case KindCashPartialCut:
		return "CORTE PARCIAL #" + strconv.Itoa(sequence)
	case KindCashFinalCut:
		return "CORTE DEFINITIVO"
	case KindCashIncome:
		return "COMPROBANTE DE INGRESO"
	case KindCashWithdrawal:
		return "COMPROBANTE DE RETIRO"
	}
	return "COMPROBANTE DE GASTO"
}

// renderField imprime un campo de datos en línea («Etiqueta valor · valor») o como renglón
// etiqueta-valor. Con OmitEmpty, un renglón sin ningún valor no se imprime.
func renderField(kind Kind, options *FieldOptions, data Data, w *writer) {
	parts := []string{}
	anyValue := false
	for _, item := range options.Items {
		value, present := fieldValue(kind, item.Key, data)
		if present {
			anyValue = true
		}
		if item.Label == "" {
			parts = append(parts, value)
		} else {
			parts = append(parts, item.Label+" "+value)
		}
	}
	if options.OmitEmpty && !anyValue {
		return
	}
	if options.Layout == FieldPair && len(options.Items) == 1 {
		value, _ := fieldValue(kind, options.Items[0].Key, data)
		w.row(options.Items[0].Label, value)
		return
	}
	w.text(strings.Join(parts, " · "))
}
