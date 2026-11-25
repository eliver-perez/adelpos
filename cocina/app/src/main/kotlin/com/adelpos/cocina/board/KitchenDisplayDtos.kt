package com.adelpos.cocina.board

import com.adelpos.core.network.license.LicenseSummaryDto
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/*
 * Las formas de las rutas de la pantalla de cocina (`kitchen-display/…`, capacidad
 * `kitchen-display-v1`; `docs/API_V1.md`, «Pantalla de cocina», y el contrato de la Fase A de la
 * etapa K1). Solo lo que usa la tableta; lo demás lo ignora `ApiJson`. Ningún precio.
 */

/** Una estación que muestra la pantalla, con su seguimiento efectivo (`item` u `order`). */
@Serializable
data class DisplayStationDto(
    val id: String,
    val name: String = "",
    val tracking: String = TRACKING_ITEM,
)

/** La configuración de la pantalla que se elige en Terminales y en la configuración de cocina. */
@Serializable
data class DisplayConfigDto(
    @SerialName("device_id") val deviceId: String = "",
    val name: String = "",
    @SerialName("text_size") val textSize: String = "normal",
    val theme: String = "dark",
    @SerialName("sound_enabled") val soundEnabled: Boolean = true,
    @SerialName("sound_volume") val soundVolume: String = "medium",
    @SerialName("warning_minutes") val warningMinutes: Int = 10,
    @SerialName("late_minutes") val lateMinutes: Int = 20,
    @SerialName("all_stations") val allStations: Boolean = false,
    val stations: List<DisplayStationDto> = emptyList(),
)

@Serializable data class EstablishmentDto(val name: String = "", val timezone: String = "")

/** Cuerpo de `kitchen-display/connect`: `{}` o, con «Usar esta pantalla», `take_seat`. */
@Serializable data class ConnectRequest(@SerialName("take_seat") val takeSeat: Boolean? = null)

/** Respuesta de `kitchen-display/connect`. */
@Serializable
data class ConnectResponseDto(
    val display: DisplayConfigDto = DisplayConfigDto(),
    val establishment: EstablishmentDto = EstablishmentDto(),
    val license: LicenseSummaryDto? = null,
    @SerialName("server_time") val serverTime: String = "",
)

/** Una tarea de preparación (una unidad) con la versión que vio la pantalla. */
@Serializable data class TaskRefDto(val id: String, @SerialName("version_id") val versionId: String)

/** Un renglón de una parte: un cambio (nivel 0) o una opción de un grupo (nivel 1). */
@Serializable data class PartLineDto(val level: Int = 0, val text: String = "")

/** Lo que se prepara de un platillo, con las mismas reglas de la comanda impresa. */
@Serializable
data class PartDto(
    val name: String = "",
    val quantity: Int = 1,
    @SerialName("show_name") val showName: Boolean = false,
    val lines: List<PartLineDto> = emptyList(),
)

/** Un platillo de una tarjeta: las unidades iguales de una partida, estación, envío y estado. */
@Serializable
data class OrderItemDto(
    val id: String,
    @SerialName("station_id") val stationId: String = "",
    @SerialName("station_name") val stationName: String = "",
    val name: String = "",
    val quantity: Int = 1,
    val status: String = STATUS_PENDING,
    val preparing: Boolean = false,
    @SerialName("is_new") val isNew: Boolean = false,
    @SerialName("pending_change") val pendingChange: Boolean = false,
    val tracking: String = TRACKING_ITEM,
    val notes: String = "",
    val parts: List<PartDto> = emptyList(),
    @SerialName("ready_at") val readyAt: String? = null,
    val tasks: List<TaskRefDto> = emptyList(),
)

/** Una tarjeta: una cuenta en las estaciones de la pantalla, en su ronda en curso (K1-D5). */
@Serializable
data class OrderCardDto(
    val id: String,
    @SerialName("account_id") val accountId: String = "",
    val folio: Int? = null,
    val mode: String = "dine_in",
    val title: String = "",
    val subtitle: String = "",
    @SerialName("waiter_name") val waiterName: String = "",
    @SerialName("sent_at") val sentAt: String = "",
    @SerialName("elapsed_seconds") val elapsedSeconds: Long = 0,
    val status: String = CARD_OPEN,
    val items: List<OrderItemDto> = emptyList(),
)

/** Una comanda que la pantalla ya muestra y nadie ha confirmado (K1-D12). */
@Serializable
data class ReceiptDto(
    @SerialName("ticket_id") val ticketId: String,
    @SerialName("station_id") val stationId: String,
)

/** Respuesta de `kitchen-display/orders`: el estado completo con su cursor. */
@Serializable
data class OrdersResponseDto(
    val cursor: Long = 0,
    @SerialName("server_time") val serverTime: String = "",
    val display: DisplayConfigDto = DisplayConfigDto(),
    val establishment: EstablishmentDto = EstablishmentDto(),
    val license: LicenseSummaryDto? = null,
    val orders: List<OrderCardDto> = emptyList(),
    @SerialName("receipts_pending") val receiptsPending: List<ReceiptDto> = emptyList(),
)

/** Cuerpo de las marcas y de «Recuperar». */
@Serializable data class TasksRequest(val tasks: List<TaskRefDto>)

/** Cuerpo del acuse de recepción. */
@Serializable data class ReceiptsRequest(val receipts: List<ReceiptDto>)

/** Lo que no se marcó o no se recuperó, y por qué (`already_ready`, `cancelled`, `changed`…). */
@Serializable data class SkippedDto(val id: String = "", val reason: String = "")

/** Respuesta de las marcas (idempotentes por estado y versión). */
@Serializable
data class MarkResultDto(
    val updated: Int = 0,
    val dismissed: Int = 0,
    val skipped: List<SkippedDto> = emptyList(),
)

/** Una cuenta de «Recientes»: lo marcado listo en los últimos minutos. */
@Serializable
data class RecentOrderDto(
    @SerialName("account_id") val accountId: String = "",
    val folio: Int? = null,
    val mode: String = "dine_in",
    val title: String = "",
    val subtitle: String = "",
    @SerialName("waiter_name") val waiterName: String = "",
    @SerialName("marked_at") val markedAt: String = "",
    val items: List<OrderItemDto> = emptyList(),
)

/** Respuesta de `kitchen-display/recent`. */
@Serializable
data class RecentResponseDto(
    @SerialName("server_time") val serverTime: String = "",
    @SerialName("window_minutes") val windowMinutes: Int = 30,
    val orders: List<RecentOrderDto> = emptyList(),
)

/** Respuesta de `kitchen-display/recover`. */
@Serializable
data class RecoverResultDto(val recovered: Int = 0, val skipped: List<SkippedDto> = emptyList())

const val TRACKING_ITEM = "item"
const val TRACKING_ORDER = "order"

/** El usuario y la contraseña de un administrador (ajustes en la tableta, K1-D13 y K1-D32). */
@Serializable data class AdministratorRequest(val username: String, val password: String)

@Serializable data class SettingsAccessDto(val authorized: Boolean = false)

const val STATUS_PENDING = "pending"
const val STATUS_READY = "ready"
const val STATUS_CANCELLED = "cancelled"
const val CARD_OPEN = "open"
const val CARD_CANCELLED = "cancelled"
