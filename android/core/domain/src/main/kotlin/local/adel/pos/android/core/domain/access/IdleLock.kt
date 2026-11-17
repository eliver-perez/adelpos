package local.adel.pos.android.core.domain.access

import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * Bloqueo por inactividad (§3.2): tras el plazo sin tocar la pantalla, el dispositivo vuelve a la
 * pantalla de entrada con PIN, **excepto** si el mesero tiene una captura en curso o pedidos sin
 * enviar ([WaiterWork.blocksIdleLock]); en ese caso vuelve a comprobarlo cada [recheckMillis]. El
 * tiempo lo da Go (`idle_lock_seconds`, 120 por omisión, en Administración › Terminales).
 *
 * Hay **uno por proceso** (revisión R3, A2 y B4): la última actividad es del proceso, no de una
 * pantalla, así que volver a crear la actividad o empezar a vigilar **nunca** reinicia el plazo;
 * solo lo reinician un toque ([touch], también desde los diálogos) y la entrada de un mesero. Al
 * volver a primer plano se comprueba en ese momento ([supervise]): si el plazo ya venció, se
 * bloquea sin esperar.
 *
 * El bloqueo no cierra la sesión en el servidor: solo regresa a la lista de meseros.
 *
 * @param clock milisegundos de un reloj monótono que sigue contando con el dispositivo dormido
 *   (`SystemClock.elapsedRealtime`); en las pruebas, el tiempo virtual.
 */
class IdleLock(
    private val clock: () -> Long,
    private val work: WaiterWorkGate,
    private val recheckMillis: Long = DEFAULT_RECHECK_MILLIS,
) {
    @Volatile private var lastActivity: Long = clock()

    /** El mesero tocó la pantalla (o escribió, o usó un diálogo): el plazo vuelve a empezar. */
    fun touch() {
        lastActivity = clock()
    }

    /** Milisegundos que faltan para bloquear con [timeoutMillis] (0 si ya venció). */
    fun remainingMillis(timeoutMillis: Long): Long =
        (lastActivity + timeoutMillis - clock()).coerceAtLeast(0)

    /**
     * Vigila al mesero [userId] con el plazo [timeoutSeconds] y llama a [lock] cuando corresponde;
     * termina al bloquear. **No reinicia el plazo**: cuenta desde la última actividad del proceso,
     * así que si ya venció bloquea en ese momento. Se cancela con la corrutina.
     */
    suspend fun watch(userId: String, timeoutSeconds: Int, lock: suspend (String) -> Unit) {
        val timeout = timeoutSeconds.coerceAtLeast(MIN_SECONDS) * 1000L
        while (true) {
            val remaining = remainingMillis(timeout)
            if (remaining > 0) {
                // La espera exacta: un toque en medio mueve el plazo y la siguiente vuelta lo ve.
                delay(remaining)
                continue
            }
            if (work.workOf(userId).blocksIdleLock) {
                // Con trabajo pendiente nunca se bloquea: se vuelve a mirar más tarde.
                delay(recheckMillis)
                continue
            }
            lock(userId)
            return
        }
    }

    /**
     * Vigilancia de todo el proceso: el mesero activo ([waiters], su `user_id` o null), el plazo de
     * Go ([timeouts]) y si la aplicación está a la vista ([foreground]). Cada cambio vuelve a
     * empezar la comprobación **sin reiniciar el plazo**: al volver a primer plano (también al
     * encender la pantalla, porque la espera no avanza con el dispositivo dormido) se bloquea en
     * ese momento si ya venció, y un plazo que Go acorta vale enseguida. Solo la entrada de un
     * mesero distinto del anterior cuenta como actividad (entró escribiendo su PIN).
     */
    suspend fun supervise(
        waiters: Flow<String?>,
        timeouts: Flow<Int>,
        foreground: Flow<Boolean>,
        lock: suspend (String) -> Unit,
    ) {
        var previous: String? = null
        combine(
                waiters.distinctUntilChanged(),
                timeouts.distinctUntilChanged(),
                foreground.distinctUntilChanged(),
            ) { waiter, seconds, _ ->
                waiter to seconds
            }
            .collectLatest { (waiter, seconds) ->
                if (waiter != previous) {
                    previous = waiter
                    if (waiter != null) {
                        touch()
                    }
                }
                if (waiter != null) {
                    watch(waiter, seconds, lock)
                }
            }
    }

    companion object {
        /** El mínimo que acepta Go (`devices.idle_lock_seconds`, 30 a 3600). */
        const val MIN_SECONDS = 30
        const val DEFAULT_SECONDS = 120
        const val DEFAULT_RECHECK_MILLIS = 15_000L
    }
}
