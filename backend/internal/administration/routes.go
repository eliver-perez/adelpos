package administration

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (service *Service) protected(ctx context.Context, tx pgx.Tx, request Request, principal Principal, route string) (any, error) {
	if request.Method == "GET" && route == "auth/me" {
		return currentIdentity(ctx, tx, principal)
	}
	if request.Method == "POST" && route == "auth/logout" {
		if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=now(),revoke_reason='logout' WHERE id=$1", principal.SessionID); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, audit(ctx, tx, principal, "session.logout", "session", &principal.SessionID, nil, nil, true)
	}
	if request.Method == "POST" && route == "auth/password" {
		return changePassword(ctx, tx, principal, request.Body)
	}
	if route == "preferences" {
		return preferences(ctx, tx, request, principal, principal.UserID, false)
	}
	if route == "terminal/preferences" && request.Method == "PUT" {
		return terminalPreferences(ctx, tx, request, principal)
	}
	parts := strings.Split(route, "/")
	if len(parts) > 1 && parts[1] != "invitations" && !identifierPattern.MatchString(parts[1]) {
		return nil, invalid("El identificador no es válido.")
	}
	switch parts[0] {
	case "users":
		return users(ctx, tx, request, principal, parts)
	case "roles":
		return roles(ctx, tx, request, principal, parts)
	case "permissions":
		if err := principal.require("permissions.read"); err != nil {
			return nil, err
		}
		if request.Method == "GET" && len(parts) == 1 {
			return Permissions, nil
		}
	case "settings":
		return settings(ctx, tx, request, principal)
	case "registers":
		return registers(ctx, tx, request, principal, parts)
	case "devices":
		return devices(ctx, tx, request, principal, parts)
	case "sessions":
		return sessions(ctx, tx, request, principal, parts)
	case "audit":
		return auditLog(ctx, tx, request, principal)
	}
	return nil, &Fault{Status: 404, Code: "NOT_FOUND", Message: "Ruta no disponible."}
}

func currentIdentity(ctx context.Context, tx pgx.Tx, principal Principal) (any, error) {
	preference, err := readPreferences(ctx, tx, principal.UserID, principal.Permissions)
	if err != nil {
		return nil, err
	}
	terminal, err := object(ctx, tx, "SELECT id,name,register_id,keyboard_mode FROM app.devices WHERE id=$1", principal.DeviceID)
	if err != nil {
		return nil, err
	}
	establishment, err := object(ctx, tx, "SELECT name,timezone,currency FROM app.establishment")
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": principal, "preferences": preference, "terminal": terminal, "establishment": establishment}, nil
}
