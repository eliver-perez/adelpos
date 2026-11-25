package com.adelpos.cocina.access

import com.adelpos.core.model.access.RepairReason
import com.adelpos.core.model.server.SemanticVersion
import com.adelpos.core.model.server.ServerRequirements

/** Estado de la pantalla de cocina frente a la computadora principal: decide qué se ve. */
sealed interface KitchenStatus {
    /** Leyendo lo guardado. */
    data object Starting : KitchenStatus

    /** Sin servidor emparejado: «Buscar el servidor» (también después de que la revoquen, §2). */
    data object Unpaired : KitchenStatus

    /**
     * Emparejada: la pantalla de pedidos. Si hay conexión no lo dice este estado sino el estado
     * único de conexión (`ServerLink`, etapa A3), como en la aplicación de meseros.
     */
    data class Ready(val serverName: String, val deviceName: String) : KitchenStatus

    /** «Esta tableta necesita emparejarse de nuevo». */
    data class NeedsRepair(val reason: RepairReason) : KitchenStatus

    /** «Actualiza esta aplicación» (426 o la mínima de cocina que anuncia el servidor). */
    data class UpdateApp(val appVersion: String, val minClientVersion: String?) : KitchenStatus

    /** «Actualiza el servidor del restaurante» (anterior a la 0.16.0 o sin la capacidad). */
    data class UpdateServer(val serverVersion: String?) : KitchenStatus
}

/**
 * Lo que la aplicación de cocina exige al servidor del restaurante (etapa K1, K1-D3): la 0.16.0 o
 * posterior con la capacidad `kitchen-display-v1` (la pantalla de cocina como tipo de dispositivo,
 * sus rutas y sus eventos). Un servidor anterior responde `CLIENT_KIND_INVALID` a una pantalla de
 * cocina: se dice «Actualiza el servidor del restaurante», como en la aplicación de meseros.
 */
object KitchenServerRequirements {
    const val CAPABILITY_KITCHEN_DISPLAY = "kitchen-display-v1"
    val MIN_SERVER_VERSION = SemanticVersion(0, 16, 0)

    val REQUIREMENTS = ServerRequirements(MIN_SERVER_VERSION, listOf(CAPABILITY_KITCHEN_DISPLAY))
}
