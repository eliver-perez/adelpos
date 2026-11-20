package com.adelpos.meseros.core.data.ready

import com.adelpos.meseros.core.data.access.AccessRepository
import com.adelpos.meseros.core.data.api.ApiErrors
import com.adelpos.meseros.core.data.api.ApiJson
import com.adelpos.meseros.core.data.session.AuthorizedServer
import com.adelpos.meseros.core.data.session.apiUrl
import com.adelpos.meseros.core.data.session.execute
import com.adelpos.meseros.core.data.session.request
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.channels.trySendBlocking
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources

/**
 * Lo que el canal de avisos necesita de la entrada al dispositivo: **solo la credencial del
 * dispositivo** (A2-D9, como el agente de impresión): sin sesión de mesero ni credencial de envío,
 * para que el aviso llegue también en «Selecciona tu usuario» y con la aplicación cerrada.
 */
interface ReadyAlertsServer {
    /**
     * Si el dispositivo está listo **ahora** (emparejado y compatible): false si se retiró, hay que
     * emparejarlo de nuevo o pide actualizar; null mientras se lee lo guardado. Se lee en el
     * momento (no un flujo derivado): justo después de [prepare] ya dice el estado leído.
     */
    val ready: Boolean?

    /**
     * Lee el servidor emparejado guardado si todavía nadie lo leyó: el servicio pudo arrancar sin
     * pantallas (reinicio del equipo o reinicio pegajoso tras morir el proceso).
     */
    suspend fun prepare()

    /**
     * La conexión en la dirección vigente con la huella fijada y las cabeceras **del dispositivo**
     * (`X-Device-ID`, `X-Device-Token`; nunca `Authorization`), o null sin dispositivo listo.
     */
    fun connection(): AuthorizedServer?

    /**
     * Un error de acceso de Go (dispositivo retirado o no reconocido, otra huella, versión vieja):
     * lo resuelve la entrada al dispositivo como cualquier otro (`DeviceAccess.report`).
     */
    fun report(error: AppError)

    /**
     * Vuelve a comprobar el servidor, lo que incluye buscarlo en otra dirección si cambió de IP
     * (`PairedConnection` lo encuentra por su `server_id` y su huella).
     */
    suspend fun relocate()
}

/** [ReadyAlertsServer] sobre [AccessRepository]. */
class DeviceReadyAlertsServer(private val access: AccessRepository) : ReadyAlertsServer {
    override val ready: Boolean?
        get() =
            when (access.status.value) {
                DeviceStatus.Starting -> null
                is DeviceStatus.Ready -> true
                else -> false
            }

    override suspend fun prepare() {
        if (access.status.value is DeviceStatus.Starting) {
            access.start()
        }
    }

    override fun connection(): AuthorizedServer? {
        if (access.status.value !is DeviceStatus.Ready) {
            return null
        }
        val credentials = access.deviceCredentials() ?: return null
        val initial = access.serverTransport() ?: return null
        return AuthorizedServer(
            transport = { access.serverTransport() ?: initial },
            credentials = credentials.headers(),
            userId = null,
        )
    }

    override fun report(error: AppError) {
        access.report(error, null)
    }

    override suspend fun relocate() {
        access.recheck()
    }
}

/** Resultado de `GET ready-alerts?after=`. */
sealed interface ReadyRead {
    data class Success(val batch: ReadyAlertsBatch) : ReadyRead

    data class Failed(val error: AppError) : ReadyRead
}

/** Cómo terminó una conexión del canal `ready-alerts/stream`. */
sealed interface ReadyStreamEnd {
    /**
     * Go cerró el flujo (lo hace a los 5 minutos) después de [received] mensajes; [opened] dice si
     * llegó a abrir y [lastedMillis] cuánto duró abierto (con solo `: ping` no hay mensajes).
     */
    data class Closed(val received: Int, val opened: Boolean = true, val lastedMillis: Long = 0) :
        ReadyStreamEnd

    /**
     * Se cortó o se rechazó (sin red, conexión muerta, `event: interrupted`, 401, 429…) después de
     * [received] mensajes.
     */
    data class Failed(val received: Int, val error: AppError) : ReadyStreamEnd

    /** Quien recibe los lotes pidió terminar (por ejemplo, `active: false`). */
    data object Stopped : ReadyStreamEnd
}

/**
 * Cliente HTTP de los avisos de platillos listos (`GET ready-alerts` y `GET ready-alerts/stream`,
 * contrato v1, parte D del servidor). Habla por el cliente con la huella fijada de la conexión
 * ([AuthorizedServer.client]): nunca acepta otro certificado.
 *
 * - El canal manda un mensaje `alerts` con `id: <cursor>` solo cuando hay avisos nuevos o cambia
 *   `active`, la configuración o `day_ends_at`; callado, un comentario `: ping` cada 15 s, y Go lo
 *   cierra a los 5 minutos. Un [readTimeoutMillis] (45 s) sin ningún byte da la conexión por
 *   muerta.
 * - Al reconectar se manda el cursor en `?after=` **y** en `Last-Event-ID` (Go usa el primero).
 * - `event: interrupted` (`id: 0`, `{"code"}`): solo `DEVICE_REVOKED` es el dispositivo retirado;
 *   cualquier otro código es temporal (se reintenta). Su `id` nunca es un cursor.
 */
class ReadyAlertsClient(
    private val readTimeoutMillis: Long = DEFAULT_READ_TIMEOUT_MILLIS,
    private val json: Json = ApiJson,
) {
    /** `GET ready-alerts?after=<after>`: la lectura de recuperación (sin límite de conexiones). */
    suspend fun read(server: AuthorizedServer, after: Long): ReadyRead {
        val url = server.apiUrl(READ_PATH).newBuilder()
        url.addQueryParameter("after", after.toString())
        val request = server.request(url.build()).header("Accept", "application/json").get().build()
        val response =
            try {
                server.execute(request)
            } catch (exception: IOException) {
                return ReadyRead.Failed(ApiErrors.fromException(exception))
            }
        return withContext(Dispatchers.IO) {
            response.use { answer ->
                val text =
                    try {
                        answer.body.string()
                    } catch (exception: IOException) {
                        return@withContext ReadyRead.Failed(ApiErrors.fromException(exception))
                    }
                if (!answer.isSuccessful) {
                    return@withContext ReadyRead.Failed(
                        ApiErrors.toAppError(ApiErrors.problemOf(answer.code, text)),
                    )
                }
                val batch = decode(text)
                if (batch == null) {
                    ReadyRead.Failed(AppError.InvalidResponse(answer.code))
                } else {
                    ReadyRead.Success(batch)
                }
            }
        }
    }

    /**
     * Abre una conexión del canal desde [after] y la atiende hasta que termina: [onOpened] cuando
     * Go la acepta y cada lote a [onBatch] en orden; si devuelve false, la conexión se cierra
     * ([ReadyStreamEnd.Stopped]).
     */
    suspend fun stream(
        server: AuthorizedServer,
        after: Long,
        onOpened: () -> Unit = {},
        onBatch: suspend (ReadyAlertsBatch) -> Boolean,
    ): ReadyStreamEnd {
        val url = server.apiUrl(STREAM_PATH).newBuilder()
        url.addQueryParameter("after", after.toString())
        val request =
            server
                .request(url.build())
                .header("Accept", EVENT_STREAM)
                .header("Last-Event-ID", after.toString())
                .get()
                .build()
        val client =
            server.client.newBuilder().readTimeout(readTimeoutMillis, TimeUnit.MILLISECONDS).build()
        val items = Channel<StreamItem>(capacity = 64)
        val source = EventSources.createFactory(client).newEventSource(request, Listener(items))
        var received = 0
        var openedAt: Long? = null
        try {
            for (item in items) {
                when (item) {
                    StreamItem.Opened -> {
                        openedAt = System.nanoTime()
                        onOpened()
                    }
                    is StreamItem.Message ->
                        when (item.type) {
                            ALERTS -> {
                                // RD7: un lote que no se entiende no se salta (Go ya avanzó su
                                // cursor): se reconecta desde el último procesado.
                                val batch =
                                    decode(item.data)
                                        ?: return ReadyStreamEnd.Failed(
                                            received,
                                            AppError.InvalidResponse(200),
                                        )
                                received += 1
                                if (!onBatch(batch)) {
                                    return ReadyStreamEnd.Stopped
                                }
                            }
                            INTERRUPTED -> return ReadyStreamEnd.Failed(received, interrupted(item))
                            // Un tipo de mensaje que esta versión no conoce se ignora (contrato
                            // v1).
                            else -> Unit
                        }
                    StreamItem.Closed -> return closed(received, openedAt)
                    is StreamItem.Failed -> return ReadyStreamEnd.Failed(received, failure(item))
                }
            }
            return closed(received, openedAt)
        } finally {
            // Cancelar cierra el socket; cancelar el canal evita que el hilo de OkHttp se quede
            // esperando para entregar un mensaje que ya nadie va a leer.
            source.cancel()
            items.cancel()
        }
    }

    private fun decode(text: String): ReadyAlertsBatch? =
        try {
            json.decodeFromString(ReadyAlertsBatchDto.serializer(), text).toDomain()
        } catch (_: SerializationException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        }

    private fun closed(received: Int, openedAt: Long?): ReadyStreamEnd.Closed =
        ReadyStreamEnd.Closed(
            received = received,
            opened = openedAt != null,
            lastedMillis =
                if (openedAt == null) {
                    0
                } else {
                    (System.nanoTime() - openedAt) / 1_000_000
                },
        )

    /**
     * `event: interrupted`: solo `DEVICE_REVOKED` es el dispositivo retirado; cualquier otro código
     * (o un mensaje que no se entiende) es temporal, como un 5xx: se reintenta con espera
     * creciente.
     */
    private fun interrupted(message: StreamItem.Message): AppError {
        val code =
            try {
                json.decodeFromString(ReadyInterruptedDto.serializer(), message.data).code()
            } catch (_: SerializationException) {
                null
            } catch (_: IllegalArgumentException) {
                null
            }
        return if (code == DEVICE_REVOKED) {
            AppError.DeviceRevoked(null, null)
        } else {
            AppError.ServerProblem(TEMPORARY_STATUS, code, null, null)
        }
    }

    /** Rechazo al abrir (JSON de error con su estado) o falla de red (o de la huella). */
    private fun failure(failure: StreamItem.Failed): AppError {
        val status = failure.status
        if (status == null || status in 200..299) {
            val cause = failure.cause as? IOException ?: return AppError.Unreachable
            return ApiErrors.fromException(cause)
        }
        return ApiErrors.toAppError(ApiErrors.problemOf(status, failure.body))
    }

    private sealed interface StreamItem {
        data object Opened : StreamItem

        class Message(val type: String?, val data: String) : StreamItem

        data object Closed : StreamItem

        class Failed(val status: Int?, val body: String?, val cause: Throwable?) : StreamItem
    }

    /** Pasa lo que entrega okhttp-sse (en su hilo) al canal que atiende la corrutina. */
    private class Listener(private val items: Channel<StreamItem>) : EventSourceListener() {
        override fun onOpen(eventSource: EventSource, response: Response) {
            items.trySend(StreamItem.Opened)
        }

        override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
            // Bloquea el hilo de lectura si el dispositivo va atrasado (contrapresión).
            items.trySendBlocking(StreamItem.Message(type, data))
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
            items.trySend(StreamItem.Failed(response?.code, body, t))
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
        /** Sin ningún byte en 45 s (Go manda `: ping` cada 15 s) la conexión se da por muerta. */
        const val DEFAULT_READ_TIMEOUT_MILLIS: Long = 45_000

        private const val DEVICE_REVOKED = "DEVICE_REVOKED"
        private const val TEMPORARY_STATUS = 503

        const val READ_PATH = "ready-alerts"
        const val STREAM_PATH = "ready-alerts/stream"
        private const val EVENT_STREAM = "text/event-stream"
        private const val ALERTS = "alerts"
        private const val INTERRUPTED = "interrupted"
    }
}
