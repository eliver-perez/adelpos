package local.adel.pos.android.core.domain.waiter

/**
 * Lo que puede hacer el mesero con un pedido en «Requiere tu revisión» (§6.4). Ninguna regla de
 * negocio: el pedido se vuelve a enviar **solo después** de que el mesero acepta los cambios (con
 * la misma clave de operación) y Go vuelve a revisarlo todo al recibirlo.
 */
object OrderReviewRules {
    /**
     * «Aceptar los cambios y enviar»: solo con cambios aceptables (precio, mesa con otra cuenta,
     * renglones del borrador). La confirmación lleva la versión del catálogo del rechazo y los
     * cambios tal como los dio Go.
     */
    fun accept(order: PendingOrder, nowMillis: Long): PendingOrder {
        val review = requireNotNull(order.review) { "El pedido no tiene revisión pendiente" }
        check(order.state == OrderState.NEEDS_REVIEW) { "El pedido no requiere revisión" }
        check(review.canAccept) { "Hay cambios que se deben corregir antes de enviar" }
        val catalogVersion = requireNotNull(review.catalogVersion)
        return requeued(
            order = order,
            confirmation = OrderConfirmation(catalogVersion, review.changes),
            nowMillis = nowMillis,
        )
    }

    /**
     * «Quitar» una partida (agotada, ya no disponible…). Los demás cambios siguen a la vista. El
     * renglón sigue en [PendingOrder.replaces] si ya estaba guardado en Go: al aceptar el pedido,
     * Go lo quita del borrador (es lo que pidió el mesero). Sin partidas no queda nada que enviar y
     * el resultado es null.
     */
    fun removeLine(order: PendingOrder, itemId: String, nowMillis: Long): PendingOrder? {
        check(order.state == OrderState.NEEDS_REVIEW) { "El pedido no requiere revisión" }
        val lines = order.lines.filter { it.item.id != itemId }
        if (lines.isEmpty()) {
            return null
        }
        val review =
            order.review?.let { current ->
                current.copy(changes = current.changes.filter { it.itemId != itemId })
            }
        return order.copy(lines = lines, review = review, updatedAtMillis = nowMillis)
    }

    /**
     * «Enviar» después de corregir: si ya no queda ningún cambio, sin confirmación; si los que
     * quedan son aceptables, con su confirmación (el mesero los vio en la misma pantalla).
     */
    fun resend(order: PendingOrder, nowMillis: Long): PendingOrder {
        check(order.state == OrderState.NEEDS_REVIEW) { "El pedido no requiere revisión" }
        val review = order.review
        if (review == null || review.changes.isEmpty()) {
            return requeued(order, confirmation = null, nowMillis = nowMillis)
        }
        return accept(order, nowMillis)
    }

    /**
     * «Abrir una cuenta nueva en la mesa» (la cuenta ya se cobró, se canceló o se combinó): el
     * pedido va a la mesa y ya no sustituye renglones del borrador (Go solo lo admite con una
     * cuenta). Go vuelve a revisarlo todo y avisa si la mesa ya tiene otra cuenta.
     */
    fun moveToNewAccount(order: PendingOrder, nowMillis: Long): PendingOrder {
        check(order.state == OrderState.NEEDS_REVIEW) { "El pedido no requiere revisión" }
        val destination =
            order.destination.copy(
                accountId = null,
                accountName = null,
                seenAccountIds = emptyList(),
            )
        return requeued(
                order = order.copy(destination = destination, replaces = emptyList()),
                confirmation = null,
                nowMillis = nowMillis,
            )
            .copy(onlineConfirm = null)
    }

    /** Si la revisión ofrece abrir una cuenta nueva en la mesa (la cuenta ya no está abierta). */
    fun offersNewAccount(review: OrderReview): Boolean =
        review.reason == ReviewReason.ACCOUNT_NOT_OPEN ||
            review.changes.any { it.kind == ChangeKind.ACCOUNT_CLOSED }

    /** Partidas que el mesero puede quitar: las que Go señaló con un cambio que exige corregir. */
    fun removableItemIds(order: PendingOrder): Set<String> {
        val review = order.review ?: return emptySet()
        val lineIds = order.lines.map { it.item.id }.toSet()
        return review.blocking.mapNotNull { it.itemId }.filter { it in lineIds }.toSet()
    }

    private fun requeued(
        order: PendingOrder,
        confirmation: OrderConfirmation?,
        nowMillis: Long,
    ): PendingOrder {
        OrderTransitions.require(order.state, OrderState.QUEUED)
        return order.copy(
            state = OrderState.QUEUED,
            confirmation = confirmation,
            review = null,
            lastError = null,
            updatedAtMillis = nowMillis,
        )
    }
}
