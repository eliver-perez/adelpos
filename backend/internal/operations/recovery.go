package operations

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Recuperación del estado al reconectar (etapa 9G, §5.6). Una terminal o un teléfono que perdió la conexión no
// depende de haber recibido todos los eventos en tiempo real: al volver consulta el estado vigente con
// GET restaurant/state (administration/recovery.go), que junta la sesión y los permisos actuales del usuario con lo
// que devuelve Recovery: las mesas con su ocupación y las cuentas abiertas con su revisión y sus estados de
// preparación, pago y entrega, calculados por Go igual que en Mesas y Cuentas.

// RecoveryView es la parte del restaurante del estado. Cada lista es nil si el usuario ya no tiene el permiso de
// consultarla (restaurant.tables.read para las mesas, accounts.read para las cuentas; las de para llevar exigen
// además takeaway.read).
type RecoveryView struct {
	Tables   []Table   `json:"tables"`
	Accounts []Account `json:"accounts"`
}

// Recovery lee el estado con el lock de la operación compartido, como cualquier consulta de Mesas o Cuentas.
func Recovery(ctx context.Context, tx pgx.Tx, request Request) (*RecoveryView, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148305)"); err != nil {
		return nil, err
	}
	handler := service{ctx: ctx, tx: tx, request: request}
	view := &RecoveryView{}
	if request.Actor.Can("restaurant.tables.read") {
		tables, err := readList[Table](ctx, tx, tableQuery+" ORDER BY t.area_id,t.name,t.id")
		if err != nil {
			return nil, err
		}
		view.Tables = tables
	}
	if !request.Actor.Can("accounts.read") {
		return view, nil
	}
	accounts, err := readList[Account](ctx, tx, accountQuery+` WHERE a.state='open' AND (a.mode<>'takeaway' OR $1)
        ORDER BY a.created_at,a.id`, request.Actor.Can("takeaway.read"))
	if err != nil {
		return nil, err
	}
	for index := range accounts {
		if err := handler.decorateAccount(&accounts[index]); err != nil {
			return nil, err
		}
	}
	view.Accounts = accounts
	return view, nil
}
