package com.adelpos.meseros.core.domain.realtime

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class RealtimeTest {
    @Test
    fun esperaCrecienteHastaElMaximo() {
        val backoff = ReconnectBackoff(initialMillis = 1_000, maximumMillis = 30_000)
        assertEquals(0L, backoff.delayFor(0))
        assertEquals(
            listOf(1_000L, 2_000L, 4_000L, 8_000L, 16_000L, 30_000L, 30_000L),
            (1..7).map { backoff.delayFor(it) },
        )
        assertEquals(30_000L, backoff.delayFor(10_000))
    }

    private class FakeFeed : RealtimeFeed {
        val mutable = MutableSharedFlow<RealtimeSignal>()
        override val signals: SharedFlow<RealtimeSignal> = mutable
        override val state: StateFlow<RealtimeState> = MutableStateFlow(RealtimeState.Stopped)

        override fun reconnectNow() = Unit
    }

    private fun event(sequence: Long, kind: String, scope: String) =
        RealtimeEvent(sequence, "id-$sequence", 1, kind, scope, null, null, null)

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun losEventosSeFiltranPorAmbitoYTipo() =
        runTest(UnconfinedTestDispatcher()) {
            val feed = FakeFeed()
            val catalog = mutableListOf<RealtimeEvent>()
            val preparation = mutableListOf<RealtimeEvent>()
            val rebuilds = mutableListOf<RealtimeSignal.Rebuild>()
            val jobs =
                listOf(
                    launch { feed.events(RealtimeScopes.CATALOG).take(2).toList(catalog) },
                    launch {
                        feed
                            .events(
                                RealtimeScopes.RESTAURANT,
                                setOf(RealtimeKinds.ACCOUNT_PREPARATION_CHANGED),
                            )
                            .take(1)
                            .toList(preparation)
                    },
                    launch { feed.rebuilds().take(1).toList(rebuilds) },
                )
            feed.mutable.emit(
                RealtimeSignal.Changes(
                    cursor = 3,
                    events =
                        listOf(
                            event(1, RealtimeKinds.CATALOG_CHANGED, RealtimeScopes.CATALOG),
                            event(2, "restaurant.draft.saved", RealtimeScopes.RESTAURANT),
                            event(
                                3,
                                RealtimeKinds.ACCOUNT_PREPARATION_CHANGED,
                                RealtimeScopes.RESTAURANT,
                            ),
                        ),
                ),
            )
            feed.mutable.emit(RealtimeSignal.Rebuild(9, RebuildReason.SERVER_RESET))
            feed.mutable.emit(
                RealtimeSignal.Changes(
                    10,
                    listOf(event(10, RealtimeKinds.PRODUCT_SOLD_OUT, RealtimeScopes.CATALOG)),
                ),
            )
            jobs.forEach { it.join() }
            assertEquals(listOf(1L, 10L), catalog.map { it.sequence })
            assertEquals(listOf(3L), preparation.map { it.sequence })
            assertEquals(listOf(RealtimeSignal.Rebuild(9, RebuildReason.SERVER_RESET)), rebuilds)
        }
}
