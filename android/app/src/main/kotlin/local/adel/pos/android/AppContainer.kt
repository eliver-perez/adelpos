package local.adel.pos.android

import android.content.Context
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.network.ConnectionMonitor
import local.adel.pos.android.core.data.pending.NoPendingOrders
import local.adel.pos.android.core.data.preferences.DevicePreferencesRepository
import local.adel.pos.android.core.data.security.KeystoreSecretSealer
import local.adel.pos.android.core.data.session.FixedServerAccess
import local.adel.pos.android.core.data.session.ServerAccess
import local.adel.pos.android.core.domain.connection.PendingOrdersSource
import local.adel.pos.android.core.domain.realtime.AccessInterruptions
import local.adel.pos.android.core.domain.security.SecretSealer

/**
 * Dependencias compartidas de la aplicación (inyección manual): una instancia de cada cosa por
 * proceso, creada al primer uso. Los ViewModel las reciben desde sus fábricas (`initializer {}`);
 * nada se busca por su cuenta.
 */
class AppContainer(context: Context) {
    private val appContext: Context = context.applicationContext

    /**
     * Identificación ante el servidor: la versión sale de versionName (android/gradle.properties).
     */
    val clientIdentity: ClientIdentity = ClientIdentity(version = BuildConfig.VERSION_NAME)

    val connectionMonitor: ConnectionMonitor by lazy { ConnectionMonitor(appContext) }

    /** Contador de pedidos por enviar; la cola real llega en la Fase E. */
    val pendingOrders: PendingOrdersSource = NoPendingOrders

    val database: AppDatabase by lazy { AppDatabase.open(appContext) }

    val devicePreferences: DevicePreferencesRepository by lazy {
        DevicePreferencesRepository(database.devicePreferences())
    }

    /** Cifrado de credenciales y del verificador del PIN con el Android Keystore (fases B y C). */
    val secretSealer: SecretSealer by lazy { KeystoreSecretSealer.create(appContext) }

    /**
     * Conexión con el servidor a nombre del mesero actual (interfaz estrecha de la Fase D). La
     * llena el acceso de las fases B y C (emparejamiento y PIN); hasta conectarlo, sin sesión.
     */
    val serverAccess: ServerAccess = FixedServerAccess()

    /**
     * Punto único de los avisos de dispositivo retirado, sesión terminada, permiso perdido y
     * versión vieja (del canal de tiempo real y del catálogo). Lo conecta el acceso (fases B y C).
     */
    val accessInterruptions: AccessInterruptions = AccessInterruptions {}

    /** Primer o segundo plano de toda la aplicación (lo observa AdelPosApplication). */
    val appForeground: AppForeground = AppForeground()

    /** Catálogo en el dispositivo, imágenes y el canal de tiempo real (Fase D). */
    val catalogRealtime: CatalogRealtimeContainer by lazy {
        CatalogRealtimeContainer(
            context = appContext,
            access = serverAccess,
            interruptions = accessInterruptions,
            linkState = connectionMonitor.linkState,
            foreground = appForeground.inForeground,
        )
    }
}
