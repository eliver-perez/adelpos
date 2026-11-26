//go:build integration

package operations_test

import (
	"fmt"
	"slices"
	"testing"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/kitchen"
	"local/ad-el-pos/server/internal/operations"
	"local/ad-el-pos/server/internal/payments"
)

// Etapa K1.1 (Fase 0, prueba de reproducción, y Fase A; §1, §2, §3 y §6 de la especificación) contra PostgreSQL con la
// API real: la configuración del propietario —estación «Cocina» con recepción «Solo pantalla de cocina», Desayunos y
// Hamburguesas a Cocina, Refrescos (Coca Cola 600ml) y Extras (Papas a la francesa) sin ruta y una pantalla de cocina
// con todas las estaciones— y lo que debe pasar con la regla nueva de la §2 (K11-D1):
//   - los cuatro pedidos (solo machacado; machacado + hamburguesa; machacado + Coca Cola; machacado + Coca Cola +
//     papas) llegan a la pantalla con lo que tiene ruta, y la Coca Cola y las papas no aparecen;
//   - ninguno queda en «Por enviar a cocina» ni bloqueado, y todas sus unidades se liberan una vez (lo que no necesita
//     preparación, como una ruta «Sin preparación», sin tarea);
//   - la tarjeta no se cierra sola por lo que no necesita preparación y el aviso de platillos listos al mesero sale
//     cuando lo que tiene ruta está listo;
//   - con una estación que imprime, la comanda impresa lleva solo lo que tiene ruta.
//
// Las demás reglas de la Fase A (K11-D1 a K11-D5) están en stagek11_rules_test.go. Con la 0.16.0 fallaba en los casos 3
// y 4: un solo producto sin ruta dejaba la liberación entera en «blocked» y la pantalla no recibía nada. Usa su propia
// base desde cero.

type unroutedFixture struct {
	*kitchenDisplayFixture
	cocina      kitchen.Station
	machacado   catalog.Product
	hamburguesa catalog.Product
	coca        catalog.Product
	papas       catalog.Product
	// cashMethod es el efectivo, para cobrar las cuentas de las pruebas de pendientes (Fase A).
	cashMethod string
}

// product crea una categoría con un solo producto.
func (fixture *unroutedFixture) product(t *testing.T, category, name, code, price string) (catalog.Category, catalog.Product) {
	t.Helper()
	created := decodeResponse[catalog.Category](t, fixture.adminCall(t, "POST", "catalog/categories", catalog.Category{
		Metadata: catalog.Metadata{Name: category, Active: true},
	}, 200))
	product := decodeResponse[catalog.Product](t, fixture.adminCall(t, "POST", "catalog/products", catalog.Product{
		Metadata: catalog.Metadata{Name: name, Active: true}, CategoryID: created.ID, Code: code, Kind: "product", BasePrice: price,
	}, 200))
	return created, product
}

// expectArrived comprueba que la cuenta llegó a la pantalla con exactamente esos platillos de Cocina, sin bloqueo ni
// pendiente, con todas sus unidades liberadas.
func (fixture *unroutedFixture) expectArrived(t *testing.T, accountID string, units int, names ...string) *kitchen.DisplayCard {
	t.Helper()
	found := card(t, fixture.orders(t, fixture.display), accountID)
	if found == nil {
		t.Fatalf("la pantalla de cocina no recibió el pedido; liberaciones: %s", fixture.releaseStates(t, accountID))
	}
	shown := []string{}
	for _, item := range found.Items {
		if item.StationID != fixture.cocina.ID {
			t.Fatalf("platillo de otra estación en la tarjeta: %+v", item)
		}
		shown = append(shown, item.Name)
	}
	slices.Sort(shown)
	slices.Sort(names)
	if !slices.Equal(shown, names) {
		t.Fatalf("platillos en la pantalla: %v, esperados %v", shown, names)
	}
	if blocked := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_releases WHERE account_id=$1 AND state<>'released'`,
		accountID); blocked != 0 {
		t.Fatalf("liberaciones sin liberar: %s", fixture.releaseStates(t, accountID))
	}
	// Cada unidad se libera una sola vez, también la que no necesita preparación (sin tarea): así cuenta para el
	// inventario y para «Listo», como hoy una ruta «Sin preparación».
	if released := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_unit_releases k JOIN app.restaurant_item_units u
        ON u.id=k.unit_id WHERE u.account_id=$1`, accountID); released != units {
		t.Fatalf("unidades liberadas: %d de %d", released, units)
	}
	if tasks := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_tasks t JOIN app.restaurant_item_units u ON u.id=t.unit_id
        WHERE u.account_id=$1`, accountID); tasks != len(names) {
		t.Fatalf("tareas de cocina: %d, esperadas %d (solo lo que tiene ruta)", tasks, len(names))
	}
	board := decodeResponse[struct {
		Releases []map[string]any `json:"releases"`
	}](t, fixture.adminCall(t, "GET", "kitchen/board", nil, 200))
	for _, release := range board.Releases {
		if release["account_id"] == accountID {
			t.Fatalf("el pedido quedó en «Pendientes de liberación y avisos»: %v", release)
		}
	}
	return found
}

// releaseStates describe las liberaciones de la cuenta para los mensajes de error.
func (fixture *unroutedFixture) releaseStates(t *testing.T, accountID string) string {
	t.Helper()
	rows, err := fixture.pool.Query(fixture.ctx, `SELECT state,warnings::text FROM app.kitchen_releases WHERE account_id=$1
        ORDER BY created_at`, accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := ""
	for rows.Next() {
		var state, warnings string
		if err := rows.Scan(&state, &warnings); err != nil {
			t.Fatal(err)
		}
		result += fmt.Sprintf("[%s %s] ", state, warnings)
	}
	return result
}

func TestStageK11UnroutedProducts(t *testing.T) {
	resetStage9GDatabase(t)
	fixture := &unroutedFixture{kitchenDisplayFixture: &kitchenDisplayFixture{waiterModeFixture: &waiterModeFixture{
		restaurantFixture: newFixture(t)}}}
	desayunos, machacado := fixture.product(t, "Desayunos", "Machacado Natural", "MACHACADO", "95")
	hamburguesas, hamburguesa := fixture.product(t, "Hamburguesas", "Hamburguesa sencilla", "HAMBURGUESA", "120")
	_, coca := fixture.product(t, "Refrescos", "Coca Cola 600ml", "COCA-600", "35")
	_, papas := fixture.product(t, "Extras", "Papas a la francesa", "PAPAS-FRANCESA", "45")
	fixture.machacado, fixture.hamburguesa, fixture.coca, fixture.papas = machacado, hamburguesa, coca, papas
	fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "POST", "kitchen/stations", kitchen.Station{
		Name: "Cocina", Active: true, Reception: "kds", Tracking: "inherit", Grouping: "inherit",
	}, 200))
	for _, category := range []catalog.Category{desayunos, hamburguesas} {
		fixture.mutate(t, "POST", "kitchen/routes", kitchen.Route{CategoryID: &category.ID, Mode: "prepare",
			StationIDs: []string{fixture.cocina.ID}}, 200)
	}
	fixture.area = decodeResponse[operations.Area](t, fixture.mutate(t, "POST", "restaurant/areas", operations.Area{Name: "Salón K1.1", Active: true}, 200))
	for index := range 6 {
		fixture.tables = append(fixture.tables, decodeResponse[operations.Table](t, fixture.mutate(t, "POST", "restaurant/tables", operations.Table{
			AreaID: fixture.area.ID, Name: fmt.Sprintf("Mesa %d", index+1), Shape: "round", X: 20 + index*110, Y: 30,
			Width: 100, Height: 72, Seats: 4, Active: true,
		}, 200)))
	}
	inventoryUser(t, fixture.restaurantFixture, "mesero_k11", waiterPermissions)
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT id::text FROM app.users WHERE username='mesero_k11'").Scan(&fixture.waiterID); err != nil {
		t.Fatal(err)
	}
	fixture.display = fixture.kitchenDevice(t, "Tableta de cocina K1.1")
	fixture.displayCall(t, fixture.display, "POST", "kitchen-display/connect", map[string]any{}, 200)
	for _, method := range decodeResponse[[]payments.Method](t, fixture.adminCall(t, "GET", "payments/methods", nil, 200)) {
		if method.Code != nil && *method.Code == "cash" {
			fixture.cashMethod = method.ID
		}
	}
	fixture.adminCall(t, "POST", "payments/shifts", map[string]any{"operation_id": randomID(), "opening_float": "0"}, 200)

	t.Run("los cuatro pedidos del propietario llegan a la pantalla con lo que tiene ruta", func(t *testing.T) {
		cases := []struct {
			name     string
			products []catalog.Product
			shown    []string
		}{
			{"solo machacado", []catalog.Product{machacado}, []string{"Machacado Natural"}},
			{"machacado + hamburguesa", []catalog.Product{machacado, hamburguesa}, []string{"Machacado Natural", "Hamburguesa sencilla"}},
			{"machacado + Coca Cola", []catalog.Product{machacado, coca}, []string{"Machacado Natural"}},
			{"machacado + Coca Cola + papas", []catalog.Product{machacado, coca, papas}, []string{"Machacado Natural"}},
		}
		for index, entry := range cases {
			t.Run(entry.name, func(t *testing.T) {
				lines := []operations.DraftItem{}
				for _, product := range entry.products {
					lines = append(lines, draftLine(product, 1))
				}
				accountID := fixture.order(t, index, lines...)
				fixture.expectArrived(t, accountID, len(entry.products), entry.shown...)
			})
		}
	})

	t.Run("la tarjeta no se cierra sola por lo que no necesita preparación; el aviso sale con lo que tiene ruta", func(t *testing.T) {
		accountID := fixture.order(t, 4, draftLine(machacado, 1), draftLine(coca, 1), draftLine(papas, 1))
		found := fixture.expectArrived(t, accountID, 3, "Machacado Natural")
		if found.Status != "open" || fixture.notices(t, accountID) != 0 {
			t.Fatalf("la tarjeta se cerró o avisó por lo que no necesita preparación: %+v, avisos %d", found, fixture.notices(t, accountID))
		}
		if state := fixture.refresh(t, accountID).Account.PreparationState; state == "ready" || state == "pending_release" {
			t.Fatalf("preparación de la cuenta antes de marcar: %q", state)
		}
		fixture.mark(t, fixture.display, "orders/ready", refs(found.Items...))
		if card(t, fixture.orders(t, fixture.display), accountID) != nil {
			t.Fatal("la tarjeta marcada sigue en la pantalla")
		}
		// Un aviso por la unidad con tarea (el machacado); la Coca Cola y las papas no avisan ni lo detienen.
		if notices := fixture.notices(t, accountID); notices != 1 {
			t.Fatalf("avisos de platillos listos: %d, esperado 1", notices)
		}
		if state := fixture.refresh(t, accountID).Account.PreparationState; state != "ready" {
			t.Fatalf("preparación de la cuenta después de marcar: %q", state)
		}
	})

	t.Run("con una estación que imprime, la comanda impresa lleva solo lo que tiene ruta", func(t *testing.T) {
		printer := decodeResponse[kitchen.Printer](t, fixture.mutate(t, "POST", "kitchen/printers", kitchen.Printer{
			Name: "Comandas K1.1", Transport: "escpos_tcp", Destination: "127.0.0.1", Port: 9108, Active: true, PaperWidth: 80, Copies: 1,
		}, 200))
		both := fixture.cocina
		both.Reception, both.PrinterID = "both", &printer.ID
		fixture.cocina = decodeResponse[kitchen.Station](t, fixture.mutate(t, "PATCH", "kitchen/stations/"+both.ID, both, 200))
		accountID := fixture.order(t, 5, draftLine(machacado, 1), draftLine(coca, 1))
		fixture.expectArrived(t, accountID, 2, "Machacado Natural")
		var names []string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT coalesce(array_agg(item->>'name' ORDER BY item->>'name'),'{}')
            FROM app.kitchen_tickets t JOIN app.kitchen_releases r ON r.id=t.release_id
            CROSS JOIN LATERAL jsonb_array_elements(t.document->'sections') section
            CROSS JOIN LATERAL jsonb_array_elements(section->'items') item WHERE r.account_id=$1 AND t.kind='normal'`,
			accountID).Scan(&names); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(names, []string{"Machacado Natural"}) {
			t.Fatalf("renglones de la comanda: %v", names)
		}
		if jobs := fixture.count(t, `SELECT count(*)::int FROM app.kitchen_print_jobs j JOIN app.kitchen_tickets t ON t.id=j.ticket_id
            JOIN app.kitchen_releases r ON r.id=t.release_id WHERE r.account_id=$1`, accountID); jobs != 1 {
			t.Fatalf("trabajos de impresión: %d, esperado 1", jobs)
		}
	})

	// Fase A (stagek11_rules_test.go).
	t.Run("con «Enviar a una estación» la Coca Cola y las papas llegan a esa estación", func(t *testing.T) {
		testStageK11SendToStation(t, fixture)
	})
	t.Run("una modificación sigue la ruta congelada aunque después se configure la ruta", func(t *testing.T) {
		testStageK11FrozenModification(t, fixture)
	})
	t.Run("una ruta a una estación desactivada no detiene lo demás", func(t *testing.T) {
		testStageK11InactiveStation(t, fixture)
	})
	t.Run("una impresora desactivada no detiene nada y la pantalla recibe la comanda", func(t *testing.T) {
		testStageK11InactivePrinter(t, fixture)
	})
	t.Run("combo con componentes sin ruta propia y combo vacío", func(t *testing.T) {
		testStageK11Combos(t, fixture)
	})
	t.Run("al cobrar se da por terminado el aviso pendiente y reabrir no lo revive", func(t *testing.T) {
		testStageK11PendingOnPayment(t, fixture)
	})
	t.Run("un pedido para llevar cobrado y sin entregar sigue abierto; al entregarse se cierra", func(t *testing.T) {
		testStageK11PendingTakeaway(t, fixture)
	})
	t.Run("al cancelar se da por terminado lo no enviado y el aviso pendiente", func(t *testing.T) {
		testStageK11PendingCancel(t, fixture)
	})
	t.Run("combinar conserva el pendiente en la cuenta abierta", func(t *testing.T) {
		testStageK11PendingMerge(t, fixture)
	})
	t.Run("un bloqueado de antes de la 0.17.0 se envía con la regla nueva", func(t *testing.T) {
		testStageK11OldBlocked(t, fixture)
	})
	t.Run("cobertura del enrutamiento con cada aviso y «Productos sin ruta»", func(t *testing.T) {
		testStageK11Coverage(t, fixture)
	})
}
