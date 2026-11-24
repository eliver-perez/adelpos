//go:build !windows

package licensing

import (
	"errors"
	"os"
	"testing"
	"time"
)

// R2-1 y R2-3: si estado.json no se puede escribir después de bajar la copia de la base, la §6.3 no cambia nada: vale
// el máximo de las dos copias, la hora sigue igual y la copia de la base vuelve a subir en el momento.
func TestReplaceTrustedTimeNotSavedKeepsBothCopies(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root escribe en cualquier carpeta")
	}
	h, manager := blockedHarness(t)
	before := manager.TrustedTime()
	if !h.database.trustedCopy().Equal(before) {
		t.Fatalf("preparación: la copia de la base tiene la hora confiable: %s", h.database.trustedCopy())
	}
	if err := os.Chmod(h.directory, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(h.directory, 0700)
	})
	corrected := time.Date(2026, 11, 21, 12, 1, 0, 0, time.UTC)
	if _, err := manager.ReplaceTrustedTime(corrected, ReplaceByInternetCheck); !errors.Is(err, ErrStateNotSaved) {
		t.Fatalf("sin poder escribir estado.json: %v", err)
	}
	if !manager.TrustedTime().Equal(before) || !manager.Clock().Blocked || !h.database.trustedCopy().Equal(before) {
		t.Fatalf("no cambia nada: hora %s, copia %s", manager.TrustedTime(), h.database.trustedCopy())
	}
	if err := os.Chmod(h.directory, 0700); err != nil {
		t.Fatal(err)
	}
	if restarted := h.start(); !restarted.TrustedTime().Equal(before) {
		t.Fatalf("al reiniciar sigue la hora anterior: %s", restarted.TrustedTime())
	}
}
