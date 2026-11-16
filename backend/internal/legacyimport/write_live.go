package legacyimport

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Escritura de lo vivo (catálogo, mesas y usuarios) con los locks 7820148305 → 7820148304 → 7820148308 exclusivos ya
// tomados. Cada cosa importada se escribe con el identificador ya resuelto: nueva la primera vez y, al repetir la
// migración, la misma fila actualizada en su lugar (así se conservan, por ejemplo, la contraseña asignada a un usuario
// importado o una receta ligada a un producto importado). Lo del lote anterior que ya no viene en el paquete se borra
// si nada lo usa o, si algo lo usa, se desactiva y queda en la correspondencia como «retired».

// liveTable describe una entidad viva con nombre único, para el renombre en dos fases al repetir la migración (dos
// nombres que se intercambian entre respaldos no deben chocar a mitad de la actualización).
type liveTable struct {
	entity     string
	table      string
	nameColumn string
	maxName    int
	deactivate string
	cleanup    []string
}

var liveTables = map[string]liveTable{
	MapCategory: {entity: MapCategory, table: "catalog_categories", nameColumn: "name", maxName: 100,
		deactivate: "active = false, revision = revision + 1, updated_at = now()"},
	MapProduct: {entity: MapProduct, table: "catalog_products", nameColumn: "", maxName: 100,
		deactivate: "active = false, archived = true, revision = revision + 1, updated_at = now()",
		cleanup: []string{"DELETE FROM app.catalog_group_defaults WHERE product_id = $1",
			"DELETE FROM app.catalog_product_groups WHERE product_id = $1",
			"DELETE FROM app.catalog_price_entries WHERE product_id = $1"}},
	MapComplement: {entity: MapComplement, table: "catalog_complements", nameColumn: "name", maxName: 100,
		deactivate: "active = false, revision = revision + 1, updated_at = now()"},
	MapGroup: {entity: MapGroup, table: "catalog_option_groups", nameColumn: "name", maxName: 100,
		deactivate: "active = false, revision = revision + 1, updated_at = now()"},
	MapGroupOption: {entity: MapGroupOption, table: "catalog_group_options", nameColumn: "", maxName: 0,
		deactivate: "active = false"},
	MapArea: {entity: MapArea, table: "restaurant_areas", nameColumn: "name", maxName: 100,
		deactivate: "active = false, revision = revision + 1, updated_at = now()"},
	MapTable: {entity: MapTable, table: "restaurant_tables", nameColumn: "name", maxName: 60,
		deactivate: "active = false, revision = revision + 1, updated_at = now()"},
	MapUser: {entity: MapUser, table: "users", nameColumn: "username", maxName: 40,
		deactivate: "active = false, updated_at = now()",
		cleanup:    []string{"DELETE FROM app.user_preferences WHERE user_id = $1"}},
}

// temporaryName es un nombre único y válido para el renombre en dos fases (usuarios: «m» y los 32 dígitos del
// identificador, que cumple la regla de nombres de usuario).
func temporaryName(entity string, id pgtype.UUID) string {
	text := uuidText(id)
	if entity == MapUser {
		compact := ""
		for _, character := range text {
			if character != '-' {
				compact += string(character)
			}
		}
		return "m-" + compact
	}
	return "migracion-" + text
}

// RetiredEntry es algo de un lote anterior que ya no viene en el paquete y no se pudo borrar.
type RetiredEntry struct {
	Entity string
	Key    string
	ID     pgtype.UUID
}

// liveResult es lo que se escribió en lo vivo.
type liveResult struct {
	Retired []RetiredEntry
	Deleted map[string]int
}

// replaceLive escribe el catálogo, las mesas y los usuarios del plan con los identificadores resueltos, reemplazando
// los del lote anterior (previous).
func replaceLive(ctx context.Context, tx pgx.Tx, plan *CatalogPlan, ids IDMap, previous IDMap) (liveResult, error) {
	result := liveResult{Deleted: map[string]int{}}
	// Fase 1: lo que ya existía del lote anterior toma un nombre temporal (los nombres finales se fijan después). Se
	// guardan los nombres que tenía para lo que haya que retirar.
	original := map[[16]byte]string{}
	for _, entity := range mapEntities {
		descriptor := liveTables[entity]
		if descriptor.nameColumn == "" || len(previous[entity]) == 0 {
			continue
		}
		for _, id := range previous[entity] {
			var name string
			err := tx.QueryRow(ctx, fmt.Sprintf(`WITH old AS (SELECT %[2]s AS name FROM app.%[1]s WHERE id = $1 FOR UPDATE)
                UPDATE app.%[1]s SET %[2]s = $2 FROM old WHERE app.%[1]s.id = $1 RETURNING old.name`,
				descriptor.table, descriptor.nameColumn), id, temporaryName(entity, id)).Scan(&name)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return result, err
			}
			original[id.Bytes] = name
		}
	}
	// Las relaciones de producto con grupos y sus cantidades incluidas se vuelven a escribir completas.
	for _, id := range previous[MapProduct] {
		for _, statement := range []string{"DELETE FROM app.catalog_group_defaults WHERE product_id = $1",
			"DELETE FROM app.catalog_product_groups WHERE product_id = $1"} {
			if _, err := tx.Exec(ctx, statement, id); err != nil {
				return result, err
			}
		}
	}
	if err := writeCatalog(ctx, tx, plan, ids); err != nil {
		return result, err
	}
	if err := writeTables(ctx, tx, plan, ids); err != nil {
		return result, err
	}
	if err := writeUsers(ctx, tx, plan, ids); err != nil {
		return result, err
	}
	// Fase 2: lo del lote anterior que ya no viene en el paquete, en orden inverso de dependencias.
	order := []string{MapUser, MapTable, MapArea, MapGroupOption, MapGroup, MapComplement, MapProduct, MapCategory}
	for _, entity := range order {
		for key, id := range previous[entity] {
			if current := ids.get(entity, key); current.Valid {
				continue
			}
			deleted, err := retire(ctx, tx, liveTables[entity], id, original[id.Bytes])
			if err != nil {
				return result, err
			}
			if deleted {
				result.Deleted[entity]++
				continue
			}
			result.Retired = append(result.Retired, RetiredEntry{Entity: entity, Key: key, ID: id})
		}
	}
	return result, nil
}

// retire borra una fila viva de un lote anterior si nada la usa (en un punto de guardado: un rechazo por llave
// foránea no aborta la importación) o, si algo la usa, la desactiva con un nombre que no estorba.
func retire(ctx context.Context, tx pgx.Tx, descriptor liveTable, id pgtype.UUID, name string) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	deleteErr := func() error {
		for _, statement := range descriptor.cleanup {
			if _, err := savepoint.Exec(ctx, statement, id); err != nil {
				return err
			}
		}
		_, err := savepoint.Exec(ctx, fmt.Sprintf("DELETE FROM app.%s WHERE id = $1", descriptor.table), id)
		return err
	}()
	if deleteErr == nil {
		return true, savepoint.Commit(ctx)
	}
	if err := savepoint.Rollback(ctx); err != nil {
		return false, err
	}
	var problem *pgconn.PgError
	if !errors.As(deleteErr, &problem) || problem.Code != "23503" {
		return false, deleteErr
	}
	if descriptor.nameColumn != "" {
		// El nombre temporal de la fase 1 se cambia por uno legible y único: «<nombre> (retirado 1a2b3c4d)» (para un
		// usuario, «<usuario>.retirado-1a2b3c4d», que cumple la regla de nombres de usuario).
		tag := uuidText(id)[:8]
		retiredName := truncateRunes(name, descriptor.maxName-20) + " (retirado " + tag + ")"
		if descriptor.entity == MapUser {
			retiredName = truncateRunes(name, descriptor.maxName-18) + ".retirado-" + tag
			if !usernamePattern.MatchString(retiredName) {
				retiredName = "retirado-" + tag
			}
		}
		_, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE app.%[1]s SET %[2]s = $2 WHERE id = $1`, descriptor.table,
			descriptor.nameColumn), id, retiredName)
		if err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE app.%s SET %s WHERE id = $1", descriptor.table, descriptor.deactivate), id); err != nil {
		return false, err
	}
	if descriptor.entity == MapUser {
		if _, err := tx.Exec(ctx, `UPDATE app.sessions SET revoked_at = now(), revoke_reason = 'user_disabled'
            WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
			return false, err
		}
	}
	return false, nil
}

// writeCatalog escribe categorías, productos, complementos, grupos, opciones, relaciones y cantidades incluidas.
// Precio = el más reciente del anterior; tasa de IVA = la de ese precio (decisión 1); «El precio puede bajar al
// cambiar complementos» en los productos con complementos incluidos (decisión 3). Sin presentaciones, combos ni
// imágenes (el anterior no los tiene o no viajan).
func writeCatalog(ctx context.Context, tx pgx.Tx, plan *CatalogPlan, ids IDMap) error {
	for _, category := range plan.Categories {
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_categories(id, name, sort_order, active)
            VALUES ($1, $2, $3, true)
            ON CONFLICT (id) DO UPDATE SET name = excluded.name, sort_order = excluded.sort_order, active = true,
                revision = app.catalog_categories.revision + 1, updated_at = now()`,
			ids.get(MapCategory, category.Key), category.Name, category.SortOrder)
		if err != nil {
			return err
		}
	}
	for _, product := range plan.Products {
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_products(id, category_id, name, description, code, kind, base_price,
                active, archived, sort_order, tax_rate, price_can_decrease)
            VALUES ($1, $2, $3, $4, $5, 'product', $6, $7, false, $8, $9, $10)
            ON CONFLICT (id) DO UPDATE SET category_id = excluded.category_id, name = excluded.name,
                description = excluded.description, code = excluded.code, kind = 'product', base_price = excluded.base_price,
                active = excluded.active, archived = false, sort_order = excluded.sort_order, tax_rate = excluded.tax_rate,
                price_can_decrease = excluded.price_can_decrease, revision = app.catalog_products.revision + 1,
                updated_at = now()`,
			ids.get(MapProduct, product.Key), ids.get(MapCategory, product.CategoryKey), product.Name, product.Description,
			product.Code, product.BasePrice.Numeric(), product.Active, product.SortOrder, int32(product.TaxRate),
			product.PriceCanDecrease)
		if err != nil {
			return err
		}
	}
	for _, complement := range plan.Complements {
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_complements(id, name, active) VALUES ($1, $2, true)
            ON CONFLICT (id) DO UPDATE SET name = excluded.name, active = true,
                revision = app.catalog_complements.revision + 1, updated_at = now()`,
			ids.get(MapComplement, complement.Key), complement.Name)
		if err != nil {
			return err
		}
	}
	for _, group := range plan.Groups {
		groupID := ids.get(MapGroup, group.Key)
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_option_groups(id, name, pricing_mode, min_quantity, max_quantity, active)
            VALUES ($1, $2, 'value', 0, NULL, true)
            ON CONFLICT (id) DO UPDATE SET name = excluded.name, pricing_mode = 'value', min_quantity = 0, max_quantity = NULL,
                active = true, revision = app.catalog_option_groups.revision + 1, updated_at = now()`,
			groupID, group.Name)
		if err != nil {
			return err
		}
		for _, option := range group.Options {
			_, err := tx.Exec(ctx, `INSERT INTO app.catalog_group_options(id, group_id, complement_id, portion_id, reference_value,
                    extra_price, max_quantity, active)
                VALUES ($1, $2, $3, NULL, $4, 0, NULL, true)
                ON CONFLICT (id) DO UPDATE SET group_id = excluded.group_id, complement_id = excluded.complement_id,
                    portion_id = NULL, reference_value = excluded.reference_value, extra_price = 0, max_quantity = NULL,
                    active = true`,
				ids.get(MapGroupOption, option.Key), groupID, ids.get(MapComplement, option.ComplementKey),
				option.ReferenceValue.Numeric())
			if err != nil {
				return err
			}
		}
	}
	for _, relation := range plan.ProductGroups {
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_product_groups(product_id, group_id, min_quantity, max_quantity)
            VALUES ($1, $2, NULL, NULL)`, ids.get(MapProduct, relation.ProductKey), ids.get(MapGroup, relation.GroupKey))
		if err != nil {
			return err
		}
	}
	for _, included := range plan.Defaults {
		_, err := tx.Exec(ctx, `INSERT INTO app.catalog_group_defaults(product_id, group_id, option_id, quantity)
            VALUES ($1, $2, $3, $4)`, ids.get(MapProduct, included.ProductKey), ids.get(MapGroup, included.GroupKey),
			ids.get(MapGroupOption, included.OptionKey), included.Quantity.Numeric())
		if err != nil {
			return err
		}
	}
	return nil
}

// writeTables escribe las áreas «Salón» y sus mesas en la cuadrícula.
func writeTables(ctx context.Context, tx pgx.Tx, plan *CatalogPlan, ids IDMap) error {
	for _, area := range plan.Areas {
		_, err := tx.Exec(ctx, `INSERT INTO app.restaurant_areas(id, name, sort_order, active) VALUES ($1, $2, $3, true)
            ON CONFLICT (id) DO UPDATE SET name = excluded.name, sort_order = excluded.sort_order, active = true,
                revision = app.restaurant_areas.revision + 1, updated_at = now()`,
			ids.get(MapArea, area.Key), area.Name, area.SortOrder)
		if err != nil {
			return err
		}
	}
	for _, table := range plan.Tables {
		_, err := tx.Exec(ctx, `INSERT INTO app.restaurant_tables(id, area_id, name, x, y, width, height, seats, active)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true)
            ON CONFLICT (id) DO UPDATE SET area_id = excluded.area_id, name = excluded.name, x = excluded.x, y = excluded.y,
                width = excluded.width, height = excluded.height, seats = excluded.seats, active = true,
                revision = app.restaurant_tables.revision + 1, updated_at = now()`,
			ids.get(MapTable, table.Key), ids.get(MapArea, table.AreaKey), table.Name, table.X, table.Y, tableWidth,
			tableHeight, table.Seats)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeUsers escribe los usuarios sin contraseña (password_hash nulo: sin acceso hasta «Asignar contraseña»). Al
// repetir la migración se conservan la contraseña que ya se hubiera asignado y su marca de contraseña inicial; un
// usuario que el anterior tenía inactivo pierde sus sesiones.
func writeUsers(ctx context.Context, tx pgx.Tx, plan *CatalogPlan, ids IDMap) error {
	for _, user := range plan.Users {
		id := ids.get(MapUser, user.Key)
		_, err := tx.Exec(ctx, `INSERT INTO app.users(id, username, display_name, password_hash, role_id, active, initial_password)
            VALUES ($1, $2, $3, NULL, (SELECT id FROM app.roles WHERE code = $4), $5, false)
            ON CONFLICT (id) DO UPDATE SET username = excluded.username, display_name = excluded.display_name,
                role_id = excluded.role_id, active = excluded.active, updated_at = now()`,
			id, user.Username, user.DisplayName, user.RoleCode, user.Active)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO app.user_preferences(user_id, theme)
            SELECT $1, coalesce((SELECT theme FROM app.establishment LIMIT 1), 'light') ON CONFLICT (user_id) DO NOTHING`, id)
		if err != nil {
			return err
		}
		if !user.Active {
			if _, err := tx.Exec(ctx, `UPDATE app.sessions SET revoked_at = now(), revoke_reason = 'user_disabled'
                WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeIDMap guarda la correspondencia del lote nuevo: lo importado («active») y lo retirado de lotes anteriores que
// no se pudo borrar («retired»).
func writeIDMap(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, ids IDMap, retired []RetiredEntry) error {
	rows := [][]any{}
	for _, entity := range mapEntities {
		for key, id := range ids[entity] {
			rows = append(rows, []any{batch, entity, key, id, "active"})
		}
	}
	for _, entry := range retired {
		rows = append(rows, []any{batch, entry.Entity, entry.Key, entry.ID, "retired"})
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"app", "legacy_id_map"}, []string{"batch_id", "entity", "legacy_key", "new_id", "state"},
		pgx.CopyFromRows(rows))
	return err
}
