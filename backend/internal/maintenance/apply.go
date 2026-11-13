package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/events"
)

// Result es lo que se quitó: lo eliminado, lo desactivado, la configuración desvinculada, lo que se
// conservó y el nombre del archivo de respaldo (sin carpeta). Backup queda vacío si no había nada que
// quitar y por eso no se respaldó.
type Result struct {
	Removed     []Entry `json:"removed"`
	Deactivated []Entry `json:"deactivated"`
	Unlinked    []Entry `json:"unlinked"`
	Kept        []Entry `json:"kept"`
	Backup      string  `json:"backup"`
}

// statement es una sentencia del plan con sus argumentos (arreglos de identificadores).
type statement struct {
	sql       string
	arguments []any
}

// Apply ejecuta el plan en la transacción de quien llama, que ya tomó los locks exclusivos y calculó el
// plan dentro de ella. Primero desvincula la configuración, después elimina o desactiva en el orden que
// exigen las llaves foráneas (rutas → estaciones → productos → categorías → impresoras) y al final
// publica los eventos de configuración para que las pantallas abiertas vuelvan a consultar. Nunca toca
// ventas, pagos, cortes, movimientos de inventario, cuentas, documentos ni auditoría.
func Apply(ctx context.Context, tx pgx.Tx, plan *Plan, backup string) (Result, error) {
	result := Result{
		Removed:     plan.Remove,
		Deactivated: plan.Deactivate,
		Unlinked:    plan.Unlink,
		Kept:        plan.Keep,
		Backup:      backup,
	}
	if plan.Empty() {
		return result, nil
	}
	printers := list(plan.unlinkPrinters)
	stations := list(plan.testStations)
	statements := []statement{
		// Configuración que apunta a las impresoras de prueba.
		{`UPDATE app.payment_settings SET receipt_printer_id = NULL, revision = revision + 1, updated_at = now()
            WHERE receipt_printer_id::text = ANY($1::text[])`, []any{printers}},
		{`UPDATE app.cash_drawer_settings SET printer_id = NULL, enabled = false, revision = revision + 1, updated_at = now()
            WHERE printer_id::text = ANY($1::text[])`, []any{printers}},
		{`UPDATE app.kitchen_printers SET fallback_id = NULL, revision = revision + 1, updated_at = now()
            WHERE fallback_id::text = ANY($1::text[])`, []any{printers}},
		// Estaciones de prueba fuera de las terminales y de las rutas que se conservan.
		{`DELETE FROM app.kitchen_device_stations WHERE station_id::text = ANY($1::text[])`, []any{stations}},
		{`UPDATE app.kitchen_routes SET revision = revision + 1, updated_at = now() WHERE id::text = ANY($1::text[])`,
			[]any{list(plan.trimmedRoutes)}},
		{`DELETE FROM app.kitchen_route_stations
            WHERE route_id::text = ANY($1::text[]) OR station_id::text = ANY($2::text[])`,
			[]any{list(plan.deleteRoutes), stations}},
		{`DELETE FROM app.kitchen_routes WHERE id::text = ANY($1::text[])`, []any{list(plan.deleteRoutes)}},
		// Estaciones: la asignación de almacén solo se borra con la estación (sin tareas, nunca se usó).
		{`DELETE FROM app.inventory_station_warehouses WHERE station_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteStations)}},
		{`DELETE FROM app.kitchen_stations WHERE id::text = ANY($1::text[])`, []any{list(plan.deleteStations)}},
		{`UPDATE app.kitchen_stations SET active = false, revision = revision + 1, updated_at = now()
            WHERE id::text = ANY($1::text[])`, []any{list(plan.deactivateStations)}},
		// Productos: su configuración de catálogo se elimina con ellos.
		{`DELETE FROM app.catalog_promotion_products WHERE product_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_price_entries WHERE product_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_group_defaults WHERE product_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_product_groups WHERE product_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_product_ingredients WHERE product_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_variants WHERE product_id::text = ANY($1::text[])`, []any{list(plan.deleteProducts)}},
		{`DELETE FROM app.catalog_products WHERE id::text = ANY($1::text[])`, []any{list(plan.deleteProducts)}},
		{`UPDATE app.catalog_products SET active = false, archived = true, revision = revision + 1, updated_at = now()
            WHERE id::text = ANY($1::text[])`, []any{list(plan.deactivateProducts)}},
		// Categorías: sus indicaciones rápidas y promociones por categoría se eliminan con ellas.
		{`DELETE FROM app.catalog_category_quick_notes WHERE category_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteCategories)}},
		{`DELETE FROM app.catalog_promotion_categories WHERE category_id::text = ANY($1::text[])`,
			[]any{list(plan.deleteCategories)}},
		{`DELETE FROM app.catalog_categories WHERE id::text = ANY($1::text[])`, []any{list(plan.deleteCategories)}},
		{`UPDATE app.catalog_categories SET active = false, revision = revision + 1, updated_at = now()
            WHERE id::text = ANY($1::text[])`, []any{list(plan.deactivateCategories)}},
		// Impresoras, al final: ya no las usa ninguna estación eliminada ni la configuración.
		{`DELETE FROM app.kitchen_printers WHERE id::text = ANY($1::text[])`, []any{list(plan.deletePrinters)}},
		{`UPDATE app.kitchen_printers SET active = false, revision = revision + 1, updated_at = now()
            WHERE id::text = ANY($1::text[])`, []any{list(plan.deactivatePrinters)}},
	}
	for _, step := range statements {
		if _, err := tx.Exec(ctx, step.sql, step.arguments...); err != nil {
			return Result{}, err
		}
	}
	return result, publishChanges(ctx, tx, plan)
}

// publishChanges avisa por eventos a las pantallas abiertas (Cocina, Cobros, Caja y el resto por el aviso
// general de mantenimiento). Los eventos no llevan contenido, solo el tipo de cambio.
func publishChanges(ctx context.Context, tx pgx.Tx, plan *Plan) error {
	kitchen := []struct {
		kind    string
		changed bool
	}{
		{"kitchen.printers.changed", len(plan.deletePrinters)+len(plan.deactivatePrinters)+len(plan.fallbackPrinters) > 0},
		{"kitchen.stations.changed", len(plan.deleteStations)+len(plan.deactivateStations) > 0},
		{"kitchen.routes.changed", len(plan.deleteRoutes)+len(plan.trimmedRoutes) > 0},
		{"kitchen.access.changed", plan.deviceLinks},
		{"payments.settings.changed", plan.receiptPrinter},
	}
	for _, change := range kitchen {
		if !change.changed {
			continue
		}
		if err := events.Append(ctx, tx, change.kind, "configuration", nil, nil); err != nil {
			return err
		}
	}
	for _, registerID := range plan.drawerRegisters {
		register := registerID
		if err := events.Append(ctx, tx, "cash.settings.changed", "cash", &register, nil); err != nil {
			return err
		}
	}
	return events.Append(ctx, tx, "maintenance.test_data.removed", "configuration", nil, nil)
}
