package com.adelpos.cocina.board

import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.realtime.RealtimeSignal
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.session.AuthorizedServer
import com.adelpos.core.network.session.FixedServerAccess
import com.adelpos.core.network.session.ServerAccess
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Lo que la pantalla de pedidos necesita de la entrada al dispositivo (`KitchenAccess`). */
interface BoardAccess {
    val status: StateFlow<KitchenStatus>

    /** Un rechazo del dispositivo o de la versión: revocada, otra huella, 426… */
    fun report(error: AppError)

    /** La conexión para el canal de eventos (la credencial del dispositivo); null sin emparejar. */
    fun channelServer(): AuthorizedServer?
}

/**
 * Quien hace sonar las alertas (§5.2): en la aplicación, `ToneAlertPlayer`; las pruebas lo anotan.
 */
fun interface BoardAlertSink {
    /** [volume]: «Volumen» de Terminales, `low`, `medium` o `high`. */
    fun play(sounds: Set<AlertSound>, volume: String)
}

/** Lo que se puede hacer desde la pantalla de pedidos (las pruebas de pantallas lo sustituyen). */
interface BoardActions {
    fun markDish(cardId: String, dishId: String)

    fun markOrder(cardId: String)

    fun openRecent()

    fun closeRecent()

    fun recover(tasks: List<TaskRef>)

    fun takeSeat()

    fun dismissNotice()
}

/** Lo que se muestra de la pantalla de pedidos. */
data class BoardState(
    val screen: BoardScreen = BoardScreen.Connecting,
    val board: Board? = null,
    val settings: DisplaySettings = DisplaySettings.DEFAULT,
    /**
     * Platillos tocados cuya marca va en camino o ya respondió y la siguiente lectura todavía no
     * trae: se ven ya listos (atenuados, con la palomita), la confirmación visual del toque.
     */
    val markingDishes: Set<String> = emptySet(),
    /** Tarjetas cuyo botón se tocó, igual: se esconden al momento. */
    val markingCards: Set<String> = emptySet(),
    /** Go dijo `SYSTEM_CLOCK_INVALID` al marcar (además de `license.clock.blocked`). */
    val clockInvalid: Boolean = false,
    /** Un aviso pasajero con el mensaje de Go (por ejemplo, un platillo que ya no se recupera). */
    val notice: String? = null,
    /**
     * «Sin conexión» del estado único de conexión (§5.1): el aviso grande arriba y no se puede
     * marcar. Dentro de la tolerancia (y del margen sin red de la cocina, K1-D33) sigue en false.
     */
    val offline: Boolean = false,
    /**
     * Las tarjetas con algo nuevo o cambiado y desde cuándo (reloj monotónico de la tableta): la
     * animación breve de la tarjeta nueva (§5.2), una vez por cada valor.
     */
    val freshCards: Map<String, Long> = emptyMap(),
) {
    /** La hora del servidor está mal (K1-D16): no se puede marcar hasta corregirla. */
    val clockBlocked: Boolean
        get() = clockInvalid || !BoardRules.markingAllowed(board?.license)

    /** Si se puede marcar ahora: con conexión y con la hora del servidor correcta. */
    val markingAllowed: Boolean
        get() = !offline && !clockBlocked
}

/** «Recientes»: lo marcado listo en los últimos 30 minutos, para regresarlo a la pantalla. */
sealed interface RecentState {
    data object Hidden : RecentState

    data object Loading : RecentState

    /** [message]: el aviso de Go de un platillo que ya no se puede regresar, si lo hubo. */
    data class Shown(val orders: List<RecentOrder>, val message: String?) : RecentState
}

/**
 * La pantalla de pedidos frente a Go (§4 de la etapa K1, el orden de arranque del contrato de la
 * Fase A y K1-D25), mientras la pantalla está emparejada ([BoardAccess.status] en `Ready`):
 * 1. `kitchen-display/connect` (ocupa el cupo de cocina; sin cupo, cupo liberado o sin activar, su
 *    pantalla);
 * 2. `kitchen-display/orders`: el estado completo con su cursor, que **reemplaza** todo lo que se
 *    muestra;
 * 3. el canal de eventos (`RealtimeClient` de android-core con la credencial del dispositivo,
 *    [channelAccess]): con **cualquier** evento o `reset`, volver a leer `orders`, en una sola
 *    lectura aunque lleguen varios (una en curso y como mucho una pendiente);
 * 4. después de cada lectura, el acuse de recepción de las comandas que ya se muestran (K1-D12).
 *
 * Las marcas, «Recientes» y «Recuperar» vuelven a leer al terminar. Sin respuesta, una lectura se
 * vuelve a intentar en silencio ([retryMillis]).
 *
 * **Conexión tolerante** (§5.1, la Fase D): manda el estado único de conexión ([link], el de la
 * barra). Mientras dice «En línea» (también dentro de la tolerancia) no se ve ningún aviso y una
 * marca sin respuesta se repite sola cada [markRetryMillis], con su palomita a la vista (es
 * idempotente por versión: lo que Go ya tenía vuelve en `skipped`). Con «Sin conexión»: aviso
 * grande, no se puede marcar, las marcas pendientes esperan y, al volver, se repiten y se lee todo
 * de nuevo.
 *
 * **Alertas** (§5.2): cada lectura se compara con la anterior ([BoardAlerts]); lo nuevo suena por
 * [alerts] (si «Sonido» está activado en Terminales) y su tarjeta se anima.
 *
 * @param monotonic el reloj monotónico de la tableta (milisegundos): la espera avanza con él desde
 *   el `server_time` de cada lectura.
 */
class KitchenBoard(
    private val access: BoardAccess,
    private val service: KitchenDisplayService,
    private val signals: Flow<RealtimeSignal>,
    private val monotonic: () -> Long,
    private val link: StateFlow<LinkState> = MutableStateFlow(LinkState.ONLINE),
    private val alerts: BoardAlertSink = BoardAlertSink { _, _ -> },
    private val retryMillis: Long = RETRY_MILLIS,
    private val noSeatRetryMillis: Long = NO_SEAT_RETRY_MILLIS,
    private val markRetryMillis: Long = MARK_RETRY_MILLIS,
) : BoardActions {
    private val mutableState = MutableStateFlow(BoardState())
    private val mutableRecent = MutableStateFlow<RecentState>(RecentState.Hidden)
    private val channel = FixedServerAccess()

    val state: StateFlow<BoardState> = mutableState.asStateFlow()

    val recent: StateFlow<RecentState> = mutableRecent.asStateFlow()

    /** La conexión del canal de eventos: la del dispositivo mientras la pantalla está activa. */
    val channelAccess: ServerAccess = channel

    // Pedidos de lectura del ciclo en curso: conflado = una en curso y como mucho una pendiente.
    @Volatile private var reads: Channel<Unit>? = null
    @Volatile private var needsConnect = true
    @Volatile private var takeSeatNext = false
    private var scope: CoroutineScope? = null
    private var retryJob: Job? = null

    // Las marcas cuya respuesta todavía no llega: la siguiente lectura no las quita de la vista.
    private val dishesInFlight: MutableSet<String> = ConcurrentHashMap.newKeySet()
    private val cardsInFlight: MutableSet<String> = ConcurrentHashMap.newKeySet()

    // Las marcas ya respondidas siguen a la vista hasta una lectura **empezada después** de la
    // respuesta (el número de lectura que la confirma): una lectura que ya iba en camino trae lo de
    // antes de marcar y haría parpadear el platillo (listo → pendiente → listo).
    private val readsStarted = AtomicLong(0)
    private val dishesConfirmed = ConcurrentHashMap<String, Long>()
    private val cardsConfirmed = ConcurrentHashMap<String, Long>()

    /** Una marca de listo: lo que se manda y lo que se ve marcado mientras va en camino. */
    private class PendingMark(
        val tasks: List<TaskRef>,
        val wholeOrder: Boolean,
        val dishIds: Set<String>,
        val cardIds: Set<String>,
    )

    // Marcas sin respuesta que se repiten solas (§5.1) y el aviso para repetirlas.
    private val retrying = ConcurrentLinkedQueue<PendingMark>()
    @Volatile private var markRetries: Channel<Unit>? = null

    // Tareas que esta pantalla acaba de recuperar y cuándo: regresarlas no suena como pedido nuevo.
    private val recoveredTasks = ConcurrentHashMap<String, Long>()

    /** Empieza a seguir el estado de la entrada en [scope] (una sola vez por proceso). */
    fun start(scope: CoroutineScope): Job {
        this.scope = scope
        scope.launch { link.collect(::onLink) }
        return scope.launch {
            access.status
                .map { it is KitchenStatus.Ready }
                .distinctUntilChanged()
                .collectLatest { ready ->
                    if (ready) {
                        run()
                    } else {
                        channel.set(null)
                        reads = null
                        markRetries = null
                        retrying.clear()
                        dishesInFlight.clear()
                        cardsInFlight.clear()
                        dishesConfirmed.clear()
                        cardsConfirmed.clear()
                        recoveredTasks.clear()
                        mutableState.value = BoardState(offline = link.value == LinkState.OFFLINE)
                        mutableRecent.value = RecentState.Hidden
                    }
                }
        }
    }

    // El estado único de conexión cambió. Al volver de «Sin conexión»: lectura completa y las
    // marcas
    // que esperaban se repiten.
    private fun onLink(state: LinkState) {
        val offline = state == LinkState.OFFLINE
        val wasOffline = mutableState.value.offline
        mutableState.update { it.copy(offline = offline) }
        if (wasOffline && !offline) {
            requestRead()
            markRetries?.trySend(Unit)
        }
    }

    private suspend fun run() = coroutineScope {
        val requests = Channel<Unit>(Channel.CONFLATED)
        reads = requests
        needsConnect = true
        takeSeatNext = false
        // El canal se abre desde ya (sin cupo también: trae license.changed y los cambios de
        // configuración); su primer mensaje pide volver a leer.
        channel.set(access.channelServer())
        val retries = Channel<Unit>(Channel.CONFLATED)
        markRetries = retries
        launch { retryMarks(retries) }
        launch {
            signals.collect { signal ->
                val reread =
                    (signal is RealtimeSignal.Changes && signal.events.isNotEmpty()) ||
                        signal is RealtimeSignal.Rebuild
                if (reread) {
                    requests.trySend(Unit)
                }
            }
        }
        requests.trySend(Unit)
        while (true) {
            requests.receive()
            if (needsConnect && !connect()) {
                continue
            }
            read()
        }
    }

    /** `kitchen-display/connect`; true si respondió y ya se puede leer. */
    private suspend fun connect(): Boolean {
        val takeSeat = takeSeatNext
        return when (val outcome = service.connect(takeSeat)) {
            is ApiOutcome.Success -> {
                needsConnect = false
                takeSeatNext = false
                val settings = BoardMapper.settings(outcome.value.display)
                mutableState.update { it.copy(settings = settings) }
                true
            }
            is ApiOutcome.Rejected -> {
                react(ApiErrors.toAppError(outcome.problem))
                false
            }
            is ApiOutcome.NoAnswer,
            is ApiOutcome.Malformed -> {
                retryLater(retryMillis)
                false
            }
        }
    }

    private suspend fun read() {
        val number = readsStarted.incrementAndGet()
        when (val outcome = service.orders()) {
            is ApiOutcome.Success -> {
                val now = monotonic()
                val board = BoardMapper.board(outcome.value, now)
                dishesConfirmed.values.removeIf { it <= number }
                cardsConfirmed.values.removeIf { it <= number }
                // Solo este ciclo de lecturas cambia el tablero: la lectura anterior es la que se
                // ve.
                val previous = mutableState.value.board
                recoveredTasks.values.removeIf { now - it > RECOVERED_MEMORY_MILLIS }
                val news = BoardAlerts.between(previous, board, recoveredTasks.keys.toSet())
                val present = board.cards.mapTo(HashSet()) { it.id }
                mutableState.update { current ->
                    val fresh =
                        current.freshCards.filter { (cardId, since) ->
                            cardId in present && now - since < FRESH_MILLIS
                        } + news.freshCards.associateWith { now }
                    current.copy(
                        screen = BoardScreen.Orders,
                        board = board,
                        settings = board.settings,
                        freshCards = fresh,
                        // Lo que Go ya respondió está en esta lectura (manda Go); lo que todavía
                        // va en camino se sigue viendo marcado.
                        markingDishes =
                            current.markingDishes.filterTo(mutableSetOf()) {
                                it in dishesInFlight || dishesConfirmed.containsKey(it)
                            },
                        markingCards =
                            current.markingCards.filterTo(mutableSetOf()) {
                                it in cardsInFlight || cardsConfirmed.containsKey(it)
                            },
                        clockInvalid = false,
                    )
                }
                if (news.sounds.isNotEmpty() && board.settings.soundEnabled) {
                    alerts.play(news.sounds, board.settings.soundVolume)
                }
                acknowledge(board.receiptsPending)
            }
            is ApiOutcome.Rejected -> react(ApiErrors.toAppError(outcome.problem))
            is ApiOutcome.NoAnswer,
            is ApiOutcome.Malformed -> retryLater(retryMillis)
        }
    }

    // K1-D12: el acuse de las comandas que ya se muestran. Si falla, la siguiente lectura las
    // vuelve a traer y se vuelve a intentar; la tableta no avisa nada.
    private fun acknowledge(receipts: List<ReceiptDto>) {
        if (receipts.isEmpty()) {
            return
        }
        scope?.launch { service.receipts(receipts) }
    }

    private fun react(error: AppError) {
        when (val reaction = BoardRules.reactionTo(error)) {
            is BoardReaction.Show -> {
                needsConnect = true
                mutableState.update { it.copy(screen = reaction.screen) }
                // Sin cupo se vuelve a pedir cada 30 s (y con cada evento); sin activar o con el
                // cupo liberado, solo con los eventos o con «Usar esta pantalla».
                if (reaction.screen is BoardScreen.NoSeat) {
                    retryLater(noSeatRetryMillis)
                }
            }
            BoardReaction.Reconnect -> {
                needsConnect = true
                retryLater(0)
            }
            BoardReaction.Reread -> requestRead()
            BoardReaction.ClockInvalid -> mutableState.update { it.copy(clockInvalid = true) }
            is BoardReaction.Access -> access.report(reaction.error)
            is BoardReaction.Notice -> {
                mutableState.update { it.copy(notice = reaction.message) }
                requestRead()
            }
        }
    }

    private fun retryLater(delayMillis: Long) {
        if (retryJob?.isActive == true) {
            return
        }
        retryJob = scope?.launch {
            delay(delayMillis)
            requestRead()
        }
    }

    private fun requestRead() {
        reads?.trySend(Unit)
    }

    /** Tocar un platillo: lo marca listo si se puede (§4 y K1-D14). */
    override fun markDish(cardId: String, dishId: String) {
        val current = mutableState.value
        val card = current.board?.cards?.firstOrNull { it.id == cardId } ?: return
        val dish = card.dishes.firstOrNull { it.id == dishId } ?: return
        if (
            !BoardRules.dishTappable(dish, current.markingAllowed) ||
                dishId in current.markingDishes
        ) {
            return
        }
        dishesInFlight.add(dishId)
        mutableState.update { it.copy(markingDishes = it.markingDishes + dishId) }
        val mark =
            PendingMark(
                dish.tasks,
                wholeOrder = false,
                dishIds = setOf(dishId),
                cardIds = emptySet(),
            )
        scope?.launch { send(mark) }
    }

    /** El botón grande de la tarjeta: marca el pedido completo, que desaparece (§4). */
    override fun markOrder(cardId: String) {
        val current = mutableState.value
        val card = current.board?.cards?.firstOrNull { it.id == cardId } ?: return
        if (
            !BoardRules.orderButtonEnabled(card, current.markingAllowed) ||
                cardId in current.markingCards
        ) {
            return
        }
        cardsInFlight.add(cardId)
        mutableState.update { it.copy(markingCards = it.markingCards + cardId) }
        val mark =
            PendingMark(
                BoardRules.orderTasks(card),
                wholeOrder = true,
                dishIds = emptySet(),
                cardIds = setOf(cardId),
            )
        scope?.launch { send(mark) }
    }

    private suspend fun send(mark: PendingMark) {
        val outcome =
            if (mark.wholeOrder) {
                service.markOrder(mark.tasks)
            } else {
                service.markItems(mark.tasks)
            }
        finishMark(outcome, mark)
    }

    // Las marcas sin respuesta se repiten cada [markRetryMillis] mientras haya «En línea» (dentro
    // de la tolerancia); con «Sin conexión» esperan a que vuelva.
    private suspend fun retryMarks(wakeups: Channel<Unit>) {
        while (true) {
            wakeups.receive()
            while (retrying.isNotEmpty()) {
                delay(markRetryMillis)
                link.first { it == LinkState.ONLINE }
                val batch = generateSequence { retrying.poll() }.toList()
                for (mark in batch) {
                    send(mark)
                }
            }
        }
    }

    private fun finishMark(outcome: ApiOutcome<MarkResultDto>, mark: PendingMark) {
        when (outcome) {
            // Idempotente por versión: lo que ya estaba listo vuelve en `skipped` y es un éxito.
            // Se sigue viendo marcado hasta la lectura empezada después, que trae lo que dice Go.
            // Una respuesta que no se entiende también: Go respondió y la lectura dice cómo quedó.
            is ApiOutcome.Success,
            is ApiOutcome.Malformed -> {
                // Antes de dejar de estar «en camino», para que ninguna lectura la vea sin las dos.
                val confirmingRead = readsStarted.get() + 1
                mark.dishIds.forEach { dishesConfirmed[it] = confirmingRead }
                mark.cardIds.forEach { cardsConfirmed[it] = confirmingRead }
                dishesInFlight.removeAll(mark.dishIds)
                cardsInFlight.removeAll(mark.cardIds)
                requestRead()
            }
            // Go dijo otra cosa (ya no se puede marcar, otra estación, la hora…): se quita.
            is ApiOutcome.Rejected -> {
                drop(mark)
                react(ApiErrors.toAppError(outcome.problem))
            }
            // Sin respuesta: se repite sola, en silencio, con la palomita a la vista (§5.1).
            is ApiOutcome.NoAnswer -> {
                val retries = markRetries
                if (retries == null) {
                    drop(mark)
                } else {
                    retrying.add(mark)
                    retries.trySend(Unit)
                }
            }
        }
    }

    private fun drop(mark: PendingMark) {
        dishesInFlight.removeAll(mark.dishIds)
        cardsInFlight.removeAll(mark.cardIds)
        mutableState.update {
            it.copy(
                markingDishes = it.markingDishes - mark.dishIds,
                markingCards = it.markingCards - mark.cardIds,
            )
        }
    }

    /** «Usar esta pantalla» después de «Liberar cupo» (K1-D6 y K1-D30). */
    override fun takeSeat() {
        takeSeatNext = true
        needsConnect = true
        requestRead()
    }

    /** Quita el aviso pasajero. */
    override fun dismissNotice() {
        mutableState.update { it.copy(notice = null) }
    }

    /** Abre «Recientes» y lee lo marcado en los últimos 30 minutos. */
    override fun openRecent() {
        mutableRecent.value = RecentState.Loading
        scope?.launch { loadRecent(message = null) }
    }

    override fun closeRecent() {
        mutableRecent.value = RecentState.Hidden
    }

    /**
     * «Recuperar»: regresa a la pantalla [tasks] marcadas por error (no avisa al mesero, K1-D8). Si
     * Go responde `KITCHEN_NOT_RECOVERABLE`, se muestra su mensaje y se vuelve a leer.
     */
    override fun recover(tasks: List<TaskRef>) {
        if (tasks.isEmpty()) {
            return
        }
        scope?.launch {
            when (val outcome = service.recover(tasks)) {
                is ApiOutcome.Success -> {
                    val now = monotonic()
                    tasks.forEach { recoveredTasks[it.id] = now }
                    requestRead()
                    loadRecent(message = null)
                }
                is ApiOutcome.Rejected -> {
                    val error = ApiErrors.toAppError(outcome.problem)
                    if (outcome.problem.code == BoardRules.NOT_RECOVERABLE) {
                        loadRecent(message = outcome.problem.message)
                    } else {
                        react(error)
                        loadRecent(message = null)
                    }
                }
                is ApiOutcome.NoAnswer,
                is ApiOutcome.Malformed -> loadRecent(message = null)
            }
        }
    }

    private suspend fun loadRecent(message: String?) {
        if (mutableRecent.value == RecentState.Hidden) {
            return
        }
        when (val outcome = service.recent()) {
            is ApiOutcome.Success ->
                mutableRecent.value =
                    RecentState.Shown(outcome.value.orders.map(BoardMapper::recent), message)
            is ApiOutcome.Rejected -> {
                react(ApiErrors.toAppError(outcome.problem))
                mutableRecent.value = RecentState.Shown(emptyList(), message)
            }
            is ApiOutcome.NoAnswer,
            is ApiOutcome.Malformed -> mutableRecent.value = RecentState.Shown(emptyList(), message)
        }
    }

    private companion object {
        // Una lectura sin respuesta de Go se vuelve a intentar en silencio.
        const val RETRY_MILLIS = 5_000L

        // Una marca sin respuesta se repite en silencio (§5.1).
        const val MARK_RETRY_MILLIS = 2_000L

        // La animación de la tarjeta nueva dura menos que esto; después ya no es «nueva».
        const val FRESH_MILLIS = 5_000L

        // Lo recuperado aquí no suena como pedido nuevo si vuelve en este lapso.
        const val RECOVERED_MEMORY_MILLIS = 60_000L

        // Sin cupo, la tableta vuelve a pedirlo sola cada 30 s (contrato, sección 10).
        const val NO_SEAT_RETRY_MILLIS = 30_000L
    }
}
