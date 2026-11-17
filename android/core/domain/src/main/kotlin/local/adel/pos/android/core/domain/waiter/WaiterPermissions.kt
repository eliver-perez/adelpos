package local.adel.pos.android.core.domain.waiter

/** Lo que el mesero puede intentar en el modo Mesero, cada una con los permisos que exige Go. */
enum class WaiterAction(val requiredPermissions: Set<String>) {
    /** Ver áreas y mesas (`getRestaurantOverview`). */
    VIEW_TABLES(setOf("restaurant.tables.read", "restaurant.areas.read")),

    /** Ver las cuentas de una mesa y su contenido. */
    VIEW_ACCOUNTS(setOf("accounts.read")),

    /** Empezar una cuenta nueva en una mesa (nace con el primer producto). */
    OPEN_ACCOUNT(setOf("accounts.open", "orders.capture")),

    /** Capturar y modificar lo no enviado (el borrador). */
    CAPTURE(setOf("orders.capture", "catalog.read", "catalog.prices.read")),

    /** «Enviar a cocina» (en línea o en un pedido sin conexión). */
    SEND_TO_KITCHEN(setOf("orders.confirm", "orders.capture", "accounts.read")),

    /** Modificar unidades ya enviadas (`editUnits`; reducirlas exige además cancelar). */
    EDIT_SENT(setOf("orders.edit", "accounts.read", "catalog.read", "catalog.prices.read")),

    /** Cancelar unidades ya enviadas (`cancelUnits`), con motivo. */
    CANCEL_SENT(setOf("orders.cancel", "accounts.read")),
}

/**
 * Permisos vigentes del mesero según Go (`user.permissions` de `getRecoveryState`; con PIN, ya
 * acotados a la lista de mesero, A1-D17). La aplicación **no decide** nada con ellos: solo
 * deshabilita, con su motivo, lo que Go rechazaría. El servidor no restringe mesas por mesero, así
 * que todas se muestran.
 *
 * Si no se conocen (el mesero entró sin conexión y el dispositivo nunca leyó sus permisos) no se
 * deshabilita nada: Go valida todo, incluidos los permisos, al recibir el pedido (§3.2).
 */
data class WaiterPermissions(val keys: Set<String>?) {
    val known: Boolean
        get() = keys != null

    fun allows(action: WaiterAction): Boolean {
        val granted = keys ?: return true
        return granted.containsAll(action.requiredPermissions)
    }

    /** Reducir la cantidad de lo enviado exige modificar y cancelar (Go responde 403 si no). */
    val canReduceSent: Boolean
        get() = allows(WaiterAction.EDIT_SENT) && allows(WaiterAction.CANCEL_SENT)

    companion object {
        val UNKNOWN = WaiterPermissions(null)
    }
}
