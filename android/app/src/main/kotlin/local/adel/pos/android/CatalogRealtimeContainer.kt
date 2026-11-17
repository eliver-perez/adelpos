package local.adel.pos.android

import android.content.Context
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import local.adel.pos.android.core.data.catalog.CatalogApi
import local.adel.pos.android.core.data.catalog.CatalogImageCache
import local.adel.pos.android.core.data.catalog.CatalogRepository
import local.adel.pos.android.core.data.catalog.CatalogStore
import local.adel.pos.android.core.data.catalog.CatalogSynchronizer
import local.adel.pos.android.core.data.catalog.CatalogUpdateTriggers
import local.adel.pos.android.core.data.catalog.db.CatalogDatabase
import local.adel.pos.android.core.data.realtime.RealtimeClient
import local.adel.pos.android.core.data.realtime.RealtimeCursorSeed
import local.adel.pos.android.core.data.realtime.RecoveryStateCursorSeed
import local.adel.pos.android.core.data.session.ServerAccess
import local.adel.pos.android.core.domain.catalog.CatalogImages
import local.adel.pos.android.core.domain.catalog.CatalogSource
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.realtime.AccessInterruptions
import local.adel.pos.android.core.domain.realtime.RealtimeFeed

/**
 * Si la aplicación está en primer plano (alguna pantalla visible), según el ciclo de vida del
 * proceso (`ProcessLifecycleOwner`, que espera unos 700 ms al pasar a segundo plano para no
 * confundir una rotación con una salida).
 */
class AppForeground : DefaultLifecycleObserver {
    private val state = MutableStateFlow(false)

    val inForeground: StateFlow<Boolean> = state.asStateFlow()

    override fun onStart(owner: LifecycleOwner) {
        state.value = true
    }

    override fun onStop(owner: LifecycleOwner) {
        state.value = false
    }
}

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
