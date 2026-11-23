// Package seatstest tiene los cupos y el reloj simulados de las pruebas automáticas (etapa L1, Fase D). Solo lo
// importan archivos de prueba: el servidor no lo usa, y su fuente de cupos no entrega ninguno fuera de una prueba
// (testing.Testing), así que ni por error puede llegar a un instalador.
package seatstest

import (
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licenseseats"
)

// ContractMaximum es el mayor cupo que puede traer una licencia (contrato §4: enteros de 0 a 999). En V1 no existe «sin
// límite»: las pruebas que no tratan de cupos usan este máximo.
const ContractMaximum = 999

// Source es una fuente de cupos que la prueba cambia cuando quiere: una licencia nueva con otros cupos (Set) o sin
// licencia válida (Withdraw).
type Source struct {
	mutex  sync.Mutex
	limits licenseseats.Limits
	valid  bool
}

// NewSource es una licencia válida con esos cupos de cajas, comanderos y pantallas de cocina.
func NewSource(cashRegisters, orderTakers, kitchenDisplays int) *Source {
	source := &Source{}
	source.Set(cashRegisters, orderTakers, kitchenDisplays)
	return source
}

// Set cambia los cupos, como una licencia nueva aceptada.
func (source *Source) Set(cashRegisters, orderTakers, kitchenDisplays int) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.limits = licenseseats.Limits{
		CashRegisters:   cashRegisters,
		OrderTakers:     orderTakers,
		KitchenDisplays: kitchenDisplays,
	}
	source.valid = true
}

// Withdraw deja la fuente sin licencia válida.
func (source *Source) Withdraw() {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.limits = licenseseats.Limits{}
	source.valid = false
}

// SeatLimits cumple licenseseats.LimitsSource. Fuera de una prueba no hay licencia válida.
func (source *Source) SeatLimits() (licenseseats.Limits, bool) {
	if !testing.Testing() {
		return licenseseats.Limits{}, false
	}
	source.mutex.Lock()
	defer source.mutex.Unlock()
	return source.limits, source.valid
}

// Clock es un reloj simulado (L1-D16): la prueba lo adelanta en lugar de esperar o de cambiar la hora de la Mac.
type Clock struct {
	mutex sync.Mutex
	now   time.Time
}

// NewClock empieza en la hora indicada.
func NewClock(start time.Time) *Clock {
	return &Clock{now: start}
}

// Now es la hora simulada.
func (clock *Clock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

// Advance adelanta el reloj.
func (clock *Clock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(duration)
}

// Roomy es un gestor con el máximo del contrato en los tres cupos y el reloj real, para las pruebas que no tratan de
// cupos.
func Roomy() *licenseseats.Manager {
	return licenseseats.NewManager(licenseseats.Options{
		Limits: NewSource(ContractMaximum, ContractMaximum, ContractMaximum),
	})
}
