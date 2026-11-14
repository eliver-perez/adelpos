package administration

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/kitchen"
)

// devicesQuery es la terminal tal como se audita (antes y después de cada cambio). Desde la etapa 9G
// (fase B) informa también el tipo, el sistema, la versión de la aplicación y cuándo se emparejó.
const devicesQuery = `SELECT d.id,d.name,d.register_id,r.name AS register_name,d.keyboard_mode,d.home_screen,d.created_at,d.last_seen_at,d.revoked_at,
    d.kind,d.os,d.app_version,d.paired_at
    FROM app.devices d LEFT JOIN app.registers r ON r.id=d.register_id`

// onlineWindow: una terminal está «en línea» si hizo alguna petición en los últimos 2 minutos (la ventana
// abierta consulta al servidor cada pocos segundos), como el conteo de terminales conectadas de la fase A.
const onlineWindow = "2 minutes"

// deviceListQuery es la lista de Sistema › Terminales (etapa 9G, §3.3): además, si está en línea y el
// usuario que tiene la sesión activa en ella (nunca la identidad reservada de soporte).
const deviceListQuery = `SELECT d.id,d.name,d.register_id,r.name AS register_name,d.keyboard_mode,d.home_screen,d.created_at,d.last_seen_at,d.revoked_at,
    d.kind,d.os,d.app_version,d.paired_at,
    (d.revoked_at IS NULL AND d.last_seen_at > now() - interval '` + onlineWindow + `') AS online,
    (SELECT u.display_name FROM app.sessions s JOIN app.users u ON u.id=s.user_id
        WHERE s.device_id=d.id AND s.revoked_at IS NULL AND s.expires_at > now() AND NOT u.support
        ORDER BY s.last_seen_at DESC LIMIT 1) AS "current_user"
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
		return list(ctx, tx, deviceListQuery+" ORDER BY d.created_at")
	}
	// Etapa 9G (§3.3): configuración de una terminal (pantalla de inicio, caja, estaciones; las impresoras
	// de la terminal llegan con la fase E).
	if request.Method == "GET" && len(parts) == 3 && parts[2] == "configuration" {
		if err := principal.require("devices.read"); err != nil {
			return nil, err
		}
		return deviceConfiguration(ctx, tx, parts[1])
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

// deviceConfiguration reúne lo que se configura de una terminal: pantalla de inicio, caja, teclado, las
// estaciones que muestra en Cocina (todas o una selección, etapa 9F) y su aviso sonoro. Desde la fase E de la
// 9G también sus impresoras: las de AD-EL POS conectadas a ella (printers) y las que informó su computadora para
// su agente de impresión, con si sirven para tickets (local_printers).
func deviceConfiguration(ctx context.Context, tx pgx.Tx, deviceID string) (any, error) {
	device, err := object(ctx, tx, deviceListQuery+" WHERE d.id=$1", deviceID)
	if err != nil {
		return nil, err
	}
	var allStations, soundAlert bool
	if err := tx.QueryRow(ctx, "SELECT kitchen_all_stations,kitchen_sound_alert FROM app.devices WHERE id=$1", deviceID).Scan(&allStations, &soundAlert); err != nil {
		return nil, err
	}
	stations, err := list(ctx, tx, `SELECT s.id,s.name,s.active FROM app.kitchen_device_stations ds
        JOIN app.kitchen_stations s ON s.id=ds.station_id WHERE ds.device_id=$1 ORDER BY s.sort_order,s.name`, deviceID)
	if err != nil {
		return nil, err
	}
	sets, err := kitchen.TerminalPrinters(ctx, tx, deviceID)
	if err != nil {
		return nil, err
	}
	printers := []kitchen.AssignedOutput{}
	var localPrinters any
	if len(sets) == 1 {
		printers = sets[0].Printers
		localPrinters = map[string]any{
			"agent_online":  sets[0].AgentOnline,
			"agent_seen_at": sets[0].AgentSeenAt,
			"reported_at":   sets[0].ReportedAt,
			"queues":        sets[0].Queues,
		}
	}
	return map[string]any{
		"device":               device,
		"kitchen_all_stations": allStations,
		"kitchen_sound_alert":  soundAlert,
		"kitchen_stations":     stations,
		"printers":             printers,
		"local_printers":       localPrinters,
	}, nil
}

// sessionsSummary cuenta las sesiones activas por tipo de dispositivo (etapa 9G, §3.4 y D9): escritorio,
// comandero Android y cocina. Solo informa: no limita nada todavía (las licencias por sesiones
// simultáneas son de la etapa 10). Cuenta sesiones vigentes de terminales no revocadas.
func sessionsSummary(ctx context.Context, tx pgx.Tx, principal Principal) (any, error) {
	if err := principal.require("devices.read"); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, kind := range DeviceKinds {
		counts[kind] = 0
	}
	rows, err := tx.Query(ctx, `SELECT d.kind, count(*) FROM app.sessions s JOIN app.devices d ON d.id=s.device_id
        WHERE s.revoked_at IS NULL AND s.expires_at > now() AND d.revoked_at IS NULL
        GROUP BY d.kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		counts[kind] = count
		total += count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"by_kind": counts, "total": total, "limited": false}, nil
}
