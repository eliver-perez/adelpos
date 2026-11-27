package tableunions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Snapshot es la unión vigente de una mesa al reconocer una venta (M1-D3): su identificador, su nombre y sus mesas
// [{id, name}] en el orden del nombre. Es lo que se copia en sales_recognitions, como table_name.
type Snapshot struct {
	ID     string
	Name   string
	Tables json.RawMessage
}

// OpenSnapshot lee la unión abierta de la mesa; nil si la mesa no está unida (o no hay mesa).
func OpenSnapshot(ctx context.Context, tx pgx.Tx, tableID *string) (*Snapshot, error) {
	if tableID == nil {
		return nil, nil
	}
	var unionID, name string
	err := tx.QueryRow(ctx, `SELECT u.id::text,u.name FROM app.table_union_members m JOIN app.table_unions u ON u.id=m.union_id
        WHERE m.table_id=$1 AND m.left_at IS NULL AND u.state='open'`, *tableID).Scan(&unionID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tables, err := OpenMembers(ctx, tx, unionID)
	if err != nil {
		return nil, err
	}
	encoded := make([]map[string]string, 0, len(tables))
	for _, table := range tables {
		encoded = append(encoded, map[string]string{"id": table.ID, "name": table.Name})
	}
	raw, err := json.Marshal(encoded)
	if err != nil {
		return nil, err
	}
	return &Snapshot{ID: unionID, Name: name, Tables: raw}, nil
}

// OpenMembers son las mesas actuales de la unión, en el orden de su nombre.
func OpenMembers(ctx context.Context, tx pgx.Tx, unionID string) ([]NamedTable, error) {
	rows, err := tx.Query(ctx, `SELECT t.id::text,t.name,t.seats FROM app.table_union_members m
        JOIN app.restaurant_tables t ON t.id=m.table_id WHERE m.union_id=$1 AND m.left_at IS NULL`, unionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := []NamedTable{}
	for rows.Next() {
		var table NamedTable
		if err := rows.Scan(&table.ID, &table.Name, &table.Seats); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	SortTables(tables)
	return tables, nil
}
