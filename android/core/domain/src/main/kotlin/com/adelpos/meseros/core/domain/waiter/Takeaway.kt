package com.adelpos.meseros.core.domain.waiter

/*
 * Para llevar en el dispositivo (etapa A2, §3 de la especificación, A2-D1 y A2-D2). Un pedido para
 * llevar es una cuenta de Go con `mode: "takeaway"` y sin mesa (§0-B: no hay mesa ficticia). La
 * aplicación solo muestra lo que Go calcula (preparación, pago y entrega) y nunca cobra.
 */

/** Modalidad de una captura, de un pedido de la cola o de una cuenta (`mode` de Go). */
enum class OrderMode(val wireName: String) {
    DINE_IN("dine_in"),
    TAKEAWAY("takeaway");

    companion object {
        /** Sin modalidad o una que esta versión no conoce: la de siempre, de salón. */
        fun fromWireName(name: String?): OrderMode =
            entries.firstOrNull { it.wireName == name } ?: DINE_IN
    }
}

/**
 * Nombre del cliente y teléfono de un pedido para llevar (A2-D1): los dos opcionales (vacío = sin
 * dato) y se editan en cualquier momento. No se pide formato ni se valida más allá de los límites
 * de Go, que cuenta **bytes** del texto ya recortado (100 y 40).
 */
data class TakeawayCustomer(val name: String, val phone: String) {
    val isEmpty: Boolean
        get() = name.isBlank() && phone.isBlank()

    /** Como lo guarda Go: sin espacios al principio ni al final. */
    fun trimmed(): TakeawayCustomer = TakeawayCustomer(name.trim(), phone.trim())

    /** Cabe en los límites de Go (los dos campos, ya recortados). */
    val fitsLimits: Boolean
        get() = fitsName(name) && fitsPhone(phone)

    companion object {
        val NONE = TakeawayCustomer("", "")

        /** Límites de Go (`maxCustomerNameLength` y `maxPhoneLength`), en bytes del UTF-8. */
        const val NAME_MAX_BYTES = 100
        const val PHONE_MAX_BYTES = 40

        fun fitsName(text: String): Boolean = text.trim().encodeToByteArray().size <= NAME_MAX_BYTES

        fun fitsPhone(text: String): Boolean =
            text.trim().encodeToByteArray().size <= PHONE_MAX_BYTES
    }
}

/** Valores fijos de Para llevar que comparten el dispositivo, la caja y Go. */
object Takeaway {
    /**
     * Origen con que nace un pedido para llevar del dispositivo: el mismo de la caja
     * (`defaultTakeawayOrigin`) y el que pone Go al pedido para llevar capturado sin conexión.
     */
    const val ORIGIN = "Para llevar"
}

/**
 * Preparación de un pedido para llevar en la lista (§3.3: pendiente, en preparación, listo), a
 * partir del estado que calcula Go (`preparation_state`, el mismo modelo de la cocina).
 */
enum class TakeawayPreparation {
    /** Todavía no se envió nada a cocina (`without_items`, `draft`). */
    NOT_SENT,

    /** Enviado y esperando a cocina (`pending_release`, `scheduled`, `released`…). */
    PENDING,

    /** Cocina ya empezó (`preparing`). */
    PREPARING,

    /** Todo listo (`ready`). */
    READY,

    /** Se canceló lo enviado (`cancelled`). */
    CANCELLED,

    /** Go no dijo el estado o esta versión no lo conoce: no se inventa uno. */
    UNKNOWN,
}

/** Pago del pedido según Go (`payment_state`): la aplicación nunca cobra. */
enum class TakeawayPayment {
    /** `not_recorded`. */
    UNPAID,

    /** `partial`: como lo diga Go («Pago parcial»). */
    PARTIAL,

    /** `paid`. */
    PAID,
    UNKNOWN,
}

/** Entrega del pedido según Go (`delivery_state`). */
enum class TakeawayDelivery {
    NOT_DELIVERED,
    DELIVERED,
    UNKNOWN,
}

/** Lo que el dispositivo ofrece para la entrega de un pedido (A2-D2). */
enum class DeliveryOffer {
    /** «Entregado»: con `takeaway.deliver` y el pedido pagado según Go. */
    DELIVER,

    /** «Cobrar en caja antes de entregar»: con el permiso, pero Go no lo da por pagado. */
    COLLECT_FIRST,

    /** Sin el permiso: solo se ven el pago y la entrega (la entrega se marca en la caja). */
    STATUS_ONLY,

    /** Ya se entregó. */
    DELIVERED,
}

/** Correspondencia entre los estados de Go y los de la lista de Para llevar (§3.3). */
object TakeawayStates {
    fun preparation(state: PreparationState?): TakeawayPreparation =
        when (state) {
            PreparationState.WITHOUT_ITEMS,
            PreparationState.DRAFT -> TakeawayPreparation.NOT_SENT
            // Enviado: confirmado, por liberar, programado, liberado o con un cambio que cocina
            // todavía no ve. Nada de eso es «en preparación» hasta que cocina lo empiece.
            PreparationState.CONFIRMED_PENDING_DISPATCH,
            PreparationState.PENDING_RELEASE,
            PreparationState.SCHEDULED,
            PreparationState.RELEASED,
            PreparationState.CHANGE_PENDING -> TakeawayPreparation.PENDING
            PreparationState.PREPARING -> TakeawayPreparation.PREPARING
            PreparationState.READY -> TakeawayPreparation.READY
            PreparationState.CANCELLED -> TakeawayPreparation.CANCELLED
            null -> TakeawayPreparation.UNKNOWN
        }

    fun payment(wireName: String?): TakeawayPayment =
        when (wireName) {
            PAYMENT_NOT_RECORDED -> TakeawayPayment.UNPAID
            PAYMENT_PARTIAL -> TakeawayPayment.PARTIAL
            PAYMENT_PAID -> TakeawayPayment.PAID
            else -> TakeawayPayment.UNKNOWN
        }

    fun delivery(wireName: String?): TakeawayDelivery =
        when (wireName) {
            DELIVERY_NOT_DELIVERED -> TakeawayDelivery.NOT_DELIVERED
            DELIVERY_DELIVERED -> TakeawayDelivery.DELIVERED
            else -> TakeawayDelivery.UNKNOWN
        }

    /**
     * Cuándo se ofrece «Entregado» (A2-D2): **solo** con el permiso de Go y el pedido pagado según
     * Go. La regla «no se entrega un pedido sin pagar» la impone Go (`BALANCE_PENDING`); el
     * dispositivo no decide nada, solo no ofrece lo que Go rechazaría. Con permisos desconocidos
     * (nunca se leyeron) no se ofrece: la entrega exige conexión y, con ella, se leen.
     */
    fun deliveryOffer(
        permissions: WaiterPermissions,
        payment: TakeawayPayment,
        delivery: TakeawayDelivery,
    ): DeliveryOffer =
        when {
            delivery == TakeawayDelivery.DELIVERED -> DeliveryOffer.DELIVERED
            !permissions.grants(WaiterAction.DELIVER_TAKEAWAY) -> DeliveryOffer.STATUS_ONLY
            payment == TakeawayPayment.PAID -> DeliveryOffer.DELIVER
            else -> DeliveryOffer.COLLECT_FIRST
        }

    const val PAYMENT_NOT_RECORDED = "not_recorded"
    const val PAYMENT_PARTIAL = "partial"
    const val PAYMENT_PAID = "paid"
    const val DELIVERY_NOT_DELIVERED = "not_delivered"
    const val DELIVERY_DELIVERED = "delivered"
}

/**
 * Edición del nombre y el teléfono que todavía no está en Go (A2-D1): se guarda en el dispositivo y
 * se envía en cuanto el mesero que la hizo tenga sesión con conexión; la última gana y nunca se
 * pierde por falta de conexión.
 *
 * @param key el pedido: la cuenta de Go si ya se conoce; si no, la captura o el pedido de la cola
 *   (los dos tienen el mismo identificador, A1-D19).
 * @param accountId la cuenta de Go, en cuanto se sabe (null mientras el pedido no nace en Go).
 */
data class CustomerEdit(
    val key: String,
    val accountId: String?,
    val waiterUserId: String,
    val customer: TakeawayCustomer,
    val operationId: String,
    val updatedAtMillis: Long,
)

/** A qué pedido de la lista se le cambia el nombre y el teléfono. */
sealed interface CustomerTarget {
    /** Un pedido que ya está en Go. */
    data class Account(val accountId: String) : CustomerTarget

    /** Un pedido de la cola que todavía no nace en Go («Por enviar»). */
    data class QueuedOrder(val operationId: String) : CustomerTarget
}

/** Resultado de «Entregado» (A2-D2). */
sealed interface DeliveryResult {
    /** Go lo marcó entregado; con [closed] el pedido se cerró y sale de la lista. */
    data class Delivered(val closed: Boolean) : DeliveryResult

    /** `BALANCE_PENDING`: «Cobrar en caja antes de entregar». */
    data object CollectFirst : DeliveryResult

    /** `ALREADY_DELIVERED`: ya se había entregado; se vuelve a leer. */
    data object AlreadyDelivered : DeliveryResult

    /** Sin conexión (la entrega no va a la cola): «Para marcarlo necesitas conexión». */
    data object NoConnection : DeliveryResult

    /** Go lo rechazó por otro motivo (el mensaje de Go). */
    data class Failed(val message: String?) : DeliveryResult
}
