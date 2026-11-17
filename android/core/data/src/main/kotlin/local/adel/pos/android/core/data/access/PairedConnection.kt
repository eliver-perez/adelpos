package local.adel.pos.android.core.data.access

import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.api.DeviceCredentials
import local.adel.pos.android.core.data.api.PinLoginRequest
import local.adel.pos.android.core.data.api.PinLoginResponse
import local.adel.pos.android.core.data.api.PinUsersResponse
import local.adel.pos.android.core.data.api.PinnedHttpClient
import local.adel.pos.android.core.data.api.ServerApi
import local.adel.pos.android.core.data.api.VersionInfo
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.server.ServerRelocator
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient

/**
 * Conexión con el servidor emparejado: la huella SPKI fijada (nunca otra), la credencial del
 * dispositivo y la dirección vigente. Si una petición **no llegó a salir** (no se pudo conectar o
 * respondió otro certificado), busca el servidor en la red por su `server_id` y su huella
 * ([ServerRelocator]) y, si lo encuentra en otra dirección con la huella completa, guarda la nueva
 * ([onMoved]) y repite la petición allí una sola vez: como no llegó, repetirla no duplica nada.
 */
internal class PairedConnection(
    val serverId: String,
    private val fingerprint: ByteArray,
    initialBase: String,
    val credentials: DeviceCredentials,
    identity: ClientIdentity,
    private val relocator: ServerRelocator,
    private val onMoved: suspend (String) -> Unit,
    private val client: OkHttpClient = PinnedHttpClient.create(fingerprint, identity),
) {
    @Volatile
    var base: String = initialBase
        private set

    suspend fun version(): ApiOutcome<VersionInfo> = call { api ->
        api.get("version", VersionInfo.serializer())
    }

    suspend fun pinUsers(): ApiOutcome<PinUsersResponse> = call { api ->
        api.get("auth/pin-users", PinUsersResponse.serializer(), credentials.headers())
    }

    suspend fun pinLogin(userId: String, pin: String): ApiOutcome<PinLoginResponse> = call { api ->
        api.post(
            "auth/pin-login",
            PinLoginRequest(userId, pin),
            PinLoginRequest.serializer(),
            PinLoginResponse.serializer(),
            credentials.headers(),
        )
    }

    private suspend fun <T> call(block: suspend (ServerApi) -> ApiOutcome<T>): ApiOutcome<T> {
        val failedBase = base
        val first = block(api(failedBase))
        if (first !is ApiOutcome.NoAnswer || first.requestSent || !first.error.mayHaveMoved()) {
            return first
        }
        val moved =
            relocator.relocate(serverId, fingerprint, failedBase, currentBase = { base })
                ?: return first
        if (moved != base) {
            base = moved
            onMoved(moved)
        }
        return block(api(moved))
    }

    /** API en la dirección vigente, con la huella fijada (sin buscar otra dirección). */
    fun api(): ServerApi = api(base)

    private fun api(base: String): ServerApi = ServerApi(base.toHttpUrl(), client)

    // Sin respuesta en la dirección o con otro certificado ahí: puede que el servidor cambió de IP
    // (y otro equipo tomó la anterior). Solo se acepta la dirección nueva con la huella completa.
    private fun AppError.mayHaveMoved(): Boolean =
        this == AppError.Unreachable || this == AppError.ServerIdentityChanged
}
