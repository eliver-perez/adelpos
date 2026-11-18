package com.adelpos.meseros.core.domain.realtime

/**
 * Espera creciente entre intentos de reconexión: [initialMillis] tras la primera falla, el doble en
 * cada falla seguida y como mucho [maximumMillis]. Una conexión que llegó a recibir mensajes
 * reinicia la cuenta (Go cierra cada flujo a los 30 s y eso no es una falla).
 */
class ReconnectBackoff(
    private val initialMillis: Long = 1_000,
    private val maximumMillis: Long = 30_000,
) {
    init {
        require(initialMillis > 0 && maximumMillis >= initialMillis) {
            "La espera inicial debe ser positiva y no mayor que la máxima"
        }
    }

    /** Espera antes del intento que sigue a [failures] fallas seguidas (1, 2, 3…). */
    fun delayFor(failures: Int): Long {
        if (failures <= 0) {
            return 0
        }
        var delay = initialMillis
        repeat(failures - 1) {
            if (delay >= maximumMillis) {
                return maximumMillis
            }
            delay *= 2
        }
        return minOf(delay, maximumMillis)
    }
}
