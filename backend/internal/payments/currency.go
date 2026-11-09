package payments

import (
	"regexp"

	"github.com/shopspring/decimal"
)

// Pago en dólares en efectivo (función usd_cash).
//
// Reglas de redondeo, aplicadas en cada conversión y nunca en la interfaz:
//   - Dólares a pesos: importe × tipo de cambio, redondeado a centavos mitad alejándose de cero
//     (Decimal.Round(2), igual que round(numeric, 2) en PostgreSQL).
//   - Pesos a dólares (equivalentes del saldo y cambio entregado en dólares): división exacta
//     redondeada a centavos mitad alejándose de cero (Decimal.DivRound(…, 2)).
//   - Lo que falta en dólares cuando el efectivo no alcanza se redondea hacia arriba al centavo:
//     es el mínimo que cubre el faltante, no una conversión que se registre.
//
// El total recibido en pesos (received) es la suma de los pesos recibidos y la conversión de los
// dólares; el cambio se calcula siempre en pesos (change_given) y, si se entrega en dólares, se
// registra además su conversión (change_usd). Las devoluciones salen solo en pesos.

const (
	currencyMXN = "MXN"
	currencyUSD = "USD"
)

// El tipo de cambio cabe en numeric(8,2): hasta seis enteros y dos decimales, sin signos.
var rateRegex = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,2})?$`)

// El porcentaje de alerta admite de 0 a 100 con hasta dos decimales.
var percentRegex = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})(\.[0-9]{1,2})?$`)

// usdToMXN convierte dólares a pesos con el tipo de cambio del turno.
func usdToMXN(usd, rate decimal.Decimal) decimal.Decimal {
	return roundMoney(usd.Mul(rate))
}

// mxnToUSD convierte pesos a dólares con el tipo de cambio del turno.
func mxnToUSD(mxn, rate decimal.Decimal) decimal.Decimal {
	return mxn.DivRound(rate, 2)
}

// usdToCover es la menor cantidad de dólares (en centavos) cuya conversión cubre el importe.
func usdToCover(mxn, rate decimal.Decimal) decimal.Decimal {
	if mxn.Sign() <= 0 {
		return zero
	}
	return mxn.Div(rate).RoundUp(2)
}

// cashPhysical es el efecto físico de un movimiento de efectivo por moneda: lo que entra y sale
// del cajón en pesos y en dólares. rate es nil cuando no intervienen dólares.
type cashPhysical struct {
	mxnIn  decimal.Decimal
	mxnOut decimal.Decimal
	usdIn  decimal.Decimal
	usdOut decimal.Decimal
	rate   *decimal.Decimal
}

// reversed invierte entradas y salidas: lo usa el reverso de una corrección de método.
func (physical cashPhysical) reversed() cashPhysical {
	return cashPhysical{
		mxnIn:  physical.mxnOut,
		mxnOut: physical.mxnIn,
		usdIn:  physical.usdOut,
		usdOut: physical.usdIn,
		rate:   physical.rate,
	}
}

// cashSettlement es la liquidación en efectivo de un cobro con dólares.
type cashSettlement struct {
	receivedMXN    decimal.Decimal
	receivedUSD    decimal.Decimal
	receivedUSDMXN decimal.Decimal
	receivedTotal  decimal.Decimal
	change         decimal.Decimal
	changeUSD      decimal.Decimal
	changeCurrency string
	covered        bool
	missing        decimal.Decimal
}

// settleCashWithUSD calcula, en pesos, lo recibido y el cambio de un cobro que combina pesos y
// dólares. charged es el importe aplicado más la propina. Si el cambio es cero, su moneda se
// registra como pesos: no se entregó nada en dólares.
func settleCashWithUSD(charged, receivedMXN, receivedUSD, rate decimal.Decimal, changeCurrency string) cashSettlement {
	settlement := cashSettlement{
		receivedMXN:    receivedMXN,
		receivedUSD:    receivedUSD,
		receivedUSDMXN: usdToMXN(receivedUSD, rate),
		changeCurrency: changeCurrency,
	}
	settlement.receivedTotal = settlement.receivedMXN.Add(settlement.receivedUSDMXN)
	if settlement.receivedTotal.LessThan(charged) {
		settlement.missing = charged.Sub(settlement.receivedTotal)
		settlement.change = zero
		settlement.changeCurrency = currencyMXN
		return settlement
	}
	settlement.covered = true
	settlement.change = settlement.receivedTotal.Sub(charged)
	if settlement.change.Sign() == 0 {
		settlement.changeCurrency = currencyMXN
	}
	settlement.changeUSD = mxnToUSD(settlement.change, rate)
	return settlement
}

// physical devuelve lo que entra y sale del cajón por moneda con esta liquidación.
func (settlement cashSettlement) physical(rate decimal.Decimal) cashPhysical {
	result := cashPhysical{mxnIn: settlement.receivedMXN, usdIn: settlement.receivedUSD, rate: &rate}
	if settlement.changeCurrency == currencyUSD {
		result.usdOut = settlement.changeUSD
	} else {
		result.mxnOut = settlement.change
	}
	return result
}

// usesDollars indica si la liquidación debe registrarse con columnas por moneda: hubo dólares
// recibidos o el cambio salió en dólares. Si no, el pago es un efectivo en pesos ordinario.
func (settlement cashSettlement) usesDollars() bool {
	return settlement.receivedUSD.Sign() > 0 || settlement.changeCurrency == currencyUSD
}

// parseRate valida el formato del tipo de cambio: positivo, hasta dos decimales.
func parseRate(text string) (decimal.Decimal, error) {
	if !rateRegex.MatchString(text) {
		return zero, &Error{
			Status:  400,
			Code:    "USD_RATE_INVALID",
			Message: "El tipo de cambio debe ser un número de pesos por dólar con hasta dos decimales, por ejemplo 19.50.",
		}
	}
	value := decimal.RequireFromString(text)
	if value.Sign() <= 0 {
		return zero, &Error{Status: 400, Code: "USD_RATE_INVALID", Message: "El tipo de cambio debe ser mayor que cero."}
	}
	return value, nil
}

// rateDifference es la diferencia porcentual absoluta entre dos tipos de cambio, con dos decimales.
func rateDifference(rate, previous decimal.Decimal) decimal.Decimal {
	return rate.Sub(previous).Abs().Mul(hundred).DivRound(previous, 2)
}

func featureDisabled() error {
	return conflict("FEATURE_DISABLED", "El pago en dólares está desactivado en las funciones del establecimiento.")
}
