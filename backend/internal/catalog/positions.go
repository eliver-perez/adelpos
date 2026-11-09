package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/ordering"
)

// placement es una posición validada, pendiente de aplicar después de guardar el registro.
type placement struct {
	scope    ordering.Scope
	group    string
	id       string
	after    string
	resource string
	// joining indica que el registro se incorpora a la lista: es nuevo o cambió de categoría.
	joining bool
}

// preparePlacement valida la posición recibida antes de cualquier escritura. Cuando el cuerpo
// trae posición, el sort_order enviado no se usa: el registro conserva su número actual (o uno
// provisional si es nuevo) y Place lo recalcula dentro de la misma transacción. Sin posición se
// conserva el comportamiento anterior (se guarda el sort_order recibido).
func preparePlacement(document any, previous any) (*placement, error) {
	switch value := document.(type) {
	case *Category:
		if value.Position == nil {
			return nil, nil
		}
		after, err := ordering.Validate(value.Position, value.ID)
		if err != nil {
			return nil, invalid(err.Error())
		}
		value.Position = nil
		value.SortOrder = 0
		old, existing := previous.(*Category)
		if existing {
			value.SortOrder = old.SortOrder
		}
		return &placement{
			scope:    ordering.CatalogCategories,
			id:       value.ID,
			after:    after,
			resource: "categories",
			joining:  !existing,
		}, nil
	case *Product:
		if value.Position == nil {
			return nil, nil
		}
		after, err := ordering.Validate(value.Position, value.ID)
		if err != nil {
			return nil, invalid(err.Error())
		}
		value.Position = nil
		value.SortOrder = 0
		old, existing := previous.(*Product)
		if existing {
			value.SortOrder = old.SortOrder
		}
		// Los productos se ordenan dentro de su categoría: el hermano debe pertenecer a la categoría
		// con la que se guarda el producto, aunque esta acabe de cambiar.
		return &placement{
			scope:    ordering.CatalogProducts,
			group:    value.CategoryID,
			id:       value.ID,
			after:    after,
			resource: "products",
			joining:  !existing || old.CategoryID != value.CategoryID,
		}, nil
	}
	return nil, nil
}

// applyPlacement coloca el registro ya guardado y audita la renumeración de sus hermanos en la
// misma transacción. El cambio del propio registro queda en su auditoría de creación o edición.
func applyPlacement(ctx context.Context, tx pgx.Tx, request *placement, audit Audit) error {
	if request == nil {
		return nil
	}
	result, err := ordering.Place(

		ctx,
		tx,
		request.scope,
		request.group,
		request.id,
		request.after,
		request.joining,
	)
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
	return audit(

		ctx,
		tx,
		"catalog."+request.resource+".reordered",
		request.resource,
		&request.id,
		nil,
		map[string]any{
			"placed_id":  request.id,
			"after":      request.after,
			"sort_order": result.SortOrder,
			"changes":    result.Changes,
		},
	)
}
