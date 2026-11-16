package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// Etapa 9H, fase A (§2.3): opción «Mostrar precio unitario» de la lista de productos.
//   - Desactivada por omisión: las plantillas predeterminadas no cambian y siguen idénticas byte a byte
//     (su JSON canónico y lo que imprimen, aunque los datos traigan el precio unitario).
//   - Las plantillas guardadas sin la opción siguen siendo válidas y su forma canónica no cambia.
//   - Activada, el precio unitario va junto a la cantidad en el mismo renglón si cabe («3 × $150.00
//     Tacos … $450.00») y, si no, en un segundo renglón debajo del nombre; con 42, 48, 30 y 32 columnas.

// defaultTemplateHashes es el SHA-256 de Default(kind).Encode() calculado antes de agregar la opción
// (rama etapa-9h, antes de la fase A).
var defaultTemplateHashes = map[Kind]string{
	KindReceipt:             "1b32c76d199cd8dac0eecab9cc715d748e8ce401e2794d584756f06a648f644d",
	KindPrecheck:            "7f7338364dc609490fc578252ab87e9e0de8462bf461aa775f210e9d5821862e",
	KindKitchenOrder:        "49b58672c3d6eb02d78f943a4f95906a0ee6a78a8251f9d8b6aa7ae99da612db",
	KindKitchenModification: "49b58672c3d6eb02d78f943a4f95906a0ee6a78a8251f9d8b6aa7ae99da612db",
	KindKitchenCancellation: "49b58672c3d6eb02d78f943a4f95906a0ee6a78a8251f9d8b6aa7ae99da612db",
	KindCashIncome:          "969600574607f2b4db11408ea1c9c53f7e2e6d21ba67495c283168b5bb78ce8c",
	KindCashWithdrawal:      "969600574607f2b4db11408ea1c9c53f7e2e6d21ba67495c283168b5bb78ce8c",
	KindCashExpense:         "969600574607f2b4db11408ea1c9c53f7e2e6d21ba67495c283168b5bb78ce8c",
	KindCashPartialCut:      "e3cbe924a9a3ccc2ea4d03cf9c5ea29cf048760315249196a75f8c835aa20b4f",
	KindCashFinalCut:        "312a2a18d48e5153d267f3b85bf009dbbf5e865f4f7fcb2cdc1c8be5558436e4",
}

// defaultRenderHashes es el SHA-256 del texto y de los bytes ESC/POS de la precuenta y el ticket
// predeterminados con los datos de muestra, por columnas, calculado antes de agregar la opción (y antes
// de que los datos de muestra trajeran el precio unitario).
var defaultRenderHashes = map[Kind]map[int][2]string{
	KindReceipt: {
		42: {"3312679674a9a3903899761bfe8c75b24800b1c05236cec46ea731ea42456ee5", "834e6834f237be6b5b75e7fed76e868f145f4e5829b0ca5b2c5ef8a41e80d171"},
		48: {"67531a76451f38d3e605c9950e892e05b2ad6bf6ddf197cc61a43eb3297704a2", "87c079ea4a247ac3d5ca1cd20e0615ac7c6c86589226aa65da99bbc279286a61"},
		30: {"93827ff796d80ec74f70bd0f859363042b936033f208e4f67aad2e6f28e0b9a2", "945093f1997478ba31eb842f8a2f5e0c1e009dddd61ffba3d0a68ee307baf0b0"},
		32: {"4dab3f7e9ed023684b24ecc9d2a7959102f0b2a8adff89bb2fe26ca7fadbbf3b", "327cdb5f886fd16a6bd5cdd34546b354c59b4abd5aa0fe678605c95e4d5b5fa8"},
	},
	KindPrecheck: {
		42: {"d725a04b59693b15b83bc83fc9d338fe32e1b88f4397f50e8f7ed317c201ed8a", "e2298a51e8f831610fe03857902d536ec35f9f993613361893e9c4cec818c6b4"},
		48: {"8a2d6f8247dcf3344a762a3ecd9361d9d7db4fc52f7790c042aad520484d4db5", "e0a9f9a6373c7d6d39d26a5a6d58193d5b83c26740b1628099c086059d4010e3"},
		30: {"0025c4a2798a61dd7bd5c2f1ef2a75c7484fda5cefad3ed2d836a4191b0cf785", "3ac993fe157177eb546b9f26d4a2bfba5f43785f54728904f43074b0bc1306e4"},
		32: {"1298b7c9e752a1e4984655271e477de15f2722d0685f0c750549dd249011b2c3", "f8fbb27ee81113f03d3054d9e5cb871b5ea4844ae62bd1cff8f942f58e17cfcc"},
	},
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// paperFor es el ancho de papel de unas columnas de la lista cerrada.
func paperFor(columns int) int {
	if columns == 30 || columns == 32 {
		return 58
	}
	return 80
}

func TestDefaultTemplatesStayIdenticalByteForByte(t *testing.T) {
	for _, info := range Kinds {
		encoded := Default(info.Kind).Encode()
		if strings.Contains(string(encoded), "show_unit_price") {
			t.Fatalf("La predeterminada de %s no debe llevar la opción nueva: %s", info.Kind, encoded)
		}
		if got := sha256Hex(encoded); got != defaultTemplateHashes[info.Kind] {
			t.Fatalf("La predeterminada de %s cambió: %s", info.Kind, got)
		}
		// Volver a validarla (lo que hace Go al guardar) da exactamente los mismos bytes.
		parsed, err := ParseTemplate(info.Kind, encoded)
		if err != nil {
			t.Fatal(err)
		}
		if string(parsed.Encode()) != string(encoded) {
			t.Fatalf("La forma canónica de la predeterminada de %s cambió al validarla", info.Kind)
		}
	}
	for kind, widths := range defaultRenderHashes {
		data := SampleData(kind, Identity{Name: "Muestra"})
		priced := 0
		for _, line := range data.Commercial.Lines {
			if line.UnitPrice != "" {
				priced++
			}
		}
		if priced != len(data.Commercial.Lines) {
			t.Fatalf("Los datos de muestra de %s deben traer el precio unitario de cada partida", kind)
		}
		for columns, expected := range widths {
			layout, err := Render(kind, Default(kind), data, RenderOptions{PaperWidth: paperFor(columns), Columns: columns})
			if err != nil {
				t.Fatal(err)
			}
			if got := sha256Hex([]byte(layout.Text())); got != expected[0] {
				t.Fatalf("El texto de la predeterminada de %s a %d columnas cambió:\n%s", kind, columns, layout.Text())
			}
			if got := sha256Hex(EncodeESCPos(layout, nil, NoPaperCut)); got != expected[1] {
				t.Fatalf("Los bytes ESC/POS de la predeterminada de %s a %d columnas cambiaron", kind, columns)
			}
			if strings.Contains(layout.Text(), "×") {
				t.Fatalf("La predeterminada de %s no debe mostrar el precio unitario:\n%s", kind, layout.Text())
			}
		}
	}
}

func TestSavedTemplatesWithoutTheOptionStayValid(t *testing.T) {
	// Una plantilla guardada antes de la 9H (sin la clave) conserva su forma canónica y no muestra el
	// precio unitario; activarla y desactivarla vuelve a la misma forma.
	cases := []struct {
		options  string
		expected string
		enabled  bool
	}{
		{`{"show_notes":true}`, `{"show_notes":true}`, false},
		{`{"show_notes":false}`, `{"show_notes":false}`, false},
		{`{}`, `{"show_notes":true}`, false},
		{`{"show_notes":true,"show_unit_price":true}`, `{"show_notes":true,"show_unit_price":true}`, true},
		{`{"show_notes":true,"show_unit_price":false}`, `{"show_notes":true}`, false},
	}
	for _, item := range cases {
		raw := `{"options":{"text_size":"normal"},"blocks":[` +
			`{"id":"products","type":"products","options":` + item.options + `},` +
			`{"id":"fiscal","type":"fiscal_notice"}]}`
		template, err := ParseTemplate(KindPrecheck, []byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", item.options, err)
		}
		block := template.Blocks[0]
		if string(block.Options) != item.expected {
			t.Fatalf("%s: forma canónica %s, se esperaba %s", item.options, block.Options, item.expected)
		}
		options := block.parsed.(*ProductsOptions)
		if options.ShowUnitPrice != item.enabled {
			t.Fatalf("%s: precio unitario %v", item.options, options.ShowUnitPrice)
		}
	}
	// Un valor que no es booleano se rechaza como cualquier opción desconocida o inválida.
	raw := `{"blocks":[{"id":"products","type":"products","options":{"show_unit_price":"sí"}},{"id":"fiscal","type":"fiscal_notice"}]}`
	if _, err := ParseTemplate(KindReceipt, []byte(raw)); err == nil {
		t.Fatal("Un precio unitario que no es booleano debía rechazarse")
	}
}

func TestCatalogOffersUnitPriceOnlyOnProducts(t *testing.T) {
	for _, entry := range BuildCatalog().Kinds {
		for _, block := range entry.Blocks {
			for _, option := range block.Options {
				if option.Key != "show_unit_price" {
					continue
				}
				if block.Type != BlockProducts || entry.Family != FamilyCommercial {
					t.Fatalf("«Mostrar precio unitario» solo existe en la lista de productos: %s/%s", entry.Kind, block.Type)
				}
				if option.Label != "Mostrar precio unitario" || option.Type != "boolean" || option.Default != false {
					t.Fatalf("Opción del catálogo: %+v", option)
				}
			}
		}
	}
	encoded, err := json.Marshal(BuildCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), `"key":"show_unit_price"`) != 2 {
		t.Fatal("La opción debía ofrecerse en la precuenta y en el ticket de pago")
	}
}

// unitPriceTemplate es una precuenta mínima con la lista de productos y la opción indicada.
func unitPriceTemplate(t *testing.T, enabled bool, size string) *Template {
	t.Helper()
	options := `{"show_notes":true,"show_unit_price":false}`
	if enabled {
		options = `{"show_notes":true,"show_unit_price":true}`
	}
	raw := `{"blocks":[{"id":"products","type":"products","size":"` + size + `","options":` + options + `},` +
		`{"id":"fiscal","type":"fiscal_notice","visible":true}]}`
	template, err := ParseTemplate(KindPrecheck, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return template
}

// productLines son los renglones que imprime la lista de productos (sin el aviso fiscal).
func productLines(t *testing.T, template *Template, data *CommercialData, columns int) []string {
	t.Helper()
	layout, err := Render(KindPrecheck, template, Data{Commercial: data}, RenderOptions{PaperWidth: paperFor(columns), Columns: columns})
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, line := range layout.Lines {
		if line.BlockID == "products" {
			lines = append(lines, line.Text)
		}
	}
	return lines
}

func unitPriceData() *CommercialData {
	return &CommercialData{
		Kind: KindPrecheck, Total: "0.00", Balance: "0.00", Paid: "0.00", Tips: "0.00",
		Lines: []CommercialLine{
			{Quantity: 3, Name: "Tacos", Amount: "450.00", UnitPrice: "150.00"},
			{Quantity: 2, Name: "Hamburguesa doble con queso", Notes: "Sin cebolla", Amount: "371.00", UnitPrice: "185.50"},
			// Un documento anterior a la 9H no guardó el precio unitario: se imprime como siempre.
			{Quantity: 1, Name: "Agua", Amount: "20.00"},
		},
	}
}

func TestUnitPriceWithAndWithoutRoomAtEveryWidth(t *testing.T) {
	// Con los anchos de 80 mm (42 y 48) y 58 mm (30 y 32): los tacos caben en un renglón en los cuatro;
	// la hamburguesa solo a 48, y en los demás su renglón es el de siempre (cortado como antes a 30 y 32)
	// con el precio unitario debajo, antes de la observación; el agua no trae precio unitario.
	cases := map[int][]string{
		42: {
			"3 × $150.00 Tacos" + strings.Repeat(" ", 18) + "$450.00",
			"2 x Hamburguesa doble con queso" + strings.Repeat(" ", 4) + "$371.00",
			"   2 × $185.50",
			"   Nota: Sin cebolla",
			"1 x Agua" + strings.Repeat(" ", 28) + "$20.00",
		},
		48: {
			"3 × $150.00 Tacos" + strings.Repeat(" ", 24) + "$450.00",
			"2 × $185.50 Hamburguesa doble con queso" + strings.Repeat(" ", 2) + "$371.00",
			"   Nota: Sin cebolla",
			"1 x Agua" + strings.Repeat(" ", 34) + "$20.00",
		},
		30: {
			"3 × $150.00 Tacos" + strings.Repeat(" ", 6) + "$450.00",
			"2 x Hamburguesa doble ",
			"con queso" + strings.Repeat(" ", 14) + "$371.00",
			"   2 × $185.50",
			"   Nota: Sin cebolla",
			"1 x Agua" + strings.Repeat(" ", 16) + "$20.00",
		},
		32: {
			"3 × $150.00 Tacos" + strings.Repeat(" ", 8) + "$450.00",
			"2 x Hamburguesa doble co",
			"n queso" + strings.Repeat(" ", 18) + "$371.00",
			"   2 × $185.50",
			"   Nota: Sin cebolla",
			"1 x Agua" + strings.Repeat(" ", 18) + "$20.00",
		},
	}
	for columns, expected := range cases {
		got := productLines(t, unitPriceTemplate(t, true, SizeNormal), unitPriceData(), columns)
		if strings.Join(got, "\n") != strings.Join(expected, "\n") {
			t.Fatalf("A %d columnas:\n%s\nse esperaba:\n%s", columns, strings.Join(got, "\n"), strings.Join(expected, "\n"))
		}
		for _, line := range got {
			if len([]rune(line)) > columns {
				t.Fatalf("A %d columnas un renglón excede el ancho: %q", columns, line)
			}
		}
		// Desactivada, la lista es la de siempre aunque los datos traigan el precio unitario.
		withoutPrices := unitPriceData()
		for index := range withoutPrices.Lines {
			withoutPrices.Lines[index].UnitPrice = ""
		}
		disabled := productLines(t, unitPriceTemplate(t, false, SizeNormal), unitPriceData(), columns)
		plain := productLines(t, unitPriceTemplate(t, false, SizeNormal), withoutPrices, columns)
		if strings.Join(disabled, "\n") != strings.Join(plain, "\n") || strings.Contains(strings.Join(disabled, "\n"), "×") {
			t.Fatalf("A %d columnas, desactivada no debía mostrar el precio unitario:\n%s", columns, strings.Join(disabled, "\n"))
		}
	}
}

func TestUnitPriceFollowsBlockWidthAndUppercase(t *testing.T) {
	// Con doble ancho el bloque tiene la mitad de columnas (21 a 80 mm): «3 × $150.00 Tacos» ya no cabe
	// junto al importe y va debajo, como un nombre largo.
	got := productLines(t, unitPriceTemplate(t, true, SizeDoubleWidth), &CommercialData{
		Kind: KindPrecheck, Lines: []CommercialLine{{Quantity: 3, Name: "Tacos", Amount: "450.00", UnitPrice: "150.00"}},
	}, 42)
	expected := []string{"3 x Tacos     $450.00", "   3 × $150.00"}
	if strings.Join(got, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("Doble ancho:\n%s", strings.Join(got, "\n"))
	}
}
