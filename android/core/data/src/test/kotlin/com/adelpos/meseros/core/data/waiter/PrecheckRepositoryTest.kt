package com.adelpos.meseros.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.json
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.waiter.PrecheckFeedback
import com.adelpos.meseros.core.domain.waiter.PrecheckName
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinter
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PrintAnswer
import com.adelpos.meseros.core.domain.waiter.PrintMessage
import java.util.concurrent.ConcurrentLinkedQueue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
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
import org.junit.runner.RunWith

/**
 * Imprimir la precuenta desde el dispositivo (etapa A2, Fase C) contra un Go simulado que responde
 * con los JSON del contrato de la parte C (`Identity`, `PrintedDocument`, `PrinterRejection`,
 * `DocumentStatus`; `docs/API_V1.md`, «Imprimir la precuenta»), con Room (Robolectric, SDK 36): el
 * nombre visible y la impresora guardados para verse sin conexión (y borrados con los datos del
 * dispositivo), la sesión del mesero (nunca la credencial de envío), la misma clave si se pierde la
 * respuesta, cada rechazo y sin conexión sin pedir nada.
 */
@RunWith(AndroidJUnit4::class)
class PrecheckRepositoryTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private val connected = MutableStateFlow(true)

    /** Respuestas por «MÉTODO ruta» (sin `/api/v1/` ni la consulta), en orden. */
    private val routes = mutableMapOf<String, ArrayDeque<() -> MockResponse>>()
    private val received = ConcurrentLinkedQueue<RecordedRequest>()

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    received.add(request)
                    val key = keyOf(request)
                    val next = synchronized(routes) { routes[key]?.removeFirstOrNull() }
                    return next?.invoke() ?: WaiterTestSupport.error(500, "SIN_RESPUESTA")
                }
            }
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        // Ana tiene sesión de Go y también credencial de envío: imprimir solo usa la sesión.
        server.sessions[ANA] = "sesion-de-ana"
        server.credentials[ANA] = "credencial-de-ana"
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private fun repository() =
        PrecheckRepository(
            dao = database.waiter(),
            server = server,
            api = WaiterApi(),
            connected = connected,
            scope = scope,
            wallClock = { 1_700_000_000_000L },
            retryDelayMillis = 10,
        )

    private fun keyOf(request: RecordedRequest) =
        "${request.method} ${request.url.encodedPath.removePrefix("/api/v1/")}"

    private fun respond(key: String, response: () -> MockResponse) {
        synchronized(routes) { routes.getOrPut(key) { ArrayDeque() }.add(response) }
    }

    private fun respond(key: String, response: MockResponse) = respond(key) { response }

    private fun requests(key: String): List<RecordedRequest> = received.filter { keyOf(it) == key }

    private fun bodyOf(request: RecordedRequest) =
        Json.parseToJsonElement(request.body!!.utf8()).jsonObject

    /** `Identity` de `getIdentity` como lo entrega Go (parte C: `terminal.precheck_printer`). */
    private fun identity(name: String, printer: String?) =
        json(
            200,
            """{"user":{"id":"$ANA","username":"ana","display_name":"Ana López",""" +
                """"permissions":["payments.tickets.print","accounts.read"]},""" +
                """"preferences":{"theme":"light","home_screen":"restaurant"},""" +
                """"terminal":{"id":"d1","name":"Tableta","register_id":null,""" +
                """"keyboard_mode":"auto","home_screen":"restaurant","idle_lock_seconds":120,""" +
                """"precheck_printer_id":${printer?.let { "\"p1\"" } ?: "null"},""" +
                """"precheck_printer":${printer ?: "null"}},""" +
                """"establishment":{"name":"Restaurante","timezone":"America/Monterrey",""" +
                """"currency":"MXN","precheck_name":"$name"}}""",
        )

    /** `PrintedDocument` de `printPrecheck` y `retryDocument`. */
    private fun printed(problem: String?, printer: String, terminal: String?) =
        json(
            200,
            """{"requested":true,"status":"pending","job_id":"j1","ticket_id":"t1",""" +
                """"message":"","problem":${problem?.let { "\"$it\"" } ?: "null"},""" +
                """"printer_name":"$printer",""" +
                """"terminal_name":${terminal?.let { "\"$it\"" } ?: "null"}}""",
        )

    /** `PrinterRejection` (409) con su `details`, como lo arma Go para un dispositivo android. */
    private fun printerRejection(code: String, message: String, problem: String) =
        json(
            409,
            """{"error":{"code":"$code","message":"$message","request_id":"r1",""" +
                """"details":{"document":"precheck","problem":"$problem",""" +
                """"printer_name":"Red","scope":"device"}}}""",
        )

    private fun documentStatus(state: String, problem: String?, terminal: String?) =
        json(
            200,
            """{"ticket_id":"t1","kind":"precheck","payment_id":null,""" +
                """"created_at":"2026-11-18T20:00:00Z","job_id":"j1","job_state":"$state",""" +
                """"job_problem":${problem?.let { "\"$it\"" } ?: "null"},""" +
                """"printer_name":"USB Caja 2","copies":1,""" +
                """"terminal_name":${terminal?.let { "\"$it\"" } ?: "null"}}""",
        )

    private fun active(session: Boolean) =
        ActiveWaiter(WaiterProfile(ANA, "Ana", "A", null, 1, null), hasServerSession = session)

    @Test
    fun nombreVisibleEImpresoraSeLeenYSeGuardanParaVerseSinConexion() = runBlocking {
        respond(
            "GET auth/me",
            identity(
                "sales_ticket",
                """{"id":"p1","name":"USB Caja 2","terminal_name":"Caja 2"}""",
            ),
        )
        val first = repository()
        first.start()
        // Al entrar un mesero con conexión se lee de Go.
        server.active.value = active(session = true)
        val read = withTimeout(5_000) { first.setup.first { it.read } }
        assertEquals(PrecheckName.SALES_TICKET, read.name)
        assertEquals(PrecheckPrinter("USB Caja 2", "Caja 2"), read.printer)
        assertEquals("Caja 2", read.printer!!.destination)
        val request = requests("GET auth/me").single()
        assertEquals("Bearer sesion-de-ana", request.headers["Authorization"])

        // Otro arranque sin conexión: lo guardado, sin pedir nada.
        server.active.value = active(session = false)
        val offline = repository()
        offline.start()
        val saved = withTimeout(5_000) { offline.setup.first { it.read } }
        assertEquals(read, saved)
        assertEquals(1, requests("GET auth/me").size)

        // Cambió en la principal: «Cuenta» y sin impresora.
        respond("GET auth/me", identity("bill", printer = null))
        offline.refreshSetup(ANA)
        assertEquals(
            PrecheckSetup(PrecheckName.BILL, printer = null, read = true),
            offline.setup.value,
        )
        // Sin sesión de ese mesero no se pide nada y se queda lo último.
        server.sessions.remove(ANA)
        offline.refreshSetup(ANA)
        assertEquals(2, requests("GET auth/me").size)

        // «Borrar los datos de este dispositivo» (WaiterDao.eraseAll, en deviceDataWipe) lo quita.
        database.waiter().eraseAll()
        assertNull(database.waiter().snapshot(PrecheckRepository.SNAPSHOT_KIND))
    }

    @Test
    fun imprimirConLaSesionYLaMismaClaveSiSePierdeLaRespuesta() = runBlocking {
        val precheck = repository()
        // La primera respuesta se pierde después de que Go la procesó.
        respond("POST payments/accounts/c1/precheck/print") {
            MockResponse.Builder().onResponseStart(SocketEffect.ShutdownConnection).build()
        }
        respond(
            "POST payments/accounts/c1/precheck/print",
            printed("agent_offline", "USB Caja 2", "Caja 2"),
        )
        respond("GET auth/me", identity("precheck", printer = null))
        val answer = precheck.print(ANA, "c1", "op-1")
        assertEquals(
            PrintAnswer.Queued("t1", "j1", "pending", "agent_offline", "USB Caja 2", "Caja 2"),
            answer,
        )
        val sent = requests("POST payments/accounts/c1/precheck/print")
        assertEquals(2, sent.size)
        // La misma clave en el reintento: Go no imprime dos.
        for (request in sent) {
            assertEquals("op-1", bodyOf(request)["operation_id"]!!.jsonPrimitive.content)
            assertEquals(setOf("operation_id"), bodyOf(request).keys)
            assertEquals("Bearer sesion-de-ana", request.headers["Authorization"])
            assertNull(request.headers[DeviceWaiterServer.OFFLINE_CREDENTIAL_HEADER])
        }
        // Lo que se le dice al mesero.
        assertEquals(
            PrintMessage.TerminalOffline("Caja 2"),
            PrecheckFeedback.message(
                (answer as PrintAnswer.Queued).status,
                answer.problem,
                answer.printerName,
                answer.terminalName,
            ),
        )
    }

    @Test
    fun cadaRechazoDeGoConSuCodigo() = runBlocking {
        val precheck = repository()
        respond(
            "POST payments/accounts/c1/precheck/print",
            printerRejection(
                "PRINTER_NOT_CONFIGURED",
                "Este dispositivo no tiene impresora asignada. Pide al administrador que la " +
                    "elija en Terminales.",
                "missing",
            ),
        )
        respond(
            "POST payments/accounts/c1/precheck/print",
            printerRejection(
                "PRINTER_UNAVAILABLE",
                "La impresora Red está desactivada.",
                "inactive",
            ),
        )
        respond(
            "POST payments/accounts/c1/precheck/print",
            WaiterTestSupport.error(403, "PERMISSION_DENIED"),
        )
        respond(
            "POST payments/accounts/c1/precheck/print",
            WaiterTestSupport.error(401, "SESSION_EXPIRED"),
        )
        repeat(4) { respond("GET auth/me", identity("precheck", printer = null)) }

        val missing = precheck.print(ANA, "c1", "op-1") as PrintAnswer.Rejected
        assertEquals("PRINTER_NOT_CONFIGURED", missing.code)
        assertEquals(
            PrintMessage.NoPrinter,
            PrecheckFeedback.rejection(missing.code, missing.status, missing.message),
        )
        val unavailable = precheck.print(ANA, "c1", "op-2") as PrintAnswer.Rejected
        assertEquals(
            PrintMessage.PrinterUnavailable("La impresora Red está desactivada."),
            PrecheckFeedback.rejection(unavailable.code, unavailable.status, unavailable.message),
        )
        val denied = precheck.print(ANA, "c1", "op-3") as PrintAnswer.Rejected
        assertEquals(
            PrintMessage.NoPermission,
            PrecheckFeedback.rejection(denied.code, denied.status, denied.message),
        )
        // La sesión que terminó va al punto único de la entrada (pide el PIN otra vez).
        precheck.print(ANA, "c1", "op-4")
        assertTrue(server.reports.single().first is AppError.SessionEnded)
        assertEquals(ANA, server.reports.single().second)
    }

    @Test
    fun sinConexionNoSePideNadaNiQuedaParaDespues() = runBlocking {
        val precheck = repository()
        // Sin red.
        connected.value = false
        assertEquals(PrintAnswer.NoConnection, precheck.print(ANA, "c1", "op-1"))
        assertEquals(PrintAnswer.NoConnection, precheck.retry(ANA, "c1", "t1", "op-2"))
        // Sin sesión de Go (entró sin conexión): tampoco con la credencial de envío.
        connected.value = true
        server.sessions.remove(ANA)
        assertEquals(PrintAnswer.NoConnection, precheck.print(ANA, "c1", "op-3"))
        // Sin servidor emparejado.
        server.sessions[ANA] = "sesion-de-ana"
        server.paired = false
        assertEquals(PrintAnswer.NoConnection, precheck.print(ANA, "c1", "op-4"))
        assertTrue(received.isEmpty())
        // Sin respuesta dos veces: sin conexión (y nada queda en la cola).
        server.paired = true
        repeat(2) {
            respond("POST payments/accounts/c1/precheck/print") {
                MockResponse.Builder().onResponseStart(SocketEffect.ShutdownConnection).build()
            }
        }
        assertEquals(PrintAnswer.NoConnection, precheck.print(ANA, "c1", "op-5"))
        assertTrue(database.waiter().ordersOf(ANA).isEmpty())
    }

    @Test
    fun estadoDelTrabajoYReintentarSinMotivo() = runBlocking {
        val precheck = repository()
        respond(
            "GET payments/accounts/c1/documents/status",
            documentStatus("pending", "agent_offline", "Caja 2"),
        )
        respond(
            "GET payments/accounts/c1/documents/status",
            documentStatus("error", "unreachable", null),
        )
        val waiting = precheck.status(ANA, "c1", "t1")
        assertEquals(
            PrintAnswer.Status("pending", "agent_offline", "USB Caja 2", "Caja 2"),
            waiting,
        )
        val failed = precheck.status(ANA, "c1", "t1") as PrintAnswer.Status
        assertEquals(
            PrintMessage.NotResponding("USB Caja 2"),
            PrecheckFeedback.message(
                failed.jobState,
                failed.jobProblem,
                failed.printerName,
                failed.terminalName,
            ),
        )
        assertTrue(PrecheckFeedback.canRetry(failed.jobState, failed.jobProblem))
        val statusRequest = requests("GET payments/accounts/c1/documents/status").first()
        assertEquals("t1", statusRequest.url.queryParameter("ticket_id"))

        respond("POST payments/accounts/c1/documents/retry", printed(null, "Red", null))
        respond(
            "POST payments/accounts/c1/documents/retry",
            WaiterTestSupport.error(409, "PRINT_NOT_FAILED"),
        )
        repeat(2) { respond("GET auth/me", identity("precheck", printer = null)) }
        val retried = precheck.retry(ANA, "c1", "t1", "op-9") as PrintAnswer.Queued
        assertEquals("t1", retried.ticketId)
        assertEquals("Red", retried.printerName)
        assertNull(retried.terminalName)
        val body = bodyOf(requests("POST payments/accounts/c1/documents/retry").single())
        // Sin motivo: solo la clave y la precuenta.
        assertEquals(setOf("operation_id", "ticket_id"), body.keys)
        assertEquals("op-9", body["operation_id"]!!.jsonPrimitive.content)
        assertEquals("t1", body["ticket_id"]!!.jsonPrimitive.content)
        val stale = precheck.retry(ANA, "c1", "t1", "op-10") as PrintAnswer.Rejected
        assertTrue(PrecheckFeedback.isStaleRetry(stale.code))
    }
}
