package local.adel.pos.android.core.data.api

/**
 * Cómo se identifica la aplicación ante el servidor en cada petición (API v1, «Reglas para
 * cualquier cliente», punto 2). [version] es el `versionName` del APK, que se escribe en un solo
 * lugar (`android/gradle.properties`); el servidor responde 426 `CLIENT_UPDATE_REQUIRED` si es
 * anterior a la mínima para Android.
 */
data class ClientIdentity(val version: String) {
    val kind: String = KIND

    companion object {
        const val KIND = "android"
        const val VERSION_HEADER = "X-Client-Version"
        const val KIND_HEADER = "X-Client-Kind"
    }
}
