package com.adelpos.core.network.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import okhttp3.Headers

/**
 * Credencial del dispositivo emparejado (`X-Device-ID` y `X-Device-Token`) y, si la hay, la sesión
 * del mesero (`Authorization: Bearer`). El token solo vive en memoria mientras se usa: guardado,
 * siempre va cifrado con el Android Keystore. [toString] nunca lo muestra.
 */
class DeviceCredentials(val deviceId: String, private val deviceToken: String) {
    /** Cabeceras de la petición, con la sesión del mesero si se indica. */
    fun headers(sessionToken: String? = null): Headers {
        val builder =
            Headers.Builder().add(DEVICE_ID_HEADER, deviceId).add(DEVICE_TOKEN_HEADER, deviceToken)
        if (sessionToken != null) {
            builder.add("Authorization", "Bearer $sessionToken")
        }
        return builder.build()
    }

    override fun toString(): String = "DeviceCredentials($deviceId)"

    companion object {
        const val DEVICE_ID_HEADER = "X-Device-ID"
        const val DEVICE_TOKEN_HEADER = "X-Device-Token"
    }
}

/** Cuerpo de `POST pairing/requests` (`createPairingRequest`). */
@Serializable
data class CreatePairingRequest(
    val name: String,
    @SerialName("device_kind") val deviceKind: String,
    val os: String,
    @SerialName("app_version") val appVersion: String,
    val commitment: String,
)

/** Respuesta de `createPairingRequest`. [pairingSecret] nunca sale de la sesión. */
@Serializable
data class CreatedPairing(
    @SerialName("request_id") val requestId: String,
    @SerialName("server_nonce") val serverNonce: String,
    @SerialName("pairing_secret") val pairingSecret: String,
    @SerialName("expires_at") val expiresAt: String,
    @SerialName("server_id") val serverId: String,
    @SerialName("server_name") val serverName: String = "",
    val name: String = "",
) {
    override fun toString(): String = "CreatedPairing($requestId)"
}

/** Cuerpo de `revealPairingNonce`. */
@Serializable data class RevealPairingNonce(@SerialName("terminal_nonce") val terminalNonce: String)

/** Respuesta de `getPairingRequest`, `revealPairingNonce`, `renamePairingRequest` y `cancel…`. */
@Serializable
data class PairingStatusDto(
    val state: String,
    @SerialName("expires_at") val expiresAt: String? = null,
    val name: String? = null,
)

/** Cuerpo de `renamePairingRequest`. */
@Serializable data class RenamePairingRequest(val name: String)

/** Respuesta de `claimPairingCredential`: la credencial se entrega una sola vez. */
@Serializable
data class ClaimedCredentialDto(
    @SerialName("device_id") val deviceId: String,
    @SerialName("device_token") val deviceToken: String,
    @SerialName("server_id") val serverId: String,
    @SerialName("server_name") val serverName: String = "",
    val name: String = "",
) {
    override fun toString(): String = "ClaimedCredentialDto($deviceId)"
}

/** Cuerpo vacío (`{}`) de las acciones sin datos. */
@Serializable class EmptyBody

/** Respuesta que no se lee (solo importa el estado HTTP). */
typealias IgnoredBody = JsonObject
