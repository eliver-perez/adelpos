package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money

/**
 * Estado de preparación de una cuenta o de un producto enviado (`preparation_state`), el mismo
 * modelo del escritorio y de la pantalla de cocina; lo calcula Go. Los textos están en la interfaz
 * (los mismos de `preparationLabels` del escritorio).
 */
enum class PreparationState(val wireName: String) {
    WITHOUT_ITEMS("without_items"),
    DRAFT("draft"),
    CONFIRMED_PENDING_DISPATCH("confirmed_pending_dispatch"),
    PENDING_RELEASE("pending_release"),
    SCHEDULED("scheduled"),
    RELEASED("released"),
    PREPARING("preparing"),
    READY("ready"),
    CHANGE_PENDING("change_pending"),
    CANCELLED("cancelled");

    companion object {
        /**
         * Estado por su nombre en la API; null si esta versión no lo conoce (no se inventa uno).
         */
        fun fromWireName(name: String?): PreparationState? = entries.firstOrNull {
            it.wireName == name
        }
    }
}

/** Importes de una cuenta o partida tal como los calculó Go (IVA incluido). */
data class Totals(val subtotal: Money, val discount: Money, val total: Money) {
    companion object {
        val ZERO = Totals(Money("0.00"), Money("0.00"), Money("0.00"))
    }
}

/**
 * Cuenta del restaurante (`Account` del contrato) para elegirla y mostrarla.
 *
 * @param revision revisión vigente: toda escritura la lleva y Go responde `ACCOUNT_CONFLICT` si
 *   otra terminal la cambió.
 * @param mergedIntoId la cuenta en que se combinó (solo si [state] es `merged`).
 * @param mode de salón o para llevar (etapa A2).
 * @param customer nombre y teléfono del cliente (vacíos en salón).
 * @param paymentState y [deliveryState]: como los da Go (`payment_state`, `delivery_state`); la
 *   aplicación nunca los calcula ([TakeawayStates]).
 * @param discardable Go dice que es una cuenta vacía que se puede descartar (`discardable`: sin
 *   nada enviado ni cobrado); la aplicación nunca lo decide (etapa A3, §4.10).
 */
data class AccountSummary(
    val id: String,
    val folio: Int,
    val name: String,
    val tableId: String?,
    val tableName: String?,
    val state: String,
    val revision: Int,
    val preparationState: PreparationState?,
    val confirmedQuantity: Int,
    val draftQuantity: Int,
    val totals: Totals,
    val assignedName: String?,
    val elapsedSeconds: Long?,
    val longOpen: Boolean,
    val mergedIntoId: String? = null,
    val mode: OrderMode = OrderMode.DINE_IN,
    val customer: TakeawayCustomer = TakeawayCustomer.NONE,
    val paymentState: String? = null,
    val deliveryState: String? = null,
    val discardable: Boolean = false,
) {
    val isOpen: Boolean
        get() = state == STATE_OPEN

    companion object {
        const val STATE_OPEN = "open"
    }
}

/**
 * Renglón del borrador de una cuenta (`DraftItem`): su identificador (lo genera quien lo captura y
 * se conserva al enviarlo a cocina, A1-D6) y lo pedido.
 */
data class DraftLine(val id: String, val line: LineSelection) {
    /** Mismo producto, presentación, cantidad y personalización (sin mirar el identificador). */
    fun sameContentAs(other: DraftLine): Boolean = line == other.line

    fun withQuantity(quantity: Int): DraftLine = copy(line = line.copy(quantity = quantity))
}

/**
 * Producto ya enviado a cocina (`ConfirmedLine`): se identifica por su versión ([versionId]) para
 * modificarlo o cancelarlo.
 *
 * @param state `active` o `cancelled` (Go).
 * @param name y los importes salen de la cotización de Go al confirmarlo.
 */
data class SentLine(
    val versionId: String,
    val itemId: String,
    val state: String,
    val quantity: Int,
    val preparationState: PreparationState?,
    val request: LineSelection,
    val name: String,
    val unitPrice: Money,
    val total: Money,
) {
    val cancelled: Boolean
        get() = state == STATE_CANCELLED

    companion object {
        const val STATE_CANCELLED = "cancelled"
    }
}

/** Cuenta completa (`Bill`): la cuenta, su borrador y lo enviado. */
data class AccountBill(
    val account: AccountSummary,
    val draft: List<DraftLine>,
    val lines: List<SentLine>,
)

/**
 * Cotización de Go de un borrador (`QuoteResult` de `previewDraft`): la huella que exige «Enviar a
 * cocina» y el precio por unidad de cada renglón, en el orden del borrador.
 */
data class DraftQuote(
    val accountId: String,
    val revision: Int,
    val fingerprint: String,
    val lines: List<QuotedLine>,
    val draftTotal: Money?,
    val combinedTotal: Money,
)

/** Renglón cotizado: nombre, precio por unidad (antes de promociones) e importe. */
data class QuotedLine(
    val productId: String,
    val name: String,
    val unitPrice: Money,
    val quantity: Int,
    val total: Money,
)
