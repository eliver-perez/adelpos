//go:build integration

package operations_test

import (
	"fmt"
	"testing"
)

// Etapa M1 (M1-D31, hallazgo R3 del revisor): si el índice único de miembros (table_union_members_open_table_idx) llegara
// a rechazar una mesa (una escritura que se saltara el lock 7820148305), la respuesta es la del contrato,
// TABLE_IN_UNION con la unión que ya tiene la mesa, y no el genérico 409 ALREADY_EXISTS. Por la API no se alcanza (Go
// comprueba antes con el mismo criterio y con el lock), así que la prueba lo provoca con un disparador temporal que,
// justo después de crear la unión nueva y antes de agregar sus mesas, mete la mesa en otra unión abierta. Nada queda
// guardado. Usa su propia base desde cero.
func TestStageM1MemberIndexAnswersTableInUnion(t *testing.T) {
	fixture := newStageM1Fixture(t)
	first := fixture.sent(t, "Mesa 1", draftLine(fixture.coca, 1))
	existing := fixture.joinNow(t, "Mesa 1", "Mesa 2")
	second := fixture.sent(t, "Mesa 5", draftLine(fixture.coca, 1))
	trigger := fmt.Sprintf(`CREATE FUNCTION app.stagem1_steal_table() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN
            IF NEW.id <> '%[1]s' THEN
                INSERT INTO app.table_union_members(id,union_id,table_id,joined_by)
                VALUES (gen_random_uuid(),'%[1]s','%[2]s',NEW.opened_by);
            END IF;
            RETURN NEW;
        END $$;
        CREATE TRIGGER stagem1_steal_table AFTER INSERT ON app.table_unions FOR EACH ROW
            EXECUTE FUNCTION app.stagem1_steal_table();`, existing.ID, fixture.id("Mesa 6"))
	if _, err := fixture.pool.Exec(fixture.ctx, trigger); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.pool.Exec(fixture.ctx, `DROP TRIGGER IF EXISTS stagem1_steal_table ON app.table_unions;
            DROP FUNCTION IF EXISTS app.stagem1_steal_table()`)
	})
	unions := fixture.count(t, "SELECT count(*)::int FROM app.table_unions")
	problem := fixture.reject(t, fixture.joiner, "POST", "restaurant/table-unions", map[string]any{
		"start_table_id": fixture.id("Mesa 5"), "table_ids": fixture.ids("Mesa 6")}, 409, "TABLE_IN_UNION",
		"La Mesa 6 ya está unida con otras mesas (Mesa 1 + 2).")
	union, _ := problem.Error.Details["union"].(map[string]any)
	if problem.Error.Details["union_id"] != existing.ID || problem.Error.Details["table_id"] != fixture.id("Mesa 6") ||
		union["id"] != existing.ID {
		t.Fatalf("detalles del rechazo por el índice: %+v", problem.Error.Details)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `DROP TRIGGER stagem1_steal_table ON app.table_unions;
        DROP FUNCTION app.stagem1_steal_table()`); err != nil {
		t.Fatal(err)
	}
	// La operación se revirtió entera: ni la unión nueva ni la mesa robada.
	if fixture.count(t, "SELECT count(*)::int FROM app.table_unions") != unions ||
		fixture.count(t, "SELECT count(*)::int FROM app.table_union_members WHERE table_id=$1", fixture.id("Mesa 6")) != 0 {
		t.Fatal("el rechazo dejó algo guardado")
	}
	expectStageM1Invariants(t, fixture.restaurantFixture)
	fixture.pay(t, first.Account.ID)
	fixture.pay(t, second.Account.ID)
}
