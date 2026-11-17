package local.adel.pos.android.core.domain.access

/**
 * Intentos fallidos del PIN de un mesero contra el verificador local de este dispositivo. Se
 * guardan en Room para que reiniciar la aplicación no los borre.
 *
 * @param failures fallos acumulados desde la última entrada correcta.
 * @param closed la entrada sin conexión de ese mesero está cerrada hasta que entre **con conexión**
 *   (demasiados fallos o su PIN bloqueado en Go).
 */
data class LocalAttempts(val failures: Int, val closed: Boolean) {
    init {
        require(failures >= 0) { "Los fallos no pueden ser negativos" }
    }

    companion object {
        val NONE = LocalAttempts(failures = 0, closed = false)
    }
}

/**
 * Límite de intentos del verificador sin conexión (§3.2; revisión R3, hallazgos M1 y M2, y la
 * decisión del coordinador). **No depende de ningún reloj**: el reloj de una tableta compartida lo
 * cambia cualquiera (Ajustes › Fecha y hora; sin red no hay hora automática), así que una espera
 * por tiempo se saltaría adelantándolo.
 * - Los fallos se acumulan y solo vuelven a cero con una entrada correcta.
 * - Al llegar a [MAX_FAILURES], la entrada sin conexión de ese mesero queda **cerrada** hasta que
 *   entre con conexión (Go verifica el PIN con su propio bloqueo, A1-D5 y A1-D16).
 * - Un bloqueo de Go (`PIN_LOCKED` o el `locked_until`/`requires_unlock` de `auth/pin-users`)
 *   también la cierra: sin conexión no se puede seguir probando un PIN que Go ya bloqueó.
 * - Los `PIN_INVALID` con conexión cuentan igual: el presupuesto es uno solo, con o sin red.
 */
object OfflineAttemptPolicy {
    const val MAX_FAILURES = 5

    /**
     * Cuenta un fallo; el quinto desde la última entrada correcta cierra la entrada sin conexión.
     */
    fun afterFailure(attempts: LocalAttempts): LocalAttempts {
        val failures = attempts.failures + 1
        return LocalAttempts(
            failures = failures,
            closed = attempts.closed || failures >= MAX_FAILURES,
        )
    }

    /**
     * Entrada correcta sin conexión (solo posible con la entrada abierta): los fallos vuelven a
     * cero.
     */
    fun afterOfflineSuccess(attempts: LocalAttempts): LocalAttempts = attempts.copy(failures = 0)

    /** Entrada correcta con conexión: Go verificó el PIN; todo vuelve a empezar. */
    fun afterOnlineSuccess(): LocalAttempts = LocalAttempts.NONE

    /** Go bloqueó el PIN de ese mesero: la entrada sin conexión se cierra. */
    fun afterServerLock(attempts: LocalAttempts): LocalAttempts = attempts.copy(closed = true)
}
