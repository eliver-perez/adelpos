package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.AppResult
import com.adelpos.core.model.realtime.RealtimeFeed
import com.adelpos.core.model.realtime.RealtimeKinds
import com.adelpos.core.model.realtime.RealtimeScopes
import com.adelpos.core.model.realtime.events
import com.adelpos.core.model.realtime.rebuilds
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiJson
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.license.LicenseStatusRepository
import com.adelpos.meseros.core.data.waiter.db.RestaurantSnapshotEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterPermissionsEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import java.util.UUID
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
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.builtins.ListSerializer

/**
 * Mesas, cuentas abiertas y permisos del mesero activo (§6.1 y §6.2), con la sesión de **ese**
 * mesero. Lo leído de Go se guarda en Room para mostrarlo sin conexión (también después de
 * reiniciar), y el canal de tiempo real lo mantiene al momento: cada evento de los ámbitos
 * `restaurant` y `takeaway` (etapa A2) con una cuenta como agregado avisa que esa cuenta cambió
 * ([accountChanges]) y vuelve a leer las mesas y las cuentas (con una espera corta, para juntar
 * varios eventos); «reconstruir» vuelve a leer todo. Los pedidos para llevar abiertos salen de la
 * misma lectura de `restaurant/state` ([allOpenAccounts], con la hora de la lectura en
 * [accountsReadAt]: sin conexión se ve lo último, con su hora).
 *
 * @param elapsedClock reloj monótono (el tiempo de las mesas se cuenta desde la lectura).
 * @param wallClock hora del dispositivo, solo para traducir el momento de una lectura guardada.
 * @param link el estado único de conexión (etapa A3, A3-D1): al volver a «En línea» se vuelve a
 *   leer, y una lectura sin respuesta con «En línea» se repite sola a los [retryMillis] (la
 *   pantalla dice «Sin conexión» solo con el estado único, nunca por una lectura vieja).
 * @param license etapa L1: cada lectura de `restaurant/state` dice cómo está la licencia y la hora
 *   de la computadora principal; el evento `license.changed` hace volver a leer.
 */
class RestaurantRepository(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val realtime: RealtimeFeed?,
    private val scope: CoroutineScope,
    private val elapsedClock: () -> Long,
    private val wallClock: () -> Long = System::currentTimeMillis,
    private val link: StateFlow<LinkState>? = null,
    private val retryMillis: Long = RETRY_AFTER_NO_ANSWER_MILLIS,
    private val newId: () -> String = { UUID.randomUUID().toString() },
    private val license: LicenseStatusRepository? = null,
) : RestaurantSource {
    private val mutableView = MutableStateFlow(RestaurantView.EMPTY)
    private val mutablePermissions = MutableStateFlow(WaiterPermissions.UNKNOWN)
    private val mutableStatus = MutableStateFlow(RestaurantStatus(false, false, null))
    private val mutableAccounts = MutableStateFlow<List<AccountSummary>>(emptyList())
    private val mutableAllAccounts = MutableStateFlow<List<AccountSummary>>(emptyList())
    private val mutableReadAt = MutableStateFlow<TakeawayRead?>(null)
    private val changes = MutableSharedFlow<String>(extraBufferCapacity = CHANGES_BUFFER)
    private val refreshing = Mutex()
    private var pendingRefresh: Job? = null
    private var started = false

    override val view: StateFlow<RestaurantView> = mutableView.asStateFlow()
    override val permissions: StateFlow<WaiterPermissions> = mutablePermissions.asStateFlow()
    override val status: StateFlow<RestaurantStatus> = mutableStatus.asStateFlow()
    override val openAccounts: StateFlow<List<AccountSummary>> = mutableAccounts.asStateFlow()
    override val accountChanges: Flow<String> = changes.asSharedFlow()

    /**
     * Todas las cuentas abiertas de la última lectura, de salón y para llevar (las para llevar solo
     * vienen con `takeaway.read`).
     */
    val allOpenAccounts: StateFlow<List<AccountSummary>> = mutableAllAccounts.asStateFlow()

    /** Cuándo se leyeron esas cuentas (null si nunca). */
    val accountsReadAt: StateFlow<TakeawayRead?> = mutableReadAt.asStateFlow()

    /** Lee lo guardado y empieza a seguir al mesero activo y el canal de tiempo real. */
    fun start() {
        if (started) {
            return
        }
        started = true
        followLink()
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
        // Para llevar (etapa A2): los pedidos para llevar avisan en su propio ámbito.
        scope.launch {
            feed.events(RealtimeScopes.TAKEAWAY).collect { event ->
                event.aggregateId?.let { changes.tryEmit(it) }
                refreshSoon()
            }
        }
        // Etapa L1: cambió la licencia o el bloqueo por la hora; el evento no trae datos (se vuelve
        // a leer el estado, que los trae).
        scope.launch {
            feed
                .events(RealtimeScopes.CONFIGURATION, setOf(RealtimeKinds.LICENSE_CHANGED))
                .collect { refreshSoon() }
        }
        scope.launch { feed.rebuilds().collect { refreshSoon() } }
    }

    // Etapa A3: lo que dice la pantalla sale del estado único de conexión; al volver a «En línea»
    // se vuelve a leer, para no quedarse con la lectura de antes.
    private fun followLink() {
        val state = link ?: return
        scope.launch { state.drop(1).filter { it == LinkState.ONLINE }.collect { refreshSoon() } }
    }

    override fun refresh() {
        scope.launch { refreshNow() }
    }

    /** Vuelve a leer después de una pausa corta: varios eventos seguidos dan una sola lectura. */
    private fun refreshSoon(delayMillis: Long = REFRESH_DELAY_MILLIS) {
        pendingRefresh?.cancel()
        pendingRefresh = scope.launch {
            delay(delayMillis)
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
            // Reintento sin decir nada: Go no respondió pero el estado único sigue «En línea» (una
            // falla aislada). Con «Sin conexión» se espera a que vuelva (followLink).
            if (error == AppError.Unreachable && link?.value == LinkState.ONLINE) {
                refreshSoon(retryMillis)
            }
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

    override suspend fun discardEmptyAccount(account: AccountSummary): DiscardAccountResult {
        val userId = server.activeWaiter.value?.userId ?: return DiscardAccountResult.NoConnection
        val transport = server.transport() ?: return DiscardAccountResult.NoConnection
        val headers = server.sessionHeaders(userId) ?: return DiscardAccountResult.NoConnection
        val body = MutationBody(operationId = newId(), revision = account.revision)
        var outcome = api.discardAccount(transport, headers, account.id, body)
        if (outcome is ApiOutcome.NoAnswer && outcome.requestSent) {
            // Pudo haber entrado: se repite **igual** (misma clave) y Go no descarta dos veces.
            delay(DISCARD_RETRY_MILLIS)
            outcome = api.discardAccount(transport, headers, account.id, body)
        }
        val result =
            when (outcome) {
                is ApiOutcome.Success -> DiscardAccountResult.Discarded
                is ApiOutcome.Rejected ->
                    when (outcome.problem.code) {
                        ACCOUNT_NOT_EMPTY,
                        ACCOUNT_CONFLICT -> DiscardAccountResult.Changed
                        else -> {
                            failure(outcome, userId)
                            DiscardAccountResult.Failed(ApiErrors.shownMessage(outcome.problem))
                        }
                    }
                is ApiOutcome.NoAnswer -> DiscardAccountResult.NoConnection
                is ApiOutcome.Malformed -> DiscardAccountResult.Failed(null)
            }
        // La lista se vuelve a leer en cualquier caso (el canal también avisa).
        refresh()
        return result
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
        license?.apply(state.license)
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
        // La hora primero: quien ve las cuentas nuevas ya ve también cuándo se leyeron.
        mutableReadAt.value = TakeawayRead(wallMillis = wallClock(), elapsedMillis = elapsedClock())
        mutableAllAccounts.value = openAnyMode(accounts)
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
        if (accounts != null && mutableReadAt.value == null) {
            val saved = ApiJson.decodeFromString(ACCOUNTS, accounts.document)
            mutableAccounts.value = openDineIn(saved)
            val passed = maxOf(0L, wallClock() - accounts.readAt)
            mutableReadAt.value =
                TakeawayRead(wallMillis = accounts.readAt, elapsedMillis = elapsedClock() - passed)
            mutableAllAccounts.value = openAnyMode(saved)
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

    private fun openAnyMode(accounts: List<AccountDto>): List<AccountSummary> =
        accounts.filter { it.state == AccountSummary.STATE_OPEN }.map(WaiterMapper::account)

    companion object {
        private const val KIND_OVERVIEW = "overview"
        private const val KIND_ACCOUNTS = "accounts"
        private const val DINE_IN = "dine_in"
        private const val REFRESH_DELAY_MILLIS = 400L

        /**
         * Una lectura sin respuesta con el estado «En línea» se repite a los 5 s, el mismo ritmo de
         * la sonda mientras el estado duda (`ServerLinkTracker.DOUBT_PROBE_MILLIS`).
         */
        private const val RETRY_AFTER_NO_ANSWER_MILLIS = 5_000L

        /** Pausa antes de repetir con la misma clave un descarte sin respuesta. */
        private const val DISCARD_RETRY_MILLIS = 1_500L
        private const val ACCOUNT_NOT_EMPTY = "ACCOUNT_NOT_EMPTY"
        private const val ACCOUNT_CONFLICT = "ACCOUNT_CONFLICT"
        private const val CHANGES_BUFFER = 64
        private val ACCOUNTS = ListSerializer(AccountDto.serializer())
    }
}
