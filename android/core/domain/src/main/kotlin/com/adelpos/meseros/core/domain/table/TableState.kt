package com.adelpos.meseros.core.domain.table

/**
 * Estado de una mesa tal como lo calcula Go (`state` en la API v1). La aplicación solo lo muestra,
 * con el mismo color, texto e ícono que el escritorio (etapa 9D); nunca lo deduce.
 */
enum class TableState(val wireName: String) {
    FREE("free"),
    OCCUPIED("occupied"),
    AWAITING_PAYMENT("awaiting_payment"),
    PARTIAL_PAYMENT("partial_payment"),
    OUT_OF_SERVICE("out_of_service");

    companion object {
        /** Estado por su nombre en la API; null si esta versión de la aplicación no lo conoce. */
        fun fromWireName(name: String): TableState? = entries.firstOrNull { it.wireName == name }
    }
}
