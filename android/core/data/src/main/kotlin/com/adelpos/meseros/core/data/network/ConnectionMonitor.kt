package com.adelpos.meseros.core.data.network

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import com.adelpos.meseros.core.domain.connection.DefaultNetworkTracker
import com.adelpos.meseros.core.domain.connection.LinkState
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * Estado de la red del dispositivo para el indicador «En línea» / «Sin conexión», con
 * `registerDefaultNetworkCallback` (API 24+) y la regla de [DefaultNetworkTracker]: no exige una
 * red validada por Android, porque la wifi del restaurante puede no tener internet.
 */
class ConnectionMonitor(context: Context) {
    private val manager: ConnectivityManager =
        context.applicationContext.getSystemService(ConnectivityManager::class.java)

    /**
     * Estado al momento, para el primer cuadro de la pantalla (sin esperar al primer aviso del
     * sistema).
     */
    fun current(): LinkState =
        if (manager.activeNetwork != null) {
            LinkState.ONLINE
        } else {
            LinkState.OFFLINE
        }

    /**
     * Cambios del estado mientras alguien lo observa; se deja de escuchar al sistema al dejar de
     * observarlo.
     */
    val linkState: Flow<LinkState> = states(initialEvenIfOffline = true)

    /**
     * Lo mismo, pero solo con lo que Android **informa** (etapa A3): si al empezar todavía no hay
     * red predeterminada, esa primera lectura no se manda (no es un aviso de que no haya red; al
     * arrancar el proceso Android puede tardar en dar la red a la aplicación). Lo usa el estado
     * único de conexión, que mientras tanto duda y se ve «En línea».
     */
    val reportedLinkState: Flow<LinkState> = states(initialEvenIfOffline = false)

    private fun states(initialEvenIfOffline: Boolean): Flow<LinkState> = callbackFlow {
        val tracker = DefaultNetworkTracker()
        // activeNetwork es null sin red predeterminada o si el sistema la bloquea para la
        // aplicación.
        manager.activeNetwork?.let { network -> tracker.onAvailable(network.networkHandle) }
        if (initialEvenIfOffline || tracker.state == LinkState.ONLINE) {
            trySend(tracker.state)
        }
        val callback =
            object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    trySend(tracker.onAvailable(network.networkHandle))
                }

                override fun onLost(network: Network) {
                    trySend(tracker.onLost(network.networkHandle))
                }

                // Solo lo llama Android 10 (API 29) y posteriores.
                override fun onBlockedStatusChanged(network: Network, blocked: Boolean) {
                    trySend(tracker.onBlockedStatusChanged(network.networkHandle, blocked))
                }
            }
        manager.registerDefaultNetworkCallback(callback)
        awaitClose { manager.unregisterNetworkCallback(callback) }
    }
        .distinctUntilChanged()
}
