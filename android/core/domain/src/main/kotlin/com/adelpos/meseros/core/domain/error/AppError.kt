package com.adelpos.meseros.core.domain.error

/**
 * Error de la aplicación al hablar con el servidor, ya traducido desde la respuesta de Go
 * (`{"error": {"code", "message", "request_id", "details"}}`) o desde la falla de red. Las
 * pantallas deciden por el tipo (y por [code]), nunca por el texto. [serverMessage] es el mensaje
 * en español de Go, pensado para mostrarse tal cual; [requestId] solo va en la sección de soporte.
 */
sealed interface AppError {
    val code: String?
    val serverMessage: String?
    val requestId: String?

    /**
     * El servidor exige una versión más reciente de esta aplicación (426 `CLIENT_UPDATE_REQUIRED`)
     * o no reconoce su versión o su tipo (400 `CLIENT_VERSION_INVALID`, `CLIENT_KIND_INVALID`): en
     * todos los casos, «Actualiza esta aplicación».
     */
    data class UpdateRequired(
        override val code: String,
        val minClientVersion: String?,
        val serverVersion: String?,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /**
     * El dispositivo fue retirado del restaurante (401 `DEVICE_REVOKED`): se borran sus datos
     * locales y su credencial (avisando antes de los pedidos sin enviar, Fase B).
     */
    data class DeviceRevoked(override val serverMessage: String?, override val requestId: String?) :
        AppError {
        override val code: String = "DEVICE_REVOKED"
    }

    /**
     * El servidor no reconoce la credencial del dispositivo (401 `DEVICE_REQUIRED` y otros
     * `DEVICE_*`).
     */
    data class DeviceNotRecognized(
        override val code: String,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /** La sesión del mesero terminó (401 `SESSION_*`): volver a entrar con el PIN. */
    data class SessionEnded(
        override val code: String,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /** El usuario no tiene el permiso (403 `PERMISSION_DENIED`). */
    data class PermissionDenied(
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError {
        override val code: String = "PERMISSION_DENIED"
    }

    /** Demasiados intentos (429): esperar unos minutos. */
    data class TryLater(
        override val code: String,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /** El servidor no pudo atender (5xx). */
    data class ServerProblem(
        val status: Int,
        override val code: String?,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /**
     * Cualquier otro rechazo con su código (409 `ACCOUNT_CONFLICT`, `ORDER_REQUIRES_CONFIRMATION`,
     * 400 `VALIDATION_ERROR`…): lo decide la pantalla que hizo la petición según [code].
     */
    data class Rejected(
        val status: Int,
        override val code: String,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /**
     * La licencia, un cupo o la hora de la computadora principal no permiten la operación (etapa
     * L1: 409 con uno de los cinco códigos de [LicenseRefusal.CODES]). Go no guardó nada. El texto
     * que se muestra sale de [refusal] con [LicenseMessages] (también con [shownMessage]), nunca de
     * [serverMessage].
     */
    data class LicenseRefused(
        val refusal: LicenseRefusal,
        override val code: String,
        override val serverMessage: String?,
        override val requestId: String?,
    ) : AppError

    /** No se pudo llegar al servidor (sin red, apagado o sin respuesta a tiempo). */
    data object Unreachable : AppError {
        override val code: String? = null
        override val serverMessage: String? = null
        override val requestId: String? = null
    }

    /**
     * El servidor presentó un certificado con otra huella: no es el servidor emparejado (o cambió
     * su identidad). Nunca se continúa; se avisa y se ofrece emparejar de nuevo (§4).
     */
    data object ServerIdentityChanged : AppError {
        override val code: String? = null
        override val serverMessage: String? = null
        override val requestId: String? = null
    }

    /** La respuesta no tiene la forma que espera esta versión de la aplicación. */
    data class InvalidResponse(val status: Int) : AppError {
        override val code: String? = null
        override val serverMessage: String? = null
        override val requestId: String? = null
    }
}

/** Resultado de una operación con el servidor. */
sealed interface AppResult<out T> {
    data class Success<T>(val value: T) : AppResult<T>

    data class Failure(val error: AppError) : AppResult<Nothing>
}
