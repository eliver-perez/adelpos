package local.adel.pos.android.core.data.waiter

import kotlinx.coroutines.flow.StateFlow
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.data.session.ServerTransport
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.error.AppError
import okhttp3.Headers

/**
 * Interfaz estrecha entre la entrada al dispositivo (fases B y C, dueñas de las credenciales) y el
 * modo Mesero. Las cabeceras se piden **para un mesero concreto, el dueño de la operación** (la
 * captura o el pedido guardan su mesero al crearse), nunca «las del mesero activo» (revisión R3,
 * M5): un guardado o un envío que termina después de cambiar de mesero sale igual a nombre de quien
 * lo capturó.
 */
interface WaiterServer {
    /** El mesero que usa el dispositivo (para las lecturas de las pantallas). */
    val activeWaiter: StateFlow<ActiveWaiter?>

    /** Dirección vigente y cliente con la huella fijada; null sin servidor emparejado. */
    fun transport(): ServerTransport?

    /**
     * Antes de enviar sin pantallas (WorkManager despertó el proceso con la aplicación cerrada):
     * lee el servidor emparejado guardado si todavía nadie lo leyó.
     */
    suspend fun prepare()

    /**
     * Cabeceras del dispositivo y la sesión de Go de [userId]; null si ese mesero no tiene sesión
     * (entró sin conexión o se cerró la aplicación): entonces solo se puede enviar con su
     * credencial de envío.
     */
    fun sessionHeaders(userId: String): Headers?

    /**
     * Cabeceras del dispositivo y la credencial de envío de [userId] (`X-Offline-Credential`,
     * A1-D1) para `submitOfflineOrder` sin sesión; null si no la hay (nunca entró con conexión aquí
     * o Go la invalidó).
     */
    suspend fun offlineHeaders(userId: String): Headers?

    /**
     * Punto único de los errores de acceso de Go (dispositivo retirado, sesión terminada,
     * credencial no válida, versión vieja), con el mesero dueño de la operación.
     */
    fun report(error: AppError, userId: String)
}

/**
 * [WaiterServer] sobre [AccessRepository]: la sesión de cada mesero vive solo en memoria y la
 * credencial de envío, cifrada con el Android Keystore.
 */
class DeviceWaiterServer(private val access: AccessRepository) : WaiterServer {
    override val activeWaiter: StateFlow<ActiveWaiter?> = access.activeWaiter

    override fun transport(): ServerTransport? = access.serverTransport()

    override suspend fun prepare() {
        if (access.status.value is DeviceStatus.Starting) {
            access.start()
        }
    }

    override fun sessionHeaders(userId: String): Headers? {
        // requestHeaders(userId) trae Authorization solo si ese mesero tiene sesión de Go (una
        // entrada sin conexión nunca usa la de una entrada anterior; revisión R3, A1).
        val headers = access.requestHeaders(userId) ?: return null
        if (headers[AUTHORIZATION_HEADER] == null) {
            return null
        }
        return headers
    }

    override suspend fun offlineHeaders(userId: String): Headers? {
        val credentials = access.deviceCredentials() ?: return null
        val credential = access.offlineCredential(userId) ?: return null
        return credentials.headers().newBuilder().add(OFFLINE_CREDENTIAL_HEADER, credential).build()
    }

    override fun report(error: AppError, userId: String) {
        access.report(error, userId)
    }

    companion object {
        /** Cabecera de la credencial de envío (solo en `POST restaurant/offline-orders`). */
        const val OFFLINE_CREDENTIAL_HEADER = "X-Offline-Credential"

        private const val AUTHORIZATION_HEADER = "Authorization"
    }
}
