package local.adel.pos.android.core.domain.waiter

import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.money.Money

/**
 * Arma el pedido de la cola cuando el mesero toca «Enviar a cocina» (§7): con su clave de operación
 * nueva, la versión y la época del catálogo, los precios que vio y, si la cuenta ya existía, los
 * renglones del borrador de Go que sustituye (A1-D6).
 */
object OrderFactory {
    /**
     * Lo que el catálogo guardado dice de un renglón para el pedido: el nombre que se muestra y el
     * precio de referencia que se vio (el de la presentación o el del producto).
     */
    fun interface CatalogLookup {
        fun describe(line: LineSelection): Described?
    }

    data class Described(val name: String, val referencePrice: Money?)

    /** Versión y época del catálogo guardado en el dispositivo al enviar. */
    data class CatalogStamp(val version: Long, val epoch: String?)

    /**
     * @param onlineConfirm si se va a intentar primero confirmar el borrador en línea (el pedido
     *   nace «Enviando»); si no, nace «Por enviar» para el trabajador.
     * @return null si no hay nada de este mesero que enviar.
     */
    fun fromCapture(
        capture: Capture,
        waiterName: String,
        operationId: String,
        catalog: CatalogStamp,
        lookup: CatalogLookup,
        onlineConfirm: OnlineConfirm?,
        nowMillis: Long,
    ): PendingOrder? {
        val plan = OfflineOrderPlan.of(capture)
        if (plan.items.isEmpty()) {
            return null
        }
        val lines =
            plan.items.map { item ->
                val described = lookup.describe(item.line)
                // Precio que vio el mesero: el de la cotización de Go si la hubo (en línea) o el de
                // referencia del catálogo. Nunca se calcula uno: si Go cobra otro, lo avisa como
                // cambio de precio y el mesero lo acepta antes de que el pedido entre.
                val seen =
                    capture.seenPrices[item.id] ?: described?.referencePrice ?: Money(NO_PRICE)
                OrderLine(
                    item = item,
                    productName = described?.name ?: UNKNOWN_PRODUCT,
                    seenUnitPrice = seen,
                )
            }
        val state =
            if (onlineConfirm != null) {
                OrderState.SENDING
            } else {
                OrderState.QUEUED
            }
        OrderTransitions.require(OrderState.DRAFT, state)
        return PendingOrder(
            operationId = operationId,
            waiterUserId = capture.waiterUserId,
            waiterName = waiterName,
            state = state,
            destination =
                OrderDestination(
                    tableId = capture.tableId,
                    tableName = capture.tableName,
                    accountId = capture.accountId,
                    accountName = capture.accountName,
                    seenAccountIds = capture.seenAccountIds,
                ),
            lines = lines,
            replaces = plan.replaces,
            catalogVersion = catalog.version,
            catalogEpoch = catalog.epoch,
            capturedAtMillis = nowMillis,
            confirmation = null,
            review = null,
            attempts = 0,
            lastError = null,
            credentialRejected = false,
            onlineConfirm = onlineConfirm,
            receivedAccountId = null,
            receivedAtMillis = null,
            updatedAtMillis = nowMillis,
        )
    }

    /** Sin precio conocido (no debería pasar: un producto sin precio no se puede elegir). */
    private const val NO_PRICE = "0.00"

    /** Nombre de un producto que ya no está en el catálogo guardado. */
    const val UNKNOWN_PRODUCT = "Producto"
}
