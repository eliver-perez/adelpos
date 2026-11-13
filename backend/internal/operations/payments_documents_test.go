//go:build integration

package operations_test

import (
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/payments"
)

type printingResult struct {
	Requested bool   `json:"requested"`
	Status    string `json:"status"`
	JobID     string `json:"job_id"`
	TicketID  string `json:"ticket_id"`
}

func (fixture *paymentsFixture) receiptSettings(t *testing.T, printerID any, printDefault bool) {
	t.Helper()
	settings := decodeResponse[payments.Settings](t, fixture.adminCall(t, "GET", "payments/settings", nil, 200))
	fixture.adminCall(t, "PUT", "payments/settings", map[string]any{
		"operation_id": randomID(), "revision": settings.Revision, "shared_shifts": true, "print_receipt_default": printDefault,
		"receipt_printer_id": printerID, "refund_authorization_threshold": nil,
	}, 200)
}

func testPaymentDocuments(t *testing.T, fixture *paymentsFixture) {
	cash := fixture.methods["cash"].ID
	receipts := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
		Name: "Tickets de caja de aceptación", Transport: "cups", Destination: "ADEL_TEST_RECEIPT", Active: true, PaperWidth: 80, Copies: 1,
	}, 200))

	t.Run("51_precuenta_con_iva_y_saldo_sin_registrar_venta", func(t *testing.T) {
		bill := fixture.account(t, "Precuenta", line(fixture.burger, 1), line(fixture.soda, 1))
		fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{"method_id": cash, "mode": "partial", "amount": "45"}, 200)
		paymentCount := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		document := decodeResponse[payments.CommercialDocument](t, fixture.adminCall(t, "GET", "payments/accounts/"+bill.Account.ID+"/precheck", nil, 200))
		for _, fragment := range []string{"PRECUENTA", "Subtotal sin IVA 16 %", "IVA 16 %", "TOTAL", "$145.00", "PAGOS REGISTRADOS", "SALDO PENDIENTE", "$100.00", "CFDI"} {
			if !strings.Contains(document.Content, fragment) {
				t.Fatalf("La precuenta no muestra %q:\n%s", fragment, document.Content)
			}
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments") != paymentCount || fixture.countRows(t, "SELECT count(*) FROM app.payment_documents") != 0 {
			t.Fatal("Consultar la precuenta registró una venta o un documento")
		}
		fixture.post(t, fixture.admin, bill.Account.ID, "precheck/print", map[string]any{}, 409)
	})

	t.Run("52_53_54_ticket_con_toggle_y_pago_valido_aunque_falle_la_impresora", func(t *testing.T) {
		fixture.receiptSettings(t, nil, true)
		unconfigured := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.admin, fixture.account(t, "Sin impresora", line(fixture.soda, 1)).Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "print": true,
		}, 200))
		if unconfigured.Printing.Status != "not_configured" {
			t.Fatal("La falta de impresora no se informó sin revertir el pago")
		}
		fixture.receiptSettings(t, receipts.ID, true)
		off := decodeResponse[struct {
			Printing printingResult `json:"printing"`
		}](t, fixture.pay(t, fixture.admin, fixture.account(t, "Sin ticket", line(fixture.soda, 1)).Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "print": false,
		}, 200))
		if off.Printing.Requested {
			t.Fatal("El toggle desactivado imprimió un ticket")
		}
		bill := fixture.account(t, "Con ticket", line(fixture.burger, 1))
		before := fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets WHERE kind NOT IN ('receipt','precheck')")
		printed := decodeResponse[struct {
			Payment  payments.PaymentRecord `json:"payment"`
			Printing printingResult         `json:"printing"`
		}](t, fixture.pay(t, fixture.admin, bill.Account.ID, map[string]any{
			"method_id": cash, "mode": "total", "received": "200", "print": true,
		}, 200))
		if !printed.Printing.Requested || printed.Printing.Status != "pending" {
			t.Fatalf("El ticket no se encoló: %+v", printed.Printing)
		}
		var content string
		if err := fixture.pool.QueryRow(fixture.ctx, "SELECT content FROM app.kitchen_print_jobs WHERE id=$1", printed.Printing.JobID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		for _, fragment := range []string{"TICKET DE PAGO", "ESTE PAGO", "Efectivo recibido", "$200.00", "Cambio", "$80.00", "IVA 16 %", "CFDI"} {
			if !strings.Contains(content, fragment) {
				t.Fatalf("El ticket no muestra %q:\n%s", fragment, content)
			}
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.kitchen_tickets WHERE kind NOT IN ('receipt','precheck')") != before {
			t.Fatal("El ticket de pago se registró como comanda")
		}
		// 54: la impresora falla y el pago permanece. Las comandas pendientes de otras pruebas se
		// cancelan para que el trabajador procese exactamente este ticket.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state='pending' AND id<>$1", printed.Printing.JobID); err != nil {
			t.Fatal(err)
		}
		adapter := &recordingTransport{state: "error"}
		if _, err := (&kitchen.Worker{Pool: fixture.pool, Transport: adapter}).PrintNext(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		if fixture.countRows(t, "SELECT count(*) FROM app.payments WHERE id=$1", printed.Payment.ID) != 1 {
			t.Fatal("Un fallo de impresión revirtió el pago")
		}
		finance := fixture.finance(t, bill.Account.ID)
		if len(finance.Documents) != 1 || *finance.Documents[0].JobState != "error" {
			t.Fatal("La cuenta no muestra el estado del ticket con error")
		}
		fixture.registers["receipt_ticket"] = printed.Printing.TicketID
		fixture.registers["receipt_account"] = bill.Account.ID
		fixture.registers["receipt_job"] = printed.Printing.JobID
	})

	t.Run("55_reimpresion_sin_duplicar_cobros", func(t *testing.T) {
		accountID := fixture.registers["receipt_account"]
		ticketID := fixture.registers["receipt_ticket"]
		fixture.post(t, fixture.admin, accountID, "documents/reprint", map[string]any{"ticket_id": ticketID, "reason": "Copia para el cliente"}, 400)
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='submitted' WHERE id=$1", fixture.registers["receipt_job"]); err != nil {
			t.Fatal(err)
		}
		paymentsBefore := fixture.countRows(t, "SELECT count(*) FROM app.payments")
		copy := decodeResponse[printingResult](t, fixture.post(t, fixture.admin, accountID, "documents/reprint", map[string]any{"ticket_id": ticketID, "reason": "Copia para el cliente"}, 200))
		fixture.post(t, fixture.admin, accountID, "documents/reprint", map[string]any{"ticket_id": ticketID, "reason": "Copia para el cliente"}, 409)
		var original, duplicate string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT (SELECT content FROM app.kitchen_print_jobs WHERE id=$1),
            (SELECT content FROM app.kitchen_print_jobs WHERE id=$2)`, fixture.registers["receipt_job"], copy.JobID).Scan(&original, &duplicate); err != nil {
			t.Fatal(err)
		}
		if original != duplicate || fixture.countRows(t, "SELECT count(*) FROM app.payments") != paymentsBefore {
			t.Fatal("La reimpresión alteró el documento o duplicó el cobro")
		}
		ticket := decodeResponse[kitchen.Document](t, fixture.adminCall(t, "GET", "kitchen/tickets/"+ticketID, nil, 200))
		if !strings.Contains(ticket.Preview, "TICKET DE PAGO") {
			t.Fatal("La cola no muestra el contenido del ticket comercial")
		}
		listed := string(fixture.adminCall(t, "GET", "kitchen/tickets", nil, 200))
		if strings.Contains(listed, ticketID) {
			t.Fatal("El ticket comercial apareció entre las comandas de cocina")
		}
		precheck := decodeResponse[printingResult](t, fixture.post(t, fixture.admin, accountID, "precheck/print", map[string]any{}, 200))
		if precheck.Status != "pending" || fixture.countRows(t, "SELECT count(*) FROM app.payment_documents WHERE kind='precheck'") != 1 {
			t.Fatal("La precuenta no se encoló para impresión")
		}
		// Se cancelan los trabajos de aceptación para no dejar la cola con documentos de prueba.
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE app.kitchen_print_jobs SET state='cancelled' WHERE state IN ('pending','error')"); err != nil {
			t.Fatal(err)
		}
		fixture.receiptSettings(t, nil, true)
	})
}
