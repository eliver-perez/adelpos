package com.adelpos.core.model.text

import java.time.OffsetDateTime
import java.time.format.DateTimeParseException

/** Las fechas de Go en la API v1 (RFC 3339 con zona). */
object GoTime {
    /** Fecha RFC 3339 de Go («2026-11-16T14:35:00.123-06:00») en milisegundos, o null. */
    fun epochMillis(text: String): Long? =
        try {
            OffsetDateTime.parse(text).toInstant().toEpochMilli()
        } catch (_: DateTimeParseException) {
            null
        }
}
