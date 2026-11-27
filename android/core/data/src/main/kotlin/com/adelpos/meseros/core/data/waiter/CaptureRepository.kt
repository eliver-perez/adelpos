package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.LicenseMessages
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiJson
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ApiProblem
import com.adelpos.meseros.core.data.waiter.db.CustomerEditEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.DiscardResult
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftQuote
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OfflineOrderPlan
import com.adelpos.meseros.core.domain.waiter.OnlineConfirm
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.OrderSendRules
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.SeenUnion
import com.adelpos.meseros.core.domain.waiter.SendOutcome
import com.adelpos.meseros.core.domain.waiter.SendRequest
import com.adelpos.meseros.core.domain.waiter.TableAccountRef
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.UnionJoin
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.SerializationException

/**
 * Capturas del mesero (§6.2 y §6.3) y el guardado del borrador en Go, como `draftSync.ts` del
 * escritorio, con todo guardado en Room por mesero:
 * - cada toque se guarda primero en el dispositivo (sobrevive a la rotación, al cierre del proceso
 *   y a perder la conexión) y, con conexión, se guarda en Go con una espera corta
 *   ([SAVE_DELAY_MILLIS]) y la revisión vigente; como mucho una petición a la vez en todo el
 *   proceso;
 * - **creación diferida**: una cuenta pendiente nace en Go con el primer producto (`openAccount`
 *   con `seen_account_ids`; `TABLE_HAS_ACCOUNT` lo decide el mesero);
 * - un guardado sin respuesta se repite **igual** (misma clave y mismo contenido) antes de mandar
 *   nada más, también después de reiniciar;
 * - si otra terminal cambió la cuenta (`ACCOUNT_CONFLICT` o una revisión nueva por el canal) con
 *   toques sin guardar, el conflicto es explícito: «Conservar mis cambios» o «Descartarlos»;
 * - «Enviar a cocina» con conexión no arma el pedido mientras el guardado anterior no tenga
 *   respuesta (lo espera o lo repite con su clave) y la confirmación en línea usa **la clave del
 *   pedido** (revisión R6, C1 y A1);
 * - «Descartar la captura» también quita de Go lo suyo guardado (y la cuenta vacía que nació con la
 *   captura); sin conexión el descarte queda pendiente (revisión R6, A2);
 * - **Para llevar** (etapa A2): la misma captura sin mesa; el pedido nace con el primer producto
 *   (`openAccount` con `mode: takeaway`, el origen «Para llevar» y el nombre y el teléfono) o, sin
 *   conexión, al enviarse (`destination.takeaway`). Mientras el pedido no sale a Go, el nombre y el
 *   teléfono se guardan en la captura; después, como una edición aparte (A2-D1), para que todo
 *   reintento con la misma clave lleve el mismo contenido.
 *
 * Todo sale con las cabeceras del **mesero que capturó** (revisión R3, M5).
 */
class CaptureRepository(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val scheduler: OrderSendScheduler,
    private val catalog: CatalogLookups,
    private val scope: CoroutineScope,
    private val clock: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
    private val saveDelayMillis: Long = SAVE_DELAY_MILLIS,
    private val serverClock: ServerClock? = null,
    private val inFlight: OrdersInFlight = OrdersInFlight(),
    private val settleTimeoutMillis: Long = SETTLE_TIMEOUT_MILLIS,
    /**
     * Etapa M1 (§5.6): la cuenta que devolvió Go al abrirla, guardar su borrador o confirmarlo,
     * para que Mesas la muestre al momento (`RestaurantRepository.applyAccount`).
     */
    private val accountAnswered: (AccountSummary) -> Unit = {},
) : CaptureService {
    /** Lo que el modo Mesero necesita del catálogo guardado. */
    interface CatalogLookups {
        val lookup: OrderFactory.CatalogLookup

        /** Versión y época del catálogo guardado (las que viajan con el pedido sin conexión). */
        fun stamp(): OrderFactory.CatalogStamp
    }

    private val edits = Mutex()
    private val queue = Channel<SyncRequest>(Channel.UNLIMITED)
    private val timers = ConcurrentHashMap<String, Job>()
    private val quotes = MutableStateFlow<Map<String, DraftQuote>>(emptyMap())
    private var started = false

    private class SyncRequest(val captureId: String, val done: CompletableDeferred<Unit>?)

    /** Arranca el único guardador del proceso (una petición a la vez). */
    fun start() {
        if (started) {
            return
        }
        started = true
        scope.launch {
            for (request in queue) {
                try {
                    syncOnce(request.captureId)
                } finally {
                    request.done?.complete(Unit)
                }
            }
        }
    }

    override fun captures(userId: String): Flow<List<Capture>> =
        dao.observeCaptures(userId).map { rows -> rows.map(WaiterStorage::toCapture) }

    override fun capture(captureId: String): Flow<Capture?> =
        dao.observeCapture(captureId).map { row -> row?.let(WaiterStorage::toCapture) }

    override fun quote(captureId: String): Flow<DraftQuote?> = quotes.map { it[captureId] }

    override suspend fun open(
        userId: String,
        tableId: String,
        tableName: String,
        account: AccountSummary?,
        seenAccountIds: List<String>,
        seenUnion: SeenUnion?,
        unionJoin: UnionJoin?,
    ): Capture {
        val existing = edits.withLock {
            val found =
                dao.capturesOf(userId).map(WaiterStorage::toCapture).firstOrNull { capture ->
                    capture.tableId == tableId &&
                        capture.accountId == account?.id &&
                        capture.sync != DraftSync.DISCARDING
                }
            // Etapa M1: una captura de cuenta nueva que todavía no salió toma la unión que ve
            // ahora el mesero (por ejemplo, unió la mesa en pantalla después de empezarla).
            if (found != null && found.accountId == null && found.pendingOperationId == null) {
                val updated =
                    found.copy(
                        tableName = tableName,
                        seenAccountIds = seenAccountIds,
                        seenUnion = seenUnion ?: found.seenUnion,
                        unionJoin = unionJoin,
                    )
                if (updated != found) {
                    save(updated)
                }
                updated
            } else {
                found
            }
        }
        if (existing != null) {
            requestSync(existing.id)
            return existing
        }
        val created =
            Capture(
                id = newId(),
                waiterUserId = userId,
                tableId = tableId,
                tableName = tableName,
                accountId = account?.id,
                accountName = account?.name,
                seenAccountIds = seenAccountIds,
                base = emptyList(),
                // Se desconoce el borrador de Go hasta leerlo (0): nunca se guarda encima de uno
                // que no se vio.
                baseRevision = 0,
                operations = emptyList(),
                ownItemIds = emptySet(),
                seenPrices = emptyMap(),
                sync = DraftSync.SAVED,
                syncMessage = null,
                tableAccounts = emptyList(),
                pendingOperationId = null,
                inFlightCount = 0,
                createdAtMillis = clock(),
                seenUnion = seenUnion,
                unionJoin = unionJoin.takeIf { account == null },
            )
        edits.withLock { save(created) }
        if (account != null) {
            // Con conexión, el borrador vigente de Go es la base (puede tener renglones de otras
            // terminales); sin ella, la captura empieza vacía y Go revisa al recibir.
            refreshFromGo(created.id)
        }
        return created
    }

    override suspend fun openTakeaway(userId: String, account: AccountSummary?): Capture {
        val existing = edits.withLock {
            val mine =
                dao.capturesOf(userId).map(WaiterStorage::toCapture).filter { capture ->
                    capture.takeaway && capture.sync != DraftSync.DISCARDING
                }
            if (account != null) {
                mine.firstOrNull { it.accountId == account.id }
            } else {
                // «Nuevo pedido» siempre empieza vacío; se reutiliza una captura nueva que quedó
                // vacía (por ejemplo, si Android cerró la aplicación con ella abierta).
                mine
                    .firstOrNull { capture ->
                        capture.accountId == null &&
                            !capture.inProgress &&
                            capture.pendingOperationId == null
                    }
                    ?.let { idle ->
                        val fresh = idle.copy(customer = TakeawayCustomer.NONE)
                        save(fresh)
                        dao.deleteCustomerEdit(fresh.id)
                        fresh
                    }
            }
        }
        if (existing != null) {
            requestSync(existing.id)
            return existing
        }
        val created =
            Capture(
                id = newId(),
                waiterUserId = userId,
                tableId = null,
                tableName = "",
                accountId = account?.id,
                accountName = account?.name,
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
                createdAtMillis = clock(),
                mode = OrderMode.TAKEAWAY,
            )
        edits.withLock { save(created) }
        if (account != null) {
            refreshFromGo(created.id)
        }
        return created
    }

    override suspend fun setCustomer(captureId: String, customer: TakeawayCustomer) {
        val wanted = customer.trimmed()
        var queued = false
        edits.withLock {
            val capture = load(captureId) ?: return
            if (!capture.takeaway || capture.sync == DraftSync.DISCARDING) {
                return
            }
            if (capture.customerEditable) {
                // Todavía no sale nada a Go: viajan con el pedido al crearse. Una edición anterior
                // (de cuando salía una apertura que Go no aceptó) queda vieja: esta es la última.
                save(capture.copy(customer = wanted))
                dao.deleteCustomerEdit(capture.id)
                return
            }
            // El pedido ya salió (o está saliendo) a Go: la edición va aparte y se envía en cuanto
            // se pueda, con su propia clave (A2-D1). Si ya había una igual, se conserva su clave.
            val key = capture.accountId ?: capture.id
            val previous = dao.customerEditFor(key, capture.accountId)
            val same =
                previous != null &&
                    previous.customerName == wanted.name &&
                    previous.phone == wanted.phone
            if (same) {
                return
            }
            dao.replaceCustomerEdit(
                CustomerEditEntity(
                    key = key,
                    accountId = capture.accountId,
                    waiterUserId = capture.waiterUserId,
                    customerName = wanted.name,
                    phone = wanted.phone,
                    operationId = newId(),
                    updatedAt = clock(),
                ),
            )
            queued = true
        }
        if (queued) {
            scheduler.requestSend()
        }
    }

    override suspend fun stage(captureId: String, operation: DraftOperation, seenPrice: Money?) {
        edits.withLock {
            val capture = load(captureId) ?: return
            if (capture.sync == DraftSync.DISCARDING) {
                return
            }
            val own =
                when (operation) {
                    is DraftOperation.Add -> capture.ownItemIds + operation.item.id
                    is DraftOperation.Quantity -> capture.ownItemIds + operation.fallback.id
                    else -> capture.ownItemIds
                }
            val priced =
                when {
                    seenPrice == null -> capture.seenPrices
                    operation is DraftOperation.Add ->
                        capture.seenPrices + (operation.item.id to seenPrice)
                    operation is DraftOperation.Replace ->
                        capture.seenPrices + (operation.id to seenPrice)
                    else -> capture.seenPrices
                }
            val next =
                capture.copy(
                    operations = capture.operations + operation,
                    ownItemIds = own,
                    seenPrices = priced,
                    sync = pendingStateAfterTouch(capture),
                    syncMessage =
                        if (capture.sync == DraftSync.REJECTED) null else capture.syncMessage,
                )
            save(next)
        }
        scheduleSync(captureId)
    }

    override suspend fun keepMine(captureId: String) {
        val bill = fetchBill(captureId) ?: return
        edits.withLock {
            val capture = load(captureId) ?: return
            save(
                capture.copy(
                    base = bill.draft,
                    baseRevision = bill.account.revision,
                    sync = DraftSync.PENDING,
                    syncMessage = null,
                ),
            )
        }
        requestSync(captureId)
    }

    override suspend fun discardMine(captureId: String) {
        val bill = fetchBill(captureId) ?: return
        edits.withLock {
            val capture = load(captureId) ?: return
            save(
                capture.copy(
                    base = bill.draft,
                    baseRevision = bill.account.revision,
                    operations = emptyList(),
                    sync = DraftSync.SAVED,
                    syncMessage = null,
                ),
            )
        }
    }

    override suspend fun joinAccount(captureId: String, account: TableAccountRef) {
        edits.withLock {
            val capture = load(captureId) ?: return
            save(
                capture.copy(
                    accountId = account.id,
                    accountName = account.name.ifEmpty { capture.accountName },
                    // Seguir una cuenta combinada (M3): la combinación movió el borrador con sus
                    // identificadores, así que lo que vio el mesero se compara con el de la cuenta
                    // nueva. Sin cuenta antes (otra terminal abrió una), no había base.
                    baseRevision =
                        if (capture.accountId == null) {
                            0
                        } else {
                            Capture.SEEN_BASE_REVISION
                        },
                    tableAccounts = emptyList(),
                    sync = DraftSync.PENDING,
                    syncMessage = null,
                    pendingOperationId = null,
                    inFlightCount = 0,
                    createdAccount = false,
                ),
            )
        }
        // La base pasa a ser el borrador de esa cuenta; los toques se guardan encima.
        refreshFromGo(captureId)
        requestSync(captureId)
    }

    override suspend fun openAnotherAccount(captureId: String) {
        edits.withLock {
            val capture = load(captureId) ?: return
            if (capture.accountMerged) {
                // Regla del coordinador: lo que la combinación movió se enviaría dos veces.
                return
            }
            if (capture.sync == DraftSync.TABLE_HAS_ACCOUNT) {
                // Ya vio las otras cuentas: la creación se reenvía sabiéndolo (clave nueva: el
                // rechazo no creó nada).
                save(
                    capture.copy(
                        seenAccountIds =
                            capture.seenAccountIds + capture.tableAccounts.map { it.id },
                        tableAccounts = emptyList(),
                        sync = DraftSync.PENDING,
                        pendingOperationId = null,
                        inFlightCount = 0,
                    ),
                )
            } else {
                var moved = movedToNewAccount(capture)
                if (capture.takeaway) {
                    // Un pedido para llevar nuevo lleva el último nombre y teléfono del mesero; la
                    // edición pendiente era de un pedido que ya no está abierto.
                    val pending = dao.customerEditFor(capture.id, capture.accountId)
                    if (pending != null) {
                        moved =
                            moved.copy(
                                customer = TakeawayCustomer(pending.customerName, pending.phone),
                            )
                        dao.deleteCustomerEdit(pending.key)
                    }
                }
                save(moved)
            }
        }
        requestSync(captureId)
    }

    override suspend fun retrySave(captureId: String) {
        requestSync(captureId)
    }

    override suspend fun observe(captureId: String, bill: AccountBill) {
        edits.withLock {
            val capture = load(captureId) ?: return
            val ignored =
                capture.accountId != bill.account.id ||
                    capture.pendingOperationId != null ||
                    capture.sync == DraftSync.DISCARDING
            if (ignored) {
                // Con un guardado propio en curso, su respuesta es la autoridad; un descarte
                // pendiente ya no muestra la cuenta.
                return
            }
            if (bill.account.revision <= capture.baseRevision) {
                return
            }
            if (!bill.account.isOpen) {
                // Se cobró, se canceló o se combinó en otra terminal: no es un conflicto que se
                // resuelva conservando los toques, sino una cuenta que ya no admite nada (§6.4).
                save(
                    capture.copy(
                        baseRevision = bill.account.revision,
                        sync = DraftSync.ACCOUNT_CLOSED,
                        syncMessage = null,
                        tableAccounts = mergedTarget(bill.account.mergedIntoId),
                    ),
                )
                return
            }
            if (capture.baseRevision == Capture.SEEN_BASE_REVISION) {
                save(reconciled(capture, bill))
                return
            }
            // Sin base leída, los toques nunca vieron otra: se aplican encima de la de Go.
            if (capture.operations.isEmpty() || capture.baseRevision == 0) {
                save(
                    capture.copy(
                        base = bill.draft,
                        baseRevision = bill.account.revision,
                        sync = DraftSync.SAVED,
                        syncMessage = null,
                    ),
                )
            } else if (capture.sync != DraftSync.CONFLICT) {
                save(capture.copy(sync = DraftSync.CONFLICT))
            }
        }
    }

    override suspend fun sendToKitchen(captureId: String, waiterName: String): SendRequest {
        settle(captureId, settleTimeoutMillis)
        var capture = edits.withLock { load(captureId) } ?: return SendRequest.NothingToSend
        if (capture.sync == DraftSync.DISCARDING) {
            return SendRequest.NothingToSend
        }
        if (capture.waitsForUnion) {
            // Etapa M1 (M1-D13): una unión en pantalla nunca entra en la cola. Sin conexión (o si
            // Go la rechazó) el mesero elige «Enviar solo a la mesa» o «Esperar».
            return SendRequest.UnionPending
        }
        if (capture.pendingOperationId != null && hasSession(capture)) {
            // Revisión R6, C1: con conexión no se arma el pedido mientras el guardado anterior no
            // tenga respuesta. Se espera a que llegue (su lectura tarda hasta 15 s) o se repite
            // con su clave, y el pedido sale con lo que Go guardó de verdad.
            settle(captureId, RESOLVE_TIMEOUT_MILLIS)
            capture = edits.withLock { load(captureId) } ?: return SendRequest.NothingToSend
            if (capture.pendingOperationId != null && capture.sync == DraftSync.SAVING) {
                return SendRequest.StillSaving
            }
        }
        val plan = OfflineOrderPlan.of(capture)
        if (plan.items.isEmpty()) {
            return SendRequest.NothingToSend
        }
        if (plan.items.size > MAXIMUM_ORDER_LINES || plan.replaces.size > MAXIMUM_ORDER_LINES) {
            return SendRequest.TooManyLines
        }
        val online = onlineConfirmFor(capture)
        // La hora de captura en la hora de Go si se conoce la diferencia (revisión R6, B4).
        val offset = serverClock?.offsetMillis()
        val factoryOrder =
            OrderFactory.fromCapture(
                capture = capture,
                waiterName = waiterName,
                // La clave del pedido es la de la captura, **la misma** en la confirmación en línea
                // y en el pedido sin conexión (A1-D19): si la confirmación entró y su respuesta se
                // perdió, Go lo reconoce como ya aceptado. Si vuelve a la captura para corregirlo,
                // sale con la misma.
                operationId = capture.id,
                catalog = catalog.stamp(),
                lookup = catalog.lookup,
                onlineConfirm = online,
                nowMillis = clock() + (offset ?: 0L),
            ) ?: return SendRequest.NothingToSend
        val order =
            factoryOrder.copy(
                unresolvedSave = capture.unresolvedSave(),
                clockCorrected = offset != null,
            )
        if (online != null) {
            // Antes de que exista «Enviando»: el trabajador no lo toma por interrumpido (B2).
            inFlight.add(order.operationId)
        }
        try {
            edits.withLock {
                dao.queueFromCapture(WaiterStorage.toEntity(order), captureId)
                timers.remove(captureId)?.cancel()
                quotes.value = quotes.value - captureId
            }
        } catch (failure: Throwable) {
            inFlight.remove(order.operationId)
            throw failure
        }
        if (online != null) {
            scope.launch { confirmOnline(order) }
        } else {
            scheduler.requestSend()
        }
        return SendRequest.Queued(order.operationId, order.state)
    }

    override suspend fun discard(captureId: String): DiscardResult {
        edits.withLock {
            val capture = load(captureId) ?: return DiscardResult.DONE
            timers.remove(captureId)?.cancel()
            quotes.value = quotes.value - captureId
            // Lo suyo pudo llegar al borrador de Go: renglones guardados o un guardado sin
            // respuesta. Si no, no hay nada que quitar fuera del dispositivo.
            val inGo =
                capture.pendingOperationId != null ||
                    (capture.accountId != null && capture.base.any { capture.isOwn(it.id) })
            if (!inGo) {
                dao.deleteCapture(captureId)
                return DiscardResult.DONE
            }
            save(capture.copy(sync = DraftSync.DISCARDING, syncMessage = null))
        }
        return discardNow(captureId)
    }

    /**
     * Termina un descarte (también el de «Descartar el pedido», que pasa el pedido a una captura en
     * descarte): lo intenta enseguida y dice si quedó pendiente (sin conexión o sin sesión).
     */
    suspend fun discardNow(captureId: String): DiscardResult {
        settle(captureId, settleTimeoutMillis)
        return if (edits.withLock { load(captureId) } == null) {
            DiscardResult.DONE
        } else {
            DiscardResult.PENDING
        }
    }

    override fun closeIfIdle(captureId: String) {
        scope.launch {
            edits.withLock {
                val capture = load(captureId) ?: return@withLock
                if (!capture.inProgress && capture.pendingOperationId == null) {
                    timers.remove(captureId)?.cancel()
                    dao.deleteCapture(captureId)
                    quotes.value = quotes.value - captureId
                }
            }
        }
    }

    /**
     * Con conexión de nuevo (o el mesero volvió a entrar): las capturas de [userId] con algo por
     * guardar se guardan.
     */
    suspend fun resume(userId: String) {
        for (row in dao.capturesOf(userId)) {
            val capture = WaiterStorage.toCapture(row)
            val waiting =
                capture.sync == DraftSync.PENDING ||
                    capture.sync == DraftSync.UNCERTAIN ||
                    capture.sync == DraftSync.SAVING ||
                    capture.sync == DraftSync.DISCARDING
            if (waiting) {
                requestSync(capture.id)
            }
        }
    }

    // --- Guardado en Go ---

    private fun scheduleSync(captureId: String) {
        timers.remove(captureId)?.cancel()
        timers[captureId] = scope.launch {
            delay(saveDelayMillis)
            requestSync(captureId)
        }
    }

    private fun requestSync(captureId: String) {
        queue.trySend(SyncRequest(captureId, done = null))
    }

    /**
     * Guarda ya lo pendiente (o termina un descarte) y espera hasta [timeoutMillis] a que termine,
     * antes de «Enviar a cocina» o de decir cómo quedó un descarte.
     */
    private suspend fun settle(captureId: String, timeoutMillis: Long) {
        timers.remove(captureId)?.cancel()
        val done = CompletableDeferred<Unit>()
        queue.trySend(SyncRequest(captureId, done))
        withTimeoutOrNull(timeoutMillis) { done.await() }
    }

    private fun hasSession(capture: Capture): Boolean =
        server.transport() != null && server.sessionHeaders(capture.waiterUserId) != null

    private suspend fun syncOnce(captureId: String) {
        val capture = edits.withLock { load(captureId) } ?: return
        if (capture.sync == DraftSync.DISCARDING) {
            discardInGo(captureId)
            return
        }
        val blocked =
            capture.sync == DraftSync.CONFLICT ||
                capture.sync == DraftSync.TABLE_HAS_ACCOUNT ||
                capture.sync == DraftSync.ACCOUNT_CLOSED
        if (blocked) {
            return
        }
        val transport = server.transport()
        val headers = server.sessionHeaders(capture.waiterUserId)
        if (transport == null || headers == null) {
            // Sin sesión de Go (sin conexión al entrar o la sesión terminó): queda en el
            // dispositivo y se guarda al volver.
            if (capture.operations.isNotEmpty() && capture.sync != DraftSync.UNCERTAIN) {
                edits.withLock { load(captureId)?.let { save(it.copy(sync = DraftSync.PENDING)) } }
            }
            return
        }
        if (
            capture.accountId != null &&
                capture.baseRevision <= 0 &&
                capture.pendingOperationId == null
        ) {
            // Primero el borrador vigente de Go (puede tener renglones de otras terminales): un
            // guardado sustituye el borrador completo. Con una base vista (M2), si otra terminal
            // la cambió, el conflicto se muestra en lugar de guardar encima.
            refreshFromGo(captureId)
            val loaded = edits.withLock { load(captureId) } ?: return
            val stopped =
                loaded.sync == DraftSync.CONFLICT || loaded.sync == DraftSync.ACCOUNT_CLOSED
            if (loaded.baseRevision <= 0 || stopped) {
                return
            }
        }
        val sending = edits.withLock { prepareSend(captureId) } ?: return
        val items = sending.inFlightItems
        val operationId = requireNotNull(sending.pendingOperationId)
        val accountId = sending.accountId
        val outcome =
            if (accountId == null) {
                api.openAccount(
                    transport,
                    headers,
                    WaiterMapper.openBody(
                        operationId = operationId,
                        mode = sending.mode,
                        tableId = sending.tableId,
                        seenAccountIds = sending.seenAccountIds,
                        customer = sending.customer,
                        items = items,
                        seenUnion = sending.seenUnion,
                        unionJoin = sending.unionJoin,
                    ),
                )
            } else {
                api.saveDraft(
                    transport,
                    headers,
                    accountId,
                    SaveDraftBody(
                        operationId = operationId,
                        revision = sending.baseRevision,
                        items = items.map(WaiterMapper::draftItem),
                    ),
                )
            }
        when (outcome) {
            is ApiOutcome.Success -> acknowledge(captureId, WaiterMapper.bill(outcome.value))
            is ApiOutcome.Rejected -> rejectSave(captureId, outcome.problem)
            is ApiOutcome.NoAnswer -> uncertain(captureId, null)
            is ApiOutcome.Malformed -> uncertain(captureId, null)
        }
    }

    /**
     * Marca el guardado que va a salir: la clave (nueva o la del guardado incierto, que se repite
     * igual) y cuántos toques incluye. Sin nada que guardar devuelve null.
     */
    private suspend fun prepareSend(captureId: String): Capture? {
        val capture = load(captureId) ?: return null
        if (capture.pendingOperationId != null) {
            return capture
        }
        if (capture.operations.isEmpty()) {
            if (capture.sync != DraftSync.SAVED && capture.sync != DraftSync.REJECTED) {
                save(capture.copy(sync = DraftSync.SAVED))
            }
            return null
        }
        if (capture.accountId == null && capture.visible.isEmpty()) {
            // Los toques se anularon entre sí (agregar y quitar): no hay cuenta que crear.
            save(capture.copy(operations = emptyList(), sync = DraftSync.SAVED))
            return null
        }
        val sending =
            capture.copy(
                pendingOperationId = newId(),
                inFlightCount = capture.operations.size,
                sync = DraftSync.SAVING,
                syncMessage = null,
            )
        save(sending)
        return sending
    }

    private suspend fun acknowledge(captureId: String, bill: AccountBill) {
        accountAnswered(bill.account)
        val acknowledged = edits.withLock {
            val capture = load(captureId) ?: return
            val remaining = capture.operations.drop(capture.inFlightCount)
            val next =
                capture.copy(
                    // La cuenta nació con esta captura (`openAccount`): si el mesero la descarta
                    // vacía, también se descarta la cuenta (A2).
                    createdAccount = capture.createdAccount || capture.accountId == null,
                    accountId = bill.account.id,
                    accountName = bill.account.name,
                    // Etapa M1: la unión en pantalla nació con la cuenta; desde ahora es la que se
                    // ve.
                    seenUnion =
                        if (capture.accountId == null && capture.seenUnion != null) {
                            SeenUnion(bill.account.tableUnion?.id)
                        } else {
                            capture.seenUnion
                        },
                    unionJoin = null,
                    tableName = bill.account.tableUnion?.name ?: capture.tableName,
                    base = bill.draft,
                    baseRevision = bill.account.revision,
                    operations = remaining,
                    pendingOperationId = null,
                    inFlightCount = 0,
                    sync =
                        if (remaining.isEmpty()) {
                            DraftSync.SAVED
                        } else {
                            DraftSync.PENDING
                        },
                    syncMessage = null,
                )
            save(next)
            if (capture.accountId == null && capture.takeaway) {
                // El pedido para llevar nació en Go: una edición del cliente hecha mientras salía
                // ya sabe a qué cuenta va (A2-D1).
                dao.bindCustomerEdit(captureId, bill.account.id)
            }
            next
        }
        if (
            acknowledged.takeaway && dao.customerEditFor(captureId, acknowledged.accountId) != null
        ) {
            scheduler.requestSend()
        }
        if (acknowledged.operations.isNotEmpty()) {
            requestSync(captureId)
        } else {
            refreshQuote(captureId)
        }
    }

    private suspend fun uncertain(captureId: String, message: String?) {
        edits.withLock {
            val capture = load(captureId) ?: return
            save(capture.copy(sync = DraftSync.UNCERTAIN, syncMessage = message))
        }
        // Se repite igual en un momento (o al volver la conexión).
        scope.launch {
            delay(UNCERTAIN_RETRY_MILLIS)
            requestSync(captureId)
        }
    }

    private suspend fun rejectSave(captureId: String, problem: ApiProblem) {
        val capture = edits.withLock { load(captureId) } ?: return
        val error = ApiErrors.toAppError(problem)
        when {
            problem.code == TABLE_HAS_ACCOUNT && capture.accountId == null -> {
                val accounts = tableAccounts(problem)
                edits.withLock {
                    load(captureId)?.let {
                        save(
                            it.copy(
                                sync = DraftSync.TABLE_HAS_ACCOUNT,
                                tableAccounts = accounts,
                                pendingOperationId = null,
                                inFlightCount = 0,
                            ),
                        )
                    }
                }
            }
            // Etapa M1 (M1-D9): la mesa ya está en una unión que el mesero no veía. Se ofrece lo
            // mismo que con TABLE_HAS_ACCOUNT, con las cuentas de la unión: agregar a una de ellas
            // o abrir otra en la unión (que desde ahora es la vista).
            problem.code == TABLE_IN_UNION &&
                capture.accountId == null &&
                capture.unionJoin == null -> {
                val details = inUnionDetails(problem)
                val accounts = unionAccounts(capture, details)
                edits.withLock {
                    load(captureId)?.let {
                        save(
                            it.copy(
                                sync = DraftSync.TABLE_HAS_ACCOUNT,
                                syncMessage = problem.message,
                                tableAccounts = accounts,
                                seenUnion = SeenUnion(details?.union?.id),
                                pendingOperationId = null,
                                inFlightCount = 0,
                            ),
                        )
                    }
                }
            }
            // Una cuenta combinada ofrece solo seguir en la cuenta en que se combinó (M3); se
            // escribe todo junto para que nunca se vea «Abrir una cuenta nueva» con ella.
            isAccountNotOpen(problem) ->
                clearInFlight(
                    captureId,
                    DraftSync.ACCOUNT_CLOSED,
                    problem.message,
                    tableAccounts = mergedTarget(accountNotOpenDetails(problem)?.mergedIntoId),
                )
            problem.code == ACCOUNT_CONFLICT || problem.code == IDEMPOTENCY_CONFLICT ->
                clearInFlight(captureId, DraftSync.CONFLICT, null)
            error is AppError.SessionEnded ||
                error is AppError.DeviceRevoked ||
                error is AppError.DeviceNotRecognized ||
                error is AppError.UpdateRequired -> {
                server.report(error, capture.waiterUserId)
                // Go no aplicó nada: se vuelve a guardar cuando el mesero vuelva a entrar.
                clearInFlight(captureId, DraftSync.PENDING, null)
            }
            error is AppError.TryLater || error is AppError.ServerProblem ->
                uncertain(captureId, problem.message)
            // Etapa L1: con la licencia, un cupo o la hora, el texto sale del código.
            else -> clearInFlight(captureId, DraftSync.REJECTED, ApiErrors.shownMessage(problem))
        }
    }

    private suspend fun clearInFlight(
        captureId: String,
        sync: DraftSync,
        message: String?,
        tableAccounts: List<TableAccountRef>? = null,
    ) {
        edits.withLock {
            load(captureId)?.let {
                save(
                    it.copy(
                        sync = sync,
                        syncMessage = message,
                        pendingOperationId = null,
                        inFlightCount = 0,
                        tableAccounts = tableAccounts ?: it.tableAccounts,
                    ),
                )
            }
        }
    }

    private fun inUnionDetails(problem: ApiProblem): TableInUnionDetailsDto? {
        val details = problem.details ?: return null
        return try {
            ApiJson.decodeFromJsonElement(TableInUnionDetailsDto.serializer(), details)
        } catch (_: SerializationException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        }
    }

    /**
     * Las cuentas abiertas de la unión (`details.accounts` son solo identificadores): su folio y su
     * nombre se leen de Go; si no se pueden leer, con el nombre de la unión.
     */
    private suspend fun unionAccounts(
        capture: Capture,
        details: TableInUnionDetailsDto?,
    ): List<TableAccountRef> {
        val ids = details?.accounts.orEmpty().take(MAXIMUM_UNION_ACCOUNTS_READ)
        val transport = server.transport()
        val headers = server.sessionHeaders(capture.waiterUserId)
        val fallback = details?.union?.name.orEmpty()
        return ids.map { id ->
            val bill =
                if (transport != null && headers != null) {
                    (api.bill(transport, headers, id) as? ApiOutcome.Success)?.value
                } else {
                    null
                }
            TableAccountRef(id, bill?.account?.folio ?: 0, bill?.account?.name ?: fallback)
        }
    }

    /**
     * Etapa M1 (M1-D13): «Enviar solo a la mesa». La captura deja la unión en pantalla y sigue como
     * una cuenta nueva de la mesa de inicio (si estaba rechazada, se vuelve a guardar).
     */
    override suspend fun dropUnion(captureId: String) {
        edits.withLock {
            val capture = load(captureId) ?: return
            if (capture.unionJoin == null || capture.accountId != null) {
                return
            }
            save(
                capture.copy(
                    unionJoin = null,
                    seenUnion = SeenUnion(null),
                    tableName = capture.unionJoin?.startName ?: capture.tableName,
                    sync =
                        if (capture.sync == DraftSync.REJECTED) {
                            DraftSync.PENDING
                        } else {
                            capture.sync
                        },
                    syncMessage = null,
                ),
            )
        }
        requestSync(captureId)
    }

    private fun tableAccounts(problem: ApiProblem): List<TableAccountRef> {
        val details = problem.details ?: return emptyList()
        return try {
            ApiJson.decodeFromJsonElement(TableHasAccountDetailsDto.serializer(), details)
                .accounts
                .map { TableAccountRef(it.id, it.folio, it.name) }
        } catch (_: SerializationException) {
            emptyList()
        } catch (_: IllegalArgumentException) {
            emptyList()
        }
    }

    /** Lee de Go la cuenta de la captura y, sin toques pendientes, la adopta como base. */
    private suspend fun refreshFromGo(captureId: String) {
        val bill = fetchBill(captureId) ?: return
        edits.withLock {
            val capture = load(captureId) ?: return
            if (capture.pendingOperationId != null || capture.sync == DraftSync.DISCARDING) {
                return
            }
            if (!bill.account.isOpen) {
                // Se cobró, se canceló o se combinó mientras no se veía (p. ej. una captura que
                // nace de «Corregir»): no es un conflicto, es una cuenta que ya no admite nada.
                save(
                    capture.copy(
                        baseRevision = bill.account.revision,
                        sync = DraftSync.ACCOUNT_CLOSED,
                        syncMessage = null,
                        tableAccounts = mergedTarget(bill.account.mergedIntoId),
                    ),
                )
            } else if (capture.baseRevision == Capture.SEEN_BASE_REVISION) {
                save(reconciled(capture, bill).copy(accountName = bill.account.name))
            } else if (capture.operations.isEmpty() || capture.baseRevision == 0) {
                save(
                    capture.copy(
                        base = bill.draft,
                        baseRevision = bill.account.revision,
                        accountName = bill.account.name,
                    ),
                )
            } else if (bill.account.revision > capture.baseRevision) {
                save(capture.copy(sync = DraftSync.CONFLICT))
            }
        }
        refreshQuote(captureId)
    }

    private suspend fun fetchBill(captureId: String): AccountBill? {
        val capture = edits.withLock { load(captureId) } ?: return null
        val accountId = capture.accountId ?: return null
        val transport = server.transport() ?: return null
        val headers = server.sessionHeaders(capture.waiterUserId) ?: return null
        return when (val outcome = api.bill(transport, headers, accountId)) {
            is ApiOutcome.Success -> WaiterMapper.bill(outcome.value)
            is ApiOutcome.Rejected -> {
                reportAccess(ApiErrors.toAppError(outcome.problem), capture.waiterUserId)
                null
            }
            else -> null
        }
    }

    /**
     * Cotización de Go del borrador guardado: la huella que exige «Enviar a cocina» y el precio por
     * unidad de cada renglón (lo que «vio» el mesero; viaja como `seen_unit_price` si el pedido
     * termina saliendo sin conexión).
     */
    private suspend fun refreshQuote(captureId: String) {
        val capture = edits.withLock { load(captureId) } ?: return
        val accountId = capture.accountId ?: return
        if (capture.base.isEmpty()) {
            // Sin borrador guardado no hay nada que cotizar.
            quotes.value = quotes.value - captureId
            return
        }
        val transport = server.transport() ?: return
        val headers = server.sessionHeaders(capture.waiterUserId) ?: return
        val outcome = api.previewDraft(transport, headers, accountId)
        if (outcome !is ApiOutcome.Success) {
            quotes.value = quotes.value - captureId
            return
        }
        val quote = WaiterMapper.quote(outcome.value)
        edits.withLock {
            val current = load(captureId) ?: return
            if (current.baseRevision != quote.revision) {
                return
            }
            quotes.value = quotes.value + (captureId to quote)
            // Las líneas de la cotización vienen en el orden del borrador guardado.
            val prices =
                current.base.zip(quote.lines).associate { (item, line) ->
                    item.id to line.unitPrice
                }
            save(current.copy(seenPrices = current.seenPrices + prices))
        }
    }

    // --- «Enviar a cocina» en línea ---

    /**
     * Con conexión, el borrador guardado sin toques pendientes y su cotización vigente, se intenta
     * confirmar en línea con la huella que vio el mesero.
     */
    private suspend fun onlineConfirmFor(capture: Capture): OnlineConfirm? {
        val accountId = capture.accountId ?: return null
        val clean =
            capture.operations.isEmpty() &&
                capture.pendingOperationId == null &&
                capture.sync == DraftSync.SAVED
        if (!clean || server.sessionHeaders(capture.waiterUserId) == null) {
            return null
        }
        // `confirmDraft` confirma el borrador completo: con un descarte pendiente en la misma
        // cuenta (lo que el mesero quitó todavía está en Go), el pedido sale sin conexión y solo
        // con lo suyo.
        val discarding =
            dao.capturesOf(capture.waiterUserId).map(WaiterStorage::toCapture).any {
                it.sync == DraftSync.DISCARDING && it.accountId == accountId
            }
        if (discarding) {
            return null
        }
        var quote = quotes.value[capture.id]
        if (quote == null || quote.revision != capture.baseRevision) {
            refreshQuote(capture.id)
            quote = quotes.value[capture.id]
        }
        if (
            quote == null || quote.revision != capture.baseRevision || quote.fingerprint.isEmpty()
        ) {
            return null
        }
        return OnlineConfirm(
            // La misma clave que el pedido (A1-D19).
            operationId = capture.id,
            accountId = accountId,
            revision = capture.baseRevision,
            fingerprint = quote.fingerprint,
        )
    }

    /**
     * Confirma en Go (`confirmDraft`). Cualquier cosa que no sea «confirmado», «la cuenta ya no
     * está abierta» o «sin permiso» deja el pedido como pedido sin conexión: Go lo revisa todo al
     * recibirlo y el mesero ve los cambios con los mensajes de Go.
     */
    private suspend fun confirmOnline(order: PendingOrder) {
        val online = order.onlineConfirm ?: return
        try {
            var outcome = confirmOnce(order, online)
            if (outcome == null) {
                // Salió y no hubo respuesta: se repite una vez con la misma clave (Go devuelve lo
                // mismo si ya lo había confirmado).
                delay(UNCERTAIN_RETRY_MILLIS)
                outcome = confirmOnce(order, online) ?: SendOutcome.ContinueOffline(null)
            }
            val current = dao.order(order.operationId)?.let(WaiterStorage::toOrder) ?: return
            if (current.state != OrderState.SENDING || current.onlineConfirm != online) {
                return
            }
            val next = OrderSendRules.after(current, outcome, clock())
            // Solo si sigue «Enviando» (nunca revive un pedido borrado; M1).
            val written =
                dao.transition(WaiterStorage.toEntity(next), OrderState.SENDING.storedName)
            if (written && next.state == OrderState.QUEUED) {
                scheduler.requestSend()
            }
        } finally {
            inFlight.remove(order.operationId)
        }
    }

    /** Un intento de `confirmDraft`; null si salió y no hubo respuesta. */
    private suspend fun confirmOnce(order: PendingOrder, online: OnlineConfirm): SendOutcome? {
        val transport = server.transport() ?: return SendOutcome.ContinueOffline(null)
        val headers =
            server.sessionHeaders(order.waiterUserId) ?: return SendOutcome.ContinueOffline(null)
        val outcome =
            api.confirmDraft(
                transport,
                headers,
                online.accountId,
                ConfirmDraftBody(
                    operationId = online.operationId,
                    revision = online.revision,
                    expectedFingerprint = online.fingerprint,
                ),
            )
        return when (outcome) {
            is ApiOutcome.Success -> {
                // §5.6: el panel de la mesa ve el total de Go en cuanto llega la respuesta.
                accountAnswered(WaiterMapper.account(outcome.value.account))
                SendOutcome.Accepted(outcome.value.account.id)
            }
            is ApiOutcome.NoAnswer ->
                if (outcome.requestSent) {
                    null
                } else {
                    SendOutcome.ContinueOffline(null)
                }
            is ApiOutcome.Malformed -> null
            is ApiOutcome.Rejected -> onlineRejection(order, outcome.problem)
        }
    }

    private fun onlineRejection(order: PendingOrder, problem: ApiProblem): SendOutcome {
        val error = ApiErrors.toAppError(problem)
        if (isAccountNotOpen(problem)) {
            return SendOutcome.AccountNotOpen(
                message = problem.message ?: ACCOUNT_NOT_OPEN_FALLBACK,
                mergedIntoId = accountNotOpenDetails(problem)?.mergedIntoId,
            )
        }
        if (error is AppError.PermissionDenied) {
            return SendOutcome.Rejected(problem.message ?: PERMISSION_FALLBACK)
        }
        // Etapa L1: la licencia, un cupo o la hora no lo permitieron. El pedido sin conexión
        // pasaría por la misma puerta de Go: queda marcado con su motivo, sin reintentarse solo.
        if (error is AppError.LicenseRefused) {
            return SendOutcome.NotAllowed(LicenseMessages.of(error.refusal))
        }
        reportAccess(error, order.waiterUserId)
        return SendOutcome.ContinueOffline(problem.message)
    }

    private fun isAccountNotOpen(problem: ApiProblem): Boolean =
        accountNotOpenDetails(problem)?.reason == ACCOUNT_NOT_OPEN

    private fun accountNotOpenDetails(problem: ApiProblem): AccountNotOpenDetailsDto? {
        val details = problem.details ?: return null
        return try {
            ApiJson.decodeFromJsonElement(AccountNotOpenDetailsDto.serializer(), details)
        } catch (_: SerializationException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        }
    }

    // --- Descartar (revisión R6, A2) ---

    /**
     * Quita del borrador de Go lo que el mesero descartó, con su sesión: primero, una apertura sin
     * respuesta se repite con su clave (si abrió la cuenta, se sabe cuál); después se quitan de la
     * cuenta los renglones suyos que siguen **igual** a alguna versión que conoce este dispositivo
     * (lo que otra terminal cambió se queda: nunca se borra en silencio lo de otro). Si la cuenta
     * nació con esta captura y queda vacía, se descarta la cuenta. Sin conexión o sin sesión, el
     * descarte queda pendiente y se repite al volver.
     */
    private suspend fun discardInGo(captureId: String) {
        var capture = edits.withLock { load(captureId) } ?: return
        val transport = server.transport() ?: return
        val headers = server.sessionHeaders(capture.waiterUserId) ?: return
        if (capture.accountId == null) {
            val operationId = capture.pendingOperationId
            if (operationId == null) {
                forget(captureId)
                return
            }
            val opened =
                api.openAccount(
                    transport,
                    headers,
                    WaiterMapper.openBody(
                        operationId = operationId,
                        mode = capture.mode,
                        tableId = capture.tableId,
                        seenAccountIds = capture.seenAccountIds,
                        customer = capture.customer,
                        items = capture.inFlightItems,
                        seenUnion = capture.seenUnion,
                        unionJoin = capture.unionJoin,
                    ),
                )
            when (opened) {
                is ApiOutcome.Success -> {
                    val bill = WaiterMapper.bill(opened.value)
                    capture =
                        capture.copy(
                            accountId = bill.account.id,
                            accountName = bill.account.name,
                            pendingOperationId = null,
                            inFlightCount = 0,
                            createdAccount = true,
                        )
                    edits.withLock { save(capture) }
                }
                is ApiOutcome.Rejected -> {
                    val error = ApiErrors.toAppError(opened.problem)
                    if (stillPending(error, capture)) {
                        return
                    }
                    // Go la rechazó: nunca se abrió y no hay nada que quitar.
                    forget(captureId)
                    return
                }
                is ApiOutcome.NoAnswer,
                is ApiOutcome.Malformed -> return
            }
        }
        repeat(MAXIMUM_DISCARD_ATTEMPTS) {
            val accountId = requireNotNull(capture.accountId)
            val bill =
                when (val read = api.bill(transport, headers, accountId)) {
                    is ApiOutcome.Success -> WaiterMapper.bill(read.value)
                    is ApiOutcome.Rejected -> {
                        if (stillPending(ApiErrors.toAppError(read.problem), capture)) {
                            return
                        }
                        forget(captureId)
                        return
                    }
                    else -> return
                }
            if (!bill.account.isOpen) {
                val merged = bill.account.mergedIntoId
                if (merged != null && merged != accountId) {
                    // La combinación movió el borrador con sus identificadores: se sigue ahí.
                    capture = capture.copy(accountId = merged, createdAccount = false)
                    edits.withLock { save(capture) }
                    return@repeat
                }
                forget(captureId)
                return
            }
            val known = capture.base + capture.visible + capture.inFlightItems
            val removable =
                bill.draft.filter { row ->
                    capture.isOwn(row.id) &&
                        known.any { version -> version.id == row.id && version.sameContentAs(row) }
                }
            if (removable.isEmpty()) {
                forget(captureId)
                return
            }
            val removableIds = removable.map(DraftLine::id).toSet()
            val remaining = bill.draft.filter { it.id !in removableIds }
            val discardAccount =
                remaining.isEmpty() && bill.lines.isEmpty() && capture.createdAccount
            val written =
                if (discardAccount) {
                    api.discardAccount(
                        transport,
                        headers,
                        accountId,
                        MutationBody(operationId = newId(), revision = bill.account.revision),
                    )
                } else {
                    api.saveDraft(
                        transport,
                        headers,
                        accountId,
                        SaveDraftBody(
                            operationId = newId(),
                            revision = bill.account.revision,
                            items = remaining.map(WaiterMapper::draftItem),
                        ),
                    )
                }
            when (written) {
                is ApiOutcome.Success -> {
                    forget(captureId)
                    return
                }
                is ApiOutcome.Rejected -> {
                    val problem = written.problem
                    when {
                        // Otra terminal escribió a la vez: se vuelve a leer.
                        problem.code == ACCOUNT_CONFLICT || isAccountNotOpen(problem) -> Unit
                        // La cuenta ya tiene algo más: solo se quitan los renglones.
                        problem.code == ACCOUNT_NOT_EMPTY -> {
                            capture = capture.copy(createdAccount = false)
                            edits.withLock { save(capture) }
                        }
                        stillPending(ApiErrors.toAppError(problem), capture) -> return
                        else -> {
                            val message = ApiErrors.shownMessage(problem)
                            edits.withLock { save(capture.copy(syncMessage = message)) }
                            return
                        }
                    }
                }
                is ApiOutcome.NoAnswer,
                is ApiOutcome.Malformed -> return
            }
        }
    }

    /** Un error de acceso deja el descarte pendiente (lo decide la entrada al dispositivo). */
    private fun stillPending(error: AppError, capture: Capture): Boolean {
        val access =
            error is AppError.SessionEnded ||
                error is AppError.DeviceRevoked ||
                error is AppError.DeviceNotRecognized ||
                error is AppError.UpdateRequired
        if (access) {
            server.report(error, capture.waiterUserId)
        }
        return access || error is AppError.TryLater || error is AppError.ServerProblem
    }

    /** El descarte terminó: la captura ya no existe en el dispositivo. */
    private suspend fun forget(captureId: String) {
        edits.withLock {
            timers.remove(captureId)?.cancel()
            dao.deleteCapture(captureId)
            quotes.value = quotes.value - captureId
        }
    }

    private fun reportAccess(error: AppError, userId: String) {
        when (error) {
            is AppError.SessionEnded,
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired -> server.report(error, userId)
            else -> Unit
        }
    }

    // --- Utilidades ---

    private suspend fun load(captureId: String): Capture? =
        dao.capture(captureId)?.let(WaiterStorage::toCapture)

    private suspend fun save(capture: Capture) {
        dao.saveCapture(WaiterStorage.toEntity(capture, clock()))
    }

    /**
     * Revisión R6, M2: una base vista sin su revisión se compara con el borrador vigente de Go. Si
     * cada renglón que vio el mesero sigue igual, el de Go pasa a ser la base y los toques se
     * aplican encima; si otra terminal cambió o quitó alguno, el conflicto se muestra («Conservar
     * mis cambios» o «Descartarlos»), nunca se guarda encima.
     */
    private fun reconciled(capture: Capture, bill: AccountBill): Capture {
        val current = bill.draft.associateBy(DraftLine::id)
        val intact = capture.base.all { seen -> current[seen.id]?.sameContentAs(seen) == true }
        if (!intact) {
            return capture.copy(sync = DraftSync.CONFLICT, syncMessage = null)
        }
        return capture.copy(
            base = bill.draft,
            baseRevision = bill.account.revision,
            sync =
                if (capture.operations.isEmpty()) {
                    DraftSync.SAVED
                } else {
                    DraftSync.PENDING
                },
            syncMessage = null,
        )
    }

    /** La cuenta en que se combinó, para ofrecer seguir ahí (M3). */
    private fun mergedTarget(mergedIntoId: String?): List<TableAccountRef> =
        listOfNotNull(mergedIntoId?.let { TableAccountRef(it, folio = 0, name = "") })

    /** Un toque nuevo: lo que estaba guardado pasa a «por guardar» (un conflicto se conserva). */
    private fun pendingStateAfterTouch(capture: Capture): DraftSync =
        when (capture.sync) {
            DraftSync.CONFLICT,
            DraftSync.TABLE_HAS_ACCOUNT,
            DraftSync.ACCOUNT_CLOSED,
            DraftSync.UNCERTAIN,
            DraftSync.SAVING -> capture.sync
            else -> DraftSync.PENDING
        }

    /**
     * La cuenta ya no está abierta (se cobró, se canceló o se combinó): lo del mesero pasa a una
     * cuenta nueva de la mesa, con renglones nuevos (los anteriores pertenecían a la otra cuenta).
     */
    private fun movedToNewAccount(capture: Capture): Capture {
        val plan = OfflineOrderPlan.of(capture)
        val renamed = plan.items.map { item -> DraftLine(newId(), item.line) }
        val prices =
            plan.items.zip(renamed).mapNotNull { (old, new) ->
                capture.seenPrices[old.id]?.let { new.id to it }
            }
        return capture.copy(
            accountId = null,
            accountName = null,
            seenAccountIds = emptyList(),
            base = emptyList(),
            baseRevision = 0,
            operations = renamed.map { DraftOperation.Add(it) },
            ownItemIds = renamed.map(DraftLine::id).toSet(),
            seenPrices = prices.toMap(),
            sync = DraftSync.PENDING,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
        )
    }

    companion object {
        /** Espera corta después del último toque antes de guardar en Go (la del escritorio). */
        const val SAVE_DELAY_MILLIS = 700L

        /** Cuánto espera «Enviar a cocina» a que termine el guardado pendiente. */
        const val SETTLE_TIMEOUT_MILLIS = 6_000L

        /**
         * Con conexión y un guardado sin respuesta, cuánto más se espera su respuesta o su
         * repetición antes de decir «Todavía se está guardando» (la lectura tarda hasta 15 s).
         */
        const val RESOLVE_TIMEOUT_MILLIS = 20_000L

        /** Lecturas y escrituras de un descarte si otra terminal escribe a la vez. */
        private const val MAXIMUM_DISCARD_ATTEMPTS = 3

        private const val UNCERTAIN_RETRY_MILLIS = 1_500L

        /** Un pedido sin conexión admite hasta 20 partidas (Go, `submitOfflineOrder`). */
        const val MAXIMUM_ORDER_LINES = 20

        private const val TABLE_HAS_ACCOUNT = "TABLE_HAS_ACCOUNT"
        private const val TABLE_IN_UNION = "TABLE_IN_UNION"

        /** Cuentas de una unión que se leen para ofrecerlas (una unión casi nunca tiene más). */
        private const val MAXIMUM_UNION_ACCOUNTS_READ = 6
        private const val ACCOUNT_CONFLICT = "ACCOUNT_CONFLICT"
        private const val ACCOUNT_NOT_EMPTY = "ACCOUNT_NOT_EMPTY"
        private const val IDEMPOTENCY_CONFLICT = "IDEMPOTENCY_CONFLICT"
        private const val ACCOUNT_NOT_OPEN = "account_not_open"
        private const val ACCOUNT_NOT_OPEN_FALLBACK = "La cuenta ya no está abierta."
        private const val PERMISSION_FALLBACK = "No tienes permiso para enviar a cocina."
    }
}
