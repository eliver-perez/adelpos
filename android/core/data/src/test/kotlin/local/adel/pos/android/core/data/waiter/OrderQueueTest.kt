package local.adel.pos.android.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.BackoffPolicy
import androidx.work.ListenableWorker
import androidx.work.NetworkType
import androidx.work.testing.TestListenableWorkerBuilder
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
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
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.line
import local.adel.pos.android.core.data.waiter.WaiterTestSupport.queuedOrder
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.ReviewReason
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
 * La cola sin conexión (§7) con Room (Robolectric, SDK 36) contra un Go simulado: persistencia al
 * reabrir la base (como un reinicio del proceso), datos por mesero, envío con la credencial del
 * mesero que capturó, la misma clave en cada reintento, «Requiere tu revisión» y el trabajador de
 * WorkManager.
 */
@RunWith(AndroidJUnit4::class)
class OrderQueueTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private val sendRequests = AtomicInteger(0)
    private var dropResponses = 0
    private lateinit var queue: OrderQueueRepository
    private lateinit var sender: OrderSender

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        server.credentials[ANA] = "credencial-de-ana"
        server.credentials[BETO] = "credencial-de-beto"
        build()
    }

    private fun build() {
        queue =
            OrderQueueRepository(
                dao = database.waiter(),
                scheduler = { sendRequests.incrementAndGet() },
                describe = { selection ->
                    if (selection.productId == BURGER) "Hamburguesa" else null
                },
                clock = { 1_700_000_100_000L },
            )
        val lost = LostResponses { method, path ->
            val drop =
                method == "POST" && path == WaiterApi.OFFLINE_ORDERS_PATH && dropResponses > 0
            if (drop) {
                dropResponses--
            }
            drop
        }
        sender = OrderSender(database.waiter(), server, WaiterApi(lostResponses = lost))
    }

    @After
    fun tearDown() {
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private suspend fun insert(order: PendingOrder) {
        database.waiter().insertOrder(WaiterStorage.toEntity(order))
    }

    private suspend fun stateOf(operationId: String): OrderState? =
        queue.order(operationId).first()?.state

    private fun body(request: RecordedRequest): JsonObject =
        Json.parseToJsonElement(request.body!!.utf8()).jsonObject

    @Test
    fun losPedidosSobrevivenAReabrirLaBaseComoUnReinicio() = runBlocking {
        insert(queuedOrder("op-1"))
        val sending = queuedOrder("op-2").copy(state = OrderState.SENDING)
        insert(sending)
        database.close()

        // «Proceso nuevo»: otra base sobre el mismo archivo y otra cola.
        database = AppDatabase.open(context)
        build()
        assertEquals(OrderState.QUEUED, stateOf("op-1"))
        assertEquals(OrderState.SENDING, stateOf("op-2"))
        queue.recoverAfterRestart()
        // Lo que quedó «Enviando» vuelve a «Por enviar» con la misma clave.
        assertEquals(OrderState.QUEUED, stateOf("op-2"))
        val restored = queue.order("op-1").first()!!
        assertEquals(queuedOrder("op-1").lines, restored.lines)
        assertEquals(41L, restored.catalogVersion)
        assertEquals("epoca-1", restored.catalogEpoch)
        assertEquals(2, queue.pendingCount().first())
    }

    @Test
    fun cadaMeseroVeSoloSusPedidos() = runBlocking {
        insert(queuedOrder("op-ana", userId = ANA))
        insert(queuedOrder("op-beto", userId = BETO))
        assertEquals(listOf("op-ana"), queue.orders(ANA).first().map { it.operationId })
        assertEquals(listOf("op-beto"), queue.orders(BETO).first().map { it.operationId })
        assertEquals(1, queue.workOf(ANA).unsentOrders)
        assertTrue(queue.workOf(ANA).blocksSwitch)
        assertEquals(2, queue.pendingCount().first())
        assertEquals(1, queue.pendingCountOf(BETO).first())
        // Lo que no se envió, para la bitácora del retiro (con quién lo capturó).
        val unsent = queue.unsentOrders()
        assertEquals(setOf("Ana", "Beto"), unsent.map { it.waiterName }.toSet())
        assertTrue(unsent.all { it.description.startsWith("Mesa 4: 2 × Hamburguesa") })
        queue.eraseAll()
        assertEquals(0, queue.pendingCount().first())
    }

    @Test
    fun seEnviaConLaCredencialDelMeseroQueCapturo() = runBlocking {
        val saved = DraftLine("guardado", line(BURGER))
        insert(queuedOrder("op-ana", base = listOf(saved), own = setOf("guardado")))
        insert(queuedOrder("op-beto", userId = BETO, accountId = null))
        mock.enqueue(accepted())
        mock.enqueue(accepted("cuenta-nueva"))
        val run = sender.sendPending()
        assertEquals(SendRun(sent = 2, reviews = 0, retryLater = false), run)
        val first = mock.takeRequest()
        assertEquals("/api/v1/restaurant/offline-orders", first.url.encodedPath)
        assertEquals("credencial-de-ana", first.headers["X-Offline-Credential"])
        assertNull(first.headers["Authorization"])
        val anaBody = body(first)
        assertEquals("op-ana", anaBody["operation_id"]!!.jsonPrimitive.content)
        assertEquals(
            ACCOUNT,
            anaBody["destination"]!!.jsonObject["account_id"]!!.jsonPrimitive.content,
        )
        val item =
            anaBody["items"]!!.jsonArray.single {
                it.jsonObject["id"]!!.jsonPrimitive.content != "guardado"
            }
        assertEquals("120.00", item.jsonObject["seen_unit_price"]!!.jsonPrimitive.content)
        // El renglón que ya estaba guardado en Go viaja como lo vio el dispositivo.
        assertEquals(
            listOf("guardado"),
            anaBody["replaces_draft_items"]!!.jsonArray.map {
                it.jsonObject["id"]!!.jsonPrimitive.content
            },
        )
        assertEquals("2023-11-14T22:13:20Z", anaBody["captured_at"]!!.jsonPrimitive.content)
        val second = mock.takeRequest()
        assertEquals("credencial-de-beto", second.headers["X-Offline-Credential"])
        val betoBody = body(second)
        // Sin cuenta: a la mesa, sin sustituir borrador.
        assertEquals(
            TABLE,
            betoBody["destination"]!!.jsonObject["table_id"]!!.jsonPrimitive.content,
        )
        assertFalse(betoBody.containsKey("replaces_draft_items"))
        assertFalse(betoBody.containsKey("confirmation"))
        assertEquals(OrderState.RECEIVED, stateOf("op-ana"))
        assertEquals("cuenta-nueva", queue.order("op-beto").first()!!.receivedAccountId)
        assertEquals(0, queue.pendingCount().first())
    }

    @Test
    fun respuestaPerdidaSeReintentaConLaMismaClave() = runBlocking {
        insert(queuedOrder("op-1"))
        // Go procesa el primer envío, pero su respuesta se pierde.
        dropResponses = 1
        mock.enqueue(accepted())
        mock.enqueue(accepted())
        val first = sender.sendPending()
        assertTrue(first.retryLater)
        val afterLoss = queue.order("op-1").first()!!
        assertEquals(OrderState.QUEUED, afterLoss.state)
        assertEquals(1, afterLoss.attempts)
        val second = sender.sendPending()
        assertEquals(1, second.sent)
        val keys =
            listOf(mock.takeRequest(), mock.takeRequest()).map {
                body(it)["operation_id"]!!.jsonPrimitive.content
            }
        assertEquals(listOf("op-1", "op-1"), keys)
        assertEquals(OrderState.RECEIVED, stateOf("op-1"))
    }

    @Test
    fun yaAceptadoConOtroContenidoSeMarcaRecibidoSinDuplicar() = runBlocking {
        insert(queuedOrder("op-1"))
        mock.enqueue(
            error(
                409,
                "OFFLINE_ORDER_ALREADY_ACCEPTED",
                """{"status":"already_accepted","result":{"status":"accepted","account_id":"la-que-entro"}}""",
            ),
        )
        sender.sendPending()
        val order = queue.order("op-1").first()!!
        assertEquals(OrderState.RECEIVED, order.state)
        assertEquals("la-que-entro", order.receivedAccountId)
    }

    @Test
    fun cambiosDePrecioEsperanAlMeseroYSeConfirmanConLaMismaClave() = runBlocking {
        val order = queuedOrder("op-1")
        insert(order)
        val itemId = order.lines.single().item.id
        val change =
            """{"kind":"price_changed","item_id":"$itemId","product_id":"$BURGER",""" +
                """"product_name":"Hamburguesa","before":"120.00","now":"130.00",""" +
                """"message":"Hamburguesa: $120.00 → $130.00","acceptable":true}"""
        mock.enqueue(
            error(
                409,
                "ORDER_REQUIRES_CONFIRMATION",
                """{"status":"requires_confirmation","catalog_version":42,""" +
                    """"captured_catalog_version":41,"requires_changes":false,"changes":[$change]}""",
            ),
        )
        val run = sender.sendPending()
        assertEquals(1, run.reviews)
        val review = queue.order("op-1").first()!!
        assertEquals(OrderState.NEEDS_REVIEW, review.state)
        assertEquals(ReviewReason.CHANGES, review.review!!.reason)
        assertEquals("Hamburguesa: $120.00 → $130.00", review.review!!.changes.single().message)
        // Nunca se reenvía solo: sin la aceptación del mesero el trabajador no lo toca.
        mock.takeRequest()
        assertEquals(SendRun(0, 0, false), sender.sendPending())

        queue.accept("op-1")
        assertEquals(OrderState.QUEUED, stateOf("op-1"))
        mock.enqueue(accepted())
        sender.sendPending()
        val confirmed = body(mock.takeRequest())
        assertEquals("op-1", confirmed["operation_id"]!!.jsonPrimitive.content)
        val confirmation = confirmed["confirmation"]!!.jsonObject
        assertEquals(42, confirmation["catalog_version"]!!.jsonPrimitive.content.toInt())
        assertEquals(
            "130.00",
            confirmation["changes"]!!.jsonArray.single().jsonObject["now"]!!.jsonPrimitive.content,
        )
        assertEquals(OrderState.RECEIVED, stateOf("op-1"))
    }

    @Test
    fun credencialRechazadaConservaElPedidoHastaQueElMeseroEntreConConexion() = runBlocking {
        insert(queuedOrder("op-1"))
        mock.enqueue(error(401, "OFFLINE_CREDENTIAL_INVALID"))
        sender.sendPending()
        val held = queue.order("op-1").first()!!
        assertEquals(OrderState.QUEUED, held.state)
        assertTrue(held.credentialRejected)
        assertEquals("OFFLINE_CREDENTIAL_INVALID", server.reports.single().first.code)
        assertEquals(ANA, server.reports.single().second)
        // Mientras no vuelva a entrar no se intenta.
        assertEquals(SendRun(0, 0, false), sender.sendPending())
        assertEquals(1, mock.requestCount)

        server.credentials[ANA] = "credencial-nueva"
        queue.credentialRenewed(ANA)
        assertTrue(sendRequests.get() > 0)
        mock.enqueue(accepted())
        assertEquals(1, sender.sendPending().sent)
        mock.takeRequest()
        assertEquals("credencial-nueva", mock.takeRequest().headers["X-Offline-Credential"])
    }

    @Test
    fun sinRespuestaSeDetieneYPideReintentar() = runBlocking {
        insert(queuedOrder("op-1"))
        insert(queuedOrder("op-2"))
        mock.close()
        val run = sender.sendPending()
        assertTrue(run.retryLater)
        // Se detuvo en el primero: el segundo ni siquiera salió.
        assertEquals(1, queue.order("op-1").first()!!.attempts)
        assertEquals(0, queue.order("op-2").first()!!.attempts)
        assertEquals(OrderState.QUEUED, stateOf("op-1"))
    }

    @Test
    fun rechazoDeGoPasaARevisionConSuMensaje() = runBlocking {
        insert(queuedOrder("op-1"))
        mock.enqueue(error(403, "PERMISSION_DENIED"))
        sender.sendPending()
        val order = queue.order("op-1").first()!!
        assertEquals(OrderState.NEEDS_REVIEW, order.state)
        assertEquals(ReviewReason.REJECTED, order.review!!.reason)
        assertEquals("Mensaje de Go", order.review!!.message)
        // Corregir en la captura: vuelve a ser borrador con la misma clave.
        val captureId = queue.backToCapture("op-1")
        assertEquals("op-1", captureId)
        assertNull(queue.order("op-1").first())
        val capture = WaiterStorage.toCapture(database.waiter().capture("op-1")!!)
        assertEquals(order.lines.map { it.item }, capture.visible)
        assertTrue(capture.inProgress)
    }

    @Test
    fun elTrabajadorReintentaSinRespuestaYTerminaAlEnviar() = runBlocking {
        insert(queuedOrder("op-1"))
        val factory = OrderWorkerFactory({ sender }, OrderSendSignals())
        mock.enqueue(error(503, "SERVER_BUSY"))
        val retrying =
            TestListenableWorkerBuilder<OrderSendWorker>(context).setWorkerFactory(factory).build()
        assertEquals(ListenableWorker.Result.retry(), retrying.doWork())
        assertEquals(OrderState.QUEUED, stateOf("op-1"))
        mock.enqueue(accepted())
        val sending =
            TestListenableWorkerBuilder<OrderSendWorker>(context).setWorkerFactory(factory).build()
        assertEquals(ListenableWorker.Result.success(), sending.doWork())
        assertEquals(OrderState.RECEIVED, stateOf("op-1"))
    }

    @Test
    fun elEnvioNoExigeRedValidada() {
        // Riesgo de la Fase 0 (§1.6): sin restricción de red; el trabajador prueba el servidor.
        val request = WorkManagerOrderScheduler.sendRequest()
        assertEquals(
            NetworkType.NOT_REQUIRED,
            request.workSpec.constraints.requiredNetworkType,
        )
        assertEquals(BackoffPolicy.EXPONENTIAL, request.workSpec.backoffPolicy)
        assertEquals(
            NetworkType.NOT_REQUIRED,
            WorkManagerOrderScheduler.periodicRequest().workSpec.constraints.requiredNetworkType,
        )
    }

    @Test
    fun elBloqueoPorInactividadNoOcurreConPedidosSinEnviar() = runBlocking {
        insert(queuedOrder("op-1"))
        var now = 0L
        val idle = IdleLock(clock = { now }, work = queue, recheckMillis = 50)
        var locked: String? = null
        val watcher =
            CoroutineScope(Dispatchers.Default).launch {
                idle.watch(ANA, timeoutSeconds = 120) { locked = it }
            }
        // Una hora sin tocar la pantalla, con un pedido «Por enviar»: no se bloquea.
        now = 3_600_000L
        delay(400)
        assertNull(locked)
        // La captura en curso también lo impide, pero no impide cambiar de mesero (A1-D15).
        database
            .waiter()
            .saveCapture(
                WaiterStorage.toEntity(
                    WaiterTestSupport.capture(
                        id = "captura",
                        operations =
                            listOf(
                                DraftOperation.Add(
                                    DraftLine("r1", line(BURGER)),
                                ),
                            ),
                        own = setOf("r1"),
                    ),
                    1L,
                ),
            )
        val order = queue.order("op-1").first()!!
        database.waiter().saveOrder(WaiterStorage.toEntity(order.copy(state = OrderState.RECEIVED)))
        assertTrue(queue.workOf(ANA).blocksIdleLock)
        assertFalse(queue.workOf(ANA).blocksSwitch)
        delay(400)
        assertNull(locked)
        // Sin nada pendiente se bloquea en la siguiente revisión.
        database.waiter().deleteCapture("captura")
        withTimeout(5_000) {
            while (locked == null) {
                delay(50)
            }
        }
        assertEquals(ANA, locked)
        watcher.cancel()
    }
}
