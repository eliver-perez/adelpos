package com.adelpos.meseros.feature.waiter

import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.AppResult
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogQuery
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.catalog.CatalogSyncStatus
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.preferences.TablesViewPreference
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.CustomerEdit
import com.adelpos.meseros.core.domain.waiter.CustomerTarget
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import com.adelpos.meseros.core.domain.waiter.DiscardResult
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftQuote
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.OthersPending
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PrecheckService
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PrintAnswer
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.SendRequest
import com.adelpos.meseros.core.domain.waiter.SentChangeResult
import com.adelpos.meseros.core.domain.waiter.SentLineService
import com.adelpos.meseros.core.domain.waiter.TableAccountRef
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.TakeawayService
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map

/** Modo Mesero falso para las pruebas de los ViewModel (sin Room ni servidor). */
class FakeWaiter(catalog: Catalog = Catalog.EMPTY) {
    val opened = mutableListOf<Triple<String, AccountSummary?, List<String>>>()
    val staged = mutableListOf<Pair<DraftOperation, Money?>>()
    val sent = mutableListOf<String>()
    val captureFlow = MutableStateFlow<Capture?>(null)
    val permissions = MutableStateFlow(WaiterPermissions.UNKNOWN)
    val accounts = MutableStateFlow<List<AccountSummary>>(emptyList())
    val view = MutableStateFlow(RestaurantView.EMPTY)
    val storedView = MutableStateFlow(TablesView.DEFAULT)
    val edits = MutableStateFlow<List<CustomerEdit>>(emptyList())
    val customers = mutableListOf<Pair<String, TakeawayCustomer>>()

    // La precuenta (etapa A2, Fase C): respuestas de Go en orden y lo que se le pidió.
    val precheckSetup = MutableStateFlow(PrecheckSetup.UNKNOWN)
    val connected = MutableStateFlow(true)
    val printAnswers = ArrayDeque<PrintAnswer>()
    val statusAnswers = ArrayDeque<PrintAnswer>()
    val retryAnswers = ArrayDeque<PrintAnswer>()
    val printed = mutableListOf<Pair<String, String>>()
    val retried = mutableListOf<String>()
    val setupReads = mutableListOf<String>()

    val restaurant =
        object : RestaurantSource {
            override val view: StateFlow<RestaurantView> = this@FakeWaiter.view
            override val permissions: StateFlow<WaiterPermissions> = this@FakeWaiter.permissions
            override val status: StateFlow<RestaurantStatus> =
                MutableStateFlow(RestaurantStatus(loading = false, live = true, lastError = null))
            override val openAccounts: StateFlow<List<AccountSummary>> = accounts

            override fun refresh() = Unit

            override val accountChanges: Flow<String> = MutableSharedFlow()

            override suspend fun bill(accountId: String): AppResult<AccountBill> =
                AppResult.Failure(AppError.Unreachable)

            override suspend fun discardEmptyAccount(
                account: AccountSummary,
            ): DiscardAccountResult {
                discarded.add(account.id)
                return discardAnswer
            }
        }

    /** «Descartar cuenta vacía» que pidió la pantalla (etapa A3) y lo que responde Go. */
    val discarded = mutableListOf<String>()
    var discardAnswer: DiscardAccountResult = DiscardAccountResult.Discarded

    val captures =
        object : CaptureService {
            override fun captures(userId: String): Flow<List<Capture>> = captureFlow.map {
                listOfNotNull(it)
            }

            override fun capture(captureId: String): Flow<Capture?> = captureFlow

            override suspend fun open(
                userId: String,
                tableId: String,
                tableName: String,
                account: AccountSummary?,
                seenAccountIds: List<String>,
            ): Capture {
                opened.add(Triple(tableId, account, seenAccountIds))
                val capture = emptyCapture("captura-${opened.size}", tableId)
                captureFlow.value = capture
                return capture
            }

            override suspend fun openTakeaway(userId: String, account: AccountSummary?): Capture =
                error("El modo Mesero no abre pedidos para llevar")

            override suspend fun setCustomer(captureId: String, customer: TakeawayCustomer) {
                customers.add(captureId to customer)
            }

            override suspend fun stage(
                captureId: String,
                operation: DraftOperation,
                seenPrice: Money?,
            ) {
                staged.add(operation to seenPrice)
            }

            override suspend fun keepMine(captureId: String) = Unit

            override suspend fun discardMine(captureId: String) = Unit

            override suspend fun joinAccount(captureId: String, account: TableAccountRef) = Unit

            override suspend fun openAnotherAccount(captureId: String) = Unit

            override suspend fun retrySave(captureId: String) = Unit

            override suspend fun observe(captureId: String, bill: AccountBill) = Unit

            override suspend fun sendToKitchen(captureId: String, waiterName: String): SendRequest {
                sent.add(captureId)
                return SendRequest.Queued(captureId, OrderState.QUEUED)
            }

            override fun quote(captureId: String): Flow<DraftQuote?> = flowOf(null)

            override suspend fun discard(captureId: String) = DiscardResult.DONE

            override fun closeIfIdle(captureId: String) = Unit
        }

    val orders =
        object : OrderQueueService {
            override fun orders(userId: String): Flow<List<PendingOrder>> = flowOf(emptyList())

            override fun order(operationId: String): Flow<PendingOrder?> = flowOf(null)

            override fun retryNow() = Unit

            override suspend fun accept(operationId: String) = Unit

            override suspend fun removeLine(operationId: String, itemId: String) = Unit

            override suspend fun resend(operationId: String) = Unit

            override suspend fun moveToNewAccount(operationId: String) = Unit

            override suspend fun sendToAccount(operationId: String, accountId: String) = Unit

            override fun othersPending(userId: String): Flow<List<OthersPending>> =
                flowOf(emptyList())

            override suspend fun backToCapture(operationId: String): String? = null

            override suspend fun discard(operationId: String) = DiscardResult.DONE
        }

    val sentLines =
        object : SentLineService {
            override suspend fun previewEdit(
                userId: String,
                accountId: String,
                revision: Int,
                versionId: String,
                line: LineSelection,
            ): AppResult<DraftQuote> = AppResult.Failure(AppError.Unreachable)

            override suspend fun edit(
                userId: String,
                accountId: String,
                revision: Int,
                versionId: String,
                line: LineSelection,
                fingerprint: String,
                reason: String,
            ): SentChangeResult = SentChangeResult.Failed(AppError.Unreachable)

            override suspend fun cancel(
                userId: String,
                accountId: String,
                revision: Int,
                versionId: String,
                quantity: Int,
                reason: String,
            ): SentChangeResult = SentChangeResult.Failed(AppError.Unreachable)
        }

    val takeaway =
        object : TakeawayService {
            override val accounts: StateFlow<List<AccountSummary>> = this@FakeWaiter.accounts
            override val readAt: StateFlow<TakeawayRead?> = MutableStateFlow(null)

            override fun refresh() = Unit

            override fun customerEdits(): Flow<List<CustomerEdit>> = edits

            override suspend fun editCustomer(
                userId: String,
                target: CustomerTarget,
                customer: TakeawayCustomer,
            ) = Unit

            override suspend fun deliver(userId: String, accountId: String): DeliveryResult =
                DeliveryResult.NoConnection
        }

    val precheck =
        object : PrecheckService {
            override val setup: StateFlow<PrecheckSetup> = precheckSetup
            override val connected: StateFlow<Boolean> = this@FakeWaiter.connected

            override suspend fun refreshSetup(userId: String) {
                setupReads.add(userId)
            }

            override suspend fun print(
                userId: String,
                accountId: String,
                operationId: String,
            ): PrintAnswer {
                printed.add(accountId to operationId)
                return printAnswers.removeFirstOrNull() ?: PrintAnswer.NoConnection
            }

            override suspend fun status(
                userId: String,
                accountId: String,
                ticketId: String,
            ): PrintAnswer = statusAnswers.removeFirstOrNull() ?: PrintAnswer.NoConnection

            override suspend fun retry(
                userId: String,
                accountId: String,
                ticketId: String,
                operationId: String,
            ): PrintAnswer {
                retried.add(ticketId)
                return retryAnswers.removeFirstOrNull() ?: PrintAnswer.NoConnection
            }
        }

    val catalogSource =
        object : CatalogSource {
            override val catalog: StateFlow<Catalog> = MutableStateFlow(catalog)
            override val status: StateFlow<CatalogSyncStatus> =
                MutableStateFlow(CatalogSyncStatus())

            override fun requestRefresh() = Unit

            override suspend fun search(query: CatalogQuery) = catalog.products
        }

    val tablesView =
        object : TablesViewPreference {
            override val tablesView: Flow<TablesView> = storedView

            override suspend fun setTablesView(view: TablesView) {
                storedView.value = view
            }
        }

    val active =
        MutableStateFlow<ActiveWaiter?>(
            ActiveWaiter(
                WaiterProfile("ana", "Ana", "A", null, 1, null),
                hasServerSession = true,
            ),
        )

    /** El estado único de conexión que ven las pantallas (A3-D1). */
    val link = MutableStateFlow(LinkState.ONLINE)

    fun captureDependencies(): CaptureDependencies =
        CaptureDependencies(
            restaurant = restaurant,
            captures = captures,
            orders = orders,
            sentLines = sentLines,
            catalog = catalogSource,
            images = null,
            activeWaiter = active,
            takeaway = takeaway,
            precheck = precheck,
            link = link,
        )

    fun dependencies(): WaiterDependencies =
        WaiterDependencies(
            capture = captureDependencies(),
            tablesView = tablesView,
            elapsedClock = { 0L },
        )

    companion object {
        fun emptyCapture(id: String, tableId: String) =
            Capture(
                id = id,
                waiterUserId = "ana",
                tableId = tableId,
                tableName = "Mesa 4",
                accountId = null,
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
                createdAtMillis = 0,
            )
    }
}
