package reports

// Filtros de los reportes de inventarios, costos y compras (fase B2). Se agregan a la lista cerrada
// de fuentes de GET reports/filters/{source} y a la de nombres legibles de los metadatos; como las
// demás, solo devuelven nombres y estados, nunca existencias, costos ni importes.

func init() {
	sources := map[string]string{
		"warehouses": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactivo' END AS detail
            FROM app.inventory_warehouses ORDER BY is_default DESC,name`,
		"inventory_items": `SELECT id::text AS value,name AS label,code AS detail FROM app.inventory_items
            WHERE @search='' OR name ILIKE '%'||@search||'%' OR code ILIKE '%'||@search||'%'
            ORDER BY active DESC,name LIMIT 100`,
		"inventory_categories": `SELECT id::text AS value,name AS label,CASE WHEN active THEN '' ELSE 'Inactiva' END AS detail
            FROM app.inventory_categories ORDER BY name`,
		"suppliers": `SELECT id::text AS value,trade_name AS label,
                CASE WHEN kind='counter' THEN 'Compras de mostrador' WHEN active THEN '' ELSE 'Inactivo' END AS detail
            FROM app.inventory_suppliers ORDER BY kind DESC,trade_name`,
	}
	names := map[string]string{
		"warehouses":           `SELECT name FROM app.inventory_warehouses WHERE id=@value::uuid`,
		"inventory_items":      `SELECT code||' · '||name FROM app.inventory_items WHERE id=@value::uuid`,
		"inventory_categories": `SELECT name FROM app.inventory_categories WHERE id=@value::uuid`,
		"suppliers":            `SELECT trade_name FROM app.inventory_suppliers WHERE id=@value::uuid`,
	}
	for key, query := range sources {
		if _, exists := filterSources[key]; exists {
			panic("fuente de filtro duplicada: " + key)
		}
		filterSources[key] = query
		filterNames[key] = names[key]
	}
}

func filterWarehouse() Filter { return idFilter("warehouse_id", "Almacén", "warehouses") }
func filterItem() Filter      { return idFilter("item_id", "Insumo", "inventory_items") }
func filterInventoryCategory() Filter {
	return idFilter("category_id", "Categoría de insumo", "inventory_categories")
}
func filterSupplier() Filter { return idFilter("supplier_id", "Proveedor", "suppliers") }
