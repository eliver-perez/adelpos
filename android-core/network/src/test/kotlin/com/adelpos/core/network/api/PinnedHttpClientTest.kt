package com.adelpos.core.network.api

import com.adelpos.core.model.connection.ServerAnswers
import com.adelpos.core.network.security.PinnedTrustManager
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.Serializable
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.SocketEffect
import okhttp3.OkHttpClient
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * El cliente del servidor emparejado contra un MockWebServer con TLS (etapa A3, A3-D1): una
 * conexión libre más tiempo que el pool no se reutiliza; un `GET` sobre una conexión que el
 * servidor cerró se repite una vez con otra conexión y un `POST` no; y cada respuesta o falta de
 * respuesta llega al estado único de conexión.
 */
class PinnedHttpClientTest {
    private val serverCertificate = HeldCertificate.Builder().commonName("AD-EL POS prueba").build()
    private val identity = ClientIdentity(version = "0.3.0", kind = ClientKinds.ANDROID)
    private lateinit var server: MockWebServer
    private val answers = CountingAnswers()

    @Serializable private data class Marker(val respuesta: String)

    private class CountingAnswers : ServerAnswers {
        val answered = AtomicInteger()
        val noAnswer = AtomicInteger()

        override fun answered() {
            answered.incrementAndGet()
        }

        override fun noAnswer() {
            noAnswer.incrementAndGet()
        }
    }

    @Before
    fun start() {
        server = MockWebServer()
        val handshake = HandshakeCertificates.Builder().heldCertificate(serverCertificate).build()
        server.useHttps(handshake.sslSocketFactory())
        // Como la escucha de red de Go: HTTP/1.1 sin ALPN (Fase 0, §0.3).
        server.protocolNegotiationEnabled = false
        server.start()
    }

    @After
    fun stop() {
        server.close()
    }

    private val pin: ByteArray
        get() = PinnedTrustManager.spkiFingerprint(serverCertificate.certificate)

    private fun api(client: OkHttpClient) = ServerApi(server.url("/"), client)

    private fun respond(text: String, status: Int = 200) {
        server.enqueue(
            MockResponse.Builder()
                .code(status)
                .addHeader("Content-Type", "application/json; charset=utf-8")
                .body("""{"respuesta":"$text"}""")
                .build(),
        )
    }

    // El servidor lee la petición y cierra la conexión sin responder, como Go con una conexión
    // keep-alive que llevaba 60 s libre.
    private fun closeWithoutAnswer() {
        server.enqueue(
            MockResponse.Builder().onResponseStart(SocketEffect.ShutdownConnection).build(),
        )
    }

    @Test
    fun elPoolSueltaLasConexionesLibresAntesQueGo() {
        // Go cierra una conexión sin uso a los 60 s (IdleTimeout de lanlistener).
        assertTrue(PinnedHttpClient.IDLE_CONNECTION_SECONDS * 2 <= 60)
    }

    @Test
    fun unaConexionLibreMasTiempoQueElPoolNoSeReutiliza() = runBlocking {
        val client =
            PinnedHttpClient.createWithIdleMillis(pin, identity, idleConnectionMillis = 400)
        repeat(3) { respond("r$it") }
        api(client).get("uno", Marker.serializer())
        api(client).get("dos", Marker.serializer())
        Thread.sleep(1_200)
        api(client).get("tres", Marker.serializer())
        val first = server.takeRequest()
        val second = server.takeRequest()
        val third = server.takeRequest()
        assertEquals("dentro del plazo se reutiliza", first.connectionIndex, second.connectionIndex)
        assertNotEquals(
            "pasado el plazo, otra conexión",
            second.connectionIndex,
            third.connectionIndex,
        )
    }

    @Test
    fun unGetSobreUnaConexionCerradaSeRepiteUnaVezConOtraConexion() = runBlocking {
        val client = PinnedHttpClient.create(pin, identity, answers = answers)
        respond("primera")
        closeWithoutAnswer()
        respond("repetida")
        assertEquals(
            ApiOutcome.Success(Marker("primera")),
            api(client).get("a", Marker.serializer()),
        )
        val outcome = api(client).get("b", Marker.serializer())
        assertEquals(ApiOutcome.Success(Marker("repetida")), outcome)
        val first = server.takeRequest()
        val lost = server.takeRequest()
        val repeated = server.takeRequest()
        assertEquals("/api/v1/b", lost.url.encodedPath)
        assertEquals("/api/v1/b", repeated.url.encodedPath)
        assertEquals(
            "la que se perdió iba en la conexión reutilizada",
            first.connectionIndex,
            lost.connectionIndex,
        )
        assertNotEquals("la repetición va en otra", lost.connectionIndex, repeated.connectionIndex)
        // Para el estado de conexión fueron dos respuestas: la falla repetida no cuenta.
        assertEquals(2, answers.answered.get())
        assertEquals(0, answers.noAnswer.get())
    }

    @Test
    fun unPostSobreUnaConexionCerradaNuncaSeRepite() = runBlocking {
        val client = PinnedHttpClient.create(pin, identity, answers = answers)
        respond("primera")
        closeWithoutAnswer()
        respond("siguiente")
        api(client).get("a", Marker.serializer())
        val outcome = api(client).post("b", Marker("x"), Marker.serializer(), Marker.serializer())
        assertTrue("$outcome", outcome is ApiOutcome.NoAnswer && outcome.requestSent)
        // La respuesta que sigue en la cola es para la siguiente petición: el POST no se repitió.
        assertEquals(
            ApiOutcome.Success(Marker("siguiente")),
            api(client).get("c", Marker.serializer()),
        )
        assertEquals(2, answers.answered.get())
        assertEquals(1, answers.noAnswer.get())
    }

    @Test
    fun cualquierRespuestaDeGoCuentaComoRespuesta() = runBlocking {
        val client = PinnedHttpClient.create(pin, identity, answers = answers)
        respond("bien")
        server.enqueue(
            MockResponse.Builder()
                .code(503)
                .addHeader("Content-Type", "application/json; charset=utf-8")
                .body("""{"error":{"code":"X","message":"Ocupado.","request_id":"r"}}""")
                .build(),
        )
        api(client).get("a", Marker.serializer())
        val rejected = api(client).get("b", Marker.serializer())
        assertTrue(rejected is ApiOutcome.Rejected)
        assertEquals(2, answers.answered.get())
        assertEquals(0, answers.noAnswer.get())
    }

    @Test
    fun sinServidorEsUnaFaltaDeRespuestaYSeRepiteSoloUnaVez() = runBlocking {
        val client = PinnedHttpClient.create(pin, identity, answers = answers)
        // Una dirección donde ya no escucha nadie.
        val gone = MockWebServer()
        gone.start()
        val url = gone.url("/")
        gone.close()
        val outcome = ServerApi(url, client).get("a", Marker.serializer())
        assertTrue("$outcome", outcome is ApiOutcome.NoAnswer && !outcome.requestSent)
        assertEquals(0, answers.answered.get())
        assertEquals("una sola falla aunque se intentó dos veces", 1, answers.noAnswer.get())
    }
}
