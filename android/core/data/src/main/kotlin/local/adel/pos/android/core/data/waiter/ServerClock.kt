package local.adel.pos.android.core.data.waiter

/**
 * La diferencia entre el reloj del dispositivo y la hora de Go (revisión R6, B4). Go rechaza un
 * pedido sin conexión con una hora de captura más de 10 minutos posterior a la suya, así que una
 * tableta con la hora mal nunca podría enviar: la hora de captura se corrige con esta diferencia.
 *
 * Se aprende de la cabecera `Date` de cada respuesta de Go ([observe]) y vive en memoria: después de
 * reiniciar, la primera respuesta la vuelve a dar (también la de un rechazo).
 */
class ServerClock(private val deviceClock: () -> Long = System::currentTimeMillis) {
    /** La última diferencia observada (se lee y se escribe desde varios hilos). */
    @Volatile private var offset: Long? = null

    /** Go respondió con la hora [serverMillis] cuando el reloj del dispositivo marcaba [deviceMillis]. */
    fun observe(serverMillis: Long, deviceMillis: Long) {
        offset = serverMillis - deviceMillis
    }

    /** Cuánto hay que sumar al reloj del dispositivo para tener la hora de Go; null si no se sabe. */
    fun offsetMillis(): Long? = offset

    /** Ahora, en la hora de Go si se conoce la diferencia (si no, la del dispositivo). */
    fun now(): Long = deviceClock() + (offsetMillis() ?: 0L)

    /** El reloj del dispositivo, sin corregir. */
    fun deviceNow(): Long = deviceClock()
}
