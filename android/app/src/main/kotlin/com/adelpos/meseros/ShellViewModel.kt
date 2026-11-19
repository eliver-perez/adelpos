package com.adelpos.meseros

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider.AndroidViewModelFactory.Companion.APPLICATION_KEY
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.adelpos.meseros.core.domain.connection.ConnectionSummary
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.connection.PendingOrdersSource
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn

/**
 * Estado común a todas las pantallas: el indicador de conexión y de pedidos por enviar de la barra
 * superior.
 */
class ShellViewModel(
    linkState: Flow<LinkState>,
    initialLink: LinkState,
    pendingOrders: PendingOrdersSource,
) : ViewModel() {
    val connection: StateFlow<ConnectionSummary> =
        combine(linkState, pendingOrders.pendingCount()) { link, pending ->
                ConnectionSummary(link, pending)
            }
            .stateIn(
                scope = viewModelScope,
                started = SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
                initialValue = ConnectionSummary(initialLink, 0),
            )

    companion object {
        // Sigue escuchando unos segundos al rotar la pantalla para no repetir el registro con el
        // sistema.
        private const val STOP_TIMEOUT_MILLIS = 5_000L

        val Factory = viewModelFactory {
            initializer {
                val container = (this[APPLICATION_KEY] as AdelPosApplication).container
                ShellViewModel(
                    linkState = container.connectionMonitor.linkState,
                    initialLink = container.connectionMonitor.current(),
                    pendingOrders = container.pendingOrders,
                )
            }
        }
    }
}
