package local.adel.pos.android.core.data.api

import java.io.IOException
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.error.AppResult
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response

/**
 * Llamadas tipadas a la API v1 del servidor del restaurante (`<dirección>/api/v1/…`). Cada llamada
 * devuelve el valor o el [AppError] que le corresponde, nunca una excepción de red. En la Fase A
 * solo existe `GET version`; las rutas del emparejamiento, el PIN, el catálogo y las cuentas llegan
 * en las fases siguientes.
 *
 * @param baseUrl dirección del servidor (por ejemplo `https://10.0.2.2:8453/`).
 * @param client normalmente [PinnedHttpClient.create], con la huella fijada y las cabeceras del
 *   cliente.
 */
class ServerApi(
    baseUrl: HttpUrl,
    private val client: OkHttpClient,
    private val json: Json = ApiJson,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    private val apiRoot: HttpUrl = baseUrl.newBuilder().encodedPath("/api/v1/").build()

    /** Versión del servidor, versión mínima de cliente y funciones disponibles (`getVersion`). */
    suspend fun getVersion(): AppResult<VersionInfo> = get("version", VersionInfo.serializer())

    private suspend fun <T> get(
        path: String,
        deserializer: DeserializationStrategy<T>,
    ): AppResult<T> {
        val url = requireNotNull(apiRoot.resolve(path)) { "Ruta no válida: $path" }
        val request = Request.Builder().url(url).get().build()
        return execute(request, deserializer)
    }

    private suspend fun <T> execute(
        request: Request,
        deserializer: DeserializationStrategy<T>,
    ): AppResult<T> {
        val response =
            try {
                client.newCall(request).await()
            } catch (exception: IOException) {
                return AppResult.Failure(ApiErrors.fromException(exception))
            }
        return withContext(ioDispatcher) {
            response.use { answer ->
                val text =
                    try {
                        answer.body.string()
                    } catch (exception: IOException) {
                        return@withContext AppResult.Failure(ApiErrors.fromException(exception))
                    }
                if (!answer.isSuccessful) {
                    val problem = ApiErrors.problemOf(answer.code, text)
                    return@withContext AppResult.Failure(ApiErrors.toAppError(problem))
                }
                try {
                    AppResult.Success(json.decodeFromString(deserializer, text))
                } catch (_: SerializationException) {
                    AppResult.Failure(AppError.InvalidResponse(answer.code))
                } catch (_: IllegalArgumentException) {
                    AppResult.Failure(AppError.InvalidResponse(answer.code))
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
