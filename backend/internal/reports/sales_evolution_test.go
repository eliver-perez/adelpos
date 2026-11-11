package reports

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/businessday"
)

// dailySales simula una venta neta distinta cada día comercial del periodo (100 + día del mes) y la
// agrupa por cubeta como bucketTotals (date_trunc de la semana ISO o del mes); devuelve también la
// suma del periodo completo.
func dailySales(period *businessday.Period, grouping string) (map[string]decimal.Decimal, decimal.Decimal) {
	totals := map[string]decimal.Decimal{}
	sum := decimal.Zero
	for day := period.First(); !day.After(period.Last()); day = day.AddDate(0, 0, 1) {
		value := decimal.NewFromInt(int64(100 + day.Day()))
		key := bucketStart(day, grouping).Format("2006-01-02")
		totals[key] = totals[key].Add(value)
		sum = sum.Add(value)
	}
	return totals, sum
}

// La evolución de ventas compara siempre los dos periodos completos: el total «anterior» y su
// variación son la suma del periodo anterior completo aunque tenga más cubetas (febrero contra
// enero, un año de 54 semanas parciales, un rango corto), y las cubetas sobrantes aparecen al final
// con solo su valor anterior. Con 53 contra 52 semanas ISO (2026 contra 2025 y 2027 contra 2026) las
// cubetas por semana coinciden en número y los totales siguen siendo los completos.
func TestSalesEvolutionComparesWholePeriods(t *testing.T) {
	config := businessday.DefaultConfig()
	now := time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC)
	definition := definitionOf(t, "sales.evolution")
	cases := []struct {
		name            string
		grouping        string
		spec            businessday.Spec
		currentBuckets  int
		previousBuckets int
	}{
		{"febrero contra enero por día", "day", businessday.Spec{Kind: businessday.KindMonth, Month: "2026-02"}, 28, 31},
		{"marzo contra febrero por día", "day", businessday.Spec{Kind: businessday.KindMonth, Month: "2026-03"}, 31, 28},
		{"2026 (53 semanas ISO) contra 2025 (52) por semana", "week", businessday.Spec{Kind: businessday.KindYear, Year: "2026"}, 53, 53},
		{"2027 (52 semanas ISO) contra 2026 (53) por semana", "week", businessday.Spec{Kind: businessday.KindYear, Year: "2027"}, 53, 53},
		{"2013 contra 2012 (bisiesto que empieza en domingo) por semana", "week",
			businessday.Spec{Kind: businessday.KindYear, Year: "2013"}, 53, 54},
		{"febrero de 2027 contra enero por semana", "week", businessday.Spec{Kind: businessday.KindMonth, Month: "2027-02"}, 4, 5},
		{"martes a jueves contra sábado a lunes por semana", "week",
			businessday.Spec{Kind: businessday.KindRange, From: "2026-11-10", Until: "2026-11-12"}, 1, 2},
		{"año por mes", "month", businessday.Spec{Kind: businessday.KindYear, Year: "2026"}, 12, 12},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			period, err := businessday.Resolve(scenario.spec, now, config)
			if err != nil {
				t.Fatal(err)
			}
			previous := period.Previous(now, config)
			current, currentSum := dailySales(&period, scenario.grouping)
			before, previousSum := dailySales(&previous, scenario.grouping)
			rows, extra := evolutionRows(scenario.grouping, &period, &previous, current, before)
			if got := len(buckets(&period, scenario.grouping)); got != scenario.currentBuckets {
				t.Fatalf("Cubetas del periodo: %d, se esperaban %d", got, scenario.currentBuckets)
			}
			if got := len(buckets(&previous, scenario.grouping)); got != scenario.previousBuckets {
				t.Fatalf("Cubetas del periodo anterior: %d, se esperaban %d", got, scenario.previousBuckets)
			}
			wantRows := max(scenario.currentBuckets, scenario.previousBuckets)
			if len(rows) != wantRows || extra != max(scenario.previousBuckets-scenario.currentBuckets, 0) {
				t.Fatalf("Renglones: %d (extra %d), se esperaban %d", len(rows), extra, wantRows)
			}
			totals := sumRows(definition.Columns, rows)
			definition.Totals(rows, totals)
			if !decimalOf(totals, "net").Equal(currentSum) || !decimalOf(totals, "previous_net").Equal(previousSum) ||
				!decimalOf(totals, "change").Equal(currentSum.Sub(previousSum)) {
				t.Fatalf("Totales: neta %s (esperada %s), anterior %s (esperada %s), variación %s", totals["net"], currentSum,
					totals["previous_net"], previousSum, totals["change"])
			}
			wantPercent := percentOf(currentSum.Sub(previousSum), previousSum.Abs())
			gotPercent, isDecimal := totals["change_percent"].(decimal.Decimal)
			if wantPercent == nil || !isDecimal || !gotPercent.Equal(wantPercent.(decimal.Decimal)) {
				t.Fatalf("Variación %%: %v, esperada %v", totals["change_percent"], wantPercent)
			}
			for index, row := range rows {
				if index < scenario.currentBuckets {
					if row["bucket_start"] == nil || row["net"] == nil {
						t.Fatalf("Renglón %d del periodo sin fecha o venta: %+v", index, row)
					}
					continue
				}
				// Cubeta sobrante del anterior: solo su valor anterior, con su fecha.
				if row["bucket_start"] != nil || row["net"] != nil || row["change"] != nil || row["change_percent"] != nil ||
					row["previous_start"] == nil || row["previous_net"] == nil {
					t.Fatalf("Renglón sobrante %d: %+v", index, row)
				}
			}
			// Ordenar por la fecha deja las cubetas sobrantes al final (sin fecha del periodo).
			sortRows(rows, definition.Columns, Sort{Key: "bucket_start", Descending: true})
			if extra > 0 && rows[len(rows)-1]["bucket_start"] != nil {
				t.Fatal("Las cubetas sobrantes deben quedar al final")
			}
		})
	}
}
