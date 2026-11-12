package reports

import "context"

// Etapa 9C (§5.1): cuentas anuladas y modificaciones de cantidades, los dos reportes del sistema
// anterior que faltaban, con los datos que AD-EL POS guarda hoy (Fase 0, punto 6):
//   - La anulación de una cuenta (evento account.void, con su motivo) y la liberación de una cuenta
//     vacía (account.released, motivo fijo «Cuenta vacía liberada») dejan la cuenta en «voided» y
//     registran quién, desde qué terminal y cuándo en restaurant_events. Solo se anula una cuenta sin
//     productos activos ni borradores y con cobros netos en cero, de modo que su importe útil es el
//     de las unidades que se cancelaron en ella antes de anularla.
//   - Una partida se reduce o se cancela después de confirmarse con «Cancelar» (item.cancelled) o al
//     modificarla con menos unidades (item.modified con cancelled_unit_ids). Cada evento guarda las
//     unidades de antes con sus importes, las que se retiraron, el motivo y el autor.
//
// No hay columna «Autorizó»: AD-EL POS no pide la autorización de otra persona para anular una
// cuenta ni para reducir o cancelar cantidades (basta el permiso del propio usuario, accounts.manage
// u orders.cancel), así que no existe ese dato y no se inventa. Los dos reportes usan el permiso de
// «Productos cancelados» (reports.sales.read) y no toman locks: solo leen.

const voidedDateBasis = "Fecha de la anulación de la cuenta, con la hora de inicio del día."

const changesDateBasis = "Fecha de la cancelación o modificación de la partida, con la hora de inicio del día."

// noAuthorizationNote explica por qué no hay columna «Autorizó».
const noAuthorizationNote = "No hay columna «Autorizó»: AD-EL POS no pide la autorización de otra persona para esta acción " +
	"(basta el permiso de quien la hace)."

func filterVoidKind() Filter {
	return filterChoice("kind", "Tipo", "",
		Option{Value: "void", Label: "Anuladas con motivo"},
		Option{Value: "released", Label: "Cuentas vacías liberadas"})
}

// placeSQL es la mesa (salón) o el origen (para llevar) de una cuenta con la mesa actual.
const placeSQL = "CASE WHEN a.mode='dine_in' THEN coalesce(place.name,'Sin mesa') ELSE a.origin END"

const modeSQL = "CASE a.mode WHEN 'dine_in' THEN 'Salón' ELSE 'Para llevar' END"

func init() {
	register(Definition{
		ID: "sales.voided_accounts", Group: "sales", Title: "Cuentas anuladas",
		Description: "Cuentas anuladas con su motivo y las cuentas vacías liberadas, con quién, desde qué terminal y el importe " +
			"de las unidades que se cancelaron en ellas.",
		Permissions: []string{PermissionSales}, DateBasis: voidedDateBasis,
		Filters: []Filter{filterVoidKind(), filterMode(), filterArea(), filterUser("Usuario que anuló")},
		Columns: []Column{dateTimeColumn("voided_at", "Anulación"), codeColumn("folio", "Folio"), textColumn("kind", "Tipo"),
			textColumn("voided_by", "Anuló"), textColumn("terminal", "Terminal"), textColumn("reason", "Motivo"),
			textColumn("mode", "Modalidad"), textColumn("place", "Mesa u origen"), textColumn("area", "Área"),
			described(countColumn("cancelled_units", "Unidades canceladas"),
				"Unidades confirmadas que se cancelaron en la cuenta antes de anularla."),
			described(moneyColumn("cancelled_amount", "Importe cancelado"),
				"Importe de esas unidades. Una cuenta solo se anula sin productos activos y con cobros netos en cero."),
			optional(textColumn("account", "Cuenta")), optional(dateTimeColumn("opened_at", "Apertura")),
			optional(textColumn("opened_by", "Abrió"))},
		DefaultSort: Sort{Key: "voided_at"},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			// Las cuentas anuladas son pocas: se parte de ellas y el evento de cada una se lee por el
			// índice de restaurant_events por cuenta.
			list := conditions{inPeriod("e.created_at")}
			list.when(query, "kind", "e.kind = CASE @kind::text WHEN 'released' THEN 'account.released' ELSE 'account.void' END")
			list.when(query, "mode", "a.mode = @mode")
			list.when(query, "area_id", "place.area_id = @area_id::uuid")
			list.when(query, "user_id", "e.author_id = @user_id::uuid")
			rows, err := query.collect(ctx, `SELECT e.created_at AS voided_at,a.folio::text AS folio,
                    CASE e.kind WHEN 'account.released' THEN 'Cuenta vacía liberada' ELSE 'Anulada' END AS kind,
                    author.display_name AS voided_by,coalesce(device.name,'') AS terminal,
                    coalesce(e.payload->'after'->>'reason','') AS reason,`+modeSQL+` AS mode,`+placeSQL+` AS place,
                    coalesce(area.name,'') AS area,coalesce(units.quantity,0)::bigint AS cancelled_units,
                    coalesce(units.amount,0) AS cancelled_amount,a.name AS account,a.created_at AS opened_at,
                    opener.display_name AS opened_by
                FROM app.restaurant_accounts a
                JOIN LATERAL (
                    SELECT ev.kind,ev.created_at,ev.payload,ev.author_id,ev.device_id FROM app.restaurant_events ev
                    WHERE ev.account_id=a.id AND ev.kind IN ('account.void','account.released')
                    ORDER BY ev.sequence DESC LIMIT 1
                ) e ON true
                JOIN app.users author ON author.id=e.author_id
                JOIN app.users opener ON opener.id=a.opened_by
                LEFT JOIN app.devices device ON device.id=e.device_id
                LEFT JOIN app.restaurant_tables place ON place.id=a.table_id
                LEFT JOIN app.restaurant_areas area ON area.id=place.area_id
                LEFT JOIN LATERAL (
                    SELECT count(*) AS quantity,sum(u.total) AS amount FROM app.restaurant_item_units u
                    WHERE u.account_id=a.id AND u.state='cancelled'
                ) units ON true
                WHERE a.state='voided'`+list.sql(), query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{
				"«Cuenta vacía liberada» es la liberación de una mesa sin productos (motivo fijo). La mesa y el área son las actuales de la cuenta.",
				"Las unidades canceladas también aparecen, una por una, en «Productos cancelados» y en «Modificaciones de cantidades».",
				noAuthorizationNote,
			}}, nil
		},
	})
	register(Definition{
		ID: "products.quantity_changes", Group: "products", Title: "Modificaciones de cantidades",
		Description: "Partidas confirmadas cuya cantidad se redujo o que se cancelaron: cantidad anterior y nueva, diferencia en " +
			"unidades e importe, quién, motivo y fecha.",
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
			textColumn("changed_by", "Modificó"), textColumn("reason", "Motivo"),
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
// en lugar de inventarse; la diferencia y el importe salen siempre del evento.
func quantityChanges(ctx context.Context, query *Query) (*Dataset, error) {
	list := conditions{}
	list.when(query, "product_id", "c.product_id = @product_id::text")
	list.when(query, "mode", "a.mode = @mode")
	list.when(query, "user_id", "c.author_id = @user_id::uuid")
	rows, err := query.collect(ctx, `WITH removed AS (
            SELECT DISTINCT e.id FROM app.restaurant_item_units u
            JOIN LATERAL (
                SELECT ev.id FROM app.restaurant_events ev
                WHERE ev.account_id=u.account_id AND ev.kind IN ('item.cancelled','item.modified')
                    AND (coalesce(ev.payload->'after'->'unit_ids','[]'::jsonb) ? u.id::text
                        OR coalesce(ev.payload->'after'->'cancelled_unit_ids','[]'::jsonb) ? u.id::text)
                ORDER BY ev.sequence DESC LIMIT 1
            ) e ON true
            WHERE u.state='cancelled' AND `+inPeriod("u.cancelled_at")+`
        ), changes AS (
            SELECT e.created_at,e.account_id,e.kind,e.author_id,e.device_id,e.operation_id,
                coalesce(e.payload->'after'->>'reason','') AS reason,
                coalesce(e.payload->'before'->'quote_line'->>'name','') AS product,
                e.payload->'before'->'request'->>'product_id' AS product_id,
                coalesce(e.payload->'before'->'units','[]'::jsonb) AS units_before,
                CASE e.kind WHEN 'item.cancelled' THEN coalesce(e.payload->'after'->'unit_ids','[]'::jsonb)
                    ELSE coalesce(e.payload->'after'->'cancelled_unit_ids','[]'::jsonb) END AS removed_ids,
                coalesce(e.payload->'after'->>'version_id','') AS version_id
            FROM removed r JOIN app.restaurant_events e ON e.id=r.id
        ), measured AS (
            SELECT c.*,jsonb_array_length(c.removed_ids) AS removed,
                CASE WHEN c.kind='item.modified' THEN jsonb_array_length(c.units_before)
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
            author.display_name AS changed_by,c.reason,
            CASE c.kind WHEN 'item.cancelled' THEN 'Cancelación' ELSE 'Modificación de la partida' END AS action,
            coalesce(device.name,'') AS terminal,`+modeSQL+` AS mode,`+placeSQL+` AS place,c.released_units::bigint AS released_units
        FROM measured c
        JOIN app.restaurant_accounts a ON a.id=c.account_id
        JOIN app.users author ON author.id=c.author_id
        LEFT JOIN app.devices device ON device.id=c.device_id
        LEFT JOIN app.restaurant_tables place ON place.id=a.table_id
        WHERE c.removed > 0`+list.sql(), query.Args())
	if err != nil {
		return nil, err
	}
	return &Dataset{Rows: rows, Notes: []string{
		"Solo partidas ya confirmadas: los borradores no se registran. Las unidades retiradas nunca se reconocen como venta.",
		"Una modificación que además cambió la personalización recalculó las unidades que siguieron; aquí solo cuenta el importe de las retiradas.",
		noAuthorizationNote,
	}}, nil
}
