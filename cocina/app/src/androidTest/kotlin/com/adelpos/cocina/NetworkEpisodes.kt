package com.adelpos.cocina

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.os.SystemClock

/**
 * Cuánto dura la falta de red **para Android** en cada corte (etapa K1, K1-D33): escucha la red
 * predeterminada como el estado de conexión de la aplicación (`ConnectionMonitor`) y anota cuándo
 * se pierde y cuándo vuelve (reloj monotónico). Se cierra con [close].
 */
class NetworkEpisodes(context: Context) : AutoCloseable {
    private val manager = context.getSystemService(ConnectivityManager::class.java)
    private val lost = mutableListOf<Long>()
    private val back = mutableListOf<Long>()
    private var current: Network? = manager.activeNetwork

    private val callback =
        object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                synchronized(this@NetworkEpisodes) {
                    if (current == null && lost.size > back.size) {
                        back += SystemClock.elapsedRealtime()
                    }
                    current = network
                }
            }

            override fun onLost(network: Network) {
                synchronized(this@NetworkEpisodes) {
                    if (network != current) {
                        return
                    }
                    current = null
                    val now = SystemClock.elapsedRealtime()
                    // Otra red que duró menos de un momento (los datos del emulador entre la wifi y
                    // nada): es el mismo corte.
                    if (
                        back.isNotEmpty() &&
                            back.size == lost.size &&
                            now - back.last() < MERGE_MILLIS
                    ) {
                        back.removeAt(back.size - 1)
                    } else {
                        lost += now
                    }
                }
            }
        }

    init {
        manager.registerDefaultNetworkCallback(callback)
    }

    /** Cuándo empezó cada falta de red. */
    @Synchronized fun starts(): List<Long> = lost.toList()

    /** Cuánto duró cada falta de red que ya terminó (milisegundos). */
    @Synchronized
    fun durations(): List<Long> = back.indices.map { index -> back[index] - lost[index] }

    override fun close() {
        manager.unregisterNetworkCallback(callback)
    }

    private companion object {
        const val MERGE_MILLIS = 1_000L
    }
}
