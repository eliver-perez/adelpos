package com.adelpos.cocina

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.printToString
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.cocina.board.BoardScreen
import com.adelpos.cocina.board.BoardState
import com.adelpos.cocina.board.DishState
import com.adelpos.cocina.board.DisplaySettings
import com.adelpos.cocina.board.DisplayTheme
import com.adelpos.cocina.board.RecentOrder
import com.adelpos.cocina.board.RecentState
import com.adelpos.cocina.board.TextSize
import com.adelpos.cocina.ui.TAG_CARD_LEVEL_PREFIX
import com.adelpos.cocina.ui.TAG_CARD_PREFIX
import com.adelpos.cocina.ui.TAG_CARD_READY_PREFIX
import com.adelpos.cocina.ui.TAG_CARD_WAIT_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_CANCELLED_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_NEW_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_READY_PREFIX
import com.adelpos.cocina.ui.TAG_NOTICE_CLOCK
import com.adelpos.cocina.ui.TAG_NOTICE_LICENSE
import com.adelpos.cocina.ui.TAG_ORDERS_GRID
import com.adelpos.cocina.ui.TAG_ORDERS_HIDDEN
import com.adelpos.cocina.ui.TAG_RECENT
import com.adelpos.cocina.ui.TAG_RECENT_MESSAGE
import com.adelpos.cocina.ui.TAG_RECENT_OPEN
import com.adelpos.cocina.ui.TAG_RECENT_RECOVER_PREFIX
import com.adelpos.core.model.license.LicenseClock
import com.adelpos.core.model.license.LicenseStateName
import com.adelpos.core.model.license.LicenseSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La pantalla de pedidos de la cocina (§4 de la etapa K1) con estados preparados, en la tableta en
 * horizontal: los casos de una tarjeta (mesa, para llevar, modificadores, notas, cancelados y
 * nuevos), los colores por tiempo con el reloj simulado, marcar un platillo y el pedido, el cierre
 * automático, «Recientes» con «Recuperar», muchos pedidos con su indicador y los dos tamaños de
 * letra por los dos temas. Guarda capturas en `files/screens`.
 */
@RunWith(AndroidJUnit4::class)
class OrdersScreenTest {
    @get:Rule val compose = createComposeRule()

    private val host = ScreenHost(compose)

    private fun exists(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    @Test
    fun losCasosDeUnaTarjeta() {
        host.show(board = Fixtures.board(Fixtures.mixed()))
        compose.onNodeWithText("Mesa 4").assertIsDisplayed()
        compose.onNodeWithText("Para llevar").assertIsDisplayed()
        compose.onNodeWithText("Luisa Martínez").assertIsDisplayed()
        // Mesa 4 y Mesa 7 son de Ana.
        compose.onAllNodesWithText("Ana López · Enviado a las 19:08")[0].assertIsDisplayed()
        compose.onNodeWithText("3 × Tacos al pastor").assertIsDisplayed()
        compose.onNodeWithText("SIN Cebolla").assertIsDisplayed()
        compose.onNodeWithText("1 x Verde").assertIsDisplayed()
        compose.onNodeWithText("Nota: Bien dorados").assertIsDisplayed()
        compose
            .onNodeWithTag("${TAG_DISH_NEW_PREFIX}agua", useUnmergedTree = true)
            .assertIsDisplayed()
        compose
            .onNodeWithTag("${TAG_DISH_CANCELLED_PREFIX}malteada", useUnmergedTree = true)
            .assertIsDisplayed()
        compose.onNodeWithText("NUEVO", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("CANCELADO", useUnmergedTree = true).assertIsDisplayed()
        // Lo listo se ve atenuado con su palomita; la estación «por pedido» lo explica.
        compose
            .onNodeWithTag("${TAG_DISH_READY_PREFIX}papas", useUnmergedTree = true)
            .assertIsDisplayed()
        compose
            .onNodeWithText("En esta estación se marca el pedido completo con el botón de abajo.")
            .assertIsDisplayed()
        compose
            .onNodeWithTag("${TAG_CARD_WAIT_PREFIX}mesa", useUnmergedTree = true)
            .assertIsDisplayed()
        screenshot("c1-tarjetas")
    }

    @Test
    fun losColoresPorTiempoConElRelojSimulado() {
        val card =
            Fixtures.card(
                "t",
                title = "Mesa 1",
                elapsedSeconds = 0,
                dishes = listOf(Fixtures.dish("a", "Sopa")),
            )
        host.clockMillis = 0
        host.show(board = Fixtures.board(listOf(card)))
        compose.onNodeWithText("0:00").assertIsDisplayed()
        assertTrue(!exists("${TAG_CARD_LEVEL_PREFIX}t"))
        // A los 10 minutos (límite de atención por omisión) cambia el color, con el reloj
        // monotónico de la tableta, no con el de pared.
        host.clockMillis = 600_000
        compose.waitUntil(timeoutMillis = 5_000) { exists("${TAG_CARD_LEVEL_PREFIX}t") }
        compose.onNodeWithText("Atención: lleva tiempo esperando").assertIsDisplayed()
        compose.onNodeWithText("10:00").assertIsDisplayed()
        host.clockMillis = 1_200_000
        compose.waitUntil(timeoutMillis = 5_000) {
            compose
                .onAllNodes(androidx.compose.ui.test.hasText("Retrasado"))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        compose.onNodeWithText("20:00").assertIsDisplayed()
        // Las tres a la vez: normal, atención y retraso, con los límites que manda Go.
        host.clockMillis = 0
        host.show(
            board =
                Fixtures.board(
                    listOf(
                        card.copy(id = "normal", title = "Mesa 2", elapsedSeconds = 200),
                        card.copy(id = "atencion", title = "Mesa 3", elapsedSeconds = 700),
                        card.copy(id = "retraso", title = "Mesa 5", elapsedSeconds = 1_300),
                    ),
                ),
        )
        compose
            .onNodeWithTag("${TAG_CARD_LEVEL_PREFIX}atencion", useUnmergedTree = true)
            .assertIsDisplayed()
        compose
            .onNodeWithTag("${TAG_CARD_LEVEL_PREFIX}retraso", useUnmergedTree = true)
            .assertIsDisplayed()
        screenshot("c2-colores")
    }

    @Test
    fun marcarPlatilloYPedidoYCierreAutomatico() {
        val cards = Fixtures.mixed()
        host.show(board = Fixtures.board(cards))
        compose.onNodeWithTag("${TAG_DISH_PREFIX}tacos").performClick()
        // Lo cancelado, lo listo y lo de una estación «por pedido» no se marcan al tocarlos.
        compose.onNodeWithTag("${TAG_DISH_PREFIX}malteada").performClick()
        compose.onNodeWithTag("${TAG_DISH_PREFIX}papas").performClick()
        compose.onNodeWithTag("${TAG_DISH_PREFIX}limonada").performClick()
        compose.onNodeWithTag("${TAG_CARD_READY_PREFIX}barra").performClick()
        compose.waitForIdle()
        assertEquals(listOf("dish:tacos", "order:barra"), host.actions.calls)
        // La confirmación visual: el platillo tocado se ve ya listo (atenuado con su palomita) y la
        // tarjeta del botón se va al momento.
        host.show(
            board =
                Fixtures.board(cards)
                    .copy(
                        markingDishes = setOf("tacos"),
                        markingCards = setOf("barra"),
                    ),
        )
        compose
            .onNodeWithTag("${TAG_DISH_READY_PREFIX}tacos", useUnmergedTree = true)
            .assertIsDisplayed()
        assertTrue(!exists("${TAG_CARD_PREFIX}barra"))
        screenshot("c3-marcado")
        // Cierre automático: cuando todo lo vivo de la tarjeta está listo, Go ya no la manda.
        host.show(board = Fixtures.board(cards.filter { it.id != "llevar" && it.id != "barra" }))
        assertTrue(!exists("${TAG_CARD_PREFIX}llevar"))
        compose.onNodeWithText("Mesa 4").assertIsDisplayed()
    }

    @Test
    fun conLaHoraIncorrectaSeVenLosPedidosPeroNoSeMarcan() {
        val blocked = LicenseSummary(LicenseStateName.ACTIVE, null, LicenseClock(true, null, null))
        host.show(board = Fixtures.board(Fixtures.mixed(), license = blocked))
        compose.onNodeWithTag(TAG_NOTICE_CLOCK).assertIsDisplayed()
        compose.onNodeWithText("Mesa 4").assertIsDisplayed()
        compose.onNodeWithTag("${TAG_CARD_READY_PREFIX}mesa").assertIsNotEnabled()
        compose.onNodeWithTag("${TAG_DISH_PREFIX}tacos").performClick()
        compose.waitForIdle()
        assertTrue(host.actions.calls.isEmpty())
        screenshot("c4-hora-incorrecta")
        // La licencia vencida no detiene la cocina: aviso y se sigue marcando (§2).
        val expired = LicenseSummary(LicenseStateName.EXPIRED, null, LicenseClock.NOT_BLOCKED)
        host.show(board = Fixtures.board(Fixtures.mixed(), license = expired))
        compose.onNodeWithTag(TAG_NOTICE_LICENSE).assertIsDisplayed()
        compose.onNodeWithTag("${TAG_CARD_READY_PREFIX}mesa").assertIsEnabled()
    }

    @Test
    fun recientesYRecuperar() {
        host.show(board = Fixtures.board(Fixtures.mixed()))
        compose.onNodeWithTag(TAG_RECENT_OPEN).performClick()
        compose.waitForIdle()
        assertEquals(listOf("recent"), host.actions.calls)
        val recent =
            RecentOrder(
                accountId = "cuenta-9",
                title = "Mesa 9",
                subtitle = "",
                waiterName = "Ana López",
                markedAtMillis = 1_795_569_000_000L,
                dishes =
                    listOf(
                        Fixtures.dish("sopa", "Sopa de tortilla", state = DishState.READY),
                        Fixtures.dish("flan", "Flan", state = DishState.READY),
                    ),
            )
        host.show(
            board = Fixtures.board(Fixtures.mixed()),
            recent = RecentState.Shown(listOf(recent), message = null),
        )
        compose.onNodeWithTag(TAG_RECENT).assertIsDisplayed()
        compose.onNodeWithText("Mesa 9").assertIsDisplayed()
        compose.onNodeWithText("Ana López · Listo a las 19:10").assertIsDisplayed()
        screenshot("c5-recientes")
        compose.onNodeWithTag("${TAG_RECENT_RECOVER_PREFIX}sopa").performClick()
        compose.waitForIdle()
        assertEquals(listOf("sopa-tarea"), host.actions.recovered.single().map { it.id })
        host.show(
            board = Fixtures.board(Fixtures.mixed()),
            recent =
                RecentState.Shown(
                    listOf(recent),
                    message = "Ese platillo ya no se puede regresar a la pantalla.",
                ),
        )
        compose.onNodeWithTag(TAG_RECENT_MESSAGE).assertIsDisplayed()
    }

    @Test
    fun muchosPedidosConSuIndicador() {
        val many =
            (1..18).map { number ->
                Fixtures.card(
                    "m$number",
                    title = "Mesa $number",
                    elapsedSeconds = number * 60L,
                    dishes =
                        listOf(
                            Fixtures.dish("m$number-a", "Enchiladas verdes", quantity = 2),
                            Fixtures.dish("m$number-b", "Agua de jamaica"),
                        ),
                )
            }
        host.show(board = Fixtures.board(many))
        compose.waitUntil(timeoutMillis = 5_000) { exists(TAG_ORDERS_HIDDEN) }
        compose.onNodeWithTag(TAG_ORDERS_HIDDEN, useUnmergedTree = true).assertIsDisplayed()
        screenshot("c6-muchos-pedidos")
        // Al desplazarse hasta el final, ya no queda nada abajo que avisar o el número baja.
        val before =
            compose.onNodeWithTag(TAG_ORDERS_HIDDEN, useUnmergedTree = true).printToString()
        compose.onNodeWithTag(TAG_ORDERS_GRID).performScrollToIndex(many.size - 1)
        compose.waitForIdle()
        compose.onNodeWithText("Mesa 18").assertIsDisplayed()
        assertTrue(before.isNotEmpty())
    }

    @Test
    fun losDosTamanosDeLetraPorLosDosTemas() {
        val heights = mutableMapOf<TextSize, Int>()
        for (theme in DisplayTheme.entries) {
            for (size in TextSize.entries) {
                val settings = DisplaySettings.DEFAULT.copy(theme = theme, textSize = size)
                host.show(board = Fixtures.board(Fixtures.mixed(), settings = settings))
                compose.onNodeWithText("3 × Tacos al pastor").assertIsDisplayed()
                heights[size] =
                    compose
                        .onNodeWithText("3 × Tacos al pastor", useUnmergedTree = true)
                        .fetchSemanticsNode()
                        .size
                        .height
                screenshot("c8-${theme.name.lowercase()}-${size.name.lowercase()}")
            }
        }
        // La letra grande agranda parejo (×1.25), también los nombres de los platillos.
        val normal = heights.getValue(TextSize.NORMAL)
        val large = heights.getValue(TextSize.LARGE)
        assertTrue("alto del platillo: $normal → $large", large >= normal * 1.2)
        // Sin pedidos y conectando también respetan el tema claro.
        host.show(
            board =
                BoardState(
                    screen = BoardScreen.Connecting,
                    settings = DisplaySettings.DEFAULT.copy(theme = DisplayTheme.LIGHT),
                ),
        )
        compose.onNodeWithText("Conectando con la computadora principal…").assertIsDisplayed()
    }
}
