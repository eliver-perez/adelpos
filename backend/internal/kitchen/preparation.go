package kitchen

import (
	"sort"

	"local/ad-el-pos/server/internal/events"
)

// Evento de preparación para el mesero (etapa A1, decisión A1-D11 del coordinador). Los cambios de cocina se publican
// en el ámbito «kitchen» con su estación (kitchen.state.changed, kitchen.released…), que solo recibe quien tiene
// kitchen.states.read y ve esa estación; un mesero con accounts.read y sin permisos de cocina no se enteraba de que su
// pedido pasó a «Preparando» o «Listo». Por eso, en la misma transacción en que cambian los registros de cocina que
// deciden el estado de preparación de una cuenta (liberaciones, unidades liberadas, tareas por estación y avisos de
// cambio pendientes; kitchen.VersionPreparation), se publica además restaurant.account.preparation_changed con la
// cuenta como agregado:
//
//   - en el ámbito de las demás novedades de la cuenta (queries.go de operations): «restaurant» (accounts.read) o
//     «takeaway» (takeaway.read) para una cuenta para llevar;
//   - uno solo por cuenta y por transacción, aunque cambien muchas unidades, partidas o estaciones;
//   - como todo evento, sin contenido: el cliente vuelve a leer la cuenta (GET restaurant/accounts/{id}).
//
// Lo publican la API de cocina (avance a En preparación o Listo, liberar o programar un pedido y reintentar un aviso
// pendiente), la liberación automática de los pedidos programados (worker.go) y la integración con cocina de las
// operaciones de la cuenta (confirmar, modificar, cancelar unidades o la cuenta y reprogramar), que además publican su
// propio evento restaurant.* de la cuenta.
//
// Registro de platillos listos (etapa A2, parte D; decisión A2-D9). El aviso al mesero necesita saber qué pasó a
// «Listo» y para quién, y que nada se pierda ni se repita aunque se corte la red: en la misma transacción en que se
// publica el evento de preparación, cada unidad de esas cuentas cuya última tarea viva pasó a «Listo» en esta
// transacción queda en app.ready_notices con el responsable de su cuenta en ese momento (recordReadyNotices). Lo leen
// los dispositivos con GET ready-alerts (administration/ready_alerts.go).

// PreparationChangedKind es el tipo del evento.
const PreparationChangedKind = "restaurant.account.preparation_changed"

// notePreparationAccount anota que cambió la preparación de la cuenta en esta transacción.
func (handler *service) notePreparationAccount(accountID string) {
	if handler.preparation == nil {
		handler.preparation = map[string]bool{}
	}
	handler.preparation[accountID] = true
}

// notePreparationUnits anota las cuentas en que están hoy esas unidades. Se usa la pertenencia actual (no la cuenta
// de la liberación) porque una unidad transferida o dividida lleva su preparación a la cuenta donde está.
func (handler *service) notePreparationUnits(unitIDs []string) error {
	if len(unitIDs) == 0 {
		return nil
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT DISTINCT account_id::text FROM app.restaurant_item_units
        WHERE id=ANY($1::uuid[])`, unitIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	accounts := []string{}
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return err
		}
		accounts = append(accounts, accountID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, accountID := range accounts {
		handler.notePreparationAccount(accountID)
	}
	return nil
}

// publishPreparation publica un evento por cada cuenta anotada, en orden estable, y vacía la lista para que una
// segunda llamada en la misma operación no los repita.
func (handler *service) publishPreparation() error {
	if len(handler.preparation) == 0 {
		return nil
	}
	accountIDs := make([]string, 0, len(handler.preparation))
	for accountID := range handler.preparation {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Strings(accountIDs)
	handler.preparation = nil
	if err := handler.recordReadyNotices(accountIDs); err != nil {
		return err
	}
	rows, err := handler.tx.Query(handler.ctx, `SELECT id::text,mode FROM app.restaurant_accounts
        WHERE id=ANY($1::uuid[]) ORDER BY id`, accountIDs)
	if err != nil {
		return err
	}
	type accountScope struct {
		id   string
		mode string
	}
	accounts := []accountScope{}
	for rows.Next() {
		var account accountScope
		if err := rows.Scan(&account.id, &account.mode); err != nil {
			rows.Close()
			return err
		}
		accounts = append(accounts, account)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, account := range accounts {
		// La misma regla de ámbito que los eventos de la cuenta (operations/queries.go): una cuenta para llevar solo
		// la ve quien consulta el tablero de para llevar.
		scope := "restaurant"
		if account.mode == "takeaway" {
			scope = "takeaway"
		}
		accountID := account.id
		if err := events.Append(handler.ctx, handler.tx, PreparationChangedKind, scope, &accountID, nil); err != nil {
			return err
		}
	}
	return nil
}

// recordReadyNotices registra cada unidad activa de las cuentas que quedó en «Listo» en esta transacción: todas sus
// tareas vivas (no canceladas, de su versión vigente) están listas y, o bien la última se marcó ahora (ready_at es
// now(), el instante de la transacción; states.go conserva el ready_at de una tarea que ya estaba lista), o bien ahora
// se canceló la que faltaba sin haber estado lista (revisión R1: una modificación quita la parte pendiente de un combo
// y lo que queda ya estaba listo; la cancelación conserva ready_at, así que quitar una parte que ya estaba lista no
// vuelve a avisar). Los caminos que escriben tareas son tres y todos pasan por aquí: el avance del KDS (states.go: una
// unidad, el pedido de una estación o la cuenta completa), las modificaciones y cancelaciones con sus reintentos
// (changes.go) y la liberación (releases.go, que solo crea tareas pendientes). Nunca se avisa una unidad que ya estaba
// lista: una modificación que no cambia la receta conserva sus tareas listas; una que la cambia las reinicia y su nueva
// versión se avisa al estar lista otra vez. UNIQUE (unit_id, version_id) impide repetir la misma preparación.
//
// Las unidades de una misma cuenta comparten alert_key, un uuid aleatorio que nunca se reutiliza (revisión R3): es la
// clave con que el dispositivo descarta avisos repetidos, aunque se restaure un respaldo y sequence repita números. El
// destinatario es el responsable actual de la cuenta en que está la unidad.
//
// El dispositivo destinatario (A2-D4) se fija aquí: entre los dispositivos android que pueden recibir el aviso (no
// revocados, con «Avisar platillos listos» activado y con la aplicación 0.2.0 o posterior si Go conoce su versión, la
// primera que lee avisos; revisión R7), el de la sesión con PIN vigente del responsable con last_seen_at más reciente;
// si ninguno sirve, ninguno (entonces el aviso va a todos los dispositivos donde entró; lo decide la lectura,
// administration/ready_alerts.go). Fijarlo al «Listo» y no al leer hace que cada dispositivo lo reciba o no sin
// importar cuándo consulte: si el mesero cambia de dispositivo justo entonces o uno se reconecta tarde con su cursor,
// el aviso no se pierde ni se duplica. Al registrar algo se podan los avisos de más de 2 días (dato derivado).
func (handler *service) recordReadyNotices(accountIDs []string) error {
	recorded, err := handler.tx.Exec(handler.ctx, `WITH ready AS (
            SELECT u.id AS unit_id,u.version_id,u.account_id,a.assigned_user_id,u.item_id,u.ordinal
            FROM app.restaurant_item_units u JOIN app.restaurant_accounts a ON a.id=u.account_id
            CROSS JOIN LATERAL (SELECT
                count(*) FILTER (WHERE t.version_id=u.version_id AND t.state<>'cancelled') AS live,
                count(*) FILTER (WHERE t.version_id=u.version_id AND t.state='ready') AS ready,
                max(t.ready_at) FILTER (WHERE t.version_id=u.version_id AND t.state='ready') AS last_ready,
                count(*) FILTER (WHERE t.state='cancelled' AND t.ready_at IS NULL AND t.updated_at=now()) AS cancelled_now
                FROM app.kitchen_tasks t WHERE t.unit_id=u.id) tasks
            WHERE u.account_id=ANY($1::uuid[]) AND u.state='active' AND tasks.live>0 AND tasks.ready=tasks.live
                AND (tasks.last_ready=now() OR tasks.cancelled_now>0)
        ), keys AS (
            SELECT account_id,gen_random_uuid() AS alert_key FROM (SELECT DISTINCT account_id FROM ready) accounts
        )
        INSERT INTO app.ready_notices(unit_id,version_id,account_id,recipient_user_id,recipient_device_id,alert_key,ready_at)
        SELECT r.unit_id,r.version_id,r.account_id,r.assigned_user_id,
            (SELECT s.device_id FROM app.sessions s JOIN app.devices d ON d.id=s.device_id
                WHERE s.user_id=r.assigned_user_id AND s.opened_with_pin AND s.revoked_at IS NULL AND s.expires_at>now()
                    AND d.kind='android' AND d.revoked_at IS NULL AND d.ready_alerts_enabled
                    AND (coalesce(d.app_version,'') !~ '^\d{1,6}\.\d{1,6}\.\d{1,6}'
                        OR (regexp_match(d.app_version,'^(\d{1,6})\.(\d{1,6})\.(\d{1,6})'))::int[] >= ARRAY[0,2,0])
                ORDER BY s.last_seen_at DESC,s.created_at DESC,s.id LIMIT 1),
            k.alert_key,now()
        FROM ready r JOIN keys k ON k.account_id=r.account_id
        ORDER BY r.account_id,r.item_id,r.ordinal,r.unit_id
        ON CONFLICT (unit_id,version_id) DO NOTHING`, accountIDs)
	if err != nil {
		return err
	}
	if recorded.RowsAffected() == 0 {
		return nil
	}
	_, err = handler.tx.Exec(handler.ctx, "DELETE FROM app.ready_notices WHERE ready_at < now() - interval '2 days'")
	return err
}
