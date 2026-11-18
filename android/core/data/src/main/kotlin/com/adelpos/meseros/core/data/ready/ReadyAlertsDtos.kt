package com.adelpos.meseros.core.data.ready

import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyItem
import com.adelpos.meseros.core.domain.waiter.OrderMode
import java.time.OffsetDateTime
import java.time.format.DateTimeParseException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Respuesta de `GET ready-alerts` y contenido (`data`) de cada mensaje `alerts` de su canal
 * (`ready-alerts/stream`): `ReadyAlertsBatch` del contrato v1 (etapa A2, parte D del servidor).
 */
@Serializable
data class ReadyAlertsBatchDto(
    val cursor: Long,
    val active: Boolean,
    @SerialName("day_ends_at") val dayEndsAt: String,
    val settings: ReadyAlertSettingsDto,
    val alerts: List<ReadyAlertDto> = emptyList(),
) {
    /**
     * El lote del dominio; null si `day_ends_at` no es una fecha (respuesta que no se entiende).
     */
    fun toDomain(): ReadyAlertsBatch? {
        val dayEnds = epochMillis(dayEndsAt) ?: return null
        return ReadyAlertsBatch(
            cursor = cursor,
            active = active,
            dayEndsAtMillis = dayEnds,
            settings = settings.toDomain(),
            alerts = alerts.map(ReadyAlertDto::toDomain),
        )
    }

    private fun epochMillis(text: String): Long? =
        try {
            OffsetDateTime.parse(text).toInstant().toEpochMilli()
        } catch (_: DateTimeParseException) {
            null
        }
}

@Serializable
data class ReadyAlertSettingsDto(val enabled: Boolean, val sound: Boolean, val vibration: Boolean) {
    fun toDomain(): ReadyAlertSettings = ReadyAlertSettings(enabled, sound, vibration)
}

/** Un aviso (`ReadyAlert`): solo los datos del aviso, nunca precios ni importes. */
@Serializable
data class ReadyAlertDto(
    val id: Long,
    /** La identidad del aviso, única y nunca reutilizada (el `id` solo es el cursor). */
    val key: String = "",
    @SerialName("account_id") val accountId: String,
    val mode: String,
    @SerialName("table_name") val tableName: String? = null,
    val folio: Long = 0,
    @SerialName("customer_name") val customerName: String = "",
    @SerialName("waiter_user_id") val waiterUserId: String,
    @SerialName("waiter_short_name") val waiterShortName: String = "",
    val items: List<ReadyItemDto> = emptyList(),
    @SerialName("ready_at") val readyAt: String,
) {
    fun toDomain(): ReadyAlert =
        ReadyAlert(
            id = id,
            // Un servidor que no la mandara (no debería pasar con `ready-alerts-v1`): número y
            // hora.
            key = key.ifBlank { "$id@$readyAt" },
            accountId = accountId,
            mode = OrderMode.fromWireName(mode),
            tableName = tableName,
            folio = folio,
            customerName = customerName,
            waiterUserId = waiterUserId,
            waiterShortName = waiterShortName,
            items = items.map { ReadyItem(it.productName, it.quantity) },
            readyAt = readyAt,
        )
}

@Serializable
data class ReadyItemDto(
    @SerialName("product_name") val productName: String,
    val quantity: Int,
)

/**
 * Contenido de `event: interrupted` del canal de avisos: `{"code": …}` (antes `{"error",
 * "status"}`; se aceptan las dos formas).
 */
@Serializable
data class ReadyInterruptedDto(
    val code: String? = null,
    val error: String? = null,
    val status: Int? = null,
) {
    fun code(): String? = code ?: error
}
