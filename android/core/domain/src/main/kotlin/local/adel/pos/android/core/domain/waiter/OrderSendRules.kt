package local.adel.pos.android.core.domain.waiter

/**
 * Lo que respondió Go (o la red) a un envío, ya interpretado por la capa de datos
 * (`submitOfflineOrder` o `confirmDraft`).
 */
sealed interface SendOutcome {
    /** Go lo aceptó (200): el pedido entró en [accountId]. */
    data class Accepted(val accountId: String) : SendOutcome

    /**
     * La misma clave ya había entrado con otro contenido (`OFFLINE_ORDER_ALREADY_ACCEPTED`): nunca
     * se duplica y el pedido se marca recibido con el resultado de Go.
     */
    data class AlreadyAccepted(val accountId: String) : SendOutcome

    /** `ORDER_REQUIRES_CONFIRMATION` con los cambios. */
    data class RequiresConfirmation(val review: OrderReview) : SendOutcome

    /** La cuenta ya no está abierta (en línea: `details.reason: account_not_open`). */
    data class AccountNotOpen(val message: String) : SendOutcome

    /**
     * La confirmación en línea no se pudo hacer, pero el pedido sigue como pedido sin conexión (Go
     * lo revisa todo al recibirlo): cambió el cálculo, otra terminal cambió la cuenta, un producto
     * se agotó o no hubo respuesta.
     */
    data class ContinueOffline(val note: String?) : SendOutcome

    /**
     * Go no reconoció la credencial de envío del mesero (`OFFLINE_CREDENTIAL_INVALID`): el pedido
     * se conserva hasta que entre con su PIN con conexión.
     */
    data object CredentialRejected : SendOutcome

    /** Sin respuesta, servidor ocupado o con falla, versión por actualizar…: se reintenta igual. */
    data class TryAgain(val note: String?) : SendOutcome

    /** Go lo rechazó por otro motivo que el mesero debe ver (permisos, datos). */
    data class Rejected(val message: String) : SendOutcome
}

/** Estado siguiente de un pedido después de un envío (las transiciones de [OrderTransitions]). */
object OrderSendRules {
    /** El trabajador toma el pedido: «Enviando». */
    fun startSending(order: PendingOrder, nowMillis: Long): PendingOrder {
        OrderTransitions.require(order.state, OrderState.SENDING)
        return order.copy(state = OrderState.SENDING, updatedAtMillis = nowMillis)
    }

    fun after(order: PendingOrder, outcome: SendOutcome, nowMillis: Long): PendingOrder {
        check(order.state == OrderState.SENDING) { "Solo se interpreta un envío en curso" }
        return when (outcome) {
            is SendOutcome.Accepted -> received(order, outcome.accountId, nowMillis)
            is SendOutcome.AlreadyAccepted -> received(order, outcome.accountId, nowMillis)
            is SendOutcome.RequiresConfirmation -> needsReview(order, outcome.review, nowMillis)
            is SendOutcome.AccountNotOpen ->
                needsReview(
                    order = order,
                    review =
                        OrderReview(
                            reason = ReviewReason.ACCOUNT_NOT_OPEN,
                            message = outcome.message,
                            changes = emptyList(),
                            catalogVersion = null,
                        ),
                    nowMillis = nowMillis,
                )
            is SendOutcome.Rejected ->
                needsReview(
                    order = order,
                    review =
                        OrderReview(
                            reason = ReviewReason.REJECTED,
                            message = outcome.message,
                            changes = emptyList(),
                            catalogVersion = null,
                        ),
                    nowMillis = nowMillis,
                )
            is SendOutcome.ContinueOffline ->
                order.copy(
                    state = OrderState.QUEUED,
                    onlineConfirm = null,
                    lastError = outcome.note,
                    updatedAtMillis = nowMillis,
                )
            SendOutcome.CredentialRejected ->
                order.copy(
                    state = OrderState.QUEUED,
                    credentialRejected = true,
                    attempts = order.attempts + 1,
                    updatedAtMillis = nowMillis,
                )
            is SendOutcome.TryAgain ->
                order.copy(
                    state = OrderState.QUEUED,
                    attempts = order.attempts + 1,
                    lastError = outcome.note,
                    updatedAtMillis = nowMillis,
                )
        }
    }

    /**
     * Al arrancar el proceso, un pedido que quedó «Enviando» (se cerró la aplicación a media
     * petición) vuelve a «Por enviar»: el reintento usa la misma clave y Go nunca lo duplica. Un
     * intento de confirmación en línea interrumpido sigue como pedido sin conexión (el trabajador
     * no tiene la sesión del mesero, solo su credencial de envío).
     */
    fun recoverInterrupted(order: PendingOrder, nowMillis: Long): PendingOrder {
        if (order.state != OrderState.SENDING) {
            return order
        }
        return order.copy(
            state = OrderState.QUEUED,
            onlineConfirm = null,
            updatedAtMillis = nowMillis,
        )
    }

    private fun received(order: PendingOrder, accountId: String, nowMillis: Long): PendingOrder =
        order.copy(
            state = OrderState.RECEIVED,
            receivedAccountId = accountId,
            receivedAtMillis = nowMillis,
            review = null,
            lastError = null,
            credentialRejected = false,
            onlineConfirm = null,
            updatedAtMillis = nowMillis,
        )

    private fun needsReview(order: PendingOrder, review: OrderReview, nowMillis: Long) =
        order.copy(
            state = OrderState.NEEDS_REVIEW,
            review = review,
            confirmation = null,
            onlineConfirm = null,
            attempts = order.attempts + 1,
            updatedAtMillis = nowMillis,
        )
}
