package com.adelpos.cocina.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.adelpos.cocina.CocinaContainer
import com.adelpos.cocina.access.DisplayConnection
import com.adelpos.cocina.access.KitchenAccess
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.connection.ServerLink
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch

/**
 * La raíz de la aplicación de cocina: qué pantalla se ve ([status]) y el estado único de conexión
 * de la barra ([link]). Lee lo guardado al crearse y vuelve a comprobar el servidor cuando vuelve
 * la conexión.
 */
class CocinaViewModel(private val access: KitchenAccess, serverLink: ServerLink) : ViewModel() {
    val status: StateFlow<KitchenStatus> = access.status

    val link: StateFlow<LinkState> = serverLink.state

    val display: StateFlow<DisplayConnection> = access.display

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
            initializer { CocinaViewModel(container.access, container.serverLink) }
        }
    }
}
