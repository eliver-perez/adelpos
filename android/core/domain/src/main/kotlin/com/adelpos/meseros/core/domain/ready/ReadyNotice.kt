package com.adelpos.meseros.core.domain.ready

import com.adelpos.meseros.core.domain.waiter.OrderMode

/**
 * La notificación de **una cuenta** (§4.4: agrupado por mesa o pedido): una sola por cuenta, que se
 * reemplaza con el acumulado mientras siga a la vista («Mesa 4 · 3 platillos listos», «2 × Filete,
 * 1 × Tacos»). Guarda qué avisos ya contiene para no mostrar dos veces el mismo (ver
 * [ReadyAlertPresenter.present]).
 *
 * @param shown los avisos que ya contiene (número y hora).
 * @param items los platillos acumulados, en el orden en que llegaron; el mismo nombre se suma.
 * @param waiterUserId el mesero del último aviso (si la cuenta se transfirió, el nuevo): al tocarla
 *   en «¿Quién eres?» se le pide su PIN.
 */
data class ReadyNotice(
    val accountId: String,
    val mode: OrderMode,
    val tableName: String?,
    val folio: Long,
    val customerName: String,
    val waiterUserId: String,
    val items: List<ReadyItem>,
    val shown: List<ReadyAlertKey>,
) {
    /** Cuántos platillos listos acumula (la suma de las cantidades). */
    val dishCount: Int
        get() = items.sumOf { it.quantity }

    fun contains(alert: ReadyAlert): Boolean = alert.key in shown

    /**
     * El acumulado con [alert]: suma sus platillos (el mismo nombre en un solo renglón) y toma de
     * él la mesa, el cliente y el mesero (pudieron cambiar: otra mesa, nombre capturado después o
     * cuenta transferida). Un aviso que ya contiene no cambia nada.
     */
    fun plus(alert: ReadyAlert): ReadyNotice {
        if (contains(alert)) {
            return this
        }
        return copy(
            mode = alert.mode,
            tableName = alert.tableName,
            folio = alert.folio,
            customerName = alert.customerName,
            waiterUserId = alert.waiterUserId,
            items = merge(items, alert.items),
            shown = shown + alert.key,
        )
    }

    companion object {
        /** La notificación de una cuenta con su primer aviso. */
        fun of(alert: ReadyAlert): ReadyNotice =
            ReadyNotice(
                accountId = alert.accountId,
                mode = alert.mode,
                tableName = alert.tableName,
                folio = alert.folio,
                customerName = alert.customerName,
                waiterUserId = alert.waiterUserId,
                items = merge(emptyList(), alert.items),
                shown = listOf(alert.key),
            )

        /**
         * Junta los avisos de un lote con lo que ya estaba a la vista, **por cuenta**: una
         * notificación por cuenta, en el orden en que llegaron. [current] da la notificación que ya
         * está a la vista de una cuenta (o null).
         */
        fun accumulate(
            alerts: List<ReadyAlert>,
            current: (String) -> ReadyNotice?,
        ): List<ReadyNotice> {
            val notices = LinkedHashMap<String, ReadyNotice?>()
            for (alert in alerts) {
                if (!notices.containsKey(alert.accountId)) {
                    notices[alert.accountId] = current(alert.accountId)
                }
                val previous = notices[alert.accountId]
                notices[alert.accountId] = previous?.plus(alert) ?: of(alert)
            }
            return notices.values.filterNotNull()
        }

        // El mismo platillo (el mismo nombre visible, con su presentación) se suma en un renglón.
        private fun merge(current: List<ReadyItem>, added: List<ReadyItem>): List<ReadyItem> {
            val quantities = LinkedHashMap<String, Int>()
            for (item in current + added) {
                if (item.quantity <= 0) {
                    continue
                }
                quantities[item.productName] = (quantities[item.productName] ?: 0) + item.quantity
            }
            return quantities.map { (name, quantity) -> ReadyItem(name, quantity) }
        }
    }
}

/**
 * Los textos del aviso (A2-D10), con las plantillas de `strings.xml` de la aplicación (regla 2 de
 * `android/CLAUDE.md`: los textos viven en los recursos). Go solo concuerda contadores con palabras
 * fijas (`plural.Count`) y no puede dar plural ni género a un producto cualquiera, así que el
 * título lleva el contador con «platillo» y el texto los platillos con su cantidad: «Mesa 4 · 2
 * platillos listos» / «2 × Filete de res»; «Para llevar #12 (Juan) · 1 platillo listo». Nunca
 * precios.
 *
 * @param dishOne y [dishOther] «platillo listo» y «platillos listos» (como `plural.Count` de Go).
 * @param takeaway «Para llevar #%1$d»; [takeawayNamed] «Para llevar #%1$d (%2$s)».
 * @param account una cuenta sin mesa ni modalidad conocida: «Cuenta #%1$d».
 * @param title «%1$s · %2$s» (lugar y contador); [item] «%1$d × %2$s»; [itemSeparator] «, ».
 * @param summaryTitle el resumen del grupo con varias cuentas: «Platillos listos».
 * @param summarySeparator entre los lugares del resumen: «, ».
 */
data class ReadyAlertWords(
    val dishOne: String,
    val dishOther: String,
    val takeaway: String,
    val takeawayNamed: String,
    val account: String,
    val title: String,
    val item: String,
    val itemSeparator: String,
    val summaryTitle: String,
    val summarySeparator: String,
) {
    /** «Mesa 4», «Para llevar #12 (Juan)» o «Para llevar #12». */
    fun place(notice: ReadyNotice): String {
        val customer = notice.customerName.trim()
        return when {
            notice.mode == OrderMode.TAKEAWAY && customer.isNotEmpty() ->
                takeawayNamed.format(notice.folio, customer)
            notice.mode == OrderMode.TAKEAWAY -> takeaway.format(notice.folio)
            !notice.tableName.isNullOrBlank() -> notice.tableName.trim()
            else -> account.format(notice.folio)
        }
    }

    /** «1 platillo listo», «2 platillos listos». */
    fun dishes(count: Int): String =
        if (count == 1) {
            "$count $dishOne"
        } else {
            "$count $dishOther"
        }

    /** «Mesa 4 · 2 platillos listos». */
    fun title(notice: ReadyNotice): String = title.format(place(notice), dishes(notice.dishCount))

    /** «2 × Filete · Grande, 1 × Tacos». */
    fun text(notice: ReadyNotice): String =
        notice.items.joinToString(itemSeparator) { item.format(it.quantity, it.productName) }

    /** El texto del resumen con varias cuentas: «Mesa 4, Para llevar #12». */
    fun summaryText(notices: List<ReadyNotice>): String =
        notices.joinToString(summarySeparator) { place(it) }
}
