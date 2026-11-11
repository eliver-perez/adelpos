package reports

import (
	"context"

	"github.com/shopspring/decimal"
)

// Cocina y operación (§0-B: la demanda y la cocina pueden usar la fecha de liberación a cocina,
// siempre indicándolo). La fuente es la liberación canónica por unidad (kitchen_unit_releases): una
// unidad se cuenta una vez aunque se prepare en varias estaciones o se reimprima su comanda. Las
// tareas por estación (kitchen_tasks) solo se usan para tiempos de preparación, nunca como ventas.

const kitchenDateBasis = "Fecha de liberación a cocina de cada unidad, con la hora de inicio del día (no es la fecha de venta)."

func kitchenConditions(query *Query) conditions {
	list := conditions{inPeriod("k.released_at")}
	list.when(query, "product_id", "i.product_id = @product_id::uuid")
	list.when(query, "category_id", "v.quote_line->>'category_id' = @category_id")
	list.when(query, "mode", "a.mode = @mode")
	return list
}

const kitchenUnits = `FROM app.kitchen_unit_releases k
    JOIN app.restaurant_item_units u ON u.id=k.unit_id
    JOIN app.restaurant_item_versions v ON v.id=k.version_id
    JOIN app.restaurant_items i ON i.id=u.item_id
    JOIN app.restaurant_accounts a ON a.id=u.account_id`

func init() {
	kitchenFilters := []Filter{filterProduct(), filterCategory(), filterMode()}
	register(Definition{
		ID: "kitchen.demand_by_product", Group: "kitchen", Title: "Demanda por producto",
		Description: "Unidades liberadas a cocina por producto, incluidas las que se cancelaron después.",
		Permissions: []string{PermissionSales}, DateBasis: kitchenDateBasis, Filters: kitchenFilters,
		Columns: []Column{textColumn("product", "Producto"),
			described(countColumn("released", "Liberadas"), "Unidades liberadas a cocina en el periodo."),
			described(countColumn("cancelled_after", "Canceladas después"), "Unidades liberadas que después se cancelaron.")},
		DefaultSort: Sort{Key: "released", Descending: true},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT (array_agg(coalesce(v.quote_line->>'name','') ORDER BY k.released_at DESC))[1] AS product,
                    count(*)::bigint AS released,count(*) FILTER (WHERE u.state='cancelled')::bigint AS cancelled_after
                `+kitchenUnits+` WHERE true`+kitchenConditions(query).sql()+` GROUP BY i.product_id`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows}, nil
		},
	})
	hourColumns := []Column{textColumn("hour", "Hora"),
		described(countColumn("released", "Liberadas"), "Unidades liberadas a cocina en esa hora."),
		described(countColumn("cancelled_after", "Canceladas después"), "Unidades liberadas que después se cancelaron.")}
	register(Definition{
		ID: "kitchen.demand_by_hour", Group: "kitchen", Title: "Demanda por hora",
		Description: "Unidades liberadas a cocina por hora del día, en el orden del día comercial.",
		Permissions: []string{PermissionSales}, DateBasis: kitchenDateBasis, Filters: kitchenFilters, Columns: hourColumns,
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT `+localHour("k.released_at")+` AS hour_number,count(*)::bigint AS released,
                    count(*) FILTER (WHERE u.state='cancelled')::bigint AS cancelled_after
                `+kitchenUnits+` WHERE true`+kitchenConditions(query).sql()+` GROUP BY 1`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: fillHours(rows, query.Config, hourColumns)}, nil
		},
	})
	register(Definition{
		ID: "kitchen.stations", Group: "kitchen", Title: "Preparación por estación",
		Description: "Tareas de cocina por estación y tiempo promedio desde la liberación hasta «Listo».",
		Permissions: []string{PermissionSales}, DateBasis: "Fecha de liberación de cada tarea a su estación, con la hora de inicio del día.",
		Columns: []Column{textColumn("station", "Estación"), countColumn("tasks", "Tareas"), countColumn("ready", "Listas"),
			countColumn("cancelled", "Canceladas"),
			optional(described(countColumn("finished", "Con hora de «Listo»"), "Tareas con hora de «Listo» (base del promedio).")),
			described(Column{Key: "average_minutes", Label: "Minutos promedio", Type: TypeQuantity, Total: TotalCustom, Sortable: true},
				"Promedio de minutos entre la liberación y «Listo» de las tareas terminadas.")},
		DefaultSort: Sort{Key: "tasks", Descending: true},
		Totals: func(rows []Row, totals Row) {
			minutes, ready := decimal.Zero, decimal.Zero
			for _, row := range rows {
				if average, ok := row["average_minutes"].(decimal.Decimal); ok {
					count := decimalOf(row, "finished")
					minutes = minutes.Add(average.Mul(count))
					ready = ready.Add(count)
				}
			}
			if !ready.IsZero() {
				totals["average_minutes"] = minutes.DivRound(ready, 1)
			}
		},
		Query: func(ctx context.Context, query *Query) (*Dataset, error) {
			rows, err := query.collect(ctx, `SELECT s.name AS station,count(*)::bigint AS tasks,
                    count(*) FILTER (WHERE t.state='ready')::bigint AS ready,
                    count(*) FILTER (WHERE t.state='cancelled')::bigint AS cancelled,
                    count(*) FILTER (WHERE t.ready_at IS NOT NULL)::bigint AS finished,
                    round((avg(extract(epoch FROM t.ready_at-t.released_at)/60) FILTER (WHERE t.ready_at IS NOT NULL))::numeric,1)
                        AS average_minutes
                FROM app.kitchen_tasks t JOIN app.kitchen_stations s ON s.id=t.station_id
                WHERE `+inPeriod("t.released_at")+` GROUP BY s.id,s.name`, query.Args())
			if err != nil {
				return nil, err
			}
			return &Dataset{Rows: rows, Notes: []string{"Una tarea por unidad y estación: las tareas no son ventas ni unidades vendidas."}}, nil
		},
	})
}
