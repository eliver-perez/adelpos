package licensing_test

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensetest"
	"local/ad-el-pos/server/internal/licensing"
)

// Pruebas del cliente en línea contra el servidor de licencias simulado (internal/licensetest), con httptest y TLS en
// loopback y las llaves de vectors.json. Ninguna habla con el servidor de licencias real.

const vectorsFile = "../../../contracts/licencias/v1/fixtures/vectors.json"

// Códigos del servidor simulado (licensetest.Behaviors): el primer grupo decide qué hace la licencia.
const (
	perpetualKey   = "PRP-2222-2222-2222"
	monthlyKey     = "mns-3333-3333-3333" // en minúsculas: el cliente lo normaliza
	revokedKey     = "RVK-2222-2222-2222"
	elsewhereKey   = "ACT-2222-2222-2222"
	pendingKey     = "ACN-2222-2222-2222"
	unavailableKey = "TMP-2222-2222-2222"
	limitedKey     = "RAT-2222-2222-2222"
	proofKey       = "PRF-2222-2222-2222"
	unknownKey     = "ZZZ-2222-2222-2222"
)

type clientHarness struct {
	t        *testing.T
	fixtures licensetest.Fixtures
	server   *licensetest.Server
	url      string
	roots    *x509.CertPool
	mutex    sync.Mutex
	sleeps   []time.Duration
}

// station es una computadora principal de prueba: su gestor y su cliente.
type station struct {
	manager *licensing.Manager
	client  *licensing.Client
}

func newClientHarness(t *testing.T) *clientHarness {
	t.Helper()
	fixtures, err := licensetest.LoadFixtures(vectorsFile)
	if err != nil {
		t.Fatal(err)
	}
	server := licensetest.NewServer(fixtures)
	listener, roots := server.StartTLS()
	t.Cleanup(listener.Close)
	return &clientHarness{t: t, fixtures: fixtures, server: server, url: listener.URL, roots: roots}
}

// station arma una computadora con una de las instalaciones de los archivos de prueba.
func (h *clientHarness) station(name string) station {
	h.t.Helper()
	installation := h.fixtures.Installations[name]
	trust := h.fixtures.Trust()
	manager := licensing.NewManager(licensing.Options{
		Directory:    filepath.Join(h.t.TempDir(), "licencia"),
		Installation: &licensing.Installation{ServerID: installation.ServerID, PublicKey: installation.PublicKey},
		PrivateKey:   installation.PrivateKey,
		Trust:        &trust,
		MachineID: func() (string, error) {
			return "CCCCCCCC-1111-2222-3333-444444444444", nil
		},
	})
	if err := manager.Load(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	return station{manager: manager, client: h.client(manager, h.url)}
}

func (h *clientHarness) client(manager *licensing.Manager, url string) *licensing.Client {
	return licensing.NewClient(manager, licensing.ClientOptions{
		BaseURL:    url,
		AppVersion: "0.15.0",
		TestMode:   true,
		RootCAs:    h.roots,
		Sleep:      h.sleep,
	})
}

func (h *clientHarness) sleep(_ context.Context, wait time.Duration) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.sleeps = append(h.sleeps, wait)
	return nil
}

func (h *clientHarness) waits() []time.Duration {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return append([]time.Duration(nil), h.sleeps...)
}

func serverCode(t *testing.T, err error) *licensing.ServerError {
	t.Helper()
	var serverErr *licensing.ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("se esperaba un error del servidor de licencias y resultó %v", err)
	}
	return serverErr
}

// Activar, volver a activar desde la misma instalación, consultar y liberar (§8.3, §8.5 y §8.6).
func TestClientActivateRefreshDeactivate(t *testing.T) {
	h := newClientHarness(t)
	computer := h.station("installation-fixture-a")
	// La hora del servidor una hora adelante: su server_time alimenta la última hora confiable.
	h.server.SetOffset(time.Hour)
	status, err := computer.client.Activate(context.Background(), perpetualKey)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != licensing.StateActive || !status.Perpetual || status.Revision != 1 || status.Limits.OrderTakers != 3 {
		t.Fatalf("activada: %+v", status)
	}
	if computer.manager.TrustedTime().Before(time.Now().Add(50 * time.Minute)) {
		t.Fatal("el server_time de una respuesta correcta es hora confiable")
	}
	again, err := computer.client.Activate(context.Background(), perpetualKey)
	if err != nil || again.Revision != 1 || again.ActivationID != status.ActivationID {
		t.Fatalf("la misma instalación recibe su revisión vigente: %+v %v", again, err)
	}
	if refreshed, err := computer.client.Refresh(context.Background()); err != nil || refreshed.Revision != 1 {
		t.Fatalf("consultar sin cambios: %+v %v", refreshed, err)
	}
	// Una renovación en el panel llega como una revisión mayor.
	monthly := h.station("installation-fixture-b")
	if _, err := monthly.client.Activate(context.Background(), monthlyKey); err != nil {
		t.Fatal(err)
	}
	if err := h.server.Renew("MNS-3333-3333-3333", 1); err != nil {
		t.Fatal(err)
	}
	renewed, err := monthly.client.Refresh(context.Background())
	if err != nil || renewed.Revision != 2 || renewed.State != licensing.StateActive || renewed.LicenseType != "subscription" {
		t.Fatalf("la renovación llega al consultar: %+v %v", renewed, err)
	}
	closed, err := computer.client.Deactivate(context.Background())
	if err != nil || closed.State != licensing.StateRevoked || closed.RevocationReason != licensing.ReasonDeactivated || closed.Revision != 2 {
		t.Fatalf("liberada: %+v %v", closed, err)
	}
	// Liberada, se activa en otra computadora sin soporte.
	other := h.station("installation-fixture-b")
	if moved, err := other.client.Activate(context.Background(), perpetualKey); err != nil || moved.State != licensing.StateActive || moved.Revision != 3 {
		t.Fatalf("la otra computadora activa la licencia liberada: %+v %v", moved, err)
	}
}

// Cada código de error del contrato llega con su código; se decide por él, nunca por el HTTP ni el mensaje.
func TestClientErrorCodes(t *testing.T) {
	h := newClientHarness(t)
	computer := h.station("installation-fixture-a")
	ctx := context.Background()

	_, err := computer.client.Activate(ctx, unknownKey)
	if code := serverCode(t, err); code.Code != licensing.CodeLicenseNotFound || code.HTTPStatus != 404 {
		t.Fatalf("código inexistente: %+v", code)
	}
	_, err = computer.client.Activate(ctx, revokedKey)
	if serverCode(t, err).Code != licensing.CodeRevoked {
		t.Fatal("licencia revocada")
	}
	_, err = computer.client.Activate(ctx, proofKey)
	if serverCode(t, err).Code != licensing.CodeInvalidProof {
		t.Fatal("prueba de posesión rechazada")
	}
	if _, err := computer.client.Activate(ctx, "no es un código"); !errors.Is(err, licensing.ErrLicenseKeyFormat) {
		t.Fatalf("formato del código: %v", err)
	}
	// Los errores que el cliente no provoca por sí mismo se piden al servidor simulado.
	for _, code := range []string{licensing.CodeInvalidRequest, licensing.CodeIncompatibleSchema, licensing.CodeRequestConflict} {
		h.server.FailNext(licensetest.Scripted{Action: licensing.ActionActivate, Code: code})
		_, err := computer.client.Activate(ctx, perpetualKey)
		if serverCode(t, err).Code != code {
			t.Fatalf("se esperaba %s", code)
		}
	}
	// RATE_LIMITED con un Retry-After largo: no se espera dentro de la operación, se devuelve con los segundos.
	_, err = computer.client.Activate(ctx, limitedKey)
	limited := serverCode(t, err)
	if limited.Code != licensing.CodeRateLimited || limited.RetryAfter != 30*time.Second {
		t.Fatalf("RATE_LIMITED con Retry-After: %+v", limited)
	}
	for _, wait := range h.waits() {
		if wait >= 30*time.Second {
			t.Fatal("un Retry-After largo no se espera dentro de la operación")
		}
	}
	// TEMPORARY_UNAVAILABLE: se reintenta con el mismo cuerpo y, si sigue, se devuelve; la licencia no cambia.
	_, err = computer.client.Activate(ctx, unavailableKey)
	if serverCode(t, err).Code != licensing.CodeTemporaryUnavailable {
		t.Fatal("falla temporal")
	}
	activations := 0
	for _, call := range h.server.Calls() {
		if call.Action == licensing.ActionActivate && call.Code == licensing.CodeTemporaryUnavailable {
			activations++
		}
	}
	if activations != 3 {
		t.Fatalf("una falla temporal se reintenta hasta tres veces: %d", activations)
	}
	if status := computer.manager.Status(); status.HasLicense() {
		t.Fatalf("ningún error instala nada: %+v", status)
	}
}

// ACTIVATION_LIMIT con transfer_available true y false, la transferencia y TRANSFER_LIMIT; la consulta de la
// computadora anterior recibe su cierre y confirma la transferencia (§8.4).
func TestClientTransfer(t *testing.T) {
	h := newClientHarness(t)
	ctx := context.Background()
	original := h.station("installation-fixture-a")
	if _, err := original.client.Activate(ctx, perpetualKey); err != nil {
		t.Fatal(err)
	}
	newer := h.station("installation-fixture-b")
	_, err := newer.client.Activate(ctx, perpetualKey)
	limit := serverCode(t, err)
	if limit.Code != licensing.CodeActivationLimit || limit.TransferAvailable == nil || !*limit.TransferAvailable {
		t.Fatalf("activa en otra computadora, con transferencia disponible: %+v", limit)
	}
	moved, err := newer.client.Transfer(ctx, perpetualKey)
	if err != nil || moved.State != licensing.StateActive || moved.Revision != 3 {
		t.Fatalf("transferida: %+v %v", moved, err)
	}
	// Con una transferencia pendiente (la licencia ACN del servidor simulado), no se ofrece ni se acepta otra.
	_, err = h.station("installation-fixture-a").client.Activate(ctx, pendingKey)
	pendingLimit := serverCode(t, err)
	if pendingLimit.Code != licensing.CodeActivationLimit || pendingLimit.TransferAvailable == nil || *pendingLimit.TransferAvailable {
		t.Fatalf("con una transferencia pendiente, transfer_available false: %+v", pendingLimit)
	}
	_, err = h.station("installation-fixture-a").client.Transfer(ctx, pendingKey)
	if serverCode(t, err).Code != licensing.CodeTransferLimit {
		t.Fatal("con una transferencia pendiente, TRANSFER_LIMIT")
	}
	// La computadora anterior consulta: recibe su cierre transferred y confirma la transferencia.
	closed, err := original.client.Refresh(ctx)
	if err != nil || closed.State != licensing.StateRevoked || closed.RevocationReason != licensing.ReasonTransferred {
		t.Fatalf("la anterior recibe su cierre: %+v %v", closed, err)
	}
	for _, license := range h.server.Snapshot().Licenses {
		if license.LicenseKey == perpetualKey && license.PendingTransfer {
			t.Fatal("la consulta de la anterior confirma la transferencia")
		}
	}
	// Con el cierre, la anterior ya no consulta en segundo plano, y la nueva sigue activa.
	if status := newer.manager.Status(); status.State != licensing.StateActive {
		t.Fatalf("la nueva sigue activa: %+v", status)
	}
}

// Una respuesta perdida se reintenta con el mismo cuerpo y el mismo request_id, y recibe la misma respuesta; una
// falla temporal o un Retry-After corto se reintentan igual.
func TestClientRetriesWithTheSameBody(t *testing.T) {
	h := newClientHarness(t)
	ctx := context.Background()
	computer := h.station("installation-fixture-a")
	h.server.DropNext(licensing.ActionActivate)
	status, err := computer.client.Activate(ctx, perpetualKey)
	if err != nil || status.State != licensing.StateActive {
		t.Fatalf("la activación se recupera con el reintento: %+v %v", status, err)
	}
	var activations []licensetest.Call
	for _, call := range h.server.Calls() {
		if call.Action == licensing.ActionActivate {
			activations = append(activations, call)
		}
	}
	if len(activations) != 2 || activations[0].RequestID != activations[1].RequestID || !activations[1].Replayed {
		t.Fatalf("el reintento lleva el mismo request_id y recibe la respuesta guardada: %+v", activations)
	}
	// Retry-After corto en una consulta: se espera lo que pide y se reintenta.
	h.server.FailNext(licensetest.Scripted{Action: licensing.ActionRefresh, Code: licensing.CodeRateLimited, RetryAfter: 2})
	h.server.FailNext(licensetest.Scripted{Action: licensing.ActionRefresh, Code: licensing.CodeTemporaryUnavailable})
	if _, err := computer.client.Refresh(ctx); err != nil {
		t.Fatalf("la consulta se recupera: %v", err)
	}
	waits := h.waits()
	if len(waits) < 2 || waits[len(waits)-2] != 2*time.Second {
		t.Fatalf("se respeta Retry-After: %v", waits)
	}
	var refreshes []licensetest.Call
	for _, call := range h.server.Calls() {
		if call.Action == licensing.ActionRefresh {
			refreshes = append(refreshes, call)
		}
	}
	if len(refreshes) != 3 || refreshes[0].RequestID != refreshes[2].RequestID {
		t.Fatalf("los reintentos llevan el mismo request_id: %+v", refreshes)
	}
}

// Una licencia alterada en la respuesta se ignora (y su server_time no cuenta); TEMPORARY_UNAVAILABLE no afecta la
// licencia instalada.
func TestClientIgnoresAlteredResponsesAndTemporaryFailures(t *testing.T) {
	h := newClientHarness(t)
	ctx := context.Background()
	computer := h.station("installation-fixture-a")
	before, err := computer.client.Activate(ctx, perpetualKey)
	if err != nil {
		t.Fatal(err)
	}
	trusted := computer.manager.TrustedTime()
	h.server.SetOffset(10 * time.Hour)
	h.server.AlterNext()
	_, err = computer.client.Refresh(ctx)
	var rejected *licensing.ResponseRejectedError
	if !errors.As(err, &rejected) || rejected.Reason != licensing.ErrInvalidSignature {
		t.Fatalf("una licencia alterada se rechaza: %v", err)
	}
	if after := computer.manager.Status(); after.LicenseID != before.LicenseID || after.Revision != before.Revision || after.State != licensing.StateActive {
		t.Fatalf("sigue la licencia instalada: %+v", after)
	}
	if !computer.manager.TrustedTime().Equal(trusted) {
		t.Fatal("el server_time de una respuesta incorrecta no es hora confiable")
	}
	for attempt := 0; attempt < 3; attempt++ {
		h.server.FailNext(licensetest.Scripted{Action: licensing.ActionRefresh, Code: licensing.CodeTemporaryUnavailable})
	}
	if _, err := computer.client.Refresh(ctx); serverCode(t, err).Code != licensing.CodeTemporaryUnavailable {
		t.Fatal("falla temporal")
	}
	if after := computer.manager.Status(); after.State != licensing.StateActive || after.Revision != before.Revision {
		t.Fatalf("una falla temporal no cambia la licencia: %+v", after)
	}
	// Sin activación no hay consulta ni liberación.
	if _, err := h.station("installation-fixture-b").client.Refresh(ctx); !errors.Is(err, licensing.ErrNotActivated) {
		t.Fatalf("sin activación: %v", err)
	}
}

// Errores de red y de TLS: un *TransportError con su motivo, después de los reintentos.
func TestClientTransportFailures(t *testing.T) {
	h := newClientHarness(t)
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedURL := "https://" + listener.Addr().String()
	listener.Close()
	computer := h.station("installation-fixture-a")
	offline := h.client(computer.manager, closedURL)
	_, err = offline.Activate(ctx, perpetualKey)
	var transport *licensing.TransportError
	if !errors.As(err, &transport) || transport.Reason != licensing.ReasonNetwork {
		t.Fatalf("sin red: %v", err)
	}
	// Sin la autoridad del servidor simulado, el TLS no se verifica y no se envía nada.
	untrusted := licensing.NewClient(computer.manager, licensing.ClientOptions{BaseURL: h.url, TestMode: true, Sleep: h.sleep})
	_, err = untrusted.Activate(ctx, perpetualKey)
	if !errors.As(err, &transport) || transport.Reason != licensing.ReasonTLS {
		t.Fatalf("TLS no verificado: %v", err)
	}
	for _, call := range h.server.Calls() {
		if call.Action == licensing.ActionActivate {
			t.Fatal("sin TLS verificado no llega ninguna activación")
		}
	}
}

// La hora del servidor de licencias para la §6.3 (L1-D5): el desafío sin activación y refresh con ella.
func TestClientServerTime(t *testing.T) {
	h := newClientHarness(t)
	ctx := context.Background()
	h.server.SetOffset(-3 * time.Hour)
	computer := h.station("installation-fixture-a")
	fromChallenge, err := computer.client.ServerTime(ctx)
	if err != nil || fromChallenge.Source != licensing.ServerTimeFromChallenge {
		t.Fatalf("sin activación, el desafío: %+v %v", fromChallenge, err)
	}
	if difference := fromChallenge.Time.Sub(time.Now().Add(-3 * time.Hour)); difference > time.Minute || difference < -time.Minute {
		t.Fatalf("la hora del desafío es su expires_at menos 5 minutos: %v", fromChallenge.Time)
	}
	if !computer.manager.TrustedTime().IsZero() {
		t.Fatal("la hora del desafío no es hora confiable (L1-D26)")
	}
	if _, err := computer.client.Activate(ctx, perpetualKey); err != nil {
		t.Fatal(err)
	}
	fromRefresh, err := computer.client.ServerTime(ctx)
	if err != nil || fromRefresh.Source != licensing.ServerTimeFromRefresh {
		t.Fatalf("con activación, refresh: %+v %v", fromRefresh, err)
	}
	if difference := fromRefresh.Time.Sub(time.Now().Add(-3 * time.Hour)); difference > time.Minute || difference < -time.Minute {
		t.Fatalf("la hora de refresh es su server_time: %v", fromRefresh.Time)
	}
}

// La consulta en segundo plano corre sola con una activación, y no hace nada sin ella.
func TestClientBackgroundRefresh(t *testing.T) {
	h := newClientHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idle := h.station("installation-fixture-b")
	computer := h.station("installation-fixture-a")
	if _, err := computer.client.Activate(context.Background(), perpetualKey); err != nil {
		t.Fatal(err)
	}
	schedule := licensing.Schedule{
		First:   func() time.Duration { return time.Millisecond },
		Next:    func() time.Duration { return 10 * time.Millisecond },
		Failure: 10 * time.Millisecond,
	}
	go idle.client.RunBackground(ctx, schedule)
	go computer.client.RunBackground(ctx, schedule)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		refreshes := 0
		for _, call := range h.server.Calls() {
			if call.Action == licensing.ActionRefresh {
				refreshes++
			}
		}
		if refreshes >= 2 {
			cancel()
			for _, call := range h.server.Calls() {
				if call.Action == "challenge" && call.Code != "OK" {
					t.Fatalf("la consulta en segundo plano falló: %+v", call)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la consulta en segundo plano no corrió")
}

// En test_mode el cliente solo habla con loopback (L1-D18): la dirección del entorno compilado se rechaza sin
// conectarse a nada, y también una IP de otra red.
func TestClientRefusesRemoteServersInTestMode(t *testing.T) {
	h := newClientHarness(t)
	computer := h.station("installation-fixture-a")
	for _, base := range []string{"", licensing.DefaultServerURL, "https://192.0.2.10"} {
		client := licensing.NewClient(computer.manager, licensing.ClientOptions{BaseURL: base, TestMode: true, Sleep: h.sleep})
		_, err := client.Activate(context.Background(), perpetualKey)
		var transport *licensing.TransportError
		if !errors.As(err, &transport) || transport.Reason != licensing.ReasonRemoteNotAllowed {
			t.Fatalf("%q en test_mode: %v", base, err)
		}
		if _, err := client.ServerTime(context.Background()); !errors.As(err, &transport) {
			t.Fatalf("%q en test_mode, la hora: %v", base, err)
		}
	}
	// Una dirección que no es https nunca se usa.
	plain := licensing.NewClient(computer.manager, licensing.ClientOptions{BaseURL: "http://127.0.0.1:1", TestMode: true, Sleep: h.sleep})
	_, err := plain.Activate(context.Background(), perpetualKey)
	var transport *licensing.TransportError
	if !errors.As(err, &transport) || transport.Reason != licensing.ReasonNotConfigured {
		t.Fatalf("http sin TLS: %v", err)
	}
}
