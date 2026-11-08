package operations

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestStableCentAllocation(t *testing.T) {
	for _, amount := range []string{"0.00", "0.01", "10.00", "40.00", "999999999999.99"} {
		for _, quantity := range []int{1, 2, 3, 7, 1000} {
			sum := decimal.Zero
			for index := 0; index < quantity; index++ {
				unit := value(unitMoney(amount, quantity, index))
				if unit.IsNegative() {
					t.Fatal("Una unidad comercial no puede tener importe negativo")
				}
				sum = sum.Add(unit)
			}
			if !sum.Equal(value(amount)) {
				t.Fatalf("Se perdieron centavos al distribuir %s entre %d: %s", amount, quantity, sum)
			}
		}
	}
	if unitMoney("40.00", 3, 0) != "13.34" || unitMoney("40.00", 3, 1) != "13.33" {
		t.Fatal("Los centavos restantes deben pertenecer siempre a las primeras unidades estables")
	}
}
