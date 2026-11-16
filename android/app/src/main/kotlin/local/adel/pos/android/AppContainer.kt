package local.adel.pos.android

import android.content.Context
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.network.ConnectionMonitor
import local.adel.pos.android.core.data.pending.NoPendingOrders
import local.adel.pos.android.core.data.preferences.DevicePreferencesRepository
import local.adel.pos.android.core.data.security.KeystoreSecretSealer
import local.adel.pos.android.core.domain.connection.PendingOrdersSource
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
}
