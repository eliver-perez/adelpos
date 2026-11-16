package local.adel.pos.android.core.data.discovery

import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.api.IgnoredBody
import local.adel.pos.android.core.data.api.PinnedHttpClient
import local.adel.pos.android.core.data.api.ServerApi
import local.adel.pos.android.core.domain.server.ServerProbe
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
