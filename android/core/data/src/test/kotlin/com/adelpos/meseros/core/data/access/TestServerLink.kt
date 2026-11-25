package com.adelpos.meseros.core.data.access

import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.network.connection.ServerLinkMonitor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flowOf

/**
 * El estado único de conexión de las pruebas de la entrada (etapa A3, A3-D1): el
 * [ServerLinkMonitor] real con sus reglas, sin su revisión periódica (nunca se arranca), con la red
 * de Android que pone la prueba ([online]) y la sonda del momento contra el servidor emparejado de
 * [access].
 */
class TestServerLink(scope: CoroutineScope) {
    /** La entrada a la que pregunta la sonda del momento (la última que creó la prueba). */
    lateinit var access: AccessRepository

    val monitor =
        ServerLinkMonitor(
            network = emptyFlow(),
            initialNetwork = LinkState.ONLINE,
            foreground = MutableStateFlow(false),
            channelOpen = flowOf(false),
            probing = { relocate, quick -> access.probeServer(relocate, quick) },
            scope = scope,
            clock = { System.nanoTime() / NANOS_PER_MILLI },
        )

    /** La red de Android: sin ella, «Sin conexión» al instante. */
    var online: Boolean = true
        set(value) {
            field = value
            monitor.onNetwork(if (value) LinkState.ONLINE else LinkState.OFFLINE)
        }

    private companion object {
        const val NANOS_PER_MILLI = 1_000_000L
    }
}
