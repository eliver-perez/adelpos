package licensing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// La solicitud sin internet que arma AD-EL POS con los datos de un caso del contrato es idéntica, byte por byte, a la
// publicada (Ed25519 es determinista), y pasa la verificación de referencia.
func TestOfflineRequestsMatchTheContractFixtures(t *testing.T) {
	loaded := loadFixtures(t)
	for _, item := range loaded.set.Requests {
		payload := offlinePayload{
			Action:                item.Payload["action"],
			RequestID:             item.Payload["request_id"],
			CreatedAt:             item.Payload["created_at"],
			ProductID:             item.Payload["product_id"],
			ServerID:              item.Payload["server_id"],
			InstallationPublicKey: item.Payload["installation_public_key"],
			AppVersion:            item.Payload["app_version"],
			LicenseKey:            item.Payload["license_key"],
			LicenseID:             item.Payload["license_id"],
			ActivationID:          item.Payload["activation_id"],
		}
		envelope, err := buildOfflineRequest(payload, loaded.privateKeys[item.Installation])
		if err != nil {
			t.Fatalf("%s: %v", item.Action, err)
		}
		if envelope != item.EnvelopeJSON {
			t.Fatalf("%s: la solicitud de AD-EL POS no es la publicada", item.Action)
		}
	}
}

// Manager.OfflineRequest: activate con o sin código, renew y deactivate con la activación instalada; cada una
// verificada con la referencia y con exactamente los campos de la §9.1.
func TestManagerOfflineRequests(t *testing.T) {
	h := newHarness(t)
	installation := h.fixtures.installations["installation-fixture-a"]
	manager := NewManager(Options{
		Directory:    h.directory,
		Installation: &installation,
		PrivateKey:   h.fixtures.privateKeys["installation-fixture-a"],
		Trust:        h.fixtures.testTrust(),
		Now:          h.clock.now,
		MachineID: func() (string, error) {
			return machineOriginal, nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	withKey, err := manager.OfflineRequest(OfflineActivate, " k7m-9r3x-6tvp-8chd ", "0.15.0")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := VerifyOfflineRequest([]byte(withKey.Content))
	if err != nil || payload["license_key"] != "K7M-9R3X-6TVP-8CHD" || payload["server_id"] != installation.ServerID || len(payload) != 8 {
		t.Fatalf("activate con código, normalizado: %v %v", payload, err)
	}
	if !strings.HasPrefix(withKey.Filename, "solicitud-activar-") || !strings.HasSuffix(withKey.Filename, ".licreq") || strings.HasSuffix(withKey.Content, "\n") {
		t.Fatalf("nombre y contenido del archivo: %s", withKey.Filename)
	}
	if created, err := time.Parse(time.RFC3339Nano, payload["created_at"]); err != nil || !created.Equal(h.clock.now()) {
		t.Fatal("created_at es la hora local, informativa")
	}
	withoutKey, err := manager.OfflineRequest(OfflineActivate, "", "0.15.0")
	if err != nil {
		t.Fatal(err)
	}
	if payload, err := VerifyOfflineRequest([]byte(withoutKey.Content)); err != nil || len(payload) != 7 || withoutKey.RequestID == withKey.RequestID {
		t.Fatalf("activate sin código, otra request_id: %v %v", payload, err)
	}
	if _, err := manager.OfflineRequest(OfflineActivate, "mal", "0.15.0"); !errors.Is(err, ErrLicenseKeyFormat) {
		t.Fatal("un código mal escrito se rechaza")
	}
	for _, action := range []string{OfflineRenew, OfflineDeactivate} {
		if _, err := manager.OfflineRequest(action, "", "0.15.0"); !errors.Is(err, ErrNotActivated) {
			t.Fatalf("%s sin activación: %v", action, err)
		}
	}
	h.accept(manager, "subscription-monthly")
	for _, action := range []string{OfflineRenew, OfflineDeactivate} {
		request, err := manager.OfflineRequest(action, "", "0.15.0")
		if err != nil {
			t.Fatal(err)
		}
		payload, err := VerifyOfflineRequest([]byte(request.Content))
		if err != nil || payload["action"] != action || payload["license_id"] != "11111111-1111-4111-8111-111111111111" ||
			payload["activation_id"] != "33333333-3333-4333-8333-333333333333" || len(payload) != 9 {
			t.Fatalf("%s: %v %v", action, payload, err)
		}
		if _, err := manager.OfflineRequest(action, "K7M-9R3X-6TVP-8CHD", "0.15.0"); err == nil {
			t.Fatalf("%s no admite el código", action)
		}
	}
	if _, err := manager.OfflineRequest("transfer", "", "0.15.0"); err == nil {
		t.Fatal("transfer no es una acción sin internet")
	}
}

// Importar un .lic es aceptar el mismo JWS con la §10: con los espacios y saltos de línea que agrega un editor, y
// uno alterado o de otra instalación se ignora.
func TestImportLicenseFile(t *testing.T) {
	h := newHarness(t)
	manager := h.start()
	status, err := manager.ImportLicense("\ufeff" + h.jws("subscription-monthly") + "\r\n")
	if err != nil || status.State != StateActive || h.licenseFile() != h.jws("subscription-monthly") {
		t.Fatalf("importado: %+v %v", status, err)
	}
	if _, err := manager.ImportLicense(h.negative("altered-signature")); codeOf(err) != ErrInvalidSignature {
		t.Fatalf("alterado: %v", err)
	}
	if _, err := manager.ImportLicense(h.jws("self-service-transfer-new")); codeOf(err) != ErrOtherInstallation {
		t.Fatalf("de otra instalación: %v", err)
	}
	if _, err := manager.ImportLicense(strings.Repeat("a", maxLicenseFile+1)); codeOf(err) != ErrMalformed {
		t.Fatal("un archivo demasiado grande se rechaza")
	}
	if after := manager.Status(); after.LicenseID != status.LicenseID || after.State != StateActive {
		t.Fatalf("sigue la importada: %+v", after)
	}
}

// El formato del código comercial (L1-D4).
func TestNormalizeLicenseKey(t *testing.T) {
	valid := map[string]string{
		"K7M-9R3X-6TVP-8CHD":     "K7M-9R3X-6TVP-8CHD",
		"k7m9r3x6tvp8chd":        "K7M-9R3X-6TVP-8CHD",
		"  k7m-9r3x-6tvp-8chd  ": "K7M-9R3X-6TVP-8CHD",
	}
	for input, expected := range valid {
		if normalized, err := NormalizeLicenseKey(input); err != nil || normalized != expected {
			t.Fatalf("%q: %q %v", input, normalized, err)
		}
	}
	for _, input := range []string{"", "K7M-9R3X-6TVP", "K7M-9R3X-6TVP-8CHDX", "K7M9-R3X6-TVP8-CHD", "K7M-9R3X-6TVP-8CH0", "K7M-9R3X-6TVP-8CHI", "K7M--9R3X6TVP8CHD"} {
		if _, err := NormalizeLicenseKey(input); !errors.Is(err, ErrLicenseKeyFormat) {
			t.Fatalf("%q debe rechazarse", input)
		}
	}
}

// La conexión del cliente en test_mode se niega a cualquier dirección que no sea de loopback, aunque un nombre la
// resuelva así (L1-D18).
func TestTestModeDialRefusesRemoteAddresses(t *testing.T) {
	for _, address := range []string{"192.0.2.10:443", "[2001:db8::1]:443", "10.0.0.5:443", "sin-puerto"} {
		if err := loopbackOnly("tcp", address, nil); !errors.Is(err, errRemoteNotAllowed) {
			t.Fatalf("%s debe rechazarse: %v", address, err)
		}
	}
	for _, address := range []string{"127.0.0.1:8471", "[::1]:8471"} {
		if err := loopbackOnly("tcp", address, nil); err != nil {
			t.Fatalf("%s es loopback: %v", address, err)
		}
	}
	if newHTTPClient(ClientOptions{TestMode: true}).Transport.(*http.Transport).Proxy != nil {
		t.Fatal("el cliente no usa proxy")
	}
}
