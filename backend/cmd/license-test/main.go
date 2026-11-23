//go:build !adelpos_production

// Orden **solo de pruebas** de las licencias (etapa L1, Fase B), para Playwright (Fase E) y Android (Fase F). Es un
// programa aparte del servidor: los instaladores solo compilan ./cmd/ad-el-pos-server, así que nunca llega a un
// instalador, y una compilación de producción no la compila (etiqueta). guard_test.go lo comprueba. Exige
// ADEL_TEST_MODE=1 y nunca llama al servidor de licencias real: firma con las llaves públicas de vectors.json.
//
//	license-test issue  [opciones]   emite un .lic de prueba para una instalación
//	license-test server [opciones]   servidor de licencias simulado, con TLS, en loopback
//
// La guía de uso está en el resumen de la Fase B del avance (docs/ETAPA_L1_AVANCE.md).
package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"local/ad-el-pos/server/internal/licensetest"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, nil))
}

// result es la única línea JSON que escribe cada orden (más la de «ready» del servidor).
type result struct {
	Event     string `json:"event"`
	Output    string `json:"output,omitempty"`
	LicenseID string `json:"license_id,omitempty"`
	Revision  int64  `json:"license_revision,omitempty"`
	URL       string `json:"url,omitempty"`
	CA        string `json:"ca,omitempty"`
	Message   string `json:"message,omitempty"`
}

func emit(stdout io.Writer, value result) {
	encoded, _ := json.Marshal(value)
	_, _ = stdout.Write(append(encoded, '\n'))
}

func failed(stdout io.Writer, code int, message string) int {
	emit(stdout, result{Event: "error", Message: message})
	return code
}

// run atiende la orden. ready, si no es nil, recibe la dirección del servidor simulado cuando ya atiende (pruebas).
func run(ctx context.Context, arguments []string, stdout io.Writer, ready chan<- string) int {
	if os.Getenv("ADEL_TEST_MODE") != "1" {
		return failed(stdout, 2, "license-test solo corre en modo de pruebas (ADEL_TEST_MODE=1).")
	}
	if len(arguments) == 0 {
		return failed(stdout, 2, "Uso: license-test issue|server [opciones]")
	}
	switch arguments[0] {
	case "issue":
		return runIssue(arguments[1:], stdout)
	case "server":
		return runServer(ctx, arguments[1:], stdout, ready)
	}
	return failed(stdout, 2, "Orden desconocida: usa issue o server.")
}

// fixturesPath es --fixtures o, sin él, ADEL_TEST_LICENSE_TRUST (la misma ruta que recibe el Go temporal).
func fixturesPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("ADEL_TEST_LICENSE_TRUST")
}

func runIssue(arguments []string, stdout io.Writer) int {
	flags := flag.NewFlagSet("issue", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fixtures := flags.String("fixtures", "", "vectors.json del contrato (por omisión, ADEL_TEST_LICENSE_TRUST)")
	identity := flags.String("identity", "", "carpeta identity de la instalación destinataria")
	serverID := flags.String("server-id", "", "server_id de la instalación (sin --identity)")
	publicKey := flags.String("public-key", "", "llave pública de licencias en base64url (sin --identity)")
	output := flags.String("output", "", "archivo .lic que se crea (sin él, el .lic va en «output» de la línea JSON)")
	kid := flags.String("kid", licensetest.DefaultKid, "llave de firma de prueba")
	licenseType := flags.String("type", licensetest.TypePerpetual, "perpetual, subscription o trial")
	billing := flags.String("billing-period", "", "monthly, quarterly, semiannual o annual (suscripción)")
	issuedAt := flags.String("issued-at", "", "issued_at: RFC 3339 o desfase desde ahora (-2d, 36h)")
	startsAt := flags.String("period-starts-at", "", "period_starts_at: RFC 3339 o desfase")
	expiresAt := flags.String("expires-at", "", "expires_at: RFC 3339 o desfase (suscripción y prueba)")
	updatesUntil := flags.String("updates-until", "", "updates_until: RFC 3339 o desfase")
	grace := flags.Int("grace-days", -1, "grace_days (por omisión, el del tipo: 5 en suscripción, 0 en los demás)")
	plan := flags.String("plan", "elemental", "basico, elemental o premium")
	cash := flags.Int("cash-registers", -1, "cupos de caja (por omisión, los del plan)")
	takers := flags.Int("order-takers", -1, "cupos de comandero (por omisión, los del plan)")
	kitchens := flags.Int("kitchen-displays", -1, "cupos de pantalla de cocina (por omisión, los del plan)")
	status := flags.String("status", "active", "active o revoked")
	reason := flags.String("reason", "", "status_reason de una revocada: deactivated, transferred o revoked")
	revision := flags.Int64("revision", 1, "license_revision")
	licenseID := flags.String("license-id", "", "license_id (por omisión, uno nuevo)")
	activationID := flags.String("activation-id", "", "activation_id (por omisión, uno nuevo)")
	licensee := flags.String("licensee", "", "licensee_name")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return failed(stdout, 2, "Opciones inválidas de license-test issue.")
	}
	loaded, err := licensetest.LoadFixtures(fixturesPath(*fixtures))
	if err != nil {
		return failed(stdout, 1, err.Error())
	}
	options := licensetest.IssueOptions{
		Kid: *kid, ServerID: *serverID, PublicKey: *publicKey, Type: *licenseType, BillingPeriod: *billing,
		PlanID: *plan, Status: *status, Reason: *reason, Revision: *revision, LicenseID: *licenseID,
		ActivationID: *activationID, LicenseeName: *licensee,
	}
	if *identity != "" {
		loadedIdentity, err := localsetup.LoadIdentity(*identity)
		if err != nil {
			return failed(stdout, 1, "No se pudo leer la identidad de "+*identity+".")
		}
		key, err := localsetup.LoadLicenseKey(*identity)
		if err != nil {
			return failed(stdout, 1, "No se pudo leer la llave de licencias de "+*identity+" (arranca antes el servidor).")
		}
		options.ServerID, options.PublicKey = loadedIdentity.ServerID, key.PublicKey
	}
	now := time.Now()
	for _, field := range []struct {
		text   string
		target *time.Time
	}{
		{*issuedAt, &options.IssuedAt}, {*startsAt, &options.PeriodStartsAt}, {*expiresAt, &options.ExpiresAt},
		{*updatesUntil, &options.UpdatesUntil},
	} {
		if field.text == "" {
			continue
		}
		value, err := parseWhen(field.text, now)
		if err != nil {
			return failed(stdout, 2, err.Error())
		}
		*field.target = value
	}
	if *grace >= 0 {
		options.GraceDays = grace
	}
	if *cash >= 0 || *takers >= 0 || *kitchens >= 0 {
		limits, err := customLimits(*plan, *cash, *takers, *kitchens)
		if err != nil {
			return failed(stdout, 2, err.Error())
		}
		options.Limits = &limits
	}
	jws, err := loaded.Issue(options)
	if err != nil {
		return failed(stdout, 1, err.Error())
	}
	licenseIDOut, revisionOut := describe(jws)
	if *output == "" {
		emit(stdout, result{Event: "issued", Output: jws, LicenseID: licenseIDOut, Revision: revisionOut})
		return 0
	}
	if err := os.WriteFile(*output, []byte(jws), 0600); err != nil {
		return failed(stdout, 1, "No se pudo escribir "+*output+".")
	}
	emit(stdout, result{Event: "issued", Output: *output, LicenseID: licenseIDOut, Revision: revisionOut})
	return 0
}

// customLimits completa con los del plan los cupos que no se dieron.
func customLimits(plan string, cash, takers, kitchens int) (licensing.Limits, error) {
	defaults := map[string]licensing.Limits{
		"basico":    {CashRegisters: 1},
		"elemental": {CashRegisters: 3, OrderTakers: 3, KitchenDisplays: 3},
		"premium":   {CashRegisters: 5, OrderTakers: 10, KitchenDisplays: 5},
	}
	limits, ok := defaults[plan]
	if !ok {
		return licensing.Limits{}, errors.New("plan desconocido: usa basico, elemental o premium")
	}
	if cash >= 0 {
		limits.CashRegisters = cash
	}
	if takers >= 0 {
		limits.OrderTakers = takers
	}
	if kitchens >= 0 {
		limits.KitchenDisplays = kitchens
	}
	return limits, nil
}

// parseWhen acepta un instante RFC 3339 o un desfase desde ahora: una duración de Go («36h», «-90m») o días («-2d»).
func parseWhen(text string, now time.Time) (time.Time, error) {
	if instant, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return instant.UTC(), nil
	}
	if strings.HasSuffix(text, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(text, "d"))
		if err == nil {
			return now.Add(time.Duration(days) * 24 * time.Hour).UTC(), nil
		}
	}
	if offset, err := time.ParseDuration(text); err == nil {
		return now.Add(offset).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("fecha inválida %q: usa RFC 3339 o un desfase como -2d o 36h", text)
}

// describe lee license_id y license_revision del contenido (para la línea JSON).
func describe(jws string) (string, int64) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return "", 0
	}
	raw, err := decodeSegment(parts[1])
	if err != nil {
		return "", 0
	}
	var content struct {
		LicenseID string `json:"license_id"`
		Revision  int64  `json:"license_revision"`
	}
	_ = json.Unmarshal(raw, &content)
	return content.LicenseID, content.Revision
}

func runServer(ctx context.Context, arguments []string, stdout io.Writer, ready chan<- string) int {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	fixtures := flags.String("fixtures", "", "vectors.json del contrato (por omisión, ADEL_TEST_LICENSE_TRUST)")
	listen := flags.String("listen", "127.0.0.1:0", "dirección de loopback con su puerto")
	caOut := flags.String("ca-out", "", "archivo PEM con el certificado del servidor (ADEL_TEST_LICENSE_CA del Go temporal)")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *caOut == "" {
		return failed(stdout, 2, "Uso: license-test server --listen 127.0.0.1:<puerto> --ca-out <certificado.pem> [--fixtures vectors.json]")
	}
	host, _, err := net.SplitHostPort(*listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		return failed(stdout, 2, "El servidor de licencias simulado solo escucha en loopback (127.0.0.1 o ::1).")
	}
	loaded, err := licensetest.LoadFixtures(fixturesPath(*fixtures))
	if err != nil {
		return failed(stdout, 1, err.Error())
	}
	certificate, certificatePEM, err := licensetest.SelfSignedCertificate()
	if err != nil {
		return failed(stdout, 1, "No se pudo crear el certificado del servidor simulado.")
	}
	if err := os.WriteFile(*caOut, certificatePEM, 0600); err != nil {
		return failed(stdout, 1, "No se pudo escribir "+*caOut+".")
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return failed(stdout, 1, "No se pudo abrir "+*listen+".")
	}
	server := &http.Server{
		Handler:           licensetest.NewServer(loaded),
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}},
	}
	done := make(chan error, 1)
	go func() {
		done <- server.ServeTLS(listener, "", "")
	}()
	url := "https://" + listener.Addr().String()
	emit(stdout, result{Event: "ready", URL: url, CA: *caOut})
	if ready != nil {
		ready <- url
	}
	select {
	case <-ctx.Done():
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return failed(stdout, 1, "El servidor simulado se detuvo.")
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	emit(stdout, result{Event: "stopped"})
	return 0
}

func decodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}
