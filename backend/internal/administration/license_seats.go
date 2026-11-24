package administration

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licenseseats"
)

// Cupos de la licencia (etapa L1, Fase D; §5 de la especificación y §7 del contrato). La clasificación de qué operación
// ocupa, renueva o libera cada cupo es licenseseats.ForOperation; aquí se aplica en un solo punto (withSeats, que llama
// dispatch después de autenticar) y viven las dos rutas que solo tratan de cupos: el modo Cocina y «Liberar cupo».

// failClosedSeats es el gestor de un Service sin Seats: sin fuente de límites no hay licencia válida y ninguna
// operación que pide cupo procede. Nunca hay un «sin límite».
var failClosedSeats = licenseseats.NewManager(licenseseats.Options{})

// seats es el gestor de cupos del servicio.
func (service *Service) seats() *licenseseats.Manager {
	if service.Seats != nil {
		return service.Seats
	}
	return failClosedSeats
}

// withSeats ejecuta la operación (run, que la autoriza y la aplica en su savepoint) y aplica su efecto en los cupos, en
// la misma transacción:
//
//   - Capture y OpenShift: después de que el manejador hizo su trabajo (con 7820148305 tomado y sus eventos ya
//     publicados con 7820148306), se cuenta y se ocupa el cupo con 7820148311, el último lock de la operación: con él
//     tomado no se publica nada ni se pide otro lock (R3-3). Sin cupo, la respuesta es 409 LICENSE_SEAT_UNAVAILABLE y
//     toda la transacción se revierte: no se guarda el pedido, ni la clave de operación, ni el turno. Así los
//     errores propios de la operación (datos inválidos, mesa ocupada, turno ya abierto…) se informan primero y un
//     rechazo de la operación nunca ocupa un cupo.
//   - Una operación cuya clave (operation_id) ya está registrada no vuelve a pedir cupo: el manejador responde con su
//     resultado guardado o con su conflicto y nunca registra nada nuevo. Así un reintento de algo que ya entró (por
//     ejemplo, la cola de Android después de una respuesta perdida) no se rechaza por un cupo que ya venció.
//   - Preview: renueva la actividad sin el lock de los cupos (Renew, R3-4).
//   - SessionEnd: después de cerrar sesiones, el comandero de cada terminal que se quedó sin ninguna vigente se borra en
//     ese momento (ReleaseClosedSessions, revisión R3, R3-1): cuenta por actividad, así que no basta con el conteo.
//   - Los demás efectos no piden nada aquí: cerrar el turno libera sin escribir (el conteo es derivado) y el modo Cocina
//     y «Liberar cupo» lo hacen en su propia ruta.
func (service *Service) withSeats(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string,
	run func() (any, error)) (any, error) {
	effect := licenseseats.ForOperation(request.Method, route)
	switch effect {
	case licenseseats.Capture, licenseseats.OpenShift:
		recorded, err := operationRecorded(ctx, tx, request.Body)
		if err != nil {
			return nil, err
		}
		result, err := run()
		if err != nil || recorded {
			return result, err
		}
		if effect == licenseseats.Capture {
			err = service.seats().Capture(ctx, tx, principal.DeviceID)
		} else {
			err = service.seats().OpenShift(ctx, tx, principal.DeviceID)
		}
		if err != nil {
			return nil, seatFault(err)
		}
		return result, nil
	case licenseseats.Preview:
		result, err := run()
		if err != nil {
			return result, err
		}
		if err := service.seats().Renew(ctx, tx, principal.DeviceID); err != nil {
			return nil, err
		}
		return result, nil
	case licenseseats.SessionEnd:
		result, err := run()
		if err != nil {
			return result, err
		}
		if err := service.seats().ReleaseClosedSessions(ctx, tx); err != nil {
			return nil, err
		}
		return result, nil
	}
	return run()
}

// operationRecorded dice si la clave de operación del cuerpo ya tiene un resultado guardado (app.restaurant_operations,
// la tabla de idempotencia de cuentas, pedidos y cobros). Un cuerpo sin clave válida no está registrado: el manejador
// lo rechaza.
func operationRecorded(ctx context.Context, tx pgx.Tx, body json.RawMessage) (bool, error) {
	var mutation struct {
		OperationID string `json:"operation_id"`
	}
	if json.Unmarshal(body, &mutation) != nil || !identifierPattern.MatchString(mutation.OperationID) {
		return false, nil
	}
	var recorded bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM app.restaurant_operations WHERE id=$1)", mutation.OperationID).Scan(&recorded)
	return recorded, err
}

// seatUnavailable dice si la operación se rechazó por falta de cupo (409 LICENSE_SEAT_UNAVAILABLE de seatFault).
func seatUnavailable(err error) bool {
	var fault *Fault
	return errors.As(err, &fault) && fault.Code == "LICENSE_SEAT_UNAVAILABLE"
}

// settleFutureSeatActivity guarda, después de un rechazo por falta de cupo, la actividad de comandero con fecha futura
// bajada a «ahora» (hallazgo R2-2, L1-D43; licenseseats.SettleFutureActivity), en una transacción corta propia que solo
// toma 7820148311. Sin esto, el rechazo revierte lo que bajó claim, cada intento vuelve a encontrar esa actividad en el
// futuro y el cupo sigue «en uso» hasta que el reloj real alcance la fecha del salto: la otra tableta nunca entra.
//
// Así el cupo vence 15 minutos reales después del primer intento con la hora corregida. Si falla, el rechazo se responde
// igual (el siguiente intento lo vuelve a hacer) y el detalle va al registro.
func (service *Service) settleFutureSeatActivity(ctx context.Context) {
	err := pgx.BeginFunc(ctx, service.Pool, func(tx pgx.Tx) error {
		return service.seats().SettleFutureActivity(ctx, tx)
	})
	if err != nil && service.Logger != nil {
		service.Logger.Warn("license_seats_settle_failed", "reason", err.Error())
	}
}

// seatFault traduce el rechazo de los cupos a su error público (L1-D17, todos 409 para que no entren en la auditoría de
// los 403). El mensaje va en español sencillo para los clientes anteriores a la etapa; los nuevos deciden por el código.
func seatFault(err error) error {
	var unavailable *licenseseats.UnavailableError
	if errors.As(err, &unavailable) {
		return &Fault{Status: 409, Code: "LICENSE_SEAT_UNAVAILABLE", Message: unavailable.Message(), Details: map[string]any{
			"seat":   unavailable.Seat,
			"in_use": unavailable.InUse,
			"limit":  unavailable.Limit,
		}}
	}
	// La sesión que pedía entrar a Preparación se cerró mientras llegaba la petición (por ejemplo, «Liberar cupo»).
	if errors.Is(err, licenseseats.ErrSessionClosed) {
		return unauthorized("SESSION_REVOKED", "Tu sesión fue revocada. Inicia sesión nuevamente.")
	}
	// Protección de respaldo: la puerta de la Fase C rechaza antes estas operaciones sin licencia válida, con el mismo
	// código y el estado de la licencia en details.
	if errors.Is(err, licenseseats.ErrNoValidLicense) {
		return &Fault{Status: 409, Code: "LICENSE_NOT_ACTIVATED",
			Message: "La licencia del sistema no permite esta operación. Avisa al encargado."}
	}
	return err
}

// kitchenDisplayMode atiende POST kitchen/display-mode {active} (L1-D7): la terminal entra a Preparación (ocupa el cupo
// de cocina, ligado a esta sesión) o sale (lo libera). Lo pide quien puede ver Preparación (kitchen.states.read). Los
// teléfonos y tabletas de meseros no tienen modo Cocina. No lleva clave de operación: repetirlo deja el mismo estado.
func (service *Service) kitchenDisplayMode(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if err := principal.require("kitchen.states.read"); err != nil {
		return nil, err
	}
	var input struct {
		Active *bool `json:"active"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Active == nil {
		return nil, invalid("Indica si la terminal entra o sale de Preparación.")
	}
	var kind string
	if err := tx.QueryRow(ctx, "SELECT kind FROM app.devices WHERE id=$1", principal.DeviceID).Scan(&kind); err != nil {
		return nil, err
	}
	if kind == "android" {
		return nil, invalid("Los teléfonos y tabletas de meseros no tienen Preparación.")
	}
	if err := service.seats().SetKitchenDisplay(ctx, tx, principal.DeviceID, principal.SessionID, *input.Active); err != nil {
		return nil, seatFault(err)
	}
	return map[string]bool{"active": *input.Active}, nil
}

// releaseDeviceSeat atiende POST devices/{id}/seat/release (devices.manage): libera el cupo de comandero o de cocina de
// esa terminal y cierra sus sesiones, con la auditoría license.seat_released. Las cajas se liberan cerrando el turno.
//
// Las sesiones se cierran antes de tomar el lock de los cupos: una captura en curso de esa terminal ya tiene bloqueada
// la fila de su sesión (authenticateSession) y después pide el lock; al revés se esperarían una a la otra.
func releaseDeviceSeat(ctx context.Context, tx pgx.Tx, principal Principal, deviceID string, previous json.RawMessage,
	seats *licenseseats.Manager) (any, error) {
	held, err := seats.DeviceSeats(ctx, tx, deviceID)
	if err != nil {
		return nil, err
	}
	released := []licenseseats.Seat{}
	for _, seat := range held {
		if seat != licenseseats.CashRegisters {
			released = append(released, seat)
		}
	}
	if len(released) == 0 {
		message := "Esta terminal no ocupa un cupo de comandero ni de pantalla de cocina."
		if slices.Contains(held, licenseseats.CashRegisters) {
			message = "Esta terminal ocupa un cupo de caja: se libera al cerrar su turno."
		}
		return nil, &Fault{Status: 409, Code: "LICENSE_SEAT_NOT_HELD", Message: message, Details: map[string]any{"seats": held}}
	}
	closed, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='seat_released' WHERE device_id=$1 AND revoked_at IS NULL", deviceID)
	if err != nil {
		return nil, err
	}
	if err := seats.ReleaseDevice(ctx, tx, deviceID); err != nil {
		return nil, err
	}
	after := map[string]any{"released": released, "sessions_closed": closed.RowsAffected()}
	if err := audit(ctx, tx, principal, "license.seat_released", "device", &deviceID, previous, after, true); err != nil {
		return nil, err
	}
	return after, nil
}
