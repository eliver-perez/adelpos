package administration

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const devicesQuery = `SELECT d.id,d.name,d.register_id,r.name AS register_name,d.keyboard_mode,d.home_screen,d.created_at,d.last_seen_at,d.revoked_at
    FROM app.devices d LEFT JOIN app.registers r ON r.id=d.register_id`

// terminalHomeScreens es la lista cerrada de pantallas de inicio de una terminal (etapa 9C, §1.5):
// las secciones operativas de la barra superior. Coincide con el CHECK devices_home_screen_check de
// la migración 0020; «Estado del sistema» y las pantallas de administración nunca son el inicio.
var terminalHomeScreens = []string{"tables", "takeaway", "kitchen", "cash"}

// validTerminalHome indica si la pantalla pedida pertenece a la lista cerrada.
func validTerminalHome(screen string) bool {
	return slices.Contains(terminalHomeScreens, screen)
}

func devices(ctx context.Context, tx pgx.Tx, request Request, principal Principal, parts []string) (any, error) {
	if request.Method == "GET" && len(parts) == 1 {
		if err := principal.require("devices.read"); err != nil {
			return nil, err
		}
		return list(ctx, tx, devicesQuery+" ORDER BY d.created_at")
	}
	if err := principal.require("devices.manage"); err != nil {
		return nil, err
	}
	if request.Method == "POST" && len(parts) == 2 && parts[1] == "invitations" {
		token := NewToken()
		expires := time.Now().Add(10 * time.Minute)
		var enrollmentID string
		err := tx.QueryRow(ctx, `INSERT INTO app.enrollments(code_hash,created_by,expires_at)
            VALUES ($1,$2,$3) RETURNING id`, TokenHash(token), principal.UserID, expires).Scan(&enrollmentID)
		if err != nil {
			return nil, err
		}
		if err := audit(ctx, tx, principal, "device.invitation_created", "enrollment", &enrollmentID, nil, map[string]any{"expires_at": expires}, true); err != nil {
			return nil, err
		}
		return map[string]any{"code": token, "expires_at": expires}, nil
	}
	if len(parts) < 2 {
		return nil, invalid("Operación de terminal no válida.")
	}
	deviceID := parts[1]
	previous, err := object(ctx, tx, devicesQuery+" WHERE d.id=$1", deviceID)
	if err != nil {
		return nil, err
	}
	if request.Method == "POST" && len(parts) == 3 && parts[2] == "revoke" {
		if _, err := tx.Exec(ctx, "UPDATE app.devices SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1", deviceID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='device_revoked' WHERE device_id=$1 AND revoked_at IS NULL", deviceID); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, audit(ctx, tx, principal, "device.revoked", "device", &deviceID, previous, map[string]bool{"revoked": true}, true)
	}
	if request.Method != "PATCH" || len(parts) != 2 {
		return nil, invalid("Operación de terminal no válida.")
	}
	var input struct {
		Name         string  `json:"name"`
		RegisterID   *string `json:"register_id"`
		KeyboardMode string  `json:"keyboard_mode"`
		// Pantalla de inicio de la terminal (etapa 9C). Opcional: si no se envía, se conserva la
		// guardada, de modo que los clientes anteriores siguen editando la terminal sin cambiarla.
		HomeScreen *string `json:"home_screen"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 || (input.KeyboardMode != "auto" && input.KeyboardMode != "always" && input.KeyboardMode != "disabled") {
		return nil, invalid("Revisa el nombre de terminal y el modo de teclado.")
	}
	if input.HomeScreen != nil && !validTerminalHome(*input.HomeScreen) {
		return nil, invalid("Elige la pantalla inicial de la terminal: Mesas, Para llevar, Cocina o Caja.")
	}
	if input.RegisterID != nil {
		if !identifierPattern.MatchString(*input.RegisterID) {
			return nil, invalid("Identificador de caja inválido.")
		}
		var active bool
		if err := tx.QueryRow(ctx, "SELECT active FROM app.registers WHERE id=$1 FOR UPDATE", *input.RegisterID).Scan(&active); err != nil {
			return nil, err
		}
		if !active {
			return nil, invalid("La caja seleccionada está inactiva.")
		}
	}
	// Mientras la caja actual de la terminal tenga un turno activo (abierto o en arqueo de cierre),
	// sus cobros pertenecen a ese turno: no se reasigna la terminal a otra caja hasta cerrarlo.
	var registerChanged, shiftOpen bool
	err = tx.QueryRow(ctx, `SELECT d.register_id IS DISTINCT FROM $2::uuid,
        EXISTS(SELECT FROM app.cash_shifts s WHERE s.register_id=d.register_id AND s.state IN ('open','closing'))
        FROM app.devices d WHERE d.id=$1`, deviceID, input.RegisterID).Scan(&registerChanged, &shiftOpen)
	if err != nil {
		return nil, err
	}
	if registerChanged && shiftOpen {
		return nil, &Fault{Status: 409, Code: "SHIFT_OPEN", Message: "La caja actual de esta terminal tiene un turno abierto o en arqueo de cierre. No se puede cambiar de caja hasta cerrarlo."}
	}
	// coalesce conserva la pantalla de inicio guardada cuando la petición no la incluye.
	if _, err := tx.Exec(ctx, "UPDATE app.devices SET name=$1,register_id=$2,keyboard_mode=$3,home_screen=coalesce($4,home_screen) WHERE id=$5", input.Name, input.RegisterID, input.KeyboardMode, input.HomeScreen, deviceID); err != nil {
		return nil, err
	}
	next, err := object(ctx, tx, devicesQuery+" WHERE d.id=$1", deviceID)
	if err != nil {
		return nil, err
	}
	return next, audit(ctx, tx, principal, "device.updated", "device", &deviceID, previous, next, true)
}
