package operations

import (
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
	"local/ad-el-pos/server/internal/tableunions"
)

// Uniones de mesas (etapa M1, §1 y §2; decisiones M1-D1 a M1-D10 y contrato de la etapa §2, §3 y §5). Todas las rutas
// van por Handle, con 7820148305 exclusivo tomado antes de leer nada (la vista previa, compartido), su idempotencia de
// restaurant_operations y el evento restaurant.table-unions.changed de la regla genérica. Go decide todo: el nombre,
// la capacidad, qué mesas se pueden unir y qué pasa con las cuentas; el escritorio y Android solo muestran.
//
// Rutas (restaurant/…):
//   - POST table-unions/preview: nombre, capacidad y problemas, sin escribir (restaurant.tables.join);
//   - POST table-unions: unir desde una mesa ocupada (sus cuentas quedan en la unión); con todas libres, 409
//     TABLE_UNION_DEFERRED: la unión nace con el primer producto en POST restaurant/accounts (accounts_write.go);
//   - POST table-unions/{id}/tables y …/tables/{table_id}/remove: agregar y quitar una mesa;
//   - GET table-unions/{id}: la unión (también deshecha) con sus mesas y su historial (restaurant.tables.read).

// maxUnionTables es el máximo de mesas de una unión o de un grupo.
const maxUnionTables = 40

// Códigos de error del contrato (§5).
const (
	codeUnionArea             = "TABLE_UNION_AREA"
	codeUnionOccupied         = "TABLE_UNION_OCCUPIED"
	codeUnionBothOccupied     = "TABLE_UNION_BOTH_OCCUPIED"
	codeUnionUnavailable      = "TABLE_UNION_UNAVAILABLE"
	codeTableInUnion          = "TABLE_IN_UNION"
	codeGroupSectionOccupied  = "TABLE_GROUP_SECTION_OCCUPIED"
	codeUnionNotOpen          = "TABLE_UNION_NOT_OPEN"
	codeUnionConflict         = "TABLE_UNION_CONFLICT"
	codeUnionDeferred         = "TABLE_UNION_DEFERRED"
	codeGroupNameTaken        = "TABLE_GROUP_NAME_TAKEN"
	codeGroupSectionTaken     = "TABLE_GROUP_SECTION_TAKEN"
	codeGroupInUse            = "TABLE_GROUP_IN_USE"
	codeGroupSectionAreaGuard = "TABLE_GROUP_SECTION_AREA"
)

// TableUnion es la unión del contrato (§2): en overview, restaurant/state y las respuestas de las rutas de uniones.
// state es el de una mesa (el más avanzado de sus cuentas) o «dissolved» si ya se deshizo.
type TableUnion struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	AreaID          string     `json:"area_id"`
	GroupID         *string    `json:"group_id"`
	TableIDs        []string   `json:"table_ids"`
	Seats           int        `json:"seats"`
	State           string     `json:"state"`
	AccountCount    int        `json:"account_count"`
	Balance         string     `json:"balance"`
	OccupiedSince   *time.Time `json:"occupied_since"`
	OccupiedSeconds int64      `json:"occupied_seconds"`
	LongOpen        bool       `json:"long_open"`
	Revision        int        `json:"revision"`
}

// TableUnionDetail es la respuesta de getTableUnion: la unión con sus mesas y su historial.
type TableUnionDetail struct {
	TableUnion
	Tables  []tableunions.NamedTable `json:"tables"`
	History []UnionHistoryEntry      `json:"history"`
}

// UnionHistoryEntry es un cambio de la unión: unir (una fila por mesa inicial), agregar, quitar o deshacer.
type UnionHistoryEntry struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	TableID    *string   `json:"table_id"`
	TableName  *string   `json:"table_name"`
	UserName   string    `json:"user_name"`
	DeviceName string    `json:"device_name"`
}

// UnionProblem es un motivo por el que no se puede unir, con el mismo código y texto que el rechazo (contrato §5).
type UnionProblem struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	TableID *string `json:"table_id"`
}

// unionRow es la fila de una unión con sus mesas (las actuales si está abierta; las que tenía al deshacerse, si no).
type unionRow struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	AreaID   string                   `json:"area_id"`
	GroupID  *string                  `json:"group_id"`
	State    string                   `json:"state"`
	Revision int                      `json:"revision"`
	Members  []tableunions.NamedTable `json:"members"`
}

const unionRowQuery = `SELECT jsonb_build_object('id',u.id,'name',u.name,'area_id',u.area_id,'group_id',u.group_id,
        'state',u.state,'revision',u.revision,
        'members',coalesce((SELECT jsonb_agg(jsonb_build_object('id',t.id,'name',t.name,'seats',t.seats))
            FROM app.table_union_members m JOIN app.restaurant_tables t ON t.id=m.table_id
            WHERE m.union_id=u.id AND CASE WHEN u.state='open' THEN m.left_at IS NULL ELSE m.left_at=u.dissolved_at END),
            '[]'::jsonb))
    FROM app.table_unions u`

// tableReference es «la Mesa 6» de los mensajes (M1-D6): el nombre con documents.TableContext, así que nunca dice
// «Mesa Mesa 6».
func tableReference(name string) string {
	return documents.TableContext(name)
}

// unionFrom arma la unión del contrato con su fila y la ocupación que Go calculó para sus mesas (tableQuery: todas las
// mesas de una unión abierta tienen la de la unión, M1-D10).
func unionFrom(row unionRow, occupancy *Table) TableUnion {
	members := slices.Clone(row.Members)
	tableunions.SortTables(members)
	union := TableUnion{ID: row.ID, Name: row.Name, AreaID: row.AreaID, GroupID: row.GroupID, TableIDs: []string{},
		Seats: tableunions.Seats(members), Revision: row.Revision, State: "dissolved", Balance: "0.00"}
	for _, table := range members {
		union.TableIDs = append(union.TableIDs, table.ID)
	}
	if row.State == "open" && occupancy != nil {
		union.State = occupancy.State
		union.AccountCount = occupancy.AccountCount
		union.Balance = occupancy.Balance
		union.OccupiedSince = occupancy.OccupiedSince
		union.OccupiedSeconds = occupancy.OccupiedSeconds
		union.LongOpen = occupancy.LongOpen
	}
	return union
}

// openUnions son las uniones abiertas (para overview y restaurant/state), con la ocupación de las mesas ya leídas.
func (handler *service) openUnions(tables []Table) ([]TableUnion, error) {
	rows, err := readList[unionRow](handler.ctx, handler.tx, unionRowQuery+" WHERE u.state='open' ORDER BY u.opened_at,u.id")
	if err != nil {
		return nil, err
	}
	byID := map[string]*Table{}
	for index := range tables {
		byID[tables[index].ID] = &tables[index]
	}
	unions := make([]TableUnion, 0, len(rows))
	for _, row := range rows {
		var occupancy *Table
		for _, member := range row.Members {
			if table := byID[member.ID]; table != nil {
				occupancy = table
				break
			}
		}
		unions = append(unions, unionFrom(row, occupancy))
	}
	return unions, nil
}

// readUnionRow lee una unión; 404 si no existe.
func (handler *service) readUnionRow(id string) (*unionRow, error) {
	row, err := readOne[unionRow](handler.ctx, handler.tx, unionRowQuery+" WHERE u.id=$1", id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "La unión de mesas no existe."}
	}
	return row, err
}

// readUnion lee una unión del contrato con su ocupación vigente.
func (handler *service) readUnion(id string) (*TableUnion, error) {
	row, err := handler.readUnionRow(id)
	if err != nil {
		return nil, err
	}
	var occupancy *Table
	if row.State == "open" && len(row.Members) > 0 {
		occupancy, err = readOne[Table](handler.ctx, handler.tx, tableQuery+" WHERE t.id=$1", row.Members[0].ID)
		if err != nil {
			return nil, err
		}
	}
	union := unionFrom(*row, occupancy)
	return &union, nil
}

// unionOfTable es la unión abierta de la mesa (nil si no está unida).
func (handler *service) unionOfTable(tableID string) (*TableUnion, error) {
	var unionID string
	err := handler.tx.QueryRow(handler.ctx, `SELECT union_id::text FROM app.table_union_members
        WHERE table_id=$1 AND left_at IS NULL`, tableID).Scan(&unionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return handler.readUnion(unionID)
}

// unionAccountIDs son las cuentas abiertas de las mesas de la unión, en orden de apertura.
func (handler *service) unionAccountIDs(unionID string) ([]string, error) {
	rows, err := handler.tx.Query(handler.ctx, `SELECT a.id::text FROM app.restaurant_accounts a
        JOIN app.table_union_members m ON m.table_id=a.table_id AND m.left_at IS NULL
        WHERE m.union_id=$1 AND a.state='open' ORDER BY a.created_at,a.id`, unionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// tableInUnion es el rechazo TABLE_IN_UNION (contrato §5): la mesa ya está en otra unión, la terminal no veía la unión
// vigente de la mesa o se quiere cambiar de área una mesa unida. Los detalles traen la unión y sus cuentas abiertas
// (identificadores), para «agregar a esa cuenta» o «abrir otra en la unión».
func (handler *service) tableInUnion(tableID, tableName string, union *TableUnion) error {
	accounts, err := handler.unionAccountIDs(union.ID)
	if err != nil {
		return err
	}
	return &Error{
		Status:  409,
		Code:    codeTableInUnion,
		Message: "La " + tableReference(tableName) + " ya está unida con otras mesas (" + union.Name + ").",
		Details: map[string]any{"table_id": tableID, "union_id": union.ID, "union": union, "accounts": accounts},
	}
}

// problemError convierte el primer problema en el rechazo 409 del contrato, con la mesa (y la unión) en los detalles.
func (handler *service) problemError(problem UnionProblem, tables []tableunions.MemberTable) error {
	details := map[string]any{"table_id": problem.TableID}
	if problem.Code == codeTableInUnion && problem.TableID != nil {
		for _, table := range tables {
			if table.ID == *problem.TableID && table.UnionID != nil {
				union, err := handler.readUnion(*table.UnionID)
				if err != nil {
					return err
				}
				return handler.tableInUnion(table.ID, table.Name, union)
			}
		}
	}
	return &Error{Status: 409, Code: problem.Code, Message: problem.Message, Details: details}
}

func problemFor(code, message string, table *tableunions.MemberTable) UnionProblem {
	problem := UnionProblem{Code: code, Message: message}
	if table != nil {
		id := table.ID
		problem.TableID = &id
	}
	return problem
}

func unavailableProblem(table tableunions.MemberTable) UnionProblem {
	return problemFor(codeUnionUnavailable, "La "+tableReference(table.Name)+" está fuera de servicio o desactivada.", &table)
}

func areaProblem(table tableunions.MemberTable) UnionProblem {
	return problemFor(codeUnionArea, "Las mesas deben ser de la misma área.", &table)
}

func inUnionProblem(table tableunions.MemberTable) UnionProblem {
	name := ""
	if table.UnionName != nil {
		name = *table.UnionName
	}
	return problemFor(codeTableInUnion, "La "+tableReference(table.Name)+" ya está unida con otras mesas ("+name+").", &table)
}

func occupiedProblem(table tableunions.MemberTable) UnionProblem {
	return problemFor(codeUnionOccupied, "Solo se pueden unir mesas libres: la "+tableReference(table.Name)+" ya está ocupada.",
		&table)
}

func bothOccupiedProblem(start, other tableunions.MemberTable) UnionProblem {
	return problemFor(codeUnionBothOccupied, "La "+tableReference(start.Name)+" y la "+tableReference(other.Name)+
		" tienen cuentas abiertas. Para juntar sus cuentas usa «Combinar cuentas».", &other)
}

func sectionOccupiedProblem(table tableunions.MemberTable) UnionProblem {
	return problemFor(codeGroupSectionOccupied, "La sección "+table.Name+
		" está ocupada. Únela con las secciones libres para crecer.", &table)
}

// joinMode dice cómo se arma la unión, para elegir el rechazo de una mesa ocupada.
type joinMode int

const (
	// joinNow: «Unir con…» desde una mesa (la de inicio puede estar ocupada; las que se agregan, libres).
	joinNow joinMode = iota
	// joinDeferred: la unión que nace con el primer producto: todas libres.
	joinDeferred
	// joinGroup: el atajo del grupo sin mesa de inicio (vista previa): ninguna sección ocupada.
	joinGroup
)

// joinProblems aplica las reglas de §1.1, §2.2 y §3 (M1-D6) a un conjunto de mesas: la de inicio (start, puede ser
// nil) y las que se agregan (others). Cada mesa da a lo más un problema, en este orden: fuera de servicio o
// desactivada, de otra área, ya unida y ocupada. union, si no es nil, es la unión abierta a la que se agregan (sus
// mesas no son problema). group indica que es el atajo de un grupo (una sección ocupada da su propio texto).
func joinProblems(start *tableunions.MemberTable, others []tableunions.MemberTable, areaID string, mode joinMode,
	group bool) []UnionProblem {
	problems := []UnionProblem{}
	if start != nil {
		switch {
		case !start.Available():
			problems = append(problems, unavailableProblem(*start))
		case start.UnionID != nil:
			problems = append(problems, inUnionProblem(*start))
		case mode == joinDeferred && start.AccountCount > 0:
			if group {
				problems = append(problems, sectionOccupiedProblem(*start))
			} else {
				problems = append(problems, occupiedProblem(*start))
			}
		}
	}
	startOccupied := start != nil && start.AccountCount > 0 && mode == joinNow
	for _, table := range others {
		switch {
		case !table.Available():
			problems = append(problems, unavailableProblem(table))
		case table.AreaID != areaID:
			problems = append(problems, areaProblem(table))
		case table.UnionID != nil:
			problems = append(problems, inUnionProblem(table))
		case table.AccountCount > 0 && startOccupied:
			problems = append(problems, bothOccupiedProblem(*start, table))
		case table.AccountCount > 0 && (group || mode == joinGroup):
			problems = append(problems, sectionOccupiedProblem(table))
		case table.AccountCount > 0:
			problems = append(problems, occupiedProblem(table))
		}
	}
	return problems
}

// validateTableIDs revisa una lista de mesas: identificadores válidos, sin repetir, entre min y maxUnionTables.
func validateTableIDs(ids []string, minimum int) error {
	if len(ids) < minimum || len(ids) > maxUnionTables {
		if minimum >= 2 {
			return invalid("Una unión necesita al menos dos mesas (hasta 40).")
		}
		return invalid("Indica de 1 a 40 mesas.")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if err := validateID(id); err != nil {
			return err
		}
		if seen[id] {
			return invalid("Una mesa está repetida en la unión.")
		}
		seen[id] = true
	}
	return nil
}

// readMemberTables lee las mesas indicadas; 400 si alguna no existe.
func (handler *service) readMemberTables(ids []string) ([]tableunions.MemberTable, error) {
	tables, err := tableunions.ReadTables(handler.ctx, handler.tx, ids)
	if err != nil {
		return nil, err
	}
	if len(tables) != len(ids) {
		return nil, invalid("Una de las mesas no existe.")
	}
	return tables, nil
}

// splitStart separa la mesa de inicio de las demás.
func splitStart(tables []tableunions.MemberTable, startID string) (*tableunions.MemberTable, []tableunions.MemberTable) {
	var start *tableunions.MemberTable
	others := []tableunions.MemberTable{}
	for index := range tables {
		if tables[index].ID == startID {
			table := tables[index]
			start = &table
			continue
		}
		others = append(others, tables[index])
	}
	return start, others
}

// groupTables lee un grupo activo y sus secciones; 404 si no existe o ya se quitó.
func (handler *service) groupTables(groupID string) (*tableunions.Group, []tableunions.MemberTable, error) {
	if err := validateID(groupID); err != nil {
		return nil, nil, err
	}
	group, active, err := tableunions.ReadGroup(handler.ctx, handler.tx, groupID)
	if err != nil {
		return nil, nil, err
	}
	if group == nil || !active {
		return nil, nil, &Error{Status: 404, Code: "NOT_FOUND", Message: "El grupo de mesas no existe."}
	}
	tables, err := handler.readMemberTables(group.TableIDs)
	if err != nil {
		return nil, nil, err
	}
	return group, tables, nil
}

// previewTableUnion (POST restaurant/table-unions/preview) calcula sin escribir el nombre, la capacidad y los problemas
// de una unión: {table_ids} (todas, la primera es la mesa de inicio), {group_id} (el atajo del grupo) o {union_id,
// table_ids} (las que se agregan a una unión abierta). problems vacío = se puede.
func (handler *service) previewTableUnion() (any, error) {
	var input struct {
		TableIDs []string `json:"table_ids"`
		GroupID  *string  `json:"group_id"`
		UnionID  *string  `json:"union_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	var all []tableunions.MemberTable
	problems := []UnionProblem{}
	switch {
	case input.UnionID != nil:
		if input.GroupID != nil {
			return nil, invalid("Indica la unión y las mesas que se agregan, sin grupo.")
		}
		if err := validateID(*input.UnionID); err != nil {
			return nil, err
		}
		if err := validateTableIDs(input.TableIDs, 1); err != nil {
			return nil, err
		}
		row, err := handler.readUnionRow(*input.UnionID)
		if err != nil {
			return nil, err
		}
		added, err := handler.readMemberTables(input.TableIDs)
		if err != nil {
			return nil, err
		}
		members, err := handler.readMemberTables(memberIDs(row.Members))
		if err != nil {
			return nil, err
		}
		if row.State != "open" {
			problems = append(problems, UnionProblem{Code: codeUnionNotOpen, Message: unionNotOpenMessage})
		}
		problems = append(problems, joinProblems(nil, added, row.AreaID, joinNow, false)...)
		all = append(members, added...)
	case input.GroupID != nil:
		if len(input.TableIDs) > 0 {
			return nil, invalid("Indica el grupo o las mesas, no los dos.")
		}
		group, tables, err := handler.groupTables(*input.GroupID)
		if err != nil {
			return nil, err
		}
		problems = joinProblems(nil, tables, group.AreaID, joinGroup, true)
		all = tables
	default:
		if err := validateTableIDs(input.TableIDs, 2); err != nil {
			return nil, err
		}
		tables, err := handler.readMemberTables(input.TableIDs)
		if err != nil {
			return nil, err
		}
		start, others := splitStart(tables, input.TableIDs[0])
		problems = joinProblems(start, others, start.AreaID, joinNow, false)
		all = tables
	}
	description, err := tableunions.Describe(handler.ctx, handler.tx, all)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": description.Name, "seats": description.Seats, "table_ids": description.TableIDs,
		"problems": problems}, nil
}

func memberIDs(members []tableunions.NamedTable) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.ID)
	}
	return ids
}

const unionNotOpenMessage = "Esta unión de mesas ya se deshizo."

func unionNotOpen() error {
	return &Error{Status: 409, Code: codeUnionNotOpen, Message: unionNotOpenMessage}
}

func unionConflict() error {
	return &Error{Status: 409, Code: codeUnionConflict, Message: "La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez."}
}

// unionActor es quien hace la operación, para el historial de la unión.
func (handler *service) unionActor() tableunions.Actor {
	return tableunions.Actor{UserID: handler.request.Actor.UserID, DeviceID: handler.request.Actor.DeviceID}
}

// insertUnion registra una unión nueva con sus mesas (ya validadas) y su historial: una fila «joined» por mesa, en el
// orden del nombre. El nombre, la capacidad y el grupo los calcula Go (M1-D2).
func (handler *service) insertUnion(tables []tableunions.MemberTable, areaID string) (string, tableunions.Description, error) {
	description, err := tableunions.Describe(handler.ctx, handler.tx, tables)
	if err != nil {
		return "", description, err
	}
	id := newID()
	actor := handler.unionActor()
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.table_unions(id,area_id,group_id,name,opened_by,opened_device_id)
        VALUES ($1,$2,$3,$4,$5,$6)`, id, areaID, description.GroupID, description.Name, actor.UserID, actor.DeviceID); err != nil {
		return "", description, err
	}
	for _, table := range description.Tables {
		if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.table_union_members(id,union_id,table_id,joined_by)
            VALUES ($1,$2,$3,$4)`, newID(), id, table.ID, actor.UserID); err != nil {
			return "", description, err
		}
	}
	for _, table := range description.Tables {
		tableID, tableName := table.ID, table.Name
		if err := tableunions.AppendHistory(handler.ctx, handler.tx, id, "joined", &tableID, &tableName, actor,
			handler.operationID); err != nil {
			return "", description, err
		}
	}
	return id, description, nil
}

// unionReference es la unión tal como queda en el historial de cada cuenta.
func unionReference(id, name string, tableIDs []string) map[string]any {
	return map[string]any{"id": id, "name": name, "table_ids": tableIDs}
}

// renameUnionAccounts aplica un cambio de la unión a sus cuentas abiertas (M1-D14): las que se llamaban como el lugar
// anterior (previousName: el nombre de la mesa al unir, el de la unión al agregar o quitar) toman el nombre nuevo, y
// cada cuenta abierta de la unión registra el cambio en su historial (account.table_union, con quién y la terminal) y
// publica su evento, que la pantalla de cocina usa para volver a leer el encabezado. No sube la revisión de la cuenta
// (tableunions.RestoreAccountNames explica por qué).
func (handler *service) renameUnionAccounts(unionID, previousName, name, change string, before, after map[string]any) error {
	accounts, err := handler.unionAccountIDs(unionID)
	if err != nil {
		return err
	}
	for _, accountID := range accounts {
		var previous string
		if err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&previous); err != nil {
			return err
		}
		current := previous
		if previous == previousName && previousName != name {
			current = name
			if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.restaurant_accounts SET name=$1,updated_at=now() WHERE id=$2",
				name, accountID); err != nil {
				return err
			}
		}
		if err := handler.unionAccountEvent(accountID, change, previous, current, before, after); err != nil {
			return err
		}
	}
	return nil
}

// unionAccountEvent registra el cambio de la unión en el historial de una cuenta abierta.
func (handler *service) unionAccountEvent(accountID, change, previousName, name string, before, after map[string]any) error {
	return handler.event(accountID, nil, tableunions.AccountKind,
		map[string]any{"union": before, "name": previousName},
		map[string]any{"change": change, "union": after, "name": name})
}

// createTableUnion (POST restaurant/table-unions) une mesas desde una mesa ocupada (§1.1): las cuentas de la mesa de
// inicio quedan en la unión y se conservan; las que se agregan deben estar libres. Con todas libres no guarda nada y
// responde TABLE_UNION_DEFERRED: la unión nace con el primer producto (M1-D9).
func (handler *service) createTableUnion() (any, error) {
	var input struct {
		OperationID  string   `json:"operation_id"`
		StartTableID string   `json:"start_table_id"`
		TableIDs     []string `json:"table_ids"`
		GroupID      *string  `json:"group_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.StartTableID); err != nil {
		return nil, err
	}
	var tables []tableunions.MemberTable
	group := input.GroupID != nil
	if group {
		if len(input.TableIDs) > 0 {
			return nil, invalid("Indica el grupo o las mesas, no los dos.")
		}
		_, sections, err := handler.groupTables(*input.GroupID)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(sections, func(table tableunions.MemberTable) bool { return table.ID == input.StartTableID }) {
			return nil, invalid("La mesa de inicio debe ser una sección del grupo.")
		}
		tables = sections
	} else {
		ids := append([]string{input.StartTableID}, input.TableIDs...)
		if err := validateTableIDs(ids, 2); err != nil {
			return nil, err
		}
		read, err := handler.readMemberTables(ids)
		if err != nil {
			return nil, err
		}
		tables = read
	}
	start, others := splitStart(tables, input.StartTableID)
	if problems := joinProblems(start, others, start.AreaID, joinNow, group); len(problems) > 0 {
		return nil, handler.problemError(problems[0], tables)
	}
	if start.AccountCount == 0 {
		return nil, &Error{Status: 409, Code: codeUnionDeferred, Message: "La unión se guarda con el primer producto.",
			Details: map[string]any{"table_id": start.ID}}
	}
	unionID, description, err := handler.insertUnion(tables, start.AreaID)
	if err != nil {
		return nil, err
	}
	after := unionReference(unionID, description.Name, description.TableIDs)
	if err := handler.renameUnionAccounts(unionID, start.Name, description.Name, "joined", nil, after); err != nil {
		return nil, err
	}
	union, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"union": union}, handler.audit(handler.ctx, handler.tx, "restaurant.table_union.joined",
		"table_union", &unionID, nil, union)
}

// deferredUnion crea la unión que nace con el primer producto (M1-D9, POST restaurant/accounts con union_table_ids o
// union_group_id): todas las mesas deben estar libres, del mismo área y sin unir. Si una se ocupó mientras tanto,
// responde 409 y la operación no guarda nada (la terminal conserva lo capturado).
func (handler *service) deferredUnion(input *OpenAccountInput) (string, error) {
	if err := handler.request.Actor.Require("restaurant.tables.join"); err != nil {
		return "", err
	}
	if input.Mode != "dine_in" || input.TableID == nil {
		return "", invalid("La unión de mesas necesita una cuenta de salón con su mesa de inicio.")
	}
	if err := validateID(*input.TableID); err != nil {
		return "", err
	}
	var tables []tableunions.MemberTable
	group := input.UnionGroupID != nil
	if group {
		if len(input.UnionTableIDs) > 0 {
			return "", invalid("Indica el grupo o las mesas, no los dos.")
		}
		_, sections, err := handler.groupTables(*input.UnionGroupID)
		if err != nil {
			return "", err
		}
		if !slices.ContainsFunc(sections, func(table tableunions.MemberTable) bool { return table.ID == *input.TableID }) {
			return "", invalid("La mesa de la cuenta debe ser una sección del grupo.")
		}
		tables = sections
	} else {
		ids := append([]string{*input.TableID}, input.UnionTableIDs...)
		if err := validateTableIDs(ids, 2); err != nil {
			return "", err
		}
		read, err := handler.readMemberTables(ids)
		if err != nil {
			return "", err
		}
		tables = read
	}
	start, others := splitStart(tables, *input.TableID)
	if problems := joinProblems(start, others, start.AreaID, joinDeferred, group); len(problems) > 0 {
		return "", handler.problemError(problems[0], tables)
	}
	unionID, _, err := handler.insertUnion(tables, start.AreaID)
	if err != nil {
		return "", err
	}
	union, err := handler.readUnion(unionID)
	if err != nil {
		return "", err
	}
	return unionID, handler.audit(handler.ctx, handler.tx, "restaurant.table_union.joined", "table_union", &unionID, nil, union)
}

// openUnionForChange lee una unión que se va a cambiar: abierta (TABLE_UNION_NOT_OPEN) y con la revisión que vio la
// terminal (TABLE_UNION_CONFLICT).
func (handler *service) openUnionForChange(unionID string, revision int) (*unionRow, error) {
	row, err := handler.readUnionRow(unionID)
	if err != nil {
		return nil, err
	}
	if row.State != "open" {
		return nil, unionNotOpen()
	}
	if row.Revision != revision {
		return nil, unionConflict()
	}
	return row, nil
}

// updateUnionName guarda el nombre y el grupo nuevos de la unión y sube su revisión.
func (handler *service) updateUnionName(unionID string, description tableunions.Description) error {
	_, err := handler.tx.Exec(handler.ctx, `UPDATE app.table_unions SET name=$2,group_id=$3,revision=revision+1,updated_at=now()
        WHERE id=$1 AND state='open'`, unionID, description.Name, description.GroupID)
	return err
}

// addTableToUnion (POST restaurant/table-unions/{id}/tables) agrega una mesa libre del mismo área a una unión abierta
// (§1.1). El nombre y la capacidad cambian; las cuentas que se llamaban como la unión toman el nombre nuevo.
func (handler *service) addTableToUnion(unionID string) (any, error) {
	var input struct {
		Mutation
		TableID string `json:"table_id"`
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(input.TableID); err != nil {
		return nil, err
	}
	row, err := handler.openUnionForChange(unionID, input.Revision)
	if err != nil {
		return nil, err
	}
	if len(row.Members)+1 > maxUnionTables {
		return nil, invalid("Una unión admite hasta 40 mesas.")
	}
	added, err := handler.readMemberTables([]string{input.TableID})
	if err != nil {
		return nil, err
	}
	if problems := joinProblems(nil, added, row.AreaID, joinNow, false); len(problems) > 0 {
		return nil, handler.problemError(problems[0], added)
	}
	members, err := handler.readMemberTables(memberIDs(row.Members))
	if err != nil {
		return nil, err
	}
	before, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	description, err := tableunions.Describe(handler.ctx, handler.tx, append(members, added...))
	if err != nil {
		return nil, err
	}
	actor := handler.unionActor()
	if _, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.table_union_members(id,union_id,table_id,joined_by)
        VALUES ($1,$2,$3,$4)`, newID(), unionID, added[0].ID, actor.UserID); err != nil {
		return nil, err
	}
	if err := handler.updateUnionName(unionID, description); err != nil {
		return nil, err
	}
	tableID, tableName := added[0].ID, added[0].Name
	if err := tableunions.AppendHistory(handler.ctx, handler.tx, unionID, "table_added", &tableID, &tableName, actor,
		handler.operationID); err != nil {
		return nil, err
	}
	if err := handler.renameUnionAccounts(unionID, row.Name, description.Name, "table_added",
		unionReference(unionID, row.Name, before.TableIDs),
		unionReference(unionID, description.Name, description.TableIDs)); err != nil {
		return nil, err
	}
	union, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"union": union}, handler.audit(handler.ctx, handler.tx, "restaurant.table_union.table_added",
		"table_union", &unionID, before, union)
}

// removeTableFromUnion (POST restaurant/table-unions/{id}/tables/{table_id}/remove) quita una mesa de la unión. Quitar
// una mesa no saca ninguna cuenta de la unión (§1.1, M1-D1): las cuentas de esa mesa pasan a la primera de las que
// quedan. Si queda una sola mesa, la unión se deshace y las cuentas quedan en esa mesa con su nombre.
func (handler *service) removeTableFromUnion(unionID, tableID string) (any, error) {
	var input Mutation
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	if err := validateID(tableID); err != nil {
		return nil, err
	}
	row, err := handler.openUnionForChange(unionID, input.Revision)
	if err != nil {
		return nil, err
	}
	members, err := handler.readMemberTables(memberIDs(row.Members))
	if err != nil {
		return nil, err
	}
	var removed *tableunions.MemberTable
	remaining := []tableunions.MemberTable{}
	for index := range members {
		if members[index].ID == tableID {
			table := members[index]
			removed = &table
			continue
		}
		remaining = append(remaining, members[index])
	}
	if removed == nil {
		return nil, invalid("La mesa no es parte de esta unión.")
	}
	before, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	tableunions.SortMembers(remaining)
	// Las cuentas de la mesa que sale pasan a la primera mesa que queda (en el orden del nombre): siguen en la unión.
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.restaurant_accounts SET table_id=$2,updated_at=now()
        WHERE table_id=$1 AND state='open'`, removed.ID, remaining[0].ID); err != nil {
		return nil, err
	}
	actor := handler.unionActor()
	if _, err := handler.tx.Exec(handler.ctx, `UPDATE app.table_union_members SET left_at=now(),left_by=$3
        WHERE union_id=$1 AND table_id=$2 AND left_at IS NULL`, unionID, removed.ID, actor.UserID); err != nil {
		return nil, err
	}
	removedID, removedName := removed.ID, removed.Name
	beforeReference := unionReference(unionID, row.Name, before.TableIDs)
	dissolved := len(remaining) < 2
	if dissolved {
		if err := tableunions.AppendHistory(handler.ctx, handler.tx, unionID, "table_removed", &removedID, &removedName, actor,
			handler.operationID); err != nil {
			return nil, err
		}
		if err := handler.dissolveWithAccounts(unionID, beforeReference); err != nil {
			return nil, err
		}
	} else {
		description, err := tableunions.Describe(handler.ctx, handler.tx, remaining)
		if err != nil {
			return nil, err
		}
		if err := handler.updateUnionName(unionID, description); err != nil {
			return nil, err
		}
		if err := tableunions.AppendHistory(handler.ctx, handler.tx, unionID, "table_removed", &removedID, &removedName, actor,
			handler.operationID); err != nil {
			return nil, err
		}
		if err := handler.renameUnionAccounts(unionID, row.Name, description.Name, "table_removed", beforeReference,
			unionReference(unionID, description.Name, description.TableIDs)); err != nil {
			return nil, err
		}
	}
	union, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"union": union, "dissolved": dissolved}, handler.audit(handler.ctx, handler.tx,
		"restaurant.table_union.table_removed", "table_union", &unionID, before, union)
}

// dissolveWithAccounts deshace una unión que todavía tiene cuentas abiertas (quedó con una sola mesa): sus cuentas que
// se llamaban como la unión vuelven al nombre de su mesa (como movedAccountName) y cada cuenta registra que la unión
// se deshizo. SettleEmpty deshace las que se quedan sin cuentas.
func (handler *service) dissolveWithAccounts(unionID string, before map[string]any) error {
	accounts, err := handler.unionAccountIDs(unionID)
	if err != nil {
		return err
	}
	names := map[string]string{}
	for _, accountID := range accounts {
		var name string
		if err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&name); err != nil {
			return err
		}
		names[accountID] = name
	}
	if _, err := tableunions.RestoreAccountNames(handler.ctx, handler.tx, unionID); err != nil {
		return err
	}
	if err := tableunions.Dissolve(handler.ctx, handler.tx, unionID, tableunions.ReasonSingleTable, handler.unionActor(),
		handler.operationID); err != nil {
		return err
	}
	for _, accountID := range accounts {
		var name string
		if err := handler.tx.QueryRow(handler.ctx, "SELECT name FROM app.restaurant_accounts WHERE id=$1", accountID).Scan(&name); err != nil {
			return err
		}
		if err := handler.unionAccountEvent(accountID, "dissolved", names[accountID], name, before, nil); err != nil {
			return err
		}
	}
	return nil
}

// getTableUnion (GET restaurant/table-unions/{id}) es la unión (también deshecha) con sus mesas y su historial.
func (handler *service) getTableUnion(unionID string) (any, error) {
	row, err := handler.readUnionRow(unionID)
	if err != nil {
		return nil, err
	}
	union, err := handler.readUnion(unionID)
	if err != nil {
		return nil, err
	}
	tables := slices.Clone(row.Members)
	tableunions.SortTables(tables)
	history, err := readList[UnionHistoryEntry](handler.ctx, handler.tx, `SELECT jsonb_build_object('at',e.created_at,
            'kind',e.kind,'table_id',e.table_id,'table_name',e.table_name,'user_name',u.display_name,'device_name',d.name)
        FROM app.table_union_events e JOIN app.users u ON u.id=e.author_id JOIN app.devices d ON d.id=e.device_id
        WHERE e.union_id=$1 ORDER BY e.sequence`, unionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"union": TableUnionDetail{TableUnion: *union, Tables: tables, History: history}}, nil
}

// tableUnions atiende restaurant/table-unions/….
func (handler *service) tableUnions(parts []string) (any, error) {
	switch {
	case len(parts) == 1:
		return handler.createTableUnion()
	case len(parts) == 2 && parts[1] == "preview":
		return handler.previewTableUnion()
	case len(parts) == 2:
		return handler.getTableUnion(parts[1])
	case len(parts) == 3:
		return handler.addTableToUnion(parts[1])
	case len(parts) == 5:
		return handler.removeTableFromUnion(parts[1], parts[3])
	}
	return nil, invalid("Operación de unión no disponible.")
}

// tableUnionPermission es el permiso de cada ruta de uniones; vacío si la ruta no existe.
func tableUnionPermission(method string, parts []string) string {
	switch {
	case method == "POST" && len(parts) == 1:
		return joinPermission
	case method == "POST" && len(parts) == 2 && parts[1] == "preview":
		return joinPermission
	case method == "GET" && len(parts) == 2 && parts[1] != "preview":
		return "restaurant.tables.read"
	case method == "POST" && len(parts) == 3 && parts[2] == "tables":
		return joinPermission
	case method == "POST" && len(parts) == 5 && parts[2] == "tables" && parts[4] == "remove":
		return joinPermission
	}
	return ""
}

// joinPermission es «Unir mesas» (M1-D8).
const joinPermission = "restaurant.tables.join"

// unionPreviewRoute reconoce la vista previa de una unión: no escribe, toma el lock compartido y no lleva clave de
// operación.
func unionPreviewRoute(method string, parts []string) bool {
	return method == "POST" && len(parts) == 2 && parts[0] == "table-unions" && parts[1] == "preview"
}
