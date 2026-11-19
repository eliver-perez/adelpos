package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.access.WaiterWork
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.ACCOUNT
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.BURGER
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.SHAKE
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.draft
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Para llevar en el dominio (etapa A2, §3 y A2-D1/A2-D2): la correspondencia de los estados de Go,
 * cuándo se ofrece «Entregado», los límites del cliente, el pedido de la cola con el destino para
 * llevar, el trabajo pendiente del mesero y la lista.
 */
class TakeawayRulesTest {
    private val takeawayAccount = "77777777-7777-4777-8777-777777777777"

    private fun takeawayCapture(
        accountId: String? = null,
        operations: List<DraftOperation> = emptyList(),
        own: Set<String> = emptySet(),
        customer: TakeawayCustomer = TakeawayCustomer.NONE,
        id: String = "capture-1",
    ) =
        WaiterFixtures.capture(operations = operations, own = own, accountId = accountId)
            .copy(
                id = id,
                tableId = null,
                tableName = "",
                accountName = null,
                mode = OrderMode.TAKEAWAY,
                customer = customer,
            )

    private fun account(
        id: String,
        mode: OrderMode = OrderMode.TAKEAWAY,
        preparation: PreparationState? = PreparationState.RELEASED,
        payment: String? = "not_recorded",
        customer: TakeawayCustomer = TakeawayCustomer("Juan", ""),
    ) =
        AccountSummary(
            id = id,
            folio = 12,
            name = "Pedido para llevar",
            tableId = null,
            tableName = null,
            state = AccountSummary.STATE_OPEN,
            revision = 3,
            preparationState = preparation,
            confirmedQuantity = 1,
            draftQuantity = 0,
            totals = Totals.ZERO,
            assignedName = "Ana",
            elapsedSeconds = 300,
            longOpen = false,
            mode = mode,
            customer = customer,
            paymentState = payment,
            deliveryState = "not_delivered",
        )

    @Test
    fun preparacionComoLaListaDeLaEspecificacion() {
        val expected =
            mapOf(
                PreparationState.WITHOUT_ITEMS to TakeawayPreparation.NOT_SENT,
                PreparationState.DRAFT to TakeawayPreparation.NOT_SENT,
                PreparationState.CONFIRMED_PENDING_DISPATCH to TakeawayPreparation.PENDING,
                PreparationState.PENDING_RELEASE to TakeawayPreparation.PENDING,
                PreparationState.SCHEDULED to TakeawayPreparation.PENDING,
                PreparationState.RELEASED to TakeawayPreparation.PENDING,
                PreparationState.CHANGE_PENDING to TakeawayPreparation.PENDING,
                PreparationState.PREPARING to TakeawayPreparation.PREPARING,
                PreparationState.READY to TakeawayPreparation.READY,
                PreparationState.CANCELLED to TakeawayPreparation.CANCELLED,
            )
        // Todos los estados de Go tienen su correspondencia (uno nuevo obliga a decidirla).
        assertEquals(PreparationState.entries.toSet(), expected.keys)
        for ((state, preparation) in expected) {
            assertEquals(state.name, preparation, TakeawayStates.preparation(state))
        }
        // Un estado que esta versión no conoce no se inventa.
        assertEquals(TakeawayPreparation.UNKNOWN, TakeawayStates.preparation(null))
    }

    @Test
    fun pagoYEntregaComoLosDaGo() {
        assertEquals(TakeawayPayment.UNPAID, TakeawayStates.payment("not_recorded"))
        assertEquals(TakeawayPayment.PARTIAL, TakeawayStates.payment("partial"))
        assertEquals(TakeawayPayment.PAID, TakeawayStates.payment("paid"))
        assertEquals(TakeawayPayment.UNKNOWN, TakeawayStates.payment("otro"))
        assertEquals(TakeawayPayment.UNKNOWN, TakeawayStates.payment(null))
        assertEquals(TakeawayDelivery.NOT_DELIVERED, TakeawayStates.delivery("not_delivered"))
        assertEquals(TakeawayDelivery.DELIVERED, TakeawayStates.delivery("delivered"))
        assertEquals(TakeawayDelivery.UNKNOWN, TakeawayStates.delivery(null))
    }

    /** A2-D2: «Entregado» solo con el permiso **y** el pedido pagado según Go. */
    @Test
    fun entregadoSoloPagadoYConPermiso() {
        val withDeliver =
            WaiterPermissions(setOf("takeaway.read", "takeaway.manage", "takeaway.deliver"))
        val withoutDeliver = WaiterPermissions(setOf("takeaway.read", "takeaway.manage"))
        val notDelivered = TakeawayDelivery.NOT_DELIVERED
        assertEquals(
            DeliveryOffer.DELIVER,
            TakeawayStates.deliveryOffer(withDeliver, TakeawayPayment.PAID, notDelivered),
        )
        for (payment in listOf(TakeawayPayment.UNPAID, TakeawayPayment.PARTIAL)) {
            assertEquals(
                "sin pagar: «Cobrar en caja antes de entregar»",
                DeliveryOffer.COLLECT_FIRST,
                TakeawayStates.deliveryOffer(withDeliver, payment, notDelivered),
            )
        }
        assertEquals(
            DeliveryOffer.COLLECT_FIRST,
            TakeawayStates.deliveryOffer(withDeliver, TakeawayPayment.UNKNOWN, notDelivered),
        )
        assertEquals(
            "sin el permiso, solo el estado",
            DeliveryOffer.STATUS_ONLY,
            TakeawayStates.deliveryOffer(withoutDeliver, TakeawayPayment.PAID, notDelivered),
        )
        assertEquals(
            "con permisos desconocidos no se ofrece",
            DeliveryOffer.STATUS_ONLY,
            TakeawayStates.deliveryOffer(
                WaiterPermissions.UNKNOWN,
                TakeawayPayment.PAID,
                notDelivered,
            ),
        )
        assertEquals(
            DeliveryOffer.DELIVERED,
            TakeawayStates.deliveryOffer(
                withDeliver,
                TakeawayPayment.PAID,
                TakeawayDelivery.DELIVERED,
            ),
        )
    }

    /** §3.1: «Para llevar» solo aparece con `takeaway.read` de Go; capturar exige manage. */
    @Test
    fun permisosDeParaLlevar() {
        val readOnly = WaiterPermissions(setOf("takeaway.read", "accounts.open", "orders.capture"))
        assertTrue(readOnly.grants(WaiterAction.VIEW_TAKEAWAY))
        assertFalse(readOnly.grants(WaiterAction.MANAGE_TAKEAWAY))
        assertFalse(readOnly.grants(WaiterAction.OPEN_TAKEAWAY))
        val manager = WaiterPermissions(readOnly.keys!! + "takeaway.manage")
        assertTrue(manager.grants(WaiterAction.OPEN_TAKEAWAY))
        assertFalse(manager.grants(WaiterAction.DELIVER_TAKEAWAY))
        // Sin permisos conocidos, «allows» no deshabilita nada (A1) pero «grants» no muestra.
        assertTrue(WaiterPermissions.UNKNOWN.allows(WaiterAction.VIEW_TAKEAWAY))
        assertFalse(WaiterPermissions.UNKNOWN.grants(WaiterAction.VIEW_TAKEAWAY))
        assertFalse(WaiterPermissions(emptySet()).grants(WaiterAction.VIEW_TAKEAWAY))
    }

    /** A2-D1: los límites de Go se cuentan en bytes del texto recortado (100 y 40). */
    @Test
    fun limitesDelClienteComoGo() {
        assertTrue(TakeawayCustomer.fitsName("a".repeat(100)))
        assertFalse(TakeawayCustomer.fitsName("a".repeat(101)))
        assertTrue("se recorta", TakeawayCustomer.fitsName("  " + "a".repeat(100) + "  "))
        // «ñ» ocupa dos bytes: 50 caben, 51 no.
        assertTrue(TakeawayCustomer.fitsName("ñ".repeat(50)))
        assertFalse(TakeawayCustomer.fitsName("ñ".repeat(51)))
        assertTrue(TakeawayCustomer.fitsPhone("5".repeat(40)))
        assertFalse(TakeawayCustomer.fitsPhone("5".repeat(41)))
        assertTrue("sin formato", TakeawayCustomer.fitsPhone("(55) 1234-5678 ext. 2"))
        assertEquals(
            TakeawayCustomer("Juan", "555"),
            TakeawayCustomer("  Juan ", " 555 ").trimmed(),
        )
        assertTrue(TakeawayCustomer(" ", "").isEmpty)
    }

    /** La cola con el destino para llevar: sin mesa, con el cliente con que sale. */
    @Test
    fun pedidoParaLlevarDeLaCola() {
        val capture =
            takeawayCapture(
                operations = listOf(DraftOperation.Add(draft("n1", BURGER, 2))),
                own = setOf("n1"),
                customer = TakeawayCustomer(" Juan ", "555 1234"),
            )
        assertTrue(capture.customerEditable)
        val order = WaiterFixtures.order(capture)
        assertTrue(order.destination.takeaway)
        assertNull(order.destination.tableId)
        assertNull(order.destination.accountId)
        assertEquals(TakeawayCustomer("Juan", "555 1234"), order.destination.customer)
        assertEquals(emptyList<DraftLine>(), order.replaces)
        assertEquals(listOf("n1"), order.lines.map { it.item.id })

        // `items_in_account` (Go ya tiene las partidas en un pedido para llevar abierto): el
        // mismo pedido, con la misma clave, a esa cuenta; sigue siendo para llevar.
        val sending = OrderSendRules.startSending(order, 3_000L)
        val redirected =
            OrderSendRules.after(sending, SendOutcome.Redirect(takeawayAccount), 4_000L)
        assertEquals(OrderState.QUEUED, redirected.state)
        assertEquals(order.operationId, redirected.operationId)
        assertEquals(takeawayAccount, redirected.destination.accountId)
        assertTrue(redirected.destination.takeaway)
        assertEquals(order.lines, redirected.lines)

        // El pedido para llevar ya no está abierto: «pedido nuevo para llevar» con renglones
        // nuevos y el mismo cliente.
        val rejected =
            OrderSendRules.after(
                OrderSendRules.startSending(redirected, 5_000L),
                SendOutcome.AccountNotOpen("El pedido ya se cerró."),
                6_000L,
            )
        assertTrue(OrderReviewRules.offersNewAccount(requireNotNull(rejected.review)))
        val moved = OrderReviewRules.moveToNewAccount(rejected, 7_000L) { "nuevo" }
        assertNull(moved.destination.accountId)
        assertTrue(moved.destination.takeaway)
        assertEquals(order.destination.customer, moved.destination.customer)
        assertEquals(listOf("nuevo"), moved.lines.map { it.item.id })
    }

    /** Un guardado sin respuesta de un pedido para llevar se repite igual: sin mesa, su cliente. */
    @Test
    fun aperturaSinRespuestaDeUnPedidoParaLlevar() {
        val capture =
            takeawayCapture(
                    operations = listOf(DraftOperation.Add(draft("n1", BURGER))),
                    own = setOf("n1"),
                    customer = TakeawayCustomer("Juan", ""),
                )
                .copy(pendingOperationId = "op-open", inFlightCount = 1)
        assertFalse(
            "mientras sale, el cliente ya no cambia en la captura",
            capture.customerEditable,
        )
        val unresolved = requireNotNull(capture.unresolvedSave())
        assertEquals(OrderMode.TAKEAWAY, unresolved.mode)
        assertNull(unresolved.tableId)
        assertNull(unresolved.accountId)
        assertEquals(TakeawayCustomer("Juan", ""), unresolved.customer)
    }

    /**
     * Las capturas para llevar cuentan para el trabajo pendiente del mesero (bloqueo por
     * inactividad, cambiar de mesero y cerrar sesión), igual que las de mesa.
     */
    @Test
    fun trabajoPendienteConParaLlevar() {
        val empty = takeawayCapture()
        assertEquals(WaiterWork(false, 0), WaiterWorkSummary.of(listOf(empty), emptyList()))
        // Solo el nombre y el teléfono, sin productos, no es trabajo pendiente: salir no deja nada.
        assertFalse(empty.copy(customer = TakeawayCustomer("Juan", "")).inProgress)
        val inProgress =
            takeawayCapture(
                operations = listOf(DraftOperation.Add(draft("n1", BURGER))),
                own = setOf("n1"),
            )
        val work = WaiterWorkSummary.of(listOf(inProgress), emptyList())
        assertTrue(work.captureInProgress)
        assertTrue(work.blocksIdleLock)
        assertTrue(work.blocksLogout)
        assertFalse("una captura en curso no impide cambiar de mesero", work.blocksSwitch)
        val queued = WaiterFixtures.order(inProgress)
        val withOrder = WaiterWorkSummary.of(emptyList(), listOf(queued))
        assertEquals(1, withOrder.unsentOrders)
        assertTrue(withOrder.blocksSwitch)
        assertTrue(withOrder.blocksIdleLock)
    }

    @Test
    fun listaDeParaLlevar() {
        val inGo = account(takeawayAccount)
        val dineIn = account(ACCOUNT, mode = OrderMode.DINE_IN)
        val queuedNew =
            WaiterFixtures.order(
                takeawayCapture(
                    id = "op-nuevo",
                    operations = listOf(DraftOperation.Add(draft("n1", BURGER))),
                    own = setOf("n1"),
                    customer = TakeawayCustomer("Luisa", ""),
                ),
                operationId = "op-nuevo",
            )
        val queuedToGo =
            WaiterFixtures.order(
                takeawayCapture(
                    accountId = takeawayAccount,
                    operations = listOf(DraftOperation.Add(draft("n2", SHAKE))),
                    own = setOf("n2"),
                ),
                operationId = "op-a-go",
            )
        val received =
            WaiterFixtures.order(
                takeawayCapture(
                    operations = listOf(DraftOperation.Add(draft("n3", BURGER))),
                    own = setOf("n3"),
                ),
                state = OrderState.RECEIVED,
                operationId = "op-recibido",
            )
        val draftCapture =
            takeawayCapture(
                id = "captura-sin-conexion",
                operations = listOf(DraftOperation.Add(draft("n4", BURGER))),
                own = setOf("n4"),
                customer = TakeawayCustomer("Pedro", ""),
            )
        val emptyCapture = takeawayCapture(id = "captura-vacia")
        val captureInGo =
            takeawayCapture(
                id = "captura-en-go",
                accountId = takeawayAccount,
                operations = listOf(DraftOperation.Add(draft("n5", SHAKE))),
                own = setOf("n5"),
            )
        val edits =
            listOf(
                CustomerEdit(
                    key = takeawayAccount,
                    accountId = takeawayAccount,
                    waiterUserId = WaiterFixtures.ANA,
                    customer = TakeawayCustomer("Juan Pérez", "555"),
                    operationId = "e1",
                    updatedAtMillis = 1L,
                ),
                CustomerEdit(
                    key = "op-nuevo",
                    accountId = null,
                    waiterUserId = WaiterFixtures.ANA,
                    customer = TakeawayCustomer("Luisa M.", ""),
                    operationId = "e2",
                    updatedAtMillis = 2L,
                ),
            )
        val entries =
            TakeawayBoard.entries(
                accounts = listOf(dineIn, inGo),
                orders = listOf(queuedNew, queuedToGo, received),
                captures = listOf(draftCapture, emptyCapture, captureInGo),
                edits = edits,
            )
        // Primero lo que todavía no nace en Go, después los de Go (sin los de salón).
        assertEquals(
            listOf("op-nuevo", "captura-sin-conexion", takeawayAccount),
            entries.map(TakeawayEntry::key),
        )
        val queued = entries[0] as TakeawayEntry.Queued
        assertEquals("la última edición gana", TakeawayCustomer("Luisa M.", ""), queued.customer)
        assertTrue(queued.customerPending)
        val draftEntry = entries[1] as TakeawayEntry.Draft
        assertEquals(TakeawayCustomer("Pedro", ""), draftEntry.customer)
        val go = entries[2] as TakeawayEntry.InGo
        assertEquals(TakeawayCustomer("Juan Pérez", "555"), go.customer)
        assertTrue(go.customerPending)
        assertEquals("captura-en-go", go.captureId)
        assertEquals("op-a-go", go.queued?.operationId)
        assertEquals(TakeawayPreparation.PENDING, go.preparation)
        assertEquals(TakeawayPayment.UNPAID, go.payment)

        // Si el pedido de la cola va a un pedido que ya no está en la lista, se sigue viendo.
        val alone =
            TakeawayBoard.entries(
                accounts = emptyList(),
                orders = listOf(queuedToGo),
                captures = emptyList(),
                edits = emptyList(),
            )
        assertEquals(listOf("op-a-go"), alone.map(TakeawayEntry::key))
        assertFalse(alone.single().customerPending)
    }

    /**
     * «Enviar a cocina» sin conexión pasa la captura a la cola en una transacción, pero la lista
     * puede leer la cola nueva antes de que se vaya la captura: nunca dos renglones con la misma
     * clave (Compose cerraría la aplicación).
     */
    @Test
    fun laCapturaYSuPedidoDeLaColaNuncaSeVenDosVeces() {
        val capture =
            takeawayCapture(
                id = "op-1",
                operations = listOf(DraftOperation.Add(draft("n1", BURGER))),
                own = setOf("n1"),
            )
        val queued = WaiterFixtures.order(capture, operationId = "op-1")
        val entries =
            TakeawayBoard.entries(
                accounts = emptyList(),
                orders = listOf(queued),
                captures = listOf(capture),
                edits = emptyList(),
            )
        assertEquals(listOf("op-1"), entries.map(TakeawayEntry::key))
        assertTrue(entries.single() is TakeawayEntry.Queued)
        // Ya recibido (o «Corregir en la captura» a medias): tampoco se repite la clave.
        val received = queued.copy(state = OrderState.RECEIVED)
        val keys =
            TakeawayBoard.entries(
                    accounts = emptyList(),
                    orders = listOf(received),
                    captures = listOf(capture),
                    edits = emptyList(),
                )
                .map(TakeawayEntry::key)
        assertEquals(keys.toSet().size, keys.size)
    }
}
