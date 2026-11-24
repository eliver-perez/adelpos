package administration

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
)

// Etapa L1, Fase D (hallazgo R2-2, L1-D43): los cupos de LicenseSeats consideran confiable la hora del gestor mientras
// la protección de la hora no esté en «Hora del equipo incorrecta». Solo entonces bajan a «ahora» una actividad de
// comandero con fecha futura.
func TestLicenseSeatsTrustTheClockOnlyWhenItIsNotBlocked(t *testing.T) {
	clock := licensingtest.NewClock(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	licenses := licensingtest.New(t, clock)
	manager := licenses.Start()
	seats := LicenseSeats(manager)
	if !seats.ClockTrusted() {
		t.Fatal("sin última hora confiable la hora no se bloquea: es confiable")
	}
	trusted := clock.Now()
	manager.ObserveTime(trusted, licensing.TimeFromOperation)
	// El reloj se atrasa 3 horas: «Hora del equipo incorrecta».
	clock.Set(trusted.Add(-3 * time.Hour))
	if !manager.Clock().Blocked || seats.ClockTrusted() {
		t.Fatal("con el reloj atrasado la hora de los cupos no es confiable")
	}
	// Dentro del margen de 10 minutos vuelve a ser confiable.
	clock.Set(trusted.Add(-5 * time.Minute))
	if manager.Clock().Blocked || !seats.ClockTrusted() {
		t.Fatal("dentro del margen la hora de los cupos es confiable")
	}
	if !LicenseSeats(nil).ClockTrusted() {
		t.Fatal("sin gestor de licencias la hora es la del sistema y es confiable")
	}
}
