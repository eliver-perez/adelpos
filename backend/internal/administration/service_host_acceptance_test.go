//go:build integration

package administration_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/product"
)

// fakeHost sustituye al servicio de macOS (host_unix.go): informa un estado fijo y cuenta los reinicios.
type fakeHost struct {
	mutex    sync.Mutex
	status   administration.HostStatus
	restarts int
	refuse   bool
}

func (host *fakeHost) HostStatus() administration.HostStatus {
	return host.status
}

func (host *fakeHost) Restart() bool {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	if host.refuse {
		return false
	}
	host.restarts++
	return true
}

func (host *fakeHost) count() int {
	host.mutex.Lock()
	defer host.mutex.Unlock()
	return host.restarts
}

type serviceStatusBody struct {
	Managed          bool      `json:"managed"`
	RestartAvailable bool      `json:"restart_available"`
	InstallationMode string    `json:"installation_mode"`
	ServiceEnabled   bool      `json:"service_enabled"`
	StartedAt        time.Time `json:"started_at"`
	UptimeSeconds    int64     `json:"uptime_seconds"`
	ServerVersion    string    `json:"server_version"`
	Terminals        struct {
		Connected  int `json:"connected"`
		Registered int `json:"registered"`
	} `json:"terminals"`
	LastBackup json.RawMessage `json:"last_backup"`
}

// testServiceStatus cubre la etapa 9G, fase A (§2.2): estado del servicio y reinicio desde Mantenimiento,
// solo con settings.manage; sin servicio administrado (serve) no hay reinicio; con él, el reinicio se
// audita y se programa después de confirmar; las terminales conectadas se cuentan por su última petición
// reciente sin contar la que pregunta ni las revocadas.
func testServiceStatus(t *testing.T, env *delegationEnvironment) {
	status := func(actor terminal) serviceStatusBody {
		t.Helper()
		var value serviceStatusBody
		if err := json.Unmarshal(env.call(actor, "GET", "maintenance/service", nil, 200), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	restartAudits := func() int {
		return env.scalar(t, "SELECT count(*) FROM app.audit_events WHERE action='maintenance.service_restarted'")
	}

	t.Run("stage9g_servicio_sin_administrar", func(t *testing.T) {
		value := status(env.admin)
		if value.Managed || value.RestartAvailable || value.ServerVersion != product.Read().Version {
			t.Fatalf("serve no es un servicio administrado: %+v", value)
		}
		if string(value.LastBackup) != "null" || value.StartedAt.IsZero() {
			t.Fatalf("el último respaldo es de la fase F y el arranque debe informarse: %+v", value)
		}
		before := restartAudits()
		env.call(env.admin, "POST", "maintenance/service/restart", map[string]any{}, 409)
		if restartAudits() != before {
			t.Fatal("un reinicio imposible no se audita")
		}
	})

	t.Run("stage9g_terminales_conectadas", func(t *testing.T) {
		expectedRegistered := env.scalar(t, "SELECT count(*) FROM app.devices WHERE revoked_at IS NULL AND id <> $1", env.admin.id)
		expectedConnected := env.scalar(t, `SELECT count(*) FROM app.devices
            WHERE revoked_at IS NULL AND id <> $1 AND last_seen_at > now() - interval '2 minutes'`, env.admin.id)
		value := status(env.admin)
		if value.Terminals.Registered != expectedRegistered || value.Terminals.Connected != expectedConnected {
			t.Fatalf("terminales: %+v, se esperaban %d de %d", value.Terminals, expectedConnected, expectedRegistered)
		}
		// Una terminal sin peticiones recientes deja de contar como conectada; una revocada no cuenta.
		var otherID string
		err := env.pool.QueryRow(env.ctx, `INSERT INTO app.devices(name, secret_hash, last_seen_at)
            VALUES ('Caja 9G antigua', md5(random()::text), now() - interval '10 minutes') RETURNING id`).Scan(&otherID)
		if err != nil {
			t.Fatal(err)
		}
		value = status(env.admin)
		if value.Terminals.Registered != expectedRegistered+1 || value.Terminals.Connected != expectedConnected {
			t.Fatalf("la terminal sin peticiones recientes no está conectada: %+v", value.Terminals)
		}
		if _, err := env.pool.Exec(env.ctx, "UPDATE app.devices SET revoked_at = now() WHERE id = $1", otherID); err != nil {
			t.Fatal(err)
		}
		value = status(env.admin)
		if value.Terminals.Registered != expectedRegistered {
			t.Fatalf("una terminal revocada no cuenta: %+v", value.Terminals)
		}
	})

	t.Run("stage9g_servicio_requiere_settings_manage", func(t *testing.T) {
		role := env.createRole(t, "Lectura de configuración 9G", "settings.read")
		_, reader := env.createUser(t, "lector.servicio9g", role)
		host := &fakeHost{status: administration.HostStatus{Managed: true, StartedAt: time.Now()}}
		env.service.Host = host
		defer func() {
			env.service.Host = nil
		}()
		env.call(reader, "GET", "maintenance/service", nil, 403)
		env.call(reader, "POST", "maintenance/service/restart", map[string]any{}, 403)
		if host.count() != 0 {
			t.Fatal("sin permiso no se reinicia")
		}
	})

	t.Run("stage9g_reinicio_del_servicio_administrado", func(t *testing.T) {
		started := time.Now().Add(-90 * time.Second)
		host := &fakeHost{status: administration.HostStatus{
			Managed:          true,
			InstallationMode: "server-only",
			ServiceEnabled:   true,
			StartedAt:        started,
		}}
		env.service.Host = host
		defer func() {
			env.service.Host = nil
		}()
		value := status(env.admin)
		if !value.Managed || !value.RestartAvailable || value.InstallationMode != "server-only" || !value.ServiceEnabled {
			t.Fatalf("estado del servicio administrado: %+v", value)
		}
		if value.UptimeSeconds < 89 || value.StartedAt.Sub(started).Abs() > time.Second {
			t.Fatalf("tiempo activo inesperado: %+v", value)
		}
		before := restartAudits()
		env.call(env.admin, "POST", "maintenance/service/restart", map[string]any{"extra": true}, 400)
		if host.count() != 0 || restartAudits() != before {
			t.Fatal("una petición inválida no reinicia ni audita")
		}
		body := env.call(env.admin, "POST", "maintenance/service/restart", map[string]any{}, 200)
		var result map[string]bool
		if err := json.Unmarshal(body, &result); err != nil || !result["restarting"] {
			t.Fatalf("respuesta del reinicio: %s", body)
		}
		if host.count() != 1 || restartAudits() != before+1 {
			t.Fatalf("el reinicio se programa una vez y queda en el registro: %d reinicios, %d auditorías", host.count(), restartAudits()-before)
		}
		// Si el servicio ya no puede relanzarse (se quitó el archivo de habilitación), la respuesta lo dice
		// en lugar de prometer un reinicio, sin auditarlo.
		host.status.ServiceEnabled = false
		if status(env.admin).RestartAvailable {
			t.Fatal("sin archivo de habilitación no se ofrece el reinicio")
		}
		env.call(env.admin, "POST", "maintenance/service/restart", nil, 409)
		if host.count() != 1 || restartAudits() != before+1 {
			t.Fatal("un reinicio imposible no se programa ni se audita")
		}
		// Carrera: el archivo desaparece entre la autorización y el reinicio; la respuesta también lo dice.
		host.status.ServiceEnabled = true
		host.refuse = true
		env.call(env.admin, "POST", "maintenance/service/restart", nil, 409)
	})
}
