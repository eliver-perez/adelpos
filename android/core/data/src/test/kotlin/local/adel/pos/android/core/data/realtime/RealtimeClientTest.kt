package local.adel.pos.android.core.data.realtime

import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import local.adel.pos.android.core.data.catalog.CatalogTestSupport
import local.adel.pos.android.core.data.session.FixedServerAccess
import local.adel.pos.android.core.domain.realtime.AccessInterruption
import local.adel.pos.android.core.domain.realtime.RealtimeSignal
import local.adel.pos.android.core.domain.realtime.RealtimeState
import local.adel.pos.android.core.domain.realtime.RebuildReason
import local.adel.pos.android.core.domain.realtime.ReconnectBackoff
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import mockwebserver3.SocketEffect
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Cliente del canal de tiempo real contra un servidor simulado que escribe como Go
 * (`administration/events.go`): cursor, `interrupted` con `id: 0`, `reset`, reconexión con espera
 * creciente, conexión muerta, sesión nueva y segundo plano.
 */
class RealtimeClientTest {
    private lateinit var mock: MockWebServer
    private lateinit var access: FixedServerAccess
    private lateinit var scope: CoroutineScope
    private val active = MutableStateFlow(true)
    private val interruptions = mutableListOf<AccessInterruption>()
    private val signals = Channel<RealtimeSignal>(Channel.UNLIMITED)
    private val states = mutableListOf<RealtimeState>()

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
        access = FixedServerAccess(CatalogTestSupport.server(mock))
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    }

    @After
    fun tearDown() {
        scope.cancel()
        mock.close()
    }

    private fun client(seed: RealtimeCursorSeed? = null): RealtimeClient {
        val client =
            RealtimeClient(
                access = access,
                active = active,
                interruptions = { interruption, _ ->
                    synchronized(interruptions) { interruptions.add(interruption) }
                },
                cursorSeed = seed,
                backoff = ReconnectBackoff(initialMillis = 50, maximumMillis = 400),
                readTimeoutMillis = 300,
            )
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            client.signals.collect { signals.send(it) }
        }
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            client.state.collect { state -> synchronized(states) { states.add(state) } }
        }
        client.start(scope)
        return client
    }

    private fun stream(vararg messages: String, stall: Boolean = false): MockResponse {
        val builder =
            MockResponse.Builder()
                .code(200)
                .addHeader("Content-Type", "text/event-stream; charset=utf-8")
                .body(messages.joinToString(""))
        if (stall) {
            // La conexión sigue abierta sin escribir nada más (conexión muerta o en espera).
            builder.onResponseEnd(SocketEffect.Stall)
        }
        return builder.build()
    }

    private fun changes(cursor: Long, vararg events: String, reset: Boolean = false): String =
        "id: $cursor\nevent: changes\n" +
            "data: {\"cursor\":$cursor,\"reset\":$reset,\"events\":[${events.joinToString(",")}]}\n\n"

    private fun event(sequence: Long, kind: String, scope: String): String =
        "{\"sequence\":$sequence,\"id\":\"e$sequence\",\"version\":1,\"kind\":\"$kind\"," +
            "\"scope\":\"$scope\",\"station_id\":null,\"aggregate_id\":null," +
            "\"created_at\":\"2026-11-16T12:00:00Z\"}"

    private fun interrupted(code: String, status: Int): String =
        "id: 0\nevent: interrupted\ndata: {\"error\":\"$code\",\"status\":$status}\n\n"

    private suspend fun nextSignal(): RealtimeSignal = withTimeout(5_000) { signals.receive() }

    private fun takeRequest(): RecordedRequest =
        requireNotNull(mock.takeRequest(5, TimeUnit.SECONDS)) { "No llegó la petición esperada" }

    @Test
    fun elCursorSaleDeLosMensajesChangesYNuncaDelCeroDeInterrupted() = runBlocking {
        mock.enqueue(
            stream(changes(11, event(11, "catalog.changed", "catalog")), changes(12)),
        )
        mock.enqueue(
            stream(
                changes(13, event(13, "restaurant.draft.saved", "restaurant")),
                interrupted("SESSION_REVOKED", 401),
            ),
        )
        val client = client(seed = { 10L })

        // Abierto desde la semilla (restaurant/state): el primer mensaje avisa de reconstruir.
        assertEquals(RealtimeSignal.Rebuild(11, RebuildReason.FRESH_START), nextSignal())
        val changes = nextSignal() as RealtimeSignal.Changes
        assertEquals(13L, changes.cursor)
        assertEquals(listOf("restaurant.draft.saved"), changes.events.map { it.kind })
        assertEquals(
            RealtimeSignal.Interrupted(AccessInterruption.SessionEnded("SESSION_REVOKED")),
            nextSignal(),
        )

        val first = takeRequest()
        assertEquals("/api/v1/events", first.url.encodedPath)
        assertEquals("10", first.url.queryParameter("after"))
        assertEquals("Bearer ${CatalogTestSupport.SESSION}", first.headers["Authorization"])
        // Go cerró el primer flujo: se reconecta enseguida desde el último cursor.
        assertEquals("12", takeRequest().url.queryParameter("after"))
        // El 0 de interrupted nunca se toma como cursor y no se reconecta con esa sesión.
        delay(400)
        assertEquals(13L, client.currentCursor)
        assertEquals(2, mock.requestCount)
        assertEquals(listOf(AccessInterruption.SessionEnded("SESSION_REVOKED")), interruptions)
        assertTrue(client.state.value is RealtimeState.Interrupted)
    }

    @Test
    fun resetDelServidorPideReconstruirUnaSolaVez() = runBlocking {
        mock.enqueue(
            stream(
                changes(50, reset = true),
                changes(51, event(51, "catalog.product.sold_out", "catalog")),
                stall = true,
            ),
        )
        client(seed = null)
        assertEquals(RealtimeSignal.Rebuild(50, RebuildReason.SERVER_RESET), nextSignal())
        val changes = nextSignal() as RealtimeSignal.Changes
        assertEquals(listOf("catalog.product.sold_out"), changes.events.map { it.kind })
        // Sin semilla se abre sin cursor.
        assertNull(takeRequest().url.queryParameter("after"))
    }

    @Test
    fun reconectaConEsperaCrecienteTrasFallasSeguidas() = runBlocking {
        // Se anota cuándo llega cada petición: tres fallas (500) y después un flujo que abre.
        val arrivals = CopyOnWriteArrayList<Long>()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    arrivals.add(System.nanoTime())
                    return if (arrivals.size <= 3) {
                        CatalogTestSupport.error(500, "INTERNAL")
                    } else {
                        stream(changes(5), stall = true)
                    }
                }
            }
        val client = client(seed = { 4L })
        withTimeout(5_000) {
            while (arrivals.size < 4) {
                delay(10)
            }
        }
        // Esperas de al menos 50, 100 y 200 ms entre intentos (espera creciente).
        val gaps = (1 until 4).map { (arrivals[it] - arrivals[it - 1]) / 1_000_000 }
        assertTrue("Esperas entre intentos: $gaps", gaps[0] >= 45)
        assertTrue("Esperas entre intentos: $gaps", gaps[1] >= 95)
        assertTrue("Esperas entre intentos: $gaps", gaps[2] >= 195)
        assertTrue(interruptions.isEmpty())
        assertTrue(client.state.value !is RealtimeState.Interrupted)
    }

    @Test
    fun dispositivoRetiradoAlAbrirSeAvisaYNoSeReintenta() = runBlocking {
        mock.enqueue(CatalogTestSupport.error(401, "DEVICE_REVOKED"))
        client(seed = null)
        assertEquals(RealtimeSignal.Interrupted(AccessInterruption.DeviceRevoked), nextSignal())
        delay(300)
        assertEquals(1, mock.requestCount)
        assertEquals(listOf<AccessInterruption>(AccessInterruption.DeviceRevoked), interruptions)
    }

    @Test
    fun conDosConexionesAbiertasEsperaYVuelveAIntentar() = runBlocking {
        mock.enqueue(CatalogTestSupport.error(429, "EVENT_CONNECTION_LIMIT"))
        mock.enqueue(
            stream(changes(9), changes(10, event(10, "catalog.changed", "catalog")), stall = true),
        )
        client(seed = { 8L })
        assertEquals(RealtimeSignal.Rebuild(9, RebuildReason.FRESH_START), nextSignal())
        assertTrue(nextSignal() is RealtimeSignal.Changes)
        assertEquals(2, mock.requestCount)
        assertTrue(interruptions.isEmpty())
    }

    @Test
    fun unaConexionMuertaSeDetectaConElTiempoDeLecturaYSeReabre() = runBlocking {
        mock.enqueue(stream(changes(7), stall = true))
        mock.enqueue(stream(changes(8), stall = true))
        client(seed = { 6L })
        takeRequest()
        // Sin bytes durante 300 ms la conexión se da por muerta y se reabre desde el cursor 7.
        assertEquals("7", takeRequest().url.queryParameter("after"))
    }

    @Test
    fun alCambiarDeMeseroSeReabreConLaSesionNueva() = runBlocking {
        mock.enqueue(stream(changes(1), stall = true))
        mock.enqueue(stream(changes(2), stall = true))
        client(seed = null)
        assertEquals("Bearer ${CatalogTestSupport.SESSION}", takeRequest().headers["Authorization"])
        access.set(CatalogTestSupport.server(mock, session = "sesion-de-beto"))
        val reopened = takeRequest()
        assertEquals("Bearer sesion-de-beto", reopened.headers["Authorization"])
        // Sesión nueva: se abre desde la semilla, no desde el cursor de la sesión anterior.
        assertNull(reopened.url.queryParameter("after"))
        assertEquals(CatalogTestSupport.DEVICE_ID, reopened.headers["X-Device-ID"])
    }

    @Test
    fun enSegundoPlanoSeCierraYAlVolverSeAbre() = runBlocking {
        active.value = false
        mock.enqueue(stream(changes(3), stall = true))
        val client = client(seed = { 2L })
        delay(300)
        assertEquals(0, mock.requestCount)
        assertEquals(RealtimeState.Stopped, client.state.value)

        active.value = true
        assertEquals("2", takeRequest().url.queryParameter("after"))
        assertEquals(RealtimeSignal.Rebuild(3, RebuildReason.FRESH_START), nextSignal())
        active.value = false
        withTimeout(2_000) {
            while (client.state.value != RealtimeState.Stopped) {
                delay(10)
            }
        }
    }
}
