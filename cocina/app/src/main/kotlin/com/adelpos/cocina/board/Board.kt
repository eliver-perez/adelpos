package com.adelpos.cocina.board

import com.adelpos.core.model.license.LicenseSummary
import com.adelpos.core.model.text.GoTime
import com.adelpos.core.network.license.toSummary

/** Tamaño de letra de la pantalla, elegido en Terminales (§4). */
enum class TextSize {
    NORMAL,
    LARGE,
}

/** Tema de la pantalla, elegido en Terminales: oscuro por omisión en la cocina (§4). */
enum class DisplayTheme {
    DARK,
    LIGHT,
}

/** La configuración de la pantalla que manda Go (`display` de `connect` y de `orders`). */
data class DisplaySettings(
    val textSize: TextSize,
    val theme: DisplayTheme,
    val warningMinutes: Int,
    val lateMinutes: Int,
    val soundEnabled: Boolean,
    val soundVolume: String,
    val stationCount: Int,
) {
    companion object {
        /** Antes de la primera respuesta de Go: tema oscuro, letra normal y 10 y 20 minutos. */
        val DEFAULT =
            DisplaySettings(
                textSize = TextSize.NORMAL,
                theme = DisplayTheme.DARK,
                warningMinutes = 10,
                lateMinutes = 20,
                soundEnabled = true,
                soundVolume = "medium",
                stationCount = 0,
            )
    }
}

/** Una tarea con la versión que vio la pantalla: lo que se manda al marcar o recuperar. */
data class TaskRef(val id: String, val versionId: String)

/** El estado de un platillo como lo dice Go. */
enum class DishState {
    PENDING,
    READY,
    CANCELLED,
}

/** Un renglón de lo que se prepara: un cambio o una opción de un grupo ([level] 1, con sangría). */
data class DishLine(val level: Int, val text: String)

/** Una parte de un platillo (un componente de combo o una variante) con sus renglones. */
data class DishPart(val name: String?, val lines: List<DishLine>)

/** Un platillo de una tarjeta. */
data class Dish(
    val id: String,
    val name: String,
    val quantity: Int,
    val state: DishState,
    val preparing: Boolean,
    val isNew: Boolean,
    val pendingChange: Boolean,
    /** La estación marca el pedido completo: tocar el platillo no lo marca (K1-D14). */
    val byOrder: Boolean,
    val notes: String,
    val parts: List<DishPart>,
    val tasks: List<TaskRef>,
)

/** Una tarjeta: una cuenta en las estaciones de la pantalla (K1-D5), tal como la arma Go. */
data class OrderCard(
    val id: String,
    val accountId: String,
    val title: String,
    val subtitle: String,
    val waiterName: String,
    /** Hora del primer envío de la ronda, en milisegundos (null si Go no la dio). */
    val sentAtMillis: Long?,
    /** La espera en `server_time` de la lectura. */
    val elapsedSeconds: Long,
    /** La ronda se cerró por una cancelación y espera que alguien la quite (K1-D23). */
    val closedByCancellation: Boolean,
    val dishes: List<Dish>,
)

/** Una cuenta de «Recientes». */
data class RecentOrder(
    val accountId: String,
    val title: String,
    val subtitle: String,
    val waiterName: String,
    val markedAtMillis: Long?,
    val dishes: List<Dish>,
)

/**
 * Lo que muestra la pantalla después de una lectura de `orders`: las tarjetas, la configuración, la
 * licencia, el cursor del canal y las comandas por confirmar. [readAtMonotonic] es el reloj
 * monotónico de la tableta al recibirla: la espera avanza con él desde [serverTimeMillis], nunca
 * con el reloj de pared de la tableta.
 */
data class Board(
    val cursor: Long,
    val serverTimeMillis: Long?,
    val readAtMonotonic: Long,
    val settings: DisplaySettings,
    val license: LicenseSummary?,
    val timezone: String,
    val cards: List<OrderCard>,
    val receiptsPending: List<ReceiptDto>,
)

/** De las formas de Go al modelo de la pantalla (sin reglas de negocio: todo lo decide Go). */
object BoardMapper {
    fun settings(dto: DisplayConfigDto): DisplaySettings =
        DisplaySettings(
            textSize = if (dto.textSize == "large") TextSize.LARGE else TextSize.NORMAL,
            theme = if (dto.theme == "light") DisplayTheme.LIGHT else DisplayTheme.DARK,
            warningMinutes = dto.warningMinutes,
            lateMinutes = dto.lateMinutes,
            soundEnabled = dto.soundEnabled,
            soundVolume = dto.soundVolume,
            stationCount = dto.stations.size,
        )

    fun board(dto: OrdersResponseDto, readAtMonotonic: Long): Board =
        Board(
            cursor = dto.cursor,
            serverTimeMillis = GoTime.epochMillis(dto.serverTime),
            readAtMonotonic = readAtMonotonic,
            settings = settings(dto.display),
            license = dto.license?.toSummary(),
            timezone = dto.establishment.timezone,
            cards = BoardRules.ordered(dto.orders.map(::card)),
            receiptsPending = dto.receiptsPending,
        )

    fun card(dto: OrderCardDto): OrderCard =
        OrderCard(
            id = dto.id,
            accountId = dto.accountId,
            title = dto.title,
            subtitle = dto.subtitle,
            waiterName = dto.waiterName,
            sentAtMillis = GoTime.epochMillis(dto.sentAt),
            elapsedSeconds = dto.elapsedSeconds.coerceAtLeast(0),
            closedByCancellation = dto.status == CARD_CANCELLED,
            dishes = dto.items.map(::dish),
        )

    fun recent(dto: RecentOrderDto): RecentOrder =
        RecentOrder(
            accountId = dto.accountId,
            title = dto.title,
            subtitle = dto.subtitle,
            waiterName = dto.waiterName,
            markedAtMillis = GoTime.epochMillis(dto.markedAt),
            dishes = dto.items.map(::dish),
        )

    fun dish(dto: OrderItemDto): Dish =
        Dish(
            id = dto.id,
            name = dto.name,
            quantity = dto.quantity,
            state =
                when (dto.status) {
                    STATUS_READY -> DishState.READY
                    STATUS_CANCELLED -> DishState.CANCELLED
                    else -> DishState.PENDING
                },
            preparing = dto.preparing,
            isNew = dto.isNew,
            pendingChange = dto.pendingChange,
            byOrder = dto.tracking == TRACKING_ORDER,
            notes = dto.notes.trim(),
            parts =
                dto.parts.map { part ->
                    DishPart(
                        name = part.name.takeIf { part.showName && it.isNotBlank() },
                        lines = part.lines.map { DishLine(it.level, it.text) },
                    )
                },
            tasks = dto.tasks.map { TaskRef(it.id, it.versionId) },
        )
}
