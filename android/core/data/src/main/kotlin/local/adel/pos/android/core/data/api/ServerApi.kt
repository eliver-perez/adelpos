package local.adel.pos.android.core.data.api

import java.io.IOException
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.SerializationStrategy
import kotlinx.serialization.json.Json
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.error.AppResult
import okhttp3.Call
import okhttp3.Callback
import okhttp3.EventListener
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

/**
 * Resultado de una petición a Go con todo lo que la pantalla puede necesitar: el valor, el rechazo
 * con sus `details` (por ejemplo `locked_until` de `PIN_LOCKED`), la falta de respuesta (y si la
 * petición llegó a salir, para saber si se puede repetir en otra dirección sin duplicar nada) o una
 * respuesta que esta versión no entiende.
 */
sealed interface ApiOutcome<out T> {
    data class Success<T>(val value: T) : ApiOutcome<T>

    data class Rejected(val problem: ApiProblem) : ApiOutcome<Nothing>

    /**
     * Sin respuesta. [requestSent] es falso si la petición no salió (no se pudo conectar o el
     * certificado no era el fijado): repetirla en otra dirección no puede duplicar nada.
     */
    data class NoAnswer(val error: AppError, val requestSent: Boolean) : ApiOutcome<Nothing>

    data class Malformed(val status: Int) : ApiOutcome<Nothing>

    /** El mismo resultado como [AppResult], con el error ya traducido. */
    fun toAppResult(): AppResult<T> =
        when (this) {
            is Success -> AppResult.Success(value)
            is Rejected -> AppResult.Failure(ApiErrors.toAppError(problem))
            is NoAnswer -> AppResult.Failure(error)
            is Malformed -> AppResult.Failure(AppError.InvalidResponse(status))
        }
}

/**
 * Llamadas tipadas a la API v1 del servidor del restaurante (`<dirección>/api/v1/…`). Cada llamada
 * devuelve el valor o el error que le corresponde, nunca una excepción de red.
 *
 * @param baseUrl dirección del servidor (por ejemplo `https://10.0.2.2:8453/`).
 * @param client normalmente [PinnedHttpClient.create], con la huella fijada y las cabeceras del
 *   cliente.
 */
class ServerApi(
    baseUrl: HttpUrl,
    client: OkHttpClient,
    private val json: Json = ApiJson,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    private val apiRoot: HttpUrl = baseUrl.newBuilder().encodedPath("/api/v1/").build()

    // Comparte conexiones y hilos con el cliente original; solo agrega el aviso de «la petición ya
    // salió» (ver RequestSent).
    private val client: OkHttpClient =
        client.newBuilder().eventListenerFactory(RequestSent.listenerFactory).build()

    /** Versión del servidor, versión mínima de cliente y funciones disponibles (`getVersion`). */
    suspend fun getVersion(): AppResult<VersionInfo> =
        get("version", VersionInfo.serializer()).toAppResult()

    /** GET a [path] (relativa a `/api/v1/`) con [headers]. */
    suspend fun <T> get(
        path: String,
        deserializer: DeserializationStrategy<T>,
        headers: Headers = Headers.headersOf(),
    ): ApiOutcome<T> {
        val request = Request.Builder().url(resolve(path)).headers(headers).get()
        return execute(request, deserializer)
    }

    /** POST de [body] en JSON a [path] (relativa a `/api/v1/`) con [headers]. */
    suspend fun <B, T> post(
        path: String,
        body: B,
        serializer: SerializationStrategy<B>,
        deserializer: DeserializationStrategy<T>,
        headers: Headers = Headers.headersOf(),
    ): ApiOutcome<T> {
        val text = json.encodeToString(serializer, body)
        val request =
            Request.Builder()
                .url(resolve(path))
                .headers(headers)
                .post(text.toRequestBody(JSON_MEDIA_TYPE))
        return execute(request, deserializer)
    }

    private fun resolve(path: String): HttpUrl =
        requireNotNull(apiRoot.resolve(path)) { "Ruta no válida: $path" }

    private suspend fun <T> execute(
        builder: Request.Builder,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> {
        val sent = RequestSent()
        val request = builder.tag(RequestSent::class.java, sent).build()
        val response =
            try {
                client.newCall(request).await()
            } catch (exception: IOException) {
                return ApiOutcome.NoAnswer(ApiErrors.fromException(exception), sent.value)
            }
        return withContext(ioDispatcher) {
            response.use { answer ->
                val text =
                    try {
                        answer.body.string()
                    } catch (exception: IOException) {
                        return@withContext ApiOutcome.NoAnswer(
                            ApiErrors.fromException(exception),
                            requestSent = true,
                        )
                    }
                if (!answer.isSuccessful) {
                    return@withContext ApiOutcome.Rejected(ApiErrors.problemOf(answer.code, text))
                }
                try {
                    ApiOutcome.Success(json.decodeFromString(deserializer, text))
                } catch (_: SerializationException) {
                    ApiOutcome.Malformed(answer.code)
                } catch (_: IllegalArgumentException) {
                    ApiOutcome.Malformed(answer.code)
                }
            }
        }
    }

    private companion object {
        val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
    }
}

/**
 * Marca de una petición que ya empezó a enviarse al servidor (sus cabeceras salieron por una
 * conexión ya establecida y verificada). Si la conexión o el intercambio TLS fallan antes, la marca
 * sigue en falso: el servidor nunca la recibió.
 */
internal class RequestSent {
    @Volatile
    var value: Boolean = false
        private set

    companion object {
        val listenerFactory = EventListener.Factory { call ->
            val marker = call.request().tag(RequestSent::class.java)
            if (marker == null) {
                EventListener.NONE
            } else {
                object : EventListener() {
                    override fun requestHeadersStart(call: Call) {
                        marker.value = true
                    }
                }
            }
        }
    }
}

/** Ejecuta la llamada sin bloquear el hilo; cancelar la corrutina cancela la petición. */
private suspend fun Call.await(): Response = suspendCancellableCoroutine { continuation ->
    continuation.invokeOnCancellation { cancel() }
    enqueue(
        object : Callback {
            override fun onFailure(call: Call, e: IOException) {
                continuation.resumeWithException(e)
            }

            override fun onResponse(call: Call, response: Response) {
                continuation.resume(response) { _, value, _ -> value.close() }
            }
        },
    )
}
