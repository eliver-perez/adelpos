package com.adelpos.meseros.core.domain.ready

import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.realtime.ReconnectBackoff

/**
 * Cuándo arranca y cuándo se detiene el servicio de avisos (§4.3 y §0.5.5 de la Fase 0, A2-D8).
 *
 * - **Arranca** al entrar un mesero con conexión (de ahí sale «entró en el día»), al abrir la
 *   aplicación si lo último que dijo Go fue `active: true` y aún no termina el día comercial, y al
 *   reiniciar el equipo o actualizar la aplicación con esa misma condición guardada.
 * - **Se detiene** con `active: false`, al terminar el día comercial sin poder volver a preguntar,
 *   al retirar el dispositivo o con «Emparejar de nuevo». Sin destinatarios en el día no arranca:
 *   no hay notificación permanente.
 */
object ReadyAlertsLifecycle {
    /**
     * Margen después de `day_ends_at` para volver a preguntar a Go antes de detenerse sin su
     * respuesta (la red puede estar cortada justo a la hora de corte).
     */
    const val DAY_END_GRACE_MILLIS: Long = 10 * 60_000L

    /** Fallas seguidas tras las que se pide buscar el servidor en otra dirección (cambio de IP). */
    const val RELOCATE_AFTER_FAILURES: Int = 3

    /** Espera mínima entre dos búsquedas de otra dirección. */
    const val RELOCATE_EVERY_MILLIS: Long = 60_000L

    /** Espera creciente entre reconexiones: 2, 4, 8, 16 y 30 segundos (§0.5.5). */
    fun backoff(): ReconnectBackoff =
        ReconnectBackoff(initialMillis = 2_000, maximumMillis = 30_000)

    /** Por qué se pide arrancar el servicio. */
    enum class Trigger {
        /** Un mesero acaba de entrar con conexión: Go ya tiene su sesión con PIN de hoy. */
        ONLINE_ENTRY,

        /** La aplicación se abrió (o volvió a primer plano) con el dispositivo listo. */
        OPENED,

        /**
         * Reinicio del equipo o aplicación actualizada (`BOOT_COMPLETED`, `MY_PACKAGE_REPLACED`).
         */
        BOOT,
    }

    /** Qué hacer ante un pedido de arranque. */
    enum class StartDecision {
        /** Arrancar el servicio ya (desde primer plano o con su exención). */
        START,

        /** Preguntar antes a Go (`GET ready-alerts`) y arrancar solo si dice `active: true`. */
        ASK_FIRST,

        /** No arrancar. */
        NONE,
    }

    /**
     * Con lo guardado ([stored]; null si nunca se preguntó):
     * - al entrar con conexión, arrancar salvo que el aviso se sepa apagado en Terminales (entonces
     *   preguntar: pudo volver a activarse);
     * - al abrir la aplicación, arrancar si Go dijo `active: true` y no ha terminado el día; si no,
     *   preguntar;
     * - tras reiniciar el equipo, solo con esa misma condición guardada (la red puede no estar
     *   lista y no hay pantalla).
     */
    fun decide(trigger: Trigger, stored: ReadyAlertsStatus?, nowMillis: Long): StartDecision {
        val activeToday = stored != null && stored.active && nowMillis < stored.dayEndsAtMillis
        return when (trigger) {
            Trigger.ONLINE_ENTRY ->
                if (stored != null && !stored.settings.enabled) {
                    StartDecision.ASK_FIRST
                } else {
                    StartDecision.START
                }
            Trigger.OPENED ->
                if (activeToday) {
                    StartDecision.START
                } else {
                    StartDecision.ASK_FIRST
                }
            Trigger.BOOT ->
                if (activeToday) {
                    StartDecision.START
                } else {
                    StartDecision.NONE
                }
        }
    }

    /**
     * Terminó el día comercial y no se pudo volver a preguntar: pasó [DAY_END_GRACE_MILLIS] desde
     * [dayEndsAtMillis] y la última lectura buena de Go ([lastReadMillis], null si ninguna) es de
     * antes. Mientras Go responda, es él quien dice si sigue activo (un mesero con su sesión
     * vigente) con el `day_ends_at` del día siguiente.
     */
    fun dayEnded(dayEndsAtMillis: Long, lastReadMillis: Long?, nowMillis: Long): Boolean =
        nowMillis >= dayEndsAtMillis + DAY_END_GRACE_MILLIS &&
            (lastReadMillis == null || lastReadMillis < dayEndsAtMillis)

    /** Qué hacer ante un rechazo o una falla del canal de avisos. */
    enum class ErrorAction {
        /**
         * La credencial del dispositivo dejó de valer (retirado, no reconocido), el servidor tiene
         * otra identidad o pide actualizar la aplicación: detener el servicio y avisar a la entrada
         * al dispositivo (`DeviceAccess.report`), que hace lo de siempre.
         */
        STOP_AND_REPORT,

        /** Un rechazo que no se arregla reintentando (no es `android`, petición inválida). */
        STOP,

        /**
         * El dispositivo ya tiene su canal abierto (429 `READY_ALERTS_CONNECTION_LIMIT`, casi
         * siempre el anterior que Go todavía no cierra): esperar y, mientras, leer con `GET`.
         */
        WAIT_AND_READ,

        /** Sin red, 5xx o respuesta que no se entiende: reintentar con espera creciente. */
        RETRY,
    }

    fun onError(error: AppError): ErrorAction =
        when (error) {
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired,
            AppError.ServerIdentityChanged -> ErrorAction.STOP_AND_REPORT
            is AppError.TryLater -> ErrorAction.WAIT_AND_READ
            is AppError.PermissionDenied -> ErrorAction.STOP
            is AppError.Rejected ->
                if (error.status == 403 || error.status == 400 || error.status == 404) {
                    ErrorAction.STOP
                } else {
                    ErrorAction.RETRY
                }
            // Una sesión no se usa en este canal (solo la credencial del dispositivo).
            is AppError.SessionEnded -> ErrorAction.RETRY
            is AppError.ServerProblem,
            is AppError.InvalidResponse,
            AppError.Unreachable -> ErrorAction.RETRY
        }
}

/**
 * Canales de notificaciones (A2-D7): «AD-EL POS conectado» (baja importancia, la notificación
 * permanente) y **un solo canal visible «Platillos listos»**, cuyo identificador depende de la
 * combinación de sonido y vibración de Terminales, porque un canal ya creado no se puede cambiar.
 * Al cambiar la combinación se crea el nuevo y se borran los otros: en Ajustes de Android siempre
 * hay uno solo con ese nombre (si se vuelve a una combinación anterior, Android recupera lo que el
 * usuario le haya ajustado).
 */
object ReadyChannels {
    const val CONNECTED = "conectado"
    const val SOUND_AND_VIBRATION = "listos_sonido_vibracion"
    const val SOUND_ONLY = "listos_sonido"
    const val VIBRATION_ONLY = "listos_vibracion"
    const val SILENT = "listos_silencio"

    /** Los cuatro identificadores posibles de «Platillos listos». */
    val READY: List<String> = listOf(SOUND_AND_VIBRATION, SOUND_ONLY, VIBRATION_ONLY, SILENT)

    fun readyChannel(settings: ReadyAlertSettings): String =
        when {
            settings.sound && settings.vibration -> SOUND_AND_VIBRATION
            settings.sound -> SOUND_ONLY
            settings.vibration -> VIBRATION_ONLY
            else -> SILENT
        }

    /** Los canales «Platillos listos» que sobran con [settings] (se borran). */
    fun stale(settings: ReadyAlertSettings): List<String> = READY - readyChannel(settings)
}
