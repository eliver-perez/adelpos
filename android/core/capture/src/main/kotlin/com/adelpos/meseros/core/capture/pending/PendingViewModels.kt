package com.adelpos.meseros.core.capture.pending

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
import com.adelpos.core.model.error.AppResult
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DiscardResult
import com.adelpos.meseros.core.domain.waiter.OthersPending
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * «Pendientes» (§7): lo que el mesero activo tiene sin enviar (sus capturas y sus pedidos de la
 * cola con su estado) y lo recibido hace poco; «Reintentar ya». De los demás meseros solo se dice
 * cuántos pedidos tienen por enviar en este dispositivo y por qué no salen (credencial que ya no
 * vale, revisión pendiente; revisión R6, B3), nunca cuáles.
 */
class PendingOrdersViewModel(private val dependencies: CaptureDependencies) : ViewModel() {
    private val userId: String? = dependencies.activeWaiter.value?.userId

    val orders: StateFlow<List<PendingOrder>> = userFlow {
        dependencies.orders.orders(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    val captures: StateFlow<List<Capture>> = userFlow {
        dependencies.captures.captures(it)
    }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

    val others: StateFlow<List<OthersPending>> = userFlow {
        dependencies.orders.othersPending(it)
    }
        .stateIn(
            viewModelScope,
            SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
            emptyList(),
        )

    fun retryNow() {
        dependencies.orders.retryNow()
    }

    private fun <T> userFlow(block: (String) -> Flow<List<T>>): Flow<List<T>> =
        userId?.let(block) ?: flowOf(emptyList())

    companion object {
        private const val STOP_TIMEOUT_MILLIS = 5_000L

        fun factory(dependencies: CaptureDependencies): ViewModelProvider.Factory =
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
    private val dependencies: CaptureDependencies,
    savedState: SavedStateHandle,
) : ViewModel() {
    val operationId: String = requireNotNull(savedState[KEY_OPERATION_ID]) { "Falta el pedido" }

    val order: StateFlow<PendingOrder?> =
        dependencies.orders
            .order(operationId)
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), null)

    var working: Boolean by mutableStateOf(false)
        private set

    /**
     * La cuenta en que se combinó la del pedido, si se sabe (se lee con la sesión del mesero; M3):
     * con ella se ofrece «Enviar a la cuenta en que se combinó».
     */
    var mergedTarget: String? by mutableStateOf(null)
        private set

    init {
        viewModelScope.launch {
            val current = dependencies.orders.order(operationId).filterNotNull().first()
            val review = current.review ?: return@launch
            // En línea, Go ya dijo en qué cuenta se combinó; si no, se lee la cuenta.
            review.mergedIntoId?.let {
                mergedTarget = it
                return@launch
            }
            val accountId = current.destination.accountId ?: return@launch
            if (review.accountMerged || review.reason == ReviewReason.ACCOUNT_NOT_OPEN) {
                val bill = dependencies.restaurant.bill(accountId)
                if (bill is AppResult.Success) {
                    mergedTarget = bill.value.account.mergedIntoId
                }
            }
        }
    }

    fun accept(onDone: () -> Unit) = run(onDone) { dependencies.orders.accept(operationId) }

    fun sendToAccount(accountId: String, onDone: () -> Unit) =
        run(onDone) { dependencies.orders.sendToAccount(operationId, accountId) }

    fun resend(onDone: () -> Unit) = run(onDone) { dependencies.orders.resend(operationId) }

    fun removeLine(itemId: String) = run({}) { dependencies.orders.removeLine(operationId, itemId) }

    fun moveToNewAccount(onDone: () -> Unit) =
        run(onDone) { dependencies.orders.moveToNewAccount(operationId) }

    /** «Descartar el pedido»; [onDone] recibe si lo guardado en la cuenta quedó por quitar. */
    fun discard(onDone: (DiscardResult) -> Unit) {
        if (working) {
            return
        }
        working = true
        viewModelScope.launch {
            try {
                onDone(dependencies.orders.discard(operationId))
            } finally {
                working = false
            }
        }
    }

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

        fun factory(dependencies: CaptureDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { ReviewViewModel(dependencies, createSavedStateHandle()) }
            }
    }
}
