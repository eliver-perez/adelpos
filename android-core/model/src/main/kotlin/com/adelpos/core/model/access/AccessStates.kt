package com.adelpos.core.model.access

// Etapa K1 (Fase B): lo que necesitan las pantallas compartidas de la entrada al dispositivo
// (android-core:ui), separado de DeviceAccess.kt y WaiterWork.kt de la aplicación de meseros sin
// cambiar nada.

/** Por qué el dispositivo necesita emparejarse de nuevo. */
enum class RepairReason {
    /** El servidor presentó otro certificado (huella distinta): nunca se conecta. */
    IDENTITY_CHANGED,

    /** El servidor ya no reconoce la credencial del dispositivo (`DEVICE_*`). */
    NOT_RECOGNIZED,

    /** La credencial cifrada ya no se puede abrir (se perdió la llave del Keystore). */
    CREDENTIAL_LOST,
}

/**
 * Permisos de Android que hacen falta para hablar con la red local del restaurante (§4), según la
 * versión del sistema y el `targetSdk` de la aplicación. Hoy (`targetSdk` 36, A1-D7) ninguno:
 * `NsdManager` y las conexiones a la red local no piden permiso. Android 17 (API 37) exige
 * `ACCESS_LOCAL_NETWORK` (grupo «Dispositivos cercanos») a las aplicaciones con `targetSdk` 37 o
 * mayor para **todo** el tráfico a la red local (búsqueda y conexiones); la pantalla de explicación
 * ya existe y se mostrará sola al subir el `targetSdk`.
 */
object NetworkPermissions {
    const val ACCESS_LOCAL_NETWORK = "android.permission.ACCESS_LOCAL_NETWORK"
    private const val ANDROID_17 = 37

    fun required(sdkInt: Int, targetSdk: Int): List<String> =
        if (sdkInt >= ANDROID_17 && targetSdk >= ANDROID_17) {
            listOf(ACCESS_LOCAL_NETWORK)
        } else {
            emptyList()
        }
}

/**
 * Un pedido que no se envió y queda en la bitácora del dispositivo al retirarlo (§3.2). Solo texto
 * para mostrar: la mesa, lo capturado y quién lo capturó (nunca credenciales).
 */
data class UnsentOrder(
    val waiterName: String,
    val description: String,
    val capturedAtEpochMillis: Long,
)
