package payments

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/documents"
)

// Documentos reales para la vista previa del diseñador (etapa 9B). Solo leen y aplican las mismas
// reglas de consulta que Cobros y Caja: quien no puede ver un documento en su módulo de origen
// tampoco lo ve en el diseñador, y el esperado de caja sigue la regla de siempre (permiso y fuera
// del arqueo). Quien llama ya tomó el lock 7820148305 compartido.

func readerFor(ctx context.Context, tx pgx.Tx, actor Actor) *service {
	return &service{ctx: ctx, tx: tx, request: Request{Actor: actor}}
}

// CommercialPreviewSource devuelve los datos de una precuenta o ticket emitido. Exige consultar
// cobros y la cuenta (y para llevar, su permiso).
func CommercialPreviewSource(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string) (*documents.CommercialData, documents.Kind, error) {
	if err := actor.Require("payments.read"); err != nil {
		return nil, "", err
	}
	var kind, accountID string
	err := tx.QueryRow(ctx, "SELECT kind,account_id FROM app.payment_documents WHERE ticket_id=$1", ticketID).Scan(&kind, &accountID)
	if isNoRows(err) {
		return nil, "", notFound("El documento no existe.")
	}
	if err != nil {
		return nil, "", err
	}
	handler := readerFor(ctx, tx, actor)
	if err := handler.authorizeAccount(accountID); err != nil {
		return nil, "", err
	}
	var stored []byte
	if err := tx.QueryRow(ctx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&stored); err != nil {
		return nil, "", err
	}
	var document CommercialDocument
	if err := json.Unmarshal(stored, &document); err != nil {
		return nil, "", err
	}
	data := CommercialData(&document)
	return &data, documents.Kind(kind), nil
}

// CashPreviewSource devuelve los datos de un comprobante o corte impreso, leídos de su corte
// (snapshot inmutable) o de su operación, con las reglas de consulta de su turno. includeExpected
// indica si quien consulta puede ver el esperado y las diferencias ahora.
func CashPreviewSource(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string) (*documents.CashData, bool, documents.Kind, error) {
	if err := actor.Require(permissionShiftsRead); err != nil {
		return nil, false, "", err
	}
	handler := readerFor(ctx, tx, actor)
	records, err := handler.listCashDocuments("d.ticket_id=$1", ticketID)
	if err != nil {
		return nil, false, "", err
	}
	if len(records) == 0 {
		return nil, false, "", notFound("El documento de caja no existe.")
	}
	record := records[0]
	var source cashDocumentSource
	if record.ReportID != nil {
		source, err = handler.reportSource(*record.ReportID)
	} else {
		if err := actor.Require(permissionMovementsRead); err != nil {
			return nil, false, "", err
		}
		source, err = handler.operationSource(*record.CashOperationID)
	}
	if err != nil {
		return nil, false, "", err
	}
	header, err := handler.readDocumentHeader()
	if err != nil {
		return nil, false, "", err
	}
	includeExpected := handler.reportIncludesExpected(source)
	data := cashData(header, source, includeExpected)
	identity, err := documents.LoadIdentity(ctx, tx)
	if err != nil {
		return nil, false, "", err
	}
	identity.Name = header.establishment
	data.Identity = identity
	kind, _ := documents.CashKind(source.kind)
	return &data, includeExpected, kind, nil
}

// RecentCommercialDocuments lista las precuentas o tickets recientes que el actor puede consultar.
func RecentCommercialDocuments(ctx context.Context, tx pgx.Tx, actor Actor, kind string) ([]documents.RecentDocument, error) {
	if err := actor.Require("payments.read", "accounts.read"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT d.ticket_id,d.created_at,a.folio,a.name FROM app.payment_documents d
        JOIN app.restaurant_accounts a ON a.id=d.account_id
        WHERE d.kind=$1 AND (a.mode<>'takeaway' OR $2) ORDER BY d.created_at DESC,d.ticket_id DESC LIMIT 20`,
		kind, actor.Can("takeaway.read"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []documents.RecentDocument{}
	for rows.Next() {
		var item documents.RecentDocument
		var folio int64
		var name string
		if err := rows.Scan(&item.TicketID, &item.CreatedAt, &folio, &name); err != nil {
			return nil, err
		}
		item.Kind = documents.Kind(kind)
		item.Label = "Folio " + strconv.FormatInt(folio, 10) + " · " + name
		result = append(result, item)
	}
	return result, rows.Err()
}

// RecentCashDocuments lista los comprobantes o cortes recientes de un tipo (cash_documents.kind) de
// los turnos que el actor puede consultar. La etiqueta no lleva importes.
func RecentCashDocuments(ctx context.Context, tx pgx.Tx, actor Actor, documentKind string) ([]documents.RecentDocument, error) {
	if err := actor.Require(permissionShiftsRead); err != nil {
		return nil, err
	}
	kind, found := documents.CashKind(documentKind)
	if !found {
		return nil, invalid("El tipo de documento de caja no existe.")
	}
	handler := readerFor(ctx, tx, actor)
	header, err := handler.readDocumentHeader()
	if err != nil {
		return nil, err
	}
	records, err := handler.listCashDocuments("d.kind=$1 AND d.created_at > now() - interval '120 days'", documentKind)
	if err != nil {
		return nil, err
	}
	result := []documents.RecentDocument{}
	for index := len(records) - 1; index >= 0 && len(result) < 20; index-- {
		record := records[index]
		shift, err := handler.readShift("s.id=$1", record.ShiftID)
		if err != nil {
			return nil, err
		}
		if shift == nil || handler.shiftReadable(shift) != nil {
			continue
		}
		result = append(result, documents.RecentDocument{
			TicketID: record.TicketID, Kind: kind, CreatedAt: record.CreatedAt,
			Label: shift.RegisterName + " · " + header.local(record.CreatedAt) + " · " + record.CreatedByName,
		})
	}
	return result, nil
}

// shiftReadable aplica sin escribir las reglas de readableShift a un turno ya leído.
func (handler *service) shiftReadable(shift *Shift) error {
	if err := handler.requireShiftAccess(shift); err != nil {
		return err
	}
	closedByActor := shift.ClosedBy != nil && *shift.ClosedBy == handler.request.Actor.UserID
	if shift.State == shiftClosed && !closedByActor {
		return handler.request.Actor.Require(permissionHistoryRead)
	}
	return nil
}
