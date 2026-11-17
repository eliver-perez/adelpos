package local.adel.pos.android.core.data.catalog

/** Descargas de ejemplo (`GET catalog/sync`) generadas para las pruebas en el dispositivo. */
object CatalogJsonSamples {
    private const val CATEGORY = "c9000000-0000-4000-8000-000000000001"

    /**
     * Descarga completa con [products] productos numerados («Producto 1»…); con [duplicateLast], el
     * último repite el identificador del primero.
     */
    fun full(version: Long, products: Int, duplicateLast: Boolean = false): String {
        val items =
            (1..products).map { number ->
                val id =
                    if (duplicateLast && number == products) {
                        productId(1)
                    } else {
                        productId(number)
                    }
                product(id, "Producto $number", number)
            }
        return """
            {
              "catalog_version": $version,
              "catalog_epoch": "6f1f5f3e-1c2b-4d3e-9a8b-7c6d5e4f3a21",
              "since": 0, "full": true, "reset": false,
              "generated_at": "2026-11-16T18:00:00Z", "timezone": "America/Monterrey", "tax_rate": 16,
              "price_list": {"id": "a9000000-0000-4000-8000-000000000001", "name": "General", "revision": 1},
              "categories": [{"id": "$CATEGORY", "name": "Prueba", "description": "", "active": true,
                "revision": 1, "sort_order": 1, "color": "#d64545", "image_id": null, "availability": {},
                "available_now": true}],
              "products": [${items.joinToString(",")}],
              "ingredients": [], "complements": [], "portions": [], "groups": [], "quick_notes": [],
              "images": [], "deleted": []
            }
            """
            .trimIndent()
    }

    private fun productId(number: Int): String = "b9000000-0000-4000-8000-%012d".format(number)

    private fun product(id: String, name: String, order: Int): String =
        """
        {"id": "$id", "name": "$name", "description": "", "active": true, "revision": 1,
         "category_id": "$CATEGORY", "code": "P$order", "barcode": null, "kind": "product",
         "base_price": "10.00", "image_id": null, "sort_order": $order, "archived": false,
         "temporary_unavailable": false, "availability": {}, "variants": [], "ingredients": [],
         "groups": [], "slots": [], "available_now": true, "availability_reason": "",
         "sold_out": false, "sold_out_indefinite": false, "sold_out_until": null,
         "effective_tax_rate": 16, "price": "10.00", "price_origin": "list", "pricing_error": null}
        """
            .trimIndent()
}
