package ordering

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Scope identifica una lista de hermanos. Es una enumeración cerrada: la tabla y la columna de
// agrupación salen de esta definición del código, nunca de datos enviados por el cliente.
type Scope int

const (
	// CatalogCategories ordena todas las categorías del catálogo.
	CatalogCategories Scope = iota + 1
	// CatalogProducts ordena los productos (combos incluidos) dentro de su categoría.
	CatalogProducts
	// RestaurantAreas ordena las áreas del restaurante.
	RestaurantAreas
	// KitchenStations ordena las estaciones de preparación.
	KitchenStations
	// PaymentMethods ordena los métodos de pago.
	PaymentMethods
	// CashExpenseCategories ordena las categorías de gasto pagado desde caja.
	CashExpenseCategories
	// DiscountReasons ordena los motivos de descuento (etapa 7.1).
	DiscountReasons
	// CategoryQuickNotes ordena las indicaciones rápidas dentro de su categoría (etapa 7.1).
	CategoryQuickNotes
)

type scopeDefinition struct {
	// table es la tabla calificada; todas tienen id, sort_order, revision y updated_at.
	table string
	// group es la columna que separa listas independientes; vacía si la lista es única.
	group string
	// label es la columna visible que desempata el orden; vacía equivale a name.
	label string
}

// tiebreak devuelve la columna que desempata registros con el mismo sort_order.
func (definition scopeDefinition) tiebreak() string {
	if definition.label == "" {
		return "name"
	}
	return definition.label
}

var scopes = map[Scope]scopeDefinition{
	CatalogCategories: {table: "app.catalog_categories"},
	CatalogProducts:   {table: "app.catalog_products", group: "category_id"},
	RestaurantAreas:   {table: "app.restaurant_areas"},
	KitchenStations:   {table: "app.kitchen_stations"},
	PaymentMethods:    {table: "app.payment_methods"},
	// Categorías de gasto de caja (etapa 7).
	CashExpenseCategories: {table: "app.cash_expense_categories"},
	// Motivos de descuento e indicaciones rápidas por categoría (etapa 7.1); las indicaciones se
	// ordenan dentro de su categoría y desempatan por su texto.
	DiscountReasons:    {table: "app.discount_reasons"},
	CategoryQuickNotes: {table: "app.catalog_category_quick_notes", group: "category_id", label: "text"},
}

// Result informa el orden final del registro colocado y los hermanos que cambiaron de número.
type Result struct {
	SortOrder int      `json:"sort_order"`
	Changes   []Change `json:"changes"`
}

// Place coloca id en su lista de hermanos dentro de la transacción del módulo, después de que
// el módulo guardó el registro (por lo que ya pertenece a la lista y a su grupo definitivo).
// joining indica que el registro se incorpora a la lista en esta transacción (es nuevo o cambió
// de grupo): su número provisional no cuenta para el orden actual.
//
// Bloquea a los hermanos con FOR UPDATE en el orden visible (sort_order, nombre, identificador),
// valida que after sea otro hermano, recalcula la secuencia y renumera en múltiplos de 10. Los
// hermanos cuyo número cambia incrementan su revisión: otra terminal que los tenga abiertos
// detectará el conflicto en lugar de restablecer un orden obsoleto. El propio registro no
// incrementa otra vez su revisión porque el módulo ya la incrementó (o la creó) en esta misma
// transacción. Los locks consultivos de cada módulo siguen siendo la primera barrera.
func Place(ctx context.Context, tx pgx.Tx, scope Scope, group string, id, after string, joining bool) (Result, error) {
	definition, known := scopes[scope]
	if !known {
		return Result{}, fmt.Errorf("ordering: alcance no registrado %d", scope)
	}
	query := "SELECT id::text,sort_order FROM " + definition.table
	arguments := []any{}
	if definition.group != "" {
		query += " WHERE " + definition.group + "=$1"
		arguments = append(arguments, group)
	}
	query += " ORDER BY sort_order," + definition.tiebreak() + ",id FOR UPDATE"
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return Result{}, err
	}
	siblings := []Sibling{}
	for rows.Next() {
		var sibling Sibling
		if err := rows.Scan(&sibling.ID, &sibling.SortOrder); err != nil {
			rows.Close()
			return Result{}, err
		}
		siblings = append(siblings, sibling)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	result := Result{Changes: []Change{}}
	present := false
	others := make([]Sibling, 0, len(siblings))
	for _, sibling := range siblings {
		if sibling.ID == id {
			present = true
			result.SortOrder = sibling.SortOrder
			if joining {
				continue
			}
		}
		others = append(others, sibling)
	}
	if !present {
		return Result{}, fmt.Errorf("ordering: el registro %s no está en su lista", id)
	}
	changes, err := Plan(others, id, after)
	if err != nil {
		return Result{}, err
	}
	for _, change := range changes {
		statement := "UPDATE " + definition.table + " SET sort_order=$1,revision=revision+1,updated_at=now() WHERE id=$2"
		if change.ID == id {
			statement = "UPDATE " + definition.table + " SET sort_order=$1 WHERE id=$2"
			result.SortOrder = change.To
		} else {
			result.Changes = append(result.Changes, change)
		}
		if _, err := tx.Exec(ctx, statement, change.To, change.ID); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}
