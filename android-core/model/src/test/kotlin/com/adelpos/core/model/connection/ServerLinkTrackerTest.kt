package com.adelpos.core.model.connection

import com.adelpos.core.model.connection.ServerLinkTracker.Companion.DOUBT_PROBE_MILLIS
import com.adelpos.core.model.connection.ServerLinkTracker.Companion.IDLE_CHECK_MILLIS
import com.adelpos.core.model.connection.ServerLinkTracker.Companion.MIN_OFFLINE_MILLIS
import com.adelpos.core.model.connection.ServerLinkTracker.Companion.OFFLINE_PROBE_MILLIS
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El estado único de conexión (etapa A3, §2 y §6) con tiempo virtual: una falla aislada no cambia a
 * «Sin conexión», varias seguidas durante 20 s sí, el primer éxito lo devuelve, sin red de Android
 * al instante, y la revisión según el uso (sonda cada 5 s mientras duda, cada 60 s sin canal y
 * ninguna con el canal abierto o en segundo plano).
 */
class ServerLinkTrackerTest {
    private companion object {
        const val GRACE = 10_000L
    }

    private var now = 1_000_000L
    private val tracker = ServerLinkTracker(LinkState.ONLINE) { now }

    private fun advance(millis: Long) {
        now += millis
    }

    @Test
    fun unaFallaAisladaNoMuestraSinConexion() {
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
        assertTrue("duda, pero sigue «En línea»", tracker.doubting)
        // Aunque pase mucho tiempo: una sola falla nunca basta.
        advance(10 * MIN_OFFLINE_MILLIS)
        assertEquals(LinkState.ONLINE, tracker.state)
        assertEquals(LinkState.ONLINE, tracker.onAnswer())
        assertFalse(tracker.doubting)
    }

    @Test
    fun variasFallasSeguidasDuranteVeinteSegundosSiMuestranSinConexion() {
        tracker.onNoAnswer()
        advance(DOUBT_PROBE_MILLIS)
        tracker.onNoAnswer()
        advance(DOUBT_PROBE_MILLIS)
        // Tres fallas en 10 s: todavía no pasan los 20 s desde la primera.
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
        advance(DOUBT_PROBE_MILLIS)
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
        advance(DOUBT_PROBE_MILLIS)
        // A los 20 s desde la primera y con cuatro fallas seguidas.
        assertEquals(LinkState.OFFLINE, tracker.onNoAnswer())
        assertFalse("ya no duda: está sin conexión", tracker.doubting)
    }

    @Test
    fun dosFallasEnVeinticincoSegundosNoBastan() {
        tracker.onNoAnswer()
        advance(25_000)
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
    }

    @Test
    fun tresFallasJuntasEsperanLosVeinteSegundos() {
        repeat(3) { tracker.onNoAnswer() }
        assertEquals(LinkState.ONLINE, tracker.state)
        advance(MIN_OFFLINE_MILLIS - 1)
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
        advance(1)
        assertEquals(LinkState.OFFLINE, tracker.onNoAnswer())
    }

    @Test
    fun unExitoEnMedioReiniciaLaRacha() {
        tracker.onNoAnswer()
        advance(10_000)
        tracker.onNoAnswer()
        tracker.onAnswer()
        advance(10_000)
        tracker.onNoAnswer()
        advance(5_000)
        // Tres fallas en total, pero la racha vigente empezó hace 5 s con una sola.
        assertEquals(LinkState.ONLINE, tracker.onNoAnswer())
    }

    @Test
    fun recuperaConElPrimerExito() {
        goOffline()
        assertEquals(LinkState.OFFLINE, tracker.state)
        assertEquals(LinkState.ONLINE, tracker.onAnswer())
    }

    @Test
    fun sinRedDeAndroidAlInstanteYAlVolverEsperaElPrimerExito() {
        assertEquals(LinkState.OFFLINE, tracker.onNetwork(LinkState.OFFLINE))
        assertFalse(tracker.hasNetwork)
        // Sin red no se pregunta y una respuesta tardía no la contradice.
        assertNull(tracker.nextProbeIn(foreground = true, channelOpen = false))
        assertEquals(LinkState.OFFLINE, tracker.onAnswer())
        // La red vuelve: sigue «Sin conexión» hasta que Go responda, y se sondea ya.
        assertEquals(LinkState.OFFLINE, tracker.onNetwork(LinkState.ONLINE))
        assertEquals(0L, tracker.nextProbeIn(foreground = true, channelOpen = true))
        assertEquals(LinkState.ONLINE, tracker.onAnswer())
    }

    @Test
    fun sinServidorEmparejadoSigueALaRedDeAndroid() {
        tracker.onNetwork(LinkState.OFFLINE)
        tracker.onNetwork(LinkState.ONLINE)
        assertEquals(LinkState.ONLINE, tracker.onNothingToProbe())
    }

    @Test
    fun mientrasDudaSondeaCadaCincoSegundos() {
        tracker.onAnswer()
        tracker.onNoAnswer()
        assertEquals(DOUBT_PROBE_MILLIS, tracker.nextProbeIn(foreground = true, channelOpen = true))
        advance(2_000)
        assertEquals(3_000L, tracker.nextProbeIn(foreground = true, channelOpen = false))
        advance(3_000)
        assertEquals(0L, tracker.nextProbeIn(foreground = true, channelOpen = false))
        tracker.onProbe()
        assertEquals(
            DOUBT_PROBE_MILLIS,
            tracker.nextProbeIn(foreground = true, channelOpen = false),
        )
    }

    @Test
    fun sinConexionConRedSondeaCadaDiezSegundos() {
        goOffline()
        tracker.onProbe()
        assertEquals(
            OFFLINE_PROBE_MILLIS,
            tracker.nextProbeIn(foreground = true, channelOpen = false),
        )
    }

    @Test
    fun conElCanalDeEventosAbiertoNoHayRevisionExtra() {
        tracker.onAnswer()
        assertNull(tracker.nextProbeIn(foreground = true, channelOpen = true))
        advance(10 * IDLE_CHECK_MILLIS)
        assertNull(tracker.nextProbeIn(foreground = true, channelOpen = true))
    }

    @Test
    fun sinCanalRevisaCadaMinutoYSeSaltaSiHuboOtroExito() {
        // Al empezar no se sabe nada: se revisa ya.
        assertEquals(0L, tracker.nextProbeIn(foreground = true, channelOpen = false))
        tracker.onProbe()
        tracker.onAnswer()
        assertEquals(
            IDLE_CHECK_MILLIS,
            tracker.nextProbeIn(foreground = true, channelOpen = false),
        )
        advance(40_000)
        // La lista de «Selecciona tu usuario» respondió: la revisión se corre 60 s desde ese éxito.
        tracker.onAnswer()
        advance(30_000)
        assertEquals(30_000L, tracker.nextProbeIn(foreground = true, channelOpen = false))
    }

    @Test
    fun enSegundoPlanoNoRevisaYAlVolverRevisaEnseguida() {
        tracker.onProbe()
        tracker.onAnswer()
        assertNull(tracker.nextProbeIn(foreground = false, channelOpen = false))
        tracker.onNoAnswer()
        assertNull(tracker.nextProbeIn(foreground = false, channelOpen = false))
        advance(10 * IDLE_CHECK_MILLIS)
        assertEquals(0L, tracker.nextProbeIn(foreground = true, channelOpen = false))
    }

    @Test
    fun empiezaSinConexionSiAndroidNoTieneRed() {
        val offline = ServerLinkTracker(LinkState.OFFLINE) { now }
        assertEquals(LinkState.OFFLINE, offline.state)
        offline.onNetwork(LinkState.ONLINE)
        assertEquals(LinkState.OFFLINE, offline.state)
        assertEquals(LinkState.ONLINE, offline.onAnswer())
    }

    @Test
    fun alArrancarSinSaberDeLaRedSeVeEnLineaHastaLaPrimeraRespuesta() {
        // Android todavía no informa la red (al crear el proceso): nunca «Sin conexión» por eso.
        val starting = ServerLinkTracker(initialNetwork = null) { now }
        assertEquals(LinkState.ONLINE, starting.state)
        assertTrue("duda", starting.doubting)
        assertTrue("puede haber red: se sondea", starting.hasNetwork)
        assertEquals(0L, starting.nextProbeIn(foreground = true, channelOpen = false))
        starting.onProbe()
        assertEquals(
            DOUBT_PROBE_MILLIS,
            starting.nextProbeIn(foreground = true, channelOpen = true),
        )
        advance(MIN_OFFLINE_MILLIS / 2)
        assertEquals(LinkState.ONLINE, starting.onAnswer())
        assertFalse(starting.doubting)
        assertNull("ya no cambia solo", starting.nextDecisionIn())
        advance(10 * MIN_OFFLINE_MILLIS)
        assertEquals(LinkState.ONLINE, starting.state)
    }

    @Test
    fun alArrancarElPrimerAvisoDeQueNoHayRedEsSinConexionAlInstante() {
        val starting = ServerLinkTracker(initialNetwork = null) { now }
        assertEquals(LinkState.OFFLINE, starting.onNetwork(LinkState.OFFLINE))
        assertNull(starting.nextDecisionIn())
        // Y al volver la red, «En línea» con el primer éxito.
        assertEquals(LinkState.OFFLINE, starting.onNetwork(LinkState.ONLINE))
        assertEquals(LinkState.ONLINE, starting.onAnswer())
    }

    @Test
    fun alArrancarElAvisoDeRedSigueEnLineaYSeQuedaAsiSinFallas() {
        val starting = ServerLinkTracker(initialNetwork = null) { now }
        assertEquals(LinkState.ONLINE, starting.onNetwork(LinkState.ONLINE))
        assertFalse("Android ya informó la red", starting.doubting)
        advance(10 * MIN_OFFLINE_MILLIS)
        assertEquals(LinkState.ONLINE, starting.state)
    }

    @Test
    fun alArrancarSinRespuestaNiAvisoPasaASinConexionAlVencerLaTolerancia() {
        val starting = ServerLinkTracker(initialNetwork = null) { now }
        assertEquals(MIN_OFFLINE_MILLIS, starting.nextDecisionIn())
        advance(MIN_OFFLINE_MILLIS - 1)
        assertEquals(LinkState.ONLINE, starting.state)
        assertEquals(1L, starting.nextDecisionIn())
        advance(1)
        assertEquals(LinkState.OFFLINE, starting.state)
        assertNull("el cambio ya ocurrió", starting.nextDecisionIn())
        // Ya se mostraba «Sin conexión»: el aviso de red no lo quita, el primer éxito sí.
        assertEquals(LinkState.OFFLINE, starting.onNetwork(LinkState.ONLINE))
        assertEquals(LinkState.ONLINE, starting.onAnswer())
    }

    @Test
    fun alArrancarSinServidorEmparejadoTambienEsperaElAvisoDeAndroid() {
        val starting = ServerLinkTracker(initialNetwork = null) { now }
        assertEquals(LinkState.ONLINE, starting.onNothingToProbe())
        assertEquals(LinkState.ONLINE, starting.onNetwork(LinkState.ONLINE))
        assertEquals(LinkState.ONLINE, starting.onNothingToProbe())
    }

    // --- El margen sin red (etapa K1, K1-D33): 0 en meseros, 10 s en la pantalla de cocina ---

    @Test
    fun sinMargenLaFaltaDeRedEsSinConexionAlInstante() {
        assertEquals(LinkState.OFFLINE, tracker.onNetwork(LinkState.OFFLINE))
        assertNull("nada cambia solo", tracker.nextDecisionIn())
        advance(1_000)
        assertEquals(LinkState.OFFLINE, tracker.onNetwork(LinkState.ONLINE))
        assertEquals(LinkState.ONLINE, tracker.onAnswer())
    }

    @Test
    fun conMargenUnCorteQueVuelveJustoAntesNoSeVe() {
        val kitchen = ServerLinkTracker(LinkState.ONLINE, networkLossGraceMillis = GRACE) { now }
        assertEquals(LinkState.ONLINE, kitchen.onNetwork(LinkState.OFFLINE))
        assertTrue("duda mientras corre el margen", kitchen.doubting)
        assertFalse("sin red de Android no se sondea", kitchen.hasNetwork)
        assertEquals(GRACE, kitchen.nextDecisionIn())
        // Las peticiones sin red no cuentan como fallas de la tolerancia.
        assertEquals(LinkState.ONLINE, kitchen.onNoAnswer())
        advance(GRACE - 1)
        assertEquals(LinkState.ONLINE, kitchen.state)
        assertEquals(1L, kitchen.nextDecisionIn())
        // La red vuelve 1 ms antes del margen: nunca se vio «Sin conexión», ni hace falta un éxito.
        assertEquals(LinkState.ONLINE, kitchen.onNetwork(LinkState.ONLINE))
        assertNull(kitchen.nextDecisionIn())
        advance(10 * GRACE)
        assertEquals(LinkState.ONLINE, kitchen.state)
    }

    @Test
    fun conMargenUnCorteQueVuelveJustoDespuesSiSeVeHastaElPrimerExito() {
        val kitchen = ServerLinkTracker(LinkState.ONLINE, networkLossGraceMillis = GRACE) { now }
        kitchen.onNetwork(LinkState.OFFLINE)
        advance(GRACE)
        assertEquals("al cumplirse el margen", LinkState.OFFLINE, kitchen.state)
        assertNull("el cambio ya ocurrió", kitchen.nextDecisionIn())
        assertFalse(kitchen.doubting)
        advance(1)
        // Volvió la red: sigue «Sin conexión» hasta que Go responda (como sin margen).
        assertEquals(LinkState.OFFLINE, kitchen.onNetwork(LinkState.ONLINE))
        assertEquals(LinkState.ONLINE, kitchen.onAnswer())
    }

    @Test
    fun conMargenElMargenSeCuentaDesdeElAvisoYNoSeReiniciaConAvisosRepetidos() {
        val kitchen = ServerLinkTracker(LinkState.ONLINE, networkLossGraceMillis = GRACE) { now }
        kitchen.onNetwork(LinkState.OFFLINE)
        advance(GRACE / 2)
        kitchen.onNetwork(LinkState.OFFLINE)
        advance(GRACE / 2)
        assertEquals(LinkState.OFFLINE, kitchen.state)
    }

    @Test
    fun conMargenLaToleranciaDeLasFallasNoCambia() {
        val kitchen = ServerLinkTracker(LinkState.ONLINE, networkLossGraceMillis = GRACE) { now }
        kitchen.onNoAnswer()
        repeat(3) {
            advance(DOUBT_PROBE_MILLIS)
            assertEquals(LinkState.ONLINE, kitchen.onNoAnswer())
        }
        advance(DOUBT_PROBE_MILLIS)
        assertEquals(LinkState.OFFLINE, kitchen.onNoAnswer())
    }

    @Test
    fun conMargenEmpezarSinRedTambienEsperaElMargen() {
        val kitchen = ServerLinkTracker(LinkState.OFFLINE, networkLossGraceMillis = GRACE) { now }
        assertEquals(LinkState.ONLINE, kitchen.state)
        advance(GRACE)
        assertEquals(LinkState.OFFLINE, kitchen.state)
    }

    private fun goOffline() {
        tracker.onNoAnswer()
        repeat(4) {
            advance(DOUBT_PROBE_MILLIS)
            tracker.onNoAnswer()
        }
        assertEquals(LinkState.OFFLINE, tracker.state)
    }
}
