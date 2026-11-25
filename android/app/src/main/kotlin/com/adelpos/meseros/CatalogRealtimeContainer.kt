package com.adelpos.meseros

import android.content.Context
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.realtime.AccessInterruptions
import com.adelpos.core.model.realtime.RealtimeFeed
import com.adelpos.core.network.realtime.RealtimeClient
import com.adelpos.core.network.realtime.RealtimeCursorSeed
import com.adelpos.core.network.session.ServerAccess
import com.adelpos.meseros.core.data.catalog.CatalogApi
import com.adelpos.meseros.core.data.catalog.CatalogImageCache
import com.adelpos.meseros.core.data.catalog.CatalogRepository
import com.adelpos.meseros.core.data.catalog.CatalogStore
import com.adelpos.meseros.core.data.catalog.CatalogSynchronizer
import com.adelpos.meseros.core.data.catalog.CatalogUpdateTriggers
import com.adelpos.meseros.core.data.catalog.db.CatalogDatabase
import com.adelpos.meseros.core.data.realtime.RecoveryStateCursorSeed
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow

/**
 * Catálogo en el dispositivo y canal de tiempo real (Fase D), una instancia por proceso:
 * - [catalog]: catálogo guardado (`catalog.db`) y sus descargas, para las pantallas;
 * - [images]: caché propia de imágenes;
 * - [realtime]: **el único** canal SSE del proceso; la Fase E se suscribe a él para cuentas, mesas
 *   y preparación (`realtime.events(RealtimeScopes.RESTAURANT)`, `realtime.rebuilds()`).
 *
 * Todo habla con Go a través de [access] (la interfaz estrecha del acceso) y avisa las
 * interrupciones de acceso a [interruptions].
 */
class CatalogRealtimeContainer(
    context: Context,
    private val access: ServerAccess,
    private val interruptions: AccessInterruptions,
    private val linkState: Flow<LinkState>,
    private val foreground: StateFlow<Boolean>,
    cursorSeed: RealtimeCursorSeed = RecoveryStateCursorSeed(),
) {
    private val appContext: Context = context.applicationContext
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val api = CatalogApi()
    private var started = false

    val database: CatalogDatabase by lazy { CatalogDatabase.open(appContext) }

    private val store: CatalogStore by lazy { CatalogStore(database) }

    private val imageCache: CatalogImageCache by lazy {
        CatalogImageCache(
            directory = File(appContext.cacheDir, CatalogImageCache.DIRECTORY_NAME),
            dao = database.catalog(),
            api = api,
            access = access,
        )
    }

    private val repository: CatalogRepository by lazy {
        CatalogRepository(
            store = store,
            synchronizer = CatalogSynchronizer(access, api, store, interruptions),
            images = imageCache,
            scope = scope,
        )
    }

    private val realtimeClient: RealtimeClient =
        RealtimeClient(
            access = access,
            active = foreground,
            interruptions = interruptions,
            cursorSeed = cursorSeed,
        )

    val catalog: CatalogSource
        get() = repository

    val images: CatalogImages
        get() = imageCache

    val realtime: RealtimeFeed
        get() = realtimeClient

    /**
     * «Borrar los datos de este dispositivo»: catalog.db, las imágenes guardadas y el catálogo en
     * memoria (registrado en `DeviceDataWipe` por `AppContainer`).
     */
    suspend fun eraseDeviceData() {
        repository.eraseAll()
    }

    /** Lee el catálogo guardado, abre el canal en primer plano y programa las actualizaciones. */
    fun start() {
        if (started) {
            return
        }
        started = true
        repository.start()
        realtimeClient.start(scope)
        CatalogUpdateTriggers(
                request = repository::requestRefresh,
                access = access,
                foreground = foreground,
                link = linkState,
                realtime = realtimeClient,
            )
            .start(scope)
    }
}
