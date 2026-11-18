package com.adelpos.meseros.core.data.pending

import com.adelpos.meseros.core.domain.connection.PendingOrdersSource
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf

/** Fuente del contador de pedidos por enviar mientras no existe la cola (Fase E): siempre 0. */
object NoPendingOrders : PendingOrdersSource {
    override fun pendingCount(): Flow<Int> = flowOf(0)
}
