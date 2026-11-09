package payments

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"local/ad-el-pos/server/internal/events"
)

// Arqueo de cierre y cierre ciego (decisión del propietario, §0-B):
//
//   - Iniciar el arqueo pasa el turno a «Cierre en curso» (closing) con quién, cuándo y desde qué
//     terminal, y crea el borrador del conteo en el servidor. La caja queda pausada: no acepta
//     cobros, devoluciones, ingresos, retiros, gastos ni correcciones. No se mantienen bloqueos de
//     base de datos mientras se cuentan billetes; el estado persistente hace la pausa.
//   - La captura es ciega para todos, incluido el administrador: ninguna respuesta del arqueo
//     contiene el efectivo esperado, diferencias ni un indicador de cercanía.
//   - Cancelar vuelve a «Abierto»; cancelar un arqueo ajeno exige cash.closing.cancel_any.
//   - Confirmar vuelve a verificar, bajo el lock exclusivo 7820148305 y en su transacción, que el
//     turno sigue en arqueo, que el conteo es el revisado y que no se registró ningún movimiento
//     después de iniciar el arqueo; calcula esperado y diferencias por moneda, guarda el corte
//     definitivo y el cierre, y cierra el turno. Las diferencias y las cuentas abiertas no bloquean.
//   - Todas las operaciones son idempotentes (restaurant_operations) y auditadas sin importes
//     esperados.

// ClosingView es el estado del arqueo en curso: solo lo declarado.
type ClosingView struct {
	Shift *Shift     `json:"shift"`
	Count *CountView `json:"count"`
}

// closingShift lee el turno de la ruta y aplica las reglas de acceso de caja.
func (handler *service) closingShift(id string) (*Shift, error) {
	shift, err := handler.readShift("s.id=$1", id)
	if err != nil {
		return nil, err
	}
	if shift == nil {
		return nil, notFound("El turno no existe.")
	}
	if err := handler.requireShiftAccess(shift); err != nil {
		return nil, err
	}
	return shift, nil
}

// requireClosing exige que el turno esté en arqueo de cierre.
func requireClosing(shift *Shift) error {
	if shift.State == shiftClosed {
		return shiftStateError(shift)
	}
	if shift.State != shiftClosing {
		return conflict("SHIFT_NOT_CLOSING", "El turno de "+shift.RegisterName+" no está en arqueo de cierre.")
	}
	return nil
}

// closingView arma la respuesta del arqueo con el turno y el borrador vigentes.
func (handler *service) closingView(shiftID string) (*ClosingView, error) {
	shift, err := handler.readShift("s.id=$1", shiftID)
	if err != nil {
		return nil, err
	}
	count, err := handler.readCount(shiftID, "closing")
	if err != nil {
		return nil, err
	}
	return &ClosingView{Shift: shift, Count: count}, nil
}

// startClosing inicia el arqueo de cierre y pausa la caja.
func (handler *service) startClosing(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	shift, err := handler.closingShift(parts[1])
	if err != nil {
		return nil, err
	}
	if err := shiftStateError(shift); err != nil {
		return nil, err
	}
	movements, operations, err := handler.shiftRecordCounts(shift.ID)
	if err != nil {
		return nil, err
	}
	// clock_timestamp() se toma después del lock exclusivo: todo registro confirmado antes tiene una
	// hora anterior y todo intento posterior encuentra el turno en arqueo.
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_shifts SET state='closing',closing_started_by=$2,
        closing_started_at=clock_timestamp(),closing_device_id=$3,closing_movement_count=$4,closing_operation_count=$5,
        revision=revision+1 WHERE id=$1 AND state='open'`,
		shift.ID, handler.request.Actor.UserID, handler.request.Actor.DeviceID, movements, operations)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, conflict("SHIFT_CLOSING", "El turno de "+shift.RegisterName+" ya no está abierto. Actualiza la pantalla.")
	}
	if err := handler.resetClosingCount(shift.ID); err != nil {
		return nil, err
	}
	view, err := handler.closingView(shift.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.closing_started", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return view, handler.audit(handler.ctx, handler.tx, "cash.closing_started", "cash_shift", &shift.ID,
		map[string]string{"state": shiftOpen}, map[string]any{"state": shiftClosing, "register": shift.RegisterName})
}

// resetClosingCount crea el borrador del arqueo o, si un arqueo anterior se canceló, lo vacía: el
// efectivo pudo cambiar al reanudar la caja y debe contarse de nuevo.
func (handler *service) resetClosingCount(shiftID string) error {
	existing, err := handler.readCount(shiftID, "closing")
	if err != nil {
		return err
	}
	if existing == nil {
		_, err := handler.insertCount(shiftID, "closing", "draft", countTally{}, "")
		return err
	}
	return handler.replaceDraftCount(existing.ID, countTally{}, "")
}

// readClosing devuelve el arqueo en curso con su borrador. No incluye importes esperados.
func (handler *service) readClosing(parts []string) (any, error) {
	shift, err := handler.closingShift(parts[1])
	if err != nil {
		return nil, err
	}
	if err := requireClosing(shift); err != nil {
		return nil, err
	}
	return handler.closingView(shift.ID)
}

// ClosingCountInput es el borrador del arqueo: denominaciones, adicional y observaciones.
type ClosingCountInput struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
	CountInput
	Notes string `json:"notes"`
}

// saveClosingCount guarda el borrador del conteo en el servidor para que sobreviva a cambios de
// pantalla o al cierre de la aplicación. La revisión evita sobrescribir el de otra terminal.
func (handler *service) saveClosingCount(parts []string) (any, error) {
	var input ClosingCountInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	notes := strings.TrimSpace(input.Notes)
	if textLength(notes) > 2000 {
		return nil, invalid("Las observaciones del arqueo admiten hasta 2000 caracteres.")
	}
	shift, err := handler.closingShift(parts[1])
	if err != nil {
		return nil, err
	}
	if err := requireClosing(shift); err != nil {
		return nil, err
	}
	count, err := handler.readCount(shift.ID, "closing")
	if err != nil {
		return nil, err
	}
	if count == nil || count.Revision != input.Revision {
		return nil, conflict("COUNT_CHANGED", "El conteo cambió en otra terminal. Actualiza la pantalla antes de guardar.")
	}
	tally, err := handler.evaluateCount(input.CountInput, shiftHandlesDollars(shift))
	if err != nil {
		return nil, err
	}
	if err := handler.replaceDraftCount(count.ID, tally, notes); err != nil {
		return nil, err
	}
	view, err := handler.closingView(shift.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.closing.count_saved", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return view, handler.audit(handler.ctx, handler.tx, "cash.closing_count_saved", "cash_shift", &shift.ID, nil, map[string]any{
		"counted_mxn": view.Count.CountedMXN, "counted_usd": view.Count.CountedUSD, "revision": view.Count.Revision,
	})
}

// cancelClosing termina el arqueo sin cerrar y reanuda la caja.
func (handler *service) cancelClosing(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Reason      string `json:"reason"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if textLength(reason) > 500 {
		return nil, invalid("El motivo admite hasta 500 caracteres.")
	}
	shift, err := handler.closingShift(parts[1])
	if err != nil {
		return nil, err
	}
	if err := requireClosing(shift); err != nil {
		return nil, err
	}
	if shift.ClosingStartedBy == nil || *shift.ClosingStartedBy != handler.request.Actor.UserID {
		if err := handler.request.Actor.Require(permissionCancelAnyClosing); err != nil {
			return nil, err
		}
	}
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_shifts SET state='open',closing_started_by=NULL,closing_started_at=NULL,
        closing_device_id=NULL,closing_movement_count=NULL,closing_operation_count=NULL,revision=revision+1
        WHERE id=$1 AND state='closing'`, shift.ID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, conflict("SHIFT_NOT_CLOSING", "El arqueo ya no está en curso. Actualiza la pantalla.")
	}
	saved, err := handler.readShift("s.id=$1", shift.ID)
	if err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.closing_cancelled", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	return map[string]any{"shift": saved}, handler.audit(handler.ctx, handler.tx, "cash.closing_cancelled", "cash_shift", &shift.ID,
		map[string]any{"state": shiftClosing, "started_by": shift.ClosingStartedByName, "started_at": shift.ClosingStartedAt},
		map[string]any{"state": shiftOpen, "reason": reason})
}

// confirmClosing confirma el arqueo y cierra el turno de forma transaccional.
func (handler *service) confirmClosing(parts []string) (any, error) {
	var input struct {
		OperationID   string `json:"operation_id"`
		CountRevision int    `json:"count_revision"`
		// Print solicita imprimir el corte definitivo al cerrar; un fallo de impresión nunca
		// revierte ni bloquea el cierre.
		Print *bool `json:"print"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	printDocument, err := handler.printRequested(input.Print)
	if err != nil {
		return nil, err
	}
	shift, err := handler.closingShift(parts[1])
	if err != nil {
		return nil, err
	}
	if err := requireClosing(shift); err != nil {
		return nil, err
	}
	count, err := handler.readCount(shift.ID, "closing")
	if err != nil {
		return nil, err
	}
	if count == nil || count.State != "draft" || count.Revision != input.CountRevision {
		return nil, conflict("COUNT_CHANGED", "El conteo cambió desde que lo revisaste. Actualiza la pantalla y confirma de nuevo.")
	}
	if err := handler.requireNoLateRecords(shift); err != nil {
		return nil, err
	}
	// Snapshot consistente: bajo el lock exclusivo ningún cobro ni movimiento puede intercalarse.
	expected, err := handler.expectedCash(shift)
	if err != nil {
		return nil, err
	}
	countedMXN := decimal.RequireFromString(count.CountedMXN)
	countedUSD := decimal.RequireFromString(count.CountedUSD)
	differences := computeDifferences(countedMXN, countedUSD, expected)
	reportID, err := handler.recordReport(shift, "final", &reportExtras{
		declared: &declaredCash{
			CountedMXN: count.CountedMXN, CountedUSD: count.CountedUSD, ExtraMXN: count.ExtraMXN, ExtraUSD: count.ExtraUSD,
			CashierNotes: count.Notes,
		},
		differences: &differences,
		expected:    expected,
	})
	if err != nil {
		return nil, err
	}
	if err := handler.insertClosure(shift, count, reportID, expected, differences); err != nil {
		return nil, err
	}
	if err := handler.finishShift(shift, count.ID); err != nil {
		return nil, err
	}
	closed, err := handler.readShift("s.id=$1", shift.ID)
	if err != nil {
		return nil, err
	}
	final, err := handler.readCount(shift.ID, "closing")
	if err != nil {
		return nil, err
	}
	printing := &PrintOutcome{Requested: false}
	if printDocument {
		// El turno ya está cerrado: el esperado y las diferencias se imprimen solo si quien cierra
		// tiene cash.expected.read. La respuesta solo informa el estado del trabajo.
		report, err := handler.readReportRecord(reportID)
		if err != nil {
			return nil, err
		}
		outcome := handler.printAfterRecording(cashDocumentSource{kind: documentFinalCut, shift: closed, report: report}, "el cierre")
		printing = &outcome
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.shift.closed", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	// La auditoría y la respuesta guardada para reintentos contienen solo lo declarado; el esperado
	// y las diferencias se consultan después con su permiso.
	if err := handler.audit(handler.ctx, handler.tx, "cash.shift_closed", "cash_shift", &shift.ID,
		map[string]string{"state": shiftClosing}, map[string]any{
			"state": shiftClosed, "counted_mxn": count.CountedMXN, "counted_usd": count.CountedUSD, "final_report_id": reportID,
		}); err != nil {
		return nil, err
	}
	return map[string]any{"shift": closed, "count": final, "final_report_id": reportID, "closed": true, "printing": printing}, nil
}

// requireNoLateRecords comprueba que no haya movimientos ni operaciones posteriores al inicio del
// arqueo: ni por cantidad (contra lo guardado al iniciar) ni por fecha.
func (handler *service) requireNoLateRecords(shift *Shift) error {
	movements, operations, err := handler.shiftRecordCounts(shift.ID)
	if err != nil {
		return err
	}
	var late bool
	err = handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.cash_movements WHERE shift_id=$1 AND created_at>=$2)
        OR EXISTS(SELECT FROM app.cash_operations WHERE shift_id=$1 AND created_at>=$2)`, shift.ID, shift.ClosingStartedAt).Scan(&late)
	if err != nil {
		return err
	}
	changed := shift.closingMovements == nil || shift.closingOperations == nil ||
		movements != *shift.closingMovements || operations != *shift.closingOperations
	if late || changed {
		return conflict("CLOSING_MOVEMENTS_CHANGED", "Se registraron movimientos en el turno después de iniciar el arqueo. "+
			"Cancela el arqueo, revisa la caja y vuelve a contar.")
	}
	return nil
}

// insertClosure guarda el resultado histórico del cierre por moneda.
func (handler *service) insertClosure(shift *Shift, count *CountView, reportID string, expected cashExpectation, differences cashDifferences) error {
	var rate, equivalent *string
	if expected.rate != nil {
		rate = optionalMoney(*expected.rate)
	}
	if differences.equivalent != nil {
		equivalent = optionalMoney(*differences.equivalent)
	}
	_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_shift_closures(shift_id,count_id,final_report_id,expected_mxn,expected_usd,
        counted_mxn,counted_usd,difference_mxn,difference_usd,usd_rate,difference_equivalent_mxn,result_mxn,result_usd,cashier_notes,
        closed_by,device_id,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::numeric,$11::numeric,$12,$13,$14,$15,$16,$17)`,
		shift.ID, count.ID, reportID, money(expected.mxn), money(expected.usd), count.CountedMXN, count.CountedUSD,
		money(differences.mxn), money(differences.usd), rate, equivalent, differenceResult(differences.mxn),
		differenceResult(differences.usd), count.Notes, handler.request.Actor.UserID, handler.request.Actor.DeviceID, handler.operationID)
	return err
}

// finishShift fija el conteo como definitivo y cierra el turno, verificando de nuevo su estado.
func (handler *service) finishShift(shift *Shift, countID string) error {
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.cash_counts SET state='final',updated_at=now() WHERE id=$1 AND state='draft'",
		countID); err != nil {
		return err
	}
	tag, err := handler.tx.Exec(handler.ctx, `UPDATE app.cash_shifts SET state='closed',closed_at=now(),closed_by=$2,closed_device_id=$3,
        revision=revision+1 WHERE id=$1 AND state='closing'`, shift.ID, handler.request.Actor.UserID, handler.request.Actor.DeviceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return conflict("SHIFT_CLOSED", "El turno ya se cerró en otra terminal.")
	}
	return nil
}

// ClosureView es el resultado de un turno cerrado. Lo declarado y las regularizaciones enlazadas
// son visibles para quien puede consultar el turno; el esperado, las diferencias y los comentarios
// del supervisor solo con cash.expected.read (los comentarios, también con cash.closures.review).
type ClosureView struct {
	Shift           *Shift           `json:"shift"`
	Count           *CountView       `json:"count"`
	CashierNotes    string           `json:"cashier_notes"`
	ClosedByName    string           `json:"closed_by_name"`
	DeviceName      string           `json:"device_name"`
	ClosedAt        time.Time        `json:"closed_at"`
	FinalReportID   string           `json:"final_report_id"`
	Regularizations []CashOperation  `json:"regularizations"`
	ExpectedVisible bool             `json:"expected_visible"`
	Expected        *CashExpectation `json:"expected,omitempty"`
	Differences     *CashDifferences `json:"differences,omitempty"`
	Comments        []ClosureComment `json:"comments,omitempty"`
}

// ClosureComment es un comentario posterior del supervisor sobre el cierre.
type ClosureComment struct {
	ID         string    `json:"id"`
	AuthorName string    `json:"author_name"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

// readClosure devuelve el resultado del cierre según los permisos de quien consulta.
func (handler *service) readClosure(parts []string) (any, error) {
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	if shift.State != shiftClosed {
		return nil, conflict("SHIFT_NOT_CLOSED", "El turno de "+shift.RegisterName+" aún no está cerrado.")
	}
	view := &ClosureView{Shift: shift}
	var expectedMXN, expectedUSD, differenceMXN, differenceUSD string
	var rate, equivalent *string
	err = handler.tx.QueryRow(handler.ctx, `SELECT c.cashier_notes,u.display_name,d.name,c.closed_at,c.final_report_id,
        c.expected_mxn::text,c.expected_usd::text,c.difference_mxn::text,c.difference_usd::text,c.usd_rate::text,
        c.difference_equivalent_mxn::text
        FROM app.cash_shift_closures c JOIN app.users u ON u.id=c.closed_by JOIN app.devices d ON d.id=c.device_id
        WHERE c.shift_id=$1`, shift.ID).Scan(&view.CashierNotes, &view.ClosedByName, &view.DeviceName, &view.ClosedAt,
		&view.FinalReportID, &expectedMXN, &expectedUSD, &differenceMXN, &differenceUSD, &rate, &equivalent)
	if isNoRows(err) {
		return nil, notFound("El turno se cerró antes de la etapa 7 y no tiene arqueo registrado.")
	}
	if err != nil {
		return nil, err
	}
	if view.Count, err = handler.readCount(shift.ID, "closing"); err != nil {
		return nil, err
	}
	if view.Regularizations, err = handler.listCashOperations(`o.regularization AND o.corrects_operation_id IN
        (SELECT id FROM app.cash_operations WHERE shift_id=$1)`, shift.ID); err != nil {
		return nil, err
	}
	actor := handler.request.Actor
	if actor.Can(permissionExpectedRead) {
		view.ExpectedVisible = true
		view.Expected = &CashExpectation{MXN: expectedMXN, USD: expectedUSD, USDRate: rate}
		if rate != nil {
			usdEquivalent := usdToMXN(decimal.RequireFromString(expectedUSD), decimal.RequireFromString(*rate))
			view.Expected.USDEquivalentMXN = optionalMoney(usdEquivalent)
			view.Expected.TotalEquivalentMXN = optionalMoney(decimal.RequireFromString(expectedMXN).Add(usdEquivalent))
		}
		view.Differences = &CashDifferences{
			MXN: differenceMXN, USD: differenceUSD, EquivalentMXN: equivalent,
			ResultMXN: differenceResult(decimal.RequireFromString(differenceMXN)),
			ResultUSD: differenceResult(decimal.RequireFromString(differenceUSD)),
		}
	}
	if actor.Can(permissionExpectedRead) || actor.Can(permissionClosuresReview) {
		if view.Comments, err = handler.closureComments(shift.ID); err != nil {
			return nil, err
		}
	}
	return view, nil
}

func (handler *service) closureComments(shiftID string) ([]ClosureComment, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT c.id,u.display_name,c.comment,c.created_at FROM app.cash_closure_comments c
        JOIN app.users u ON u.id=c.author_id WHERE c.shift_id=$1 ORDER BY c.created_at,c.id`, shiftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ClosureComment{}
	for rows.Next() {
		var item ClosureComment
		if err := rows.Scan(&item.ID, &item.AuthorName, &item.Comment, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// commentClosure agrega un comentario posterior del supervisor. Nunca cambia la diferencia.
func (handler *service) commentClosure(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Comment     string `json:"comment"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	comment := strings.TrimSpace(input.Comment)
	if textLength(comment) < 1 || textLength(comment) > 2000 {
		return nil, invalid("El comentario debe tener entre 1 y 2000 caracteres.")
	}
	shift, err := handler.readableShift(parts[1])
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.cash_shift_closures WHERE shift_id=$1)", shift.ID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, conflict("SHIFT_NOT_CLOSED", "Solo se comentan turnos cerrados con arqueo registrado.")
	}
	id := newID()
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.cash_closure_comments(id,shift_id,author_id,device_id,comment,operation_id)
        VALUES ($1,$2,$3,$4,$5,$6)`, id, shift.ID, handler.request.Actor.UserID, handler.request.Actor.DeviceID, comment,
		handler.operationID); err != nil {
		return nil, err
	}
	var saved ClosureComment
	if err := handler.tx.QueryRow(handler.ctx, `SELECT c.id,u.display_name,c.comment,c.created_at FROM app.cash_closure_comments c
        JOIN app.users u ON u.id=c.author_id WHERE c.id=$1`, id).Scan(&saved.ID, &saved.AuthorName, &saved.Comment, &saved.CreatedAt); err != nil {
		return nil, err
	}
	if err := events.Append(handler.ctx, handler.tx, "cash.closure.commented", cashScope, &shift.ID, nil); err != nil {
		return nil, err
	}
	// El texto puede mencionar importes del cierre: la auditoría registra el comentario sin copiarlo.
	return saved, handler.audit(handler.ctx, handler.tx, "cash.closure_commented", "cash_shift", &shift.ID, nil, map[string]any{
		"comment_id": id, "length": textLength(comment),
	})
}
