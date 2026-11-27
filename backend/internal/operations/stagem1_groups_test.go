//go:build integration

package operations_test

import (
	"slices"
	"testing"

	"local/ad-el-pos/server/internal/operations"
)

// Etapa M1 (Fase A, §3 y §7 «Divisibles»): el grupo «Mesa 3» con las secciones «3A», «3B» y «3C»; su atajo (una unión
// diferida con union_group_id que se llama como el grupo), las combinaciones parciales («3A + 3B»), una sección ocupada
// que impide el atajo, el crecimiento desde la sección ocupada (también se llama «Mesa 3» si las une todas) y la
// administración del grupo con sus rechazos (nombre repetido, sección de otro grupo, grupo con mesas unidas, cambiar de
// área una sección). Usa su propia base desde cero.
func TestStageM1TableGroups(t *testing.T) {
	fixture := newStageM1Fixture(t)
	t.Cleanup(func() { expectStageM1Invariants(t, fixture.restaurantFixture) })
	route := "restaurant/table-groups"
	// Nombre repetido frente a una mesa del área (sin distinguir mayúsculas).
	fixture.reject(t, fixture.admin, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "mesa 1",
		"table_ids": fixture.ids("3A", "3B")}, 409, "TABLE_GROUP_NAME_TAKEN", "Ya hay una mesa o un grupo con ese nombre en esta área.")
	fixture.reject(t, fixture.admin, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "Mesa 3",
		"table_ids": append(fixture.ids("3A"), fixture.id("T1"))}, 409, "TABLE_UNION_AREA", "Las mesas deben ser de la misma área.")
	fixture.reject(t, fixture.admin, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "Mesa 3",
		"table_ids": fixture.ids("3A")}, 400, "OPERATION_VALIDATION", "")
	// Sin restaurant.tables.manage no se administra.
	if code := errorCode(t, fixture.call(t, fixture.joiner, "POST", route, map[string]any{"operation_id": randomID(),
		"area_id": fixture.salon.ID, "name": "Mesa 3", "table_ids": fixture.ids("3A", "3B", "3C")}, 403)); code != "PERMISSION_DENIED" {
		t.Fatalf("crear un grupo sin el permiso: %s", code)
	}
	group := decodeResponse[struct {
		Group struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			TableIDs []string `json:"table_ids"`
			Seats    int      `json:"seats"`
			Revision int      `json:"revision"`
		} `json:"group"`
	}](t, fixture.mutate(t, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "Mesa 3",
		"table_ids": fixture.ids("3A", "3B", "3C")}, 200)).Group
	if group.Name != "Mesa 3" || group.Seats != 12 || !slices.Equal(group.TableIDs, fixture.ids("3A", "3B", "3C")) {
		t.Fatalf("el grupo: %+v", group)
	}
	view := fixture.overview(t)
	if len(view.Groups) != 1 || view.Groups[0].ID != group.ID || fixture.table(t, "3B").GroupID == nil ||
		*fixture.table(t, "3B").GroupID != group.ID {
		t.Fatalf("el grupo en el mapa: %+v", view.Groups)
	}
	fixture.reject(t, fixture.admin, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "Otra U",
		"table_ids": fixture.ids("3C", "Mesa 12")}, 409, "TABLE_GROUP_SECTION_TAKEN", "La Mesa 3C ya es parte del grupo Mesa 3.")
	fixture.reject(t, fixture.admin, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "MESA 3",
		"table_ids": fixture.ids("Mesa 11", "Mesa 12")}, 409, "TABLE_GROUP_NAME_TAKEN", "")
	// Una sección no cambia de área.
	section := fixture.table(t, "3A")
	moved := section
	moved.AreaID = fixture.terraza.ID
	fixture.reject(t, fixture.admin, "PATCH", "restaurant/tables/"+section.ID, structMap(t, moved), 409,
		"TABLE_GROUP_SECTION_AREA", "Quita la mesa del grupo antes de cambiarla de área.")

	t.Run("el atajo une las tres secciones y se llama como el grupo", func(t *testing.T) {
		preview := decodeResponse[struct {
			Name     string                    `json:"name"`
			Seats    int                       `json:"seats"`
			Problems []operations.UnionProblem `json:"problems"`
		}](t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/preview", map[string]any{"group_id": group.ID}, 200))
		if preview.Name != "Mesa 3" || preview.Seats != 12 || len(preview.Problems) != 0 {
			t.Fatalf("vista previa del atajo: %+v", preview)
		}
		// Con todas libres, unir es diferido también con el grupo.
		fixture.reject(t, fixture.joiner, "POST", "restaurant/table-unions", map[string]any{"start_table_id": fixture.id("3A"),
			"group_id": group.ID}, 409, "TABLE_UNION_DEFERRED", "")
		bill := fixture.openAt(t, fixture.joiner, "3A", map[string]any{"union_group_id": group.ID, "seen_union_id": nil},
			draftLine(fixture.hamburguesa, 1))
		union := fixture.union(t, "3C")
		if union == nil || union.Name != "Mesa 3" || union.GroupID == nil || *union.GroupID != group.ID ||
			bill.Account.Name != "Mesa 3" || union.Seats != 12 {
			t.Fatalf("la unión del atajo: %+v; cuenta %s", union, bill.Account.Name)
		}
		// Un grupo con mesas unidas no se cambia ni se quita.
		fixture.reject(t, fixture.admin, "PATCH", route+"/"+group.ID, map[string]any{"revision": group.Revision, "name": "U"},
			409, "TABLE_GROUP_IN_USE", "El grupo tiene mesas unidas. Cámbialo cuando se liberen.")
		fixture.reject(t, fixture.admin, "PATCH", route+"/"+group.ID, map[string]any{"revision": group.Revision, "active": false},
			409, "TABLE_GROUP_IN_USE", "")
		bill = fixture.confirm(t, fixture.joiner, bill.Account.ID)
		fixture.pay(t, bill.Account.ID)
	})

	t.Run("combinación parcial", func(t *testing.T) {
		bill := fixture.deferred(t, "3A", "3B")
		union := fixture.union(t, "3A")
		if union == nil || union.Name != "3A + 3B" || union.GroupID != nil || bill.Account.Name != "3A + 3B" {
			t.Fatalf("3A + 3B: %+v", union)
		}
		if table := fixture.table(t, "3C"); table.State != "free" || table.UnionID != nil {
			t.Fatalf("la 3C debe seguir libre: %+v", table)
		}
		bill = fixture.confirm(t, fixture.joiner, bill.Account.ID)
		fixture.pay(t, bill.Account.ID)
	})

	t.Run("una sección ocupada impide el atajo y se crece desde ella", func(t *testing.T) {
		occupied := fixture.sent(t, "3B", draftLine(fixture.coca, 1))
		message := "La sección 3B está ocupada. Únela con las secciones libres para crecer."
		preview := decodeResponse[struct {
			Problems []operations.UnionProblem `json:"problems"`
		}](t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/preview", map[string]any{"group_id": group.ID}, 200))
		if len(preview.Problems) != 1 || preview.Problems[0].Code != "TABLE_GROUP_SECTION_OCCUPIED" ||
			preview.Problems[0].Message != message || *preview.Problems[0].TableID != fixture.id("3B") {
			t.Fatalf("vista previa con una sección ocupada: %+v", preview.Problems)
		}
		fixture.reject(t, fixture.joiner, "POST", "restaurant/accounts", map[string]any{"mode": "dine_in",
			"table_id": fixture.id("3A"), "union_group_id": group.ID, "items": []operations.DraftItem{draftLine(fixture.coca, 1)}},
			409, "TABLE_GROUP_SECTION_OCCUPIED", message)
		fixture.reject(t, fixture.joiner, "POST", "restaurant/table-unions", map[string]any{"start_table_id": fixture.id("3A"),
			"group_id": group.ID}, 409, "TABLE_GROUP_SECTION_OCCUPIED", message)
		// Crecer desde la sección ocupada hacia las libres: conserva sus cuentas y, con todas, se llama «Mesa 3».
		grown := fixture.joinNow(t, "3B", "3A", "3C")
		account := fixture.refresh(t, occupied.Account.ID).Account
		if grown.Name != "Mesa 3" || grown.GroupID == nil || *grown.GroupID != group.ID || account.Name != "Mesa 3" ||
			*account.TableID != fixture.id("3B") {
			t.Fatalf("crecer desde la 3B: %+v; cuenta %s", grown, account.Name)
		}
		// Quitar una sección: deja de ser el grupo completo.
		removed := decodeResponse[struct {
			Union operations.TableUnion `json:"union"`
		}](t, fixture.call(t, fixture.joiner, "POST", "restaurant/table-unions/"+grown.ID+"/tables/"+fixture.id("3C")+"/remove",
			map[string]any{"operation_id": randomID(), "revision": grown.Revision}, 200)).Union
		if removed.Name != "3A + 3B" || removed.GroupID != nil || fixture.refresh(t, occupied.Account.ID).Account.Name != "3A + 3B" {
			t.Fatalf("sin la 3C: %+v", removed)
		}
		fixture.pay(t, occupied.Account.ID)
	})

	t.Run("cambiar y quitar el grupo", func(t *testing.T) {
		current := fixture.overview(t).Groups[0]
		changed := decodeResponse[struct {
			Group struct {
				Name     string   `json:"name"`
				TableIDs []string `json:"table_ids"`
				Revision int      `json:"revision"`
			} `json:"group"`
		}](t, fixture.mutate(t, "PATCH", route+"/"+group.ID, map[string]any{"revision": current.Revision, "name": "Mesa en U",
			"table_ids": fixture.ids("3C", "3B", "3A")}, 200)).Group
		if changed.Name != "Mesa en U" || !slices.Equal(changed.TableIDs, fixture.ids("3C", "3B", "3A")) {
			t.Fatalf("grupo cambiado: %+v", changed)
		}
		fixture.reject(t, fixture.admin, "PATCH", route+"/"+group.ID, map[string]any{"revision": current.Revision,
			"name": "Otra"}, 409, "ACCOUNT_CONFLICT", "")
		fixture.mutate(t, "PATCH", route+"/"+group.ID, map[string]any{"revision": changed.Revision, "active": false}, 200)
		if len(fixture.overview(t).Groups) != 0 || fixture.table(t, "3A").GroupID != nil {
			t.Fatal("el grupo quitado se sigue leyendo")
		}
		// Su nombre queda libre y sus secciones pueden ser de otro grupo.
		fixture.mutate(t, "POST", route, map[string]any{"area_id": fixture.salon.ID, "name": "Mesa en U",
			"table_ids": fixture.ids("3A", "3B")}, 200)
		fixture.reject(t, fixture.admin, "PATCH", route+"/"+group.ID, map[string]any{"revision": changed.Revision + 1,
			"active": true}, 400, "OPERATION_VALIDATION", "")
	})
}
