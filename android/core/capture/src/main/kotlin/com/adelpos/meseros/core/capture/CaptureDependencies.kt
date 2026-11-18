package com.adelpos.meseros.core.capture

import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.waiter.CaptureService
import com.adelpos.meseros.core.domain.waiter.OrderQueueService
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.SentLineService
import com.adelpos.meseros.core.domain.waiter.TakeawayService
import kotlinx.coroutines.flow.StateFlow

/**
 * Lo que necesita la captura compartida de Mesas y Para llevar (inyección manual desde
 * `AppContainer`): solo interfaces de `core:domain`. `feature:waiter` y `feature:takeaway` la
 * reciben y la pasan a las pantallas de este módulo; ninguno depende del otro.
 */
class CaptureDependencies(
    val restaurant: RestaurantSource,
    val captures: CaptureService,
    val orders: OrderQueueService,
    val sentLines: SentLineService,
    val catalog: CatalogSource,
    val images: CatalogImages?,
    val activeWaiter: StateFlow<ActiveWaiter?>,
    /** Para llevar (etapa A2): el nombre y el teléfono por enviar y la lista. */
    val takeaway: TakeawayService,
)
