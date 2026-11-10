package inventory

import (
	"time"

	"local/ad-el-pos/server/internal/features"
)

// Resumen básico de la pestaña «Resumen» (fase A). Con inventory.read muestra conteos que no
// revelan costos; la cantidad de insumos con costo pendiente solo con inventory.costs.read y el
// valor total del inventario solo con inventory.valuation.read. La fase D lo completa con alertas.

type Summary struct {
	Feature          FeatureState `json:"feature"`
	ActiveItems      int          `json:"active_items"`
	InactiveItems    int          `json:"inactive_items"`
	ActiveWarehouses int          `json:"active_warehouses"`
	// NegativeStock cuenta pares (insumo activo, almacén) con existencia menor que cero.
	NegativeStock int `json:"negative_stock"`
	// BelowMinimum y AtReorderPoint cuentan insumos activos por su existencia total.
	BelowMinimum      int     `json:"below_minimum"`
	AtReorderPoint    int     `json:"at_reorder_point"`
	CostsVisible      bool    `json:"costs_visible"`
	PendingCostItems  *int    `json:"pending_cost_items"`
	ValuationVisible  bool    `json:"valuation_visible"`
	TotalValue        *string `json:"total_value"`
	TotalValueDisplay *string `json:"total_value_display"`
}

// FeatureState es el estado de la función «Control de inventario».
type FeatureState struct {
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	EnabledAt *time.Time `json:"enabled_at"`
	Revision  int        `json:"revision"`
}

func (handler *service) summary(_ []string) (any, error) {
	feature, err := features.Read(handler.ctx, handler.tx, features.InventoryControl)
	if err != nil {
		return nil, err
	}
	result := Summary{
		Feature: FeatureState{
			Code: feature.Code, Name: feature.Name, Enabled: feature.Enabled, EnabledAt: feature.EnabledAt, Revision: feature.Revision,
		},
		CostsVisible:     handler.costsVisible(),
		ValuationVisible: handler.valuationVisible(),
	}
	err = handler.tx.QueryRow(handler.ctx, `SELECT
            (SELECT count(*) FROM app.inventory_items WHERE active),
            (SELECT count(*) FROM app.inventory_items WHERE NOT active),
            (SELECT count(*) FROM app.inventory_warehouses WHERE active),
            (SELECT count(*) FROM app.inventory_stock s JOIN app.inventory_items i ON i.id=s.item_id
                WHERE i.active AND s.quantity<0),
            (SELECT count(*) FROM app.inventory_items i LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
                WHERE i.active AND i.min_stock IS NOT NULL AND coalesce(v.quantity,0) < i.min_stock),
            (SELECT count(*) FROM app.inventory_items i LEFT JOIN app.inventory_valuations v ON v.item_id=i.id
                WHERE i.active AND i.reorder_point IS NOT NULL AND coalesce(v.quantity,0) <= i.reorder_point)`).Scan(
		&result.ActiveItems, &result.InactiveItems, &result.ActiveWarehouses, &result.NegativeStock, &result.BelowMinimum,
		&result.AtReorderPoint)
	if err != nil {
		return nil, err
	}
	if result.CostsVisible {
		var pending int
		if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM app.inventory_valuations WHERE pending_cost_quantity>0").
			Scan(&pending); err != nil {
			return nil, err
		}
		result.PendingCostItems = &pending
	}
	if result.ValuationVisible {
		total, err := handler.totalInventoryValue()
		if err != nil {
			return nil, err
		}
		exact, display := exactMoneyText(total), displayMoneyText(total)
		result.TotalValue, result.TotalValueDisplay = &exact, &display
	}
	return result, nil
}
