package catalog

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Catálogo descargable por versión (etapa 9G, §5.3) para la futura aplicación Android (y cualquier terminal que
// quiera guardar el catálogo). La versión sale de app.catalog_change_log (migración 0023): cada transacción que
// cambia el catálogo toma un número nuevo y lo deja en cada registro que tocó. GET catalog/sync entrega el catálogo
// completo y GET catalog/sync?since=X solo lo que cambió después de la versión X, con las bajas.
//
// Reglas de la descarga:
//   - Los precios son los de la lista predeterminada, calculados en Go con las reglas de catalog.Calculate (la misma
//     función effectivePrice que usan la cotización y la confirmación); la tasa de IVA efectiva de cada producto es
//     la propia o la general. Ningún cliente calcula precios: los que muestra son estos y el servidor vuelve a
//     calcular al recibir un pedido (pedido capturado sin conexión, §5.5).
//   - La disponibilidad (activo, «Pausar», horarios y «Agotado») va calculada al momento de la descarga en
//     available_now y sold_out; los horarios cambian con la hora sin cambiar la versión, así que el cliente solo los
//     usa para mostrar y el servidor decide al recibir el pedido.
//   - Los productos archivados no van en la descarga completa; en la de cambios llegan con archived = true, y los
//     registros borrados, en deleted. El cliente los quita.
//   - Si cambió la lista predeterminada o la tasa general de IVA, la descarga de cambios incluye todos los productos
//     vigentes y, además, los archivados que cambiaron (revisión de la fase D, hallazgo 1); si cambió una categoría,
//     también sus productos (su disponibilidad depende de ella). Un producto que cambió (agotado, inactivo, en otra
//     categoría…) trae también los combos que lo usan y los grupos cuyos complementos lo usan, con la disponibilidad
//     de cada componente y opción (hallazgo 5, sync_dependents.go).
//   - La descarga de cambios exige la época de la anterior (catalog_epoch); con otra época se entrega todo.
//   - Las imágenes no viajan en la descarga: solo su referencia (id y checksum). Se descargan aparte con
//     GET catalog/images/{id}/content, con caché por ETag.
//   - Requiere catalog.read y catalog.prices.read. Lee con el lock del catálogo compartido (Catalog.Handle), así que
//     la versión y los registros son consistentes entre sí.

// taxEntityID es el identificador fijo con que la bitácora anota los cambios de la tasa general de IVA.
const taxEntityID = "00000000-0000-0000-0000-000000000000"

// Epoch devuelve la época del catálogo (revisión de la fase D, hallazgo 2): un identificador que cambia cuando la base
// deja de ser la misma línea de tiempo aunque los números de versión se repitan. Combina el identificador guardado
// en app.catalog_epoch (0023) con la identidad de la base (su OID) y del clúster (system_identifier): restaurar un
// respaldo crea la base de nuevo (DROP/CREATE en el comando restore) o la lleva a otro clúster, así que la época
// cambia sola, sin depender de que cada procedimiento de restauración se acuerde de cambiarla. Mover los datos a
// otro clúster (la carpeta del sistema de la fase A) también la cambia: los clientes descargan todo una vez.
func Epoch(ctx context.Context, tx pgx.Tx) (string, error) {
	var epoch string
	err := tx.QueryRow(ctx, `SELECT md5(e.epoch::text||':'||d.oid::text||':'||s.system_identifier::text)::uuid::text
        FROM app.catalog_epoch e
        CROSS JOIN pg_database d
        CROSS JOIN pg_control_system() s
        WHERE d.datname=current_database()`).Scan(&epoch)
	return epoch, err
}

// Version devuelve la versión vigente del catálogo descargable. Toma el lock del catálogo compartido para no leer
// una versión mayor mientras otra transacción con un número menor sigue sin confirmarse.
func Version(ctx context.Context, tx pgx.Tx) (int64, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7820148304)"); err != nil {
		return 0, err
	}
	return currentVersion(ctx, tx)
}

// currentVersion lee la versión con el lock del catálogo ya tomado.
func currentVersion(ctx context.Context, tx pgx.Tx) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, "SELECT coalesce(max(version),0) FROM app.catalog_change_log").Scan(&version)
	return version, err
}

// syncChanges son los registros que cambiaron después de una versión, por entidad, y los que se borraron.
type syncChanges struct {
	changed map[string][]string
	deleted []map[string]string
}

func readChanges(ctx context.Context, tx pgx.Tx, since int64) (syncChanges, error) {
	changes := syncChanges{changed: map[string][]string{}, deleted: []map[string]string{}}
	rows, err := tx.Query(ctx, `SELECT entity,entity_id::text,deleted FROM app.catalog_change_log
        WHERE version>$1 ORDER BY version,entity,entity_id`, since)
	if err != nil {
		return changes, err
	}
	defer rows.Close()
	for rows.Next() {
		var entity, id string
		var deleted bool
		if err := rows.Scan(&entity, &id, &deleted); err != nil {
			return changes, err
		}
		if deleted {
			changes.deleted = append(changes.deleted, map[string]string{"entity": entity, "id": id})
			continue
		}
		changes.changed[entity] = append(changes.changed[entity], id)
	}
	return changes, rows.Err()
}

// catalogSync atiende GET catalog/sync[?since=X&epoch=E].
func catalogSync(ctx context.Context, tx pgx.Tx, request Request) (any, error) {
	if err := request.Actor.Require("catalog.prices.read"); err != nil {
		return nil, err
	}
	version, err := currentVersion(ctx, tx)
	if err != nil {
		return nil, err
	}
	epoch, err := Epoch(ctx, tx)
	if err != nil {
		return nil, err
	}
	full, reset := true, false
	var since int64
	if text := request.Query.Get("since"); text != "" {
		since, err = strconv.ParseInt(text, 10, 64)
		if err != nil || since < 0 {
			return nil, invalid("La versión de partida debe ser un entero no negativo.")
		}
		// Revisión de la fase D (hallazgo 2): la descarga de cambios exige la época de la descarga anterior. Una
		// base restaurada vuelve a dar los números de versión que siguieron al respaldo, así que una versión igual
		// no basta: con otra época, el cliente tiene una línea de tiempo que ya no existe y se le entrega todo.
		clientEpoch := request.Query.Get("epoch")
		if since > 0 && clientEpoch == "" {
			return nil, invalid("La descarga de cambios requiere la época del catálogo (catalog_epoch) de la descarga anterior.")
		}
		if clientEpoch != "" && !idPattern.MatchString(clientEpoch) {
			return nil, invalid("La época del catálogo no es válida.")
		}
		// Desde 0 no hay bitácora suficiente (lo anterior a la 0023 no se anotó); una versión mayor que la vigente
		// o de otra época viene de otro servidor o de una base restaurada: se entrega todo y el cliente reemplaza.
		stale := since > 0 && (since > version || clientEpoch != epoch)
		full = since == 0 || stale
		reset = stale
	}
	location, err := restaurantLocation(ctx, tx)
	if err != nil {
		return nil, err
	}
	list, err := defaultPriceList(ctx, tx)
	if err != nil {
		return nil, err
	}
	taxRate, err := generalTaxRate(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := map[string]any{
		"catalog_version": version,
		"catalog_epoch":   epoch,
		"since":           since,
		"full":            full,
		"reset":           reset,
		"generated_at":    now.UTC(),
		"timezone":        location.String(),
		"tax_rate":        taxRate,
		"price_list":      map[string]any{"id": list.ID, "name": list.Name, "revision": list.Revision},
		"deleted":         []map[string]string{},
	}
	plan := syncPlan{full: true}
	if !full {
		changes, err := readChanges(ctx, tx, since)
		if err != nil {
			return nil, err
		}
		result["deleted"] = changes.deleted
		if plan, err = planDelta(ctx, tx, changes); err != nil {
			return nil, err
		}
	}
	availability, err := loadSyncAvailability(ctx, tx, now, location)
	if err != nil {
		return nil, err
	}
	categories, err := syncDocuments(ctx, tx, "categories", plan.selection("category"), "")
	if err != nil {
		return nil, err
	}
	categoryViews := []any{}
	images := map[string]bool{}
	for _, document := range categories {
		category := *document.(*Category)
		view := RedactPrices(category, request.Actor).(map[string]any)
		view["available_now"] = category.Active && scheduleAvailable(category.Availability, now, location)
		categoryViews = append(categoryViews, view)
		if category.ImageID != nil {
			images[*category.ImageID] = true
		}
	}
	result["categories"] = categoryViews
	products, err := plan.products(ctx, tx)
	if err != nil {
		return nil, err
	}
	productViews := []any{}
	for _, document := range products {
		product := *document.(*Product)
		view := syncProductView(product, availability.category(product.CategoryID), list, taxRate, now, location, request.Actor)
		availability.annotateCombo(view, product)
		productViews = append(productViews, view)
		if product.ImageID != nil {
			images[*product.ImageID] = true
		}
	}
	result["products"] = productViews
	for _, entity := range []struct{ resource, name, key string }{
		{"ingredients", "ingredient", "ingredients"},
		{"complements", "complement", "complements"},
		{"portions", "portion", "portions"},
		{"groups", "group", "groups"},
	} {
		documents, err := syncDocuments(ctx, tx, entity.resource, plan.selection(entity.name), "")
		if err != nil {
			return nil, err
		}
		views := RedactPrices(documents, request.Actor)
		if entity.resource == "groups" {
			availability.annotateGroups(views)
		}
		result[entity.key] = views
	}
	notes, err := listQuickNotes(ctx, tx)
	if err != nil {
		return nil, err
	}
	if wanted := plan.selection("quick_note"); wanted != nil {
		filtered := []CategoryQuickNote{}
		for _, note := range notes {
			if slices.Contains(wanted, note.ID) {
				filtered = append(filtered, note)
			}
		}
		notes = filtered
	}
	result["quick_notes"] = notes
	for _, id := range plan.selection("image") {
		images[id] = true
	}
	references, err := imageReferences(ctx, tx, images)
	if err != nil {
		return nil, err
	}
	result["images"] = references
	return result, nil
}

// syncDocuments lee los documentos de un recurso: todos (ids nil) o solo los indicados, con un filtro opcional.
func syncDocuments(ctx context.Context, tx pgx.Tx, resource string, ids []string, filter string) ([]any, error) {
	if ids != nil && len(ids) == 0 {
		return []any{}, nil
	}
	aliases := map[string]string{
		"categories":  "c",
		"products":    "p",
		"ingredients": "i",
		"complements": "c",
		"portions":    "p",
		"groups":      "g",
	}
	alias := aliases[resource]
	clauses := []string{"true"}
	arguments := []any{}
	if ids != nil {
		arguments = append(arguments, ids)
		clauses = append(clauses, alias+".id::text = ANY($1::text[])")
	}
	if filter != "" {
		clauses = append(clauses, filter)
	}
	where := " WHERE " + clauses[0]
	for _, clause := range clauses[1:] {
		where += " AND " + clause
	}
	rows, err := tx.Query(ctx, "SELECT data FROM ("+documentQueries[resource]+where+") records ORDER BY data->>'name',data->>'id'",
		arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []any{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		document := newDocument(resource)
		if err := json.Unmarshal(encoded, document); err != nil {
			return nil, err
		}
		result = append(result, document)
	}
	return result, rows.Err()
}

// syncProductView es el producto de la descarga: su documento con los precios de la lista predeterminada, la tasa
// de IVA efectiva, la disponibilidad de este momento y «Agotado».
func syncProductView(product Product, category Category, list PriceList, taxRate int, now time.Time, location *time.Location,
	actor Actor) map[string]any {
	view := RedactPrices(product, actor).(map[string]any)
	reason := productAvailabilityReason(product, category, now, location, false)
	view["available_now"] = reason == ""
	view["availability_reason"] = reason
	state := soldOutState(product, now)
	view["sold_out"] = state.SoldOut
	view["sold_out_indefinite"] = state.Indefinite
	view["sold_out_until"] = state.SoldOutUntil
	effectiveTax := taxRate
	if product.TaxRate != nil {
		effectiveTax = *product.TaxRate
	}
	view["effective_tax_rate"] = effectiveTax
	if !actor.Can("catalog.prices.read") {
		return view
	}
	// Precio sin presentación y de cada presentación, con las mismas reglas de la cotización. La disponibilidad no
	// interviene aquí (override): el precio existe aunque el producto esté agotado o fuera de horario.
	view["price"], view["price_origin"], view["pricing_error"] = syncPrice(product, nil, list, now, location)
	if variants, ok := view["variants"].([]any); ok {
		for index, entry := range variants {
			variant, isMap := entry.(map[string]any)
			if !isMap || index >= len(product.Variants) {
				continue
			}
			id := product.Variants[index].ID
			variant["effective_price"], variant["price_origin"], variant["pricing_error"] = syncPrice(product, &id, list, now, location)
		}
	}
	return view
}

// syncPrice devuelve el precio efectivo con su origen, o el motivo por el que la lista no le da precio.
func syncPrice(product Product, variantID *string, list PriceList, now time.Time, location *time.Location) (any, any, any) {
	price, origin, err := effectivePrice(product, variantID, list, now, location, true)
	if err != nil {
		message := "El producto no tiene precio en la lista predeterminada."
		if problem, ok := err.(*Error); ok {
			message = problem.Message
		}
		return nil, nil, message
	}
	return money(price), origin, nil
}

func defaultPriceList(ctx context.Context, tx pgx.Tx) (PriceList, error) {
	var id string
	if err := tx.QueryRow(ctx, "SELECT id::text FROM app.catalog_price_lists WHERE is_default").Scan(&id); err != nil {
		return PriceList{}, err
	}
	document, err := readDocument(ctx, tx, "price-lists", id)
	if err != nil {
		return PriceList{}, err
	}
	return *document.(*PriceList), nil
}

// generalTaxRate es la tasa general de IVA del establecimiento (la de los productos sin tasa propia).
func generalTaxRate(ctx context.Context, tx pgx.Tx) (int, error) {
	var rate int
	err := tx.QueryRow(ctx, "SELECT tax_rate FROM app.payment_settings").Scan(&rate)
	return rate, err
}

func productsOfCategories(ctx context.Context, tx pgx.Tx, categories []string) ([]string, error) {
	rows, err := tx.Query(ctx, "SELECT id::text FROM app.catalog_products WHERE category_id::text = ANY($1::text[])", categories)
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

// imageReferences devuelve la referencia de cada imagen (sin su contenido): el cliente la descarga aparte y la
// guarda por su checksum, que también es su ETag.
func imageReferences(ctx context.Context, tx pgx.Tx, ids map[string]bool) ([]map[string]any, error) {
	list := []string{}
	for id := range ids {
		list = append(list, id)
	}
	references := []map[string]any{}
	if len(list) == 0 {
		return references, nil
	}
	rows, err := tx.Query(ctx, `SELECT id::text,media_type,width,height,octet_length(content),checksum
        FROM app.catalog_images WHERE id::text = ANY($1::text[]) ORDER BY id`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, mediaType, checksum string
		var width, height, size int
		if err := rows.Scan(&id, &mediaType, &width, &height, &size, &checksum); err != nil {
			return nil, err
		}
		references = append(references, map[string]any{
			"id":         id,
			"media_type": mediaType,
			"width":      width,
			"height":     height,
			"size":       size,
			"checksum":   checksum,
		})
	}
	return references, rows.Err()
}
