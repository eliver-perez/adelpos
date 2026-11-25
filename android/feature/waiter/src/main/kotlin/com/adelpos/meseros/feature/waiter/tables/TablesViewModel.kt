package com.adelpos.meseros.feature.waiter.tables

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.createSavedStateHandle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.adelpos.core.model.connection.LinkState
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinting
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.feature.waiter.WaiterDependencies
import java.util.UUID
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * Estado de «Mesas» (§6.1 y §6.2): áreas y mesas con su estado y tiempo (de Go), la vista de lista
 * o de mapa (recordada en el dispositivo), la búsqueda, la mesa elegida con sus cuentas abiertas y
 * lo del mesero en ella (captura en curso y pedidos de la cola).
 *
 * La búsqueda, la mesa elegida, el área del mapa y el panel abierto en el teléfono se guardan en
 * [SavedStateHandle]: sobreviven a la rotación y al cierre del proceso.
 */
class TablesViewModel(
    private val dependencies: WaiterDependencies,
    private val savedState: SavedStateHandle,
) : ViewModel() {
    val view: StateFlow<RestaurantView> = dependencies.restaurant.view
    val permissions: StateFlow<WaiterPermissions> = dependencies.restaurant.permissions
    val status: StateFlow<RestaurantStatus> = dependencies.restaurant.status

    /** El estado único de conexión (etapa A3, A3-D1): «Sin conexión» solo con él. */
    val link: StateFlow<LinkState> = dependencies.link
    val openAccounts: StateFlow<List<AccountSummary>> = dependencies.restaurant.openAccounts

    private val userId: String? = dependencies.activeWaiter.value?.userId

    val tablesView: StateFlow<TablesView> =
        dependencies.tablesView.tablesView.stateIn(
            viewModelScope,
            SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
            TablesView.DEFAULT,
        )

    /** Capturas en curso del mesero activo (solo las suyas). */
    val captures: StateFlow<List<Capture>> = flowOfUser {
        dependencies.captures.captures(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    /** Pedidos de la cola del mesero activo (solo los suyos). */
    val orders: StateFlow<List<PendingOrder>> = flowOfUser {
        dependencies.orders.orders(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    /**
     * Reloj monótono que avanza cada medio minuto para el tiempo de las mesas, solo mientras la
     * pantalla lo mira.
     */
    val now: StateFlow<Long> = flow {
        while (true) {
            emit(dependencies.elapsedClock())
            delay(TICK_MILLIS)
        }
    }
        .stateIn(
            viewModelScope,
            SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
            dependencies.elapsedClock(),
        )

    var showingAccount: Boolean by mutableStateOf(savedState[KEY_SHOWING_ACCOUNT] ?: false)
        private set

    var selectedTableId: String? by mutableStateOf(savedState[KEY_SELECTED_TABLE])
        private set

    /** El área elegida en la fila de arriba (lista y mapa, etapa A3, §4.7). */
    var areaId: String? by mutableStateOf(savedState[KEY_AREA])
        private set

    /** Mientras se abre una captura (la cuenta se lee de Go): evita tocar dos veces. */
    var opening: Boolean by mutableStateOf(false)
        private set

    // --- La precuenta desde el panel de la mesa (etapa A3, §4.9) ---

    private val printing =
        PrecheckPrinting(dependencies.capture.precheck, viewModelScope, newId = ::newId)

    /** La cuenta cuya precuenta se está imprimiendo o se imprimió por última vez (una a la vez). */
    var printingAccountId: String? by mutableStateOf(null)
        private set

    val precheckSetup: StateFlow<PrecheckSetup> = dependencies.capture.precheck.setup

    /** Cómo va la impresión de [printingAccountId]. */
    val precheckState: StateFlow<PrecheckPrintState> = printing.state

    /**
     * Hay sesión de Go y conexión (el estado único): imprimir y descartar nunca entran en la cola.
     */
    val canWriteNow: StateFlow<Boolean> =
        combine(dependencies.activeWaiter, dependencies.capture.precheck.connected) {
                waiter,
                connected ->
                waiter?.hasServerSession == true && !waiter.mustReenterPin && connected
            }
            .stateIn(viewModelScope, SharingStarted.Eagerly, false)

    // --- «Descartar cuenta vacía» (etapa A3, §4.10) ---

    /** La cuenta que se está descartando (el botón espera la respuesta). */
    var discardingAccountId: String? by mutableStateOf(null)
        private set

    /** Lo que respondió Go al último descarte (se muestra en el panel hasta otro). */
    var discardResult: DiscardAccountResult? by mutableStateOf(null)
        private set

    init {
        dependencies.restaurant.refresh()
    }

    /** «Imprimir precuenta» de una cuenta del panel: la anterior que se seguía se olvida. */
    fun onPrintPrecheck(account: AccountSummary) {
        val waiter = userId ?: return
        if (printingAccountId != account.id) {
            printing.reset()
            printingAccountId = account.id
        }
        printing.print(waiter, account.id)
    }

    /** «Reintentar» la precuenta que nunca salió en papel (regla de la 9F). */
    fun onRetryPrecheck(account: AccountSummary) {
        val waiter = userId ?: return
        if (printingAccountId == account.id) {
            printing.retry(waiter, account.id)
        }
    }

    /** «Descartar cuenta vacía» confirmado: con conexión y la sesión del mesero. */
    fun onDiscardEmpty(account: AccountSummary) {
        if (discardingAccountId != null || !account.discardable) {
            return
        }
        discardingAccountId = account.id
        discardResult = null
        viewModelScope.launch {
            try {
                discardResult = dependencies.restaurant.discardEmptyAccount(account)
            } finally {
                discardingAccountId = null
            }
        }
    }

    private fun newId(): String = UUID.randomUUID().toString()

    private fun <T> flowOfUser(block: (String) -> Flow<List<T>>): Flow<List<T>> =
        if (userId == null) {
            flowOf(emptyList())
        } else {
            block(userId)
        }

    fun onShowingAccountChange(showing: Boolean) {
        showingAccount = showing
        savedState[KEY_SHOWING_ACCOUNT] = showing
    }

    fun onTableSelect(table: DiningTable) {
        selectedTableId = table.id
        savedState[KEY_SELECTED_TABLE] = table.id
        onShowingAccountChange(true)
    }

    fun onAreaSelect(areaId: String) {
        this.areaId = areaId
        savedState[KEY_AREA] = areaId
    }

    fun onTablesViewChange(view: TablesView) {
        viewModelScope.launch { dependencies.tablesView.setTablesView(view) }
    }

    fun refresh() {
        dependencies.restaurant.refresh()
    }

    /** Cuentas abiertas de una mesa según la última lectura. */
    fun accountsOf(tableId: String, accounts: List<AccountSummary>): List<AccountSummary> =
        accounts.filter {
            it.tableId == tableId
        }

    /**
     * Abre la captura del mesero en [account] (o en una cuenta nueva de la mesa, que nace con el
     * primer producto) y avisa su identificador.
     */
    fun openCapture(table: DiningTable, account: AccountSummary?, onOpened: (String) -> Unit) {
        val waiter = userId ?: return
        if (opening) {
            return
        }
        if (account == null && !permissions.value.allows(WaiterAction.OPEN_ACCOUNT)) {
            return
        }
        opening = true
        viewModelScope.launch {
            try {
                val seen = accountsOf(table.id, openAccounts.value).map { it.id }
                val capture =
                    dependencies.captures.open(
                        userId = waiter,
                        tableId = table.id,
                        tableName = table.name,
                        account = account,
                        seenAccountIds = seen,
                    )
                onOpened(capture.id)
            } finally {
                opening = false
            }
        }
    }

    companion object {
        const val KEY_SHOWING_ACCOUNT = "tables.showingAccount"
        const val KEY_SELECTED_TABLE = "tables.selected"

        /** La misma clave de la A2 (el área del mapa): ahora vale para la lista y el mapa. */
        const val KEY_AREA = "tables.mapArea"

        private const val STOP_TIMEOUT_MILLIS = 5_000L
        private const val TICK_MILLIS = 30_000L

        fun factory(dependencies: WaiterDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { TablesViewModel(dependencies, createSavedStateHandle()) }
            }
    }
}
