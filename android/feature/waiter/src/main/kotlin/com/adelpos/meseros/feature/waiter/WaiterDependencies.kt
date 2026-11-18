package com.adelpos.meseros.feature.waiter

import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.preferences.TablesViewPreference
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.SentLineService
import kotlinx.coroutines.flow.StateFlow

/**
 * Lo que necesitan las pantallas del modo Mesero (inyección manual desde `AppContainer`): solo
 * interfaces de `core:domain`, para que la A2 (Para llevar) las reutilice.
 *
 * @param elapsedClock reloj monótono para el tiempo de las mesas.
 */
class WaiterDependencies(
    val restaurant: RestaurantSource,
    val captures: CaptureService,
    val orders: OrderQueueService,
    val sentLines: SentLineService,
    val catalog: CatalogSource,
    val images: CatalogImages?,
    val tablesView: TablesViewPreference,
    val activeWaiter: StateFlow<ActiveWaiter?>,
    val elapsedClock: () -> Long,
)
