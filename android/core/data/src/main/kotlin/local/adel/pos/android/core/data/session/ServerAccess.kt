package local.adel.pos.android.core.data.session

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.OkHttpClient

/**
 * Conexión autorizada con el servidor del restaurante a nombre del mesero actual: lo que necesitan
 * el catálogo y el tiempo real para hablar con Go.
 *
 * @property baseUrl dirección del servidor emparejado (por ejemplo `https://192.168.1.10:8443/`).
 * @property client cliente con la huella SPKI fijada y las cabeceras `X-Client-*`
 *   (`PinnedHttpClient`).
 * @property credentials cabeceras del dispositivo y del mesero: `X-Device-ID`, `X-Device-Token` y
 *   `Authorization: Bearer <sesión>`. Viven solo en memoria.
 *
 * Dos conexiones son la misma si apuntan al mismo servidor con las mismas credenciales: una sesión
 * nueva (otro mesero o el mismo que volvió a entrar) es otra conexión, y el canal de tiempo real se
 * vuelve a abrir con ella.
 */
class AuthorizedServer(val baseUrl: HttpUrl, val client: OkHttpClient, val credentials: Headers) {
    override fun equals(other: Any?): Boolean =
        other is AuthorizedServer && other.baseUrl == baseUrl && other.credentials == credentials

    override fun hashCode(): Int = 31 * baseUrl.hashCode() + credentials.hashCode()

    // Nunca imprime las credenciales.
    override fun toString(): String = "AuthorizedServer($baseUrl)"

    companion object {
        const val DEVICE_ID_HEADER = "X-Device-ID"
        const val DEVICE_TOKEN_HEADER = "X-Device-Token"
        const val AUTHORIZATION_HEADER = "Authorization"

        /** Cabeceras del dispositivo emparejado y de la sesión del mesero. */
        fun credentials(deviceId: String, deviceToken: String, sessionToken: String): Headers =
            Headers.Builder()
                .add(DEVICE_ID_HEADER, deviceId)
                .add(DEVICE_TOKEN_HEADER, deviceToken)
                .add(AUTHORIZATION_HEADER, "Bearer $sessionToken")
                .build()
    }
}

/**
 * Interfaz estrecha entre el acceso (emparejamiento y entrada con PIN, fases B y C, dueñas de la
 * credencial del dispositivo y de la sesión del mesero) y lo que la Fase D necesita: «dame la
 * conexión con las cabeceras del dispositivo y del mesero actual, o nada si no hay sesión».
 *
 * [current] es null sin servidor emparejado, sin mesero dentro o con un mesero que entró sin
 * conexión (no tiene sesión de Go); cambia al entrar, al cambiar de mesero y al salir. Quien la
 * implementa crea un [AuthorizedServer] nuevo solo cuando cambian el servidor, el dispositivo o la
 * sesión.
 */
interface ServerAccess {
    val current: StateFlow<AuthorizedServer?>
}

/** Acceso fijo, para pruebas y mientras no se conecta el acceso real. */
class FixedServerAccess(initial: AuthorizedServer? = null) : ServerAccess {
    private val state = MutableStateFlow(initial)

    override val current: StateFlow<AuthorizedServer?> = state.asStateFlow()

    fun set(server: AuthorizedServer?) {
        state.value = server
    }
}
