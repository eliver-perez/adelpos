package operations

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/payments"
)

// «Cancelar cuenta» (etapa 9D, §5 y decisiones 3–5 del coordinador). Cualquier cuenta abierta sin
// pagos netos ni ajustes vigentes, de mesa o para llevar, con o sin productos enviados, se cancela con
// un motivo del catálogo («Otro» exige descripción). En una sola transacción (lock 7820148305
// exclusivo, tomado por Handle):
//   - se descarta el borrador sin enviar;
//   - se cancelan todas las unidades activas con un solo evento de cuenta, account.cancelled, que
//     cocina (kitchen.Integrate: cancela lo no liberado sin papel y avisa la cancelación a las
//     estaciones de lo ya liberado), inventario (inventory.RecordChange: solicitudes «cancel» de lo
//     ya consumido, preclasificadas como «Preparado / merma» si el motivo lo marca) y los reportes
//     de productos cancelados entienden igual que item.cancelled (after.unit_ids);
//   - la cuenta queda en el estado anulado actual (voided), la mesa libre, con su registro en
//     account_cancellations (motivo y marca congelados, quién canceló y quién autorizó);
//   - auditoría restaurant.account.cancelled (sin contraseñas) y evento en tiempo real.
// Nunca cuenta como venta: una cuenta anulada no se cierra ni se reconoce en sales_recognitions.
//
// Una cuenta sin productos enviados ni registros (discardable, aunque tenga borrador) no se cancela:
// se descarta sin motivo (§4.2, «no cuenta como cancelación ni aparece en reportes»). La ruta responde
// 409 ACCOUNT_DISCARDABLE antes de validar el motivo o pedir la contraseña del supervisor.
//
// Permiso: con accounts.cancel basta el propio; sin él, la autorización puntual de un supervisor que
// lo tenga, con el mismo mecanismo de los descuentos (validada antes de escribir, sin sesión, con el
// límite de intentos del inicio de sesión y sin autoautorización). La contraseña nunca forma parte de
// la huella idempotente (requestFingerprint) ni de la auditoría.

// cancelPermission es el permiso propio para cancelar cuentas.
const cancelPermission = "accounts.cancel"

type cancelAccountInput struct {
	Mutation
	ReasonID      string                       `json:"reason_id"`
	Description   string                       `json:"description"`
	Authorization *payments.AuthorizationInput `json:"authorization"`
}

// AccountCancellation es el registro de una cuenta cancelada, tal como se guardó al cancelarla.
type AccountCancellation struct {
	ReasonID         string    `json:"reason_id"`
	ReasonName       string    `json:"reason_name"`
	Description      *string   `json:"description"`
	PreparedIsWaste  bool      `json:"prepared_is_waste"`
	CancelledByName  string    `json:"cancelled_by_name"`
	AuthorizedByName *string   `json:"authorized_by_name"`
	CancelledUnits   int       `json:"cancelled_units"`
	CancelledAmount  string    `json:"cancelled_amount"`
	CreatedAt        time.Time `json:"created_at"`
}

// cancelledLine agrupa por versión las unidades que la cancelación retiró, con la misma forma que
// el evento item.cancelled (request, quote_line y units) para los reportes de cantidades.
type cancelledLine struct {
	VersionID string `json:"version_id"`
	versionRecord
	Units []Unit `json:"units"`
}

// resolveCancelReason valida el motivo elegido antes de escribir: debe existir y estar activo; la
// descripción es obligatoria (5–500 caracteres) si el motivo lo exige y opcional (hasta 500) en otro
// caso. Devuelve la descripción recortada o nil si quedó vacía.
func (handler *service) resolveCancelReason(reasonID, text string) (*CancelReason, *string, error) {
	if strings.TrimSpace(reasonID) == "" {
		return nil, nil, invalid("Elige el motivo de la cancelación.")
	}
	if err := validateID(reasonID); err != nil {
		return nil, nil, err
	}
	reason, err := handler.readCancelReason(reasonID)
	if err != nil {
		return nil, nil, err
	}
	if !reason.Active {
		return nil, nil, &Error{Status: 409, Code: "CANCEL_REASON_INACTIVE", Message: "El motivo «" + reason.Name + "» está desactivado. Elige otro motivo."}
	}
	description := strings.TrimSpace(text)
	length := utf8.RuneCountInString(description)
	if reason.RequiresDescription && length < 5 {
		return nil, nil, &Error{
			Status:  422,
			Code:    "CANCEL_DESCRIPTION_REQUIRED",
			Message: "El motivo «" + reason.Name + "» requiere una descripción de 5 a 500 caracteres.",
		}
	}
	if length > 500 {
		return nil, nil, &Error{Status: 422, Code: "CANCEL_DESCRIPTION_TOO_LONG", Message: "La descripción de la cancelación admite hasta 500 caracteres."}
	}
	if description == "" {
		return reason, nil, nil
	}
	return reason, &description, nil
}

// approveCancellation decide, antes de cualquier escritura, si el usuario cancela con su propio
// permiso o si hace falta una autorización de supervisor válida. Devuelve el supervisor que autorizó
// o nil si bastó el permiso propio (una autorización enviada en ese caso se ignora).
func (handler *service) approveCancellation(input *payments.AuthorizationInput) (*payments.Supervisor, error) {
	if handler.request.Actor.Can(cancelPermission) {
		return nil, nil
	}
	if input == nil || strings.TrimSpace(input.Username) == "" {
		return nil, &Error{
			Status:  403,
			Code:    "AUTHORIZATION_REQUIRED",
			Message: "Cancelar una cuenta requiere la autorización de un supervisor con permiso para cancelar cuentas.",
		}
	}
	if handler.request.Authorize == nil {
		return nil, &Error{Status: 403, Code: "PERMISSION_DENIED", Message: "No tienes permiso para esta operación del restaurante."}
	}
	supervisor, err := handler.request.Authorize(handler.ctx, handler.tx, input.Username, input.Password)
	if err != nil {
		return nil, err
	}
	if supervisor.UserID == handler.request.Actor.UserID {
		return nil, &Error{Status: 403, Code: "SELF_AUTHORIZATION", Message: "No puedes autorizar tu propia operación."}
	}
	if !supervisor.Can(cancelPermission) {
		return nil, &Error{
			Status:  403,
			Code:    "AUTHORIZATION_INSUFFICIENT",
			Message: "El supervisor indicado no tiene permiso para cancelar cuentas.",
		}
	}
	return supervisor, nil
}

// activeUnits lee las unidades activas de la cuenta en el orden de sus partidas.
func (handler *service) activeUnits(accountID string) ([]Unit, error) {
	return readList[Unit](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',u.id,'item_id',u.item_id,
        'version_id',u.version_id,'ordinal',u.ordinal,'subtotal',u.subtotal::text,'discount',u.discount::text,'total',u.total::text)
        FROM app.restaurant_item_units u JOIN app.restaurant_item_versions v ON v.id=u.version_id
        WHERE u.account_id=$1 AND u.state='active' ORDER BY v.created_at,v.id,u.ordinal,u.id`, accountID)
}

// cancelledLines agrupa las unidades por versión con su solicitud y su cotización congeladas.
func (handler *service) cancelledLines(units []Unit) ([]cancelledLine, error) {
	lines := []cancelledLine{}
	index := map[string]int{}
	for _, unit := range units {
		position, found := index[unit.VersionID]
		if !found {
			version, err := readOne[versionRecord](handler.ctx, handler.tx,
				"SELECT to_jsonb(v) FROM app.restaurant_item_versions v WHERE id=$1", unit.VersionID)
			if err != nil {
				return nil, err
			}
			lines = append(lines, cancelledLine{VersionID: unit.VersionID, versionRecord: *version, Units: []Unit{}})
			position = len(lines) - 1
			index[unit.VersionID] = position
		}
		lines[position].Units = append(lines[position].Units, unit)
	}
	return lines, nil
}

// cancelAccount atiende POST restaurant/accounts/{id}/cancel-account.
func (handler *service) cancelAccount(id string) (any, error) {
	var input cancelAccountInput
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	account, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	if err := handler.authorizeAccount(account, true); err != nil {
		return nil, err
	}
	if err := ensureOpen(account, input.Revision); err != nil {
		return nil, err
	}
	// Una cuenta vacía o solo con borrador se descarta: cancelarla dejaría en los reportes una
	// cancelación de $0 (revisión independiente 9D, M1). Se decide antes del motivo y del supervisor.
	if account.Discardable {
		return nil, &Error{Status: 409, Code: "ACCOUNT_DISCARDABLE", Message: discardableMessage}
	}
	reason, description, err := handler.resolveCancelReason(input.ReasonID, input.Description)
	if err != nil {
		return nil, err
	}
	// Guardas de la etapa 6: sin pagos netos ni propinas (primero la devolución) y sin ajustes
	// vigentes (primero quitar el descuento o el cargo); ninguna unidad con pago o descuento manual.
	if err := payments.RequireCancellable(handler.ctx, handler.tx, id); err != nil {
		return nil, err
	}
	units, err := handler.activeUnits(id)
	if err != nil {
		return nil, err
	}
	if err := handler.requireUnitsFree(units); err != nil {
		return nil, err
	}
	supervisor, err := handler.approveCancellation(input.Authorization)
	if err != nil {
		return nil, err
	}
	lines, err := handler.cancelledLines(units)
	if err != nil {
		return nil, err
	}
	draft, err := handler.readDraft(id)
	if err != nil {
		return nil, err
	}
	amount := unitTotals(units).Total
	// Desde aquí se escribe: la autorización y todos los permisos ya se comprobaron.
	if err := handler.touch(id, input.Revision); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.restaurant_draft_items WHERE account_id=$1", id); err != nil {
		return nil, err
	}
	identifiers := unitIDs(units)
	result, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_item_units SET state='cancelled',cancelled_at=now()
        WHERE id=ANY($1::uuid[]) AND account_id=$2 AND state='active'`, identifiers, id)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() != int64(len(identifiers)) {
		return nil, conflict()
	}
	if err := handler.requireAccountCovered(id); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET state='voided' WHERE id=$1", id); err != nil {
		return nil, err
	}
	var authorizedBy, authorizedUsername *string
	if supervisor != nil {
		authorizedBy, authorizedUsername = &supervisor.UserID, &supervisor.Username
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.account_cancellations(account_id,operation_id,reason_id,reason_name,
            description,prepared_is_waste,cancelled_by,authorized_by,device_id,cancelled_units,cancelled_amount)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, handler.operationID, reason.ID, reason.Name, description,
		reason.PreparedIsWaste, handler.request.Actor.UserID, authorizedBy, handler.request.Actor.DeviceID, len(units), amount)
	if err != nil {
		return nil, err
	}
	if supervisor != nil {
		// Como en los cobros, la autorización queda auditada por sí misma (sin contraseña).
		if err := handler.audit(handler.ctx, handler.tx, "restaurant.supervisor_authorized", "account", &id, nil, map[string]string{
			"scope": "account_cancel", "supervisor": supervisor.Username, "amount": amount,
		}); err != nil {
			return nil, err
		}
	}
	// El evento lleva la cuenta tal como queda antes de integrar cocina y, del registro recién
	// guardado, el nombre visible del supervisor (como el detalle y el reporte); nunca su contraseña.
	saved, err := handler.readAccount(id)
	if err != nil {
		return nil, err
	}
	record, err := handler.readCancellation(id)
	if err != nil {
		return nil, err
	}
	var authorizedName *string
	if record != nil {
		authorizedName = record.AuthorizedByName
	}
	before := map[string]any{"account": account, "lines": lines, "draft": draft}
	after := map[string]any{
		"account": saved, "reason_id": reason.ID, "reason": reason.Name, "description": description,
		"prepared_is_waste": reason.PreparedIsWaste, "unit_ids": identifiers, "cancelled_units": len(units),
		"cancelled_amount": amount, "authorized_by": authorizedBy, "authorized_by_username": authorizedUsername,
		"authorized_by_name": authorizedName, "discarded_draft_items": len(draft),
	}
	if err := handler.event(id, nil, "account.cancelled", before, after); err != nil {
		return nil, err
	}
	// La respuesta (y el resultado idempotente que guarda Handle) refleja la cuenta después de
	// integrar cocina: con una estación inactiva el cambio queda pendiente y la lectura lo dice
	// (revisión independiente 9D, B2). after.account conserva el estado previo a la integración.
	return handler.readBill(id)
}

// discardableMessage explica por qué una cuenta sin productos enviados no se cancela.
const discardableMessage = "Esta cuenta no tiene productos enviados: descártala; no hace falta motivo."

// readCancellation lee el registro de cancelación de una cuenta; nil si la cuenta no se canceló con
// motivo (anulaciones anteriores, liberaciones y descartes).
func (handler *service) readCancellation(accountID string) (*AccountCancellation, error) {
	var record AccountCancellation
	err := handler.tx.QueryRow(handler.ctx, `SELECT c.reason_id,c.reason_name,c.description,c.prepared_is_waste,
            author.display_name,approver.display_name,c.cancelled_units,c.cancelled_amount::text,c.created_at
        FROM app.account_cancellations c JOIN app.users author ON author.id=c.cancelled_by
        LEFT JOIN app.users approver ON approver.id=c.authorized_by WHERE c.account_id=$1`, accountID).Scan(
		&record.ReasonID, &record.ReasonName, &record.Description, &record.PreparedIsWaste, &record.CancelledByName,
		&record.AuthorizedByName, &record.CancelledUnits, &record.CancelledAmount, &record.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}
