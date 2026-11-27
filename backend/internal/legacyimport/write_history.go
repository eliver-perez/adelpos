package legacyimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Escritura del historial con COPY de pgx (cientos de miles de filas): una tabla a la vez, renglón por renglón desde
// el modelo, sin materializar copias. Los nombres se copian (el historial no depende del catálogo vivo) y los
// identificadores nuevos salen de la correspondencia ya resuelta.

// historyNames son los nombres y los identificadores de lo importado, por clave anterior.
type historyNames struct {
	products     map[int32]PlannedProduct
	categories   map[string]PlannedCategory
	categoryIDs  map[string]int32
	complements  map[int32]PlannedComplement
	tables       map[int32]PlannedTable
	users        map[int32]PlannedUser
	registers    map[int32]string
	ids          IDMap
	discountType map[int32]string
}

func newHistoryNames(plan *Plan, ids IDMap) *historyNames {
	names := &historyNames{
		products:    map[int32]PlannedProduct{},
		categories:  map[string]PlannedCategory{},
		categoryIDs: map[string]int32{},
		complements: map[int32]PlannedComplement{},
		tables:      map[int32]PlannedTable{},
		users:       map[int32]PlannedUser{},
		registers:   map[int32]string{},
		ids:         ids,
	}
	for _, product := range plan.Catalog.Products {
		names.products[product.LegacyID] = product
	}
	for _, category := range plan.Catalog.Categories {
		names.categories[category.Key] = category
		names.categoryIDs[category.Key] = category.LegacyID
	}
	for _, complement := range plan.Catalog.Complements {
		names.complements[complement.LegacyID] = complement
	}
	for _, table := range plan.Catalog.Tables {
		names.tables[table.LegacyID] = table
	}
	// Etapa M1 (M1-D5): la mesa que se volvió una mesa divisible no existe en lo vivo (su identificador queda nulo),
	// pero sus ventas conservan su nombre y su área.
	for _, table := range plan.Catalog.AbsorbedTables {
		names.tables[table.LegacyID] = table
	}
	for _, user := range plan.Catalog.Users {
		names.users[user.LegacyID] = user
	}
	for _, register := range plan.Model.Registers {
		names.registers[register.ID] = cleanName(register.Name, 100, fmt.Sprintf("Caja %d", register.ID))
	}
	return names
}

// user es el identificador y el nombre visible de un usuario anterior (nulos si no existe o no se indicó).
func (names *historyNames) user(legacy NullInt) (*int32, pgtype.UUID, *string) {
	if !legacy.Valid {
		return nil, pgtype.UUID{}, nil
	}
	number := legacy.Value
	user, found := names.users[number]
	if !found {
		return &number, pgtype.UUID{}, nil
	}
	display := user.DisplayName
	return &number, names.ids.get(MapUser, user.Key), &display
}

// table es la mesa importada de una venta: su identificador, su nombre, su área (nulos para llevar o si no existe).
func (names *historyNames) table(legacy NullInt) (pgtype.UUID, *string, pgtype.UUID, *string) {
	if !legacy.Valid || legacy.Value == takeawayTableID {
		return pgtype.UUID{}, nil, pgtype.UUID{}, nil
	}
	table, found := names.tables[legacy.Value]
	if !found {
		return pgtype.UUID{}, nil, pgtype.UUID{}, nil
	}
	tableName, areaName := table.Name, table.AreaName
	return names.ids.get(MapTable, table.Key), &tableName, names.ids.get(MapArea, table.AreaKey), &areaName
}

func (names *historyNames) register(legacy NullInt) *string {
	if !legacy.Valid {
		return nil
	}
	name, found := names.registers[legacy.Value]
	if !found {
		return nil
	}
	return &name
}

// product son los datos copiados del producto de un renglón (con un nombre propio si no existía en el anterior).
type productCopy struct {
	id           pgtype.UUID
	code         *string
	name         string
	categoryKey  *int32
	categoryID   pgtype.UUID
	categoryName *string
}

func (names *historyNames) product(legacy int32) productCopy {
	product, found := names.products[legacy]
	if !found {
		return productCopy{name: fmt.Sprintf("Producto %d (no existe en el sistema anterior)", legacy)}
	}
	code := product.Code
	category := names.categories[product.CategoryKey]
	categoryName := category.Name
	copied := productCopy{
		id:           names.ids.get(MapProduct, product.Key),
		code:         &code,
		name:         product.Name,
		categoryID:   names.ids.get(MapCategory, product.CategoryKey),
		categoryName: &categoryName,
	}
	if legacyCategory, ok := names.categoryIDs[product.CategoryKey]; ok && product.CategoryKey != "sin-categoria" {
		copied.categoryKey = &legacyCategory
	}
	return copied
}

func ratePointer(rate int, has bool) *int32 {
	if !has {
		return nil
	}
	value := int32(rate)
	return &value
}

// copyCounter informa el avance de las filas escritas.
type copyCounter struct {
	written  int64
	total    int64
	progress func(written, total int64)
}

func (counter *copyCounter) row() {
	counter.written++
	if counter.progress != nil && counter.written%progressEvery == 0 {
		counter.progress(counter.written, counter.total)
	}
}

// writeHistory escribe todas las tablas del historial del lote y devuelve las filas escritas por tabla.
func writeHistory(ctx context.Context, tx pgx.Tx, plan *Plan, ids IDMap, batch pgtype.UUID, progress func(written, total int64)) (map[string]int64, error) {
	model, history := plan.Model, plan.History
	names := newHistoryNames(plan, ids)
	ledgerRows := int64(0)
	_ = plan.Ledger(func(LedgerEntry) error {
		ledgerRows++
		return nil
	})
	counter := &copyCounter{progress: progress}
	counter.total = int64(len(model.Sales)+len(model.Lines)+len(model.LineComplements)+len(model.QuantityChanges)+
		len(model.Discounts)+len(model.Cancellations)+len(model.Payments)+len(model.Shifts)+len(model.Withdrawals)+
		len(model.Expenses)+
		len(history.Prices)) + ledgerRows
	written := map[string]int64{}
	copyTable := func(table string, columns []string, rows int, next func(int) ([]any, error)) error {
		count, err := tx.CopyFrom(ctx, pgx.Identifier{"app", table}, columns, pgx.CopyFromSlice(rows, func(index int) ([]any, error) {
			counter.row()
			return next(index)
		}))
		written[table] = count
		return err
	}
	if err := copyTable("legacy_sales", saleColumns, len(model.Sales), func(index int) ([]any, error) {
		return saleRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_sale_lines", lineColumns, len(model.Lines), func(index int) ([]any, error) {
		return lineRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_line_complements", complementColumns, len(model.LineComplements), func(index int) ([]any, error) {
		return complementRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_quantity_changes", changeColumns, len(model.QuantityChanges), func(index int) ([]any, error) {
		return changeRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_discounts", discountColumns, len(model.Discounts), func(index int) ([]any, error) {
		discount := model.Discounts[index]
		state := history.Discounts[index]
		return []any{batch, discount.Position, discount.Sale, discount.Type.Pointer(), textPointer(state.TypeName, state.HasType),
			discount.Description, discount.Amount.Numeric(), state.Flags.array()}, nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_cancellations", cancellationColumns, len(model.Cancellations), func(index int) ([]any, error) {
		cancellation := model.Cancellations[index]
		legacyUser, userID, userName := names.user(cancellation.AuthorizedBy)
		return []any{batch, cancellation.ID, cancellation.Sale, legacyUser, userID, userName, cancellation.CancelledAt}, nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_payments", paymentColumns, len(model.Payments), func(index int) ([]any, error) {
		return paymentRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_cash_shifts", shiftColumns, len(model.Shifts), func(index int) ([]any, error) {
		return shiftRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_cash_withdrawals", withdrawalColumns, len(model.Withdrawals), func(index int) ([]any, error) {
		withdrawal := model.Withdrawals[index]
		legacyUser, userID, userName := names.user(withdrawal.AuthorizedBy)
		return []any{batch, withdrawal.ID, withdrawal.Shift.Pointer(), withdrawal.Amount.Numeric(), withdrawal.Reason,
			timePointer(withdrawal.WithdrawnAt), legacyUser, userID, userName, history.Withdrawals[index].Flags.array()}, nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_cash_expenses", expenseColumns, len(model.Expenses), func(index int) ([]any, error) {
		expense := model.Expenses[index]
		legacyUser, userID, userName := names.user(expense.AuthorizedBy)
		return []any{batch, expense.Position, expense.Shift.Pointer(), expense.Amount.Numeric(), timePointer(expense.SpentAt),
			legacyUser, userID, userName, []string{}}, nil
	}); err != nil {
		return nil, err
	}
	if err := copyTable("legacy_price_history", priceColumns, len(history.Prices), func(index int) ([]any, error) {
		return priceRow(model, history, names, batch, index), nil
	}); err != nil {
		return nil, err
	}
	iterator := plan.LedgerIterator()
	count, err := tx.CopyFrom(ctx, pgx.Identifier{"app", "legacy_sales_recognitions"}, ledgerColumns,
		pgx.CopyFromFunc(func() ([]any, error) {
			entry, ok := iterator.Next()
			if !ok {
				return nil, nil
			}
			counter.row()
			return ledgerRow(model, history, names, batch, entry), nil
		}))
	written["legacy_sales_recognitions"] = count
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress(counter.written, counter.total)
	}
	return written, nil
}

var saleColumns = []string{"batch_id", "legacy_id", "folio", "status", "opened_at", "recognized_at", "last_payment_at",
	"cancelled_at", "mode", "legacy_table_id", "table_id", "table_name", "area_id", "area_name", "legacy_register_id",
	"register_name", "legacy_cashier_id", "cashier_user_id", "cashier_name", "legacy_shift_id", "account_name", "subtotal",
	"tax", "total", "amount_paid", "paid_flag", "in_table_flag", "invoiced_flag", "cancelled_flag", "charge_concept",
	"charge_amount", "discount_amount", "amount_due", "lines_amount", "lines_count", "payments_amount", "payments_count",
	"tax_estimated", "flags", "note"}

func saleRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	sale := model.Sales[index]
	state := history.Sales[index]
	tableID, tableName, areaID, areaName := names.table(sale.Table)
	legacyCashier, cashierID, cashierName := names.user(sale.Cashier)
	return []any{batch, sale.ID, sale.Folio, state.Status, sale.OpenedAt, timePointer(state.RecognizedAt),
		timePointer(state.LastPaymentAt), timePointer(state.CancelledAt), state.Mode, sale.Table.Pointer(), tableID, tableName,
		areaID, areaName, sale.Register.Pointer(), names.register(sale.Register), legacyCashier, cashierID, cashierName,
		sale.Shift.Pointer(), sale.AccountName, sale.Subtotal.Numeric(), state.Tax.Numeric(), sale.Total.Numeric(),
		sale.AmountPaid.Numeric(), sale.Paid.Pointer(), sale.InTable.Pointer(), sale.Invoiced.Pointer(),
		sale.Cancelled.Pointer(), sale.Charge, sale.ChargeTotal.Numeric(), state.DiscountTotal.Numeric(), state.Due.Numeric(),
		state.LinesAmount.Numeric(), int32(state.LinesCount), state.PaymentsAmount.Numeric(), int32(state.PaymentsCount),
		state.TaxEstimated, state.Flags.array(), saleNote(sale, state)}
}

// saleNote explica las marcas de dinero de una venta con sus importes (queda en la base y en el PDF).
func saleNote(sale Sale, state SalePlan) string {
	parts := []string{}
	if state.Flags.has(FlagTotalMismatch) {
		parts = append(parts, fmt.Sprintf("Total guardado %s; renglones %s; diferencia %s.", sale.Total, state.LinesAmount,
			sale.Total-state.LinesAmount))
	}
	if state.Flags.has(FlagTaxMismatch) {
		parts = append(parts, fmt.Sprintf("IVA guardado %s; IVA de los renglones %s.", state.Tax, state.LinesTax))
	}
	if state.Flags.has(FlagOverpaid) || state.Flags.has(FlagUnderpaid) {
		parts = append(parts, fmt.Sprintf("Se debía %s y se pagó %s.", state.Due, state.PaymentsAmount))
	}
	if state.Flags.has(FlagTaxEstimated) {
		parts = append(parts, "IVA estimado con la tasa del producto vigente en la fecha de la venta.")
	}
	return strings.Join(parts, " ")
}

var lineColumns = []string{"batch_id", "legacy_id", "legacy_sale_id", "legacy_product_id", "product_id", "product_code",
	"product_name", "legacy_category_id", "category_id", "category_name", "quantity", "original_quantity", "subtotal", "tax",
	"total", "tax_rate", "tax_estimated", "complements_count", "complements_subtotal", "complements_tax",
	"complements_total", "notes", "flags"}

func lineRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	line := model.Lines[index]
	state := history.Lines[index]
	product := names.product(line.Product)
	return []any{batch, line.ID, line.Sale, line.Product, product.id, product.code, product.name, product.categoryKey,
		product.categoryID, product.categoryName, line.Quantity.Numeric(), line.OriginalQuantity.Numeric(),
		line.Subtotal.Numeric(), state.Tax.Numeric(), line.Total.Numeric(), ratePointer(state.TaxRate, state.HasRate),
		state.TaxEstimated, int32(state.ComplementsCount), state.ComplementsSubtotal.Numeric(), state.ComplementsTax.Numeric(),
		state.ComplementsTotal.Numeric(), line.Notes, state.Flags.array()}
}

var complementColumns = []string{"batch_id", "position", "legacy_line_id", "legacy_sale_id", "legacy_complement_id",
	"complement_id", "complement_name", "included", "requested", "subtotal", "tax", "total", "tax_estimated", "flags"}

func complementRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	complement := model.LineComplements[index]
	state := history.Complements[index]
	var sale *int32
	if state.SaleIndex >= 0 {
		id := model.Sales[state.SaleIndex].ID
		sale = &id
	}
	var id pgtype.UUID
	name := fmt.Sprintf("Complemento %d (no existe en el sistema anterior)", complement.Complement)
	if planned, found := names.complements[complement.Complement]; found {
		id = names.ids.get(MapComplement, planned.Key)
		name = planned.Name
	}
	return []any{batch, complement.Position, complement.Line, sale, complement.Complement, id, name,
		complement.Included.Numeric(), complement.Requested.Numeric(), complement.Subtotal.Numeric(), state.Tax.Numeric(),
		complement.Total.Numeric(), state.TaxEstimated, state.Flags.array()}
}

var changeColumns = []string{"batch_id", "position", "legacy_line_id", "legacy_sale_id", "original_quantity",
	"new_quantity", "legacy_authorized_by", "authorized_user_id", "authorized_name", "changed_at"}

func changeRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	change := model.QuantityChanges[index]
	var sale *int32
	if line := history.line(change.Line); line >= 0 {
		id := model.Lines[line].Sale
		sale = &id
	}
	legacyUser, userID, userName := names.user(change.AuthorizedBy)
	return []any{batch, change.Position, change.Line, sale, change.Original.Numeric(), change.New.Numeric(), legacyUser,
		userID, userName, timePointer(change.ChangedAt)}
}

var discountColumns = []string{"batch_id", "position", "legacy_sale_id", "legacy_type_id", "type_name", "description",
	"amount", "flags"}

var cancellationColumns = []string{"batch_id", "legacy_id", "legacy_sale_id", "legacy_authorized_by", "authorized_user_id",
	"authorized_name", "cancelled_at"}

var paymentColumns = []string{"batch_id", "legacy_id", "legacy_sale_id", "legacy_method_id", "method", "method_name",
	"reference", "amount", "legacy_received_by", "received_user_id", "received_name", "paid_at", "legacy_shift_id",
	"sale_status", "flags"}

func paymentRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	payment := model.Payments[index]
	state := history.Payments[index]
	legacyUser, userID, userName := names.user(payment.ReceivedBy)
	status := SaleOpen
	if state.SaleIndex >= 0 {
		status = history.Sales[state.SaleIndex].Status
	}
	return []any{batch, payment.ID, payment.Sale, payment.Type.Pointer(), state.Method, state.MethodName, payment.Reference,
		payment.Amount.Or(0).Numeric(), legacyUser, userID, userName, payment.PaidAt, payment.Shift.Pointer(), status,
		state.Flags.array()}
}

var shiftColumns = []string{"batch_id", "legacy_id", "legacy_register_id", "register_name", "legacy_cashier_id",
	"cashier_user_id", "cashier_name", "status", "legacy_status", "opening_cash", "closing_cash", "sales_amount",
	"withdrawals_amount", "difference", "legacy_authorized_by", "authorized_user_id", "authorized_name", "opened_at",
	"closed_at", "flags"}

func shiftRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	shift := model.Shifts[index]
	state := history.Shifts[index]
	legacyCashier, cashierID, cashierName := names.user(shift.Cashier)
	legacyAuthorized, authorizedID, authorizedName := names.user(shift.AuthorizedBy)
	return []any{batch, shift.ID, shift.Register.Pointer(), names.register(shift.Register), legacyCashier, cashierID,
		cashierName, state.Status, textPointer(shift.Status, shift.StatusSet), shift.OpeningCash.Numeric(),
		shift.ClosingCash.Numeric(), shift.Sales.Numeric(), shift.Withdrawals.Numeric(), shift.Difference.Numeric(),
		legacyAuthorized, authorizedID, authorizedName, timePointer(state.OpenedAt), timePointer(state.ClosedAt),
		state.Flags.array()}
}

var withdrawalColumns = []string{"batch_id", "legacy_id", "legacy_shift_id", "amount", "reason", "withdrawn_at",
	"legacy_authorized_by", "authorized_user_id", "authorized_name", "flags"}

var expenseColumns = []string{"batch_id", "position", "legacy_shift_id", "amount", "spent_at", "legacy_authorized_by",
	"authorized_user_id", "authorized_name", "flags"}

var priceColumns = []string{"batch_id", "position", "item_kind", "legacy_item_id", "item_id", "item_name", "starts_at",
	"valid_until", "subtotal", "tax", "total", "tax_rate", "flags"}

func priceRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, index int) []any {
	state := history.Prices[index]
	var price Price
	var id pgtype.UUID
	var name string
	if state.Kind == "product" {
		price = model.ProductPrices[state.Index]
		product := names.product(price.Item)
		id, name = product.id, product.name
	} else {
		price = model.ComplementPrices[state.Index]
		name = fmt.Sprintf("Complemento %d (no existe en el sistema anterior)", price.Item)
		if planned, found := names.complements[price.Item]; found {
			id, name = names.ids.get(MapComplement, planned.Key), planned.Name
		}
	}
	return []any{batch, int32(index + 1), state.Kind, price.Item, id, name, price.StartsAt, timePointer(price.ValidUntil),
		price.Subtotal.Numeric(), price.Tax.Or(0).Numeric(), price.Total.Numeric(), int32(state.TaxRate), state.Flags.array()}
}

var ledgerColumns = []string{"batch_id", "entry_kind", "legacy_sale_id", "sale_folio", "legacy_line_id", "recognized_at",
	"opened_at", "mode", "legacy_table_id", "table_id", "table_name", "area_id", "area_name", "legacy_register_id",
	"register_name", "legacy_shift_id", "legacy_cashier_id", "responsible_user_id", "responsible_name", "legacy_product_id",
	"product_id", "product_code", "product_name", "legacy_category_id", "category_id", "category_name", "concept",
	"tax_rate", "tax_estimated", "quantity", "gross", "discount", "charge", "net", "base", "tax"}

func ledgerRow(model *Model, history *HistoryPlan, names *historyNames, batch pgtype.UUID, entry LedgerEntry) []any {
	sale := model.Sales[entry.SaleIndex]
	state := history.Sales[entry.SaleIndex]
	tableID, tableName, areaID, areaName := names.table(sale.Table)
	legacyCashier, cashierID, cashierName := names.user(sale.Cashier)
	var legacyLine, legacyProduct *int32
	product := productCopy{}
	var productName *string
	if entry.Kind == LedgerLine {
		line := model.Lines[entry.LineIndex]
		lineID, productID := line.ID, line.Product
		legacyLine, legacyProduct = &lineID, &productID
		product = names.product(line.Product)
		productName = &product.name
	}
	var concept *string
	if entry.Concept != "" {
		text := entry.Concept
		concept = &text
	}
	return []any{batch, entry.Kind, sale.ID, sale.Folio, legacyLine, state.RecognizedAt, sale.OpenedAt, state.Mode,
		sale.Table.Pointer(), tableID, tableName, areaID, areaName, sale.Register.Pointer(), names.register(sale.Register),
		sale.Shift.Pointer(), legacyCashier, cashierID, cashierName, legacyProduct, product.id, product.code, productName,
		product.categoryKey, product.categoryID, product.categoryName, concept, ratePointer(entry.TaxRate, entry.HasRate),
		entry.TaxEstimated, entry.Quantity.Numeric(), entry.Gross.Numeric(), entry.Discount.Numeric(),
		entry.Charge.Numeric(), entry.Net.Numeric(), entry.Base.Numeric(), entry.Tax.Numeric()}
}
