package legacyimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Escalas de los valores del sistema anterior: MONEY guarda cuatro decimales y las cantidades NUMERIC(18,3) tres.
const (
	moneyScale    = 4
	quantityScale = 3
	moneyUnit     = 10000
	quantityUnit  = 1000
)

// Money es un importe del sistema anterior en diezmilésimos de peso. MONEY de SQL Server es justamente un entero de 64
// bits con cuatro decimales, así que cualquier importe guardado cabe exacto y se suma sin redondeos.
type Money int64

// Quantity es una cantidad del sistema anterior en milésimos (NUMERIC(18,3)).
type Quantity int64

// NullMoney es un importe que puede faltar (columna nula).
type NullMoney struct {
	Value Money
	Valid bool
}

// NullQuantity es una cantidad que puede faltar.
type NullQuantity struct {
	Value Quantity
	Valid bool
}

// NullInt es un entero que puede faltar (llaves y banderas TINYINT del anterior).
type NullInt struct {
	Value int32
	Valid bool
}

// errValue es un valor que no tiene la forma esperada; el lector lo convierte en un error de formato con la tabla y la
// columna, sin el valor (que es dato del restaurante).
var errValue = errors.New("valor con formato inesperado")

// parseFixed convierte el texto decimal exacto de un número JSON en un entero escalado. Admite signo, parte entera y
// hasta scale decimales (o más si los sobrantes son ceros: «1.500000» de un NUMERIC ancho); rechaza exponentes, que
// el lector nunca produce para MONEY ni NUMERIC.
func parseFixed(text string, scale int) (int64, error) {
	if text == "" {
		return 0, errValue
	}
	negative := false
	if text[0] == '-' {
		negative = true
		text = text[1:]
	}
	integer, fraction, hasPoint := strings.Cut(text, ".")
	if integer == "" || (hasPoint && fraction == "") {
		return 0, errValue
	}
	for len(fraction) > scale && strings.HasSuffix(fraction, "0") {
		fraction = fraction[:len(fraction)-1]
	}
	if len(fraction) > scale {
		return 0, errValue
	}
	// Sin ceros a la izquierda inútiles, 18 dígitos caben siempre en un int64.
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	if len(integer)+scale > 18 {
		return 0, errValue
	}
	digits := integer + fraction + strings.Repeat("0", scale-len(fraction))
	for _, character := range digits {
		if character < '0' || character > '9' {
			return 0, errValue
		}
	}
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, errValue
	}
	if negative {
		value = -value
	}
	return value, nil
}

// numberText es el texto de un valor numérico del paquete (json.Number; también se acepta texto con un número, por
// si una versión del lector entrega así algún tipo).
func numberText(value any) (string, bool) {
	switch typed := value.(type) {
	case json.Number:
		return string(typed), true
	case string:
		return strings.TrimSpace(typed), true
	}
	return "", false
}

// ParseMoney lee un importe (nil = nulo).
func ParseMoney(value any) (NullMoney, error) {
	if value == nil {
		return NullMoney{}, nil
	}
	text, ok := numberText(value)
	if !ok {
		return NullMoney{}, errValue
	}
	parsed, err := parseFixed(text, moneyScale)
	if err != nil {
		return NullMoney{}, err
	}
	return NullMoney{Value: Money(parsed), Valid: true}, nil
}

// ParseQuantity lee una cantidad (nil = nula).
func ParseQuantity(value any) (NullQuantity, error) {
	if value == nil {
		return NullQuantity{}, nil
	}
	text, ok := numberText(value)
	if !ok {
		return NullQuantity{}, errValue
	}
	parsed, err := parseFixed(text, quantityScale)
	if err != nil {
		return NullQuantity{}, err
	}
	return NullQuantity{Value: Quantity(parsed), Valid: true}, nil
}

// ParseInt lee un entero (llave o bandera; nil = nulo). Un booleano del paquete (bit) cuenta como 0 o 1.
func ParseInt(value any) (NullInt, error) {
	switch typed := value.(type) {
	case nil:
		return NullInt{}, nil
	case bool:
		if typed {
			return NullInt{Value: 1, Valid: true}, nil
		}
		return NullInt{Value: 0, Valid: true}, nil
	}
	text, ok := numberText(value)
	if !ok {
		return NullInt{}, errValue
	}
	parsed, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return NullInt{}, errValue
	}
	return NullInt{Value: int32(parsed), Valid: true}, nil
}

// ParseText lee un texto (nil = vacío y no presente). Un número se acepta como su texto (folios guardados como número
// en alguna versión).
func ParseText(value any) (string, bool, error) {
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case string:
		return typed, true, nil
	case json.Number:
		return string(typed), true, nil
	}
	return "", false, errValue
}

// ParseLocalTime interpreta una fecha DATETIME del anterior (hora local del equipo, sin zona: «AAAA-MM-DDTHH:MM:SS»
// con o sin fracción, o con un espacio en lugar de la «T») en la zona del establecimiento. nil = cero. En un atraso
// del horario de verano, una hora que ocurre dos veces se toma en su primera ocurrencia (regla de time.Date).
func ParseLocalTime(value any, location *time.Location) (time.Time, error) {
	if value == nil {
		return time.Time{}, nil
	}
	text, ok := value.(string)
	if !ok {
		return time.Time{}, errValue
	}
	text = strings.Replace(strings.TrimSpace(text), " ", "T", 1)
	layouts := []string{"2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, text, location)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errValue
}

// String escribe el importe con sus cuatro decimales («-12.3400»).
func (money Money) String() string {
	return formatFixed(int64(money), moneyScale)
}

// String escribe la cantidad con sus tres decimales.
func (quantity Quantity) String() string {
	return formatFixed(int64(quantity), quantityScale)
}

// Display escribe el importe redondeado a centavos (mitad alejándose de cero), como lo muestra AD-EL POS: «1234.57».
func (money Money) Display() string {
	return formatFixed(int64(roundMoney(money, 2))/100, 2)
}

func formatFixed(value int64, scale int) string {
	negative := value < 0
	magnitude := uint64(value)
	if negative {
		magnitude = uint64(-value)
	}
	digits := strconv.FormatUint(magnitude, 10)
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	text := digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	if negative {
		return "-" + text
	}
	return text
}

// roundMoney redondea un importe a decimals decimales (mitad alejándose de cero) y lo deja en diezmilésimos.
func roundMoney(money Money, decimals int) Money {
	step := int64(1)
	for index := decimals; index < moneyScale; index++ {
		step *= 10
	}
	value := int64(money)
	remainder := value % step
	value -= remainder
	if remainder*2 >= step {
		value += step
	} else if remainder*2 <= -step {
		value -= step
	}
	return Money(value)
}

// Numeric convierte el importe al numeric de PostgreSQL sin pasar por texto ni float.
func (money Money) Numeric() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(money)), Exp: -moneyScale, Valid: true}
}

// Numeric convierte la cantidad al numeric de PostgreSQL.
func (quantity Quantity) Numeric() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(quantity)), Exp: -quantityScale, Valid: true}
}

// Numeric de un importe que puede faltar.
func (money NullMoney) Numeric() pgtype.Numeric {
	if !money.Valid {
		return pgtype.Numeric{}
	}
	return money.Value.Numeric()
}

// Numeric de una cantidad que puede faltar.
func (quantity NullQuantity) Numeric() pgtype.Numeric {
	if !quantity.Valid {
		return pgtype.Numeric{}
	}
	return quantity.Value.Numeric()
}

// Or es el importe o, si falta, otro.
func (money NullMoney) Or(fallback Money) Money {
	if money.Valid {
		return money.Value
	}
	return fallback
}

// Pointer es el entero para un parámetro nullable de pgx.
func (value NullInt) Pointer() *int32 {
	if !value.Valid {
		return nil
	}
	number := value.Value
	return &number
}

// Is dice si el entero existe y vale number.
func (value NullInt) Is(number int32) bool {
	return value.Valid && value.Value == number
}

// ParseDisplayMoney lee un importe escrito en pesos («123.45», «-0.5»); lo usan las pruebas y la validación al leer la
// base.
func ParseDisplayMoney(text string) (Money, error) {
	parsed, err := parseFixed(text, moneyScale)
	if err != nil {
		return 0, fmt.Errorf("importe inválido: %w", err)
	}
	return Money(parsed), nil
}

// timePointer es un instante para un parámetro nullable (el cero es nulo).
func timePointer(instant time.Time) *time.Time {
	if instant.IsZero() {
		return nil
	}
	return &instant
}

// textPointer es un texto para un parámetro nullable (vacío y ausente es nulo).
func textPointer(text string, present bool) *string {
	if !present {
		return nil
	}
	return &text
}
