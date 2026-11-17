package local.adel.pos.android.core.domain.access

import kotlinx.coroutines.flow.StateFlow
import local.adel.pos.android.core.domain.error.AppError

/**
 * Mesero en la pantalla de entrada (`auth/pin-users`): solo lo que necesita su botón.
 *
 * @param color hexadecimal de la paleta de Go (`#3591d4`) o null; la aplicación nunca lo elige.
 * @param lockedUntilEpochMillis hasta cuándo está bloqueado su PIN por intentos fallidos (Go).
 * @param requiresUnlock su PIN quedó bloqueado hasta que un administrador lo desbloquee (A1-D16:
 *   tres bloqueos seguidos en 24 horas sin una entrada correcta).
 * @param offlineEntryClosed solo en la lista sin conexión: su entrada sin conexión está cerrada
 *   (demasiados fallos o un bloqueo de Go) hasta que entre con conexión.
 */
data class WaiterProfile(
    val userId: String,
    val name: String,
    val initial: String,
    val color: String?,
    val pinVersion: Int,
    val lockedUntilEpochMillis: Long?,
    val requiresUnlock: Boolean = false,
    val offlineEntryClosed: Boolean = false,
)

/**
 * Lista de la pantalla de entrada. Con conexión, la de Go; sin ella, solo los meseros que ya
 * entraron con conexión en **este** dispositivo y conservan su verificador local (§3.2).
 */
data class WaiterRoster(val waiters: List<WaiterProfile>, val online: Boolean)

/**
 * El mesero que está usando el dispositivo.
 *
 * @param hasServerSession entró con conexión y tiene sesión de Go; si entró sin conexión no la
 *   tiene: lo que capture lo valida Go al recibirlo (cola de la Fase E con su credencial de envío).
 *   Una entrada sin conexión **nunca** usa una sesión de Go anterior (revisión R3, A1).
 * @param mustReenterPin su acceso dejó de valer (sesión terminada o revocada, PIN cambiado,
 *   credencial de envío rechazada): sigue siendo el mesero activo, con su captura y sus pendientes,
 *   pero la pantalla le pide «Vuelve a escribir tu PIN» antes de seguir (revisión R3, M3).
 */
data class ActiveWaiter(
    val profile: WaiterProfile,
    val hasServerSession: Boolean,
    val mustReenterPin: Boolean = false,
) {
    val userId: String
        get() = profile.userId
}

/** Resultado de escribir el PIN. */
sealed interface EntryResult {
    data class Entered(val waiter: ActiveWaiter) : EntryResult

    /** PIN incorrecto (con o sin conexión). */
    data object WrongPin : EntryResult

    /**
     * Go bloqueó el PIN por intentos fallidos: hasta [untilEpochMillis] o, con [requiresUnlock],
     * hasta que un administrador lo desbloquee (A1-D16). También cierra la entrada sin conexión de
     * ese mesero.
     */
    data class Locked(val untilEpochMillis: Long?, val requiresUnlock: Boolean = false) :
        EntryResult

    /**
     * La entrada sin conexión de ese mesero está cerrada (demasiados fallos o su PIN bloqueado en
     * Go): solo puede entrar con conexión.
     */
    data object OfflineClosed : EntryResult

    /** PIN correcto, pero sin el permiso `waiter_devices.use` (403 `PERMISSION_DENIED`). */
    data object NotAllowed : EntryResult

    /** Sin conexión, y ese mesero nunca entró con conexión en este dispositivo. */
    data object UnknownOffline : EntryResult

    /** Cualquier otro error con el servidor (sin respuesta y sin verificador, 426, 5xx…). */
    data class Failed(val error: AppError) : EntryResult
}

/** Por qué el dispositivo necesita emparejarse de nuevo. */
enum class RepairReason {
    /** El servidor presentó otro certificado (huella distinta): nunca se conecta. */
    IDENTITY_CHANGED,

    /** El servidor ya no reconoce la credencial del dispositivo (`DEVICE_*`). */
    NOT_RECOGNIZED,

    /** La credencial cifrada ya no se puede abrir (se perdió la llave del Keystore). */
    CREDENTIAL_LOST,
}

/** Estado del dispositivo frente al servidor del restaurante: decide qué pantalla se ve. */
sealed interface DeviceStatus {
    /** Leyendo lo guardado. */
    data object Starting : DeviceStatus

    /**
     * Sin servidor emparejado: «Buscar el servidor». [deviceLog] son los pedidos que no se enviaron
     * y siguen en la bitácora del dispositivo (después de retirarlo o de emparejarlo de nuevo).
     */
    data class Unpaired(val deviceLog: List<UnsentOrder> = emptyList()) : DeviceStatus

    /**
     * Emparejado y listo para la entrada con PIN. [online] dice si el servidor respondió en la
     * última comprobación (sin respuesta, solo entran los meseros conocidos).
     */
    data class Ready(
        val serverName: String,
        val deviceName: String,
        val idleLockSeconds: Int,
        val online: Boolean,
    ) : DeviceStatus

    /** «Esta tableta necesita emparejarse de nuevo». */
    data class NeedsRepair(val reason: RepairReason) : DeviceStatus

    /** «Actualiza esta aplicación» (426 o la mínima de Android que anuncia el servidor). */
    data class UpdateApp(val appVersion: String, val minClientVersion: String?) : DeviceStatus

    /** «Actualiza el servidor del restaurante» (anterior a 0.12.0 o sin `waiter-pin-v1`). */
    data class UpdateServer(val serverVersion: String?) : DeviceStatus

    /**
     * El dispositivo fue retirado del restaurante (`DEVICE_REVOKED`): ya se borraron sus datos y
     * credenciales; [unsentOrders] son los pedidos que no se enviaron y quedan en la bitácora.
     */
    data class Retired(val unsentOrders: List<UnsentOrder>) : DeviceStatus
}

/**
 * Entrada al dispositivo (§3.2 y §4): servidor emparejado, comprobación de versiones, meseros y
 * entrada con PIN con y sin conexión, cambio de mesero, revocación y nuevo emparejamiento. La
 * implementa `core:data` (Room, Android Keystore y la API v1); las pantallas de `feature:access`
 * solo usan esta interfaz.
 */
interface DeviceAccess {
    val status: StateFlow<DeviceStatus>

    /** El mesero que está usando el dispositivo; null en la pantalla de entrada. */
    val activeWaiter: StateFlow<ActiveWaiter?>

    /** Lee el servidor emparejado y lo comprueba (versiones, meseros, invalidaciones). */
    suspend fun start()

    /**
     * Vuelve a comprobar el servidor: «Volver a comprobar», al recuperar la red y periódicamente.
     * Con la lista de Go borra los verificadores de quien ya no aparece o cambió de PIN.
     */
    suspend fun recheck()

    /** Meseros para la pantalla de entrada (con conexión, de Go; sin ella, los conocidos). */
    suspend fun roster(): WaiterRoster

    /** Entra con PIN: con conexión en Go; sin ella, con el verificador local. */
    suspend fun enter(userId: String, pin: String): EntryResult

    /**
     * Vuelve a la pantalla de entrada (cambio rápido de mesero o bloqueo por inactividad). La
     * sesión del mesero en Go sigue vigente: cada mesero tiene la suya en este dispositivo.
     */
    fun leave()

    /**
     * Pedidos que no han llegado al servidor (la cola de la Fase E), para mostrarlos **antes** de
     * confirmar «Emparejar de nuevo»: al confirmarlo pasan a la bitácora y se borran de la cola.
     */
    suspend fun pendingOrders(): List<UnsentOrder>

    /**
     * Deja de usar el servidor emparejado para emparejar de nuevo: los pedidos sin enviar pasan a
     * la bitácora y se borran **todos los datos de este dispositivo** (registro de borrado: acceso,
     * catálogo e imágenes y, en la Fase E, la cola).
     */
    suspend fun forgetServer()

    /** Borra la bitácora del dispositivo (los pedidos que no se enviaron al retirarlo). */
    suspend fun clearDeviceLog()

    /** El mesero leyó el aviso de dispositivo retirado: sigue «Buscar el servidor». */
    fun acknowledgeRetirement()

    /**
     * Otra parte de la aplicación (la cola, el catálogo o el canal de eventos) recibió un error de
     * Go para el mesero [userId]: `DEVICE_REVOKED` retira el dispositivo; `SESSION_*` y
     * `OFFLINE_CREDENTIAL_INVALID` dejan a ese mesero activo pero le piden «Vuelve a escribir tu
     * PIN» (nunca lo sacan ni pierden su trabajo); `DEVICE_*` y una huella distinta piden emparejar
     * de nuevo.
     */
    fun report(error: AppError, userId: String?)
}

/**
 * Qué verificadores locales dejan de valer con la lista de Go (§3.2): los de meseros que ya no
 * aparecen (desactivados, sin PIN o sin `waiter_devices.use`) y los guardados con otra
 * `pin_version` (PIN cambiado o quitado).
 */
object KnownWaiterInvalidation {
    data class Known(val userId: String, val pinVersion: Int)

    fun stale(known: List<Known>, listed: List<WaiterProfile>): Set<String> {
        val current = listed.associate { it.userId to it.pinVersion }
        return known
            .filter { waiter -> current[waiter.userId] != waiter.pinVersion }
            .map(Known::userId)
            .toSet()
    }
}

/**
 * Permisos de Android que hacen falta para hablar con la red local del restaurante (§4), según la
 * versión del sistema y el `targetSdk` de la aplicación. Hoy (`targetSdk` 36, A1-D7) ninguno:
 * `NsdManager` y las conexiones a la red local no piden permiso. Android 17 (API 37) exige
 * `ACCESS_LOCAL_NETWORK` (grupo «Dispositivos cercanos») a las aplicaciones con `targetSdk` 37 o
 * mayor para **todo** el tráfico a la red local (búsqueda y conexiones); la pantalla de explicación
 * ya existe y se mostrará sola al subir el `targetSdk`.
 */
object NetworkPermissions {
    const val ACCESS_LOCAL_NETWORK = "android.permission.ACCESS_LOCAL_NETWORK"
    private const val ANDROID_17 = 37

    fun required(sdkInt: Int, targetSdk: Int): List<String> =
        if (sdkInt >= ANDROID_17 && targetSdk >= ANDROID_17) {
            listOf(ACCESS_LOCAL_NETWORK)
        } else {
            emptyList()
        }
}
