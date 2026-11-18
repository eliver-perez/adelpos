package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.api.ApiJson
import com.adelpos.meseros.core.data.api.ApiOutcome
import com.adelpos.meseros.core.data.waiter.db.RestaurantSnapshotEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterPermissionsEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.error.AppResult
import com.adelpos.meseros.core.domain.realtime.RealtimeFeed
import com.adelpos.meseros.core.domain.realtime.RealtimeScopes
import com.adelpos.meseros.core.domain.realtime.events
import com.adelpos.meseros.core.domain.realtime.rebuilds
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.builtins.ListSerializer

/**
 * Mesas, cuentas abiertas y permisos del mesero activo (§6.1 y §6.2), con la sesión de **ese**
 * mesero. Lo leído de Go se guarda en Room para mostrarlo sin conexión (también después de
 * reiniciar), y el canal de tiempo real lo mantiene al momento: cada evento del ámbito `restaurant`
 * con una cuenta como agregado avisa que esa cuenta cambió ([accountChanges]) y vuelve a leer las
 * mesas (con una espera corta, para juntar varios eventos); «reconstruir» vuelve a leer todo.
 *
 * @param elapsedClock reloj monótono (el tiempo de las mesas se cuenta desde la lectura).
 * @param wallClock hora del dispositivo, solo para traducir el momento de una lectura guardada.
 */
class RestaurantRepository(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val realtime: RealtimeFeed?,
    private val scope: CoroutineScope,
    private val elapsedClock: () -> Long,
    private val wallClock: () -> Long = System::currentTimeMillis,
) : RestaurantSource {
    private val mutableView = MutableStateFlow(RestaurantView.EMPTY)
    private val mutablePermissions = MutableStateFlow(WaiterPermissions.UNKNOWN)
    private val mutableStatus = MutableStateFlow(RestaurantStatus(false, false, null))
    private val mutableAccounts = MutableStateFlow<List<AccountSummary>>(emptyList())
    private val changes = MutableSharedFlow<String>(extraBufferCapacity = CHANGES_BUFFER)
    private val refreshing = Mutex()
    private var pendingRefresh: Job? = null
    private var started = false

    override val view: StateFlow<RestaurantView> = mutableView.asStateFlow()
    override val permissions: StateFlow<WaiterPermissions> = mutablePermissions.asStateFlow()
    override val status: StateFlow<RestaurantStatus> = mutableStatus.asStateFlow()
    override val openAccounts: StateFlow<List<AccountSummary>> = mutableAccounts.asStateFlow()
    override val accountChanges: Flow<String> = changes.asSharedFlow()

    /** Lee lo guardado y empieza a seguir al mesero activo y el canal de tiempo real. */
    fun start() {
        if (started) {
            return
        }
        started = true
        scope.launch { loadSaved() }
        scope.launch {
            server.activeWaiter
                .map { it?.takeIf { waiter -> waiter.hasServerSession }?.userId to it?.userId }
                .distinctUntilChanged()
                .collect { (withSession, active) ->
                    mutablePermissions.value = savedPermissions(active)
                    if (withSession != null) {
                        refreshNow()
                    }
                }
        }
        val feed = realtime ?: return
        scope.launch {
            feed.events(RealtimeScopes.RESTAURANT).collect { event ->
                event.aggregateId?.let { changes.tryEmit(it) }
                refreshSoon()
            }
        }
        scope.launch { feed.rebuilds().collect { refreshSoon() } }
    }

    override fun refresh() {
        scope.launch { refreshNow() }
    }

    /** Vuelve a leer después de una pausa corta: varios eventos seguidos dan una sola lectura. */
    private fun refreshSoon() {
        pendingRefresh?.cancel()
        pendingRefresh = scope.launch {
            delay(REFRESH_DELAY_MILLIS)
            refreshNow()
        }
    }

    private suspend fun refreshNow() {
        refreshing.withLock {
            val userId = server.activeWaiter.value?.userId ?: return
            val transport = server.transport()
            val headers = server.sessionHeaders(userId)
            if (transport == null || headers == null) {
                mutableStatus.value = mutableStatus.value.copy(loading = false, live = false)
                return
            }
            mutableStatus.value = mutableStatus.value.copy(loading = true)
            val state = api.state(transport, headers)
            val overview = api.overview(transport, headers)
            var error: AppError? = null
            when (state) {
                is ApiOutcome.Success -> applyState(userId, state.value)
                else -> error = failure(state, userId)
            }
            when (overview) {
                is ApiOutcome.Success -> applyOverview(overview.value)
                else -> error = error ?: failure(overview, userId)
            }
            mutableStatus.value =
                RestaurantStatus(
                    loading = false,
                    live = error == null,
                    lastError = error,
                )
        }
    }

    override suspend fun bill(accountId: String): AppResult<AccountBill> {
        val userId =
            server.activeWaiter.value?.userId ?: return AppResult.Failure(AppError.Unreachable)
        val transport = server.transport() ?: return AppResult.Failure(AppError.Unreachable)
        val headers =
            server.sessionHeaders(userId) ?: return AppResult.Failure(AppError.Unreachable)
        return when (val outcome = api.bill(transport, headers, accountId)) {
            is ApiOutcome.Success -> AppResult.Success(WaiterMapper.bill(outcome.value))
            else -> AppResult.Failure(failure(outcome, userId))
        }
    }

    /** El error de una lectura; los de acceso (sesión, dispositivo, versión) van al punto único. */
    private fun failure(outcome: ApiOutcome<*>, userId: String): AppError {
        val error = outcome.toAppResult().let { (it as AppResult.Failure).error }
        when (error) {
            is AppError.SessionEnded,
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired,
            is AppError.ServerIdentityChanged -> server.report(error, userId)
            else -> Unit
        }
        return error
    }

    private suspend fun applyState(userId: String, state: RecoveryStateDto) {
        val permissions = WaiterPermissions(state.user.permissions.toSet())
        mutablePermissions.value = permissions
        dao.savePermissions(
            WaiterPermissionsEntity(
                userId = userId,
                permissions = WaiterStorage.encodeStrings(state.user.permissions),
                readAt = wallClock(),
            ),
        )
        val accounts = state.accounts ?: return
        mutableAccounts.value = openDineIn(accounts)
        dao.saveSnapshot(
            RestaurantSnapshotEntity(
                kind = KIND_ACCOUNTS,
                document = ApiJson.encodeToString(ACCOUNTS, accounts),
                readAt = wallClock(),
            ),
        )
    }

    private suspend fun applyOverview(overview: RestaurantOverviewDto) {
        mutableView.value = viewOf(overview, readAtElapsed = elapsedClock())
        dao.saveSnapshot(
            RestaurantSnapshotEntity(
                kind = KIND_OVERVIEW,
                document = ApiJson.encodeToString(RestaurantOverviewDto.serializer(), overview),
                readAt = wallClock(),
            ),
        )
    }

    private suspend fun loadSaved() {
        val overview = dao.snapshot(KIND_OVERVIEW)
        if (overview != null && mutableView.value === RestaurantView.EMPTY) {
            val dto =
                ApiJson.decodeFromString(RestaurantOverviewDto.serializer(), overview.document)
            // El momento de la lectura en el reloj monótono de este arranque.
            val passed = maxOf(0L, wallClock() - overview.readAt)
            mutableView.value = viewOf(dto, readAtElapsed = elapsedClock() - passed)
        }
        val accounts = dao.snapshot(KIND_ACCOUNTS)
        if (accounts != null && mutableAccounts.value.isEmpty()) {
            mutableAccounts.value =
                openDineIn(ApiJson.decodeFromString(ACCOUNTS, accounts.document))
        }
        mutablePermissions.value = savedPermissions(server.activeWaiter.value?.userId)
    }

    private suspend fun savedPermissions(userId: String?): WaiterPermissions {
        val id = userId ?: return WaiterPermissions.UNKNOWN
        val saved = dao.permissions(id) ?: return WaiterPermissions.UNKNOWN
        return WaiterPermissions(WaiterStorage.decodeStrings(saved.permissions).toSet())
    }

    private fun viewOf(dto: RestaurantOverviewDto, readAtElapsed: Long): RestaurantView =
        RestaurantView(
            areas = dto.areas.map(WaiterMapper::area),
            tables = dto.tables.mapNotNull(WaiterMapper::table),
            settings = dto.settings?.let(WaiterMapper::settings),
            readAtElapsedMillis = readAtElapsed,
        )

    private fun openDineIn(accounts: List<AccountDto>): List<AccountSummary> =
        accounts
            .filter { it.mode == DINE_IN && it.state == AccountSummary.STATE_OPEN }
            .map(WaiterMapper::account)

    companion object {
        private const val KIND_OVERVIEW = "overview"
        private const val KIND_ACCOUNTS = "accounts"
        private const val DINE_IN = "dine_in"
        private const val REFRESH_DELAY_MILLIS = 400L
        private const val CHANGES_BUFFER = 64
        private val ACCOUNTS = ListSerializer(AccountDto.serializer())
    }
}
