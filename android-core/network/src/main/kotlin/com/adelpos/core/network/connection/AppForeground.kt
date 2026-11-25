package com.adelpos.core.network.connection

import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

// Etapa K1: separado de CatalogRealtimeContainer.kt de la aplicación de meseros sin cambiarlo; lo
// usa
// el estado único de conexión ([ServerLinkMonitor]) de las dos aplicaciones.

/**
 * Si la aplicación está en primer plano (alguna pantalla visible), según el ciclo de vida del
 * proceso (`ProcessLifecycleOwner`, que espera unos 700 ms al pasar a segundo plano para no
 * confundir una rotación con una salida).
 */
class AppForeground : DefaultLifecycleObserver {
    private val state = MutableStateFlow(false)

    val inForeground: StateFlow<Boolean> = state.asStateFlow()

    override fun onStart(owner: LifecycleOwner) {
        state.value = true
    }

    override fun onStop(owner: LifecycleOwner) {
        state.value = false
    }
}
