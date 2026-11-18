package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.data.api.ApiJson
import com.adelpos.meseros.core.data.catalog.db.CatalogStateEntity
import com.adelpos.meseros.core.data.catalog.db.CategoryEntity
import com.adelpos.meseros.core.data.catalog.db.ComplementEntity
import com.adelpos.meseros.core.data.catalog.db.ImageRefEntity
import com.adelpos.meseros.core.data.catalog.db.IngredientEntity
import com.adelpos.meseros.core.data.catalog.db.OptionGroupEntity
import com.adelpos.meseros.core.data.catalog.db.PortionEntity
import com.adelpos.meseros.core.data.catalog.db.ProductEntity
import com.adelpos.meseros.core.data.catalog.db.QuickNoteEntity
import com.adelpos.meseros.core.domain.catalog.CatalogSearch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject

/**
 * Una descarga del catálogo ya interpretada y lista para guardarse: las filas de cada tabla y las
 * bajas. Se prepara **antes** de abrir la transacción, así un registro con una forma inesperada
 * hace fallar la descarga sin tocar el catálogo guardado.
 *
 * @property full la descarga trae el catálogo completo (sustituye lo guardado).
 * @property reset Go entregó todo porque la época o la versión no coincidían (base restaurada u
 *   otro servidor): además de reemplazar todo, las imágenes guardadas se revalidan con su `ETag`.
 * @property archivedProducts productos que llegaron con `archived: true` en una descarga de
 *   cambios: se quitan, como las bajas.
 */
class CatalogPayload(
    val state: CatalogStateEntity,
    val full: Boolean,
    val reset: Boolean,
    val categories: List<CategoryEntity>,
    val products: List<ProductEntity>,
    val archivedProducts: List<String>,
    val groups: List<OptionGroupEntity>,
    val ingredients: List<IngredientEntity>,
    val complements: List<ComplementEntity>,
    val portions: List<PortionEntity>,
    val quickNotes: List<QuickNoteEntity>,
    val images: List<ImageRefEntity>,
    val deleted: Map<String, List<String>>,
) {
    /** Cuántos registros trae la descarga (para el registro y las pruebas). */
    val recordCount: Int
        get() =
            categories.size +
                products.size +
                archivedProducts.size +
                groups.size +
                ingredients.size +
                complements.size +
                portions.size +
                quickNotes.size +
                deleted.values.sumOf { it.size }

    companion object {
        /** Entidades de `deleted` que se guardan en el dispositivo (las demás no tienen tabla). */
        const val CATEGORY = "category"
        const val PRODUCT = "product"
        const val INGREDIENT = "ingredient"
        const val COMPLEMENT = "complement"
        const val PORTION = "portion"
        const val GROUP = "group"
        const val QUICK_NOTE = "quick_note"
        const val IMAGE = "image"

        /** Lee el cuerpo de `GET catalog/sync`. Lanza `SerializationException` si no es válido. */
        fun parse(body: String, syncedAtMillis: Long, json: Json = ApiJson): CatalogPayload {
            val envelope = json.decodeFromString(CatalogSyncEnvelope.serializer(), body)
            return from(envelope, syncedAtMillis, json)
        }

        fun from(envelope: CatalogSyncEnvelope, syncedAtMillis: Long, json: Json): CatalogPayload {
            val state =
                CatalogStateEntity(
                    version = envelope.catalogVersion,
                    epoch = envelope.catalogEpoch,
                    generatedAt = envelope.generatedAt,
                    timezone = envelope.timezone,
                    taxRate = envelope.taxRate,
                    priceListId = envelope.priceList.id,
                    priceListName = envelope.priceList.name,
                    syncedAtMillis = syncedAtMillis,
                )
            val products = mutableListOf<ProductEntity>()
            val archived = mutableListOf<String>()
            for (document in envelope.products) {
                val product = json.decodeFromJsonElement(ProductDto.serializer(), document)
                if (product.archived) {
                    archived.add(product.id)
                    continue
                }
                products.add(productEntity(product, document))
            }
            val deleted = envelope.deleted.groupBy({ it.entity }, { it.id })
            return CatalogPayload(
                state = state,
                full = envelope.full,
                reset = envelope.reset,
                categories = envelope.categories.map { categoryEntity(json, it) },
                products = products,
                archivedProducts = archived,
                groups =
                    envelope.groups.map { document ->
                        val group =
                            json.decodeFromJsonElement(OptionGroupDto.serializer(), document)
                        OptionGroupEntity(id = group.id, document = document.toString())
                    },
                ingredients =
                    envelope.ingredients.map { document ->
                        val record = named(json, document)
                        IngredientEntity(record.id, record.name, document.toString())
                    },
                complements =
                    envelope.complements.map { document ->
                        val record = named(json, document)
                        ComplementEntity(record.id, record.name, document.toString())
                    },
                portions =
                    envelope.portions.map { document ->
                        val record = named(json, document)
                        PortionEntity(record.id, record.name, document.toString())
                    },
                quickNotes =
                    envelope.quickNotes.map { document ->
                        val note = json.decodeFromJsonElement(QuickNoteDto.serializer(), document)
                        QuickNoteEntity(
                            id = note.id,
                            categoryId = note.categoryId,
                            sortOrder = note.sortOrder,
                            document = document.toString(),
                        )
                    },
                images =
                    envelope.images.map { image ->
                        ImageRefEntity(
                            id = image.id,
                            mediaType = image.mediaType,
                            width = image.width,
                            height = image.height,
                            size = image.size,
                            checksum = image.checksum,
                        )
                    },
                deleted = deleted,
            )
        }

        private fun categoryEntity(json: Json, document: JsonObject): CategoryEntity {
            val category = json.decodeFromJsonElement(CategoryDto.serializer(), document)
            return CategoryEntity(
                id = category.id,
                name = category.name,
                sortOrder = category.sortOrder,
                active = category.active,
                document = document.toString(),
            )
        }

        private fun productEntity(product: ProductDto, document: JsonObject): ProductEntity =
            ProductEntity(
                id = product.id,
                categoryId = product.categoryId,
                name = product.name,
                sortOrder = product.sortOrder,
                active = product.active,
                searchKey = CatalogSearch.searchKey(product.name, product.code, product.barcode),
                imageId = product.imageId,
                document = document.toString(),
            )

        private fun named(json: Json, document: JsonObject): NamedRecordDto =
            json.decodeFromJsonElement(NamedRecordDto.serializer(), document)
    }
}
