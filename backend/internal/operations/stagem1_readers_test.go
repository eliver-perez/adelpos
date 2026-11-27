//go:build integration

package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa M1 (Fase A, §1.4, §1.5 y contrato §8; M1-D3 y M1-D14): el nombre de la unión en todos los lectores. Con las
// mesas «5» y «6» la unión se llama «5 + 6» y los documentos, la comanda y el KDS dicen «Mesa 5 + 6»
// (documents.TableContext). También Preparación, el aviso de platillos listos, la instantánea de la venta y la
// precuenta de una cuenta ya cobrada (con el nombre que tenía la unión al cerrarse). Un cambio de la unión con la
// tarjeta en cocina le manda a la pantalla el evento de encabezado (restaurant.account.table_union). Usa su propia base
// desde cero.
func TestStageM1UnionReaders(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	// El mesero entra con PIN en una tableta 0.2.0 (con avisos de platillos listos).
	fixture.adminCall(t, "PUT", "users/"+fixture.joinerID+"/pin", map[string]any{"pin": "4829", "short_name": "Uni",
		"badge_color": "#3591d4"}, 200)
	secret := administration.NewToken()
	var deviceID string
	if err := fixture.pool.QueryRow(fixture.ctx, `INSERT INTO app.devices(name,secret_hash,kind,os,app_version,paired_at)
        VALUES ('Tableta M1',$1,'android','Android 15','0.2.0',now()) RETURNING id`, administration.TokenHash(secret)).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	device := testTerminal{deviceID: deviceID, deviceToken: secret}
	code, response := fixture.raw(device, "", "POST", "auth/pin-login", map[string]string{"user_id": fixture.joinerID, "pin": "4829"})
	login := decodeResponse[struct {
		Session string `json:"session_token"`
	}](t, response)
	if code != 200 {
		t.Fatalf("entrada con PIN: %d %s", code, response)
	}
	tablet := device
	tablet.sessionToken = login.Session

	opened := fixture.openAt(t, tablet, "5", map[string]any{"union_table_ids": fixture.ids("6"), "seen_union_id": nil},
		draftLine(fixture.hamburguesa, 1), draftLine(fixture.coca, 1))
	bill := fixture.confirm(t, tablet, opened.Account.ID)
	accountID := bill.Account.ID
	if bill.Account.Name != "5 + 6" || bill.Account.TableUnion == nil || bill.Account.TableUnion.Name != "5 + 6" {
		t.Fatalf("la cuenta de la unión «5 + 6»: %+v", bill.Account)
	}

	t.Run("precuenta y estado financiero", func(t *testing.T) {
		document := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID+"/precheck",
			nil, 200))
		if document.Context != "Mesa 5 + 6" || !strings.Contains(document.Content, "Mesa 5 + 6") {
			t.Fatalf("precuenta: %q\n%s", document.Context, document.Content)
		}
		finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
		if finance.TableName == nil || *finance.TableName != "5 + 6" {
			t.Fatalf("finance.table_name: %v", finance.TableName)
		}
	})

	t.Run("comanda, KDS y Preparación", func(t *testing.T) {
		var table string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT context_snapshot->>'table' FROM app.kitchen_releases
            WHERE account_id=$1 AND state='released'`, accountID).Scan(&table); err != nil || table != "Mesa 5 + 6" {
			t.Fatalf("la comanda: %q %v", table, err)
		}
		found := card(t, fixture.orders(t, fixture.display), accountID)
		if found == nil || found.Title != "Mesa 5 + 6" || found.Subtitle != "" {
			t.Fatalf("tarjeta del KDS: %+v", found)
		}
		board := decodeResponse[struct {
			Tasks []map[string]any `json:"tasks"`
		}](t, fixture.adminCall(t, "GET", "kitchen/board", nil, 200))
		names := []any{}
		for _, task := range board.Tasks {
			if task["account_id"] == accountID || task["folio"] == float64(bill.Account.Folio) {
				names = append(names, task["table_name"])
			}
		}
		if len(names) == 0 || names[0] != "5 + 6" {
			t.Fatalf("Preparación: %v", names)
		}
	})

	t.Run("un cambio de la unión actualiza el encabezado del KDS", func(t *testing.T) {
		after := fixture.head(t)
		union := fixture.union(t, "5")
		fixture.call(t, tablet, "POST", "restaurant/table-unions/"+union.ID+"/tables", map[string]any{"operation_id": randomID(),
			"revision": union.Revision, "table_id": fixture.id("Mesa 12")}, 200)
		batch := fixture.displayEvents(t, fixture.display, after)
		if !hasEvent(batch, "restaurant.account.table_union", "", accountID) {
			t.Fatalf("la pantalla no recibió el cambio de encabezado: %+v", batch.Events)
		}
		found := card(t, fixture.orders(t, fixture.display), accountID)
		if found == nil || found.Title != "Mesa 5 + 6 + Mesa 12" {
			t.Fatalf("encabezado nuevo: %+v", found)
		}
	})

	t.Run("aviso de platillos listos", func(t *testing.T) {
		found := card(t, fixture.orders(t, fixture.display), accountID)
		fixture.mark(t, fixture.display, "orders/ready", refs(found.Items...))
		code, response := fixture.raw(device, "", "GET", "ready-alerts?after=0", nil)
		if code != 200 {
			t.Fatalf("avisos: %d %s", code, response)
		}
		batch := decodeResponse[administration.ReadyAlertsBatch](t, response)
		var name *string
		for _, alert := range batch.Alerts {
			if alert.AccountID == accountID {
				name = alert.TableName
			}
		}
		if name == nil || *name != "5 + 6 + Mesa 12" {
			t.Fatalf("aviso de listos: %s", response)
		}
	})

	t.Run("la venta queda en la unión", func(t *testing.T) {
		fixture.pay(t, accountID)
		var unionID, name, tables string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT DISTINCT table_union_id::text,table_union_name,table_union_tables::text
            FROM app.sales_recognitions WHERE account_id=$1`, accountID).Scan(&unionID, &name, &tables); err != nil {
			t.Fatal(err)
		}
		var members []map[string]string
		if err := json.Unmarshal([]byte(tables), &members); err != nil {
			t.Fatal(err)
		}
		if unionID != bill.Account.TableUnion.ID || name != "5 + 6 + Mesa 12" || len(members) != 3 ||
			members[0]["id"] != fixture.id("5") || members[2]["name"] != "Mesa 12" {
			t.Fatalf("instantánea de la venta: %s %s %s", unionID, name, tables)
		}
		// La unión ya se deshizo, pero la cuenta cobrada se sigue nombrando como estaba al cerrarse.
		if fixture.union(t, "5") != nil {
			t.Fatal("la unión sigue abierta")
		}
		finance := decodeResponse[payments.Finance](t, fixture.adminCall(t, "GET", "payments/accounts/"+accountID, nil, 200))
		if finance.TableName == nil || *finance.TableName != "5 + 6 + Mesa 12" {
			t.Fatalf("la cuenta cobrada: %v", finance.TableName)
		}
		// Una cuenta de una mesa sola no tiene unión en la venta.
		single := fixture.sent(t, "Mesa 9", draftLine(fixture.coca, 1))
		fixture.pay(t, single.Account.ID)
		var empty bool
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT bool_and(table_union_id IS NULL AND table_union_name IS NULL
            AND table_union_tables IS NULL) FROM app.sales_recognitions WHERE account_id=$1`, single.Account.ID).Scan(&empty); err != nil || !empty {
			t.Fatalf("venta de una mesa sola: %v %v", empty, err)
		}
	})
}
