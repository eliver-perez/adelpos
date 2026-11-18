package com.adelpos.meseros.core.domain.connection

/** Si el dispositivo tiene una red por la que intentar llegar al servidor del restaurante. */
enum class LinkState {
    ONLINE,
    OFFLINE,
}

/**
 * Lo que muestra el indicador de conexión de todas las pantallas: «En línea» o «Sin conexión» y, si
 * los hay, cuántos pedidos faltan por llegar al servidor.
 */
data class ConnectionSummary(val link: LinkState, val pendingOrders: Int) {
    init {
        require(pendingOrders >= 0) { "El número de pedidos por enviar no puede ser negativo" }
    }
}
