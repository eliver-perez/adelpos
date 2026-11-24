package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.BURGER
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.capture
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.draft
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa L1 (§8): un pedido de la cola que la licencia, un cupo o la hora no permitieron no se
 * pierde ni se reintenta solo: queda en «Requiere tu revisión» con su motivo y el mesero lo envía
 * de nuevo con la misma clave.
 */
class LicenseQueueRulesTest {
    private val sending =
        WaiterFixtures.order(
            capture(
                operations = listOf(DraftOperation.Add(draft("hamburguesa", BURGER))),
                own = setOf("hamburguesa"),
            ),
            state = OrderState.SENDING,
        )

    @Test
    fun quedaEnRevisionConSuMotivoYSinReintentarseSolo() {
        val message = LicenseMessages.CLOCK_INVALID
        val next = OrderSendRules.after(sending, SendOutcome.NotAllowed(message), 5_000)
        assertEquals(OrderState.NEEDS_REVIEW, next.state)
        assertEquals(ReviewReason.NOT_ALLOWED, next.review!!.reason)
        assertEquals(message, next.review!!.message)
        assertTrue(next.review!!.changes.isEmpty())
        assertEquals(sending.operationId, next.operationId)
        assertEquals(sending.lines, next.lines)
        assertEquals(sending.attempts + 1, next.attempts)
        // Sigue contando como sin enviar (indicador, cambio de mesero y bitácora del retiro).
        assertTrue(next.state.unsent)
        // La cola solo envía «Por enviar»: desde «Requiere tu revisión» solo sale con el mesero.
        assertFalse(OrderTransitions.isAllowed(OrderState.NEEDS_REVIEW, OrderState.SENDING))
    }

    @Test
    fun elMeseroLoEnviaDeNuevoConLaMismaClave() {
        val review = OrderSendRules.after(sending, SendOutcome.NotAllowed("Sin cupo"), 5_000)
        assertTrue(OrderReviewRules.offersResendAsIs(review.review!!))
        assertFalse(review.review!!.canAccept)
        assertFalse(OrderReviewRules.offersNewAccount(review.review!!))
        val again = OrderReviewRules.resend(review, 6_000)
        assertEquals(OrderState.QUEUED, again.state)
        assertEquals(sending.operationId, again.operationId)
        assertEquals(sending.lines, again.lines)
        assertEquals(sending.destination, again.destination)
        assertNull(again.review)
        assertNull(again.confirmation)
    }

    @Test
    fun losDemasRechazosNoOfrecenEnviarDeNuevo() {
        val rejected = OrderSendRules.after(sending, SendOutcome.Rejected("Sin permiso"), 5_000)
        assertEquals(ReviewReason.REJECTED, rejected.review!!.reason)
        assertFalse(OrderReviewRules.offersResendAsIs(rejected.review!!))
        val closed =
            OrderSendRules.after(sending, SendOutcome.AccountNotOpen("La cuenta se cobró"), 5_000)
        assertFalse(OrderReviewRules.offersResendAsIs(closed.review!!))
    }
}
