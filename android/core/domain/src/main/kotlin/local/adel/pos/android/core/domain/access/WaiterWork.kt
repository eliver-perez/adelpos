package local.adel.pos.android.core.domain.access

/**
 * Trabajo de un mesero en este dispositivo que importa para bloquear o cambiar de mesero (§0-B,
 * §3.2 y §7). La captura y la cola de pedidos llegan en la Fase E; hasta entonces nadie tiene
 * trabajo ([NoWaiterWork]).
 *
 * @param captureInProgress hay una captura sin enviar (un borrador abierto).
 * @param unsentOrders pedidos de ese mesero en «Por enviar», «Enviando» o «Requiere tu revisión».
 */
data class WaiterWork(val captureInProgress: Boolean, val unsentOrders: Int) {
    init {
        require(unsentOrders >= 0) { "El número de pedidos sin enviar no puede ser negativo" }
    }

    /**
     * El bloqueo por inactividad no ocurre con una captura en curso ni con pedidos sin enviar (§0-B
     * y §3.2).
     */
    val blocksIdleLock: Boolean
        get() = captureInProgress || unsentOrders > 0

    /**
     * Cambiar de mesero no se permite con pedidos sin enviar (§7). Una captura en curso sí lo
     * permite: queda guardada con el mesero que la capturó y el siguiente no la ve (§3.2).
     */
    val blocksSwitch: Boolean
        get() = unsentOrders > 0

    companion object {
        val NONE = WaiterWork(captureInProgress = false, unsentOrders = 0)
    }
}

/**
 * «¿Este mesero tiene trabajo que impide bloquear o cambiar de mesero?». La Fase E la implementa
 * con la captura y la cola sin conexión (Room, por mesero).
 */
fun interface WaiterWorkGate {
    suspend fun workOf(userId: String): WaiterWork
}

/** Mientras no existen la captura ni la cola (Fase E): nadie tiene trabajo pendiente. */
object NoWaiterWork : WaiterWorkGate {
    override suspend fun workOf(userId: String): WaiterWork = WaiterWork.NONE
}

/**
 * Un pedido que no se envió y queda en la bitácora del dispositivo al retirarlo (§3.2). Solo texto
 * para mostrar: la mesa, lo capturado y quién lo capturó (nunca credenciales).
 */
data class UnsentOrder(
    val waiterName: String,
    val description: String,
    val capturedAtEpochMillis: Long,
)

/**
 * Pedidos sin enviar del dispositivo, para avisar antes de borrar todo al retirarlo o al
 * emparejarlo de nuevo. La Fase E la implementa con la cola; hoy no hay pedidos ([NoUnsentOrders]).
 */
fun interface UnsentOrdersSource {
    suspend fun unsentOrders(): List<UnsentOrder>
}

/** Mientras no existe la cola (Fase E): ningún pedido sin enviar. */
object NoUnsentOrders : UnsentOrdersSource {
    override suspend fun unsentOrders(): List<UnsentOrder> = emptyList()
}
