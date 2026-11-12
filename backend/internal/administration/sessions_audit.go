package administration

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func sessions(ctx context.Context, tx pgx.Tx, request Request, principal Principal, parts []string) (any, error) {
	if request.Method == "GET" && len(parts) == 1 {
		if err := principal.require("sessions.read"); err != nil {
			return nil, err
		}
		return list(ctx, tx, `SELECT s.id,u.username,d.name AS device_name,s.device_id,s.created_at,s.expires_at,s.last_seen_at,s.revoked_at,s.revoke_reason
            FROM app.sessions s JOIN app.users u ON u.id=s.user_id JOIN app.devices d ON d.id=s.device_id
            WHERE NOT u.support OR $1 ORDER BY s.created_at DESC LIMIT 200`, principal.Support)
	}
	if err := principal.require("sessions.manage"); err != nil {
		return nil, err
	}
	if request.Method != "POST" || len(parts) != 3 || parts[2] != "revoke" {
		return nil, invalid("Operación de sesión no válida.")
	}
	sessionID := parts[1]
	var support bool
	if err := tx.QueryRow(ctx, "SELECT u.support FROM app.sessions s JOIN app.users u ON u.id=s.user_id WHERE s.id=$1", sessionID).Scan(&support); err != nil {
		return nil, err
	}
	if support && !principal.Support {
		return nil, denied()
	}
	if _, err := tx.Exec(ctx, "UPDATE app.sessions SET revoked_at=coalesce(revoked_at,now()),revoke_reason='admin_revoked' WHERE id=$1", sessionID); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, audit(ctx, tx, principal, "session.revoked", "session", &sessionID, nil, map[string]bool{"revoked": true}, true)
}

func auditLog(ctx context.Context, tx pgx.Tx, request Request, principal Principal) (any, error) {
	if err := principal.require("audit.read"); err != nil {
		return nil, err
	}
	if request.Method != "GET" {
		return nil, invalid("El registro de movimientos es de solo consulta.")
	}
	page, size := 1, 25
	if request.Query.Get("page") != "" {
		value, err := strconv.Atoi(request.Query.Get("page"))
		if err != nil || value < 1 || value > 10000 {
			return nil, invalid("Número de página inválido.")
		}
		page = value
	}
	if request.Query.Get("page_size") != "" {
		value, err := strconv.Atoi(request.Query.Get("page_size"))
		if err != nil || value < 1 || value > 100 {
			return nil, invalid("El tamaño de página debe ser de 1 a 100.")
		}
		size = value
	}
	action, actor, device := request.Query.Get("action"), request.Query.Get("actor"), request.Query.Get("device_id")
	if device != "" && !identifierPattern.MatchString(device) {
		return nil, invalid("Identificador de terminal inválido.")
	}
	var from, until *time.Time
	for key, target := range map[string]**time.Time{"from": &from, "until": &until} {
		if request.Query.Get(key) != "" {
			value, err := time.Parse(time.RFC3339, request.Query.Get(key))
			if err != nil {
				return nil, invalid("Las fechas deben usar RFC3339.")
			}
			*target = &value
		}
	}
	filter := ` FROM app.audit_events a WHERE ($1='' OR a.action=$1) AND ($2='' OR a.actor_username=$2)
        AND ($3='' OR a.device_id::text=$3) AND ($4::timestamptz IS NULL OR a.occurred_at >= $4)
        AND ($5::timestamptz IS NULL OR a.occurred_at <= $5)`
	var total int
	if err := tx.QueryRow(ctx, "SELECT count(*)"+filter, action, actor, device, from, until).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT a.id,a.occurred_at,a.actor_id,a.actor_username,a.actor_kind,a.device_id,
        a.action,a.entity_type,a.entity_id,a.success,a.before_state,a.after_state` + filter +
		" ORDER BY a.occurred_at DESC,a.id LIMIT $6 OFFSET $7"
	items, err := list(ctx, tx, query, action, actor, device, from, until, size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "page": page, "page_size": size}, nil
}
