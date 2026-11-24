package com.adelpos.meseros.core.domain.license

import com.adelpos.meseros.core.domain.error.LicenseMessages
import kotlinx.coroutines.flow.StateFlow

/**
 * El estado de la licencia de la computadora principal tal como lo da Go (etapa L1: `license` de
 * `auth/me` y de `restaurant/state`, `LicenseSummary` del contrato v1). Go es la única autoridad:
 * los días ya vienen calculados y el dispositivo nunca habla con el servidor de licencias. Solo lo
 * que usan los avisos de la barra; lo demás lo ignora la lectura.
 *
 * @param graceDaysRemaining en Tolerancia, los días que quedan antes de que se bloquee la captura
 *   (`grace_days_remaining`); null en los demás estados.
 */
data class LicenseSummary(
    val state: LicenseStateName,
    val graceDaysRemaining: Int?,
    val clock: LicenseClock,
)

/**
 * La hora de la computadora principal (`LicenseClock`, §6.2 de la especificación): [blocked] es
 * «Hora del equipo incorrecta». Los textos son las dos fechas que Go ya escribió en la zona del
 * restaurante («lunes 23 nov 2026, 10:15»); null si no las dio.
 */
data class LicenseClock(
    val blocked: Boolean,
    val localTimeText: String?,
    val lastActivityText: String?,
) {
    companion object {
        val NOT_BLOCKED =
            LicenseClock(blocked = false, localTimeText = null, lastActivityText = null)
    }
}

/** `LicenseSummary.state` del contrato v1. */
enum class LicenseStateName(val wireName: String) {
    ACTIVE("active"),
    GRACE("grace"),
    EXPIRED("expired"),
    REVOKED("revoked"),
    NOT_ACTIVATED("not_activated"),
    NOT_ACTIVATED_WITH_DATA("not_activated_with_data"),

    /** Un estado que esta versión no conoce: ningún aviso de la licencia. */
    UNKNOWN("");

    companion object {
        fun fromWireName(name: String?): LicenseStateName =
            entries.firstOrNull { it != UNKNOWN && it.wireName == name } ?: UNKNOWN
    }
}

/**
 * Lo último que dijo Go de la licencia (null si todavía no lo dijo en este proceso o es un servidor
 * anterior a la etapa, que no lo envía). Lo alimentan las lecturas de `restaurant/state` y
 * `auth/me`; el evento `license.changed` del canal hace volver a leer.
 */
interface LicenseStatusSource {
    val summary: StateFlow<LicenseSummary?>
}

/**
 * El aviso fijo de la hora incorrecta (§6.2): el texto del rechazo y, si Go las dio, las fechas.
 */
data class ClockNotice(val text: String, val detail: String?)

/** El aviso discreto de la barra con la licencia en tolerancia: corto y completo (al tocarlo). */
data class GraceNotice(val days: Int, val short: String, val long: String)

/** Qué avisos de la licencia se muestran, a partir de lo que dijo Go (ninguna regla propia). */
object LicenseNotices {
    /** Mientras Go diga que la hora está bloqueada, en todas las pantallas. */
    fun clock(summary: LicenseSummary?): ClockNotice? {
        val clock = summary?.clock ?: return null
        if (!clock.blocked) {
            return null
        }
        return ClockNotice(
            text = LicenseMessages.CLOCK_INVALID,
            detail = LicenseMessages.clockDetail(clock.localTimeText, clock.lastActivityText),
        )
    }

    /** Solo en Tolerancia, con los días que Go calculó. */
    fun grace(summary: LicenseSummary?): GraceNotice? {
        if (summary?.state != LicenseStateName.GRACE) {
            return null
        }
        val days = summary.graceDaysRemaining ?: return null
        return GraceNotice(
            days = days,
            short = LicenseMessages.graceShort(days),
            long = LicenseMessages.graceLong(days),
        )
    }
}
