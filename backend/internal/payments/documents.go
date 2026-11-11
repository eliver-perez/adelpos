package payments

import (
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/features"
	"local/ad-el-pos/server/internal/kitchen"
)

// Precuentas y tickets: los datos se separan de la plantilla y del transporte, que pertenece a la
// cola durable de cocina. Desde la etapa 9B el texto lo produce el renderizador único de
// internal/documents con la plantilla activa del tipo; el documento guarda la versión usada y una
// copia de la identidad del establecimiento. La reimpresión exacta copia el trabajo original.

type DocumentLine struct {
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Amount   string `json:"amount"`
}

// DocumentAdjustment es una línea de ajuste del documento. Reason es el nombre del motivo del
// catálogo de un descuento o cortesía (etapa 7.1); vacío en cargos y descuentos sin motivo.
type DocumentAdjustment struct {
	Concept string `json:"concept"`
	Reason  string `json:"reason,omitempty"`
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

// CommercialDocument es la información de una precuenta o ticket. No es un CFDI. USDTotal y
// USDRate son el total equivalente en dólares (etapa 7.1, informativo): solo con usd_cash activa y
// tipo de cambio del turno disponible; nil en otro caso. No alteran ningún importe.
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
	USDTotal      *string              `json:"usd_total,omitempty"`
	USDRate       *string              `json:"usd_rate,omitempty"`
	Payment       *DocumentPayment     `json:"payment"`
	Content       string               `json:"content"`
	// Etapa 9B: versión de plantilla con la que se renderizó y copia de la identidad usada.
	TemplateVersionID *string             `json:"template_version_id,omitempty"`
	Identity          *documents.Identity `json:"identity,omitempty"`
}

// buildDocument arma la precuenta o el ticket y lo renderiza con la plantilla activa de su tipo al
// ancho de papel indicado. Devuelve también la IR para encolarla.
func (handler *service) buildDocument(finance *Finance, kind string, payment *PaymentRecord, paper int) (*CommercialDocument, *documents.Layout, error) {
	var establishment, timezone, cashier string
	err := handler.tx.QueryRow(handler.ctx, "SELECT e.name,e.timezone,u.display_name FROM app.establishment e, app.users u WHERE u.id=$1",
		handler.request.Actor.UserID).Scan(&establishment, &timezone, &cashier)
	if err != nil {
		return nil, nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, nil, err
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
		item := DocumentAdjustment{Concept: adjustment.Concept, Amount: amount}
		if adjustment.ReasonName != nil {
			item.Reason = *adjustment.ReasonName
		}
		document.Adjustments = append(document.Adjustments, item)
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
	if err := handler.addDollarTotal(document, kind, payment); err != nil {
		return nil, nil, err
	}
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, documents.Kind(kind))
	if err != nil {
		return nil, nil, err
	}
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, nil, err
	}
	identity.Name = establishment
	versionID := version.ID
	document.TemplateVersionID = &versionID
	document.Identity = &identity
	layout, err := renderCommercial(version, document, nil, paper)
	if err != nil {
		return nil, nil, err
	}
	document.Content = layout.Text()
	return document, layout, nil
}

// addDollarTotal agrega el total equivalente en dólares (decisión 16) cuando la función usd_cash
// está activa y hay tipo de cambio: el de la precuenta es el del turno activo (abierto o en
// arqueo) de la caja de la terminal que la imprime o consulta, y el del ticket, el del turno en
// que se registró su pago. Es informativo: se calcula con mxnToUSD sobre el total que ya calculó
// Go y no cambia ningún importe del documento.
func (handler *service) addDollarTotal(document *CommercialDocument, kind string, payment *PaymentRecord) error {
	enabled, err := features.Enabled(handler.ctx, handler.tx, features.USDCash)
	if err != nil || !enabled {
		return err
	}
	var rate *string
	if kind == "receipt" && payment != nil {
		if err := handler.tx.QueryRow(handler.ctx, "SELECT usd_rate::text FROM app.cash_shifts WHERE id=$1", payment.ShiftID).Scan(&rate); err != nil {
			return err
		}
	} else if kind == "precheck" {
		register, err := handler.deviceRegister()
		if err != nil || register == nil {
			return err
		}
		shift, err := handler.activeShift(register.ID)
		if err != nil || shift == nil {
			return err
		}
		rate = shift.USDRate
	}
	if rate == nil {
		return nil
	}
	value, err := decimal.NewFromString(*rate)
	if err != nil || value.Sign() <= 0 {
		return nil
	}
	total := money(mxnToUSD(decimal.RequireFromString(document.Total), value))
	formatted := money(value)
	document.USDTotal = &total
	document.USDRate = &formatted
	return nil
}

// paperWidth normaliza el ancho de papel de una impresora (58 u 80 mm).
func paperWidth(paper int) int {
	if paper == 58 {
		return 58
	}
	return 80
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
	var paper int
	if err := handler.tx.QueryRow(handler.ctx, "SELECT paper_width FROM app.kitchen_printers WHERE id=$1", *settings.ReceiptPrinterID).Scan(&paper); err != nil {
		return nil, err
	}
	document, layout, err := handler.buildDocument(finance, kind, payment, paperWidth(paper))
	if err != nil {
		return nil, err
	}
	job, ticketID, err := kitchen.EnqueueCommercial(handler.ctx, handler.tx, kitchen.CommercialPrint{
		Kind: kind, Document: document, Content: document.Content, Layout: layout, TemplateVersionID: document.TemplateVersionID,
		PrinterID:   *settings.ReceiptPrinterID,
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	var paymentID *string
	if payment != nil {
		paymentID = &payment.ID
	}
	// Etapa 9A (§33): la precuenta guarda la revisión de la cuenta con la que se armó el documento y
	// el número de ajustes, ambos leídos bajo el lock exclusivo de cobros que esta transacción ya
	// tiene; es vigente mientras la cuenta conserve los dos (AccountStandingSQL). Un cambio que espera
	// el lock mientras se imprime incrementa la revisión después y la deja obsoleta, sin comparar
	// instantes de inicio de transacción. Los tickets de pago no guardan estos datos.
	var accountRevision, adjustments *int
	if kind == "precheck" {
		count, err := adjustmentCount(handler.ctx, handler.tx, finance.AccountID)
		if err != nil {
			return nil, err
		}
		revision := finance.Revision
		accountRevision, adjustments = &revision, &count
	}
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.payment_documents
        (ticket_id,account_id,payment_id,kind,actor_id,device_id,account_revision,adjustment_count)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, ticketID, finance.AccountID, paymentID, kind, handler.request.Actor.UserID,
		handler.request.Actor.DeviceID, accountRevision, adjustments); err != nil {
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
	// La vista previa usa la misma plantilla que la impresión, al ancho de 80 mm como antes.
	document, _, err := handler.buildDocument(finance, "precheck", nil, 80)
	return document, err
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
