package com.adelpos.meseros.core.data.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// La entrada con PIN de los meseros (separada en la etapa K1 de AccessDtos.kt, que pasó a
// android-core con el emparejamiento, sin cambiar nada).

/** Respuesta de `GET auth/pin-users` (`listPinUsers`, esquema `PinUsers`). */
@Serializable data class PinUsersResponse(val device: PinDevice, val users: List<PinUserDto>)

@Serializable
data class PinDevice(
    val id: String,
    val name: String,
    @SerialName("idle_lock_seconds") val idleLockSeconds: Int,
)

/** Mesero de la lista (esquema `PinUser`): nada de roles ni permisos. */
@Serializable
data class PinUserDto(
    val id: String,
    val name: String,
    val initial: String,
    val color: String? = null,
    @SerialName("pin_version") val pinVersion: Int,
    @SerialName("locked_until") val lockedUntil: String? = null,
    /** A1-D16: bloqueado hasta que un administrador lo desbloquee (`locked_until` es null). */
    @SerialName("requires_unlock") val requiresUnlock: Boolean = false,
)

/** Cuerpo de `POST auth/pin-login` (`pinLogin`). */
@Serializable
data class PinLoginRequest(@SerialName("user_id") val userId: String, val pin: String) {
    override fun toString(): String = "PinLoginRequest($userId)"
}

/** Respuesta de `pinLogin` (esquema `PinLoginResult`). */
@Serializable
data class PinLoginResponse(
    @SerialName("session_token") val sessionToken: String,
    @SerialName("pin_version") val pinVersion: Int,
    @SerialName("offline_credential") val offlineCredential: String,
) {
    override fun toString(): String = "PinLoginResponse(pin_version=$pinVersion)"
}
