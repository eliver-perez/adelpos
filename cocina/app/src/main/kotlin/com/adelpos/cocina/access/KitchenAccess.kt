package com.adelpos.cocina.access

import com.adelpos.cocina.db.PairedServerDao
import com.adelpos.cocina.db.PairedServerEntity
import com.adelpos.core.model.access.RepairReason
import com.adelpos.core.model.connection.ServerAnswers
import com.adelpos.core.model.connection.ServerLink
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.LicenseMessages
import com.adelpos.core.model.security.SecretSealer
import com.adelpos.core.model.security.SecretUnavailableException
import com.adelpos.core.model.server.ServerCompatibility
import com.adelpos.core.model.server.ServerRelocator
import com.adelpos.core.model.text.Hex
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.DeviceCredentials
import com.adelpos.core.network.api.IgnoredBody
import com.adelpos.core.network.api.ServerApi
import com.adelpos.core.network.pairing.PairedConnection
import com.adelpos.core.network.pairing.PairedDevice
import com.adelpos.core.network.pairing.PairingStore
import com.adelpos.core.network.session.ServerTransport
import com.adelpos.core.network.wipe.DeviceDataWipe
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * La pantalla de cocina frente a la computadora principal (etapa K1): el servidor emparejado en su
 * base propia, la credencial del dispositivo cifrada con el Android Keystore y la conexión con la
 * huella fijada. Es la única fuente del estado de la pantalla ([status]). Usa las piezas
 * compartidas de android-core (emparejamiento, conexión con la búsqueda en otra dirección, estado
 * único de conexión); no tiene meseros, PIN ni cola.
 *
 * @param destroyKey borra la llave del Keystore al olvidar el servidor: todo lo cifrado queda
 *   ilegible aunque algo sobreviviera al borrado.
 * @param link el estado único de conexión (etapa A3): con «Sin conexión» no se pregunta a Go salvo
 *   que la sonda del momento diga que ya responde.
 * @param answers a quién informa el cliente del servidor emparejado cada respuesta de Go (el mismo
 *   estado único; null en pruebas que no lo miran).
 * @param wipe registro único de «borrar los datos de este dispositivo»: se usa al olvidar el
 *   servidor (revocada o «Emparejar de nuevo»).
 */
class KitchenAccess(
    private val dao: PairedServerDao,
    private val sealer: SecretSealer,
    private val destroyKey: () -> Unit,
    private val identity: ClientIdentity,
    private val relocator: ServerRelocator,
    private val link: ServerLink,
    private val scope: CoroutineScope,
    private val wipe: DeviceDataWipe = DeviceDataWipe(),
    private val answers: ServerAnswers? = null,
) : PairingStore {
    private val mutableStatus = MutableStateFlow<KitchenStatus>(KitchenStatus.Starting)

    val status: StateFlow<KitchenStatus> = mutableStatus.asStateFlow()

    private val mutableDisplay = MutableStateFlow<DisplayConnection>(DisplayConnection.Connecting)

    /** Si la pantalla ya se conectó como pantalla de cocina (`kitchen-display/connect`). */
    val display: StateFlow<DisplayConnection> = mutableDisplay.asStateFlow()

    // Un cambio de estado a la vez (comprobar, emparejar, olvidar).
    private val mutex = Mutex()

    // Se lee sin el candado en la sonda del estado de conexión (probeServer).
    @Volatile private var connection: PairedConnection? = null

    /** Lee lo guardado y comprueba el servidor (al abrir la aplicación). */
    suspend fun start() {
        mutex.withLock {
            loadLocked()
            checkLocked()
        }
    }

    /** Vuelve a comprobar el servidor (al volver la conexión, «Volver a comprobar»). */
    suspend fun recheck() {
        mutex.withLock { checkLocked() }
    }

    /** Lo que deja el emparejamiento compartido (`PairingRepository`). */
    override suspend fun onPaired(device: PairedDevice) {
        mutex.withLock {
            dao.save(
                PairedServerEntity(
                    serverId = device.serverId,
                    serverName = device.serverName,
                    baseUrl = device.baseUrl,
                    fingerprintHex = device.fingerprintHex,
                    deviceId = device.deviceId,
                    deviceName = device.deviceName,
                    sealedDeviceToken = device.sealedDeviceToken,
                    pairedAtMillis = device.pairedAtMillis,
                ),
            )
            loadLocked()
            checkLocked()
        }
    }

    /** «Emparejar de nuevo»: borra todo lo de este dispositivo y vuelve a «Buscar el servidor». */
    suspend fun forgetServer() {
        mutex.withLock {
            forgetLocked()
            mutableStatus.value = KitchenStatus.Unpaired
        }
    }

    /**
     * Punto único para los errores de Go de cualquier petición o del canal de eventos: revocada
     * (`DEVICE_REVOKED`) vuelve a «Buscar el servidor» con todo borrado (§2 de la etapa K1);
     * `DEVICE_*` u otra huella piden emparejar de nuevo; 426 pide actualizar la aplicación. Ninguno
     * dice «sin conexión»: eso lo decide el estado único de conexión.
     */
    fun report(error: AppError) {
        scope.launch { mutex.withLock { applyErrorLocked(error) } }
    }

    /**
     * La sonda del estado único de conexión (`GET health/live` al servidor emparejado; su respuesta
     * llega al estado por el cliente). No toma el candado. false si no hay servidor emparejado al
     * que preguntar. Si la sonda que busca el servidor ([relocate]) encuentra otro certificado en
     * su dirección y tampoco lo halla en otra, lo informa como cualquier otra petición: la pantalla
     * pide emparejar de nuevo (como en la aplicación de meseros, A3).
     */
    suspend fun probeServer(relocate: Boolean, quick: Boolean): Boolean {
        val current = connection ?: return false
        val outcome = current.live(relocate, quick)
        if (
            relocate &&
                outcome is ApiOutcome.NoAnswer &&
                outcome.error == AppError.ServerIdentityChanged &&
                connection === current
        ) {
            report(AppError.ServerIdentityChanged)
        }
        return true
    }

    /**
     * La API del servidor emparejado en la dirección vigente con la huella fijada, o null sin
     * servidor emparejado. Se pide en cada uso (la dirección cambia con el cambio de IP).
     */
    fun serverApi(): ServerApi? = connection?.api()

    /** Credencial del dispositivo emparejado (`X-Device-ID` y `X-Device-Token`). */
    fun deviceCredentials(): DeviceCredentials? = connection?.credentials

    /** Dirección vigente y cliente con la huella fijada, para el canal de eventos. */
    fun serverTransport(): ServerTransport? = connection?.transport()

    private suspend fun loadLocked() {
        val entity = dao.pairedServer()
        mutableDisplay.value = DisplayConnection.Connecting
        if (entity == null) {
            connection = null
            mutableStatus.value = KitchenStatus.Unpaired
            return
        }
        val token =
            try {
                String(
                    sealer.open(entity.sealedDeviceToken, PairedDevice.CREDENTIAL_PURPOSE),
                    Charsets.UTF_8,
                )
            } catch (_: SecretUnavailableException) {
                connection = null
                mutableStatus.value = KitchenStatus.NeedsRepair(RepairReason.CREDENTIAL_LOST)
                return
            }
        connection =
            PairedConnection(
                serverId = entity.serverId,
                fingerprint = Hex.decode(entity.fingerprintHex),
                initialBase = entity.baseUrl,
                credentials = DeviceCredentials(entity.deviceId, token),
                identity = identity,
                relocator = relocator,
                onMoved = { base -> dao.moveTo(base) },
                answers = answers,
            )
        mutableStatus.value = KitchenStatus.Ready(entity.serverName, entity.deviceName)
    }

    // Comprueba la versión del servidor. Solo desde un estado que tenga servidor.
    private suspend fun checkLocked() {
        val current = connection ?: return
        val status = mutableStatus.value
        if (status is KitchenStatus.Unpaired) {
            return
        }
        if (status is KitchenStatus.NeedsRepair && status.reason != RepairReason.IDENTITY_CHANGED) {
            return
        }
        // Con «Sin conexión» solo se pregunta si la sonda del momento dice que Go ya responde.
        if (!link.confirm()) {
            return
        }
        when (val version = current.version()) {
            is ApiOutcome.Success -> {
                val info = version.value
                val verdict =
                    ServerCompatibility.check(
                        appVersion = identity.version,
                        serverVersion = info.serverVersion,
                        capabilities = info.capabilities,
                        minClientVersion = info.minClientVersions[identity.kind],
                        requirements = KitchenServerRequirements.REQUIREMENTS,
                    )
                mutableStatus.value =
                    when (verdict) {
                        is ServerCompatibility.Verdict.UpdateServer ->
                            KitchenStatus.UpdateServer(verdict.serverVersion)
                        is ServerCompatibility.Verdict.UpdateApp ->
                            KitchenStatus.UpdateApp(identity.version, verdict.minClientVersion)
                        ServerCompatibility.Verdict.Compatible -> readyOrStatus(status)
                    }
                if (verdict == ServerCompatibility.Verdict.Compatible) {
                    connectLocked(current)
                }
            }
            is ApiOutcome.Rejected -> applyErrorLocked(ApiErrors.toAppError(version.problem))
            is ApiOutcome.NoAnswer -> applyErrorLocked(version.error)
            // Una respuesta que esta versión no entiende: nada cambia.
            is ApiOutcome.Malformed -> Unit
        }
    }

    /**
     * Se conecta como pantalla de cocina (§4 del orden de arranque de la Fase A): `POST
     * kitchen-display/connect` con `{}` ocupa su cupo de cocina si no lo tiene y es idempotente.
     * Sin respuesta, nada cambia (se vuelve a intentar al comprobar); un rechazo de acceso va a
     * [applyErrorLocked]; uno de la licencia, el cupo o la hora se muestra con su texto.
     */
    private suspend fun connectLocked(current: PairedConnection) {
        val outcome = current.call { api ->
            api.post(
                CONNECT_PATH,
                ConnectRequest(),
                ConnectRequest.serializer(),
                IgnoredBody.serializer(),
                current.credentials.headers(),
            )
        }
        when (outcome) {
            is ApiOutcome.Success -> mutableDisplay.value = DisplayConnection.Connected
            is ApiOutcome.Rejected -> {
                val error = ApiErrors.toAppError(outcome.problem)
                if (error is AppError.LicenseRefused) {
                    mutableDisplay.value =
                        DisplayConnection.Refused(LicenseMessages.of(error.refusal))
                } else {
                    applyErrorLocked(error)
                }
            }
            is ApiOutcome.NoAnswer -> applyErrorLocked(outcome.error)
            is ApiOutcome.Malformed -> Unit
        }
    }

    // Con el servidor compatible: de vuelta a la pantalla de pedidos (por ejemplo, después de
    // «Actualiza el servidor» y «Volver a comprobar»).
    private suspend fun readyOrStatus(status: KitchenStatus): KitchenStatus {
        if (status is KitchenStatus.Ready) {
            return status
        }
        val entity = dao.pairedServer() ?: return KitchenStatus.Unpaired
        return KitchenStatus.Ready(entity.serverName, entity.deviceName)
    }

    private suspend fun applyErrorLocked(error: AppError) {
        when (error) {
            is AppError.DeviceRevoked -> {
                // §2: revocar la pantalla en Terminales la regresa a la pantalla de emparejamiento.
                if (mutableStatus.value !is KitchenStatus.Unpaired) {
                    forgetLocked()
                    mutableStatus.value = KitchenStatus.Unpaired
                }
            }
            is AppError.DeviceNotRecognized ->
                mutableStatus.value = KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED)
            AppError.ServerIdentityChanged ->
                mutableStatus.value = KitchenStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED)
            // Un servidor que no conoce el tipo de cliente `kitchen` (anterior a la etapa K1): no
            // es
            // esta aplicación la que debe actualizarse.
            is AppError.UpdateRequired ->
                mutableStatus.value =
                    if (error.code == CLIENT_KIND_INVALID) {
                        KitchenStatus.UpdateServer(error.serverVersion)
                    } else {
                        KitchenStatus.UpdateApp(identity.version, error.minClientVersion)
                    }
            // El dispositivo emparejado no es una pantalla de cocina para Go: emparejarla de nuevo.
            is AppError.Rejected ->
                if (error.code == CLIENT_KIND_MISMATCH) {
                    mutableStatus.value = KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED)
                }
            // Una falta de respuesta u otro error no cambian el estado de la pantalla.
            else -> Unit
        }
    }

    // Olvidar el servidor, en este orden: se borran los datos de este dispositivo del registro, se
    // olvida el emparejamiento y se destruye la llave del Keystore.
    private suspend fun forgetLocked() {
        wipe.eraseAll()
        dao.forget()
        connection = null
        mutableDisplay.value = DisplayConnection.Connecting
        try {
            destroyKey()
        } catch (_: SecretUnavailableException) {
            // Lo cifrado ya no tiene servidor que lo use; la llave se reemplaza al emparejar.
        }
    }

    /**
     * Cuerpo de `kitchen-display/connect`: `{}` al arrancar o reconectar. `take_seat` (solo con
     * «Usar esta pantalla», después de «Liberar cupo») lo usa la Fase E.
     */
    @Serializable
    private class ConnectRequest(@SerialName("take_seat") val takeSeat: Boolean? = null)

    private companion object {
        const val CONNECT_PATH = "kitchen-display/connect"
        const val CLIENT_KIND_INVALID = "CLIENT_KIND_INVALID"
        const val CLIENT_KIND_MISMATCH = "CLIENT_KIND_MISMATCH"
    }
}
