package com.adelpos.meseros.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.error
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.json
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import mockwebserver3.SocketEffect
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3, §4.10: las cuentas del panel de la mesa con `discardable` de Go y «Descartar cuenta
 * vacía» (`POST restaurant/accounts/{id}/discard`, con conexión y la sesión del mesero, nunca en la
 * cola), con Room (Robolectric) y un Go simulado por ruta.
 */
@RunWith(AndroidJUnit4::class)
class TablePanelAccountsTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private lateinit var restaurant: RestaurantRepository
    private val ids = AtomicInteger(0)
    private val routes = mutableMapOf<String, ArrayDeque<() -> MockResponse>>()
    private val received = ConcurrentLinkedQueue<RecordedRequest>()

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    received.add(request)
                    val key =
                        "${request.method} ${request.url.encodedPath.removePrefix("/api/v1/")}"
                    val next = synchronized(routes) { routes[key]?.removeFirstOrNull() }
                    return next?.invoke() ?: error(500, "SIN_RESPUESTA")
                }
            }
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        restaurant =
            RestaurantRepository(
                dao = database.waiter(),
                server = server,
                api = WaiterApi(),
                realtime = null,
                scope = scope,
                elapsedClock = { 50_000L },
                newId = { "00000000-0000-4000-8000-%012d".format(ids.incrementAndGet()) },
            )
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private fun respond(key: String, response: () -> MockResponse) {
        synchronized(routes) { routes.getOrPut(key) { ArrayDeque() }.add(response) }
    }

    private fun respond(key: String, response: MockResponse) = respond(key) { response }

    private fun requests(key: String): List<RecordedRequest> = received.filter {
        "${it.method} ${it.url.encodedPath.removePrefix("/api/v1/")}" == key
    }

    private fun stateWith(vararg accounts: String) =
        json(
            200,
            """{"catalog_version":41,"user":{"id":"$ANA","permissions":["accounts.read",""" +
                """"accounts.open"]},"accounts":[${accounts.joinToString(",")}],""" +
                """"event_cursor":9}""",
        )

    private fun account(id: String, folio: Int, discardable: Boolean?) =
        """{"id":"$id","folio":$folio,"name":"Mesa 6","mode":"dine_in","state":"open",""" +
            """"table_id":"t6","table_name":"Mesa 6","revision":3""" +
            (discardable?.let { ""","discardable":$it""" } ?: "") +
            "}"

    @Test
    fun laCuentaVaciaSeDescartaSoloComoLoDiceGo() = runBlocking {
        respond(
            "GET restaurant/state",
            stateWith(
                account(EMPTY, 12, discardable = true),
                account(FULL, 13, discardable = false),
                // Un servidor que no lo informa: nunca se ofrece descartar.
                account(OLD, 14, discardable = null),
            ),
        )
        respond("GET restaurant/overview", json(200, """{"areas":[],"tables":[]}"""))
        repeat(4) {
            respond("GET restaurant/state", stateWith(account(FULL, 13, discardable = false)))
            respond("GET restaurant/overview", json(200, """{"areas":[],"tables":[]}"""))
        }
        restaurant.start()
        server.sessions[ANA] = "sesion-de-ana"
        server.active.value =
            ActiveWaiter(WaiterProfile(ANA, "Ana", "A", null, 1, null), hasServerSession = true)
        val accounts = withTimeout(5_000) { restaurant.openAccounts.first { it.size == 3 } }
        val empty = accounts.single { it.id == EMPTY }
        assertTrue(empty.discardable)
        assertFalse(accounts.single { it.id == FULL }.discardable)
        assertFalse(accounts.single { it.id == OLD }.discardable)

        val path = "POST restaurant/accounts/$EMPTY/discard"
        respond(path, json(200, """{"account":${account(EMPTY, 12, discardable = false)}}"""))
        assertEquals(DiscardAccountResult.Discarded, restaurant.discardEmptyAccount(empty))
        val body = Json.parseToJsonElement(requests(path).single().body!!.utf8()).jsonObject
        assertTrue(body.containsKey("operation_id"))
        assertEquals("3", body.getValue("revision").jsonPrimitive.content)

        // Otra terminal le agregó algo: se vuelve a leer y se ve como está.
        respond(path, error(409, "ACCOUNT_NOT_EMPTY"))
        assertEquals(DiscardAccountResult.Changed, restaurant.discardEmptyAccount(empty))
        respond(path, error(409, "ACCOUNT_CONFLICT"))
        assertEquals(DiscardAccountResult.Changed, restaurant.discardEmptyAccount(empty))

        // Sin respuesta se repite igual (misma clave) una vez.
        respond(path) {
            MockResponse.Builder().onResponseStart(SocketEffect.ShutdownConnection).build()
        }
        respond(path, json(200, """{"account":${account(EMPTY, 12, discardable = false)}}"""))
        assertEquals(DiscardAccountResult.Discarded, restaurant.discardEmptyAccount(empty))
        val keys =
            requests(path).takeLast(2).map {
                Json.parseToJsonElement(it.body!!.utf8())
                    .jsonObject
                    .getValue("operation_id")
                    .jsonPrimitive
                    .content
            }
        assertEquals(keys[0], keys[1])

        // Sin sesión de Go: nunca va a la cola, se dice que hace falta conexión.
        server.sessions.clear()
        val before = requests(path).size
        assertEquals(DiscardAccountResult.NoConnection, restaurant.discardEmptyAccount(empty))
        assertEquals(before, requests(path).size)
    }

    private companion object {
        const val EMPTY = "aaaaaaaa-0000-4000-8000-00000000000c"
        const val FULL = "aaaaaaaa-0000-4000-8000-00000000000d"
        const val OLD = "aaaaaaaa-0000-4000-8000-00000000000e"
    }
}
