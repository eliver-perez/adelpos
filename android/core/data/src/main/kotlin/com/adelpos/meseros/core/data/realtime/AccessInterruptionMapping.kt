package com.adelpos.meseros.core.data.realtime

import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.realtime.AccessInterruption

/**
 * Qué interrupción de acceso corresponde a un rechazo de Go, o null si el rechazo no tiene que ver
 * con el dispositivo, la sesión o la versión (un 500, un 429, una falla de red…).
 */
object AccessInterruptionMapping {
    /** Para cualquier ruta (catálogo, imágenes): sin permiso no corta la sesión del mesero. */
    fun fromAppError(error: AppError): AccessInterruption? =
        when (error) {
            is AppError.DeviceRevoked -> AccessInterruption.DeviceRevoked
            is AppError.DeviceNotRecognized -> AccessInterruption.DeviceNotRecognized(error.code)
            is AppError.SessionEnded -> AccessInterruption.SessionEnded(error.code)
            is AppError.UpdateRequired -> AccessInterruption.UpdateRequired(error.code)
            else -> null
        }

    /**
     * Para el canal de eventos (`event: interrupted` o el rechazo al abrirlo): sin `events.read` el
     * canal no sirve y no se reconecta hasta que cambie la sesión.
     */
    fun forEvents(error: AppError): AccessInterruption? =
        when (error) {
            is AppError.PermissionDenied -> AccessInterruption.PermissionLost(error.code)
            is AppError.Rejected ->
                if (error.code == OFFLINE_CREDENTIAL_NOT_ALLOWED) {
                    AccessInterruption.SessionEnded(error.code)
                } else {
                    null
                }
            else -> fromAppError(error)
        }

    /** Código de Go cuando llega la credencial de envío donde no se acepta (etapa A1, fase B). */
    private const val OFFLINE_CREDENTIAL_NOT_ALLOWED = "OFFLINE_CREDENTIAL_NOT_ALLOWED"
}
