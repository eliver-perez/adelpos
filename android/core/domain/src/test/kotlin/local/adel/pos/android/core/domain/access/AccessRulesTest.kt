package local.adel.pos.android.core.domain.access

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Reglas de la entrada al dispositivo: límite local de intentos sin conexión, bloqueo por
 * inactividad con reloj inyectable (también que no bloquea con trabajo pendiente), invalidación de
 * los verificadores por `pin_version` y permisos de Android según el `targetSdk`.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class AccessRulesTest {
    @Test
    fun cincoFallosSinConexionBloqueanCincoMinutos() {
        var attempts = LocalAttempts.NONE
        val start = 1_000_000L
        for (failure in 1..4) {
            attempts = OfflineAttemptPolicy.afterFailure(attempts, start + failure * 1_000)
            assertNull(OfflineAttemptPolicy.lockedUntil(attempts, start + failure * 1_000))
            assertEquals(failure, attempts.failures)
        }
        val fifth = start + 5_000
        attempts = OfflineAttemptPolicy.afterFailure(attempts, fifth)
        assertEquals(
            fifth + OfflineAttemptPolicy.LOCK_MILLIS,
            OfflineAttemptPolicy.lockedUntil(attempts, fifth),
        )
        assertEquals(0, attempts.failures)
        // Al vencer el bloqueo hay otros cinco intentos.
        val later = fifth + OfflineAttemptPolicy.LOCK_MILLIS
        assertNull(OfflineAttemptPolicy.lockedUntil(attempts, later))
        attempts = OfflineAttemptPolicy.afterFailure(attempts, later)
        assertEquals(1, attempts.failures)
        assertEquals(LocalAttempts.NONE, OfflineAttemptPolicy.afterSuccess())
    }

    @Test
    fun losFallosViejosNoCuentan() {
        var attempts = LocalAttempts.NONE
        for (failure in 1..4) {
            attempts = OfflineAttemptPolicy.afterFailure(attempts, failure * 1_000L)
        }
        // El quinto llega después de la ventana de 15 minutos: empieza una cuenta nueva.
        val late = 4_000L + OfflineAttemptPolicy.WINDOW_MILLIS
        attempts = OfflineAttemptPolicy.afterFailure(attempts, late)
        assertEquals(1, attempts.failures)
        assertNull(OfflineAttemptPolicy.lockedUntil(attempts, late))
    }

    private class FakeWork(var work: WaiterWork = WaiterWork.NONE) : WaiterWorkGate {
        val asked = mutableListOf<String>()

        override suspend fun workOf(userId: String): WaiterWork {
            asked.add(userId)
            return work
        }
    }

    @Test
    fun bloqueaTrasElTiempoSinUsoYTocarReinicia() = runTest {
        val gate = FakeWork()
        val idle = IdleLock(clock = { testScheduler.currentTime }, work = gate)
        var locked = false
        val job = launch { idle.watch("ana", timeoutSeconds = { 120 }) { locked = true } }
        advanceTimeBy(119_000)
        runCurrent()
        assertFalse(locked)
        idle.touch()
        advanceTimeBy(119_000)
        runCurrent()
        assertFalse("tocar la pantalla reinicia el plazo", locked)
        advanceTimeBy(1_001)
        runCurrent()
        assertTrue(locked)
        assertTrue(job.isCompleted)
        assertEquals(listOf("ana"), gate.asked)
    }

    @Test
    fun noBloqueaConCapturaEnCursoOPedidosSinEnviar() = runTest {
        val gate = FakeWork(WaiterWork(captureInProgress = true, unsentOrders = 0))
        val idle =
            IdleLock(clock = { testScheduler.currentTime }, work = gate, recheckMillis = 15_000)
        var locked = false
        launch { idle.watch("ana", timeoutSeconds = { 60 }) { locked = true } }
        advanceTimeBy(10 * 60_000)
        runCurrent()
        assertFalse("con una captura en curso nunca se bloquea", locked)
        gate.work = WaiterWork(captureInProgress = false, unsentOrders = 2)
        advanceTimeBy(10 * 60_000)
        runCurrent()
        assertFalse("con pedidos sin enviar nunca se bloquea", locked)
        gate.work = WaiterWork.NONE
        advanceTimeBy(15_001)
        runCurrent()
        assertTrue("sin trabajo pendiente se bloquea en la siguiente revisión", locked)
    }

    @Test
    fun elPlazoMinimoEsElDeGoYSeLeeEnCadaVuelta() = runTest {
        var seconds = 10
        val idle = IdleLock(clock = { testScheduler.currentTime }, work = NoWaiterWork)
        var lockedAt = -1L
        launch {
            idle.watch("ana", timeoutSeconds = { seconds }) { lockedAt = testScheduler.currentTime }
        }
        advanceTimeBy(29_000)
        runCurrent()
        assertEquals("menos de 30 s cuenta como 30", -1L, lockedAt)
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals(30_000L, lockedAt)
        // Un plazo nuevo de Go vale desde la siguiente vigilancia.
        seconds = 300
        lockedAt = -1L
        launch {
            idle.watch("ana", timeoutSeconds = { seconds }) { lockedAt = testScheduler.currentTime }
        }
        advanceTimeBy(299_000)
        runCurrent()
        assertEquals(-1L, lockedAt)
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals("empezó a vigilar en 30 001 ms", 330_001L, lockedAt)
    }

    @Test
    fun elTrabajoPendienteDecideQueSeImpide() {
        assertFalse(WaiterWork.NONE.blocksIdleLock)
        assertFalse(WaiterWork.NONE.blocksSwitch)
        val capturing = WaiterWork(captureInProgress = true, unsentOrders = 0)
        assertTrue(capturing.blocksIdleLock)
        assertFalse("la captura se queda con su mesero", capturing.blocksSwitch)
        val unsent = WaiterWork(captureInProgress = false, unsentOrders = 1)
        assertTrue(unsent.blocksIdleLock)
        assertTrue(unsent.blocksSwitch)
    }

    private fun profile(id: String, version: Int) =
        WaiterProfile(id, id, id.take(1).uppercase(), null, version, null)

    @Test
    fun losVerificadoresSeInvalidanPorPinVersionOAusencia() {
        val known =
            listOf(
                KnownWaiterInvalidation.Known("ana", 1),
                KnownWaiterInvalidation.Known("beto", 4),
                KnownWaiterInvalidation.Known("carla", 2),
            )
        val listed = listOf(profile("ana", 1), profile("beto", 5), profile("dora", 1))
        assertEquals(setOf("beto", "carla"), KnownWaiterInvalidation.stale(known, listed))
        assertEquals(emptySet<String>(), KnownWaiterInvalidation.stale(emptyList(), listed))
    }

    @Test
    fun permisoDeRedLocalSoloConTargetSdk37EnAndroid17() {
        assertEquals(emptyList<String>(), NetworkPermissions.required(sdkInt = 26, targetSdk = 36))
        assertEquals(emptyList<String>(), NetworkPermissions.required(sdkInt = 37, targetSdk = 36))
        assertEquals(emptyList<String>(), NetworkPermissions.required(sdkInt = 36, targetSdk = 37))
        assertEquals(
            listOf("android.permission.ACCESS_LOCAL_NETWORK"),
            NetworkPermissions.required(sdkInt = 37, targetSdk = 37),
        )
    }
}
