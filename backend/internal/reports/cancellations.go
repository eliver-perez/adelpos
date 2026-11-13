package reports

import "context"

// Etapa 9C (§5.1), ampliada en la etapa 9D (§5): cuentas canceladas, su total por motivo y
// modificaciones de cantidades, con los datos que AD-EL POS guarda:
//   - Desde la 9D, «Cancelar cuenta» escribe un registro por cuenta en account_cancellations (motivo
//     del catálogo con su nombre congelado, descripción, quién canceló, quién autorizó si hizo falta un
//     supervisor, terminal, unidades e importe activos al cancelar) y un evento account.cancelled con
//     todas las unidades retiradas. «Cuentas canceladas» (antes «Cuentas anuladas»; se conserva el
//     identificador sales.voided_accounts para no romper filtros guardados ni exportaciones) parte de
//     ese registro y de las anulaciones anteriores (evento account.void, motivo libre), que no tienen
//     «Autorizó» porque nadie autorizaba. No incluye las cuentas vacías liberadas (account.released) ni
//     descartadas (account.discarded): no son cancelaciones.
//   - Una partida se reduce o se cancela después de confirmarse con «Cancelar» (item.cancelled), al
//     modificarla con menos unidades (item.modified con cancelled_unit_ids) o al cancelar su cuenta
//     (account.cancelled, con las partidas en before.lines). Cada evento guarda las unidades de antes
//     con sus importes, las que se retiraron, el motivo y el autor. Desde la etapa 9F los tres guardan
//     además quién autorizó (after.authorized_by, null si bastó el permiso propio); los eventos
//     item.cancelled e item.modified anteriores no lo tienen y su «Autorizó» queda vacía.
// Una cuenta cancelada nunca es venta: no se cierra, de modo que no tiene reconocimientos en
// sales_recognitions. Los reportes usan el permiso de «Productos cancelados» (reports.sales.read) y no
// toman locks: solo leen.

const cancelledDateBasis = "Fecha de la cancelación de la cuenta, con la hora de inicio del día."

const changesDateBasis = "Fecha de la cancelación o modificación de la partida, con la hora de inicio del día."

// legacyCancelReason agrupa en el total por motivo las cancelaciones anteriores, de motivo libre.
const legacyCancelReason = "Cancelaciones anteriores con motivo libre"

// changesAuthorizationNote explica de dónde sale «Autorizó» en los reportes de unidades canceladas
// (etapa 9F, §7.2).
const changesAuthorizationNote = "«Autorizó» es la persona que dio su autorización porque quien canceló o redujo no tenía permiso " +
	"para hacerlo; queda vacía si bastó su propio permiso y en los registros anteriores, cuando no se pedía autorización."

// unitsAuthorizedDescription describe la columna «Autorizó» de los reportes de unidades canceladas.
const unitsAuthorizedDescription = "Supervisor que autorizó la cancelación o la reducción porque quien la hizo no tenía el permiso " +
	"«Cancelar partidas o cantidades confirmadas» (en una cuenta cancelada, quien autorizó cancelarla); vacía si bastó su permiso " +
	"y en los registros anteriores, cuando no se pedía autorización."

// placeSQL es la mesa (salón) o el origen (para llevar) de una cuenta con la mesa actual.
const placeSQL = "CASE WHEN a.mode='dine_in' THEN coalesce(place.name,'Sin mesa') ELSE a.origin END"

const modeSQL = "CASE a.mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar' END"

// filterCancelReason filtra por motivo del catálogo; «Sin motivo del catálogo» (NoReason) elige las
// cancelaciones anteriores con motivo libre.
func filterCancelReason() Filter {
	return Filter{Key: "reason_id", Label: "Motivo", Type: FilterID, Source: "cancel_reasons"}
}

// cancelledAccountsSQL es el conjunto de cuentas canceladas del periodo, una por cuenta:
//   - las de «Cancelar cuenta» (registro account_cancellations, leído por su fecha);
//   - las anuladas antes con motivo libre: cuentas anuladas sin registro cuyo último evento terminal es
//     account.void (las liberadas y las descartadas quedan fuera). Se parte de las cuentas anuladas,
//     que son pocas, y el evento de cada una se lee por el índice de restaurant_events por cuenta. Su
//     importe es el de las unidades que se cancelaron en ella antes de anularla (solo se anulaba una
//     cuenta sin productos activos).
const cancelledAccountsSQL = `WITH cancelled AS (
        SELECT c.created_at AS cancelled_at,c.account_id,c.reason_id,c.reason_name AS reason,
            coalesce(c.description,'') AS description,c.cancelled_by AS author_id,c.authorized_by,c.device_id,
            c.cancelled_units::bigint AS cancelled_units,c.cancelled_amount,'Cancelación' AS kind
        FROM app.account_cancellations c WHERE ` + "c.created_at >= @from AND c.created_at < @until" + `
        UNION ALL
        SELECT e.created_at,a.id,NULL::uuid,coalesce(e.payload->'after'->>'reason',''),'',e.author_id,NULL::uuid,e.device_id,
            coalesce(units.quantity,0)::bigint,coalesce(units.amount,0::numeric),'Cancelación anterior con motivo libre'
        FROM app.restaurant_accounts a
        JOIN LATERAL (
            SELECT ev.kind,ev.created_at,ev.payload,ev.author_id,ev.device_id FROM app.restaurant_events ev
            WHERE ev.account_id=a.id AND ev.kind IN ('account.void','account.released','account.discarded','account.cancelled')
            ORDER BY ev.sequence DESC LIMIT 1
        ) e ON true
        LEFT JOIN LATERAL (
            SELECT count(*) AS quantity,sum(u.total) AS amount FROM app.restaurant_item_units u
            WHERE u.account_id=a.id AND u.state='cancelled'
        ) units ON true
        WHERE a.state='voided' AND e.kind='account.void' AND ` + "e.created_at >= @from AND e.created_at < @until" + `
            AND NOT EXISTS(SELECT FROM app.account_cancellations x WHERE x.account_id=a.id)
    )`

// cancelledConditions son los filtros comunes de los dos reportes de cuentas canceladas.
func cancelledConditions(query *Query) conditions {
	list := conditions{}
	if query.Params.Value("reason_id") == NoReason {
		list.add("x.reason_id IS NULL")
	} else {
		list.when(query, "reason_id", "x.reason_id = @reason_id::uuid")
	}
	list.when(query, "mode", "a.mode = @mode")
	list.when(query, "area_id", "place.area_id = @area_id::uuid")
	list.when(query, "user_id", "x.author_id = @user_id::uuid")
	return list
}

// cancelledShare calcula la participación de cada motivo en el importe cancelado.
func cancelledShare(rows []Row, totals Row) {
	total := decimalOf(totals, "cancelled_amount")
	for _, row := range rows {
		row["share"] = percentOf(decimalOf(row, "cancelled_amount"), total)
	}
	totals["share"] = percentOf(total, total)
}

func init() {
	register(Definition{
		ID: "sales.voided_accounts", Group: "sales", Title: "Cuentas canceladas",
		Description: "Cuentas canceladas con su motivo, descripción, importe, quién las canceló y quién lo autorizó. No incluye las " +
			"cuentas vacías liberadas ni descartadas, que no son cancelaciones.",
		Permissions: []string{PermissionSales}, DateBasis: cancelledDateBasis,
		Filters: []Filter{filterCancelReason(), filterMode(), filterArea(), filterUser("Usuario que canceló")},
		Columns: []Column{dateTimeColumn("cancelled_at", "Cancelación"), codeColumn("folio", "Folio"),
			textColumn("reason", "Motivo"), textColumn("description", "Descripción"),
			described(moneyColumn("cancelled_amount", "Importe"),
				"Importe de los productos que tenía la cuenta al cancelarla (en las cancelaciones anteriores con motivo libre, el de "+
					"las unidades que se cancelaron en ella). Nunca es venta."),
			described(countColumn("cancelled_units", "Unidades"), "Unidades que se cancelaron con la cuenta."),
			textColumn("cancelled_by", "Canceló"),
			described(textColumn("authorized_by", "Autorizó"),
				"Supervisor que autorizó la cancelación porque quien canceló no tenía el permiso «Cancelar cuentas»; vacío si bastó "+
					"su permiso y en las cancelaciones anteriores con motivo libre."),
			textColumn("terminal", "Terminal"), textColumn("mode", "Modalidad"), textColumn("place", "Mesa u origen"),
			textColumn("area", "Área"),
			optional(textColumn("kind", "Registro")), optional(textColumn("account", "Cuenta")),
			optional(dateTimeColumn("opened_at", "Apertura")), optional(textColumn("opened_by", "Abrió"))},
		DefaultSort: Sort{Key: "cancelled_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, cancelledAccountsSQL+`
                SELECT x.cancelled_at,a.folio::text AS folio,x.reason,x.description,x.cancelled_amount,x.cancelled_units,
                    author.display_name AS cancelled_by,coalesce(approver.display_name,'') AS authorized_by,
                    coalesce(device.name,'') AS terminal,`+modeSQL+` AS mode,`+placeSQL+` AS place,
                    coalesce(area.name,'') AS area,x.kind,a.name AS account,a.created_at AS opened_at,opener.display_name AS opened_by
                FROM cancelled x
                JOIN app.restaurant_accounts a ON a.id=x.account_id
                JOIN app.users author ON author.id=x.author_id
                LEFT JOIN app.users approver ON approver.id=x.authorized_by
                JOIN app.users opener ON opener.id=a.opened_by
                LEFT JOIN app.devices device ON device.id=x.device_id
                LEFT JOIN app.restaurant_tables place ON place.id=a.table_id
                LEFT JOIN app.restaurant_areas area ON area.id=place.area_id
                WHERE true`+cancelledConditions(query).sql(), query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{
				"Una cuenta cancelada nunca cuenta como venta. El motivo es el nombre que tenía al cancelar: renombrarlo no cambia el historial.",
				"Las cuentas vacías liberadas o descartadas no son cancelaciones y no aparecen aquí. La mesa y el área son las actuales de la cuenta.",
				"Las unidades canceladas también aparecen, una por una, en «Productos cancelados» y en «Modificaciones de cantidades».",
			}}, nil
		},
	})
	register(Definition{
		ID: "sales.cancellations_by_reason", Group: "sales", Title: "Cancelaciones por motivo",
		Description: "Total de cuentas canceladas por motivo: cuántas, cuántas unidades y qué importe se dejó de cobrar.",
		Permissions: []string{PermissionSales}, DateBasis: cancelledDateBasis,
		Filters: []Filter{filterMode(), filterArea(), filterUser("Usuario que canceló")},
		Columns: []Column{textColumn("reason", "Motivo"),
			described(countColumn("accounts", "Cuentas"), "Cuentas canceladas con ese motivo en el periodo."),
			described(countColumn("cancelled_units", "Unidades"), "Unidades que se cancelaron con esas cuentas."),
			described(moneyColumn("cancelled_amount", "Importe"), "Importe de los productos de esas cuentas al cancelarlas. Nunca es venta."),
			described(percentColumn("share", "% del importe"), "Participación en el importe cancelado del periodo.")},
		DefaultSort: Sort{Key: "cancelled_amount", Descending: true}, Totals: cancelledShare,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, cancelledAccountsSQL+`
                SELECT CASE WHEN x.reason_id IS NULL THEN '`+legacyCancelReason+`' ELSE x.reason END AS reason,
                    count(*)::bigint AS accounts,coalesce(sum(x.cancelled_units),0)::bigint AS cancelled_units,
                    coalesce(sum(x.cancelled_amount),0) AS cancelled_amount
                FROM cancelled x
                JOIN app.restaurant_accounts a ON a.id=x.account_id
                LEFT JOIN app.restaurant_tables place ON place.id=a.table_id
                WHERE true`+cancelledConditions(query).sql()+`
                GROUP BY CASE WHEN x.reason_id IS NULL THEN '`+legacyCancelReason+`' ELSE x.reason END`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{
				"El motivo es el nombre que tenía al cancelar: renombrarlo no junta ni separa el historial. El detalle de cada cuenta está " +
					"en «Cuentas canceladas».",
			}}, nil
		},
	})
	register(Definition{
		ID: "products.quantity_changes", Group: "products", Title: "Modificaciones de cantidades",
		Description: "Partidas confirmadas cuya cantidad se redujo o que se cancelaron (también al cancelar su cuenta): cantidad " +
			"anterior y nueva, diferencia en unidades e importe, quién, quién lo autorizó, motivo y fecha.",
		Permissions: []string{PermissionSales}, DateBasis: changesDateBasis,
		Filters: []Filter{filterProduct(), filterMode(), filterUser("Usuario que modificó")},
		Columns: []Column{dateTimeColumn("changed_at", "Fecha"), codeColumn("folio", "Folio"), textColumn("product", "Producto"),
			textColumn("change", "Cambio"),
			described(Column{Key: "previous_quantity", Label: "Cantidad anterior", Type: TypeInteger, Sortable: true},
				"Unidades activas de la partida en la cuenta antes del cambio."),
			described(Column{Key: "new_quantity", Label: "Cantidad nueva", Type: TypeInteger, Sortable: true},
				"Unidades que siguieron en la partida."),
			described(countColumn("difference", "Diferencia (unidades)"), "Cantidad nueva − cantidad anterior."),
			described(moneyColumn("amount_difference", "Diferencia (importe)"),
				"Importe de las unidades retiradas, con signo negativo (el que tenían al confirmarse, con sus promociones)."),
			textColumn("changed_by", "Modificó"),
			described(textColumn("authorized_by", "Autorizó"), unitsAuthorizedDescription),
			textColumn("reason", "Motivo"),
			optional(textColumn("action", "Acción")), optional(textColumn("terminal", "Terminal")),
			optional(textColumn("mode", "Modalidad")), optional(textColumn("place", "Mesa u origen")),
			optional(described(countColumn("released_units", "Ya liberadas a cocina"),
				"Unidades retiradas que ya se habían liberado a cocina."))},
		DefaultSort: Sort{Key: "changed_at"},
		Query:       quantityChanges,
	})
}

// quantityChanges arma un renglón por cancelación o modificación con unidades retiradas. Se parte de
// las unidades canceladas en el periodo (índice restaurant_item_units_cancelled_idx de la 0017) y,
// para cada una, se busca el evento que la retiró por el índice de restaurant_events por cuenta: una
// unidad se cancela una sola vez y siempre en la cuenta donde estaba (dividir, transferir y combinar
// solo mueven unidades activas).
//
// Cantidades: una modificación guarda en su evento las unidades activas de antes (before.units) y
// las retiradas (after.cancelled_unit_ids). Una cancelación guarda solo las retiradas; las que
// siguieron en la partida se leen de la respuesta idempotente de esa misma operación
// (restaurant_operations.response, la cuenta tal como quedó), de modo que la cantidad anterior es la
// que siguió más la retirada. Si esa respuesta no tuviera las partidas, las cantidades quedan vacías
// en lugar de inventarse; la diferencia y el importe salen siempre del evento. La cancelación de una
// cuenta (account.cancelled, etapa 9D) retira todas sus partidas en un solo evento: guarda cada partida
// con sus unidades en before.lines y se reporta un renglón por partida («Partida cancelada»).
//
// «Autorizó» (etapa 9F) es el supervisor de after.authorized_by con su nombre visible actual, como
// «Cuentas canceladas»; vacía si bastó el permiso propio y en los eventos anteriores, que no lo guardan.
func quantityChanges(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{}
	list.when(query, "product_id", "c.product_id = @product_id::text")
	list.when(query, "mode", "a.mode = @mode")
	list.when(query, "user_id", "c.author_id = @user_id::uuid")
	rows, err := query.collect(ctx, `WITH removed AS (
            SELECT DISTINCT e.id FROM app.restaurant_item_units u
            JOIN LATERAL (
                SELECT ev.id FROM app.restaurant_events ev
                WHERE ev.account_id=u.account_id AND ev.kind IN ('item.cancelled','item.modified','account.cancelled')
                    AND (coalesce(ev.payload->'after'->'unit_ids','[]'::jsonb) ? u.id::text
                        OR coalesce(ev.payload->'after'->'cancelled_unit_ids','[]'::jsonb) ? u.id::text)
                ORDER BY ev.sequence DESC LIMIT 1
            ) e ON true
            WHERE u.state='cancelled' AND `+inPeriod("u.cancelled_at")+`
        ), changes AS (
            SELECT e.created_at,e.account_id,e.kind,e.author_id,e.device_id,e.operation_id,
                e.payload->'after'->>'authorized_by' AS authorized_by,
                coalesce(e.payload->'after'->>'reason','') AS reason,
                coalesce(e.payload->'before'->'quote_line'->>'name','') AS product,
                e.payload->'before'->'request'->>'product_id' AS product_id,
                coalesce(e.payload->'before'->'units','[]'::jsonb) AS units_before,
                CASE e.kind WHEN 'item.cancelled' THEN coalesce(e.payload->'after'->'unit_ids','[]'::jsonb)
                    ELSE coalesce(e.payload->'after'->'cancelled_unit_ids','[]'::jsonb) END AS removed_ids,
                coalesce(e.payload->'after'->>'version_id','') AS version_id
            FROM removed r JOIN app.restaurant_events e ON e.id=r.id WHERE e.kind<>'account.cancelled'
            UNION ALL
            SELECT e.created_at,e.account_id,e.kind,e.author_id,e.device_id,e.operation_id,
                e.payload->'after'->>'authorized_by',
                coalesce(e.payload->'after'->>'reason',''),
                coalesce(line.value->'quote_line'->>'name',''),
                line.value->'request'->>'product_id',
                coalesce(line.value->'units','[]'::jsonb),
                coalesce((SELECT jsonb_agg(unit.value->'id') FROM jsonb_array_elements(coalesce(line.value->'units','[]'::jsonb)) unit),
                    '[]'::jsonb),
                coalesce(line.value->>'version_id','')
            FROM removed r JOIN app.restaurant_events e ON e.id=r.id
            CROSS JOIN LATERAL jsonb_array_elements(coalesce(e.payload->'before'->'lines','[]'::jsonb)) line
            WHERE e.kind='account.cancelled'
        ), measured AS (
            SELECT c.*,jsonb_array_length(c.removed_ids) AS removed,
                CASE WHEN c.kind IN ('item.modified','account.cancelled') THEN jsonb_array_length(c.units_before)
                    ELSE (SELECT CASE WHEN jsonb_typeof(o.response->'lines')='array' THEN
                            coalesce((SELECT sum((line.value->>'quantity')::integer) FROM jsonb_array_elements(o.response->'lines') line
                                WHERE line.value->>'version_id'=c.version_id AND line.value->>'state'='active'),0)
                            + jsonb_array_length(c.removed_ids) END
                        FROM app.restaurant_operations o WHERE o.id=c.operation_id) END AS previous_quantity,
                (SELECT coalesce(sum((unit.value->>'total')::numeric),0) FROM jsonb_array_elements(c.units_before) unit
                    WHERE c.removed_ids ? (unit.value->>'id')) AS removed_amount,
                (SELECT count(*) FROM app.kitchen_unit_releases k
                    WHERE k.unit_id IN (SELECT retired.value::uuid FROM jsonb_array_elements_text(c.removed_ids) retired)) AS released_units
            FROM changes c
        )
        SELECT c.created_at AS changed_at,a.folio::text AS folio,c.product,
            CASE WHEN c.previous_quantity IS NULL THEN 'Unidades canceladas'
                WHEN c.previous_quantity = c.removed THEN 'Partida cancelada'
                ELSE 'Cantidad reducida' END AS change,
            c.previous_quantity::bigint AS previous_quantity,(c.previous_quantity - c.removed)::bigint AS new_quantity,
            (-c.removed)::bigint AS difference,-c.removed_amount AS amount_difference,
            author.display_name AS changed_by,coalesce(approver.display_name,'') AS authorized_by,c.reason,
            CASE c.kind WHEN 'item.cancelled' THEN 'Cancelación' WHEN 'account.cancelled' THEN 'Cancelación de la cuenta'
                ELSE 'Modificación de la partida' END AS action,
            coalesce(device.name,'') AS terminal,`+modeSQL+` AS mode,`+placeSQL+` AS place,c.released_units::bigint AS released_units
        FROM measured c
        JOIN app.restaurant_accounts a ON a.id=c.account_id
        JOIN app.users author ON author.id=c.author_id
        LEFT JOIN app.users approver ON approver.id=c.authorized_by::uuid
        LEFT JOIN app.devices device ON device.id=c.device_id
        LEFT JOIN app.restaurant_tables place ON place.id=a.table_id
        WHERE c.removed > 0`+list.sql(), query.Args())
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Notes: []string{
		"Solo partidas ya confirmadas: los borradores no se registran. Las unidades retiradas nunca se reconocen como venta.",
		"Una modificación que además cambió la personalización recalculó las unidades que siguieron; aquí solo cuenta el importe de las retiradas.",
		changesAuthorizationNote,
	}}, nil
}
