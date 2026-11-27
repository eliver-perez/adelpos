package operations

import (
	"strings"

	"local/ad-el-pos/server/internal/tableunions"
)

// Mesas divisibles (etapa M1, §3; decisión M1-D4): un grupo predefinido («Mesa 3») sobre varias mesas normales del
// mismo área (sus secciones «3A», «3B» y «3C», que conservan su nombre y se abren, se cobran y se liberan por
// separado). Se administra en Áreas y distribución con restaurant.tables.manage:
//   - POST restaurant/table-groups {operation_id, area_id, name, table_ids (≥ 2, en orden)};
//   - PATCH restaurant/table-groups/{id} {revision, name?, table_ids?, active?}: active false lo quita.
//
// Los grupos activos salen en overview y restaurant/state (groups) y cada sección lleva su group_id. El atajo del grupo
// une todas sus secciones con el mecanismo de las uniones (table_unions.go).

// groupAudit es lo que queda del grupo en el Registro de movimientos.
type groupAudit struct {
	tableunions.Group
	Active bool `json:"active"`
}

// tableGroups atiende restaurant/table-groups.
func (handler *service) tableGroups(parts []string) (any, error) {
	if len(parts) == 1 {
		return handler.createTableGroup()
	}
	return handler.updateTableGroup(parts[1])
}

// validateGroupSections revisa las secciones de un grupo: de 2 a 40 mesas que existen, del área del grupo, que no son
// sección de otro grupo activo y que no están en una unión abierta (una unión abierta toma su nombre de los grupos).
func (handler *service) validateGroupSections(groupID, areaID string, ids []string) ([]tableunions.MemberTable, error) {
	if len(ids) < 2 || len(ids) > maxUnionTables {
		return nil, invalid("Un grupo necesita de 2 a 40 mesas.")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if err := validateID(id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, invalid("Una mesa está repetida en el grupo.")
		}
		seen[id] = true
	}
	tables, err := handler.readMemberTables(ids)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if table.AreaID != areaID {
			id := table.ID
			return nil, &Error{Status: 409, Code: codeUnionArea, Message: "Las mesas deben ser de la misma área.",
				Details: map[string]any{"table_id": id}}
		}
	}
	for _, table := range tables {
		if table.GroupID != nil && *table.GroupID != groupID {
			var name string
			if err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.table_groups WHERE id=$1", *table.GroupID).Scan(&name); err != nil {
				return nil, err
			}
			return nil, &Error{Status: 409, Code: codeGroupSectionTaken,
				Message: "La " + tableReference(table.Name) + " ya es parte del grupo " + name + ".",
				Details: map[string]any{"table_id": table.ID, "group_id": *table.GroupID}}
		}
	}
	for _, table := range tables {
		if table.UnionID != nil {
			return nil, groupInUse(table.ID)
		}
	}
	return tables, nil
}

func groupInUse(tableID string) error {
	return &Error{Status: 409, Code: codeGroupInUse, Message: "El grupo tiene mesas unidas. Cámbialo cuando se liberen.",
		Details: map[string]any{"table_id": tableID}}
}

// validateGroupName revisa el nombre: de 1 a 60 caracteres y sin repetir en el área (sin distinguir mayúsculas) el de
// una mesa o el de otro grupo activo (TABLE_GROUP_NAME_TAKEN).
func (handler *service) validateGroupName(groupID, areaID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return "", invalid("Indica un nombre de grupo de hasta 60 caracteres.")
	}
	var taken bool
	err := handler.tx.QueryRow(handler.ctx, `SELECT EXISTS(SELECT FROM app.restaurant_tables t
            WHERE t.area_id=$1 AND lower(t.name)=lower($2))
        OR EXISTS(SELECT FROM app.table_groups g WHERE g.area_id=$1 AND g.active AND lower(g.name)=lower($2)
            AND g.id::text<>$3)`, areaID, name, groupID).Scan(&taken)
	if err != nil {
		return "", err
	}
	if taken {
		return "", &Error{Status: 409, Code: codeGroupNameTaken, Message: "Ya hay una mesa o un grupo con ese nombre en esta área."}
	}
	return name, nil
}

// writeSections reemplaza las secciones del grupo por las indicadas, en su orden.
func (handler *service) writeSections(groupID string, ids []string) error {
	if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.table_group_sections WHERE group_id=$1", groupID); err != nil {
		return err
	}
	for position, id := range ids {
		if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.table_group_sections(group_id,table_id,position)
            VALUES ($1,$2,$3)`, groupID, id, position); err != nil {
			return err
		}
	}
	return nil
}

// readGroupAudit lee el grupo para la respuesta y el Registro de movimientos.
func (handler *service) readGroupAudit(groupID string) (*groupAudit, error) {
	group, active, err := tableunions.ReadGroup(handler.ctx, handler.tx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "El grupo de mesas no existe."}
	}
	return &groupAudit{Group: *group, Active: active}, nil
}

// createTableGroup (POST restaurant/table-groups) crea un grupo con sus secciones en orden.
func (handler *service) createTableGroup() (any, error) {
	var input struct {
		OperationID string   `json:"operation_id"`
		AreaID      string   `json:"area_id"`
		Name        string   `json:"name"`
		TableIDs    []string `json:"table_ids"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.AreaID); err != nil {
		return nil, err
	}
	area, err := readOne[Area](handler.ctx, handler.tx, areaQuery+" WHERE a.id=$1", input.AreaID)
	if err != nil {
		return nil, err
	}
	if !area.Active {
		return nil, invalid("Activa el área antes de configurar sus mesas.")
	}
	id := newID()
	name, err := handler.validateGroupName(id, area.ID, input.Name)
	if err != nil {
		return nil, err
	}
	if _, err := handler.validateGroupSections(id, area.ID, input.TableIDs); err != nil {
		return nil, err
	}
	if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.table_groups(id,area_id,name) VALUES ($1,$2,$3)",
		id, area.ID, name); err != nil {
		return nil, err
	}
	if err := handler.writeSections(id, input.TableIDs); err != nil {
		return nil, err
	}
	saved, err := handler.readGroupAudit(id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"group": saved.Group}, handler.audit(handler.ctx, handler.tx, "restaurant.table_group.saved",
		"table_group", &id, nil, saved)
}

// updateTableGroup (PATCH restaurant/table-groups/{id}) cambia el nombre, las secciones o lo quita (active false). No
// se cambia ni se quita un grupo con una sección en una unión abierta (TABLE_GROUP_IN_USE); un grupo ya quitado no se
// vuelve a usar (se crea otro).
func (handler *service) updateTableGroup(groupID string) (any, error) {
	var input struct {
		OperationID string    `json:"operation_id"`
		Revision    int       `json:"revision"`
		Name        *string   `json:"name"`
		TableIDs    *[]string `json:"table_ids"`
		Active      *bool     `json:"active"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	before, err := handler.readGroupAudit(groupID)
	if err != nil {
		return nil, err
	}
	if !before.Active {
		return nil, invalid("Este grupo ya se quitó. Crea uno nuevo en Áreas y distribución.")
	}
	if before.Revision != input.Revision {
		return nil, conflict()
	}
	sections, err := handler.readMemberTables(before.TableIDs)
	if err != nil {
		return nil, err
	}
	for _, table := range sections {
		if table.UnionID != nil {
			return nil, groupInUse(table.ID)
		}
	}
	if input.Active != nil && !*input.Active {
		if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.table_group_sections WHERE group_id=$1", groupID); err != nil {
			return nil, err
		}
		if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.table_groups SET active=false,removed_at=now(),revision=revision+1,
            updated_at=now() WHERE id=$1`, groupID); err != nil {
			return nil, err
		}
		after := *before
		after.Active = false
		after.Revision++
		return map[string]any{"group": after.Group}, handler.audit(handler.ctx, handler.tx, "restaurant.table_group.removed",
			"table_group", &groupID, before, after)
	}
	name := before.Name
	if input.Name != nil {
		name, err = handler.validateGroupName(groupID, before.AreaID, *input.Name)
		if err != nil {
			return nil, err
		}
	}
	if input.TableIDs != nil {
		if _, err := handler.validateGroupSections(groupID, before.AreaID, *input.TableIDs); err != nil {
			return nil, err
		}
		if err := handler.writeSections(groupID, *input.TableIDs); err != nil {
			return nil, err
		}
	}
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.table_groups SET name=$2,revision=revision+1,updated_at=now()
        WHERE id=$1`, groupID, name); err != nil {
		return nil, err
	}
	saved, err := handler.readGroupAudit(groupID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"group": saved.Group}, handler.audit(handler.ctx, handler.tx, "restaurant.table_group.saved",
		"table_group", &groupID, before, saved)
}
