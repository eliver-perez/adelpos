package local.adel.pos.android.core.data.catalog

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.CatalogQuery
import local.adel.pos.android.core.domain.catalog.CatalogSource
import local.adel.pos.android.core.domain.catalog.CatalogSyncStatus

/**
 * Catálogo del dispositivo para las pantallas ([CatalogSource]): la fotografía guardada en
 * catalog.db, las búsquedas sobre ella y las descargas.
 *
 * Las peticiones de actualización ([requestRefresh]) llegan de varios lados (abrir la aplicación,
 * recuperar la red, el evento `catalog.changed`, el temporizador). Se atienden **de una en una**:
 * un canal de capacidad uno («conflated») guarda como mucho una petición pendiente, así que si
 * llegan varias mientras una descarga corre, al terminar se hace exactamente una más.
 */
class CatalogRepository(
    private val store: CatalogStore,
    private val synchronizer: CatalogSynchronizer,
    private val images: CatalogImageCache?,
    private val scope: CoroutineScope,
    private val clock: () -> Long = System::currentTimeMillis,
) : CatalogSource {
    private val snapshot = MutableStateFlow(Catalog.EMPTY)
    private val syncStatus = MutableStateFlow(CatalogSyncStatus())
    private val requests = Channel<Unit>(Channel.CONFLATED)
    private var started = false

    // Una descarga y un borrado nunca se mezclan: lo que llegue después de borrar se guarda sobre
    // un catálogo vacío, nunca a medias con el anterior.
    private val work = Mutex()

    override val catalog: StateFlow<Catalog> = snapshot.asStateFlow()

    override val status: StateFlow<CatalogSyncStatus> = syncStatus.asStateFlow()

    /** Lee el catálogo guardado y empieza a atender las peticiones de actualización. */
    fun start() {
        if (started) {
            return
        }
        started = true
        scope.launch {
            snapshot.value = store.load()
            for (request in requests) {
                refreshNow()
            }
        }
    }

    override fun requestRefresh() {
        requests.trySend(Unit)
    }

    /** Descarga ya (esperando a la que esté en curso) y actualiza el catálogo en memoria. */
    suspend fun refreshNow(): CatalogSyncOutcome = work.withLock { refreshLocked() }

    /**
     * «Borrar los datos de este dispositivo» (retirarlo o emparejarlo de nuevo): catalog.db, las
     * imágenes guardadas y la fotografía en memoria. Espera a la descarga en curso.
     */
    suspend fun eraseAll() {
        work.withLock {
            images?.eraseAll()
            store.eraseAll()
            snapshot.value = Catalog.EMPTY
            syncStatus.value = CatalogSyncStatus()
        }
    }

    private suspend fun refreshLocked(): CatalogSyncOutcome {
        syncStatus.update { it.copy(running = true) }
        val outcome = synchronizer.synchronize()
        when (outcome) {
            CatalogSyncOutcome.NoSession ->
                syncStatus.update { it.copy(running = false, waitingForSession = true) }
            is CatalogSyncOutcome.Failed ->
                syncStatus.update {
                    it.copy(running = false, waitingForSession = false, lastError = outcome.error)
                }
            is CatalogSyncOutcome.Saved -> {
                // Sin cambios (misma versión y ningún registro) la fotografía en memoria sigue
                // igual y no hace falta volver a leer la base. La versión importa: la cola sin
                // conexión la envía con cada pedido.
                val sameVersion = snapshot.value.version?.version == outcome.version
                if (outcome.full || outcome.records > 0 || !sameVersion) {
                    snapshot.value = store.load()
                }
                syncStatus.update {
                    CatalogSyncStatus(running = false, lastSuccessMillis = clock())
                }
                refreshImages()
            }
        }
        return outcome
    }

    override suspend fun search(query: CatalogQuery): List<CatalogProduct> {
        val current = snapshot.value
        if (!query.searching) {
            val categoryId = query.categoryId ?: return current.products
            return current.productsIn(categoryId)
        }
        // La búsqueda recorre todo el catálogo, como en el escritorio: el mesero encuentra el
        // producto sin saber en qué categoría está.
        return store.searchProductIds(query.text, null).mapNotNull { current.product(it) }
    }

    private fun refreshImages() {
        val cache = images ?: return
        scope.launch {
            val referenced = store.referencedImageIds()
            cache.prune(referenced)
            cache.prefetch(prefetchOrder(referenced))
        }
    }

    // Primero las imágenes de los productos en el orden de la captura, después las demás
    // (categorías).
    private fun prefetchOrder(referenced: Set<String>): List<String> {
        val ordered = snapshot.value.products.mapNotNull { it.imageId }.filter { it in referenced }
        return (ordered + referenced).distinct()
    }
}
