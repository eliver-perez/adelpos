package payments

import (
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Precuentas y tickets: los datos se separan de la plantilla de texto y del transporte, que
// pertenece a la cola durable de cocina. El editor visual de documentos es de la etapa 9.

type DocumentLine struct {
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Amount   string `json:"amount"`
}

type DocumentAdjustment struct {
	Concept string `json:"concept"`
	Amount  string `json:"amount"`
}

type DocumentPayment struct {
	Method    string  `json:"method"`
	Amount    string  `json:"amount"`
	Tip       string  `json:"tip"`
	Reference string  `json:"reference"`
	Received  *string `json:"received"`
	Change    string  `json:"change"`
	// Efectivo con dólares: pesos y dólares recibidos, tipo de cambio y moneda del cambio.
	ReceivedMXN    *string `json:"received_mxn"`
	ReceivedUSD    *string `json:"received_usd"`
	USDRate        *string `json:"usd_rate"`
	ChangeCurrency *string `json:"change_currency"`
	ChangeUSD      *string `json:"change_usd"`
}

// CommercialDocument es la información de una precuenta o ticket. No es un CFDI.
type CommercialDocument struct {
	Kind          string               `json:"kind"`
	Establishment string               `json:"establishment"`
	Folio         int64                `json:"folio"`
	AccountName   string               `json:"account_name"`
	Context       string               `json:"context"`
	IssuedAt      time.Time            `json:"issued_at"`
	LocalTime     string               `json:"local_time"`
	Cashier       string               `json:"cashier"`
	Lines         []DocumentLine       `json:"lines"`
	Adjustments   []DocumentAdjustment `json:"adjustments"`
	Tax           []TaxLine            `json:"tax"`
	TaxBase       string               `json:"tax_base"`
	TaxTotal      string               `json:"tax_total"`
	Total         string               `json:"total"`
	Payments      []DocumentPayment    `json:"payments"`
	Paid          string               `json:"paid"`
	Tips          string               `json:"tips"`
	Balance       string               `json:"balance"`
	Payment       *DocumentPayment     `json:"payment"`
	Content       string               `json:"content"`
}

func (handler *service) buildDocument(finance *Finance, kind string, payment *PaymentRecord, width int) (*CommercialDocument, error) {
	var establishment, timezone, cashier string
	err := handler.tx.QueryRow(handler.ctx, "SELECT e.name,e.timezone,u.display_name FROM app.establishment e, app.users u WHERE u.id=$1",
		handler.request.Actor.UserID).Scan(&establishment, &timezone, &cashier)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	issued := time.Now().UTC()
	context := "Para llevar"
	if finance.Mode == "dine_in" {
		context = "Salón"
		if finance.TableName != nil {
			context = "Mesa " + *finance.TableName
		}
	}
	document := &CommercialDocument{
		Kind: kind, Establishment: establishment, Folio: finance.Folio, AccountName: finance.Name, Context: context,
		IssuedAt: issued, LocalTime: issued.In(location).Format("02/01/2006 15:04"), Cashier: cashier,
		Lines: []DocumentLine{}, Adjustments: []DocumentAdjustment{}, Tax: finance.Tax, TaxBase: finance.TaxBase,
		TaxTotal: finance.TaxTotal, Total: finance.Total, Payments: []DocumentPayment{}, Paid: finance.Paid,
		Tips: finance.Tips, Balance: finance.Balance,
	}
	for _, line := range finance.Lines {
		amount := zero
		for _, unit := range line.Units {
			amount = amount.Add(decimal.RequireFromString(unit.Total))
		}
		document.Lines = append(document.Lines, DocumentLine{Quantity: line.Quantity, Name: line.Name, Notes: line.Notes, Amount: money(amount)})
	}
	for _, adjustment := range finance.Adjustments {
		if adjustment.Voided {
			continue
		}
		amount := adjustment.Amount
		if adjustment.Kind == "discount" {
			amount = "-" + amount
		}
		document.Adjustments = append(document.Adjustments, DocumentAdjustment{Concept: adjustment.Concept, Amount: amount})
	}
	for _, record := range finance.Payments {
		item := DocumentPayment{
			Method: record.MethodName, Amount: record.Amount, Tip: record.Tip, Reference: record.Reference,
			Received: record.Received, Change: record.Change, ReceivedMXN: record.ReceivedMXN, ReceivedUSD: record.ReceivedUSD,
			USDRate: record.USDRate, ChangeCurrency: record.ChangeCurrency, ChangeUSD: record.ChangeUSD,
		}
		document.Payments = append(document.Payments, item)
		if payment != nil && record.ID == payment.ID {
			copied := item
			document.Payment = &copied
		}
	}
	document.Content = renderDocument(document, width)
	return document, nil
}

// printable elimina controles e invisibles: solo el controlador de impresión genera comandos.
func printable(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, value)
}

func fit(text string, width int) []string {
	runes := []rune(printable(text))
	lines := []string{}
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	return append(lines, string(runes))
}

// row alinea un concepto a la izquierda y un importe a la derecha dentro del ancho del papel.
func row(left, right string, width int) []string {
	left = printable(left)
	right = printable(right)
	space := width - len([]rune(right)) - 1
	if space < 4 {
		return append(fit(left, width), right)
	}
	parts := fit(left, space)
	last := parts[len(parts)-1]
	parts[len(parts)-1] = last + strings.Repeat(" ", width-len([]rune(last))-len([]rune(right))) + right
	return parts
}

func renderDocument(document *CommercialDocument, width int) string {
	lines := []string{}
	add := func(values ...string) {
		lines = append(lines, values...)
	}
	divider := strings.Repeat("-", width)
	add(fit(document.Establishment, width)...)
	if document.Kind == "precheck" {
		add("PRECUENTA")
	} else {
		add("TICKET DE PAGO")
	}
	add(fit("Folio "+decimal.NewFromInt(document.Folio).String()+" · "+document.AccountName, width)...)
	add(fit(document.Context+" · "+document.LocalTime, width)...)
	add(fit("Atendió: "+document.Cashier, width)...)
	add(divider)
	for _, line := range document.Lines {
		add(row(decimal.NewFromInt(int64(line.Quantity)).String()+" x "+line.Name, "$"+line.Amount, width)...)
		if strings.TrimSpace(line.Notes) != "" {
			add(fit("   Nota: "+line.Notes, width)...)
		}
	}
	for _, adjustment := range document.Adjustments {
		add(row(adjustment.Concept, "$"+adjustment.Amount, width)...)
	}
	add(divider)
	for _, tax := range document.Tax {
		add(row("Subtotal sin IVA "+decimal.NewFromInt(int64(tax.Rate)).String()+" %", "$"+tax.Base, width)...)
		add(row("IVA "+decimal.NewFromInt(int64(tax.Rate)).String()+" %", "$"+tax.Tax, width)...)
	}
	add(row("TOTAL", "$"+document.Total, width)...)
	if document.Payment != nil {
		add(divider)
		add("ESTE PAGO")
		add(row(document.Payment.Method, "$"+document.Payment.Amount, width)...)
		if document.Payment.Reference != "" {
			add(fit("   Ref. "+document.Payment.Reference, width)...)
		}
		if document.Payment.ReceivedUSD != nil {
			add(cashWithDollars(*document.Payment, width)...)
		} else if document.Payment.Received != nil {
			add(row("   Efectivo recibido", "$"+*document.Payment.Received, width)...)
			add(row("   Cambio", "$"+document.Payment.Change, width)...)
		}
		if document.Payment.Tip != "0.00" {
			add(row("   Propina (no es venta)", "$"+document.Payment.Tip, width)...)
		}
	}
	if len(document.Payments) > 0 {
		add(divider)
		add("PAGOS REGISTRADOS")
		for _, payment := range document.Payments {
			add(row(payment.Method, "$"+payment.Amount, width)...)
		}
		add(row("Pagado", "$"+document.Paid, width)...)
		if document.Tips != "0.00" {
			add(row("Propinas", "$"+document.Tips, width)...)
		}
	}
	if document.Balance != "0.00" {
		add(row("SALDO PENDIENTE", "$"+document.Balance, width)...)
	}
	add(divider)
	add(fit("Documento comercial. No es un comprobante fiscal (CFDI).", width)...)
	return strings.Join(lines, "\n")
}

// cashWithDollars detalla un efectivo con dólares: lo recibido en cada moneda, el tipo de cambio
// del turno, el total en pesos y el cambio con la moneda en que se entregó.
func cashWithDollars(payment DocumentPayment, width int) []string {
	lines := []string{}
	lines = append(lines, row("   Pesos recibidos", "$"+*payment.ReceivedMXN, width)...)
	lines = append(lines, row("   Dólares recibidos", "US$"+*payment.ReceivedUSD, width)...)
	lines = append(lines, row("   Tipo de cambio", "$"+*payment.USDRate+"/US$", width)...)
	if payment.Received != nil {
		lines = append(lines, row("   Total recibido", "$"+*payment.Received, width)...)
	}
	if payment.ChangeCurrency != nil && *payment.ChangeCurrency == currencyUSD && payment.ChangeUSD != nil {
		lines = append(lines, row("   Cambio en dólares", "US$"+*payment.ChangeUSD, width)...)
		lines = append(lines, row("   Equivale a", "$"+payment.Change, width)...)
	} else {
		lines = append(lines, row("   Cambio en pesos", "$"+payment.Change, width)...)
	}
	return lines
}

func paperWidth(columns int) int {
	if columns == 58 {
		return 30
	}
	return 42
}

// requirePrintPermission valida antes de escribir el permiso para imprimir el ticket solicitado.
func (handler *service) requirePrintPermission(print *bool) error {
	if print != nil && *print {
		return handler.request.Actor.Require("payments.tickets.print")
	}
	return nil
}

// enqueueDocument encola un documento comercial en la impresora de tickets configurada.
func (handler *service) enqueueDocument(finance *Finance, kind string, payment *PaymentRecord) (map[string]any, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if settings.ReceiptPrinterID == nil {
		return map[string]any{"requested": true, "status": "not_configured",
			"message": "No hay impresora de tickets configurada; el pago quedó registrado."}, nil
	}
	var columns int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT paper_width FROM app.kitchen_printers WHERE id=$1", *settings.ReceiptPrinterID).Scan(&columns); err != nil {
		return nil, err
	}
	document, err := handler.buildDocument(finance, kind, payment, paperWidth(columns))
	if err != nil {
		return nil, err
	}
	job, ticketID, err := kitchen.EnqueueCommercial(handler.ctx, handler.tx, kitchen.CommercialPrint{
		Kind: kind, Document: document, Content: document.Content, PrinterID: *settings.ReceiptPrinterID,
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	var paymentID *string
	if payment != nil {
		paymentID = &payment.ID
	}
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_documents(ticket_id,account_id,payment_id,kind,actor_id,device_id)
        VALUES ($1,$2,$3,$4,$5,$6)`, ticketID, finance.AccountID, paymentID, kind, handler.request.Actor.UserID, handler.request.Actor.DeviceID); err != nil {
		return nil, err
	}
	return map[string]any{"requested": true, "status": job.State, "job_id": job.ID, "ticket_id": ticketID, "message": job.LastError}, nil
}

// printReceiptIfRequested encola el ticket de un pago cuando el cajero lo solicita o, sin
// indicación expresa, según la preferencia del establecimiento. Un fallo de impresión nunca
// revierte el pago: el trabajo queda en la cola durable con su estado.
func (handler *service) printReceiptIfRequested(finance *Finance, payment *PaymentRecord, print *bool) (map[string]any, error) {
	requested := false
	if print != nil {
		requested = *print
	} else {
		settings, err := handler.settings()
		if err != nil {
			return nil, err
		}
		requested = settings.PrintReceiptDefault && handler.request.Actor.Can("payments.tickets.print")
	}
	if !requested || payment == nil {
		return map[string]any{"requested": false}, nil
	}
	return handler.enqueueDocument(finance, "receipt", payment)
}

// precheck devuelve la precuenta sin registrar ninguna venta ni pago.
func (handler *service) precheck(parts []string) (any, error) {
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	return handler.buildDocument(finance, "precheck", nil, 42)
}

func (handler *service) printPrecheck(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	finance, err := loadFinance(handler.ctx, handler.tx, parts[1])
	if err != nil {
		return nil, err
	}
	result, err := handler.enqueueDocument(finance, "precheck", nil)
	if err != nil {
		return nil, err
	}
	if result["status"] == "not_configured" {
		return nil, conflict("PRINTER_NOT_CONFIGURED", "No hay impresora de tickets configurada en Cobros.")
	}
	if err := events.Append(handler.ctx, handler.tx, "payments.document.printed", handler.scope(finance), &finance.AccountID, nil); err != nil {
		return nil, err
	}
	return result, handler.audit(handler.ctx, handler.tx, "payments.precheck_printed", "account", &finance.AccountID, nil, result)
}

// reprintDocument solicita otra copia idéntica de un ticket o precuenta, sin duplicar el pago.
func (handler *service) reprintDocument(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		TicketID    string `json:"ticket_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.TicketID); err != nil {
		return nil, err
	}
	var found bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.payment_documents WHERE ticket_id=$1 AND account_id=$2)",
		input.TicketID, parts[1]).Scan(&found); err != nil {
		return nil, err
	}
	if !found {
		return nil, notFound("El documento no pertenece a esta cuenta.")
	}
	job, err := kitchen.ReprintCommercial(handler.ctx, handler.tx, input.TicketID, handler.operationID, handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return nil, err
	}
	accountID := parts[1]
	if err := handler.audit(handler.ctx, handler.tx, "payments.document_reprinted", "account", &accountID, nil, map[string]string{"ticket_id": input.TicketID, "job_id": job.ID}); err != nil {
		return nil, err
	}
	return map[string]any{"status": job.State, "job_id": job.ID, "ticket_id": input.TicketID}, nil
}

type DocumentRecord struct {
	TicketID    string    `json:"ticket_id"`
	Kind        string    `json:"kind"`
	PaymentID   *string   `json:"payment_id"`
	CreatedAt   time.Time `json:"created_at"`
	JobID       *string   `json:"job_id"`
	JobState    *string   `json:"job_state"`
	PrinterName *string   `json:"printer_name"`
	Copies      int       `json:"copies"`
}
