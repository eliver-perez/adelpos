package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.money.Money
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa M1, §5.5 (prueba de reproducción de la Fase 0; la Fase C cambió la regla): al entrar a una
 * mesa, el mesero ve **una sola tarjeta por cuenta** —con su folio, total, envío y cocina— y la
 * marca «Tu cuenta» en la suya; lo que todavía no tiene cuenta (un pedido sin conexión por enviar)
 * sigue como su propia tarjeta «Por enviar»; ya no hay «Lo tuyo en esta mesa».
 *
 * Antes de la Fase C fallaba: el pedido que el restaurante ya recibió se veía dos veces («Mesa 2 ·
 * 4 productos · Recibido por el restaurante» en «Lo tuyo en esta mesa» y «Mesa 2 · folio 27 ·
 * $280.00 · Listo» en «Cuentas abiertas») y nada decía cuál era la cuenta del mesero.
 */
class StageM1TableCardsTest {
    private val other = "77777777-7777-4777-8777-777777777777"

    private fun account(id: String, folio: Int, assignedUserId: String?) =
        AccountSummary(
            id = id,
            folio = folio,
            name = "Mesa 2",
            tableId = WaiterFixtures.TABLE,
            tableName = "Mesa 2",
            state = AccountSummary.STATE_OPEN,
            revision = 4,
            preparationState = PreparationState.READY,
            confirmedQuantity = 4,
            draftQuantity = 0,
            totals = Totals(Money("280.00"), Money("0.00"), Money("280.00")),
            assignedName = "Ana",
            elapsedSeconds = 600,
            longOpen = false,
            assignedUserId = assignedUserId,
        )

    /** El pedido de Ana que el restaurante ya recibió y quedó en [accountId] (estado RECEIVED). */
    private fun received(accountId: String) =
        WaiterFixtures.order(
                WaiterFixtures.capture(
                    base = emptyList(),
                    operations =
                        listOf(
                            DraftOperation.Add(
                                WaiterFixtures.draft("d1", WaiterFixtures.BURGER, 4),
                            ),
                        ),
                    accountId = null,
                ),
                state = OrderState.RECEIVED,
                operationId = "op-recibido",
            )
            .copy(receivedAccountId = accountId, receivedAtMillis = 3_000L)

    @Test
    fun unaSolaTarjetaPorCuentaConTuCuenta() {
        val mine = account(WaiterFixtures.ACCOUNT, 27, WaiterFixtures.ANA)
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(mine),
                captures = emptyList(),
                orders = listOf(received(mine.id)),
                userId = WaiterFixtures.ANA,
            )
        assertEquals("una sola tarjeta para la cuenta folio 27: $cards", 1, cards.size)
        val card = cards.single() as TableCard.AccountCard
        assertEquals(mine.id, card.account.id)
        assertTrue("la cuenta es de Ana: «Tu cuenta»", card.mine)
        assertEquals(listOf("op-recibido"), card.orders.map { it.operationId })
    }

    @Test
    fun loQueAunNoTieneCuentaSigueComoSuPropiaTarjeta() {
        val theirs = account("88888888-8888-4888-8888-888888888888", 31, other)
        val queued =
            WaiterFixtures.order(
                WaiterFixtures.capture(
                    operations =
                        listOf(
                            DraftOperation.Add(WaiterFixtures.draft("d2", WaiterFixtures.TACOS)),
                        ),
                    accountId = null,
                ),
                state = OrderState.QUEUED,
                operationId = "op-por-enviar",
            )
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(theirs),
                captures = emptyList(),
                orders = listOf(queued),
                userId = WaiterFixtures.ANA,
            )
        assertEquals(2, cards.size)
        val accountCard = cards.filterIsInstance<TableCard.AccountCard>().single()
        assertFalse("la cuenta de otro mesero no dice «Tu cuenta»", accountCard.mine)
        val orderCard = cards.filterIsInstance<TableCard.OrderCard>().single()
        assertEquals("op-por-enviar", orderCard.order.operationId)
    }

    @Test
    fun unPedidoPorEnviarAUnaCuentaVaEnLaTarjetaDeEsaCuenta() {
        val mine = account(WaiterFixtures.ACCOUNT, 27, WaiterFixtures.ANA)
        val queued =
            WaiterFixtures.order(
                WaiterFixtures.capture(
                    operations =
                        listOf(
                            DraftOperation.Add(WaiterFixtures.draft("d3", WaiterFixtures.SHAKE)),
                        ),
                ),
                state = OrderState.QUEUED,
                operationId = "op-a-la-cuenta",
            )
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(mine),
                captures = emptyList(),
                orders = listOf(queued),
                userId = WaiterFixtures.ANA,
            )
        assertEquals(
            "el pedido por enviar a la cuenta folio 27 va en su tarjeta: $cards",
            1,
            cards.size,
        )
        val card = cards.single() as TableCard.AccountCard
        assertTrue(card.mine)
        assertEquals(listOf("op-a-la-cuenta"), card.orders.map { it.operationId })
    }

    @Test
    fun elPedidoRecibidoDiceRecibidoEnSuTarjetaYElPorEnviarManda() {
        val mine = account(WaiterFixtures.ACCOUNT, 27, WaiterFixtures.ANA)
        val received = received(mine.id)
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(mine),
                captures = emptyList(),
                orders = listOf(received),
                userId = WaiterFixtures.ANA,
            )
        val card = cards.single() as TableCard.AccountCard
        assertEquals(CardDelivery.Received(received), card.delivery)
        val queued =
            WaiterFixtures.order(
                    WaiterFixtures.capture(
                        operations =
                            listOf(
                                DraftOperation.Add(
                                    WaiterFixtures.draft("d4", WaiterFixtures.SHAKE),
                                ),
                            ),
                    ),
                    state = OrderState.QUEUED,
                    operationId = "op-nuevo",
                )
                .copy(capturedAtMillis = 9_000L)
        val again =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(mine),
                captures = emptyList(),
                orders = listOf(received, queued),
                userId = WaiterFixtures.ANA,
            )
        val both = again.single() as TableCard.AccountCard
        assertEquals(
            "lo por enviar manda sobre lo recibido",
            CardDelivery.Order(queued),
            both.delivery,
        )
    }

    @Test
    fun unaCapturaDeCuentaNuevaEsSuTarjetaYLaVaciaNo() {
        val working =
            WaiterFixtures.capture(
                operations =
                    listOf(DraftOperation.Add(WaiterFixtures.draft("d5", WaiterFixtures.TACOS))),
                accountId = null,
            )
        val idle = WaiterFixtures.capture(accountId = null).copy(id = "capture-vacia")
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = emptyList(),
                captures = listOf(working, idle),
                orders = emptyList(),
                userId = WaiterFixtures.ANA,
            )
        assertEquals(listOf(TableCard.CaptureCard(working)), cards)
    }

    @Test
    fun unaCapturaEnLaCuentaVaEnSuTarjetaConPorEnviar() {
        val mine = account(WaiterFixtures.ACCOUNT, 27, WaiterFixtures.ANA)
        val working =
            WaiterFixtures.capture(
                operations =
                    listOf(DraftOperation.Add(WaiterFixtures.draft("d6", WaiterFixtures.TACOS))),
            )
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = listOf(mine),
                captures = listOf(working),
                orders = emptyList(),
                userId = WaiterFixtures.ANA,
            )
        val card = cards.single() as TableCard.AccountCard
        assertEquals(CardDelivery.CaptureWork(working), card.delivery)
    }

    @Test
    fun unPedidoRecibidoEnUnaCuentaQueYaNoEstaAbiertaNoSeMuestra() {
        val cards =
            TableCards.of(
                tableId = WaiterFixtures.TABLE,
                accounts = emptyList(),
                captures = emptyList(),
                orders = listOf(received(WaiterFixtures.ACCOUNT)),
                userId = WaiterFixtures.ANA,
            )
        assertTrue(cards.isEmpty())
    }

    @Test
    fun sinPreparacionLaCuentaNoLlevaEtiquetaDeCocina() {
        val drinks = account(WaiterFixtures.ACCOUNT, 27, other).copy(preparationRequired = false)
        val card =
            TableCards.of(
                    tableId = WaiterFixtures.TABLE,
                    accounts = listOf(drinks),
                    captures = emptyList(),
                    orders = emptyList(),
                    userId = WaiterFixtures.ANA,
                )
                .single() as TableCard.AccountCard
        assertFalse(card.showsKitchen)
        assertFalse(card.mine)
        assertEquals(null, card.delivery)
    }

    @Test
    fun lasCuentasDeUnaUnionSonLasDeTodasSusMesas() {
        val other6 = "99999999-9999-4999-8999-999999999999"
        val first = account(WaiterFixtures.ACCOUNT, 27, WaiterFixtures.ANA)
        val second =
            account("88888888-8888-4888-8888-888888888888", 28, other).copy(tableId = other6)
        val elsewhere =
            account("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 29, other).copy(tableId = "otra")
        val cards =
            TableCards.of(
                tableIds = setOf(WaiterFixtures.TABLE, other6),
                accounts = listOf(first, second, elsewhere),
                captures = emptyList(),
                orders = emptyList(),
                userId = WaiterFixtures.ANA,
            )
        assertEquals(
            listOf(first.id, second.id),
            cards.filterIsInstance<TableCard.AccountCard>().map { it.account.id },
        )
    }

    @Test
    fun unRenglonSinPreparacionNoLlevaEtiquetaPeroElCanceladoSi() {
        val line =
            SentLine(
                versionId = "v1",
                itemId = "i1",
                state = "active",
                quantity = 1,
                preparationState = PreparationState.READY,
                request = WaiterFixtures.line(WaiterFixtures.SHAKE),
                name = "Coca Cola",
                unitPrice = Money("30.00"),
                total = Money("30.00"),
                preparationRequired = false,
            )
        assertFalse(line.showsPreparation)
        assertTrue(line.copy(preparationRequired = true).showsPreparation)
        assertTrue(line.copy(state = SentLine.STATE_CANCELLED).showsPreparation)
    }
}
