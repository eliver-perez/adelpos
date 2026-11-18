package local.adel.pos.android.core.data.realtime

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import local.adel.pos.android.core.domain.realtime.RealtimeEvent

/** Contenido (`data`) de cada mensaje `changes` del canal (`EventBatch` del contrato v1). */
@Serializable
data class EventBatchDto(
    val cursor: Long,
    val reset: Boolean = false,
    val events: List<EventDto> = emptyList(),
)

@Serializable
data class EventDto(
    val sequence: Long,
    val id: String,
    val version: Int = 1,
    val kind: String,
    val scope: String,
    @SerialName("station_id") val stationId: String? = null,
    @SerialName("aggregate_id") val aggregateId: String? = null,
    @SerialName("created_at") val createdAt: String? = null,
) {
    fun toDomain(): RealtimeEvent =
        RealtimeEvent(
            sequence = sequence,
            id = id,
            version = version,
            kind = kind,
            scope = scope,
            stationId = stationId,
            aggregateId = aggregateId,
            createdAt = createdAt,
        )
}

/** Contenido de `event: interrupted`: el código de error y el estado HTTP equivalente. */
@Serializable data class InterruptedDto(val error: String, val status: Int)

/**
 * Lo que la Fase D lee de `GET restaurant/state`: el cursor del canal en el momento de la lectura
 * (etapa A1, fase E del servidor) y la versión y época del catálogo.
 */
@Serializable
data class RecoveryCursorDto(
    @SerialName("event_cursor") val eventCursor: Long? = null,
    @SerialName("catalog_version") val catalogVersion: Long? = null,
    @SerialName("catalog_epoch") val catalogEpoch: String? = null,
)
