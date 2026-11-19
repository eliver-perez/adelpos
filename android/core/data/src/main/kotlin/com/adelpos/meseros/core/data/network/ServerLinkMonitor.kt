package com.adelpos.meseros.core.data.network

import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.connection.ServerAnswers
import com.adelpos.meseros.core.domain.connection.ServerLink
import com.adelpos.meseros.core.domain.connection.ServerLinkTracker
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

/** Cómo pregunta [ServerLinkMonitor] a Go si responde (`GET health/live`). */
fun interface ServerProbing {
    /**
     * Pregunta al servidor emparejado. La respuesta (o su falta) llega al estado por el punto único
     * del cliente HTTP ([ServerAnswers]). Devuelve false si no hay servidor emparejado al que
     * preguntar.
     *
     * @param relocate si no responde en su dirección, buscarlo en la red (cambio de IP): solo con
     *   «Sin conexión», porque la búsqueda tarda unos segundos.
     * @param quick el mesero está esperando (reintento en el momento): plazo de conexión corto y
     *   sin buscar en otra dirección.
     */
    suspend fun probe(relocate: Boolean, quick: Boolean): Boolean
}

/**
 * **El estado único de conexión** del proceso ([ServerLink], etapa A3, A3-D1), una instancia en
 * `AppContainer`. Junta la red de Android ([network]) y cada respuesta de Go ([ServerAnswers], que
 * le informa `PinnedHttpClient`) con las reglas de [ServerLinkTracker], y hace la revisión según el
 * uso con [probing]: ninguna con el canal de eventos abierto ([channelOpen]), cada 60 s sin él (y
 * se salta si hubo otro éxito), cada 5 s mientras duda, cada 10 s con «Sin conexión» y nada en
 * segundo plano ([foreground]).
 *
 * @param log una línea por cambio de estado y por falla (el registro `AdelLink` de la aplicación;
 *   lo lee la prueba de una hora sin uso).
 */
class ServerLinkMonitor(
    private val network: Flow<LinkState>,
    initialNetwork: LinkState,
    private val foreground: StateFlow<Boolean>,
    private val channelOpen: Flow<Boolean>,
    private val probing: ServerProbing,
    private val scope: CoroutineScope,
    clock: () -> Long,
    private val log: (String) -> Unit = {},
) : ServerLink, ServerAnswers {
    private val tracker = ServerLinkTracker(initialNetwork, clock)
    private val mutableState = MutableStateFlow(tracker.state)
    private val wakeups = Channel<Unit>(Channel.CONFLATED)
    private val probeLock = Mutex()

    @Volatile private var channelIsOpen = false
    private var started = false

    private val answeredCount = AtomicLong()
    private val noAnswerCount = AtomicLong()
    private val repeatedCount = AtomicLong()

    override val state: StateFlow<LinkState> = mutableState.asStateFlow()

    /** Respuestas de Go desde que arrancó el proceso (diagnóstico y pruebas). */
    val answers: Long
        get() = answeredCount.get()

    /** Peticiones que se quedaron sin respuesta (después de repetir una lectura). */
    val noAnswers: Long
        get() = noAnswerCount.get()

    /** Lecturas que fallaron sin respuesta y se repitieron con otra conexión. */
    val repeats: Long
        get() = repeatedCount.get()

    /** Empieza a seguir la red, el primer plano y el canal de eventos (una sola vez). */
    fun start() {
        if (started) {
            return
        }
        started = true
        scope.launch { network.distinctUntilChanged().collect(::onNetwork) }
        scope.launch { foreground.collect { wake() } }
        scope.launch {
            channelOpen.distinctUntilChanged().collect { open ->
                channelIsOpen = open
                wake()
            }
        }
        scope.launch { reviewLoop() }
    }

    /** La red de Android cambió (sin red: «Sin conexión» al instante). */
    fun onNetwork(link: LinkState) {
        change { onNetwork(link) }
        wake()
    }

    override fun answered() {
        answeredCount.incrementAndGet()
        change { onAnswer() }
    }

    override fun noAnswer() {
        noAnswerCount.incrementAndGet()
        log("sin respuesta de la computadora principal (${noAnswerCount.get()})")
        change { onNoAnswer() }
        // De la revisión de cada minuto a la sonda de cada 5 s.
        wake()
    }

    override fun repeated() {
        repeatedCount.incrementAndGet()
        log("una lectura sin respuesta se repitió con otra conexión (${repeatedCount.get()})")
    }

    override suspend fun confirm(): Boolean {
        if (tracker.state == LinkState.ONLINE) {
            return true
        }
        if (!tracker.hasNetwork) {
            return false
        }
        probeOnce(relocate = false, quick = true)
        return tracker.state == LinkState.ONLINE
    }

    private suspend fun reviewLoop() {
        while (true) {
            val wait = tracker.nextProbeIn(foreground.value, channelIsOpen)
            when {
                wait == null -> wakeups.receive()
                wait > 0 -> withTimeoutOrNull(wait) { wakeups.receive() }
                else -> probeOnce(relocate = tracker.state == LinkState.OFFLINE, quick = false)
            }
        }
    }

    private suspend fun probeOnce(relocate: Boolean, quick: Boolean) {
        probeLock.withLock {
            tracker.onProbe()
            val asked =
                try {
                    probing.probe(relocate, quick)
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (unexpected: RuntimeException) {
                    // Nunca detiene la revisión: cuenta como una sonda sin respuesta.
                    log("la sonda falló: ${unexpected.javaClass.simpleName}")
                    noAnswer()
                    true
                }
            if (!asked) {
                change { onNothingToProbe() }
            }
        }
    }

    private fun wake() {
        wakeups.trySend(Unit)
    }

    // Cambia el estado y lo publica juntos: dos hilos nunca publican fuera de orden.
    private fun change(block: ServerLinkTracker.() -> Unit) {
        synchronized(tracker) {
            val before = tracker.state
            tracker.block()
            val after = tracker.state
            mutableState.value = after
            if (after != before) {
                log(
                    if (after == LinkState.ONLINE) {
                        "estado: En línea"
                    } else {
                        "estado: Sin conexión"
                    },
                )
            }
        }
    }
}
