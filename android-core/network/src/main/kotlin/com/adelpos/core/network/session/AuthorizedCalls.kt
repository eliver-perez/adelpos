package com.adelpos.core.network.session

import com.adelpos.core.network.api.awaitResponse
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
