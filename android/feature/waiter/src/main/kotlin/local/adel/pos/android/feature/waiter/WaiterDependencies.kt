package local.adel.pos.android.feature.waiter

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.catalog.CatalogImages
import local.adel.pos.android.core.domain.catalog.CatalogSource
import local.adel.pos.android.core.domain.preferences.TablesViewPreference
import local.adel.pos.android.core.domain.waiter.CaptureService
import local.adel.pos.android.core.domain.waiter.OrderQueueService
import local.adel.pos.android.core.domain.waiter.RestaurantSource
import local.adel.pos.android.core.domain.waiter.SentLineService

/**
 * Lo que necesitan las pantallas del modo Mesero (inyección manual desde `AppContainer`): solo
 * interfaces de `core:domain`, para que la A2 (Para llevar) las reutilice.
 *
 * @param pendingOfOthers pedidos sin enviar de **otros** meseros en este dispositivo (solo el
 *   número: el siguiente mesero nunca ve lo capturado por otro).
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
    val pendingOfOthers: (String) -> Flow<Int>,
    val elapsedClock: () -> Long,
)
