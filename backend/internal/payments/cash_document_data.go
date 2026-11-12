package payments

import (
	"encoding/json"
	"strings"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/kitchen"
)

// Conversión de comprobantes y cortes a los datos tipados del renderizador único (etapa 9B) e
// impresión de un documento de caja histórico con la plantilla vigente.

// cashData convierte un corte o una operación de efectivo en los datos del renderizador, con las
// fechas en la zona horaria del establecimiento. El esperado y las diferencias solo se copian si el
// documento puede incluirlos (includeExpected): así nunca quedan en los datos de un documento que
// no los imprime.
func cashData(header *documentHeader, source cashDocumentSource, includeExpected bool) documents.CashData {
	kind, _ := documents.CashKind(source.kind)
	data := documents.CashData{Kind: kind, Identity: documents.Identity{Name: header.establishment}}
	if source.report == nil {
		operation := source.operation
		data.Register = operation.RegisterName
		data.BusinessDate = displayDate(source.shift.BusinessDate)
		data.Voucher = voucherData(header, operation)
		return data
	}
	report := source.report
	shift := source.shift
	control := report.Snapshot.Control
	data.Register = control.RegisterName
	data.BusinessDate = displayDate(control.BusinessDate)
	cutAt := header.local(report.CoveredTo)
	date, clock, _ := strings.Cut(cutAt, " ")
	cut := &documents.CashCutData{
		Sequence: report.Sequence, OpenedByName: control.OpenedByName, OpenedAt: header.local(shift.OpenedAt), CutAt: cutAt,
		CutDate: date, CutTime: clock, CreatedByName: report.CreatedByName, DeviceName: report.DeviceName,
		OpeningFloat: control.OpeningFloat, FloatUSD: control.FloatUSD, USDRate: control.USDRate,
		Totals: cashTotals(report.Snapshot.Accumulated), Dollars: shiftHandlesDollars(shift),
	}
	if report.Kind == "final" {
		cut.ClosedByName = shift.ClosedByName
	} else if report.PreviousReportID != nil {
		previous := header.local(report.CoveredFrom)
		cut.PreviousCutAt = &previous
	}
	if declared := report.Snapshot.Declared; declared != nil {
		cut.Declared = &documents.CashDeclared{
			CountedMXN: declared.CountedMXN, CountedUSD: declared.CountedUSD, ExtraMXN: declared.ExtraMXN, ExtraUSD: declared.ExtraUSD,
			CashierNotes: declared.CashierNotes,
		}
	}
	if sensitive := report.Snapshot.Sensitive; includeExpected && sensitive != nil {
		cut.Sensitive = &documents.CashSensitive{
			ExpectedMXN: sensitive.Expected.MXN, ExpectedUSD: sensitive.Expected.USD, TotalEquivalentMXN: sensitive.Expected.TotalEquivalentMXN,
		}
		if differences := sensitive.Differences; differences != nil {
			cut.Sensitive.Differences = &documents.CashDifferences{
				MXN: differences.MXN, USD: differences.USD, ResultMXN: differences.ResultMXN, ResultUSD: differences.ResultUSD,
				EquivalentMXN: differences.EquivalentMXN,
			}
		}
	}
	data.Cut = cut
	return data
}

func voucherData(header *documentHeader, operation *CashOperation) *documents.CashVoucherData {
	local := header.local(operation.CreatedAt)
	date, clock, _ := strings.Cut(local, " ")
	voucher := &documents.CashVoucherData{
		LocalDateTime: local, LocalDate: date, LocalTime: clock, TypeLabel: operation.TypeLabel, Concept: operation.Concept,
		Currency: operation.Currency, Amount: operation.Amount, USDRate: operation.USDRate, EquivalentMXN: operation.EquivalentMXN,
		Reference: operation.Reference, Notes: operation.Notes, Exceptional: operation.Exceptional, Reason: operation.Reason,
		ActorName: operation.ActorName, DeviceName: operation.DeviceName, AuthorizedByName: operation.AuthorizedByName,
		AuthorizationMode: operation.AuthorizationMode, Corrected: operation.Status == "corrected",
	}
	if operation.Kind == operationExpense {
		voucher.Category = operation.CategoryName
	}
	return voucher
}

func cashTotals(totals shiftTotals) documents.CashTotals {
	converted := documents.CashTotals{
		Payments: totals.Payments, Sales: totals.Sales, Tips: totals.Tips, Refunded: totals.Refunded, Discounts: totals.Discounts,
		DiscountCount: totals.DiscountCount, Courtesies: totals.Courtesies, CourtesyCount: totals.CourtesyCount,
		Regularizations: totals.Regularizations,
		Methods:         make([]documents.CashMethodTotals, 0, len(totals.Methods)),
		Cash:            make([]documents.CashCurrencyTotals, 0, len(totals.Cash)),
	}
	for _, method := range totals.Methods {
		converted.Methods = append(converted.Methods, documents.CashMethodTotals{
			Name: method.MethodName, Sales: method.Sales, Tips: method.Tips, Refunds: method.Refunds,
		})
	}
	for _, cash := range totals.Cash {
		converted.Cash = append(converted.Cash, documents.CashCurrencyTotals{
			Currency: cash.Currency, Income: cash.Income, Withdrawals: cash.Withdrawals, Expenses: cash.Expenses,
			CorrectionsIn: cash.CorrectionsIn, CorrectionsOut: cash.CorrectionsOut,
		})
	}
	return converted
}

// storedCashTicket es lo que guarda el ticket de un documento de caja emitido desde la 9B.
type storedCashTicket struct {
	Data *documents.CashData `json:"data"`
}

// rerenderCashDocument imprime un documento de caja histórico con la plantilla vigente
// (POST payments/cash-documents/{id}/rerender). Exige reimprimir documentos de caja y
// documents.history.render_current; si el documento incluye el esperado, además consultarlo y que
// el turno no esté en arqueo. Usa los datos estructurados que guarda el ticket desde la 9B: los
// documentos anteriores solo admiten la reimpresión exacta. Nunca crea un cash_documents nuevo.
func (handler *service) rerenderCashDocument(parts []string) (any, error) {
	if err := handler.request.Actor.Require(documents.PermissionRenderCurrent); err != nil {
		return nil, err
	}
	if err := decodeOperationOnly(handler.request.Body); err != nil {
		return nil, err
	}
	records, err := handler.listCashDocuments("d.id=$1", parts[1])
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, notFound("El documento de caja no existe.")
	}
	record := records[0]
	shift, err := handler.readableShift(record.ShiftID)
	if err != nil {
		return nil, err
	}
	if record.IncludesExpected {
		if err := handler.request.Actor.Require(permissionExpectedRead); err != nil {
			return nil, err
		}
		if shift.State == shiftClosing {
			return nil, conflict("SHIFT_CLOSING", "El documento contiene el efectivo esperado y "+shift.RegisterName+
				" está en arqueo de cierre: se imprime cuando termine el arqueo.")
		}
	}
	var stored []byte
	var ticketKind string
	if err := handler.tx.QueryRow(handler.ctx, "SELECT kind,document FROM app.kitchen_tickets WHERE id=$1", record.TicketID).Scan(&ticketKind, &stored); err != nil {
		return nil, err
	}
	var ticket storedCashTicket
	if err := json.Unmarshal(stored, &ticket); err != nil || ticket.Data == nil {
		return nil, conflict("DOCUMENT_DATA_UNAVAILABLE",
			"Este documento se emitió antes de las plantillas de documentos y solo conserva su texto: admite únicamente la reimpresión exacta.")
	}
	printer, err := kitchen.RerenderTarget(handler.ctx, handler.tx, record.TicketID)
	if err != nil {
		return nil, err
	}
	data := *ticket.Data
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	data.Identity = identity
	// Lo protegido solo se imprime si el documento original lo incluía, quien imprime puede verlo y
	// el turno no está en arqueo; en otro caso el bloque imprime «no incluidos».
	includeExpected := data.Cut != nil && data.Cut.Sensitive != nil && handler.expectedVisible(shift)
	version, err := documents.ActiveVersion(handler.ctx, handler.tx, data.Kind)
	if err != nil {
		return nil, err
	}
	options := printer.RenderOptions()
	options.IncludeExpected = includeExpected
	layout, err := documents.Render(data.Kind, version.Template, documents.Data{Cash: &data}, options)
	if err != nil {
		return nil, err
	}
	document := map[string]any{
		"kind": documents.TicketKindRerender, "source_ticket_id": record.TicketID, "source_kind": ticketKind,
		"cash_document_id": record.ID, "document_kind": data.Kind, "shift_id": record.ShiftID, "includes_expected": includeExpected,
		"template_version_id": version.ID, "content": layout.Text(),
	}
	versionID := version.ID
	job, ticketID, err := kitchen.EnqueueRendered(handler.ctx, handler.tx, kitchen.RenderedPrint{
		Kind: documents.TicketKindRerender, Document: document, Layout: layout, TemplateVersionID: &versionID, Printer: printer,
		History:     "print.rerendered",
		OperationID: handler.operationID, ActorID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID,
	})
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.document.rerendered", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	result := PrintOutcome{
		Requested: true, Status: job.State, DocumentID: record.ID, TicketID: ticketID, JobID: job.ID,
		IncludesExpected: includeExpected, Message: job.LastError,
	}
	return result, handler.audit(handler.ctx, handler.tx, "cash.document_rerendered", "cash_shift", &shift.ID, nil, map[string]any{
		"document_id": record.ID, "kind": record.Kind, "job_id": job.ID, "ticket_id": ticketID, "template_version_id": version.ID,
	})
}
