package payments

import (
	"encoding/json"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/kitchen"
)

// Conversión de la precuenta y el ticket a los datos tipados del renderizador único (etapa 9B) e
// impresión de un ticket o precuenta histórico con la plantilla vigente.

// CommercialData convierte un documento comercial guardado en los datos del renderizador. El nombre
// del establecimiento es el copiado en el documento; la identidad, la copia guardada si existe.
func CommercialData(document *CommercialDocument) documents.CommercialData {
	identity := documents.Identity{}
	if document.Identity != nil {
		identity = *document.Identity
	}
	identity.Name = document.Establishment
	data := documents.CommercialData{
		Kind: documents.Kind(document.Kind), Title: document.Title, Folio: document.Folio, AccountName: document.AccountName, Context: document.Context,
		LocalTime: document.LocalTime, Cashier: document.Cashier, Total: document.Total, USDTotal: document.USDTotal,
		USDRate: document.USDRate, Paid: document.Paid, Tips: document.Tips, Balance: document.Balance, Identity: identity,
		Lines:       make([]documents.CommercialLine, 0, len(document.Lines)),
		Adjustments: make([]documents.CommercialAdjustment, 0, len(document.Adjustments)),
		Tax:         make([]documents.CommercialTax, 0, len(document.Tax)),
		Payments:    make([]documents.CommercialPayment, 0, len(document.Payments)),
	}
	for _, line := range document.Lines {
		data.Lines = append(data.Lines, documents.CommercialLine{Quantity: line.Quantity, Name: line.Name, Notes: line.Notes, Amount: line.Amount})
	}
	for _, adjustment := range document.Adjustments {
		data.Adjustments = append(data.Adjustments, documents.CommercialAdjustment{Concept: adjustment.Concept, Reason: adjustment.Reason, Amount: adjustment.Amount})
	}
	for _, tax := range document.Tax {
		data.Tax = append(data.Tax, documents.CommercialTax{Rate: tax.Rate, Base: tax.Base, Tax: tax.Tax})
	}
	for _, payment := range document.Payments {
		data.Payments = append(data.Payments, commercialPayment(payment))
	}
	if document.Payment != nil {
		payment := commercialPayment(*document.Payment)
		data.Payment = &payment
	}
	return data
}

func commercialPayment(payment DocumentPayment) documents.CommercialPayment {
	return documents.CommercialPayment{
		Method: payment.Method, Amount: payment.Amount, Tip: payment.Tip, Reference: payment.Reference, Received: payment.Received,
		Change: payment.Change, ReceivedMXN: payment.ReceivedMXN, ReceivedUSD: payment.ReceivedUSD, USDRate: payment.USDRate,
		ChangeCurrency: payment.ChangeCurrency, ChangeUSD: payment.ChangeUSD,
	}
}

// renderCommercial renderiza un documento comercial con una versión de plantilla al ancho indicado
// (papel y columnas de la impresora). identity, si no es nil, sustituye la identidad copiada
// (impresión con la plantilla vigente).
func renderCommercial(version *documents.Version, document *CommercialDocument, identity *documents.Identity, width documents.RenderOptions) (*documents.Layout, error) {
	data := CommercialData(document)
	if identity != nil {
		data.Identity = *identity
	}
	return documents.Render(version.Kind, version.Template, documents.Data{Commercial: &data}, width)
}

// rerenderDocument imprime un ticket o precuenta histórico con la plantilla vigente y la identidad
// vigente (POST payments/accounts/{id}/documents/rerender). Es una acción distinta de la
// reimpresión exacta, con su propio permiso y auditoría: crea un ticket «document_rerender» y su
// trabajo, nunca un payment_documents nuevo, de modo que no cambia «Esperando pago» ni ningún cobro.
func (handler *service) rerenderDocument(parts []string) (any, error) {
	if err := handler.request.Actor.Require(documents.PermissionRenderCurrent); err != nil {
		return nil, err
	}
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
	var kind string
	err := handler.tx.QueryRow(handler.ctx, "SELECT kind FROM app.payment_documents WHERE ticket_id=$1 AND account_id=$2",
		input.TicketID, parts[1]).Scan(&kind)
	if isNoRows(err) {
		return nil, notFound("El documento no pertenece a esta cuenta.")
	}
	if err != nil {
		return nil, err
	}
	var stored []byte
	if err := handler.tx.QueryRow(handler.ctx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", input.TicketID).Scan(&stored); err != nil {
		return nil, err
	}
	var document CommercialDocument
	if err := json.Unmarshal(stored, &document); err != nil {
		return nil, err
	}
	printer, err := kitchen.RerenderTarget(handler.ctx, handler.tx, input.TicketID)
	if err != nil {
		return nil, err
	}
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, documents.Kind(kind))
	if err != nil {
		return nil, err
	}
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	layout, err := renderCommercial(version, &document, &identity, printer.RenderOptions())
	if err != nil {
		return nil, err
	}
	accountID := parts[1]
	ticket := map[string]any{
		"kind": documents.TicketKindRerender, "source_ticket_id": input.TicketID, "source_kind": kind, "account_id": accountID,
		"document_kind": version.Kind, "template_version_id": version.ID, "content": layout.Text(),
	}
	versionID := version.ID
	job, ticketID, err := kitchen.EnqueueRendered(handler.ctx, handler.tx, kitchen.RenderedPrint{
		Kind: documents.TicketKindRerender, Document: ticket, Layout: layout, TemplateVersionID: &versionID, Printer: printer,
		History:     "print.rerendered",
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"status": job.State, "job_id": job.ID, "ticket_id": ticketID, "source_ticket_id": input.TicketID, "template_version_id": version.ID,
	}
	return result, handler.audit(handler.ctx, handler.tx, "payments.document_rerendered", "account", &accountID, nil, map[string]string{
		"source_ticket_id": input.TicketID, "ticket_id": ticketID, "job_id": job.ID, "template_version_id": version.ID,
	})
}
