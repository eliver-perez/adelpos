package com.adelpos.meseros.core.domain.access

import com.adelpos.core.model.access.NetworkPermissions
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
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
 * Reglas de la entrada al dispositivo: límite de intentos sin conexión que no depende de ningún
 * reloj (revisión R3, M1 y M2), bloqueo por inactividad del proceso con reloj inyectable (también
 * que no bloquea con trabajo pendiente y que volver a primer plano no da otro plazo, R3 A2),
 * invalidación de los verificadores por `pin_version` y permisos de Android según el `targetSdk`.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class AccessRulesTest {
    @Test
    fun losFallosSeAcumulanYElQuintoCierraLaEntradaSinConexion() {
        var attempts = LocalAttempts.NONE
        for (failure in 1..4) {
            attempts = OfflineAttemptPolicy.afterFailure(attempts)
            assertEquals(failure, attempts.failures)
            assertFalse(attempts.closed)
        }
        attempts = OfflineAttemptPolicy.afterFailure(attempts)
        assertTrue("el quinto fallo cierra la entrada sin conexión", attempts.closed)
        // Ningún reloj la vuelve a abrir (revisión R3, M1): seguir fallando la deja cerrada.
        repeat(100) { attempts = OfflineAttemptPolicy.afterFailure(attempts) }
        assertTrue(attempts.closed)
        // Una entrada correcta sin conexión no es posible con la entrada cerrada; solo una con
        // conexión la abre.
        assertEquals(LocalAttempts.NONE, OfflineAttemptPolicy.afterOnlineSuccess())
    }

    @Test
    fun unaEntradaCorrectaSinConexionVuelveACeroLosFallosSinAbrirLoCerrado() {
        var attempts = LocalAttempts.NONE
        repeat(3) { attempts = OfflineAttemptPolicy.afterFailure(attempts) }
        attempts = OfflineAttemptPolicy.afterOfflineSuccess(attempts)
        assertEquals(LocalAttempts(failures = 0, closed = false), attempts)
        // Los fallos no vuelven a cero solos: con el tiempo nada cambia (no hay ventana).
        repeat(4) { attempts = OfflineAttemptPolicy.afterFailure(attempts) }
        assertFalse(attempts.closed)
        assertTrue(OfflineAttemptPolicy.afterFailure(attempts).closed)
        val closed = LocalAttempts(failures = 0, closed = true)
        assertTrue(OfflineAttemptPolicy.afterOfflineSuccess(closed).closed)
    }

    @Test
    fun unBloqueoDeGoCierraLaEntradaSinConexion() {
        val attempts =
            OfflineAttemptPolicy.afterServerLock(LocalAttempts(failures = 1, closed = false))
        assertTrue(attempts.closed)
        assertEquals(1, attempts.failures)
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
        var locked: String? = null
        val job = launch { idle.watch("ana", timeoutSeconds = 120) { locked = it } }
        advanceTimeBy(119_000)
        runCurrent()
        assertNull(locked)
        idle.touch()
        advanceTimeBy(119_000)
        runCurrent()
        assertNull("tocar la pantalla reinicia el plazo", locked)
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals("ana", locked)
        assertTrue(job.isCompleted)
        assertEquals(listOf("ana"), gate.asked)
    }

    @Test
    fun empezarAVigilarNoReiniciaElPlazo() = runTest {
        val idle = IdleLock(clock = { testScheduler.currentTime }, work = NoWaiterWork)
        idle.touch()
        advanceTimeBy(10 * 60_000)
        var locked: String? = null
        // La vigilancia empieza 10 minutos después del último toque: bloquea en ese momento.
        launch { idle.watch("ana", timeoutSeconds = 120) { locked = it } }
        runCurrent()
        assertEquals("ana", locked)
    }

    @Test
    fun noBloqueaConCapturaEnCursoOPedidosSinEnviar() = runTest {
        val gate = FakeWork(WaiterWork(captureInProgress = true, unsentOrders = 0))
        val idle =
            IdleLock(clock = { testScheduler.currentTime }, work = gate, recheckMillis = 15_000)
        var locked = false
        launch { idle.watch("ana", timeoutSeconds = 60) { locked = true } }
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
    fun elPlazoMinimoEsElDeGo() = runTest {
        val idle = IdleLock(clock = { testScheduler.currentTime }, work = NoWaiterWork)
        var lockedAt = -1L
        launch { idle.watch("ana", timeoutSeconds = 10) { lockedAt = testScheduler.currentTime } }
        advanceTimeBy(29_000)
        runCurrent()
        assertEquals("menos de 30 s cuenta como 30", -1L, lockedAt)
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals(30_000L, lockedAt)
    }

    /**
     * Revisión R3, A2 y B4: Ana está activa y la aplicación pasa a segundo plano (la actividad
     * termina con Atrás en Android 8 a 11, o la pantalla se apaga). Pasa una hora con el
     * dispositivo dormido: el reloj del sistema avanza, pero la espera de la corrutina no. Al
     * volver a primer plano se bloquea **en ese momento**, sin conceder otro plazo; la vigilancia
     * es del proceso y no vuelve a empezar con la actividad.
     */
    @Test
    fun volverAPrimerPlanoDespuesDelPlazoBloqueaEnElMomento() = runTest {
        var now = 0L
        val idle = IdleLock(clock = { now }, work = NoWaiterWork)
        val waiter = MutableStateFlow<String?>(null)
        val foreground = MutableStateFlow(true)
        val locks = mutableListOf<Pair<String, Long>>()
        val supervisor = launch {
            idle.supervise(waiter, MutableStateFlow(120), foreground) { user ->
                locks.add(user to now)
                waiter.value = null
            }
        }
        waiter.value = "ana"
        runCurrent()
        now = 30_000
        foreground.value = false
        runCurrent()
        // Una hora con el dispositivo dormido: el reloj avanza y ninguna espera termina.
        now = 30_000 + 60 * 60_000
        runCurrent()
        assertEquals(emptyList<Pair<String, Long>>(), locks)
        foreground.value = true
        runCurrent()
        assertEquals(listOf("ana" to 30_000L + 60 * 60_000L), locks)
        supervisor.cancel()
    }

    @Test
    fun unPlazoQueGoAcortaValeEnseguidaYUnMeseroNuevoEmpiezaSuPlazo() = runTest {
        val idle = IdleLock(clock = { testScheduler.currentTime }, work = NoWaiterWork)
        val waiter = MutableStateFlow<String?>("ana")
        val timeout = MutableStateFlow(600)
        val locks = mutableListOf<Pair<String, Long>>()
        val supervisor = launch {
            idle.supervise(waiter, timeout, MutableStateFlow(true)) { user ->
                locks.add(user to testScheduler.currentTime)
                waiter.value = null
            }
        }
        runCurrent()
        advanceTimeBy(100_000)
        runCurrent()
        // El administrador baja el plazo a 60 s: ya venció y se bloquea sin esperar los 600 s.
        timeout.value = 60
        runCurrent()
        assertEquals(listOf("ana" to 100_000L), locks)
        // Beto entra: su plazo empieza al entrar.
        waiter.value = "beto"
        runCurrent()
        advanceTimeBy(59_000)
        runCurrent()
        assertEquals(1, locks.size)
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals("beto" to 160_000L, locks.last())
        supervisor.cancel()
    }

    @Test
    fun elTrabajoPendienteDecideQueSeImpide() {
        assertFalse(WaiterWork.NONE.blocksIdleLock)
        assertFalse(WaiterWork.NONE.blocksSwitch)
        assertFalse(WaiterWork.NONE.blocksLogout)
        val capturing = WaiterWork(captureInProgress = true, unsentOrders = 0)
        assertTrue(capturing.blocksIdleLock)
        assertFalse("la captura se queda con su mesero", capturing.blocksSwitch)
        assertTrue("nunca se cierra la sesión con una captura en curso", capturing.blocksLogout)
        val unsent = WaiterWork(captureInProgress = false, unsentOrders = 1)
        assertTrue(unsent.blocksIdleLock)
        assertTrue(unsent.blocksSwitch)
        assertTrue(unsent.blocksLogout)
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
