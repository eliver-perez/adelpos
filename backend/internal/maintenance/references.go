package maintenance

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// reference es una columna que apunta con llave foránea al identificador de una tabla del esquema app.
type reference struct {
	table  string
	column string
}

// referenceFinder averigua qué registros dependen de un dato de prueba. Las llaves foráneas se leen del
// catálogo de PostgreSQL y no de una lista escrita a mano: una tabla nueva de una migración futura que
// apunte a una estación, impresora, producto o categoría cuenta como «registro relacionado» y el dato se
// desactiva en lugar de borrarse, en vez de fallar o de borrar algo que no se revisó.
type referenceFinder struct {
	ctx   context.Context
	tx    pgx.Tx
	cache map[string][]reference
}

// references devuelve las llaves foráneas que apuntan a la columna id de la tabla app.<table>.
func (finder *referenceFinder) references(table string) ([]reference, error) {
	if cached, ok := finder.cache[table]; ok {
		return cached, nil
	}
	rows, err := finder.tx.Query(finder.ctx, `SELECT source.relname::text, local_column.attname::text
        FROM pg_constraint foreign_key
        JOIN pg_class source ON source.oid = foreign_key.conrelid
        JOIN pg_namespace namespace ON namespace.oid = source.relnamespace
        CROSS JOIN LATERAL unnest(foreign_key.conkey, foreign_key.confkey) AS pair(local_number, foreign_number)
        JOIN pg_attribute local_column ON local_column.attrelid = foreign_key.conrelid
            AND local_column.attnum = pair.local_number
        JOIN pg_attribute foreign_column ON foreign_column.attrelid = foreign_key.confrelid
            AND foreign_column.attnum = pair.foreign_number
        WHERE foreign_key.contype = 'f'
            AND foreign_key.confrelid = $1::text::regclass
            AND namespace.nspname = 'app'
            AND foreign_column.attname = 'id'
        ORDER BY 1, 2`, "app."+table)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reference, error) {
		var item reference
		err := row.Scan(&item.table, &item.column)
		return item, err
	})
	if err != nil {
		return nil, err
	}
	finder.cache[table] = found
	return found, nil
}

// blocking devuelve las tablas con registros que apuntan al dato id de app.<table>, salvo las referencias
// que el plan maneja por su cuenta (configuración que se borra o se desvincula junto con el dato).
func (finder *referenceFinder) blocking(table, id string, handled map[reference]bool) ([]string, error) {
	candidates, err := finder.references(table)
	if err != nil {
		return nil, err
	}
	found := []string{}
	for _, candidate := range candidates {
		if handled[candidate] {
			continue
		}
		query := "SELECT EXISTS(SELECT FROM " + pgx.Identifier{"app", candidate.table}.Sanitize() +
			" WHERE " + pgx.Identifier{candidate.column}.Sanitize() + " = $1::uuid)"
		var exists bool
		if err := finder.tx.QueryRow(finder.ctx, query, id).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			found = append(found, candidate.table)
		}
	}
	return found, nil
}

// recordNames traduce las tablas con registros a palabras del restaurante (sin nombres técnicos).
var recordNames = map[string]string{
	"kitchen_print_jobs":           "trabajos de impresión",
	"kitchen_tasks":                "tareas de cocina",
	"kitchen_kds_receipts":         "comandas recibidas en pantalla",
	"realtime_events":              "avisos de cocina",
	"restaurant_items":             "ventas",
	"restaurant_item_versions":     "ventas",
	"restaurant_draft_items":       "productos en cuentas abiertas",
	"inventory_recipes":            "recetas de inventario",
	"inventory_station_warehouses": "almacenes de inventario asignados",
	"catalog_combo_slots":          "combos que lo incluyen",
	"catalog_combo_options":        "combos que lo incluyen",
	"catalog_complements":          "complementos que lo usan",
	"catalog_price_entries":        "listas de precios",
}

// operationsReason explica por qué un dato se desactiva: «Ya tiene registros: ventas y tareas de cocina.»
func operationsReason(tables []string) string {
	names := []string{}
	for _, table := range tables {
		name, ok := recordNames[table]
		if !ok {
			name = "otros registros"
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return "Ya tiene registros: " + spanishList(names) + "."
}

// spanishList une palabras como en español: «a», «a y b», «a, b y c».
func spanishList(words []string) string {
	if len(words) <= 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " y " + words[len(words)-1]
}
