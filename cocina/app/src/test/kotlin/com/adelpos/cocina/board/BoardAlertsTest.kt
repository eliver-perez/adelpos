package com.adelpos.cocina.board

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Qué suena y qué tarjeta se anima (§5.2): pedido nuevo, agregado, cancelación y cambio; nada en la
 * primera lectura, por lo que recupera esta pantalla ni por lo que otra marca listo; y al volver de
 * «Sin conexión», una sola vez por tipo aunque hayan llegado varios pedidos.
 */
class BoardAlertsTest {
    @Test
    fun laPrimeraLecturaNoSuenaNiAnima() {
        val alerts = BoardAlerts.between(null, board(card("t1", dish("a"))), emptySet())
        assertEquals(BoardAlerts.NONE, alerts)
    }

    @Test
    fun unaTarjetaNuevaSuenaComoPedidoNuevoYSeAnima() {
        val before = board(card("t1", dish("a")))
        val after = board(card("t1", dish("a")), card("t2", dish("b")))
        val alerts = BoardAlerts.between(before, after, emptySet())
        assertEquals(setOf(AlertSound.NEW_ORDER), alerts.sounds)
        assertEquals(setOf("t2"), alerts.freshCards)
    }

    @Test
    fun loAgregadoAUnaTarjetaTambienEsPedidoNuevo() {
        val before = board(card("t1", dish("a")))
        val after = board(card("t1", dish("a"), dish("b", isNew = true)))
        val alerts = BoardAlerts.between(before, after, emptySet())
        assertEquals(setOf(AlertSound.NEW_ORDER), alerts.sounds)
        assertEquals(setOf("t1"), alerts.freshCards)
    }

    @Test
    fun unaCancelacionSuenaComoCambio() {
        val before = board(card("t1", dish("a"), dish("b")))
        val after = board(card("t1", dish("a"), dish("b", state = DishState.CANCELLED)))
        val alerts = BoardAlerts.between(before, after, emptySet())
        assertEquals(setOf(AlertSound.CHANGE), alerts.sounds)
        assertEquals(setOf("t1"), alerts.freshCards)
        // Y la lectura siguiente, igual, ya no suena.
        assertEquals(BoardAlerts.NONE, BoardAlerts.between(after, after, emptySet()))
    }

    @Test
    fun unCambioPendienteSuenaComoCambioUnaVez() {
        val before = board(card("t1", dish("a")))
        val after = board(card("t1", dish("a", pendingChange = true)))
        assertEquals(
            setOf(AlertSound.CHANGE),
            BoardAlerts.between(before, after, emptySet()).sounds,
        )
        assertEquals(BoardAlerts.NONE, BoardAlerts.between(after, after, emptySet()))
    }

    @Test
    fun unaModificacionSuenaSoloComoCambio() {
        // Go la manda como la versión vieja cancelada y la nueva pendiente, en la misma tarjeta.
        val before = board(card("t1", dish("a")))
        val after =
            board(
                card("t1", dish("a", state = DishState.CANCELLED), dish("a2", isNew = true)),
            )
        assertEquals(
            setOf(AlertSound.CHANGE),
            BoardAlerts.between(before, after, emptySet()).sounds,
        )
    }

    @Test
    fun loQueOtraPantallaMarcaListoNoSuena() {
        val before = board(card("t1", dish("a"), dish("b")))
        val after = board(card("t1", dish("a", state = DishState.READY), dish("b")))
        assertEquals(BoardAlerts.NONE, BoardAlerts.between(before, after, emptySet()))
    }

    @Test
    fun loQueEstaPantallaRecuperaNoSuena() {
        val before = board()
        val after = board(card("t1", dish("a")))
        assertEquals(BoardAlerts.NONE, BoardAlerts.between(before, after, setOf("a-1")))
        // Si alguien lo recuperó en otra parte (no está en la lista), es como uno nuevo.
        assertEquals(
            setOf(AlertSound.NEW_ORDER),
            BoardAlerts.between(before, after, emptySet()).sounds,
        )
    }

    @Test
    fun alVolverDeSinConexionSuenaUnaSolaVezPorTipo() {
        // Lo último que se vio antes del corte, y lo que llegó mientras tanto: tres pedidos nuevos
        // y una cancelación.
        val before = board(card("t1", dish("a"), dish("b")))
        val after =
            board(
                card("t1", dish("a"), dish("b", state = DishState.CANCELLED)),
                card("t2", dish("c")),
                card("t3", dish("d")),
                card("t4", dish("e")),
            )
        val alerts = BoardAlerts.between(before, after, emptySet())
        assertEquals(setOf(AlertSound.NEW_ORDER, AlertSound.CHANGE), alerts.sounds)
        assertEquals(setOf("t1", "t2", "t3", "t4"), alerts.freshCards)
    }

    @Test
    fun unPedidoQueLlegaYaCanceladoEsCambio() {
        val before = board()
        val after = board(card("t1", dish("a", state = DishState.CANCELLED)))
        assertEquals(
            setOf(AlertSound.CHANGE),
            BoardAlerts.between(before, after, emptySet()).sounds,
        )
    }

    private fun board(vararg cards: OrderCard) =
        Board(
            cursor = 1,
            serverTimeMillis = 0,
            readAtMonotonic = 0,
            settings = DisplaySettings.DEFAULT,
            license = null,
            timezone = "America/Mexico_City",
            cards = cards.toList(),
            receiptsPending = emptyList(),
        )

    private fun card(id: String, vararg dishes: Dish) =
        OrderCard(
            id = id,
            accountId = "cuenta-$id",
            title = "Mesa $id",
            subtitle = "",
            waiterName = "Ana",
            sentAtMillis = 0,
            elapsedSeconds = 0,
            closedByCancellation = false,
            dishes = dishes.toList(),
        )

    // Cada platillo con su tarea «<id>-1»: las tareas mandan, no los platillos.
    private fun dish(
        id: String,
        state: DishState = DishState.PENDING,
        isNew: Boolean = false,
        pendingChange: Boolean = false,
    ) =
        Dish(
            id = id,
            name = "Platillo $id",
            quantity = 1,
            state = state,
            preparing = false,
            isNew = isNew,
            pendingChange = pendingChange,
            byOrder = false,
            notes = "",
            parts = emptyList(),
            tasks = listOf(TaskRef("$id-1", "v1")),
        )
}
