//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/payments"
)

// Etapa 9H, fase A (§2.4) contra PostgreSQL con la API real: el mismo usuario con sesión a la vez en dos terminales
// (decisión del propietario). Iniciar sesión en la segunda no cierra la primera; iniciar sesión de nuevo en la misma
// terminal sí cierra la anterior de esa terminal; «Sesiones activas por tipo de terminal» cuenta sesiones, no
// usuarios. Lo que depende de la terminal sigue siendo de cada terminal: el «usuario actual» de Sistema › Terminales,
// el turno de caja de su caja física, el arqueo (pausa solo el turno de esa caja), el estado para reconectarse y el
// canal de eventos; los permisos se leen en cada petición de cada sesión. Cerrar sesión cierra solo la de esa terminal
// y cambiar la contraseña, las de todas (como antes).

type stage9HSessionsFixture struct {
	*paymentsFixture
	username string
	password string
	userID   string
}

// terminalFor enrola una terminal nueva asociada a la caja física indicada e inicia sesión con el usuario.
func (fixture *stage9HSessionsFixture) terminalFor(t *testing.T, name, registerID string) testTerminal {
	t.Helper()
	invitation := decodeResponse[struct {
		Code string `json:"code"`
	}](t, fixture.adminCall(t, "POST", "devices/invitations", nil, 200))
	device := decodeResponse[struct {
		ID    string `json:"device_id"`
		Token string `json:"device_token"`
	}](t, fixture.call(t, testTerminal{}, "POST", "devices/enroll", map[string]any{"code": invitation.Code, "name": name}, 200))
	fixture.adminCall(t, "PATCH", "devices/"+device.ID, map[string]any{"name": name, "register_id": registerID, "keyboard_mode": "auto"}, 200)
	return fixture.login(t, testTerminal{deviceID: device.ID, deviceToken: device.Token})
}

// login inicia sesión con el usuario en la terminal y devuelve la terminal con la sesión nueva.
func (fixture *stage9HSessionsFixture) login(t *testing.T, terminal testTerminal) testTerminal {
	t.Helper()
	terminal.sessionToken = decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, terminal, "POST", "auth/login", map[string]string{"username": fixture.username, "password": fixture.password}, 200)).Token
	return terminal
}

// stage9HError devuelve el código y el mensaje de un rechazo.
func stage9HError(t *testing.T, body []byte) (string, string) {
	t.Helper()
	problem := decodeResponse[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](t, body).Error
	return problem.Code, problem.Message
}

type stage9HCurrentShift struct {
	Shift  *payments.Shift `json:"shift"`
	Usable bool            `json:"usable"`
	Reason string          `json:"reason"`
}

func TestStage9HSameUserOnTwoTerminals(t *testing.T) {
	resetStage9DDatabase(t)
	base := &paymentsFixture{restaurantFixture: newFixture(t), methods: map[string]payments.Method{}, registers: map[string]string{}}
	base.seedCatalog(t)
	register := func(name string) string {
		return decodeResponse[struct {
			ID string `json:"id"`
		}](t, base.adminCall(t, "POST", "registers", map[string]any{"name": name, "description": "", "active": true}, 200)).ID
	}
	registerA, registerB := register("Caja norte 9H"), register("Caja sur 9H")
	fixture := &stage9HSessionsFixture{paymentsFixture: base, username: "cajera_dos_9h", password: "Clave temporal de cajera_dos_9h"}
	permissions := append(append([]string{}, cashierPermissions...), "accounts.open", "restaurant.tables.read", "events.read")
	first := newCashier(t, base, fixture.username, permissions, registerA)
	if err := base.pool.QueryRow(base.ctx, "SELECT id::text FROM app.users WHERE username=$1", fixture.username).Scan(&fixture.userID); err != nil {
		t.Fatal(err)
	}
	activeSessions := func(where string, args ...any) int {
		return rowCount(t, base.restaurantFixture, `SELECT count(*) FROM app.sessions s JOIN app.devices d ON d.id=s.device_id
            WHERE s.revoked_at IS NULL AND s.expires_at > now() AND d.revoked_at IS NULL`+where, args...)
	}
	before := activeSessions("")

	// 1. El mismo usuario inicia sesión en una segunda terminal: la primera sigue vigente.
	second := fixture.terminalFor(t, "Terminal sur 9H", registerB)
	for _, terminal := range []testTerminal{first, second} {
		me := decodeResponse[struct {
			User struct {
				Username string `json:"username"`
				DeviceID string `json:"device_id"`
			} `json:"user"`
		}](t, fixture.call(t, terminal, "GET", "auth/me", nil, 200)).User
		if me.Username != fixture.username || me.DeviceID != terminal.deviceID {
			t.Fatalf("Sesión de la terminal %s: %+v", terminal.deviceID, me)
		}
	}
	if count := activeSessions(" AND s.user_id=$1", fixture.userID); count != 2 {
		t.Fatalf("El usuario debía tener una sesión vigente en cada terminal: %d", count)
	}

	// 2. «Sesiones activas por tipo de terminal» cuenta sesiones, no usuarios.
	summary := decodeResponse[struct {
		ByKind map[string]int `json:"by_kind"`
		Total  int            `json:"total"`
	}](t, fixture.adminCall(t, "GET", "devices/sessions-summary", nil, 200))
	if summary.Total != before+1 || summary.Total != activeSessions("") || summary.ByKind["desktop"] != summary.Total {
		t.Fatalf("Conteo de sesiones: %+v (antes %d)", summary, before)
	}

	// 3. Sistema › Terminales: el usuario es el usuario actual de las dos terminales.
	devices := decodeResponse[[]struct {
		ID          string  `json:"id"`
		CurrentUser *string `json:"current_user"`
	}](t, fixture.adminCall(t, "GET", "devices", nil, 200))
	seen := 0
	for _, device := range devices {
		if device.ID == first.deviceID || device.ID == second.deviceID {
			if device.CurrentUser == nil || *device.CurrentUser != "Prueba "+fixture.username {
				t.Fatalf("Usuario actual de la terminal %s: %v", device.ID, device.CurrentUser)
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("La lista de terminales no tiene las dos terminales del usuario: %d", seen)
	}

	// 4. Turnos de caja por terminal: cada terminal abre y ve el turno de su propia caja.
	shifts := map[string]string{}
	for name, terminal := range map[string]testTerminal{"norte": first, "sur": second} {
		shift := decodeResponse[payments.Shift](t, fixture.call(t, terminal, "POST", "payments/shifts", map[string]any{
			"operation_id": randomID(), "opening_float": "500",
		}, 200))
		shifts[name] = shift.ID
	}
	current := func(terminal testTerminal) stage9HCurrentShift {
		t.Helper()
		return decodeResponse[stage9HCurrentShift](t, fixture.call(t, terminal, "GET", "payments/shifts/current", nil, 200))
	}
	for name, terminal := range map[string]testTerminal{"norte": first, "sur": second} {
		view := current(terminal)
		if view.Shift == nil || view.Shift.ID != shifts[name] || !view.Usable {
			t.Fatalf("La terminal %s debía ver el turno de su caja: %+v", name, view)
		}
	}
	if shifts["norte"] == shifts["sur"] {
		t.Fatal("Las dos cajas debían tener turnos distintos")
	}

	// 5. El arqueo de la caja norte pausa solo su turno: la terminal sur sigue cobrando con el suyo.
	fixture.call(t, first, "POST", "payments/shifts/"+shifts["norte"]+"/closing", map[string]any{"operation_id": randomID()}, 200)
	if view := current(first); view.Shift == nil || view.Shift.State != "closing" || view.Usable {
		t.Fatalf("El turno de la caja norte debía quedar en arqueo: %+v", view)
	}
	if view := current(second); view.Shift == nil || view.Shift.ID != shifts["sur"] || view.Shift.State != "open" || !view.Usable {
		t.Fatalf("El arqueo de la caja norte no debía pausar la caja sur: %+v", view)
	}

	// 6. El estado para reconectarse y el canal de eventos son de cada terminal.
	for _, terminal := range []testTerminal{first, second} {
		state := decodeResponse[struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
			Terminal struct {
				ID         string  `json:"id"`
				RegisterID *string `json:"register_id"`
			} `json:"terminal"`
		}](t, fixture.call(t, terminal, "GET", "restaurant/state", nil, 200))
		if state.User.Username != fixture.username || state.Terminal.ID != terminal.deviceID || state.Terminal.RegisterID == nil {
			t.Fatalf("Estado de reconexión de la terminal %s: %+v", terminal.deviceID, state)
		}
		fixture.call(t, terminal, "GET", "events/snapshot?after=0", nil, 200)
	}

	// 7. Los permisos se leen en cada petición de cada sesión: quitar uno alcanza a las dos terminales a la vez.
	fixture.adminCall(t, "PUT", "users/"+fixture.userID+"/permissions", map[string]any{"overrides": map[string]string{"accounts.read": "deny"}}, 200)
	for _, terminal := range []testTerminal{first, second} {
		fixture.call(t, terminal, "GET", "restaurant/accounts", nil, 403)
	}
	fixture.adminCall(t, "PUT", "users/"+fixture.userID+"/permissions", map[string]any{"overrides": map[string]string{"accounts.read": "inherit"}}, 200)
	for _, terminal := range []testTerminal{first, second} {
		fixture.call(t, terminal, "GET", "restaurant/accounts", nil, 200)
	}

	// 8. Iniciar sesión de nuevo en la terminal norte cierra solo la sesión anterior de esa terminal.
	again := fixture.login(t, first)
	code, message := stage9HError(t, fixture.call(t, first, "GET", "auth/me", nil, 401))
	if code != "SESSION_REPLACED" || !strings.Contains(message, "de nuevo en esta terminal") {
		t.Fatalf("Sesión reemplazada en la misma terminal: %s %s", code, message)
	}
	fixture.call(t, again, "GET", "auth/me", nil, 200)
	fixture.call(t, second, "GET", "auth/me", nil, 200)
	if count := activeSessions(" AND s.user_id=$1", fixture.userID); count != 2 {
		t.Fatalf("Después de iniciar de nuevo en la misma terminal debía haber una sesión por terminal: %d", count)
	}
	if view := current(again); view.Shift == nil || view.Shift.ID != shifts["norte"] || view.Shift.State != "closing" {
		t.Fatalf("La sesión nueva de la terminal norte debía ver su turno en arqueo: %+v", view)
	}

	// 9. Cerrar sesión en la terminal sur cierra solo esa sesión.
	fixture.call(t, second, "POST", "auth/logout", map[string]any{}, 200)
	fixture.call(t, second, "GET", "auth/me", nil, 401)
	fixture.call(t, again, "GET", "auth/me", nil, 200)

	// 10. Cambiar la contraseña cierra las sesiones del usuario en todas las terminales, como antes.
	second = fixture.login(t, second)
	changed := decodeResponse[struct {
		Token string `json:"session_token"`
	}](t, fixture.call(t, again, "POST", "auth/password", map[string]string{
		"current_password": fixture.password, "new_password": "Clave nueva de cajera 9H",
	}, 200))
	code, _ = stage9HError(t, fixture.call(t, second, "GET", "auth/me", nil, 401))
	if code != "SESSION_REVOKED" {
		t.Fatalf("Cambiar la contraseña debía cerrar la sesión de la otra terminal: %s", code)
	}
	fixture.call(t, again, "GET", "auth/me", nil, 401)
	again.sessionToken = changed.Token
	fixture.call(t, again, "GET", "auth/me", nil, 200)
}
