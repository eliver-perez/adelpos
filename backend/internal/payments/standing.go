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
// Es también el único predicado de vigencia de la precuenta (precheck_current, etapa 9A §33): una
// precuenta es vigente si se imprimió con la revisión de la cuenta y el número de ajustes actuales
// (ambos guardados en payment_documents al imprimirla, bajo el lock de cobros). Cualquier cambio de
// contenido (borrador, confirmación, modificación, cancelación, datos, mesa, combinación,
// transferencia, cierre o reapertura) incrementa la revisión, y cada ajuste o cancelación de ajuste
// incrementa el número de ajustes; un pago no cambia ninguno de los dos. Las precuentas anteriores a
// la 0015 (sin revisión) conservan la regla por marcas de tiempo: impresa después del último cambio
// de la cuenta y del último ajuste. Nadie debe volver a comparar instantes por su cuenta.
//
// Columnas: units, items, discounts, charges, paid, tips, total, balance, precheck_at (última
// precuenta impresa, informativa), adjusted_at (último ajuste registrado o cancelado, informativo) y
// precheck_current.
func AccountStandingSQL(account string) string {
	return fmt.Sprintf(`SELECT s.units,s.items,s.discounts,s.charges,s.paid,s.tips,
        (s.items-s.discounts+s.charges)::numeric(18,2) AS total,
        (s.items-s.discounts+s.charges-s.paid)::numeric(18,2) AS balance,
        s.precheck_at,s.adjusted_at,
        (EXISTS(SELECT FROM app.payment_documents d WHERE d.account_id=%[1]s AND d.kind='precheck'
                AND d.account_revision=s.revision AND d.adjustment_count=s.adjustment_count)
            OR coalesce((SELECT max(d.created_at) FROM app.payment_documents d
                WHERE d.account_id=%[1]s AND d.kind='precheck' AND d.account_revision IS NULL)
                > greatest(s.updated_at,s.adjusted_at),false)) AS precheck_current
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
            ) AS adjusted_at,
            (SELECT r.revision FROM app.restaurant_accounts r WHERE r.id=%[1]s) AS revision,
            (SELECT r.updated_at FROM app.restaurant_accounts r WHERE r.id=%[1]s) AS updated_at,
            %[3]s AS adjustment_count
        ) s`, account, activeAdjustment, adjustmentCountSQL(account))
}

// adjustmentCountSQL es el número de ajustes de una cuenta: descuentos y cargos registrados más sus
// cancelaciones. Las dos tablas son de solo inserción, así que el número solo crece con cada ajuste
// o cancelación. La precuenta lo guarda al imprimirse y AccountStandingSQL lo compara: es la misma
// expresión en los dos lados (etapa 9A, §33).
func adjustmentCountSQL(account string) string {
	return fmt.Sprintf(`((SELECT count(*) FROM app.account_adjustments j WHERE j.account_id=%[1]s)
            + (SELECT count(*) FROM app.account_adjustment_voids v
                JOIN app.account_adjustments j ON j.id=v.adjustment_id WHERE j.account_id=%[1]s))::integer`, account)
}

// adjustmentCount lee el número de ajustes de la cuenta con la misma expresión que la vigencia.
func adjustmentCount(ctx context.Context, tx pgx.Tx, accountID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, "SELECT "+adjustmentCountSQL("$1"), accountID).Scan(&count)
	return count, err
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
