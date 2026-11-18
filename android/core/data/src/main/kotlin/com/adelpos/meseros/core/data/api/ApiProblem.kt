package com.adelpos.meseros.core.data.api

import com.adelpos.meseros.core.data.security.PinMismatchException
import com.adelpos.meseros.core.domain.error.AppError
import java.io.IOException
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Rechazo del servidor tal como llega: `{"error": {"code", "message", "request_id", "details"}}`
 * (forma de `backend/internal/httpapi/router.go`, `fail` y `failWithDetails`). [details] solo
 * existe en algunos códigos (por ejemplo, `min_client_version` en el 426 o `result` en
 * `OFFLINE_ORDER_ALREADY_ACCEPTED`); las fases que lo necesiten lo leen de aquí antes de
 * convertirlo con [toAppError].
 */
data class ApiProblem(
    val status: Int,
    val code: String?,
    val message: String?,
    val requestId: String?,
    val details: JsonObject?,
)

@Serializable private data class ErrorEnvelope(val error: ErrorBody)

@Serializable
private data class ErrorBody(
    val code: String,
    val message: String? = null,
    @SerialName("request_id") val requestId: String? = null,
    val details: JsonElement? = null,
)

/** Correspondencia entre las respuestas de error de Go y los errores de la aplicación. */
object ApiErrors {
    /**
     * Lee el cuerpo de una respuesta de error; si no tiene la forma de Go (un proxy, una página),
     * queda sin código.
     */
    fun problemOf(status: Int, body: String?): ApiProblem {
        val parsed =
            try {
                body?.let { ApiJson.decodeFromString(ErrorEnvelope.serializer(), it).error }
            } catch (_: SerializationException) {
                null
            } catch (_: IllegalArgumentException) {
                null
            }
        return ApiProblem(
            status = status,
            code = parsed?.code,
            message = parsed?.message,
            requestId = parsed?.requestId,
            details = parsed?.details as? JsonObject,
        )
    }

    /**
     * Error de la aplicación que corresponde a un rechazo del servidor. La pantalla decide por tipo
     * y código.
     */
    fun toAppError(problem: ApiProblem): AppError {
        val code = problem.code
        val message = problem.message
        val requestId = problem.requestId
        return when {
            problem.status == 426 || code in UPDATE_CODES ->
                AppError.UpdateRequired(
                    code = code ?: "CLIENT_UPDATE_REQUIRED",
                    minClientVersion = problem.details.text("min_client_version"),
                    serverVersion = problem.details.text("server_version"),
                    serverMessage = message,
                    requestId = requestId,
                )
            code == "DEVICE_REVOKED" -> AppError.DeviceRevoked(message, requestId)
            code != null && code.startsWith("DEVICE_") ->
                AppError.DeviceNotRecognized(code, message, requestId)
            code != null && code.startsWith("SESSION_") ->
                AppError.SessionEnded(code, message, requestId)
            code == "PERMISSION_DENIED" -> AppError.PermissionDenied(message, requestId)
            problem.status == 429 ->
                AppError.TryLater(code ?: "TOO_MANY_REQUESTS", message, requestId)
            problem.status >= 500 ->
                AppError.ServerProblem(problem.status, code, message, requestId)
            code == null -> AppError.InvalidResponse(problem.status)
            else -> AppError.Rejected(problem.status, code, message, requestId)
        }
    }

    /**
     * Error de la aplicación para una petición que no recibió respuesta. Un certificado con otra
     * huella es un caso aparte: no es una falla de red sino otro servidor (o un servidor que cambió
     * de identidad), y nunca se reintenta.
     */
    fun fromException(exception: IOException): AppError {
        if (exception.involves<PinMismatchException>()) {
            return AppError.ServerIdentityChanged
        }
        return AppError.Unreachable
    }

    private val UPDATE_CODES =
        setOf("CLIENT_UPDATE_REQUIRED", "CLIENT_VERSION_INVALID", "CLIENT_KIND_INVALID")

    private fun JsonObject?.text(key: String): String? {
        val value = this?.get(key) as? JsonPrimitive ?: return null
        return if (value.isString) value.content else null
    }

    // Busca el tipo en las causas y en las excepciones suprimidas: OkHttp puede intentar varias
    // direcciones de un mismo nombre y lanzar la primera falla con las demás como suprimidas.
    private inline fun <reified T : Throwable> Throwable.involves(): Boolean {
        val pending = ArrayDeque<Throwable>()
        val seen = mutableSetOf<Throwable>()
        pending.add(this)
        while (pending.isNotEmpty()) {
            val current = pending.removeFirst()
            if (!seen.add(current)) {
                continue
            }
            if (current is T) {
                return true
            }
            current.cause?.let { pending.add(it) }
            pending.addAll(current.suppressed)
        }
        return false
    }
}
