package administration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
)

// sampleID sustituye los identificadores de las rutas.
const sampleID = "11111111-2222-4333-8444-555555555555"

type gateRoute struct {
	method, route, source string
}

// bridgeRoutes son las operaciones de la lista cerrada del puente del escritorio (route() de
// desktop/src-tauri/src/api.rs), con un identificador de ejemplo.
func bridgeRoutes(t *testing.T) []gateRoute {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "desktop", "src-tauri", "src", "api.rs"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start, end := strings.Index(source, "fn route("), strings.Index(source, "pub struct LocalState")
	if start < 0 || end < start {
		t.Fatal("no se encontró route() en api.rs")
	}
	body := source[start:end]
	routes := []gateRoute{}
	fixed := regexp.MustCompile(`Some\(\(Method::(\w+),\s*"([^"]+)"\)\)`)
	formatted := regexp.MustCompile(`\(\s*Method::(\w+),\s*format!\("([^"]+)"\),?\s*\)`)
	for _, pattern := range []*regexp.Regexp{fixed, formatted} {
		for _, match := range pattern.FindAllStringSubmatch(body, -1) {
			route := strings.ReplaceAll(match[2], "{identifier}", sampleID)
			routes = append(routes, gateRoute{method: match[1], route: route, source: "api.rs"})
		}
	}
	return routes
}

// contractRoutes son las operaciones de backend/internal/apicontract/openapi.json.
func contractRoutes(t *testing.T) []gateRoute {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "apicontract", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	parameter := regexp.MustCompile(`\{[^}]+\}`)
	routes := []gateRoute{}
	for path, operations := range document.Paths {
		for method := range operations {
			upper := strings.ToUpper(method)
			if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE"}, upper) {
				continue
			}
			route := parameter.ReplaceAllString(strings.TrimPrefix(path, "/"), sampleID)
			routes = append(routes, gateRoute{method: upper, route: route, source: "openapi.json"})
		}
	}
	return routes
}

// localRoutes son las que no están en route() ni en el contrato: las de los otros módulos nativos del escritorio
// (respaldos, migración del sistema anterior, exportaciones, emparejamiento, agente de impresión y canal de eventos),
// las que dispatch atiende por dentro (el canal por cursor) y las de la licencia y los cupos.
var localRoutes = []gateRoute{
	{"PUT", "maintenance/backups/directory", "backup_folder.rs"},
	{"POST", "maintenance/legacy/uploads", "legacy_migration.rs"},
	{"GET", "maintenance/legacy/uploads/" + sampleID, "legacy_migration.rs"},
	{"POST", "maintenance/legacy/uploads/" + sampleID + "/parts", "legacy_migration.rs"},
	{"POST", "maintenance/legacy/preparations", "legacy_migration.rs"},
	{"GET", "maintenance/legacy/batches/" + sampleID + "/report.pdf", "legacy_migration.rs"},
	{"GET", "reports/export/sales.by_day", "export.rs"},
	{"GET", "pairing/requests/" + sampleID, "pairing.rs"},
	{"POST", "pairing/requests/" + sampleID + "/claim", "pairing.rs"},
	{"POST", "print-agent/printers", "print_agent.rs"},
	{"POST", "print-agent/jobs/claim", "print_agent.rs"},
	{"POST", "print-agent/jobs/" + sampleID + "/result", "print_agent.rs"},
	{"GET", "events", "realtime.rs"},
	{"GET", "events/snapshot", "dispatch (canal de eventos)"},
	{"GET", "health/live", "discovery.rs"},
	{"GET", "ready-alerts/stream", "Android"},
	// Fase D (cupos).
	{"POST", "kitchen/display-mode", "Fase D"},
	{"POST", "devices/" + sampleID + "/seat/release", "Fase D"},
	// Fase B y Fase C (la licencia y la hora).
	{"GET", "license/status", "Fase B"},
	{"POST", "license/activate", "Fase B"},
	{"POST", "license/transfer", "Fase B"},
	{"POST", "license/refresh", "Fase B"},
	{"POST", "license/deactivate", "Fase B"},
	{"POST", "license/offline-request", "Fase B"},
	{"POST", "license/import", "Fase B"},
	{"POST", "license/identity/regenerate", "Fase B"},
	{"POST", "license/time-check", "Fase C"},
	{"POST", "license/support-request", "Fase C"},
	{"POST", "license/support-unlock", "Fase C"},
}

// Prueba de completitud de la puerta (L1-D12): toda operación del puente del escritorio, del contrato y de las rutas
// locales está en la tabla de la §6, y la clasificación de los cupos de la Fase D es coherente con ella: solo ocupan cupo
// la captura, abrir turno y el modo Cocina.
func TestEveryOperationHasItsLicenseClass(t *testing.T) {
	bridge, contract := bridgeRoutes(t), contractRoutes(t)
	if len(bridge) < 300 || len(contract) < 35 {
		t.Fatalf("se leyeron %d operaciones del puente y %d del contrato: la lectura no encontró las fuentes", len(bridge), len(contract))
	}
	all := append(append(bridge, contract...), localRoutes...)
	occupying := map[licensing.Row]bool{licensing.RowCapture: true, licensing.RowOpenShift: true, licensing.RowKitchen: true}
	seen := map[string]bool{}
	for _, item := range all {
		operation := ClassifyGateOperation(item.method, item.route)
		key := item.method + " " + item.route
		if !operation.Known {
			t.Errorf("%s (%s) no está en la tabla de la §6", key, item.source)
		}
		if len(operation.SeatEffect.Occupies()) > 0 && !occupying[operation.Row] {
			t.Errorf("%s ocupa cupo (%s) pero su fila es %s", key, operation.SeatEffect, operation.Row)
		}
		if operation.SeatEffect == licenseseats.Capture && operation.Row != licensing.RowCapture {
			t.Errorf("%s es captura para los cupos y %s para la tabla", key, operation.Row)
		}
		if operation.SeatEffect == licenseseats.OpenShift && operation.Row != licensing.RowOpenShift {
			t.Errorf("%s abre turno para los cupos y es %s para la tabla", key, operation.Row)
		}
		if operation.Row == licensing.RowCapture && operation.SeatEffect != licenseseats.Capture {
			t.Errorf("%s es captura en la tabla pero no pide cupo (%s)", key, operation.SeatEffect)
		}
		// Lo que no registra con fecha nunca es una escritura del negocio: ni captura, ni cobro, ni turnos.
		if !operation.Dated && (operation.Row == licensing.RowCapture || operation.Row == licensing.RowCaptured ||
			operation.Row == licensing.RowOpenShift || operation.Row == licensing.RowCloseShift) {
			t.Errorf("%s es %s y no registra con fecha", key, operation.Row)
		}
		seen[key] = true
	}
	// El tablero de Preparación es una lectura (F6): lo lee también la vista de una cuenta. Entrar en modo Cocina es
	// kitchen/display-mode (F5, con cupo).
	if board := ClassifyGateOperation("GET", "kitchen/board"); board.Row != licensing.RowRecords || board.Dated {
		t.Errorf("GET kitchen/board: %+v", board)
	}
	if mode := ClassifyGateOperation("POST", "kitchen/display-mode"); mode.Row != licensing.RowKitchen ||
		mode.SeatEffect != licenseseats.KitchenDisplay {
		t.Errorf("POST kitchen/display-mode: %+v", mode)
	}
	t.Logf("operaciones clasificadas: %d (%d del puente, %d del contrato, %d locales)", len(seen), len(bridge), len(contract),
		len(localRoutes))
}

// LicenseSeats da los cupos de la licencia real del gestor: los de la licencia en Vigente, Tolerancia, Vencida y
// Revocada; ninguno sin licencia válida; y el reloj del gestor.
func TestLicenseSeatsUseTheLicense(t *testing.T) {
	clock := licensingtest.NewClock(time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC))
	licenses := licensingtest.New(t, clock)
	manager := licenses.Start()
	seats := LicenseSeats(manager)
	if _, valid := seats.Limits(); valid {
		t.Fatal("sin licencia no hay cupos")
	}
	expires := clock.Now().Add(24 * time.Hour)
	licenses.Accept(manager, licensingtest.Subscription(expires, 1, 2, 0))
	for _, instant := range []time.Time{clock.Now(), expires.Add(time.Hour), expires.Add(6 * 24 * time.Hour)} {
		clock.Set(instant)
		limits, valid := seats.Limits()
		if !valid || limits != (licenseseats.Limits{CashRegisters: 1, OrderTakers: 2, KitchenDisplays: 0}) {
			t.Fatalf("%s: %+v %v", manager.Status().State, limits, valid)
		}
	}
	licenses.Accept(manager, licensingtest.Revoked(licensing.ReasonRevoked))
	if limits, valid := seats.Limits(); !valid || limits.CashRegisters != 3 {
		t.Fatalf("revocada: %+v %v", limits, valid)
	}
	if now, _ := seats.Window(); !now.Equal(clock.Now()) {
		t.Fatal("los cupos usan el reloj del gestor")
	}
	licenses.SetMachine(licensingtest.OtherMachineID)
	if _, valid := LicenseSeats(licenses.Start()).Limits(); valid {
		t.Fatal("una carpeta copiada no tiene cupos")
	}
	if _, valid := LicenseSeats(nil).Limits(); valid {
		t.Fatal("sin gestor no hay cupos")
	}
}
