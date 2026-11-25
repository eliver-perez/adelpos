package com.adelpos.core.network.api

import com.adelpos.core.model.connection.ServerAnswers
import com.adelpos.core.network.security.FirstSeenTrustManager
import com.adelpos.core.network.security.PinnedTrustManager
import java.security.SecureRandom
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager
import okhttp3.ConnectionPool
import okhttp3.ConnectionSpec
import okhttp3.OkHttpClient

/** Cliente HTTPS hacia el servidor del restaurante por su escucha de red. */
object PinnedHttpClient {
    private const val CONNECT_TIMEOUT_SECONDS = 5L
    private const val READ_TIMEOUT_SECONDS = 15L

    /**
     * Cuánto se guarda una conexión libre para reutilizarla (etapa A3, A3-D1; causa encontrada en
     * la Fase 0, §0.3 del avance). Go cierra una conexión keep-alive sin uso a los 60 s
     * (`IdleTimeout`), OkHttp la guardaba 5 minutos y **no revisa** una conexión reutilizada antes
     * de un `GET`: la comprobación de cada 60 s caía siempre sobre una conexión muerta y fallaba
     * («Sin conexión» con la barra en «En línea»). Con 30 s, OkHttp la suelta mucho antes de que Go
     * la cierre (margen para la latencia y los relojes); abrir otra cuesta un intercambio TLS de
     * decenas de milisegundos en la red local.
     */
    const val IDLE_CONNECTION_SECONDS = 30L

    /** Conexiones libres que se guardan (el valor por omisión de OkHttp). */
    private const val MAX_IDLE_CONNECTIONS = 5

    /**
     * Cliente con la huella SPKI [spkiSha256] fijada (32 bytes) y las cabeceras de [identity].
     * - Solo TLS 1.3 y 1.2 con las suites restringidas de OkHttp (`RESTRICTED_TLS`); Android 8 a 9
     *   negocian TLS 1.2 (`ECDHE-ECDSA-AES128-GCM`, verificado contra Go en la Fase 0) y desde
     *   Android 10, TLS 1.3.
     * - Un `SSLContext` propio por cliente: su caché de sesiones TLS solo guarda sesiones validadas
     *   con esta huella (equivale a desactivar la reanudación, como el escritorio).
     * - Sin reintentos automáticos de OkHttp: cada escritura del servidor lleva su clave de
     *   operación y la aplicación decide cuándo reintentar con la misma clave. Solo una lectura
     *   (`GET`) sin respuesta se repite una vez, con una conexión nueva
     *   ([ServerAnswersInterceptor]).
     * - Conexiones libres solo [IDLE_CONNECTION_SECONDS] (antes de que Go las cierre).
     * - Cada respuesta de Go y cada petición sin respuesta van a [answers] (el estado único de
     *   conexión, A3-D1): solo el cliente del servidor emparejado lo informa.
     */
    fun create(
        spkiSha256: ByteArray,
        identity: ClientIdentity,
        connectTimeoutSeconds: Long = CONNECT_TIMEOUT_SECONDS,
        answers: ServerAnswers? = null,
    ): OkHttpClient =
        build(PinnedTrustManager(spkiSha256), identity, connectTimeoutSeconds, answers)

    /**
     * Cliente de una sesión de emparejamiento: fija el primer certificado que ve ([trust]) y exige
     * el mismo en el resto de la sesión. Mismas reglas de TLS que [create].
     */
    fun createForPairing(trust: FirstSeenTrustManager, identity: ClientIdentity): OkHttpClient =
        build(trust, identity, CONNECT_TIMEOUT_SECONDS, answers = null)

    /**
     * El cliente con un tiempo de conexión libre distinto (solo pruebas: comprobar que una conexión
     * libre más tiempo que el pool no se reutiliza).
     */
    internal fun createWithIdleMillis(
        spkiSha256: ByteArray,
        identity: ClientIdentity,
        idleConnectionMillis: Long,
        answers: ServerAnswers? = null,
    ): OkHttpClient =
        build(
            PinnedTrustManager(spkiSha256),
            identity,
            CONNECT_TIMEOUT_SECONDS,
            answers,
            idleConnectionMillis,
        )

    private fun build(
        trustManager: X509TrustManager,
        identity: ClientIdentity,
        connectTimeoutSeconds: Long,
        answers: ServerAnswers?,
        idleConnectionMillis: Long = TimeUnit.SECONDS.toMillis(IDLE_CONNECTION_SECONDS),
    ): OkHttpClient {
        val context = SSLContext.getInstance("TLS")
        context.init(null, arrayOf(trustManager), SecureRandom())
        val pool = ConnectionPool(MAX_IDLE_CONNECTIONS, idleConnectionMillis, TimeUnit.MILLISECONDS)
        return OkHttpClient.Builder()
            .sslSocketFactory(context.socketFactory, trustManager)
            // La verificación del nombre se sustituye por la huella: el certificado de Go no trae
            // nombres alternativos (solo O=AD-EL POS, CN=AD-EL POS <server_id>) y la dirección del
            // servidor puede cambiar en la red local (se vuelve a encontrar por server_id y
            // huella). La identidad la da el administrador de confianza, que rechaza cualquier
            // llave distinta de la fijada antes de que viaje un solo byte de la petición.
            .hostnameVerifier { _, _ -> true }
            .connectionSpecs(listOf(ConnectionSpec.RESTRICTED_TLS))
            .connectTimeout(connectTimeoutSeconds, TimeUnit.SECONDS)
            .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .retryOnConnectionFailure(false)
            .connectionPool(pool)
            // Nunca seguir redirecciones: el servidor del restaurante no las usa y una redirección
            // podría llevar la credencial a otro lugar.
            .followRedirects(false)
            .followSslRedirects(false)
            .addInterceptor(ClientHeadersInterceptor(identity))
            .addInterceptor(ServerAnswersInterceptor(answers, pool))
            .build()
    }
}
