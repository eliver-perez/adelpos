package com.adelpos.meseros.core.data.session

import java.io.IOException
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.Request
import okhttp3.Response

/** Dirección de una ruta de la API v1 (`<servidor>/api/v1/<ruta>`) en este servidor. */
fun AuthorizedServer.apiUrl(path: String): HttpUrl =
    requireNotNull(baseUrl.newBuilder().encodedPath("/api/v1/").build().resolve(path)) {
        "Ruta no válida: $path"
    }

/** Petición a la API v1 con las credenciales del dispositivo y del mesero. */
fun AuthorizedServer.request(url: HttpUrl): Request.Builder =
    Request.Builder().url(url).headers(credentials)

/**
 * Ejecuta la petición sin bloquear el hilo; cancelar la corrutina cancela la petición. Lanza
 * `IOException` si no hubo respuesta.
 */
suspend fun AuthorizedServer.execute(request: Request): Response =
    client.newCall(request).awaitResponse()

private suspend fun Call.awaitResponse(): Response = suspendCancellableCoroutine { continuation ->
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
