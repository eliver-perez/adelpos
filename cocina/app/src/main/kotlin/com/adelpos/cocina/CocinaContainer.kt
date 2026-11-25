package com.adelpos.cocina

import android.content.Context
import android.os.Build
import android.os.SystemClock
import android.util.Log
import com.adelpos.cocina.access.KitchenAccess
import com.adelpos.cocina.access.KitchenServerRequirements
import com.adelpos.cocina.db.KitchenDatabase
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.pairing.PairingFlow
import com.adelpos.core.model.server.ServerAnnouncements
import com.adelpos.core.model.server.ServerRelocator
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.ClientKinds
import com.adelpos.core.network.connection.AppForeground
import com.adelpos.core.network.connection.ConnectionMonitor
import com.adelpos.core.network.connection.ServerLinkMonitor
import com.adelpos.core.network.discovery.HttpsServerProbe
import com.adelpos.core.network.discovery.NsdServerBrowser
import com.adelpos.core.network.pairing.PairingRepository
import com.adelpos.core.network.security.KeystoreSecretSealer
import com.adelpos.core.network.wipe.DeviceDataWipe
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.flowOf

/**
 * Dependencias de la aplicación de cocina (inyección manual, como la de meseros): una instancia de
 * cada cosa por proceso, creada al primer uso. Los ViewModel las reciben desde sus fábricas.
 */
class CocinaContainer(context: Context, private val settings: Settings = Settings()) {
    /**
     * Ajustes que solo cambian las pruebas instrumentadas (`CocinaTestRunner`), antes del primer
     * uso.
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
     * Identificación ante el servidor: la versión sale de versionName (cocina/gradle.properties) y
     * el tipo de cliente es `kitchen` (K1-D3), fijo para esta aplicación.
     */
    val clientIdentity: ClientIdentity =
        ClientIdentity(version = settings.clientVersion, kind = ClientKinds.KITCHEN)

    /** Si la aplicación está a la vista (lo registra [CocinaApplication]). */
    val appForeground: AppForeground = AppForeground()

    /** La red de Android (solo alimenta a [serverLink]). */
    private val connectionMonitor: ConnectionMonitor by lazy { ConnectionMonitor(appContext) }

    /**
     * **El estado único de conexión** con la computadora principal (etapa A3, el mismo de la
     * aplicación de meseros): lo leen la barra y la pantalla de pedidos; lo alimentan la red de
     * Android y cada respuesta de Go del cliente del servidor emparejado. Lo arranca
     * [CocinaApplication]. El canal de eventos llega en las fases siguientes: hasta entonces la
     * revisión es la de «sin canal».
     */
    val serverLink: ServerLinkMonitor by lazy {
        ServerLinkMonitor(
            network = connectionMonitor.reportedLinkState,
            initialNetwork = connectionMonitor.current().takeIf { it == LinkState.ONLINE },
            foreground = appForeground.inForeground,
            channelOpen = flowOf(false),
            probing = { relocate, quick -> access.probeServer(relocate, quick) },
            scope = appScope,
            clock = SystemClock::elapsedRealtime,
            log = { message -> Log.i(SERVER_LINK_TAG, message) },
        )
    }

    private val database: KitchenDatabase by lazy { KitchenDatabase.open(appContext) }

    /** Llave del Android Keystore para la credencial del dispositivo. */
    private val keystoreSealer: KeystoreSecretSealer by lazy {
        KeystoreSecretSealer.create(appContext)
    }

    /**
     * Registro único de «borrar los datos de este dispositivo» (revocada o «Emparejar de nuevo»).
     * Cada parte que guarde datos del restaurante se registra aquí.
     */
    val deviceDataWipe: DeviceDataWipe = DeviceDataWipe()

    private val serverBrowser: NsdServerBrowser by lazy {
        NsdServerBrowser(appContext, serviceType = { settings.discoveryServiceType })
    }

    /** La pantalla frente a la computadora principal: el estado que decide qué se ve. */
    val access: KitchenAccess by lazy {
        KitchenAccess(
            dao = database.pairedServer(),
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
            scope = appScope,
            wipe = deviceDataWipe,
            answers = serverLink,
        )
    }

    /** Emparejamiento como pantalla de cocina (`device_kind: kitchen`, K1-D3). */
    val pairing: PairingFlow by lazy {
        PairingRepository(
            browse = serverBrowser::browse,
            identity = clientIdentity,
            osLabel = "Android ${Build.VERSION.RELEASE}",
            sealer = keystoreSealer,
            store = access,
            requirements = KitchenServerRequirements.REQUIREMENTS,
        )
    }

    private companion object {
        const val SERVER_LINK_TAG = "CocinaLink"
    }
}
