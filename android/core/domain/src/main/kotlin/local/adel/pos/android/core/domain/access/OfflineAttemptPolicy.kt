package local.adel.pos.android.core.domain.access

/**
 * Intentos fallidos del PIN **sin conexión** de un mesero en este dispositivo (el servidor no se
 * entera hasta reconectar). Se guardan en Room para que reiniciar la aplicación no los borre.
 */
data class LocalAttempts(
    val failures: Int,
    val windowStartEpochMillis: Long,
    val lockedUntilEpochMillis: Long?,
) {
    companion object {
        val NONE =
            LocalAttempts(failures = 0, windowStartEpochMillis = 0, lockedUntilEpochMillis = null)
    }
}

/**
 * Límite local de intentos del verificador sin conexión (§3.2), con la misma regla que el servidor
 * aplica al PIN (A1-D5): 5 fallos dentro de 15 minutos bloquean la entrada sin conexión de ese
 * mesero 5 minutos. Al bloquearse, la cuenta vuelve a cero (como en el servidor): al vencer hay
 * otros 5 intentos. Usa el reloj de pared porque debe sobrevivir a reiniciar el dispositivo; quien
 * atrase el reloj del dispositivo para saltarse la espera sigue sin poder leer el verificador
 * (cifrado con el Keystore) ni probar más rápido que la derivación lenta.
 */
object OfflineAttemptPolicy {
    const val MAX_FAILURES = 5
    const val WINDOW_MILLIS = 15 * 60 * 1000L
    const val LOCK_MILLIS = 5 * 60 * 1000L

    /** Hasta cuándo está bloqueado a la hora [now], o null si puede intentar. */
    fun lockedUntil(attempts: LocalAttempts, now: Long): Long? =
        attempts.lockedUntilEpochMillis?.takeIf { it > now }

    /** Cuenta un fallo a la hora [now]; el quinto en la ventana bloquea. */
    fun afterFailure(attempts: LocalAttempts, now: Long): LocalAttempts {
        val windowExpired = now - attempts.windowStartEpochMillis >= WINDOW_MILLIS
        val failures = if (windowExpired) 1 else attempts.failures + 1
        val windowStart = if (windowExpired) now else attempts.windowStartEpochMillis
        if (failures >= MAX_FAILURES) {
            return LocalAttempts(0, now, now + LOCK_MILLIS)
        }
        return LocalAttempts(failures, windowStart, null)
    }

    /** Un acierto borra los intentos. */
    fun afterSuccess(): LocalAttempts = LocalAttempts.NONE
}
