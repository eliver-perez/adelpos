package inventory

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Preclasificación de lo ya preparado al cancelar una cuenta (etapa 9D, §5.4 y decisión 4 del
// coordinador). Cada motivo de cancelación lleva la marca «lo enviado a cocina cuenta como merma»
// (account_cancel_reasons.prepared_is_waste), congelada en el registro de la cancelación
// (account_cancellations). Cuando la solicitud «cancel» de una unidad consumida proviene de un evento
// account.cancelled con esa marca, su clasificación nace como «Preparado / merma» (conserva el consumo,
// sin movimientos nuevos) en lugar de «Pendiente de clasificar»:
//   - la registra la persona que canceló la cuenta (resolución manual, con la nota del motivo), de
//     modo que la restricción de forma de la etapa 8 se cumple sin cambios;
//   - el encargado puede corregirla con inventory.waste.record hasta el corte del día operativo y,
//     después, solo inventory.adjustments.authorize, exactamente como una clasificación manual;
//   - el reconciliador no la toca (solo toma las pendientes).
// Con los demás motivos, la cancelación sigue «por clasificar» como en la etapa 8. El inventario no
// importa operations: lee el registro de la cancelación por SQL a partir del evento de la solicitud.

// preclassification es la cancelación de cuenta que marca la unidad como merma.
type preclassification struct {
	cancelledBy string
	reasonName  string
}

// wastePreclassification devuelve la cancelación de cuenta con marca de merma que originó la
// solicitud, o nil si la solicitud viene de otro evento o el motivo no la marca.
func wastePreclassification(ctx context.Context, tx pgx.Tx, request *consumptionRequest) (*preclassification, error) {
	var item preclassification
	err := tx.QueryRow(ctx, `SELECT c.cancelled_by::text,c.reason_name FROM app.restaurant_events e
        JOIN app.account_cancellations c ON c.operation_id=e.operation_id AND c.account_id=e.account_id
        WHERE e.id=$1 AND e.kind='account.cancelled' AND c.prepared_is_waste`, request.SourceID).Scan(&item.cancelledBy, &item.reasonName)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// recordPreclassifiedWaste crea la clasificación «Preparado / merma» de la unidad con el día operativo
// de la cancelación y la audita a nombre del proceso. Si la unidad ya tenía clasificación, no la
// cambia (idempotente, como la pendiente).
func recordPreclassifiedWaste(ctx context.Context, tx pgx.Tx, request *consumptionRequest, day OperatingDay, item *preclassification) error {
	notes := "Preclasificada al cancelar la cuenta con el motivo «" + item.reasonName + "»."
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO app.inventory_cancellation_classifications(restaurant_unit_id,request_id,cancelled_at,
            operating_date,deadline_at,state,resolution,classified_by,classified_at,notes)
        VALUES ($1,$2,$3,$4,$5,'prepared_waste','manual',$6,clock_timestamp(),$7)
        ON CONFLICT ON CONSTRAINT inventory_cancellation_classifications_unit_key DO NOTHING RETURNING id`,
		request.UnitID, request.ID, request.OccurredAt, day.Date, day.Deadline, item.cancelledBy, notes).Scan(&id)
	if err != nil && !isNoRows(err) {
		return err
	}
	if err == nil {
		after := map[string]any{"state": "prepared_waste", "resolution": "manual", "classified_by": item.cancelledBy,
			"reason": item.reasonName, "deadline_at": day.Deadline, "movements": 0}
		_, err = tx.Exec(ctx, `INSERT INTO app.audit_events(actor_id,actor_username,actor_kind,device_id,action,entity_type,entity_id,
                success,before_state,after_state)
            VALUES (NULL,$1,'process',NULL,'inventory.cancellation_preclassified','inventory_cancellation',$2,true,NULL,$3)`,
			workerProcess, id, jsonValue(after))
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE app.inventory_unit_consumptions SET last_request_id=$2,updated_at=clock_timestamp()
        WHERE restaurant_unit_id=$1`, request.UnitID, request.ID)
	return err
}
