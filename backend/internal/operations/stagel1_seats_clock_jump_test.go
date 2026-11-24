//go:build integration

package operations_test

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/operations"
)

// Etapa L1, Fase D: corrección del hallazgo R2-2 (decisión L1-D43) por la API real. La fecha saltó al futuro, una
// terminal capturó con esa fecha y después se corrigió el reloj. Su comandero cuenta como actividad de ahora y vence 15
// minutos reales después:
//
//   - el primer intento de otra terminal con la hora corregida se rechaza (1 de 1), pero deja guardada esa actividad en
//     «ahora» (administration la guarda en su propia transacción, porque el rechazo revierte la de la petición);
//   - la terminal del salto que sigue trabajando renueva con la hora corregida al entrar a la mesa (la previa).
//
// Usa el andamiaje de la revisión R3 (newR3SeatsFixture), con su propia base desde cero y sin la puerta de la licencia
// (la hora de los cupos es confiable).
func TestStageL1SeatActivityFromAClockJump(t *testing.T) {
	fixture := newR3SeatsFixture(t, 1, 1, 0, 0)
	first := fixture.desktop(t, "salto_reloj_1", "")
	second := fixture.desktop(t, "salto_reloj_2", "")
	activityAtNow := func(terminal testTerminal) {
		t.Helper()
		var activity time.Time
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT last_activity_at FROM app.license_seats
            WHERE device_id=$1 AND kind='order_taker'`, terminal.deviceID).Scan(&activity); err != nil {
			t.Fatal(err)
		}
		if difference := activity.Sub(fixture.clock.Now()); difference >= time.Millisecond || difference <= -time.Millisecond {
			t.Fatalf("la actividad del comandero no está en ahora: %v (ahora %v)", activity, fixture.clock.Now())
		}
	}
	preview := func(terminal testTerminal, accountID string) {
		t.Helper()
		current := fixture.refresh(t, accountID)
		fixture.call(t, terminal, "POST", "restaurant/accounts/"+accountID+"/draft-preview", map[string]any{
			"revision": current.Account.Revision, "items": []operations.DraftItem{draftLine(fixture.fries, 2)},
		}, 200)
	}

	t.Run("el intento rechazado de otra terminal empieza los 15 minutos", func(t *testing.T) {
		fixture.clock.Advance(48 * time.Hour)
		fixture.capture(t, first)
		fixture.clock.Advance(-48*time.Hour + time.Minute)
		fixture.inUse(t, 0, 1, 0)
		fixture.refused(t, second, licenseseats.OrderTakers, 1, 1)
		activityAtNow(first)
		fixture.clock.Advance(14 * time.Minute)
		fixture.refused(t, second, licenseseats.OrderTakers, 1, 1)
		fixture.clock.Advance(time.Minute)
		// 15 minutos reales desde el primer intento con la hora corregida.
		fixture.inUse(t, 0, 0, 0)
		if seats := fixture.seatsOf(t, first); len(seats) != 0 {
			t.Fatalf("el comandero vencido sigue en Terminales: %v", seats)
		}
		fixture.capture(t, second)
		fixture.inUse(t, 0, 1, 0)
	})

	t.Run("la terminal del salto renueva con la hora corregida al entrar a la mesa", func(t *testing.T) {
		// La segunda tiene el cupo; el reloj salta, captura y se corrige.
		fixture.clock.Advance(48 * time.Hour)
		opened := fixture.capture(t, second)
		fixture.clock.Advance(-48*time.Hour + time.Minute)
		preview(second, opened.Account.ID)
		activityAtNow(second)
		fixture.clock.Advance(14 * time.Minute)
		fixture.refused(t, first, licenseseats.OrderTakers, 1, 1)
		fixture.clock.Advance(time.Minute)
		fixture.capture(t, first)
	})
}
