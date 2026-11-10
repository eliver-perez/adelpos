package inventory

// Destinos de receta: productos (y sus variantes) y complementos del catálogo con su estado de
// receta, para la pestaña Recetas y para la alerta de productos sin receta (fase D).
//   - status: with_recipe (receta activa propia o, en una variante, la del producto),
//     no_preparation (sin receta y con ruta de cocina «Sin preparación») o without_recipe;
//   - configured: la unidad consumiría algo aunque no tenga receta (ingredientes con porción
//     habitual, o el producto o ingrediente vinculado del complemento).
// Los productos archivados no se listan.

var targetStatuses = map[string]bool{"with_recipe": true, "without_recipe": true, "no_preparation": true}

// RecipeTarget es un producto, variante o complemento del catálogo con su receta.
type RecipeTarget struct {
	Kind                   string  `json:"kind"`
	ProductID              *string `json:"product_id"`
	VariantID              *string `json:"variant_id"`
	ComplementID           *string `json:"complement_id"`
	Name                   string  `json:"name"`
	VariantName            *string `json:"variant_name"`
	CategoryName           *string `json:"category_name"`
	ProductKind            *string `json:"product_kind"`
	Active                 bool    `json:"active"`
	RecipeID               *string `json:"recipe_id"`
	RecipeName             *string `json:"recipe_name"`
	RecipeActive           *bool   `json:"recipe_active"`
	InheritedRecipeID      *string `json:"inherited_recipe_id"`
	Status                 string  `json:"status"`
	KitchenRouteMode       *string `json:"kitchen_route_mode"`
	IngredientsTotal       int     `json:"ingredients_total"`
	IngredientsWithPortion int     `json:"ingredients_with_portion"`
	LinkedProductName      *string `json:"linked_product_name"`
	LinkedIngredientName   *string `json:"linked_ingredient_name"`
	Configured             bool    `json:"configured"`
}

// RecipeTargetPage es una página de destinos de receta.
type RecipeTargetPage struct {
	Items    []RecipeTarget `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// targetsQuery calcula el estado de cada destino a partir de sus recetas, su ruta de cocina y las
// porciones vigentes de sus ingredientes.
const targetsQuery = `WITH current_portions AS (
        SELECT DISTINCT ON (ingredient_id) ingredient_id, item_id FROM app.inventory_ingredient_portions
        ORDER BY ingredient_id, effective_from DESC, seq DESC
    ), targets AS (
        SELECT 'product'::text AS kind, p.id AS product_id, NULL::uuid AS variant_id, NULL::uuid AS complement_id, p.name,
            NULL::text AS variant_name, c.name AS category_name, p.kind AS product_kind, p.active AS active, p.category_id
        FROM app.catalog_products p JOIN app.catalog_categories c ON c.id=p.category_id WHERE NOT p.archived
        UNION ALL
        SELECT 'variant', p.id, v.id, NULL, p.name, v.name, c.name, p.kind, p.active AND v.active, p.category_id
        FROM app.catalog_variants v JOIN app.catalog_products p ON p.id=v.product_id
        JOIN app.catalog_categories c ON c.id=p.category_id WHERE NOT p.archived
        UNION ALL
        SELECT 'complement', NULL, NULL, m.id, m.name, NULL, NULL, NULL, m.active, NULL FROM app.catalog_complements m
    ), described AS (
        SELECT t.*, own.id AS recipe_id, own.name AS recipe_name, own.active AS recipe_active, base.id AS inherited_recipe_id,
            base.active AS inherited_active,
            (SELECT r.mode FROM app.kitchen_routes r WHERE r.product_id=t.product_id OR r.category_id=t.category_id
                ORDER BY (r.product_id IS NOT NULL) DESC LIMIT 1) AS route_mode,
            (SELECT count(*) FROM app.catalog_product_ingredients pi WHERE pi.product_id=t.product_id)::int AS ingredients_total,
            (SELECT count(*) FROM app.catalog_product_ingredients pi JOIN current_portions cp ON cp.ingredient_id=pi.ingredient_id
                WHERE pi.product_id=t.product_id AND cp.item_id IS NOT NULL)::int AS ingredients_with_portion,
            linked.name AS linked_product_name, linked_ingredient.name AS linked_ingredient_name,
            (linked.id IS NOT NULL AND EXISTS(SELECT FROM app.inventory_recipes lr WHERE lr.owner_kind='product'
                AND lr.product_id=linked.id AND lr.variant_id IS NULL AND lr.active))
            OR EXISTS(SELECT FROM current_portions cp WHERE cp.ingredient_id=m.ingredient_id AND cp.item_id IS NOT NULL)
            OR (linked.id IS NOT NULL AND EXISTS(SELECT FROM app.catalog_product_ingredients pi JOIN current_portions cp
                ON cp.ingredient_id=pi.ingredient_id WHERE pi.product_id=linked.id AND cp.item_id IS NOT NULL)) AS linked_configured
        FROM targets t
        LEFT JOIN app.inventory_recipes own ON (t.kind='product' AND own.owner_kind='product' AND own.product_id=t.product_id
                AND own.variant_id IS NULL)
            OR (t.kind='variant' AND own.owner_kind='product' AND own.product_id=t.product_id AND own.variant_id=t.variant_id)
            OR (t.kind='complement' AND own.owner_kind='complement' AND own.complement_id=t.complement_id)
        LEFT JOIN app.inventory_recipes base ON t.kind='variant' AND base.owner_kind='product' AND base.product_id=t.product_id
            AND base.variant_id IS NULL
        LEFT JOIN app.catalog_complements m ON m.id=t.complement_id
        LEFT JOIN app.catalog_products linked ON linked.id=m.product_id
        LEFT JOIN app.catalog_ingredients linked_ingredient ON linked_ingredient.id=m.ingredient_id
    ), classified AS (
        SELECT d.*, CASE
                WHEN coalesce(d.recipe_active,false) OR coalesce(d.inherited_active,false) THEN 'with_recipe'
                WHEN d.route_mode='none' THEN 'no_preparation'
                ELSE 'without_recipe' END AS status
        FROM described d
    )
    SELECT kind,product_id,variant_id,complement_id,name,variant_name,category_name,product_kind,active,recipe_id,recipe_name,
        recipe_active,inherited_recipe_id,status,route_mode,ingredients_total,ingredients_with_portion,linked_product_name,
        linked_ingredient_name,status='with_recipe' OR ingredients_with_portion>0 OR coalesce(linked_configured,false)
    FROM classified`

// listRecipeTargets lista destinos con búsqueda, tipo (product, variant, complement), estado y
// paginación.
func (handler *service) listRecipeTargets(_ []string) (any, error) {
	query := handler.request.Query
	if err := allowQuery(query, "search", "kind", "state", "active", "page", "page_size"); err != nil {
		return nil, err
	}
	page, size, err := pagination(query)
	if err != nil {
		return nil, err
	}
	set := filters{}
	pattern, err := searchPattern(query)
	if err != nil {
		return nil, err
	}
	if pattern != "" {
		set.add("(name ILIKE $%[1]d OR variant_name ILIKE $%[1]d OR category_name ILIKE $%[1]d)", pattern)
	}
	if kind := query.Get("kind"); kind != "" {
		if kind != "product" && kind != "variant" && kind != "complement" {
			return nil, invalid("El filtro de tipo de destino no es válido.")
		}
		set.add("kind=$%d", kind)
	}
	if state := query.Get("state"); state != "" {
		if !targetStatuses[state] {
			return nil, invalid("El filtro de estado de receta no es válido.")
		}
		set.add("status=$%d", state)
	}
	active, err := optionalBool(query, "active", "estado")
	if err != nil {
		return nil, err
	}
	if active != nil {
		set.add("active=$%d", *active)
	}
	result := RecipeTargetPage{Items: []RecipeTarget{}, Page: page, PageSize: size}
	inner := "SELECT * FROM (" + targetsQuery + ") listed(kind,product_id,variant_id,complement_id,name,variant_name,category_name," +
		"product_kind,active,recipe_id,recipe_name,recipe_active,inherited_recipe_id,status,route_mode,ingredients_total," +
		"ingredients_with_portion,linked_product_name,linked_ingredient_name,configured)" + set.where()
	if err := handler.tx.QueryRow(handler.ctx, "SELECT count(*) FROM ("+inner+") counted", set.arguments...).Scan(&result.Total); err != nil {
		return nil, err
	}
	rows, err := handler.tx.Query(handler.ctx, inner+` ORDER BY active DESC, array_position(ARRAY['product','variant','complement'],kind),
        lower(name), lower(coalesce(variant_name,'')), coalesce(product_id,complement_id), variant_id NULLS FIRST LIMIT `+set.next(size)+
		" OFFSET "+set.next((page-1)*size), set.arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var target RecipeTarget
		if err := rows.Scan(&target.Kind, &target.ProductID, &target.VariantID, &target.ComplementID, &target.Name, &target.VariantName,
			&target.CategoryName, &target.ProductKind, &target.Active, &target.RecipeID, &target.RecipeName, &target.RecipeActive,
			&target.InheritedRecipeID, &target.Status, &target.KitchenRouteMode, &target.IngredientsTotal,
			&target.IngredientsWithPortion, &target.LinkedProductName, &target.LinkedIngredientName, &target.Configured); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, target)
	}
	return result, rows.Err()
}
