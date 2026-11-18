package com.adelpos.meseros.feature.takeaway

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CustomerTarget
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.TakeawayBoard
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayEntry
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
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
 * Lo que necesitan las pantallas de Para llevar (inyección manual desde `AppContainer`): la captura
 * compartida de `core:capture` y los relojes (el monótono para el tiempo de los pedidos de Go y la
 * hora del dispositivo para los que todavía no nacen en Go).
 */
class TakeawayDependencies(
    val capture: CaptureDependencies,
    val elapsedClock: () -> Long,
    val wallClock: () -> Long = System::currentTimeMillis,
)

/** Aviso de una sola vez de la lista (después de «Entregado»). */
enum class TakeawayNotice {
    DELIVERED,
    COLLECT_FIRST,
    ALREADY_DELIVERED,
    NO_CONNECTION,
    FAILED,
}

/** El reloj de la lista: el monótono y la hora del dispositivo en el mismo momento. */
data class TakeawayClock(val elapsedMillis: Long, val wallMillis: Long)

/**
 * La lista de Para llevar del mesero activo (§3.3): los pedidos para llevar abiertos de Go con su
 * preparación, pago y entrega, y lo de este dispositivo que todavía no nace en Go («Por enviar» y
 * las capturas sin enviar a cocina). «Nuevo pedido» entra directo al catálogo (§3.2); «Nombre y
 * teléfono» en cualquier momento (A2-D1); «Entregado» solo pagado y con permiso (A2-D2).
 */
class TakeawayViewModel(private val dependencies: TakeawayDependencies) : ViewModel() {
    private val capture = dependencies.capture
    private val userId: String? = capture.activeWaiter.value?.userId

    val permissions: StateFlow<WaiterPermissions> = capture.restaurant.permissions

    val status: StateFlow<RestaurantStatus> = capture.restaurant.status

    val readAt: StateFlow<TakeawayRead?> = capture.takeaway.readAt

    val entries: StateFlow<List<TakeawayEntry>> =
        combine(
                capture.takeaway.accounts,
                userFlow { capture.orders.orders(it) },
                userFlow { capture.captures.captures(it) },
                capture.takeaway.customerEdits(),
            ) { accounts, orders, captures, edits ->
                TakeawayBoard.entries(accounts, orders, captures, edits)
            }
            .stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    /** El tiempo de cada pedido avanza aunque no haya lecturas nuevas (cada 30 s). */
    val clock: StateFlow<TakeawayClock> = flow {
        while (true) {
            emit(now())
            delay(TICK_MILLIS)
        }
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), now())

    /** El pedido con «Entregado» en curso (su clave), para no mandarlo dos veces. */
    var delivering: String? by mutableStateOf(null)
        private set

    var notice: TakeawayNotice? by mutableStateOf(null)
        private set

    /** El mensaje de Go del último rechazo de «Entregado», si lo hubo. */
    var noticeMessage: String? by mutableStateOf(null)
        private set

    /** El mesero tiene sesión de Go: «Entregado» exige conexión (no va a la cola). */
    val online: Boolean
        get() = capture.activeWaiter.value?.hasServerSession == true

    init {
        refresh()
    }

    fun refresh() {
        capture.takeaway.refresh()
    }

    /** «Nuevo pedido»: una captura nueva que entra directo al catálogo (sin formularios). */
    fun newOrder(onCapture: (String) -> Unit) {
        val user = userId ?: return
        if (!permissions.value.allows(WaiterAction.OPEN_TAKEAWAY)) {
            return
        }
        viewModelScope.launch { onCapture(capture.captures.openTakeaway(user, account = null).id) }
    }

    /**
     * Tocar un pedido: seguir capturando en él (con `takeaway.manage`), retomar una captura sin
     * enviar o revisar un pedido que lo requiere. Sin el permiso de capturar, solo se ve la lista.
     */
    fun open(entry: TakeawayEntry, onCapture: (String) -> Unit, onReview: (String) -> Unit) {
        val user = userId ?: return
        when (entry) {
            is TakeawayEntry.InGo -> {
                if (!permissions.value.allows(WaiterAction.MANAGE_TAKEAWAY)) {
                    return
                }
                val resumed = entry.captureId
                if (resumed != null) {
                    onCapture(resumed)
                    return
                }
                viewModelScope.launch {
                    onCapture(capture.captures.openTakeaway(user, entry.account).id)
                }
            }
            is TakeawayEntry.Draft -> onCapture(entry.capture.id)
            is TakeawayEntry.Queued ->
                if (entry.order.state == OrderState.NEEDS_REVIEW) {
                    onReview(entry.order.operationId)
                }
        }
    }

    /** «Nombre y teléfono» desde la lista (A2-D1). */
    fun editCustomer(entry: TakeawayEntry, customer: TakeawayCustomer) {
        val user = userId ?: return
        viewModelScope.launch {
            when (entry) {
                is TakeawayEntry.InGo ->
                    capture.takeaway.editCustomer(
                        user,
                        CustomerTarget.Account(entry.account.id),
                        customer,
                    )
                is TakeawayEntry.Queued ->
                    capture.takeaway.editCustomer(
                        user,
                        CustomerTarget.QueuedOrder(entry.order.operationId),
                        customer,
                    )
                is TakeawayEntry.Draft -> capture.captures.setCustomer(entry.capture.id, customer)
            }
        }
    }

    /** «Entregado» (A2-D2): con conexión, a nombre del mesero activo. */
    fun deliver(entry: TakeawayEntry.InGo) {
        val user = userId ?: return
        if (delivering != null) {
            return
        }
        delivering = entry.key
        viewModelScope.launch {
            try {
                val result = capture.takeaway.deliver(user, entry.account.id)
                noticeMessage = (result as? DeliveryResult.Failed)?.message
                notice =
                    when (result) {
                        is DeliveryResult.Delivered -> TakeawayNotice.DELIVERED
                        DeliveryResult.CollectFirst -> TakeawayNotice.COLLECT_FIRST
                        DeliveryResult.AlreadyDelivered -> TakeawayNotice.ALREADY_DELIVERED
                        DeliveryResult.NoConnection -> TakeawayNotice.NO_CONNECTION
                        is DeliveryResult.Failed -> TakeawayNotice.FAILED
                    }
            } finally {
                delivering = null
            }
        }
    }

    fun onNoticeShown() {
        notice = null
        noticeMessage = null
    }

    private fun now(): TakeawayClock =
        TakeawayClock(
            elapsedMillis = dependencies.elapsedClock(),
            wallMillis = dependencies.wallClock(),
        )

    private fun <T> userFlow(block: (String) -> Flow<List<T>>): Flow<List<T>> =
        userId?.let(block) ?: flowOf(emptyList())

    companion object {
        private const val TICK_MILLIS = 30_000L
        private const val STOP_TIMEOUT_MILLIS = 5_000L

        fun factory(dependencies: TakeawayDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { TakeawayViewModel(dependencies) }
            }
    }
}

/** Lo que el mesero ve como «tiempo» de un pedido que todavía no nace en Go. */
internal fun localSinceMillis(entry: TakeawayEntry): Long? =
    when (entry) {
        is TakeawayEntry.Queued -> entry.order.capturedAtMillis
        is TakeawayEntry.Draft -> entry.capture.createdAtMillis
        is TakeawayEntry.InGo -> null
    }

/** El pedido de la cola o la captura de un renglón, si lo hay (para la etiqueta de estado). */
internal fun localState(entry: TakeawayEntry): OrderState? =
    when (entry) {
        is TakeawayEntry.Queued -> entry.order.state
        is TakeawayEntry.Draft -> OrderState.DRAFT
        is TakeawayEntry.InGo -> entry.queued?.state ?: entry.captureId?.let { OrderState.DRAFT }
    }

/** Productos de un pedido que todavía no nace en Go («2 × Hamburguesa»). */
internal fun localLines(order: PendingOrder): String =
    order.lines.joinToString(", ") { "${it.item.line.quantity} × ${it.productName}" }

/** Cantidad de productos de una captura sin enviar a cocina. */
internal fun draftCount(capture: Capture): Int =
    capture.visible.filter { capture.isOwn(it.id) }.sumOf { it.line.quantity }
