package kitchen

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/events"
)

const routeQuery = `SELECT to_jsonb(r)||jsonb_build_object('station_ids',
    coalesce((SELECT jsonb_agg(s.station_id ORDER BY s.station_id) FROM app.kitchen_route_stations s WHERE s.route_id=r.id),'[]'))
    FROM app.kitchen_routes r`

func (handler *service) saveRoute(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Route
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	route := input.Route
	// unrouted y omitted_station_ids solo existen en la fotografía de una liberación (K11-D1), nunca en una ruta
	// guardada: se rechazan como cualquier campo desconocido.
	if route.Unrouted || route.OmittedStationIDs != nil {
		return nil, invalid("Revisa los campos y tipos del documento de cocina.")
	}
	if (route.ProductID == nil) == (route.CategoryID == nil) {
		return nil, invalid("Selecciona un producto o una categoría.")
	}
	if !slices.Contains([]string{"prepare", "none"}, route.Mode) {
		return nil, invalid("La ruta debe preparar o indicar expresamente que no requiere preparación.")
	}
	if (route.Mode == "prepare" && len(route.StationIDs) == 0) || (route.Mode == "none" && len(route.StationIDs) != 0) || len(route.StationIDs) > 20 {
		return nil, invalid("Preparación requiere entre 1 y 20 estaciones; Sin preparación no admite destinos.")
	}
	seen := map[string]bool{}
	for _, id := range route.StationIDs {
		if err := validID(id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, invalid("La misma estación aparece más de una vez.")
		}
		seen[id] = true
		var active bool
		if err := handler.tx.QueryRow(handler.ctx, "SELECT active FROM app.kitchen_stations WHERE id=$1", id).Scan(&active); err != nil {
			return nil, err
		}
		if !active {
			return nil, invalid("No se puede asignar una ruta a una estación inactiva.")
		}
	}
	var before *Route
	if len(parts) == 1 {
		if route.ID != "" || route.Revision != 0 {
			return nil, invalid("Una ruta nueva no reutiliza identidad ni revisión.")
		}
		route.ID = events.NewID()
		_, err := handler.tx.Exec(handler.ctx,
			"INSERT INTO app.kitchen_routes(id,product_id,category_id,mode) VALUES ($1,$2,$3,$4)",
			route.ID, route.ProductID, route.CategoryID, route.Mode)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		before, err = one[Route](handler.ctx, handler.tx, routeQuery+" WHERE r.id=$1", parts[1])
		if err != nil {
			return nil, err
		}
		if route.ID != parts[1] || route.Revision != before.Revision {
			return nil, conflict()
		}
		if before.ProductID != nil && (route.ProductID == nil || *before.ProductID != *route.ProductID) ||
			before.CategoryID != nil && (route.CategoryID == nil || *before.CategoryID != *route.CategoryID) {
			return nil, invalid("La ruta conserva su producto o categoría; edita sus destinos.")
		}
		if _, err := handler.tx.Exec(handler.ctx, "UPDATE app.kitchen_routes SET mode=$1,revision=revision+1,updated_at=now() WHERE id=$2", route.Mode, route.ID); err != nil {
			return nil, err
		}
		if _, err := handler.tx.Exec(handler.ctx, "DELETE FROM app.kitchen_route_stations WHERE route_id=$1", route.ID); err != nil {
			return nil, err
		}
	}
	for _, id := range route.StationIDs {
		if _, err := handler.tx.Exec(handler.ctx, "INSERT INTO app.kitchen_route_stations(route_id,station_id) VALUES ($1,$2)", route.ID, id); err != nil {
			return nil, err
		}
	}
	saved, err := one[Route](handler.ctx, handler.tx, routeQuery+" WHERE r.id=$1", route.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.history("route.saved", &route.ID, before, saved)
}

type routingResult struct {
	Stations map[string]Station
	Parts    map[string][]Part
	Snapshot map[string]Route
}

// routeSource es la configuración vigente que necesita la resolución de rutas (etapa K1.1, K11-D1): la ruta propia o
// de la categoría de un producto, las estaciones y la estación del ajuste «Productos sin ruta». El envío la lee de la
// base en su transacción (liveRoutes); la cobertura del escritorio (coverage.go) la carga completa una vez. Las dos
// deciden con decideRoute, así que lo que dice la cobertura es lo que hace el envío.
type routeSource interface {
	productRoute(productID, categoryID string) (*Route, error)
	station(id string) (Station, error)
	unroutedStation() (*Station, error)
}

// liveRoutes lee la configuración en la transacción del envío o del cambio, con las estaciones ya leídas en memoria.
type liveRoutes struct {
	ctx            context.Context
	tx             pgx.Tx
	stations       map[string]Station
	unrouted       *Station
	unroutedLoaded bool
}

func newLiveRoutes(ctx context.Context, tx pgx.Tx) *liveRoutes {
	return &liveRoutes{ctx: ctx, tx: tx, stations: map[string]Station{}}
}

// productRoute es la ruta del producto o, sin ella, la de su categoría (la del producto prevalece).
func (source *liveRoutes) productRoute(productID, categoryID string) (*Route, error) {
	routes, err := many[Route](source.ctx, source.tx, routeQuery+`
		WHERE r.product_id=$1 OR r.category_id=$2
		ORDER BY (r.product_id IS NOT NULL) DESC LIMIT 1`, productID, categoryID)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, nil
	}
	return &routes[0], nil
}

func (source *liveRoutes) station(id string) (Station, error) {
	if station, found := source.stations[id]; found {
		return station, nil
	}
	found, err := one[Station](source.ctx, source.tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", id)
	if err != nil {
		return Station{}, err
	}
	source.stations[id] = *found
	return *found, nil
}

// unroutedStation es la estación elegida en «Productos sin ruta» (nil con «No necesitan preparación»).
func (source *liveRoutes) unroutedStation() (*Station, error) {
	if source.unroutedLoaded {
		return source.unrouted, nil
	}
	var id *string
	if err := source.tx.QueryRow(source.ctx, "SELECT unrouted_station_id::text FROM app.kitchen_settings").Scan(&id); err != nil {
		return nil, err
	}
	source.unroutedLoaded = true
	if id == nil {
		return nil, nil
	}
	station, err := source.station(*id)
	if err != nil {
		return nil, err
	}
	source.unrouted = &station
	return source.unrouted, nil
}

// routeDecision es lo que se decide para un producto: la ruta que se congela en el routing_snapshot (si Freeze), las
// estaciones que reciben su parte y, si no le quedó ninguna estación por una ruta, por qué.
type routeDecision struct {
	Route  Route
	Freeze bool
	// None: no se prepara y tampoco se recorren sus componentes (una ruta «Sin preparación», o «No necesitan
	// preparación» en un producto sin componentes).
	None     bool
	Stations []string
	// Reason es «no_route» (sin ruta propia, de su categoría ni heredada de su combo) o «inactive_station» (su ruta solo
	// iba a estaciones desactivadas) cuando el producto no llega a ninguna estación por una ruta; vacío si llega.
	Reason string
}

// decideRoute decide con la configuración vigente a dónde va un producto (etapa K1.1, K11-D1: un envío nunca se
// detiene por la configuración):
//   - con su ruta (la propia o la de su categoría) o, sin ella, a las estaciones de su combo (inherited), solo a las
//     estaciones activas: las desactivadas se omiten y se anotan;
//   - con «Sin preparación», sin parte (como siempre);
//   - si no le queda ninguna estación y tiene componentes (un combo), sin parte propia: cada componente se resuelve
//     solo. El ajuste nunca se aplica a un combo como «Sin preparación», que dejaría sin visitar sus componentes;
//   - si no le queda ninguna estación y no tiene componentes (un producto simple o un combo vacío), sigue el ajuste
//     «Productos sin ruta»: a la estación elegida si está activa o, si no, «No necesitan preparación» (sin parte).
//
// leaf dice si el producto no tiene componentes activos en este pedido.
func decideRoute(source routeSource, productID, categoryID string, inherited []string, leaf bool) (routeDecision, error) {
	configured, err := source.productRoute(productID, categoryID)
	if err != nil {
		return routeDecision{}, err
	}
	if configured == nil && len(inherited) > 0 {
		// Un componente sin ruta propia ni de su categoría llega a las estaciones que recibieron su combo.
		route := Route{Mode: "prepare", StationIDs: inherited}
		return routeDecision{Route: route, Freeze: true, Stations: inherited}, nil
	}
	if configured != nil && configured.Mode == "none" {
		return routeDecision{Route: *configured, Freeze: true, None: true}, nil
	}
	reason := "no_route"
	var omitted []string
	if configured != nil {
		active := []string{}
		for _, id := range configured.StationIDs {
			station, err := source.station(id)
			if err != nil {
				return routeDecision{}, err
			}
			if station.Active {
				active = append(active, id)
			} else {
				omitted = append(omitted, id)
			}
		}
		route := *configured
		route.StationIDs = active
		route.OmittedStationIDs = omitted
		if len(active) > 0 {
			return routeDecision{Route: route, Freeze: true, Stations: active}, nil
		}
		reason = "inactive_station"
	}
	if !leaf {
		decision := routeDecision{Reason: reason}
		if configured != nil {
			// Su ruta solo iba a estaciones desactivadas: se congela así, sin estaciones, para que una modificación siga
			// la misma decisión aunque después se reactive la estación.
			route := *configured
			route.StationIDs = []string{}
			route.OmittedStationIDs = omitted
			decision.Route, decision.Freeze = route, true
		}
		return decision, nil
	}
	route := Route{Mode: "none", StationIDs: []string{}, Unrouted: true, OmittedStationIDs: omitted}
	if configured != nil {
		route.ID, route.ProductID, route.CategoryID, route.Revision = configured.ID, configured.ProductID, configured.CategoryID, configured.Revision
	}
	unrouted, err := source.unroutedStation()
	if err != nil {
		return routeDecision{}, err
	}
	if unrouted != nil && unrouted.Active {
		route.Mode = "prepare"
		route.StationIDs = []string{unrouted.ID}
		return routeDecision{Route: route, Freeze: true, Stations: route.StationIDs, Reason: reason}, nil
	}
	return routeDecision{Route: route, Freeze: true, None: true, Reason: reason}, nil
}

// frozenDecision sigue la ruta congelada de algo ya enviado (K11-D2): las rutas congeladas no cambian. De sus
// estaciones solo reciben las activas; el aviso para una estación desactivada que ya tenía la comanda lo deja pendiente
// integrateChange, como siempre.
func frozenDecision(source routeSource, route Route) (routeDecision, error) {
	if route.Mode == "none" {
		return routeDecision{Route: route, Freeze: true, None: true}, nil
	}
	stations := []string{}
	for _, id := range route.StationIDs {
		station, err := source.station(id)
		if err != nil {
			return routeDecision{}, err
		}
		if station.Active {
			stations = append(stations, id)
		}
	}
	return routeDecision{Route: route, Freeze: true, Stations: stations}, nil
}

// activeComponents cuenta los componentes que el pedido sí lleva (un combo vacío no tiene ninguno).
func activeComponents(product catalog.ProductBreakdown) int {
	count := 0
	for _, component := range product.Components {
		if component.Product != nil && component.Quantity > 0 {
			count++
		}
	}
	return count
}

// resolveRouting reparte una unidad en partes por estación. Con frozen (la ruta congelada de una liberación) sigue esa
// decisión para los productos que ya tenía; los demás se deciden con la configuración vigente (decideRoute).
func resolveRouting(ctx context.Context, tx pgx.Tx, breakdown catalog.ProductBreakdown, snapshot catalog.Snapshot, frozen map[string]Route) (*routingResult, error) {
	source := newLiveRoutes(ctx, tx)
	result := &routingResult{Stations: map[string]Station{}, Parts: map[string][]Part{}, Snapshot: map[string]Route{}}
	var visit func(catalog.ProductBreakdown, string, int, []string) error
	visit = func(product catalog.ProductBreakdown, path string, quantity int, inherited []string) error {
		var decision routeDecision
		var err error
		if route, exists := frozen[product.ProductID]; exists {
			decision, err = frozenDecision(source, route)
		} else {
			decision, err = decideRoute(source, product.ProductID, product.CategoryID, inherited, activeComponents(product) == 0)
		}
		if err != nil {
			return err
		}
		if decision.Freeze {
			result.Snapshot[product.ProductID] = decision.Route
		}
		if decision.None {
			return nil
		}
		variantName := ""
		if product.VariantID != nil {
			for _, variant := range snapshot.Products[product.ProductID].Variants {
				if variant.ID == *product.VariantID {
					variantName = variant.Name
				}
			}
		}
		part := Part{Path: path, ProductID: product.ProductID, Name: product.Name, Variant: variantName, Quantity: quantity,
			Ingredients: preparationIngredients(product.Ingredients), Groups: preparationGroups(product.Groups, snapshot), Notes: product.Notes}
		// Las indicaciones rápidas resueltas viajan con su parte para que la comanda imprima una
		// sola vez la que ya cubre la línea estructurada de su ingrediente.
		part.QuickNotes = product.QuickNotes
		if path == "root" {
			part.Notes = ""
		}
		for _, id := range decision.Stations {
			station, err := source.station(id)
			if err != nil {
				return err
			}
			result.Stations[id] = station
			result.Parts[id] = append(result.Parts[id], part)
		}
		for index, component := range product.Components {
			if component.Product != nil && component.Quantity > 0 {
				if err := visit(*component.Product, fmt.Sprintf("%s/%d", path, index), quantity*component.Quantity, decision.Stations); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(breakdown, "root", 1, nil); err != nil {
		return nil, err
	}
	return result, nil
}
