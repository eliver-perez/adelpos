package operations

// tableService pone una mesa fuera de servicio o la habilita desde la operación (etapa 7.1, decisión
// 9): POST restaurant/tables/{id}/service con {operation_id, revision, out_of_service}. Requiere
// restaurant.tables.manage (comprobado en Handle antes de cualquier escritura). Una mesa con cuentas
// abiertas no se pone fuera de servicio; habilitarla siempre procede. La auditoría se escribe aquí y
// el evento restaurant.tables.changed lo publica Handle, ambos en la misma transacción.
func (handler *service) tableService(id string) (any, error) {
	var input struct {
		Mutation
		OutOfService *bool `json:"out_of_service"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.OutOfService == nil {
		return nil, invalid("Indica si la mesa queda fuera de servicio o habilitada.")
	}
	old, err := readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", id)
	if err != nil {
		return nil, err
	}
	if old.Revision != input.Revision {
		return nil, conflict()
	}
	if !old.Active {
		return nil, invalid("La mesa está desactivada. Actívala en Áreas y distribución antes de cambiar su servicio.")
	}
	if old.OutOfService == *input.OutOfService {
		if old.OutOfService {
			return nil, invalid("La mesa ya está fuera de servicio.")
		}
		return nil, invalid("La mesa ya está habilitada.")
	}
	if *input.OutOfService && old.AccountCount > 0 {
		return nil, tableHasOpenAccounts()
	}
	result, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_tables SET out_of_service=$1,
        revision=revision+1,updated_at=now() WHERE id=$2 AND revision=$3`, *input.OutOfService, id, input.Revision)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() != 1 {
		return nil, conflict()
	}
	saved, err := readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", id)
	if err != nil {
		return nil, err
	}
	action := "restaurant.table.enabled"
	if saved.OutOfService {
		action = "restaurant.table.out_of_service"
	}
	return saved, handler.audit(handler.ctx, handler.tx, action, "table", &id, old, saved)
}
