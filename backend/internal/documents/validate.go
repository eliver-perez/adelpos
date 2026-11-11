package documents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Validación estricta de una plantilla. Go es la autoridad: el editor solo ofrece los bloques
// compatibles, pero cualquier JSON se valida aquí antes de guardarse (§24 de la especificación).
// Una plantilla válida se normaliza (opciones completas, textos saneados) y se guarda en su forma
// canónica (Encode).

// ValidationError es un rechazo de la plantilla con el bloque afectado (vacío si es del documento).
type ValidationError struct {
	BlockID string
	Message string
}

func (problem *ValidationError) Error() string {
	return problem.Message
}

func invalidTemplate(blockID, message string) error {
	return &ValidationError{BlockID: blockID, Message: message}
}

var blockIDPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// rawBlock es un bloque tal como llega, con las propiedades comunes predeterminadas: las ausentes
// conservan su valor (visible, izquierda, normal, sin negrita ni mayúsculas, sin espaciado).
type rawBlock struct {
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
}

// strictDecode decodifica un objeto JSON rechazando claves desconocidas y contenido sobrante.
func strictDecode(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("contenido sobrante")
	}
	return nil
}

// ParseTemplate decodifica, valida y normaliza el JSON de una plantilla para un tipo de documento.
func ParseTemplate(kind Kind, raw []byte) (*Template, error) {
	if kind.Family() == "" {
		return nil, invalidTemplate("", "El tipo de documento no existe.")
	}
	if len(raw) > MaxTemplateBytes {
		return nil, invalidTemplate("", "La plantilla excede el tamaño máximo de 32 KiB.")
	}
	var envelope struct {
		Options json.RawMessage   `json:"options"`
		Blocks  []json.RawMessage `json:"blocks"`
	}
	if err := strictDecode(raw, &envelope); err != nil {
		return nil, invalidTemplate("", "La plantilla no tiene la estructura esperada (opciones y bloques).")
	}
	template := &Template{Options: TemplateOptions{TextSize: defaultTextSize(kind)}, Blocks: []Block{}}
	if len(envelope.Options) > 0 && string(envelope.Options) != "null" {
		if err := strictDecode(envelope.Options, &template.Options); err != nil {
			return nil, invalidTemplate("", "Las opciones del documento no son válidas.")
		}
	}
	if err := validateTextSize(kind, template.Options.TextSize); err != nil {
		return nil, err
	}
	if len(envelope.Blocks) == 0 {
		return nil, invalidTemplate("", "La plantilla debe tener al menos un elemento.")
	}
	if len(envelope.Blocks) > MaxBlocks {
		return nil, invalidTemplate("", "La plantilla admite como máximo 60 elementos.")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for index, encoded := range envelope.Blocks {
		block, err := parseBlock(kind, index, encoded)
		if err != nil {
			return nil, err
		}
		if seen[block.ID] {
			return nil, invalidTemplate(block.ID, "Dos elementos de la plantilla tienen el mismo identificador.")
		}
		seen[block.ID] = true
		counts[block.Type]++
		template.Blocks = append(template.Blocks, block)
	}
	if err := validateCardinality(kind, template, counts); err != nil {
		return nil, err
	}
	return template, nil
}

func defaultTextSize(kind Kind) string {
	if kind.Family() == FamilyKitchen {
		return TextSizeInherit
	}
	return TextSizeNormal
}

func validateTextSize(kind Kind, value string) error {
	if !slices.Contains(textSizesFor(kind), value) {
		if kind.Family() == FamilyKitchen {
			return invalidTemplate("", "El tamaño de texto debe ser el de Cocina, normal o grande.")
		}
		return invalidTemplate("", "Este documento solo admite el tamaño de texto normal.")
	}
	return nil
}

// parseBlock valida las propiedades comunes de un bloque y decodifica sus opciones con el esquema
// cerrado de su tipo.
func parseBlock(kind Kind, index int, encoded json.RawMessage) (Block, error) {
	draft := rawBlock{Visible: true, Align: AlignLeft, Size: SizeNormal}
	if err := strictDecode(encoded, &draft); err != nil {
		return Block{}, invalidTemplate(blockIDAt(encoded), fmt.Sprintf("El elemento %d tiene propiedades desconocidas o de un tipo incorrecto.", index+1))
	}
	if !blockIDPattern.MatchString(draft.ID) {
		return Block{}, invalidTemplate("", fmt.Sprintf("El elemento %d no tiene un identificador válido.", index+1))
	}
	item, found := definition(draft.Type)
	if !found {
		return Block{}, invalidTemplate(draft.ID, "El elemento no es de un tipo conocido.")
	}
	if !item.accepts(kind) {
		if item.Protected {
			return Block{}, invalidTemplate(draft.ID, "«"+item.Name+"» solo puede incluirse en el corte parcial y en el corte definitivo.")
		}
		return Block{}, invalidTemplate(draft.ID, "«"+item.Name+"» no está disponible para "+strings.ToLower(kind.Name())+".")
	}
	if err := validateCommonProperties(draft); err != nil {
		return Block{}, err
	}
	options := item.defaults(kind)
	if len(draft.Options) > 0 && string(draft.Options) != "null" {
		if err := strictDecode(draft.Options, options); err != nil {
			return Block{}, invalidTemplate(draft.ID, "Las opciones de «"+item.Name+"» no son válidas.")
		}
	}
	if err := normalizeOptions(kind, draft.ID, item, options); err != nil {
		return Block{}, err
	}
	canonical, err := json.Marshal(options)
	if err != nil {
		return Block{}, err
	}
	return Block{
		ID: draft.ID, Type: draft.Type, Visible: draft.Visible, Align: draft.Align, Size: draft.Size, Bold: draft.Bold,
		Uppercase: draft.Uppercase, SpacingBefore: draft.SpacingBefore, SpacingAfter: draft.SpacingAfter,
		Options: canonical, parsed: options,
	}, nil
}

// blockIDAt intenta leer el identificador de un bloque inválido para señalarlo en el editor.
func blockIDAt(encoded json.RawMessage) string {
	var probe struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(encoded, &probe) == nil && blockIDPattern.MatchString(probe.ID) {
		return probe.ID
	}
	return ""
}

func validateCommonProperties(block rawBlock) error {
	if !slices.Contains([]string{AlignLeft, AlignCenter, AlignRight}, block.Align) {
		return invalidTemplate(block.ID, "La alineación debe ser izquierda, centro o derecha.")
	}
	if !slices.Contains([]string{SizeNormal, SizeDoubleHeight, SizeDoubleWidth, SizeDouble}, block.Size) {
		return invalidTemplate(block.ID, "El tamaño debe ser normal, doble alto, doble ancho o doble.")
	}
	if block.SpacingBefore < 0 || block.SpacingBefore > MaxSpacing || block.SpacingAfter < 0 || block.SpacingAfter > MaxSpacing {
		return invalidTemplate(block.ID, "El espaciado antes y después debe estar entre 0 y 3 líneas.")
	}
	return nil
}

// validateCardinality exige el aviso fiscal donde corresponde y los máximos por tipo de bloque.
func validateCardinality(kind Kind, template *Template, counts map[string]int) error {
	for _, item := range definitions {
		if !item.accepts(kind) {
			continue
		}
		if item.Required && counts[item.Type] != 1 {
			return invalidTemplate("", "La plantilla debe incluir exactamente un «"+item.Name+"».")
		}
		if item.Max > 0 && counts[item.Type] > item.Max {
			return invalidTemplate(lastBlockOf(template, item.Type), "«"+item.Name+"» admite como máximo "+strconv.Itoa(item.Max)+" en la plantilla.")
		}
	}
	for _, block := range template.Blocks {
		item, _ := definition(block.Type)
		if item.Required && !block.Visible {
			return invalidTemplate(block.ID, "«"+item.Name+"» es obligatorio y no se puede ocultar.")
		}
	}
	return nil
}

func lastBlockOf(template *Template, blockType string) string {
	identifier := ""
	for _, block := range template.Blocks {
		if block.Type == blockType {
			identifier = block.ID
		}
	}
	return identifier
}

// normalizeOptions valida las opciones de un bloque y sanea sus textos.
func normalizeOptions(kind Kind, blockID string, item *blockDefinition, options any) error {
	switch value := options.(type) {
	case *EstablishmentNameOptions:
		if kind.Family() == FamilyKitchen {
			if value.When != WhenKitchenSetting && value.When != WhenAlways {
				return invalidTemplate(blockID, "Indica cuándo se imprime el nombre del establecimiento.")
			}
		} else if value.When != "" {
			return invalidTemplate(blockID, "Las opciones de «"+item.Name+"» no son válidas.")
		}
	case *FixedTextOptions:
		text, err := cleanFreeText(blockID, value.Text, MaxFixedTextChars, MaxFixedTextLines)
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			return invalidTemplate(blockID, "Escribe el texto fijo.")
		}
		value.Text = text
	case *TitleOptions:
		text, err := cleanFreeText(blockID, value.Text, MaxLabelChars, 1)
		if err != nil {
			return err
		}
		value.Text = strings.TrimSpace(text)
	case *DividerOptions:
		if !slices.Contains([]string{"-", "=", "*", "."}, value.Character) {
			return invalidTemplate(blockID, "El separador admite guiones, doble línea, asteriscos o puntos.")
		}
	case *SpacerOptions:
		if value.Lines < 1 || value.Lines > MaxSpacerLines {
			return invalidTemplate(blockID, "El espacio admite de 1 a 5 líneas en blanco.")
		}
	case *FieldOptions:
		return normalizeField(kind, blockID, value)
	case *KitchenItemsOptions:
		return validateKitchenItems(blockID, value)
	}
	return nil
}

func normalizeField(kind Kind, blockID string, options *FieldOptions) error {
	if len(options.Items) == 0 || len(options.Items) > MaxFieldItems {
		return invalidTemplate(blockID, "El campo de datos lleva de 1 a 4 datos.")
	}
	if options.Layout != FieldInline && options.Layout != FieldPair {
		return invalidTemplate(blockID, "La disposición del campo de datos debe ser en línea o etiqueta y valor.")
	}
	if options.Layout == FieldPair && len(options.Items) != 1 {
		return invalidTemplate(blockID, "La disposición etiqueta y valor admite un solo dato.")
	}
	for index := range options.Items {
		item := &options.Items[index]
		if !fieldKeyAllowed(kind, item.Key) {
			return invalidTemplate(blockID, "El dato «"+CleanText(item.Key)+"» no está disponible para "+strings.ToLower(kind.Name())+".")
		}
		label, err := cleanFreeText(blockID, item.Label, MaxLabelChars, 1)
		if err != nil {
			return err
		}
		item.Label = strings.TrimSpace(label)
	}
	return nil
}

func validateKitchenItems(blockID string, options *KitchenItemsOptions) error {
	tristate := []string{Inherit, OptionYes, OptionNo}
	if !slices.Contains(tristate, options.ShowNotes) {
		return invalidTemplate(blockID, "Las observaciones se imprimen según Cocina, siempre o nunca.")
	}
	// Los importes solo se imprimen según Cocina o se ocultan: la comanda guarda los precios solo si
	// Cocina los muestra («Incluir importes comerciales en comandas»), de modo que «siempre» no
	// podría cumplirse en papel.
	if !slices.Contains([]string{Inherit, OptionNo}, options.ShowPrices) {
		return invalidTemplate(blockID, "Los importes se imprimen según la configuración de Cocina o se ocultan. "+
			"Para imprimirlos, activa «Incluir importes comerciales en comandas» en Cocina.")
	}
	if !slices.Contains([]string{Inherit, QuantityPrefix, QuantityMulti}, options.QuantityStyle) {
		return invalidTemplate(blockID, "El estilo de cantidad debe ser el de Cocina, «2 x» o «× 2».")
	}
	if !slices.Contains([]string{SpacingInherit, "0", "1", "2", "3"}, options.LineSpacing) {
		return invalidTemplate(blockID, "El espacio entre partidas debe ser el de Cocina o de 0 a 3 líneas.")
	}
	return nil
}

// cleanFreeText sanea un texto libre de la plantilla: unifica los saltos de línea, convierte los
// tabuladores en espacios, elimina controles e invisibles (nunca llegan comandos a la impresora) y
// aplica los límites de caracteres y de líneas.
func cleanFreeText(blockID, value string, maxChars, maxLines int) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	cleaned := strings.Map(func(character rune) rune {
		if character == '\n' && maxLines > 1 {
			return character
		}
		if character == '\n' || character == '\t' {
			return ' '
		}
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return -1
		}
		return character
	}, value)
	if runeCount(cleaned) > maxChars {
		return "", invalidTemplate(blockID, "El texto admite como máximo "+strconv.Itoa(maxChars)+" caracteres.")
	}
	if strings.Count(cleaned, "\n")+1 > maxLines {
		return "", invalidTemplate(blockID, "El texto admite como máximo "+strconv.Itoa(maxLines)+" líneas.")
	}
	return cleaned, nil
}

// Encode devuelve el JSON canónico de la plantilla (el que se guarda en la versión).
func (template *Template) Encode() []byte {
	encoded, _ := json.Marshal(template)
	return encoded
}
