package payments

import (
	"github.com/shopspring/decimal"
)

// Efectivo esperado del turno. Es la única función que lo calcula: la usan los movimientos del
// turno, los cortes, el cierre y la validación de retiros y gastos. Proviene exclusivamente de
// registros reales:
//
//   - Fondo inicial por moneda (el de dólares es opcional).
//   - Cobros, devoluciones y reversos de corrección en efectivo (cash_movements): lo que entra y
//     sale físicamente por moneda, de modo que el cambio entregado nunca suma dos veces. Un
//     movimiento anterior a la 0007, sin columnas por moneda, fue solo en pesos y su efecto es su
//     neto. Tarjeta, transferencia y plataformas no mueven efectivo.
//   - Operaciones de efectivo (cash_operations): ingresos, retiros, gastos y correcciones en su
//     moneda. Las regularizaciones de turnos cerrados no cuentan: corrigen el registro de otro
//     turno sin mover dinero en este, y así no pueden usarse para ocultar una diferencia.
//
// La equivalencia en pesos de los dólares usa el tipo de cambio fijo del turno, redondeado a
// centavos (mitad alejándose de cero), y nunca sustituye las cantidades físicas por moneda.

// cashFlows son los componentes físicos del efectivo de un turno, por moneda.
type cashFlows struct {
	floatMXN         decimal.Decimal
	floatUSD         decimal.Decimal
	paymentsMXN      decimal.Decimal
	paymentsUSD      decimal.Decimal
	operationsInMXN  decimal.Decimal
	operationsOutMXN decimal.Decimal
	operationsInUSD  decimal.Decimal
	operationsOutUSD decimal.Decimal
}

// CashFlowTotals son los mismos componentes que cashFlows, exportados para los consumidores de solo
// lectura (los reportes de la etapa 9A): los suman en su propia transacción con las expresiones
// ExpectedPaymentsMXNSQL y ExpectedPaymentsUSDSQL sobre cash_movements en efectivo y con las
// operaciones de efectivo sin regularizaciones, igual que readCashFlows, y obtienen el esperado con
// ExpectedCash, que es este mismo cálculo (nunca una copia de la fórmula).
type CashFlowTotals struct {
	FloatMXN         decimal.Decimal
	FloatUSD         decimal.Decimal
	PaymentsMXN      decimal.Decimal
	PaymentsUSD      decimal.Decimal
	OperationsInMXN  decimal.Decimal
	OperationsOutMXN decimal.Decimal
	OperationsInUSD  decimal.Decimal
	OperationsOutUSD decimal.Decimal
}

// Expresiones por renglón de app.cash_movements (con is_cash) que suma el esperado: un movimiento
// anterior a la 0007, sin columnas por moneda, fue solo en pesos y su efecto es su neto.
const (
	ExpectedPaymentsMXNSQL = "CASE WHEN mxn_in IS NULL THEN net_amount ELSE mxn_in-mxn_out END"
	ExpectedPaymentsUSDSQL = "coalesce(usd_in,0)-coalesce(usd_out,0)"
)

// ExpectedCash devuelve el efectivo esperado por moneda con el cálculo único del turno.
func ExpectedCash(totals CashFlowTotals) (decimal.Decimal, decimal.Decimal) {
	flows := cashFlows{
		floatMXN:         totals.FloatMXN,
		floatUSD:         totals.FloatUSD,
		paymentsMXN:      totals.PaymentsMXN,
		paymentsUSD:      totals.PaymentsUSD,
		operationsInMXN:  totals.OperationsInMXN,
		operationsOutMXN: totals.OperationsOutMXN,
		operationsInUSD:  totals.OperationsInUSD,
		operationsOutUSD: totals.OperationsOutUSD,
	}
	expected := flows.expectation(nil)
	return expected.mxn, expected.usd
}

// cashExpectation es el efectivo físico esperado por moneda y el tipo de cambio del turno.
type cashExpectation struct {
	mxn  decimal.Decimal
	usd  decimal.Decimal
	rate *decimal.Decimal
}

// expectation suma los componentes de cada moneda por separado, sin convertir.
func (flows cashFlows) expectation(rate *decimal.Decimal) cashExpectation {
	mxn := flows.floatMXN.Add(flows.paymentsMXN).Add(flows.operationsInMXN).Sub(flows.operationsOutMXN)
	usd := flows.floatUSD.Add(flows.paymentsUSD).Add(flows.operationsInUSD).Sub(flows.operationsOutUSD)
	return cashExpectation{mxn: mxn, usd: usd, rate: rate}
}

// amount devuelve el efectivo esperado de una moneda.
func (expected cashExpectation) amount(currency string) decimal.Decimal {
	if currency == currencyUSD {
		return expected.usd
	}
	return expected.mxn
}

// public expresa el esperado con la equivalencia en pesos cuando el turno tiene tipo de cambio.
func (expected cashExpectation) public() CashExpectation {
	result := CashExpectation{MXN: money(expected.mxn), USD: money(expected.usd)}
	if expected.rate != nil {
		equivalent := usdToMXN(expected.usd, *expected.rate)
		result.USDRate = optionalMoney(*expected.rate)
		result.USDEquivalentMXN = optionalMoney(equivalent)
		result.TotalEquivalentMXN = optionalMoney(expected.mxn.Add(equivalent))
	}
	return result
}

// cashDifferences son las diferencias de un arqueo por moneda.
type cashDifferences struct {
	mxn        decimal.Decimal
	usd        decimal.Decimal
	equivalent *decimal.Decimal
}

// differenceResult clasifica una diferencia: sobrante si es positiva, faltante si es negativa.
func differenceResult(difference decimal.Decimal) string {
	if difference.Sign() > 0 {
		return resultOver
	}
	if difference.Sign() < 0 {
		return resultShort
	}
	return resultEven
}

// computeDifferences calcula contado − esperado en cada moneda. La equivalencia en pesos suma la
// diferencia en pesos y la conversión de la diferencia en dólares con el tipo de cambio del turno.
// Las diferencias no bloquean el cierre ni se convierten en ajustes.
func computeDifferences(countedMXN, countedUSD decimal.Decimal, expected cashExpectation) cashDifferences {
	result := cashDifferences{
		mxn: countedMXN.Sub(expected.mxn),
		usd: countedUSD.Sub(expected.usd),
	}
	if expected.rate != nil {
		equivalent := result.mxn.Add(usdToMXN(result.usd, *expected.rate))
		result.equivalent = &equivalent
	}
	return result
}

// public expresa las diferencias con su resultado por moneda.
func (differences cashDifferences) public() CashDifferences {
	result := CashDifferences{
		MXN:       money(differences.mxn),
		USD:       money(differences.usd),
		ResultMXN: differenceResult(differences.mxn),
		ResultUSD: differenceResult(differences.usd),
	}
	if differences.equivalent != nil {
		result.EquivalentMXN = optionalMoney(*differences.equivalent)
	}
	return result
}

// shiftRate devuelve el tipo de cambio fijo del turno, o nil si no se capturó.
func shiftRate(shift *Shift) *decimal.Decimal {
	if shift.USDRate == nil {
		return nil
	}
	rate := decimal.RequireFromString(*shift.USDRate)
	return &rate
}

// readCashFlows obtiene de PostgreSQL los componentes del efectivo del turno. Las sumas se hacen
// en numeric (exacto) y se combinan en Go con decimal.
func (handler *service) readCashFlows(shift *Shift) (cashFlows, error) {
	flows := cashFlows{floatMXN: decimal.RequireFromString(shift.OpeningFloat)}
	if shift.OpeningFloatUSD != nil {
		flows.floatUSD = decimal.RequireFromString(*shift.OpeningFloatUSD)
	}
	var paymentsMXN, paymentsUSD string
	err := handler.tx.QueryRow(handler.ctx, `SELECT
        coalesce(sum(`+ExpectedPaymentsMXNSQL+`),0)::text,
        coalesce(sum(`+ExpectedPaymentsUSDSQL+`),0)::text
        FROM app.cash_movements WHERE shift_id=$1 AND is_cash`, shift.ID).Scan(&paymentsMXN, &paymentsUSD)
	if err != nil {
		return flows, err
	}
	flows.paymentsMXN = decimal.RequireFromString(paymentsMXN)
	flows.paymentsUSD = decimal.RequireFromString(paymentsUSD)
	var inMXN, outMXN, inUSD, outUSD string
	err = handler.tx.QueryRow(handler.ctx, `SELECT
        coalesce(sum(amount) FILTER (WHERE currency='MXN' AND direction='in'),0)::text,
        coalesce(sum(amount) FILTER (WHERE currency='MXN' AND direction='out'),0)::text,
        coalesce(sum(amount) FILTER (WHERE currency='USD' AND direction='in'),0)::text,
        coalesce(sum(amount) FILTER (WHERE currency='USD' AND direction='out'),0)::text
        FROM app.cash_operations WHERE shift_id=$1 AND NOT regularization`, shift.ID).Scan(&inMXN, &outMXN, &inUSD, &outUSD)
	if err != nil {
		return flows, err
	}
	flows.operationsInMXN = decimal.RequireFromString(inMXN)
	flows.operationsOutMXN = decimal.RequireFromString(outMXN)
	flows.operationsInUSD = decimal.RequireFromString(inUSD)
	flows.operationsOutUSD = decimal.RequireFromString(outUSD)
	return flows, nil
}

// expectedCash calcula el efectivo esperado vigente del turno por moneda.
func (handler *service) expectedCash(shift *Shift) (cashExpectation, error) {
	flows, err := handler.readCashFlows(shift)
	if err != nil {
		return cashExpectation{}, err
	}
	return flows.expectation(shiftRate(shift)), nil
}

// shiftRecordCounts cuenta los movimientos de cobro y las operaciones de efectivo del turno. El
// inicio del arqueo guarda estos conteos y la confirmación los compara para detectar cualquier
// registro posterior.
func (handler *service) shiftRecordCounts(shiftID string) (int, int, error) {
	var movements, operations int
	err := handler.tx.QueryRow(handler.ctx, `SELECT
        (SELECT count(*) FROM app.cash_movements WHERE shift_id=$1),
        (SELECT count(*) FROM app.cash_operations WHERE shift_id=$1)`, shiftID).Scan(&movements, &operations)
	return movements, operations, err
}
