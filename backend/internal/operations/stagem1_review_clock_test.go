//go:build integration

package operations_test

import (
	"testing"
	"time"

	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1, revisor independiente (§1.4, §2.3 y M1-D14): el orden en que se aplican las escrituras del restaurante lo
// decide el lock 7820148305, pero las horas que se guardan son now(), la hora en que EMPEZÓ cada transacción. Una
// petición que empezó antes y tomó el lock después (por ejemplo, porque esperó la fila de su sesión: la misma terminal
// tenía otra petición en curso) guarda una hora anterior a la de escrituras que ya se confirmaron antes que ella.
// UnionNameAtSQL (el nombre de la unión «en un instante», que usan el ticket, la precuenta y el comprobante de una cuenta
// cobrada y «Cuentas canceladas») compara esas horas, así que con esa intercalación:
//   - el ticket que se imprime al cobrar (y al reimprimirlo) dice la mesa sola, o el nombre de una unión anterior de la
//     mesa, en vez de la unión en la que estaba la cuenta al cobrarse (que sí queda bien en la venta, sales_recognitions);
//   - la unión queda deshecha antes de haberse abierto y su historial va hacia atrás en el tiempo.
//
// La prueba arma esa intercalación sin depender de la suerte: retiene la fila de la sesión de la caja, manda el cobro
// (su transacción empieza y espera), confirma las escrituras de las mesas desde la terminal del mesero, suelta la fila y
// deja terminar el cobro.
func TestStageM1ReviewClockOrder(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })

	// delayedPayment cobra la cuenta desde la caja con su transacción empezada antes que las escrituras de meanwhile.
	delayedPayment := func(t *testing.T, accountID string, meanwhile func()) payments.Finance {
		t.Helper()
		finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
		held, err := fixture.pool.Begin(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Rollback(fixture.ctx)
		if _, err := held.Exec(fixture.ctx, "SELECT 1 FROM app.sessions WHERE device_id=$1 AND revoked_at IS NULL FOR UPDATE",
			fixture.admin.deviceID); err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			status int
			body   []byte
		}
		done := make(chan outcome, 1)
		go func() {
			status, body := fixture.raw(fixture.admin, "", "POST", "payments/accounts/"+accountID+"/payments", map[string]any{
				"operation_id": randomID(), "method_id": fixture.cashMethod, "mode": "total", "expected_balance": finance.Balance})
			done <- outcome{status, body}
		}()
		// El cobro ya empezó su transacción y espera la fila de su sesión.
		deadline := time.Now().Add(10 * time.Second)
		for fixture.count(t, `SELECT count(*)::int FROM pg_stat_activity WHERE datname=current_database()
                AND wait_event_type='Lock' AND query LIKE 'UPDATE app.sessions SET last_seen_at%'`) == 0 {
			if time.Now().After(deadline) {
				t.Fatal("el cobro no llegó a esperar la fila de su sesión")
			}
			time.Sleep(10 * time.Millisecond)
		}
		meanwhile()
		if err := held.Rollback(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		result := <-done
		if result.status != 200 {
			t.Fatalf("el cobro: %d %s", result.status, result.body)
		}
		return decodeResponse[struct {
			Finance payments.Finance `json:"finance"`
		}](t, result.body).Finance
	}
	expectOrderedHistory := func(t *testing.T, unionID string) {
		t.Helper()
		if count := fixture.count(t, `SELECT count(*)::int FROM app.table_unions WHERE id=$1 AND dissolved_at<opened_at`,
			unionID); count != 0 {
			t.Errorf("la unión se deshizo antes de abrirse")
		}
		if count := fixture.count(t, `SELECT count(*)::int FROM app.table_union_members WHERE union_id=$1 AND left_at<joined_at`,
			unionID); count != 0 {
			t.Errorf("%d mesas salieron de la unión antes de entrar", count)
		}
		history := fixture.detail(t, unionID).Union.History
		for index := 1; index < len(history); index++ {
			if history[index].At.Before(history[index-1].At) {
				t.Errorf("el historial de la unión va hacia atrás: %s (%s) después de %s (%s)", history[index].Kind,
					history[index].At.Format(time.RFC3339Nano), history[index-1].Kind, history[index-1].At.Format(time.RFC3339Nano))
			}
		}
	}
	expectPlace := func(t *testing.T, accountID string, finance payments.Finance, want string) {
		t.Helper()
		var sold string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(table_union_name,table_name,'') FROM app.sales_recognitions
            WHERE account_id=$1 ORDER BY recognized_at DESC LIMIT 1`, accountID).Scan(&sold); err != nil {
			t.Fatal(err)
		}
		if sold != want {
			t.Errorf("la venta quedó en %q, se esperaba %q", sold, want)
		}
		if finance.TableName == nil || *finance.TableName != want {
			t.Errorf("el ticket del cobro dice %v, se esperaba %q (la venta dice %q)", deref(finance.TableName), want, sold)
		}
		again := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
		if again.TableName == nil || *again.TableName != want {
			t.Errorf("la reimpresión dice %v, se esperaba %q", deref(again.TableName), want)
		}
	}

	t.Run("la unión nace mientras el cobro espera", func(t *testing.T) {
		bill := fixture.sent(t, "Mesa 5", draftLine(fixture.coca, 1))
		var union operations.TableUnion
		finance := delayedPayment(t, bill.Account.ID, func() {
			union = fixture.joinNow(t, "Mesa 5", "Mesa 6")
		})
		expectPlace(t, bill.Account.ID, finance, "Mesa 5 + 6")
		expectOrderedHistory(t, union.ID)
	})

	t.Run("la mesa pasa de una unión a otra mientras el cobro espera", func(t *testing.T) {
		first := fixture.sent(t, "Mesa 9", draftLine(fixture.coca, 1))
		earlier := fixture.joinNow(t, "Mesa 9", "Mesa 8")
		other := fixture.openAt(t, fixture.joiner, "Mesa 8", map[string]any{"seen_union_id": earlier.ID,
			"seen_account_ids": []string{first.Account.ID}}, draftLine(fixture.coca, 1))
		fixture.confirm(t, fixture.joiner, other.Account.ID)
		var later operations.TableUnion
		finance := delayedPayment(t, first.Account.ID, func() {
			// Quitar Mesa 8 pasa su cuenta a Mesa 9 y deshace «Mesa 8 + 9»; después Mesa 9 crece con Mesa 10. Todo desde la
			// terminal del mesero: la sesión de la caja está retenida. Abrir otra cuenta no cambia la revisión de la unión.
			fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/"+earlier.ID+"/tables/"+fixture.id("Mesa 8")+
				"/remove", map[string]any{"operation_id": randomID(), "revision": earlier.Revision}, 200)
			later = fixture.joinNow(t, "Mesa 9", "Mesa 10")
		})
		expectPlace(t, first.Account.ID, finance, "Mesa 9 + 10")
		expectOrderedHistory(t, later.ID)
		fixture.pay(t, other.Account.ID)
	})
}

func deref(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
