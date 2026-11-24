package com.adelpos.meseros.core.domain.error

/**
 * Los textos de la licencia en el dispositivo (etapa L1, §8 de la especificación): **el único
 * lugar** donde se escriben. Salen del código de Go y de sus `details` ([LicenseRefusal]) o del
 * estado de la licencia que Go entrega en `auth/me` y `restaurant/state`, nunca del `message` de
 * una respuesta (ni del servidor de licencias, con el que el dispositivo no habla). Los vigila
 * `PlainLanguageTest` como cualquier texto de la interfaz.
 */
object LicenseMessages {
    /** Licencia sin activar, vencida o revocada al capturar (§8). */
    const val CAPTURE_NOT_ALLOWED =
        "La licencia del sistema no permite capturar pedidos. Avisa al encargado."

    /** Licencia sin activar, vencida o revocada en otra operación (imprimir, entregar…). */
    const val OPERATION_NOT_ALLOWED =
        "La licencia del sistema no permite esta operación. Avisa al encargado."

    /**
     * Al emparejar con una computadora principal sin activar: emparejar es administración y Go no
     * lo permite sin una licencia (L1-D6).
     */
    const val PAIRING_NOT_ACTIVATED =
        "La computadora principal no tiene la licencia activada. Pide al encargado que la active " +
            "y vuelve a intentarlo."

    /**
     * Hora incorrecta (§6.2): el rechazo y el aviso fijo dicen lo mismo, para que nadie lo confunda
     * con la licencia, la conexión o los permisos.
     */
    const val CLOCK_INVALID =
        "No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. " +
            "Avisa al encargado."

    /** Sin cupo, cuando Go no dijo cuál falta o con cuántos. */
    const val NO_SEAT = "No hay cupo libre en la licencia del sistema. Avisa al encargado."

    /** El texto de un rechazo de la licencia. */
    fun of(refusal: LicenseRefusal): String =
        when (refusal) {
            is LicenseRefusal.LicenseState ->
                if (refusal.capture) CAPTURE_NOT_ALLOWED else OPERATION_NOT_ALLOWED
            is LicenseRefusal.NoSeat -> noSeat(refusal)
            LicenseRefusal.ClockInvalid -> CLOCK_INVALID
        }

    /** El texto de un rechazo al emparejar: sin activar, que se active antes; lo demás, igual. */
    fun forPairing(refusal: LicenseRefusal): String =
        if (
            refusal is LicenseRefusal.LicenseState && refusal.state == RefusedLicense.NOT_ACTIVATED
        ) {
            PAIRING_NOT_ACTIVATED
        } else {
            of(refusal)
        }

    /**
     * Sin cupo (§5 de la especificación y los mensajes de L1-D27), con cuántos están en uso y el
     * límite de la licencia. Con límite 0 la licencia no trae ese cupo: «pide que se cierre otra
     * sesión» no tendría sentido.
     */
    private fun noSeat(refusal: LicenseRefusal.NoSeat): String {
        val inUse = refusal.inUse
        val limit = refusal.limit
        if (inUse == null || limit == null) {
            return when (refusal.seat) {
                LicenseSeat.ORDER_TAKERS ->
                    "No hay cupo de comandero libre. Pide que se cierre la sesión de otra terminal."
                LicenseSeat.CASH_REGISTERS ->
                    "No hay cupo de caja libre. Se libera al cerrar un turno de caja."
                LicenseSeat.KITCHEN_DISPLAYS ->
                    "No hay cupo de pantalla de cocina libre. Pide que otra terminal salga de " +
                        "Preparación."
                LicenseSeat.UNKNOWN -> NO_SEAT
            }
        }
        val usage = "($inUse de $limit en uso)"
        return when (refusal.seat) {
            LicenseSeat.ORDER_TAKERS ->
                if (limit == 0) {
                    "La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una " +
                        "terminal con el turno de caja abierto."
                } else {
                    "No hay cupo de comandero libre $usage. Pide que se cierre la sesión de otra " +
                        "terminal."
                }
            LicenseSeat.CASH_REGISTERS ->
                if (limit == 0) {
                    "La licencia no incluye cajas (0 de 0). Avisa al encargado."
                } else {
                    "No hay cupo de caja libre $usage. Se libera al cerrar un turno de caja."
                }
            LicenseSeat.KITCHEN_DISPLAYS ->
                if (limit == 0) {
                    "La licencia no incluye pantallas de cocina (0 de 0): Preparación no está " +
                        "disponible."
                } else {
                    "No hay cupo de pantalla de cocina libre $usage. Pide que otra terminal salga " +
                        "de Preparación."
                }
            LicenseSeat.UNKNOWN -> NO_SEAT
        }
    }

    /**
     * La causa del aviso fijo de la hora con las dos fechas que da Go, ya escritas en la zona del
     * restaurante («lunes 23 nov 2026, 10:15»); null si falta alguna.
     */
    fun clockDetail(localTimeText: String?, lastActivityText: String?): String? {
        if (localTimeText.isNullOrBlank() || lastActivityText.isNullOrBlank()) {
            return null
        }
        return "La computadora principal tiene la hora $localTimeText y la última actividad " +
            "registrada fue el $lastActivityText."
    }

    /**
     * El aviso discreto de la barra con la licencia en tolerancia: cuántos días quedan antes de que
     * se bloquee la captura ([days], el `grace_days_remaining` de Go).
     */
    fun graceShort(days: Int): String =
        when {
            days <= 0 -> "Licencia: último día"
            days == 1 -> "Licencia: queda 1 día"
            else -> "Licencia: quedan $days días"
        }

    /** El mismo aviso completo, al tocarlo. */
    fun graceLong(days: Int): String {
        val left =
            when {
                days <= 0 -> "Hoy es el último día para renovarla"
                days == 1 -> "Queda 1 día para renovarla"
                else -> "Quedan $days días para renovarla"
            }
        return "La licencia del sistema venció. $left; después ya no se podrán capturar " +
            "pedidos. Avisa al encargado."
    }
}

/**
 * El texto de un error para el mesero: el de la licencia sale de su código ([LicenseMessages]); los
 * demás, el mensaje en español de Go (null si Go no lo dio).
 */
fun AppError.shownMessage(): String? =
    when (this) {
        is AppError.LicenseRefused -> LicenseMessages.of(refusal)
        else -> serverMessage
    }
