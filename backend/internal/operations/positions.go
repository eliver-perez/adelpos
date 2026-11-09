package operations

import (
	"errors"

	"local/ad-el-pos/server/internal/ordering"
)

// validatePosition revisa la posición recibida antes de escribir y la traduce al error público
// del módulo.
func validatePosition(position *ordering.Position, id string) (string, error) {
	after, err := ordering.Validate(position, id)
	if err != nil {
		return "", invalid(err.Error())
	}
	return after, nil
}

// place coloca el registro ya guardado entre sus hermanos y audita, en la misma transacción, la
// renumeración de los hermanos que cambiaron de orden.
func (handler *service) place(
	scope ordering.Scope,
	group, id, after string,
	joining bool,
	action, entity string,
) error {
	result, err := ordering.Place(

		handler.ctx,
		handler.tx,
		scope,
		group,
		id,
		after,
		joining,
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
	return handler.audit(handler.ctx, handler.tx, action, entity, &id, nil, map[string]any{
		"placed_id":  id,
		"after":      after,
		"sort_order": result.SortOrder,
		"changes":    result.Changes,
	})
}
