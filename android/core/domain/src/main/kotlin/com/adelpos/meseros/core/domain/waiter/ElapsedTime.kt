package com.adelpos.meseros.core.domain.waiter

/**
 * Tiempo que lleva ocupada una mesa o abierta una cuenta, con el formato del escritorio
 * (`elapsedTime` de `desktop/src/services/restaurant.ts`): «45 min», «5 h 12 min» y «3 d 14 h». Los
 * segundos los da Go; la aplicación solo suma lo que pasó desde la última lectura.
 */
object ElapsedTime {
    fun format(seconds: Long): String {
        val minutes = maxOf(0L, seconds) / SECONDS_PER_MINUTE
        if (minutes < MINUTES_PER_HOUR) {
            return "$minutes min"
        }
        val hours = minutes / MINUTES_PER_HOUR
        if (hours < HOURS_PER_DAY) {
            return "$hours h ${minutes % MINUTES_PER_HOUR} min"
        }
        return "${hours / HOURS_PER_DAY} d ${hours % HOURS_PER_DAY} h"
    }

    /**
     * Segundos de Go más los que pasaron en el dispositivo desde la lectura (reloj monótono, que no
     * cambia si alguien mueve la hora).
     */
    fun secondsNow(secondsAtRead: Long, readAtElapsedMillis: Long, nowElapsedMillis: Long): Long {
        val passed = maxOf(0L, nowElapsedMillis - readAtElapsedMillis) / MILLIS_PER_SECOND
        return secondsAtRead + passed
    }

    private const val SECONDS_PER_MINUTE = 60L
    private const val MINUTES_PER_HOUR = 60L
    private const val HOURS_PER_DAY = 24L
    private const val MILLIS_PER_SECOND = 1000L
}
