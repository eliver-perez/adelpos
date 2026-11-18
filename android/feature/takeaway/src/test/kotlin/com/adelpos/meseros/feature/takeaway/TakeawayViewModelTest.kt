package com.adelpos.meseros.feature.takeaway

import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.CustomerEdit
import com.adelpos.meseros.core.domain.waiter.CustomerTarget
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PrecheckService
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.SentLineService
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayEntry
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.TakeawayService
import com.adelpos.meseros.core.domain.waiter.Totals
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import java.lang.reflect.Proxy
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * La lista de Para llevar (§3.3) en su ViewModel, con el modo Mesero falso (solo lo que usa):
 * «Nuevo pedido» directo al catálogo, abrir un pedido según los permisos, «Nombre y teléfono» según
 * dónde está el pedido y los avisos de «Entregado» (A2-D2).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TakeawayViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun main() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun reset() {
        Dispatchers.resetMain()
    }

    private val all =
        WaiterPermissions(
            setOf(
                "takeaway.read",
                "takeaway.manage",
                "takeaway.deliver",
                "accounts.open",
                "orders.capture",
            ),
        )

    private class Fake {
        val permissions = MutableStateFlow(WaiterPermissions.UNKNOWN)
        val accounts = MutableStateFlow<List<AccountSummary>>(emptyList())
        val orders = MutableStateFlow<List<PendingOrder>>(emptyList())
        val captures = MutableStateFlow<List<Capture>>(emptyList())
        val edits = MutableStateFlow<List<CustomerEdit>>(emptyList())
        val opened = mutableListOf<AccountSummary?>()
        val customerEdits = mutableListOf<Pair<CustomerTarget, TakeawayCustomer>>()
        val captureCustomers = mutableListOf<Pair<String, TakeawayCustomer>>()
        var delivery: DeliveryResult = DeliveryResult.Delivered(closed = true)
        val delivered = mutableListOf<String>()
        var refreshes = 0

        val restaurant =
            object : RestaurantSource by unused() {
                override val permissions: StateFlow<WaiterPermissions> = this@Fake.permissions
                override val status: StateFlow<RestaurantStatus> =
                    MutableStateFlow(RestaurantStatus(loading = false, live = true, null))
            }

        val captureService =
            object : CaptureService by unused() {
                override fun captures(userId: String): Flow<List<Capture>> =
                    this@Fake.captures.map { list -> list.filter { it.waiterUserId == userId } }

                override suspend fun openTakeaway(userId: String, account: AccountSummary?) =
                    takeawayCapture("captura-${opened.size + 1}", account?.id).also {
                        opened.add(account)
                    }

                override suspend fun setCustomer(captureId: String, customer: TakeawayCustomer) {
                    captureCustomers.add(captureId to customer)
                }
            }

        val queue =
            object : OrderQueueService by unused() {
                override fun orders(userId: String): Flow<List<PendingOrder>> =
                    this@Fake.orders.map { list -> list.filter { it.waiterUserId == userId } }
            }

        val takeaway =
            object : TakeawayService {
                override val accounts: StateFlow<List<AccountSummary>> = this@Fake.accounts
                override val readAt: StateFlow<TakeawayRead?> =
                    MutableStateFlow(TakeawayRead(1L, 1L))

                override fun refresh() {
                    refreshes++
                }

                override fun customerEdits(): Flow<List<CustomerEdit>> = edits

                override suspend fun editCustomer(
                    userId: String,
                    target: CustomerTarget,
                    customer: TakeawayCustomer,
                ) {
                    customerEdits.add(target to customer)
                }

                override suspend fun deliver(userId: String, accountId: String): DeliveryResult {
                    delivered.add(accountId)
                    return delivery
                }
            }

        fun dependencies() =
            TakeawayDependencies(
                capture =
                    CaptureDependencies(
                        restaurant = restaurant,
                        captures = captureService,
                        orders = queue,
                        sentLines = unused<SentLineService>(),
                        catalog = unused<CatalogSource>(),
                        images = null,
                        activeWaiter =
                            MutableStateFlow(
                                ActiveWaiter(
                                    WaiterProfile(ANA, "Ana", "A", null, 1, null),
                                    hasServerSession = true,
                                ),
                            ),
                        takeaway = takeaway,
                        precheck = unused<PrecheckService>(),
                    ),
                elapsedClock = { 10_000L },
                wallClock = { 1_700_000_000_000L },
            )
    }

    @Test
    fun laListaJuntaLoDeGoYLoDelDispositivo() =
        runTest(dispatcher) {
            val fake = Fake()
            fake.accounts.value =
                listOf(account(TAKEAWAY), account("salon", mode = OrderMode.DINE_IN))
            fake.orders.value = listOf(order("op-1"), order("op-beto", userId = "beto"))
            val model = TakeawayViewModel(fake.dependencies())
            advanceUntilIdle()
            // Lo de Beto no se muestra a Ana (solo se cuenta en el indicador).
            assertEquals(listOf("op-1", TAKEAWAY), model.entries.value.map(TakeawayEntry::key))
            assertEquals("se lee al abrir la lista", 1, fake.refreshes)
        }

    @Test
    fun nuevoPedidoEntraDirectoAlCatalogoSoloConPermiso() =
        runTest(dispatcher) {
            val fake = Fake()
            fake.permissions.value = WaiterPermissions(setOf("takeaway.read"))
            val model = TakeawayViewModel(fake.dependencies())
            val opened = mutableListOf<String>()
            model.newOrder { opened.add(it) }
            advanceUntilIdle()
            assertTrue("sin takeaway.manage solo se ve la lista", opened.isEmpty())
            fake.permissions.value = all
            model.newOrder { opened.add(it) }
            advanceUntilIdle()
            assertEquals(listOf("captura-1"), opened)
            assertEquals(listOf<AccountSummary?>(null), fake.opened)
        }

    @Test
    fun abrirUnPedidoSegunDondeEsta() =
        runTest(dispatcher) {
            val fake = Fake()
            fake.permissions.value = all
            val inGo = account(TAKEAWAY)
            fake.accounts.value = listOf(inGo)
            fake.orders.value = listOf(order("op-revisar", state = OrderState.NEEDS_REVIEW))
            val model = TakeawayViewModel(fake.dependencies())
            advanceUntilIdle()
            val captures = mutableListOf<String>()
            val reviews = mutableListOf<String>()
            for (entry in model.entries.value) {
                model.open(entry, { captures.add(it) }, { reviews.add(it) })
            }
            advanceUntilIdle()
            assertEquals(listOf("op-revisar"), reviews)
            assertEquals(listOf("captura-1"), captures)
            assertEquals(listOf<AccountSummary?>(inGo), fake.opened)

            // Con una captura sin enviar en ese pedido, se retoma esa (no se abre otra).
            fake.captures.value = listOf(inProgress("en-curso", TAKEAWAY))
            advanceUntilIdle()
            val resumed = model.entries.value.first { it.key == TAKEAWAY }
            model.open(resumed, { captures.add(it) }, { reviews.add(it) })
            assertEquals("en-curso", captures.last())
            assertEquals(1, fake.opened.size)

            // Sin takeaway.manage no se captura: solo la lista.
            fake.permissions.value = WaiterPermissions(setOf("takeaway.read"))
            model.open(resumed, { captures.add(it) }, { reviews.add(it) })
            assertEquals(2, captures.size)
        }

    @Test
    fun nombreYTelefonoSegunDondeEstaElPedido() =
        runTest(dispatcher) {
            val fake = Fake()
            fake.permissions.value = all
            fake.accounts.value = listOf(account(TAKEAWAY))
            fake.orders.value = listOf(order("op-1"))
            fake.captures.value = listOf(inProgress("borrador", accountId = null))
            val model = TakeawayViewModel(fake.dependencies())
            advanceUntilIdle()
            val juan = TakeawayCustomer("Juan", "555")
            for (entry in model.entries.value) {
                model.editCustomer(entry, juan)
            }
            advanceUntilIdle()
            assertEquals(
                listOf(
                    CustomerTarget.QueuedOrder("op-1") to juan,
                    CustomerTarget.Account(TAKEAWAY) to juan,
                ),
                fake.customerEdits,
            )
            // Una captura que todavía no sale a Go lo guarda en la captura misma.
            assertEquals(listOf("borrador" to juan), fake.captureCustomers)
        }

    @Test
    fun avisosDeEntregado() =
        runTest(dispatcher) {
            val fake = Fake()
            fake.permissions.value = all
            fake.accounts.value = listOf(account(TAKEAWAY, payment = "paid"))
            val model = TakeawayViewModel(fake.dependencies())
            advanceUntilIdle()
            val entry = model.entries.value.single() as TakeawayEntry.InGo
            val expected =
                mapOf(
                    DeliveryResult.Delivered(closed = true) to TakeawayNotice.DELIVERED,
                    DeliveryResult.CollectFirst to TakeawayNotice.COLLECT_FIRST,
                    DeliveryResult.AlreadyDelivered to TakeawayNotice.ALREADY_DELIVERED,
                    DeliveryResult.NoConnection to TakeawayNotice.NO_CONNECTION,
                    DeliveryResult.Failed("Mensaje de Go") to TakeawayNotice.FAILED,
                )
            for ((result, notice) in expected) {
                fake.delivery = result
                model.deliver(entry)
                advanceUntilIdle()
                assertEquals(notice, model.notice)
                assertNull(model.delivering)
                if (result is DeliveryResult.Failed) {
                    assertEquals("Mensaje de Go", model.noticeMessage)
                }
                model.onNoticeShown()
            }
            assertEquals(List(expected.size) { TAKEAWAY }, fake.delivered)
        }

    companion object {
        private const val ANA = "ana"
        private const val TAKEAWAY = "77777777-7777-4777-8777-777777777777"
        private const val BURGER = "hamburguesa"

        /** Una interfaz sin implementar: falla si la prueba usa algo que no preparó. */
        inline fun <reified T> unused(): T =
            Proxy.newProxyInstance(T::class.java.classLoader, arrayOf(T::class.java)) { _, method, _
                ->
                error("La prueba no prepara ${method.name}")
            } as T

        fun takeawayCapture(id: String, accountId: String?, userId: String = ANA) =
            Capture(
                id = id,
                waiterUserId = userId,
                tableId = null,
                tableName = "",
                accountId = accountId,
                accountName = null,
                seenAccountIds = emptyList(),
                base = emptyList(),
                baseRevision = 0,
                operations = emptyList(),
                ownItemIds = emptySet(),
                seenPrices = emptyMap(),
                sync = DraftSync.SAVED,
                syncMessage = null,
                tableAccounts = emptyList(),
                pendingOperationId = null,
                inFlightCount = 0,
                createdAtMillis = 1L,
                mode = OrderMode.TAKEAWAY,
            )

        fun inProgress(id: String, accountId: String?, userId: String = ANA): Capture {
            val line = DraftLine("$id-r1", LineSelection(BURGER, null, 1, Customization.ORIGINAL))
            return takeawayCapture(id, accountId, userId)
                .copy(operations = listOf(DraftOperation.Add(line)), ownItemIds = setOf(line.id))
        }

        fun order(
            operationId: String,
            userId: String = ANA,
            state: OrderState = OrderState.QUEUED,
        ): PendingOrder =
            requireNotNull(
                    OrderFactory.fromCapture(
                        capture = inProgress(operationId, accountId = null, userId = userId),
                        waiterName = "Ana",
                        operationId = operationId,
                        catalog = OrderFactory.CatalogStamp(41, null),
                        lookup = { OrderFactory.Described("Hamburguesa", Money("120.00")) },
                        onlineConfirm = null,
                        nowMillis = 2L,
                    ),
                )
                .copy(state = state)

        fun account(
            id: String,
            mode: OrderMode = OrderMode.TAKEAWAY,
            payment: String = "not_recorded",
        ) =
            AccountSummary(
                id = id,
                folio = 12,
                name = "Pedido para llevar",
                tableId = null,
                tableName = null,
                state = AccountSummary.STATE_OPEN,
                revision = 2,
                preparationState = PreparationState.READY,
                confirmedQuantity = 1,
                draftQuantity = 0,
                totals = Totals.ZERO,
                assignedName = "Ana",
                elapsedSeconds = 120,
                longOpen = false,
                mode = mode,
                customer = TakeawayCustomer("Juan", ""),
                paymentState = payment,
                deliveryState = "not_delivered",
            )
    }
}
