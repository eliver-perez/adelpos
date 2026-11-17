package administration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A1-D17: la lista de permisos de mesero está en un solo lugar (WaiterPermissions), solo tiene permisos registrados,
// nada administrativo ni de Para llevar, y el contrato (pinLogin), docs/API_V1.md y API_CONTRACTS.md la repiten
// completa (si la lista cambia, la documentación debe cambiar con ella).
func TestWaiterPermissionsAreClosedAndDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, key := range WaiterPermissions {
		if !knownPermission(key) || seen[key] {
			t.Fatalf("permiso de mesero desconocido o repetido: %s", key)
		}
		seen[key] = true
		for _, prefix := range []string{"users.", "roles.", "permissions.", "devices.", "settings.", "sessions.", "registers.",
			"audit.", "payments.", "cash.", "takeaway.", "inventory.", "reports.", "documents.", "kitchen."} {
			if strings.HasPrefix(key, prefix) {
				t.Fatalf("la lista de mesero no admite %s", key)
			}
		}
	}
	for _, required := range []string{"waiter_devices.use", "orders.confirm", "accounts.read", "events.read"} {
		if !seen[required] {
			t.Fatalf("falta %s en la lista de mesero", required)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "apicontract", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			Description string `json:"description"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{"openapi.json (pinLogin)": document.Paths["/auth/pin-login"]["post"].Description}
	for name, path := range map[string]string{
		"docs/API_V1.md":   filepath.Join("..", "..", "..", "docs", "API_V1.md"),
		"API_CONTRACTS.md": filepath.Join("..", "..", "..", "API_CONTRACTS.md"),
	} {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = string(text)
	}
	for name, text := range sources {
		for _, key := range WaiterPermissions {
			if !strings.Contains(text, key) {
				t.Fatalf("%s no menciona el permiso de mesero %s", name, key)
			}
		}
	}
}

func TestWaiterScopeKeepsOnlyWaiterPermissions(t *testing.T) {
	all := []string{"accounts.read", "users.manage", "orders.confirm", "takeaway.read", "settings.manage", "events.read"}
	if scoped := waiterScope(all); !slices.Equal(scoped, []string{"accounts.read", "orders.confirm", "events.read"}) {
		t.Fatalf("permisos acotados: %v", scoped)
	}
	if scoped := waiterScope(nil); len(scoped) != 0 {
		t.Fatalf("sin permisos: %v", scoped)
	}
}

// Revisión R3, O4: pocas verificaciones de PIN a la vez; la que no consigue turno responde 503 PIN_LOGIN_BUSY sin
// esperar más que el plazo de su petición, y un turno liberado sirve a la siguiente.
func TestPINLoginSlotsAreLimited(t *testing.T) {
	releases := []func(){}
	for range cap(pinLoginSlots) {
		release, err := reservePINCheck(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	waiting, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := reservePINCheck(waiting)
	var fault *Fault
	if !errors.As(err, &fault) || fault.Status != 503 || fault.Code != "PIN_LOGIN_BUSY" || time.Since(started) > time.Second {
		t.Fatalf("sin turno libre: %v", err)
	}
	releases[0]()
	release, err := reservePINCheck(context.Background())
	if err != nil {
		t.Fatalf("con un turno liberado: %v", err)
	}
	release()
	for _, release := range releases[1:] {
		release()
	}
	if len(pinLoginSlots) != 0 {
		t.Fatalf("quedaron turnos ocupados: %d", len(pinLoginSlots))
	}
	if !pinLoginRequest(Request{Method: "POST", Path: "/api/v1/auth/pin-login"}) ||
		pinLoginRequest(Request{Method: "GET", Path: "/api/v1/auth/pin-users"}) ||
		pinLoginRequest(Request{Method: "POST", Path: "/api/v1/auth/login"}) {
		t.Fatal("solo la entrada con PIN toma turno")
	}
}
