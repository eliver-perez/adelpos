package legacyimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/plural"
)

// Instalación limpia (§4.3 y decisión 4 del propietario): la importación solo se permite si AD-EL POS no tiene
// operación propia (ventas, turnos, pagos, cocina, inventario, compras: nota técnica 6 de la Fase 0) ni catálogo,
// mesas o usuarios propios fuera de lo que crea una instalación nueva (semillas de las migraciones y el asistente
// inicial) y de lo que pertenece al lote importado vigente. Lo que lo impide se explica en lenguaje sencillo.

// Blocker es una razón por la que no se puede importar, con su conteo.
type Blocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Count   int64  `json:"count"`
}

// operationCheck es una tabla operativa que debe estar vacía.
type operationCheck struct {
	code     string
	table    string
	singular string
	plural   string
}

var operationChecks = []operationCheck{
	{"own_accounts", "restaurant_accounts", "cuenta registrada en AD-EL POS", "cuentas registradas en AD-EL POS"},
	{"own_orders", "restaurant_orders", "pedido registrado", "pedidos registrados"},
	{"own_items", "restaurant_item_units", "producto pedido", "productos pedidos"},
	{"own_drafts", "restaurant_draft_items", "producto en captura sin enviar", "productos en captura sin enviar"},
	{"own_payments", "payments", "cobro", "cobros"},
	{"own_refunds", "payment_refunds", "devolución", "devoluciones"},
	{"own_adjustments", "account_adjustments", "descuento o cargo de cuenta", "descuentos o cargos de cuenta"},
	{"own_cancellations", "account_cancellations", "cuenta cancelada", "cuentas canceladas"},
	{"own_sales", "sales_recognitions", "venta reconocida", "ventas reconocidas"},
	{"own_deliveries", "takeaway_deliveries", "entrega para llevar", "entregas para llevar"},
	{"own_shifts", "cash_shifts", "turno de caja", "turnos de caja"},
	{"own_cash_operations", "cash_operations", "retiro o gasto de caja", "retiros o gastos de caja"},
	{"own_cash_movements", "cash_movements", "movimiento de caja", "movimientos de caja"},
	{"own_cash_counts", "cash_counts", "arqueo de caja", "arqueos de caja"},
	{"own_kitchen", "kitchen_unit_releases", "envío a cocina", "envíos a cocina"},
	{"own_inventory_movements", "inventory_movements", "movimiento de inventario", "movimientos de inventario"},
	{"own_receipts", "inventory_receipts", "recepción de compra", "recepciones de compra"},
	{"own_inventory_adjustments", "inventory_adjustments", "ajuste de inventario", "ajustes de inventario"},
	{"own_inventory_counts", "inventory_counts", "conteo de inventario", "conteos de inventario"},
	{"own_transfers", "inventory_transfers", "traspaso de inventario", "traspasos de inventario"},
	{"own_purchase_orders", "inventory_purchase_orders", "orden de compra", "órdenes de compra"},
	{"own_productions", "inventory_productions", "producción de inventario", "producciones de inventario"},
	{"own_supplier_returns", "inventory_supplier_returns", "devolución a proveedor", "devoluciones a proveedor"},
}

// ownCheck es lo propio del catálogo, las mesas y los usuarios: filas que no son del lote vigente ni de una
// instalación nueva. query cuenta con $1 = el lote vigente (o nulo).
type ownCheck struct {
	code     string
	query    string
	singular string
	plural   string
}

// notInBatch es la condición «no pertenece al lote vigente» para la entidad y la columna indicadas.
func notInBatch(entity, column string) string {
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM app.legacy_id_map m WHERE m.batch_id = $1 AND m.entity = '%s'
        AND m.new_id = %s)`, entity, column)
}

var ownChecks = []ownCheck{
	{"own_categories", "SELECT count(*) FROM app.catalog_categories c WHERE " + notInBatch("category", "c.id"),
		"categoría creada en AD-EL POS", "categorías creadas en AD-EL POS"},
	{"own_products", "SELECT count(*) FROM app.catalog_products p WHERE " + notInBatch("product", "p.id"),
		"producto o combo creado en AD-EL POS", "productos o combos creados en AD-EL POS"},
	{"own_variants", "SELECT count(*) FROM app.catalog_variants", "presentación de producto", "presentaciones de producto"},
	{"own_ingredients", "SELECT count(*) FROM app.catalog_ingredients", "ingrediente", "ingredientes"},
	{"own_portions", "SELECT count(*) FROM app.catalog_portions", "porción", "porciones"},
	{"own_complements", "SELECT count(*) FROM app.catalog_complements c WHERE " + notInBatch("complement", "c.id"),
		"complemento creado en AD-EL POS", "complementos creados en AD-EL POS"},
	{"own_groups", "SELECT count(*) FROM app.catalog_option_groups g WHERE " + notInBatch("option_group", "g.id"),
		"grupo de complementos creado en AD-EL POS", "grupos de complementos creados en AD-EL POS"},
	{"own_options", "SELECT count(*) FROM app.catalog_group_options o WHERE " + notInBatch("group_option", "o.id"),
		"opción de complementos creada en AD-EL POS", "opciones de complementos creadas en AD-EL POS"},
	{"own_promotions", "SELECT count(*) FROM app.catalog_promotions", "promoción", "promociones"},
	{"own_images", "SELECT count(*) FROM app.catalog_images", "imagen del catálogo", "imágenes del catálogo"},
	// Una instalación nueva tiene una sola lista de precios («Restaurante», de la migración 0003).
	{"own_price_lists", "SELECT greatest(count(*) - 1, 0) FROM app.catalog_price_lists", "lista de precios adicional",
		"listas de precios adicionales"},
	{"own_quick_notes", "SELECT count(*) FROM app.catalog_category_quick_notes", "indicación rápida para cocina",
		"indicaciones rápidas para cocina"},
	{"own_areas", "SELECT count(*) FROM app.restaurant_areas a WHERE " + notInBatch("area", "a.id"),
		"área de mesas creada en AD-EL POS", "áreas de mesas creadas en AD-EL POS"},
	{"own_tables", "SELECT count(*) FROM app.restaurant_tables t WHERE " + notInBatch("table", "t.id"),
		"mesa creada en AD-EL POS", "mesas creadas en AD-EL POS"},
	// El asistente inicial crea al administrador en la misma transacción que el establecimiento (misma marca de
	// tiempo: now() es la hora de la transacción); sa es la identidad de soporte reservada.
	{"own_users", `SELECT count(*) FROM app.users u WHERE NOT u.support
            AND u.created_at <> coalesce((SELECT configured_at FROM app.establishment), 'epoch'::timestamptz)
            AND ` + notInBatch("user", "u.id"),
		"usuario creado en AD-EL POS", "usuarios creados en AD-EL POS"},
}

// CurrentBatchID es el lote vigente o "" si no hay.
func CurrentBatchID(ctx context.Context, tx pgx.Tx) (string, error) {
	var id *string
	err := tx.QueryRow(ctx, `SELECT (SELECT id::text FROM app.legacy_import_batches WHERE status = 'completed')`).Scan(&id)
	if err != nil || id == nil {
		return "", err
	}
	return *id, nil
}

// CheckInstallation dice qué impide importar (vacío si la instalación está limpia), con lo del lote vigente
// excluido.
func CheckInstallation(ctx context.Context, tx pgx.Tx) ([]Blocker, error) {
	current, err := CurrentBatchID(ctx, tx)
	if err != nil {
		return nil, err
	}
	var batch *string
	if current != "" {
		batch = &current
	}
	blockers := []Blocker{}
	for _, check := range operationChecks {
		var count int64
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM app."+check.table).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			blockers = append(blockers, blocker(check.code, count, check.singular, check.plural))
		}
	}
	for _, check := range ownChecks {
		var count int64
		arguments := []any{}
		if strings.Contains(check.query, "$1") {
			arguments = append(arguments, batch)
		}
		if err := tx.QueryRow(ctx, check.query, arguments...).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			blockers = append(blockers, blocker(check.code, count, check.singular, check.plural))
		}
	}
	return blockers, nil
}

func blocker(code string, count int64, singular, pluralText string) Blocker {
	return Blocker{
		Code:    code,
		Count:   count,
		Message: "Hay " + plural.Count(int(count), singular, pluralText) + ".",
	}
}

// notCleanError es el rechazo de una instalación con operación o datos propios.
func notCleanError(blockers []Blocker) *Error {
	return &Error{
		Code: CodeNotClean,
		Message: "Esta instalación ya tiene datos propios de AD-EL POS y no se puede importar el sistema anterior. " +
			"Importa en una instalación nueva o limpia.",
		Blockers: blockers,
	}
}
