package reports

import (
	"context"

	"local/ad-el-pos/server/internal/tableunions"
)

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
//     con sus importes, las que se retiraron, el motivo y el autor. account.cancelled guarda además
//     quién autorizó cancelar la cuenta (after.authorized_by, null si bastó el permiso propio). En la
//     etapa 9F item.cancelled e item.modified también lo guardaban, porque cancelar o reducir unidades
//     sin orders.cancel procedía con un supervisor; desde la 9H (decisión del propietario) ya no se
//     pide autorización para eso y los eventos nuevos no lo tienen. «Autorizó» se conserva: vacía en
//     lo nuevo y en lo anterior a la 9F, y con el supervisor en los registros de la 9F.
// Una cuenta cancelada nunca es venta: no se cierra, de modo que no tiene reconocimientos en
// sales_recognitions. Los reportes usan el permiso de «Productos cancelados» (reports.sales.read) y no
// toman locks: solo leen.
//
// Etapa 9H: con un lote importado vigente y el filtro «Datos», las ventas canceladas del sistema anterior
// (legacy_sales con status cancelled, fechadas con su primera cancelación) y sus cambios de cantidad
// (legacy_quantity_changes) se agregan como renglones propios marcados «Sistema anterior». El anterior no
// guardaba motivo ni quién canceló o modificó (solo quién autorizó); su importe cancelado es lo que se
// debía de la venta (total + cargo − descuentos, lo que el anterior contaba como vendido).

const cancelledDateBasis = "Fecha de la cancelación de la cuenta, con la hora de inicio del día."

const changesDateBasis = "Fecha de la cancelación o modificación de la partida, con la hora de inicio del día."

// legacyCancelReason agrupa en el total por motivo las cancelaciones anteriores, de motivo libre.
const legacyCancelReason = "Cancelaciones anteriores con motivo libre"

// changesAuthorizationNote explica de dónde sale «Autorizó» en los reportes de unidades canceladas
// (etapa 9F, §7.2; desde la 9H cancelar o reducir unidades ya no pide autorización).
const changesAuthorizationNote = "Cancelar o reducir unidades no pide autorización: basta el permiso de quien lo hace y queda " +
	"registrado quién fue. «Autorizó» solo aparece en las cuentas canceladas con la autorización de un supervisor y en los " +
	"registros de cuando también se pedía para cancelar o reducir unidades."

// unitsAuthorizedDescription describe la columna «Autorizó» de los reportes de unidades canceladas.
const unitsAuthorizedDescription = "En una cuenta cancelada, el supervisor que autorizó cancelarla porque quien la canceló no tenía " +
	"el permiso «Cancelar cuentas». Cancelar o reducir unidades no pide autorización, así que en esos renglones queda vacía, salvo " +
	"en los registros de cuando sí se pedía."

// placeAt es la mesa (salón) u origen (para llevar) de la cuenta en un instante (la cancelación). Etapa M1 (M1-D14): si la mesa estaba en una
// unión en ese momento, el nombre que tenía la unión («Mesa 5 + 6»), aunque después se haya deshecho.
func placeAt(at string) string {
	return "CASE WHEN a.mode='dine_in' THEN coalesce(" + tableunions.UnionNameAtSQL("a.table_id", at) +
		",place.name,'Sin mesa') ELSE a.origin END"
}

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

// legacyCancelledReason es el motivo de las cancelaciones del sistema anterior, que no lo guardaba.
const legacyCancelledReason = "Sin motivo (sistema anterior)"

// legacyCancelledSQL son las ventas canceladas del historial en el periodo (alias v) con sus unidades y quién
// autorizó la primera cancelación.
const legacyCancelledSQL = `WITH legacy_cancelled AS (
        SELECT v.*,coalesce((SELECT sum(x.quantity) FROM app.legacy_sale_lines x
                WHERE x.batch_id=v.batch_id AND x.legacy_sale_id=v.legacy_id),0)::bigint AS cancelled_units,
            (SELECT c.authorized_name FROM app.legacy_cancellations c WHERE c.batch_id=v.batch_id AND c.legacy_sale_id=v.legacy_id
                ORDER BY c.cancelled_at,c.legacy_id LIMIT 1) AS authorizer
        FROM app.legacy_sales v WHERE v.status='cancelled' AND v.cancelled_at >= @from AND v.cancelled_at < @until
    )`

// legacyCancelledConditions son los filtros de cuentas canceladas sobre el historial: el motivo del catálogo y quién
// canceló no existen en el anterior (lo excluyen; «Sin motivo del catálogo» lo incluye).
func legacyCancelledConditions(query *Query) conditions {
	list := conditions{}
	if value := query.Params.Value("reason_id"); value != "" && value != NoReason {
		list.add("false")
	}
	list.when(query, "mode", "v.mode = @mode")
	list.when(query, "area_id", "v.area_id = @area_id::uuid")
	list.excludeLegacy(query, "user_id")
	return list
}

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
		Filters: withDataSource([]Filter{filterCancelReason(), filterMode(), filterArea(), filterUser("Usuario que canceló")}),
		Columns: withLegacyColumns([]Column{dateTimeColumn("cancelled_at", "Cancelación"), codeColumn("folio", "Folio"),
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
			optional(dateTimeColumn("opened_at", "Apertura")), optional(textColumn("opened_by", "Abrió"))}, false),
		DefaultSort: Sort{Key: "cancelled_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows := []Row{}
			if query.legacy.Native {
				native, err := query.collect(ctx, cancelledAccountsSQL+`
                SELECT x.cancelled_at,a.folio::text AS folio,x.reason,x.description,x.cancelled_amount,x.cancelled_units,
                    author.display_name AS cancelled_by,coalesce(approver.display_name,'') AS authorized_by,
                    coalesce(device.name,'') AS terminal,`+modeSQL+` AS mode,`+placeAt("x.cancelled_at")+` AS place,
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
				query.markNative(native)
				rows = native
			}
			notes := []string{
				"Una cuenta cancelada nunca cuenta como venta. El motivo es el nombre que tenía al cancelar: renombrarlo no cambia el historial.",
				"Las cuentas vacías liberadas o descartadas no son cancelaciones y no aparecen aquí. La mesa y el área son las actuales de la " +
					"cuenta; si estaba en una unión de mesas al cancelarse, la mesa es el nombre de la unión.",
				"Las unidades canceladas también aparecen, una por una, en «Productos cancelados» y en «Modificaciones de cantidades».",
			}
			if query.legacy.History {
				legacy, err := query.collect(ctx, legacyCancelledSQL+`
                    SELECT v.cancelled_at,'`+legacyFolioPrefix+`'||v.folio AS folio,'`+legacyCancelledReason+`' AS reason,
                        '' AS description,v.amount_due AS cancelled_amount,v.cancelled_units,'' AS cancelled_by,
                        coalesce(v.authorizer,'') AS authorized_by,coalesce(v.register_name,'') AS terminal,
                        CASE v.mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar' END AS mode,
                        CASE WHEN v.mode='dine_in' THEN coalesce(v.table_name,'Sin mesa') ELSE 'Para llevar' END AS place,
                        coalesce(v.area_name,'') AS area,'Cancelación del sistema anterior' AS kind,v.account_name AS account,
                        v.opened_at,coalesce(v.cashier_name,'') AS opened_by,'`+LegacySourceLabel+`' AS data_source
                    FROM legacy_cancelled v WHERE true`+legacyCancelledConditions(query).sql()+`
                    ORDER BY v.cancelled_at,v.legacy_id`, query.Args())
				if err != nil {
					return nil, err
				}
				rows = append(rows, legacy...)
				notes = append(notes, "Sistema anterior: no guardaba el motivo ni quién canceló, solo quién autorizó; el importe es lo "+
					"que se debía de la venta (total + cargo − descuentos), lo que el anterior contaba como vendido. La terminal es su caja.")
			}
			return &Dataset{Rows: rows, Notes: notes}, nil
		},
	})
	register(Definition{
		ID: "sales.cancellations_by_reason", Group: "sales", Title: "Cancelaciones por motivo",
		Description: "Total de cuentas canceladas por motivo: cuántas, cuántas unidades y qué importe se dejó de cobrar.",
		Permissions: []string{PermissionSales}, DateBasis: cancelledDateBasis,
		Filters: withDataSource([]Filter{filterMode(), filterArea(), filterUser("Usuario que canceló")}),
		Columns: withLegacyColumns([]Column{textColumn("reason", "Motivo"),
			described(countColumn("accounts", "Cuentas"), "Cuentas canceladas con ese motivo en el periodo."),
			described(countColumn("cancelled_units", "Unidades"), "Unidades que se cancelaron con esas cuentas."),
			described(moneyColumn("cancelled_amount", "Importe"), "Importe de los productos de esas cuentas al cancelarlas. Nunca es venta."),
			described(percentColumn("share", "% del importe"), "Participación en el importe cancelado del periodo.")}, false),
		DefaultSort: Sort{Key: "cancelled_amount", Descending: true}, Totals: cancelledShare,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows := []Row{}
			if query.legacy.Native {
				native, err := query.collect(ctx, cancelledAccountsSQL+`
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
				query.markNative(native)
				rows = native
			}
			notes := []string{
				"El motivo es el nombre que tenía al cancelar: renombrarlo no junta ni separa el historial. El detalle de cada cuenta está " +
					"en «Cuentas canceladas».",
			}
			if query.legacy.History {
				legacy, err := query.collect(ctx, legacyCancelledSQL+`
                    SELECT '`+legacyCancelledReason+`' AS reason,count(*)::bigint AS accounts,
                        coalesce(sum(v.cancelled_units),0)::bigint AS cancelled_units,coalesce(sum(v.amount_due),0) AS cancelled_amount,
                        '`+LegacySourceLabel+`' AS data_source
                    FROM legacy_cancelled v WHERE true`+legacyCancelledConditions(query).sql()+`
                    GROUP BY 1`, query.Args())
				if err != nil {
					return nil, err
				}
				rows = append(rows, legacy...)
				notes = append(notes, "Sistema anterior: no guardaba el motivo; su importe es lo que se debía de cada venta cancelada.")
			}
			return &Dataset{Rows: rows, Notes: notes}, nil
		},
	})
	register(Definition{
		ID: "products.quantity_changes", Group: "products", Title: "Modificaciones de cantidades",
		Description: "Partidas confirmadas cuya cantidad se redujo o que se cancelaron (también al cancelar su cuenta): cantidad " +
			"anterior y nueva, diferencia en unidades e importe, quién, quién lo autorizó, motivo y fecha.",
		Permissions: []string{PermissionSales}, DateBasis: changesDateBasis,
		Filters: withDataSource([]Filter{filterProduct(), filterMode(), filterUser("Usuario que modificó")}),
		Columns: withLegacyColumns([]Column{dateTimeColumn("changed_at", "Fecha"), codeColumn("folio", "Folio"),
			textColumn("product", "Producto"),
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
				"Unidades retiradas que ya se habían liberado a cocina."))}, false),
		DefaultSort: Sort{Key: "changed_at"},
		Query:       quantityChangesWithHistory,
	})
}

// quantityChangesWithHistory son los cambios de AD-EL POS y, con el historial, los del anterior
// (legacyQuantityChanges).
func quantityChangesWithHistory(ctx context.Context, query *Query) (*Dataset, error) {
	dataset := &Dataset{Rows: []Row{}}
	if query.legacy.Native {
		native, err := quantityChanges(ctx, query)
		if err != nil {
			return nil, err
		}
		query.markNative(native.Rows)
		dataset = native
	}
	if !query.legacy.History {
		return dataset, nil
	}
	rows, err := legacyQuantityChanges(ctx, query)
	if err != nil {
		return nil, err
	}
	dataset.Rows = append(dataset.Rows, rows...)
	dataset.Notes = append(dataset.Notes, "Sistema anterior: cada cambio de la cantidad de un renglón (también los aumentos) frente a "+
		"su cantidad original, que es lo que guardaba como «cantidad anterior»; sin el importe de la diferencia, el motivo ni quién "+
		"modificó, que no guardaba (sí quién autorizó). Los registros sin cambio de cantidad (complementos u observaciones) no aparecen.")
	return dataset, nil
}

// legacyQuantityChanges son los cambios de cantidad del historial en el periodo (los que tienen fecha y cambian la
// cantidad), con el producto del renglón y la venta.
func legacyQuantityChanges(ctx context.Context, query *Query) ([]Row, error) {
	list := conditions{inPeriod("q.changed_at"), "q.new_quantity IS DISTINCT FROM q.original_quantity"}
	list.when(query, "product_id", "x.product_id = @product_id::uuid")
	list.when(query, "mode", "v.mode = @mode")
	list.excludeLegacy(query, "user_id")
	return query.collect(ctx, `SELECT q.changed_at,coalesce('`+legacyFolioPrefix+`'||v.folio,'') AS folio,
            coalesce(x.product_name,'') AS product,
            CASE WHEN q.new_quantity < q.original_quantity THEN 'Cantidad reducida' ELSE 'Cantidad aumentada' END AS change,
            q.original_quantity::bigint AS previous_quantity,q.new_quantity::bigint AS new_quantity,
            (q.new_quantity-q.original_quantity)::bigint AS difference,'' AS changed_by,coalesce(q.authorized_name,'') AS authorized_by,
            '' AS reason,'Modificación en el sistema anterior' AS action,coalesce(v.register_name,'') AS terminal,
            CASE v.mode WHEN 'dine_in' THEN 'Salón' WHEN 'takeaway' THEN 'Para llevar' END AS mode,
            CASE WHEN v.mode='dine_in' THEN coalesce(v.table_name,'Sin mesa') WHEN v.mode='takeaway' THEN 'Para llevar' END AS place,
            '`+LegacySourceLabel+`' AS data_source
        FROM app.legacy_quantity_changes q
        LEFT JOIN app.legacy_sale_lines x ON x.batch_id=q.batch_id AND x.legacy_id=q.legacy_line_id
        LEFT JOIN app.legacy_sales v ON v.batch_id=q.batch_id AND v.legacy_id=coalesce(q.legacy_sale_id,x.legacy_sale_id)
        WHERE true`+list.sql()+`
        ORDER BY q.changed_at,q.position`, query.Args())
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
// «Cuentas canceladas»; vacía si bastó el permiso propio, en los eventos anteriores a la 9F y en los
// item.cancelled e item.modified desde la 9H, que no lo guardan.
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
            coalesce(device.name,'') AS terminal,`+modeSQL+` AS mode,`+placeAt("c.created_at")+` AS place,c.released_units::bigint AS released_units
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
