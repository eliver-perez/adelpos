package payments

import (
	"testing"

	"github.com/shopspring/decimal"
)

// ExpectedCash (etapa 9A) es el mismo cálculo que usan Caja, los cortes y el cierre: los reportes lo
// reciben con los componentes que suman por SQL y nunca copian la fórmula.
func TestExpectedCashIsTheShiftExpectation(t *testing.T) {
	value := func(text string) decimal.Decimal {
		return decimal.RequireFromString(text)
	}
	flows := cashFlows{
		floatMXN: value("512.34"), floatUSD: value("20"), paymentsMXN: value("60"), paymentsUSD: value("10"),
		operationsInMXN: value("40"), operationsOutMXN: value("130"), operationsInUSD: value("1.50"), operationsOutUSD: value("5"),
	}
	expected := flows.expectation(nil)
	mxn, usd := ExpectedCash(CashFlowTotals{
		FloatMXN: flows.floatMXN, FloatUSD: flows.floatUSD, PaymentsMXN: flows.paymentsMXN, PaymentsUSD: flows.paymentsUSD,
		OperationsInMXN: flows.operationsInMXN, OperationsOutMXN: flows.operationsOutMXN, OperationsInUSD: flows.operationsInUSD,
		OperationsOutUSD: flows.operationsOutUSD,
	})
	if !mxn.Equal(expected.mxn) || !usd.Equal(expected.usd) || mxn.String() != "482.34" || usd.String() != "26.5" {
		t.Fatalf("ExpectedCash = %s / %s; el turno espera %s / %s", mxn, usd, expected.mxn, expected.usd)
	}
}
