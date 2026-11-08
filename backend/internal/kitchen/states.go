package kitchen

import (
	"context"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
)

func (handler *service) saveStates() (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		State       string `json:"state"`
		ReleaseID   string `json:"release_id"`
		Revision    int    `json:"revision"`
		StationID   string `json:"station_id"`
		Tasks       []struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
		} `json:"tasks"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if input.State != "preparing" && input.State != "ready" {
		return nil, invalid("El avance manual permite En preparación o Listo.")
	}
	tasks := []Task{}
	settings, err := handler.settings()
	if err != nil {
		return nil, err
	}
	if input.ReleaseID != "" {
		if err := validID(input.ReleaseID); err != nil {
			return nil, err
		}
		release, err := readRelease(handler.ctx, handler.tx, input.ReleaseID)
		if err != nil {
			return nil, err
		}
		if release.Revision != input.Revision {
			return nil, conflict()
		}
		if input.StationID != "" {
			if err := handler.stationAccess(input.StationID); err != nil {
				return nil, err
			}
		}
		tasks, err = many[Task](handler.ctx, handler.tx, `SELECT to_jsonb(t) FROM app.kitchen_tasks t
            JOIN app.restaurant_item_units u ON u.id=t.unit_id WHERE t.release_id=$1
            AND ($2='' OR t.station_id::text=$2) AND t.state<>'cancelled' AND u.state='active' ORDER BY t.id`, input.ReleaseID, input.StationID)
		if err != nil {
			return nil, err
		}
	} else {
		if len(input.Tasks) < 1 || len(input.Tasks) > 1000 {
			return nil, invalid("Selecciona entre 1 y 1000 unidades de preparación.")
		}
		seen := map[string]bool{}
		for _, reference := range input.Tasks {
			if err := validID(reference.ID); err != nil {
				return nil, err
			}
			if seen[reference.ID] {
				return nil, invalid("La unidad está repetida.")
			}
			seen[reference.ID] = true
			task, err := one[Task](handler.ctx, handler.tx, "SELECT to_jsonb(t) FROM app.kitchen_tasks t WHERE id=$1", reference.ID)
			if err != nil {
				return nil, err
			}
			if task.Revision != reference.Revision {
				return nil, conflict()
			}
			var tracking string
			if err := handler.tx.QueryRow(handler.ctx, "SELECT tracking FROM app.kitchen_stations WHERE id=$1", task.StationID).Scan(&tracking); err != nil {
				return nil, err
			}
			if tracking == "inherit" {
				tracking = settings.Tracking
			}
			if tracking == "order" {
				return nil, invalid("Esta estación actualiza el pedido completo; utiliza su acción por pedido.")
			}
			tasks = append(tasks, *task)
		}
	}
	if len(tasks) == 0 {
		return nil, invalid("No quedan unidades activas en ese alcance.")
	}
	releases := map[string]bool{}
	stations := map[string]bool{}
	// Validar el lote completo antes de modificar la primera unidad.
	for _, task := range tasks {
		if err := handler.stationAccess(task.StationID); err != nil {
			return nil, err
		}
		var commercialState, currentVersion string
		err := handler.tx.QueryRow(handler.ctx,
			"SELECT state,version_id FROM app.restaurant_item_units WHERE id=$1", task.UnitID).
			Scan(&commercialState, &currentVersion)
		if err != nil {
			return nil, err
		}
		if currentVersion != task.VersionID {
			return nil, invalid("La preparación tiene una modificación pendiente. Revisa y resuelve sus avisos antes de avanzar.")
		}
		if commercialState != "active" || task.State == "cancelled" || (task.State == "ready" && input.State != "ready") {
			return nil, invalid("No se puede retroceder ni preparar una unidad cancelada. Actualiza el pedido.")
		}
		releases[task.ReleaseID], stations[task.StationID] = true, true
	}
	for _, task := range tasks {
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_tasks SET state=$1,
            started_at=coalesce(started_at,now()),ready_at=CASE WHEN $1='ready' THEN coalesce(ready_at,now()) ELSE ready_at END,
            revision=revision+1,updated_at=now() WHERE id=$2`, input.State, task.ID)
		if err != nil {
			return nil, err
		}
	}
	for id := range releases {
		if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.kitchen_releases SET revision=revision+1 WHERE id=$1", id); err != nil {
			return nil, err
		}
	}
	for id := range stations {
		if err := events.Append(handler.ctx, handler.tx, "kitchen.state.changed", "kitchen", nil, &id); err != nil {
			return nil, err
		}
	}
	result := map[string]any{"updated": len(tasks), "state": input.State}
	return result, handler.history("states.changed", nil, tasks, result)
}

// AccountPreparation utiliza la pertenencia actual de las mismas unidades, incluso tras dividir.
func AccountPreparation(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	return VersionPreparation(ctx, tx, accountID, "")
}
func VersionPreparation(ctx context.Context, tx pgx.Tx, accountID, versionID string) (string, error) {
	var pendingChange bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_pending_changes c
		WHERE c.account_id=$1 AND $2='' OR EXISTS(SELECT FROM app.restaurant_item_units u
		WHERE u.account_id=$1 AND ($2='' OR u.version_id::text=$2)
		AND (coalesce(c.payload->'modified_unit_ids','[]')||coalesce(c.payload->'added_unit_ids','[]')||
		coalesce(c.payload->'cancelled_unit_ids','[]')||coalesce(c.payload->'unit_ids','[]')) ? u.id::text))`, accountID, versionID).Scan(&pendingChange); err != nil {
		return "", err
	}
	if pendingChange {
		return "change_pending", nil
	}
	var confirmed, released, total, ready, preparing int
	err := tx.QueryRow(ctx, `SELECT count(*)::integer,count(k.unit_id)::integer,
        coalesce(sum(t.total),0)::integer,coalesce(sum(t.ready),0)::integer,coalesce(sum(t.preparing),0)::integer
        FROM app.restaurant_item_units u LEFT JOIN app.kitchen_unit_releases k ON k.unit_id=u.id
        CROSS JOIN LATERAL (SELECT count(*) FILTER(WHERE state<>'cancelled') total,
            count(*) FILTER(WHERE state='ready') ready,count(*) FILTER(WHERE state='preparing') preparing
            FROM app.kitchen_tasks WHERE unit_id=u.id) t WHERE u.account_id=$1 AND u.state='active' AND ($2='' OR u.version_id::text=$2)`, accountID, versionID).
		Scan(&confirmed, &released, &total, &ready, &preparing)
	if err != nil {
		return "", err
	}
	if confirmed == 0 {
		var cancelled bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.restaurant_item_units WHERE account_id=$1 AND state='cancelled'
			AND ($2='' OR version_id::text=$2))`, accountID, versionID).Scan(&cancelled); err != nil {
			return "", err
		}
		if cancelled {
			return "cancelled", nil
		}
		return "", nil
	}
	if released < confirmed {
		var scheduled bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.kitchen_releases r WHERE state='scheduled'
            AND EXISTS(SELECT FROM app.restaurant_item_units u WHERE u.account_id=$1 AND u.state='active'
			AND ($2='' OR u.version_id::text=$2) AND u.id=ANY(r.unit_ids)))`, accountID, versionID).Scan(&scheduled); err != nil {
			return "", err
		}
		if scheduled {
			return "scheduled", nil
		}
		return "pending_release", nil
	}
	if total == ready {
		return "ready", nil
	}
	if preparing > 0 || ready > 0 {
		return "preparing", nil
	}
	return "released", nil
}
