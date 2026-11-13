package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Tipos de dato de un renglón de la vista previa.
const (
	KindPrinter  = "printer"
	KindStation  = "station"
	KindRoute    = "route"
	KindProduct  = "product"
	KindCategory = "category"
	KindSetting  = "setting"
)

// Entry es un renglón de la vista previa o del resultado: qué dato es, su nombre visible y el motivo.
type Entry struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Preview es la lista que se muestra antes de quitar: lo que se elimina, lo que se desactiva porque ya
// tiene operaciones registradas, la configuración que se desvincula y lo que coincide con una firma pero
// se conserva (con el motivo). PlanID resume las tres primeras listas: quitar exige el mismo PlanID que
// se revisó, de modo que nunca se aplica algo distinto de lo que la persona vio.
type Preview struct {
	PlanID     string  `json:"plan_id"`
	Remove     []Entry `json:"remove"`
	Deactivate []Entry `json:"deactivate"`
	Unlink     []Entry `json:"unlink"`
	Keep       []Entry `json:"keep"`
}

// Empty indica que no hay nada que eliminar, desactivar ni desvincular.
func (preview Preview) Empty() bool {
	return len(preview.Remove)+len(preview.Deactivate)+len(preview.Unlink) == 0
}

// Plan es la vista previa más las operaciones exactas que la aplican (Apply).
type Plan struct {
	Preview
	// Impresoras de prueba que no se conservan: su configuración se desvincula.
	unlinkPrinters     []string
	deletePrinters     []string
	deactivatePrinters []string
	// Todas las estaciones de prueba: se quitan de rutas y terminales aunque se conserven desactivadas.
	testStations       []string
	deleteStations     []string
	deactivateStations []string
	deleteRoutes       []string
	// Rutas que se conservan, sin sus estaciones de prueba.
	trimmedRoutes []string
	// Rutas cuyas estaciones son todas de prueba: se eliminan, pero se presentan según su dueño (ver
	// presentStationOnlyRoutes); stationRoutesAt es dónde van en «Se eliminará».
	stationOnlyRoutes    []routeRow
	stationRoutesAt      int
	deleteProducts       []string
	deactivateProducts   []string
	deleteCategories     []string
	deactivateCategories []string
	// Categorías de prueba que no se conservan (se eliminan, se desactivan o ya estaban inactivas).
	testCategories   []string
	receiptPrinter   bool
	drawerRegisters  []string
	fallbackPrinters []string
	deviceLinks      bool
	// userLinks: algún usuario atendía una estación de prueba (etapa 9F, «Estaciones que puede atender»).
	userLinks bool
}

// Referencias que el plan maneja por su cuenta: configuración que se borra junto con el dato o que se
// desvincula. Cualquier otra referencia cuenta como registro relacionado y el dato se desactiva.
//
// Las promociones (catalog_promotion_products y catalog_promotion_categories) no están aquí a propósito
// (revisión independiente de la 9F, M1): son del restaurante. Un producto o una categoría de prueba que
// participa en una promoción se desactiva (el producto además se archiva) en lugar de borrarse, y la
// promoción no se toca; si se borrara, la promoción perdería un participante sin que nadie lo viera y,
// si era el único, quedaría activa sin participantes y ya no se podría volver a guardar.
var (
	stationHandled = map[reference]bool{
		{table: "kitchen_route_stations", column: "station_id"}:       true,
		{table: "kitchen_device_stations", column: "station_id"}:      true,
		{table: "kitchen_user_stations", column: "station_id"}:        true,
		{table: "inventory_station_warehouses", column: "station_id"}: true,
	}
	printerHandled = map[reference]bool{
		{table: "kitchen_printers", column: "fallback_id"}:        true,
		{table: "kitchen_stations", column: "printer_id"}:         true,
		{table: "payment_settings", column: "receipt_printer_id"}: true,
		{table: "cash_drawer_settings", column: "printer_id"}:     true,
	}
	productHandled = map[reference]bool{
		{table: "catalog_variants", column: "product_id"}:            true,
		{table: "catalog_product_ingredients", column: "product_id"}: true,
		{table: "catalog_product_groups", column: "product_id"}:      true,
		{table: "catalog_price_entries", column: "product_id"}:       true,
		{table: "kitchen_routes", column: "product_id"}:              true,
	}
	variantHandled = map[reference]bool{
		{table: "catalog_price_entries", column: "variant_id"}: true,
	}
	categoryHandled = map[reference]bool{
		{table: "catalog_products", column: "category_id"}:             true,
		{table: "kitchen_routes", column: "category_id"}:               true,
		{table: "catalog_category_quick_notes", column: "category_id"}: true,
	}
)

// Tablas de las promociones del restaurante en las que participa un producto o una categoría.
const (
	promotionProducts   = "catalog_promotion_products"
	promotionCategories = "catalog_promotion_categories"
)

type planBuilder struct {
	ctx        context.Context
	tx         pgx.Tx
	references *referenceFinder
	plan       *Plan
}

// Build calcula el plan con los datos vigentes. Quien llama toma los locks: compartidos para la vista
// previa y exclusivos para aplicarla (7820148305, 7820148304 y 7820148308, en ese orden).
func Build(ctx context.Context, tx pgx.Tx) (*Plan, error) {
	builder := &planBuilder{
		ctx:        ctx,
		tx:         tx,
		references: &referenceFinder{ctx: ctx, tx: tx, cache: map[string][]reference{}},
		// Listas vacías (no nulas): la vista previa siempre entrega arreglos.
		plan: &Plan{Preview: Preview{Remove: []Entry{}, Deactivate: []Entry{}, Unlink: []Entry{}, Keep: []Entry{}}},
	}
	if err := builder.build(); err != nil {
		return nil, err
	}
	plan := builder.plan
	identifier, err := planIdentifier(plan.Preview)
	if err != nil {
		return nil, err
	}
	plan.PlanID = identifier
	return plan, nil
}

type printerRow struct {
	ID     string
	Name   string
	Active bool
}

type stationRow struct {
	ID          string
	Name        string
	Active      bool
	PrinterID   string
	PrinterName string
}

type productRow struct {
	ID         string
	Name       string
	Code       string
	Active     bool
	Archived   bool
	CategoryID string
}

// Orden de las decisiones: estaciones → rutas → productos → categorías → impresoras → terminales. Cada
// paso depende de lo que decidió el anterior (una impresora no se borra si una estación de prueba se
// conserva desactivada; una categoría no se borra si conserva un producto de prueba con ventas). Las
// rutas que solo enviaban a estaciones de prueba se presentan después de decidir productos y
// categorías, porque dónde se muestran depende de si su dueño es de prueba.
func (builder *planBuilder) build() error {
	printers, err := builder.testPrinters()
	if err != nil {
		return err
	}
	printerIDs := []string{}
	for _, printer := range printers {
		printerIDs = append(printerIDs, printer.ID)
	}
	stations, err := builder.testStations(printerIDs)
	if err != nil {
		return err
	}
	if err := builder.decideStations(stations); err != nil {
		return err
	}
	if err := builder.decideStationRoutes(); err != nil {
		return err
	}
	products, err := builder.testProducts()
	if err != nil {
		return err
	}
	if err := builder.decideProducts(products); err != nil {
		return err
	}
	if err := builder.decideCategories(products); err != nil {
		return err
	}
	builder.presentStationOnlyRoutes(products)
	if err := builder.decidePrinters(printers, stations); err != nil {
		return err
	}
	if err := builder.decideDeviceLinks(); err != nil {
		return err
	}
	return builder.decideUserLinks()
}

func (builder *planBuilder) testPrinters() ([]printerRow, error) {
	names, transports, destinations := []string{}, []string{}, []string{}
	for _, signature := range TestPrinters {
		names = append(names, signature.Name)
		transports = append(transports, signature.Transport)
		destinations = append(destinations, signature.Destination)
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT id::text, name, active FROM app.kitchen_printers
        WHERE (name, transport, destination) IN (SELECT * FROM unnest($1::text[], $2::text[], $3::text[]))
        ORDER BY name, id`, names, transports, destinations)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[printerRow])
}

func (builder *planBuilder) testStations(printerIDs []string) ([]stationRow, error) {
	names := []string{}
	for _, signature := range TestStations {
		names = append(names, signature.Name)
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT s.id::text, s.name, s.active, p.id::text, p.name
        FROM app.kitchen_stations s JOIN app.kitchen_printers p ON p.id = s.printer_id
        WHERE s.name = ANY($1::text[]) AND s.printer_id::text = ANY($2::text[])
        ORDER BY s.name, s.id`, names, list(printerIDs))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[stationRow])
}

// decideStations: una estación de prueba con tareas, acuses o avisos registrados se desactiva; sin ellos
// se elimina. En ambos casos sale de las rutas y de las estaciones de cada terminal.
func (builder *planBuilder) decideStations(stations []stationRow) error {
	plan := builder.plan
	for _, station := range stations {
		plan.testStations = append(plan.testStations, station.ID)
		blocking, err := builder.references.blocking("kitchen_stations", station.ID, stationHandled)
		if err != nil {
			return err
		}
		if len(blocking) == 0 {
			plan.deleteStations = append(plan.deleteStations, station.ID)
			plan.Remove = append(plan.Remove, Entry{
				Kind:   KindStation,
				ID:     station.ID,
				Name:   station.Name,
				Reason: fmt.Sprintf("Estación de prueba con la impresora «%s».", station.PrinterName),
			})
			continue
		}
		if station.Active {
			plan.deactivateStations = append(plan.deactivateStations, station.ID)
			plan.Deactivate = append(plan.Deactivate, Entry{
				Kind:   KindStation,
				ID:     station.ID,
				Name:   station.Name,
				Reason: operationsReason(blocking),
			})
		}
	}
	return nil
}

type routeRow struct {
	ID           string
	ProductRoute bool
	OwnerID      string
	Owner        string
	StationIDs   []string
	StationNames []string
}

func routeLabel(productRoute bool, owner string) string {
	if productRoute {
		return fmt.Sprintf("Ruta del producto «%s»", owner)
	}
	return fmt.Sprintf("Ruta de la categoría «%s»", owner)
}

// decideStationRoutes: una ruta cuyas estaciones son todas de prueba se elimina (solo podía enviar a un
// destino de prueba y una ruta sin estaciones mandaría en silencio a ningún lado); una ruta con
// estaciones del restaurante se conserva sin las de prueba. Las que se eliminan se anotan primero en
// «Se desvinculará», como configuración del restaurante, y presentStationOnlyRoutes pasa a «Se
// eliminará» las de un producto o una categoría de prueba.
func (builder *planBuilder) decideStationRoutes() error {
	plan := builder.plan
	plan.stationRoutesAt = len(plan.Remove)
	if len(plan.testStations) == 0 {
		return nil
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT r.id::text, r.product_id IS NOT NULL,
            coalesce(r.product_id, r.category_id)::text, coalesce(p.name, c.name, ''),
            array_agg(rs.station_id::text ORDER BY st.name, rs.station_id), array_agg(st.name ORDER BY st.name, rs.station_id)
        FROM app.kitchen_routes r
        JOIN app.kitchen_route_stations rs ON rs.route_id = r.id
        JOIN app.kitchen_stations st ON st.id = rs.station_id
        LEFT JOIN app.catalog_products p ON p.id = r.product_id
        LEFT JOIN app.catalog_categories c ON c.id = r.category_id
        GROUP BY r.id, p.name, c.name
        HAVING bool_or(rs.station_id::text = ANY($1::text[]))
        ORDER BY 4, 1`, list(plan.testStations))
	if err != nil {
		return err
	}
	routes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[routeRow])
	if err != nil {
		return err
	}
	for _, route := range routes {
		label := routeLabel(route.ProductRoute, route.Owner)
		allTest := true
		for _, stationID := range route.StationIDs {
			if !slices.Contains(plan.testStations, stationID) {
				allTest = false
			}
		}
		if allTest {
			plan.deleteRoutes = append(plan.deleteRoutes, route.ID)
			plan.stationOnlyRoutes = append(plan.stationOnlyRoutes, route)
			plan.Unlink = append(plan.Unlink, Entry{
				Kind:   KindRoute,
				ID:     route.ID,
				Name:   label,
				Reason: restaurantRouteReason(route),
			})
			continue
		}
		plan.trimmedRoutes = append(plan.trimmedRoutes, route.ID)
		for index, stationID := range route.StationIDs {
			if !slices.Contains(plan.testStations, stationID) {
				continue
			}
			plan.Unlink = append(plan.Unlink, Entry{
				Kind:   KindRoute,
				ID:     route.ID + ":" + stationID,
				Name:   label,
				Reason: fmt.Sprintf("Se quita la estación de prueba «%s»; conserva las del restaurante.", route.StationNames[index]),
			})
		}
	}
	return nil
}

// restaurantRouteReason explica qué pasa con la ruta de un producto o una categoría del restaurante que
// solo enviaba a estaciones de prueba (revisión independiente de la 9F, B1): Go la elimina, así que el
// producto usa la ruta de su categoría (la de producto prevalece) o, sin ella, la cocina avisa «Sin ruta
// de preparación» en lugar de enviar a ningún lado.
func restaurantRouteReason(route routeRow) string {
	stations := []string{}
	for _, name := range route.StationNames {
		stations = append(stations, "«"+name+"»")
	}
	sentStations := fmt.Sprintf("Solo enviaba a estaciones de prueba (%s); se quita: ", spanishList(stations))
	if route.ProductRoute {
		return sentStations + "el producto usará la ruta de su categoría o quedará sin ruta de preparación " +
			"hasta que le asignes otra."
	}
	return sentStations + "sus productos sin ruta propia quedarán sin ruta de preparación hasta que le asignes otra."
}

// presentStationOnlyRoutes coloca las rutas que solo enviaban a estaciones de prueba: la de un producto
// de prueba o de una categoría de prueba que no se conserva pasa a «Se eliminará» (en el lugar que les
// tocaba, después de las estaciones); la de un producto o una categoría del restaurante se queda en «Se
// desvinculará» con su explicación, porque no es un dato de prueba. En ambos casos Go elimina la ruta.
func (builder *planBuilder) presentStationOnlyRoutes(products []productRow) {
	plan := builder.plan
	testOwners := slices.Clone(plan.testCategories)
	for _, product := range products {
		testOwners = append(testOwners, product.ID)
	}
	removed := []Entry{}
	for _, route := range plan.stationOnlyRoutes {
		if !slices.Contains(testOwners, route.OwnerID) {
			continue
		}
		plan.Unlink = slices.DeleteFunc(plan.Unlink, func(entry Entry) bool {
			return entry.Kind == KindRoute && entry.ID == route.ID
		})
		removed = append(removed, Entry{
			Kind:   KindRoute,
			ID:     route.ID,
			Name:   routeLabel(route.ProductRoute, route.Owner),
			Reason: "Todas sus estaciones son de prueba.",
		})
	}
	plan.Remove = slices.Insert(plan.Remove, plan.stationRoutesAt, removed...)
}

func (builder *planBuilder) testProducts() ([]productRow, error) {
	names, codes, kinds := []string{}, []string{}, []string{}
	for _, signature := range TestProducts {
		names = append(names, signature.Name)
		codes = append(codes, signature.Code)
		kinds = append(kinds, signature.Kind)
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT id::text, name, code, active, archived, category_id::text
        FROM app.catalog_products
        WHERE (name, code, kind) IN (SELECT * FROM unnest($1::text[], $2::text[], $3::text[]))
        ORDER BY name, id`, names, codes, kinds)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[productRow])
}

// decideProducts: un producto de prueba vendido (o en una cuenta abierta, una receta, un combo, una
// promoción…) se desactiva y se archiva; si no, se elimina con su configuración de catálogo y su ruta de
// preparación.
func (builder *planBuilder) decideProducts(products []productRow) error {
	plan := builder.plan
	for _, product := range products {
		blocking, err := builder.references.blocking("catalog_products", product.ID, productHandled)
		if err != nil {
			return err
		}
		variants, err := builder.variants(product.ID)
		if err != nil {
			return err
		}
		for _, variant := range variants {
			found, err := builder.references.blocking("catalog_variants", variant, variantHandled)
			if err != nil {
				return err
			}
			blocking = append(blocking, found...)
		}
		if len(blocking) > 0 {
			if product.Active || !product.Archived {
				reason, err := builder.relatedReason(blocking, promotionProducts, "product_id", product.ID)
				if err != nil {
					return err
				}
				plan.deactivateProducts = append(plan.deactivateProducts, product.ID)
				plan.Deactivate = append(plan.Deactivate, Entry{
					Kind:   KindProduct,
					ID:     product.ID,
					Name:   product.Name,
					Reason: reason + " Se desactiva y se archiva.",
				})
			}
			continue
		}
		plan.deleteProducts = append(plan.deleteProducts, product.ID)
		plan.Remove = append(plan.Remove, Entry{
			Kind:   KindProduct,
			ID:     product.ID,
			Name:   product.Name,
			Reason: fmt.Sprintf("Producto de prueba (código %s).", product.Code),
		})
		if err := builder.removeOwnedRoute("product_id", product.ID, true, product.Name, "Pertenece a un producto de prueba que se elimina."); err != nil {
			return err
		}
	}
	return nil
}

func (builder *planBuilder) variants(productID string) ([]string, error) {
	rows, err := builder.tx.Query(builder.ctx, "SELECT id::text FROM app.catalog_variants WHERE product_id = $1::uuid ORDER BY id", productID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// removeOwnedRoute agrega al plan la ruta de un producto o una categoría que se elimina, si todavía no
// estaba. column es product_id o category_id (valores fijos de este archivo, nunca del cliente).
func (builder *planBuilder) removeOwnedRoute(column, ownerID string, productRoute bool, owner, reason string) error {
	var routeID string
	err := builder.tx.QueryRow(builder.ctx, "SELECT id::text FROM app.kitchen_routes WHERE "+column+" = $1::uuid", ownerID).Scan(&routeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	plan := builder.plan
	if slices.Contains(plan.deleteRoutes, routeID) {
		return nil
	}
	plan.deleteRoutes = append(plan.deleteRoutes, routeID)
	plan.trimmedRoutes = slices.DeleteFunc(plan.trimmedRoutes, func(candidate string) bool {
		return candidate == routeID
	})
	plan.Unlink = slices.DeleteFunc(plan.Unlink, func(entry Entry) bool {
		return entry.Kind == KindRoute && strings.HasPrefix(entry.ID, routeID+":")
	})
	plan.Remove = append(plan.Remove, Entry{
		Kind:   KindRoute,
		ID:     routeID,
		Name:   routeLabel(productRoute, owner),
		Reason: reason,
	})
	return nil
}

type categoryRow struct {
	ID     string
	Name   string
	Active bool
}

// decideCategories: solo cuenta una categoría con la firma que además contiene un producto de prueba. Se
// conserva si tiene otros productos o si su ruta incluye estaciones del restaurante; se desactiva si
// conserva productos de prueba con operaciones o participa en una promoción; si no, se elimina.
func (builder *planBuilder) decideCategories(products []productRow) error {
	plan := builder.plan
	productIDs := []string{}
	for _, product := range products {
		productIDs = append(productIDs, product.ID)
	}
	names, colors := []string{}, []string{}
	for _, signature := range TestCategories {
		names = append(names, signature.Name)
		colors = append(colors, signature.Color)
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT c.id::text, c.name, c.active FROM app.catalog_categories c
        WHERE (c.name, c.color) IN (SELECT * FROM unnest($1::text[], $2::text[]))
            AND EXISTS(SELECT FROM app.catalog_products p WHERE p.category_id = c.id AND p.id::text = ANY($3::text[]))
        ORDER BY c.name, c.id`, names, colors, productIDs)
	if err != nil {
		return err
	}
	categories, err := pgx.CollectRows(rows, pgx.RowToStructByPos[categoryRow])
	if err != nil {
		return err
	}
	for _, category := range categories {
		entry := Entry{Kind: KindCategory, ID: category.ID, Name: category.Name}
		var otherProduct string
		err := builder.tx.QueryRow(builder.ctx, `SELECT name FROM app.catalog_products
            WHERE category_id = $1::uuid AND NOT (id::text = ANY($2::text[])) ORDER BY name, id LIMIT 1`,
			category.ID, productIDs).Scan(&otherProduct)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			entry.Reason = fmt.Sprintf("Tiene otros productos del restaurante, como «%s».", otherProduct)
			plan.Keep = append(plan.Keep, entry)
			continue
		}
		var routeID, mode string
		err = builder.tx.QueryRow(builder.ctx, "SELECT id::text, mode FROM app.kitchen_routes WHERE category_id = $1::uuid",
			category.ID).Scan(&routeID, &mode)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		ownRoute := err == nil
		if ownRoute && mode == "prepare" && !slices.Contains(plan.deleteRoutes, routeID) {
			entry.Reason = "Su ruta de preparación incluye estaciones del restaurante."
			plan.Keep = append(plan.Keep, entry)
			continue
		}
		plan.testCategories = append(plan.testCategories, category.ID)
		blocking, err := builder.references.blocking("catalog_categories", category.ID, categoryHandled)
		if err != nil {
			return err
		}
		keepsTestProducts := false
		for _, product := range products {
			if product.CategoryID == category.ID && !slices.Contains(plan.deleteProducts, product.ID) {
				keepsTestProducts = true
			}
		}
		if len(blocking) > 0 || keepsTestProducts {
			if category.Active {
				entry.Reason = "Conserva productos de prueba con registros."
				if len(blocking) > 0 {
					entry.Reason, err = builder.relatedReason(blocking, promotionCategories, "category_id", category.ID)
					if err != nil {
						return err
					}
				}
				plan.deactivateCategories = append(plan.deactivateCategories, category.ID)
				plan.Deactivate = append(plan.Deactivate, entry)
			}
			continue
		}
		entry.Reason = "Categoría de prueba sin productos del restaurante."
		plan.deleteCategories = append(plan.deleteCategories, category.ID)
		plan.Remove = append(plan.Remove, entry)
		if ownRoute {
			if err := builder.removeOwnedRoute("category_id", category.ID, false, category.Name, "Pertenece a una categoría de prueba que se elimina."); err != nil {
				return err
			}
		}
	}
	return nil
}

// decidePrinters: una impresora de prueba que usa una estación del restaurante se conserva sin cambios;
// con trabajos de impresión (o estaciones de prueba que se conservan desactivadas) se desactiva; si no,
// se elimina. Salvo si se conserva, se desvincula de Cobros, de los cajones y de otras impresoras.
func (builder *planBuilder) decidePrinters(printers []printerRow, stations []stationRow) error {
	plan := builder.plan
	// Primera pasada: las que se conservan, para saber cuáles se desvinculan antes de revisar respaldos.
	kept := map[string]string{}
	for _, printer := range printers {
		var realStation string
		err := builder.tx.QueryRow(builder.ctx, `SELECT name FROM app.kitchen_stations
            WHERE printer_id = $1::uuid AND NOT (id::text = ANY($2::text[])) ORDER BY name, id LIMIT 1`,
			printer.ID, list(plan.testStations)).Scan(&realStation)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			kept[printer.ID] = realStation
			continue
		}
		plan.unlinkPrinters = append(plan.unlinkPrinters, printer.ID)
	}
	for _, printer := range printers {
		entry := Entry{Kind: KindPrinter, ID: printer.ID, Name: printer.Name}
		if realStation, ok := kept[printer.ID]; ok {
			entry.Reason = fmt.Sprintf("La usa la estación «%s» del restaurante.", realStation)
			plan.Keep = append(plan.Keep, entry)
			continue
		}
		blocking, err := builder.references.blocking("kitchen_printers", printer.ID, printerHandled)
		if err != nil {
			return err
		}
		keptStations := false
		for _, station := range stations {
			if station.PrinterID == printer.ID && !slices.Contains(plan.deleteStations, station.ID) {
				keptStations = true
			}
		}
		switch {
		case len(blocking) > 0 || keptStations:
			if printer.Active {
				entry.Reason = "La usan estaciones de prueba que se conservan desactivadas."
				if len(blocking) > 0 {
					entry.Reason = operationsReason(blocking)
				}
				plan.deactivatePrinters = append(plan.deactivatePrinters, printer.ID)
				plan.Deactivate = append(plan.Deactivate, entry)
			}
		default:
			entry.Reason = "Destino de las pruebas automáticas: lo que se le envía nunca sale en papel."
			plan.deletePrinters = append(plan.deletePrinters, printer.ID)
			plan.Remove = append(plan.Remove, entry)
		}
		if err := builder.decidePrinterSettings(printer); err != nil {
			return err
		}
	}
	return nil
}

// decidePrinterSettings desvincula la configuración que apunta a una impresora de prueba.
func (builder *planBuilder) decidePrinterSettings(printer printerRow) error {
	plan := builder.plan
	var receipt bool
	if err := builder.tx.QueryRow(builder.ctx, "SELECT EXISTS(SELECT FROM app.payment_settings WHERE receipt_printer_id = $1::uuid)",
		printer.ID).Scan(&receipt); err != nil {
		return err
	}
	if receipt {
		plan.receiptPrinter = true
		plan.Unlink = append(plan.Unlink, Entry{
			Kind:   KindSetting,
			ID:     "receipt:" + printer.ID,
			Name:   "Impresora de tickets de Cobros",
			Reason: fmt.Sprintf("Era «%s»; queda sin impresora de tickets hasta que elijas una.", printer.Name),
		})
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT r.id::text, r.name FROM app.cash_drawer_settings d
        JOIN app.registers r ON r.id = d.register_id WHERE d.printer_id = $1::uuid ORDER BY r.name, r.id`, printer.ID)
	if err != nil {
		return err
	}
	drawers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[namedRow])
	if err != nil {
		return err
	}
	for _, drawer := range drawers {
		plan.drawerRegisters = append(plan.drawerRegisters, drawer.ID)
		plan.Unlink = append(plan.Unlink, Entry{
			Kind:   KindSetting,
			ID:     "drawer:" + drawer.ID,
			Name:   fmt.Sprintf("Cajón de dinero de «%s»", drawer.Name),
			Reason: fmt.Sprintf("Usaba «%s»; el cajón queda desactivado.", printer.Name),
		})
	}
	rows, err = builder.tx.Query(builder.ctx, `SELECT id::text, name FROM app.kitchen_printers
        WHERE fallback_id = $1::uuid ORDER BY name, id`, printer.ID)
	if err != nil {
		return err
	}
	fallbacks, err := pgx.CollectRows(rows, pgx.RowToStructByPos[namedRow])
	if err != nil {
		return err
	}
	for _, fallback := range fallbacks {
		plan.fallbackPrinters = append(plan.fallbackPrinters, fallback.ID)
		if slices.Contains(plan.unlinkPrinters, fallback.ID) {
			// Una impresora de prueba que respalda a otra de prueba: no hace falta mencionarla.
			continue
		}
		plan.Unlink = append(plan.Unlink, Entry{
			Kind:   KindSetting,
			ID:     "fallback:" + fallback.ID,
			Name:   fmt.Sprintf("Respaldo de la impresora «%s»", fallback.Name),
			Reason: fmt.Sprintf("Era «%s»; queda sin impresora de respaldo.", printer.Name),
		})
	}
	return nil
}

// namedRow es un identificador con su nombre (cajas e impresoras de respaldo).
type namedRow struct {
	ID   string
	Name string
}

// decideDeviceLinks quita las estaciones de prueba de las estaciones de cada terminal.
func (builder *planBuilder) decideDeviceLinks() error {
	plan := builder.plan
	if len(plan.testStations) == 0 {
		return nil
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT d.id::text, d.name, s.id::text, s.name
        FROM app.kitchen_device_stations ds
        JOIN app.devices d ON d.id = ds.device_id
        JOIN app.kitchen_stations s ON s.id = ds.station_id
        WHERE ds.station_id::text = ANY($1::text[])
        ORDER BY d.name, d.id, s.name, s.id`, list(plan.testStations))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID, deviceName, stationID, stationName string
		if err := rows.Scan(&deviceID, &deviceName, &stationID, &stationName); err != nil {
			return err
		}
		plan.deviceLinks = true
		plan.Unlink = append(plan.Unlink, Entry{
			Kind:   KindSetting,
			ID:     "terminal:" + deviceID + ":" + stationID,
			Name:   fmt.Sprintf("Estaciones de la terminal «%s»", deviceName),
			Reason: fmt.Sprintf("Se quita la estación de prueba «%s».", stationName),
		})
	}
	return rows.Err()
}

// decideUserLinks quita las estaciones de prueba de las estaciones que atiende cada usuario (etapa 9F). Es
// configuración, no operación: la estación de prueba se puede eliminar igual. Un usuario que se quede sin
// estaciones conserva su marca de selección y no ve ninguna hasta que se le asignen otras (nunca se le
// amplía el alcance a «todas» sin que nadie lo decida).
func (builder *planBuilder) decideUserLinks() error {
	plan := builder.plan
	if len(plan.testStations) == 0 {
		return nil
	}
	rows, err := builder.tx.Query(builder.ctx, `SELECT u.id::text, u.display_name, s.id::text, s.name
        FROM app.kitchen_user_stations us
        JOIN app.users u ON u.id = us.user_id
        JOIN app.kitchen_stations s ON s.id = us.station_id
        WHERE us.station_id::text = ANY($1::text[])
        ORDER BY u.display_name, u.id, s.name, s.id`, list(plan.testStations))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, userName, stationID, stationName string
		if err := rows.Scan(&userID, &userName, &stationID, &stationName); err != nil {
			return err
		}
		plan.userLinks = true
		plan.Unlink = append(plan.Unlink, Entry{
			Kind:   KindSetting,
			ID:     "user:" + userID + ":" + stationID,
			Name:   fmt.Sprintf("Estaciones que atiende «%s»", userName),
			Reason: fmt.Sprintf("Se quita la estación de prueba «%s».", stationName),
		})
	}
	return rows.Err()
}

// list evita pasar un arreglo nulo a PostgreSQL: «x = ANY(NULL)» es NULL y «NOT (x = ANY(NULL))» también
// descartaría todos los renglones.
func list(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// planIdentifier resume lo que el plan elimina, desactiva y desvincula (con sus motivos).
func planIdentifier(preview Preview) (string, error) {
	encoded, err := json.Marshal([][]Entry{preview.Remove, preview.Deactivate, preview.Unlink})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
