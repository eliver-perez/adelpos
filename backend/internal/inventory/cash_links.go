package inventory

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/moneytext"
)

// Vínculos opcionales de una recepción con gastos de caja (§20 de la especificación). Reglas:
//   - una compra no genera un gasto y un gasto no genera una compra: el vínculo es explícito
//     (inventory.purchases.link_cash) y vive en su propia tabla; nunca modifica cash_operations, el
//     turno ni su cierre, aunque el turno ya esté cerrado;
//   - leer el gasto exige cash.movements.read y las reglas de caja de payments (readableShift): un
//     gasto de otra caja exige cash.shifts.read_all y uno de un turno cerrado cash.history.read;
//   - solo se vincula un gasto (kind expense) existente, en pesos (las compras son en pesos: un gasto
//     en dólares se rechaza) y sin corrección; se admiten vínculos parciales, sin duplicar egresos:
//     Σ vínculos vigentes del gasto ≤ importe del gasto y Σ de la recepción ≤ total de la recepción;
//   - las rutas toman 7820148305 compartido antes que 7820148308, de modo que ninguna corrección de
//     caja se confirma entre la validación y el registro del vínculo;
//   - desvincular es un registro con motivo (unlinked_*), no un borrado;
//   - un gasto corregido o regularizado después del vínculo se detecta en cada lectura (estado del
//     vínculo y cash_link_attention de la recepción) para la alerta de la fase D.

var cashLinkStatusLabels = map[string]string{
	"active":              "Vigente",
	"expense_corrected":   "Gasto corregido después del vínculo",
	"expense_regularized": "Gasto regularizado después del vínculo",
	"unlinked":            "Desvinculado",
}

var cashExpenseStatusLabels = map[string]string{
	"recorded":    "Registrado",
	"corrected":   "Corregido",
	"regularized": "Regularizado",
}

var cashUnavailableLabels = map[string]string{
	"not_expense":  "No es un gasto",
	"currency":     "Gasto en dólares: las compras se vinculan solo con gastos en pesos",
	"corrected":    "El gasto fue corregido o regularizado",
	"fully_linked": "El gasto ya está vinculado por completo",
}

// CashExpense es un gasto de caja tal como lo ve el inventario (solo lectura).
type CashExpense struct {
	ID                string     `json:"id"`
	ShiftID           string     `json:"shift_id"`
	RegisterID        string     `json:"register_id"`
	RegisterName      string     `json:"register_name"`
	ShiftState        string     `json:"shift_state"`
	ShiftBusinessDate *string    `json:"shift_business_date"`
	Currency          string     `json:"currency"`
	Amount            string     `json:"amount"`
	Concept           string     `json:"concept"`
	CategoryName      *string    `json:"category_name"`
	Reference         string     `json:"reference"`
	CreatedAt         time.Time  `json:"created_at"`
	ActorName         string     `json:"actor_name"`
	Status            string     `json:"status"`
	StatusLabel       string     `json:"status_label"`
	CorrectedAt       *time.Time `json:"corrected_at"`
	LinkedAmount      string     `json:"linked_amount"`
	AvailableAmount   string     `json:"available_amount"`
	// LinkedToReceipt es lo vinculado vigente con la recepción indicada en receipt_id (si se indicó).
	LinkedToReceipt   *string `json:"linked_to_receipt"`
	Linkable          bool    `json:"linkable"`
	UnavailableReason *string `json:"unavailable_reason"`
	UnavailableLabel  *string `json:"unavailable_label"`
	kind              string
	amount            decimal.Decimal
	linked            decimal.Decimal
	regularized       bool
}

// CashExpensePage es una página de gastos candidatos (el más reciente primero).
type CashExpensePage struct {
	Items    []CashExpense `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// CashLink es un vínculo de una recepción con un gasto. El estado se ve siempre; los datos del
// gasto solo si el actor puede leerlo con las reglas de caja (expense_visible).
type CashLink struct {
	ID              string       `json:"id"`
	ReceiptID       string       `json:"receipt_id"`
	ReceiptFolio    int64        `json:"receipt_folio"`
	CashOperationID string       `json:"cash_operation_id"`
	Amount          string       `json:"amount"`
	Notes           string       `json:"notes"`
	LinkedAt        time.Time    `json:"linked_at"`
	LinkedByName    string       `json:"linked_by_name"`
	Status          string       `json:"status"`
	StatusLabel     string       `json:"status_label"`
	Active          bool         `json:"active"`
	UnlinkedAt      *time.Time   `json:"unlinked_at"`
	UnlinkedByName  *string      `json:"unlinked_by_name"`
	UnlinkReason    *string      `json:"unlink_reason"`
	ExpenseVisible  bool         `json:"expense_visible"`
	Expense         *CashExpense `json:"expense"`
}

// cashExpenseSelect y cashExpenseJoins leen un gasto (alias o) con su turno, caja, categoría,
// corrección (una como máximo por índice único de caja) y Σ vínculos vigentes.
const cashExpenseSelect = `o.id,o.shift_id,o.register_id,reg.name,s.state,s.business_date::text,o.kind,o.currency,o.amount::text,
        o.concept,c.name,o.reference,o.created_at,actor.display_name,fix.id IS NOT NULL,coalesce(fix.regularization,false),
        fix.created_at,coalesce((SELECT sum(link.amount) FROM app.inventory_receipt_cash_links link
            WHERE link.cash_operation_id=o.id AND link.unlinked_at IS NULL),0)::text`

const cashExpenseJoins = ` JOIN app.cash_shifts s ON s.id=o.shift_id
        JOIN app.registers reg ON reg.id=o.register_id
        JOIN app.users actor ON actor.id=o.actor_id
        LEFT JOIN app.cash_expense_categories c ON c.id=o.category_id
        LEFT JOIN app.cash_operations fix ON fix.corrects_operation_id=o.id`

// cashExpenseTargets devuelve los destinos de Scan de cashExpenseSelect y la función que deriva el
// estado, lo disponible y si se puede vincular.
func cashExpenseTargets(expense *CashExpense) ([]any, func()) {
	var amount, linked string
	var corrected bool
	targets := []any{&expense.ID, &expense.ShiftID, &expense.RegisterID, &expense.RegisterName, &expense.ShiftState,
		&expense.ShiftBusinessDate, &expense.kind, &expense.Currency, &amount, &expense.Concept, &expense.CategoryName,
		&expense.Reference, &expense.CreatedAt, &expense.ActorName, &corrected, &expense.regularized, &expense.CorrectedAt, &linked}
	finish := func() {
		expense.amount = parseDatabaseDecimal(amount)
		expense.linked = parseDatabaseDecimal(linked)
		expense.Amount = money(expense.amount)
		expense.LinkedAmount = money(expense.linked)
		expense.Status = "recorded"
		if corrected && expense.regularized {
			expense.Status = "regularized"
		} else if corrected {
			expense.Status = "corrected"
		}
		expense.StatusLabel = cashExpenseStatusLabels[expense.Status]
		available := positivePart(expense.amount.Sub(expense.linked))
		reason := ""
		switch {
		case expense.kind != "expense":
			reason = "not_expense"
		case expense.Currency != "MXN":
			reason = "currency"
		case corrected:
			reason = "corrected"
		case !available.IsPositive():
			reason = "fully_linked"
		}
		if reason == "" {
			expense.AvailableAmount = money(available)
			expense.Linkable = true
		} else {
			expense.AvailableAmount = money(zero)
			label := cashUnavailableLabels[reason]
			expense.UnavailableReason, expense.UnavailableLabel = &reason, &label
		}
	}
	return targets, finish
}

// cashAccess son las reglas de caja del actor para leer gastos: la caja de su terminal y si puede
// leer otras cajas y turnos cerrados.
type cashAccess struct {
	registerID *string
	readAll    bool
	history    bool
	movements  bool
}

func (handler *service) cashAccess() (cashAccess, error) {
	access := cashAccess{
		readAll:   handler.request.Actor.Can(cashPermissionShiftsReadAll),
		history:   handler.request.Actor.Can(cashPermissionHistoryRead),
		movements: handler.request.Actor.Can(cashPermissionMovementsRead),
	}
	err := handler.tx.QueryRow(handler.ctx, "SELECT register_id FROM app.devices WHERE id=$1", handler.request.Actor.DeviceID).
		Scan(&access.registerID)
	if err != nil && !isNoRows(err) {
		return access, err
	}
	return access, nil
}

// readable aplica las reglas de payments (readableShift): otra caja exige cash.shifts.read_all y un
// turno cerrado cash.history.read. Para vincular no se exceptúa a quien cerró el turno (decisión
// conservadora).
func (access cashAccess) readable(registerID, shiftState string) bool {
	if !access.movements {
		return false
	}
	if !access.readAll && (access.registerID == nil || *access.registerID != registerID) {
		return false
	}
	if shiftState == "closed" && !access.history {
		return false
	}
	return true
}

// readCashExpense lee una operación de caja como gasto candidato.
func (handler *service) readCashExpense(id string) (*CashExpense, error) {
	var expense CashExpense
	targets, finish := cashExpenseTargets(&expense)
	err := handler.tx.QueryRow(handler.ctx, "SELECT "+cashExpenseSelect+" FROM app.cash_operations o"+cashExpenseJoins+
		" WHERE o.id=$1", id).Scan(targets...)
	if isNoRows(err) {
		return nil, notFound("El gasto de caja no existe.")
	}
	if err != nil {
		return nil, err
	}
	finish()
	return &expense, nil
}

// listCashExpenses busca gastos candidatos para vincular: por días de negocio de su registro
// (AAAA-MM-DD en la zona del establecimiento), importe mínimo y máximo y caja, solo entre los que el
// actor puede leer con las reglas de caja. receipt_id agrega lo ya vinculado con esa recepción.
func (handler *service) listCashExpenses(_ []string) (any, error) {
	query := handler.request.Query
	allowed := []string{"from", "until", "min_amount", "max_amount", "register_id", "receipt_id", "page", "page_size"}
	if err := allowQuery(query, allowed...); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	access, err := handler.cashAccess()
	if err != nil {
		return nil, err
	}
	result := CashExpensePage{Items: []CashExpense{}, Page: page, PageSize: size}
	set := filters{clauses: []string{"o.kind='expense'"}}
	registerID, err := optionalQueryID(query, "register_id", "caja")
	if err != nil {
		return nil, err
	}
	if registerID != nil {
		if !access.readAll && (access.registerID == nil || *access.registerID != *registerID) {
			return nil, denied()
		}
		set.add("o.register_id=$%d", *registerID)
	} else if !access.readAll {
		if access.registerID == nil {
			// Sin caja asociada a la terminal ni permiso para otras cajas no hay gastos legibles.
			return result, nil
		}
		set.add("o.register_id=$%d", *access.registerID)
	}
	if !access.history {
		set.clauses = append(set.clauses, "s.state<>'closed'")
	}
	from, until, err := handler.dateRange(query)
	if err != nil {
		return nil, err
	}
	if from != nil {
		set.add("o.created_at>=$%d", *from)
	}
	if until != nil {
		set.add("o.created_at<$%d", *until)
	}
	for key, comparison := range map[string]string{"min_amount": ">=", "max_amount": "<="} {
		text := query.Get(key)
		if text == "" {
			continue
		}
		amount, err := parsePurchaseAmount(text, "El importe del filtro")
		if err != nil {
			return nil, err
		}
		set.add("o.amount"+comparison+"$%d::numeric", money(amount))
	}
	receiptID, err := optionalQueryID(query, "receipt_id", "recepción")
	if err != nil {
		return nil, err
	}
	if receiptID != nil {
		var exists bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.inventory_receipts WHERE id=$1)", *receiptID).
			Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, notFound("La recepción no existe.")
		}
	}
	count := "SELECT count(*) FROM app.cash_operations o JOIN app.cash_shifts s ON s.id=o.shift_id" + set.where()
	if err := handler.tx.QueryRow(handler.ctx, count, set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	statement := "SELECT " + cashExpenseSelect + " FROM app.cash_operations o" + cashExpenseJoins + set.where() +
		" ORDER BY o.created_at DESC, o.id LIMIT " + set.next(size) + " OFFSET " + set.next((page-1)*size)
	rows, err := handler.tx.Query(handler.ctx, statement, set.arguments...)
	if err != nil {
		return nil, err
	}
	identifiers := []string{}
	for rows.Next() {
		var expense CashExpense
		targets, finish := cashExpenseTargets(&expense)
		if err := rows.Scan(targets...); err != nil {
			rows.Close()
			return nil, err
		}
		finish()
		result.Items = append(result.Items, expense)
		identifiers = append(identifiers, expense.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if receiptID != nil {
		if err := handler.attachReceiptLinks(&result, *receiptID, identifiers); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// attachReceiptLinks agrega a cada gasto lo vinculado vigente con la recepción indicada.
func (handler *service) attachReceiptLinks(page *CashExpensePage, receiptID string, identifiers []string) error {
	linked := map[string]string{}
	rows, err := handler.tx.Query(handler.ctx, `SELECT cash_operation_id,sum(amount)::text FROM app.inventory_receipt_cash_links
        WHERE receipt_id=$1 AND unlinked_at IS NULL AND cash_operation_id=ANY($2::uuid[]) GROUP BY cash_operation_id`,
		receiptID, identifiers)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, amount string
		if err := rows.Scan(&id, &amount); err != nil {
			return err
		}
		linked[id] = money(parseDatabaseDecimal(amount))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for index := range page.Items {
		amount, found := linked[page.Items[index].ID]
		if !found {
			amount = money(zero)
		}
		page.Items[index].LinkedToReceipt = &amount
	}
	return nil
}

type cashLinkInput struct {
	OperationID     string `json:"operation_id"`
	CashOperationID string `json:"cash_operation_id"`
	Amount          string `json:"amount"`
	Notes           string `json:"notes"`
}

func formatPesos(value decimal.Decimal) string {
	return moneytext.FormatDecimal("MXN", value)
}

// linkCashExpense vincula una recepción con un gasto de caja existente (total o parcialmente).
func (handler *service) linkCashExpense(parts []string) (any, error) {
	var input cashLinkInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.CashOperationID); err != nil {
		return nil, invalid("Elige el gasto de caja que pagó la compra.")
	}
	amount, err := parsePurchaseAmount(input.Amount, "El importe vinculado")
	if err != nil {
		return nil, err
	}
	if !amount.IsPositive() {
		return nil, invalid("El importe vinculado debe ser mayor que cero.")
	}
	notes, err := purchaseText(input.Notes, "La nota del vínculo", 500, false)
	if err != nil {
		return nil, err
	}
	receipt, err := handler.readReceipt(parts[1])
	if err != nil {
		return nil, err
	}
	expense, err := handler.readCashExpense(strings.ToLower(input.CashOperationID))
	if err != nil {
		return nil, err
	}
	access, err := handler.cashAccess()
	if err != nil {
		return nil, err
	}
	if !access.readable(expense.RegisterID, expense.ShiftState) {
		return nil, denied()
	}
	if err := checkLinkableExpense(expense); err != nil {
		return nil, err
	}
	var paired bool
	if err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.inventory_receipt_cash_links
        WHERE receipt_id=$1 AND cash_operation_id=$2 AND unlinked_at IS NULL)`, receipt.ID, expense.ID).Scan(&paired); err != nil {
		return nil, err
	}
	if paired {
		return nil, conflict("CASH_LINK_EXISTS", "La recepción ya está vinculada con ese gasto: desvincúlala para cambiar el importe.")
	}
	// Sin egresos duplicados: el gasto no se vincula por más de su importe en todas sus recepciones.
	if expense.linked.Add(amount).GreaterThan(expense.amount) {
		return nil, conflict("CASH_EXPENSE_OVERLINKED", "El gasto de "+formatPesos(expense.amount)+" ya tiene "+
			formatPesos(expense.linked)+" vinculados: solo quedan "+formatPesos(positivePart(expense.amount.Sub(expense.linked)))+
			" disponibles.")
	}
	total := parseDatabaseDecimal(receipt.Total)
	linked := parseDatabaseDecimal(receipt.LinkedAmount)
	if linked.Add(amount).GreaterThan(total) {
		return nil, conflict("RECEIPT_OVERLINKED", "La recepción de "+formatPesos(total)+" ya tiene "+formatPesos(linked)+
			" vinculados: solo quedan "+formatPesos(positivePart(total.Sub(linked)))+" por vincular.")
	}
	id := newID()
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.inventory_receipt_cash_links(id,receipt_id,cash_operation_id,amount,notes,
            linked_by,operation_id)
        VALUES ($1,$2,$3,$4::numeric,$5,$6,$7)`,
		id, receipt.ID, expense.ID, money(amount), notes, handler.request.Actor.UserID, handler.operationReference())
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.receipts.changed", &receipt.ID); err != nil {
		return nil, err
	}
	after := map[string]any{
		"receipt_id": receipt.ID, "receipt_folio": receipt.Folio, "cash_operation_id": expense.ID, "amount": money(amount),
		"notes": notes, "expense_amount": expense.Amount, "expense_register_id": expense.RegisterID, "expense_shift_id": expense.ShiftID,
		"expense_shift_state": expense.ShiftState,
	}
	err = handler.audit(handler.ctx, handler.tx, "inventory.cash_link_created", "inventory_receipt_cash_link", &id, nil, after)
	if err != nil {
		return nil, err
	}
	return handler.readReceipt(receipt.ID)
}

// checkLinkableExpense exige un gasto en pesos sin corrección.
func checkLinkableExpense(expense *CashExpense) error {
	switch {
	case expense.kind != "expense":
		return conflict("CASH_OPERATION_NOT_EXPENSE", "Solo se vinculan gastos de caja con una compra.")
	case expense.Currency != "MXN":
		return conflict("CASH_EXPENSE_CURRENCY", "Las compras se registran en pesos: un gasto en dólares no se puede vincular.")
	case expense.Status != "recorded":
		return conflict("CASH_EXPENSE_CORRECTED", "El gasto ya fue "+strings.ToLower(expense.StatusLabel)+": no se puede vincular.")
	}
	return nil
}

type cashUnlinkInput struct {
	OperationID string `json:"operation_id"`
	Reason      string `json:"reason"`
}

// unlinkCashExpense registra la desvinculación de un vínculo vigente, con motivo. No borra el vínculo
// ni toca el gasto.
func (handler *service) unlinkCashExpense(parts []string) (any, error) {
	var input cashUnlinkInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason, err := purchaseReason(input.Reason, "El motivo de la desvinculación")
	if err != nil {
		return nil, err
	}
	links, err := handler.cashLinks("cl.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, notFound("El vínculo con el gasto de caja no existe.")
	}
	link := links[0]
	if !link.ExpenseVisible {
		return nil, denied()
	}
	if !link.Active {
		return nil, conflict("CASH_LINK_INACTIVE", "El vínculo con el gasto ya se había desvinculado.")
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.inventory_receipt_cash_links SET unlinked_by=$2,unlinked_at=clock_timestamp(),
            unlink_reason=$3,unlink_operation_id=$4
        WHERE id=$1`, link.ID, handler.request.Actor.UserID, reason, handler.operationReference())
	if err != nil {
		return nil, err
	}
	if err := handler.publish("inventory.receipts.changed", &link.ReceiptID); err != nil {
		return nil, err
	}
	before := map[string]any{"receipt_id": link.ReceiptID, "cash_operation_id": link.CashOperationID, "amount": link.Amount,
		"status": link.Status}
	after := map[string]any{"receipt_id": link.ReceiptID, "cash_operation_id": link.CashOperationID, "status": "unlinked",
		"reason": reason}
	if err := handler.audit(handler.ctx, handler.tx, "inventory.cash_link_removed", "inventory_receipt_cash_link", &link.ID, before,
		after); err != nil {
		return nil, err
	}
	return handler.readReceipt(link.ReceiptID)
}

// cashLinks lee los vínculos que cumplen la condición (el más antiguo primero) con el estado
// derivado del gasto y, si el actor puede leerlo, sus datos.
func (handler *service) cashLinks(where string, arguments ...any) ([]CashLink, error) {
	access, err := handler.cashAccess()
	if err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT cl.id,cl.receipt_id,r.folio,cl.cash_operation_id,cl.amount::text,cl.notes,
            cl.linked_at,linker.display_name,cl.unlinked_at,unlinker.display_name,cl.unlink_reason,`+cashExpenseSelect+`
        FROM app.inventory_receipt_cash_links cl
        JOIN app.inventory_receipts r ON r.id=cl.receipt_id
        JOIN app.users linker ON linker.id=cl.linked_by
        LEFT JOIN app.users unlinker ON unlinker.id=cl.unlinked_by
        JOIN app.cash_operations o ON o.id=cl.cash_operation_id`+cashExpenseJoins+`
        WHERE `+where+` ORDER BY cl.linked_at, cl.id`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []CashLink{}
	for rows.Next() {
		var link CashLink
		var amount string
		var expense CashExpense
		targets, finish := cashExpenseTargets(&expense)
		err := rows.Scan(append([]any{&link.ID, &link.ReceiptID, &link.ReceiptFolio, &link.CashOperationID, &amount, &link.Notes,
			&link.LinkedAt, &link.LinkedByName, &link.UnlinkedAt, &link.UnlinkedByName, &link.UnlinkReason}, targets...)...)
		if err != nil {
			return nil, err
		}
		finish()
		link.Amount = money(parseDatabaseDecimal(amount))
		link.Active = link.UnlinkedAt == nil
		switch {
		case !link.Active:
			link.Status = "unlinked"
		case expense.Status == "regularized":
			link.Status = "expense_regularized"
		case expense.Status == "corrected":
			link.Status = "expense_corrected"
		default:
			link.Status = "active"
		}
		link.StatusLabel = cashLinkStatusLabels[link.Status]
		link.ExpenseVisible = access.readable(expense.RegisterID, expense.ShiftState)
		if link.ExpenseVisible {
			copied := expense
			link.Expense = &copied
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
