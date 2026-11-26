package kitchen

import (
	"fmt"
	"slices"
	"strings"
)

// Etapa K1.1 (K11-D5): GET kitchen/coverage, la cobertura del enrutamiento para la configuración guiada del escritorio
// (el asistente al emparejar una pantalla de cocina, el diagnóstico de Terminales y la lista «Sin ruta» de
// Enrutamiento). Solo lee. Cada producto activo del catálogo se resuelve con decideRoute, la misma función que usa el
// envío, con la configuración vigente y el ajuste «Productos sin ruta»: el escritorio no calcula rutas, solo muestra lo
// que dice Go. Los avisos llevan su texto listo para la pantalla y los identificadores de lo que hay que corregir.

// CoverageNamed es una categoría o un producto con su nombre (y, en un producto, su categoría).
type CoverageNamed struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryID   string `json:"category_id,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
}

// CoverageCategory es una categoría cuya ruta de categoría llega a una estación, con cuántos de sus productos activos
// llegan por ella (los que tienen ruta propia van por la suya).
type CoverageCategory struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Products int    `json:"products"`
}

// CoveragePrinter es la impresora de una estación que imprime.
type CoveragePrinter struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// CoverageStation es una estación (activa o no, en el orden de Estaciones) con lo que le llega.
type CoverageStation struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Active           bool               `json:"active"`
	Reception        string             `json:"reception"`
	Printer          *CoveragePrinter   `json:"printer"`
	Categories       []CoverageCategory `json:"categories"`
	Products         []CoverageNamed    `json:"products"`
	ReceivesUnrouted bool               `json:"receives_unrouted"`
	DisplayIDs       []string           `json:"display_ids"`
}

// CoverageNoPreparation son las categorías y los productos con ruta «Sin preparación».
type CoverageNoPreparation struct {
	Categories []CoverageNamed `json:"categories"`
	Products   []CoverageNamed `json:"products"`
}

// CoverageUnroutedProduct es un producto que no va a ninguna estación por una ruta: reason «no_route» (sin ruta) o
// «inactive_station» (su ruta solo va a estaciones desactivadas). Sigue el ajuste «Productos sin ruta».
type CoverageUnroutedProduct struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// CoverageUnroutedGroup es lo sin ruta de una categoría; WholeCategory dice que toda la categoría está sin ruta.
type CoverageUnroutedGroup struct {
	CategoryID    string                    `json:"category_id"`
	CategoryName  string                    `json:"category_name"`
	WholeCategory bool                      `json:"whole_category"`
	Products      []CoverageUnroutedProduct `json:"products"`
}

// CoverageCategoryRoute es una categoría activa con su ruta de categoría, si tiene (para crearla o cambiarla con su
// revisión desde el asistente).
type CoverageCategoryRoute struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RouteID       *string  `json:"route_id"`
	RouteRevision *int     `json:"route_revision"`
	RouteMode     *string  `json:"route_mode"`
	StationIDs    []string `json:"station_ids"`
}

// CoverageDisplay es una pantalla de cocina no revocada con las estaciones que muestra (solo las activas que reciben
// comandas en pantalla, de su selección o de todas).
type CoverageDisplay struct {
	DeviceID    string   `json:"device_id"`
	Name        string   `json:"name"`
	AllStations bool     `json:"all_stations"`
	StationIDs  []string `json:"station_ids"`
}

// CoverageWarning es un aviso con su texto y lo que hay que corregir (null lo que no aplica).
type CoverageWarning struct {
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	DeviceID   *string `json:"device_id"`
	StationID  *string `json:"station_id"`
	CategoryID *string `json:"category_id"`
	ProductID  *string `json:"product_id"`
}

// Coverage es la respuesta de GET kitchen/coverage.
type Coverage struct {
	Unrouted      UnroutedView            `json:"unrouted"`
	Stations      []CoverageStation       `json:"stations"`
	NoPreparation CoverageNoPreparation   `json:"no_preparation"`
	UnroutedItems []CoverageUnroutedGroup `json:"unrouted_items"`
	Categories    []CoverageCategoryRoute `json:"categories"`
	Displays      []CoverageDisplay       `json:"displays"`
	Warnings      []CoverageWarning       `json:"warnings"`
}

// Códigos de los avisos de la cobertura (lista cerrada del contrato).
const (
	warningDisplayWithoutStations  = "display_without_stations"
	warningDisplayNoScreenStations = "display_no_screen_stations"
	warningStationWithoutRoutes    = "station_without_routes"
	warningRouteStationInactive    = "route_station_inactive"
	warningUnroutedStationInactive = "unrouted_station_inactive"
	warningStationPrinterInactive  = "station_printer_inactive"
)

// coverageProduct es un producto activo del catálogo; Leaf dice que no lleva componentes: un producto simple o un combo
// que por omisión va vacío (todos sus componentes empiezan en cero), que en el envío se trata como un producto simple.
type coverageProduct struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	Leaf         bool   `json:"leaf"`
}

// coverageDevice es una pantalla de cocina con su selección guardada.
type coverageDevice struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	AllStations bool     `json:"all_stations"`
	Selected    []string `json:"selected"`
}

// loadedRoutes es la configuración vigente ya leída: cumple routeSource para que decideRoute resuelva cada producto
// igual que el envío, sin una consulta por producto.
type loadedRoutes struct {
	byProduct  map[string]Route
	byCategory map[string]Route
	stations   map[string]Station
	unrouted   *Station
}

func (source *loadedRoutes) productRoute(productID, categoryID string) (*Route, error) {
	if route, found := source.byProduct[productID]; found {
		return &route, nil
	}
	if route, found := source.byCategory[categoryID]; found {
		return &route, nil
	}
	return nil, nil
}

func (source *loadedRoutes) station(id string) (Station, error) {
	station, found := source.stations[id]
	if !found {
		return Station{}, fmt.Errorf("estación %s sin leer", id)
	}
	return station, nil
}

func (source *loadedRoutes) unroutedStation() (*Station, error) {
	return source.unrouted, nil
}

func (handler *service) coverage() (any, error) {
	unrouted, err := handler.unroutedView()
	if err != nil {
		return nil, err
	}
	stations, err := many[Station](handler.ctx, handler.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s ORDER BY sort_order,name")
	if err != nil {
		return nil, err
	}
	printers, err := many[CoveragePrinter](handler.ctx, handler.tx,
		"SELECT jsonb_build_object('id',id,'name',name,'active',active) FROM app.kitchen_printers")
	if err != nil {
		return nil, err
	}
	routes, err := many[Route](handler.ctx, handler.tx, routeQuery+" ORDER BY r.id")
	if err != nil {
		return nil, err
	}
	categories, err := many[CoverageNamed](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',id,'name',name)
        FROM app.catalog_categories WHERE active ORDER BY sort_order,name,id`)
	if err != nil {
		return nil, err
	}
	products, err := many[coverageProduct](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',p.id,'name',p.name,
            'category_id',c.id,'category_name',c.name,
            'leaf',NOT (p.kind='combo' AND EXISTS(SELECT FROM app.catalog_combo_slots slot
                WHERE slot.combo_id=p.id AND slot.default_quantity>0)))
        FROM app.catalog_products p JOIN app.catalog_categories c ON c.id=p.category_id
        WHERE p.active AND NOT p.archived AND c.active ORDER BY c.sort_order,c.name,c.id,p.sort_order,p.name,p.id`)
	if err != nil {
		return nil, err
	}
	devices, err := many[coverageDevice](handler.ctx, handler.tx, `SELECT jsonb_build_object('id',d.id,'name',d.name,
            'all_stations',d.kitchen_all_stations,'selected',coalesce((SELECT jsonb_agg(a.station_id ORDER BY a.station_id)
                FROM app.kitchen_device_stations a WHERE a.device_id=d.id),'[]'))
        FROM app.devices d WHERE d.kind='kitchen' AND d.revoked_at IS NULL ORDER BY d.name,d.id`)
	if err != nil {
		return nil, err
	}
	source := &loadedRoutes{byProduct: map[string]Route{}, byCategory: map[string]Route{}, stations: map[string]Station{}}
	for _, station := range stations {
		source.stations[station.ID] = station
	}
	for _, route := range routes {
		if route.ProductID != nil {
			source.byProduct[*route.ProductID] = route
		} else if route.CategoryID != nil {
			source.byCategory[*route.CategoryID] = route
		}
	}
	if unrouted.StationID != nil {
		station := source.stations[*unrouted.StationID]
		source.unrouted = &station
	}
	builder := coverageBuilder{source: source, unrouted: *unrouted, stations: stations, products: products,
		categories: categories, devices: devices, printers: map[string]CoveragePrinter{}}
	for _, printer := range printers {
		builder.printers[printer.ID] = printer
	}
	return builder.build()
}

// coverageBuilder arma la cobertura con lo ya leído.
type coverageBuilder struct {
	source     *loadedRoutes
	unrouted   UnroutedView
	stations   []Station
	products   []coverageProduct
	categories []CoverageNamed
	devices    []coverageDevice
	printers   map[string]CoveragePrinter
	result     Coverage
}

func (builder *coverageBuilder) build() (*Coverage, error) {
	builder.result = Coverage{Unrouted: builder.unrouted, Stations: []CoverageStation{},
		NoPreparation: CoverageNoPreparation{Categories: []CoverageNamed{}, Products: []CoverageNamed{}},
		UnroutedItems: []CoverageUnroutedGroup{}, Categories: []CoverageCategoryRoute{}, Displays: []CoverageDisplay{},
		Warnings: []CoverageWarning{}}
	decisions := map[string]routeDecision{}
	for _, product := range builder.products {
		decision, err := decideRoute(builder.source, product.ID, product.CategoryID, nil, product.Leaf)
		if err != nil {
			return nil, err
		}
		decisions[product.ID] = decision
	}
	builder.addDisplays()
	builder.addStations()
	builder.addNoPreparation()
	builder.addUnroutedItems(decisions)
	builder.addCategories()
	builder.addStationWarnings()
	builder.addRouteWarnings()
	return &builder.result, nil
}

// screenStation dice si la estación recibe comandas en pantalla y está activa (lo que muestra una pantalla de cocina).
func screenStation(station Station) bool {
	return station.Active && (station.Reception == "kds" || station.Reception == "both")
}

// addDisplays: cada pantalla con las estaciones que muestra, y los avisos de una pantalla que no muestra ninguna.
func (builder *coverageBuilder) addDisplays() {
	for _, device := range builder.devices {
		selected := map[string]bool{}
		for _, id := range device.Selected {
			selected[id] = true
		}
		shown := []string{}
		chosen, screen := 0, 0
		for _, station := range builder.stations {
			if !device.AllStations && !selected[station.ID] {
				continue
			}
			chosen++
			if station.Reception == "kds" || station.Reception == "both" {
				screen++
			}
			if screenStation(station) {
				shown = append(shown, station.ID)
			}
		}
		builder.result.Displays = append(builder.result.Displays, CoverageDisplay{DeviceID: device.ID, Name: device.Name,
			AllStations: device.AllStations, StationIDs: shown})
		if len(shown) > 0 {
			continue
		}
		deviceID := device.ID
		// Sus estaciones existen pero todas solo imprimen; si no tiene ninguna (o las de pantalla están desactivadas),
		// la pantalla no tiene estaciones.
		if chosen > 0 && screen == 0 {
			builder.warn(CoverageWarning{Code: warningDisplayNoScreenStations,
				Message: "Ninguna de sus estaciones recibe comandas en pantalla.", DeviceID: &deviceID})
			continue
		}
		builder.warn(CoverageWarning{Code: warningDisplayWithoutStations, Message: "Esta pantalla no tiene estaciones.",
			DeviceID: &deviceID})
	}
}

// addStations: cada estación con las categorías y los productos que le llegan por una ruta, si recibe lo sin ruta y
// las pantallas que la muestran.
func (builder *coverageBuilder) addStations() {
	categoryNames := map[string]string{}
	for _, category := range builder.categories {
		categoryNames[category.ID] = category.Name
	}
	// Productos activos de cada categoría que siguen la ruta de su categoría (sin ruta propia).
	byCategory := map[string]int{}
	for _, product := range builder.products {
		if _, own := builder.source.byProduct[product.ID]; !own {
			byCategory[product.CategoryID]++
		}
	}
	for _, station := range builder.stations {
		entry := CoverageStation{ID: station.ID, Name: station.Name, Active: station.Active, Reception: station.Reception,
			Categories: []CoverageCategory{}, Products: []CoverageNamed{}, DisplayIDs: []string{}}
		if station.Reception != "kds" && station.PrinterID != nil {
			if printer, found := builder.printers[*station.PrinterID]; found {
				entry.Printer = &printer
			}
		}
		// Solo llega a una estación activa: de una ruta se omiten las desactivadas (K11-D1).
		if station.Active {
			for _, category := range builder.categories {
				route, found := builder.source.byCategory[category.ID]
				if found && route.Mode == "prepare" && slices.Contains(route.StationIDs, station.ID) {
					entry.Categories = append(entry.Categories, CoverageCategory{ID: category.ID, Name: category.Name,
						Products: byCategory[category.ID]})
				}
			}
			for _, product := range builder.products {
				route, found := builder.source.byProduct[product.ID]
				if found && route.Mode == "prepare" && slices.Contains(route.StationIDs, station.ID) {
					entry.Products = append(entry.Products, CoverageNamed{ID: product.ID, Name: product.Name,
						CategoryID: product.CategoryID, CategoryName: product.CategoryName})
				}
			}
		}
		entry.ReceivesUnrouted = builder.unrouted.Effective == "station" && builder.unrouted.StationID != nil &&
			*builder.unrouted.StationID == station.ID
		for _, display := range builder.result.Displays {
			if slices.Contains(display.StationIDs, station.ID) {
				entry.DisplayIDs = append(entry.DisplayIDs, display.DeviceID)
			}
		}
		builder.result.Stations = append(builder.result.Stations, entry)
	}
}

// addNoPreparation: las categorías y los productos activos con ruta «Sin preparación».
func (builder *coverageBuilder) addNoPreparation() {
	for _, category := range builder.categories {
		if route, found := builder.source.byCategory[category.ID]; found && route.Mode == "none" {
			builder.result.NoPreparation.Categories = append(builder.result.NoPreparation.Categories,
				CoverageNamed{ID: category.ID, Name: category.Name})
		}
	}
	for _, product := range builder.products {
		if route, found := builder.source.byProduct[product.ID]; found && route.Mode == "none" {
			builder.result.NoPreparation.Products = append(builder.result.NoPreparation.Products, CoverageNamed{ID: product.ID,
				Name: product.Name, CategoryID: product.CategoryID, CategoryName: product.CategoryName})
		}
	}
}

// addUnroutedItems: lo que no va a ninguna estación por una ruta, por categoría. Un combo con componentes y sin ruta no
// se lista (sus componentes se resuelven solos); un combo vacío sí.
func (builder *coverageBuilder) addUnroutedItems(decisions map[string]routeDecision) {
	groups := map[string]*CoverageUnroutedGroup{}
	order := []string{}
	routed := map[string]bool{}
	for _, product := range builder.products {
		decision := decisions[product.ID]
		if decision.Reason == "" {
			routed[product.CategoryID] = true
		}
		if !decision.Route.Unrouted {
			continue
		}
		group := groups[product.CategoryID]
		if group == nil {
			group = &CoverageUnroutedGroup{CategoryID: product.CategoryID, CategoryName: product.CategoryName,
				Products: []CoverageUnroutedProduct{}}
			groups[product.CategoryID] = group
			order = append(order, product.CategoryID)
		}
		group.Products = append(group.Products, CoverageUnroutedProduct{ID: product.ID, Name: product.Name, Reason: decision.Reason})
	}
	for _, categoryID := range order {
		group := groups[categoryID]
		group.WholeCategory = !routed[categoryID]
		builder.result.UnroutedItems = append(builder.result.UnroutedItems, *group)
	}
}

// addCategories: todas las categorías activas con su ruta de categoría (las estaciones configuradas, activas o no).
func (builder *coverageBuilder) addCategories() {
	for _, category := range builder.categories {
		entry := CoverageCategoryRoute{ID: category.ID, Name: category.Name, StationIDs: []string{}}
		if route, found := builder.source.byCategory[category.ID]; found {
			id, revision, mode := route.ID, route.Revision, route.Mode
			entry.RouteID, entry.RouteRevision, entry.RouteMode = &id, &revision, &mode
			entry.StationIDs = append(entry.StationIDs, route.StationIDs...)
		}
		builder.result.Categories = append(builder.result.Categories, entry)
	}
}

// addStationWarnings: el ajuste que apunta a una estación desactivada, una estación de pantalla sin nada asignado y
// una impresora desactivada.
func (builder *coverageBuilder) addStationWarnings() {
	if builder.unrouted.StationID != nil && builder.unrouted.Effective == "none" {
		stationID := *builder.unrouted.StationID
		builder.warn(CoverageWarning{Code: warningUnroutedStationInactive, StationID: &stationID,
			Message: fmt.Sprintf("Los productos sin ruta iban a %s, que está desactivada. Mientras siga así, no necesitan preparación.",
				*builder.unrouted.StationName)})
	}
	for _, station := range builder.result.Stations {
		stationID := station.ID
		if station.Active && (station.Reception == "kds" || station.Reception == "both") && len(station.Categories) == 0 &&
			len(station.Products) == 0 && !station.ReceivesUnrouted {
			builder.warn(CoverageWarning{Code: warningStationWithoutRoutes, StationID: &stationID,
				Message: fmt.Sprintf("La estación %s no tiene categorías ni productos asignados.", station.Name)})
		}
		if station.Active && station.Reception != "kds" && station.Printer != nil && !station.Printer.Active {
			builder.warn(CoverageWarning{Code: warningStationPrinterInactive, StationID: &stationID,
				Message: fmt.Sprintf("La impresora %s de %s está desactivada: sus comandas esperan en la cola.",
					station.Printer.Name, station.Name)})
		}
	}
}

// addRouteWarnings: cada ruta (de una categoría o de un producto activos) que va a una estación desactivada, con lo
// que pasa con lo que lleva.
func (builder *coverageBuilder) addRouteWarnings() {
	for _, category := range builder.categories {
		if route, found := builder.source.byCategory[category.ID]; found {
			categoryID := category.ID
			builder.routeWarnings(route, category.Name, &categoryID, nil)
		}
	}
	for _, product := range builder.products {
		if route, found := builder.source.byProduct[product.ID]; found {
			productID := product.ID
			builder.routeWarnings(route, product.Name, nil, &productID)
		}
	}
}

func (builder *coverageBuilder) routeWarnings(route Route, owner string, categoryID, productID *string) {
	if route.Mode != "prepare" {
		return
	}
	active := []string{}
	inactive := []Station{}
	for _, id := range route.StationIDs {
		station := builder.source.stations[id]
		if station.Active {
			active = append(active, station.Name)
		} else {
			inactive = append(inactive, station)
		}
	}
	for _, station := range inactive {
		stationID := station.ID
		outcome := ""
		switch {
		case len(active) > 0 && categoryID != nil:
			outcome = "sus productos llegan solo a " + spanishNames(active) + "."
		case len(active) > 0:
			outcome = "llega solo a " + spanishNames(active) + "."
		case categoryID != nil:
			outcome = "sus productos se tratan como productos sin ruta."
		default:
			outcome = "se trata como producto sin ruta."
		}
		builder.warn(CoverageWarning{Code: warningRouteStationInactive, StationID: &stationID, CategoryID: categoryID,
			ProductID: productID, Message: fmt.Sprintf("%s va a %s, que está desactivada: %s", owner, station.Name, outcome)})
	}
}

func (builder *coverageBuilder) warn(warning CoverageWarning) {
	builder.result.Warnings = append(builder.result.Warnings, warning)
}

// spanishNames une nombres como se dicen: «Cocina», «Cocina y Barra», «Cocina, Barra y Postres».
func spanishNames(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " y " + names[len(names)-1]
}
