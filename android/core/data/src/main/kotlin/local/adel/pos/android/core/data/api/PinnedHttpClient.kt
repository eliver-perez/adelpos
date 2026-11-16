package local.adel.pos.android.core.data.api

import java.security.SecureRandom
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import local.adel.pos.android.core.data.security.PinnedTrustManager
import okhttp3.ConnectionSpec
import okhttp3.OkHttpClient

/** Cliente HTTPS hacia el servidor del restaurante por su escucha de red. */
object PinnedHttpClient {
    private const val CONNECT_TIMEOUT_SECONDS = 5L
    private const val READ_TIMEOUT_SECONDS = 15L

    /**
     * Cliente con la huella SPKI [spkiSha256] fijada (32 bytes) y las cabeceras de [identity].
     * - Solo TLS 1.3 y 1.2 con las suites restringidas de OkHttp (`RESTRICTED_TLS`); Android 8 a 9
     *   negocian TLS 1.2 (`ECDHE-ECDSA-AES128-GCM`, verificado contra Go en la Fase 0) y desde
     *   Android 10, TLS 1.3.
     * - Un `SSLContext` propio por cliente: su caché de sesiones TLS solo guarda sesiones validadas
     *   con esta huella (equivale a desactivar la reanudación, como el escritorio).
     * - Sin reintentos automáticos de OkHttp: cada escritura del servidor lleva su clave de
     *   operación y la aplicación decide cuándo reintentar con la misma clave.
     */
    fun create(spkiSha256: ByteArray, identity: ClientIdentity): OkHttpClient {
        val trustManager = PinnedTrustManager(spkiSha256)
        val context = SSLContext.getInstance("TLS")
        context.init(null, arrayOf(trustManager), SecureRandom())
        return OkHttpClient.Builder()
            .sslSocketFactory(context.socketFactory, trustManager)
            // La verificación del nombre se sustituye por la huella: el certificado de Go no trae
            // nombres alternativos (solo O=AD-EL POS, CN=AD-EL POS <server_id>) y la dirección del
            // servidor puede cambiar en la red local (se vuelve a encontrar por server_id y
            // huella). La identidad la da PinnedTrustManager, que rechaza cualquier llave distinta
            // de la fijada antes de que viaje un solo byte de la petición.
            .hostnameVerifier { _, _ -> true }
            .connectionSpecs(listOf(ConnectionSpec.RESTRICTED_TLS))
            .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .retryOnConnectionFailure(false)
            .addInterceptor(ClientHeadersInterceptor(identity))
            .build()
    }
}
