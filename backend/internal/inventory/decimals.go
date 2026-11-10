package inventory

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// Precisión de inventario (decisión de la etapa 8, documentada en docs/ETAPA_8_AVANCE.md):
//   - cantidades con 6 decimales en la unidad base del insumo (numeric(20,6));
//   - valores con 6 decimales (numeric(20,6));
//   - costos unitarios con 10 decimales (numeric(24,10));
//   - redondeo mitad alejándose de cero (decimal.Round y decimal.DivRound), igual que el dinero
//     del resto del sistema; los importes se redondean a centavos solo para mostrar e imprimir.
const (
	quantityScale = 6
	valueScale    = 6
	costScale     = 10
)

var (
	zero = decimal.Zero
	// maximumQuantity limita cualquier cantidad capturada o convertida para que los valores sigan
	// cabiendo en numeric(20,6) (menos de 10^14).
	maximumQuantity = decimal.NewFromInt(1000000000)
	// maximumUnitCost limita los costos capturados por unidad.
	maximumUnitCost = decimal.NewFromInt(1000000)
)

var (
	quantityPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,6})?$`)
	costPattern     = regexp.MustCompile(`^(0|[1-9][0-9]{0,6})(\.[0-9]{1,10})?$`)
)

func itoa(value int) string {
	return strconv.Itoa(value)
}

// parseQuantity acepta una cantidad positiva con hasta seis decimales, sin signo, espacios,
// separadores de miles ni notación científica.
func parseQuantity(text, subject string) (decimal.Decimal, error) {
	value, err := parseNonNegativeQuantity(text, subject)
	if err != nil {
		return zero, err
	}
	if !value.IsPositive() {
		return zero, invalid(subject + " debe ser mayor que cero.")
	}
	return value, nil
}

// parseNonNegativeQuantity acepta cero o una cantidad positiva con hasta seis decimales.
func parseNonNegativeQuantity(text, subject string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(text)
	if !quantityPattern.MatchString(trimmed) {
		return zero, invalid(subject + " no es válida: usa un número sin signo con hasta seis decimales.")
	}
	value, err := decimal.NewFromString(trimmed)
	if err != nil {
		return zero, invalid(subject + " no es válida.")
	}
	if value.GreaterThan(maximumQuantity) {
		return zero, invalid(subject + " excede el máximo admitido.")
	}
	return value, nil
}

// parseOptionalQuantity trata nil o una cadena vacía como «sin valor».
func parseOptionalQuantity(text *string, subject string) (*decimal.Decimal, error) {
	if text == nil || strings.TrimSpace(*text) == "" {
		return nil, nil
	}
	value, err := parseNonNegativeQuantity(*text, subject)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// parseCost acepta un costo no negativo con hasta diez decimales.
func parseCost(text, subject string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(text)
	if !costPattern.MatchString(trimmed) {
		return zero, invalid(subject + " no es válido: usa un número sin signo con hasta diez decimales.")
	}
	value, err := decimal.NewFromString(trimmed)
	if err != nil {
		return zero, invalid(subject + " no es válido.")
	}
	if value.GreaterThan(maximumUnitCost) {
		return zero, invalid(subject + " excede el máximo admitido.")
	}
	return value, nil
}

// parseDatabaseDecimal interpreta un numeric leído como texto de PostgreSQL.
func parseDatabaseDecimal(text string) decimal.Decimal {
	return decimal.RequireFromString(text)
}

// parseDatabaseOptional interpreta un numeric opcional leído como texto.
func parseDatabaseOptional(text *string) *decimal.Decimal {
	if text == nil {
		return nil
	}
	value := decimal.RequireFromString(*text)
	return &value
}

// quantityText es la cadena JSON de una cantidad: sin ceros finales («20», «0.25», «-1.5»).
func quantityText(value decimal.Decimal) string {
	return value.Round(quantityScale).String()
}

// optionalQuantityText devuelve nil para una cantidad ausente.
func optionalQuantityText(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	text := quantityText(*value)
	return &text
}

// exactMoneyText es la cadena JSON exacta de un valor o costo: al menos dos decimales y los
// demás que tenga, sin ceros finales sobrantes («500.00», «33.3333333333», «-50.00»).
func exactMoneyText(value decimal.Decimal) string {
	text := value.String()
	separator := strings.IndexByte(text, '.')
	if separator < 0 || len(text)-separator-1 < 2 {
		return value.StringFixed(2)
	}
	return text
}

// displayMoneyText redondea un valor a centavos para mostrarlo.
func displayMoneyText(value decimal.Decimal) string {
	return value.Round(2).StringFixed(2)
}

// displayUnitCostText redondea un costo unitario a cuatro decimales para mostrarlo: los costos por
// gramo o mililitro suelen ser fracciones de centavo.
func displayUnitCostText(value decimal.Decimal) string {
	return exactMoneyText(value.Round(4))
}
