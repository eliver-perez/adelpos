package com.adelpos.meseros.core.domain.waiter

/**
 * La pantalla inicial de este dispositivo que se guarda en Administración › Terminales
 * (`terminal.home_screen` de `getIdentity`, etapa A3, §4.12 y A3-D7): un `android` solo admite
 * Mesas o Para llevar. Cualquier otro valor (`kitchen`, `cash`, uno desconocido o ninguno, por
 * ejemplo un servidor 0.13.0 con «Cocina» guardada) es Mesas.
 */
enum class DeviceHome {
    TABLES,
    TAKEAWAY;

    companion object {
        /** El valor de Go: solo `takeaway` es Para llevar. */
        fun fromWire(value: String?): DeviceHome =
            if (value == TAKEAWAY_WIRE) {
                TAKEAWAY
            } else {
                TABLES
            }

        /**
         * Si el mesero empieza en Para llevar: es la pantalla inicial **y** Go le da permiso de ver
         * los pedidos para llevar (sin él, la sección ni aparece: empieza en Mesas).
         */
        fun startsInTakeaway(home: DeviceHome, permissions: WaiterPermissions): Boolean =
            home == TAKEAWAY && permissions.grants(WaiterAction.VIEW_TAKEAWAY)

        private const val TAKEAWAY_WIRE = "takeaway"
    }
}
