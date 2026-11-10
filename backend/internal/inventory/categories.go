package inventory

import (
	"errors"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/ordering"
)

// Categorías de insumos: opcionales, con nombre único sin distinguir mayúsculas y orden visual por
// posiciones (internal/ordering). Se desactivan, nunca se borran; los insumos conservan su
// categoría aunque se desactive, pero no se puede asignar una categoría inactiva.

type Category struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sort_order"`
	ItemCount int       `json:"item_count"`
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

const categoryColumns = `SELECT c.id,c.name,c.active,c.sort_order,c.revision,c.updated_at,
        (SELECT count(*) FROM app.inventory_items i WHERE i.category_id=c.id) AS item_count
        FROM app.inventory_categories c`

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	var category Category
	err := row.Scan(&category.ID, &category.Name, &category.Active, &category.SortOrder, &category.Revision,
		&category.UpdatedAt, &category.ItemCount)
	if err != nil {
		return nil, err
	}
	return &category, nil
}

func (handler *service) readCategory(id string) (*Category, error) {
	category, err := scanCategory(handler.tx.QueryRow(handler.ctx, categoryColumns+" WHERE c.id=$1", id))
	if isNoRows(err) {
		return nil, notFound("La categoría de insumos no existe.")
	}
	return category, err
}

// listCategories devuelve todas las categorías en su orden visual, activas e inactivas.
func (handler *service) listCategories(_ []string) (any, error) {
	rows, err := handler.tx.Query(handler.ctx, categoryColumns+" ORDER BY c.sort_order,c.name,c.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := []Category{}
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, *category)
	}
	return categories, rows.Err()
}

type categoryInput struct {
	OperationID string             `json:"operation_id"`
	Revision    int                `json:"revision"`
	Name        string             `json:"name"`
	Active      bool               `json:"active"`
	Position    *ordering.Position `json:"position"`
}

func (input *categoryInput) validate(id string) error {
	if err := validateName(&input.Name, "El nombre de la categoría", 60); err != nil {
		return err
	}
	if input.Position != nil {
		if _, err := ordering.Validate(input.Position, id); err != nil {
			return invalid(err.Error())
		}
	}
	return nil
}

func categoryConflict(err error) error {
	if uniqueViolation(err) {
		return conflict("DUPLICATE_NAME", "Ya existe una categoría de insumos con ese nombre.")
	}
	return err
}

// placeCategory aplica la posición elegida en la misma transacción y audita la renumeración.
func (handler *service) placeCategory(input categoryInput, id string, joining bool) error {
	if input.Position == nil {
		return nil
	}
	after, err := ordering.Validate(input.Position, id)
	if err != nil {
		return invalid(err.Error())
	}
	result, err := ordering.Place(handler.ctx, handler.tx, ordering.InventoryCategories, "", id, strings.ToLower(after), joining)
	var problem *ordering.Error
	if errors.As(err, &problem) {
		return invalid(problem.Message)
	}
	if err != nil {
		return err
	}
	if len(result.Changes) == 0 {
		return nil
	}
	return handler.audit(handler.ctx, handler.tx, "inventory.categories_reordered", "inventory_category", &id, nil, map[string]any{
		"placed_id":  id,
		"after":      after,
		"sort_order": result.SortOrder,
		"changes":    result.Changes,
	})
}

// createCategory agrega una categoría al final de la lista (o en la posición indicada).
func (handler *service) createCategory(_ []string) (any, error) {
	var input categoryInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(""); err != nil {
		return nil, err
	}
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_categories(id,name,active,sort_order)
        VALUES ($1,$2,$3,(SELECT coalesce(max(sort_order),0)+10 FROM app.inventory_categories))`, id, input.Name, input.Active)
	if err != nil {
		return nil, categoryConflict(err)
	}
	if err := handler.placeCategory(input, id, true); err != nil {
		return nil, err
	}
	saved, err := handler.readCategory(id)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &id); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.category_created", "inventory_category", &id, nil, saved)
}

// updateCategory renombra, activa o desactiva una categoría o cambia su posición.
func (handler *service) updateCategory(parts []string) (any, error) {
	var input categoryInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(parts[1]); err != nil {
		return nil, err
	}
	previous, err := handler.readCategory(parts[1])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, revisionConflict("La categoría")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_categories SET name=$2,active=$3,revision=revision+1,
        updated_at=now() WHERE id=$1`, previous.ID, input.Name, input.Active)
	if err != nil {
		return nil, categoryConflict(err)
	}
	if err := handler.placeCategory(input, previous.ID, false); err != nil {
		return nil, err
	}
	saved, err := handler.readCategory(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.items.changed", &saved.ID); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "inventory.category_updated", "inventory_category", &saved.ID, previous, saved)
}
