package com.adelpos.meseros.core.domain.server

import com.adelpos.core.model.server.SemanticVersion
import com.adelpos.core.model.server.ServerRequirements

/**
 * Lo que la aplicación de meseros exige al servidor del restaurante (§4, «Versiones»): un servidor
 * 0.13.0 o posterior con las capacidades que usa: `waiter-pin-v1` (A1-D4: la entrada con PIN y la
 * credencial de envío) y, desde la A2, `takeaway-v1` (Para llevar: el pedido para llevar sin
 * conexión, el nombre y el teléfono y «Entregado»), `device-precheck-printer-v1` (la precuenta en
 * la impresora de este dispositivo) y `ready-alerts-v1` (el aviso de platillos listos con la
 * credencial del dispositivo); A2-D6, cada fase agrega la suya. Además, la mínima de Android que
 * anuncia el servidor (`min_client_versions.android`). La comprobación es la compartida
 * (`ServerCompatibility.check` de android-core, etapa K1).
 */
object WaiterServerRequirements {
    const val CAPABILITY_WAITER_PIN = "waiter-pin-v1"
    const val CAPABILITY_TAKEAWAY = "takeaway-v1"
    const val CAPABILITY_PRECHECK_PRINTER = "device-precheck-printer-v1"
    const val CAPABILITY_READY_ALERTS = "ready-alerts-v1"
    val MIN_SERVER_VERSION = SemanticVersion(0, 13, 0)

    /** Capacidades que esta versión de la aplicación exige al servidor. */
    val REQUIRED_CAPABILITIES: List<String> =
        listOf(
            CAPABILITY_WAITER_PIN,
            CAPABILITY_TAKEAWAY,
            CAPABILITY_PRECHECK_PRINTER,
            CAPABILITY_READY_ALERTS,
        )

    /** Las dos cosas juntas, para `ServerCompatibility.check` y el emparejamiento. */
    val REQUIREMENTS = ServerRequirements(MIN_SERVER_VERSION, REQUIRED_CAPABILITIES)
}
