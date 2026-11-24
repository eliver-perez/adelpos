//go:build integration

package licenseseats_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
)

// Etapa L1, Fase D: corrección del hallazgo R2-2 (decisión L1-D43). La fecha saltó al futuro, una tableta capturó con
// esa fecha y después se corrigió el reloj. Su actividad «en el futuro» cuenta como actividad de ahora y vence 15
// minutos reales después: la primera asignación o previa con la hora corregida la baja a «ahora» y la guarda. Si esa
// asignación se rechaza por falta de cupo, la transacción se revierte y la guarda SettleFutureActivity en una propia
// (administration lo hace en Execute). Con la hora en «Hora del equipo incorrecta» no se baja nada.
func TestSeatActivityFromAClockJumpCountsAsNow(t *testing.T) {
	db := newSeatsDatabase(t)
	source := seatstest.NewSource(1, 1, 1)
	clock := seatstest.NewClock(time.Now())
	trusted := true
	manager := licenseseats.NewManager(licenseseats.Options{Limits: source, Now: clock.Now, ClockTrusted: func() bool {
		return trusted
	}})
	capture := func(member terminal) error {
		return db.within(func(tx pgx.Tx) error {
			return manager.Capture(db.ctx, tx, member.device)
		})
	}
	renew := func(member terminal) {
		t.Helper()
		if err := db.within(func(tx pgx.Tx) error {
			return manager.Renew(db.ctx, tx, member.device)
		}); err != nil {
			t.Fatal(err)
		}
	}
	settle := func() {
		t.Helper()
		if err := db.within(func(tx pgx.Tx) error {
			return manager.SettleFutureActivity(db.ctx, tx)
		}); err != nil {
			t.Fatal(err)
		}
	}
	activity := func(member terminal) time.Time {
		t.Helper()
		var instant time.Time
		if err := db.pool.QueryRow(db.ctx, "SELECT last_activity_at FROM app.license_seats WHERE device_id=$1 AND kind='order_taker'",
			member.device).Scan(&instant); err != nil {
			t.Fatal(err)
		}
		return instant
	}
	// La base guarda microsegundos: la actividad está «en ahora» si difiere menos de un milisegundo del reloj.
	atNow := func(member terminal) bool {
		difference := activity(member).Sub(clock.Now())
		return difference < time.Millisecond && difference > -time.Millisecond
	}
	first := db.terminal("Comandero salto uno", false)
	second := db.terminal("Comandero salto dos", false)
	// La primera tableta captura con el reloj dos días adelantado; un minuto real después se corrige el reloj.
	jump := func() {
		t.Helper()
		if _, err := db.pool.Exec(db.ctx, "DELETE FROM app.license_seats"); err != nil {
			t.Fatal(err)
		}
		clock.Advance(48 * time.Hour)
		if err := capture(first); err != nil {
			t.Fatal(err)
		}
		clock.Advance(-48*time.Hour + time.Minute)
	}

	t.Run("el primer intento de otra tableta, aunque se rechace, empieza los 15 minutos", func(t *testing.T) {
		jump()
		// Con la hora corregida la actividad del salto cuenta como de ahora: no hay cupo.
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 1 {
			t.Fatalf("la actividad del salto no cuenta como de ahora: %d en uso", inUse)
		}
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		// El rechazo revirtió lo que claim bajó: por eso administration lo guarda aparte.
		if !activity(first).After(clock.Now().Add(47 * time.Hour)) {
			t.Fatal("el rechazo debía revertir toda su transacción")
		}
		settle()
		if !atNow(first) {
			t.Fatalf("la actividad del salto no se bajó a ahora: %v (ahora %v)", activity(first), clock.Now())
		}
		clock.Advance(14 * time.Minute)
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		settle()
		clock.Advance(time.Minute)
		// 15 minutos reales desde el primer intento con la hora corregida: se libera.
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("comanderos en uso 15 minutos después de corregir el reloj: %d", inUse)
		}
		if err := capture(second); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("la tableta del salto renueva con la hora corregida (previa y captura)", func(t *testing.T) {
		jump()
		// La previa ya no conserva la fecha futura (antes, greatest): queda en ahora y vence a los 15 minutos.
		renew(first)
		if !atNow(first) {
			t.Fatalf("la previa conservó la fecha del salto: %v", activity(first))
		}
		clock.Advance(15 * time.Minute)
		if inUse := db.inUse(manager, licenseseats.OrderTakers); inUse != 0 {
			t.Fatalf("comanderos en uso 15 minutos después de la previa: %d", inUse)
		}
		// La captura de la misma tableta también renueva con la hora corregida.
		jump()
		if err := capture(first); err != nil {
			t.Fatal(err)
		}
		if !atNow(first) {
			t.Fatalf("la captura conservó la fecha del salto: %v", activity(first))
		}
		clock.Advance(14 * time.Minute)
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		clock.Advance(time.Minute)
		if err := capture(second); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("con la hora bloqueada no se baja nada", func(t *testing.T) {
		jump()
		// «Hora del equipo incorrecta»: no se sabe si la equivocada es la actividad o la hora de ahora.
		trusted = false
		renew(first)
		settle()
		unavailable(t, capture(second), licenseseats.OrderTakers, 1, 1)
		settle()
		if !activity(first).After(clock.Now().Add(47 * time.Hour)) {
			t.Fatalf("se bajó la actividad con la hora bloqueada: %v", activity(first))
		}
		// Una actividad que no está en el futuro sí se renueva (nunca retrocede).
		clock.Advance(48 * time.Hour)
		renew(first)
		if !atNow(first) {
			t.Fatalf("la previa con la hora bloqueada no renovó: %v", activity(first))
		}
		clock.Advance(-48 * time.Hour)
		// Con la hora confiable otra vez, la previa la baja a ahora.
		trusted = true
		renew(first)
		if !atNow(first) {
			t.Fatalf("la previa con la hora confiable no bajó la actividad: %v", activity(first))
		}
		clock.Advance(15 * time.Minute)
		if err := capture(second); err != nil {
			t.Fatal(err)
		}
	})
}
