package com.adelpos.meseros.core.data.ready

import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLedger
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.ErrorAction
import com.adelpos.meseros.core.domain.ready.ReadyAlertsProcessor
import com.adelpos.meseros.core.domain.ready.ReadyBatchOutcome
import com.adelpos.meseros.core.domain.realtime.ReconnectBackoff
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withTimeoutOrNull

/** Cómo va el canal de avisos (para la notificación permanente y la ayuda). */
sealed interface ReadyChannelState {
    /** Abriendo la conexión. */
    data object Connecting : ReadyChannelState

    /** Conectado: los avisos llegan al momento. */
    data object Connected : ReadyChannelState

    /** Sin conexión: [failures] intentos seguidos, el siguiente en [delayMillis]. */
    data class Waiting(val failures: Int, val delayMillis: Long) : ReadyChannelState
}

/** Qué dice la notificación permanente del servicio. */
enum class ReadyOngoing {
    /** El canal no está abierto: no dice «Recibiendo avisos» (RD5). */
    CONNECTING,

    /** El canal está abierto: «Recibiendo avisos de platillos listos». */
    CONNECTED,

    /** Sin conexión con la computadora principal: «Reintentando…». */
    WAITING;

    companion object {
        /**
         * El texto con el estado del canal y el estado único de conexión [link] (etapa A3, A3-D1):
         * «Sin conexión…» solo cuando la barra también lo dice (una falla del canal, que se
         * reconecta solo, no basta); «Recibiendo avisos» solo con el canal abierto (RD5); si no,
         * «Conectando…». Mientras dure «Sin conexión», el texto no cambia en cada intento.
         */
        fun of(state: ReadyChannelState, link: LinkState): ReadyOngoing =
            when {
                link == LinkState.OFFLINE -> WAITING
                state == ReadyChannelState.Connected -> CONNECTED
                else -> CONNECTING
            }
    }
}

/** Por qué terminó el canal (y con él, el servicio en primer plano). */
enum class ReadyStopReason {
    /** Go dijo `active: false`: nadie a quien avisar hoy, o el aviso se apagó en Terminales. */
    INACTIVE,

    /** El dispositivo ya no está listo (retirado, sin emparejar, por emparejar o actualizar). */
    DEVICE_NOT_READY,

    /** Go rechazó la credencial o la versión: ya se avisó a la entrada al dispositivo. */
    REPORTED,

    /** Un rechazo que reintentar no arregla (no es un dispositivo `android`, 400). */
    REJECTED,

    /**
     * A2-D12: pasó el fin del día comercial (y su margen), Go no responde y nadie está usando el
     * dispositivo: se detiene sin mover el cursor (lo de mientras llega al volver a arrancar).
     */
    DAY_ENDED,
}

/**
 * **El único canal de avisos del proceso** (regla 18 de `android/CLAUDE.md`: un solo canal de
 * eventos y un solo canal de avisos por proceso; Go admite uno por dispositivo). Lo mantiene el
 * servicio en primer plano mientras corra [run]:
 *
 * - Abre `ready-alerts/stream` desde el cursor guardado en Room y procesa cada lote
 *   ([ReadyAlertsProcessor]: quita lo ya avisado, muestra lo nuevo y guarda el cursor).
 * - Go manda solo lo que cambia, `: ping` cada 15 s y cierra cada flujo a los 5 minutos: un cierre
 *   normal (la conexión duró o trajo mensajes) se reabre **enseguida** desde el último cursor. Sin
 *   red, 5xx, un `interrupted` temporal o una conexión muerta (45 s sin un byte): espera creciente
 *   (2, 4, 8, 16 y 30 s), que [reconnectNow] acorta (al recuperar la red). Lo que pasó mientras
 *   tanto llega al volver, una sola vez.
 * - Pasado `day_ends_at`, vuelve a preguntar con `GET ready-alerts` (aunque el canal siga callado):
 *   mientras Go diga `active: true`, sigue.
 * - 429 (no debería pasar: un canal nuevo reemplaza al anterior en Go): no abre otro en paralelo;
 *   espera y mientras lee con `GET ready-alerts?after=`.
 * - Tras [ReadyAlertsLifecycle.RELOCATE_AFTER_FAILURES] fallas seguidas pide buscar el servidor en
 *   otra dirección (cambio de IP), como mucho una vez por minuto.
 * - Termina con `active: false`, si el dispositivo deja de estar listo o si Go lo da por retirado
 *   (`DEVICE_REVOKED`, que avisa a la entrada). **A2-D12**: también pasados 10 minutos del fin del
 *   día comercial sin alcanzar a Go y sin nadie usando el dispositivo ([inUse]), sin mover el
 *   cursor ([ReadyStopReason.DAY_ENDED]).
 * - Un lote que no se entiende o algo inesperado al procesarlo cuenta como una falla: se reintenta
 *   desde el último cursor guardado ([onUnexpectedError] lo deja en el registro), sin tumbar el
 *   proceso ni la captura del mesero.
 *
 * @param onReconnected se llama cuando el canal vuelve a recibir después de fallar: ahí se pide un
 *   envío de la cola (como los demás disparadores de la A1); el canal nunca envía pedidos.
 * @param inUse un mesero con sesión tiene la aplicación a la vista: el canal no se detiene por el
 *   fin del día aunque Go no responda.
 */
class ReadyAlertsChannel(
    private val server: ReadyAlertsServer,
    private val ledger: ReadyAlertsLedger,
    private val processor: ReadyAlertsProcessor,
    private val client: ReadyAlertsClient = ReadyAlertsClient(),
    private val clock: () -> Long = System::currentTimeMillis,
    private val backoff: ReconnectBackoff = ReadyAlertsLifecycle.backoff(),
    private val onReconnected: () -> Unit = {},
    private val healthyConnectionMillis: Long = ReadyAlertsLifecycle.HEALTHY_CONNECTION_MILLIS,
    private val inUse: () -> Boolean = { false },
    private val onUnexpectedError: (Exception) -> Unit = {},
) {
    private val mutableState = MutableStateFlow<ReadyChannelState>(ReadyChannelState.Connecting)
    private val wakeUps = Channel<Unit>(Channel.CONFLATED)

    val state: StateFlow<ReadyChannelState> = mutableState.asStateFlow()

    /** Acorta la espera actual (al recuperar la red). */
    fun reconnectNow() {
        wakeUps.trySend(Unit)
    }

    /** Mantiene el canal hasta que corresponde detenerse; cancelar la corrutina lo cierra. */
    suspend fun run(): ReadyStopReason {
        processor.restart()
        mutableState.value = ReadyChannelState.Connecting
        server.prepare()
        val stored = ledger.status()
        val loop = Loop(cursor = stored?.cursor ?: 0L, dayEndsAt = stored?.dayEndsAtMillis)
        while (true) {
            if (server.ready != true) {
                return ReadyStopReason.DEVICE_NOT_READY
            }
            val connection = server.connection() ?: return ReadyStopReason.DEVICE_NOT_READY
            mutableState.value = ReadyChannelState.Connecting
            var stop: ReadyStopReason? = null
            // Al llegar el fin del día comercial (y un margen) se cierra el flujo para preguntar.
            val dayCheck = ReadyAlertsLifecycle.dayCheckIn(loop.dayEndsAt, clock())
            val streamed: suspend () -> ReadyStreamEnd = {
                client.stream(connection, loop.cursor, onOpened = { loop.onOpened() }) { batch ->
                    stop = loop.onBatch(batch)
                    stop == null
                }
            }
            var dayCheckDue = false
            val end: ReadyStreamEnd? =
                try {
                    if (dayCheck == null) {
                        streamed()
                    } else {
                        withTimeoutOrNull(dayCheck) { streamed() }.also { dayCheckDue = it == null }
                    }
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (unexpected: Exception) {
                    // RD4: algo inesperado al procesar un lote (Room lleno, el sistema de
                    // notificaciones): cuenta como una falla y se reintenta desde el último cursor
                    // procesado, sin tumbar el proceso.
                    onUnexpectedError(unexpected)
                    null
                }
            stop?.let {
                return it
            }
            val reason =
                when {
                    // Cambió el día: Go dice si sigue activo (un mesero con su sesión vigente).
                    dayCheckDue -> readMeanwhile(loop)
                    end == null -> {
                        loop.failures += 1
                        null
                    }
                    else -> afterStream(loop, end)
                }
            if (reason != null) {
                return reason
            }
            if (loop.failures > 0) {
                loop.interrupted = true
                if (
                    ReadyAlertsLifecycle.dayOverWithoutGo(
                        dayEndsAtMillis = loop.dayEndsAt,
                        lastReachedMillis = loop.lastReached,
                        nowMillis = clock(),
                        inUse = inUse(),
                    )
                ) {
                    return ReadyStopReason.DAY_ENDED
                }
            }
            relocateIfStuck(loop)
            if (loop.failures > 0) {
                val delayMillis = backoff.delayFor(loop.failures)
                mutableState.value = ReadyChannelState.Waiting(loop.failures, delayMillis)
                withTimeoutOrNull(delayMillis) { wakeUps.receive() }
            }
        }
    }

    /** Cómo terminó una conexión: actualiza las fallas y dice si hay que detenerse. */
    private suspend fun afterStream(loop: Loop, end: ReadyStreamEnd): ReadyStopReason? {
        when (end) {
            ReadyStreamEnd.Stopped -> Unit
            is ReadyStreamEnd.Closed ->
                loop.failures =
                    if (
                        end.opened &&
                            (end.received > 0 || end.lastedMillis >= healthyConnectionMillis)
                    ) {
                        0
                    } else {
                        loop.failures + 1
                    }
            is ReadyStreamEnd.Failed -> return onFailure(loop, end)
        }
        return null
    }

    /** Lo que se recuerda entre conexiones mientras corre [run]. */
    private inner class Loop(var cursor: Long, var dayEndsAt: Long?) {
        var failures = 0
        var lastRelocate = 0L

        /** Cuándo se alcanzó a Go por última vez (el canal abrió o llegó una lectura), o null. */
        var lastReached: Long? = null

        /** El canal falló desde la última vez que recibió (para pedir el envío de la cola). */
        var interrupted = false

        /**
         * El canal abrió (con el protocolo nuevo, Go puede quedarse callado hasta que haya algo).
         */
        fun onOpened() {
            lastReached = clock()
            mutableState.value = ReadyChannelState.Connected
            if (interrupted) {
                interrupted = false
                onReconnected()
            }
        }

        /** Procesa un lote; devuelve por qué detenerse, o null para seguir. */
        suspend fun onBatch(batch: ReadyAlertsBatch): ReadyStopReason? {
            dayEndsAt = batch.dayEndsAtMillis
            failures = 0
            onOpened()
            val outcome = processor.onBatch(batch)
            cursor = batch.cursor
            return if (outcome == ReadyBatchOutcome.INACTIVE) {
                ReadyStopReason.INACTIVE
            } else {
                null
            }
        }
    }

    /** Un corte o un rechazo del canal; devuelve por qué detenerse, o null para reintentar. */
    private suspend fun onFailure(loop: Loop, end: ReadyStreamEnd.Failed): ReadyStopReason? =
        when (ReadyAlertsLifecycle.onError(end.error)) {
            ErrorAction.STOP_AND_REPORT -> {
                server.report(end.error)
                ReadyStopReason.REPORTED
            }
            ErrorAction.STOP -> ReadyStopReason.REJECTED
            ErrorAction.WAIT_AND_READ -> {
                // La espera sigue creciendo mientras el canal no se pueda abrir, aunque la lectura
                // de mientras tanto funcione.
                val attempts = loop.failures + 1
                val stop = readMeanwhile(loop)
                loop.failures = attempts
                stop
            }
            ErrorAction.RETRY -> {
                // Una conexión que funcionó y se cayó espera lo mínimo; las fallas seguidas sin un
                // solo mensaje esperan cada vez más.
                loop.failures =
                    if (end.received > 0) {
                        1
                    } else {
                        loop.failures + 1
                    }
                null
            }
        }

    /**
     * Lee con `GET ready-alerts?after=`: con el canal ocupado (429, no se espera a que Go suelte el
     * anterior para mostrar los avisos) y al cambiar el día. Si no se pudo leer, cuenta como falla.
     */
    private suspend fun readMeanwhile(loop: Loop): ReadyStopReason? {
        val connection = server.connection() ?: return ReadyStopReason.DEVICE_NOT_READY
        return when (val read = client.read(connection, loop.cursor)) {
            is ReadyRead.Success -> loop.onBatch(read.batch)
            is ReadyRead.Failed -> {
                loop.failures += 1
                stopFor(read.error)
            }
        }
    }

    private fun stopFor(error: AppError): ReadyStopReason? =
        when (ReadyAlertsLifecycle.onError(error)) {
            ErrorAction.STOP_AND_REPORT -> {
                server.report(error)
                ReadyStopReason.REPORTED
            }
            ErrorAction.STOP -> ReadyStopReason.REJECTED
            ErrorAction.WAIT_AND_READ,
            ErrorAction.RETRY -> null
        }

    private suspend fun relocateIfStuck(loop: Loop) {
        val now = clock()
        if (
            loop.failures >= ReadyAlertsLifecycle.RELOCATE_AFTER_FAILURES &&
                now - loop.lastRelocate >= ReadyAlertsLifecycle.RELOCATE_EVERY_MILLIS
        ) {
            loop.lastRelocate = now
            server.relocate()
        }
    }
}
