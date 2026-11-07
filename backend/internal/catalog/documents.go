package catalog

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// Cada documento expone únicamente campos del contrato y convierte los numeric monetarios en cadenas.
var documentQueries = map[string]string{
	"categories":  `SELECT to_jsonb(c) AS data FROM app.catalog_categories c`,
	"ingredients": `SELECT to_jsonb(i) AS data FROM app.catalog_ingredients i`,
	"complements": `SELECT to_jsonb(c) AS data FROM app.catalog_complements c`,
	"portions":    `SELECT to_jsonb(p) || jsonb_build_object('factor',p.factor::text) AS data FROM app.catalog_portions p`,
	"groups": `SELECT to_jsonb(g) || jsonb_build_object(
        'min_quantity',g.min_quantity::text,'max_quantity',g.max_quantity::text,
        'options',coalesce((SELECT jsonb_agg(to_jsonb(o) - 'group_id' || jsonb_build_object(
            'reference_value',o.reference_value::text,'extra_price',o.extra_price::text,
            'max_quantity',o.max_quantity::text,'name',c.name,'portion_name',coalesce(p.name,'Porción'),
            'portion_factor',coalesce(p.factor::text,'1.000'),
            'tiers',coalesce((SELECT jsonb_agg(jsonb_build_object('from',t.from_quantity,'unit_price',t.unit_price::text)
                ORDER BY t.from_quantity) FROM app.catalog_option_tiers t WHERE t.option_id=o.id),'[]')) ORDER BY o.id)
            FROM app.catalog_group_options o JOIN app.catalog_complements c ON c.id=o.complement_id
            LEFT JOIN app.catalog_portions p ON p.id=o.portion_id WHERE o.group_id=g.id),'[]')) AS data
        FROM app.catalog_option_groups g`,
	"products": `SELECT to_jsonb(p) || jsonb_build_object('base_price',p.base_price::text,
        'variants',coalesce((SELECT jsonb_agg(to_jsonb(v) - 'product_id' || jsonb_build_object('price',v.price::text) ORDER BY v.name)
            FROM app.catalog_variants v WHERE v.product_id=p.id),'[]'),
        'ingredients',coalesce((SELECT jsonb_agg(to_jsonb(i) - 'product_id' || jsonb_build_object(
            'included_quantity',i.included_quantity::text,'max_quantity',i.max_quantity::text,
            'extra_price',i.extra_price::text,'recipe_quantity',i.recipe_quantity::text,'name',n.name) ORDER BY n.name)
            FROM app.catalog_product_ingredients i JOIN app.catalog_ingredients n ON n.id=i.ingredient_id WHERE i.product_id=p.id),'[]'),
        'groups',coalesce((SELECT jsonb_agg(to_jsonb(g) - 'product_id' || jsonb_build_object(
            'min_quantity',g.min_quantity::text,'max_quantity',g.max_quantity::text,
            'included',coalesce((SELECT jsonb_agg(jsonb_build_object('option_id',d.option_id,'quantity',d.quantity::text) ORDER BY d.option_id)
                FROM app.catalog_group_defaults d WHERE d.product_id=p.id AND d.group_id=g.group_id),'[]')) ORDER BY g.group_id)
            FROM app.catalog_product_groups g WHERE g.product_id=p.id),'[]'),
        'slots',coalesce((SELECT jsonb_agg(to_jsonb(s) - 'combo_id' || jsonb_build_object(
            'options',coalesce((SELECT jsonb_agg(to_jsonb(o) - 'slot_id' || jsonb_build_object(
                'substitution_price',o.substitution_price::text,
                'tiers',coalesce((SELECT jsonb_agg(jsonb_build_object('from',t.from_quantity,'unit_price',t.unit_price::text) ORDER BY t.from_quantity)
                    FROM app.catalog_combo_tiers t WHERE t.slot_id=s.id AND t.product_id=o.product_id),'[]')) ORDER BY o.product_id)
                FROM app.catalog_combo_options o WHERE o.slot_id=s.id),'[]')) ORDER BY s.id)
            FROM app.catalog_combo_slots s WHERE s.combo_id=p.id),'[]')) AS data
        FROM app.catalog_products p`,
	"price-lists": `SELECT to_jsonb(l) || jsonb_build_object(
        'entries',coalesce((SELECT jsonb_agg(jsonb_build_object('product_id',e.product_id,'variant_id',e.variant_id,'price',e.price::text)
            ORDER BY e.product_id,e.variant_id) FROM app.catalog_price_entries e WHERE e.list_id=l.id),'[]')) AS data
        FROM app.catalog_price_lists l`,
	"promotions": `SELECT to_jsonb(p) || jsonb_build_object('percent',p.percent::text,'special_price',p.special_price::text,
        'product_ids',coalesce((SELECT jsonb_agg(x.product_id ORDER BY x.product_id) FROM app.catalog_promotion_products x WHERE x.promotion_id=p.id),'[]'),
        'category_ids',coalesce((SELECT jsonb_agg(x.category_id ORDER BY x.category_id) FROM app.catalog_promotion_categories x WHERE x.promotion_id=p.id),'[]'),
        'price_list_ids',coalesce((SELECT jsonb_agg(x.list_id ORDER BY x.list_id) FROM app.catalog_promotion_lists x WHERE x.promotion_id=p.id),'[]')) AS data
        FROM app.catalog_promotions p`,
}

func newDocument(resource string) any {
	switch resource {
	case "categories":
		return &Category{}
	case "products":
		return &Product{}
	case "ingredients":
		return &Ingredient{}
	case "complements":
		return &Complement{}
	case "portions":
		return &Portion{}
	case "groups":
		return &OptionGroup{}
	case "price-lists":
		return &PriceList{}
	case "promotions":
		return &Promotion{}
	}
	return nil
}

func readDocument(ctx context.Context, tx pgx.Tx, resource, id string) (any, error) {
	var encoded []byte
	aliases := map[string]string{
		"categories":  "c",
		"products":    "p",
		"ingredients": "i",
		"complements": "c",
		"portions":    "p",
		"groups":      "g",
		"price-lists": "l",
		"promotions":  "p",
	}
	query := documentQueries[resource] + " WHERE " + aliases[resource] + ".id=$1"
	if err := tx.QueryRow(ctx, query, id).Scan(&encoded); err != nil {
		return nil, err
	}
	result := newDocument(resource)
	if err := json.Unmarshal(encoded, result); err != nil {
		return nil, err
	}
	return result, nil
}

func readDocuments(ctx context.Context, tx pgx.Tx, resource string) ([]any, error) {
	rows, err := tx.Query(ctx, "SELECT data FROM ("+documentQueries[resource]+") records ORDER BY data->>'name',data->>'id'")
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
