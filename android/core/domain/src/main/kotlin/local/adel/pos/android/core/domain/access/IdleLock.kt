package local.adel.pos.android.core.domain.access

import kotlinx.coroutines.delay

/**
 * Bloqueo por inactividad (§3.2): tras [timeout][watch] sin tocar la pantalla, el dispositivo
 * vuelve a la pantalla de entrada con PIN, **excepto** si el mesero tiene una captura en curso o
 * pedidos sin enviar ([WaiterWork.blocksIdleLock]); en ese caso vuelve a comprobarlo cada
 * [recheckMillis] hasta que ya no los tenga (o alguien toque la pantalla). El tiempo lo da Go
 * (`idle_lock_seconds`, 120 por omisión, en Administración › Terminales).
 *
 * El bloqueo no cierra la sesión en el servidor: solo regresa a la lista de meseros.
 *
 * @param clock milisegundos de un reloj monótono (el que no cambia al ajustar la hora); en las
 *   pruebas, el tiempo virtual.
 */
class IdleLock(
    private val clock: () -> Long,
    private val work: WaiterWorkGate,
    private val recheckMillis: Long = DEFAULT_RECHECK_MILLIS,
) {
    @Volatile private var lastActivity: Long = clock()

    /** El mesero tocó la pantalla (o escribió): el plazo vuelve a empezar. */
    fun touch() {
        lastActivity = clock()
    }

    /** Milisegundos que faltan para bloquear con [timeoutMillis] (0 si ya venció). */
    fun remainingMillis(timeoutMillis: Long): Long =
        (lastActivity + timeoutMillis - clock()).coerceAtLeast(0)

    /**
     * Vigila al mesero [userId] y llama a [lock] cuando corresponde; termina al bloquear. Se
     * cancela con la corrutina (por ejemplo, al cambiar de mesero). [timeoutSeconds] se lee en cada
     * vuelta: si el administrador lo cambia, vale desde el siguiente plazo.
     */
    suspend fun watch(userId: String, timeoutSeconds: () -> Int, lock: suspend () -> Unit) {
        touch()
        while (true) {
            val timeout = timeoutSeconds().coerceAtLeast(MIN_SECONDS) * 1000L
            val remaining = remainingMillis(timeout)
            if (remaining > 0) {
                delay(remaining.coerceAtMost(MAX_WAIT_MILLIS))
                continue
            }
            if (work.workOf(userId).blocksIdleLock) {
                // Con trabajo pendiente nunca se bloquea: se vuelve a mirar más tarde.
                delay(recheckMillis)
                continue
            }
            lock()
            return
        }
    }

    companion object {
        /** El mínimo que acepta Go (`devices.idle_lock_seconds`, 30 a 3600). */
        const val MIN_SECONDS = 30
        const val DEFAULT_SECONDS = 120
        const val DEFAULT_RECHECK_MILLIS = 15_000L

        // Espera como máximo un minuto por vuelta: un plazo que se acorta en el servidor se nota
        // pronto.
        private const val MAX_WAIT_MILLIS = 60_000L
    }
}
