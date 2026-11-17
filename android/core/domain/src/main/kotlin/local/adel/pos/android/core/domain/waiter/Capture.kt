package local.adel.pos.android.core.domain.waiter

import local.adel.pos.android.core.domain.money.Money

/** Cuenta abierta que otra terminal abrió en la mesa (`TABLE_HAS_ACCOUNT`, `details.accounts`). */
data class TableAccountRef(val id: String, val folio: Int, val name: String)

/**
 * Cómo va el guardado del borrador en Go (en línea se guarda con una espera corta, como el
 * escritorio; sin conexión todo queda en el dispositivo).
 */
enum class DraftSync {
    /** Lo que se ve es lo que Go tiene. */
    SAVED,

    /** Hay toques por guardar (esperando la pausa corta o la conexión). */
    PENDING,

    /** Hay una petición en curso. */
    SAVING,

    /**
     * Se envió y no hubo respuesta: se repite con la misma clave de operación antes de mandar nada
     * más.
     */
    UNCERTAIN,

    /** Otra terminal cambió la cuenta mientras había toques por guardar: decide el mesero. */
    CONFLICT,

    /** Otra terminal abrió una cuenta en la mesa: el mesero elige agregarse a ella o abrir otra. */
    TABLE_HAS_ACCOUNT,

    /** Go rechazó el guardado (el mensaje de Go está en [Capture.syncMessage]). */
    REJECTED,

    /**
     * La cuenta ya no está abierta (se cobró, se canceló o se combinó; `account_not_open`): lo del
     * mesero puede ir a una cuenta nueva de la mesa.
     */
    ACCOUNT_CLOSED,
}

/**
 * Captura en curso de un mesero en una mesa (el «Borrador» de la cola, §7), guardada en Room **por
 * mesero**: sobrevive a la rotación, al cierre del proceso y a perder la conexión, y el siguiente
 * mesero no la ve.
 *
 * El borrador visible es [base] (lo último que Go aceptó o mostró) con las [operations] locales
 * encima; con [baseRevision] 0 todavía no se leyó el borrador de Go (se lee antes del primer
 * guardado y los toques se aplican encima). Sin cuenta ([accountId] null) es una «cuenta
 * pendiente»: nace en Go con el primer producto (`openAccount`, creación diferida) o, sin conexión,
 * al enviarse el pedido.
 *
 * @param ownItemIds renglones creados en este dispositivo: los que el pedido sin conexión manda y,
 *   si ya estaban guardados en Go, sustituye (A1-D6).
 * @param seenPrices precio por unidad que vio el mesero por renglón: el de la cotización de Go si
 *   hubo (en línea) o el de referencia del catálogo; viaja como `seen_unit_price`.
 * @param pendingOperationId clave del guardado en Go en curso o sin respuesta: se repite **igual**
 *   (misma clave y mismo contenido: [base] con las primeras [inFlightCount] operaciones) antes de
 *   mandar nada más, también después de reiniciar el proceso.
 */
data class Capture(
    val id: String,
    val waiterUserId: String,
    val tableId: String,
    val tableName: String,
    val accountId: String?,
    val accountName: String?,
    val seenAccountIds: List<String>,
    val base: List<DraftLine>,
    val baseRevision: Int,
    val operations: List<DraftOperation>,
    val ownItemIds: Set<String>,
    val seenPrices: Map<String, Money>,
    val sync: DraftSync,
    val syncMessage: String?,
    val tableAccounts: List<TableAccountRef>,
    val pendingOperationId: String?,
    val inFlightCount: Int,
    val createdAtMillis: Long,
) {
    /** El borrador como se ve: lo de Go con los toques locales aplicados. */
    val visible: List<DraftLine>
        get() = DraftOperations.apply(base, operations)

    /** Sin cuenta en Go todavía (creación diferida). */
    val pendingAccount: Boolean
        get() = accountId == null

    /**
     * Hay algo de este mesero sin enviar a cocina: toques sin guardar o renglones suyos en el
     * borrador. Impide el bloqueo por inactividad y «Cerrar sesión» (A1-D15).
     */
    val inProgress: Boolean
        get() = operations.isNotEmpty() || base.any { it.id in ownItemIds }

    /** Lo que se mandó (o se va a repetir) en el guardado en curso. */
    val inFlightItems: List<DraftLine>
        get() = DraftOperations.apply(base, operations.take(inFlightCount))

    /** Renglón del borrador que se capturó en este dispositivo. */
    fun isOwn(itemId: String): Boolean = itemId in ownItemIds
}

/**
 * Lo que manda un pedido sin conexión hecho a partir de una captura (A1-D6, `submitOfflineOrder`).
 *
 * - [items]: los renglones de este dispositivo y los que tocó aquí (nuevos, cambiados o suyos que
 *   ya estaban guardados en Go), con su identificador (Go lo conserva como `draft_item_id`).
 * - [replaces]: los renglones del borrador de Go que el pedido sustituye **tal como los vio** el
 *   dispositivo (`replaces_draft_items`): los suyos guardados, los que cambió y los que quitó. Go
 *   quita solo los que siguen exactamente así; lo demás del borrador (otras terminales) no se toca.
 */
data class OfflineOrderPlan(val items: List<DraftLine>, val replaces: List<DraftLine>) {
    companion object {
        fun of(capture: Capture): OfflineOrderPlan {
            val visible = capture.visible
            val visibleById = visible.associateBy(DraftLine::id)
            val baseById = capture.base.associateBy(DraftLine::id)
            val items = visible.filter { item ->
                val saved = baseById[item.id]
                saved == null || !saved.sameContentAs(item) || capture.isOwn(item.id)
            }
            // Sin cuenta en Go no hay borrador que sustituir (Go lo rechaza sin account_id).
            if (capture.accountId == null) {
                return OfflineOrderPlan(items = items, replaces = emptyList())
            }
            val replaces =
                capture.base.filter { saved ->
                    val now = visibleById[saved.id]
                    when {
                        // Lo quitó este dispositivo (los toques locales son los únicos que quitan).
                        now == null -> true
                        !saved.sameContentAs(now) -> true
                        else -> capture.isOwn(saved.id)
                    }
                }
            return OfflineOrderPlan(items = items, replaces = replaces)
        }
    }
}
