package com.adelpos.meseros.core.domain.connection

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class DefaultNetworkTrackerTest {
    @Test
    fun empiezaSinConexion() {
        assertEquals(LinkState.OFFLINE, DefaultNetworkTracker().state)
    }

    @Test
    fun hayConexionConUnaRedPredeterminadaAunqueNoEsteValidada() {
        // La wifi del restaurante sin internet también cuenta: el tracker no recibe ni exige
        // «validada».
        val tracker = DefaultNetworkTracker()
        assertEquals(LinkState.ONLINE, tracker.onAvailable(100))
        assertEquals(LinkState.OFFLINE, tracker.onLost(100))
    }

    @Test
    fun alCambiarDeRedLaPerdidaDeLaAnteriorNoCortaLaConexion() {
        val tracker = DefaultNetworkTracker()
        tracker.onAvailable(1)
        assertEquals(LinkState.ONLINE, tracker.onAvailable(2))
        assertEquals(LinkState.ONLINE, tracker.onLost(1))
        assertEquals(LinkState.OFFLINE, tracker.onLost(2))
    }

    @Test
    fun unaRedBloqueadaCuentaComoSinConexion() {
        val tracker = DefaultNetworkTracker()
        tracker.onAvailable(7)
        assertEquals(LinkState.OFFLINE, tracker.onBlockedStatusChanged(7, true))
        // Un bloqueo de otra red no importa.
        assertEquals(LinkState.OFFLINE, tracker.onBlockedStatusChanged(8, false))
        assertEquals(LinkState.ONLINE, tracker.onBlockedStatusChanged(7, false))
        tracker.onBlockedStatusChanged(7, true)
        // Una red nueva llega desbloqueada.
        assertEquals(LinkState.ONLINE, tracker.onAvailable(9))
    }

    @Test
    fun elResumenNoAdmitePendientesNegativos() {
        assertEquals(3, ConnectionSummary(LinkState.OFFLINE, 3).pendingOrders)
        assertThrows(IllegalArgumentException::class.java) {
            ConnectionSummary(LinkState.ONLINE, -1)
        }
    }
}
