package com.adelpos.meseros

import android.content.Context
import android.os.Build
import android.os.SystemClock
import android.util.Log
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.data.access.AccessRepository
import com.adelpos.meseros.core.data.access.PairingRepository
import com.adelpos.meseros.core.data.api.ClientIdentity
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.discovery.HttpsServerProbe
import com.adelpos.meseros.core.data.discovery.NsdServerBrowser
import com.adelpos.meseros.core.data.network.ConnectionMonitor
import com.adelpos.meseros.core.data.network.ServerLinkMonitor
import com.adelpos.meseros.core.data.preferences.DevicePreferencesRepository
import com.adelpos.meseros.core.data.security.KeystoreSecretSealer
import com.adelpos.meseros.core.data.session.DeviceServerAccess
import com.adelpos.meseros.core.data.waiter.CaptureDiscards
import com.adelpos.meseros.core.data.waiter.DeviceWaiterServer
import com.adelpos.meseros.core.data.waiter.LostResponses
import com.adelpos.meseros.core.data.waiter.OrderQueueRepository
import com.adelpos.meseros.core.data.waiter.OrderSendSignals
import com.adelpos.meseros.core.data.waiter.OrderWorkerFactory
import com.adelpos.meseros.core.data.waiter.OrdersInFlight
import com.adelpos.meseros.core.data.waiter.ServerClock
import com.adelpos.meseros.core.data.waiter.WaiterApi
import com.adelpos.meseros.core.data.waiter.WorkManagerOrderScheduler
import com.adelpos.meseros.core.data.wipe.DeviceDataWipe
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.IdleLock
import com.adelpos.meseros.core.domain.access.UnsentOrdersSource
import com.adelpos.meseros.core.domain.access.WaiterWorkGate
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.connection.PendingOrdersSource
import com.adelpos.meseros.core.domain.realtime.RealtimeState
import com.adelpos.meseros.core.domain.security.SecretSealer
import com.adelpos.meseros.core.domain.server.ServerAnnouncements
import com.adelpos.meseros.core.domain.server.ServerRelocator
import com.adelpos.meseros.feature.access.AccessDependencies
import com.adelpos.meseros.feature.takeaway.TakeawayDependencies
import com.adelpos.meseros.feature.waiter.WaiterDependencies
import com.adelpos.meseros.ready.ReadyAlertsController
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

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
     * @param readyAlerts el aviso de platillos listos (etapa A2, Fase D); las pruebas de la A1 lo
     *   apagan para que su servicio y sus notificaciones no las cambien.
     * @param askAlertPermissions explicar y pedir los permisos del aviso al arrancar su servicio;
     *   las pruebas los conceden ellas.
     */
    data class Settings(
        val clientVersion: String = BuildConfig.VERSION_NAME,
        val discoveryServiceType: String = ServerAnnouncements.SERVICE_TYPE,
        val lostResponses: LostResponses? = null,
        val readyAlerts: Boolean = true,
        val askAlertPermissions: Boolean = true,
    )

    private val appContext: Context = context.applicationContext

    /** Trabajo de fondo que sobrevive a las pantallas (comprobaciones, revocación). */
    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /**
     * Identificación ante el servidor: la versión sale de versionName (android/gradle.properties).
     */
    val clientIdentity: ClientIdentity = ClientIdentity(version = settings.clientVersion)

    /** La red de Android (solo alimenta a [serverLink] y a los disparadores de «volvió la red»). */
    val connectionMonitor: ConnectionMonitor by lazy { ConnectionMonitor(appContext) }

    /**
     * **El estado único de conexión** con la computadora principal (etapa A3, A3-D1): lo leen la
     * barra, «¿Quién eres?», Mesas, Para llevar, la precuenta, «Entregado» y la notificación del
     * canal de avisos; lo alimentan la red de Android y cada respuesta de Go del cliente del
     * servidor emparejado. Lo arranca AdelPosApplication.
     */
    val serverLink: ServerLinkMonitor by lazy {
        ServerLinkMonitor(
            network = connectionMonitor.linkState,
            initialNetwork = connectionMonitor.current(),
            foreground = appForeground.inForeground,
            // Se arma al recogerse (en start()): el canal de tiempo real necesita la entrada, que
            // necesita este estado; así no se crean en círculo.
            channelOpen =
                flow {
                    emitAll(catalogRealtime.realtime.state.map { it is RealtimeState.Connected })
                },
            probing = { relocate, quick -> deviceAccess.probeServer(relocate, quick) },
            scope = appScope,
            clock = SystemClock::elapsedRealtime,
            log = { message -> Log.i(SERVER_LINK_TAG, message) },
        )
    }

    /** El estado único como «hay conexión» para quien lo necesita al momento (la precuenta). */
    private val serverReachable: StateFlow<Boolean> by lazy {
        serverLink.state
            .map { it == LinkState.ONLINE }
            .stateIn(appScope, SharingStarted.Eagerly, serverLink.state.value == LinkState.ONLINE)
    }

    val database: AppDatabase by lazy { AppDatabase.open(appContext) }

    /** Avisos entre quien pide enviar la cola y el trabajador que ya está enviando. */
    private val orderSendSignals = OrderSendSignals()

    /**
     * La diferencia con la hora de Go, para la hora de captura de los pedidos (revisión R6, B4).
     */
    private val serverClock = ServerClock()

    /** Pedidos que este proceso confirma en línea en este momento (revisión R6, B2). */
    private val ordersInFlight = OrdersInFlight()

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
            // «Descartar el pedido» quita de Go lo suyo con la captura (revisión R6, A2).
            discards = CaptureDiscards { captureId -> waiterMode.captures.discardNow(captureId) },
            inFlight = ordersInFlight,
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
            // no enviado a la bitácora); desde la A2, también las ediciones del cliente de Para
            // llevar por enviar (WaiterDao.eraseAll). Revisión R6, M1 y B5: primero se cancela el
            // envío y se
            // espera a que termine la vuelta en curso (nada vuelve a escribir un pedido borrado) y,
            // al final, se vuelve a programar el envío periódico.
            register("cola de pedidos y capturas") {
                orderScheduler.cancelAndWait()
                waiterMode.sender.exclusive { orderQueue.eraseAll() }
                orderScheduler.schedulePeriodic()
            }
            // Etapa A2, Fase D: detiene el canal y el servicio de avisos, borra el cursor y los
            // avisos ya mostrados y quita los avisos a la vista.
            register("avisos de platillos listos") { readyAlerts.eraseDeviceData() }
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
            link = serverLink,
            unsentOrders = unsentOrders,
            scope = appScope,
            wipe = deviceDataWipe,
            answers = serverLink,
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

    private val waiterApi: WaiterApi by lazy {
        WaiterApi(lostResponses = settings.lostResponses, serverClock = serverClock)
    }

    /** Modo Mesero (Fase E): mesas, capturas, cola y modificar o cancelar lo enviado. */
    val waiterMode: WaiterModeContainer by lazy {
        WaiterModeContainer(
            dao = database.waiter(),
            server = waiterServer,
            api = waiterApi,
            queue = orderQueue,
            serverClock = serverClock,
            inFlight = ordersInFlight,
            catalog = catalogRealtime.catalog,
            realtime = catalogRealtime.realtime,
            linkState = connectionMonitor.linkState,
            serverLink = serverLink,
            serverReachable = serverReachable,
            foreground = appForeground.inForeground,
        )
    }

    /** Fábrica del trabajador de la cola para la configuración de WorkManager. */
    val orderWorkerFactory: OrderWorkerFactory by lazy {
        OrderWorkerFactory(
            sender = { waiterMode.sender },
            signals = orderSendSignals,
            customers = { waiterMode.customerEdits },
        )
    }

    /**
     * Lo que necesita la captura compartida de Mesas y Para llevar (`core:capture`): catálogo,
     * cuenta visible, «Enviar a cocina», pendientes y «Requiere tu revisión».
     */
    val captureDependencies: CaptureDependencies by lazy {
        CaptureDependencies(
            restaurant = waiterMode.restaurant,
            captures = waiterMode.captures,
            orders = orderQueue,
            sentLines = waiterMode.sentLines,
            catalog = catalogRealtime.catalog,
            images = catalogRealtime.images,
            activeWaiter = deviceAccess.activeWaiter,
            takeaway = waiterMode.takeaway,
            precheck = waiterMode.precheck,
            link = serverLink.state,
        )
    }

    /** Lo que necesitan las pantallas del modo Mesero (`feature:waiter`). */
    val waiterDependencies: WaiterDependencies by lazy {
        WaiterDependencies(
            capture = captureDependencies,
            tablesView = devicePreferences,
            elapsedClock = SystemClock::elapsedRealtime,
        )
    }

    /** Lo que necesitan las pantallas de Para llevar (`feature:takeaway`, etapa A2). */
    val takeawayDependencies: TakeawayDependencies by lazy {
        TakeawayDependencies(
            capture = captureDependencies,
            elapsedClock = SystemClock::elapsedRealtime,
        )
    }

    /**
     * El aviso de platillos listos (etapa A2, Fase D): el único canal de avisos del proceso, su
     * servicio en primer plano y sus notificaciones (lo arranca AdelPosApplication).
     */
    val readyAlerts: ReadyAlertsController by lazy {
        ReadyAlertsController(
            context = appContext,
            access = deviceAccess,
            dao = database.readyAlerts(),
            preferences = devicePreferences,
            foreground = appForeground.inForeground,
            linkState = connectionMonitor.linkState,
            serverLink = serverLink.state,
            retryOrders = { orderQueue.retryNow() },
            enabled = settings.readyAlerts,
            askPermissions = settings.askAlertPermissions,
        )
    }

    /** Lo que necesitan las pantallas de entrada (`feature:access`). */
    val accessDependencies: AccessDependencies by lazy {
        AccessDependencies(
            access = deviceAccess,
            pairing = pairing,
            work = waiterWork,
            idleLock = idleLock,
            serverLink = serverLink,
        )
    }

    private companion object {
        /** El registro de los cambios del estado único de conexión y de cada falla. */
        const val SERVER_LINK_TAG = "AdelLink"
    }
}
