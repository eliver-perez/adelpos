//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
// escritura del archivo (aquí: la inserción en audit_events espera un lock y se agota el plazo; en producción, un
// servidor de licencias lento que consume casi todo el plazo de 10 s, una desconexión de la base o un fallo al
// confirmar), la transacción se revierte. Pasa igual en etapa-l1 (91f17ba), donde la comprobación con internet ya corre
// fuera de la petición pero ReplaceTrustedTime sigue antes de futureOperations, la auditoría y el commit de su
// transacción corta:
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

	// sendWhileAuditIsLocked hace la petición por la API v1 (ServeHTTP, con su plazo de 10 s) mientras otra conexión tiene
	// app.audit_events en modo EXCLUSIVE: las lecturas siguen, pero ninguna inserción en la auditoría termina antes de que
	// se agote el plazo (el de la petición, o el de la transacción corta de la auditoría donde la comprobación con
	// internet ya corre fuera de la petición). Devuelve la respuesta que vio la persona.
	sendWhileAuditIsLocked := func(t *testing.T, route string, body any) licenseReply {
		t.Helper()
		locker, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer locker.Rollback(context.Background())
		if _, err := locker.Exec(fixture.ctx, "LOCK TABLE app.audit_events IN EXCLUSIVE MODE"); err != nil {
			t.Fatal(err)
		}
		return fixture.send("POST", route, body)
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
		server := &serverClock{instant: real.Add(time.Minute)}
		fixture.service.LicenseServerClock = server
		defer func() { fixture.service.LicenseServerClock = nil }()

		failed := sendWhileAuditIsLocked(t, "license/time-check", map[string]any{})
		if failed.status == 200 {
			t.Fatalf("preparación: con la auditoría bloqueada la petición debía fallar: %s", failed.body)
		}
		audited := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_corrected'")
		onDisk := fixture.licenses.Start().TrustedTime()
		t.Logf("petición: %d %s; hora confiable antes %s, en memoria %s, en estado.json %s; license.clock_corrected: %d",
			failed.status, failed.code, future.Format(time.RFC3339), fixture.manager.TrustedTime().Format(time.RFC3339), onDisk.Format(time.RFC3339), audited)
		if audited == 0 && (!fixture.manager.TrustedTime().Equal(future) || !onDisk.Equal(future)) {
			t.Errorf("R2-1: la petición falló, la última hora confiable bajó de %s a %s (también en estado.json) y no "+
				"quedó license.clock_corrected con los dos valores ni las operaciones con fecha futura",
				future.Format(time.RFC3339), onDisk.Format(time.RFC3339))
		}
		// Corrección de R2-1: la persona vuelve a intentar sin el lock y la baja se aplica, con su auditoría (las dos
		// juntas). Así la subprueba siguiente parte de la hora corregida, como antes de la corrección.
		retry := fixture.send("POST", "license/time-check", map[string]any{})
		corrected := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.clock_corrected' AND success")
		if retry.status != 200 || corrected != 1 || !fixture.manager.TrustedTime().Equal(server.instant.UTC()) ||
			!fixture.licenses.Start().TrustedTime().Equal(server.instant.UTC()) {
			t.Fatalf("al reintentar, la baja se aplica con su auditoría: %d %s; license.clock_corrected: %d", retry.status,
				retry.body, corrected)
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

		failed := sendWhileAuditIsLocked(t, "license/support-unlock", map[string]any{"response": response, "source": "text"})
		if failed.status == 200 {
			t.Fatalf("preparación: con la auditoría bloqueada la petición debía fallar: %s", failed.body)
		}
		audited := fixture.count(t, "SELECT count(*)::int FROM app.audit_events WHERE action='license.support_unlocked'")
		lowered := !fixture.manager.TrustedTime().Equal(future)
		// La persona vio un error y vuelve a pegar la misma respuesta.
		retry := fixture.send("POST", "license/support-unlock", map[string]any{"response": response, "source": "text"})
		t.Logf("petición: %d %s; hora confiable antes %s, ahora %s; license.support_unlocked: %d; reintento: %d %s",
			failed.status, failed.code, future.Format(time.RFC3339), fixture.manager.TrustedTime().Format(time.RFC3339), audited, retry.status, retry.body)
		if audited == 0 && lowered {
			t.Errorf("R2-1: la petición falló, la última hora confiable bajó sin license.support_unlocked y la respuesta " +
				"quedó usada")
		}
		if retry.status == 400 && retry.details["reason"] == licensing.SupportReasonUsed {
			t.Errorf("R2-1: al reintentar después del error, la respuesta ya se había gastado: %s", retry.body)
		}
	})
}
