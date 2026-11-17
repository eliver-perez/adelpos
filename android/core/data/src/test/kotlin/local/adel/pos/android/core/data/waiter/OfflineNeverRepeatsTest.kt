package local.adel.pos.android.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.ACCOUNT
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.ANA
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.BETO
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.BURGER
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.TABLE
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.accepted
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.error
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.json
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.line
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.waiter.AccountSummary
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.ChangeKind
import local.adel.pos.android.core.domain.waiter.DiscardResult
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.DraftSync
import local.adel.pos.android.core.domain.waiter.OrderChange
import local.adel.pos.android.core.domain.waiter.OrderFactory
import local.adel.pos.android.core.domain.waiter.OrderReview
import local.adel.pos.android.core.domain.waiter.OrderReviewRules
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.ReviewReason
import local.adel.pos.android.core.domain.waiter.SendRequest
import local.adel.pos.android.core.domain.waiter.TableAccountRef
import local.adel.pos.android.core.domain.waiter.Totals
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Revisión R6 en el dispositivo (A1-D19): las reproducciones del revisor (`R6ReviewTest`, que solo
 * vivían en su copia) convertidas en pruebas que exigen lo correcto, y una por cada corrección.
 * «Go» es MockWebServer con las respuestas del contrato v1 de A1-D18.
 */
@RunWith(AndroidJUnit4::class)
class OfflineNeverRepeatsTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private lateinit var captures: CaptureRepository
    private lateinit var queue: OrderQueueRepository
    private lateinit var sender: OrderSender
    private lateinit var serverClock: ServerClock
    private val ids = AtomicInteger(0)
    private val routes = mutableMapOf<String, ArrayDeque<MockResponse>>()
    private val received = ConcurrentLinkedQueue<RecordedRequest>()
    private val lostConfirms = AtomicInteger(0)
    private val inFlight = OrdersInFlight()

    // Un envío lento: el «Go» avisa que llegó la petición y espera a que la prueba lo suelte.
    @Volatile private var holdOffline: Boolean = false
    private val offlineArrived = CountDownLatch(1)
    private val offlineRelease = CountDownLatch(1)

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    received.add(request)
                    val key = keyOf(request)
                    if (holdOffline && key == "POST restaurant/offline-orders") {
                        offlineArrived.countDown()
                        offlineRelease.await(10, TimeUnit.SECONDS)
                    }
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
        server.credentials[ANA] = "credencial-de-ana"
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        serverClock = ServerClock(deviceClock = { DEVICE_NOW })
        // La respuesta de la confirmación en línea se pierde después de que Go la procesó.
        val api =
            WaiterApi(
                lostResponses =
                    LostResponses { method, path ->
                        method == "POST" &&
                            path.endsWith("/confirm") &&
                            lostConfirms.getAndDecrement() > 0
                    },
                serverClock = serverClock,
            )
        captures =
            CaptureRepository(
                dao = database.waiter(),
                server = server,
                api = api,
                scheduler = {},
                catalog =
                    object : CaptureRepository.CatalogLookups {
                        override val lookup = WaiterTestSupport.lookup

                        override fun stamp() = OrderFactory.CatalogStamp(41, "epoca-1")
                    },
                scope = scope,
                clock = { DEVICE_NOW },
                newId = { "00000000-0000-4000-8000-%012d".format(ids.incrementAndGet()) },
                saveDelayMillis = 20,
                serverClock = serverClock,
                inFlight = inFlight,
                settleTimeoutMillis = 3_000,
            )
        captures.start()
        queue =
            OrderQueueRepository(
                dao = database.waiter(),
                scheduler = {},
                clock = { DEVICE_NOW },
                newId = { "00000000-0000-4000-8000-1%011d".format(ids.incrementAndGet()) },
                discards = CaptureDiscards { captureId -> captures.discardNow(captureId) },
                inFlight = inFlight,
            )
        sender =
            OrderSender(
                database.waiter(),
                server,
                api,
                clock = { DEVICE_NOW },
                inFlight = inFlight,
                serverClock = serverClock,
            )
    }

    @After
    fun tearDown() {
        offlineRelease.countDown()
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    // --- C1: el guardado sin respuesta se resuelve antes de armar el pedido ---

    @Test
    fun conConexionElGuardadoSinRespuestaSeRepiteConSuClaveAntesDeEnviar() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture = openOnAccount()
        // El primer guardado no tiene respuesta útil; el segundo (la repetición) sí.
        respond("PUT restaurant/accounts/$ACCOUNT/draft", error(503, "SERVER_BUSY"))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        respond("POST restaurant/accounts/$ACCOUNT/confirm", bill(5))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.UNCERTAIN }
        val result = captures.sendToKitchen(capture.id, "Ana")
        // El pedido se armó con lo que Go guardó: confirmación en línea, nunca a ciegas.
        assertEquals(SendRequest.Queued(capture.id, OrderState.SENDING), result)
        awaitOrder(capture.id) { it.state == OrderState.RECEIVED }
        val puts = requests("PUT restaurant/accounts/$ACCOUNT/draft").map(::body)
        assertEquals(2, puts.size)
        assertEquals(operationOf(puts[0]), operationOf(puts[1]))
        assertTrue(requests("POST restaurant/offline-orders").isEmpty())
    }

    // Revisor (C1): sin conexión, el pedido lleva el guardado sin respuesta y el trabajador lo
    // repite igual (misma clave y contenido) con la sesión del mesero antes de mandarlo.
    @Test
    fun sinConexionElPedidoLlevaElGuardadoSinRespuestaYElTrabajadorLoResuelve() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture = openOnAccount()
        respond("PUT restaurant/accounts/$ACCOUNT/draft", error(503, "SERVER_BUSY"))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.UNCERTAIN }
        val first = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").first())
        server.paired = false
        val result = captures.sendToKitchen(capture.id, "Ana")
        assertEquals(SendRequest.Queued(capture.id, OrderState.QUEUED), result)
        val order = WaiterStorage.toOrder(database.waiter().order(capture.id)!!)
        val unresolved = assertNotNullValue(order.unresolvedSave)
        assertEquals(operationOf(first), unresolved.operationId)
        assertEquals(listOf("r1"), unresolved.items.map { it.id })
        // Vuelve la conexión: la repetición entra (o ya había entrado) y el pedido sale.
        server.paired = true
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("POST restaurant/offline-orders", accepted())
        sender.sendPending()
        val puts = requests("PUT restaurant/accounts/$ACCOUNT/draft").map(::body)
        assertEquals(first, puts.last())
        val sent = body(requests("POST restaurant/offline-orders").single())
        assertEquals(capture.id, operationOf(sent))
        // Va con el identificador del renglón: Go lo quita del borrador aunque no lo nombre.
        assertEquals(
            "r1",
            sent["items"]!!.jsonArray.single().jsonObject["id"]!!.jsonPrimitive.content,
        )
        val done = WaiterStorage.toOrder(database.waiter().order(capture.id)!!)
        assertEquals(OrderState.RECEIVED, done.state)
        assertNull(done.unresolvedSave)
    }

    @Test
    fun laAperturaSinRespuestaSeRepiteYElPedidoVaALaCuentaQueAbrio() = runBlocking {
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = null, seenAccountIds = emptyList())
        respond("POST restaurant/accounts", error(503, "SERVER_BUSY"))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.UNCERTAIN }
        server.paired = false
        captures.sendToKitchen(capture.id, "Ana")
        server.paired = true
        respond("POST restaurant/accounts", bill(1, "[${draftJson("r1")}]"))
        respond("POST restaurant/offline-orders", accepted())
        sender.sendPending()
        val openings = requests("POST restaurant/accounts").map(::body)
        assertEquals(operationOf(openings[0]), operationOf(openings[1]))
        val sent = body(requests("POST restaurant/offline-orders").single())
        assertEquals(
            ACCOUNT,
            sent["destination"]!!.jsonObject["account_id"]!!.jsonPrimitive.content,
        )
    }

    @Test
    fun lasPartidasYaEnUnaCuentaDeLaMesaSeMandanAEsaCuentaConLaMismaClave() = runBlocking {
        // Sin sesión (solo la credencial de envío): Go dice dónde están las partidas.
        server.sessions.clear()
        insert(WaiterTestSupport.queuedOrder("op-1", accountId = null))
        val change =
            """{"kind":"items_in_account","item_id":null,"product_id":null,"product_name":"",""" +
                """"before":null,"now":"folio 12","message":"Lo que capturaste ya está en la """ +
                """cuenta.","acceptable":false,"account_id":"$ACCOUNT"}"""
        respond(
            "POST restaurant/offline-orders",
            error(
                409,
                "ORDER_REQUIRES_CONFIRMATION",
                """{"status":"requires_confirmation","catalog_version":41,""" +
                    """"captured_catalog_version":41,"requires_changes":true,"changes":[$change]}""",
            ),
        )
        respond("POST restaurant/offline-orders", accepted())
        sender.sendPending()
        val sent = requests("POST restaurant/offline-orders").map(::body)
        assertEquals(2, sent.size)
        assertEquals("op-1", operationOf(sent[1]))
        assertEquals(
            ACCOUNT,
            sent[1]["destination"]!!.jsonObject["account_id"]!!.jsonPrimitive.content,
        )
        assertEquals(OrderState.RECEIVED, queue.order("op-1").first()!!.state)
    }

    // --- A1: la misma clave en la confirmación en línea y en la cola ---

    @Test
    fun laConfirmacionAplicadaSinRespuestaQuedaRecibidaSinDuplicar() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture = openOnAccount()
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.seenPrices["r1"] != null }
        // Go confirma las dos veces (la segunda repite la respuesta) y ambas se pierden.
        lostConfirms.set(2)
        respond("POST restaurant/accounts/$ACCOUNT/confirm", bill(5))
        respond("POST restaurant/accounts/$ACCOUNT/confirm", bill(5))
        captures.sendToKitchen(capture.id, "Ana")
        awaitOrder(capture.id) { it.state == OrderState.QUEUED }
        respond(
            "POST restaurant/offline-orders",
            error(
                409,
                OrderSender.ALREADY_ACCEPTED,
                """{"status":"already_accepted","origin":"confirm_draft","result":""" +
                    """{"status":"accepted","operation_id":"${capture.id}","order_id":"o1",""" +
                    """"account_id":"$ACCOUNT","already_sent_item_ids":["r1"]}}""",
            ),
        )
        sender.sendPending()
        val done = WaiterStorage.toOrder(database.waiter().order(capture.id)!!)
        assertEquals(OrderState.RECEIVED, done.state)
        assertEquals(ACCOUNT, done.receivedAccountId)
        val confirm = body(requests("POST restaurant/accounts/$ACCOUNT/confirm").first())
        val offline = body(requests("POST restaurant/offline-orders").single())
        assertEquals(capture.id, operationOf(confirm))
        assertEquals(capture.id, operationOf(offline))
    }

    // Revisor (A3): draft_item_sent ya no invita a duplicar: todo ya estaba en cocina, la
    // confirmación es «Confirmar sin volver a enviar» y Go responde sin pedido nuevo.
    @Test
    fun yaEnviadoSeConfirmaSinVolverAEnviarYLaCapturaNoLoRepite() = runBlocking {
        val order = orderReplacingR1("op-1")
        insert(order)
        val change =
            """{"kind":"draft_item_sent","item_id":"${order.lines.single().item.id}",""" +
                """"product_id":"$BURGER","product_name":"Hamburguesa","before":null,"now":null,""" +
                """"message":"Hamburguesa: ya se había enviado a cocina desde este dispositivo y """ +
                """no se vuelve a enviar.","acceptable":true}"""
        respond(
            "POST restaurant/offline-orders",
            error(
                409,
                "ORDER_REQUIRES_CONFIRMATION",
                """{"status":"requires_confirmation","catalog_version":41,""" +
                    """"captured_catalog_version":41,"requires_changes":false,"changes":[$change]}""",
            ),
        )
        sender.sendPending()
        val review = queue.order("op-1").first()!!
        assertEquals(OrderState.NEEDS_REVIEW, review.state)
        assertTrue(OrderReviewRules.everythingAlreadySent(review))
        // «Corregir en la captura» no lo vuelve a poner por enviar.
        val captureId = queue.backToCapture("op-1")!!
        val capture = captures.capture(captureId).first()!!
        assertTrue(capture.operations.isEmpty())
        assertTrue(capture.visible.none { it.id == review.lines.single().item.id })
    }

    // --- A2: descartar también quita de Go lo guardado ---

    // Revisor (A2), ahora exigiendo lo correcto.
    @Test
    fun descartarLaCapturaQuitaDeGoLoGuardadoYLoDeOtraTerminalSeQueda() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3, "[${draftJson("otro", 1)}]"))
        val capture = openOnAccount()
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            bill(4, "[${draftJson("otro", 1)},${draftJson("r1")}]"),
        )
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.base.any { b -> b.id == "r1" } }
        respond(
            "GET restaurant/accounts/$ACCOUNT",
            bill(4, "[${draftJson("otro", 1)},${draftJson("r1")}]"),
        )
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(5, "[${draftJson("otro", 1)}]"))
        assertEquals(DiscardResult.DONE, captures.discard(capture.id))
        assertNull(database.waiter().capture(capture.id))
        val removal = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").last())
        assertEquals(4, removal["revision"]!!.jsonPrimitive.content.toInt())
        // Solo se quitó lo suyo: el renglón de otra terminal se queda.
        assertEquals(
            listOf("otro"),
            removal["items"]!!.jsonArray.map { it.jsonObject["id"]!!.jsonPrimitive.content },
        )
    }

    @Test
    fun laCuentaQueNacioConLaCapturaSeDescartaSiQuedaVacia() = runBlocking {
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = null, seenAccountIds = emptyList())
        respond("POST restaurant/accounts", bill(1, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(1))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.accountId == ACCOUNT && it.sync == DraftSync.SAVED }
        assertTrue(captures.capture(capture.id).first()!!.createdAccount)
        respond("GET restaurant/accounts/$ACCOUNT", bill(1, "[${draftJson("r1")}]"))
        respond("POST restaurant/accounts/$ACCOUNT/discard", bill(2, state = "voided"))
        assertEquals(DiscardResult.DONE, captures.discard(capture.id))
        val discard = body(requests("POST restaurant/accounts/$ACCOUNT/discard").single())
        assertEquals(1, discard["revision"]!!.jsonPrimitive.content.toInt())
        assertTrue(requests("PUT restaurant/accounts/$ACCOUNT/draft").isEmpty())
    }

    @Test
    fun sinConexionElDescarteQuedaPendienteYSeTerminaAlVolver() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3))
        val capture = openOnAccount()
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(4, "[${draftJson("r1")}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(4))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER, 2))), null)
        awaitCapture(capture.id) { it.sync == DraftSync.SAVED && it.base.isNotEmpty() }
        server.paired = false
        assertEquals(DiscardResult.PENDING, captures.discard(capture.id))
        val pending = captures.capture(capture.id).first()!!
        assertEquals(DraftSync.DISCARDING, pending.sync)
        // Sigue contando como trabajo del mesero (no se pierde en silencio).
        assertTrue(pending.inProgress)
        // Al volver: se quita de Go y la captura desaparece.
        server.paired = true
        respond("GET restaurant/accounts/$ACCOUNT", bill(4, "[${draftJson("r1")}]"))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(5))
        captures.resume(ANA)
        withTimeout(5_000) { captures.capture(capture.id).first { it == null } }
        val removal = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").last())
        assertTrue(removal["items"]!!.jsonArray.isEmpty())
    }

    @Test
    fun descartarElPedidoQuitaDeGoLoQueSustituia() = runBlocking {
        val order = orderReplacingR1("op-1")
        insert(order.copy(state = OrderState.NEEDS_REVIEW))
        respond("GET restaurant/accounts/$ACCOUNT", bill(7, "[${draftJson("r1")}]"))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(8))
        assertEquals(DiscardResult.DONE, queue.discard("op-1"))
        assertNull(database.waiter().order("op-1"))
        assertNull(database.waiter().capture("op-1"))
        val removal = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").single())
        assertEquals(7, removal["revision"]!!.jsonPrimitive.content.toInt())
        assertTrue(removal["items"]!!.jsonArray.isEmpty())
    }

    // --- M1 y O1: escrituras condicionales al estado vigente ---

    // Revisor (M1): borrar mientras el trabajador espera la respuesta ya no revive el pedido.
    @Test
    fun borrarDuranteUnEnvioNoRevivenElPedido() = runBlocking {
        insert(orderReplacingR1("op-1"))
        holdOffline = true
        respond("POST restaurant/offline-orders", error(503, "SERVER_BUSY"))
        val job = scope.launch { sender.sendPending() }
        assertTrue(offlineArrived.await(5, TimeUnit.SECONDS))
        queue.eraseAll()
        offlineRelease.countDown()
        job.join()
        assertNull("el pedido borrado no vuelve", database.waiter().order("op-1"))
    }

    // Revisor (M1, variante con la cancelación del trabajador).
    @Test
    fun cancelarElTrabajadorDespuesDeBorrarNoRevivenElPedido() = runBlocking {
        insert(orderReplacingR1("op-1"))
        holdOffline = true
        respond("POST restaurant/offline-orders", error(503, "SERVER_BUSY"))
        val job = scope.launch { sender.sendPending() }
        assertTrue(offlineArrived.await(5, TimeUnit.SECONDS))
        queue.eraseAll()
        job.cancel()
        job.join()
        offlineRelease.countDown()
        assertNull("el pedido borrado no vuelve", database.waiter().order("op-1"))
    }

    @Test
    fun unaTransicionSoloSeEscribeSiElEstadoSigueSiendoElQueSeLeyo() = runBlocking {
        val order = orderReplacingR1("op-1")
        insert(order)
        val dao = database.waiter()
        val received = order.copy(state = OrderState.RECEIVED)
        // Se leyó «Enviando», pero sigue «Por enviar»: no se escribe.
        assertFalse(dao.transition(WaiterStorage.toEntity(received), "sending"))
        assertEquals("queued", dao.order("op-1")!!.state)
        dao.deleteOrder("op-1")
        assertFalse(dao.transition(WaiterStorage.toEntity(order), "queued"))
        assertNull(dao.order("op-1"))
    }

    // --- M2: «Corregir en la captura» conserva la base que vio ---

    // Revisor (M2), ahora exigiendo lo correcto: el cambio de la otra terminal se muestra.
    @Test
    fun corregirEnLaCapturaMuestraElConflictoConOtraTerminal() = runBlocking {
        val order = orderReplacingR1("00000000-0000-4000-8000-0000000000aa", quantity = 3)
        insert(order)
        val change =
            """{"kind":"draft_item_changed","item_id":"r1","product_id":"$BURGER",""" +
                """"product_name":"Hamburguesa","before":null,"now":null,""" +
                """"message":"Hamburguesa: lo cambiaron en otra terminal.","acceptable":true}"""
        respond(
            "POST restaurant/offline-orders",
            error(
                409,
                "ORDER_REQUIRES_CONFIRMATION",
                """{"status":"requires_confirmation","catalog_version":41,""" +
                    """"captured_catalog_version":41,"requires_changes":false,"changes":[$change]}""",
            ),
        )
        sender.sendPending()
        // Otra terminal dejó r1 con 5 unidades (revisión 7 de la cuenta).
        respond("GET restaurant/accounts/$ACCOUNT", bill(7, "[${draftJson("r1", 5)}]"))
        val captureId = queue.backToCapture(order.operationId)!!
        assertEquals(
            Capture.SEEN_BASE_REVISION,
            captures.capture(captureId).first()!!.baseRevision,
        )
        captures.retrySave(captureId)
        awaitCapture(captureId) { it.sync == DraftSync.CONFLICT }
        assertTrue(requests("PUT restaurant/accounts/$ACCOUNT/draft").isEmpty())
        // «Conservar mis cambios» es la decisión explícita del mesero.
        respond("GET restaurant/accounts/$ACCOUNT", bill(7, "[${draftJson("r1", 5)}]"))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(8, "[${draftJson("r1", 3)}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(8))
        captures.keepMine(captureId)
        awaitCapture(captureId) { it.sync == DraftSync.SAVED && it.baseRevision == 8 }
        val put = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").single())
        assertEquals(7, put["revision"]!!.jsonPrimitive.content.toInt())
    }

    @Test
    fun corregirSinCambiosDeOtraTerminalGuardaSobreLaBaseVista() = runBlocking {
        val order = orderReplacingR1("00000000-0000-4000-8000-0000000000bb", quantity = 3)
        insert(order.copy(state = OrderState.NEEDS_REVIEW, review = reviewOf("draft_item_changed")))
        respond("GET restaurant/accounts/$ACCOUNT", bill(7, "[${draftJson("r1", 2)}]"))
        respond("PUT restaurant/accounts/$ACCOUNT/draft", bill(8, "[${draftJson("r1", 3)}]"))
        respond("GET restaurant/accounts/$ACCOUNT/preview", quote(8))
        val captureId = queue.backToCapture(order.operationId)!!
        captures.retrySave(captureId)
        awaitCapture(captureId) { it.sync == DraftSync.SAVED && it.baseRevision == 8 }
        val put = body(requests("PUT restaurant/accounts/$ACCOUNT/draft").single())
        assertEquals(7, put["revision"]!!.jsonPrimitive.content.toInt())
        val item = put["items"]!!.jsonArray.single().jsonObject
        assertEquals(3, item["quantity"]!!.jsonPrimitive.content.toInt())
    }

    // --- M3: cuenta combinada ---

    @Test
    fun unaCuentaCombinadaOfreceMandarElPedidoALaCuentaEnQueSeCombino() = runBlocking {
        val order = orderReplacingR1("op-1")
        insert(order.copy(state = OrderState.NEEDS_REVIEW, review = reviewOf("account_closed")))
        queue.sendToAccount("op-1", ACCOUNT_B)
        respond("POST restaurant/offline-orders", accepted(ACCOUNT_B))
        sender.sendPending()
        val sent = body(requests("POST restaurant/offline-orders").single())
        assertEquals("op-1", operationOf(sent))
        assertEquals(
            ACCOUNT_B,
            sent["destination"]!!.jsonObject["account_id"]!!.jsonPrimitive.content,
        )
        // Lo que sustituía va igual: la combinación movió el borrador con sus identificadores.
        assertEquals(
            "r1",
            sent["replaces_draft_items"]!!
                .jsonArray
                .single()
                .jsonObject["id"]!!
                .jsonPrimitive
                .content,
        )
        assertEquals(OrderState.RECEIVED, queue.order("op-1").first()!!.state)
    }

    @Test
    fun laCapturaDeUnaCuentaCombinadaSigueEnLaCuentaEnQueSeCombino() = runBlocking {
        respond("GET restaurant/accounts/$ACCOUNT", bill(3, "[${draftJson("r1")}]"))
        val capture = openOnAccount()
        awaitCapture(capture.id) { it.base.isNotEmpty() }
        respond(
            "PUT restaurant/accounts/$ACCOUNT/draft",
            error(
                400,
                "OPERATION_VALIDATION",
                """{"reason":"account_not_open","account_id":"$ACCOUNT",""" +
                    """"account_state":"merged","merged_into_id":"$ACCOUNT_B"}""",
            ),
        )
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r2", line(BURGER, 1))), null)
        val closed =
            awaitCapture(capture.id) {
                it.sync == DraftSync.ACCOUNT_CLOSED && it.tableAccounts.isNotEmpty()
            }
        assertEquals(ACCOUNT_B, closed.tableAccounts.single().id)
        // La cuenta B tiene el borrador movido con sus identificadores: se guarda encima.
        respond(
            "GET restaurant/accounts/$ACCOUNT_B",
            bill(9, "[${draftJson("r1")}]", accountId = ACCOUNT_B),
        )
        respond(
            "PUT restaurant/accounts/$ACCOUNT_B/draft",
            bill(10, "[${draftJson("r1")},${draftJson("r2", 1)}]", accountId = ACCOUNT_B),
        )
        respond("GET restaurant/accounts/$ACCOUNT_B/preview", quote(10, ACCOUNT_B))
        captures.joinAccount(capture.id, TableAccountRef(ACCOUNT_B, 0, ""))
        awaitCapture(capture.id) { it.accountId == ACCOUNT_B && it.sync == DraftSync.SAVED }
        val put = body(requests("PUT restaurant/accounts/$ACCOUNT_B/draft").single())
        assertEquals(9, put["revision"]!!.jsonPrimitive.content.toInt())
        assertEquals(
            listOf("r1", "r2"),
            put["items"]!!.jsonArray.map { it.jsonObject["id"]!!.jsonPrimitive.content },
        )
    }

    // --- B2, B3 y B4 ---

    @Test
    fun unPedidoQueQuedoEnviandoSinPeticionSeRecuperaAlEmpezarLaVuelta() = runBlocking {
        insert(orderReplacingR1("op-1").copy(state = OrderState.SENDING))
        // Uno que la captura confirma en línea ahora mismo no se toca.
        insert(orderReplacingR1("op-2").copy(state = OrderState.SENDING))
        inFlight.add("op-2")
        respond("POST restaurant/offline-orders", accepted())
        sender.sendPending()
        assertEquals(OrderState.RECEIVED, queue.order("op-1").first()!!.state)
        assertEquals(OrderState.SENDING, queue.order("op-2").first()!!.state)
    }

    @Test
    fun losPedidosDeOtroMeseroDicenPorQueNoSalen() = runBlocking {
        insert(WaiterTestSupport.queuedOrder("op-1", userId = BETO).copy(credentialRejected = true))
        insert(
            WaiterTestSupport.queuedOrder("op-2", userId = BETO)
                .copy(state = OrderState.NEEDS_REVIEW, review = reviewOf("price_changed")),
        )
        insert(WaiterTestSupport.queuedOrder("op-3", userId = ANA))
        val others = queue.othersPending(ANA).first()
        assertEquals(1, others.size)
        val beto = others.single()
        assertEquals("Beto", beto.waiterName)
        assertEquals(2, beto.unsent)
        assertEquals(1, beto.waitingForPin)
        assertEquals(1, beto.needingReview)
    }

    @Test
    fun laHoraDeCapturaSeCorrigeConLaHoraDeGo() = runBlocking {
        // El dispositivo va 30 minutos adelantado: Go rechaza la hora y su respuesta trae la suya.
        val goNow = DEVICE_NOW - 30 * 60 * 1000L
        insert(orderReplacingR1("op-1").copy(capturedAtMillis = DEVICE_NOW))
        respond(
            "POST restaurant/offline-orders",
            withDate(error(400, "OPERATION_VALIDATION"), goNow),
        )
        respond("POST restaurant/offline-orders", withDate(accepted(), goNow))
        sender.sendPending()
        val sent = requests("POST restaurant/offline-orders").map(::body)
        assertEquals(2, sent.size)
        assertEquals(isoOf(DEVICE_NOW), sent[0]["captured_at"]!!.jsonPrimitive.content)
        assertEquals(isoOf(goNow), sent[1]["captured_at"]!!.jsonPrimitive.content)
        assertEquals(OrderState.RECEIVED, queue.order("op-1").first()!!.state)
    }

    // --- Utilidades ---

    private fun keyOf(request: RecordedRequest): String =
        "${request.method} ${request.url.encodedPath.removePrefix("/api/v1/")}"

    private fun respond(key: String, response: MockResponse) {
        synchronized(routes) { routes.getOrPut(key) { ArrayDeque() }.add(response) }
    }

    private fun requests(key: String): List<RecordedRequest> = received.filter { keyOf(it) == key }

    private fun body(request: RecordedRequest): JsonObject =
        Json.parseToJsonElement(request.body!!.utf8()).jsonObject

    private fun operationOf(body: JsonObject): String = body["operation_id"]!!.jsonPrimitive.content

    private suspend fun insert(order: PendingOrder) {
        database.waiter().insertOrder(WaiterStorage.toEntity(order))
    }

    private fun <T> assertNotNullValue(value: T?): T {
        assertNotNull(value)
        return value!!
    }

    private fun bill(
        revision: Int,
        draft: String = "[]",
        accountId: String = ACCOUNT,
        state: String = "open",
    ) =
        json(
            200,
            """{"account":{"id":"$accountId","folio":7,"name":"Mesa 4","mode":"dine_in",""" +
                """"state":"$state","table_id":"$TABLE","revision":$revision},""" +
                """"draft":$draft,"lines":[],"server_time":"x","timezone":"y"}""",
        )

    private fun quote(revision: Int, accountId: String = ACCOUNT) =
        json(
            200,
            """{"account_id":"$accountId","revision":$revision,"fingerprint":"huella-$revision",""" +
                """"quote":{"lines":[{"product_id":"$BURGER","name":"Hamburguesa",""" +
                """"unit_price":"125.00","quantity":2,"total":"250.00"}],"total":"250.00"},""" +
                """"combined":{"subtotal":"250.00","discount":"0.00","total":"250.00"}}""",
        )

    private fun draftJson(id: String, quantity: Int = 2) =
        """{"id":"$id","product_id":"$BURGER","variant_id":null,"quantity":$quantity,""" +
            """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}"""

    private fun withDate(response: MockResponse, millis: Long): MockResponse =
        response.newBuilder().setHeader("Date", httpDate(millis)).build()

    private fun httpDate(millis: Long): String =
        DateTimeFormatter.RFC_1123_DATE_TIME.format(
            Instant.ofEpochMilli(millis).atOffset(ZoneOffset.UTC),
        )

    private fun isoOf(millis: Long): String =
        DateTimeFormatter.ISO_INSTANT.format(Instant.ofEpochMilli(millis))

    private fun reviewOf(kind: String) =
        OrderReview(
            reason = ReviewReason.CHANGES,
            message = "Mensaje de Go",
            changes =
                listOf(
                    OrderChange(
                        kind = ChangeKind.fromWireName(kind),
                        wireKind = kind,
                        itemId = "r1",
                        productId = BURGER,
                        productName = "Hamburguesa",
                        before = null,
                        now = if (kind == "account_closed") "merged" else null,
                        message = "Mensaje de Go",
                        acceptable = kind != "account_closed",
                    ),
                ),
            catalogVersion = 41,
        )

    private suspend fun openOnAccount(): Capture {
        val capture =
            captures.open(ANA, TABLE, "Mesa 4", account = account, seenAccountIds = emptyList())
        awaitCapture(capture.id) { it.baseRevision == 3 }
        return capture
    }

    private suspend fun awaitCapture(id: String, condition: (Capture) -> Boolean): Capture =
        withTimeout(8_000) { captures.capture(id).first { it != null && condition(it) }!! }

    private suspend fun awaitOrder(id: String, condition: (PendingOrder) -> Boolean): PendingOrder =
        withTimeout(8_000) { queue.order(id).first { it != null && condition(it) }!! }

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

    /** Pedido «Por enviar» que sustituye el renglón r1 que Ana ya había guardado en Go. */
    private fun orderReplacingR1(operationId: String, quantity: Int = 2): PendingOrder {
        val saved = DraftLine("r1", line(BURGER, 2))
        val capture =
            WaiterTestSupport.capture(
                id = operationId,
                base = listOf(saved),
                operations =
                    if (quantity == 2) {
                        emptyList()
                    } else {
                        listOf(
                            DraftOperation.Replace("r1", DraftLine("r1", line(BURGER, quantity))),
                        )
                    },
                own = setOf("r1"),
            )
        return requireNotNull(
            OrderFactory.fromCapture(
                capture = capture,
                waiterName = "Ana",
                operationId = operationId,
                catalog = OrderFactory.CatalogStamp(41, "epoca-1"),
                lookup = WaiterTestSupport.lookup,
                onlineConfirm = null,
                nowMillis = DEVICE_NOW,
            ),
        )
    }

    private companion object {
        const val DEVICE_NOW = 1_700_000_000_000L
        const val ACCOUNT_B = "77777777-7777-4777-8777-777777777777"
    }
}
