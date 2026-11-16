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
