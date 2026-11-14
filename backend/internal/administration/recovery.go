package administration

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/operations"
)

// recoveryState atiende GET restaurant/state (etapa 9G, §5.6): lo que necesita un cliente al reconectarse. Responde
// a cualquier sesión válida, aunque el usuario ya no pueda consultar cuentas, para que el cliente se entere de los
// permisos que perdió: la sesión con los permisos vigentes (leídos de PostgreSQL en esta misma petición), la
// terminal, la versión del catálogo (para saber si debe descargar cambios) y, según sus permisos, las mesas y las
// cuentas abiertas con su revisión y sus estados de preparación. Una terminal revocada no llega aquí: recibe
// 401 DEVICE_REVOKED, y una sesión cerrada, 401 SESSION_*.
func recoveryState(ctx context.Context, tx pgx.Tx, principal Principal) (any, error) {
	restaurant, err := operations.Recovery(ctx, tx, operations.Request{
		Method: "GET",
		Route:  "restaurant/state",
		Actor:  operations.Actor{UserID: principal.UserID, DeviceID: principal.DeviceID, Permissions: principal.Permissions},
	})
	if err != nil {
		return nil, err
	}
	// El lock del catálogo se toma después del de la operación (7820148305 antes que 7820148304).
	version, err := catalog.Version(ctx, tx)
	if err != nil {
		return nil, err
	}
	// Con la época el cliente sabe si su catálogo guardado sigue siendo de esta línea de tiempo (revisión de la fase
	// D, hallazgo 2): con otra época descarga todo.
	epoch, err := catalog.Epoch(ctx, tx)
	if err != nil {
		return nil, err
	}
	terminal, err := object(ctx, tx, "SELECT id,name,register_id,keyboard_mode,home_screen FROM app.devices WHERE id=$1", principal.DeviceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"server_time":     time.Now().UTC(),
		"catalog_version": version,
		"catalog_epoch":   epoch,
		"user":            principal,
		"terminal":        terminal,
		"tables":          restaurant.Tables,
		"accounts":        restaurant.Accounts,
	}, nil
}
