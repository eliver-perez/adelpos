package local.adel.pos.android.core.data.waiter

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
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiJson
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ApiProblem
import local.adel.pos.android.core.data.waiter.db.WaiterDao
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.waiter.AccountBill
import local.adel.pos.android.core.domain.waiter.AccountSummary
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.CaptureService
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.DraftQuote
import local.adel.pos.android.core.domain.waiter.DraftSync
import local.adel.pos.android.core.domain.waiter.OfflineOrderPlan
import local.adel.pos.android.core.domain.waiter.OnlineConfirm
import local.adel.pos.android.core.domain.waiter.OrderFactory
import local.adel.pos.android.core.domain.waiter.OrderSendRules
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.SendOutcome
import local.adel.pos.android.core.domain.waiter.SendRequest
import local.adel.pos.android.core.domain.waiter.TableAccountRef

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
 *   toques sin guardar, el conflicto es explícito: «Conservar mis cambios» o «Descartarlos».
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
    ): Capture {
        val existing = edits.withLock {
            dao.capturesOf(userId).map(WaiterStorage::toCapture).firstOrNull { capture ->
                capture.tableId == tableId && capture.accountId == account?.id
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
            )
        edits.withLock { save(created) }
        if (account != null) {
            // Con conexión, el borrador vigente de Go es la base (puede tener renglones de otras
            // terminales); sin ella, la captura empieza vacía y Go revisa al recibir.
            refreshFromGo(created.id)
        }
        return created
    }

    override suspend fun stage(captureId: String, operation: DraftOperation, seenPrice: Money?) {
        edits.withLock {
            val capture = load(captureId) ?: return
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
                    accountName = account.name,
                    tableAccounts = emptyList(),
                    sync = DraftSync.PENDING,
                    pendingOperationId = null,
                    inFlightCount = 0,
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
                save(movedToNewAccount(capture))
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
            if (capture.accountId != bill.account.id || capture.pendingOperationId != null) {
                // Con un guardado propio en curso, su respuesta es la autoridad.
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
                    ),
                )
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
        settle(captureId)
        val capture = edits.withLock { load(captureId) } ?: return SendRequest.NothingToSend
        val plan = OfflineOrderPlan.of(capture)
        if (plan.items.isEmpty()) {
            return SendRequest.NothingToSend
        }
        if (plan.items.size > MAXIMUM_ORDER_LINES || plan.replaces.size > MAXIMUM_ORDER_LINES) {
            return SendRequest.TooManyLines
        }
        val online = onlineConfirmFor(capture)
        val order =
            OrderFactory.fromCapture(
                capture = capture,
                waiterName = waiterName,
                // La clave del pedido es la de la captura: si vuelve a la captura para corregirlo,
                // sale con la misma.
                operationId = capture.id,
                catalog = catalog.stamp(),
                lookup = catalog.lookup,
                onlineConfirm = online,
                nowMillis = clock(),
            ) ?: return SendRequest.NothingToSend
        edits.withLock {
            dao.queueFromCapture(WaiterStorage.toEntity(order), captureId)
            timers.remove(captureId)?.cancel()
            quotes.value = quotes.value - captureId
        }
        if (online != null) {
            scope.launch { confirmOnline(order) }
        } else {
            scheduler.requestSend()
        }
        return SendRequest.Queued(order.operationId, order.state)
    }

    override suspend fun discard(captureId: String) {
        edits.withLock {
            timers.remove(captureId)?.cancel()
            dao.deleteCapture(captureId)
            quotes.value = quotes.value - captureId
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
                    capture.sync == DraftSync.SAVING
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

    /** Guarda ya lo pendiente y espera (un poco) a que termine, antes de «Enviar a cocina». */
    private suspend fun settle(captureId: String) {
        timers.remove(captureId)?.cancel()
        val done = CompletableDeferred<Unit>()
        queue.trySend(SyncRequest(captureId, done))
        withTimeoutOrNull(SETTLE_TIMEOUT_MILLIS) { done.await() }
    }

    private suspend fun syncOnce(captureId: String) {
        val capture = edits.withLock { load(captureId) } ?: return
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
                capture.baseRevision == 0 &&
                capture.pendingOperationId == null
        ) {
            // Primero el borrador vigente de Go (puede tener renglones de otras terminales): un
            // guardado sustituye el borrador completo.
            refreshFromGo(captureId)
            val loaded = edits.withLock { load(captureId) } ?: return
            if (loaded.baseRevision == 0) {
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
                    OpenAccountBody(
                        operationId = operationId,
                        mode = DINE_IN,
                        tableId = sending.tableId,
                        seenAccountIds = sending.seenAccountIds,
                        items = items.map(WaiterMapper::draftItem),
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
        val acknowledged = edits.withLock {
            val capture = load(captureId) ?: return
            val remaining = capture.operations.drop(capture.inFlightCount)
            val next =
                capture.copy(
                    accountId = bill.account.id,
                    accountName = bill.account.name,
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
            next
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
            isAccountNotOpen(problem) ->
                clearInFlight(captureId, DraftSync.ACCOUNT_CLOSED, problem.message)
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
            else -> clearInFlight(captureId, DraftSync.REJECTED, problem.message)
        }
    }

    private suspend fun clearInFlight(captureId: String, sync: DraftSync, message: String?) {
        edits.withLock {
            load(captureId)?.let {
                save(
                    it.copy(
                        sync = sync,
                        syncMessage = message,
                        pendingOperationId = null,
                        inFlightCount = 0,
                    ),
                )
            }
        }
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
            if (capture.pendingOperationId != null) {
                return
            }
            if (capture.operations.isEmpty() || capture.baseRevision == 0) {
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
            operationId = newId(),
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
        dao.saveOrder(WaiterStorage.toEntity(next))
        if (next.state == OrderState.QUEUED) {
            scheduler.requestSend()
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
            is ApiOutcome.Success -> SendOutcome.Accepted(outcome.value.account.id)
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
            return SendOutcome.AccountNotOpen(problem.message ?: ACCOUNT_NOT_OPEN_FALLBACK)
        }
        if (error is AppError.PermissionDenied) {
            return SendOutcome.Rejected(problem.message ?: PERMISSION_FALLBACK)
        }
        reportAccess(error, order.waiterUserId)
        return SendOutcome.ContinueOffline(problem.message)
    }

    private fun isAccountNotOpen(problem: ApiProblem): Boolean {
        val details = problem.details ?: return false
        return try {
            ApiJson.decodeFromJsonElement(AccountNotOpenDetailsDto.serializer(), details).reason ==
                ACCOUNT_NOT_OPEN
        } catch (_: SerializationException) {
            false
        } catch (_: IllegalArgumentException) {
            false
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

        private const val UNCERTAIN_RETRY_MILLIS = 1_500L

        /** Un pedido sin conexión admite hasta 20 partidas (Go, `submitOfflineOrder`). */
        const val MAXIMUM_ORDER_LINES = 20

        private const val DINE_IN = "dine_in"
        private const val TABLE_HAS_ACCOUNT = "TABLE_HAS_ACCOUNT"
        private const val ACCOUNT_CONFLICT = "ACCOUNT_CONFLICT"
        private const val IDEMPOTENCY_CONFLICT = "IDEMPOTENCY_CONFLICT"
        private const val ACCOUNT_NOT_OPEN = "account_not_open"
        private const val ACCOUNT_NOT_OPEN_FALLBACK = "La cuenta ya no está abierta."
        private const val PERMISSION_FALLBACK = "No tienes permiso para enviar a cocina."
    }
}
