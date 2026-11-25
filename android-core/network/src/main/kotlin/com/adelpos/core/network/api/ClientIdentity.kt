package com.adelpos.core.network.api

/**
 * Cómo se identifica la aplicación ante el servidor en cada petición (API v1, «Reglas para
 * cualquier cliente», punto 2). [version] es el `versionName` del APK, que se escribe en un solo
 * lugar (`gradle.properties` de cada aplicación); el servidor responde 426 `CLIENT_UPDATE_REQUIRED`
 * si es anterior a la mínima para su tipo de cliente.
 *
 * [kind] es el tipo de cliente de la aplicación ([ClientKinds]): va en `X-Client-Kind` de cada
 * petición y como `device_kind` al emparejar. Etapa K1 (K1-D2): es un valor fijo de cada
 * aplicación, nunca derivado del paquete de Kotlin.
 */
data class ClientIdentity(val version: String, val kind: String) {
    companion object {
        const val VERSION_HEADER = "X-Client-Version"
        const val KIND_HEADER = "X-Client-Kind"
    }
}

/**
 * Los tipos de cliente que conoce Go (`httpapi.ClientKinds` y el tipo de dispositivo de la 0023),
 * uno por aplicación Android. Go exige que cada dispositivo emparejado hable siempre con el suyo
 * (`CLIENT_KIND_MISMATCH`).
 */
object ClientKinds {
    /** La aplicación de meseros (`com.adelpos.meseros`). */
    const val ANDROID = "android"

    /** La aplicación de cocina (`com.adelpos.cocina`, etapa K1, K1-D3). */
    const val KITCHEN = "kitchen"
}
