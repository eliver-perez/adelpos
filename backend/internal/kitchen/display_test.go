package kitchen

import (
	"testing"
	"time"
)

// Reglas puras de la tarjeta de la pantalla de cocina (etapa K1, Fase A, K1-D5): rondas, «NUEVO» y la tarjeta que se
// cierra por una cancelación.
func displayTestTask(id string, released time.Time, state string, ready *time.Time, updated time.Time) displayTask {
	return displayTask{ID: id, ReleaseID: "r-" + id, StationID: "s", ItemID: "i-" + id, VersionID: "v", UnitVersionID: "v",
		State: state, ReleasedAt: released, ReadyAt: ready, UpdatedAt: updated, UnitState: "active", AccountID: "a"}
}

func TestDisplayRoundsAndStatus(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	ready := func(minutes int) *time.Time { value := at(minutes); return &value }
	now := at(60)

	// Una ronda cerrada (todo listo antes de que llegara lo siguiente) y una nueva abierta: solo se muestra la nueva.
	tasks := []displayTask{
		displayTestTask("1", at(0), "ready", ready(5), at(5)),
		displayTestTask("2", at(1), "ready", ready(6), at(6)),
		displayTestTask("3", at(10), "released", nil, at(10)),
	}
	round := currentRound(tasks)
	if len(round) != 1 || round[0].ID != "3" {
		t.Fatalf("ronda nueva: %+v", round)
	}
	if status, visible := roundStatus(round, now); !visible || status != "open" {
		t.Fatalf("ronda abierta: %s %v", status, visible)
	}
	// Lo que llega mientras la tarjeta sigue abierta entra en ella (NUEVO), aunque lo anterior ya esté listo.
	tasks = []displayTask{
		displayTestTask("1", at(0), "ready", ready(8), at(8)),
		displayTestTask("2", at(5), "released", nil, at(5)),
	}
	if round := currentRound(tasks); len(round) != 2 {
		t.Fatalf("agregado con la tarjeta abierta: %+v", round)
	}
	items := (&displayService{stations: map[string]DisplayStation{"s": {ID: "s", Name: "Cocina", Tracking: "item"}}}).
		items(currentRound(tasks), at(0))
	if len(items) != 2 || items[0].IsNew || !items[1].IsNew || items[0].Status != "ready" || items[1].Status != "pending" {
		t.Fatalf("NUEVO y estados: %+v", items)
	}
	// Todo listo: la tarjeta se cierra sola.
	closed := []displayTask{displayTestTask("1", at(0), "ready", ready(8), at(8))}
	if _, visible := roundStatus(closed, now); visible {
		t.Fatal("una ronda lista se muestra")
	}
	// Lo último fue cancelar: se queda hasta que se marque o pasen 30 minutos.
	cancelled := []displayTask{
		displayTestTask("1", at(0), "ready", ready(8), at(8)),
		displayTestTask("2", at(1), "cancelled", nil, at(50)),
	}
	if status, visible := roundStatus(cancelled, now); !visible || status != "cancelled" {
		t.Fatalf("cerrada por una cancelación: %s %v", status, visible)
	}
	if _, visible := roundStatus(cancelled, at(81)); visible {
		t.Fatal("una cancelación de hace más de 30 minutos se muestra")
	}
	seen := at(55)
	cancelled[1].DismissedAt = &seen
	if _, visible := roundStatus(cancelled, now); visible {
		t.Fatal("una cancelación vista se muestra")
	}
	// Si después de cancelar se marcó lo último, se cierra sola.
	marked := []displayTask{
		displayTestTask("1", at(0), "cancelled", nil, at(20)),
		displayTestTask("2", at(1), "ready", ready(30), at(30)),
	}
	if _, visible := roundStatus(marked, now); visible {
		t.Fatal("una tarjeta cerrada al marcar se muestra")
	}
	// Una unidad cancelada cuya tarea no se procesó cuenta como cancelada desde su cancelación.
	unit := displayTestTask("1", at(0), "released", nil, at(0))
	unit.UnitState = "cancelled"
	unitCancelled := at(40)
	unit.UnitCancelledAt = &unitCancelled
	if !unit.cancelled() || unit.pending() || !unit.cancelledAt().Equal(unitCancelled) {
		t.Fatalf("unidad cancelada: %+v", unit)
	}
}

func TestDisplayCardHeader(t *testing.T) {
	table := "Mesa 4"
	for name, check := range map[string]struct {
		header          displayHeader
		title, subtitle string
		waiter          string
	}{
		"mesa":                 {displayHeader{Mode: "dine_in", Name: "mesa  4", TableName: &table, WaiterName: "Ana"}, "Mesa 4", "", "Ana"},
		"mesa con otro nombre": {displayHeader{Mode: "dine_in", Name: "Cumpleaños", TableName: &table, WaiterName: "Ana"}, "Mesa 4", "Cumpleaños", "Ana"},
		"sin mesa":             {displayHeader{Mode: "dine_in", Name: "Barra 2"}, "Barra 2", "", "Beto"},
		"para llevar":          {displayHeader{Mode: "takeaway", Name: "Para llevar 7", CustomerName: "Lucía", WaiterName: "Ana"}, "Para llevar", "Lucía", "Ana"},
		"para llevar sin cliente": {displayHeader{Mode: "takeaway", Name: "Para llevar 7", WaiterName: "Ana"}, "Para llevar",
			"Para llevar 7", "Ana"},
	} {
		header := cardHeader(check.header, "Beto")
		if header.Title != check.title || header.Subtitle != check.subtitle || header.WaiterName != check.waiter {
			t.Fatalf("%s: %+v", name, header)
		}
	}
}
