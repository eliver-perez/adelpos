package licensing

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Pruebas de las correcciones de la revisión R2 (protección de la hora), además de las del revisor
// (review_r2_clock_test.go).

// R2-3: la última hora confiable vale el máximo de sus dos copias (estado.json y la base). Editar estado.json hacia atrás
// no la baja; un respaldo viejo de la base tampoco, y la copia de la base vuelve a subir. Solo ReplaceTrustedTime baja las
// dos; sin poder bajar la de la base no cambia nada.
func TestTrustedTimeUsesBothCopies(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "perpetual-0-months")
	future := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	h.clock.set("2026-11-25T12:00:00Z")
	manager.ObserveTime(h.clock.now(), TimeFromOperation)
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}
	if copied := h.database.trustedCopy(); !copied.Equal(future) {
		t.Fatalf("Flush sube la copia de la base: %s", copied)
	}
	h.clock.set("2026-11-21T12:00:00Z")

	// estado.json editado hacia atrás: manda la base, y estado.json vuelve a subir.
	edited := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	state := h.readState()
	state.TrustedTime = &edited
	h.writeState(state)
	restarted := h.start()
	if !restarted.TrustedTime().Equal(future) || !restarted.Clock().Blocked {
		t.Fatalf("editar estado.json no baja la hora confiable: %s", restarted.TrustedTime())
	}
	if saved := h.readState().TrustedTime; saved == nil || !saved.Equal(future) {
		t.Fatalf("estado.json vuelve a subir al máximo de las dos copias: %v", saved)
	}

	// Un respaldo viejo de la base: manda estado.json, y al arrancar la copia de la base vuelve a subir.
	h.database.setTrustedCopy(edited)
	restarted = h.start()
	if !restarted.TrustedTime().Equal(future) || !h.database.trustedCopy().Equal(future) {
		t.Fatalf("un respaldo viejo de la base no la baja: %s, copia %s", restarted.TrustedTime(), h.database.trustedCopy())
	}

	// Sin poder escribir la copia de la base, la §6.3 no cambia nada.
	corrected := time.Date(2026, 11, 21, 12, 1, 0, 0, time.UTC)
	h.database.mutex.Lock()
	h.database.copyFail = errors.New("la base no respondió")
	h.database.mutex.Unlock()
	if _, err := restarted.ReplaceTrustedTime(corrected, ReplaceByInternetCheck); !errors.Is(err, ErrStateNotSaved) {
		t.Fatalf("sin la copia de la base, ErrStateNotSaved: %v", err)
	}
	if !restarted.TrustedTime().Equal(future) || !h.readState().TrustedTime.Equal(future) || !h.database.trustedCopy().Equal(future) {
		t.Fatal("sin la copia de la base no cambia nada")
	}

	// Con la base, la §6.3 baja las dos copias.
	h.database.mutex.Lock()
	h.database.copyFail = nil
	h.database.mutex.Unlock()
	previous, err := restarted.ReplaceTrustedTime(corrected, ReplaceByInternetCheck)
	if err != nil || !previous.Equal(future) {
		t.Fatalf("la §6.3: %v %v", previous, err)
	}
	if !restarted.TrustedTime().Equal(corrected) || !h.readState().TrustedTime.Equal(corrected) ||
		!h.database.trustedCopy().Equal(corrected) || restarted.Clock().Blocked {
		t.Fatal("la §6.3 baja las dos copias y quita el bloqueo")
	}
	if again := h.start(); !again.TrustedTime().Equal(corrected) {
		t.Fatalf("la corrección sigue al reiniciar (la auditoría no vuelve a subirla): %s", again.TrustedTime())
	}
}

// R2-4: si la base no responde al arrancar, la semilla queda pendiente. Mientras tanto, el reloj local y las operaciones
// no se vuelven la primera hora confiable; el server_time sí (no es el reloj local). Con una hora confiable ya guardada,
// el reloj se sigue observando.
func TestPendingSeedDoesNotTrustTheLocalClock(t *testing.T) {
	h := newHarness(t)
	h.database.mutex.Lock()
	h.database.latest = time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	h.database.failure = errors.New("la base no respondió a tiempo")
	h.database.mutex.Unlock()
	manager := h.start()
	manager.ObserveTime(h.clock.now(), TimeFromOperation)
	manager.tick(context.Background())
	if !manager.TrustedTime().IsZero() {
		t.Fatalf("con la semilla pendiente, el reloj local no es la primera hora confiable: %s", manager.TrustedTime())
	}
	server := time.Date(2026, 11, 21, 11, 0, 0, 0, time.UTC)
	manager.ObserveTime(server, TimeFromServer)
	if !manager.TrustedTime().Equal(server) {
		t.Fatalf("el server_time sí se toma: %s", manager.TrustedTime())
	}
	// Ya con una hora confiable, el reloj se observa aunque la base siga sin responder.
	manager.tick(context.Background())
	if !manager.TrustedTime().Equal(h.clock.now()) {
		t.Fatalf("con hora confiable, el reloj sube: %s", manager.TrustedTime())
	}
	// Cuando la base responde, la semilla la sube a su actividad (por delante del reloj) y se bloquea.
	h.database.mutex.Lock()
	h.database.failure = nil
	h.database.mutex.Unlock()
	manager.tick(context.Background())
	if !manager.TrustedTime().Equal(h.database.latest) || !manager.Clock().Blocked || !h.database.trustedCopy().Equal(h.database.latest) {
		t.Fatalf("la semilla se reintenta: %s, copia %s", manager.TrustedTime(), h.database.trustedCopy())
	}
	calls := h.database.activityCalls
	manager.tick(context.Background())
	if h.database.activityCalls != calls {
		t.Fatal("ya sembrada, la auditoría no se vuelve a leer")
	}
}

// R2-1: CheckSupportUnlock comprueba sin cambiar nada; si el desbloqueo no se puede guardar (aquí, la copia de la base),
// la hora no cambia y la respuesta de un solo uso sigue sin usar; al reintentar, se acepta una sola vez.
func TestSupportUnlockIsNotSpentWhenNotSaved(t *testing.T) {
	h, manager := blockedHarness(t)
	request, err := manager.SupportRequest(true)
	if err != nil {
		t.Fatal(err)
	}
	private, trust := supportTestKey(t)
	response := signSupport(t, private, request.Code, h.clock.now(), h.clock.now())
	before := manager.TrustedTime()
	for range 2 {
		checked, err := manager.CheckSupportUnlock(response, trust)
		if err != nil || !checked.PreviousTrustedTime.Equal(before) || !checked.NewTrustedTime.Equal(h.clock.now()) {
			t.Fatalf("la comprobación: %+v %v", checked, err)
		}
	}
	if !manager.TrustedTime().Equal(before) || h.readState().Support.Pending == nil {
		t.Fatal("comprobar no cambia nada")
	}
	h.database.mutex.Lock()
	h.database.copyFail = errors.New("la base no respondió")
	h.database.mutex.Unlock()
	if _, err := manager.AcceptSupportUnlock(response, trust); !errors.Is(err, ErrStateNotSaved) {
		t.Fatalf("sin la copia de la base: %v", err)
	}
	if !manager.TrustedTime().Equal(before) || !manager.Clock().Blocked || h.readState().Support.Pending == nil ||
		len(h.readState().Support.Used) != 0 {
		t.Fatal("sin guardar, la hora no baja y la respuesta no se gasta")
	}
	h.database.mutex.Lock()
	h.database.copyFail = nil
	h.database.mutex.Unlock()
	if _, err := manager.AcceptSupportUnlock(response, trust); err != nil {
		t.Fatalf("al reintentar se acepta: %v", err)
	}
	if manager.Clock().Blocked || !h.database.trustedCopy().Equal(h.clock.now()) || !h.readState().TrustedTime.Equal(h.clock.now()) {
		t.Fatal("el desbloqueo baja las dos copias")
	}
	var rejected *SupportUnlockError
	if _, err := manager.CheckSupportUnlock(response, trust); !errors.As(err, &rejected) || rejected.Reason != SupportReasonUsed {
		t.Fatalf("una sola vez: %v", err)
	}
}

// R2-5: una carpeta copiada no pide ni acepta un desbloqueo de soporte, y sin la huella comprobada tampoco.
func TestSupportUnlockOnlyInTheOriginalInstallation(t *testing.T) {
	h, manager := blockedHarness(t)
	if _, err := manager.SupportRequest(true); err != nil {
		t.Fatal(err)
	}
	h.machine = machineOther
	copied := h.start()
	if !copied.Status().Copied {
		t.Fatal("preparación: la copia debía detectarse")
	}
	if _, err := copied.SupportRequest(true); !errors.Is(err, ErrCopiedInstallation) {
		t.Fatalf("la copia no pide un código: %v", err)
	}
	h.machine = machineOriginal
	request, err := manager.SupportRequest(false)
	if err != nil {
		t.Fatal(err)
	}
	private, trust := supportTestKey(t)
	response := signSupport(t, private, request.Code, h.clock.now(), h.clock.now())
	if _, err := copied.CheckSupportUnlock(response, trust); !errors.Is(err, ErrCopiedInstallation) {
		t.Fatalf("la copia no acepta la respuesta: %v", err)
	}

	installation := h.fixtures.installations["installation-fixture-a"]
	unverified := NewManager(Options{
		Directory: h.directory, Installation: &installation, Trust: h.fixtures.testTrust(), Now: h.clock.now,
		MachineID: func() (string, error) {
			return "", errors.New("sin ioreg")
		},
		MachineRetryDelay: time.Millisecond,
		Database:          h.database,
	})
	if err := unverified.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := unverified.SupportRequest(true); !errors.Is(err, ErrMachineUnverified) {
		t.Fatalf("sin huella no se pide un código: %v", err)
	}
	if _, err := unverified.AcceptSupportUnlock(response, trust); !errors.Is(err, ErrMachineUnverified) {
		t.Fatalf("sin huella no se acepta la respuesta: %v", err)
	}
	if _, err := manager.AcceptSupportUnlock(response, trust); err != nil {
		t.Fatalf("la instalación original sí la acepta: %v", err)
	}
}
