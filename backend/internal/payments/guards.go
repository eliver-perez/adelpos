package payments

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Guardas que la operación del restaurante invoca dentro de su misma transacción y bajo el lock
// 7820148305. Implementan las reglas sobre partidas con pago aprobadas para la etapa 6.

// BlockedUnits devuelve, de las unidades indicadas, las que tienen pago neto asignado o un
// descuento manual vigente. Esas unidades no se transfieren, dividen, combinan, cancelan ni editan.
func BlockedUnits(ctx context.Context, tx pgx.Tx, unitIDs []string) (map[string]string, error) {
	blocked := map[string]string{}
	if len(unitIDs) == 0 {
		return blocked, nil
	}
	rows, err := tx.Query(ctx, `SELECT u.id::text,
        (coalesce((SELECT sum(p.amount) FROM app.payment_allocations p WHERE p.unit_id=u.id),0)
            - coalesce((SELECT sum(r.amount) FROM app.payment_refund_allocations r WHERE r.unit_id=u.id),0)) > 0,
        EXISTS(SELECT FROM app.account_adjustment_allocations a JOIN app.account_adjustments j ON j.id=a.adjustment_id
            WHERE a.unit_id=u.id AND `+activeAdjustment+`)
        FROM app.restaurant_item_units u WHERE u.id=ANY($1::uuid[])`, unitIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var paid, discounted bool
		if err := rows.Scan(&id, &paid, &discounted); err != nil {
			return nil, err
		}
		if paid {
			blocked[id] = "payment"
		} else if discounted {
			blocked[id] = "discount"
		}
	}
	return blocked, rows.Err()
}

// RequireUnitsFree rechaza operar unidades con pago o descuento manual, con un mensaje que indica
// qué operación corresponde para liberarlas.
func RequireUnitsFree(ctx context.Context, tx pgx.Tx, unitIDs []string) error {
	blocked, err := BlockedUnits(ctx, tx, unitIDs)
	if err != nil {
		return err
	}
	for _, reason := range blocked {
		if reason == "payment" {
			return conflict("UNIT_HAS_PAYMENT", "Una o más unidades ya tienen un pago asignado. Para transferirlas, cancelarlas o modificarlas, primero registra la devolución autorizada de ese pago.")
		}
	}
	if len(blocked) > 0 {
		return conflict("UNIT_HAS_DISCOUNT", "Una o más unidades tienen un descuento manual vigente. Cancela primero ese descuento desde los ajustes de la cuenta.")
	}
	return nil
}

// RequireAccountCovered comprueba, después de un cambio, que la cuenta no quede con un total menor
// que lo pagado. El error revierte la transacción completa de la operación.
func RequireAccountCovered(ctx context.Context, tx pgx.Tx, accountID string) error {
	brief, err := AccountBrief(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if decimal.RequireFromString(brief.Balance).Sign() < 0 {
		return conflict("ACCOUNT_BELOW_PAYMENTS", "La operación dejaría la cuenta con un total menor que sus pagos registrados. Registra primero la devolución o el ajuste correspondiente.")
	}
	return nil
}

// RequireNoFinancialRecords impide combinar una cuenta de origen o anular una cuenta que tenga
// pagos, devoluciones o ajustes: cada registro financiero permanece en la cuenta donde se hizo.
func RequireNoFinancialRecords(ctx context.Context, tx pgx.Tx, accountID string) error {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.payments WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.account_adjustments j WHERE j.account_id=$1 AND `+activeAdjustment+`)`, accountID).Scan(&found)
	if err != nil {
		return err
	}
	if found {
		return conflict("ACCOUNT_HAS_PAYMENTS", "La cuenta tiene pagos o ajustes registrados. Los pagos permanecen en su cuenta: combina hacia ella desde otra cuenta o registra primero la devolución.")
	}
	return nil
}

// RequireNoNetFinance permite anular o cancelar una cuenta solo si todo lo cobrado ya se devolvió y no
// quedan descuentos ni cargos vigentes. El historial financiero se conserva intacto.
func RequireNoNetFinance(ctx context.Context, tx pgx.Tx, accountID string) error {
	brief, err := AccountBrief(ctx, tx, accountID)
	if err != nil {
		return err
	}
	var adjusted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.account_adjustments j WHERE j.account_id=$1 AND `+activeAdjustment+`)`,
		accountID).Scan(&adjusted); err != nil {
		return err
	}
	if adjusted || decimal.RequireFromString(brief.Paid).Sign() != 0 || decimal.RequireFromString(brief.Tips).Sign() != 0 {
		return conflict("ACCOUNT_HAS_PAYMENTS", "La cuenta conserva pagos, propinas o ajustes vigentes. Registra primero la devolución o cancela los ajustes antes de cancelarla.")
	}
	return nil
}

// Motivos por los que una cuenta no se puede cancelar (etapa 9D, §5.1): la interfaz los explica en
// lugar de solo deshabilitar el botón. La regla es la misma de RequireNoNetFinance; solo se separan
// los dos casos para decir qué hacer primero.
const (
	// CancelBlockedByPayments: la cuenta conserva pagos o propinas netos; primero se devuelven.
	CancelBlockedByPayments = "payments"
	// CancelBlockedByAdjustments: la cuenta tiene un descuento, una cortesía o un cargo vigente.
	CancelBlockedByAdjustments = "adjustments"
)

// CancellationBlocker indica, sin escribir, si los registros financieros de la cuenta impiden
// cancelarla: CancelBlockedByPayments, CancelBlockedByAdjustments o vacío si no hay impedimento.
// Los pagos van primero porque su devolución puede requerir además quitar el ajuste.
func CancellationBlocker(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	brief, err := AccountBrief(ctx, tx, accountID)
	if err != nil {
		return "", err
	}
	return CancellationBlockerWithBrief(ctx, tx, accountID, brief)
}

// CancellationBlockerWithBrief es CancellationBlocker con el resumen financiero ya leído en la misma
// transacción (la lectura de cuentas lo tiene), para no repetir la consulta por cada cuenta.
func CancellationBlockerWithBrief(ctx context.Context, tx pgx.Tx, accountID string, brief *Brief) (string, error) {
	if decimal.RequireFromString(brief.Paid).Sign() != 0 || decimal.RequireFromString(brief.Tips).Sign() != 0 {
		return CancelBlockedByPayments, nil
	}
	var adjusted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.account_adjustments j WHERE j.account_id=$1 AND `+activeAdjustment+`)`,
		accountID).Scan(&adjusted); err != nil {
		return "", err
	}
	if adjusted {
		return CancelBlockedByAdjustments, nil
	}
	return "", nil
}

// RequireCancellable rechaza cancelar una cuenta con pagos o propinas netos (409
// ACCOUNT_HAS_PAYMENTS: primero la devolución) o con ajustes vigentes (409 ACCOUNT_HAS_ADJUSTMENTS:
// primero quitar el descuento o el cargo). Después aplica la guarda de la etapa 6
// (RequireNoNetFinance), que no se debilita.
func RequireCancellable(ctx context.Context, tx pgx.Tx, accountID string) error {
	blocker, err := CancellationBlocker(ctx, tx, accountID)
	if err != nil {
		return err
	}
	switch blocker {
	case CancelBlockedByPayments:
		return conflict("ACCOUNT_HAS_PAYMENTS", "La cuenta tiene pagos o propinas registrados. Registra primero la devolución y después cancela la cuenta.")
	case CancelBlockedByAdjustments:
		return conflict("ACCOUNT_HAS_ADJUSTMENTS", "La cuenta tiene un descuento, una cortesía o un cargo vigente. Quítalo primero desde los ajustes de la cuenta y después cancélala.")
	}
	return RequireNoNetFinance(ctx, tx, accountID)
}

// CurrentTaxRate es la tasa de IVA vigente que se copia a cada versión confirmada.
func CurrentTaxRate(ctx context.Context, tx pgx.Tx) (int, error) {
	var rate int
	err := tx.QueryRow(ctx, "SELECT tax_rate FROM app.payment_settings").Scan(&rate)
	return rate, err
}
