package com.adelpos.meseros.core.data.api

import kotlinx.serialization.json.Json

/**
 * JSON de la API v1. Ignora los campos que esta versión no conoce: el contrato v1 solo crece de
 * forma compatible y un servidor más nuevo puede agregar campos. Los importes viajan como cadenas
 * decimales ([MoneySerializer]).
 */
val ApiJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
}
