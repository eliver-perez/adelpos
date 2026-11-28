package com.adelpos.meseros.core.domain.waiter

/** Lo que el mesero puede intentar en el modo Mesero, cada una con los permisos que exige Go. */
enum class WaiterAction(val requiredPermissions: Set<String>) {
    /** Ver áreas y mesas (`getRestaurantOverview`). */
    VIEW_TABLES(setOf("restaurant.tables.read", "restaurant.areas.read")),

    /** Ver las cuentas de una mesa y su contenido. */
    VIEW_ACCOUNTS(setOf("accounts.read")),

    /** Empezar una cuenta nueva en una mesa (nace con el primer producto). */
    OPEN_ACCOUNT(setOf("accounts.open", "orders.capture")),

    /**
     * «Descartar cuenta vacía» desde el panel de la mesa (etapa A3, §4.10): lo que exige Go en
     * `POST restaurant/accounts/{id}/discard` (además `takeaway.manage` si es para llevar).
     */
    DISCARD_EMPTY_ACCOUNT(setOf("accounts.open", "accounts.read")),

    /** Capturar y modificar lo no enviado (el borrador). */
    CAPTURE(setOf("orders.capture", "catalog.read", "catalog.prices.read")),

    /** «Enviar a cocina» (en línea o en un pedido sin conexión). */
    SEND_TO_KITCHEN(setOf("orders.confirm", "orders.capture", "accounts.read")),

    /** Modificar unidades ya enviadas (`editUnits`; reducirlas exige además cancelar). */
    EDIT_SENT(setOf("orders.edit", "accounts.read", "catalog.read", "catalog.prices.read")),

    /** Cancelar unidades ya enviadas (`cancelUnits`), con motivo. */
    CANCEL_SENT(setOf("orders.cancel", "accounts.read")),

    /** Ver «Para llevar» y la lista de pedidos para llevar con sus estados (A2, §3.1). */
    VIEW_TAKEAWAY(setOf("takeaway.read")),

    /**
     * Capturar en un pedido para llevar y cambiar el nombre y el teléfono (`takeaway.manage`; sin
     * él, solo la lista).
     */
    MANAGE_TAKEAWAY(setOf("takeaway.manage")),

    /** «Nuevo pedido» para llevar: nace con el primer producto (creación diferida). */
    OPEN_TAKEAWAY(setOf("takeaway.manage", "accounts.open", "orders.capture")),

    /** Marcar «Entregado» un pedido para llevar ya pagado (A2-D2). */
    DELIVER_TAKEAWAY(setOf("takeaway.deliver")),

    /**
     * «Imprimir precuenta» (o el nombre visible) desde el dispositivo (A2, §5): el permiso que ya
     * exige Go para imprimir tickets y precuentas.
     */
    PRINT_PRECHECK(setOf("payments.tickets.print")),

    /**
     * Etapa M1 (§1.3, M1-D8): «Unir mesas» (`restaurant.tables.join`, con lo que pide: abrir
     * cuentas y ver las mesas). Solo se ofrece si Go lo dio (`grants`).
     */
    JOIN_TABLES(setOf("restaurant.tables.join", "accounts.open", "restaurant.tables.read")),
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

    /**
     * Go **dio** el permiso (los permisos se conocen y lo incluyen). Para lo que solo debe aparecer
     * con el permiso de Go, como «Para llevar» en la navegación (§3.1) o «Entregado»: con permisos
     * desconocidos no se muestra.
     */
    fun grants(action: WaiterAction): Boolean =
        keys?.containsAll(action.requiredPermissions) == true

    /** Reducir la cantidad de lo enviado exige modificar y cancelar (Go responde 403 si no). */
    val canReduceSent: Boolean
        get() = allows(WaiterAction.EDIT_SENT) && allows(WaiterAction.CANCEL_SENT)

    companion object {
        val UNKNOWN = WaiterPermissions(null)
    }
}
