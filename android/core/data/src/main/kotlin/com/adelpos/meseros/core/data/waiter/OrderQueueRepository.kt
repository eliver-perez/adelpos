package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.access.UnsentOrder
import com.adelpos.meseros.core.domain.access.UnsentOrdersSource
import com.adelpos.meseros.core.domain.access.WaiterWork
import com.adelpos.meseros.core.domain.access.WaiterWorkGate
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.connection.PendingOrdersSource
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DiscardResult
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.OrderReviewRules
import com.adelpos.meseros.core.domain.waiter.OrderSendRules
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.OrderTransitions
import com.adelpos.meseros.core.domain.waiter.OthersPending
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.WaiterWorkSummary
import java.util.UUID
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Quien envía la cola (WorkManager en la aplicación; uno falso en las pruebas): «envía lo que falta
 * en cuanto se pueda».
 */
fun interface OrderSendScheduler {
    fun requestSend()
}

/**
 * Quien quita del borrador de Go lo que el mesero descartó (la captura, `CaptureRepository`):
 * «Descartar el pedido» pasa el pedido a una captura en descarte y le pide terminarlo (revisión R6,
 * A2).
 */
fun interface CaptureDiscards {
    suspend fun discardNow(captureId: String): DiscardResult
}

/**
 * La cola de pedidos (§7) sobre las tablas del modo Mesero de `AppDatabase`: cada cambio de estado
 * es una escritura condicional al estado que se leyó ([WaiterDao.transition], revisión R6, M1 y O1)
 * con las transiciones del dominio ([OrderTransitions]); nada se borra por cerrar la aplicación,
 * reiniciar o perder la batería. También es:
 * - el contador del indicador de conexión ([PendingOrdersSource], todo el dispositivo);
 * - el trabajo pendiente de cada mesero ([WaiterWorkGate]: capturas en curso y pedidos sin enviar);
 * - lo que se muestra antes de borrar al retirar el dispositivo o emparejarlo de nuevo
 *   ([UnsentOrdersSource]).
 *
 * @param describe nombre de un producto del catálogo guardado, para describir lo que no se envió.
 * @param discards termina los descartes que deben quitar algo del borrador de Go.
 * @param inFlight pedidos que este proceso confirma en línea ahora mismo (no se recuperan).
 */
class OrderQueueRepository(
    private val dao: WaiterDao,
    private val scheduler: OrderSendScheduler,
    private val describe: (LineSelection) -> String? = { null },
    private val clock: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
    private val discards: CaptureDiscards? = null,
    private val inFlight: OrdersInFlight = OrdersInFlight(),
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
                dao.transition(WaiterStorage.toEntity(corrected), order.state.storedName)
            }
        }
    }

    override suspend fun resend(operationId: String) {
        updateAndSend(operationId) { order -> OrderReviewRules.resend(order, clock()) }
    }

    override suspend fun moveToNewAccount(operationId: String) {
        // Con una cuenta combinada no se ofrece ni se hace (regla del coordinador, M3).
        if (load(operationId)?.review?.accountMerged == true) {
            return
        }
        updateAndSend(operationId) { order ->
            OrderReviewRules.moveToNewAccount(order, clock(), newId)
        }
    }

    override suspend fun sendToAccount(operationId: String, accountId: String) {
        updateAndSend(operationId) { order ->
            OrderReviewRules.sendToAccount(order, accountId, clock())
        }
    }

    override fun othersPending(userId: String): Flow<List<OthersPending>> =
        dao.observeOthers(userId, UNSENT_STATES).map { rows ->
            rows
                .map(WaiterStorage::toOrder)
                .groupBy { it.waiterUserId }
                .map { (_, orders) ->
                    OthersPending(
                        waiterName = orders.first().waiterName,
                        unsent = orders.size,
                        waitingForPin = orders.count { it.credentialRejected },
                        needingReview = orders.count { it.state == OrderState.NEEDS_REVIEW },
                    )
                }
                .sortedBy { it.waiterName }
        }

    override suspend fun backToCapture(operationId: String): String? = mutex.withLock {
        val order = load(operationId) ?: return@withLock null
        OrderTransitions.require(order.state, OrderState.DRAFT)
        val capture = captureFrom(order)
        dao.captureFromOrder(WaiterStorage.toEntity(capture, clock()), operationId)
        capture.id
    }

    /**
     * «Descartar el pedido» (revisión R6, A2): si nada del pedido llegó al borrador de Go (no
     * sustituía renglones guardados ni dejó un guardado sin respuesta), se borra; si no, pasa a una
     * captura en descarte que quita de Go lo suyo (sin conexión, en cuanto la haya).
     */
    override suspend fun discard(operationId: String): DiscardResult {
        val captureId = mutex.withLock {
            val order = load(operationId) ?: return DiscardResult.DONE
            // Solo lo que Go no aceptó y espera al mesero: un pedido «Por enviar» o «Enviando»
            // pudo haber entrado ya (respuesta perdida) y descartarlo invitaría a capturarlo
            // otra vez.
            check(order.state == OrderState.NEEDS_REVIEW) {
                "Solo se descarta un pedido que requiere revisión"
            }
            if (order.replaces.isEmpty() && order.unresolvedSave == null) {
                dao.deleteOrder(operationId)
                return DiscardResult.DONE
            }
            val capture = discardingCaptureFrom(order)
            dao.captureFromOrder(WaiterStorage.toEntity(capture, clock()), operationId)
            capture.id
        }
        return discards?.discardNow(captureId) ?: DiscardResult.PENDING
    }

    /**
     * Al arrancar el proceso: lo que quedó «Enviando» vuelve a «Por enviar» (misma clave) y se
     * olvidan los recibidos de hace más de [RECEIVED_KEPT_MILLIS]. (El trabajador hace lo mismo al
     * empezar cada vuelta: revisión R6, B2.)
     */
    suspend fun recoverAfterRestart() {
        mutex.withLock {
            val now = clock()
            for (row in dao.ordersIn(OrderState.SENDING.storedName)) {
                if (row.operationId in inFlight) {
                    continue
                }
                val recovered = OrderSendRules.recoverInterrupted(WaiterStorage.toOrder(row), now)
                dao.transition(WaiterStorage.toEntity(recovered), OrderState.SENDING.storedName)
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
                    dao.transition(
                        WaiterStorage.toEntity(
                            order.copy(credentialRejected = false, updatedAtMillis = clock()),
                        ),
                        order.state.storedName,
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
            dao.transition(WaiterStorage.toEntity(change(order)), order.state.storedName)
        }
        scheduler.requestSend()
    }

    private suspend fun load(operationId: String): PendingOrder? =
        dao.order(operationId)?.let(WaiterStorage::toOrder)

    /**
     * La captura que reproduce exactamente el pedido: los renglones que sustituía son la base (lo
     * que vio de Go, **con la marca de base vista**: antes de guardar se compara con la de Go y, si
     * otra terminal la cambió, el conflicto se muestra; revisión R6, M2) y las partidas del pedido,
     * toques locales encima. Lo que Go dijo que ya estaba en cocina (`draft_item_sent`) no vuelve a
     * la captura: nunca se reenvía. Si la cuenta ya no está abierta, la captura va a una cuenta
     * nueva de la mesa (con renglones nuevos al guardarse).
     */
    private fun captureFrom(order: PendingOrder): Capture {
        val newAccount = order.review?.let(OrderReviewRules::offersNewAccount) == true
        val alreadySent = order.review?.alreadySentItemIds.orEmpty()
        val base =
            if (newAccount || order.destination.accountId == null) {
                emptyList()
            } else {
                order.replaces.filter { it.id !in alreadySent }
            }
        val kept = order.lines.filter { it.item.id !in alreadySent }
        // A una cuenta nueva, con identificadores nuevos (M3): los anteriores pueden seguir en el
        // borrador de la otra cuenta.
        val lines =
            if (newAccount) {
                kept.map { line -> line.copy(item = line.item.copy(id = newId())) }
            } else {
                kept
            }
        val baseIds = base.map(DraftLine::id).toSet()
        val lineIds = lines.map { it.item.id }.toSet()
        // Lo que dejó el guardado sin respuesta pudo entrar: «Sustituir» lo cambia si está y lo
        // agrega si no (C1).
        val unresolvedIds = order.unresolvedSave?.items?.map(DraftLine::id)?.toSet().orEmpty()
        val operations = buildList {
            for (line in lines) {
                if (line.item.id in baseIds || line.item.id in unresolvedIds) {
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
            // Base vista sin su revisión: el primer guardado la compara con la de Go.
            baseRevision =
                if (base.isEmpty()) {
                    0
                } else {
                    Capture.SEEN_BASE_REVISION
                },
            operations = operations,
            ownItemIds = lineIds + baseIds,
            seenPrices = lines.associate { it.item.id to it.seenUnitPrice },
            sync = DraftSync.PENDING,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = clock(),
        )
    }

    /**
     * La captura en descarte de un pedido (revisión R6, A2): la base son las versiones de sus
     * renglones que pudieron quedar en el borrador de Go (los que sustituía y los del guardado sin
     * respuesta) y las partidas van encima; el descarte quita de Go solo los suyos que siguen igual
     * a alguna de esas versiones. Una apertura sin respuesta se repite con su clave para saber qué
     * cuenta abrió.
     */
    private fun discardingCaptureFrom(order: PendingOrder): Capture {
        val unresolved = order.unresolvedSave
        val replacedIds = order.replaces.map(DraftLine::id).toSet()
        val fromSave = unresolved?.items.orEmpty().filter { it.id !in replacedIds }
        val base = order.replaces + fromSave
        val baseIds = base.map(DraftLine::id).toSet()
        val operations =
            order.lines.map { line ->
                if (line.item.id in baseIds) {
                    DraftOperation.Replace(line.item.id, line.item)
                } else {
                    DraftOperation.Add(line.item)
                }
            }
        val accountId = order.destination.accountId ?: unresolved?.accountId
        // Solo la apertura se repite (sus renglones son la base: el guardado lleva la base).
        val repeatOpening = unresolved != null && unresolved.accountId == null && accountId == null
        return Capture(
            id = order.operationId,
            waiterUserId = order.waiterUserId,
            tableId = order.destination.tableId,
            tableName = order.destination.tableName,
            accountId = accountId,
            accountName = order.destination.accountName,
            seenAccountIds = unresolved?.seenAccountIds ?: order.destination.seenAccountIds,
            base = base,
            baseRevision = Capture.SEEN_BASE_REVISION,
            operations = operations,
            ownItemIds = baseIds + order.lines.map { it.item.id },
            seenPrices = order.lines.associate { it.item.id to it.seenUnitPrice },
            sync = DraftSync.DISCARDING,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = unresolved?.operationId?.takeIf { repeatOpening },
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
        if (capture.sync == DraftSync.DISCARDING) {
            return "${capture.tableName} (descartado, sin quitar todavía de la cuenta): $lines"
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
