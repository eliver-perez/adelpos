package local.adel.pos.android

import android.content.Context
import android.os.Build
import android.os.SystemClock
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.data.access.PairingRepository
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.discovery.HttpsServerProbe
import local.adel.pos.android.core.data.discovery.NsdServerBrowser
import local.adel.pos.android.core.data.network.ConnectionMonitor
import local.adel.pos.android.core.data.preferences.DevicePreferencesRepository
import local.adel.pos.android.core.data.security.KeystoreSecretSealer
import local.adel.pos.android.core.data.session.DeviceServerAccess
import local.adel.pos.android.core.data.waiter.DeviceWaiterServer
import local.adel.pos.android.core.data.waiter.LostResponses
import local.adel.pos.android.core.data.waiter.OrderQueueRepository
import local.adel.pos.android.core.data.waiter.OrderSendSignals
import local.adel.pos.android.core.data.waiter.OrderWorkerFactory
import local.adel.pos.android.core.data.waiter.WaiterApi
import local.adel.pos.android.core.data.waiter.WorkManagerOrderScheduler
import local.adel.pos.android.core.data.wipe.DeviceDataWipe
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.access.UnsentOrdersSource
import local.adel.pos.android.core.domain.access.WaiterWorkGate
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.connection.PendingOrdersSource
import local.adel.pos.android.core.domain.security.SecretSealer
import local.adel.pos.android.core.domain.server.ServerAnnouncements
import local.adel.pos.android.core.domain.server.ServerRelocator
import local.adel.pos.android.feature.access.AccessDependencies

/**
 * Dependencias compartidas de la aplicación (inyección manual): una instancia de cada cosa por
 * proceso, creada al primer uso. Los ViewModel las reciben desde sus fábricas (`initializer {}`);
 * nada se busca por su cuenta.
 */
class AppContainer(context: Context, private val settings: Settings = Settings()) {
    /**
     * Ajustes que solo cambian las pruebas instrumentadas (`AdelTestRunner`), antes del primer uso.
     *
     * @param clientVersion la versión que se informa al servidor (`X-Client-Version`).
     * @param discoveryServiceType el tipo NSD que se busca: las pruebas usan uno propio para no
     *   encontrar nunca el servidor real del restaurante.
     * @param lostResponses solo pruebas: respuestas de Go que se descartan después de procesarse
     *   (la «respuesta perdida» de §9); null en la aplicación.
     */
    data class Settings(
        val clientVersion: String = BuildConfig.VERSION_NAME,
        val discoveryServiceType: String = ServerAnnouncements.SERVICE_TYPE,
        val lostResponses: LostResponses? = null,
    )

    private val appContext: Context = context.applicationContext

    /** Trabajo de fondo que sobrevive a las pantallas (comprobaciones, revocación). */
    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /**
     * Identificación ante el servidor: la versión sale de versionName (android/gradle.properties).
     */
    val clientIdentity: ClientIdentity = ClientIdentity(version = settings.clientVersion)

    val connectionMonitor: ConnectionMonitor by lazy { ConnectionMonitor(appContext) }

    val database: AppDatabase by lazy { AppDatabase.open(appContext) }

    /** Avisos entre quien pide enviar la cola y el trabajador que ya está enviando. */
    private val orderSendSignals = OrderSendSignals()

    /** Envío de la cola con WorkManager (sin restricción de red; ver la clase). */
    val orderScheduler: WorkManagerOrderScheduler by lazy {
        WorkManagerOrderScheduler(appContext, orderSendSignals)
    }

    /**
     * La cola de pedidos del modo Mesero (Fase E, Room): también el contador del indicador, el
     * trabajo pendiente de cada mesero y lo que se muestra antes de borrar al retirar el
     * dispositivo.
     */
    val orderQueue: OrderQueueRepository by lazy {
        OrderQueueRepository(
            dao = database.waiter(),
            scheduler = orderScheduler,
            describe = { line ->
                catalogRealtime.catalog.catalog.value.product(line.productId)?.name
            },
        )
    }

    /** Contador de pedidos por enviar de todo el dispositivo (indicador de conexión). */
    val pendingOrders: PendingOrdersSource
        get() = orderQueue

    /** Trabajo pendiente de cada mesero: capturas en curso y pedidos sin enviar (A1-D15). */
    val waiterWork: WaiterWorkGate = WaiterWorkGate { userId -> orderQueue.workOf(userId) }

    /** Pedidos sin enviar que pasan a la bitácora al retirar el dispositivo. */
    val unsentOrders: UnsentOrdersSource = UnsentOrdersSource { orderQueue.unsentOrders() }

    val devicePreferences: DevicePreferencesRepository by lazy {
        DevicePreferencesRepository(database.devicePreferences())
    }

    /** Llave del Android Keystore para las credenciales y el verificador del PIN. */
    private val keystoreSealer: KeystoreSecretSealer by lazy {
        KeystoreSecretSealer.create(appContext)
    }

    /** Cifrado de credenciales y del verificador del PIN con el Android Keystore (fases B y C). */
    val secretSealer: SecretSealer
        get() = keystoreSealer

    /**
     * Bloqueo por inactividad, **uno por proceso** (revisión R3, A2 y B4): `MainActivity` y los
     * diálogos le avisan de cada toque y [startIdleLock] lo vigila mientras viva el proceso.
     */
    val idleLock: IdleLock by lazy { IdleLock(SystemClock::elapsedRealtime, waiterWork) }

    /**
     * Registro único de «borrar los datos de este dispositivo» (retirarlo o emparejarlo de nuevo;
     * revisión R3, M4). Cada módulo que guarda datos del restaurante se registra aquí: hoy el
     * catálogo con sus imágenes. **La Fase E registra aquí** la cola de pedidos y sus borradores, y
     * cancela su trabajo de WorkManager (`WorkManager.cancelAllWork()`).
     */
    val deviceDataWipe: DeviceDataWipe =
        DeviceDataWipe().apply {
            register("catálogo e imágenes") { catalogRealtime.eraseDeviceData() }
            // Fase E: la cola, las capturas y lo guardado de Go (UnsentOrdersSource ya entregó lo
            // no
            // enviado a la bitácora) y ningún envío queda programado.
            register("cola de pedidos y capturas") {
                orderQueue.eraseAll()
                orderScheduler.cancelAll()
            }
        }

    private val serverBrowser: NsdServerBrowser by lazy {
        NsdServerBrowser(appContext, serviceType = { settings.discoveryServiceType })
    }

    /**
     * Entrada al dispositivo (servidor emparejado, meseros, credenciales). Otros módulos piden aquí
     * la API del servidor (`serverApi()`) y las cabeceras de autenticación de un mesero concreto,
     * el dueño de la operación (`requestHeaders(userId)`), y avisan los errores de Go con
     * `report(error, userId)`.
     */
    val deviceAccess: AccessRepository by lazy {
        AccessRepository(
            dao = database.access(),
            sealer = keystoreSealer,
            destroyKey = keystoreSealer::destroyKey,
            identity = clientIdentity,
            relocator =
                ServerRelocator(
                    browser = serverBrowser,
                    probe = HttpsServerProbe(clientIdentity),
                    clock = SystemClock::elapsedRealtime,
                ),
            isOnline = { connectionMonitor.current() == LinkState.ONLINE },
            unsentOrders = unsentOrders,
            scope = appScope,
            wipe = deviceDataWipe,
        )
    }

    private val pairing: PairingRepository by lazy {
        PairingRepository(
            browse = serverBrowser::browse,
            identity = clientIdentity,
            osLabel = "Android ${Build.VERSION.RELEASE}",
            sealer = keystoreSealer,
            access = deviceAccess,
        )
    }

    /**
     * Conexión con el servidor a nombre del mesero activo para el catálogo y el canal de tiempo
     * real (interfaz estrecha de la Fase D sobre [deviceAccess]); sus avisos van a
     * `deviceAccess.report`.
     */
    private val serverAccess: DeviceServerAccess by lazy {
        DeviceServerAccess(deviceAccess, appScope)
    }

    /** Primer o segundo plano de toda la aplicación (lo observa AdelPosApplication). */
    val appForeground: AppForeground = AppForeground()

    /**
     * Catálogo en el dispositivo, imágenes y **el único** canal de tiempo real del proceso (Fase
     * D). La Fase E se suscribe a `catalogRealtime.realtime`.
     */
    val catalogRealtime: CatalogRealtimeContainer by lazy {
        CatalogRealtimeContainer(
            context = appContext,
            access = serverAccess,
            interruptions = serverAccess.interruptions,
            linkState = connectionMonitor.linkState,
            foreground = appForeground.inForeground,
        )
    }

    private var idleLockStarted = false

    /**
     * Vigila la inactividad del mesero activo durante toda la vida del proceso (lo llama
     * AdelPosApplication al arrancar): el plazo cuenta desde el último toque, no se reinicia al
     * volver a crear la actividad y, al volver a primer plano, bloquea en ese momento si ya venció.
     * Bloquear es volver a la lista de meseros ([AccessRepository.leave]) sin cerrar la sesión en
     * Go.
     */
    fun startIdleLock() {
        if (idleLockStarted) {
            return
        }
        idleLockStarted = true
        appScope.launch {
            idleLock.supervise(
                waiters = deviceAccess.activeWaiter.map { it?.userId },
                timeouts =
                    deviceAccess.status.map {
                        (it as? DeviceStatus.Ready)?.idleLockSeconds ?: IdleLock.DEFAULT_SECONDS
                    },
                foreground = appForeground.inForeground,
            ) { userId ->
                // Solo si sigue siendo el mismo mesero (pudo cambiar mientras se decidía).
                if (deviceAccess.activeWaiter.value?.userId == userId) {
                    deviceAccess.leave()
                }
            }
        }
    }

    /** Conexión del modo Mesero con las cabeceras del mesero dueño de cada operación (M5). */
    private val waiterServer: DeviceWaiterServer by lazy { DeviceWaiterServer(deviceAccess) }

    private val waiterApi: WaiterApi by lazy { WaiterApi(lostResponses = settings.lostResponses) }

    /** Modo Mesero (Fase E): mesas, capturas, cola y modificar o cancelar lo enviado. */
    val waiterMode: WaiterModeContainer by lazy {
        WaiterModeContainer(
            dao = database.waiter(),
            server = waiterServer,
            api = waiterApi,
            queue = orderQueue,
            catalog = catalogRealtime.catalog,
            realtime = catalogRealtime.realtime,
            linkState = connectionMonitor.linkState,
            foreground = appForeground.inForeground,
        )
    }

    /** Fábrica del trabajador de la cola para la configuración de WorkManager. */
    val orderWorkerFactory: OrderWorkerFactory by lazy {
        OrderWorkerFactory(sender = { waiterMode.sender }, signals = orderSendSignals)
    }

    /** Lo que necesitan las pantallas de entrada (`feature:access`). */
    val accessDependencies: AccessDependencies by lazy {
        AccessDependencies(
            access = deviceAccess,
            pairing = pairing,
            work = waiterWork,
            idleLock = idleLock,
            linkState = connectionMonitor.linkState,
        )
    }
}
