package com.adelpos.meseros.core.domain.waiter

/**
 * Qué dice la barra superior en la captura (etapa A3, §4.2): dónde se está, no «Captura». Los
 * textos están en `core:capture`.
 */
sealed interface CaptureTitle {
    /** «Mesa 4»: el nombre de la mesa. */
    data class Table(val name: String) : CaptureTitle

    /** «Para llevar #12»: el folio que dio Go. */
    data class Takeaway(val folio: Int) : CaptureTitle

    /** Sin folio todavía (no ha nacido en Go), con el nombre del cliente: «Para llevar · Juan». */
    data class NamedTakeaway(val name: String) : CaptureTitle

    /** Sin folio ni nombre: «Nuevo pedido para llevar». */
    data object NewTakeaway : CaptureTitle

    companion object {
        /**
         * El título de [capture]. [bill] es la cuenta leída de Go: su folio solo cuenta si es la
         * cuenta de esta captura.
         */
        fun of(capture: Capture, bill: AccountBill?): CaptureTitle {
            if (!capture.takeaway) {
                return Table(capture.tableName)
            }
            val folio =
                bill
                    ?.account
                    ?.takeIf { it.id == capture.accountId && capture.accountId != null }
                    ?.folio
            val name = capture.customer.name.trim()
            return when {
                folio != null -> Takeaway(folio)
                name.isNotEmpty() -> NamedTakeaway(name)
                else -> NewTakeaway
            }
        }
    }
}
