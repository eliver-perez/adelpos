package com.adelpos.meseros.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.ListenableWorker
import androidx.work.testing.TestListenableWorkerBuilder
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.BURGER
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.accepted
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.error
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.queuedOrder
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa L1 (§8): un pedido de la cola que Go rechaza por la licencia, un cupo o la hora de la
 * computadora principal **no se pierde ni se reintenta sin fin** (Room con Robolectric y un Go
 * simulado): queda en «Requiere tu revisión» con su motivo y el texto de su código, el trabajador
 * no pide reintentos, y el mesero lo envía de nuevo con **la misma clave** cuando se resuelve.
 */
@RunWith(AndroidJUnit4::class)
class LicenseQueueTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var queue: OrderQueueRepository
    private lateinit var sender: OrderSender
    private val sendRequests = AtomicInteger(0)

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        server.credentials[ANA] = "credencial-de-ana"
        queue =
            OrderQueueRepository(
                dao = database.waiter(),
                scheduler = { sendRequests.incrementAndGet() },
                describe = { selection ->
                    if (selection.productId == BURGER) "Hamburguesa" else null
                },
                clock = { 1_700_000_100_000L },
            )
        sender = OrderSender(database.waiter(), server, WaiterApi())
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

    private suspend fun orderOf(operationId: String): PendingOrder =
        queue.order(operationId).first()!!

    private fun operationIdOf(body: String): String =
        Json.parseToJsonElement(body).jsonObject["operation_id"]!!.jsonPrimitive.content

    @Test
    fun sinCupoQuedaEnRevisionYNoSeReintentaSolo() = runBlocking {
        insert(queuedOrder("op-1"))
        mock.enqueue(
            error(
                409,
                "LICENSE_SEAT_UNAVAILABLE",
                """{"seat":"order_takers","in_use":0,"limit":0}""",
            ),
        )
        val run = sender.sendPending()
        assertEquals(SendRun(sent = 0, reviews = 1, retryLater = false), run)
        val order = orderOf("op-1")
        assertEquals(OrderState.NEEDS_REVIEW, order.state)
        assertEquals(ReviewReason.NOT_ALLOWED, order.review!!.reason)
        assertEquals(
            "La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal " +
                "con el turno de caja abierto.",
            order.review!!.message,
        )
        // Nunca el mensaje de Go.
        assertNotEquals("Mensaje de Go", order.review!!.message)
        // Lo ve el mesero y sigue contando como sin enviar; no se pierde.
        assertEquals(1, queue.pendingCount().first())
        assertEquals(1, queue.unsentOrders().size)

        // Ni otra vuelta ni el trabajador lo vuelven a mandar, y el trabajador no pide reintentos.
        assertEquals(SendRun(sent = 0, reviews = 0, retryLater = false), sender.sendPending())
        val factory = OrderWorkerFactory({ sender }, OrderSendSignals())
        val worker =
            TestListenableWorkerBuilder<OrderSendWorker>(context).setWorkerFactory(factory).build()
        assertEquals(ListenableWorker.Result.success(), worker.doWork())
        assertEquals(1, mock.requestCount)
        assertEquals(OrderState.NEEDS_REVIEW, orderOf("op-1").state)

        // Volvió el cupo: el mesero lo envía de nuevo con la misma clave y entra.
        queue.resend("op-1")
        assertEquals(OrderState.QUEUED, orderOf("op-1").state)
        mock.enqueue(accepted())
        assertEquals(1, sender.sendPending().sent)
        assertEquals(OrderState.RECEIVED, orderOf("op-1").state)
        val first = mock.takeRequest()
        val second = mock.takeRequest()
        assertEquals("op-1", operationIdOf(first.body!!.utf8()))
        assertEquals("op-1", operationIdOf(second.body!!.utf8()))
        assertFalse(
            "se reenvía sin confirmación",
            Json.parseToJsonElement(second.body!!.utf8()).jsonObject.containsKey("confirmation"),
        )
    }

    @Test
    fun horaIncorrectaYLicenciaVencidaConSuTexto() = runBlocking {
        insert(queuedOrder("op-hora"))
        insert(queuedOrder("op-licencia"))
        mock.enqueue(
            error(
                409,
                "SYSTEM_CLOCK_INVALID",
                """{"blocked":true,"local_time_text":"lunes 23 nov 2026, 10:15",""" +
                    """"last_activity_time_text":"lunes 23 nov 2026, 12:40","license_ok":true}""",
            ),
        )
        mock.enqueue(error(409, "LICENSE_EXPIRED", """{"operation":"capture"}"""))
        val run = sender.sendPending()
        // Un rechazo no detiene la vuelta: el siguiente también sale (y queda con su motivo).
        assertEquals(SendRun(sent = 0, reviews = 2, retryLater = false), run)
        val clock = orderOf("op-hora")
        assertEquals(ReviewReason.NOT_ALLOWED, clock.review!!.reason)
        assertEquals(LicenseMessages.CLOCK_INVALID, clock.review!!.message)
        val license = orderOf("op-licencia")
        assertEquals(ReviewReason.NOT_ALLOWED, license.review!!.reason)
        assertEquals(
            "La licencia del sistema no permite capturar pedidos. Avisa al encargado.",
            license.review!!.message,
        )
        // El motivo sobrevive a reabrir la base (va en el JSON de la revisión, sin cambiar el
        // esquema de Room).
        database.close()
        database = AppDatabase.open(context)
        val reopened = WaiterStorage.toOrder(database.waiter().order("op-hora")!!)
        assertEquals(ReviewReason.NOT_ALLOWED, reopened.review!!.reason)
        assertEquals(LicenseMessages.CLOCK_INVALID, reopened.review!!.message)
    }
}
