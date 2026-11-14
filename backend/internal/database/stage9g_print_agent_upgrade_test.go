//go:build integration

package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// stage9GExistingPrinters son impresoras de una instalación anterior a la 0023: una de red y una «agent» sin terminal
// (como el destino de aceptación de las pruebas), que la sección E debe conservar tal cual.
const stage9GExistingPrinters = `
INSERT INTO app.kitchen_printers(id,name,transport,destination) VALUES
    ('00000000-0000-4000-8000-0000000000e1','Impresora de red','escpos_tcp','192.168.1.80'),
    ('00000000-0000-4000-8000-0000000000e2','Destino controlado E2E','agent','Agente de aceptación no disponible');
`

// checkStage9GPrintAgent comprueba la sección E de la 0023 (impresión con varias terminales): las impresoras
// existentes siguen sin terminal (la «agent» sin terminal sigue sin poder imprimir), solo una impresora «agent» puede
// pertenecer a una terminal, las impresoras que informa cada terminal y la impresora de tickets por caja solo admiten
// datos coherentes.
func checkStage9GPrintAgent(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var withTerminal int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app.kitchen_printers WHERE agent_device_id IS NOT NULL").Scan(&withTerminal); err != nil ||
		withTerminal != 0 {
		t.Fatalf("impresoras existentes con terminal: %d %v", withTerminal, err)
	}
	var seen, reported, receipts int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM app.devices WHERE print_agent_seen_at IS NOT NULL
        OR local_printers_reported_at IS NOT NULL), (SELECT count(*) FROM app.device_local_printers),
        (SELECT count(*) FROM app.register_receipt_printers)`).Scan(&seen, &reported, &receipts)
	if err != nil || seen+reported+receipts != 0 {
		t.Fatalf("la sección E agregó datos: %d %d %d %v", seen, reported, receipts, err)
	}
	rejected := map[string]string{
		"impresora de red con terminal": `UPDATE app.kitchen_printers SET agent_device_id='00000000-0000-4000-8000-000000000022'
            WHERE id='00000000-0000-4000-8000-0000000000e1'`,
		"terminal que no existe": `UPDATE app.kitchen_printers SET agent_device_id='00000000-0000-4000-8000-000000000099'
            WHERE id='00000000-0000-4000-8000-0000000000e2'`,
		"cola con espacios": `INSERT INTO app.device_local_printers(device_id,queue)
            VALUES ('00000000-0000-4000-8000-000000000022','POS 80')`,
		"conexión con dirección": `INSERT INTO app.device_local_printers(device_id,queue,connection)
            VALUES ('00000000-0000-4000-8000-000000000022','POS_80','usb://Printer/POS-80')`,
		"caja sin impresora": `INSERT INTO app.register_receipt_printers(register_id,printer_id)
            VALUES ('00000000-0000-4000-8000-000000000021',NULL)`,
	}
	for name, statement := range rejected {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, statement)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("la 0023 aceptó un dato incoherente: %s", name)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		`UPDATE app.kitchen_printers SET agent_device_id='00000000-0000-4000-8000-000000000022', destination='POS_80'
            WHERE id='00000000-0000-4000-8000-0000000000e2'`,
		`INSERT INTO app.device_local_printers(device_id,queue,model,connection,printer_type)
            VALUES ('00000000-0000-4000-8000-000000000022','POS_80','POS-80','usb',36876)`,
		`INSERT INTO app.register_receipt_printers(register_id,printer_id)
            VALUES ('00000000-0000-4000-8000-000000000021','00000000-0000-4000-8000-0000000000e2')`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("dato coherente rechazado: %v", err)
		}
	}
}
