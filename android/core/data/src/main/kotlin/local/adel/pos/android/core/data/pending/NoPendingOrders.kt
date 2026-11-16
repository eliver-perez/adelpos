package local.adel.pos.android.core.data.pending

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import local.adel.pos.android.core.domain.connection.PendingOrdersSource

/** Fuente del contador de pedidos por enviar mientras no existe la cola (Fase E): siempre 0. */
object NoPendingOrders : PendingOrdersSource {
    override fun pendingCount(): Flow<Int> = flowOf(0)
}
