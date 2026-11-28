package tableunions

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"
)

// MemberTable es una mesa con lo que deciden las reglas de unir (§1.1 y §2.2): su área, si está disponible, sus cuentas
// abiertas, la unión abierta en la que ya está y el grupo activo del que es sección.
type MemberTable struct {
	NamedTable
	AreaID       string  `json:"area_id"`
	Active       bool    `json:"active"`
	OutOfService bool    `json:"out_of_service"`
	AreaActive   bool    `json:"area_active"`
	AccountCount int     `json:"account_count"`
	UnionID      *string `json:"union_id"`
	UnionName    *string `json:"union_name"`
	GroupID      *string `json:"group_id"`
}

// Available indica si la mesa se puede usar: activa, en servicio y en un área activa.
func (table MemberTable) Available() bool {
	return table.Active && !table.OutOfService && table.AreaActive
}

// ReadTables lee las mesas indicadas con su estado para las reglas de unir, en el orden de su nombre (SortTables). Una
// mesa que no existe no aparece: quien llama compara la cantidad. account_count son las cuentas abiertas con table_id
// en esa mesa (no las de su unión): «ocupada» para unir es tener cuentas propias o estar en una unión.
func ReadTables(ctx context.Context, tx pgx.Tx, ids []string) ([]MemberTable, error) {
	rows, err := tx.Query(ctx, `SELECT jsonb_build_object('id',t.id,'name',t.name,'seats',t.seats,'area_id',t.area_id,
            'active',t.active,'out_of_service',t.out_of_service,'area_active',area.active,
            'account_count',(SELECT count(*) FROM app.restaurant_accounts a WHERE a.table_id=t.id AND a.state='open'),
            'union_id',`+UnionIDSQL("t.id")+`,'union_name',`+UnionNameSQL("t.id")+`,'group_id',`+GroupIDSQL("t.id")+`)
        FROM app.restaurant_tables t JOIN app.restaurant_areas area ON area.id=t.area_id
        WHERE t.id::text = ANY($1::text[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := []MemberTable{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var table MemberTable
		if err := json.Unmarshal(encoded, &table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	SortMembers(tables)
	return tables, nil
}

// SortMembers ordena mesas leídas con ReadTables en el orden natural de su nombre.
func SortMembers(tables []MemberTable) {
	named := make([]NamedTable, len(tables))
	byID := make(map[string]MemberTable, len(tables))
	for index, table := range tables {
		named[index] = table.NamedTable
		byID[table.ID] = table
	}
	SortTables(named)
	for index, table := range named {
		tables[index] = byID[table.ID]
	}
}

// Group es un grupo predefinido activo con sus secciones en orden.
type Group struct {
	ID       string   `json:"id"`
	AreaID   string   `json:"area_id"`
	Name     string   `json:"name"`
	TableIDs []string `json:"table_ids"`
	Seats    int      `json:"seats"`
	Revision int      `json:"revision"`
}

// groupsQuery lee los grupos activos con sus secciones en orden y la capacidad sumada.
const groupsQuery = `SELECT jsonb_build_object('id',g.id,'area_id',g.area_id,'name',g.name,'revision',g.revision,
        'table_ids',coalesce((SELECT jsonb_agg(s.table_id ORDER BY s.position) FROM app.table_group_sections s
            WHERE s.group_id=g.id),'[]'::jsonb),
        'seats',coalesce((SELECT sum(t.seats) FROM app.table_group_sections s JOIN app.restaurant_tables t ON t.id=s.table_id
            WHERE s.group_id=g.id),0))
    FROM app.table_groups g`

// ActiveGroups son los grupos activos, por área y nombre.
func ActiveGroups(ctx context.Context, tx pgx.Tx) ([]Group, error) {
	return readGroups(ctx, tx, groupsQuery+" WHERE g.active ORDER BY g.area_id,lower(g.name),g.id")
}

// ReadGroup lee un grupo (activo o no) por su identificador; nil si no existe.
func ReadGroup(ctx context.Context, tx pgx.Tx, id string) (*Group, bool, error) {
	rows, err := tx.Query(ctx, groupsQuery+" WHERE g.id::text=$1", id)
	if err != nil {
		return nil, false, err
	}
	groups, err := collectGroups(rows)
	if err != nil || len(groups) == 0 {
		return nil, false, err
	}
	var active bool
	if err := tx.QueryRow(ctx, "SELECT active FROM app.table_groups WHERE id::text=$1", id).Scan(&active); err != nil {
		return nil, false, err
	}
	return &groups[0], active, nil
}

func readGroups(ctx context.Context, tx pgx.Tx, query string, arguments ...any) ([]Group, error) {
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	return collectGroups(rows)
}

func collectGroups(rows pgx.Rows) ([]Group, error) {
	defer rows.Close()
	groups := []Group{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var group Group
		if err := json.Unmarshal(encoded, &group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// Description es el nombre, la capacidad y el grupo de un conjunto de mesas, como los calcula Go (M1-D2).
type Description struct {
	Name     string       `json:"name"`
	Seats    int          `json:"seats"`
	TableIDs []string     `json:"table_ids"`
	GroupID  *string      `json:"group_id"`
	Tables   []NamedTable `json:"tables"`
}

// Describe nombra un conjunto de mesas ya leídas: en orden natural, con la capacidad sumada y, si son exactamente todas
// las secciones de un grupo activo, con el nombre y el identificador del grupo.
func Describe(ctx context.Context, tx pgx.Tx, tables []MemberTable) (Description, error) {
	named := make([]NamedTable, 0, len(tables))
	for _, table := range tables {
		named = append(named, table.NamedTable)
	}
	SortTables(named)
	description := Description{Name: UnionName(named), Seats: Seats(named), TableIDs: []string{}, Tables: named}
	for _, table := range named {
		description.TableIDs = append(description.TableIDs, table.ID)
	}
	if len(tables) == 0 || tables[0].GroupID == nil {
		return description, nil
	}
	groupID := *tables[0].GroupID
	for _, table := range tables {
		if table.GroupID == nil || *table.GroupID != groupID {
			return description, nil
		}
	}
	group, active, err := ReadGroup(ctx, tx, groupID)
	if err != nil || group == nil || !active {
		return description, err
	}
	if len(group.TableIDs) != len(tables) {
		return description, nil
	}
	for _, id := range group.TableIDs {
		if !slices.Contains(description.TableIDs, id) {
			return description, nil
		}
	}
	description.Name = group.Name
	description.GroupID = &group.ID
	return description, nil
}
