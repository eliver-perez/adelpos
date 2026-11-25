package com.adelpos.core.network.discovery

import com.adelpos.core.model.server.ServerProbe
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.IgnoredBody
import com.adelpos.core.network.api.PinnedHttpClient
import com.adelpos.core.network.api.ServerApi
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/**
 * Prueba una dirección con la huella SPKI **completa** fijada (`GET health/live`, como `probe` del
 * escritorio): solo responde quien tiene la llave privada del servidor emparejado. Un certificado
 * con otra huella falla antes de enviar la petición.
 */
class HttpsServerProbe(private val identity: ClientIdentity) : ServerProbe {
    override suspend fun answers(base: String, fingerprint: ByteArray): Boolean {
        val url = base.toHttpUrlOrNull() ?: return false
        val client =
            PinnedHttpClient.create(fingerprint, identity, connectTimeoutSeconds = PROBE_SECONDS)
        val outcome = ServerApi(url, client).get("health/live", IgnoredBody.serializer())
        return outcome is ApiOutcome.Success
    }

    private companion object {
        const val PROBE_SECONDS = 4L
    }
}
