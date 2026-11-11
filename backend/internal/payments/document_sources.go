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

// CashPreview es lo que necesita la vista previa del diseñador para un documento de caja real: sus
// datos, si incluye el esperado, su tipo y, para los documentos anteriores a la 9B, el aviso de que
// se leyeron el corte o la operación actuales.
type CashPreview struct {
	Data            *documents.CashData
	IncludeExpected bool
	Kind            documents.Kind
	Notice          string
}

// legacyCashPreviewNotice explica que un documento anterior a la 9B no guardó sus datos.
const legacyCashPreviewNotice = "Este documento se emitió antes de las plantillas de documentos y no guardó sus datos: " +
	"la vista previa usa los datos actuales de su corte u operación y puede no coincidir con lo que se imprimió."

// CashPreviewSource devuelve los datos de un comprobante o corte impreso con las reglas de consulta
// de su turno (y, en un comprobante, cash.movements.read). Con un documento emitido desde la 9B usa
// los datos que guardó su ticket, los mismos que imprime «Imprimir con la plantilla vigente», y la
// misma regla del esperado: solo si el original lo incluía, quien consulta puede verlo y el turno no
// está en arqueo. Solo un documento anterior a la 9B (sin datos guardados) se lee de su corte
// (snapshot inmutable) o de su operación actuales, con un aviso.
func CashPreviewSource(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string) (*CashPreview, error) {
	if err := actor.Require(permissionShiftsRead); err != nil {
		return nil, err
	}
	handler := readerFor(ctx, tx, actor)
	records, err := handler.listCashDocuments("d.ticket_id=$1", ticketID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, notFound("El documento de caja no existe.")
	}
	record := records[0]
	if record.ReportID == nil {
		if err := actor.Require(permissionMovementsRead); err != nil {
			return nil, err
		}
	}
	shift, err := handler.readableShift(record.ShiftID)
	if err != nil {
		return nil, err
	}
	stored, err := handler.storedCashData(record.TicketID)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		data := *stored
		includeExpected := data.Cut != nil && data.Cut.Sensitive != nil && handler.expectedVisible(shift)
		if data.Cut != nil && !includeExpected {
			// Lo protegido nunca llega al renderizador si no se va a imprimir.
			cut := *data.Cut
			cut.Sensitive = nil
			data.Cut = &cut
		}
		return &CashPreview{Data: &data, IncludeExpected: includeExpected, Kind: data.Kind}, nil
	}
	return handler.legacyCashPreview(record)
}

// storedCashData devuelve los datos estructurados que guarda el ticket de un documento de caja
// emitido desde la 9B, o nil si es anterior.
func (handler *service) storedCashData(ticketID string) (*documents.CashData, error) {
	var stored []byte
	if err := handler.tx.QueryRow(handler.ctx, "SELECT document FROM app.kitchen_tickets WHERE id=$1", ticketID).Scan(&stored); err != nil {
		return nil, err
	}
	var ticket storedCashTicket
	if err := json.Unmarshal(stored, &ticket); err != nil {
		return nil, nil
	}
	return ticket.Data, nil
}

// legacyCashPreview lee un documento anterior a la 9B de su corte o de su operación actuales.
func (handler *service) legacyCashPreview(record CashDocumentRecord) (*CashPreview, error) {
	var source cashDocumentSource
	var err error
	if record.ReportID != nil {
		source, err = handler.reportSource(*record.ReportID)
	} else {
		source, err = handler.operationSource(*record.CashOperationID)
	}
	if err != nil {
		return nil, err
	}
	header, err := handler.readDocumentHeader()
	if err != nil {
		return nil, err
	}
	includeExpected := handler.reportIncludesExpected(source)
	data := cashData(header, source, includeExpected)
	identity, err := documents.LoadIdentity(handler.ctx, handler.tx)
	if err != nil {
		return nil, err
	}
	identity.Name = header.establishment
	data.Identity = identity
	kind, _ := documents.CashKind(source.kind)
	return &CashPreview{Data: &data, IncludeExpected: includeExpected, Kind: kind, Notice: legacyCashPreviewNotice}, nil
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
// los turnos que el actor puede consultar, con los permisos de origen de Caja: cash.shifts.read y,
// para los comprobantes de ingreso, retiro y gasto, también cash.movements.read (el mismo que exige
// su vista previa). El alcance de los turnos (la caja de la terminal o todas con
// cash.shifts.read_all; los cerrados con cash.history.read o para quien los cerró) se resuelve en una
// sola consulta con LIMIT: bajo el lock compartido no hay una consulta por documento. La etiqueta no
// lleva importes.
func RecentCashDocuments(ctx context.Context, tx pgx.Tx, actor Actor, documentKind string) ([]documents.RecentDocument, error) {
	if err := actor.Require(permissionShiftsRead); err != nil {
		return nil, err
	}
	kind, found := documents.CashKind(documentKind)
	if !found {
		return nil, invalid("El tipo de documento de caja no existe.")
	}
	if kind.IsVoucher() {
		if err := actor.Require(permissionMovementsRead); err != nil {
			return nil, err
		}
	}
	handler := readerFor(ctx, tx, actor)
	header, err := handler.readDocumentHeader()
	if err != nil {
		return nil, err
	}
	register, err := handler.deviceRegister()
	if err != nil {
		return nil, err
	}
	var registerID *string
	if register != nil {
		registerID = &register.ID
	}
	rows, err := tx.Query(ctx, `SELECT d.ticket_id,d.created_at,r.name,u.display_name FROM app.cash_documents d
        JOIN app.cash_shifts s ON s.id=d.shift_id JOIN app.registers r ON r.id=s.register_id JOIN app.users u ON u.id=d.created_by
        WHERE d.kind=$1 AND d.created_at > now() - interval '120 days'
          AND ($2 OR s.register_id=$3)
          AND (s.state<>$4 OR $5 OR s.closed_by=$6)
        ORDER BY d.created_at DESC,d.id DESC LIMIT 20`,
		documentKind, actor.Can(permissionShiftsReadAll), registerID, shiftClosed, actor.Can(permissionHistoryRead), actor.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []documents.RecentDocument{}
	for rows.Next() {
		var item documents.RecentDocument
		var registerName, createdBy string
		if err := rows.Scan(&item.TicketID, &item.CreatedAt, &registerName, &createdBy); err != nil {
			return nil, err
		}
		item.Kind = kind
		item.Label = registerName + " · " + header.local(item.CreatedAt) + " · " + createdBy
		result = append(result, item)
	}
	return result, rows.Err()
}
