package local.adel.pos.android.core.data.waiter

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import local.adel.pos.android.core.data.waiter.db.WaiterDao
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.access.UnsentOrdersSource
import local.adel.pos.android.core.domain.access.WaiterWork
import local.adel.pos.android.core.domain.access.WaiterWorkGate
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.connection.PendingOrdersSource
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.DraftSync
import local.adel.pos.android.core.domain.waiter.OrderQueueService
import local.adel.pos.android.core.domain.waiter.OrderReviewRules
import local.adel.pos.android.core.domain.waiter.OrderSendRules
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.OrderTransitions
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.WaiterWorkSummary

/**
 * Quien envía la cola (WorkManager en la aplicación; uno falso en las pruebas): «envía lo que falta
 * en cuanto se pueda».
 */
fun interface OrderSendScheduler {
    fun requestSend()
}

/**
 * La cola de pedidos (§7) sobre las tablas del modo Mesero de `AppDatabase`: cada cambio de estado
 * es una escritura en una transacción con las transiciones del dominio ([OrderTransitions]); nada
 * se borra por cerrar la aplicación, reiniciar o perder la batería. También es:
 * - el contador del indicador de conexión ([PendingOrdersSource], todo el dispositivo);
 * - el trabajo pendiente de cada mesero ([WaiterWorkGate]: capturas en curso y pedidos sin enviar);
 * - lo que se muestra antes de borrar al retirar el dispositivo o emparejarlo de nuevo
 *   ([UnsentOrdersSource]).
 *
 * @param describe nombre de un producto del catálogo guardado, para describir lo que no se envió.
 */
class OrderQueueRepository(
    private val dao: WaiterDao,
    private val scheduler: OrderSendScheduler,
    private val describe: (LineSelection) -> String? = { null },
    private val clock: () -> Long = System::currentTimeMillis,
) : OrderQueueService, UnsentOrdersSource, WaiterWorkGate, PendingOrdersSource {
    private val mutex = Mutex()

    override fun orders(userId: String): Flow<List<PendingOrder>> =
        dao.observeOrders(userId).map { rows -> rows.map(WaiterStorage::toOrder) }

    override fun order(operationId: String): Flow<PendingOrder?> =
        dao.observeOrder(operationId).map { row -> row?.let(WaiterStorage::toOrder) }

    override fun pendingCount(): Flow<Int> = dao.observeCount(UNSENT_STATES)

    /** Pedidos sin enviar de un mesero (para avisar en su pantalla de pendientes). */
    fun pendingCountOf(userId: String): Flow<Int> = dao.observeCountOf(userId, UNSENT_STATES)

    override fun retryNow() {
        scheduler.requestSend()
    }

    override suspend fun accept(operationId: String) {
        updateAndSend(operationId) { order -> OrderReviewRules.accept(order, clock()) }
    }

    override suspend fun removeLine(operationId: String, itemId: String) {
        mutex.withLock {
            val order = load(operationId) ?: return
            val corrected = OrderReviewRules.removeLine(order, itemId, clock())
            if (corrected == null) {
                // Sin partidas no queda nada que enviar: el pedido se descarta (lo pidió el
                // mesero al quitar la última).
                dao.deleteOrder(operationId)
            } else {
                dao.saveOrder(WaiterStorage.toEntity(corrected))
            }
        }
    }

    override suspend fun resend(operationId: String) {
        updateAndSend(operationId) { order -> OrderReviewRules.resend(order, clock()) }
    }

    override suspend fun moveToNewAccount(operationId: String) {
        updateAndSend(operationId) { order -> OrderReviewRules.moveToNewAccount(order, clock()) }
    }

    override suspend fun backToCapture(operationId: String): String? = mutex.withLock {
        val order = load(operationId) ?: return@withLock null
        OrderTransitions.require(order.state, OrderState.DRAFT)
        val capture = captureFrom(order)
        dao.captureFromOrder(WaiterStorage.toEntity(capture, clock()), operationId)
        capture.id
    }

    override suspend fun discard(operationId: String) {
        mutex.withLock {
            val order = load(operationId) ?: return
            // Solo lo que Go no aceptó y espera al mesero: un pedido «Por enviar» o «Enviando»
            // pudo haber entrado ya (respuesta perdida) y descartarlo invitaría a capturarlo otra
            // vez.
            check(order.state == OrderState.NEEDS_REVIEW) {
                "Solo se descarta un pedido que requiere revisión"
            }
            dao.deleteOrder(operationId)
        }
    }

    /**
     * Al arrancar el proceso: lo que quedó «Enviando» vuelve a «Por enviar» (misma clave) y se
     * olvidan los recibidos de hace más de [RECEIVED_KEPT_MILLIS].
     */
    suspend fun recoverAfterRestart() {
        mutex.withLock {
            val now = clock()
            for (row in dao.ordersIn(OrderState.SENDING.storedName)) {
                val recovered = OrderSendRules.recoverInterrupted(WaiterStorage.toOrder(row), now)
                dao.saveOrder(WaiterStorage.toEntity(recovered))
            }
            dao.pruneReceived(OrderState.RECEIVED.storedName, now - RECEIVED_KEPT_MILLIS)
        }
    }

    /**
     * El mesero volvió a entrar con conexión: su credencial de envío es nueva, así que sus pedidos
     * detenidos por `OFFLINE_CREDENTIAL_INVALID` vuelven a intentarse.
     */
    suspend fun credentialRenewed(userId: String) {
        mutex.withLock {
            for (row in dao.ordersOf(userId)) {
                val order = WaiterStorage.toOrder(row)
                if (order.credentialRejected) {
                    dao.saveOrder(
                        WaiterStorage.toEntity(
                            order.copy(credentialRejected = false, updatedAtMillis = clock()),
                        ),
                    )
                }
            }
        }
        scheduler.requestSend()
    }

    override suspend fun workOf(userId: String): WaiterWork {
        val captures = dao.capturesOf(userId).map(WaiterStorage::toCapture)
        val orders = dao.ordersOf(userId).map(WaiterStorage::toOrder)
        return WaiterWorkSummary.of(captures, orders)
    }

    override suspend fun unsentOrders(): List<UnsentOrder> {
        val orders =
            dao.ordersInAny(UNSENT_STATES).map(WaiterStorage::toOrder).map { order ->
                UnsentOrder(
                    waiterName = order.waiterName,
                    description = describeOrder(order),
                    capturedAtEpochMillis = order.capturedAtMillis,
                )
            }
        val drafts =
            dao.allCaptures().map(WaiterStorage::toCapture).filter(Capture::inProgress).map {
                capture ->
                UnsentOrder(
                    waiterName = "",
                    description = describeCapture(capture),
                    capturedAtEpochMillis = capture.createdAtMillis,
                )
            }
        return orders + drafts
    }

    /**
     * «Borrar los datos de este dispositivo» (retiro o «Emparejar de nuevo»): la cola, las capturas
     * y lo guardado de Go, en una transacción. Lo llama el registro único de borrado, **después**
     * de pasar lo no enviado a la bitácora.
     */
    suspend fun eraseAll() {
        mutex.withLock { dao.eraseAll() }
    }

    private suspend fun updateAndSend(operationId: String, change: (PendingOrder) -> PendingOrder) {
        mutex.withLock {
            val order = load(operationId) ?: return
            dao.saveOrder(WaiterStorage.toEntity(change(order)))
        }
        scheduler.requestSend()
    }

    private suspend fun load(operationId: String): PendingOrder? =
        dao.order(operationId)?.let(WaiterStorage::toOrder)

    /**
     * La captura que reproduce exactamente el pedido: los renglones que sustituía son la base (lo
     * que vio de Go) y las partidas del pedido, toques locales encima. Si la cuenta ya no está
     * abierta, la captura va a una cuenta nueva de la mesa.
     */
    private fun captureFrom(order: PendingOrder): Capture {
        val newAccount = order.review?.let(OrderReviewRules::offersNewAccount) == true
        val base =
            if (newAccount || order.destination.accountId == null) {
                emptyList()
            } else {
                order.replaces
            }
        val baseIds = base.map(DraftLine::id).toSet()
        val lineIds = order.lines.map { it.item.id }.toSet()
        val operations = buildList {
            for (line in order.lines) {
                if (line.item.id in baseIds) {
                    add(DraftOperation.Replace(line.item.id, line.item))
                } else {
                    add(DraftOperation.Add(line.item))
                }
            }
            for (saved in base) {
                if (saved.id !in lineIds) {
                    add(DraftOperation.Remove(saved.id))
                }
            }
        }
        return Capture(
            id = order.operationId,
            waiterUserId = order.waiterUserId,
            tableId = order.destination.tableId,
            tableName = order.destination.tableName,
            accountId = order.destination.accountId.takeUnless { newAccount },
            accountName = order.destination.accountName.takeUnless { newAccount },
            seenAccountIds = order.destination.seenAccountIds,
            base = base,
            // Se desconoce la revisión vigente: el primer guardado en Go la pide y, si otra
            // terminal cambió la cuenta, el mesero decide como en cualquier conflicto.
            baseRevision = 0,
            operations = operations,
            ownItemIds = lineIds + baseIds,
            seenPrices = order.lines.associate { it.item.id to it.seenUnitPrice },
            sync = DraftSync.PENDING,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = clock(),
        )
    }

    private fun describeOrder(order: PendingOrder): String {
        val lines =
            order.lines.joinToString(", ") { line ->
                "${line.item.line.quantity} × ${line.productName}"
            }
        return "${order.destination.tableName}: $lines"
    }

    private fun describeCapture(capture: Capture): String {
        val lines =
            capture.visible
                .filter { capture.isOwn(it.id) }
                .joinToString(", ") { item ->
                    "${item.line.quantity} × ${describe(item.line) ?: UNKNOWN_PRODUCT}"
                }
        return "${capture.tableName} (sin enviar a cocina): $lines"
    }

    companion object {
        /** Estados que cuentan como «sin enviar» (§7). */
        val UNSENT_STATES: List<String> =
            OrderState.entries.filter { it.unsent }.map { it.storedName }

        /** Cuánto tiempo se muestran los recibidos en «Pedidos pendientes». */
        const val RECEIVED_KEPT_MILLIS: Long = 12L * 60 * 60 * 1000

        private const val UNKNOWN_PRODUCT = "Producto"
    }
}
