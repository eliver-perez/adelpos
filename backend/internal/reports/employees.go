package reports

import (
	"context"
)

// Reportes por empleado (§16, fase B2): solo lo que los registros respaldan. Relaciones confiables:
//   - confirmó: autor de la confirmación del pedido (restaurant_orders kind='confirmation'); las
//     unidades confirmadas son las que nacieron con esa confirmación (no las agregadas después);
//   - canceló partidas: autor del evento item.cancelled;
//   - responsable de la cuenta: el responsable copiado al reconocer la venta (al cerrar la cuenta);
//     la venta se atribuye solo a él, de modo que nunca se cuenta dos veces;
//   - cobró, devolvió y corrigió: quien registró el pago o la devolución (sin turnos en arqueo);
//   - aplicó descuentos o cortesías y autorizó como supervisor: los ajustes vigentes y las
//     autorizaciones puntuales;
//   - movimientos de caja: quien registró la operación de efectivo (sin turnos en arqueo).
// No existe «capturó»: el borrador (y su autor) se borra al confirmar. No se calculan estadísticas
// de productividad que no estén registradas (tiempos, rankings). Cobro y venta son conceptos
// distintos: no se suman.

const employeesDateBasis = "Fecha de cada registro (confirmación, cierre de la cuenta para la venta, cobro, descuento, devolución " +
	"u operación de caja), con la hora de inicio del día."

func init() {
	permissions := []string{PermissionEmployees}
	register(Definition{
		ID: "employees.activity", Group: "employees", Title: "Actividad por empleado",
		Description: "Lo que cada usuario confirmó, vendió como responsable, cobró, descontó, autorizó, devolvió y registró en caja.",
		Permissions: permissions, DateBasis: employeesDateBasis, Filters: []Filter{filterUser("Usuario")},
		Columns: []Column{textColumn("user", "Usuario"),
			described(countColumn("confirmations", "Pedidos confirmados"), "Confirmaciones de pedido registradas por el usuario."),
			described(countColumn("confirmed_units", "Unidades confirmadas"), "Unidades que nacieron con sus confirmaciones."),
			optional(described(countColumn("cancelled_units", "Unidades canceladas"), "Unidades que el usuario canceló o retiró al modificar una partida.")),
			described(countColumn("accounts", "Cuentas cerradas como responsable"), "Cuentas cerradas en el periodo con el usuario como responsable."),
			described(moneyColumn("sales", "Venta como responsable"), "Venta neta de esas cuentas; cada venta se atribuye solo a su responsable."),
			described(countColumn("payments", "Cobros"), "Pagos que registró (sin turnos en arqueo)."),
			described(moneyColumn("collected", "Cobrado neto"), "Lo que cobró menos lo que devolvió o corrigió; no es venta."),
			optional(described(moneyColumn("tips", "Propinas netas"), "Propinas de sus cobros menos las devueltas.")),
			described(countColumn("discounts", "Descuentos aplicados"), "Descuentos vigentes que aplicó."),
			described(moneyColumn("discount_amount", "Importe descontado"), "Importe de esos descuentos."),
			optional(countColumn("courtesies", "Cortesías aplicadas")), optional(moneyColumn("courtesy_amount", "Importe de cortesías")),
			described(countColumn("authorizations", "Autorizaciones como supervisor"), "Descuentos, cortesías, devoluciones, correcciones y anulaciones que autorizó."),
			described(countColumn("refunds", "Devoluciones"), "Devoluciones de dinero que registró."),
			moneyColumn("refunded", "Importe devuelto"),
			described(countColumn("cash_operations", "Movimientos de caja"), "Ingresos, retiros, gastos y correcciones de efectivo que registró.")},
		DefaultSort: Sort{Key: "user"},
		Query:       employeeActivityQuery,
	})
	register(Definition{
		ID: "employees.responsible_sales", Group: "employees", Title: "Ventas por responsable",
		Description: "Productos vendidos en las cuentas de cada responsable (al cerrar la cuenta); cada venta una sola vez.",
		Permissions: permissions, DateBasis: salesDateBasis, Filters: []Filter{filterUser("Responsable"), filterCategory(), filterProduct()},
		Columns: []Column{textColumn("user", "Responsable"), textColumn("product", "Producto"), columnQuantity, columnNet,
			optional(columnTax)},
		DefaultSort: Sort{Key: "user"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := conditions{inPeriod("r.recognized_at")}
			list.when(query, "user_id", "r.responsible_user_id = @user_id::uuid")
			list.when(query, "category_id", "r.category_id = @category_id::uuid")
			list.when(query, "product_id", "r.product_id = @product_id::uuid")
			rows, err := query.collect(ctx, `SELECT coalesce(person.display_name,'Sin responsable') AS "user",
                    CASE WHEN r.product_id IS NULL THEN 'Descuentos y cargos generales de cuenta' ELSE `+latest("r.product_name")+` END AS product,
                    coalesce(sum(r.quantity),0)::bigint AS quantity,coalesce(sum(r.net),0) AS net,coalesce(sum(r.tax),0) AS tax
                FROM app.sales_recognitions r LEFT JOIN app.users person ON person.id=r.responsible_user_id
                WHERE true`+list.sql()+` GROUP BY r.responsible_user_id,person.display_name,r.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"El responsable es el de la cuenta al cerrarse; no es quien capturó, que no se conserva."}}, nil
		},
	})
	register(Definition{
		ID: "employees.confirmations", Group: "employees", Title: "Confirmaciones por empleado",
		Description: "Unidades de cada producto que nacieron con las confirmaciones de pedido de cada usuario.",
		Permissions: permissions, DateBasis: "Fecha de la confirmación del pedido, con la hora de inicio del día.",
		Filters: []Filter{filterUser("Usuario"), filterProduct()},
		Columns: []Column{textColumn("user", "Confirmó"), textColumn("product", "Producto"), countColumn("units", "Unidades confirmadas"),
			optional(described(countColumn("cancelled", "Canceladas después"), "De esas unidades, las que hoy están canceladas.")),
			countColumn("orders", "Confirmaciones")},
		DefaultSort: Sort{Key: "user"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			list := conditions{"o.kind = 'confirmation'", inPeriod("o.created_at")}
			list.when(query, "user_id", "o.author_id = @user_id::uuid")
			list.when(query, "product_id", "item.product_id = @product_id::uuid")
			rows, err := query.collect(ctx, `SELECT person.display_name AS "user",(array_agg(version.quote_line->>'name'))[1] AS product,
                    count(u.id)::bigint AS units,count(u.id) FILTER (WHERE u.state='cancelled')::bigint AS cancelled,
                    count(DISTINCT o.id)::bigint AS orders
                FROM app.restaurant_orders o
                JOIN app.users person ON person.id=o.author_id
                JOIN app.restaurant_items item ON item.original_order_id=o.id
                JOIN app.restaurant_item_versions version ON version.item_id=item.id AND version.version=1
                JOIN app.restaurant_item_units u ON u.item_id=item.id AND u.created_at=o.created_at
                WHERE true`+list.sql()+` GROUP BY o.author_id,person.display_name,item.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Quién capturó el borrador no se conserva (el borrador se borra al confirmar): " +
				"se reporta quién confirmó."}}, nil
		},
	})
}

func employeeActivityQuery(ctx context.Context, query *Query) (*Dataset, error) {
	user := ""
	if query.Has("user_id") {
		user = " WHERE x.user_id = @user_id::uuid"
	}
	rows, err := query.collect(ctx, paymentMovements+`,
        confirmed AS (
            SELECT o.author_id AS user_id,count(DISTINCT o.id)::bigint AS confirmations,count(u.id)::bigint AS confirmed_units
            FROM app.restaurant_orders o
            LEFT JOIN app.restaurant_items item ON item.original_order_id=o.id
            LEFT JOIN app.restaurant_item_units u ON u.item_id=item.id AND u.created_at=o.created_at
            WHERE o.kind='confirmation' AND `+inPeriod("o.created_at")+` GROUP BY 1
        ),
        cancelled AS (
            SELECT e.author_id AS user_id,coalesce(sum(CASE WHEN jsonb_typeof(c.units)='array' THEN jsonb_array_length(c.units) ELSE 0 END),0)::bigint
                AS cancelled_units
            FROM app.restaurant_events e
            CROSS JOIN LATERAL (SELECT CASE e.kind WHEN 'item.cancelled' THEN e.payload->'after'->'unit_ids'
                ELSE e.payload->'after'->'cancelled_unit_ids' END AS units) c
            WHERE e.kind IN ('item.cancelled','item.modified') AND `+inPeriod("e.created_at")+` GROUP BY 1
        ),
        responsible AS (
            SELECT r.responsible_user_id AS user_id,
                count(DISTINCT r.account_id) FILTER (WHERE r.recognition_kind <> 'resettlement')::bigint AS accounts,
                coalesce(sum(r.net),0) AS sales
            FROM app.sales_recognitions r WHERE r.responsible_user_id IS NOT NULL AND `+inPeriod("r.recognized_at")+` GROUP BY 1
        ),
        collected AS (
            SELECT m.actor_id AS user_id,count(*) FILTER (WHERE m.kind='payment')::bigint AS payments,coalesce(sum(m.amount),0) AS collected,
                coalesce(sum(m.tip),0) AS tips,count(*) FILTER (WHERE m.kind='refund')::bigint AS refunds,
                coalesce(-sum(m.amount) FILTER (WHERE m.kind='refund'),0) AS refunded
            FROM movements m JOIN app.cash_shifts s ON s.id=m.shift_id WHERE s.state <> 'closing' GROUP BY 1
        ),
        discounts AS (
            SELECT a.actor_id AS user_id,count(*) FILTER (WHERE NOT a.courtesy)::bigint AS discounts,
                coalesce(sum(a.amount) FILTER (WHERE NOT a.courtesy),0) AS discount_amount,
                count(*) FILTER (WHERE a.courtesy)::bigint AS courtesies,coalesce(sum(a.amount) FILTER (WHERE a.courtesy),0) AS courtesy_amount
            FROM app.account_adjustments a
            WHERE a.kind='discount' AND `+inPeriod("a.created_at")+`
                AND NOT EXISTS(SELECT FROM app.account_adjustment_voids v WHERE v.adjustment_id=a.id) GROUP BY 1
        ),
        authorizations AS (
            SELECT pa.authorized_by AS user_id,count(*)::bigint AS authorizations FROM app.payment_authorizations pa
            WHERE `+inPeriod("pa.created_at")+` GROUP BY 1
        ),
        cash AS (
            SELECT o.actor_id AS user_id,count(*)::bigint AS cash_operations FROM app.cash_operations o JOIN app.cash_shifts s ON s.id=o.shift_id
            WHERE s.state <> 'closing' AND `+inPeriod("o.created_at")+` GROUP BY 1
        ),
        x AS (
            SELECT user_id FROM confirmed UNION SELECT user_id FROM cancelled UNION SELECT user_id FROM responsible
            UNION SELECT user_id FROM collected UNION SELECT user_id FROM discounts UNION SELECT user_id FROM authorizations
            UNION SELECT user_id FROM cash
        )
        SELECT person.display_name AS "user",coalesce(confirmed.confirmations,0) AS confirmations,
            coalesce(confirmed.confirmed_units,0) AS confirmed_units,coalesce(cancelled.cancelled_units,0) AS cancelled_units,
            coalesce(responsible.accounts,0) AS accounts,coalesce(responsible.sales,0) AS sales,coalesce(collected.payments,0) AS payments,
            coalesce(collected.collected,0) AS collected,coalesce(collected.tips,0) AS tips,coalesce(discounts.discounts,0) AS discounts,
            coalesce(discounts.discount_amount,0) AS discount_amount,coalesce(discounts.courtesies,0) AS courtesies,
            coalesce(discounts.courtesy_amount,0) AS courtesy_amount,coalesce(authorizations.authorizations,0) AS authorizations,
            coalesce(collected.refunds,0) AS refunds,coalesce(collected.refunded,0) AS refunded,coalesce(cash.cash_operations,0) AS cash_operations
        FROM x JOIN app.users person ON person.id=x.user_id
        LEFT JOIN confirmed ON confirmed.user_id=x.user_id LEFT JOIN cancelled ON cancelled.user_id=x.user_id
        LEFT JOIN responsible ON responsible.user_id=x.user_id LEFT JOIN collected ON collected.user_id=x.user_id
        LEFT JOIN discounts ON discounts.user_id=x.user_id LEFT JOIN authorizations ON authorizations.user_id=x.user_id
        LEFT JOIN cash ON cash.user_id=x.user_id`+user, query.Args())
	if err != nil {
		return nil, err
	}
	dataset := &Dataset{Rows: rows, Notes: []string{"La venta se atribuye solo al responsable de la cuenta y el cobro a quien lo registró: " +
		"son conceptos distintos y no se suman. No existe «capturó» (no se conserva)."}}
	warning, err := closingShiftsWarning(ctx, query)
	if err != nil {
		return nil, err
	}
	if warning != nil {
		dataset.Warnings = append(dataset.Warnings, *warning)
	}
	return dataset, nil
}
