package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.BURGER
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.SHAKE
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.capture
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.change
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.draft
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class OrderQueueRulesTest {
    private val twoLines =
        capture(
            operations =
                listOf(
                    DraftOperation.Add(draft("hamburguesa", BURGER)),
                    DraftOperation.Add(draft("malteada", SHAKE)),
                ),
            own = setOf("hamburguesa", "malteada"),
        )

    @Test
    fun transicionesPermitidasYProhibidas() {
        val allowed =
            setOf(
                OrderState.DRAFT to OrderState.QUEUED,
                OrderState.DRAFT to OrderState.SENDING,
                OrderState.QUEUED to OrderState.SENDING,
                OrderState.SENDING to OrderState.QUEUED,
                OrderState.SENDING to OrderState.RECEIVED,
                OrderState.SENDING to OrderState.NEEDS_REVIEW,
                OrderState.NEEDS_REVIEW to OrderState.QUEUED,
                OrderState.NEEDS_REVIEW to OrderState.DRAFT,
            )
        for (from in OrderState.entries) {
            for (to in OrderState.entries) {
                assertEquals(
                    "$from → $to",
                    (from to to) in allowed,
                    OrderTransitions.isAllowed(from, to),
                )
            }
        }
        // Ejemplos de lo prohibido: nada sale de «Recibido», nunca se marca recibido sin
        // «Enviando» (la respuesta de Go) y la revisión no se envía sin que el mesero la acepte.
        assertThrows(IllegalStateException::class.java) {
            OrderTransitions.require(OrderState.QUEUED, OrderState.RECEIVED)
        }
        assertThrows(IllegalStateException::class.java) {
            OrderTransitions.require(OrderState.RECEIVED, OrderState.QUEUED)
        }
        assertThrows(IllegalStateException::class.java) {
            OrderTransitions.require(OrderState.NEEDS_REVIEW, OrderState.SENDING)
        }
    }

    @Test
    fun cincoEstadosDistintosYCualesFaltanPorEnviar() {
        assertEquals(5, OrderState.entries.map { it.storedName }.toSet().size)
        assertEquals(
            setOf(OrderState.QUEUED, OrderState.SENDING, OrderState.NEEDS_REVIEW),
            OrderState.entries.filter { it.unsent }.toSet(),
        )
        assertEquals(OrderState.NEEDS_REVIEW, OrderState.fromStoredName("needs_review"))
    }

    @Test
    fun recibidoSoloConLaRespuestaDeGo() {
        val queued = WaiterFixtures.order(twoLines)
        assertThrows(IllegalStateException::class.java) {
            OrderSendRules.after(queued, SendOutcome.Accepted("cuenta"), 3_000)
        }
        val sending = OrderSendRules.startSending(queued, 3_000)
        assertEquals(OrderState.SENDING, sending.state)
        val received = OrderSendRules.after(sending, SendOutcome.Accepted("cuenta"), 4_000)
        assertEquals(OrderState.RECEIVED, received.state)
        assertEquals("cuenta", received.receivedAccountId)
        // La clave ya había entrado con otro contenido: recibido con el resultado de Go, nunca otra
        // vez.
        val already = OrderSendRules.after(sending, SendOutcome.AlreadyAccepted("otra"), 4_000)
        assertEquals(OrderState.RECEIVED, already.state)
        assertEquals("otra", already.receivedAccountId)
        assertEquals(queued.operationId, already.operationId)
    }

    @Test
    fun sinRespuestaYCredencialRechazadaConservanElPedido() {
        val sending = OrderSendRules.startSending(WaiterFixtures.order(twoLines), 3_000)
        val again = OrderSendRules.after(sending, SendOutcome.TryAgain("sin red"), 4_000)
        assertEquals(OrderState.QUEUED, again.state)
        assertEquals(1, again.attempts)
        assertEquals(sending.operationId, again.operationId)
        val credential = OrderSendRules.after(sending, SendOutcome.CredentialRejected, 4_000)
        assertEquals(OrderState.QUEUED, credential.state)
        assertTrue(credential.credentialRejected)
        assertEquals(2, credential.lines.size)
    }

    @Test
    fun enLineaQueNoSePuedeConfirmarSigueSinConexion() {
        val online =
            WaiterFixtures.order(twoLines, state = OrderState.SENDING)
                .copy(onlineConfirm = OnlineConfirm("op-en-linea", "cuenta", 4, "huella"))
        val next = OrderSendRules.after(online, SendOutcome.ContinueOffline("cambió"), 5_000)
        assertEquals(OrderState.QUEUED, next.state)
        assertNull(next.onlineConfirm)
        // Al reiniciar el proceso a media petición también sigue como pedido sin conexión.
        val recovered = OrderSendRules.recoverInterrupted(online, 6_000)
        assertEquals(OrderState.QUEUED, recovered.state)
        assertNull(recovered.onlineConfirm)
        val queued = WaiterFixtures.order(twoLines)
        assertEquals(queued, OrderSendRules.recoverInterrupted(queued, 6_000))
    }

    @Test
    fun revisionConCambiosAceptablesSeEnviaSoloAlAceptar() {
        val price =
            change(
                ChangeKind.PRICE_CHANGED,
                "hamburguesa",
                acceptable = true,
                message = "Hamburguesa: $120.00 → $130.00",
                now = "130.00",
            )
        val review = OrderReview(ReviewReason.CHANGES, "Revisa", listOf(price), 42)
        val sending = OrderSendRules.startSending(WaiterFixtures.order(twoLines), 3_000)
        val pending = OrderSendRules.after(sending, SendOutcome.RequiresConfirmation(review), 4_000)
        assertEquals(OrderState.NEEDS_REVIEW, pending.state)
        assertTrue(pending.review!!.canAccept)
        val accepted = OrderReviewRules.accept(pending, 5_000)
        assertEquals(OrderState.QUEUED, accepted.state)
        assertEquals(OrderConfirmation(42, listOf(price)), accepted.confirmation)
        assertNull(accepted.review)
        assertEquals(pending.operationId, accepted.operationId)
    }

    @Test
    fun agotadoSeQuitaAntesDeAceptarLoDemas() {
        val soldOut =
            change(
                ChangeKind.PRODUCT_SOLD_OUT,
                "malteada",
                acceptable = false,
                message = "Malteada de fresa: agotado. Quítalo del pedido.",
            )
        val price = change(ChangeKind.PRICE_CHANGED, "hamburguesa", acceptable = true, now = "130")
        val review = OrderReview(ReviewReason.CHANGES, "Corrige", listOf(soldOut, price), 42)
        val pending =
            WaiterFixtures.order(twoLines, state = OrderState.NEEDS_REVIEW).copy(review = review)
        assertFalse(review.canAccept)
        assertThrows(IllegalStateException::class.java) { OrderReviewRules.accept(pending, 5_000) }
        assertEquals(setOf("malteada"), OrderReviewRules.removableItemIds(pending))
        val corrected = requireNotNull(OrderReviewRules.removeLine(pending, "malteada", 5_000))
        assertEquals(listOf("hamburguesa"), corrected.lines.map { it.item.id })
        assertEquals(listOf(price), corrected.review!!.changes)
        val resent = OrderReviewRules.resend(corrected, 6_000)
        assertEquals(OrderState.QUEUED, resent.state)
        assertEquals(listOf(price), resent.confirmation!!.changes)
        // Quitar la última partida deja el pedido sin nada que enviar.
        assertNull(OrderReviewRules.removeLine(corrected, "hamburguesa", 7_000))
    }

    @Test
    fun cuentaCobradaSeEnviaAUnaCuentaNuevaDeLaMesa() {
        val closed =
            change(
                ChangeKind.ACCOUNT_CLOSED,
                itemId = null,
                acceptable = false,
                message = "La cuenta «Mesa 4» ya se cobró.",
                now = "closed",
            )
        val review = OrderReview(ReviewReason.CHANGES, "Corrige", listOf(closed), 42)
        val pending =
            WaiterFixtures.order(
                    capture(base = listOf(draft("guardado", BURGER)), own = setOf("guardado")),
                    state = OrderState.NEEDS_REVIEW,
                )
                .copy(review = review)
        assertTrue(OrderReviewRules.offersNewAccount(review))
        assertEquals(1, pending.replaces.size)
        val moved = OrderReviewRules.moveToNewAccount(pending, 5_000) { "renglon-nuevo" }
        assertEquals(OrderState.QUEUED, moved.state)
        // Identificadores nuevos (revisión R6, M3): los anteriores siguen en la otra cuenta.
        assertEquals(listOf("renglon-nuevo"), moved.lines.map { it.item.id })
        assertNull(moved.destination.accountId)
        assertEquals(pending.destination.tableId, moved.destination.tableId)
        // Sin cuenta no hay borrador que sustituir (Go lo rechazaría).
        assertTrue(moved.replaces.isEmpty())
        assertNull(moved.confirmation)
        assertTrue(
            OrderReviewRules.offersNewAccount(
                OrderReview(ReviewReason.ACCOUNT_NOT_OPEN, "ya se cobró", emptyList(), null),
            ),
        )
    }

    @Test
    fun trabajoPendienteDelMesero() {
        val queued = WaiterFixtures.order(twoLines)
        val received = queued.copy(state = OrderState.RECEIVED, operationId = "op-2")
        val work = WaiterWorkSummary.of(captures = emptyList(), orders = listOf(queued, received))
        assertEquals(1, work.unsentOrders)
        assertFalse(work.captureInProgress)
        assertTrue(work.blocksSwitch)
        val capturing = WaiterWorkSummary.of(captures = listOf(twoLines), orders = emptyList())
        assertTrue(capturing.captureInProgress)
        assertTrue(capturing.blocksIdleLock)
        assertFalse(capturing.blocksSwitch)
        assertNotNull(capturing)
    }

    @Test
    fun motivosDesconocidosNoSeAceptanSolos() {
        assertEquals(ChangeKind.UNKNOWN, ChangeKind.fromWireName("algo_nuevo"))
        assertEquals(ChangeKind.UNKNOWN, ChangeKind.fromWireName(""))
        assertEquals(ChangeKind.DRAFT_ITEM_SENT, ChangeKind.fromWireName("draft_item_sent"))
    }
}
