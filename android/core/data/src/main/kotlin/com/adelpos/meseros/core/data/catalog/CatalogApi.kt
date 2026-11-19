package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.data.api.ApiErrors
import com.adelpos.meseros.core.data.session.AuthorizedServer
import com.adelpos.meseros.core.data.session.apiUrl
import com.adelpos.meseros.core.data.session.execute
import com.adelpos.meseros.core.data.session.request
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.error.AppResult
import java.io.IOException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Response

/** Resultado de pedir el contenido de una imagen. */
sealed interface ImageDownload {
    /** Go respondió 304: la copia guardada con esa `ETag` sigue siendo la buena. */
    data object NotModified : ImageDownload

    /** El contenido nuevo con su `ETag`. */
    class Content(val bytes: ByteArray, val etag: String?) : ImageDownload
}

/**
 * Rutas del catálogo de la API v1 que usa el dispositivo: `getCatalogSync` y
 * `getCatalogImageContent`. Devuelven el valor o el [AppError] de Go; nunca una excepción de red.
 */
class CatalogApi(private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO) {
    /**
     * `GET catalog/sync`: completo sin [since] (o con 0); con [since] y [epoch], solo los cambios.
     * Devuelve el cuerpo sin interpretar (se lee fuera del hilo principal en [CatalogPayload]).
     */
    suspend fun sync(server: AuthorizedServer, since: Long?, epoch: String?): AppResult<String> {
        val url = server.apiUrl("catalog/sync").newBuilder()
        if (since != null && since > 0 && epoch != null) {
            url.addQueryParameter("since", since.toString())
            url.addQueryParameter("epoch", epoch)
        }
        val request = server.request(url.build()).get().build()
        return call(server, request) { response, body -> AppResult.Success(body.decodeToString()) }
    }

    /**
     * `GET catalog/images/{id}/content`: con [etag] de una copia guardada, Go responde 304 si sigue
     * igual. El contenido se limita a [maximumBytes] (Go acepta imágenes de hasta 1 MiB).
     */
    suspend fun image(
        server: AuthorizedServer,
        imageId: String,
        etag: String?,
        maximumBytes: Long = MAXIMUM_IMAGE_BYTES,
    ): AppResult<ImageDownload> {
        val builder = server.request(server.apiUrl("catalog/images/$imageId/content")).get()
        if (etag != null) {
            builder.header("If-None-Match", etag)
        }
        val request = builder.build()
        val response =
            try {
                server.execute(request)
            } catch (exception: IOException) {
                return AppResult.Failure(ApiErrors.fromException(exception))
            }
        return withContext(ioDispatcher) {
            response.use { answer ->
                if (answer.code == HTTP_NOT_MODIFIED) {
                    return@withContext AppResult.Success(ImageDownload.NotModified)
                }
                readBody(answer, maximumBytes) { body ->
                    AppResult.Success(ImageDownload.Content(body, answer.header("ETag")))
                }
            }
        }
    }

    private suspend fun <T> call(
        server: AuthorizedServer,
        request: okhttp3.Request,
        onSuccess: (Response, ByteArray) -> AppResult<T>,
    ): AppResult<T> {
        val response =
            try {
                server.execute(request)
            } catch (exception: IOException) {
                return AppResult.Failure(ApiErrors.fromException(exception))
            }
        return withContext(ioDispatcher) {
            response.use { answer ->
                readBody(answer, Long.MAX_VALUE) { body -> onSuccess(answer, body) }
            }
        }
    }

    private fun <T> readBody(
        response: Response,
        maximumBytes: Long,
        onSuccess: (ByteArray) -> AppResult<T>,
    ): AppResult<T> {
        val declared = response.body.contentLength()
        if (response.isSuccessful && declared > maximumBytes) {
            return AppResult.Failure(AppError.InvalidResponse(response.code))
        }
        val bytes =
            try {
                response.body.bytes()
            } catch (exception: IOException) {
                return AppResult.Failure(ApiErrors.fromException(exception))
            }
        if (!response.isSuccessful) {
            val problem = ApiErrors.problemOf(response.code, bytes.decodeToString())
            return AppResult.Failure(ApiErrors.toAppError(problem))
        }
        if (bytes.size > maximumBytes) {
            return AppResult.Failure(AppError.InvalidResponse(response.code))
        }
        return onSuccess(bytes)
    }

    companion object {
        private const val HTTP_NOT_MODIFIED = 304

        /** Tope de una imagen descargada: el doble de lo que acepta Go (1 MiB). */
        const val MAXIMUM_IMAGE_BYTES: Long = 2L * 1024 * 1024
    }
}
