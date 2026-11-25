package com.adelpos.core.model.error

/**
 * Por qué Go no permitió una operación por la licencia de la computadora principal (etapa L1, los
 * cinco códigos de la puerta de L1-D17, todos con 409). Go es la única autoridad: el dispositivo
 * nunca habla con el servidor de licencias, solo muestra lo que Go responde y arma el texto con el
 * código y sus `details` ([LicenseMessages]), nunca con el `message` de la respuesta.
 *
 * En todos los casos Go no guardó nada (ni la clave de operación): el mismo envío se puede repetir
 * con la misma clave cuando se resuelva (la hora se corrige, se libera un cupo o se renueva la
 * licencia).
 */
sealed interface LicenseRefusal {
    /**
     * La licencia está sin activar, vencida o revocada (`LICENSE_NOT_ACTIVATED`, `LICENSE_EXPIRED`
     * o `LICENSE_REVOKED`).
     *
     * @param operation la fila de la tabla de operaciones que Go rechazó (`details.operation`:
     *   `capture` al capturar pedidos, `administration` al emparejar…); null si Go no la dijo.
     */
    data class LicenseState(val state: RefusedLicense, val operation: String?) : LicenseRefusal {
        /** Go rechazó una captura (crear o modificar un pedido). */
        val capture: Boolean
            get() = operation == null || operation == CAPTURE_OPERATION
    }

    /**
     * No hay cupo de la licencia (`LICENSE_SEAT_UNAVAILABLE`): [inUse] es cuántos usan otras
     * terminales y [limit] los que trae la licencia; null si Go no los dijo. Un teléfono o tableta
     * de meseros pide el de comandero; el cupo que falta lo dice Go ([seat]), nunca se supone.
     */
    data class NoSeat(val seat: LicenseSeat, val inUse: Int?, val limit: Int?) : LicenseRefusal

    /**
     * La computadora principal tiene la fecha y hora incorrectas (`SYSTEM_CLOCK_INVALID`, §6.2 de
     * la especificación de la etapa). No es un problema de la licencia: el código no empieza con
     * `LICENSE` a propósito. El bloqueo se quita solo cuando se corrige el reloj.
     */
    data object ClockInvalid : LicenseRefusal

    companion object {
        /** `details.operation` de una captura (la fila «Capturar» de la tabla de la §6). */
        const val CAPTURE_OPERATION = "capture"

        const val NOT_ACTIVATED = "LICENSE_NOT_ACTIVATED"
        const val EXPIRED = "LICENSE_EXPIRED"
        const val REVOKED = "LICENSE_REVOKED"
        const val SEAT_UNAVAILABLE = "LICENSE_SEAT_UNAVAILABLE"
        const val CLOCK_INVALID = "SYSTEM_CLOCK_INVALID"

        /** Los cinco códigos de la puerta (L1-D17). */
        val CODES: Set<String> =
            setOf(NOT_ACTIVATED, EXPIRED, REVOKED, SEAT_UNAVAILABLE, CLOCK_INVALID)

        /**
         * El motivo de un rechazo de Go con [code] y lo que se leyó de sus `details`; null si
         * [code] no es de la licencia.
         */
        fun of(
            code: String?,
            operation: String? = null,
            seat: String? = null,
            inUse: Int? = null,
            limit: Int? = null,
        ): LicenseRefusal? =
            when (code) {
                NOT_ACTIVATED -> LicenseState(RefusedLicense.NOT_ACTIVATED, operation)
                EXPIRED -> LicenseState(RefusedLicense.EXPIRED, operation)
                REVOKED -> LicenseState(RefusedLicense.REVOKED, operation)
                SEAT_UNAVAILABLE ->
                    NoSeat(
                        seat = LicenseSeat.fromWireName(seat),
                        // Un número negativo no tiene sentido: se trata como no dicho.
                        inUse = inUse?.takeIf { it >= 0 },
                        limit = limit?.takeIf { it >= 0 },
                    )
                CLOCK_INVALID -> ClockInvalid
                else -> null
            }
    }
}

/** El estado de la licencia que impidió la operación. */
enum class RefusedLicense {
    NOT_ACTIVATED,
    EXPIRED,
    REVOKED,
}

/** Los cupos de la licencia (`details.seat`, §7 del contrato de licencias). */
enum class LicenseSeat(val wireName: String) {
    ORDER_TAKERS("order_takers"),
    CASH_REGISTERS("cash_registers"),
    KITCHEN_DISPLAYS("kitchen_displays"),

    /** Un cupo que esta versión no conoce (o Go no lo dijo): el texto no nombra ninguno. */
    UNKNOWN("");

    companion object {
        fun fromWireName(name: String?): LicenseSeat =
            entries.firstOrNull { it != UNKNOWN && it.wireName == name } ?: UNKNOWN
    }
}
