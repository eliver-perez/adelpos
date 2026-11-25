package com.adelpos.cocina.board

import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.LicenseRefusal
import com.adelpos.core.model.error.LicenseSeat
import com.adelpos.core.model.error.RefusedLicense
import com.adelpos.core.model.license.LicenseClock
import com.adelpos.core.model.license.LicenseStateName
import com.adelpos.core.model.license.LicenseSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Las reglas de presentación de la pantalla de pedidos (§4 de la etapa K1): el orden de las
 * tarjetas, el tiempo de espera con el reloj monotónico, el color en los dos límites, el indicador
 * de fuera de la vista, qué se puede tocar y qué pantalla corresponde a cada respuesta de Go.
 */
class BoardRulesTest {
    private val settings = DisplaySettings.DEFAULT.copy(warningMinutes = 10, lateMinutes = 20)

    @Test
    fun lasTarjetasVanDelEnvioMasAntiguoAlMasNuevo() {
        val cards =
            listOf(
                card("c", sentAt = 3_000L),
                card("sin-hora", sentAt = null),
                card("a", sentAt = 1_000L),
                card("b", sentAt = 2_000L),
            )
        assertEquals(listOf("a", "b", "c", "sin-hora"), BoardRules.ordered(cards).map { it.id })
    }

    @Test
    fun laRondaYsusPlatillosComoLosArmaGo() {
        // Las rondas las calcula Go (K1-D5): la tableta muestra lo que llega, con «NUEVO» y
        // «CANCELADO» por platillo y la tarjeta cerrada por una cancelación.
        val board =
            BoardMapper.board(
                OrdersResponseDto(
                    cursor = 42,
                    serverTime = "2026-11-24T19:20:00Z",
                    orders =
                        listOf(
                            OrderCardDto(
                                id = "t2",
                                title = "Para llevar",
                                subtitle = "Luisa",
                                mode = "takeaway",
                                sentAt = "2026-11-24T19:15:00Z",
                                items =
                                    listOf(
                                        OrderItemDto(
                                            id = "i3",
                                            name = "Malteada",
                                            status = "cancelled",
                                        ),
                                    ),
                                status = "cancelled",
                            ),
                            OrderCardDto(
                                id = "t1",
                                title = "Mesa 4",
                                sentAt = "2026-11-24T19:08:40Z",
                                elapsedSeconds = 680,
                                items =
                                    listOf(
                                        OrderItemDto(
                                            id = "i1",
                                            name = "Tacos al pastor",
                                            quantity = 3,
                                            notes = " Bien dorados ",
                                            parts =
                                                listOf(
                                                    PartDto(
                                                        name = "Tacos al pastor",
                                                        showName = false,
                                                        lines =
                                                            listOf(
                                                                PartLineDto(0, "SIN Cebolla"),
                                                                PartLineDto(1, "1 x Verde"),
                                                            ),
                                                    ),
                                                ),
                                            tasks = listOf(TaskRefDto("t-1", "v-1")),
                                        ),
                                        OrderItemDto(
                                            id = "i2",
                                            name = "Agua",
                                            isNew = true,
                                            tracking = "order",
                                        ),
                                    ),
                            ),
                        ),
                ),
                readAtMonotonic = 0,
            )
        assertEquals(listOf("t1", "t2"), board.cards.map { it.id })
        val mesa = board.cards.first()
        val tacos = mesa.dishes.first()
        assertEquals("Bien dorados", tacos.notes)
        assertNull("sin show_name, el nombre de la parte no se repite", tacos.parts.single().name)
        assertEquals(listOf(0, 1), tacos.parts.single().lines.map { it.level })
        assertTrue(mesa.dishes[1].isNew)
        assertTrue(mesa.dishes[1].byOrder)
        val llevar = board.cards[1]
        assertTrue(llevar.closedByCancellation)
        assertEquals(DishState.CANCELLED, llevar.dishes.single().state)
        assertTrue("solo queda quitarla de la pantalla", BoardRules.onlyDismiss(llevar))
    }

    @Test
    fun laEsperaAvanzaConElRelojMonotonicoNoConElDeLaTableta() {
        val card = card("t", sentAt = 0, elapsed = 125)
        assertEquals(
            125,
            BoardRules.waitSeconds(card, readAtMonotonic = 10_000, nowMonotonic = 10_000),
        )
        assertEquals(
            185,
            BoardRules.waitSeconds(card, readAtMonotonic = 10_000, nowMonotonic = 70_999),
        )
        // Un reloj que no avanza (o retrocede) nunca resta.
        assertEquals(
            125,
            BoardRules.waitSeconds(card, readAtMonotonic = 10_000, nowMonotonic = 5_000),
        )
        assertEquals("2:05", BoardRules.waitText(125))
        assertEquals("0:00", BoardRules.waitText(0))
        assertEquals("1:02:05", BoardRules.waitText(3725))
    }

    @Test
    fun elColorEnLosDosLimites() {
        assertEquals(WaitLevel.NORMAL, BoardRules.level(599, settings))
        assertEquals(WaitLevel.ATTENTION, BoardRules.level(600, settings))
        assertEquals(WaitLevel.ATTENTION, BoardRules.level(1199, settings))
        assertEquals(WaitLevel.LATE, BoardRules.level(1200, settings))
        // Con los límites que manda Go (configuración general de cocina, K1-D17).
        val custom = settings.copy(warningMinutes = 8, lateMinutes = 15)
        assertEquals(WaitLevel.ATTENTION, BoardRules.level(8 * 60, custom))
        assertEquals(WaitLevel.LATE, BoardRules.level(15 * 60, custom))
    }

    @Test
    fun elIndicadorDeFueraDeLaVista() {
        assertEquals(0, BoardRules.hiddenCount(total = 6, visible = 6))
        assertEquals(9, BoardRules.hiddenCount(total = 15, visible = 6))
        assertEquals(0, BoardRules.hiddenCount(total = 0, visible = 0))
    }

    @Test
    fun queSePuedeTocarEnUnaEstacionPorPedidoYConLaHoraIncorrecta() {
        val pending = dish("p")
        assertTrue(BoardRules.dishTappable(pending, markingAllowed = true))
        assertFalse("hora incorrecta (K1-D16)", BoardRules.dishTappable(pending, false))
        assertFalse(
            "estación por pedido (K1-D14)",
            BoardRules.dishTappable(pending.copy(byOrder = true), true),
        )
        assertFalse(BoardRules.dishTappable(pending.copy(state = DishState.READY), true))
        assertFalse(BoardRules.dishTappable(pending.copy(state = DishState.CANCELLED), true))
        assertFalse(BoardRules.dishTappable(pending.copy(pendingChange = true), true))
        val card = card("t", sentAt = 0, dishes = listOf(pending.copy(byOrder = true)))
        assertTrue("el botón del pedido sí", BoardRules.orderButtonEnabled(card, true))
        assertFalse(BoardRules.orderButtonEnabled(card, false))
        assertTrue(BoardRules.hasByOrderDishes(card))
        assertFalse("solo por pedido: el texto va al pie", BoardRules.mixesTracking(card))
        val mixed = card.copy(dishes = card.dishes + pending.copy(id = "otro"))
        assertTrue(
            "mezclada: el texto va en cada platillo por pedido",
            BoardRules.mixesTracking(mixed),
        )
        assertTrue(BoardRules.markingAllowed(null))
        val blocked =
            LicenseSummary(
                LicenseStateName.ACTIVE,
                null,
                LicenseClock(blocked = true, localTimeText = null, lastActivityText = null),
            )
        assertFalse(BoardRules.markingAllowed(blocked))
    }

    @Test
    fun elBotonDelPedidoMandaTodasLasTareasDeLaTarjeta() {
        val card =
            card(
                "t",
                sentAt = 0,
                dishes =
                    listOf(
                        dish("a").copy(tasks = listOf(TaskRef("1", "v"), TaskRef("2", "v"))),
                        dish("b").copy(state = DishState.READY, tasks = listOf(TaskRef("3", "v"))),
                        dish("c")
                            .copy(state = DishState.CANCELLED, tasks = listOf(TaskRef("4", "v"))),
                    ),
            )
        assertEquals(listOf("1", "2", "3", "4"), BoardRules.orderTasks(card).map { it.id })
        assertEquals(
            listOf("3"),
            BoardRules.recoverableTasks(card.dishes).map { it.id },
        )
    }

    @Test
    fun quePantallaCorrespondeACadaRespuesta() {
        assertEquals(
            BoardReaction.Show(BoardScreen.NoSeat(inUse = 2, limit = 2)),
            BoardRules.reactionTo(
                AppError.LicenseRefused(
                    LicenseRefusal.NoSeat(LicenseSeat.KITCHEN_DISPLAYS, inUse = 2, limit = 2),
                    "LICENSE_SEAT_UNAVAILABLE",
                    null,
                    null,
                ),
            ),
        )
        assertEquals(
            BoardReaction.Show(BoardScreen.SeatReleased),
            BoardRules.reactionTo(rejected("KITCHEN_DISPLAY_SEAT_RELEASED")),
        )
        assertEquals(
            BoardReaction.Reconnect,
            BoardRules.reactionTo(rejected("KITCHEN_DISPLAY_NOT_CONNECTED")),
        )
        assertEquals(
            BoardReaction.Show(BoardScreen.NotActivated),
            BoardRules.reactionTo(
                AppError.LicenseRefused(
                    LicenseRefusal.LicenseState(RefusedLicense.NOT_ACTIVATED, null),
                    "LICENSE_NOT_ACTIVATED",
                    null,
                    null,
                ),
            ),
        )
        assertEquals(
            BoardReaction.ClockInvalid,
            BoardRules.reactionTo(
                AppError.LicenseRefused(
                    LicenseRefusal.ClockInvalid,
                    "SYSTEM_CLOCK_INVALID",
                    null,
                    null,
                ),
            ),
        )
        assertEquals(
            BoardReaction.Reread,
            BoardRules.reactionTo(rejected("STATION_SCOPE_DENIED", 403)),
        )
        assertEquals(
            BoardReaction.Reread,
            BoardRules.reactionTo(rejected("KITCHEN_VALIDATION", 400)),
        )
        val revoked = AppError.DeviceRevoked(serverMessage = null, requestId = null)
        assertEquals(BoardReaction.Access(revoked), BoardRules.reactionTo(revoked))
        val mismatch = rejected("CLIENT_KIND_MISMATCH", 400)
        assertEquals(BoardReaction.Access(mismatch), BoardRules.reactionTo(mismatch))
        assertEquals(
            BoardReaction.Notice("Ese platillo ya no se puede regresar a la pantalla."),
            BoardRules.reactionTo(
                AppError.Rejected(
                    409,
                    "KITCHEN_NOT_RECOVERABLE",
                    "Ese platillo ya no se puede regresar a la pantalla.",
                    null,
                ),
            ),
        )
        // Vencida o revocada: la cocina sigue (§2), con su aviso.
        val expired = LicenseSummary(LicenseStateName.EXPIRED, null, LicenseClock.NOT_BLOCKED)
        assertEquals(LicenseStateName.EXPIRED, BoardRules.licenseNotice(expired))
        assertNull(BoardRules.licenseNotice(expired.copy(state = LicenseStateName.ACTIVE)))
    }

    private fun rejected(code: String, status: Int = 409) =
        AppError.Rejected(status, code, null, null)

    private fun dish(id: String) =
        Dish(
            id = id,
            name = "Platillo $id",
            quantity = 1,
            state = DishState.PENDING,
            preparing = false,
            isNew = false,
            pendingChange = false,
            byOrder = false,
            notes = "",
            parts = emptyList(),
            tasks = listOf(TaskRef("task-$id", "v-$id")),
        )

    private fun card(
        id: String,
        sentAt: Long?,
        elapsed: Long = 0,
        dishes: List<Dish> = listOf(dish("$id-1")),
    ) =
        OrderCard(
            id = id,
            accountId = "cuenta-$id",
            title = "Mesa $id",
            subtitle = "",
            waiterName = "Ana",
            sentAtMillis = sentAt,
            elapsedSeconds = elapsed,
            closedByCancellation = false,
            dishes = dishes,
        )
}
