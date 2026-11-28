package tableunions

import (
	"context"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/events"
)

// Motivos por los que una unión se deshace (dissolve_reason).
const (
	ReasonNoAccounts  = "no_accounts"
	ReasonSingleTable = "single_table"
)

// Actor es quien hace la operación que deshace o cambia la unión (su usuario y su terminal), para el historial.
type Actor struct {
	UserID   string
	DeviceID string
}

// ChangedKind es el evento de tiempo real de cualquier cambio de uniones (ámbito restaurant). operations.Handle lo
// publica por su regla genérica en las rutas restaurant/table-unions; SettleEmpty lo publica cuando deshace una unión
// desde otra ruta (un cobro, una cancelación o mover la última cuenta).
const ChangedKind = "restaurant.table-unions.changed"

// AccountKind es el tipo del evento de cada cuenta abierta afectada por un cambio de su unión (unir, agregar, quitar o
// deshacer). Está en kitchen.displayAccountKinds: la pantalla de cocina vuelve a leer el encabezado de la tarjeta.
const AccountKind = "account.table_union"

// SettleEmpty deshace, al final de cada escritura de operations.Handle y de payments.Handle (M1-D7), las uniones
// abiertas que ya no tienen ninguna cuenta abierta en sus mesas o que tienen menos de dos mesas. Es el único lugar
// donde una unión se deshace sola: cubre el cobro, el cierre, la cancelación, el descarte, la liberación, combinar,
// mover y cualquier camino nuevo, sin una llamada en cada ruta. Quien lo llama ya tiene 7820148305 exclusivo, así que
// ninguna otra escritura del restaurante corre a la vez. Devuelve las uniones que deshizo.
//
// Las mesas quedan libres y separadas (left_at en todos sus miembros), la unión queda dissolved con su historial y se
// publica restaurant.table-unions.changed una vez. Una unión con menos de dos mesas y cuentas abiertas no debería
// existir (quitar la penúltima mesa la deshace en la misma operación, con su historial por cuenta): si aparece, sus
// cuentas que se llamaban como la unión vuelven al nombre de su mesa y cada una publica su evento de unión.
func SettleEmpty(ctx context.Context, tx pgx.Tx, actor Actor, operationID string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT u.id::text,
            NOT EXISTS(SELECT FROM app.table_union_members m JOIN app.restaurant_accounts a ON a.table_id=m.table_id
                WHERE m.union_id=u.id AND m.left_at IS NULL AND a.state='open')
        FROM app.table_unions u
        WHERE u.state='open' AND (
            NOT EXISTS(SELECT FROM app.table_union_members m JOIN app.restaurant_accounts a ON a.table_id=m.table_id
                WHERE m.union_id=u.id AND m.left_at IS NULL AND a.state='open')
            OR (SELECT count(*) FROM app.table_union_members m WHERE m.union_id=u.id AND m.left_at IS NULL) < 2)
        ORDER BY u.id FOR UPDATE OF u`)
	if err != nil {
		return nil, err
	}
	type settled struct {
		id    string
		empty bool
	}
	pending := []settled{}
	for rows.Next() {
		var item settled
		if err := rows.Scan(&item.id, &item.empty); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	dissolved := []string{}
	for _, item := range pending {
		reason := ReasonNoAccounts
		if !item.empty {
			reason = ReasonSingleTable
			renamed, err := RestoreAccountNames(ctx, tx, item.id)
			if err != nil {
				return nil, err
			}
			for _, accountID := range renamed {
				account := accountID
				if err := events.Append(ctx, tx, "restaurant."+AccountKind, "restaurant", &account, nil); err != nil {
					return nil, err
				}
			}
		}
		if err := Dissolve(ctx, tx, item.id, reason, actor, operationID); err != nil {
			return nil, err
		}
		dissolved = append(dissolved, item.id)
	}
	if len(dissolved) > 0 {
		if err := events.Append(ctx, tx, ChangedKind, "restaurant", nil, nil); err != nil {
			return nil, err
		}
	}
	return dissolved, nil
}

// Dissolve deshace una unión abierta: su historial («dissolved» con el nombre que tenía), left_at en sus mesas y el
// estado dissolved con quién, la terminal y el motivo. No toca las cuentas: quien llama ya les devolvió su nombre.
//
// Las horas de una unión (apertura, entrada y salida de mesas, deshacerse y su historial) son clock_timestamp(), la hora
// real del momento, y no now(), la hora en que empezó la transacción (M1-D30). Quien escribe ya tiene 7820148305
// exclusivo, así que esas horas siguen el orden del lock: una petición que empezó antes y esperó el lock no deja una hora
// anterior a la de cambios que ya se confirmaron antes que ella. El historial se ordena además por su secuencia.
func Dissolve(ctx context.Context, tx pgx.Tx, unionID, reason string, actor Actor, operationID string) error {
	if err := AppendHistory(ctx, tx, unionID, "dissolved", nil, nil, actor, operationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.table_union_members SET left_at=clock_timestamp(),left_by=$2
        WHERE union_id=$1 AND left_at IS NULL`, unionID, actor.UserID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE app.table_unions SET state='dissolved',dissolved_at=clock_timestamp(),dissolved_by=$2,
        dissolved_device_id=$3,dissolve_reason=$4,revision=revision+1,updated_at=clock_timestamp()
        WHERE id=$1 AND state='open'`, unionID, actor.UserID, actor.DeviceID, reason)
	return err
}

// AppendHistory agrega una fila al historial de la unión con su nombre vigente (union_name) y, si aplica, la mesa con
// su nombre de ese momento.
func AppendHistory(ctx context.Context, tx pgx.Tx, unionID, kind string, tableID, tableName *string, actor Actor,
	operationID string) error {
	var operation *string
	if operationID != "" {
		operation = &operationID
	}
	_, err := tx.Exec(ctx, `INSERT INTO app.table_union_events(id,union_id,kind,table_id,table_name,union_name,operation_id,
            author_id,device_id,created_at)
        SELECT $1,u.id,$2,$3,$4,u.name,$5,$6,$7,clock_timestamp() FROM app.table_unions u WHERE u.id=$8`,
		events.NewID(), kind, tableID, tableName, operation, actor.UserID, actor.DeviceID, unionID)
	return err
}

// RestoreAccountNames devuelve el nombre de su mesa a las cuentas abiertas de la unión que se llamaban como ella (la
// referencia neutra de una cuenta sin nombre propio, M1-D14, recortada con AccountReference si la unión tiene un nombre
// largo), como movedAccountName al mover una cuenta. Devuelve las cuentas que cambiaron. No sube la revisión de la
// cuenta: un cambio de la unión no es una edición de la cuenta, y así la terminal que está capturando en ella no recibe
// un conflicto por algo que hizo otra con las mesas.
func RestoreAccountNames(ctx context.Context, tx pgx.Tx, unionID string) ([]string, error) {
	var unionName string
	if err := tx.QueryRow(ctx, "SELECT name FROM app.table_unions WHERE id=$1", unionID).Scan(&unionName); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `UPDATE app.restaurant_accounts a SET name=t.name,updated_at=now()
        FROM app.table_union_members m, app.restaurant_tables t
        WHERE m.union_id=$1 AND m.left_at IS NULL AND t.id=m.table_id
            AND a.table_id=m.table_id AND a.state='open' AND a.name=$2
        RETURNING a.id::text`, unionID, AccountReference(unionName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	renamed := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		renamed = append(renamed, id)
	}
	return renamed, rows.Err()
}
