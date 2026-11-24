package com.adelpos.meseros.core.data.license

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.waiter.FakeWaiterServer
import com.adelpos.meseros.core.data.waiter.RestaurantRepository
import com.adelpos.meseros.core.data.waiter.WaiterApi
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.ANA
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.error
import com.adelpos.meseros.core.data.waiter.WaiterTestSupport.json
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.license.LicenseNotices
import com.adelpos.meseros.core.domain.license.LicenseStateName
import com.adelpos.meseros.core.domain.license.LicenseSummary
import com.adelpos.meseros.core.domain.realtime.RealtimeEvent
import com.adelpos.meseros.core.domain.realtime.RealtimeFeed
import com.adelpos.meseros.core.domain.realtime.RealtimeSignal
import com.adelpos.meseros.core.domain.realtime.RealtimeState
import java.util.concurrent.ConcurrentLinkedQueue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa L1 (§6.2 y §8): el estado de la licencia y de la hora llega en `license` de
 * `restaurant/state` (Room con Robolectric y un Go simulado) y el evento `license.changed` del
 * canal hace volver a leerlo: así el aviso fijo de la hora aparece y se quita, y el de la
 * tolerancia dice los días que calculó Go. Un servidor anterior a la etapa no lo envía: sin aviso.
 */
@RunWith(AndroidJUnit4::class)
class LicenseStatusTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: AppDatabase
    private lateinit var server: FakeWaiterServer
    private lateinit var scope: CoroutineScope
    private val feed = FakeFeed()
    private val license = LicenseStatusRepository()
    private val states = ConcurrentLinkedQueue<String>()
    private val reads = ConcurrentLinkedQueue<RecordedRequest>()

    private class FakeFeed : RealtimeFeed {
        val flow = MutableSharedFlow<RealtimeSignal>()
        override val signals: SharedFlow<RealtimeSignal> = flow
        override val state: StateFlow<RealtimeState> = MutableStateFlow(RealtimeState.Stopped)

        override fun reconnectNow() = Unit
    }

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    val path = request.url.encodedPath.removePrefix("/api/v1/")
                    return when (path) {
                        "restaurant/state" -> {
                            reads.add(request)
                            // El último estado se repite si se lee más veces.
                            val next = if (states.size > 1) states.poll() else states.peek()
                            json(200, next ?: stateWith(null))
                        }
                        "restaurant/overview" -> json(200, """{"areas":[],"tables":[]}""")
                        else -> error(500, "SIN_RESPUESTA")
                    }
                }
            }
        mock.start()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        database = AppDatabase.open(context)
        server = FakeWaiterServer(mock)
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        context.deleteDatabase(AppDatabase.FILE_NAME)
        mock.close()
    }

    private fun stateWith(license: String?): String =
        """{"catalog_version":41,"user":{"id":"$ANA","permissions":["accounts.read"]},""" +
            """"accounts":[],"event_cursor":9""" +
            (license?.let { ""","license":$it""" } ?: "") +
            "}"

    private fun summary(state: String, graceDays: Int?, blocked: Boolean): String =
        """{"state":"$state","environment":"test","copied":false,"machine_unverified":false,""" +
            """"revocation_reason":null,"license_type":"subscription","perpetual":false,""" +
            """"expires_at":"2026-11-21T00:00:00Z","grace_days":5,""" +
            """"grace_ends_at":"2026-11-26T00:00:00Z","days_until_expiry":null,""" +
            """"grace_days_remaining":${graceDays ?: "null"},""" +
            """"clock":{"blocked":$blocked,"local_time":"2026-11-23T16:15:00Z",""" +
            """"last_activity_time":"2026-11-23T18:40:00Z","margin_minutes":10,""" +
            """"timezone":"America/Monterrey","local_time_text":"lunes 23 nov 2026, 10:15",""" +
            """"last_activity_time_text":"lunes 23 nov 2026, 12:40",""" +
            """"license_state":"$state","license_ok":true}}"""

    private fun repository() =
        RestaurantRepository(
            dao = database.waiter(),
            server = server,
            api = WaiterApi(),
            realtime = feed,
            scope = scope,
            elapsedClock = { 50_000L },
            license = license,
        )

    private fun enter() {
        server.sessions[ANA] = "sesion-de-ana"
        server.active.value =
            ActiveWaiter(WaiterProfile(ANA, "Ana", "A", null, 1, null), hasServerSession = true)
    }

    private suspend fun awaitSummary(condition: (LicenseSummary?) -> Boolean): LicenseSummary? =
        withTimeout(5_000) { license.summary.first(condition) }

    @Test
    fun elAvisoDeLaHoraApareceYSeQuitaConElEvento() = runBlocking {
        states.add(stateWith(summary("grace", graceDays = 3, blocked = true)))
        states.add(stateWith(summary("grace", graceDays = 3, blocked = false)))
        repository().start()
        enter()
        val blocked = awaitSummary { it?.clock?.blocked == true }
        val clock = LicenseNotices.clock(blocked)!!
        assertEquals(LicenseMessages.CLOCK_INVALID, clock.text)
        assertEquals(
            "La computadora principal tiene la hora lunes 23 nov 2026, 10:15 y la última actividad " +
                "registrada fue el lunes 23 nov 2026, 12:40.",
            clock.detail,
        )
        assertEquals("Licencia: quedan 3 días", LicenseNotices.grace(blocked)!!.short)
        assertEquals(1, reads.size)

        // Go corrigió la hora y lo avisa: se vuelve a leer y el aviso fijo se quita.
        feed.flow.emit(
            RealtimeSignal.Changes(
                cursor = 10,
                events =
                    listOf(
                        RealtimeEvent(
                            10,
                            "e10",
                            1,
                            "license.changed",
                            "configuration",
                            null,
                            null,
                            null,
                        ),
                    ),
            ),
        )
        val fixed = awaitSummary { it?.clock?.blocked == false }
        assertNull(LicenseNotices.clock(fixed))
        assertEquals(LicenseStateName.GRACE, fixed!!.state)
        assertEquals(2, reads.size)
    }

    @Test
    fun unServidorAnteriorNoLoEnviaYNoHayAviso() = runBlocking {
        states.add(stateWith(null))
        val restaurant = repository()
        // Algo que quedó de antes (otro momento): la lectura sin `license` lo quita.
        license.apply(
            LicenseSummaryDto(
                state = "grace",
                graceDaysRemaining = 2,
                clock = LicenseClockDto(true),
            ),
        )
        restaurant.start()
        enter()
        val read = awaitSummary { it == null }
        assertNull(LicenseNotices.clock(read))
        assertNull(LicenseNotices.grace(read))
        // Y al borrar los datos del dispositivo se olvida.
        license.apply(LicenseSummaryDto(state = "active"))
        license.forget()
        assertNull(license.summary.value)
    }
}
