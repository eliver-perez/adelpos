package local.adel.pos.android

import android.content.Context
import android.os.Build
import android.os.SystemClock
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.data.access.PairingRepository
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.discovery.HttpsServerProbe
import local.adel.pos.android.core.data.discovery.NsdServerBrowser
import local.adel.pos.android.core.data.network.ConnectionMonitor
import local.adel.pos.android.core.data.pending.NoPendingOrders
import local.adel.pos.android.core.data.preferences.DevicePreferencesRepository
import local.adel.pos.android.core.data.security.KeystoreSecretSealer
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.access.NoUnsentOrders
import local.adel.pos.android.core.domain.access.NoWaiterWork
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
     */
    data class Settings(
        val clientVersion: String = BuildConfig.VERSION_NAME,
        val discoveryServiceType: String = ServerAnnouncements.SERVICE_TYPE,
    )

    private val appContext: Context = context.applicationContext

    /** Trabajo de fondo que sobrevive a las pantallas (comprobaciones, revocación). */
    private val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    /**
     * Identificación ante el servidor: la versión sale de versionName (android/gradle.properties).
     */
    val clientIdentity: ClientIdentity = ClientIdentity(version = settings.clientVersion)

    val connectionMonitor: ConnectionMonitor by lazy { ConnectionMonitor(appContext) }

    /** Contador de pedidos por enviar; la cola real llega en la Fase E. */
    val pendingOrders: PendingOrdersSource = NoPendingOrders

    /** Trabajo pendiente de cada mesero (captura y cola de la Fase E); hoy nadie tiene. */
    val waiterWork: WaiterWorkGate = NoWaiterWork

    /** Pedidos sin enviar que pasan a la bitácora al retirar el dispositivo (Fase E). */
    val unsentOrders: UnsentOrdersSource = NoUnsentOrders

    val database: AppDatabase by lazy { AppDatabase.open(appContext) }

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

    /** Bloqueo por inactividad: `MainActivity` le avisa de cada toque de la pantalla. */
    val idleLock: IdleLock by lazy { IdleLock(SystemClock::elapsedRealtime, waiterWork) }

    private val serverBrowser: NsdServerBrowser by lazy {
        NsdServerBrowser(appContext, serviceType = { settings.discoveryServiceType })
    }

    /**
     * Entrada al dispositivo (servidor emparejado, meseros, credenciales). Otros módulos piden aquí
     * la API del servidor (`serverApi()`) y las cabeceras de autenticación (`requestHeaders()`) y
     * avisan los errores de Go con `report()`.
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
            work = waiterWork,
            unsentOrders = unsentOrders,
            scope = appScope,
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
