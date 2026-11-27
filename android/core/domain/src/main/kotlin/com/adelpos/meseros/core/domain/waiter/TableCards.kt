package com.adelpos.meseros.core.domain.waiter

/**
 * Etapa M1 (§5.5, M1-D16): las tarjetas del panel de una mesa (o de una unión de mesas, §1) salen
 * de esta regla pura, sin Compose, que se prueba en la JVM.
 * - **Una sola tarjeta por cuenta abierta de Go**, con lo de este mesero en ella: sus pedidos de la
 *   cola que van a esa cuenta (también los ya recibidos) y su captura en curso. Antes el pedido
 *   recibido se veía dos veces: en «Lo tuyo en esta mesa» y en «Cuentas abiertas».
 * - «Tu cuenta» ([TableCard.AccountCard.mine]) cuando Go dice que el responsable de la cuenta
 *   (`assigned_user_id`) es el mesero activo; nunca se deduce de quién capturó.
 * - Lo que todavía no tiene cuenta abierta (un pedido de la cola sin cuenta, o cuya cuenta ya no
 *   está abierta, y una captura de una cuenta nueva) es **su propia tarjeta**, con «Por enviar»,
 *   para que no se pierda de vista. Va antes que las cuentas: es trabajo del mesero que falta
 *   enviar.
 */
sealed interface TableCard {
    /** Clave estable de la tarjeta en la lista. */
    val key: String

    /**
     * Una cuenta abierta de Go con lo de este mesero en ella.
     *
     * @param orders sus pedidos de la cola que van a esta cuenta (o que entraron en ella), del más
     *   nuevo al más viejo.
     * @param capture su captura en curso en esta cuenta, si tiene algo.
     */
    data class AccountCard(
        val account: AccountSummary,
        val mine: Boolean,
        val orders: List<PendingOrder>,
        val capture: Capture?,
    ) : TableCard {
        override val key: String
            get() = "cuenta-${account.id}"

        /**
         * Lo que se dice del envío de lo de este mesero en la cuenta, lo más urgente primero: un
         * pedido que requiere su revisión, uno que sale, uno por enviar, una captura con productos
         * sin enviar y, si ya no queda nada de eso, «Recibido por el restaurante». Null si no hay
         * nada suyo en la cuenta.
         */
        val delivery: CardDelivery?
            get() {
                val unsent = orders.filter { it.state.unsent }
                val order =
                    unsent.firstOrNull { it.state == OrderState.NEEDS_REVIEW }
                        ?: unsent.firstOrNull { it.state == OrderState.SENDING }
                        ?: unsent.firstOrNull()
                if (order != null) {
                    return CardDelivery.Order(order)
                }
                val working = capture
                if (working != null && (working.inProgress || working.pendingOperationId != null)) {
                    return CardDelivery.CaptureWork(working)
                }
                val received = orders.firstOrNull { it.state == OrderState.RECEIVED }
                return received?.let { CardDelivery.Received(it) }
            }

        /**
         * La etiqueta de cocina de la cuenta (§5.2, M1-D15): sin ella si Go dice que nada de la
         * cuenta necesita preparación (`preparation_required: false`).
         */
        val showsKitchen: Boolean
            get() = account.preparationRequired
    }

    /** Un pedido de la cola que todavía no tiene una cuenta abierta (por enviar, enviándose…). */
    data class OrderCard(val order: PendingOrder) : TableCard {
        override val key: String
            get() = "pedido-${order.operationId}"
    }

    /** Una captura con productos de una cuenta nueva (o de una cuenta que ya no está abierta). */
    data class CaptureCard(val capture: Capture) : TableCard {
        override val key: String
            get() = "captura-${capture.id}"
    }
}

/** Qué se dice del envío en la tarjeta de una cuenta ([TableCard.AccountCard.delivery]). */
sealed interface CardDelivery {
    /** Un pedido de la cola sin recibir todavía: se abre su detalle (o su revisión) al tocarlo. */
    data class Order(val order: PendingOrder) : CardDelivery

    /** Una captura con productos sin enviar a cocina («Por enviar»): se abre al tocarla. */
    data class CaptureWork(val capture: Capture) : CardDelivery

    /** El último pedido ya llegó: «Recibido por el restaurante». */
    data class Received(val order: PendingOrder) : CardDelivery
}

object TableCards {
    /** Las tarjetas de una sola mesa. */
    fun of(
        tableId: String,
        accounts: List<AccountSummary>,
        captures: List<Capture>,
        orders: List<PendingOrder>,
        userId: String?,
    ): List<TableCard> = of(setOf(tableId), accounts, captures, orders, userId)

    /**
     * Las tarjetas de un lugar: una mesa o las mesas de una unión ([tableIds]). [accounts] son las
     * cuentas abiertas de Go (se toman las de esas mesas, en el orden de Go); [captures] y
     * [orders], las del mesero activo [userId] (las de otro mesero nunca llegan aquí).
     */
    fun of(
        tableIds: Set<String>,
        accounts: List<AccountSummary>,
        captures: List<Capture>,
        orders: List<PendingOrder>,
        userId: String?,
    ): List<TableCard> {
        val here = accounts.filter { it.isOpen && it.tableId in tableIds }
        val open = here.map { it.id }.toSet()
        val myOrders = orders.filter { it.destination.tableId in tableIds }
        val myCaptures = captures.filter { it.tableId in tableIds }
        val accountCards = here.map { account ->
            TableCard.AccountCard(
                account = account,
                mine = userId != null && account.assignedUserId == userId,
                orders =
                    myOrders
                        .filter { accountOf(it) == account.id }
                        .sortedByDescending { it.capturedAtMillis },
                capture = myCaptures.firstOrNull { it.accountId == account.id },
            )
        }
        // Sin cuenta abierta: lo que falta enviar (un pedido recibido siempre tiene su cuenta; si
        // ya no está abierta, no queda nada que hacer con él y no se muestra).
        val orderCards =
            myOrders
                .filter { it.state.unsent && accountOf(it) !in open }
                .sortedBy { it.capturedAtMillis }
                .map { TableCard.OrderCard(it) }
        val captureCards =
            myCaptures
                .filter { capture -> capture.accountId !in open && capture.hasWork() }
                .sortedBy { it.createdAtMillis }
                .map { TableCard.CaptureCard(it) }
        return captureCards + orderCards + accountCards
    }

    /** La cuenta de un pedido: en la que entró o, si no ha entrado, a la que va. */
    private fun accountOf(order: PendingOrder): String? =
        order.receivedAccountId ?: order.destination.accountId

    /**
     * Una captura que vale una tarjeta: tiene productos sin enviar, un guardado sin respuesta o un
     * descarte pendiente. Una captura que se abrió y quedó vacía no es trabajo.
     */
    private fun Capture.hasWork(): Boolean =
        inProgress || pendingOperationId != null || sync == DraftSync.DISCARDING
}
