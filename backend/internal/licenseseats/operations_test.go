package licenseseats

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const someID = "0f7a2b1c-1d2e-4f30-8a40-5b6c7d8e9f00"

// La clasificación de los cupos (L1-D13 y L1-D3): qué ocupa, renueva o libera cada operación. Cobrar, las operaciones
// de efectivo, reorganizar cuentas, la cocina (salvo el modo Cocina) y las lecturas no tocan ningún cupo.
func TestForOperation(t *testing.T) {
	cases := []struct {
		method, route string
		effect        Effect
	}{
		// Crear o modificar un pedido: ocupa el cupo (caja o comandero) y renueva la actividad.
		{"POST", "restaurant/accounts", Capture},
		{"PUT", "restaurant/accounts/" + someID + "/draft", Capture},
		{"POST", "restaurant/accounts/" + someID + "/confirm", Capture},
		{"POST", "restaurant/accounts/" + someID + "/edit", Capture},
		{"POST", "restaurant/offline-orders", Capture},
		// Con el prefijo de la API y la diagonal final también.
		{"POST", "/api/v1/restaurant/accounts/", Capture},
		// Previas: solo renuevan.
		{"POST", "restaurant/accounts/" + someID + "/draft-preview", Preview},
		{"POST", "restaurant/accounts/" + someID + "/edit-preview", Preview},
		// Turnos.
		{"POST", "payments/shifts", OpenShift},
		{"POST", "payments/shifts/" + someID + "/closing/confirm", CloseShift},
		// Cocina y Terminales.
		{"POST", "kitchen/display-mode", KitchenDisplay},
		{"POST", "devices/" + someID + "/seat/release", DeviceRelease},
		// Cierres de sesión.
		{"POST", "auth/logout", SessionEnd},
		{"POST", "auth/password", SessionEnd},
		{"POST", "sessions/" + someID + "/revoke", SessionEnd},
		{"POST", "devices/" + someID + "/revoke", SessionEnd},
		{"PUT", "auth/pin", SessionEnd},
		{"PATCH", "users/" + someID, SessionEnd},
		{"PUT", "users/" + someID + "/pin", SessionEnd},
		{"POST", "users/" + someID + "/pin/remove", SessionEnd},
		{"POST", "license/identity/regenerate", SessionEnd},
		{"POST", "users/" + someID + "/pin/unlock", None},
		{"POST", "users", None},
		{"GET", "users/" + someID, None},
		{"GET", "auth/pin", None},
		// Lo que nunca pide cupo.
		{"GET", "restaurant/accounts", None},
		{"GET", "restaurant/accounts/" + someID, None},
		{"GET", "restaurant/accounts/" + someID + "/preview", None},
		{"POST", "restaurant/accounts/" + someID + "/cancel", None},
		{"POST", "restaurant/accounts/" + someID + "/cancel-account", None},
		{"POST", "restaurant/accounts/" + someID + "/discard", None},
		{"POST", "restaurant/accounts/" + someID + "/split", None},
		{"POST", "restaurant/accounts/" + someID + "/transfer", None},
		{"POST", "restaurant/accounts/" + someID + "/merge", None},
		{"POST", "restaurant/accounts/" + someID + "/move", None},
		{"POST", "restaurant/accounts/" + someID + "/customer", None},
		{"PATCH", "restaurant/accounts/" + someID, None},
		{"POST", "payments/accounts/" + someID + "/payments", None},
		{"POST", "payments/accounts/" + someID + "/discounts", None},
		{"POST", "payments/accounts/" + someID + "/refunds", None},
		{"POST", "payments/accounts/" + someID + "/precheck/print", None},
		{"POST", "payments/accounts/" + someID + "/deliver", None},
		{"POST", "payments/shifts/" + someID + "/income", None},
		{"POST", "payments/shifts/" + someID + "/withdrawals", None},
		{"POST", "payments/shifts/" + someID + "/expenses", None},
		{"POST", "payments/shifts/" + someID + "/closing", None},
		{"POST", "payments/shifts/" + someID + "/closing/cancel", None},
		{"POST", "payments/cash/open-drawer", None},
		{"GET", "payments/shifts/current", None},
		{"GET", "kitchen/board", None},
		{"POST", "kitchen/states", None},
		{"GET", "kitchen/display-mode", None},
		{"POST", "auth/login", None},
		{"POST", "auth/pin-login", None},
		{"GET", "auth/pin-users", None},
		{"GET", "devices", None},
		{"GET", "devices/sessions-summary", None},
		{"PATCH", "devices/" + someID, None},
		{"POST", "devices/pairing-requests/" + someID + "/approve", None},
		{"GET", "reports/sales", None},
		{"POST", "catalog/products", None},
	}
	for _, item := range cases {
		if got := ForOperation(item.method, item.route); got != item.effect {
			t.Errorf("%s %s: %q, esperado %q", item.method, item.route, got, item.effect)
		}
	}
}

// Cada efecto dice qué cupos ocupa y cuáles libera, con nombres del contrato; solo la captura y las previas renuevan.
func TestEffectsDescribeTheirSeats(t *testing.T) {
	for _, effect := range Effects {
		for _, seat := range append(effect.Occupies(), effect.Releases()...) {
			if !slices.Contains(Seats, seat) {
				t.Fatalf("%s: cupo desconocido %q", effect, seat)
			}
		}
		if effect.Renews() != (effect == Capture || effect == Preview) {
			t.Fatalf("%s: renueva %v", effect, effect.Renews())
		}
	}
	if !slices.Equal(Capture.Occupies(), []Seat{CashRegisters, OrderTakers}) || None.Occupies() != nil || None.Releases() != nil {
		t.Fatal("la captura ocupa caja o comandero y None no toca nada")
	}
}

// El mensaje sin cupo dice qué cupo falta y cuántos usa la licencia (§5 de la especificación), en español sencillo y sin
// identificadores internos.
func TestUnavailableMessage(t *testing.T) {
	expected := map[Seat]string{
		OrderTakers:     "No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra terminal.",
		CashRegisters:   "No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de caja.",
		KitchenDisplays: "No hay cupo de pantalla de cocina libre (2 de 1 en uso). Pide que otra terminal salga de Preparación.",
	}
	usage := map[Seat][2]int{OrderTakers: {3, 3}, CashRegisters: {1, 1}, KitchenDisplays: {2, 1}}
	for seat, text := range expected {
		problem := &UnavailableError{Seat: seat, InUse: usage[seat][0], Limit: usage[seat][1]}
		if problem.Message() != text || problem.Error() != text {
			t.Fatalf("%s: %q", seat, problem.Message())
		}
	}
	for _, seat := range Seats {
		message := (&UnavailableError{Seat: seat}).Message()
		if !strings.Contains(message, "La licencia no incluye") || !strings.Contains(message, "(0 de 0)") {
			t.Fatalf("%s sin cupos en la licencia: %q", seat, message)
		}
		for _, internal := range []string{"order_taker", "cash_register", "kitchen_display", "_"} {
			if strings.Contains(message, internal) {
				t.Fatalf("%s: identificador interno en el mensaje %q", seat, message)
			}
		}
	}
}

// Sin fuente o con una fuente sin licencia válida no hay cupos (falla cerrada); el reloj y los 15 minutos tienen sus
// valores del backend.
func TestManagerDefaults(t *testing.T) {
	manager := NewManager(Options{})
	if limits, valid := manager.Limits(); valid || limits != (Limits{}) {
		t.Fatalf("sin fuente: %+v %v", limits, valid)
	}
	withdrawn := NewManager(Options{Limits: LimitsFunc(func() (Limits, bool) {
		return Limits{CashRegisters: 4, OrderTakers: 4, KitchenDisplays: 4}, false
	})})
	if limits, valid := withdrawn.Limits(); valid || limits != (Limits{}) {
		t.Fatalf("sin licencia válida: %+v %v", limits, valid)
	}
	now, cutoff, sessionNow := manager.times()
	if now.Sub(cutoff) != OrderTakerIdle || OrderTakerIdle.Minutes() != 15 || sessionNow.Before(now) {
		t.Fatalf("plazo del comandero: %v", now.Sub(cutoff))
	}
	// El reloj de las sesiones no es el de los cupos (R3-2): con el de los cupos adelantado, las sesiones siguen con el
	// suyo.
	ahead := NewManager(Options{Now: func() time.Time { return time.Now().Add(13 * time.Hour) }})
	_, cutoff = ahead.Window()
	sessionNow = ahead.SessionNow()
	if cutoff.Sub(sessionNow) < 12*time.Hour || time.Since(sessionNow) > time.Minute {
		t.Fatalf("relojes de los cupos y de las sesiones: %v %v", cutoff, sessionNow)
	}
	if LockKey != 7820148311 {
		t.Fatalf("lock de los cupos: %d", LockKey)
	}
}
