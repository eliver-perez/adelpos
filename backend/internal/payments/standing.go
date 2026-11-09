package payments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AccountStandingSQL devuelve la subconsulta con la situación financiera vigente de una cuenta, para
// usarla como `CROSS JOIN LATERAL (...) f` (o como `FROM (...) f` con un parámetro). `account` es la
// expresión SQL que identifica la cuenta, por ejemplo "a.id" o "$1"; nunca un dato del usuario.
//
// Es la única fórmula del total y el saldo resumidos (AccountBrief y los estados de mesa de la etapa
// 7.1 la comparten): total = unidades activas − descuentos vigentes + cargos vigentes; pagado neto =
// pagos − devoluciones; saldo = total − pagado neto. Todo en numeric exacto de PostgreSQL.
//
// Columnas: units, items, discounts, charges, paid, tips, total, balance, precheck_at (última
// precuenta impresa) y adjusted_at (último ajuste registrado o cancelado).
func AccountStandingSQL(account string) string {
	return fmt.Sprintf(`SELECT s.units,s.items,s.discounts,s.charges,s.paid,s.tips,
        (s.items-s.discounts+s.charges)::numeric(18,2) AS total,
        (s.items-s.discounts+s.charges-s.paid)::numeric(18,2) AS balance,
        s.precheck_at,s.adjusted_at
        FROM (SELECT
            (SELECT count(*) FROM app.restaurant_item_units WHERE account_id=%[1]s AND state='active')::integer AS units,
            coalesce((SELECT sum(total) FROM app.restaurant_item_units WHERE account_id=%[1]s AND state='active'),0)::numeric(18,2) AS items,
            coalesce((SELECT sum(j.amount) FROM app.account_adjustments j WHERE j.account_id=%[1]s AND j.kind='discount' AND %[2]s),0)::numeric(18,2) AS discounts,
            coalesce((SELECT sum(j.amount) FROM app.account_adjustments j WHERE j.account_id=%[1]s AND j.kind='charge' AND %[2]s),0)::numeric(18,2) AS charges,
            (coalesce((SELECT sum(amount) FROM app.payments WHERE account_id=%[1]s),0)
                - coalesce((SELECT sum(amount) FROM app.payment_refunds WHERE account_id=%[1]s),0))::numeric(18,2) AS paid,
            (coalesce((SELECT sum(tip) FROM app.payments WHERE account_id=%[1]s),0)
                - coalesce((SELECT sum(tip) FROM app.payment_refunds WHERE account_id=%[1]s),0))::numeric(18,2) AS tips,
            (SELECT max(d.created_at) FROM app.payment_documents d WHERE d.account_id=%[1]s AND d.kind='precheck') AS precheck_at,
            greatest(
                (SELECT max(j.created_at) FROM app.account_adjustments j WHERE j.account_id=%[1]s),
                (SELECT max(v.created_at) FROM app.account_adjustment_voids v
                    JOIN app.account_adjustments j ON j.id=v.adjustment_id WHERE j.account_id=%[1]s)
            ) AS adjusted_at
        ) s`, account, activeAdjustment)
}

// HasAccountRecords indica si la cuenta tiene cualquier registro financiero: pagos, devoluciones,
// ajustes (aunque estén cancelados), autorizaciones de supervisor, movimientos de caja, documentos
// comerciales (precuentas o tickets) o una entrega. Una cuenta con alguno de ellos no está vacía
// (etapa 7.1, «Liberar mesa») y sigue el flujo normal de anulación o cancelación.
func HasAccountRecords(ctx context.Context, tx pgx.Tx, accountID string) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.payments WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.payment_refunds WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.account_adjustments WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.payment_authorizations WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.cash_movements WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.payment_documents WHERE account_id=$1)
        OR EXISTS(SELECT FROM app.takeaway_deliveries WHERE account_id=$1)`, accountID).Scan(&found)
	return found, err
}
