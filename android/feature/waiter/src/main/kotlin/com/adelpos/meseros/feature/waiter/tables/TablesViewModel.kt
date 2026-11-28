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
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PendingUnion
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinting
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.SeenUnion
import com.adelpos.meseros.core.domain.waiter.TableGroup
import com.adelpos.meseros.core.domain.waiter.TableUnion
import com.adelpos.meseros.core.domain.waiter.UnionAnswer
import com.adelpos.meseros.core.domain.waiter.UnionJoin
import com.adelpos.meseros.core.domain.waiter.UnionPreview
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.feature.waiter.WaiterDependencies
import java.util.UUID
import kotlinx.coroutines.Job
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

    /** El mesero activo al abrir Mesas (sus capturas y su cola; «Tu cuenta», §5.5). */
    val userId: String? = dependencies.activeWaiter.value?.userId

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

    // --- Etapa M1: unir mesas (§1, contrato §10) ---

    /** El servidor tiene uniones de mesas (`table-unions-v1`). */
    val unionsAvailable: StateFlow<Boolean> = dependencies.unions.available

    /**
     * La unión que vive solo en la pantalla (M1-D9): todas sus mesas estaban libres y nace con el
     * primer producto. Sobrevive a la rotación y al cierre del proceso; salir a otra mesa la quita.
     */
    var pendingUnion: PendingUnion? by mutableStateOf(restorePendingUnion())
        private set

    /** El diálogo de «Unir con…», «Agregar mesa», «Quitar mesa» o el atajo de un grupo. */
    var joinDialog: JoinDialog? by mutableStateOf(null)
        private set

    private var previewJob: Job? = null

    init {
        dependencies.restaurant.refresh()
    }

    /** «Unir con…» desde [table]: se eligen las otras mesas del área. */
    fun onJoinStart(table: DiningTable) {
        joinDialog = JoinDialog(JoinMode.JOIN, startTableId = table.id, areaId = table.areaId)
    }

    /** El atajo de un grupo («Unir Mesa 3») desde una de sus secciones o desde su nombre. */
    fun onGroupJoin(group: TableGroup, start: DiningTable?) {
        val first = start ?: group.tableIds.firstNotNullOfOrNull(view.value::table) ?: return
        joinDialog =
            JoinDialog(
                JoinMode.GROUP,
                startTableId = first.id,
                areaId = group.areaId,
                groupId = group.id,
            )
        preview()
    }

    /** «Agregar mesa» a una unión abierta. */
    fun onAddStart(union: TableUnion) {
        joinDialog =
            JoinDialog(
                JoinMode.ADD,
                startTableId = union.tableIds.first(),
                areaId = union.areaId,
                unionId = union.id,
            )
    }

    /** «Quitar mesa» de una unión abierta. */
    fun onRemoveStart(union: TableUnion) {
        joinDialog =
            JoinDialog(
                JoinMode.REMOVE,
                startTableId = union.tableIds.first(),
                areaId = union.areaId,
                unionId = union.id,
            )
    }

    fun onJoinDismiss() {
        previewJob?.cancel()
        joinDialog = null
    }

    /** Elige o deja una mesa: para unir, varias; para agregar o quitar, una. */
    fun onJoinToggle(tableId: String) {
        val dialog = joinDialog ?: return
        val selected =
            when (dialog.mode) {
                JoinMode.JOIN ->
                    if (tableId in dialog.selected) {
                        dialog.selected - tableId
                    } else {
                        dialog.selected + tableId
                    }
                JoinMode.ADD,
                JoinMode.REMOVE -> setOf(tableId)
                JoinMode.GROUP -> return
            }
        joinDialog = dialog.copy(selected = selected, preview = null, message = null)
        preview()
    }

    /** La vista previa de Go de lo elegido (no escribe; quitar no la necesita). */
    private fun preview() {
        val dialog = joinDialog ?: return
        previewJob?.cancel()
        if (
            dialog.mode == JoinMode.REMOVE ||
                (dialog.mode != JoinMode.GROUP && dialog.selected.isEmpty())
        ) {
            return
        }
        joinDialog = dialog.copy(busy = true)
        previewJob = viewModelScope.launch {
            val answer =
                when (dialog.mode) {
                    JoinMode.JOIN ->
                        dependencies.unions.preview(listOf(dialog.startTableId) + dialog.ordered())
                    JoinMode.ADD ->
                        dependencies.unions.previewAdd(
                            requireNotNull(dialog.unionId),
                            dialog.ordered(),
                        )
                    JoinMode.GROUP ->
                        dependencies.unions.previewGroup(requireNotNull(dialog.groupId))
                    JoinMode.REMOVE -> return@launch
                }
            val current = joinDialog ?: return@launch
            joinDialog =
                when (answer) {
                    is UnionAnswer.Previewed ->
                        current.copy(busy = false, preview = answer.preview, message = null)
                    else -> current.copy(busy = false, preview = null, message = answer.toNote())
                }
        }
    }

    /**
     * «Unir», «Agregar» o «Quitar». Desde una mesa ocupada la unión nace en Go al momento; desde
     * una libre vive en la pantalla hasta el primer producto (M1-D9).
     */
    fun onJoinConfirm() {
        val dialog = joinDialog ?: return
        if (dialog.busy) {
            return
        }
        val start = view.value.table(dialog.startTableId)
        val startFree =
            start?.state == TableState.FREE && view.value.unionOf(dialog.startTableId) == null
        if ((dialog.mode == JoinMode.JOIN || dialog.mode == JoinMode.GROUP) && startFree) {
            val preview = dialog.preview ?: return
            if (!preview.allowed) {
                return
            }
            keepPendingUnion(
                PendingUnion(
                    startTableId = dialog.startTableId,
                    tableIds = if (dialog.groupId == null) dialog.ordered() else emptyList(),
                    groupId = dialog.groupId,
                    name = preview.name,
                    seats = preview.seats,
                ),
            )
            joinDialog = null
            return
        }
        joinDialog = dialog.copy(busy = true, message = null)
        viewModelScope.launch {
            val union = dialog.unionId?.let { id -> view.value.unions.firstOrNull { it.id == id } }
            val answer =
                when (dialog.mode) {
                    JoinMode.JOIN,
                    JoinMode.GROUP ->
                        dependencies.unions.create(
                            dialog.startTableId,
                            dialog.ordered(),
                            dialog.groupId,
                        )
                    JoinMode.ADD ->
                        if (union == null) {
                            UnionAnswer.Rejected("")
                        } else {
                            dependencies.unions.addTable(union, dialog.selected.first())
                        }
                    JoinMode.REMOVE ->
                        if (union == null) {
                            UnionAnswer.Rejected("")
                        } else {
                            dependencies.unions.removeTable(union, dialog.selected.first())
                        }
                }
            val current = joinDialog ?: return@launch
            joinDialog =
                when (answer) {
                    is UnionAnswer.Saved -> null
                    // Otra terminal liberó la mesa de inicio mientras tanto: en la pantalla.
                    UnionAnswer.Deferred -> {
                        current.preview?.let { preview ->
                            keepPendingUnion(
                                PendingUnion(
                                    startTableId = current.startTableId,
                                    tableIds =
                                        if (current.groupId == null) current.ordered()
                                        else emptyList(),
                                    groupId = current.groupId,
                                    name = preview.name,
                                    seats = preview.seats,
                                ),
                            )
                        }
                        null
                    }
                    else -> current.copy(busy = false, message = answer.toNote())
                }
        }
    }

    /** «Deshacer la unión» que vivía en la pantalla: no hay nada que quitar en Go. */
    fun onPendingUnionCancel() {
        keepPendingUnion(null)
    }

    private fun keepPendingUnion(union: PendingUnion?) {
        pendingUnion = union
        savedState[KEY_PENDING_START] = union?.startTableId
        savedState[KEY_PENDING_TABLES] = union?.tableIds?.let { ArrayList(it) }
        savedState[KEY_PENDING_GROUP] = union?.groupId
        savedState[KEY_PENDING_NAME] = union?.name
        savedState[KEY_PENDING_SEATS] = union?.seats
    }

    private fun restorePendingUnion(): PendingUnion? {
        val start = savedState.get<String>(KEY_PENDING_START) ?: return null
        return PendingUnion(
            startTableId = start,
            tableIds = savedState.get<ArrayList<String>>(KEY_PENDING_TABLES).orEmpty(),
            groupId = savedState[KEY_PENDING_GROUP],
            name = savedState[KEY_PENDING_NAME] ?: "",
            seats = savedState[KEY_PENDING_SEATS] ?: 0,
        )
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
        // La unión en pantalla se quita al elegir una mesa que no es suya (no hay nada en Go).
        val pending = pendingUnion
        if (pending != null && table.id !in pending.allTableIds(view.value)) {
            keepPendingUnion(null)
        }
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

    /**
     * Cuentas abiertas de una mesa según la última lectura; si la mesa está en una unión (etapa
     * M1), las de todas sus mesas.
     */
    fun accountsOf(tableId: String, accounts: List<AccountSummary>): List<AccountSummary> {
        val tables = view.value.unionOf(tableId)?.tableIds?.toSet() ?: setOf(tableId)
        return accounts.filter { it.tableId in tables }
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
                // Etapa M1: la unión que ve el mesero (M1-D9) y, en una cuenta nueva, la unión
                // en pantalla que nace con el primer producto. Sin la capacidad no se manda nada
                // (como la 0.4.1).
                val union = view.value.unionOf(table.id)
                val pending = pendingUnion?.takeIf {
                    account == null && it.startTableId == table.id
                }
                val seenUnion = if (unionsAvailable.value) SeenUnion(union?.id) else null
                val join = pending?.let {
                    UnionJoin(
                        tableIds = it.tableIds,
                        groupId = it.groupId,
                        name = it.name,
                        startName = table.name,
                    )
                }
                val capture =
                    dependencies.captures.open(
                        userId = waiter,
                        tableId = account?.tableId ?: table.id,
                        tableName = union?.name ?: pending?.name ?: table.name,
                        account = account,
                        seenAccountIds = seen,
                        seenUnion = seenUnion,
                        unionJoin = join,
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

        // Etapa M1: la unión en pantalla.
        const val KEY_PENDING_START = "tables.pendingUnion.start"
        const val KEY_PENDING_TABLES = "tables.pendingUnion.tables"
        const val KEY_PENDING_GROUP = "tables.pendingUnion.group"
        const val KEY_PENDING_NAME = "tables.pendingUnion.name"
        const val KEY_PENDING_SEATS = "tables.pendingUnion.seats"

        private const val STOP_TIMEOUT_MILLIS = 5_000L
        private const val TICK_MILLIS = 30_000L

        fun factory(dependencies: WaiterDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { TablesViewModel(dependencies, createSavedStateHandle()) }
            }
    }
}

/** Qué hace el diálogo de uniones (etapa M1). */
enum class JoinMode {
    /** «Unir con…» desde una mesa. */
    JOIN,

    /** El atajo de un grupo («Unir Mesa 3»). */
    GROUP,

    /** «Agregar mesa» a una unión abierta. */
    ADD,

    /** «Quitar mesa» de una unión abierta. */
    REMOVE,
}

/**
 * El diálogo de uniones: la mesa de inicio, lo elegido y la vista previa de Go con su nombre, su
 * capacidad y sus problemas (tal cual).
 */
data class JoinDialog(
    val mode: JoinMode,
    val startTableId: String,
    val areaId: String,
    val unionId: String? = null,
    val groupId: String? = null,
    val selected: Set<String> = emptySet(),
    val preview: UnionPreview? = null,
    val message: JoinNote? = null,
    val busy: Boolean = false,
) {
    /** Lo elegido en un orden estable (Go las ordena por nombre para el nombre de la unión). */
    fun ordered(): List<String> = selected.sorted()
}

/** Lo que se dice en el diálogo de uniones cuando Go no hizo lo pedido. */
sealed interface JoinNote {
    /** El mensaje de Go tal cual. */
    data class FromGo(val message: String) : JoinNote

    /** Sin conexión: unir mesas la necesita (M1-D13). */
    data object NoConnection : JoinNote

    /** Sin mensaje de Go. */
    data object Failed : JoinNote
}

private fun UnionAnswer.toNote(): JoinNote =
    when (this) {
        is UnionAnswer.Rejected ->
            if (message.isBlank()) JoinNote.Failed else JoinNote.FromGo(message)
        UnionAnswer.NoConnection -> JoinNote.NoConnection
        else -> JoinNote.Failed
    }
