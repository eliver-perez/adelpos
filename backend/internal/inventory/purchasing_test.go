package inventory

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func amount(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

// Importes de un renglón de compra (fase C): importe = cantidad × precio sin IVA redondeado a
// centavos; subtotal = importe − descuento; IVA = subtotal × tasa redondeado a centavos; total =
// subtotal + IVA. Cada caso está calculado a mano.
func TestPurchaseLineAmounts(t *testing.T) {
	cases := []struct {
		name                                   string
		quantity, price, discount              string
		rate                                   int
		gross, subtotal, tax, total, discount2 string
	}{
		// 20 cajas a $250: 5000.00 − 100.00 = 4900.00; IVA 784.00; total 5684.00.
		{"cajas_con_descuento_16", "20", "250", "100", 16, "5000.00", "4900.00", "784.00", "5684.00", "100.00"},
		// 3 × 33.333333 = 99.999999 → 100.00; IVA 8 % = 8.00.
		{"precio_de_seis_decimales_8", "3", "33.333333", "0", 8, "100.00", "100.00", "8.00", "108.00", "0.00"},
		// 7 × 12.345 = 86.415 → 86.42 (mitad alejándose de cero); exento.
		{"tasa_cero_redondeo", "7", "12.345", "0", 0, "86.42", "86.42", "0.00", "86.42", "0.00"},
		// 10.03 × 16 % = 1.6048 → 1.60.
		{"iva_hacia_abajo", "1", "10.03", "0", 16, "10.03", "10.03", "1.60", "11.63", "0.00"},
		// 10.05 × 16 % = 1.608 → 1.61.
		{"iva_hacia_arriba", "1", "10.05", "0", 16, "10.05", "10.05", "1.61", "11.66", "0.00"},
		// 10.03 × 8 % = 0.8024 → 0.80.
		{"iva_ocho", "1", "10.03", "0", 8, "10.03", "10.03", "0.80", "10.83", "0.00"},
		// 0.5 × 0.01 = 0.005 → 0.01.
		{"medio_centavo", "0.5", "0.01", "0", 0, "0.01", "0.01", "0.00", "0.01", "0.00"},
		// 2.5 kg × 86.40 = 216.00 − 16.00 = 200.00; IVA 32.00.
		{"kilos_con_descuento", "2.5", "86.40", "16", 16, "216.00", "200.00", "32.00", "232.00", "16.00"},
		// Descuento igual al importe: subtotal e IVA en cero.
		{"descuento_total", "2", "5", "10", 16, "10.00", "0.00", "0.00", "0.00", "10.00"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			result, err := purchaseLineAmounts(amount(item.quantity), amount(item.price), amount(item.discount), item.rate)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{money(result.Gross), money(result.Subtotal), money(result.Tax), money(result.Total), money(result.Discount)}
			want := []string{item.gross, item.subtotal, item.tax, item.total, item.discount2}
			for index := range got {
				if got[index] != want[index] {
					t.Fatalf("importes %v, se esperaban %v", got, want)
				}
			}
		})
	}
	// El documento es la suma exacta de sus renglones.
	first, _ := purchaseLineAmounts(amount("20"), amount("250"), amount("100"), 16)
	second, _ := purchaseLineAmounts(amount("1"), amount("10.05"), zero, 16)
	third, _ := purchaseLineAmounts(amount("7"), amount("12.345"), zero, 0)
	document := PurchaseAmounts{}
	for _, line := range []PurchaseAmounts{first, second, third} {
		document.add(line)
	}
	if money(document.Gross) != "5096.47" || money(document.Discount) != "100.00" || money(document.Subtotal) != "4996.47" ||
		money(document.Tax) != "785.61" || money(document.Total) != "5782.08" {
		t.Fatalf("totales del documento: %+v", document)
	}
	for _, rejected := range []struct {
		quantity, price, discount string
		rate                      int
	}{
		{"2", "5", "10.01", 16}, // descuento mayor que el importe
		{"1", "10", "0", 15},    // tasa fuera de la lista cerrada
		{"0", "10", "0", 16},    // cantidad cero
		{"1", "-1", "0", 16},    // precio negativo
		{"1000000000", "1000000", "0", 16},
	} {
		_, err := purchaseLineAmounts(amount(rejected.quantity), amount(rejected.price), amount(rejected.discount), rejected.rate)
		if err == nil {
			t.Fatalf("se aceptó un renglón inválido: %+v", rejected)
		}
	}
}

// Prorrateo acumulado del descuento de un renglón de orden entre sus recepciones.
func TestProratedDiscount(t *testing.T) {
	cases := []struct {
		name                                 string
		discount, ordered, before, now, want string
	}{
		// 12 + 8 de 20 cajas con $50 de descuento: 30.00 y 20.00.
		{"doce_de_veinte", "50", "20", "0", "12", "30.00"},
		{"ocho_restantes", "50", "20", "12", "8", "20.00"},
		// $10 en tres partes iguales: 3.33, 3.34 (6.67 − 3.33) y 3.33 (10.00 − 6.67): suman 10.00.
		{"tercio_uno", "10", "3", "0", "1", "3.33"},
		{"tercio_dos", "10", "3", "1", "1", "3.34"},
		{"tercio_tres", "10", "3", "2", "1", "3.33"},
		// Lo recibido de más no lleva descuento: 18 + 5 de 20 solo completa los 50.00 (50 − 45).
		{"excedente_parcial", "50", "20", "18", "5", "5.00"},
		{"todo_excedente", "50", "20", "20", "5", "0.00"},
		{"sin_descuento", "0", "20", "0", "20", "0.00"},
		// Recibir todo de una vez lleva el descuento completo, sin redondeo.
		{"completa", "33.33", "7", "0", "7", "33.33"},
	}
	for _, item := range cases {
		got := money(proratedDiscount(amount(item.discount), amount(item.ordered), amount(item.before), amount(item.now)))
		if got != item.want {
			t.Fatalf("%s: descuento %s, se esperaba %s", item.name, got, item.want)
		}
	}
}

func TestNormalizeRFC(t *testing.T) {
	for input, want := range map[string]string{
		" abc010203xy9 ": "ABC010203XY9",  // persona moral, en mayúsculas
		"GODE561231GR8":  "GODE561231GR8", // persona física
		"XAXX010101000":  "XAXX010101000", // público en general
		"ÑAND800229AB1":  "ÑAND800229AB1", // Ñ y 29 de febrero de un año bisiesto
		"A&B000101AAA":   "A&B000101AAA",
	} {
		text := input
		got, err := normalizeRFC(&text)
		if err != nil || got == nil || *got != want {
			t.Fatalf("RFC %q: %v %v", input, got, err)
		}
	}
	invalidRFCs := []string{
		"AB010203XY9", "ABC011303XY9", "ABC010230XY9", "ABC810229XY9", "ABC0102031Y", "ABCDE010203XY9", "ABC-010203-XY9",
	}
	for _, input := range invalidRFCs {
		text := input
		if _, err := normalizeRFC(&text); err == nil {
			t.Fatalf("se aceptó el RFC inválido %q", input)
		}
	}
	empty := "  "
	if got, err := normalizeRFC(&empty); got != nil || err != nil {
		t.Fatal("un RFC vacío debe quedar sin RFC")
	}
}

// Devoluciones: nunca más de lo recibido menos lo devuelto; una parte se convierte con la
// proporción del renglón y lo restante usa la cantidad y el valor exactos.
func TestPrepareReturnLines(t *testing.T) {
	line := ReceiptLine{
		PurchaseLine: PurchaseLine{LineNo: 1, Item: ItemReference{ID: "00000000-0000-4000-8000-000000000001", Name: "Refresco"},
			CaptureLabel: "Caja con 24"},
		quantity: amount("10"), base: amount("240"), inventoryValue: amount("1160.00"), unitCost: amount("4.8333333333"),
	}
	receipt := &Receipt{Lines: []ReceiptLine{line}}
	prepared, err := prepareReturnLines(receipt, []supplierReturnLineInput{{ReceiptLineNo: 1, Quantity: "4"}})
	if err != nil {
		t.Fatal(err)
	}
	// 4 cajas = 96 piezas a 1160/240: 96 × 4.8333… = 464.000000.
	value := prepared[0].base.Mul(prepared[0].cost).Round(valueScale)
	if !prepared[0].base.Equal(amount("96")) || value.String() != "464" {
		t.Fatalf("devolución parcial: %s piezas, valor %s", prepared[0].base, value)
	}
	// Con 4 cajas ya devueltas, las 6 restantes son exactamente 144 piezas y 696.00.
	receipt.Lines[0].returned, receipt.Lines[0].returnedBase, receipt.Lines[0].returnedValue = amount("4"), amount("96"), amount("464")
	prepared, err = prepareReturnLines(receipt, []supplierReturnLineInput{{ReceiptLineNo: 1, Quantity: "6"}})
	if err != nil {
		t.Fatal(err)
	}
	value = prepared[0].base.Mul(prepared[0].cost).Round(valueScale)
	if !prepared[0].base.Equal(amount("144")) || value.String() != "696" {
		t.Fatalf("devolución del resto: %s piezas, valor %s", prepared[0].base, value)
	}
	// Una caja más de lo que queda se rechaza con el código estable.
	_, err = prepareReturnLines(receipt, []supplierReturnLineInput{{ReceiptLineNo: 1, Quantity: "7"}})
	var problem *Error
	if !errors.As(err, &problem) || problem.Code != "RETURN_EXCEEDS_RECEIVED" {
		t.Fatalf("se aceptó devolver más de lo recibido: %v", err)
	}
	// Un renglón inexistente o repetido.
	if _, err := prepareReturnLines(receipt, []supplierReturnLineInput{{ReceiptLineNo: 2, Quantity: "1"}}); err == nil {
		t.Fatal("se aceptó un renglón que no existe en la recepción")
	}
	if _, err := prepareReturnLines(receipt, []supplierReturnLineInput{{ReceiptLineNo: 1, Quantity: "1"},
		{ReceiptLineNo: 1, Quantity: "1"}}); err == nil {
		t.Fatal("se aceptó el mismo renglón dos veces")
	}
}
