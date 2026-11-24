package licensing

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Revisión independiente R2 de la etapa L1 (la protección de la hora, §6 de la especificación). Las pruebas «…Defect…»
// fallan hasta que se corrija su hallazgo; las demás comprueban lo que está bien o documentan un límite (ver
// $SP/l1/doc-R2.md).

// blockedHarness es una instalación con la licencia perpetua de los archivos de prueba (emitida el 16/11/2026 06:30
// UTC), actividad registrada el 25/11/2026 12:00 con la fecha en el futuro y el reloj corregido al 21/11/2026 12:00: el
// caso difícil de la §6.3, con el sistema bloqueado.
func blockedHarness(t *testing.T) (*harness, *Manager) {
	t.Helper()
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "perpetual-0-months")
	h.clock.set("2026-11-25T12:00:00Z")
	manager.ObserveTime(h.clock.now(), TimeFromOperation)
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}
	h.clock.set("2026-11-21T12:00:00Z")
	if !manager.Clock().Blocked {
		t.Fatal("preparación: el reloj corregido debía quedar bloqueado")
	}
	return h, manager
}

// R2-4: la última hora confiable se inicia con max(audit_events.occurred_at) solo al arrancar y solo si falta
// (L1-D15). Si esa consulta falla en ese arranque (la base tarda, un lock_timeout de 2 s, el plazo de 5 s), no se
// vuelve a intentar: el trabajador de cada minuto toma la hora local como primera hora confiable. Con un reloj atrasado
// en ese momento (la primera vez que arranca la 0.15.0, o después de un estado.json dañado) la actividad ya registrada
// queda «en el futuro» y no se bloquea nada: la protección empieza desde el reloj incorrecto, sin aviso.
func TestReviewR2DefectTrustedTimeSeedIsNotRetried(t *testing.T) {
	h := newHarness(t)
	// La base tiene actividad hasta las 15:00; el reloj de la computadora dice 12:00 (3 horas atrasado).
	h.database.mutex.Lock()
	h.database.latest = time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	h.database.failure = errors.New("la base no respondió a tiempo")
	h.database.mutex.Unlock()
	manager := h.start()
	if !manager.TrustedTime().IsZero() {
		t.Fatal("preparación: sin la consulta no hay hora confiable")
	}
	// La base ya responde; pasa un minuto del trabajador.
	h.database.mutex.Lock()
	h.database.failure = nil
	h.database.mutex.Unlock()
	manager.tick(context.Background())
	restarted := h.start()
	t.Logf("hora confiable: %s (fuente %s); actividad en la base: 15:00; reloj: %s",
		restarted.TrustedTime().Format(time.RFC3339), h.readState().TrustedSource, h.clock.now().Format(time.RFC3339))
	if !manager.Clock().Blocked || !restarted.Clock().Blocked {
		t.Errorf("R2-4: la actividad registrada a las 15:00 quedó después de la hora confiable %s y el reloj atrasado 3 "+
			"horas no se bloquea", restarted.TrustedTime().Format(time.RFC3339))
	}
}

// R2-5: una carpeta de datos copiada a otra computadora (Copied: la licencia se ignora y Accept responde
// ErrCopiedInstallation) acepta la misma respuesta de soporte de un solo uso que la computadora original, porque las dos
// tienen la misma identidad y la misma solicitud pendiente en estado.json. La §6.4 dice «solo en esa instalación»; lo
// coherente con Accept es que SupportRequest y AcceptSupportUnlock respondan ErrCopiedInstallation (y
// ErrMachineUnverified sin huella).
func TestReviewR2DefectCopiedFolderAcceptsTheSameSupportResponse(t *testing.T) {
	h, manager := blockedHarness(t)
	request, err := manager.SupportRequest(true)
	if err != nil {
		t.Fatal(err)
	}
	// La carpeta identity/licencia se copia a otra computadora antes de usar la respuesta.
	copied := newHarness(t)
	copied.clock = h.clock
	copied.machine = machineOther
	if err := os.MkdirAll(copied.directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{stateFileName, licenseFileName} {
		raw, err := os.ReadFile(filepath.Join(h.directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(copied.directory, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	other := copied.start()
	if !other.Status().Copied {
		t.Fatal("preparación: la copia debía detectarse")
	}
	private, trust := supportTestKey(t)
	response := signSupport(t, private, request.Code, h.clock.now(), h.clock.now())
	if _, err := manager.AcceptSupportUnlock(response, trust); err != nil {
		t.Fatal(err)
	}
	if _, err := other.AcceptSupportUnlock(response, trust); err == nil {
		t.Errorf("R2-5: la copia en otra computadora aceptó la misma respuesta de un solo uso (hora confiable %s)",
			other.TrustedTime().Format(time.RFC3339))
	}
}

// Límite conocido (§12 del contrato), comprobado: el piso de la §6.3 es el issued_at de la licencia vigente, y no hace
// falta un servidor falso para llegar a él. estado.json no está firmado y es de la cuenta que corre el servicio (en macOS,
// la del usuario que instaló, sin sudo): basta editar last_trusted_time y reiniciar. Al cargar, la licencia vuelve a subir
// la hora a su issued_at, que es el piso. Con una perpetua emitida hace meses, el piso es de hace meses.
func TestReviewR2LimitEditingTheStateLowersTrustedTimeToTheLicenseIssuedAt(t *testing.T) {
	h, _ := blockedHarness(t)
	issued := time.Date(2026, 11, 16, 6, 30, 0, 0, time.UTC)
	// El reloj se atrasa cuatro días más (el sistema sigue bloqueado) y se edita estado.json a mano.
	h.clock.set("2026-11-17T12:00:00Z")
	state := h.readState()
	edited := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	state.TrustedTime = &edited
	h.writeState(state)
	restarted := h.start()
	if !restarted.TrustedTime().Equal(issued) || restarted.Clock().Blocked {
		t.Fatalf("la hora confiable editada queda en el issued_at de la licencia: %s, bloqueado %v",
			restarted.TrustedTime().Format(time.RFC3339), restarted.Clock().Blocked)
	}
	// Más abajo del piso (issued_at − 10 min) sí se bloquea.
	h.clock.set("2026-11-16T06:19:00Z")
	if !restarted.Clock().Blocked {
		t.Fatal("por debajo del issued_at menos el margen debía bloquearse")
	}
}

// §6.4, intentos de engaño que se rechazan como «altered» sin cambiar nada ni gastar la solicitud (complementa
// TestSupportUnlock): el entorno de otra versión en una respuesta firmada con una llave de confianza, una licencia
// pegada en lugar de la respuesta, otro typ, un encabezado con espacios, relleno de base64, un campo desconocido y una
// hora anterior al origen del código. Después, la respuesta válida todavía se acepta una sola vez.
func TestReviewR2SupportUnlockRejectsForgeries(t *testing.T) {
	h, manager := blockedHarness(t)
	request, err := manager.SupportRequest(true)
	if err != nil {
		t.Fatal(err)
	}
	private, trust := supportTestKey(t)
	code, err := ParseSupportCode(request.Code)
	if err != nil {
		t.Fatal(err)
	}
	now := h.clock.now()
	content := func(changes map[string]any) []byte {
		t.Helper()
		fields := map[string]any{
			"schema_version": SupportSchemaVersion, "product_id": ProductID, "environment": BuildEnvironment,
			"installation_tag": code.InstallationTag, "request_nonce": code.Nonce, "request_code": code.Text,
			"new_trusted_time": now.Format(time.RFC3339), "issued_at": now.Format(time.RFC3339),
		}
		for key, value := range changes {
			fields[key] = value
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	sign := func(header string, body []byte) string {
		encodedHeader, encodedBody := encodeBase64URL([]byte(header)), encodeBase64URL(body)
		return encodedHeader + "." + encodedBody + "." +
			encodeBase64URL(ed25519.Sign(private, []byte(encodedHeader+"."+encodedBody)))
	}
	exactHeader := `{"alg":"EdDSA","kid":"prueba","typ":"adelpos-unlock+jws"}`
	valid := sign(exactHeader, content(nil))
	segments := strings.Split(valid, ".")
	otherEnvironment := "production"
	if BuildEnvironment == "production" {
		otherEnvironment = "development"
	}
	forgeries := map[string]string{
		"entorno de otra versión":     sign(exactHeader, content(map[string]any{"environment": otherEnvironment})),
		"una licencia de prueba":      h.jws("perpetual-0-months"),
		"typ de licencia":             sign(`{"alg":"EdDSA","kid":"prueba","typ":"adelpos-lic+jws"}`, content(nil)),
		"encabezado con espacios":     sign(`{"alg":"EdDSA", "kid":"prueba","typ":"adelpos-unlock+jws"}`, content(nil)),
		"firma con relleno":           segments[0] + "." + segments[1] + "." + segments[2] + "=",
		"campo desconocido":           sign(exactHeader, content(map[string]any{"valid_until": "2030-01-01T00:00:00Z"})),
		"hora anterior al origen":     sign(exactHeader, content(map[string]any{"new_trusted_time": "2025-12-31T00:00:00Z"})),
		"hora sin zona":               sign(exactHeader, content(map[string]any{"new_trusted_time": "2026-11-21T12:00:00"})),
		"otro número de la solicitud": sign(exactHeader, content(map[string]any{"request_nonce": "0102030405"})),
	}
	for name, forged := range forgeries {
		_, err := manager.AcceptSupportUnlock(forged, trust)
		var rejected *SupportUnlockError
		if !errors.As(err, &rejected) {
			t.Errorf("%s: debía rechazarse y resultó %v", name, err)
			continue
		}
		expected := SupportReasonAltered
		if name == "otro número de la solicitud" {
			expected = SupportReasonNotRequested
		}
		if rejected.Reason != expected {
			t.Errorf("%s: motivo %s, se esperaba %s", name, rejected.Reason, expected)
		}
	}
	if !manager.Clock().Blocked {
		t.Fatal("un rechazo no cambia nada")
	}
	if _, err := manager.AcceptSupportUnlock(valid, trust); err != nil {
		t.Fatalf("la respuesta válida se acepta después de los rechazos: %v", err)
	}
	if _, err := manager.AcceptSupportUnlock(valid, trust); err == nil {
		t.Fatal("una sola vez")
	}
}
