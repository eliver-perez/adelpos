package com.adelpos.cocina.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.adelpos.cocina.CocinaContainer
import com.adelpos.cocina.access.KitchenAccess
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.cocina.board.BoardState
import com.adelpos.cocina.board.KitchenBoard
import com.adelpos.cocina.board.RecentState
import com.adelpos.cocina.settings.KitchenSettings
import com.adelpos.cocina.settings.SettingsScreen
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.connection.ServerLink
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch

/**
 * La raíz de la aplicación de cocina: qué pantalla se ve ([status]), el estado único de conexión de
 * la barra ([link]) y la pantalla de pedidos ([board], [recent]; sus acciones son las de
 * [boardActions]). Lee lo guardado al crearse y vuelve a comprobar el servidor cuando vuelve la
 * conexión.
 */
class CocinaViewModel(
    private val access: KitchenAccess,
    serverLink: ServerLink,
    val boardActions: KitchenBoard,
    val settingsActions: KitchenSettings,
) : ViewModel() {
    val status: StateFlow<KitchenStatus> = access.status

    val link: StateFlow<LinkState> = serverLink.state

    val board: StateFlow<BoardState> = boardActions.state

    val recent: StateFlow<RecentState> = boardActions.recent

    /** Los ajustes de la tableta (§5.4). */
    val settings: StateFlow<SettingsScreen> = settingsActions.screen

    /** La dirección vigente de la computadora principal, para los ajustes. */
    fun serverAddress(): String = access.serverAddress().orEmpty()

    init {
        viewModelScope.launch { access.start() }
        viewModelScope.launch {
            serverLink.state.drop(1).filter { it == LinkState.ONLINE }.collect { access.recheck() }
        }
    }

    /** «Volver a comprobar» y la revisión periódica con la aplicación a la vista. */
    fun checkAgain() {
        viewModelScope.launch { access.recheck() }
    }

    /** «Emparejar de nuevo»: borra lo de este dispositivo y vuelve a «Buscar el servidor». */
    fun repair() {
        viewModelScope.launch { access.forgetServer() }
    }

    companion object {
        fun factory(container: CocinaContainer): ViewModelProvider.Factory = viewModelFactory {
            initializer {
                CocinaViewModel(
                    container.access,
                    container.serverLink,
                    container.board,
                    container.kitchenSettings,
                )
            }
        }
    }
}
