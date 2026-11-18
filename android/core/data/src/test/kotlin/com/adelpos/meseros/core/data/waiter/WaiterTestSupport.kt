package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.session.ServerTransport
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderFactory
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Headers
import okhttp3.OkHttpClient

/** Utilidades de las pruebas del modo Mesero contra MockWebServer (el «Go» simulado). */
object WaiterTestSupport {
    const val DEVICE_ID = "0b5d2f6c-6b1a-4c4b-9b70-3f0f8e1c2a10"
    const val ANA = "11111111-1111-4111-8111-111111111111"
    const val BETO = "22222222-2222-4222-8222-222222222222"
    const val TABLE = "33333333-3333-4333-8333-333333333333"
    const val ACCOUNT = "44444444-4444-4444-8444-444444444444"
    const val BURGER = "55555555-5555-4555-8555-555555555555"
    const val SHAKE = "66666666-6666-4666-8666-666666666666"

    fun line(productId: String, quantity: Int = 1) =
        LineSelection(productId, null, quantity, Customization.ORIGINAL)

    fun capture(
        id: String,
        userId: String = ANA,
        accountId: String? = ACCOUNT,
        base: List<DraftLine> = emptyList(),
        operations: List<DraftOperation> = emptyList(),
        own: Set<String> = emptySet(),
    ) =
        Capture(
            id = id,
            waiterUserId = userId,
            tableId = TABLE,
            tableName = "Mesa 4",
            accountId = accountId,
            accountName = accountId?.let { "Mesa 4" },
            seenAccountIds = emptyList(),
            base = base,
            baseRevision = 2,
            operations = operations,
            ownItemIds = own,
            seenPrices = emptyMap(),
            sync = DraftSync.SAVED,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = 1_000L,
        )

    val lookup = OrderFactory.CatalogLookup { selection ->
        when (selection.productId) {
            BURGER -> OrderFactory.Described("Hamburguesa", Money("120.00"))
            SHAKE -> OrderFactory.Described("Malteada de fresa", Money("65.00"))
            else -> null
        }
    }

    /** Un pedido «Por enviar» de [userId] con una hamburguesa nueva (y lo que se indique). */
    fun queuedOrder(
        operationId: String,
        userId: String = ANA,
        accountId: String? = ACCOUNT,
        base: List<DraftLine> = emptyList(),
        own: Set<String> = emptySet(),
    ): PendingOrder {
        val itemId = "item-$operationId"
        val capture =
            capture(
                id = operationId,
                userId = userId,
                accountId = accountId,
                base = base,
                operations = listOf(DraftOperation.Add(DraftLine(itemId, line(BURGER, 2)))),
                own = own + itemId,
            )
        return requireNotNull(
            OrderFactory.fromCapture(
                capture = capture,
                waiterName = if (userId == ANA) "Ana" else "Beto",
                operationId = operationId,
                catalog = OrderFactory.CatalogStamp(41, "epoca-1"),
                lookup = lookup,
                onlineConfirm = null,
                nowMillis = 1_700_000_000_000L,
            ),
        )
    }

    fun json(status: Int, body: String): MockResponse =
        MockResponse.Builder()
            .code(status)
            .addHeader("Content-Type", "application/json; charset=utf-8")
            .body(body)
            .build()

    fun error(status: Int, code: String, details: String? = null): MockResponse {
        val detailsPart = details?.let { ""","details":$it""" }.orEmpty()
        return json(
            status,
            """{"error":{"code":"$code","message":"Mensaje de Go","request_id":"r1"$detailsPart}}""",
        )
    }

    fun accepted(accountId: String = ACCOUNT): MockResponse =
        json(
            200,
            """{"status":"accepted","operation_id":"x","order_id":"o1","account_id":"$accountId",""" +
                """"catalog_version":41,"captured_catalog_version":41,""" +
                """"captured_at":"2026-11-16T20:00:00Z","received_at":"2026-11-16T20:01:00Z",""" +
                """"accepted_changes":[],"bill":{}}""",
        )
}

/** El acceso al servidor simulado, con las credenciales de cada mesero. */
class FakeWaiterServer(private val mock: MockWebServer) : WaiterServer {
    /** El mesero que usa el dispositivo (las pruebas de las lecturas lo cambian). */
    val active = MutableStateFlow<ActiveWaiter?>(null)
    val sessions = mutableMapOf<String, String>()
    val credentials = mutableMapOf<String, String>()
    val reports = mutableListOf<Pair<AppError, String>>()
    var paired = true

    override val activeWaiter: StateFlow<ActiveWaiter?> = active

    override fun transport(): ServerTransport? =
        if (paired) {
            ServerTransport(mock.url("/"), OkHttpClient())
        } else {
            null
        }

    override suspend fun prepare() = Unit

    override fun sessionHeaders(userId: String): Headers? {
        val session = sessions[userId] ?: return null
        return Headers.headersOf(
            "X-Device-ID",
            WaiterTestSupport.DEVICE_ID,
            "X-Device-Token",
            "token",
            "Authorization",
            "Bearer $session",
        )
    }

    override suspend fun offlineHeaders(userId: String): Headers? {
        val credential = credentials[userId] ?: return null
        return Headers.headersOf(
            "X-Device-ID",
            WaiterTestSupport.DEVICE_ID,
            "X-Device-Token",
            "token",
            DeviceWaiterServer.OFFLINE_CREDENTIAL_HEADER,
            credential,
        )
    }

    override fun report(error: AppError, userId: String) {
        reports.add(error to userId)
    }
}
