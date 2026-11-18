package com.adelpos.meseros.core.domain.waiter

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow

/**
 * Un pedido de la lista de Para llevar (§3.3): lo que dice Go y, encima, lo que este dispositivo
 * tiene pendiente del mesero activo (la cola, la captura en curso y las ediciones del cliente).
 */
sealed interface TakeawayEntry {
    val key: String

    /** El nombre y el teléfono que se muestran: la última edición del mesero o lo de Go. */
    val customer: TakeawayCustomer

    /** Hay una edición del nombre o del teléfono que todavía no está en Go. */
    val customerPending: Boolean

    /**
     * Un pedido para llevar abierto en Go.
     *
     * @param captureId la captura del mesero en este pedido, si tiene algo sin enviar a cocina.
     * @param queued el pedido de la cola del mesero que va a este pedido y todavía no se recibe.
     */
    data class InGo(
        val account: AccountSummary,
        override val customer: TakeawayCustomer,
        override val customerPending: Boolean,
        val captureId: String?,
        val queued: PendingOrder?,
    ) : TakeawayEntry {
        override val key: String
            get() = account.id

        val preparation: TakeawayPreparation
            get() = TakeawayStates.preparation(account.preparationState)

        val payment: TakeawayPayment
            get() = TakeawayStates.payment(account.paymentState)

        val delivery: TakeawayDelivery
            get() = TakeawayStates.delivery(account.deliveryState)
    }

    /**
     * Un pedido de este dispositivo que todavía está en la cola y no nace en Go («Por enviar»,
     * «Enviando» o «Requiere tu revisión»), o que va a un pedido que ya no está en la lista.
     */
    data class Queued(
        val order: PendingOrder,
        override val customer: TakeawayCustomer,
        override val customerPending: Boolean,
    ) : TakeawayEntry {
        override val key: String
            get() = order.operationId
    }

    /** Una captura nueva con productos sin enviar a cocina, que todavía no nace en Go. */
    data class Draft(val capture: Capture, override val customer: TakeawayCustomer) :
        TakeawayEntry {
        override val key: String
            get() = capture.id

        override val customerPending: Boolean
            get() = false
    }
}

/** Arma la lista de Para llevar a partir de lo de Go y lo del dispositivo (lógica pura). */
object TakeawayBoard {
    /**
     * @param accounts las cuentas abiertas leídas de Go (se toman solo las para llevar, en el orden
     *   de Go: del más antiguo al más reciente).
     * @param orders y [captures]: los del mesero activo (de los demás meseros nunca se muestra el
     *   detalle, revisión R6, B3).
     * @return primero lo que todavía no nace en Go (lo que falta enviar), después los de Go.
     */
    fun entries(
        accounts: List<AccountSummary>,
        orders: List<PendingOrder>,
        captures: List<Capture>,
        edits: List<CustomerEdit>,
    ): List<TakeawayEntry> {
        val takeaway = accounts.filter { it.mode == OrderMode.TAKEAWAY && it.isOpen }
        val inGoIds = takeaway.map(AccountSummary::id).toSet()
        val unsent = orders.filter { it.destination.takeaway && it.state.unsent }
        val local = buildList {
            for (order in unsent.sortedBy(PendingOrder::capturedAtMillis)) {
                val accountId = order.destination.accountId
                if (accountId != null && accountId in inGoIds) {
                    continue
                }
                val edit = editFor(edits, key = order.operationId, accountId = accountId)
                add(
                    TakeawayEntry.Queued(
                        order = order,
                        customer = edit?.customer ?: order.destination.customer,
                        customerPending = edit != null,
                    ),
                )
            }
            // La captura y su pedido de la cola tienen el mismo identificador (A1-D19) y «Enviar a
            // cocina» (o «Corregir en la captura») pasa de uno al otro en una transacción, pero las
            // dos lecturas de Room llegan por separado: por un momento se ven las dos. Manda el
            // pedido de la cola; dos renglones con la misma clave tumbarían la lista (visto en
            // Android 8 en la Fase C).
            val orderIds = orders.map(PendingOrder::operationId).toSet()
            val drafts = captures.filter { capture ->
                capture.takeaway &&
                    capture.accountId == null &&
                    capture.inProgress &&
                    capture.sync != DraftSync.DISCARDING &&
                    capture.id !in orderIds
            }
            for (capture in drafts.sortedBy(Capture::createdAtMillis)) {
                val edit = editFor(edits, key = capture.id, accountId = null)
                add(TakeawayEntry.Draft(capture, edit?.customer ?: capture.customer))
            }
        }
        val inGo = takeaway.map { account ->
            val edit = editFor(edits, key = account.id, accountId = account.id)
            val capture = captures.firstOrNull { capture ->
                capture.accountId == account.id &&
                    capture.inProgress &&
                    capture.sync != DraftSync.DISCARDING
            }
            TakeawayEntry.InGo(
                account = account,
                customer = edit?.customer ?: account.customer,
                customerPending = edit != null,
                captureId = capture?.id,
                queued =
                    unsent
                        .filter { it.destination.accountId == account.id }
                        .maxByOrNull(PendingOrder::capturedAtMillis),
            )
        }
        return local + inGo
    }

    /** La edición pendiente de un pedido: por su clave o, si ya se conoce, por su cuenta. */
    fun editFor(edits: List<CustomerEdit>, key: String, accountId: String?): CustomerEdit? =
        edits.firstOrNull { edit ->
            edit.key == key || (accountId != null && edit.accountId == accountId)
        }
}

/** Cuándo se leyeron de Go los pedidos para llevar (para «lo último que se vio, con su hora»). */
data class TakeawayRead(
    /** Hora del dispositivo de la lectura (para mostrarla). */
    val wallMillis: Long,
    /** El reloj monótono de este arranque en esa lectura (para contar el tiempo de cada pedido). */
    val elapsedMillis: Long,
)

/**
 * Para llevar del mesero activo (§3.3 y A2-D2) para `feature:takeaway` y la captura compartida. Con
 * conexión se lee de Go (`restaurant/state`) y el único canal de tiempo real (ámbito `takeaway`) lo
 * mantiene al momento; sin conexión se ve lo último que se leyó, con su hora.
 */
interface TakeawayService {
    /** Las cuentas abiertas de la última lectura de Go (con los pedidos para llevar). */
    val accounts: StateFlow<List<AccountSummary>>

    /** Cuándo fue esa lectura; null si nunca se leyó en este dispositivo. */
    val readAt: StateFlow<TakeawayRead?>

    /** Vuelve a leer de Go (al abrir la lista o con «Actualizar»). */
    fun refresh()

    /** Las ediciones del nombre y el teléfono que todavía no están en Go. */
    fun customerEdits(): Flow<List<CustomerEdit>>

    /**
     * Cambia el nombre y el teléfono de un pedido de la lista (A2-D1), a nombre del mesero
     * [userId]: se guarda en el dispositivo y se envía en cuanto se pueda (la última edición gana).
     */
    suspend fun editCustomer(userId: String, target: CustomerTarget, customer: TakeawayCustomer)

    /** «Entregado» (A2-D2), con conexión y a nombre del mesero [userId]; nunca va a la cola. */
    suspend fun deliver(userId: String, accountId: String): DeliveryResult
}
