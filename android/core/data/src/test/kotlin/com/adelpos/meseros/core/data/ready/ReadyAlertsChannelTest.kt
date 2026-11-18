package com.adelpos.meseros.core.data.ready

import com.adelpos.meseros.core.data.catalog.CatalogTestSupport
import com.adelpos.meseros.core.data.session.AuthorizedServer
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertPresenter
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLedger
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle
import com.adelpos.meseros.core.domain.ready.ReadyAlertsProcessor
import com.adelpos.meseros.core.domain.ready.ReadyAlertsStatus
import com.adelpos.meseros.core.domain.realtime.ReconnectBackoff
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import mockwebserver3.SocketEffect
import okhttp3.Headers
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * El canal de avisos contra un servidor simulado que escribe como Go
 * (`administration/ready_alerts.go`, con el protocolo ajustado tras la revisión del servidor):
 * `Last-Event-ID` y `after`, cierre con reconexión desde el último cursor (también el cierre normal
 * con solo `: ping`), repetidos que se muestran una vez (por `key`), `interrupted` (solo
 * `DEVICE_REVOKED` es retirado), 401, 429 con lectura mientras tanto, `active: false`, espera
 * creciente, conexión muerta y la nueva consulta al terminar el día comercial. Solo la credencial
 * del dispositivo (nunca `Authorization`).
 */
class ReadyAlertsChannelTest {
    private lateinit var mock: MockWebServer
    private val server = FakeServer()
    private val ledger = MemoryLedger()
    private val presenter = RecordingPresenter()
    private val reconnections = AtomicInteger(0)

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
    }

    @After
    fun tearDown() {
        mock.close()
    }

    private fun channel(clock: () -> Long = System::currentTimeMillis) =
        ReadyAlertsChannel(
            server = server,
            ledger = ledger,
            processor = ReadyAlertsProcessor(ledger, presenter),
            client = ReadyAlertsClient(readTimeoutMillis = 300),
            clock = clock,
            backoff = ReconnectBackoff(initialMillis = 50, maximumMillis = 400),
            onReconnected = { reconnections.incrementAndGet() },
            healthyConnectionMillis = 100,
        )

    private fun stream(
        vararg messages: String,
        stall: Boolean = false,
        bodyDelayMillis: Long = 0,
    ): MockResponse {
        val builder =
            MockResponse.Builder()
                .code(200)
                .addHeader("Content-Type", "text/event-stream; charset=utf-8")
                .body(messages.joinToString(""))
        if (stall) {
            builder.onResponseEnd(SocketEffect.Stall)
        }
        if (bodyDelayMillis > 0) {
            // Las cabeceras salen enseguida (el canal abre) y el cuerpo después.
            builder.bodyDelay(bodyDelayMillis, TimeUnit.MILLISECONDS)
        }
        return builder.build()
    }

    private fun alerts(cursor: Long, vararg alerts: String, active: Boolean = true): String =
        "id: $cursor\nevent: alerts\ndata: ${batch(cursor, *alerts, active = active)}\n\n"

    private fun batch(
        cursor: Long,
        vararg alerts: String,
        active: Boolean = true,
        dayEndsAt: String = DAY_ENDS_AT,
    ): String =
        "{\"cursor\":$cursor,\"active\":$active,\"day_ends_at\":\"$dayEndsAt\"," +
            "\"settings\":{\"enabled\":$active,\"sound\":true,\"vibration\":false}," +
            "\"alerts\":[${alerts.joinToString(",")}]}"

    private fun alert(id: Long, account: String = "cuenta-4"): String =
        "{\"id\":$id,\"key\":\"k-$id\",\"account_id\":\"$account\",\"mode\":\"dine_in\",\"table_name\":\"Mesa 4\"," +
            "\"folio\":31,\"customer_name\":\"\",\"waiter_user_id\":\"ana\"," +
            "\"waiter_short_name\":\"Ana\",\"items\":[{\"product_name\":\"Filete · Grande\"," +
            "\"quantity\":2}],\"ready_at\":\"2026-11-18T20:41:0$id.12Z\"}"

    /** `event: interrupted` con la forma nueva (`{"code"}`). */
    private fun interrupted(code: String): String =
        "id: 0\nevent: interrupted\ndata: {\"code\":\"$code\"}\n\n"

    private fun takeRequest(): RecordedRequest =
        requireNotNull(mock.takeRequest(5, TimeUnit.SECONDS)) { "No llegó la petición esperada" }

    @Test
    fun abreDesdeElCursorGuardadoYAlCerrarseReconectaDesdeElUltimo() = runBlocking {
        ledger.stored = ReadyAlertsStatus(10, true, FUTURE, ReadyAlertSettings.DEFAULT)
        // Go cierra el primer flujo (a los 30 s); el segundo dice que ya no hay a quién avisar.
        mock.enqueue(stream(alerts(11, alert(11)), alerts(12)))
        mock.enqueue(stream(alerts(12), alerts(13, active = false), stall = true))

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.INACTIVE, reason)
        val first = takeRequest()
        assertEquals("/api/v1/ready-alerts/stream", first.url.encodedPath)
        assertEquals("10", first.url.queryParameter("after"))
        assertEquals("10", first.headers["Last-Event-ID"])
        assertEquals("text/event-stream", first.headers["Accept"])
        // Solo la credencial del dispositivo.
        assertEquals("d-1", first.headers["X-Device-ID"])
        assertEquals("token", first.headers["X-Device-Token"])
        assertNull(first.headers["Authorization"])
        val second = takeRequest()
        assertEquals("12", second.url.queryParameter("after"))
        assertEquals("12", second.headers["Last-Event-ID"])
        assertEquals(listOf(11L), presenter.shown.map { it.id })
        assertEquals(13L, ledger.stored?.cursor)
        assertEquals("cerrar a los 30 s no es una falla", 0, reconnections.get())
    }

    @Test
    fun unAvisoQueVuelveALlegarSeMuestraUnaSolaVez() = runBlocking {
        // El mismo aviso en dos mensajes seguidos (cursor atrasado o reintento de Go) y después
        // en una conexión nueva.
        mock.enqueue(
            stream(alerts(5, alert(5)), alerts(5, alert(5)), alerts(6, alert(5), alert(6))),
        )
        mock.enqueue(stream(alerts(6, alert(6)), alerts(6, active = false), stall = true))

        withTimeout(5_000) { channel().run() }

        assertEquals(listOf(5L, 6L), presenter.shown.map { it.id })
        assertEquals(setOf("k-5", "k-6"), ledger.seen)
    }

    @Test
    fun dispositivoRetiradoAlAbrirDetieneYSeAvisaSinReintentar() = runBlocking {
        mock.enqueue(CatalogTestSupport.error(401, "DEVICE_REVOKED"))

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.REPORTED, reason)
        assertEquals(1, mock.requestCount)
        assertTrue(server.reported.single() is AppError.DeviceRevoked)
    }

    @Test
    fun interruptedDelCanalEsUnRechazoYSuIdNoEsUnCursor() = runBlocking {
        ledger.stored = ReadyAlertsStatus(20, true, FUTURE, ReadyAlertSettings.DEFAULT)
        mock.enqueue(stream(alerts(21, alert(21)), interrupted("DEVICE_REVOKED")))

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.REPORTED, reason)
        assertEquals(21L, ledger.stored?.cursor)
        assertEquals(listOf(21L), presenter.shown.map { it.id })
        assertTrue(server.reported.single() is AppError.DeviceRevoked)
    }

    @Test
    fun conElCanalOcupadoLeeConGetYNoAbreOtroEnParalelo() = runBlocking {
        val paths = CopyOnWriteArrayList<String>()
        val streams = AtomicInteger(0)
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    paths.add(request.url.encodedPath)
                    return when (request.url.encodedPath) {
                        "/api/v1/ready-alerts" -> CatalogTestSupport.json(200, batch(31, alert(31)))
                        else ->
                            if (streams.incrementAndGet() == 1) {
                                CatalogTestSupport.error(429, "READY_ALERTS_CONNECTION_LIMIT")
                            } else {
                                stream(alerts(31, alert(31)), alerts(32, active = false))
                            }
                    }
                }
            }

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.INACTIVE, reason)
        assertEquals(
            listOf(
                "/api/v1/ready-alerts/stream",
                "/api/v1/ready-alerts",
                "/api/v1/ready-alerts/stream",
            ),
            paths,
        )
        assertEquals(
            "el aviso leído por GET no se repite en el canal",
            listOf(31L),
            presenter.shown.map { it.id },
        )
    }

    @Test
    fun sinRedReintentaConEsperaCrecienteYAlVolverLlegaLoPendienteUnaVez() = runBlocking {
        val arrivals = CopyOnWriteArrayList<Long>()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    arrivals.add(System.nanoTime())
                    return if (arrivals.size <= 3) {
                        CatalogTestSupport.error(503, "SERVER_BUSY")
                    } else {
                        stream(alerts(42, alert(41), alert(42)), alerts(42, active = false))
                    }
                }
            }
        ledger.stored = ReadyAlertsStatus(40, true, FUTURE, ReadyAlertSettings.DEFAULT)

        withTimeout(5_000) { channel().run() }

        val gaps = (1 until 4).map { (arrivals[it] - arrivals[it - 1]) / 1_000_000 }
        assertTrue("Esperas entre intentos: $gaps", gaps[0] >= 45)
        assertTrue("Esperas entre intentos: $gaps", gaps[1] >= 95)
        assertTrue("Esperas entre intentos: $gaps", gaps[2] >= 195)
        assertEquals(listOf(41L, 42L), presenter.shown.map { it.id })
        assertEquals("se pide un envío de la cola al reconectar", 1, reconnections.get())
    }

    @Test
    fun unaConexionMuertaSeDetectaYSeReconectaDesdeElCursor() = runBlocking {
        ledger.stored = ReadyAlertsStatus(50, true, FUTURE, ReadyAlertSettings.DEFAULT)
        // Abre y se queda callada: ni avisos ni `: ping` en más de la espera de lectura (en estas
        // pruebas 300 ms; en la aplicación, 45 s).
        mock.enqueue(stream(alerts(51, alert(51)), bodyDelayMillis = 2_000))
        mock.enqueue(stream(alerts(52, alert(51), alert(52)), alerts(52, active = false)))

        withTimeout(5_000) { channel().run() }

        takeRequest()
        assertEquals("desde el cursor guardado", "50", takeRequest().url.queryParameter("after"))
        assertEquals(listOf(51L, 52L), presenter.shown.map { it.id })
        assertEquals("fue una falla: se pide el envío de la cola al volver", 1, reconnections.get())
    }

    @Test
    fun trasVariasFallasBuscaElServidorEnOtraDireccion() = runBlocking {
        val calls = AtomicInteger(0)
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse =
                    if (calls.incrementAndGet() <= ReadyAlertsLifecycle.RELOCATE_AFTER_FAILURES) {
                        CatalogTestSupport.error(503, "SERVER_BUSY")
                    } else {
                        stream(alerts(1, active = false))
                    }
            }

        withTimeout(5_000) { channel().run() }

        assertEquals(1, server.relocations.get())
    }

    @Test
    fun pasadoElFinDelDiaVuelveAPreguntarYSigueMientrasGoDigaActivo() = runBlocking {
        val dayEnds = 1_763_550_000_000L
        ledger.stored = ReadyAlertsStatus(9, true, dayEnds, ReadyAlertSettings.DEFAULT)
        val paths = CopyOnWriteArrayList<String>()
        val streams = AtomicInteger(0)
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    paths.add(request.url.encodedPath)
                    return when (request.url.encodedPath) {
                        // Un mesero con su sesión vigente: sigue activo con el día siguiente.
                        "/api/v1/ready-alerts" -> CatalogTestSupport.json(200, batch(10))
                        else ->
                            if (streams.incrementAndGet() == 1) {
                                // El canal abierto y callado cuando llega la hora de preguntar.
                                stream(": ping\n\n", bodyDelayMillis = 2_000)
                            } else {
                                stream(alerts(11, active = false))
                            }
                    }
                }
            }
        // Faltan 100 ms para volver a preguntar (fin del día y su margen): antes de que venza la
        // espera de lectura del canal callado (300 ms en estas pruebas).
        val now = dayEnds + ReadyAlertsLifecycle.DAY_CHECK_AFTER_MILLIS - 100

        val reason = withTimeout(5_000) { channel(clock = { now }).run() }

        assertEquals(
            "se detiene solo cuando Go dice active: false",
            ReadyStopReason.INACTIVE,
            reason,
        )
        assertEquals(
            listOf(
                "/api/v1/ready-alerts/stream",
                "/api/v1/ready-alerts",
                "/api/v1/ready-alerts/stream",
            ),
            paths,
        )
        assertTrue(server.reported.isEmpty())
    }

    @Test
    fun unCierreNormalConSoloPingsReconectaEnseguidaSinContarComoFalla() = runBlocking {
        // Go cierra el canal (a los 5 minutos) sin haber mandado ningún aviso: solo `: ping`.
        mock.enqueue(stream(": ping\n\n", bodyDelayMillis = 250))
        mock.enqueue(stream(alerts(3, alert(3)), alerts(3, active = false)))

        withTimeout(5_000) { channel().run() }

        assertEquals(2, mock.requestCount)
        assertEquals("no fue una falla", 0, reconnections.get())
        assertEquals(listOf(3L), presenter.shown.map { it.id })
    }

    @Test
    fun interruptedConOtroCodigoEsTemporalYSinIdNoMueveElCursor() = runBlocking {
        // Forma corregida: `interrupted` sin `id` y con `{"code"}`; `TEMPORARY` (un 5xx) se
        // reintenta.
        mock.enqueue(
            stream(alerts(1), "event: interrupted\ndata: {\"code\":\"TEMPORARY\"}\n\n"),
        )
        mock.enqueue(stream(alerts(2, alert(2)), alerts(2, active = false)))

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.INACTIVE, reason)
        assertEquals(2, mock.requestCount)
        assertTrue("nada se dio por retirado", server.reported.isEmpty())
        takeRequest()
        assertEquals("1", takeRequest().url.queryParameter("after"))
        assertEquals(listOf(2L), presenter.shown.map { it.id })
    }

    @Test
    fun unRechazoDeLaCredencialAlAbrirSeAvisaComoEnLasDemasRutas() = runBlocking {
        mock.enqueue(CatalogTestSupport.error(401, "DEVICE_REQUIRED"))

        val reason = withTimeout(5_000) { channel().run() }

        assertEquals(ReadyStopReason.REPORTED, reason)
        assertEquals(1, mock.requestCount)
        assertEquals("DEVICE_REQUIRED", server.reported.single().code)
    }

    @Test
    fun unDispositivoQueNoEstaListoNoAbreNada() = runBlocking {
        server.ready = false

        assertEquals(ReadyStopReason.DEVICE_NOT_READY, channel().run())
        assertEquals(0, mock.requestCount)
    }

    @Test
    fun laLecturaDeRecuperacionLlevaElCursorYTraduceLosErrores() = runBlocking {
        mock.enqueue(CatalogTestSupport.json(200, batch(7, alert(7))))
        mock.enqueue(CatalogTestSupport.error(403, "READY_ALERTS_DEVICE_NOT_ALLOWED"))
        val client = ReadyAlertsClient()
        val read = client.read(server.connection(), 6) as ReadyRead.Success
        assertEquals(7L, read.batch.cursor)
        assertEquals("Filete · Grande", read.batch.alerts.single().items.single().productName)
        assertEquals("6", takeRequest().url.queryParameter("after"))
        val rejected = client.read(server.connection(), 7) as ReadyRead.Failed
        assertEquals("READY_ALERTS_DEVICE_NOT_ALLOWED", rejected.error.code)
    }

    private inner class FakeServer : ReadyAlertsServer {
        @Volatile override var ready: Boolean? = true
        val reported = CopyOnWriteArrayList<AppError>()
        val relocations = AtomicInteger(0)

        override suspend fun prepare() = Unit

        override fun connection(): AuthorizedServer =
            AuthorizedServer(
                baseUrl = mock.url("/"),
                client = OkHttpClient(),
                credentials = Headers.headersOf("X-Device-ID", "d-1", "X-Device-Token", "token"),
            )

        override fun report(error: AppError) {
            reported.add(error)
        }

        override suspend fun relocate() {
            relocations.incrementAndGet()
        }
    }

    private class MemoryLedger : ReadyAlertsLedger {
        @Volatile var stored: ReadyAlertsStatus? = null
        val seen = mutableSetOf<String>()

        override suspend fun status(): ReadyAlertsStatus? = stored

        override suspend fun unseen(alerts: List<ReadyAlert>): List<ReadyAlert> = alerts.filter {
            it.key !in seen
        }

        override suspend fun commit(batch: ReadyAlertsBatch, shown: List<ReadyAlert>) {
            shown.forEach { seen.add(it.key) }
            stored = batch.status
        }

        override suspend fun rememberStatus(batch: ReadyAlertsBatch) {
            stored = batch.status.copy(cursor = stored?.cursor ?: 0L)
        }

        override suspend fun erase() {
            stored = null
            seen.clear()
        }
    }

    private class RecordingPresenter : ReadyAlertPresenter {
        val shown = CopyOnWriteArrayList<ReadyAlert>()
        private val visible = mutableSetOf<String>()

        override suspend fun apply(settings: ReadyAlertSettings) = Unit

        override suspend fun present(alerts: List<ReadyAlert>) {
            for (alert in alerts) {
                if (visible.add(alert.key)) {
                    shown.add(alert)
                }
            }
        }
    }

    private companion object {
        const val DAY_ENDS_AT = "2099-01-01T11:00:00Z"

        /** Un fin de día comercial guardado que todavía no llega. */
        const val FUTURE = 4_070_000_000_000L
    }
}
