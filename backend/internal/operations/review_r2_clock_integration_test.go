//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/licensing/licensingtest"
	"local/ad-el-pos/server/internal/payments"
)

// Revisión independiente R2 de la etapa L1 (la protección de la hora). Las pruebas «…Defect…» fallan hasta que se
// corrija su hallazgo (ver $SP/l1/doc-R2.md).
//
// Hallazgo R2-1: «Comprobar la hora con internet» (§6.3) y el desbloqueo de soporte (§6.4) bajan la última hora
// confiable en identity/licencia/estado.json (ReplaceTrustedTime y AcceptSupportUnlock escriben el archivo de inmediato)
// y solo después escriben su auditoría dentro de la transacción de la petición. Si la petición falla después de la
// escritura del archivo (aquí: la inserción en audit_events espera un lock y se agota el plazo de la petición, como
// pasaría con un servidor de licencias lento que consume casi todo el plazo de 10 s, una desconexión de la base o un
// fallo al confirmar), la transacción se revierte:
//
//   - la hora confiable ya bajó y no queda license.clock_corrected ni license.support_unlocked con los dos valores ni la
//     lista de operaciones con fecha futura (§6.3: «se registra en la auditoría con los dos valores»);
//   - la persona ve un error aunque el cambio sí se aplicó;
//   - en la §6.4, además, la respuesta de un solo uso ya quedó gastada: al reintentar, «Esta respuesta de soporte ya se
//     usó».
//
// Lo correcto es que la baja y su auditoría sean una sola cosa: o las dos o ninguna.
func TestReviewR2DefectClockLoweringIsNotAtomicWithItsAudit(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &licenseGateFixture{restaurantFixture: newFixture(t)}
	fixture.seedCatalog(t)
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cash = method.ID
		}
	}
	fixture.clock = licensingtest.NewClock(time.Now().UTC().Truncate(time.Second))
	fixture.licenses = licensingtest.New(t, fixture.clock)
	fixture.licenses.Database = licensing.PoolProbe{Pool: fixture.pool}
	fixture.manager = fixture.licenses.Start()
	fixture.use(fixture.manager)
	fixture.licenses.Accept(fixture.manager, licensingtest.Perpetual(3, 3, 3))

	// executeWhileAuditIsLocked hace la petición por Execute (la misma ruta que la API v1) con un plazo corto, mientras
	// otra conexión tiene app.audit_events en modo EXCLUSIVE: las lecturas siguen, pero ninguna inserción en la
	// auditoría puede terminar antes del plazo.
	executeWhileAuditIsLocked := func(t *testing.T, route string, body any) error {
		t.Helper()
		locker, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer locker.Rollback(context.Background())
		if _, err := locker.Exec(fixture.ctx, "LOCK TABLE app.audit_events IN EXCLUSIVE MODE"); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(body)
		ctx, cancel := context.WithTimeout(fixture.ctx, 3*time.Second)
		defer cancel()
		_, err = fixture.service.Execute(ctx, administration.Request{
			Method: "POST", Path: "/api/v1/" + route, Body: encoded, Local: true,
			DeviceID: fixture.admin.deviceID, DeviceToken: fixture.admin.deviceToken, SessionToken: fixture.admin.sessionToken,
		})
		return err
	}

	t.Run("§6.3: la hora confiable baja aunque la petición falle y no queda la auditoría", func(t *testing.T) {
		// La fecha saltó dos días al futuro, se registró actividad con esa fecha y después se corrigió el reloj.
		real := fixture.clock.Now()
		fixture.clock.Set(real.Add(48 * time.Hour))
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "R2 futuro", "active": true}, 200)
		future := fixture.manager.TrustedTime()
		fixture.clock.Set(real)
		if !fixture.manager.Clock().Blocked {
			t.Fatal("preparación: el reloj corregido debía quedar bloqueado")
		}
		fixture.service.LicenseServerClock = &serverClock{instant: real.Add(time.Minute)}
		defer func() { fixture.service.LicenseServerClock = nil }()

		err := executeWhileAuditIsLocked(t, "license/time-check", map[string]any{})
		if err == nil {
			t.Fatal("preparación: con la auditoría bloqueada la petición debía fallar")
		}
		audited := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_corrected'")
		onDisk := fixture.licenses.Start().TrustedTime()
		t.Logf("petición: %v; hora confiable antes %s, en memoria %s, en estado.json %s; license.clock_corrected: %d",
			err, future.Format(time.RFC3339), fixture.manager.TrustedTime().Format(time.RFC3339), onDisk.Format(time.RFC3339), audited)
		if audited == 0 && (!fixture.manager.TrustedTime().Equal(future) || !onDisk.Equal(future)) {
			t.Errorf("R2-1: la petición falló, la última hora confiable bajó de %s a %s (también en estado.json) y no "+
				"quedó license.clock_corrected con los dos valores ni las operaciones con fecha futura",
				future.Format(time.RFC3339), onDisk.Format(time.RFC3339))
		}
	})

	t.Run("§6.4: la respuesta queda gastada y la hora baja aunque la petición falle, sin auditoría", func(t *testing.T) {
		key := licensingtest.NewSupportKey(t)
		fixture.service.SupportTrust = &key.Trust
		defer func() { fixture.service.SupportTrust = nil }()
		// Otra vez actividad con fecha futura y el reloj corregido.
		real := fixture.clock.Now()
		fixture.clock.Set(real.Add(30 * time.Hour))
		fixture.allowed(t, "POST", "catalog/categories", map[string]any{"name": "R2 futuro soporte", "active": true}, 200)
		future := fixture.manager.TrustedTime()
		fixture.clock.Set(real)
		if !fixture.manager.Clock().Blocked {
			t.Fatal("preparación: el reloj corregido debía quedar bloqueado")
		}
		var request struct {
			Code string `json:"request_code"`
		}
		_ = json.Unmarshal(fixture.allowed(t, "POST", "license/support-request", map[string]any{"renew": true}, 200).body, &request)
		response := key.Respond(t, request.Code, fixture.clock.Now(), fixture.clock.Now())

		err := executeWhileAuditIsLocked(t, "license/support-unlock", map[string]any{"response": response, "source": "text"})
		if err == nil {
			t.Fatal("preparación: con la auditoría bloqueada la petición debía fallar")
		}
		audited := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.support_unlocked'")
		lowered := !fixture.manager.TrustedTime().Equal(future)
		// La persona vio un error y vuelve a pegar la misma respuesta.
		retry := fixture.send("POST", "license/support-unlock", map[string]any{"response": response, "source": "text"})
		t.Logf("petición: %v; hora confiable antes %s, ahora %s; license.support_unlocked: %d; reintento: %d %s",
			err, future.Format(time.RFC3339), fixture.manager.TrustedTime().Format(time.RFC3339), audited, retry.status, retry.body)
		if audited == 0 && lowered {
			t.Errorf("R2-1: la petición falló, la última hora confiable bajó sin license.support_unlocked y la respuesta " +
				"quedó usada")
		}
		if retry.status == 400 && retry.details["reason"] == licensing.SupportReasonUsed {
			t.Errorf("R2-1: al reintentar después del error, la respuesta ya se había gastado: %s", retry.body)
		}
		var fault *administration.Fault
		if errors.As(err, &fault) {
			t.Logf("código del error visto por la persona: %s", fault.Code)
		}
	})
}
