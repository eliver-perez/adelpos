package com.adelpos.cocina.board

import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.LicenseRefusal
import com.adelpos.core.model.error.LicenseSeat
import com.adelpos.core.model.error.RefusedLicense
import com.adelpos.core.model.license.LicenseStateName
import com.adelpos.core.model.license.LicenseSummary

/** El color de una tarjeta por su tiempo de espera (§4): normal, atención o retraso. */
enum class WaitLevel {
    NORMAL,
    ATTENTION,
    LATE,
}

/**
 * Qué muestra la pantalla de pedidos en vez de las tarjetas, según lo que respondió Go (la tabla de
 * errores de la sección 15 del contrato de la Fase A).
 */
sealed interface BoardScreen {
    /** Conectándose (`connect` todavía no responde). */
    data object Connecting : BoardScreen

    /** Las tarjetas. */
    data object Orders : BoardScreen

    /** Sin cupo de cocina (`LICENSE_SEAT_UNAVAILABLE`), con cuántos están en uso y el límite. */
    data class NoSeat(val inUse: Int?, val limit: Int?) : BoardScreen

    /** «Liberar cupo» en Terminales (`KITCHEN_DISPLAY_SEAT_RELEASED`): «Usar esta pantalla». */
    data object SeatReleased : BoardScreen

    /** La computadora principal sin licencia activada (`LICENSE_NOT_ACTIVATED`): sin pedidos. */
    data object NotActivated : BoardScreen
}

/** Qué hace la tableta con un rechazo de Go a una ruta de la pantalla. */
sealed interface BoardReaction {
    /** Mostrar otra pantalla en lugar de las tarjetas. */
    data class Show(val screen: BoardScreen) : BoardReaction

    /** Volver a `connect` (`KITCHEN_DISPLAY_NOT_CONNECTED`). */
    data object Reconnect : BoardReaction

    /** Volver a leer `orders` (la estación ya no es de la pantalla, el platillo cambió…). */
    data object Reread : BoardReaction

    /**
     * La hora de la computadora principal es incorrecta (`SYSTEM_CLOCK_INVALID`): se siguen
     * mostrando los pedidos y no se puede marcar (K1-D16).
     */
    data object ClockInvalid : BoardReaction

    /**
     * Un rechazo del dispositivo o de la versión: lo decide la entrada (`KitchenAccess.report`).
     */
    data class Access(val error: AppError) : BoardReaction

    /** Ninguna de las anteriores: se avisa con el mensaje de Go y se vuelve a leer. */
    data class Notice(val message: String?) : BoardReaction
}

/**
 * Las reglas de presentación de la pantalla de pedidos (§4 de la etapa K1), puras y sin reloj
 * propio: las tarjetas, sus rondas, sus estados y el cupo los decide Go; aquí solo el orden en que
 * se ven, el color por el tiempo de espera con los límites generales (K1-D17), qué se puede tocar y
 * qué pantalla corresponde a cada respuesta.
 */
object BoardRules {
    /** Del envío más antiguo al más nuevo (Go ya las manda así; sin hora, al final). */
    fun ordered(cards: List<OrderCard>): List<OrderCard> =
        cards.sortedWith(
            compareBy<OrderCard>({ it.sentAtMillis == null }, { it.sentAtMillis ?: 0L }),
        )

    /**
     * La espera de [card] ahora: la que dio Go en la lectura más lo que avanzó el reloj monotónico
     * de la tableta desde entonces ([nowMonotonic] y [Board.readAtMonotonic], en milisegundos).
     * Nunca el reloj de pared de la tableta, que puede estar mal.
     */
    fun waitSeconds(card: OrderCard, readAtMonotonic: Long, nowMonotonic: Long): Long {
        val since = (nowMonotonic - readAtMonotonic).coerceAtLeast(0) / MILLIS_PER_SECOND
        return card.elapsedSeconds + since
    }

    /** El color: atención desde [DisplaySettings.warningMinutes] y retraso desde los de retraso. */
    fun level(waitSeconds: Long, settings: DisplaySettings): WaitLevel =
        when {
            waitSeconds >= settings.lateMinutes * SECONDS_PER_MINUTE -> WaitLevel.LATE
            waitSeconds >= settings.warningMinutes * SECONDS_PER_MINUTE -> WaitLevel.ATTENTION
            else -> WaitLevel.NORMAL
        }

    /** La espera como «7:05» (minutos y segundos) o «1:02:05» desde una hora. */
    fun waitText(waitSeconds: Long): String {
        val total = waitSeconds.coerceAtLeast(0)
        val hours = total / SECONDS_PER_HOUR
        val minutes = (total % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE
        val seconds = total % SECONDS_PER_MINUTE
        return if (hours > 0) {
            "%d:%02d:%02d".format(hours, minutes, seconds)
        } else {
            "%d:%02d".format(minutes, seconds)
        }
    }

    /** Si se puede marcar en este momento: con la hora del servidor correcta (K1-D16). */
    fun markingAllowed(license: LicenseSummary?): Boolean = license?.clock?.blocked != true

    /**
     * Si tocar [dish] lo marca: solo pendiente, de una estación «por platillo» (K1-D14), sin una
     * modificación que cocina todavía no recibe y con la marca permitida.
     */
    fun dishTappable(dish: Dish, markingAllowed: Boolean): Boolean =
        markingAllowed &&
            dish.state == DishState.PENDING &&
            !dish.byOrder &&
            !dish.pendingChange &&
            dish.tasks.isNotEmpty()

    /**
     * Las tareas que manda el botón del pedido: **todas** las de la tarjeta tal como se ven
     * (pendientes, listas y canceladas): Go marca lo pendiente y da por vistas las cancelaciones.
     */
    fun orderTasks(card: OrderCard): List<TaskRef> = card.dishes.flatMap { it.tasks }.distinct()

    /** Si el botón del pedido se puede tocar. */
    fun orderButtonEnabled(card: OrderCard, markingAllowed: Boolean): Boolean =
        markingAllowed && orderTasks(card).isNotEmpty()

    /** Si todo lo vivo está listo y solo queda quitar la tarjeta (cerrada por una cancelación). */
    fun onlyDismiss(card: OrderCard): Boolean = card.dishes.none { it.state == DishState.PENDING }

    /**
     * Si la tarjeta tiene platillos de una estación «por pedido» (lleva el texto que lo explica).
     */
    fun hasByOrderDishes(card: OrderCard): Boolean =
        card.dishes.any { it.byOrder && it.state == DishState.PENDING }

    /** Cuántas tarjetas quedan fuera de la vista: las que no se ven, ni siquiera en parte. */
    fun hiddenCount(total: Int, visible: Int): Int = (total - visible).coerceAtLeast(0)

    /** Las tareas de los platillos de «Recientes» que se pueden regresar (los listos). */
    fun recoverableTasks(dishes: List<Dish>): List<TaskRef> =
        dishes.filter { it.state == DishState.READY }.flatMap { it.tasks }

    /** Si la licencia está vencida o revocada: se sigue mostrando y marcando, con un aviso. */
    fun licenseNotice(license: LicenseSummary?): LicenseStateName? =
        license?.state?.takeIf { it == LicenseStateName.EXPIRED || it == LicenseStateName.REVOKED }

    /** Qué hace la tableta con el rechazo [error] de una ruta de la pantalla (sección 15). */
    fun reactionTo(error: AppError): BoardReaction =
        when (error) {
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired,
            AppError.ServerIdentityChanged -> BoardReaction.Access(error)
            is AppError.LicenseRefused -> reactionTo(error.refusal)
            is AppError.Rejected ->
                when (error.code) {
                    SEAT_RELEASED -> BoardReaction.Show(BoardScreen.SeatReleased)
                    NOT_CONNECTED -> BoardReaction.Reconnect
                    STATION_SCOPE_DENIED,
                    KITCHEN_VALIDATION,
                    NOT_FOUND -> BoardReaction.Reread
                    CLIENT_KIND_MISMATCH -> BoardReaction.Access(error)
                    else -> BoardReaction.Notice(error.serverMessage)
                }
            else -> BoardReaction.Notice(error.serverMessage)
        }

    private fun reactionTo(refusal: LicenseRefusal): BoardReaction =
        when (refusal) {
            is LicenseRefusal.NoSeat ->
                if (
                    refusal.seat == LicenseSeat.KITCHEN_DISPLAYS ||
                        refusal.seat == LicenseSeat.UNKNOWN
                ) {
                    BoardReaction.Show(BoardScreen.NoSeat(refusal.inUse, refusal.limit))
                } else {
                    BoardReaction.Notice(null)
                }
            LicenseRefusal.ClockInvalid -> BoardReaction.ClockInvalid
            is LicenseRefusal.LicenseState ->
                if (refusal.state == RefusedLicense.NOT_ACTIVATED) {
                    BoardReaction.Show(BoardScreen.NotActivated)
                } else {
                    // Vencida o revocada no impiden la cocina (§2); si Go lo dijera, se vuelve a
                    // leer.
                    BoardReaction.Reread
                }
        }

    const val SEAT_RELEASED = "KITCHEN_DISPLAY_SEAT_RELEASED"
    const val NOT_CONNECTED = "KITCHEN_DISPLAY_NOT_CONNECTED"
    const val NOT_RECOVERABLE = "KITCHEN_NOT_RECOVERABLE"
    private const val STATION_SCOPE_DENIED = "STATION_SCOPE_DENIED"
    private const val KITCHEN_VALIDATION = "KITCHEN_VALIDATION"
    private const val NOT_FOUND = "NOT_FOUND"
    private const val CLIENT_KIND_MISMATCH = "CLIENT_KIND_MISMATCH"
    private const val MILLIS_PER_SECOND = 1000L
    private const val SECONDS_PER_MINUTE = 60L
    private const val SECONDS_PER_HOUR = 3600L
}
