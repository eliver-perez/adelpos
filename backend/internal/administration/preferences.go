package administration

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

type Preferences struct {
	Theme      string `json:"theme"`
	HomeScreen string `json:"home_screen"`
}

var screenPermissions = map[string]string{
	"status":      "",
	"users":       "users.read",
	"roles":       "roles.read",
	"devices":     "devices.read",
	"registers":   "registers.read",
	"sessions":    "sessions.read",
	"settings":    "settings.read",
	"audit":       "audit.read",
	"preferences": "",
}

func allowedHome(screen string, permissions []string) string {
	permission, exists := screenPermissions[screen]
	if !exists || (permission != "" && !slices.Contains(permissions, permission)) {
		return "status"
	}
	return screen
}

func readPreferences(ctx context.Context, tx pgx.Tx, userID string, permissions []string) (Preferences, error) {
	result := Preferences{Theme: "light", HomeScreen: "status"}
	err := tx.QueryRow(ctx, `SELECT coalesce(p.theme,'light'),coalesce(p.home_screen,'status')
        FROM app.users u LEFT JOIN app.user_preferences p ON p.user_id=u.id WHERE u.id=$1`, userID).Scan(&result.Theme, &result.HomeScreen)
	result.HomeScreen = allowedHome(result.HomeScreen, permissions)
	return result, err
}

func preferences(ctx context.Context, tx pgx.Tx, request Request, principal Principal, userID string, administrative bool) (any, error) {
	if administrative {
		if err := principal.require("users.manage"); err != nil {
			return nil, err
		}
		if err := editableUser(ctx, tx, userID); err != nil {
			return nil, err
		}
	}
	permissions := principal.Permissions
	if userID != principal.UserID {
		var err error
		permissions, err = effectivePermissions(ctx, tx, userID, false)
		if err != nil {
			return nil, err
		}
	}
	if request.Method == "GET" {
		return readPreferences(ctx, tx, userID, permissions)
	}
	if request.Method != "PUT" {
		return nil, invalid("Método no válido para preferencias.")
	}
	var input Preferences
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.Theme != "light" && input.Theme != "dark" {
		return nil, invalid("Selecciona un tema claro u oscuro.")
	}
	// Una pantalla retirada o un permiso revocado siempre vuelve a una pantalla existente.
	input.HomeScreen = allowedHome(input.HomeScreen, permissions)
	previous, err := readPreferences(ctx, tx, userID, permissions)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO app.user_preferences(user_id,theme,home_screen) VALUES ($1,$2,$3)
        ON CONFLICT(user_id) DO UPDATE SET theme=excluded.theme,home_screen=excluded.home_screen`, userID, input.Theme, input.HomeScreen)
	if err != nil {
		return nil, err
	}
	return input, audit(ctx, tx, principal, "user.preferences_changed", "user", &userID, previous, input, true)
}

func terminalPreferences(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	var input struct {
		KeyboardMode string `json:"keyboard_mode"`
	}
	if err := decode(request.Body, &input); err != nil {
		return nil, err
	}
	if input.KeyboardMode != "auto" && input.KeyboardMode != "always" && input.KeyboardMode != "disabled" {
		return nil, invalid("El teclado debe ser auto, always o disabled.")
	}
	var previous string
	if err := tx.QueryRow(ctx, "SELECT keyboard_mode FROM app.devices WHERE id=$1", principal.DeviceID).Scan(&previous); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE app.devices SET keyboard_mode=$1 WHERE id=$2", input.KeyboardMode, principal.DeviceID); err != nil {
		return nil, err
	}
	return input, audit(ctx, tx, principal, "device.preferences_changed", "device", &principal.DeviceID, map[string]string{"keyboard_mode": previous}, input, true)
}
