package com.adelpos.meseros

import android.os.SystemClock
import com.adelpos.meseros.core.data.waiter.CaptureRepository
import com.adelpos.meseros.core.data.waiter.OrderQueueRepository
import com.adelpos.meseros.core.data.waiter.OrderSender
import com.adelpos.meseros.core.data.waiter.OrdersInFlight
import com.adelpos.meseros.core.data.waiter.RestaurantRepository
import com.adelpos.meseros.core.data.waiter.SentLineRepository
import com.adelpos.meseros.core.data.waiter.ServerClock
import com.adelpos.meseros.core.data.waiter.WaiterApi
import com.adelpos.meseros.core.data.waiter.WaiterServer
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.realtime.RealtimeFeed
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * El modo Mesero (Fase E), una instancia por proceso: mesas y cuentas ([restaurant]), capturas
 * ([captures]), la cola ([queue], creada antes en `AppContainer` porque la entrada al dispositivo
 * la necesita) y modificar o cancelar lo enviado ([sentLines]). Todo habla con Go con las cabeceras
 * del mesero dueño de cada operación ([server]).
 *
 * [start] (desde `AdelPosApplication`) recupera la cola después de un reinicio y pide envíos al
 * volver la red, al volver a primer plano y cuando un mesero vuelve a entrar con conexión.
 */
class WaiterModeContainer(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    val queue: OrderQueueRepository,
    private val serverClock: ServerClock,
    private val inFlight: OrdersInFlight,
    private val catalog: CatalogSource,
    realtime: RealtimeFeed,
    private val linkState: Flow<LinkState>,
    private val foreground: StateFlow<Boolean>,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var started = false

    /** El catálogo guardado para los pedidos: nombre y precio de referencia que vio el mesero. */
    private val lookups =
        object : CaptureRepository.CatalogLookups {
            override val lookup = OrderFactory.CatalogLookup(::describe)

            override fun stamp(): OrderFactory.CatalogStamp {
                val version = catalog.catalog.value.version
                return OrderFactory.CatalogStamp(
                    version = version?.version ?: 0L,
                    epoch = version?.epoch,
                )
            }
        }

    val sender: OrderSender =
        OrderSender(dao, server, api, inFlight = inFlight, serverClock = serverClock)

    val captures: CaptureRepository =
        CaptureRepository(
            dao = dao,
            server = server,
            api = api,
            scheduler = queue::retryNow,
            catalog = lookups,
            scope = scope,
            serverClock = serverClock,
            inFlight = inFlight,
        )

    val restaurant: RestaurantRepository =
        RestaurantRepository(
            dao = dao,
            server = server,
            api = api,
            realtime = realtime,
            scope = scope,
            elapsedClock = SystemClock::elapsedRealtime,
        )

    val sentLines: SentLineRepository = SentLineRepository(server, api)

    fun start() {
        if (started) {
            return
        }
        started = true
        captures.start()
        restaurant.start()
        scope.launch {
            // Lo que quedó «Enviando» al cerrarse el proceso vuelve a la cola con su clave.
            queue.recoverAfterRestart()
            queue.retryNow()
        }
        scope.launch {
            // Al volver la red: la cola y las capturas del mesero activo.
            linkState
                .map { it == LinkState.ONLINE }
                .distinctUntilChanged()
                .drop(1)
                .filter { it }
                .collect {
                    queue.retryNow()
                    server.activeWaiter.value?.let { captures.resume(it.userId) }
                }
        }
        scope.launch { foreground.filter { it }.collect { queue.retryNow() } }
        scope.launch {
            // Un mesero que entra con conexión trae una credencial de envío nueva: sus pedidos
            // detenidos por OFFLINE_CREDENTIAL_INVALID vuelven a intentarse.
            server.activeWaiter
                .map { waiter ->
                    waiter?.takeIf { it.hasServerSession && !it.mustReenterPin }?.userId
                }
                .distinctUntilChanged()
                .collect { userId ->
                    if (userId != null) {
                        queue.credentialRenewed(userId)
                        captures.resume(userId)
                    }
                }
        }
    }

    private fun describe(line: LineSelection): OrderFactory.Described? {
        val product = catalog.catalog.value.product(line.productId) ?: return null
        val variant = product.variants.firstOrNull { it.id == line.variantId }
        return OrderFactory.Described(
            name =
                if (variant == null) {
                    product.name
                } else {
                    "${product.name} ${variant.name}"
                },
            referencePrice = variant?.referencePrice ?: product.referencePrice,
        )
    }
}
