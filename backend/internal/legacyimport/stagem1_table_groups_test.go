package legacyimport

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// stageM1Package es el paquete sintético de siempre con las mesas de la etapa M1 (§3 y §7; M1-D5): «Mesa 3» con «3a»,
// «3b» y «3c» (la base se vuelve el grupo y una venta suya conserva el nombre), «Mesa 7a» a «Mesa 7d» sin «Mesa 7»
// (como las del propietario) y tres casos dudosos: «Mesa 12a» y «Mesa 12c» (falta la b), «Mesa 15b» (una sola
// sección) y «Mesa 20a» y «Mesa 20b» con «Mesa 20» y «20» (dos mesas con el número). Todo es inventado.
func stageM1Package() *syntheticPackage {
	pkg := standardSynthetic()
	tables := pkg.table("mesas")
	rows := []struct {
		id   int
		name string
	}{
		{10, "Mesa 7a"}, {11, "3a"}, {12, "Mesa 3"}, {13, "3b"}, {14, "Mesa 12a"}, {15, "3c"}, {16, "Mesa 7b"},
		{17, "Mesa 7c"}, {18, "Mesa 7d"}, {19, "Mesa 12c"}, {20, "Mesa 15b"}, {21, "Mesa 20a"}, {22, "Mesa 20b"},
		{23, "Mesa 20"}, {24, "20"},
	}
	for _, row := range rows {
		tables.add(map[string]any{"id": row.id, "mesa": row.name, "capacidad": 2})
	}
	// Una venta de la «Mesa 3» (la que se vuelve el grupo): su historial conserva el nombre.
	ventas := pkg.table("ventas")
	column := slices.Index(ventas.columns, "mesa")
	for _, row := range ventas.rows {
		if number, ok := row[column].(json.Number); ok && number.String() == "2" {
			row[column] = json.Number("12")
			break
		}
	}
	return pkg
}

func TestStageM1LegacyTableGroups(t *testing.T) {
	model := loadSynthetic(t, stageM1Package())
	plan := BuildPlan(model, map[string]bool{"admin": true})
	groups := plan.Catalog.TableGroups
	if len(groups) != 2 {
		t.Fatalf("grupos: %+v", groups)
	}
	byGroup := map[string]PlannedTableGroup{}
	for _, group := range groups {
		byGroup[group.Name] = group
	}
	three, seven := byGroup["Mesa 3"], byGroup["Mesa 7"]
	if three.Name != "Mesa 3" || three.Key != "3" || three.Base == nil || three.Base.ID != 12 ||
		!slices.Equal(three.SectionNames, []string{"3a", "3b", "3c"}) {
		t.Fatalf("el grupo con base: %+v", three)
	}
	if seven.Name != "Mesa 7" || seven.Base != nil || !slices.Equal(seven.SectionNames, []string{"Mesa 7a", "Mesa 7b", "Mesa 7c", "Mesa 7d"}) {
		t.Fatalf("el grupo sin base: %+v", seven)
	}
	// La base no es una mesa viva; las dudosas sí, con su nombre.
	live := map[string]PlannedTable{}
	for _, table := range plan.Catalog.Tables {
		live[table.Name] = table
	}
	if _, found := live["Mesa 3"]; found {
		t.Fatal("la «Mesa 3» se creó como mesa viva")
	}
	for _, name := range []string{"Mesa 12a", "Mesa 12c", "Mesa 15b", "Mesa 20a", "Mesa 20b", "Mesa 20", "20"} {
		if _, found := live[name]; !found {
			t.Fatalf("la mesa dudosa %s no se importó", name)
		}
	}
	if len(plan.Catalog.AbsorbedTables) != 1 || plan.Catalog.AbsorbedTables[0].Name != "Mesa 3" {
		t.Fatalf("mesas que se volvieron grupo: %+v", plan.Catalog.AbsorbedTables)
	}
	// Las secciones juntas, en un mismo renglón y en el orden de sus letras.
	for _, group := range groups {
		first := live[group.SectionNames[0]]
		for index, name := range group.SectionNames {
			table := live[name]
			if table.AreaKey != first.AreaKey || table.Y != first.Y || table.X != first.X+index*tableCellX {
				t.Fatalf("la sección %s no quedó junto a las demás: %+v (primera %+v)", name, table, first)
			}
		}
	}
	// Las notas lo dicen en lenguaje sencillo: dos grupos, la base y tres dudosos.
	codes := map[string]int{}
	messages := []string{}
	for _, note := range plan.Catalog.Notes {
		codes[note.Code]++
		messages = append(messages, note.Message)
	}
	if codes["table_group"] != 2 || codes["table_group_base"] != 1 || codes["table_group_doubtful"] != 3 {
		t.Fatalf("notas: %v\n%s", codes, strings.Join(messages, "\n"))
	}
	joined := strings.Join(messages, "\n")
	for _, text := range []string{
		"Las mesas «3a», «3b» y «3c» quedaron juntas como la mesa divisible «Mesa 3».",
		"La mesa «Mesa 3» del sistema anterior ahora es la mesa divisible",
		"Las mesas «Mesa 12a» y «Mesa 12c» parecen secciones de una misma mesa",
		"La mesa «Mesa 15b» parece la sección de una mesa divisible, pero es la única con el número 15.",
		"Hay más de una mesa con el número 20 («Mesa 20» y «20»)",
	} {
		if !strings.Contains(joined, text) {
			t.Fatalf("falta la nota %q en:\n%s", text, joined)
		}
	}
	// La mesa que se volvió grupo no se cuenta como mesa en la validación.
	location := mustLocation(t)
	expected := computeExpected(model, func(instant time.Time) int { return instant.In(location).Year() })
	if expected.Counts[CountTables] != int64(len(plan.Catalog.Tables)) {
		t.Fatalf("mesas esperadas %d, en el plan %d", expected.Counts[CountTables], len(plan.Catalog.Tables))
	}
	// Sin grupos, el acomodo es el de siempre: la posición i en la cuadrícula.
	plain := BuildPlan(loadSynthetic(t, standardSynthetic()), map[string]bool{"admin": true})
	for index, table := range plain.Catalog.Tables {
		if table.X != (index%tablesPerRow)*tableCellX+tableMarginX || table.Y != (index/tablesPerRow)*tableCellY+tableMarginY {
			t.Fatalf("acomodo sin grupos cambió: %+v", table)
		}
	}
	if len(plain.Catalog.TableGroups) != 0 {
		t.Fatal("el paquete de siempre no tiene grupos")
	}
}

// TestStageM1LegacyGroupsWrapRow: un grupo que no cabe en lo que queda del renglón empieza uno nuevo.
func TestStageM1LegacyGroupsWrapRow(t *testing.T) {
	tables := []DiningTable{}
	for id := int32(2); id <= 8; id++ {
		tables = append(tables, DiningTable{ID: id, Name: "Mesa " + string(rune('0'+id))})
	}
	tables = append(tables, DiningTable{ID: 20, Name: "Mesa 30a"}, DiningTable{ID: 21, Name: "Mesa 30b"},
		DiningTable{ID: 22, Name: "Mesa 30c"})
	plan := CatalogPlan{}
	plan.buildTables(&Model{Tables: tables})
	byName := map[string]PlannedTable{}
	for _, table := range plan.Tables {
		byName[table.Name] = table
	}
	first := byName["Mesa 30a"]
	if first.X != tableMarginX || first.Y != tableCellY+tableMarginY || byName["Mesa 30c"].X != 2*tableCellX+tableMarginX {
		t.Fatalf("el grupo debía empezar el segundo renglón: %+v", plan.Tables)
	}
}
