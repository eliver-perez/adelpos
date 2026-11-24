package com.adelpos.meseros.core.domain.realtime

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.transform

/**
 * Un evento del canal de tiempo real de Go (`EventBatch.events`): solo dice **qué** cambió (tipo,
 * ámbito y agregado), nunca importes. Quien lo recibe vuelve a consultar al servidor.
 */
data class RealtimeEvent(
    val sequence: Long,
    val id: String,
    val version: Int,
    val kind: String,
    val scope: String,
    val stationId: String?,
    val aggregateId: String?,
    val createdAt: String?,
)

/** Ámbitos del canal que usa la aplicación (`events/store.go`, `allowed`). */
object RealtimeScopes {
    const val CATALOG = "catalog"
    const val RESTAURANT = "restaurant"
    const val TAKEAWAY = "takeaway"

    /** Etapa L1: el de `license.changed` (llega a todos, sin importar sus permisos). */
    const val CONFIGURATION = "configuration"
}

/** Tipos de evento conocidos; un tipo que no se conoce se ignora (contrato v1). */
object RealtimeKinds {
    const val CATALOG_CHANGED = "catalog.changed"
    const val PRODUCT_SOLD_OUT = "catalog.product.sold_out"
    const val PRODUCT_AVAILABLE = "catalog.product.available"
    const val ACCOUNT_PREPARATION_CHANGED = "restaurant.account.preparation_changed"

    /**
     * Etapa L1: cambió el estado de la licencia o el bloqueo por la hora de la computadora
     * principal; sin datos: se vuelve a leer `restaurant/state` o `auth/me`.
     */
    const val LICENSE_CHANGED = "license.changed"
}

/** Por qué hay que reconstruir el estado en lugar de seguir evento por evento. */
enum class RebuildReason {
    /** Go respondió `reset: true`: el cursor quedó fuera de su ventana de 5000 eventos. */
    SERVER_RESET,

    /**
     * El canal se abrió de nuevo (primera vez, sesión nueva o al volver de segundo plano) desde el
     * cursor de `restaurant/state` o sin cursor: lo ocurrido antes no llegó por eventos.
     */
    FRESH_START,
}

/**
 * Por qué Go cerró el canal o lo rechazó: el dispositivo, la sesión o los permisos ya no sirven. Lo
 * recibe [AccessInterruptions] (un solo punto, que conecta la entrada con PIN y el emparejamiento).
 */
sealed interface AccessInterruption {
    val code: String

    /** El dispositivo fue retirado del restaurante (`DEVICE_REVOKED`): borrar datos locales. */
    data object DeviceRevoked : AccessInterruption {
        override val code: String = "DEVICE_REVOKED"
    }

    /** El servidor no reconoce la credencial del dispositivo (otros `DEVICE_*`). */
    data class DeviceNotRecognized(override val code: String) : AccessInterruption

    /** La sesión del mesero terminó (`SESSION_*`): volver a la pantalla de entrada con PIN. */
    data class SessionEnded(override val code: String) : AccessInterruption

    /** El mesero perdió un permiso que se necesita (`PERMISSION_DENIED`). */
    data class PermissionLost(override val code: String) : AccessInterruption

    /** El servidor exige una versión más reciente de la aplicación (426). */
    data class UpdateRequired(override val code: String) : AccessInterruption
}

/**
 * Punto único al que se avisan las interrupciones de acceso (lo conecta la entrada de las fases B y
 * C con `AccessRepository.report`). [userId] es el mesero dueño de la sesión con la que se hizo la
 * petición o se abrió el canal (null si no se sabe): una sesión terminada solo saca a ese mesero.
 */
fun interface AccessInterruptions {
    fun onInterrupted(interruption: AccessInterruption, userId: String?)
}

/** Lo que entrega el canal de tiempo real a sus suscriptores. */
sealed interface RealtimeSignal {
    /** Eventos nuevos (solo cuando hay alguno; los mensajes vacíos de cada 750 ms no se emiten). */
    data class Changes(val cursor: Long, val events: List<RealtimeEvent>) : RealtimeSignal

    /**
     * Hay que volver a consultar todo (estado del restaurante y catálogo) porque no se puede saber
     * qué eventos faltan.
     */
    data class Rebuild(val cursor: Long, val reason: RebuildReason) : RealtimeSignal

    /** Go cortó el canal por el acceso (dispositivo, sesión, permisos o versión). */
    data class Interrupted(val interruption: AccessInterruption) : RealtimeSignal
}

/** Estado de la conexión del canal (para pruebas y la sección de soporte). */
sealed interface RealtimeState {
    /** En segundo plano o sin sesión: no hay conexión abierta. */
    data object Stopped : RealtimeState

    data object Connecting : RealtimeState

    data class Connected(val cursor: Long) : RealtimeState

    /** Esperando para reconectar después de [attempt] fallas seguidas. */
    data class Waiting(val attempt: Int, val delayMillis: Long) : RealtimeState

    /** Go cortó por el acceso; no se reconecta hasta que cambie la sesión. */
    data class Interrupted(val interruption: AccessInterruption) : RealtimeState
}

/**
 * Canal de tiempo real compartido por toda la aplicación: **una sola conexión por proceso** (Go
 * admite dos por dispositivo). La Fase E se suscribe a los eventos de `restaurant`/`takeaway` con
 * [events] y a [rebuilds]; el catálogo, a los de `catalog`.
 */
interface RealtimeFeed {
    val signals: SharedFlow<RealtimeSignal>

    val state: StateFlow<RealtimeState>

    /** Pide reconectar ya, sin esperar (por ejemplo, al recuperar la red). */
    fun reconnectNow()
}

/** Eventos de un ámbito y, si se indican, solo de esos tipos. */
fun RealtimeFeed.events(scope: String, kinds: Set<String> = emptySet()): Flow<RealtimeEvent> =
    signals
        .filterIsInstance<RealtimeSignal.Changes>()
        .transform { changes ->
            for (event in changes.events) {
                emit(event)
            }
        }
        .filter { event -> event.scope == scope && (kinds.isEmpty() || event.kind in kinds) }

/** Avisos de «reconstruir todo» (reset del servidor o canal abierto de nuevo). */
fun RealtimeFeed.rebuilds(): Flow<RealtimeSignal.Rebuild> =
    signals.filterIsInstance<RealtimeSignal.Rebuild>()
