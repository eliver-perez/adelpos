package legacyimport

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Mesas divisibles del sistema anterior (etapa M1, §3; decisión del coordinador M1-D5). El anterior no tiene ninguna
// idea de grupos: «mesas» es una lista plana. Lo que se convierte en una mesa divisible se reconoce solo por el nombre:
//
//   - Hay grupo cuando hay al menos dos mesas «Mesa N»+letra o «N»+letra del mismo N, con letras únicas y seguidas desde
//     la a (sin distinguir mayúsculas). Se crea el grupo «Mesa N» («N» si ninguna sección lleva la palabra) con las
//     secciones en el orden de las letras, sin renombrarlas, y se acomodan juntas en la cuadrícula (en un mismo renglón
//     si caben).
//   - Si además existe la mesa «Mesa N» (o «N»), esa mesa se vuelve el grupo, como dice la §3: no se crea como mesa
//     viva, el grupo toma su nombre y sus ventas siguen en el historial (app.legacy_sales) con el nombre copiado.
//   - Se reporta como dudoso, sin crear el grupo: letras repetidas o que no van seguidas desde la a, una sola sección o
//     más de una mesa con el mismo número. El propietario arma el grupo a mano en Áreas y distribución.
//
// Una segunda importación vuelve a calcular los grupos del sistema anterior (source legacy); los que armó el
// propietario se conservan.

// PlannedTableGroup es un grupo del sistema anterior: su clave (el número), su nombre, el área de sus secciones y las
// secciones en el orden de sus letras.
type PlannedTableGroup struct {
	Key          string
	Name         string
	AreaKey      string
	TableKeys    []string
	SectionNames []string
	// Base es la mesa «Mesa N» que se volvió el grupo (nil si no había).
	Base *DiningTable
}

// legacyTablePattern reconoce «Mesa 3», «mesa 3a», «3», «3A» (con el nombre ya sin acentos ni mayúsculas): la palabra
// opcional, el número y la letra opcional.
var legacyTablePattern = regexp.MustCompile(`^(mesa\s+)?(\d+)\s?([a-z])?$`)

// detectedGroup es un grupo reconocido en las mesas del paquete, con sus secciones en el orden de las letras.
type detectedGroup struct {
	number   string
	name     string
	sections []DiningTable
	base     *DiningTable
}

// tableGrouping es lo que se reconoció: los grupos, las mesas que se volvieron grupo (absorbed), de qué grupo es cada
// sección y las notas para el resumen y el informe.
type tableGrouping struct {
	groups    []detectedGroup
	absorbed  map[int32]bool
	sectionOf map[int32]int
	notes     []PlanNote
}

// legacyTableName es el nombre con que se importa una mesa (el mismo de buildTables).
func legacyTableName(table DiningTable) string {
	return cleanName(table.Name, 60, fmt.Sprintf("Mesa %d", table.ID))
}

// detectTableGroups aplica la regla a las mesas del paquete (sin «para llevar»), en el orden del paquete. Es una
// función pura: la usan el plan (buildTables) y los conteos esperados (una mesa que se volvió grupo no se cuenta).
func detectTableGroups(tables []DiningTable) tableGrouping {
	type parsed struct {
		table  DiningTable
		word   bool
		letter string
	}
	sections := map[string][]parsed{}
	bases := map[string][]DiningTable{}
	order := []string{}
	for _, table := range tables {
		match := legacyTablePattern.FindStringSubmatch(foldText(legacyTableName(table)))
		if match == nil {
			continue
		}
		number := strings.TrimLeft(match[2], "0")
		if number == "" {
			number = "0"
		}
		if _, seen := sections[number]; !seen {
			if _, seenBase := bases[number]; !seenBase {
				order = append(order, number)
			}
		}
		if match[3] == "" {
			bases[number] = append(bases[number], table)
			continue
		}
		sections[number] = append(sections[number], parsed{table: table, word: match[1] != "", letter: match[3]})
	}
	grouping := tableGrouping{absorbed: map[int32]bool{}, sectionOf: map[int32]int{}, notes: []PlanNote{}}
	for _, number := range order {
		candidates := sections[number]
		if len(candidates) == 0 {
			continue
		}
		names := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			names = append(names, legacyTableName(candidate.table))
		}
		if len(candidates) == 1 {
			grouping.notes = append(grouping.notes, PlanNote{Code: "table_group_doubtful", Message: fmt.Sprintf(
				"La mesa «%s» parece la sección de una mesa divisible, pero es la única con el número %s. Quedó como una mesa "+
					"normal; si es parte de una mesa divisible, arma el grupo en Áreas y distribución.", names[0], number)})
			continue
		}
		sort.SliceStable(candidates, func(left, right int) bool { return candidates[left].letter < candidates[right].letter })
		consecutive := true
		for index, candidate := range candidates {
			if candidate.letter != string(rune('a'+index)) {
				consecutive = false
			}
		}
		if !consecutive {
			grouping.notes = append(grouping.notes, PlanNote{Code: "table_group_doubtful", Message: fmt.Sprintf(
				"Las mesas %s parecen secciones de una misma mesa, pero sus letras se repiten o no van seguidas desde la a. "+
					"Quedaron como mesas normales; si son una mesa divisible, arma el grupo en Áreas y distribución.",
				quotedList(names))})
			continue
		}
		if len(bases[number]) > 1 {
			baseNames := []string{}
			for _, base := range bases[number] {
				baseNames = append(baseNames, legacyTableName(base))
			}
			grouping.notes = append(grouping.notes, PlanNote{Code: "table_group_doubtful", Message: fmt.Sprintf(
				"Hay más de una mesa con el número %s (%s), así que las mesas %s quedaron como mesas normales. Si son una mesa "+
					"divisible, arma el grupo en Áreas y distribución.", number, quotedList(baseNames), quotedList(names))})
			continue
		}
		group := detectedGroup{number: number}
		withWord := false
		for _, candidate := range candidates {
			group.sections = append(group.sections, candidate.table)
			withWord = withWord || candidate.word
		}
		group.name = number
		if withWord {
			group.name = "Mesa " + number
		}
		if len(bases[number]) == 1 {
			base := bases[number][0]
			group.base = &base
			group.name = legacyTableName(base)
			grouping.absorbed[base.ID] = true
		}
		index := len(grouping.groups)
		for _, section := range group.sections {
			grouping.sectionOf[section.ID] = index
		}
		grouping.groups = append(grouping.groups, group)
		grouping.notes = append(grouping.notes, PlanNote{Code: "table_group", Message: fmt.Sprintf(
			"Las mesas %s quedaron juntas como la mesa divisible «%s». Cada sección se abre y se cobra por separado, y «%s» "+
				"las une de un toque.", quotedList(names), group.name, group.name)})
		if group.base != nil {
			grouping.notes = append(grouping.notes, PlanNote{Code: "table_group_base", Message: fmt.Sprintf(
				"La mesa «%s» del sistema anterior ahora es la mesa divisible: ya no es una mesa aparte y sus ventas "+
					"anteriores siguen en el historial con su nombre.", group.name)})
		}
	}
	return grouping
}

// quotedList es «A», «B» y «C».
func quotedList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, "«"+name+"»")
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " y " + quoted[len(quoted)-1]
}

// writeTableGroups vuelve a calcular los grupos del sistema anterior en lo vivo: los del lote anterior se quitan (sus
// secciones se borran y el grupo queda inactivo, como «Quitar» en Áreas y distribución) y se crean los del plan con sus
// secciones. Los grupos que armó el propietario se conservan; si uno ya usa el nombre o una sección de un grupo del
// sistema anterior en la misma área, ese grupo del anterior no se crea. Corre con los locks de la operación ya tomados y
// sobre una base sin operación propia (clean.go), así que no hay uniones abiertas.
func writeTableGroups(ctx context.Context, tx pgx.Tx, plan *CatalogPlan, ids IDMap) error {
	if _, err := tx.Exec(ctx, `DELETE FROM app.table_group_sections WHERE group_id IN (
            SELECT id FROM app.table_groups WHERE source='legacy')`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE app.table_groups SET active=false,removed_at=now(),revision=revision+1,updated_at=now()
        WHERE source='legacy' AND active`); err != nil {
		return err
	}
	for _, group := range plan.TableGroups {
		areaID := ids.get(MapArea, group.AreaKey)
		tableIDs := []string{}
		for _, key := range group.TableKeys {
			tableIDs = append(tableIDs, uuidText(ids.get(MapTable, key)))
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM app.table_groups g WHERE g.area_id=$1 AND g.active
                AND lower(g.name)=lower($2))
            OR EXISTS(SELECT FROM app.table_group_sections s JOIN app.table_groups g ON g.id=s.group_id
                WHERE g.active AND s.table_id::text = ANY($3::text[]))`, areaID, group.Name, tableIDs).Scan(&taken); err != nil {
			return err
		}
		if taken {
			continue
		}
		groupID := newUUID()
		if _, err := tx.Exec(ctx, `INSERT INTO app.table_groups(id,area_id,name,source,legacy_batch_id,legacy_key)
            VALUES ($1,$2,$3,'legacy',current_setting('adel.legacy_import')::uuid,$4)`, groupID, areaID, group.Name,
			group.Key); err != nil {
			return err
		}
		for position, key := range group.TableKeys {
			if _, err := tx.Exec(ctx, `INSERT INTO app.table_group_sections(group_id,table_id,position) VALUES ($1,$2,$3)`,
				groupID, ids.get(MapTable, key), position); err != nil {
				return err
			}
		}
	}
	return nil
}

// trimManualGroups quita los grupos del propietario que la importación dejó con menos de dos secciones o con secciones
// de otra área (sus mesas del sistema anterior se retiraron o cambiaron de área).
func trimManualGroups(ctx context.Context, tx pgx.Tx) error {
	broken := `SELECT g.id FROM app.table_groups g WHERE g.active AND g.source='manual' AND (
            (SELECT count(*) FROM app.table_group_sections s WHERE s.group_id=g.id) < 2
            OR EXISTS(SELECT FROM app.table_group_sections s JOIN app.restaurant_tables t ON t.id=s.table_id
                WHERE s.group_id=g.id AND t.area_id<>g.area_id))`
	if _, err := tx.Exec(ctx, `UPDATE app.table_groups SET active=false,removed_at=now(),revision=revision+1,updated_at=now()
        WHERE id IN (`+broken+`)`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM app.table_group_sections s USING app.table_groups g
        WHERE g.id=s.group_id AND NOT g.active`)
	return err
}

// legacyGroupKey es la clave de un grupo del sistema anterior (su número).
func legacyGroupKey(number string) string {
	if value, err := strconv.Atoi(number); err == nil {
		return strconv.Itoa(value)
	}
	return number
}
