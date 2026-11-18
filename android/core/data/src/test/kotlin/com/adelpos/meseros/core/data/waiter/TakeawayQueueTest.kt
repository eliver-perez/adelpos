package com.adelpos.meseros.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.BURGER
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.accepted
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.error
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.json
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.line
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CustomerTarget
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.SendRequest
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayPayment
import com.adelpos.meseros.core.domain.waiter.TakeawayStates
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import mockwebserver3.SocketEffect
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Para llevar en el dispositivo (etapa A2, §3 y A2-D1/A2-D2) con Room (Robolectric, SDK 36) y un Go
 * simulado que responde por ruta con las formas del contrato (`Bill` con la cuenta para llevar,
 * `OfflineOrderResult`, `updateTakeawayCustomer`, `deliverTakeaway`): «Nuevo pedido» sin productos
 * que no deja nada, el pedido que nace con el primer producto o al enviarse, la misma cola con
 * `destination.takeaway`, `items_in_account`, las ediciones del cliente sin conexión (la última
 * gana y nunca se pierden) y «Entregado».
 */
@RunWith(AndroidJUnit4::class)
class TakeawayQueueTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private lateinit var captures: CaptureRepository
    private lateinit var sender: OrderSender
    private lateinit var customers: CustomerEditSender
    private lateinit var restaurant: RestaurantRepository
    private lateinit var takeaway: TakeawayRepository
    private val sends = AtomicInteger(0)
    private val ids = AtomicInteger(0)
    private var wall = 1_700_000_000_000L

    /** Respuestas por «MÉTODO ruta» (sin `/api/v1/`), en orden; lo recibido, en [received]. */
    private val routes = mutableMapOf<String, ArrayDeque<() -> MockResponse>>()
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
                    val next = synchronized(routes) { routes[key]?.removeFirstOrNull() }
                    return next?.invoke() ?: error(500, "SIN_RESPUESTA")
                }
            }
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        server.credentials[ANA] = "credencial-de-ana"
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        val newId = { "00000000-0000-4000-8000-%012d".format(ids.incrementAndGet()) }
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
                clock = { wall },
                newId = newId,
                saveDelayMillis = 20,
            )
        captures.start()
        sender = OrderSender(database.waiter(), server, WaiterApi(), clock = { wall })
        customers = CustomerEditSender(database.waiter(), server, WaiterApi(), newId = newId)
        restaurant =
            RestaurantRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                realtime = null,
                scope = scope,
                elapsedClock = { 50_000L },
                wallClock = { wall },
            )
        takeaway =
            TakeawayRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                restaurant = restaurant,
                scheduler = { sends.incrementAndGet() },
                clock = { wall },
                newId = newId,
                retryDelayMillis = 10,
            )
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private fun respond(key: String, response: () -> MockResponse) {
        synchronized(routes) { routes.getOrPut(key) { ArrayDeque() }.add(response) }
    }

    private fun respond(key: String, response: MockResponse) = respond(key) { response }

    private fun requests(key: String): List<RecordedRequest> = received.filter {
        "${it.method} ${it.url.encodedPath.removePrefix("/api/v1/")}" == key
    }

    private fun body(request: RecordedRequest): JsonObject =
        Json.parseToJsonElement(request.body!!.utf8()).jsonObject

    private fun JsonObject.text(key: String): String = this[key]!!.jsonPrimitive.content

    /** `Bill` de un pedido para llevar, como lo entrega Go (contrato v1, `Account` de la A2). */
    private fun takeawayBill(
        revision: Int = 1,
        draft: String = "[]",
        customer: String = "",
        phone: String = "",
    ) =
        json(
            200,
            """{"account":{"id":"$TAKEAWAY","folio":12,"name":"Pedido para llevar",""" +
                """"mode":"takeaway","state":"open","table_id":null,"table_name":null,""" +
                """"customer_name":"$customer","phone":"$phone","origin":"Para llevar",""" +
                """"revision":$revision,"preparation_state":"draft",""" +
                """"payment_state":"not_recorded","delivery_state":"not_delivered"},""" +
                """"draft":$draft,"lines":[],"server_time":"x","timezone":"y"}""",
        )

    private fun draftJson(id: String, quantity: Int = 1) =
        """{"id":"$id","product_id":"$BURGER","variant_id":null,"quantity":$quantity,""" +
            """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}"""

    private suspend fun awaitCapture(id: String, condition: (Capture) -> Boolean): Capture =
        withTimeout(5_000) { captures.capture(id).first { it != null && condition(it) }!! }

    private suspend fun orderOf(operationId: String) =
        database.waiter().order(operationId)?.let(WaiterStorage::toOrder)

    private suspend fun edits() = database.waiter().customerEdits()

    @Test
    fun nuevoPedidoSinProductosNoDejaNada() = runBlocking {
        server.sessions[ANA] = "sesion-de-ana"
        val capture = captures.openTakeaway(ANA, account = null)
        assertEquals(OrderMode.TAKEAWAY, capture.mode)
        assertNull(capture.tableId)
        // El nombre y el teléfono se pueden escribir antes del primer producto…
        captures.setCustomer(capture.id, TakeawayCustomer("Juan", "555"))
        val named = awaitCapture(capture.id) { it.customer.name == "Juan" }
        assertFalse("sin productos no es trabajo pendiente", named.inProgress)
        // …y salir sin productos no deja nada: ni en Go ni en el dispositivo.
        captures.closeIfIdle(capture.id)
        withTimeout(5_000) {
            while (database.waiter().capture(capture.id) != null) {
                delay(10)
            }
        }
        delay(100)
        assertTrue("ninguna petición a Go", received.isEmpty())
        assertTrue(database.waiter().allCaptures().isEmpty())
        assertTrue(edits().isEmpty())
        // Un «Nuevo pedido» después empieza vacío.
        val again = captures.openTakeaway(ANA, account = null)
        assertEquals(TakeawayCustomer.NONE, again.customer)
    }

    @Test
    fun conConexionNaceAlGuardarElPrimerProducto() = runBlocking {
        server.sessions[ANA] = "sesion-de-ana"
        respond(
            "POST restaurant/accounts",
            takeawayBill(draft = "[${draftJson("r1")}]", customer = "Juan", phone = "555"),
        )
        respond("GET restaurant/accounts/$TAKEAWAY/preview", error(503, "SERVER_BUSY"))
        val capture = captures.openTakeaway(ANA, account = null)
        captures.setCustomer(capture.id, TakeawayCustomer(" Juan ", "555"))
        awaitCapture(capture.id) { it.customer.name == "Juan" }
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        val saved = awaitCapture(capture.id) { it.accountId != null && it.sync == DraftSync.SAVED }
        assertEquals(TAKEAWAY, saved.accountId)
        assertTrue(saved.createdAccount)
        val open = body(requests("POST restaurant/accounts").single())
        assertEquals("takeaway", open.text("mode"))
        assertEquals(JsonNull, open["table_id"])
        assertFalse("sin cuentas vistas", open.containsKey("seen_account_ids"))
        assertEquals("Para llevar", open.text("origin"))
        assertEquals("Juan", open.text("customer_name"))
        assertEquals("555", open.text("phone"))
        assertEquals(
            "Bearer sesion-de-ana",
            requests("POST restaurant/accounts").single().headers["Authorization"],
        )

        // Ya nació: el nombre ya no cambia en la captura, va como edición aparte.
        val before = sends.get()
        captures.setCustomer(capture.id, TakeawayCustomer("Juan Pérez", "555"))
        val edit = edits().single()
        assertEquals(TAKEAWAY, edit.key)
        assertEquals(TAKEAWAY, edit.accountId)
        assertEquals("Juan Pérez", edit.customerName)
        assertEquals("Juan", database.waiter().capture(capture.id)!!.customerName)
        assertTrue("se pidió enviarla", sends.get() > before)
        respond(
            "POST restaurant/accounts/$TAKEAWAY/customer",
            takeawayBill(customer = "Juan Pérez", phone = "555"),
        )
        assertFalse(customers.sendPending())
        val sent = body(requests("POST restaurant/accounts/$TAKEAWAY/customer").single())
        assertEquals(edit.operationId, sent.text("operation_id"))
        assertEquals("Juan Pérez", sent.text("customer_name"))
        assertEquals("555", sent.text("phone"))
        assertFalse("sin revisión (A2-D1)", sent.containsKey("revision"))
        assertTrue(edits().isEmpty())
    }

    @Test
    fun sinConexionNaceAlEnviarseConDestinoParaLlevar() = runBlocking {
        // Ana entró sin conexión: solo tiene su credencial de envío.
        val capture = captures.openTakeaway(ANA, account = null)
        captures.setCustomer(capture.id, TakeawayCustomer("Juan", "555 1234"))
        captures.stage(
            capture.id,
            DraftOperation.Add(DraftLine("r1", line(BURGER, 2))),
            Money("120.00"),
        )
        awaitCapture(capture.id) { it.operations.size == 1 && it.sync == DraftSync.PENDING }
        assertTrue("nada sale a Go sin sesión", received.isEmpty())
        val queued = captures.sendToKitchen(capture.id, "Ana") as SendRequest.Queued
        assertEquals(OrderState.QUEUED, queued.state)
        assertEquals("la clave del pedido es la de la captura", capture.id, queued.operationId)
        val order = orderOf(capture.id)!!
        assertTrue(order.destination.takeaway)
        assertNull(order.destination.tableId)
        assertEquals(TakeawayCustomer("Juan", "555 1234"), order.destination.customer)

        respond("POST ${WaiterApi.OFFLINE_ORDERS_PATH}", accepted(TAKEAWAY))
        val run = sender.sendPending()
        assertEquals(1, run.sent)
        val request = requests("POST ${WaiterApi.OFFLINE_ORDERS_PATH}").single()
        assertEquals("credencial-de-ana", request.headers["X-Offline-Credential"])
        val destination = body(request)["destination"]!!.jsonObject
        assertEquals(setOf("takeaway"), destination.keys)
        val customer = destination["takeaway"]!!.jsonObject
        assertEquals("Juan", customer.text("customer_name"))
        assertEquals("555 1234", customer.text("phone"))
        assertEquals(capture.id, body(request).text("operation_id"))
        val received = orderOf(capture.id)!!
        assertEquals(OrderState.RECEIVED, received.state)
        assertEquals(TAKEAWAY, received.receivedAccountId)
    }

    /**
     * Las partidas ya están en un pedido para llevar abierto (`items_in_account`, la forma final de
     * la parte B): el mismo pedido, con la misma clave, a `destination.account_id`.
     */
    @Test
    fun itemsInAccountSeReenviaConLaMismaClaveALaCuenta() = runBlocking {
        val capture = captures.openTakeaway(ANA, account = null)
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        awaitCapture(capture.id) { it.operations.size == 1 }
        captures.sendToKitchen(capture.id, "Ana")
        respond(
            "POST ${WaiterApi.OFFLINE_ORDERS_PATH}",
            error(
                409,
                "ORDER_REQUIRES_CONFIRMATION",
                """{"catalog_version":41,"requires_changes":true,"changes":[""" +
                    """{"kind":"items_in_account","item_id":null,"product_id":null,""" +
                    """"product_name":"","before":null,"now":"folio 12",""" +
                    """"message":"Lo que capturaste ya está en el pedido para llevar """ +
                    """(folio 12). Envía el pedido a esa cuenta.","acceptable":false,""" +
                    """"account_id":"$TAKEAWAY"}]}""",
            ),
        )
        respond("POST ${WaiterApi.OFFLINE_ORDERS_PATH}", accepted(TAKEAWAY))
        sender.sendPending()
        val bodies = requests("POST ${WaiterApi.OFFLINE_ORDERS_PATH}").map(::body)
        assertEquals(2, bodies.size)
        assertEquals(bodies[0].text("operation_id"), bodies[1].text("operation_id"))
        assertTrue(bodies[0]["destination"]!!.jsonObject.containsKey("takeaway"))
        val second = bodies[1]["destination"]!!.jsonObject
        assertEquals(setOf("account_id"), second.keys)
        assertEquals(TAKEAWAY, second.text("account_id"))
        assertEquals(OrderState.RECEIVED, orderOf(capture.id)!!.state)
    }

    /**
     * A2-D1 sin conexión: la edición del nombre de un pedido que sigue en la cola se guarda, la
     * última gana, espera a que el pedido nazca y a que el mesero tenga sesión, y nunca se pierde.
     */
    @Test
    fun edicionesSinConexionLaUltimaGanaYNuncaSePierden() = runBlocking {
        val capture = captures.openTakeaway(ANA, account = null)
        captures.setCustomer(capture.id, TakeawayCustomer("Juan", ""))
        captures.stage(capture.id, DraftOperation.Add(DraftLine("r1", line(BURGER))), null)
        awaitCapture(capture.id) { it.operations.size == 1 }
        captures.sendToKitchen(capture.id, "Ana")
        takeaway.editCustomer(
            ANA,
            CustomerTarget.QueuedOrder(capture.id),
            TakeawayCustomer("A", ""),
        )
        takeaway.editCustomer(
            ANA,
            CustomerTarget.QueuedOrder(capture.id),
            TakeawayCustomer(" Juan Pérez ", "555"),
        )
        val pending = edits().single()
        assertEquals("la última gana", "Juan Pérez", pending.customerName)
        assertNull("el pedido todavía no nace en Go", pending.accountId)
        // El pedido sale con el nombre con que se envió (misma clave, mismo contenido).
        assertEquals("Juan", orderOf(capture.id)!!.destination.customer.name)
        assertFalse(customers.sendPending())
        assertTrue(requests("POST restaurant/accounts/$TAKEAWAY/customer").isEmpty())

        respond("POST ${WaiterApi.OFFLINE_ORDERS_PATH}", accepted(TAKEAWAY))
        sender.sendPending()
        assertEquals("ya sabe a qué pedido va", TAKEAWAY, edits().single().accountId)
        // Sin sesión de Ana (entró sin conexión) espera: no se pierde ni se envía con otra cosa.
        assertFalse(customers.sendPending())
        assertTrue(requests("POST restaurant/accounts/$TAKEAWAY/customer").isEmpty())
        assertEquals(1, edits().size)

        // Ana entra con conexión. Mientras sale su edición, edita otra vez: la respuesta de la
        // anterior no borra la nueva, que sale después.
        server.sessions[ANA] = "sesion-de-ana"
        respond("POST restaurant/accounts/$TAKEAWAY/customer") {
            runBlocking {
                takeaway.editCustomer(
                    ANA,
                    CustomerTarget.Account(TAKEAWAY),
                    TakeawayCustomer("Juan Pérez López", "555"),
                )
            }
            takeawayBill(customer = "Juan Pérez", phone = "555")
        }
        respond(
            "POST restaurant/accounts/$TAKEAWAY/customer",
            takeawayBill(customer = "Juan Pérez López", phone = "555"),
        )
        assertFalse(customers.sendPending())
        assertEquals("Juan Pérez López", edits().single().customerName)
        assertFalse(customers.sendPending())
        val sent = requests("POST restaurant/accounts/$TAKEAWAY/customer").map(::body)
        assertEquals(
            listOf("Juan Pérez", "Juan Pérez López"),
            sent.map { it.text("customer_name") },
        )
        assertNotEquals(sent[0].text("operation_id"), sent[1].text("operation_id"))
        assertTrue(edits().isEmpty())
    }

    @Test
    fun unaEdicionRechazadaParaSiempreNoSeQuedaYUnPedidoDescartadoTampoco() = runBlocking {
        server.sessions[ANA] = "sesion-de-ana"
        takeaway.editCustomer(ANA, CustomerTarget.Account(TAKEAWAY), TakeawayCustomer("Juan", ""))
        respond(
            "POST restaurant/accounts/$TAKEAWAY/customer",
            error(
                400,
                "OPERATION_VALIDATION",
                """{"reason":"account_not_open","account_id":"$TAKEAWAY",""" +
                    """"account_state":"closed","merged_into_id":null}""",
            ),
        )
        assertFalse(customers.sendPending())
        assertTrue("el pedido ya no está abierto: no hay a qué aplicarla", edits().isEmpty())
        // Sin respuesta, se queda para reintentar.
        takeaway.editCustomer(ANA, CustomerTarget.Account(TAKEAWAY), TakeawayCustomer("Luis", ""))
        respond("POST restaurant/accounts/$TAKEAWAY/customer", error(503, "SERVER_BUSY"))
        assertTrue("reintentar más tarde", customers.sendPending())
        assertEquals(1, edits().size)
    }

    @Test
    fun entregadoSoloComoLoDiceGo() = runBlocking {
        assertEquals(
            "sin sesión no va a la cola",
            DeliveryResult.NoConnection,
            takeaway.deliver(ANA, TAKEAWAY),
        )
        assertTrue(received.isEmpty())
        server.sessions[ANA] = "sesion-de-ana"
        val path = "POST payments/accounts/$TAKEAWAY/deliver"
        respond(path, json(200, """{"delivered":true,"closed":true,"finance":null}"""))
        assertEquals(DeliveryResult.Delivered(closed = true), takeaway.deliver(ANA, TAKEAWAY))
        assertTrue(body(requests(path).single()).containsKey("operation_id"))
        respond(path, error(409, "BALANCE_PENDING"))
        assertEquals(DeliveryResult.CollectFirst, takeaway.deliver(ANA, TAKEAWAY))
        respond(path, error(409, "ALREADY_DELIVERED"))
        assertEquals(DeliveryResult.AlreadyDelivered, takeaway.deliver(ANA, TAKEAWAY))
        respond(path, error(403, "PERMISSION_DENIED"))
        assertTrue(takeaway.deliver(ANA, TAKEAWAY) is DeliveryResult.Failed)
        // Sin respuesta, se repite igual (misma clave) una vez.
        respond(path) {
            MockResponse.Builder().onResponseStart(SocketEffect.ShutdownConnection).build()
        }
        respond(path, json(200, """{"delivered":true,"closed":false,"finance":null}"""))
        assertEquals(DeliveryResult.Delivered(closed = false), takeaway.deliver(ANA, TAKEAWAY))
        val last = requests(path).takeLast(2).map { body(it).text("operation_id") }
        assertEquals(last[0], last[1])
    }

    /** La lista sale de `restaurant/state` y, sin conexión, se ve lo último con su hora. */
    @Test
    fun listaConLoQueDiceGoYSinConexionLoUltimoConSuHora() = runBlocking {
        server.sessions[ANA] = "sesion-de-ana"
        respond(
            "GET restaurant/state",
            json(
                200,
                """{"catalog_version":41,"user":{"id":"$ANA","permissions":["takeaway.read"]},""" +
                    """"accounts":[{"id":"$TAKEAWAY","folio":12,"name":"Pedido para llevar",""" +
                    """"mode":"takeaway","state":"open","revision":4,"customer_name":"Juan",""" +
                    """"phone":"555","origin":"Para llevar","elapsed_seconds":300,""" +
                    """"preparation_state":"ready","payment_state":"paid",""" +
                    """"delivery_state":"not_delivered"}],"event_cursor":9}""",
            ),
        )
        respond("GET restaurant/overview", json(200, """{"areas":[],"tables":[]}"""))
        restaurant.start()
        server.active.value =
            ActiveWaiter(WaiterProfile(ANA, "Ana", "A", null, 1, null), hasServerSession = true)
        val accounts = withTimeout(5_000) { takeaway.accounts.first { it.isNotEmpty() } }
        val read = takeaway.readAt.value!!
        assertEquals(wall, read.wallMillis)
        val juan = accounts.single()
        assertEquals(OrderMode.TAKEAWAY, juan.mode)
        assertEquals(TakeawayCustomer("Juan", "555"), juan.customer)
        assertEquals(TakeawayPayment.PAID, TakeawayStates.payment(juan.paymentState))
        assertTrue(restaurant.openAccounts.value.isEmpty())

        // «Reinicio» sin conexión: lo guardado, con la hora de esa lectura.
        val readAt = wall
        wall += 10 * 60 * 1000
        server.sessions.clear()
        val offline =
            RestaurantRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                realtime = null,
                scope = scope,
                elapsedClock = { 80_000L },
                wallClock = { wall },
            )
        offline.start()
        val saved = withTimeout(5_000) { offline.allOpenAccounts.first { it.isNotEmpty() } }
        assertEquals(TakeawayCustomer("Juan", "555"), saved.single().customer)
        assertEquals(readAt, offline.accountsReadAt.value!!.wallMillis)
        assertEquals(80_000L - 10 * 60 * 1000, offline.accountsReadAt.value!!.elapsedMillis)
    }

    companion object {
        private const val TAKEAWAY = "77777777-7777-4777-8777-777777777777"
    }
}
