package legacyimport

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Entidades de legacy_id_map.
const (
	MapCategory    = "category"
	MapProduct     = "product"
	MapComplement  = "complement"
	MapGroup       = "option_group"
	MapGroupOption = "group_option"
	MapArea        = "area"
	MapTable       = "table"
	MapUser        = "user"
)

// mapEntities en el orden en que se reemplazan.
var mapEntities = []string{MapCategory, MapProduct, MapComplement, MapGroup, MapGroupOption, MapArea, MapTable, MapUser}

// newUUID crea un identificador aleatorio (versión 4).
func newUUID() pgtype.UUID {
	var id pgtype.UUID
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		panic(err)
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	id.Valid = true
	return id
}

// uuidText escribe un identificador como texto.
func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	value := hex.EncodeToString(id.Bytes[:])
	return value[0:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:32]
}

// parseUUID lee un identificador de texto.
func parseUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(text)
	return id, err
}

// IDMap es la correspondencia clave anterior → identificador de AD-EL POS por entidad.
type IDMap map[string]map[string]pgtype.UUID

func (ids IDMap) get(entity, key string) pgtype.UUID {
	return ids[entity][key]
}

func (ids IDMap) set(entity, key string, id pgtype.UUID) {
	if ids[entity] == nil {
		ids[entity] = map[string]pgtype.UUID{}
	}
	ids[entity][key] = id
}

// loadIDMap lee la correspondencia de un lote (vacía sin lote).
func loadIDMap(ctx context.Context, tx pgx.Tx, batchID string) (IDMap, error) {
	ids := IDMap{}
	if batchID == "" {
		return ids, nil
	}
	rows, err := tx.Query(ctx, `SELECT entity, legacy_key, new_id FROM app.legacy_id_map WHERE batch_id = $1`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entity, key string
		var id pgtype.UUID
		if err := rows.Scan(&entity, &key, &id); err != nil {
			return nil, err
		}
		ids.set(entity, key, id)
	}
	return ids, rows.Err()
}

// resolveIDs asigna un identificador a cada cosa que el plan importa a lo vivo: el mismo del lote anterior si la clave
// ya estaba (se actualiza en su lugar al repetir la migración) o uno nuevo.
func resolveIDs(plan *CatalogPlan, previous IDMap) IDMap {
	ids := IDMap{}
	assign := func(entity, key string) {
		if id := previous.get(entity, key); id.Valid {
			ids.set(entity, key, id)
			return
		}
		ids.set(entity, key, newUUID())
	}
	for _, category := range plan.Categories {
		assign(MapCategory, category.Key)
	}
	for _, product := range plan.Products {
		assign(MapProduct, product.Key)
	}
	for _, complement := range plan.Complements {
		assign(MapComplement, complement.Key)
	}
	for _, group := range plan.Groups {
		assign(MapGroup, group.Key)
		for _, option := range group.Options {
			assign(MapGroupOption, option.Key)
		}
	}
	for _, area := range plan.Areas {
		assign(MapArea, area.Key)
	}
	for _, table := range plan.Tables {
		assign(MapTable, table.Key)
	}
	for _, user := range plan.Users {
		assign(MapUser, user.Key)
	}
	return ids
}
