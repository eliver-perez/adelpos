package local.adel.pos.android.core.data.waiter

import java.io.IOException
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.SerializationStrategy
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiJson
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.RequestSent
import local.adel.pos.android.core.data.catalog.CustomizationJson
import local.adel.pos.android.core.data.session.ServerTransport
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

/**
 * Solo para pruebas: decide si se descarta la respuesta de una petición **después** de que Go la
 * procesó (la «respuesta perdida» de §9), para comprobar que el reintento con la misma clave nunca
 * duplica. En la aplicación de entrega no existe ([WaiterApi] la recibe en null).
 */
fun interface LostResponses {
    fun shouldDrop(method: String, path: String): Boolean
}

/**
 * Rutas del modo Mesero de la API v1 con las cabeceras del mesero dueño de la operación. Cada
 * llamada devuelve el valor o el rechazo de Go con sus `details` ([ApiOutcome]); nunca una
 * excepción de red.
 */
class WaiterApi(
    private val lostResponses: LostResponses? = null,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    suspend fun overview(server: ServerTransport, headers: Headers) =
        get(server, headers, "restaurant/overview", RestaurantOverviewDto.serializer())

    suspend fun state(server: ServerTransport, headers: Headers) =
        get(server, headers, "restaurant/state", RecoveryStateDto.serializer())

    suspend fun bill(server: ServerTransport, headers: Headers, accountId: String) =
        get(server, headers, "restaurant/accounts/$accountId", BillDto.serializer())

    suspend fun openAccount(server: ServerTransport, headers: Headers, body: OpenAccountBody) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = "restaurant/accounts",
            body = body,
            serializer = OpenAccountBody.serializer(),
            deserializer = BillDto.serializer(),
        )

    suspend fun saveDraft(
        server: ServerTransport,
        headers: Headers,
        accountId: String,
        body: SaveDraftBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "PUT",
            path = "restaurant/accounts/$accountId/draft",
            body = body,
            serializer = SaveDraftBody.serializer(),
            deserializer = BillDto.serializer(),
        )

    suspend fun previewDraft(server: ServerTransport, headers: Headers, accountId: String) =
        get(server, headers, "restaurant/accounts/$accountId/preview", QuoteResultDto.serializer())

    suspend fun confirmDraft(
        server: ServerTransport,
        headers: Headers,
        accountId: String,
        body: ConfirmDraftBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = "restaurant/accounts/$accountId/confirm",
            body = body,
            serializer = ConfirmDraftBody.serializer(),
            deserializer = BillDto.serializer(),
        )

    suspend fun cancelUnits(
        server: ServerTransport,
        headers: Headers,
        accountId: String,
        body: CancelUnitsBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = "restaurant/accounts/$accountId/cancel",
            body = body,
            serializer = CancelUnitsBody.serializer(),
            deserializer = BillDto.serializer(),
        )

    suspend fun previewEdit(
        server: ServerTransport,
        headers: Headers,
        accountId: String,
        body: EditPreviewBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = "restaurant/accounts/$accountId/edit-preview",
            body = body,
            serializer = EditPreviewBody.serializer(),
            deserializer = QuoteResultDto.serializer(),
        )

    suspend fun editUnits(
        server: ServerTransport,
        headers: Headers,
        accountId: String,
        body: EditUnitsBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = "restaurant/accounts/$accountId/edit",
            body = body,
            serializer = EditUnitsBody.serializer(),
            deserializer = BillDto.serializer(),
        )

    /** `submitOfflineOrder`, con la sesión o con la credencial de envío del mesero. */
    suspend fun submitOfflineOrder(
        server: ServerTransport,
        headers: Headers,
        body: OfflineOrderBody,
    ) =
        send(
            server = server,
            headers = headers,
            method = "POST",
            path = OFFLINE_ORDERS_PATH,
            body = body,
            serializer = OfflineOrderBody.serializer(),
            deserializer = OfflineOrderResultDto.serializer(),
        )

    private suspend fun <T> get(
        server: ServerTransport,
        headers: Headers,
        path: String,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> {
        val request = Request.Builder().url(url(server, path)).headers(headers).get()
        return execute(server, request, deserializer)
    }

    private suspend fun <B, T> send(
        server: ServerTransport,
        headers: Headers,
        method: String,
        path: String,
        body: B,
        serializer: SerializationStrategy<B>,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> {
        val text = CustomizationJson.encodeToString(serializer, body)
        val request =
            Request.Builder()
                .url(url(server, path))
                .headers(headers)
                .method(method, text.toRequestBody(JSON_MEDIA_TYPE))
        return execute(server, request, deserializer)
    }

    private fun url(server: ServerTransport, path: String): HttpUrl {
        val root = server.baseUrl.newBuilder().encodedPath("/api/v1/").build()
        return requireNotNull(root.resolve(path)) { "Ruta no válida: $path" }
    }

    private suspend fun <T> execute(
        server: ServerTransport,
        builder: Request.Builder,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> {
        val sent = RequestSent()
        val request = builder.tag(RequestSent::class.java, sent).build()
        val response =
            try {
                clientFor(server).newCall(request).await()
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
                    ApiOutcome.Success(ApiJson.decodeFromString(deserializer, text))
                } catch (_: SerializationException) {
                    ApiOutcome.Malformed(answer.code)
                } catch (_: IllegalArgumentException) {
                    ApiOutcome.Malformed(answer.code)
                }
            }
        }
    }

    // El cliente del servidor emparejado (huella fijada) con el aviso de «la petición ya salió» y,
    // solo en pruebas, el descarte de respuestas.
    private fun clientFor(server: ServerTransport): OkHttpClient {
        val builder = server.client.newBuilder().eventListenerFactory(RequestSent.listenerFactory)
        val lost = lostResponses
        if (lost != null) {
            builder.addInterceptor(LostResponseInterceptor(lost))
        }
        return builder.build()
    }

    private class LostResponseInterceptor(private val lost: LostResponses) : Interceptor {
        override fun intercept(chain: Interceptor.Chain): Response {
            val request = chain.request()
            val response = chain.proceed(request)
            val path = request.url.encodedPath.removePrefix("/api/v1/")
            if (lost.shouldDrop(request.method, path)) {
                // Go ya procesó la petición: se lee y se tira la respuesta, como si la red se
                // hubiera caído antes de que llegara.
                response.close()
                throw IOException("Respuesta descartada por la prueba")
            }
            return response
        }
    }

    companion object {
        const val OFFLINE_ORDERS_PATH = "restaurant/offline-orders"

        private val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
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
