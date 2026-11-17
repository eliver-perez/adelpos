package local.adel.pos.android.feature.waiter.pending

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
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.feature.waiter.WaiterDependencies

/**
 * «Pedidos pendientes» (§7): lo que el mesero activo tiene sin enviar (sus capturas y sus pedidos
 * de la cola con su estado) y lo recibido hace poco; «Reintentar ya». De los demás meseros solo se
 * dice cuántos pedidos tienen por enviar en este dispositivo, nunca cuáles.
 */
class PendingOrdersViewModel(private val dependencies: WaiterDependencies) : ViewModel() {
    private val userId: String? = dependencies.activeWaiter.value?.userId

    val orders: StateFlow<List<PendingOrder>> = userFlow {
        dependencies.orders.orders(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    val captures: StateFlow<List<Capture>> = userFlow {
        dependencies.captures.captures(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    val others: StateFlow<Int> =
        (userId?.let(dependencies.pendingOfOthers) ?: flowOf(0)).stateIn(
            viewModelScope,
            SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
            0,
        )

    fun retryNow() {
        dependencies.orders.retryNow()
    }

    private fun <T> userFlow(block: (String) -> Flow<List<T>>): Flow<List<T>> =
        userId?.let(block) ?: flowOf(emptyList())

    companion object {
        private const val STOP_TIMEOUT_MILLIS = 5_000L

        fun factory(dependencies: WaiterDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { PendingOrdersViewModel(dependencies) }
            }
    }
}

/**
 * «Requiere tu revisión» de un pedido (§6.4): los cambios que encontró Go con sus mensajes y lo que
 * el mesero puede hacer. El pedido se vuelve a enviar **solo** cuando el mesero acepta, con la
 * misma clave de operación.
 */
class ReviewViewModel(
    private val dependencies: WaiterDependencies,
    savedState: SavedStateHandle,
) : ViewModel() {
    val operationId: String = requireNotNull(savedState[KEY_OPERATION_ID]) { "Falta el pedido" }

    val order: StateFlow<PendingOrder?> =
        dependencies.orders
            .order(operationId)
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), null)

    var working: Boolean by mutableStateOf(false)
        private set

    fun accept(onDone: () -> Unit) = run(onDone) { dependencies.orders.accept(operationId) }

    fun resend(onDone: () -> Unit) = run(onDone) { dependencies.orders.resend(operationId) }

    fun removeLine(itemId: String) = run({}) { dependencies.orders.removeLine(operationId, itemId) }

    fun moveToNewAccount(onDone: () -> Unit) =
        run(onDone) { dependencies.orders.moveToNewAccount(operationId) }

    fun discard(onDone: () -> Unit) = run(onDone) { dependencies.orders.discard(operationId) }

    fun backToCapture(onCapture: (String) -> Unit) {
        if (working) {
            return
        }
        working = true
        viewModelScope.launch {
            try {
                dependencies.orders.backToCapture(operationId)?.let(onCapture)
            } finally {
                working = false
            }
        }
    }

    private fun run(onDone: () -> Unit, block: suspend () -> Unit) {
        if (working) {
            return
        }
        working = true
        viewModelScope.launch {
            try {
                block()
                onDone()
            } finally {
                working = false
            }
        }
    }

    companion object {
        const val KEY_OPERATION_ID = "operationId"

        private const val STOP_TIMEOUT_MILLIS = 5_000L

        fun factory(dependencies: WaiterDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { ReviewViewModel(dependencies, createSavedStateHandle()) }
            }
    }
}
