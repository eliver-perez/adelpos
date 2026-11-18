package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.ACCOUNT
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.BURGER
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.SHAKE
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.capture
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.change
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.draft
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.order
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/** Reglas del dominio de la revisión R6 en el dispositivo (A1-D19). */
class R6RulesTest {
    @Test
    fun lasPartidasYaEnUnaCuentaDeLaMesaVanAEsaCuentaConLaMismaClave() {
        val sending =
            order(
                capture(
                    operations = listOf(DraftOperation.Add(draft("r1", BURGER))),
                    own = setOf("r1"),
                    accountId = null,
                ),
                state = OrderState.SENDING,
            )
        val next = OrderSendRules.after(sending, SendOutcome.Redirect("cuenta-b"), 3_000)
        assertEquals(OrderState.QUEUED, next.state)
        assertEquals("cuenta-b", next.destination.accountId)
        assertTrue(next.destination.seenAccountIds.isEmpty())
        assertEquals(sending.operationId, next.operationId)
        assertEquals(sending.lines, next.lines)
    }

    @Test
    fun yaAceptadoSinCuentaNoInventaUna() {
        val sending =
            order(
                capture(
                    operations = listOf(DraftOperation.Add(draft("r1", BURGER))),
                    own = setOf("r1"),
                    accountId = null,
                ),
                state = OrderState.SENDING,
            )
        val next = OrderSendRules.after(sending, SendOutcome.AlreadyAccepted(null), 3_000)
        assertEquals(OrderState.RECEIVED, next.state)
        // Revisión R6, O3: null, nunca una cadena vacía.
        assertNull(next.receivedAccountId)
    }

    @Test
    fun alResolverElGuardadoSinRespuestaElPedidoVaALaCuentaQueAbrio() {
        val removedAfter = draft("quitado", SHAKE)
        val queued =
            order(
                    capture(
                        operations = listOf(DraftOperation.Add(draft("r1", BURGER))),
                        own = setOf("r1"),
                        accountId = null,
                    ),
                )
                .copy(
                    unresolvedSave =
                        UnresolvedSave(
                            operationId = "apertura",
                            accountId = null,
                            tableId = "mesa",
                            seenAccountIds = emptyList(),
                            revision = 0,
                            items = listOf(draft("r1", BURGER), removedAfter),
                        ),
                )
        val applied =
            OrderSendRules.resolved(
                queued,
                SaveResolution.Applied(
                    "cuenta-nueva",
                    "Mesa 4",
                    listOf(draft("r1", BURGER), removedAfter),
                ),
                4_000,
            )
        assertNull(applied.unresolvedSave)
        assertEquals("cuenta-nueva", applied.destination.accountId)
        // Lo que ese guardado dejó y el mesero quitó después: el pedido lo sustituye.
        assertEquals(listOf(removedAfter), applied.replaces)
        val rejected = OrderSendRules.resolved(queued, SaveResolution.NotApplied, 4_000)
        assertNull(rejected.unresolvedSave)
        assertNull(rejected.destination.accountId)
        assertTrue(rejected.replaces.isEmpty())
    }

    @Test
    fun todoYaEnviadoNoInvitaAReenviar() {
        val pending =
            order(
                    capture(
                        operations = listOf(DraftOperation.Add(draft("r1", BURGER))),
                        own = setOf("r1"),
                    ),
                    state = OrderState.NEEDS_REVIEW,
                )
                .copy(
                    review =
                        OrderReview(
                            ReviewReason.CHANGES,
                            "Mensaje de Go",
                            listOf(change(ChangeKind.DRAFT_ITEM_SENT, "r1", acceptable = true)),
                            41,
                        ),
                )
        assertEquals(setOf("r1"), pending.review!!.alreadySentItemIds)
        assertTrue(OrderReviewRules.everythingAlreadySent(pending))
        val withMore =
            pending.copy(
                lines = pending.lines + pending.lines.single().copy(item = draft("r2", SHAKE)),
            )
        assertFalse(OrderReviewRules.everythingAlreadySent(withMore))
    }

    @Test
    fun cuentaCombinadaYMandarAOtraCuenta() {
        val review =
            OrderReview(
                ReviewReason.CHANGES,
                "La cuenta se combinó con otra cuenta.",
                listOf(change(ChangeKind.ACCOUNT_CLOSED, null, acceptable = false, now = "merged")),
                41,
            )
        assertTrue(review.accountMerged)
        val pending =
            order(
                    capture(
                        base = listOf(draft("guardado", BURGER)),
                        own = setOf("guardado"),
                    ),
                    state = OrderState.NEEDS_REVIEW,
                )
                .copy(review = review)
        val sent = OrderReviewRules.sendToAccount(pending, "cuenta-b", 5_000)
        assertEquals(OrderState.QUEUED, sent.state)
        assertEquals("cuenta-b", sent.destination.accountId)
        // Los renglones que sustituía viajan igual: la combinación los movió con su id.
        assertEquals(pending.replaces, sent.replaces)
        assertEquals(pending.operationId, sent.operationId)
        assertNull(sent.review)
    }

    // Regla del coordinador (revisión R6, M3): con una cuenta combinada nunca se ofrece ni se hace
    // «Abrir una cuenta nueva»; con una cobrada o cancelada, sí.
    @Test
    fun unaCuentaCombinadaNoOfreceUnaCuentaNueva() {
        val merged =
            OrderReview(
                ReviewReason.CHANGES,
                "La cuenta se combinó con otra cuenta.",
                listOf(change(ChangeKind.ACCOUNT_CLOSED, null, acceptable = false, now = "merged")),
                41,
            )
        assertFalse(OrderReviewRules.offersNewAccount(merged))
        val mergedOnline =
            OrderReview(
                ReviewReason.ACCOUNT_NOT_OPEN,
                "La cuenta se combinó con otra cuenta.",
                emptyList(),
                null,
                mergedIntoId = "cuenta-b",
            )
        assertTrue(mergedOnline.accountMerged)
        assertFalse(OrderReviewRules.offersNewAccount(mergedOnline))
        val paid =
            OrderReview(
                ReviewReason.CHANGES,
                "La cuenta ya se cobró.",
                listOf(change(ChangeKind.ACCOUNT_CLOSED, null, acceptable = false, now = "closed")),
                41,
            )
        assertTrue(OrderReviewRules.offersNewAccount(paid))
        val pending =
            order(
                    capture(
                        base = listOf(draft("guardado", BURGER)),
                        own = setOf("guardado"),
                    ),
                    state = OrderState.NEEDS_REVIEW,
                )
                .copy(review = merged)
        assertThrows(IllegalStateException::class.java) {
            OrderReviewRules.moveToNewAccount(pending, 5_000) { "nuevo" }
        }
        // La captura: cerrada por combinación (con la cuenta destino) frente a cobrada.
        val closedByMerge =
            capture()
                .copy(
                    sync = DraftSync.ACCOUNT_CLOSED,
                    tableAccounts = listOf(TableAccountRef("cuenta-b", 0, "")),
                )
        assertTrue(closedByMerge.accountMerged)
        assertFalse(closedByMerge.copy(tableAccounts = emptyList()).accountMerged)
    }

    @Test
    fun capturaEnDescarteSigueContandoYDiceSuGuardadoSinRespuesta() {
        val discarding = capture().copy(sync = DraftSync.DISCARDING)
        assertTrue(discarding.inProgress)
        val uncertain =
            capture(
                    base = listOf(draft("viejo", BURGER)),
                    operations = listOf(DraftOperation.Add(draft("r1", SHAKE))),
                    own = setOf("r1"),
                )
                .copy(pendingOperationId = "guardado", inFlightCount = 1)
        val save = uncertain.unresolvedSave()!!
        assertEquals("guardado", save.operationId)
        assertEquals(ACCOUNT, save.accountId)
        assertEquals(listOf("viejo", "r1"), save.items.map(DraftLine::id))
        assertNull(capture().unresolvedSave())
    }
}
