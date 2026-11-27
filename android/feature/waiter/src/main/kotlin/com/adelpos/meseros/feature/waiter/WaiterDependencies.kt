package com.adelpos.meseros.feature.waiter

import com.adelpos.core.model.connection.LinkState
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.preferences.TablesViewPreference
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.TableUnionService
import kotlinx.coroutines.flow.StateFlow

/**
 * Lo que necesitan las pantallas del modo Mesero (inyección manual desde `AppContainer`): la
 * captura compartida de `core:capture` ([capture]) y lo propio de Mesas (la vista guardada y el
 * reloj monótono del tiempo de las mesas). Solo interfaces de `core:domain`.
 *
 * @param elapsedClock reloj monótono para el tiempo de las mesas.
 * @param unions etapa M1: «Unir con…», agregar y quitar mesas de una unión.
 */
class WaiterDependencies(
    val capture: CaptureDependencies,
    val tablesView: TablesViewPreference,
    val elapsedClock: () -> Long,
    /** Etapa M1: unir mesas con Go (sin servidor que lo permita, nada). */
    val unions: TableUnionService = TableUnionService.NONE,
) {
    val restaurant: RestaurantSource
        get() = capture.restaurant

    val captures: CaptureService
        get() = capture.captures

    val orders: OrderQueueService
        get() = capture.orders

    val activeWaiter: StateFlow<ActiveWaiter?>
        get() = capture.activeWaiter

    /** El estado único de conexión (etapa A3), el mismo de la barra. */
    val link: StateFlow<LinkState>
        get() = capture.link
}
