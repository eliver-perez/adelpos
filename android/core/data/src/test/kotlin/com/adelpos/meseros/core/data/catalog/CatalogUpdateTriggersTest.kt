package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.data.session.AuthorizedServer
import com.adelpos.meseros.core.data.session.FixedServerAccess
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.realtime.AccessInterruption
import com.adelpos.meseros.core.domain.realtime.RealtimeEvent
import com.adelpos.meseros.core.domain.realtime.RealtimeFeed
import com.adelpos.meseros.core.domain.realtime.RealtimeSignal
import com.adelpos.meseros.core.domain.realtime.RealtimeState
import com.adelpos.meseros.core.domain.realtime.RebuildReason
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Cuándo se pide actualizar el catálogo (§5): al abrir la aplicación, con sesión nueva, al
 * recuperar la conexión, con los eventos del ámbito `catalog` y periódicamente en primer plano.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CatalogUpdateTriggersTest {
    private class FakeFeed : RealtimeFeed {
        val flow = MutableSharedFlow<RealtimeSignal>()
        var reconnections = 0
        override val signals: SharedFlow<RealtimeSignal> = flow
        override val state: StateFlow<RealtimeState> = MutableStateFlow(RealtimeState.Stopped)

        override fun reconnectNow() {
            reconnections += 1
        }
    }

    private fun event(scope: String, kind: String) =
        RealtimeEvent(1, "e1", 1, kind, scope, null, null, null)

    @Test
    fun cadaMotivoPideUnaActualizacion() = runTest {
        val access = FixedServerAccess()
        val foreground = MutableStateFlow(false)
        val link = MutableStateFlow(LinkState.ONLINE)
        val feed = FakeFeed()
        var requests = 0
        CatalogUpdateTriggers(
                request = { requests += 1 },
                access = access,
                foreground = foreground,
                link = link,
                realtime = feed,
                periodMillis = 60_000,
            )
            .start(backgroundScope)
        runCurrent()
        assertEquals(0, requests)

        // Un mesero entra (hay sesión).
        access.set(
            AuthorizedServer(
                baseUrl = "http://127.0.0.1/".toHttpUrl(),
                client = OkHttpClient(),
                credentials = AuthorizedServer.credentials("dispositivo", "token", "sesion"),
            ),
        )
        runCurrent()
        assertEquals(1, requests)

        // La aplicación pasa a primer plano.
        foreground.value = true
        runCurrent()
        assertEquals(2, requests)

        // Se pierde y se recupera la conexión: también se pide reconectar el canal ya.
        link.value = LinkState.OFFLINE
        runCurrent()
        assertEquals(2, requests)
        link.value = LinkState.ONLINE
        runCurrent()
        assertEquals(3, requests)
        assertEquals(1, feed.reconnections)

        // Eventos del ámbito catalog y avisos de reconstruir; los de otros ámbitos no.
        feed.flow.emit(
            RealtimeSignal.Changes(5, listOf(event("restaurant", "restaurant.draft.saved"))),
        )
        runCurrent()
        assertEquals(3, requests)
        feed.flow.emit(RealtimeSignal.Changes(6, listOf(event("catalog", "catalog.changed"))))
        feed.flow.emit(RealtimeSignal.Rebuild(7, RebuildReason.SERVER_RESET))
        feed.flow.emit(
            RealtimeSignal.Interrupted(AccessInterruption.SessionEnded("SESSION_EXPIRED")),
        )
        runCurrent()
        assertEquals(5, requests)

        // Periódicamente mientras está en uso, y nunca en segundo plano.
        advanceTimeBy(60_001)
        assertEquals(6, requests)
        foreground.value = false
        runCurrent()
        advanceTimeBy(180_000)
        assertEquals(6, requests)
    }
}
