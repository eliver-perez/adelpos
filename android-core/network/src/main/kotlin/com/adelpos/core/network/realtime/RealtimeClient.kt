package com.adelpos.core.network.realtime

import com.adelpos.core.model.realtime.AccessInterruption
import com.adelpos.core.model.realtime.AccessInterruptions
import com.adelpos.core.model.realtime.RealtimeFeed
import com.adelpos.core.model.realtime.RealtimeSignal
import com.adelpos.core.model.realtime.RealtimeState
import com.adelpos.core.model.realtime.RebuildReason
import com.adelpos.core.model.realtime.ReconnectBackoff
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiJson
import com.adelpos.core.network.api.ApiProblem
import com.adelpos.core.network.session.AuthorizedServer
import com.adelpos.core.network.session.ServerAccess
import com.adelpos.core.network.session.apiUrl
import com.adelpos.core.network.session.request
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.channels.trySendBlocking
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources

/**
 * Cursor de eventos con el que abrir el canal cuando no hay uno propio (primera vez, sesión nueva o
 * al volver de segundo plano). Lo normal es el `event_cursor` de `GET restaurant/state`
 * (`RecoveryStateCursorSeed` de la aplicación de meseros); la Fase E puede dar el de su propia
 * lectura del estado. Null si no se pudo obtener: el canal se abre sin cursor.
 */
fun interface RealtimeCursorSeed {
    suspend fun eventCursor(server: AuthorizedServer): Long?
}

/**
 * Cliente del canal de tiempo real de Go (`GET events`, SSE) para toda la aplicación: **un solo
 * flujo por proceso**, compartido como [signals] (Go admite dos conexiones por dispositivo y
 * responde 429 a la tercera: nunca se abre más de una).
 *
 * Reglas (avance de la etapa, Fase 0 §3):
 * - Solo está abierto con la aplicación en primer plano ([active]) y con sesión ([ServerAccess]);
 *   en segundo plano se cierra (la cola la lleva WorkManager) y al volver se abre de nuevo.
 * - Go cierra cada flujo a los 30 s: se reconecta enseguida con `?after=<cursor>`. Si falla (sin
 *   red, 5xx, 429) se reconecta con espera creciente ([ReconnectBackoff]); [reconnectNow] la acorta
 *   (al recuperar la red).
 * - Go manda un mensaje cada 750 ms aunque no haya cambios: un `readTimeout` de [readTimeoutMillis]
 *   detecta una conexión muerta.
 * - El cursor sale solo del `id` de los mensajes `changes`, nunca del `0` de `interrupted`.
 * - `reset: true` → [RealtimeSignal.Rebuild]; abrir sin un cursor propio también (el primer mensaje
 *   avisa de reconstruir, porque lo anterior no llegó por eventos).
 * - `event: interrupted` (o el rechazo al abrir) por el dispositivo, la sesión, los permisos o la
 *   versión → [RealtimeSignal.Interrupted] y aviso a [interruptions]; no se reconecta hasta que
 *   cambie la sesión. Al cambiar de mesero se reabre con la sesión nueva.
 */
class RealtimeClient(
    private val access: ServerAccess,
    private val active: StateFlow<Boolean>,
    private val interruptions: AccessInterruptions,
    private val cursorSeed: RealtimeCursorSeed? = null,
    private val backoff: ReconnectBackoff = ReconnectBackoff(),
    private val readTimeoutMillis: Long = DEFAULT_READ_TIMEOUT_MILLIS,
    private val json: Json = ApiJson,
) : RealtimeFeed {
    private val signalFlow = MutableSharedFlow<RealtimeSignal>(extraBufferCapacity = 64)
    private val stateFlow = MutableStateFlow<RealtimeState>(RealtimeState.Stopped)
    private val wakeUps = Channel<Unit>(Channel.CONFLATED)

    /** Último cursor recibido en el periodo activo actual (null = abrir desde la semilla). */
    @Volatile private var cursor: Long? = null

    /**
     * El canal se abrió desde la semilla (o sin cursor) y todavía no llega el primer mensaje: ese
     * mensaje avisa de reconstruir. Sobrevive a los reintentos fallidos hasta que llega.
     */
    @Volatile private var rebuildPending: Boolean = false

    override val signals: SharedFlow<RealtimeSignal> = signalFlow.asSharedFlow()

    override val state: StateFlow<RealtimeState> = stateFlow.asStateFlow()

    /** Cursor actual (para pruebas y la sección de soporte). */
    val currentCursor: Long?
        get() = cursor

    override fun reconnectNow() {
        wakeUps.trySend(Unit)
    }

    /** Empieza a mantener el canal en [scope] (una sola vez por proceso). */
    fun start(scope: CoroutineScope): Job = scope.launch {
        combine(access.current, active) { server, inUse -> if (inUse) server else null }
            .distinctUntilChanged()
            .collectLatest { server ->
                // Cada periodo activo (o cada sesión) empieza desde la semilla: lo ocurrido
                // mientras estaba cerrado se recupera leyendo el estado, no evento por evento.
                cursor = null
                rebuildPending = false
                if (server == null) {
                    stateFlow.value = RealtimeState.Stopped
                } else {
                    keepOpen(server)
                }
            }
    }

    private suspend fun keepOpen(server: AuthorizedServer) {
        var failures = 0
        while (true) {
            if (cursor == null) {
                cursor = cursorSeed?.eventCursor(server)
                rebuildPending = true
            }
            stateFlow.value = RealtimeState.Connecting
            when (val end = connectOnce(server, cursor)) {
                // Go cierra cada flujo a los 30 s: si la conexión llegó a recibir mensajes, se
                // reconecta enseguida.
                is ConnectionEnd.Closed ->
                    failures =
                        if (end.receivedMessages) {
                            0
                        } else {
                            failures + 1
                        }
                // Una conexión que funcionó y se cayó (sin red, conexión muerta) espera lo mínimo;
                // las fallas seguidas sin un solo mensaje esperan cada vez más.
                is ConnectionEnd.Failed ->
                    failures =
                        if (end.receivedMessages) {
                            1
                        } else {
                            failures + 1
                        }
                is ConnectionEnd.Interrupted -> {
                    stateFlow.value = RealtimeState.Interrupted(end.interruption)
                    signalFlow.emit(RealtimeSignal.Interrupted(end.interruption))
                    interruptions.onInterrupted(end.interruption, server.userId)
                    // No se reconecta con la misma sesión: se espera a que cambie (otro mesero,
                    // nueva entrada con PIN) o a que la aplicación vuelva a primer plano.
                    awaitCancellation()
                }
            }
            if (failures > 0) {
                val delayMillis = backoff.delayFor(failures)
                stateFlow.value = RealtimeState.Waiting(failures, delayMillis)
                withTimeoutOrNull(delayMillis) { wakeUps.receive() }
            }
        }
    }

    /** Abre una conexión y la atiende hasta que termina. */
    private suspend fun connectOnce(server: AuthorizedServer, after: Long?): ConnectionEnd {
        val url = server.apiUrl("events").newBuilder()
        if (after != null) {
            url.addQueryParameter("after", after.toString())
        }
        val request = server.request(url.build()).header("Accept", EVENT_STREAM).get().build()
        val client =
            server.client.newBuilder().readTimeout(readTimeoutMillis, TimeUnit.MILLISECONDS).build()
        val items = Channel<StreamItem>(capacity = 64)
        val source = EventSources.createFactory(client).newEventSource(request, Listener(items))
        val connection = Connection()
        try {
            for (item in items) {
                val end = process(item, connection)
                if (end != null) {
                    return end
                }
            }
            return ConnectionEnd.Closed(connection.received > 0)
        } finally {
            // Cancelar cierra el socket; cancelar el canal evita que el hilo de OkHttp se quede
            // esperando para entregar un mensaje que ya nadie va a leer.
            source.cancel()
            items.cancel()
        }
    }

    private suspend fun process(item: StreamItem, connection: Connection): ConnectionEnd? =
        when (item) {
            StreamItem.Opened -> {
                stateFlow.value = RealtimeState.Connected(cursor ?: 0)
                null
            }
            is StreamItem.Message -> onMessage(item, connection)
            StreamItem.Closed -> ConnectionEnd.Closed(connection.received > 0)
            is StreamItem.Failed -> onFailure(item, connection)
        }

    private suspend fun onMessage(
        message: StreamItem.Message,
        connection: Connection,
    ): ConnectionEnd? {
        when (message.type) {
            CHANGES -> onChanges(message, connection)
            INTERRUPTED -> return onInterrupted(message.data, connection)
            // Un tipo de mensaje que esta versión no conoce se ignora (contrato v1).
            else -> Unit
        }
        return null
    }

    private suspend fun onChanges(message: StreamItem.Message, connection: Connection) {
        val batch =
            try {
                json.decodeFromString(EventBatchDto.serializer(), message.data)
            } catch (_: SerializationException) {
                return
            } catch (_: IllegalArgumentException) {
                return
            }
        connection.received += 1
        // El cursor es el id del mensaje changes (Go escribe el cursor del lote); si faltara, el
        // del
        // propio lote.
        val next = message.id?.toLongOrNull()?.takeIf { it >= 0 } ?: batch.cursor
        cursor = next
        stateFlow.value = RealtimeState.Connected(next)
        when {
            batch.reset -> {
                rebuildPending = false
                signalFlow.emit(RealtimeSignal.Rebuild(next, RebuildReason.SERVER_RESET))
            }
            rebuildPending -> {
                rebuildPending = false
                signalFlow.emit(RealtimeSignal.Rebuild(next, RebuildReason.FRESH_START))
            }
            batch.events.isNotEmpty() ->
                signalFlow.emit(RealtimeSignal.Changes(next, batch.events.map { it.toDomain() }))
        }
    }

    /** `event: interrupted` con `{"error", "status"}` e `id: 0` (que nunca se toma como cursor). */
    private fun onInterrupted(data: String, connection: Connection): ConnectionEnd {
        val failed = ConnectionEnd.Failed(connection.received > 0)
        val body =
            try {
                json.decodeFromString(InterruptedDto.serializer(), data)
            } catch (_: SerializationException) {
                return failed
            } catch (_: IllegalArgumentException) {
                return failed
            }
        val problem = ApiProblem(body.status, body.error, null, null, null)
        val interruption = AccessInterruptionMapping.forEvents(ApiErrors.toAppError(problem))
        return if (interruption != null) {
            ConnectionEnd.Interrupted(interruption)
        } else {
            failed
        }
    }

    /** Rechazo al abrir (JSON de error con su estado) o falla de red. */
    private fun onFailure(failure: StreamItem.Failed, connection: Connection): ConnectionEnd {
        val failed = ConnectionEnd.Failed(connection.received > 0)
        val status = failure.status ?: return failed
        if (status in 200..299) {
            // Se cortó un flujo que sí abrió (sin red, conexión muerta) o la respuesta no era un
            // flujo de eventos (un proxy, una página): reintentar.
            return failed
        }
        val error = ApiErrors.toAppError(ApiErrors.problemOf(status, failure.body))
        val interruption = AccessInterruptionMapping.forEvents(error) ?: return failed
        return ConnectionEnd.Interrupted(interruption)
    }

    /** Mensajes recibidos por una conexión mientras está abierta. */
    private class Connection {
        var received: Int = 0
    }

    private sealed interface ConnectionEnd {
        /** Go cerró el flujo (a los 30 s) o el flujo terminó. */
        data class Closed(val receivedMessages: Boolean) : ConnectionEnd

        data class Failed(val receivedMessages: Boolean) : ConnectionEnd

        data class Interrupted(val interruption: AccessInterruption) : ConnectionEnd
    }

    private sealed interface StreamItem {
        data object Opened : StreamItem

        class Message(val id: String?, val type: String?, val data: String) : StreamItem

        data object Closed : StreamItem

        class Failed(val status: Int?, val body: String?) : StreamItem
    }

    /** Pasa lo que entrega okhttp-sse (en su hilo) al canal que atiende la corrutina. */
    private class Listener(private val items: Channel<StreamItem>) : EventSourceListener() {
        override fun onOpen(eventSource: EventSource, response: Response) {
            items.trySend(StreamItem.Opened)
        }

        override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
            // Bloquea el hilo de lectura si la aplicación va atrasada (contrapresión).
            items.trySendBlocking(StreamItem.Message(id, type, data))
        }

        override fun onClosed(eventSource: EventSource) {
            items.trySend(StreamItem.Closed)
            items.close()
        }

        override fun onFailure(eventSource: EventSource, t: Throwable?, response: Response?) {
            // El cuerpo de un rechazo (JSON de Go) solo se puede leer aquí, dentro de la llamada.
            // Nunca el de un flujo que ya abrió: seguiría leyendo eventos.
            val body =
                if (response == null || response.isSuccessful) {
                    null
                } else {
                    readErrorBody(response)
                }
            items.trySend(StreamItem.Failed(response?.code, body))
            items.close()
        }

        private fun readErrorBody(response: Response): String? =
            try {
                response.body.string()
            } catch (_: IOException) {
                null
            } catch (_: IllegalStateException) {
                null
            }
    }

    companion object {
        /** Sin ningún byte en 10 s (Go escribe cada 750 ms) la conexión se da por muerta. */
        const val DEFAULT_READ_TIMEOUT_MILLIS: Long = 10_000

        private const val EVENT_STREAM = "text/event-stream"
        private const val CHANGES = "changes"
        private const val INTERRUPTED = "interrupted"
    }
}
