//go:build integration

package licenseseats_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/licenseseats"
	"local/ad-el-pos/server/internal/licenseseats/seatstest"
)

// Revisión independiente R2 de la etapa L1 (la protección de la hora), hallazgo R2-2. La prueba falla hasta que se
// corrija (ver $SP/l1/doc-R2.md).
//
// El caso difícil de la §6.3: la fecha saltó al futuro, se registró actividad con esa fecha y después la hora se
// corrigió. Un comandero que capturó durante el salto guarda last_activity_at en el futuro (la hora de Go en ese
// momento). Después de corregir el reloj y quitar el bloqueo (§6.3 o §6.4), esa fila sigue «en uso» hasta que el reloj
// real alcance la fecha del salto más 15 minutos, mientras la terminal conserve una sesión vigente: la regla de los 15
// minutos sin captura no se cumple, y otra terminal recibe LICENSE_SEAT_UNAVAILABLE («No hay cupo de comandero libre»)
// por algo que en realidad causó la hora. Nada lo libera solo: hace falta cerrar la sesión o «Liberar cupo».
func TestReviewR2DefectSeatActivityFromAClockJumpOutlivesTheCorrection(t *testing.T) {
	db := newSeatsDatabase(t)
	source := seatstest.NewSource(1, 1, 1)
	clock := seatstest.NewClock(time.Now())
	manager := licenseseats.NewManager(licenseseats.Options{Limits: source, Now: clock.Now})
	capture := func(member terminal) error {
		return db.within(func(tx pgx.Tx) error {
			return manager.Capture(db.ctx, tx, member.device)
		})
	}
	first := db.terminal("Comandero R2 uno", false)
	second := db.terminal("Comandero R2 dos", false)

	// El reloj salta dos días al futuro y la primera tableta captura con esa fecha.
	clock.Advance(48 * time.Hour)
	if err := capture(first); err != nil {
		t.Fatal(err)
	}
	// Un minuto después (en tiempo real) se corrige el reloj; la tableta ya no captura.
	clock.Advance(-48*time.Hour + time.Minute)
	// Pasan 16 minutos reales sin ninguna captura de la primera tableta: su cupo de comandero ya debía liberarse.
	clock.Advance(16 * time.Minute)
	inUse := db.inUse(manager, licenseseats.OrderTakers)
	err := capture(second)
	if inUse != 0 || err != nil {
		t.Errorf("R2-2: 16 minutos reales sin captura después de corregir el reloj, el comandero sigue en uso (%d de 1) y "+
			"la otra tableta recibe: %v", inUse, err)
	}
}
