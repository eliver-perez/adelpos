package legacyimport

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/businessday"
)

// La fecha de reconocimiento (revisión de la Fase C, hallazgo I-1): el pago con el que lo acumulado de los pagos
// mayores que cero alcanza lo que se debía; si nunca lo alcanza, el último mayor que cero; sin ellos, la apertura. Un
// pago repetido, en cero o negativo posterior no mueve la venta de día ni de año, ni en la correspondencia ni en la
// pasada independiente de la validación.
func TestRecognitionIgnoresLaterRepeatedZeroAndNegativePayments(t *testing.T) {
	location, err := time.LoadLocation("America/Monterrey")
	if err != nil {
		t.Fatal(err)
	}
	at := func(text string) time.Time {
		parsed, err := ParseLocalTime(text, location)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	pay := func(id int32, amount Money, when string) Payment {
		return Payment{ID: id, Sale: 1, Amount: NullMoney{Value: amount, Valid: true}, PaidAt: at(when)}
	}
	const due = Money(1000000)
	cases := []struct {
		label    string
		payments []Payment
		expected string
	}{
		{"pago repetido al día siguiente, en otro año", []Payment{pay(1, due, "2024-12-31T20:30:00"),
			pay(2, due, "2025-01-02T10:00:00")}, "2024-12-31T20:30:00"},
		{"pago de $0 posterior", []Payment{pay(1, due, "2024-12-31T20:30:00"), pay(2, 0, "2025-01-02T10:00:00")},
			"2024-12-31T20:30:00"},
		{"pago negativo posterior", []Payment{pay(1, due, "2024-12-31T20:30:00"), pay(2, -10000, "2025-01-03T10:00:00")},
			"2024-12-31T20:30:00"},
		{"pago parcial y después el resto", []Payment{pay(1, 400000, "2024-12-31T20:30:00"),
			pay(2, 600000, "2025-01-02T10:00:00")}, "2025-01-02T10:00:00"},
		{"orden por fecha y no por id", []Payment{pay(9, 600000, "2024-12-31T20:30:00"), pay(3, 400000, "2025-01-02T10:00:00"),
			pay(4, due, "2025-01-05T10:00:00")}, "2025-01-02T10:00:00"},
		{"misma hora: por id", []Payment{pay(8, due, "2024-12-31T20:30:00"), pay(7, due, "2024-12-31T20:30:00")},
			"2024-12-31T20:30:00"},
		{"nunca alcanza: el último mayor que cero", []Payment{pay(1, 400000, "2024-12-31T20:30:00"),
			pay(2, 0, "2025-01-02T10:00:00")}, "2024-12-31T20:30:00"},
		{"sin pagos", nil, "2024-12-30T20:00:00"},
		{"solo pagos en cero o negativos", []Payment{pay(1, 0, "2025-01-02T10:00:00"), pay(2, -5, "2025-01-03T10:00:00")},
			"2024-12-30T20:00:00"},
	}
	config := businessday.Config{Timezone: "America/Monterrey", Location: location, Hour: 5}
	for _, item := range cases {
		model := &Model{Location: location, Info: PackageInfo{Tables: map[string]int64{}}, TableRows: map[string]int64{},
			Sales: []Sale{{ID: 1, Folio: "1", OpenedAt: at("2024-12-30T20:00:00"), Total: due, Subtotal: due,
				Tax: NullMoney{Valid: true}, Paid: NullInt{Value: 1, Valid: true}}},
			Payments: item.payments,
		}
		history := buildHistory(model)
		recognized := history.Sales[0].RecognizedAt
		if !recognized.Equal(at(item.expected)) {
			t.Fatalf("%s: reconocida %s; se esperaba %s", item.label, recognized.In(location), item.expected)
		}
		expected := computeExpected(model, yearOf(config))
		year := config.DayOf(at(item.expected)).Date.Year()
		if expected.SalesByYear[year] != due || len(expected.SalesByYear) != 1 {
			t.Fatalf("%s: la validación la cuenta en %v; se esperaba %d", item.label, expected.SalesByYear, year)
		}
	}
}
