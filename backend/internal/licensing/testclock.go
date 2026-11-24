package licensing

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// Reloj con desfase para los procesos de prueba (L1-D16): el Go temporal de Playwright y de Android, en
// test_mode, lee un desfase de un archivo (ADEL_TEST_CLOCK_FILE) para simular un reloj atrasado o
// adelantado sin cambiar nunca la hora de la computadora. No hay ninguna ruta de prueba en la API.
//
// El archivo contiene una duración de Go («-2h», «90m», «-36h30m»); vacío o inexistente, sin desfase. Se
// vuelve a leer como mucho una vez por segundo. Una compilación de producción nunca lo usa.

const testClockRefresh = time.Second

// OffsetClock devuelve el reloj local más el desfase del archivo. Falla en una compilación de producción.
func OffsetClock(filename string) (func() time.Time, error) {
	if !testHooksAllowed {
		return nil, errors.New("una versión de producción no usa el reloj de las pruebas")
	}
	if strings.TrimSpace(filename) == "" {
		return nil, errors.New("falta el archivo del desfase del reloj")
	}
	clock := &offsetClock{filename: filename}
	return clock.now, nil
}

type offsetClock struct {
	filename string
	mutex    sync.Mutex
	readAt   time.Time
	offset   time.Duration
}

func (clock *offsetClock) now() time.Time {
	actual := time.Now()
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	if clock.readAt.IsZero() || actual.Sub(clock.readAt) >= testClockRefresh || actual.Before(clock.readAt) {
		clock.offset = readClockOffset(clock.filename)
		clock.readAt = actual
	}
	return actual.Add(clock.offset)
}

// readClockOffset lee el desfase; un archivo ausente o inválido es «sin desfase».
func readClockOffset(filename string) time.Duration {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0
	}
	offset, err := time.ParseDuration(text)
	if err != nil {
		return 0
	}
	return offset
}
