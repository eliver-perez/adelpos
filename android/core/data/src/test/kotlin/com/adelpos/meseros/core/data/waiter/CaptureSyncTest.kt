package com.adelpos.meseros.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ACCOUNT
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.BURGER
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.SHAKE
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.TABLE
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.error
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.json
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.line
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import com.adelpos.meseros.core.domain.waiter.SeenUnion
import com.adelpos.meseros.core.domain.waiter.SendRequest
import com.adelpos.meseros.core.domain.waiter.TableAccountRef
import com.adelpos.meseros.core.domain.waiter.Totals
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El guardado del borrador en Go (como `draftSync.ts` del escritorio) y «Enviar a cocina», con Room
 * (Robolectric, SDK 36) y un Go simulado que responde por ruta.
 */
@RunWith(AndroidJUnit4::class)
class CaptureSyncTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private lateinit var captures: CaptureRepository
    private val sends = AtomicInteger(0)
    private val ids = AtomicInteger(0)

    /** Respuestas por «MÉTODO ruta» (sin `/api/v1/`), en orden; lo recibido, en [received]. */
    private val routes = mutableMapOf<String, ArrayDeque<MockResponse>>()
    private val received = ConcurrentLinkedQueue<RecordedRequest>()

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    received.add(request)
                    val key =
                        "${request.method} ${request.url.encodedPath.removePrefix("/api/v1/")}"
                    synchronized(routes) {
                        return routes[key]?.removeFirstOrNull() ?: error(500, "SIN_RESPUESTA")
                    }
                }
            }
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        server.sessions[ANA] = "sesion-de-ana"
        build()
    }

    private fun build() {
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        captures =
            CaptureRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                scheduler = { sends.incrementAndGet() },
                catalog =
                    object : CaptureRepository.CatalogLookups {
                        override val lookup = WaiterTestSupport.lookup

                        override fun stamp() = OrderFactory.CatalogStamp(41, "epoca-1")
                    },
                scope = scope,
                clock = { 1_700_000_000_000L },
                newId = { "00000000-0000-4000-8000-%012d".format(ids.incrementAndGet()) },
                saveDelayMillis = 20,
            )
        captures.start()
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private fun respond(key: String, response: MockResponse) {
        synchronized(routes) { routes.getOrPut(key) { ArrayDeque() }.add(response) }
    }

    private fun bill(revision: Int, draft: String = "[]", accountId: String = ACCOUNT) =
        json(
            200,
            """{"account":{"id":"$accountId","folio":7,"name":"Mesa 4","mode":"dine_in",""" +
                """"state":"open","table_id":"$TABLE","revision":$revision},""" +
                """"draft":$draft,"lines":[],"server_time":"x","timezone":"y"}""",
        )

    private fun quote(revision: Int, fingerprint: String = "huella-$revision") =
        json(
            200,
            """{"account_id":"$ACCOUNT","revision":$revision,"fingerprint":"$fingerprint",""" +
                """"quote":{"lines":[{"product_id":"$BURGER","name":"Hamburguesa",""" +
                """"unit_price":"125.00","quantity":2,"total":"250.00"}],"total":"250.00"},""" +
                """"combined":{"subtotal":"250.00","discount":"0.00","total":"250.00"}}""",
        )

    private fun draftJson(id: String, productId: String = BURGER, quantity: Int = 2) =
        """{"id":"$id","product_id":"$productId","variant_id":null,"quantity":$quantity,""" +
            """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}"""

    private suspend fun awaitCapture(id: String, condition: (Capture) -> Boolean): Capture =
        withTimeout(5_000) { captures.capture(id).first { it != null && condition(it) }!! }

    private fun requests(key: String): List<RecordedRequest> = received.filter {
        "${it.method} ${it.url.encodedPath.removePrefix("/api/v1/")}" == key
    }

    private fun body(request: RecordedRequest): JsonObject =
        Json.parseToJsonElement(request.body!!.utf8()).jsonObject

    private val account =
        AccountSummary(
            id = ACCOUNT,
            folio = 7,
            name = "Mesa 4",
            tableId = TABLE,
            tableName = "Mesa 4",
            state = "open",
            revision = 3,
            preparationState = null,
            confirmedQuantity = 0,
            draftQuantity = 0,
            totals = Totals.ZERO,
            assignedName = null,
            elapsedSeconds = null,
            longOpen = false,
        )

    @Test
    fun laCuentaNaceConElPrimerProducto() = runBlocking {
        respond("POST restaurant/accounts", bill(1, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(1))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = null, seenAccountIds = listOf("vista"))
        assertTrue(capture.pendingAccount)
        captures.stage(
            capture.id,
            DraftOperation.Add(DraftLine("r1", line(BURGER, 2))),
            Money("120.00"),
        )
        val saved = awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.accountId != null }
        assertEquals(ACCOUNT, saved.accountId)
        assertEquals(1, saved.baseRevision)
        assertTrue(saved.operations.isEmpty())
        assertTrue(saved.isOwn("r1"))
        val create = body(requests("POST restaurant/accounts").single())
        assertEquals(TABLE, create["table_id"]!!.jsonPrimitive.content)
        assertEquals(
            listOf("vista"),
            create["seen_account_ids"]!!.jsonArray.map { it.jsonPrimitive.content },
        )
        assertEquals(
            "r1",
            create["items"]!!.jsonArray.single().jsonObject["id"]!!.jsonPrimitive.content,
        )
        assertEquals(
            "Bearer sesion-de-ana",
            requests("POST restaurant/accounts").single().headers["Authorization"],
        )
        // El precio que vio el mesero pasa a ser el de la cotización de Go.
        val priced = awaitCapture(capture.id) { it.seenPrices["r1"] == Money("125.00") }
        assertEquals(Money("125.00"), priced.seenPrices["r1"])
    }

    @Test
    fun otraCuentaEnLaMesaLaDecideElMesero() = runBlocking {
        respond(
            "POST restaurant/accounts",
            error(
                409,
                "TABLE_HAS_ACCOUNT",
                """{"accounts":[{"id":"otra","folio":9,"name":"Mesa 4 (2)"}]}""",
            ),
        )
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = null, seenAccountIds = emptyList())
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val blocked = awaitCapture(capture.id) { it.sync == DraftSync.TABLE_HAS_ACCOUNT }
        assertEquals(listOf(TableAccountRef("otra", 9, "Mesa 4 (2)")), blocked.tableAccounts)
        // «Abrir otra»: se reenvía sabiendo de la otra cuenta, con otra clave.
        respond("POST restaurant/accounts", bill(1, "[${draftJson("r1", quantity = 1)}]"))
        captures.openAnotherAccount(capture.id)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.accountId == ACCOUNT }
        val creations = requests("POST restaurant/accounts").map(::body)
        assertEquals(
            listOf("otra"),
            creations[1]["seen_account_ids"]!!.jsonArray.map { it.jsonPrimitive.content },
        )
        assertFalse(creations[0]["operation_id"] == creations[1]["operation_id"])
    }

    /**
     * Etapa M1 (M1-D32): la unión que vio el mesero ya no tiene la mesa. Go no guarda nada: su
     * mensaje se dice, Mesas vuelve a leer, lo capturado se queda y al reintentar ya no se manda la
     * unión vista (Go revisa las cuentas como siempre).
     */
    @Test
    fun laUnionCambioEnOtraTerminalSeDiceYSeConservaLoCapturado() = runBlocking {
        var refreshed = 0
        scope.cancel()
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        captures =
            CaptureRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                scheduler = { sends.incrementAndGet() },
                catalog =
                    object : CaptureRepository.CatalogLookups {
                        override val lookup = WaiterTestSupport.lookup

                        override fun stamp() = OrderFactory.CatalogStamp(41, "epoca-1")
                    },
                scope = scope,
                clock = { 1_700_000_000_000L },
                newId = { "00000000-0000-4000-8000-%012d".format(ids.incrementAndGet()) },
                saveDelayMillis = 20,
                placesChanged = { refreshed++ },
            )
        captures.start()
        val message = "La unión cambió en otra terminal. Revisa las mesas e inténtalo otra vez."
        respond(
            "POST restaurant/accounts",
            json(
                409,
                """{"error":{"code":"TABLE_UNION_CONFLICT","message":"$message",""" +
                    """"details":{"table_id":"$TABLE","union_id":"u-vista"}}}""",
            ),
        )
        val capture =
            captures.open(
                ANA,
                TABLE,
                "Mesa 5 + 6",
                account = null,
                seenAccountIds = listOf("vista"),
                seenUnion = SeenUnion("u-vista"),
            )
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val rejected = awaitCapture(capture.id) { it.sync == DraftSync.REJECTED }
        assertEquals(message, rejected.syncMessage)
        assertEquals(null, rejected.seenUnion)
        assertEquals(listOf("r1"), rejected.visible.map { it.id })
        assertEquals(1, refreshed)
        val first = body(requests("POST restaurant/accounts").single())
        assertEquals("u-vista", first["seen_union_id"]!!.jsonPrimitive.content)
        // Reintentar: sin la unión vista.
        respond("POST restaurant/accounts", bill(1, "[${draftJson("r1", quantity = 1)}]"))
        captures.retrySave(capture.id)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.accountId == ACCOUNT }
        val second = body(requests("POST restaurant/accounts")[1])
        assertFalse(second.containsKey("seen_union_id"))
    }

    /**
     * Etapa M1 (M1-D9): la mesa ya está en una unión que el mesero no veía. Se ofrece como
     * TABLE_HAS_ACCOUNT con las cuentas de la unión (se leen su folio y su nombre) y la unión pasa
     * a ser la vista.
     */
    @Test
    fun laMesaUnidaSeOfreceComoOtraCuentaDeLaUnion() = runBlocking {
        respond(
            "POST restaurant/accounts",
            json(
                409,
                """{"error":{"code":"TABLE_IN_UNION","message":"La Mesa 4 ya está unida con """ +
                    """otras mesas (Mesa 4 + 5).","details":{"table_id":"$TABLE",""" +
                    """"union_id":"u1","union":{"id":"u1","name":"Mesa 4 + 5"},""" +
                    """"accounts":["$ACCOUNT"]}}}""",
            ),
        )
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture =
            captures.open(
                ANA,
                TABLE,
                "Mesa 4",
                account = null,
                seenAccountIds = emptyList(),
                seenUnion = SeenUnion(null),
            )
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val blocked = awaitCapture(capture.id) { it.sync == DraftSync.TABLE_HAS_ACCOUNT }
        assertEquals(listOf(TableAccountRef(ACCOUNT, 7, "Mesa 4")), blocked.tableAccounts)
        assertEquals(SeenUnion("u1"), blocked.seenUnion)
        val sent = body(requests("POST restaurant/accounts").single())
        assertEquals("null", sent["seen_union_id"].toString())
    }

    @Test
    fun guardadoSinRespuestaSeRepiteIgualAunqueLleguenToques() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        respond("PUT restaurant/accounts/$ACCOUNT/draft", error(503, "SERVER_BUSY"))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val uncertain = awaitCapture(capture.id) { it.sync == DraftSync.UNCERTAIN }
        assertEquals(1, uncertain.inFlightCount)
        // Llega otro toque mientras tanto: no entra en la repetición.
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r2", line(SHAKE))), null)

        // «Reinicio»: otra instancia sobre la misma base.
        scope.cancel()
        build()
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            bill(4, "[${draftJson("r1", quantity = 1)}]"),
        )
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            bill(5, "[${draftJson("r1", quantity = 1)},${draftJson("r2", SHAKE, 1)}]"),
        )
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(5))
        captures.retrySave(capture.id)
        val saved = awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.baseRevision == 5 }
        assertEquals(listOf("r1", "r2"), saved.base.map { it.id })
        val puts = requests("PUT restaurant/accounts/$ACCOUNT/draft").map(::body)
        assertTrue(puts.size >= 3)
        // El primero y todas sus repeticiones (antes y después del reinicio): misma clave y mismo
        // contenido, sin el toque que llegó después.
        for (repeated in puts.dropLast(1)) {
            assertEquals(puts[0], repeated)
        }
        assertEquals(1, puts[0]["items"]!!.jsonArray.size)
        assertEquals(2, puts.last()["items"]!!.jsonArray.size)
        assertEquals(4, puts.last()["revision"]!!.jsonPrimitive.content.toInt())
    }

    @Test
    fun otraTerminalCambioLaCuentaYElMeseroConservaSusCambios() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3, "[${draftJson("ajeno", SHAKE, 1)}]"))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        respond("PUT restaurant/accounts/$ACCOUNT/draft", error(409, "ACCOUNT_CONFLICT"))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val conflict = awaitCapture(capture.id) { it.sync == DraftSync.CONFLICT }
        assertEquals(1, conflict.operations.size)
        // Mientras dure el conflicto no se guarda nada solo.
        captures.retrySave(capture.id)
        respond(
            "GET restaurant/accounts/$ACCOUNT",
            bill(4, "[${draftJson("ajeno", SHAKE, 1)},${draftJson("otro", SHAKE, 1)}]"),
        )
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            bill(
                5,
                "[${draftJson("ajeno", SHAKE, 1)},${draftJson("otro", SHAKE, 1)},${draftJson("r1", quantity = 1)}]",
            ),
        )
        captures.keepMine(capture.id)
        val saved = awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.baseRevision == 5 }
        assertEquals(listOf("ajeno", "otro", "r1"), saved.base.map { it.id })
        val last = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").last())
        // Lo de la otra terminal se conserva y mis toques van encima de la revisión actual.
        assertEquals(4, last["revision"]!!.jsonPrimitive.content.toInt())
        assertEquals(3, last["items"]!!.jsonArray.size)
    }

    @Test
    fun sinConexionSeQuedaEnElDispositivoYSeEnviaPorLaCola() = runBlocking {
        server.sessions.clear()
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        captures.stage(
            capture.id,
            DraftOperation.Add(DraftLine("r1", line(BURGER, 2))),
            Money("120.00"),
        )
        awaitCapture(capture.id) { it.sync == DraftSync.PENDING }
        assertTrue(received.isEmpty())
        val result = captures.sendToKitchen(capture.id, "Ana")
        assertEquals(SendRequest.Queued(capture.id, OrderState.QUEUED), result)
        assertNull(captures.capture(capture.id).first())
        val order = database.waiter().order(capture.id)!!
        assertEquals("queued", order.state)
        assertTrue(sends.get() > 0)
        assertTrue(received.isEmpty())
    }

    @Test
    fun enLineaConfirmaConLaHuellaQueVioElMesero() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        respond("POST restaurant/accounts/$ACCOUNT/confirm", bill(5))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.seenPrices["r1"] != null }
        val result = captures.sendToKitchen(capture.id, "Ana")
        assertEquals(SendRequest.Queued(capture.id, OrderState.SENDING), result)
        val received =
            withTimeout(5_000) {
                database.waiter().observeOrder(capture.id).first { it?.state == "received" }!!
            }
        assertEquals(ACCOUNT, received.receivedAccountId)
        val confirm = body(requests("POST restaurant/accounts/$ACCOUNT/confirm").single())
        assertEquals("huella-4", confirm["expected_fingerprint"]!!.jsonPrimitive.content)
        assertEquals(4, confirm["revision"]!!.jsonPrimitive.content.toInt())
        // La misma clave que el pedido (A1-D19): si la respuesta se pierde, el pedido sin conexión
        // con esa clave es «ya aceptado».
        assertEquals(capture.id, confirm["operation_id"]!!.jsonPrimitive.content)
        assertEquals(0, sends.get())
    }

    @Test
    fun siCambioElCalculoSigueComoPedidoSinConexion() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        respond("POST restaurant/accounts/$ACCOUNT/confirm", error(409, "QUOTE_CHANGED"))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.seenPrices["r1"] != null }
        captures.sendToKitchen(capture.id, "Ana")
        val queued =
            withTimeout(5_000) {
                database.waiter().observeOrder(capture.id).first { it?.state == "queued" }!!
            }
        assertNull(queued.onlineConfirm)
        assertTrue(sends.get() > 0)
        // Sustituirá el renglón que Go ya tenía guardado, con el precio de la cotización.
        assertTrue(queued.replaces.contains("\"r1\""))
        assertTrue(queued.lines.contains("125.00"))
    }

    @Test
    fun cuentaCobradaAlEnviarRequiereRevision() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        respond(
            "POST restaurant/accounts/$ACCOUNT/confirm",
            error(
                400,
                "OPERATION_VALIDATION",
                """{"reason":"account_not_open","account_id":"$ACCOUNT","account_state":"closed","merged_into_id":null}""",
            ),
        )
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.seenPrices["r1"] != null }
        captures.sendToKitchen(capture.id, "Ana")
        val review =
            withTimeout(5_000) {
                database.waiter().observeOrder(capture.id).first { it?.state == "needs_review" }!!
            }
        assertTrue(review.review!!.contains(ReviewReason.ACCOUNT_NOT_OPEN.name))
        assertEquals(0, sends.get())
    }

    @Test
    fun conLaBaseSinLeerPrimeroSeLeeElBorradorDeGo() = runBlocking {
        // Empezó sin conexión en una cuenta existente: la base no se conoce.
        server.sessions.clear()
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.PENDING }
        assertEquals(0, captures.capture(capture.id).first()!!.baseRevision)
        // Vuelve la conexión: lee el borrador vigente (con un renglón de otra terminal) y guarda
        // encima, sin borrarlo.
        server.sessions[ANA] = "sesion-de-ana"
        respond("GET restaurant/accounts/$ACCOUNT", bill(6, "[${draftJson("ajeno", SHAKE, 1)}]"))
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            bill(7, "[${draftJson("ajeno", SHAKE, 1)},${draftJson("r1", quantity = 1)}]"),
        )
        captures.resume(ANA)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.baseRevision == 7 }
        val put = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").single())
        assertEquals(6, put["revision"]!!.jsonPrimitive.content.toInt())
        assertEquals(
            listOf("ajeno", "r1"),
            put["items"]!!.jsonArray.map { it.jsonObject["id"]!!.jsonPrimitive.content },
        )
    }

    @Test
    fun laCuentaCobradaQueLlegaPorElCanalNoEsUnConflicto() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        server.sessions.clear()
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.PENDING }
        // Otra terminal la cobró: llega la cuenta cerrada con otra revisión.
        val closed =
            AccountBill(
                account = account.copy(state = "closed", revision = 4),
                draft = emptyList(),
                lines = emptyList(),
            )
        captures.observe(capture.id, closed)
        val observed = awaitCapture(capture.id) { it.sync == DraftSync.ACCOUNT_CLOSED }
        assertEquals(1, observed.operations.size)
        // «Abrir una cuenta nueva en la mesa»: lo del mesero pasa a una cuenta pendiente.
        captures.openAnotherAccount(capture.id)
        val moved = awaitCapture(capture.id) { it.accountId == null }
        assertEquals(1, moved.visible.size)
        assertFalse(moved.visible.single().id == "r1")
    }
}
