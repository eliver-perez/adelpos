//go:build integration

package operations_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Revisión independiente R3 de la etapa L1 (asignación atómica de cupos, Fase D). Estas pruebas no corrigen nada:
// demuestran por la API real y PostgreSQL lo que la revisión encontró (doc-R3.md) y comprueban, con varias conexiones a
// la vez, lo que sí está bien. Usan el mismo andamiaje que TestStageL1LicenseSeats (seatsFixture) con los cupos y el
// reloj simulados de licenseseats/seatstest, cada una con su propia base desde cero.

// newR3SeatsFixture prepara la base de la revisión: catálogo, una estación de cocina con su ruta, un área con mesas
// vacías (una por pedido sin conexión) y dos meseros con PIN. Los cupos empiezan en cash, orderTakers y kitchenDisplays.
func newR3SeatsFixture(t *testing.T, cash, orderTakers, kitchenDisplays, tables int) *seatsFixture {
	t.Helper()
	resetStage9GDatabase(t)
	base := newFixture(t)
	base.seedCatalog(t)
	fixture := &seatsFixture{
		paymentsFixture: &paymentsFixture{restaurantFixture: base, methods: map[string]payments.Method{}, registers: map[string]string{}},
		source:          seatstest.NewSource(cash, orderTakers, kitchenDisplays),
		clock:           seatstest.NewClock(time.Now()),
		waiters:         map[string]string{},
	}
	base.service.Seats = licenseseats.NewManager(licenseseats.Options{Limits: fixture.source, Now: fixture.clock.Now})
	station := decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina R3", Active: true, Reception: "kds", TargetMinutes: 12, WarningMinutes: 15, OverdueMinutes: 25,
		Grouping: "inherit", Tracking: "inherit",
	}, 200))
	fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &fixture.category.ID, Mode: "prepare", StationIDs: []string{station.ID}}, 200)
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón R3", Active: true}, 200))
	for index := range tables {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa R3 %d", index+1), Shape: "round", X: 20 + (index%8)*120, Y: 30 + (index/8)*100,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	role := decodeResponse[struct {
		ID string `json:"id"`
	}](t, fixture.adminCall(t, "POST", "roles", map[string]any{"name": "Meseros R3", "permissions": waiterPermissions}, 200)).ID
	for index, name := range []string{"ana", "beto"} {
		fixture.waiters[name] = decodeResponse[struct {
			ID string `json:"id"`
		}](t, fixture.adminCall(t, "POST", "users", map[string]any{"username": name + "_r3", "display_name": "Mesero " + name,
			"role_id": role, "active": true, "password": "Clave temporal de " + name}, 200)).ID
		fixture.adminCall(t, "PUT", "users/"+fixture.waiters[name]+"/pin", map[string]any{"pin": "4829",
			"short_name": fmt.Sprintf("R%d", index+1), "badge_color": "#3591d4"}, 200)
	}
	return fixture
}

// offlineOrder es un pedido capturado sin conexión para una mesa vacía de la revisión.
func (fixture *seatsFixture) offlineOrder(t *testing.T, table int) map[string]any {
	t.Helper()
	var version int64
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT coalesce(max(version),0) FROM app.catalog_change_log").Scan(&version); err != nil {
		t.Fatal(err)
	}
	return offlineBody(randomID(), version, map[string]any{"table_id": fixture.tables[table].ID, "seen_account_ids": []string{}},
		offlineItem(fixture.fries.ID, 1, "40.00"))
}

// sessionlessTablet es una tableta en la que el mesero entró con su PIN (sesión y credencial de envío) y después se
// quedó sin sesión vigente en Go: la cola del teléfono sigue entregando sus pedidos con la credencial (A1-D1).
func (fixture *seatsFixture) sessionlessTablet(t *testing.T, name, waiter string) (testTerminal, string) {
	t.Helper()
	tablet := fixture.tablet(t, name)
	session, credential := fixture.enter(t, tablet, waiter)
	fixture.call(t, session, "POST", "auth/logout", nil, 200)
	return testTerminal{deviceID: tablet.deviceID, deviceToken: tablet.deviceToken}, credential
}

// R3-1: el pedido sin conexión que entra con la credencial de envío de una tableta sin sesión vigente «ocupa» un cupo de
// comandero que no cuenta (inUseCondition exige una sesión vigente en la terminal), así que la tableta captura sin
// cupo y las demás terminales capturan por encima del límite. Las tres variantes deben fallar mientras no se corrija.
func TestStageL1R3OfflineOrderWithoutSessionHoldsSeat(t *testing.T) {
	fixture := newR3SeatsFixture(t, 1, 1, 0, 12)

	t.Run("después de cerrar sesión el último mesero de la tableta", func(t *testing.T) {
		tablet, credential := fixture.sessionlessTablet(t, "Tableta R3 sin sesión", "ana")
		fixture.inUse(t, 0, 0, 0)
		code, body := fixture.send(tablet, credential, "POST", "restaurant/offline-orders", fixture.offlineOrder(t, 0))
		if code != 200 {
			t.Fatalf("el pedido de la cola con el cupo libre debía entrar: %d %s", code, body)
		}
		// La tableta acaba de capturar: debe ocupar el único comandero durante 15 minutos (§5 de la especificación).
		if usage := fixture.usage(t); usage["order_takers"].InUse != 1 {
			t.Errorf("R3-1: la tableta capturó y no ocupa el comandero: %+v", usage)
		}
		if seats := fixture.seatsOf(t, tablet); !slices.Equal(seats, []string{"order_takers"}) {
			t.Errorf("R3-1: Terminales no muestra el comandero de la tableta que capturó: %v", seats)
		}
		// Con 1 comandero ya en uso, otra terminal sin turno no captura.
		desktop := fixture.desktop(t, "captura_r3_sin_sesion", "")
		code, body = fixture.send(desktop, "", "POST", "restaurant/accounts", fixture.openBody())
		if code != 409 {
			t.Errorf("R3-1: dos terminales capturan con 1 comandero (la tableta y el escritorio): %d %s", code, body)
		}
	})

	t.Run("después de «Liberar cupo», con la cola de la tableta", func(t *testing.T) {
		// Vencen los comanderos anteriores; dos comanderos.
		fixture.clock.Advance(16 * time.Minute)
		fixture.source.Set(1, 2, 0)
		tablet := fixture.tablet(t, "Tableta R3 liberada")
		beto, credential := fixture.enter(t, tablet, "beto")
		fixture.capture(t, beto)
		holder := fixture.desktop(t, "captura_r3_liberar", "")
		fixture.capture(t, holder)
		fixture.inUse(t, 0, 2, 0)
		fixture.adminCall(t, "POST", "devices/"+tablet.deviceID+"/seat/release", map[string]any{}, 200)
		fixture.inUse(t, 0, 1, 0)
		// La cola de la tableta entrega un pedido con la credencial (Liberar cupo no la revoca): hay un cupo libre y entra.
		device := testTerminal{deviceID: tablet.deviceID, deviceToken: tablet.deviceToken}
		code, body := fixture.send(device, credential, "POST", "restaurant/offline-orders", fixture.offlineOrder(t, 1))
		if code != 200 {
			t.Fatalf("el pedido de la cola con un cupo libre debía entrar: %d %s", code, body)
		}
		if usage := fixture.usage(t); usage["order_takers"].InUse != 2 {
			t.Errorf("R3-1: la tableta liberada volvió a capturar y no ocupa el comandero: %+v", usage)
		}
		third := fixture.desktop(t, "tercera_r3_liberar", "")
		code, body = fixture.send(third, "", "POST", "restaurant/accounts", fixture.openBody())
		if code != 409 {
			t.Errorf("R3-1: tres terminales capturan con 2 comanderos: %d %s", code, body)
		}
	})

	t.Run("cuatro tabletas sin sesión a la vez por el último comandero", func(t *testing.T) {
		fixture.clock.Advance(16 * time.Minute)
		fixture.source.Set(1, 1, 0)
		fixture.inUse(t, 0, 0, 0)
		type contender struct {
			tablet     testTerminal
			credential string
			order      map[string]any
		}
		contenders := []contender{}
		for index := range 4 {
			tablet, credential := fixture.sessionlessTablet(t, fmt.Sprintf("Tableta R3 carrera %d", index+1), "ana")
			contenders = append(contenders, contender{tablet, credential, fixture.offlineOrder(t, 2+index)})
		}
		codes := make([]int, len(contenders))
		bodies := make([][]byte, len(contenders))
		var wait sync.WaitGroup
		for index, entry := range contenders {
			wait.Add(1)
			go func() {
				defer wait.Done()
				codes[index], bodies[index] = fixture.send(entry.tablet, entry.credential, "POST", "restaurant/offline-orders", entry.order)
			}()
		}
		wait.Wait()
		accepted := 0
		for index, code := range codes {
			if code == 200 {
				accepted++
				continue
			}
			noSeat(t, code, bodies[index], licenseseats.OrderTakers, 1, 1)
		}
		if accepted != 1 {
			t.Errorf("R3-1: %d tabletas obtuvieron el único comandero a la vez (cupos en uso después: %+v)", accepted, fixture.usage(t))
		}
	})
}

// R3-2: el gestor de cupos evalúa las sesiones con su propio reloj (Options.Now, L1-D16), pero las sesiones vencen con
// time.Now (issueSession escribe expires_at = time.Now + 12 h y authenticateSession compara con time.Now). Con el reloj
// del gestor adelantado más de 12 horas —el desfase de prueba que L1-D16 prevé para la Fase C, por ejemplo para probar
// la tolerancia o el vencimiento—, la sesión con que la terminal acaba de capturar ya no cuenta para los cupos aunque
// Go la siga aceptando, y cualquier número de terminales captura con 1 comandero.
func TestStageL1R3SeatClockAndSessionExpiry(t *testing.T) {
	fixture := newR3SeatsFixture(t, 1, 1, 0, 0)
	fixture.clock.Advance(13 * time.Hour)
	first := fixture.desktop(t, "captura_r3_reloj_1", "")
	fixture.capture(t, first)
	if usage := fixture.usage(t); usage["order_takers"].InUse != 1 {
		t.Errorf("R3-2: la terminal capturó con su sesión vigente y no ocupa el comandero: %+v", usage)
	}
	second := fixture.desktop(t, "captura_r3_reloj_2", "")
	code, body := fixture.send(second, "", "POST", "restaurant/accounts", fixture.openBody())
	if code != 409 {
		t.Errorf("R3-2: dos terminales capturan con 1 comandero cuando el reloj de los cupos va 13 h adelante: %d %s", code, body)
	}
}

// Verificación de la revisión (debe pasar): con varias conexiones a la vez, ninguna ruta entrega el último cupo a dos
// terminales cuando cada una tiene su sesión. La cocina solo toma 7820148311 (no 7820148305), así que es la carrera que
// depende únicamente del lock de los cupos; la caja y el comandero mezclan además la apertura de turno, la cola de las
// tabletas y las cuentas del escritorio.
func TestStageL1R3ConcurrentLastSeat(t *testing.T) {
	fixture := newR3SeatsFixture(t, 1, 1, 1, 12)

	race := func(t *testing.T, requests []func() (int, []byte)) ([]int, [][]byte) {
		t.Helper()
		codes := make([]int, len(requests))
		bodies := make([][]byte, len(requests))
		start := make(chan struct{})
		var wait sync.WaitGroup
		for index, request := range requests {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				codes[index], bodies[index] = request()
			}()
		}
		close(start)
		wait.Wait()
		return codes, bodies
	}
	winners := func(t *testing.T, round int, codes []int, bodies [][]byte, seat licenseseats.Seat, held int) {
		t.Helper()
		accepted := 0
		for index, code := range codes {
			if code == 200 {
				accepted++
				continue
			}
			noSeat(t, code, bodies[index], seat, held+1, held+1)
		}
		if accepted != 1 {
			t.Fatalf("vuelta %d: %d terminales obtuvieron el último cupo de %s", round+1, accepted, seat)
		}
	}

	t.Run("cocina: seis escritorios entran a Preparación a la vez", func(t *testing.T) {
		for round := range 3 {
			held := fixture.usage(t)["kitchen_displays"].InUse
			fixture.source.Set(1, 1, held+1)
			requests := []func() (int, []byte){}
			for index := range 6 {
				terminal := fixture.desktop(t, fmt.Sprintf("cocina_r3_%d_%d", round, index), "")
				requests = append(requests, func() (int, []byte) { return fixture.displayMode(terminal, true) })
			}
			codes, bodies := race(t, requests)
			winners(t, round, codes, bodies, licenseseats.KitchenDisplays, held)
			fixture.inUse(t, 0, 0, held+1)
		}
	})

	t.Run("caja: cuatro terminales de cuatro cajas abren turno a la vez", func(t *testing.T) {
		for round := range 2 {
			held := fixture.usage(t)["cash_registers"].InUse
			fixture.source.Set(held+1, 1, 3)
			shifts := fixture.countRows(t, "SELECT count(*)::int FROM app.cash_shifts")
			requests := []func() (int, []byte){}
			for index := range 4 {
				register := decodeResponse[struct {
					ID string `json:"id"`
				}](t, fixture.adminCall(t, "POST", "registers", map[string]any{"name": fmt.Sprintf("Caja R3 %d %d", round, index),
					"description": "", "active": true}, 200)).ID
				terminal := fixture.desktop(t, fmt.Sprintf("caja_r3_%d_%d", round, index), register)
				requests = append(requests, func() (int, []byte) {
					return fixture.send(terminal, "", "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"})
				})
			}
			codes, bodies := race(t, requests)
			winners(t, round, codes, bodies, licenseseats.CashRegisters, held)
			if fixture.countRows(t, "SELECT count(*)::int FROM app.cash_shifts") != shifts+1 {
				t.Fatalf("vuelta %d: se abrió más de un turno con un solo cupo de caja", round+1)
			}
		}
	})

	t.Run("comandero: tabletas con sesión y escritorios a la vez", func(t *testing.T) {
		table := 0
		for round := range 3 {
			held := fixture.usage(t)["order_takers"].InUse
			fixture.source.Set(3, held+1, 3)
			requests := []func() (int, []byte){}
			for index := range 2 {
				tablet := fixture.tablet(t, fmt.Sprintf("Tableta R3 %d %d", round, index))
				_, credential := fixture.enter(t, tablet, "beto")
				device := testTerminal{deviceID: tablet.deviceID, deviceToken: tablet.deviceToken}
				order := fixture.offlineOrder(t, table)
				table++
				requests = append(requests, func() (int, []byte) {
					return fixture.send(device, credential, "POST", "restaurant/offline-orders", order)
				})
			}
			for index := range 2 {
				terminal := fixture.desktop(t, fmt.Sprintf("comandero_r3_%d_%d", round, index), "")
				body := fixture.openBody()
				requests = append(requests, func() (int, []byte) {
					return fixture.send(terminal, "", "POST", "restaurant/accounts", body)
				})
			}
			codes, bodies := race(t, requests)
			winners(t, round, codes, bodies, licenseseats.OrderTakers, held)
		}
	})
}
