package com.adelpos.meseros.core.domain.connection

import kotlinx.coroutines.flow.Flow

/**
 * Cuántos pedidos de este dispositivo faltan por llegar al servidor («Por enviar», «Enviando» o
 * «Requiere tu revisión»), para el contador del indicador de conexión. La cola sin conexión llega
 * en la Fase E; mientras tanto la fuente devuelve 0.
 */
fun interface PendingOrdersSource {
    fun pendingCount(): Flow<Int>
}
