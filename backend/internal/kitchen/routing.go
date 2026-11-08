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
	Warnings []string
}

func resolveRouting(ctx context.Context, tx pgx.Tx, breakdown catalog.ProductBreakdown, snapshot catalog.Snapshot, frozen map[string]Route) (*routingResult, error) {
	result := &routingResult{Stations: map[string]Station{}, Parts: map[string][]Part{}, Snapshot: map[string]Route{}, Warnings: []string{}}
	var visit func(catalog.ProductBreakdown, string, int, []string) error
	visit = func(product catalog.ProductBreakdown, path string, quantity int, inherited []string) error {
		route, exists := frozen[product.ProductID]
		if !exists {
			routes, err := many[Route](ctx, tx, routeQuery+`
				WHERE r.product_id=$1 OR r.category_id=$2
				ORDER BY (r.product_id IS NOT NULL) DESC LIMIT 1`, product.ProductID, product.CategoryID)
			if err != nil {
				return err
			}
			if len(routes) > 0 {
				route, exists = routes[0], true
			}
		}
		if !exists && len(inherited) > 0 {
			route = Route{Mode: "prepare", StationIDs: inherited}
			exists = true
		}
		if exists {
			result.Snapshot[product.ProductID] = route
		}
		if exists && route.Mode == "none" {
			return nil
		}
		destinations := route.StationIDs
		activeComponents := 0
		for _, component := range product.Components {
			if component.Product != nil && component.Quantity > 0 {
				activeComponents++
			}
		}
		if !exists && activeComponents == 0 {
			result.Warnings = append(result.Warnings, "Sin ruta de preparación: "+product.Name)
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
		if path == "root" {
			part.Notes = ""
		}
		for _, id := range destinations {
			station := result.Stations[id]
			if station.ID == "" {
				found, err := one[Station](ctx, tx, "SELECT to_jsonb(s) FROM app.kitchen_stations s WHERE id=$1", id)
				if err != nil {
					return err
				}
				station = *found
				result.Stations[id] = station
			}
			if !station.Active {
				result.Warnings = append(result.Warnings, "Estación inactiva: "+station.Name)
			}
			result.Parts[id] = append(result.Parts[id], part)
		}
		for index, component := range product.Components {
			if component.Product != nil && component.Quantity > 0 {
				if err := visit(*component.Product, fmt.Sprintf("%s/%d", path, index), quantity*component.Quantity, destinations); err != nil {
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
