package com.adelpos.core.model.connection

/**
 * Sigue la red predeterminada del sistema a partir de los avisos de
 * `registerDefaultNetworkCallback`, sin depender de Android (se prueba en la JVM). Cada red se
 * identifica por su `networkHandle`.
 *
 * Regla: hay conexión cuando existe una red predeterminada y el sistema no la bloquea para esta
 * aplicación. **No se exige que Android la haya «validado»** (que tenga salida a internet): la wifi
 * del restaurante puede no tener internet y el servidor está en esa misma red local (avance de la
 * etapa, §1.6). Si el servidor responde o no, lo dirá el cliente HTTP en las fases siguientes.
 *
 * Al cambiar de red, Android avisa primero la nueva (`onAvailable`) y puede avisar después la
 * pérdida de la anterior (`onLost`): una pérdida o un bloqueo que no son de la red vigente no
 * cambian nada.
 */
class DefaultNetworkTracker {
    private var current: Long? = null
    private var blocked = false

    /** Estado vigente. */
    val state: LinkState
        @Synchronized
        get() =
            if (current != null && !blocked) {
                LinkState.ONLINE
            } else {
                LinkState.OFFLINE
            }

    /** Hay una nueva red predeterminada. */
    @Synchronized
    fun onAvailable(network: Long): LinkState {
        if (current != network) {
            current = network
            blocked = false
        }
        return state
    }

    /** Se perdió una red; solo cuenta si es la predeterminada vigente. */
    @Synchronized
    fun onLost(network: Long): LinkState {
        if (current == network) {
            current = null
            blocked = false
        }
        return state
    }

    /**
     * El sistema bloqueó o desbloqueó la red para esta aplicación (ahorro de datos, restricciones;
     * API 29+).
     */
    @Synchronized
    fun onBlockedStatusChanged(network: Long, isBlocked: Boolean): LinkState {
        if (current == network) {
            blocked = isBlocked
        }
        return state
    }
}
