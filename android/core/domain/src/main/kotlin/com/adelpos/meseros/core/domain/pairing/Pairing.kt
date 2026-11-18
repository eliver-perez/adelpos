package com.adelpos.meseros.core.domain.pairing

import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.server.ServerAddress

/** Estado de la solicitud de emparejamiento tal como lo muestra la pantalla del código. */
enum class PairingState {
    /** Esperando a que un administrador escriba el código en la computadora principal. */
    PENDING,

    /** Emparejado: la credencial ya se guardó cifrada; sigue la entrada con PIN. */
    PAIRED,
    REJECTED,
    EXPIRED,
    CANCELLED,

    /** La credencial ya se había reclamado (no debería pasar en este dispositivo). */
    CLAIMED;

    /** Ya no espera nada: se muestra «Pedir otro código» (salvo [PAIRED]). */
    val finished: Boolean
        get() = this != PENDING

    companion object {
        /** Estado por su nombre en la API (`approved` sin reclamar todavía cuenta como espera). */
        fun fromWireName(name: String): PairingState? =
            when (name) {
                "pending",
                "approved" -> PENDING
                "rejected" -> REJECTED
                "expired" -> EXPIRED
                "cancelled" -> CANCELLED
                "claimed" -> CLAIMED
                else -> null
            }
    }
}

/**
 * Lo que ve la pantalla del código: nunca el secreto de la solicitud, los números del código ni la
 * dirección. [expiresAtEpochMillis] es el vencimiento que informó el servidor.
 */
data class PairingView(
    val state: PairingState,
    val code: String,
    val deviceName: String,
    val serverName: String,
    val expiresAtEpochMillis: Long,
)

/** A qué servidor se pide el código: uno de la búsqueda (por su `server_id`) o una dirección. */
sealed interface PairingTarget {
    data class Found(val serverId: String) : PairingTarget

    data class Address(val text: String) : PairingTarget
}

/** Por qué no se pudo pedir el código o terminar el emparejamiento. */
sealed interface PairingProblem {
    /** La dirección escrita no es válida o no es de la red local ([ServerAddress.Problem]). */
    data class InvalidAddress(val problem: ServerAddress.Problem) : PairingProblem

    /** El nombre está vacío, es muy largo (más de 80) o tiene caracteres de control. */
    data object InvalidName : PairingProblem

    /** El servidor elegido de la lista ya no aparece en la red: volver a buscar. */
    data object NoLongerFound : PairingProblem

    /**
     * El servidor que respondió no es el que se anunció (otra huella corta u otro `server_id`):
     * nunca se continúa.
     */
    data object AnnouncementMismatch : PairingProblem

    /** No hay un emparejamiento en curso: pedir un código nuevo. */
    data object NoSession : PairingProblem

    /** El servidor entregó una credencial que no tiene la forma esperada: emparejar de nuevo. */
    data object InvalidCredential : PairingProblem

    /** No se pudo guardar la credencial cifrada en el dispositivo (Android Keystore). */
    data object StorageFailed : PairingProblem

    /**
     * «Actualiza el servidor del restaurante»: la computadora principal es anterior a 0.13.0 o le
     * falta una capacidad que usa la aplicación (`waiter-pin-v1`, `takeaway-v1`), así que no puede
     * emparejar teléfonos ni tabletas
     * ([com.adelpos.meseros.core.domain.server.ServerCompatibility]). [serverVersion] es la que
     * informó, si la informó (solo para la sección de soporte).
     */
    data class UpdateServer(val serverVersion: String?) : PairingProblem

    /**
     * Error al hablar con el servidor: sin respuesta, certificado distinto a mitad de la sesión,
     * 426, o un rechazo con su código (`PAIRING_RATE_LIMITED`, `PAIRING_EXPIRED`…) y su mensaje en
     * español para mostrarse tal cual.
     */
    data class Server(val error: AppError) : PairingProblem
}

/** Resultado de un paso del emparejamiento. */
sealed interface PairingStep {
    data class Showing(val view: PairingView) : PairingStep

    data class Failed(val problem: PairingProblem) : PairingStep
}

/** Servidor encontrado tal como se muestra en la lista: solo el nombre (nunca la dirección). */
data class FoundServerChoice(val serverId: String, val name: String)

/** Resultado de buscar en la red. */
sealed interface ServerSearch {
    data class Found(val servers: List<FoundServerChoice>) : ServerSearch

    /** El sistema no permitió buscar (por ejemplo, sin el permiso de red local de Android 17). */
    data object Unavailable : ServerSearch
}

/**
 * Búsqueda del servidor y emparejamiento con el protocolo de la 9G (§4). La implementación vive en
 * `core:data` (NSD, cliente HTTPS con el primer certificado fijado, Room y Android Keystore); la
 * pantalla «Buscar el servidor» de `feature:access` solo usa esta interfaz.
 */
interface PairingFlow {
    /** Busca computadoras principales en la red local (una búsqueda de unos segundos). */
    suspend fun search(): ServerSearch

    /** Pide el código: solicitud con el compromiso, número del servidor y revelación del propio. */
    suspend fun start(target: PairingTarget, deviceName: String): PairingStep

    /**
     * Consulta el estado; aprobada la solicitud, reclama la credencial una sola vez, la guarda
     * cifrada junto con el servidor emparejado y responde [PairingState.PAIRED].
     */
    suspend fun poll(): PairingStep

    /** Corrige el nombre propuesto mientras espera. */
    suspend fun rename(deviceName: String): PairingStep

    /** Deja de esperar (si el servidor no responde, la solicitud vence sola). */
    suspend fun cancel()
}
