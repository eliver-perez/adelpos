package com.adelpos.meseros.core.data.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Respuesta de `GET /api/v1/version` (`getVersion`, esquema `VersionInfo` del contrato OpenAPI).
 * Responde siempre, sin sesión: la aplicación compara [serverVersion] con la versión mínima de
 * servidor que necesita y sabe si ella misma debe actualizarse. [minClientVersions] (mínima por
 * tipo de cliente, A1-D4) llega desde el servidor 0.12.0; con uno anterior queda vacío.
 */
@Serializable
data class VersionInfo(
    val product: String,
    @SerialName("server_version") val serverVersion: String,
    @SerialName("api_version") val apiVersion: String,
    @SerialName("schema_version") val schemaVersion: Int,
    @SerialName("supported_api_versions") val supportedApiVersions: List<String>,
    val capabilities: List<String>,
    @SerialName("events_available") val eventsAvailable: Boolean = false,
    @SerialName("test_mode") val testMode: Boolean = false,
    @SerialName("min_client_version") val minClientVersion: String,
    @SerialName("min_client_versions") val minClientVersions: Map<String, String> = emptyMap(),
    @SerialName("client_kinds") val clientKinds: List<String>,
)
