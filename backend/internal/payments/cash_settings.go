package payments

import (
	"errors"
	"strings"

	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/ordering"
)

// Configuración de caja: catálogo de denominaciones y categorías de gasto. Leer el catálogo
// requiere payments.read (lo usan la apertura, el arqueo y los gastos); modificarlo requiere
// cash.settings.manage y queda auditado. Nada se borra: una denominación o categoría usada se
// desactiva y los registros históricos la conservan.

const denominationColumns = `SELECT id,currency,value::text,kind,active,revision,updated_at FROM app.cash_denominations`

// listDenominations devuelve el catálogo ordenado por moneda, billetes antes que monedas y valor
// descendente. El valor se califica con la tabla: sin calificar, ORDER BY usaría la columna de
// salida value::text y ordenaría como texto (500, 50, 200, 20…).
func (handler *service) listDenominations() ([]Denomination, error) {
	rows, err := handler.tx.Query(handler.ctx, denominationColumns+" ORDER BY currency,kind,cash_denominations.value DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Denomination{}
	for rows.Next() {
		var item Denomination
		if err := rows.Scan(&item.ID, &item.Currency, &item.Value, &item.Kind, &item.Active, &item.Revision, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (handler *service) readDenomination(id string) (*Denomination, error) {
	var item Denomination
	err := handler.tx.QueryRow(handler.ctx, denominationColumns+" WHERE id=$1", id).Scan(
		&item.ID, &item.Currency, &item.Value, &item.Kind, &item.Active, &item.Revision, &item.UpdatedAt)
	if isNoRows(err) {
		return nil, notFound("La denominación no existe.")
	}
	return &item, err
}

const categoryColumns = `SELECT id,name,active,sort_order,revision,updated_at FROM app.cash_expense_categories`

func (handler *service) listExpenseCategories() ([]ExpenseCategory, error) {
	rows, err := handler.tx.Query(handler.ctx, categoryColumns+" ORDER BY sort_order,name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ExpenseCategory{}
	for rows.Next() {
		var item ExpenseCategory
		if err := rows.Scan(&item.ID, &item.Name, &item.Active, &item.SortOrder, &item.Revision, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (handler *service) readExpenseCategory(id string) (*ExpenseCategory, error) {
	var item ExpenseCategory
	err := handler.tx.QueryRow(handler.ctx, categoryColumns+" WHERE id=$1", id).Scan(
		&item.ID, &item.Name, &item.Active, &item.SortOrder, &item.Revision, &item.UpdatedAt)
	if isNoRows(err) {
		return nil, notFound("La categoría de gasto no existe.")
	}
	return &item, err
}

// readCashCatalog reúne lo que necesitan los formularios de caja: denominaciones, categorías de
// gasto y los tipos cerrados de ingreso y de retiro.
func (handler *service) readCashCatalog(_ []string) (any, error) {
	denominations, err := handler.listDenominations()
	if err != nil {
		return nil, err
	}
	categories, err := handler.listExpenseCategories()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"denominations":      denominations,
		"expense_categories": categories,
		"income_types":       incomeTypes,
		"withdrawal_types":   withdrawalTypes,
	}, nil
}

// createDenomination agrega un billete o una moneda. Moneda, valor y tipo no cambian después.
func (handler *service) createDenomination(_ []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Currency    string `json:"currency"`
		Value       string `json:"value"`
		Kind        string `json:"kind"`
		Active      bool   `json:"active"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.Currency != currencyMXN && input.Currency != currencyUSD {
		return nil, invalid("La moneda debe ser pesos (MXN) o dólares (USD).")
	}
	if input.Kind != "bill" && input.Kind != "coin" {
		return nil, invalid("Indica si la denominación es billete o moneda.")
	}
	value, err := parseAmount(input.Value, "valor de la denominación")
	if err != nil {
		return nil, err
	}
	if value.Sign() <= 0 {
		return nil, invalid("El valor de la denominación debe ser mayor que cero.")
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_denominations(id,currency,value,kind,active) VALUES ($1,$2,$3,$4,$5)`,
		id, input.Currency, money(value), input.Kind, input.Active)
	if duplicateName(err) {
		return nil, conflict("DENOMINATION_EXISTS", "La denominación ya existe en el catálogo.")
	}
	if err != nil {
		return nil, err
	}
	saved, err := handler.readDenomination(id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, &id, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.denomination_created", "cash_denomination", &id, nil, saved)
}

// updateDenomination activa o desactiva una denominación con revisión optimista.
func (handler *service) updateDenomination(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Revision    int    `json:"revision"`
		Active      bool   `json:"active"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	previous, err := handler.readDenomination(parts[2])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La denominación cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_denominations SET active=$2,revision=revision+1,updated_at=now()
        WHERE id=$1`, previous.ID, input.Active); err != nil {
		return nil, err
	}
	saved, err := handler.readDenomination(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, &saved.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.denomination_updated", "cash_denomination", &saved.ID, previous, saved)
}

// categoryInput es la captura de una categoría de gasto; la posición sigue el orden visual por
// posiciones del sistema (internal/ordering): nunca se captura un número de orden.
type categoryInput struct {
	OperationID string             `json:"operation_id"`
	Revision    int                `json:"revision"`
	Name        string             `json:"name"`
	Active      bool               `json:"active"`
	Position    *ordering.Position `json:"position"`
}

func (input *categoryInput) validate() error {
	input.Name = strings.TrimSpace(input.Name)
	if textLength(input.Name) < 1 || textLength(input.Name) > 80 {
		return invalid("El nombre de la categoría debe tener entre 1 y 80 caracteres.")
	}
	return nil
}

// placeCategory aplica la posición elegida dentro de la misma transacción y audita la renumeración.
func (handler *service) placeCategory(input categoryInput, id string, joining bool) error {
	if input.Position == nil {
		return nil
	}
	after, err := ordering.Validate(input.Position, id)
	if err != nil {
		return invalid(err.Error())
	}
	result, err := ordering.Place(handler.ctx, handler.tx, ordering.CashExpenseCategories, "", id, after, joining)
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
	return handler.audit(handler.ctx, handler.tx, "cash.expense_categories_reordered", "cash_expense_category", &id, nil, map[string]any{
		"placed_id": id, "after": after, "sort_order": result.SortOrder, "changes": result.Changes,
	})
}

// createExpenseCategory agrega una categoría al final de la lista (o en la posición indicada).
func (handler *service) createExpenseCategory(_ []string) (any, error) {
	var input categoryInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	if input.Position != nil {
		if _, err := ordering.Validate(input.Position, ""); err != nil {
			return nil, invalid(err.Error())
		}
	}
	id := newID()
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_expense_categories(id,name,active,sort_order)
        VALUES ($1,$2,$3,(SELECT coalesce(max(sort_order),0)+10 FROM app.cash_expense_categories))`, id, input.Name, input.Active)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe una categoría de gasto con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeCategory(input, id, true); err != nil {
		return nil, err
	}
	saved, err := handler.readExpenseCategory(id)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, &id, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.expense_category_created", "cash_expense_category", &id, nil, saved)
}

// updateExpenseCategory renombra, activa o desactiva una categoría. Los gastos registrados
// conservan su categoría aunque se desactive.
func (handler *service) updateExpenseCategory(parts []string) (any, error) {
	var input categoryInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := input.validate(); err != nil {
		return nil, err
	}
	previous, err := handler.readExpenseCategory(parts[2])
	if err != nil {
		return nil, err
	}
	if previous.Revision != input.Revision {
		return nil, conflict("REVISION_CONFLICT", "La categoría cambió en otra terminal. Actualiza y vuelve a guardar.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.cash_expense_categories SET name=$2,active=$3,revision=revision+1,updated_at=now()
        WHERE id=$1`, previous.ID, input.Name, input.Active)
	if duplicateName(err) {
		return nil, conflict("DUPLICATE_NAME", "Ya existe una categoría de gasto con ese nombre.")
	}
	if err != nil {
		return nil, err
	}
	if err := handler.placeCategory(input, previous.ID, false); err != nil {
		return nil, err
	}
	saved, err := handler.readExpenseCategory(previous.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.settings.changed", cashScope, &saved.ID, nil); err != nil {
		return nil, err
	}
	return saved, handler.audit(handler.ctx, handler.tx, "cash.expense_category_updated", "cash_expense_category", &saved.ID, previous, saved)
}
