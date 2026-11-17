package local.adel.pos.android.core.data.catalog

import androidx.room.withTransaction
import local.adel.pos.android.core.data.catalog.db.CatalogDao
import local.adel.pos.android.core.data.catalog.db.CatalogDatabase
import local.adel.pos.android.core.data.catalog.db.CatalogStateEntity
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogSearch

/**
 * Lectura y escritura de catalog.db. Cada descarga se guarda en **una sola transacción**: o queda
 * el catálogo nuevo completo, o el anterior intacto (nunca uno a medias, que podría capturar con
 * reglas mezcladas).
 */
class CatalogStore(
    private val database: CatalogDatabase,
    private val mapper: CatalogMapper = CatalogMapper(),
) {
    private val dao: CatalogDao = database.catalog()

    suspend fun state(): CatalogStateEntity? = dao.state()

    /**
     * Guarda una descarga: con [CatalogPayload.full] reemplaza todo lo guardado; si no, aplica los
     * cambios y las bajas. Con `reset`, además, cada imagen guardada se vuelve a preguntar con su
     * `ETag` (Go responde 304 si no cambió).
     */
    suspend fun save(payload: CatalogPayload) {
        database.withTransaction {
            if (payload.full) {
                replaceAll(payload)
            } else {
                applyChanges(payload)
            }
            dao.saveState(payload.state)
        }
    }

    private suspend fun replaceAll(payload: CatalogPayload) {
        dao.clearCategories()
        dao.clearProducts()
        dao.clearGroups()
        dao.clearIngredients()
        dao.clearComplements()
        dao.clearPortions()
        dao.clearQuickNotes()
        dao.clearImages()
        dao.insertCategories(payload.categories)
        dao.insertProducts(payload.products)
        dao.insertGroups(payload.groups)
        dao.insertIngredients(payload.ingredients)
        dao.insertComplements(payload.complements)
        dao.insertPortions(payload.portions)
        dao.insertQuickNotes(payload.quickNotes)
        dao.insertImages(payload.images)
        if (payload.reset) {
            dao.markImageFilesForCheck()
        }
    }

    private suspend fun applyChanges(payload: CatalogPayload) {
        dao.upsertCategories(payload.categories)
        dao.upsertProducts(payload.products)
        dao.upsertGroups(payload.groups)
        dao.upsertIngredients(payload.ingredients)
        dao.upsertComplements(payload.complements)
        dao.upsertPortions(payload.portions)
        dao.upsertQuickNotes(payload.quickNotes)
        dao.upsertImages(payload.images)
        // Un producto archivado ya no se puede pedir: se quita como una baja.
        inChunks(payload.archivedProducts) { dao.deleteProducts(it) }
        for ((entity, ids) in payload.deleted) {
            when (entity) {
                CatalogPayload.CATEGORY -> inChunks(ids) { dao.deleteCategories(it) }
                CatalogPayload.PRODUCT -> inChunks(ids) { dao.deleteProducts(it) }
                CatalogPayload.GROUP -> inChunks(ids) { dao.deleteGroups(it) }
                CatalogPayload.INGREDIENT -> inChunks(ids) { dao.deleteIngredients(it) }
                CatalogPayload.COMPLEMENT -> inChunks(ids) { dao.deleteComplements(it) }
                CatalogPayload.PORTION -> inChunks(ids) { dao.deletePortions(it) }
                CatalogPayload.QUICK_NOTE -> inChunks(ids) { dao.deleteQuickNotes(it) }
                CatalogPayload.IMAGE -> inChunks(ids) { dao.deleteImages(it) }
                // price_list y tax no tienen tabla: la lista predeterminada y la tasa general
                // llegan en cada descarga y los precios de los productos ya vienen calculados.
                else -> Unit
            }
        }
    }

    /** El catálogo guardado completo, para la captura. */
    suspend fun load(): Catalog = database.withTransaction {
        mapper.toCatalog(
            state = dao.state(),
            categories = dao.categories(),
            products = dao.products(),
            groups = dao.groups(),
            ingredients = dao.ingredients(),
            complements = dao.complements(),
            quickNotes = dao.quickNotes(),
        )
    }

    /**
     * Identificadores de los productos activos que coinciden con el texto (sin acentos ni
     * mayúsculas), en el orden de la captura. SQLite filtra por la primera palabra y aquí se exigen
     * las demás.
     */
    suspend fun searchProductIds(text: String, categoryId: String?): List<String> {
        val tokens = CatalogSearch.tokens(text)
        if (tokens.isEmpty()) {
            return emptyList()
        }
        val rows = dao.searchProducts("%" + escapeLike(tokens.first()) + "%", categoryId)
        return rows.filter { CatalogSearch.matches(it.searchKey, tokens) }.map { it.id }
    }

    /** Ids de las imágenes que usa el catálogo guardado. */
    suspend fun referencedImageIds(): Set<String> = dao.images().map { it.id }.toSet()

    suspend fun productCount(): Int = dao.productCount()

    internal fun dao(): CatalogDao = dao

    private suspend fun inChunks(ids: List<String>, action: suspend (List<String>) -> Unit) {
        for (chunk in ids.chunked(CHUNK_SIZE)) {
            action(chunk)
        }
    }

    companion object {
        // Android 8 trae SQLite 3.18: como mucho 999 variables por consulta.
        private const val CHUNK_SIZE = 500

        /** Escapa los comodines de LIKE (`%`, `_`) y el propio escape. */
        fun escapeLike(text: String): String =
            text.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
    }
}
