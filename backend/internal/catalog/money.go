package catalog

import (
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"
)

var moneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,2})?$`)
var quantityPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,3})?$`)

func parseMoney(value string) (decimal.Decimal, error) {
	if !moneyPattern.MatchString(value) {
		return decimal.Zero, invalid("Los importes deben ser cadenas decimales no negativas, con máximo dos decimales.")
	}
	result, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, invalid("Importe decimal inválido.")
	}
	return result, nil
}

func parseQuantity(value string) (decimal.Decimal, error) {
	if !quantityPattern.MatchString(value) {
		return decimal.Zero, invalid("Las porciones deben ser no negativas y tener máximo tres decimales.")
	}
	result, err := decimal.NewFromString(value)
	if err != nil || result.GreaterThan(decimal.NewFromInt(1000)) {
		return decimal.Zero, invalid("La cantidad de porciones debe estar entre 0 y 1000.")
	}
	return result, nil
}

func money(value decimal.Decimal) string {
	// Redondeo comercial a centavos, mitad alejándose de cero; los importes de entrada no se redondean.
	return value.Round(2).StringFixed(2)
}

func amount(value string) decimal.Decimal {
	// Solo se usa con configuración ya validada al persistir o con valores decimales producidos por el motor.
	result, err := decimal.NewFromString(value)
	if err != nil {
		panic(fmt.Sprintf("decimal persistido inválido: %T", err))
	}
	return result
}

func positiveDifference(requested, included decimal.Decimal) decimal.Decimal {
	return decimal.Max(decimal.Zero, requested.Sub(included))
}
