package documents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, kind Kind, raw string) *Template {
	t.Helper()
	template, err := ParseTemplate(kind, []byte(raw))
	if err != nil {
		t.Fatalf("La plantilla válida se rechazó: %v\n%s", err, raw)
	}
	return template
}

func expectInvalid(t *testing.T, kind Kind, raw, blockID, fragment string) {
	t.Helper()
	_, err := ParseTemplate(kind, []byte(raw))
	var problem *ValidationError
	if !errors.As(err, &problem) {
		t.Fatalf("Se aceptó una plantilla inválida (%s): %s", fragment, raw)
	}
	if problem.BlockID != blockID || !strings.Contains(problem.Message, fragment) {
		t.Fatalf("Rechazo inesperado: bloque %q, mensaje %q (se esperaba %q con %q)", problem.BlockID, problem.Message, blockID, fragment)
	}
}

// Las plantillas predeterminadas son válidas para su tipo y su forma canónica es estable: lo que
// siembra la migración, lo que valida Go y lo que se guarda coinciden.
func TestDefaultTemplatesAreValidAndCanonical(t *testing.T) {
	for _, info := range Kinds {
		encoded := Default(info.Kind).Encode()
		parsed, err := ParseTemplate(info.Kind, encoded)
		if err != nil {
			t.Fatalf("La plantilla predeterminada de %s no es válida: %v", info.Kind, err)
		}
		if string(parsed.Encode()) != string(encoded) {
			t.Fatalf("La forma canónica de %s cambió al validarla:\n%s\n%s", info.Kind, encoded, parsed.Encode())
		}
		if len(encoded) > MaxTemplateBytes {
			t.Fatalf("La plantilla de %s excede el límite", info.Kind)
		}
	}
}

func TestValidationRejectsUnknownIncompatibleAndProtectedBlocks(t *testing.T) {
	fiscal := `{"id":"fiscal","type":"fiscal_notice"}`
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"x","type":"script"},`+fiscal+`]}`, "x", "tipo conocido")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"pagos","type":"payments"}]}`, "pagos", "no está disponible para comanda")
	expectInvalid(t, KindCashFinalCut, `{"blocks":[{"id":"lista","type":"products"},`+fiscal+`]}`, "lista", "no está disponible")
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"esperado","type":"expected_differences"},`+fiscal+`]}`, "esperado", "solo puede incluirse en el corte parcial y en el corte definitivo")
	expectInvalid(t, KindCashIncome, `{"blocks":[{"id":"esperado","type":"expected_differences"},`+fiscal+`]}`, "esperado", "solo puede incluirse")
	expectInvalid(t, KindKitchenCancellation, `{"blocks":[{"id":"esperado","type":"expected_differences"}]}`, "esperado", "solo puede incluirse")
	expectInvalid(t, KindPrecheck, `{"blocks":[{"id":"pago","type":"this_payment"},`+fiscal+`]}`, "pago", "no está disponible para precuenta")
	expectInvalid(t, KindCashPartialCut, `{"blocks":[{"id":"declarado","type":"declared_cash"},`+fiscal+`]}`, "declarado", "no está disponible")
	mustParse(t, KindCashPartialCut, `{"blocks":[{"id":"esperado","type":"expected_differences"},`+fiscal+`]}`)
	mustParse(t, KindCashFinalCut, `{"blocks":[{"id":"esperado","type":"expected_differences"},`+fiscal+`]}`)
}

func TestValidationRequiresVisibleSingleFiscalNotice(t *testing.T) {
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"total","type":"total"}]}`, "", "exactamente un «Aviso fiscal»")
	expectInvalid(t, KindCashExpense, `{"blocks":[{"id":"a","type":"fiscal_notice"},{"id":"b","type":"fiscal_notice"}]}`, "", "exactamente un")
	expectInvalid(t, KindPrecheck, `{"blocks":[{"id":"a","type":"fiscal_notice","visible":false}]}`, "a", "no se puede ocultar")
	moved := mustParse(t, KindPrecheck, `{"blocks":[{"id":"a","type":"fiscal_notice","align":"center","size":"double_height","bold":true},{"id":"b","type":"total"}]}`)
	if moved.Blocks[0].Align != AlignCenter || !moved.Blocks[0].Bold {
		t.Fatal("El aviso fiscal no admitió alineación y énfasis")
	}
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"fiscal_notice"}]}`, "a", "no está disponible")
}

func TestValidationLimitsAndStructure(t *testing.T) {
	expectInvalid(t, KindKitchenOrder, `{"blocks":[]}`, "", "al menos un elemento")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider"}],"html":"<b>"}`, "", "estructura esperada")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","onclick":"x"}]}`, "a", "propiedades desconocidas")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"A b","type":"divider"}]}`, "", "identificador válido")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider"},{"id":"a","type":"spacer"}]}`, "a", "mismo identificador")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","align":"justify"}]}`, "a", "alineación")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","size":"triple"}]}`, "a", "tamaño")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","spacing_after":4}]}`, "a", "espaciado")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","options":{"character":"\u001b"}}]}`, "a", "separador admite")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"divider","options":{"escpos":"\u001bp"}}]}`, "a", "opciones")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"spacer","options":{"lines":9}}]}`, "a", "1 a 5")
	expectInvalid(t, KindReceipt, `{"options":{"text_size":"large"},"blocks":[{"id":"f","type":"fiscal_notice"}]}`, "", "solo admite el tamaño de texto normal")
	mustParse(t, KindKitchenOrder, `{"options":{"text_size":"large"},"blocks":[{"id":"a","type":"divider"}]}`)
	expectInvalid(t, KindKitchenOrder, `{"options":{"text_size":"huge"},"blocks":[{"id":"a","type":"divider"}]}`, "", "tamaño de texto")
	blocks := []string{}
	for index := 0; index < 61; index++ {
		blocks = append(blocks, `{"id":"b`+strings.Repeat("x", index%5)+string(rune('a'+index%26))+`-`+string(rune('a'+index/26))+`","type":"spacer"}`)
	}
	expectInvalid(t, KindKitchenOrder, `{"blocks":[`+strings.Join(blocks, ",")+`]}`, "", "60 elementos")
	huge := `{"blocks":[{"id":"a","type":"fixed_text","options":{"text":"` + strings.Repeat("x", MaxTemplateBytes) + `"}}]}`
	expectInvalid(t, KindKitchenOrder, huge, "", "32 KiB")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"fixed_text","options":{"text":"`+strings.Repeat("x", 301)+`"}}]}`, "a", "300 caracteres")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"fixed_text","options":{"text":"1\n2\n3\n4\n5\n6\n7\n8\n9"}}]}`, "a", "8 líneas")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"fixed_text","options":{"text":"   "}}]}`, "a", "Escribe el texto")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"kitchen_items"},{"id":"b","type":"kitchen_items"}]}`, "b", "como máximo 1")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"kitchen_items","options":{"line_spacing":"7"}}]}`, "a", "espacio entre partidas")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"establishment_name","options":{"when":"never"}}]}`, "a", "cuándo se imprime")
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"a","type":"establishment_name","options":{"when":"always"}},{"id":"f","type":"fiscal_notice"}]}`, "a", "no son válidas")
}

func TestValidationOfFieldBlocks(t *testing.T) {
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"field","options":{"items":[]}}]}`, "a", "de 1 a 4 datos")
	expectInvalid(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"field","options":{"items":[{"key":"cashier","label":"Cajero:"}]}}]}`, "a", "no está disponible para comanda")
	expectInvalid(t, KindCashPartialCut, `{"blocks":[{"id":"a","type":"field","options":{"items":[{"key":"closed_by","label":""}]}},{"id":"f","type":"fiscal_notice"}]}`, "a", "no está disponible")
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"a","type":"field","options":{"items":[{"key":"folio","label":"F"},{"key":"cashier","label":""}],"layout":"pair"}},{"id":"f","type":"fiscal_notice"}]}`, "a", "un solo dato")
	expectInvalid(t, KindReceipt, `{"blocks":[{"id":"a","type":"field","options":{"items":[{"key":"folio","label":"`+strings.Repeat("x", 41)+`"}]}},{"id":"f","type":"fiscal_notice"}]}`, "a", "40 caracteres")
	parsed := mustParse(t, KindReceipt, `{"blocks":[{"id":"a","type":"field","options":{"items":[{"key":"folio","label":" Folio\u001b[31m​\n:"}]}},{"id":"f","type":"fiscal_notice"}]}`)
	options := parsed.Blocks[0].parsed.(*FieldOptions)
	if options.Items[0].Label != "Folio[31m :" || !options.OmitEmpty || options.Layout != FieldInline {
		t.Fatalf("La etiqueta no se saneó o las opciones no tomaron sus valores predeterminados: %+v", options)
	}
}

// Los textos libres se sanean al guardar: nunca llegan controles ni invisibles a la impresora.
func TestFreeTextIsSanitizedOnSave(t *testing.T) {
	parsed := mustParse(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"fixed_text","options":{"text":"Hola\u001b@\u001dV\u0000\tmundo\r\nadiós‎"}}]}`)
	text := parsed.Blocks[0].parsed.(*FixedTextOptions).Text
	if text != "Hola@V mundo\nadiós" {
		t.Fatalf("El texto fijo no se saneó: %q", text)
	}
	var stored map[string]any
	if err := json.Unmarshal(parsed.Encode(), &stored); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(parsed.Encode()), "\x1b\x1d\x00") {
		t.Fatal("La forma canónica conserva controles")
	}
}

// Las propiedades ausentes toman sus valores predeterminados y las opciones de cada bloque se
// completan en la forma canónica.
func TestMissingPropertiesUseDefaults(t *testing.T) {
	parsed := mustParse(t, KindKitchenOrder, `{"blocks":[{"id":"a","type":"kitchen_items","options":{"show_prices":"no"}}]}`)
	block := parsed.Blocks[0]
	if !block.Visible || block.Align != AlignLeft || block.Size != SizeNormal || parsed.Options.TextSize != TextSizeInherit {
		t.Fatalf("Las propiedades ausentes no tomaron sus valores predeterminados: %+v", block)
	}
	options := block.parsed.(*KitchenItemsOptions)
	if options.ShowPrices != OptionNo || options.ShowNotes != Inherit || !options.StationHeadings || !options.ItemReference {
		t.Fatalf("Las opciones no se completaron: %+v", options)
	}
	if !strings.Contains(string(block.Options), `"show_station_headings":true`) {
		t.Fatalf("La forma canónica no lleva todas las opciones: %s", block.Options)
	}
}

// El catálogo solo ofrece a cada tipo sus bloques compatibles.
func TestCatalogOffersOnlyCompatibleBlocks(t *testing.T) {
	catalog := BuildCatalog()
	if len(catalog.Kinds) != 10 || len(catalog.PaperWidths) != 2 || catalog.Limits.Blocks != MaxBlocks {
		t.Fatalf("Catálogo incompleto: %+v", catalog.Limits)
	}
	find := func(kind Kind) CatalogKind {
		for _, entry := range catalog.Kinds {
			if entry.Kind == kind {
				return entry
			}
		}
		t.Fatalf("Falta %s", kind)
		return CatalogKind{}
	}
	has := func(entry CatalogKind, blockType string) bool {
		for _, block := range entry.Blocks {
			if block.Type == blockType {
				return true
			}
		}
		return false
	}
	order := find(KindKitchenOrder)
	if has(order, BlockPayments) || has(order, BlockThisPayment) || has(order, BlockFiscalNotice) || !has(order, BlockKitchenItems) || !has(order, BlockLogo) {
		t.Fatal("La comanda ofrece bloques incompatibles o le faltan los suyos")
	}
	cut := find(KindCashPartialCut)
	if has(cut, BlockProducts) || has(cut, BlockDeclaredCash) || !has(cut, BlockExpectedDifferences) {
		t.Fatal("El corte parcial ofrece bloques incompatibles")
	}
	if has(find(KindCashIncome), BlockExpectedDifferences) || has(find(KindReceipt), BlockExpectedDifferences) {
		t.Fatal("Un tipo distinto de los cortes ofrece el bloque protegido")
	}
	if !has(find(KindReceipt), BlockThisPayment) || has(find(KindPrecheck), BlockThisPayment) {
		t.Fatal("«Este pago» debe existir solo en el ticket")
	}
	for _, entry := range catalog.Kinds {
		if len(entry.FieldKeys) == 0 || len(entry.Shortcuts) != len(entry.FieldKeys) {
			t.Fatalf("%s no tiene datos de campo con sus atajos", entry.Kind)
		}
	}
}
