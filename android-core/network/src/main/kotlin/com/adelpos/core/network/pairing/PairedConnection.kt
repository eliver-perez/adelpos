package com.adelpos.core.network.pairing

import com.adelpos.core.model.connection.ServerAnswers
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.server.ServerRelocator
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.DeviceCredentials
import com.adelpos.core.network.api.IgnoredBody
import com.adelpos.core.network.api.PinnedHttpClient
import com.adelpos.core.network.api.ServerApi
import com.adelpos.core.network.api.VersionInfo
import com.adelpos.core.network.session.ServerTransport
import java.util.concurrent.TimeUnit
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient

/**
 * Conexión con el servidor emparejado: la huella SPKI fijada (nunca otra), la credencial del
 * dispositivo y la dirección vigente. Si una petición **no llegó a salir** (no se pudo conectar o
 * respondió otro certificado), busca el servidor en la red por su `server_id` y su huella
 * ([ServerRelocator]) y, si lo encuentra en otra dirección con la huella completa, guarda la nueva
 * ([onMoved]) y repite la petición allí una sola vez: como no llegó, repetirla no duplica nada.
 *
 * Cada respuesta de Go (o su falta) de este cliente llega a [answers], el estado único de conexión
 * (etapa A3, A3-D1): es el cliente de todo lo que habla con el servidor emparejado (entrada,
 * catálogo, modo Mesero, canal de eventos y de avisos).
 *
 * Etapa K1: compartida por las dos aplicaciones (android-core); lo propio de cada una (la entrada
 * con PIN de los meseros) se arma sobre [call].
 */
class PairedConnection(
    val serverId: String,
    private val fingerprint: ByteArray,
    initialBase: String,
    val credentials: DeviceCredentials,
    identity: ClientIdentity,
    private val relocator: ServerRelocator,
    private val onMoved: suspend (String) -> Unit,
    answers: ServerAnswers? = null,
    private val client: OkHttpClient =
        PinnedHttpClient.create(fingerprint, identity, answers = answers),
) {
    @Volatile
    var base: String = initialBase
        private set

    suspend fun version(): ApiOutcome<VersionInfo> = call { api ->
        api.get("version", VersionInfo.serializer())
    }

    /**
     * La sonda del estado de conexión (`GET health/live`, sin credenciales). Con [relocate], si no
     * responde en su dirección se busca en la red (cambio de IP); con [quick] (el mesero está
     * esperando), con un plazo de conexión corto.
     */
    suspend fun live(relocate: Boolean, quick: Boolean): ApiOutcome<IgnoredBody> {
        if (relocate) {
            return call { api -> api.get(LIVE_PATH, IgnoredBody.serializer()) }
        }
        val probeClient =
            if (quick) {
                client.newBuilder().connectTimeout(QUICK_CONNECT_SECONDS, TimeUnit.SECONDS).build()
            } else {
                client
            }
        return ServerApi(base.toHttpUrl(), probeClient).get(LIVE_PATH, IgnoredBody.serializer())
    }

    /**
     * [block] en la dirección vigente y, si no llegó a salir, una sola vez en la nueva dirección
     * del servidor (ver la clase). Etapa K1: público para las llamadas propias de cada aplicación
     * (la entrada con PIN de los meseros vive en `android/`).
     */
    suspend fun <T> call(block: suspend (ServerApi) -> ApiOutcome<T>): ApiOutcome<T> {
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

    /**
     * Dirección vigente y cliente con la huella fijada, para lo que no cabe en [ServerApi]: las
     * imágenes binarias con `ETag` y el canal de tiempo real (Fase D).
     */
    fun transport(): ServerTransport = ServerTransport(base.toHttpUrl(), client)

    private fun api(base: String): ServerApi = ServerApi(base.toHttpUrl(), client)

    // Sin respuesta en la dirección o con otro certificado ahí: puede que el servidor cambió de IP
    // (y otro equipo tomó la anterior). Solo se acepta la dirección nueva con la huella completa.
    private fun AppError.mayHaveMoved(): Boolean =
        this == AppError.Unreachable || this == AppError.ServerIdentityChanged

    private companion object {
        const val LIVE_PATH = "health/live"

        /**
         * Plazo de conexión de la sonda del momento: en la red local una conexión tarda
         * milisegundos; el mesero no espera los 5 s del cliente normal.
         */
        const val QUICK_CONNECT_SECONDS = 2L
    }
}
