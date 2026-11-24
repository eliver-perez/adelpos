package licensing

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// supportTestKey es una llave de soporte de prueba (nunca la de ~/.adel-pos-dev/soporte).
func supportTestKey(t *testing.T) (ed25519.PrivateKey, SupportTrust) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return private, SupportTrust{Environment: BuildEnvironment, Keys: map[string]ed25519.PublicKey{"prueba": public}}
}

func signSupport(t *testing.T, private ed25519.PrivateKey, codeText string, newTrusted, issued time.Time) string {
	t.Helper()
	code, err := ParseSupportCode(codeText)
	if err != nil {
		t.Fatal(err)
	}
	response, err := SignSupportResponse(private, "prueba", BuildEnvironment, code, newTrusted, issued)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func requireSupportReason(t *testing.T, err error, reason string) {
	t.Helper()
	var rejected *SupportUnlockError
	if !errors.As(err, &rejected) || rejected.Reason != reason {
		t.Fatalf("se esperaba el rechazo %s y resultó %v", reason, err)
	}
}

// El código de solicitud: 24 caracteres en seis grupos, legible al dictarlo (minúsculas, O por 0, I y L por 1), con la
// etiqueta de la instalación, el número y la última hora confiable, y una verificación que detecta un error al dictar.
func TestSupportCodeRoundTrip(t *testing.T) {
	tag := SupportInstallationTag("22222222-2222-4222-8222-222222222222")
	trusted := time.Date(2026, 11, 23, 18, 40, 30, 0, time.UTC)
	code, err := EncodeSupportCode(tag, "0a1b2c3d4e", trusted)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 29 || strings.Count(code, "-") != 5 || strings.ContainsAny(code, "ILOU") {
		t.Fatalf("formato del código: %q", code)
	}
	parsed, err := ParseSupportCode(code)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != code || parsed.InstallationTag != tag || parsed.Nonce != "0a1b2c3d4e" ||
		!parsed.TrustedTime.Equal(trusted.Truncate(time.Minute)) {
		t.Fatalf("código leído: %+v", parsed)
	}
	// Dictado: minúsculas, espacios y las letras que se confunden.
	dictated := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	dictated = strings.ReplaceAll(dictated, "0", "o")
	dictated = strings.ReplaceAll(dictated, "1", "l")
	again, err := ParseSupportCode(dictated)
	if err != nil || again.Text != code {
		t.Fatalf("dictado %q: %+v %v", dictated, again, err)
	}
	// Un carácter mal dictado casi siempre se detecta: la verificación es de 12 bits (1 en 4096 pasaría). Se prueban
	// las 744 variantes de un carácter de este código.
	accepted, variants := 0, 0
	for position := range len(code) {
		if code[position] == '-' {
			continue
		}
		for _, replacement := range supportAlphabet {
			if byte(replacement) == code[position] {
				continue
			}
			variants++
			if _, err := ParseSupportCode(code[:position] + string(replacement) + code[position+1:]); err == nil {
				accepted++
			}
		}
	}
	if variants != 24*31 || accepted > 2 {
		t.Fatalf("de %d variantes con un carácter cambiado se aceptaron %d", variants, accepted)
	}
	for _, invalid := range []string{"", "ABCD", code + "0", strings.Replace(code, code[:1], "U", 1)} {
		if _, err := ParseSupportCode(invalid); err == nil {
			t.Errorf("el código %q debía rechazarse", invalid)
		}
	}
}

// §6.4 completa en el gestor: válido, repetido, de otra instalación, vencido, alterado y sin solicitud.
func TestSupportUnlock(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	h.accept(manager, "perpetual-0-months")
	// La computadora registró actividad con la fecha en el futuro y después se corrigió el reloj.
	h.clock.set("2026-11-25T12:00:00Z")
	manager.ObserveTime(h.clock.now(), TimeFromOperation)
	h.clock.set("2026-11-21T12:00:00Z")
	if !manager.Clock().Blocked {
		t.Fatal("el reloj corregido queda antes de la última actividad")
	}
	private, trust := supportTestKey(t)

	// Sin solicitud no hay nada que desbloquear.
	_, other := supportTestKey(t)
	request, err := manager.SupportRequest(false)
	if err != nil {
		t.Fatal(err)
	}
	if request.Reused || !request.ExpiresAt.Equal(request.CreatedAt.Add(24*time.Hour)) ||
		!request.TrustedTime.Equal(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("solicitud: %+v", request)
	}
	// La misma solicitud se vuelve a entregar mientras sigue vigente; con renew se crea otra.
	again, err := manager.SupportRequest(false)
	if err != nil || again.Code != request.Code || !again.Reused {
		t.Fatalf("la solicitud pendiente se reutiliza: %+v %v", again, err)
	}
	replaced, err := manager.SupportRequest(true)
	if err != nil || replaced.Code == request.Code {
		t.Fatalf("renew crea otra solicitud: %+v %v", replaced, err)
	}
	stale := signSupport(t, private, request.Code, h.clock.now(), h.clock.now())
	_, err = manager.AcceptSupportUnlock(stale, trust)
	requireSupportReason(t, err, SupportReasonNotRequested)

	// Alterada: firma de otra llave, contenido cambiado y texto cortado.
	response := signSupport(t, private, replaced.Code, h.clock.now(), h.clock.now())
	_, err = manager.AcceptSupportUnlock(response, other)
	requireSupportReason(t, err, SupportReasonAltered)
	segments := strings.Split(response, ".")
	var content map[string]any
	raw, _ := decodeBase64URL(segments[1])
	_ = json.Unmarshal(raw, &content)
	content["new_trusted_time"] = "2020-01-01T00:00:00Z"
	tampered, _ := json.Marshal(content)
	_, err = manager.AcceptSupportUnlock(segments[0]+"."+encodeBase64URL(tampered)+"."+segments[2], trust)
	requireSupportReason(t, err, SupportReasonAltered)
	_, err = manager.AcceptSupportUnlock(response[:len(response)-10], trust)
	requireSupportReason(t, err, SupportReasonAltered)

	// De otra instalación: el código de otro server_id con un número cualquiera.
	foreignCode, _ := EncodeSupportCode(SupportInstallationTag("66666666-6666-4666-8666-666666666666"), "0102030405", time.Time{})
	_, err = manager.AcceptSupportUnlock(signSupport(t, private, foreignCode, h.clock.now(), h.clock.now()), trust)
	requireSupportReason(t, err, SupportReasonOtherInstallation)
	if !manager.Clock().Blocked {
		t.Fatal("un rechazo no cambia nada")
	}

	// Válida, pegada con saltos de línea como llega por un mensaje.
	pasted := "  " + response[:40] + "\n" + response[40:] + "\n"
	result, err := manager.AcceptSupportUnlock(pasted, trust)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NewTrustedTime.Equal(h.clock.now()) || !result.PreviousTrustedTime.Equal(time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)) ||
		result.RequestCode != replaced.Code {
		t.Fatalf("resultado: %+v", result)
	}
	if manager.Clock().Blocked || !manager.TrustedTime().Equal(h.clock.now()) {
		t.Fatal("el desbloqueo baja la última hora confiable y quita el bloqueo")
	}
	if h.readState().TrustedSource != string(ReplaceBySupportUnlock) {
		t.Fatal("la fuente de la hora queda guardada")
	}

	// Repetida: una sola vez, también después de reiniciar.
	_, err = manager.AcceptSupportUnlock(response, trust)
	requireSupportReason(t, err, SupportReasonUsed)
	restarted := h.start()
	_, err = restarted.AcceptSupportUnlock(response, trust)
	requireSupportReason(t, err, SupportReasonUsed)

	// Vencida: 24 horas medidas con el reloj local respecto al de la solicitud.
	late, err := restarted.SupportRequest(false)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.set("2026-11-22T12:00:00Z")
	_, err = restarted.AcceptSupportUnlock(signSupport(t, private, late.Code, h.clock.now(), h.clock.now()), trust)
	requireSupportReason(t, err, SupportReasonExpired)
	// Si el reloj se corrigió hacia atrás después de pedirla, también hay que pedir otra.
	backwards, _ := restarted.SupportRequest(true)
	h.clock.set("2026-11-22T10:00:00Z")
	_, err = restarted.AcceptSupportUnlock(signSupport(t, private, backwards.Code, h.clock.now(), h.clock.now()), trust)
	requireSupportReason(t, err, SupportReasonExpired)
}

// La solicitud pendiente se guarda en estado.json y sobrevive a un reinicio; sin identidad no hay solicitud.
func TestSupportRequestIsPersisted(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	request, err := manager.SupportRequest(false)
	if err != nil {
		t.Fatal(err)
	}
	state := h.readState()
	if state.Support == nil || state.Support.Pending == nil || state.Support.Pending.Code != request.Code {
		t.Fatalf("estado guardado: %+v", state.Support)
	}
	restarted := h.start()
	again, err := restarted.SupportRequest(false)
	if err != nil || again.Code != request.Code || !again.Reused {
		t.Fatalf("después de reiniciar: %+v %v", again, err)
	}
	withoutIdentity := NewManager(Options{Directory: filepath.Join(t.TempDir(), "licencia"), Now: h.clock.now,
		MachineID: func() (string, error) { return machineOriginal, nil }})
	if _, err := withoutIdentity.SupportRequest(false); !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("sin identidad: %v", err)
	}
}

// La llave pública de soporte de desarrollo está en la versión (sincronizada de config/licencias) y es válida; un
// archivo de llaves de otro entorno, vacío o con un kid repetido se rechaza.
func TestCompiledSupportTrust(t *testing.T) {
	trust, err := CompiledSupportTrust()
	if err != nil || len(trust.Keys) == 0 || trust.Environment != BuildEnvironment {
		t.Fatalf("llaves de soporte compiladas: %+v %v", trust, err)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "licencias", "soporte-"+BuildEnvironment+".pub.json"))
	if err != nil {
		t.Fatal(err)
	}
	fromConfig, err := ParseSupportKeys(source, BuildEnvironment)
	if err != nil || len(fromConfig.Keys) != len(trust.Keys) {
		t.Fatal("la copia embebida no coincide con config/licencias")
	}
	public := encodeBase64URL(make([]byte, 32))
	for _, invalid := range []string{
		`{"environment":"production","keys":[{"kid":"a","public_key":"` + public + `"}]}`,
		`{"environment":"` + BuildEnvironment + `","keys":[]}`,
		`{"environment":"` + BuildEnvironment + `","keys":[{"kid":"a","public_key":"` + public + `"},{"kid":"a","public_key":"` + public + `"}]}`,
		`{"environment":"` + BuildEnvironment + `","keys":[{"kid":"a","public_key":"corta"}]}`,
	} {
		if _, err := ParseSupportKeys([]byte(invalid), BuildEnvironment); err == nil {
			t.Errorf("se debía rechazar %s", invalid)
		}
	}
}
