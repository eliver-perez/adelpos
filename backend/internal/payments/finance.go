package payments

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// accountRecord es la parte de la cuenta que interesa al módulo financiero.
type accountRecord struct {
	ID            string
	Folio         int64
	Name          string
	Mode          string
	State         string
	Revision      int
	TableName     *string
	CustomerName  string
	DraftQuantity int
}

func readAccountRecord(ctx context.Context, tx pgx.Tx, accountID string, forUpdate bool) (*accountRecord, error) {
	query := `SELECT a.id,a.folio,a.name,a.mode,a.state,a.revision,t.name,a.customer_name,
        (SELECT coalesce(sum(quantity),0)::integer FROM app.restaurant_draft_items d WHERE d.account_id=a.id)
        FROM app.restaurant_accounts a LEFT JOIN app.restaurant_tables t ON t.id=a.table_id WHERE a.id=$1`
	if forUpdate {
		query += " FOR UPDATE OF a"
	}
	var record accountRecord
	err := tx.QueryRow(ctx, query, accountID).Scan(&record.ID, &record.Folio, &record.Name, &record.Mode, &record.State,
		&record.Revision, &record.TableName, &record.CustomerName, &record.DraftQuantity)
	if isNoRows(err) {
		return nil, notFound("La cuenta no existe.")
	}
	return &record, err
}

// unitState describe una unidad comercial activa con sus importes financieros vigentes.
type unitState struct {
	ID        string
	VersionID string
	ItemID    string
	Ordinal   int
	Name      string
	Notes     string
	Total     decimal.Decimal
	TaxRate   int
	Discount  decimal.Decimal
	Paid      decimal.Decimal
}

// Due es lo que la unidad debe tras descuentos manuales vigentes.
func (unit unitState) Due() decimal.Decimal {
	return unit.Total.Sub(unit.Discount)
}

// Remaining es lo que falta asignarle mediante pagos por selección.
func (unit unitState) Remaining() decimal.Decimal {
	return unit.Due().Sub(unit.Paid)
}

// Status distingue unidades sin pago, con pago parcial y liquidadas.
func (unit unitState) Status() string {
	if unit.Paid.Sign() <= 0 {
		return "unpaid"
	}
	if unit.Remaining().Sign() <= 0 {
		return "settled"
	}
	return "partial"
}

const activeAdjustment = "NOT EXISTS(SELECT FROM app.account_adjustment_voids v WHERE v.adjustment_id=j.id)"

func readUnits(ctx context.Context, tx pgx.Tx, accountID string) ([]unitState, error) {
	rows, err := tx.Query(ctx, `SELECT u.id,u.version_id,u.item_id,u.ordinal,coalesce(v.quote_line->>'name',''),coalesce(v.request->'customization'->>'notes',''),u.total::text,v.tax_rate,
        coalesce((SELECT sum(a.amount) FROM app.account_adjustment_allocations a JOIN app.account_adjustments j ON j.id=a.adjustment_id
            WHERE a.unit_id=u.id AND j.kind='discount' AND `+activeAdjustment+`),0)::numeric(18,2)::text,
        (coalesce((SELECT sum(p.amount) FROM app.payment_allocations p WHERE p.unit_id=u.id),0)
            - coalesce((SELECT sum(r.amount) FROM app.payment_refund_allocations r WHERE r.unit_id=u.id),0))::numeric(18,2)::text
        FROM app.restaurant_item_units u JOIN app.restaurant_item_versions v ON v.id=u.version_id
        WHERE u.account_id=$1 AND u.state='active' ORDER BY u.created_at,u.item_id,u.ordinal`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []unitState{}
	for rows.Next() {
		var unit unitState
		var total, discount, paid string
		if err := rows.Scan(&unit.ID, &unit.VersionID, &unit.ItemID, &unit.Ordinal, &unit.Name, &unit.Notes, &total, &unit.TaxRate,
			&discount, &paid); err != nil {
			return nil, err
		}
		unit.Total = decimal.RequireFromString(total)
		unit.Discount = decimal.RequireFromString(discount)
		unit.Paid = decimal.RequireFromString(paid)
		units = append(units, unit)
	}
	return units, rows.Err()
}

type Adjustment struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Scope        string     `json:"scope"`
	Method       string     `json:"method"`
	Value        string     `json:"value"`
	Amount       string     `json:"amount"`
	Courtesy     bool       `json:"courtesy"`
	TaxRate      *int       `json:"tax_rate"`
	Concept      string     `json:"concept"`
	Reason       string     `json:"reason"`
	ActorName    string     `json:"actor_name"`
	AuthorizedBy *string    `json:"authorized_by_name"`
	CreatedAt    time.Time  `json:"created_at"`
	Voided       bool       `json:"voided"`
	VoidedAt     *time.Time `json:"voided_at"`
	VoidReason   *string    `json:"void_reason"`
	UnitIDs      []string   `json:"unit_ids"`
}

func readAdjustments(ctx context.Context, tx pgx.Tx, accountID string) ([]Adjustment, error) {
	rows, err := tx.Query(ctx, `SELECT j.id,j.kind,j.scope,j.method,j.value::text,j.amount::text,j.courtesy,j.tax_rate,j.concept,j.reason,
        actor.display_name,authorizer.display_name,j.created_at,v.adjustment_id IS NOT NULL,v.created_at,v.reason,
        coalesce((SELECT array_agg(a.unit_id::text ORDER BY a.unit_id) FROM app.account_adjustment_allocations a WHERE a.adjustment_id=j.id),'{}')
        FROM app.account_adjustments j JOIN app.users actor ON actor.id=j.actor_id
        LEFT JOIN app.users authorizer ON authorizer.id=j.authorized_by
        LEFT JOIN app.account_adjustment_voids v ON v.adjustment_id=j.id
        WHERE j.account_id=$1 ORDER BY j.created_at,j.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Adjustment{}
	for rows.Next() {
		var item Adjustment
		if err := rows.Scan(&item.ID, &item.Kind, &item.Scope, &item.Method, &item.Value, &item.Amount, &item.Courtesy, &item.TaxRate,
			&item.Concept, &item.Reason, &item.ActorName, &item.AuthorizedBy, &item.CreatedAt, &item.Voided, &item.VoidedAt,
			&item.VoidReason, &item.UnitIDs); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type PaymentRecord struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	MethodID   string  `json:"method_id"`
	MethodName string  `json:"method_name"`
	IsCash     bool    `json:"is_cash"`
	Amount     string  `json:"amount"`
	Tip        string  `json:"tip"`
	Received   *string `json:"received"`
	Change     string  `json:"change"`
	// Columnas por moneda de un efectivo con dólares; nil en pagos solo en pesos.
	ReceivedMXN       *string   `json:"received_mxn"`
	ReceivedUSD       *string   `json:"received_usd"`
	USDRate           *string   `json:"usd_rate"`
	ChangeCurrency    *string   `json:"change_currency"`
	ChangeUSD         *string   `json:"change_usd"`
	Reference         string    `json:"reference"`
	CorrectsPaymentID *string   `json:"corrects_payment_id"`
	RefundedAmount    string    `json:"refunded_amount"`
	RefundedTip       string    `json:"refunded_tip"`
	Corrected         bool      `json:"corrected"`
	ShiftID           string    `json:"shift_id"`
	RegisterName      string    `json:"register_name"`
	ActorName         string    `json:"actor_name"`
	DeviceName        string    `json:"device_name"`
	CreatedAt         time.Time `json:"created_at"`
	UnitIDs           []string  `json:"unit_ids"`
}

func readPayments(ctx context.Context, tx pgx.Tx, accountID string) ([]PaymentRecord, error) {
	rows, err := tx.Query(ctx, `SELECT p.id,p.kind,p.method_id,p.method_name,p.is_cash,p.amount::text,p.tip::text,p.received::text,
        p.change_given::text,p.received_mxn::text,p.received_usd::text,p.usd_rate::text,p.change_currency,p.change_usd::text,
        p.reference,p.corrects_payment_id,
        coalesce((SELECT sum(r.amount) FROM app.payment_refunds r WHERE r.payment_id=p.id),0)::numeric(18,2)::text,
        coalesce((SELECT sum(r.tip) FROM app.payment_refunds r WHERE r.payment_id=p.id),0)::numeric(18,2)::text,
        EXISTS(SELECT FROM app.payment_refunds r WHERE r.payment_id=p.id AND r.kind='correction'),
        p.shift_id,register.name,actor.display_name,device.name,p.created_at,
        coalesce((SELECT array_agg(a.unit_id::text ORDER BY a.unit_id) FROM app.payment_allocations a WHERE a.payment_id=p.id),'{}')
        FROM app.payments p JOIN app.registers register ON register.id=p.register_id
        JOIN app.users actor ON actor.id=p.actor_id JOIN app.devices device ON device.id=p.device_id
        WHERE p.account_id=$1 ORDER BY p.created_at,p.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PaymentRecord{}
	for rows.Next() {
		var item PaymentRecord
		if err := rows.Scan(&item.ID, &item.Kind, &item.MethodID, &item.MethodName, &item.IsCash, &item.Amount, &item.Tip, &item.Received,
			&item.Change, &item.ReceivedMXN, &item.ReceivedUSD, &item.USDRate, &item.ChangeCurrency, &item.ChangeUSD,
			&item.Reference, &item.CorrectsPaymentID, &item.RefundedAmount, &item.RefundedTip, &item.Corrected,
			&item.ShiftID, &item.RegisterName, &item.ActorName, &item.DeviceName, &item.CreatedAt, &item.UnitIDs); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type RefundRecord struct {
	ID           string    `json:"id"`
	PaymentID    string    `json:"payment_id"`
	Kind         string    `json:"kind"`
	MethodName   string    `json:"method_name"`
	IsCash       bool      `json:"is_cash"`
	Amount       string    `json:"amount"`
	Tip          string    `json:"tip"`
	Reason       string    `json:"reason"`
	Reference    string    `json:"reference"`
	ActorName    string    `json:"actor_name"`
	AuthorizedBy *string   `json:"authorized_by_name"`
	CreatedAt    time.Time `json:"created_at"`
	UnitIDs      []string  `json:"unit_ids"`
	Tax          []TaxLine `json:"tax"`
}

func readRefunds(ctx context.Context, tx pgx.Tx, accountID string) ([]RefundRecord, error) {
	rows, err := tx.Query(ctx, `SELECT r.id,r.payment_id,r.kind,r.method_name,r.is_cash,r.amount::text,r.tip::text,r.reason,r.reference,
        r.tax_breakdown,actor.display_name,authorizer.display_name,r.created_at,
        coalesce((SELECT array_agg(a.unit_id::text ORDER BY a.unit_id) FROM app.payment_refund_allocations a WHERE a.refund_id=r.id),'{}')
        FROM app.payment_refunds r JOIN app.users actor ON actor.id=r.actor_id
        LEFT JOIN app.users authorizer ON authorizer.id=r.authorized_by
        WHERE r.account_id=$1 ORDER BY r.created_at,r.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RefundRecord{}
	for rows.Next() {
		var item RefundRecord
		if err := rows.Scan(&item.ID, &item.PaymentID, &item.Kind, &item.MethodName, &item.IsCash, &item.Amount, &item.Tip, &item.Reason,
			&item.Reference, &item.Tax, &item.ActorName, &item.AuthorizedBy, &item.CreatedAt, &item.UnitIDs); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type FinanceUnit struct {
	ID        string `json:"id"`
	Ordinal   int    `json:"ordinal"`
	Total     string `json:"total"`
	Discount  string `json:"discount"`
	Due       string `json:"due"`
	Paid      string `json:"paid"`
	Remaining string `json:"remaining"`
	Status    string `json:"status"`
	TaxRate   int    `json:"tax_rate"`
}

// FinanceLine agrupa las unidades de una misma versión de partida que conserva la cuenta.
type FinanceLine struct {
	VersionID       string        `json:"version_id"`
	ItemID          string        `json:"item_id"`
	Name            string        `json:"name"`
	Notes           string        `json:"notes"`
	Quantity        int           `json:"quantity"`
	SettledQuantity int           `json:"settled_quantity"`
	PartialQuantity int           `json:"partial_quantity"`
	Due             string        `json:"due"`
	Remaining       string        `json:"remaining"`
	Units           []FinanceUnit `json:"units"`
}

type Delivery struct {
	DeliveredAt time.Time `json:"delivered_at"`
	ActorName   string    `json:"actor_name"`
	DeviceName  string    `json:"device_name"`
}

// Finance es el estado financiero completo y autoritativo de una cuenta.
type Finance struct {
	AccountID        string           `json:"account_id"`
	Folio            int64            `json:"folio"`
	Name             string           `json:"name"`
	Mode             string           `json:"mode"`
	State            string           `json:"state"`
	Revision         int              `json:"revision"`
	TableName        *string          `json:"table_name"`
	ItemsTotal       string           `json:"items_total"`
	ItemDiscounts    string           `json:"item_discounts"`
	AccountDiscounts string           `json:"account_discounts"`
	Charges          string           `json:"charges"`
	Total            string           `json:"total"`
	Paid             string           `json:"paid"`
	Refunded         string           `json:"refunded"`
	Tips             string           `json:"tips"`
	Balance          string           `json:"balance"`
	PaymentState     string           `json:"payment_state"`
	DeliveryState    string           `json:"delivery_state"`
	Settled          bool             `json:"settled"`
	Tax              []TaxLine        `json:"tax"`
	TaxBase          string           `json:"tax_base"`
	TaxTotal         string           `json:"tax_total"`
	Lines            []FinanceLine    `json:"lines"`
	Payments         []PaymentRecord  `json:"payments"`
	Refunds          []RefundRecord   `json:"refunds"`
	Adjustments      []Adjustment     `json:"adjustments"`
	Delivery         *Delivery        `json:"delivery"`
	Documents        []DocumentRecord `json:"documents"`
	ServerTime       time.Time        `json:"server_time"`

	units   []unitState
	total   decimal.Decimal
	paid    decimal.Decimal
	balance decimal.Decimal
	record  *accountRecord
}

func sumDecimal(values ...decimal.Decimal) decimal.Decimal {
	result := zero
	for _, value := range values {
		result = result.Add(value)
	}
	return result
}

// loadFinance calcula el estado financiero completo a partir de los registros persistidos.
func loadFinance(ctx context.Context, tx pgx.Tx, accountID string) (*Finance, error) {
	record, err := readAccountRecord(ctx, tx, accountID, false)
	if err != nil {
		return nil, err
	}
	units, err := readUnits(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	adjustments, err := readAdjustments(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	payments, err := readPayments(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	refunds, err := readRefunds(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	finance := &Finance{
		AccountID: record.ID, Folio: record.Folio, Name: record.Name, Mode: record.Mode, State: record.State,
		Revision: record.Revision, TableName: record.TableName, Payments: payments, Refunds: refunds,
		Adjustments: adjustments, ServerTime: time.Now().UTC(), units: units, record: record,
	}
	itemsTotal, itemDiscounts := zero, zero
	grossByRate := map[int]decimal.Decimal{}
	for _, unit := range units {
		itemsTotal = itemsTotal.Add(unit.Total)
		itemDiscounts = itemDiscounts.Add(unit.Discount)
		grossByRate[unit.TaxRate] = grossByRate[unit.TaxRate].Add(unit.Due())
	}
	accountDiscounts, charges := zero, zero
	for _, adjustment := range adjustments {
		if adjustment.Voided {
			continue
		}
		amount := decimal.RequireFromString(adjustment.Amount)
		if adjustment.Kind == "charge" {
			charges = charges.Add(amount)
			grossByRate[*adjustment.TaxRate] = grossByRate[*adjustment.TaxRate].Add(amount)
		} else if adjustment.Scope == "account" {
			accountDiscounts = accountDiscounts.Add(amount)
		}
	}
	// El descuento general se reparte entre tasas en proporción a su importe bruto.
	rates := []int{}
	weights := []decimal.Decimal{}
	for _, rate := range allowedTaxRates {
		if gross, found := grossByRate[rate]; found {
			rates = append(rates, rate)
			weights = append(weights, gross)
		}
	}
	for index, share := range allocate(accountDiscounts, weights) {
		grossByRate[rates[index]] = grossByRate[rates[index]].Sub(share)
	}
	paid, refunded, tips := zero, zero, zero
	for _, payment := range payments {
		paid = paid.Add(decimal.RequireFromString(payment.Amount))
		tips = tips.Add(decimal.RequireFromString(payment.Tip))
	}
	for _, refund := range refunds {
		refunded = refunded.Add(decimal.RequireFromString(refund.Amount))
		tips = tips.Sub(decimal.RequireFromString(refund.Tip))
	}
	paid = paid.Sub(refunded)
	total := itemsTotal.Sub(itemDiscounts).Sub(accountDiscounts).Add(charges)
	balance := total.Sub(paid)
	finance.total, finance.paid, finance.balance = total, paid, balance
	finance.ItemsTotal = money(itemsTotal)
	finance.ItemDiscounts = money(itemDiscounts)
	finance.AccountDiscounts = money(accountDiscounts)
	finance.Charges = money(charges)
	finance.Total = money(total)
	finance.Paid = money(paid)
	finance.Refunded = money(refunded)
	finance.Tips = money(tips)
	finance.Balance = money(balance)
	finance.Tax = taxBreakdown(grossByRate)
	taxBase, taxTotal := zero, zero
	for _, line := range finance.Tax {
		taxBase = taxBase.Add(decimal.RequireFromString(line.Base))
		taxTotal = taxTotal.Add(decimal.RequireFromString(line.Tax))
	}
	finance.TaxBase = money(taxBase)
	finance.TaxTotal = money(taxTotal)
	finance.PaymentState = paymentState(len(units) > 0, total, paid)
	finance.Settled = len(units) > 0 && balance.Sign() <= 0
	finance.Lines = groupLines(units)
	finance.DeliveryState = "not_delivered"
	var delivery Delivery
	err = tx.QueryRow(ctx, `SELECT d.delivered_at,actor.display_name,device.name FROM app.takeaway_deliveries d
        JOIN app.users actor ON actor.id=d.actor_id JOIN app.devices device ON device.id=d.device_id
        WHERE d.account_id=$1`, accountID).Scan(&delivery.DeliveredAt, &delivery.ActorName, &delivery.DeviceName)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	if err == nil {
		finance.Delivery = &delivery
		finance.DeliveryState = "delivered"
	}
	finance.Documents, err = readDocuments(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	return finance, nil
}

// readDocuments lista precuentas y tickets con el estado de su último trabajo de impresión.
func readDocuments(ctx context.Context, tx pgx.Tx, accountID string) ([]DocumentRecord, error) {
	rows, err := tx.Query(ctx, `SELECT d.ticket_id,d.kind,d.payment_id,d.created_at,j.id,j.state,p.name,
        (SELECT count(*) FROM app.kitchen_print_jobs c WHERE c.ticket_id=d.ticket_id)::integer
        FROM app.payment_documents d
        LEFT JOIN LATERAL (SELECT id,state,printer_id FROM app.kitchen_print_jobs WHERE ticket_id=d.ticket_id
            ORDER BY created_at DESC LIMIT 1) j ON true
        LEFT JOIN app.kitchen_printers p ON p.id=j.printer_id
        WHERE d.account_id=$1 ORDER BY d.created_at,d.ticket_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DocumentRecord{}
	for rows.Next() {
		var item DocumentRecord
		if err := rows.Scan(&item.TicketID, &item.Kind, &item.PaymentID, &item.CreatedAt, &item.JobID, &item.JobState,
			&item.PrinterName, &item.Copies); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// paymentState distingue cuentas sin pago, con abonos y liquidadas. Una cuenta con productos y
// total cero por cortesía o descuento total queda liquidada sin un pago ficticio.
func paymentState(hasUnits bool, total, paid decimal.Decimal) string {
	if hasUnits && total.Sub(paid).Sign() <= 0 {
		return "paid"
	}
	if paid.Sign() > 0 {
		return "partial"
	}
	return "not_recorded"
}

func groupLines(units []unitState) []FinanceLine {
	lines := []FinanceLine{}
	positions := map[string]int{}
	for _, unit := range units {
		position, found := positions[unit.VersionID]
		if !found {
			position = len(lines)
			positions[unit.VersionID] = position
			lines = append(lines, FinanceLine{VersionID: unit.VersionID, ItemID: unit.ItemID, Name: unit.Name, Notes: unit.Notes, Due: "0.00", Remaining: "0.00"})
		}
		line := &lines[position]
		line.Quantity++
		switch unit.Status() {
		case "settled":
			line.SettledQuantity++
		case "partial":
			line.PartialQuantity++
		}
		line.Due = money(decimal.RequireFromString(line.Due).Add(unit.Due()))
		line.Remaining = money(decimal.RequireFromString(line.Remaining).Add(unit.Remaining()))
		line.Units = append(line.Units, FinanceUnit{
			ID: unit.ID, Ordinal: unit.Ordinal, Total: money(unit.Total), Discount: money(unit.Discount), Due: money(unit.Due()),
			Paid: money(unit.Paid), Remaining: money(unit.Remaining()), Status: unit.Status(), TaxRate: unit.TaxRate,
		})
	}
	return lines
}

// Brief es el resumen financiero que la operación del restaurante muestra con cada cuenta.
type Brief struct {
	Total         string `json:"total"`
	Paid          string `json:"paid"`
	Balance       string `json:"balance"`
	Tips          string `json:"tips"`
	PaymentState  string `json:"payment_state"`
	DeliveryState string `json:"delivery_state"`
}

// AccountBrief resume con una sola consulta el total, lo pagado y la entrega de una cuenta.
func AccountBrief(ctx context.Context, tx pgx.Tx, accountID string) (*Brief, error) {
	var units int
	var itemsText, discountsText, chargesText, paidText, tipsText string
	var delivered bool
	err := tx.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM app.restaurant_item_units WHERE account_id=$1 AND state='active'),
        coalesce((SELECT sum(total) FROM app.restaurant_item_units WHERE account_id=$1 AND state='active'),0)::numeric(18,2)::text,
        coalesce((SELECT sum(j.amount) FROM app.account_adjustments j WHERE j.account_id=$1 AND j.kind='discount' AND `+activeAdjustment+`),0)::numeric(18,2)::text,
        coalesce((SELECT sum(j.amount) FROM app.account_adjustments j WHERE j.account_id=$1 AND j.kind='charge' AND `+activeAdjustment+`),0)::numeric(18,2)::text,
        (coalesce((SELECT sum(amount) FROM app.payments WHERE account_id=$1),0)
            - coalesce((SELECT sum(amount) FROM app.payment_refunds WHERE account_id=$1),0))::numeric(18,2)::text,
        (coalesce((SELECT sum(tip) FROM app.payments WHERE account_id=$1),0)
            - coalesce((SELECT sum(tip) FROM app.payment_refunds WHERE account_id=$1),0))::numeric(18,2)::text,
        EXISTS(SELECT FROM app.takeaway_deliveries WHERE account_id=$1)`, accountID).Scan(
		&units, &itemsText, &discountsText, &chargesText, &paidText, &tipsText, &delivered)
	if err != nil {
		return nil, err
	}
	items := decimal.RequireFromString(itemsText)
	discounts := decimal.RequireFromString(discountsText)
	charges := decimal.RequireFromString(chargesText)
	paid := decimal.RequireFromString(paidText)
	tips := decimal.RequireFromString(tipsText)
	total := items.Sub(discounts).Add(charges)
	brief := &Brief{
		Total: money(total), Paid: money(paid), Balance: money(total.Sub(paid)), Tips: money(tips),
		PaymentState: paymentState(units > 0, total, paid), DeliveryState: "not_delivered",
	}
	if delivered {
		brief.DeliveryState = "delivered"
	}
	return brief, nil
}

// physicalEffect devuelve lo que un pago en efectivo movió por moneda en el cajón. nil si el pago
// no fue en efectivo. Un pago solo en pesos movió lo recibido y su cambio.
func (payment PaymentRecord) physicalEffect() *cashPhysical {
	if !payment.IsCash {
		return nil
	}
	if payment.ReceivedUSD == nil {
		charged := decimal.RequireFromString(payment.Amount).Add(decimal.RequireFromString(payment.Tip))
		received := charged
		if payment.Received != nil {
			received = decimal.RequireFromString(*payment.Received)
		}
		return &cashPhysical{mxnIn: received, mxnOut: decimal.RequireFromString(payment.Change)}
	}
	rate := decimal.RequireFromString(*payment.USDRate)
	effect := &cashPhysical{
		mxnIn: decimal.RequireFromString(*payment.ReceivedMXN),
		usdIn: decimal.RequireFromString(*payment.ReceivedUSD),
		rate:  &rate,
	}
	if payment.ChangeCurrency != nil && *payment.ChangeCurrency == currencyUSD {
		effect.usdOut = decimal.RequireFromString(*payment.ChangeUSD)
	} else {
		effect.mxnOut = decimal.RequireFromString(payment.Change)
	}
	return effect
}
