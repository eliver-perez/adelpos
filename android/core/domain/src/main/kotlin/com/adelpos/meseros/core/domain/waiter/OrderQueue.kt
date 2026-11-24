package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.access.WaiterWork
import com.adelpos.meseros.core.domain.money.Money

/**
 * Estado de un pedido del mesero (§7), siempre visible y distinto de los demás. El «Borrador» es la
 * captura en curso ([Capture]); los demás son pedidos de la cola ([PendingOrder]).
 */
enum class OrderState(val storedName: String) {
    /** En el dispositivo, sin enviar (la captura). */
    DRAFT("draft"),

    /** El mesero ya lo confirmó con «Enviar a cocina», pero no ha salido (sin conexión). */
    QUEUED("queued"),

    /** Saliendo hacia el servidor (la respuesta todavía no llega). */
    SENDING("sending"),

    /** Recibido por el restaurante: **solo** con la confirmación de Go. */
    RECEIVED("received"),

    /** Cambió un precio, se agotó un producto, se cobró la mesa…: lo revisa el mesero. */
    NEEDS_REVIEW("needs_review");

    /** Cuenta para el indicador, para el retiro del dispositivo y para cambiar de mesero (§7). */
    val unsent: Boolean
        get() = this == QUEUED || this == SENDING || this == NEEDS_REVIEW

    companion object {
        fun fromStoredName(name: String): OrderState =
            requireNotNull(entries.firstOrNull { it.storedName == name }) {
                "Estado de pedido desconocido: $name"
            }
    }
}

/**
 * Transiciones permitidas de la cola. Cualquier otra es un error de programación y se rechaza (la
 * base nunca guarda un salto prohibido):
 * - un borrador nunca se envía solo: pasa a «Por enviar» o «Enviando» **solo** con «Enviar a
 *   cocina» del mesero;
 * - «Recibido» solo llega desde «Enviando» (con la respuesta de Go) y es final;
 * - «Requiere tu revisión» solo llega desde «Enviando» (con el rechazo de Go) y solo sale cuando el
 *   mesero acepta o corrige («Por enviar») o lo regresa a la captura («Borrador»).
 */
object OrderTransitions {
    private val allowed: Map<OrderState, Set<OrderState>> =
        mapOf(
            OrderState.DRAFT to setOf(OrderState.QUEUED, OrderState.SENDING),
            OrderState.QUEUED to setOf(OrderState.SENDING),
            OrderState.SENDING to
                setOf(OrderState.QUEUED, OrderState.RECEIVED, OrderState.NEEDS_REVIEW),
            OrderState.NEEDS_REVIEW to setOf(OrderState.QUEUED, OrderState.DRAFT),
            OrderState.RECEIVED to emptySet(),
        )

    fun isAllowed(from: OrderState, to: OrderState): Boolean = to in allowed.getValue(from)

    fun require(from: OrderState, to: OrderState) {
        check(isAllowed(from, to)) { "Transición de pedido no permitida: $from → $to" }
    }
}

/**
 * A dónde va el pedido: una cuenta abierta, una mesa en la que se abre una cuenta nueva (con las
 * cuentas que el mesero veía abiertas en ella; Go avisa `table_has_account` si había otra) o, en un
 * pedido para llevar sin cuenta todavía, un pedido para llevar nuevo (`destination.takeaway`, con
 * [customer]; etapa A2).
 *
 * @param tableId null en un pedido para llevar.
 * @param customer el nombre y el teléfono **con que salió** el pedido (no cambian: el reintento con
 *   la misma clave lleva el mismo contenido; una edición posterior es una [CustomerEdit]).
 */
data class OrderDestination(
    val tableId: String?,
    val tableName: String,
    val accountId: String?,
    val accountName: String?,
    val seenAccountIds: List<String>,
    val mode: OrderMode = OrderMode.DINE_IN,
    val customer: TakeawayCustomer = TakeawayCustomer.NONE,
) {
    val takeaway: Boolean
        get() = mode == OrderMode.TAKEAWAY
}

/**
 * Partida de un pedido de la cola: el renglón (con su identificador), el nombre que se mostró y el
 * precio por unidad que vio el mesero (`seen_unit_price`, sin calcular nada: el de la cotización de
 * Go o el de referencia del catálogo).
 */
data class OrderLine(val item: DraftLine, val productName: String, val seenUnitPrice: Money)

/** Motivo de un cambio que Go encontró al recibir el pedido (`OfflineChange.kind`). */
enum class ChangeKind(val wireName: String) {
    PRICE_CHANGED("price_changed"),
    PRODUCT_SOLD_OUT("product_sold_out"),
    PRODUCT_UNAVAILABLE("product_unavailable"),
    PRODUCT_REMOVED("product_removed"),
    COMPLEMENT_UNAVAILABLE("complement_unavailable"),
    ITEM_UNAVAILABLE("item_unavailable"),
    ITEM_INVALID("item_invalid"),
    ACCOUNT_CLOSED("account_closed"),
    TABLE_UNAVAILABLE("table_unavailable"),
    TABLE_HAS_ACCOUNT("table_has_account"),
    DRAFT_ITEM_CHANGED("draft_item_changed"),
    DRAFT_ITEM_REMOVED("draft_item_removed"),
    DRAFT_ITEM_SENT("draft_item_sent"),

    /**
     * El pedido iba a la mesa, pero sus partidas ya están en una cuenta abierta de esa mesa
     * ([OrderChange.accountId]; A1-D18): el pedido se manda a esa cuenta con la misma clave.
     */
    ITEMS_IN_ACCOUNT("items_in_account"),

    /**
     * Un motivo que esta versión no conoce: se muestra el mensaje de Go y vale lo que diga su
     * `acceptable` (en la confirmación se devuelve con su `kind` original).
     */
    UNKNOWN("");

    companion object {
        fun fromWireName(name: String): ChangeKind =
            entries.firstOrNull { it.wireName == name && it != UNKNOWN } ?: UNKNOWN
    }
}

/**
 * Un cambio de «Requiere tu revisión» tal como lo dio Go (`OfflineChange`), con el texto que se
 * muestra ([message], en español, de Go). Se devuelve tal cual en la confirmación (`kind`,
 * `item_id`, `product_id` y `now` son los que Go compara).
 *
 * @param wireKind el `kind` original (también uno que esta versión no conoce).
 * @param accountId solo en [ChangeKind.ITEMS_IN_ACCOUNT]: la cuenta de la mesa donde ya están las
 *   partidas.
 */
data class OrderChange(
    val kind: ChangeKind,
    val wireKind: String,
    val itemId: String?,
    val productId: String?,
    val productName: String,
    val before: String?,
    val now: String?,
    val message: String,
    val acceptable: Boolean,
    val accountId: String? = null,
)

/** Por qué el pedido requiere la revisión del mesero. */
enum class ReviewReason {
    /** `ORDER_REQUIRES_CONFIRMATION` con su lista de cambios. */
    CHANGES,

    /** La cuenta ya no está abierta (`details.reason: account_not_open` en línea). */
    ACCOUNT_NOT_OPEN,

    /** Go no aceptó el pedido por otro motivo (permisos, datos): el mensaje de Go lo explica. */
    REJECTED,

    /**
     * Etapa L1: la licencia, un cupo o la hora de la computadora principal no lo permitieron
     * ([SendOutcome.NotAllowed]). Go no guardó nada: el mesero lo envía de nuevo, con la misma
     * clave, cuando se resuelva; nunca se reintenta solo. Se guarda en Room por su nombre, dentro
     * del JSON de la revisión (`pending_orders.review`): no cambia el esquema.
     */
    NOT_ALLOWED,
}

/**
 * Revisión pendiente de un pedido: lo que dijo Go la última vez que lo rechazó.
 *
 * @param catalogVersion la versión del catálogo del rechazo; la confirmación debe llevarla.
 * @param mergedIntoId la cuenta en que se combinó la del pedido, si Go la dijo (en línea,
 *   `account_not_open` con `merged_into_id`).
 */
data class OrderReview(
    val reason: ReviewReason,
    val message: String,
    val changes: List<OrderChange>,
    val catalogVersion: Long?,
    val mergedIntoId: String? = null,
) {
    /** Cambios que el mesero tiene que corregir (quitar o cambiar la partida, otro destino). */
    val blocking: List<OrderChange>
        get() = changes.filter { !it.acceptable }

    /** Con solo cambios aceptables, «Aceptar los cambios y enviar» basta. */
    val canAccept: Boolean
        get() = reason == ReviewReason.CHANGES && changes.isNotEmpty() && blocking.isEmpty()

    /**
     * Partidas que Go ya tiene en cocina (`draft_item_sent`): al confirmar, Go **no** las vuelve a
     * enviar y manda solo lo demás (A1-D18).
     */
    val alreadySentItemIds: Set<String>
        get() =
            changes
                .filter { it.kind == ChangeKind.DRAFT_ITEM_SENT }
                .mapNotNull { it.itemId }
                .toSet()

    /** Cuentas de la mesa donde ya están las partidas del pedido (`items_in_account`). */
    val accountsWithItems: List<String>
        get() =
            changes
                .filter { it.kind == ChangeKind.ITEMS_IN_ACCOUNT }
                .mapNotNull { it.accountId }
                .distinct()

    /**
     * La cuenta del pedido se combinó con otra (`account_closed` con su estado `merged`, o
     * `account_not_open` con la cuenta en que se combinó).
     */
    val accountMerged: Boolean
        get() =
            mergedIntoId != null ||
                changes.any { it.kind == ChangeKind.ACCOUNT_CLOSED && it.now == MERGED }

    companion object {
        /** Estado de Go de una cuenta combinada con otra. */
        const val MERGED = "merged"
    }
}

/**
 * Guardado del borrador en Go que salió y no tuvo respuesta cuando el mesero tocó «Enviar a cocina»
 * (revisión R6, C1; A1-D19): pudo haber entrado. Antes de mandar el pedido, quien tenga la sesión
 * del mesero lo repite **igual** (misma clave y mismo contenido) para saber qué pasó y, si abrió la
 * cuenta, mandar el pedido a esa cuenta. Sin sesión, Go reconoce los renglones por su identificador
 * (A1-D18).
 *
 * @param accountId null: era la apertura de la cuenta en [tableId] (`openAccount`) o, en un pedido
 *   para llevar ([mode]), la de un pedido para llevar nuevo con [customer].
 */
data class UnresolvedSave(
    val operationId: String,
    val accountId: String?,
    val tableId: String?,
    val seenAccountIds: List<String>,
    val revision: Int,
    val items: List<DraftLine>,
    val mode: OrderMode = OrderMode.DINE_IN,
    val customer: TakeawayCustomer = TakeawayCustomer.NONE,
)

/**
 * Intento de confirmar en línea el borrador de Go (`confirmDraft`): su clave de operación, la
 * revisión de la cuenta y la huella de la cotización que vio el mesero.
 */
data class OnlineConfirm(
    val operationId: String,
    val accountId: String,
    val revision: Int,
    val fingerprint: String,
)

/** La confirmación del mesero (misma clave de operación, `confirmation` del contrato). */
data class OrderConfirmation(val catalogVersion: Long, val changes: List<OrderChange>)

/**
 * Pedido de la cola (§7), guardado en Room en una transacción: nunca se borra por cerrar la
 * aplicación, reiniciar el dispositivo o perder la batería.
 *
 * @param operationId la **clave de operación**: la misma en todos sus reenvíos (Go nunca lo
 *   duplica).
 * @param catalogVersion y [catalogEpoch]: el catálogo con que se capturó (la época solo se guarda;
 *   el contrato recibe la versión).
 * @param replaces renglones del borrador de Go que el pedido sustituye (A1-D6).
 * @param credentialRejected Go no reconoció la credencial de envío del mesero
 *   (`OFFLINE_CREDENTIAL_INVALID`): el pedido se conserva hasta que el mesero entre con su PIN con
 *   conexión.
 * @param onlineConfirm con conexión y el borrador ya guardado en Go, «Enviar a cocina» primero
 *   intenta confirmar ese borrador (`confirmDraft`) con su propia clave; si no se puede (sin
 *   respuesta, cambió el cálculo, otra terminal cambió la cuenta…), el pedido sigue como pedido sin
 *   conexión con [operationId] y Go revisa todo al recibirlo.
 * @param receivedAccountId cuenta en que entró (al recibirlo Go).
 * @param unresolvedSave el guardado del borrador que quedó sin respuesta al enviarlo (C1).
 * @param clockCorrected [capturedAtMillis] ya está en la hora de Go (se conocía la diferencia con
 *   el reloj del dispositivo al capturar); si no, se corrige al enviarlo (revisión R6, B4).
 */
data class PendingOrder(
    val operationId: String,
    val waiterUserId: String,
    val waiterName: String,
    val state: OrderState,
    val destination: OrderDestination,
    val lines: List<OrderLine>,
    val replaces: List<DraftLine>,
    val catalogVersion: Long,
    val catalogEpoch: String?,
    val capturedAtMillis: Long,
    val confirmation: OrderConfirmation?,
    val review: OrderReview?,
    val attempts: Int,
    val lastError: String?,
    val credentialRejected: Boolean,
    val onlineConfirm: OnlineConfirm?,
    val receivedAccountId: String?,
    val receivedAtMillis: Long?,
    val updatedAtMillis: Long,
    val unresolvedSave: UnresolvedSave? = null,
    val clockCorrected: Boolean = false,
)

/**
 * Lo pendiente de otro mesero en este dispositivo, sin el detalle de sus pedidos (§7): cuántos
 * tiene sin enviar y por qué no salen (revisión R6, B3).
 *
 * @param waitingForPin Go no reconoció su credencial de envío: PIN cambiado, usuario desactivado o
 *   sin permiso para usar dispositivos de meseros.
 * @param needingReview esperan a que ese mesero los revise (también los que Go rechazó por falta de
 *   permiso).
 */
data class OthersPending(
    val waiterName: String,
    val unsent: Int,
    val waitingForPin: Int,
    val needingReview: Int,
)

/**
 * Trabajo pendiente de un mesero (§0-B, §7 y A1-D15) a partir de sus capturas y sus pedidos: la
 * captura en curso impide el bloqueo por inactividad y «Cerrar sesión»; los pedidos sin enviar
 * impiden además cambiar de mesero.
 */
object WaiterWorkSummary {
    fun of(captures: List<Capture>, orders: List<PendingOrder>): WaiterWork =
        WaiterWork(
            captureInProgress = captures.any(Capture::inProgress),
            unsentOrders = orders.count { it.state.unsent },
        )
}
