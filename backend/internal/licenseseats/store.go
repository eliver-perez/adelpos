package licenseseats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// inUseCondition es la condición SQL «la fila alias de app.license_seats ocupa su cupo ahora» (la única definición; la
// usan el conteo, Terminales y sessions-summary). cutoff es el marcador del corte de inactividad del comandero y
// sessionNow el de la hora de las sesiones (Manager.Window); nunca now() de PostgreSQL (L1-D16):
//
//   - caja: el turno de la fila sigue abierto o en arqueo de cierre. Si la terminal se cae o se revoca, el turno sigue
//     abierto y el cupo sigue ocupado hasta cerrarlo (§7 del contrato);
//   - comandero: hubo actividad de captura después del corte. Cuenta por actividad y no por sesión (revisión R3, R3-1):
//     el pedido que entrega la cola de una tableta sin sesión, con la credencial de envío, también ocupa su cupo. Al
//     cerrarse la última sesión de la terminal la fila se borra (ReleaseClosedSessions, ReleaseDevice);
//   - cocina: la sesión que entró a Preparación sigue vigente, con el reloj de las sesiones (R3-2).
func inUseCondition(alias, cutoff, sessionNow string) string {
	return `CASE ` + alias + `.kind
        WHEN 'cash_register' THEN EXISTS (SELECT FROM app.cash_shifts shift
            WHERE shift.id = ` + alias + `.shift_id AND shift.state IN ('open', 'closing'))
        WHEN 'order_taker' THEN ` + alias + `.last_activity_at > ` + cutoff + `::timestamptz
        WHEN 'kitchen_display' THEN EXISTS (SELECT FROM app.sessions session WHERE session.id = ` + alias + `.session_id
            AND session.revoked_at IS NULL AND session.expires_at > ` + sessionNow + `::timestamptz)
        ELSE false
    END`
}

// DeviceSeatsColumn es una expresión SQL con los cupos que ocupa la terminal deviceAlias.id, como arreglo JSON de
// nombres (cash_registers, order_takers, kitchen_displays; vacío si no ocupa ninguno). cutoff y sessionNow son los
// marcadores del corte de Manager.Window y de Manager.SessionNow, en ese orden. La usa la lista de Terminales (GET
// devices).
func DeviceSeatsColumn(deviceAlias, cutoff, sessionNow string) string {
	return `(SELECT coalesce(jsonb_agg(CASE seat.kind WHEN 'cash_register' THEN 'cash_registers'
            WHEN 'order_taker' THEN 'order_takers' ELSE 'kitchen_displays' END
            ORDER BY CASE seat.kind WHEN 'cash_register' THEN 1 WHEN 'order_taker' THEN 2 ELSE 3 END), '[]'::jsonb)
        FROM app.license_seats seat WHERE seat.device_id = ` + deviceAlias + `.id AND ` + inUseCondition("seat", cutoff, sessionNow) + `)`
}

// takeLock toma el lock consultivo de los cupos (de transacción y exclusivo). Con él tomado no se publica ningún evento
// ni se pide otro lock (R3-3).
func takeLock(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(LockKey))
	return err
}

// lock toma el lock de los cupos y devuelve las horas leídas ya con el lock: así dos asignaciones nunca se intercalan y
// la segunda ve la hora y las filas de la primera.
func (manager *Manager) lock(ctx context.Context, tx pgx.Tx) (now, cutoff, sessionNow time.Time, err error) {
	if err := takeLock(ctx, tx); err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}
	now, cutoff, sessionNow = manager.times()
	return now, cutoff, sessionNow, nil
}

// claim ocupa el cupo seat para la terminal, o renueva el que ya tiene, con el lock tomado. shiftID es el turno de una
// caja y sessionID la sesión que entra a Preparación (vacíos en los demás).
//
// Contar y ocupar ocurren en la misma transacción y bajo el lock (asignación atómica, §7 del contrato). La primera
// sentencia borra las filas que ya no ocupan su cupo y cuenta las demás con la misma foto de la base. Nunca se acepta
// una fila que no vaya a contar (R3-1): la del comandero cuenta por la actividad que se escribe ahora, la de la caja
// por su turno activo (la operación tiene 7820148305) y la de la cocina por la sesión que entra, que se comprueba
// vigente en la misma sentencia (si se cerró mientras llegaba la petición, ErrSessionClosed).
func (manager *Manager) claim(ctx context.Context, tx pgx.Tx, deviceID string, seat Seat, shiftID, sessionID string) error {
	now, cutoff, sessionNow, err := manager.lock(ctx, tx)
	if err != nil {
		return err
	}
	limits, valid := manager.Limits()
	if !valid {
		return ErrNoValidLicense
	}
	var others int
	var held, sessionOpen bool
	err = tx.QueryRow(ctx, `WITH swept AS (
            DELETE FROM app.license_seats seat WHERE NOT (`+inUseCondition("seat", "$1", "$2")+`)
            RETURNING seat.device_id
        )
        SELECT count(*) FILTER (WHERE seat.kind = $3 AND seat.device_id <> $4::uuid)::int,
            coalesce(bool_or(seat.kind = $3 AND seat.device_id = $4::uuid
                AND seat.shift_id IS NOT DISTINCT FROM nullif($5, '')::uuid), false),
            ($6 = '' OR EXISTS (SELECT FROM app.sessions session WHERE session.id = nullif($6, '')::uuid
                AND session.revoked_at IS NULL AND session.expires_at > $2::timestamptz))
        FROM app.license_seats seat WHERE `+inUseCondition("seat", "$1", "$2"),
		cutoff, sessionNow, seat.kind(), deviceID, shiftID, sessionID).Scan(&others, &held, &sessionOpen)
	if err != nil {
		return err
	}
	if !sessionOpen {
		return ErrSessionClosed
	}
	if held {
		// Quien ya tiene su cupo lo conserva aunque una licencia nueva traiga menos (no se interrumpe a nadie): el
		// comandero renueva su actividad y la cocina pasa a la sesión que está en Preparación.
		_, err := tx.Exec(ctx, `UPDATE app.license_seats SET last_activity_at = greatest(last_activity_at, $3),
            session_id = coalesce(nullif($4, '')::uuid, session_id)
            WHERE device_id = $1 AND kind = $2`, deviceID, seat.kind(), now, sessionID)
		return err
	}
	limit := limits.Of(seat)
	if others >= limit {
		return &UnavailableError{Seat: seat, InUse: others, Limit: limit}
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.license_seats(device_id, kind, shift_id, session_id, acquired_at, last_activity_at)
        VALUES ($1, $2, nullif($3, '')::uuid, nullif($4, '')::uuid, $5, $5)
        ON CONFLICT (device_id, kind) DO UPDATE SET shift_id = EXCLUDED.shift_id, session_id = EXCLUDED.session_id,
            acquired_at = EXCLUDED.acquired_at, last_activity_at = EXCLUDED.last_activity_at`,
		deviceID, seat.kind(), shiftID, sessionID, now)
	return err
}

// activeShift es el turno activo (abierto o en arqueo de cierre) de la caja de la terminal, o vacío. La operación que
// pide cupo ya tomó 7820148305, con el que cambian los turnos: no cambia hasta el final de la transacción.
func activeShift(ctx context.Context, tx pgx.Tx, deviceID string) (string, error) {
	var shiftID string
	err := tx.QueryRow(ctx, `SELECT shift.id::text FROM app.devices device
        JOIN app.cash_shifts shift ON shift.register_id = device.register_id AND shift.state IN ('open', 'closing')
        WHERE device.id = $1`, deviceID).Scan(&shiftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return shiftID, err
}

// Capture aplica Capture después de que el manejador creó o modificó el pedido, en su transacción (con 7820148305 ya
// tomado): con un turno activo en su caja, la terminal ocupa su cupo de caja (L1-D3) y deja de ser comandero; si no,
// ocupa o renueva su cupo de comandero. Devuelve *UnavailableError o ErrNoValidLicense si no hay cupo: la transacción se
// revierte entera y el pedido no se guarda.
func (manager *Manager) Capture(ctx context.Context, tx pgx.Tx, deviceID string) error {
	shiftID, err := activeShift(ctx, tx, deviceID)
	if err != nil {
		return err
	}
	if shiftID == "" {
		return manager.claim(ctx, tx, deviceID, OrderTakers, "", "")
	}
	if err := manager.claim(ctx, tx, deviceID, CashRegisters, shiftID, ""); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "DELETE FROM app.license_seats WHERE device_id = $1 AND kind = $2", deviceID, kindOrderTaker)
	return err
}

// OpenShift aplica OpenShift después de abrir el turno, en su transacción: la terminal que lo abrió ocupa el cupo de
// caja de ese turno, y las terminales de esa caja dejan de ser comanderos (desde ahí capturan como caja). Sin cupo, la
// transacción se revierte y el turno no se abre.
func (manager *Manager) OpenShift(ctx context.Context, tx pgx.Tx, deviceID string) error {
	var shiftID, registerID string
	err := tx.QueryRow(ctx, `SELECT shift.id::text, shift.register_id::text FROM app.devices device
        JOIN app.cash_shifts shift ON shift.register_id = device.register_id AND shift.state = 'open'
        WHERE device.id = $1`, deviceID).Scan(&shiftID, &registerID)
	if err != nil {
		return fmt.Errorf("turno recién abierto de la terminal: %w", err)
	}
	if err := manager.claim(ctx, tx, deviceID, CashRegisters, shiftID, ""); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM app.license_seats seat USING app.devices device
        WHERE device.id = seat.device_id AND device.register_id = $1 AND seat.kind = $2`, registerID, kindOrderTaker)
	return err
}

// Renew aplica Preview: renueva la actividad del comandero de la terminal solo si todavía lo tiene. Nunca ocupa un cupo
// ni rechaza la previa, y no toma el lock de los cupos (revisión R3, R3-4): las previas no se forman en fila.
//
// Es seguro sin el lock porque el UPDATE es condicional y nunca revive un cupo vencido: solo toca la fila que sigue en
// uso con la hora de esta previa. La previa tiene 7820148305 compartido (su manejador) y toda asignación de comandero lo
// tiene exclusivo (captura y pedido sin conexión), así que nunca se intercalan: si la asignación fue antes, la previa lee
// su hora después y ve el cupo tan vencido como lo vio la asignación; si la previa fue antes, la asignación ve la
// actividad renovada. Los borrados (liberar, cerrar sesión, abrir turno, el barrido) chocan en la fila: si el borrado va
// primero, la previa no encuentra nada; si va después, la borra. La hora nunca retrocede (greatest).
func (manager *Manager) Renew(ctx context.Context, tx pgx.Tx, deviceID string) error {
	now, cutoff, _ := manager.times()
	_, err := tx.Exec(ctx, `UPDATE app.license_seats SET last_activity_at = greatest(last_activity_at, $1)
        WHERE device_id = $3 AND kind = $4 AND last_activity_at > $2`,
		now, cutoff, deviceID, kindOrderTaker)
	return err
}

// SetKitchenDisplay ocupa el cupo de cocina de la terminal al entrar a Preparación (ligado a la sesión que entra) o lo
// libera al salir. Salir nunca falla por la licencia.
func (manager *Manager) SetKitchenDisplay(ctx context.Context, tx pgx.Tx, deviceID, sessionID string, active bool) error {
	if active {
		return manager.claim(ctx, tx, deviceID, KitchenDisplays, "", sessionID)
	}
	if err := takeLock(ctx, tx); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "DELETE FROM app.license_seats WHERE device_id = $1 AND kind = $2", deviceID, kindKitchenDisplay)
	return err
}

// closedSessionsCondition es la condición sobre la fila seat «comandero de una terminal a la que esta transacción le
// cerró una sesión y que ya no tiene ninguna vigente». Las sesiones que se cierran en la transacción tienen revoked_at
// = now(), la hora de inicio de la transacción, igual para todas sus sentencias (cerrar sesión, revocarla o revocar la
// terminal, cambiar la contraseña o el PIN, desactivar al usuario, la sesión vencida). $1 es la hora de las sesiones.
const closedSessionsCondition = `seat.kind = 'order_taker'
        AND EXISTS (SELECT FROM app.sessions closed WHERE closed.device_id = seat.device_id AND closed.revoked_at = now())
        AND NOT EXISTS (SELECT FROM app.sessions open WHERE open.device_id = seat.device_id AND open.revoked_at IS NULL
            AND open.expires_at > $1::timestamptz)`

// ReleaseClosedSessions borra el comandero de las terminales a las que esta transacción les cerró la última sesión
// (revisión R3, R3-1): al cerrar sesión el último usuario de esa terminal se libera en ese momento, aunque su última
// captura tenga menos de 15 minutos. sessionNow es el reloj con que se escriben y validan las sesiones. Solo toma el
// lock de los cupos si hay algo que borrar; se llama después del trabajo de la operación (después de 7820148306).
func ReleaseClosedSessions(ctx context.Context, tx pgx.Tx, sessionNow time.Time) error {
	var pending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT FROM app.license_seats seat WHERE `+closedSessionsCondition+`)`,
		sessionNow).Scan(&pending); err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if err := takeLock(ctx, tx); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM app.license_seats seat WHERE `+closedSessionsCondition, sessionNow)
	return err
}

// ReleaseClosedSessions es la misma liberación con el reloj de las sesiones del gestor.
func (manager *Manager) ReleaseClosedSessions(ctx context.Context, tx pgx.Tx) error {
	return ReleaseClosedSessions(ctx, tx, manager.sessionNow())
}

// ReleaseDevice libera los cupos de comandero y de cocina de la terminal (Terminales › «Liberar cupo» y revocar la
// terminal). Quien llama ya cerró sus sesiones; las cajas se liberan cerrando el turno.
func (manager *Manager) ReleaseDevice(ctx context.Context, tx pgx.Tx, deviceID string) error {
	if err := takeLock(ctx, tx); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "DELETE FROM app.license_seats WHERE device_id = $1 AND kind IN ($2, $3)",
		deviceID, kindOrderTaker, kindKitchenDisplay)
	return err
}

// DeviceSeats son los cupos que ocupa la terminal en este instante, en el orden de Seats.
func (manager *Manager) DeviceSeats(ctx context.Context, tx pgx.Tx, deviceID string) ([]Seat, error) {
	_, cutoff, sessionNow := manager.times()
	rows, err := tx.Query(ctx, `SELECT seat.kind FROM app.license_seats seat
        WHERE seat.device_id = $3 AND `+inUseCondition("seat", "$1", "$2"), cutoff, sessionNow, deviceID)
	if err != nil {
		return nil, err
	}
	kinds, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	held := []Seat{}
	for _, seat := range Seats {
		for _, kind := range kinds {
			if seatOfKind(kind) == seat {
				held = append(held, seat)
			}
		}
	}
	return held, nil
}

// Usage es cuántos cupos de cada tipo están en uso y el límite de la licencia (0 sin licencia válida), para
// sessions-summary y Configuración › Licencia.
func (manager *Manager) Usage(ctx context.Context, tx pgx.Tx) (map[Seat]Usage, error) {
	_, cutoff, sessionNow := manager.times()
	limits, _ := manager.Limits()
	usage := map[Seat]Usage{}
	for _, seat := range Seats {
		usage[seat] = Usage{Limit: limits.Of(seat)}
	}
	rows, err := tx.Query(ctx, `SELECT seat.kind, count(*)::int FROM app.license_seats seat
        WHERE `+inUseCondition("seat", "$1", "$2")+` GROUP BY seat.kind`, cutoff, sessionNow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		seat := seatOfKind(kind)
		current := usage[seat]
		current.InUse = count
		usage[seat] = current
	}
	return usage, rows.Err()
}
