package kitchen

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/events"
	"local/ad-el-pos/server/internal/inventory"
	"local/ad-el-pos/server/internal/tableunions"
)

type unitRecord struct {
	ID        string `json:"id"`
	ItemID    string `json:"item_id"`
	VersionID string `json:"version_id"`
	AccountID string `json:"account_id"`
	State     string `json:"state"`
	Total     string `json:"total"`
}
type versionSnapshot struct {
	Request catalog.LineRequest `json:"request"`
	Quote   catalog.QuoteLine   `json:"quote_line"`
	Catalog catalog.Snapshot    `json:"catalog"`
}
type preparedUnit struct {
	Unit    unitRecord
	Version *versionSnapshot
	Routing *routingResult
}

func readRelease(ctx context.Context, tx pgx.Tx, id string) (*Release, error) {
	return one[Release](ctx, tx, "SELECT to_jsonb(r) FROM app.kitchen_releases r WHERE id=$1", id)
}
func loadUnits(ctx context.Context, tx pgx.Tx, ids []string) ([]unitRecord, error) {
	return many[unitRecord](ctx, tx, `SELECT to_jsonb(u)||jsonb_build_object('total',u.total::text)
        FROM app.restaurant_item_units u WHERE id=ANY($1::uuid[]) ORDER BY u.item_id,u.ordinal,u.id`, ids)
}
func readVersion(ctx context.Context, tx pgx.Tx, id string) (*versionSnapshot, error) {
	return one[versionSnapshot](ctx, tx, `SELECT jsonb_build_object('request',v.request,'quote_line',v.quote_line,
        'catalog',o.pricing_snapshot) FROM app.restaurant_item_versions v JOIN app.restaurant_orders o ON o.id=v.order_id WHERE v.id=$1`, id)
}

// readContext es el encabezado de la comanda en el momento de liberarla. Etapa M1 (M1-D14): la mesa de una cuenta unida
// es el nombre de su unión con documents.TableContext («Salón · Mesa 5 + 6»); una mesa sola se imprime como siempre.
func readContext(ctx context.Context, tx pgx.Tx, accountID, orderID string) (*TicketContext, error) {
	context, err := one[TicketContext](ctx, tx, `SELECT jsonb_build_object('establishment',e.name,'timezone',e.timezone,
        'folio',a.folio,'account_name',a.name,'table',coalesce(t.name,''),'area',coalesce(z.name,''),
        'mode',a.mode,'customer',a.customer_name,'origin',a.origin,'employee',person.display_name,'order_id',o.id)
        FROM app.restaurant_accounts a CROSS JOIN app.establishment e
        JOIN app.restaurant_orders o ON o.id=$2 JOIN app.users person ON person.id=o.author_id
        LEFT JOIN app.restaurant_tables t ON t.id=a.table_id LEFT JOIN app.restaurant_areas z ON z.id=t.area_id WHERE a.id=$1`, accountID, orderID)
	if err != nil {
		return nil, err
	}
	var union *string
	if err := tx.QueryRow(ctx, "SELECT "+tableunions.UnionNameSQL("a.table_id")+" FROM app.restaurant_accounts a WHERE a.id=$1",
		accountID).Scan(&union); err != nil {
		return nil, err
	}
	if union != nil {
		context.Table = documents.TableContext(*union)
	}
	return context, nil
}

func (handler *service) createRelease(eventID, accountID, orderID string, unitIDs []string) (*Release, error) {
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	var scheduledAt *time.Time
	if err := handler.tx.QueryRow(handler.ctx, "SELECT scheduled_at FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&scheduledAt); err != nil {
		return nil, err
	}
	release := &Release{ID: events.NewID(), SourceEventID: eventID, AccountID: accountID, OrderID: orderID, UnitIDs: unitIDs,
		State: "pending", Automatic: settings.AutomaticRelease, LeadMinutes: settings.LeadMinutes, ScheduledAt: scheduledAt}
	if scheduledAt != nil {
		release.State = "scheduled"
		due := scheduledAt.Add(-time.Duration(settings.LeadMinutes) * time.Minute)
		release.DueAt = &due
	}
	_, err = handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_releases(id,source_event_id,order_id,account_id,unit_ids,
        state,automatic,lead_minutes,scheduled_at,due_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, release.ID, eventID,
		orderID, accountID, unitIDs, release.State, release.Automatic, release.LeadMinutes, scheduledAt, release.DueAt)
	if err != nil {
		return nil, err
	}
	if scheduledAt == nil || (release.Automatic && release.DueAt != nil && !release.DueAt.After(time.Now())) {
		if err := handler.prepareRelease(release); err != nil {
			return nil, err
		}
	}
	return readRelease(handler.ctx, handler.tx, release.ID)
}

// prepareRelease manda a la cocina lo que todavía no recibió de una liberación. Desde la etapa K1.1 (K11-D1) un envío
// nunca se detiene por la configuración: lo que tiene ruta llega a sus estaciones activas; lo que no (o lo que solo
// iba a estaciones desactivadas) sigue el ajuste «Productos sin ruta»; una impresora desactivada no detiene nada (el
// trabajo queda en «Requiere intervención», enqueueJob, y con «Impresora y pantalla» la pantalla lo recibe igual). Toda
// unidad activa se libera una sola vez, con o sin tarea, como con «Sin preparación». Ya no escribe «blocked»: ese
// valor queda solo en las liberaciones anteriores a la 0.17.0.
func (handler *service) prepareRelease(release *Release) error {
	if release.State == "released" || release.State == "cancelled" {
		return nil
	}
	units, err := loadUnits(handler.ctx, handler.tx, release.UnitIDs)
	if err != nil {
		return err
	}
	versions := map[string]*versionSnapshot{}
	plan := []preparedUnit{}
	routingSnapshot := map[string]Route{}
	stationSnapshot := map[string]Station{}
	for _, unit := range units {
		if unit.State != "active" {
			continue
		}
		var alreadyReleased bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT EXISTS(SELECT FROM app.kitchen_unit_releases WHERE unit_id=$1)", unit.ID).Scan(&alreadyReleased); err != nil {
			return err
		}
		if alreadyReleased {
			continue
		}
		version := versions[unit.VersionID]
		if version == nil {
			version, err = readVersion(handler.ctx, handler.tx, unit.VersionID)
			if err != nil {
				return err
			}
			versions[unit.VersionID] = version
		}
		routing, err := resolveRouting(handler.ctx, handler.tx, version.Quote.ProductBreakdown, version.Catalog, nil)
		if err != nil {
			return err
		}
		for id, route := range routing.Snapshot {
			routingSnapshot[id] = route
		}
		for _, station := range routing.Stations {
			stationSnapshot[station.ID] = station
		}
		plan = append(plan, preparedUnit{Unit: unit, Version: version, Routing: routing})
	}
	if len(plan) == 0 {
		_, err := handler.tx.Exec(handler.ctx, "UPDATE app.kitchen_releases SET state='cancelled',revision=revision+1 WHERE id=$1", release.ID)
		return err
	}
	contextSnapshot, err := readContext(handler.ctx, handler.tx, release.AccountID, release.OrderID)
	if err != nil {
		return err
	}
	// Cada unidad se libera una sola vez, independientemente del número de estaciones.
	for _, entry := range plan {
		_, err := handler.tx.Exec(handler.ctx,
			"INSERT INTO app.kitchen_unit_releases(unit_id,release_id,version_id) VALUES ($1,$2,$3)",
			entry.Unit.ID, release.ID, entry.Unit.VersionID)
		if err != nil {
			return err
		}
		for id, parts := range entry.Routing.Parts {
			instructions := Instructions{ItemID: entry.Unit.ItemID, VersionID: entry.Unit.VersionID, UnitIDs: []string{entry.Unit.ID},
				Name: entry.Version.Quote.Name, Quantity: 1, Parts: parts, Price: entry.Unit.Total, Notes: entry.Version.Request.Customization.Notes}
			_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_tasks(id,unit_id,release_id,station_id,version_id,instructions)
                VALUES ($1,$2,$3,$4,$5,$6)`, events.NewID(), entry.Unit.ID, release.ID, id, entry.Unit.VersionID, encoded(instructions))
			if err != nil {
				return err
			}
		}
	}
	// Inventario (etapa 8, §0-B): con «Control de inventario» activado, una solicitud de consumo por
	// unidad liberada en esta misma transacción. Es una sola inserción, sin el lock de inventario ni
	// cálculo de recetas: el trabajador de inventario la procesa después.
	released := make([]inventory.ReleasedUnit, 0, len(plan))
	for _, entry := range plan {
		released = append(released, inventory.ReleasedUnit{UnitID: entry.Unit.ID, VersionID: entry.Unit.VersionID})
	}
	if err := inventory.RecordReleases(handler.ctx, handler.tx, release.ID, released, contextSnapshot.Mode); err != nil {
		return err
	}
	_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_releases SET state='released',released_at=now(),warnings='[]',
        routing_snapshot=$1,context_snapshot=$2,station_snapshot=$3,revision=revision+1 WHERE id=$4`,
		encoded(routingSnapshot), encoded(contextSnapshot), encoded(stationSnapshot), release.ID)
	if err != nil {
		return err
	}
	if err := handler.ticketsForRelease(release, plan); err != nil {
		return err
	}
	return events.Append(handler.ctx, handler.tx, "kitchen.released", "kitchen", &release.ID, nil)
}

func (handler *service) releaseAction(id, action string) (any, error) {
	var input struct {
		OperationID    string `json:"operation_id"`
		Revision       int    `json:"revision"`
		Automatic      bool   `json:"automatic"`
		LeadMinutes    int    `json:"lead_minutes"`
		ScheduledLocal string `json:"scheduled_local"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if !handler.request.Actor.Can("kitchen.stations.all") {
		return nil, &Error{Status: 403, Code: "STATION_SCOPE_DENIED", Message: "Liberar o programar un pedido completo requiere operar todas sus estaciones."}
	}
	release, err := readRelease(handler.ctx, handler.tx, id)
	if err != nil {
		return nil, err
	}
	// Etapa K1.1 (K11-D4): «Enviar a cocina ahora» de algo que ya no está pendiente (ya enviado, cancelado o de una cuenta
	// que ya no está abierta) responde 409 en lugar de 200 sin hacer nada, o de mandar a la cocina el pedido de una cuenta
	// ya cobrada. Va antes de la revisión: quien vio el pedido pendiente recibe por qué ya no lo está.
	if action == "release" {
		pending, err := releaseStillPending(handler.ctx, handler.tx, release.ID)
		if err != nil {
			return nil, err
		}
		if !pending {
			return nil, releaseNotPending()
		}
	}
	if release.Revision != input.Revision {
		return nil, conflict()
	}
	// Liberar o programar cambia la preparación de las cuentas de sus unidades (A1-D11, preparation.go).
	if err := handler.notePreparationUnits(release.UnitIDs); err != nil {
		return nil, err
	}
	if action == "release" {
		if release.DueAt != nil && release.DueAt.After(time.Now()) {
			if err := handler.request.Actor.Require("kitchen.release_early"); err != nil {
				return nil, err
			}
		}
		if err := handler.prepareRelease(release); err != nil {
			return nil, err
		}
	} else {
		if release.State == "released" || release.State == "cancelled" {
			return nil, invalid("Una liberación finalizada conserva su programación histórica.")
		}
		if input.LeadMinutes < 0 || input.LeadMinutes > 1440 {
			return nil, invalid("La anticipación admite de 0 a 1440 minutos.")
		}
		var timezone string
		if err := handler.tx.QueryRow(handler.ctx, "SELECT timezone FROM app.establishment").Scan(&timezone); err != nil {
			return nil, err
		}
		location, err := time.LoadLocation(timezone)
		if err != nil {
			return nil, err
		}
		scheduled, err := time.ParseInLocation("2006-01-02T15:04", input.ScheduledLocal, location)
		if err != nil || scheduled.Format("2006-01-02T15:04") != input.ScheduledLocal {
			return nil, invalid("La fecha/hora local no existe o no es válida.")
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_releases SET scheduled_at=$1,due_at=$2,automatic=$3,
            lead_minutes=$4,state='scheduled',warnings='[]',revision=revision+1 WHERE id=$5`, scheduled,
			scheduled.Add(-time.Duration(input.LeadMinutes)*time.Minute), input.Automatic, input.LeadMinutes, id)
		if err != nil {
			return nil, err
		}
	}
	saved, err := readRelease(handler.ctx, handler.tx, id)
	if err != nil {
		return nil, err
	}
	return saved, handler.history("release."+action, &id, release, saved)
}

// accountScheduleChanged compara la programación anterior y posterior de un evento
// account.updated. Dos ausencias equivalen; dos instantes se comparan por igualdad temporal.
func accountScheduleChanged(payload []byte) (bool, error) {
	var change struct {
		Before struct {
			ScheduledAt *time.Time `json:"scheduled_at"`
		} `json:"before"`
		After struct {
			ScheduledAt *time.Time `json:"scheduled_at"`
		} `json:"after"`
	}
	if err := json.Unmarshal(payload, &change); err != nil {
		return false, err
	}
	before := change.Before.ScheduledAt
	after := change.After.ScheduledAt
	if before == nil || after == nil {
		return (before == nil) != (after == nil), nil
	}
	return !before.Equal(*after), nil
}

// Integrate consume el mismo evento dentro de la transacción comercial que lo produce.
func Integrate(ctx context.Context, tx pgx.Tx, eventID, accountID, kind string, payload []byte, actor Actor) error {
	var found bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.kitchen_event_receipts WHERE event_id=$1)", eventID).Scan(&found); err != nil {
		return err
	}
	if found {
		return nil
	}
	handler := service{ctx: ctx, tx: tx, request: Request{Actor: actor}, preparation: map[string]bool{}}
	var change struct {
		After json.RawMessage `json:"after"`
	}
	if err := json.Unmarshal(payload, &change); err != nil {
		return err
	}
	switch kind {
	case "order.confirmed":
		var order struct {
			OrderID string `json:"order_id"`
			Items   []struct {
				UnitIDs []string `json:"unit_ids"`
			} `json:"items"`
		}
		if err := json.Unmarshal(change.After, &order); err != nil {
			return err
		}
		ids := []string{}
		for _, item := range order.Items {
			ids = append(ids, item.UnitIDs...)
		}
		if _, err := handler.createRelease(eventID, accountID, order.OrderID, ids); err != nil {
			return err
		}
		handler.notePreparationAccount(accountID)
	case "item.modified", "item.cancelled", "account.cancelled":
		// Inventario (etapa 8): las modificaciones y cancelaciones de unidades ya liberadas se
		// registran antes de procesar cocina, para que queden aunque el cambio de cocina quede
		// pendiente por advertencias. Desde la etapa 9D, «Cancelar cuenta» (account.cancelled) retira
		// en un solo evento todas las unidades activas de la cuenta con la misma forma (after.unit_ids
		// y after.reason) y recibe el mismo trato que item.cancelled.
		if err := inventory.RecordChange(ctx, tx, eventID, kind, payload); err != nil {
			return err
		}
		if err := handler.integrateChange(eventID, accountID, kind, change.After); err != nil {
			return err
		}
		// Etapa K1.1 (K11-D2 y K11-D4): una cuenta cancelada no deja nada por enviar a cocina. Lo que la cocina nunca
		// recibió se cancela y el aviso pendiente (también el de esta cancelación, si una estación desactivada tenía la
		// comanda) se da por terminado, con su historial.
		if kind == "account.cancelled" {
			if err := handler.dismissAccountPending(accountID, dismissedAccountCancelled); err != nil {
				return err
			}
		}
		handler.notePreparationAccount(accountID)
	case "account.updated":
		changed, err := accountScheduleChanged(payload)
		if err != nil {
			return err
		}
		// Solo una nueva programación capturada en la cuenta sustituye la de cocina. Editar
		// teléfono, notas u otros datos no debe revertir una reprogramación hecha en cocina.
		if !changed {
			break
		}
		rows, err := tx.Query(ctx, `UPDATE app.kitchen_releases r SET scheduled_at=a.scheduled_at,
            due_at=a.scheduled_at-make_interval(mins=>r.lead_minutes),revision=r.revision+1,
            state=CASE WHEN a.scheduled_at IS NULL THEN 'pending' ELSE 'scheduled' END
            FROM app.restaurant_accounts a WHERE a.id=$1 AND r.account_id=a.id
            AND r.state IN ('pending','scheduled','blocked') AND r.scheduled_at IS DISTINCT FROM a.scheduled_at
            RETURNING r.id::text,a.scheduled_at IS NULL`, accountID)
		if err != nil {
			return err
		}
		rescheduled, unscheduled := 0, []string{}
		for rows.Next() {
			var id string
			var withoutTime bool
			if err := rows.Scan(&id, &withoutTime); err != nil {
				rows.Close()
				return err
			}
			rescheduled++
			if withoutTime {
				unscheduled = append(unscheduled, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if rescheduled > 0 {
			handler.notePreparationAccount(accountID)
		}
		// Etapa M1 (§5.3, decisión del propietario M1-D17, reemplaza K11-D9): quitar la hora a un pedido programado lo
		// envía a cocina en ese momento, en esta misma transacción, como un pedido sin hora (createRelease). Antes
		// quedaba «Por enviar a cocina» hasta que alguien tocara «Enviar a cocina ahora».
		for _, id := range unscheduled {
			release, err := readRelease(ctx, tx, id)
			if err != nil {
				return err
			}
			if err := handler.prepareRelease(release); err != nil {
				return err
			}
		}
	}
	// Etapa A1 (A1-D11): la cuenta ya publica su propio evento restaurant.*; el de preparación dice además que cambió lo
	// que ve cocina, una sola vez por cuenta (cada operación de la cuenta integra un solo evento con cocina).
	if err := handler.publishPreparation(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO app.kitchen_event_receipts(event_id,result) VALUES ($1,'integrated')", eventID)
	return err
}
