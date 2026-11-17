package local.adel.pos.android.core.domain.server

/**
 * Versión `MAJOR.MINOR.PATCH` con una etiqueta opcional («0.12.0-beta»), como las compara Go: una
 * versión con etiqueta es anterior a la misma sin etiqueta.
 */
data class SemanticVersion(
    val major: Int,
    val minor: Int,
    val patch: Int,
    val label: String? = null,
) : Comparable<SemanticVersion> {
    override fun compareTo(other: SemanticVersion): Int =
        compareValuesBy(
            this,
            other,
            SemanticVersion::major,
            SemanticVersion::minor,
            SemanticVersion::patch,
            // Sin etiqueta va después que con etiqueta; entre etiquetas, orden de texto.
            { version -> if (version.label == null) 1 else 0 },
            { version -> version.label.orEmpty() },
        )

    override fun toString(): String =
        if (label == null) "$major.$minor.$patch" else "$major.$minor.$patch-$label"

    companion object {
        private val pattern =
            Regex(
                "^(0|[1-9]\\d{0,8})\\.(0|[1-9]\\d{0,8})\\.(0|[1-9]\\d{0,8})(?:-([0-9A-Za-z.-]+))?$",
            )

        /** Lee la versión; null si no tiene la forma `MAJOR.MINOR.PATCH[-etiqueta]`. */
        fun parse(text: String): SemanticVersion? {
            val match = pattern.matchEntire(text.trim()) ?: return null
            val (major, minor, patch, label) = match.destructured
            return SemanticVersion(
                major.toInt(),
                minor.toInt(),
                patch.toInt(),
                label.ifEmpty { null },
            )
        }
    }
}

/**
 * Si el servidor del restaurante sirve para esta aplicación (§4, «Versiones»). La aplicación exige
 * un servidor 0.12.0 o posterior con la capacidad `waiter-pin-v1` (A1-D4: la entrada con PIN y la
 * credencial de envío) y respeta la mínima de Android que anuncia el servidor
 * (`min_client_versions.android`). El servidor también responde 426 a una versión vieja en
 * cualquier ruta; esto solo lo dice antes, al comprobar `GET version`.
 */
object ServerCompatibility {
    const val CAPABILITY_WAITER_PIN = "waiter-pin-v1"
    val MIN_SERVER_VERSION = SemanticVersion(0, 12, 0)

    /** Resultado de la comprobación. */
    sealed interface Verdict {
        data object Compatible : Verdict

        /** «Actualiza esta aplicación»: el servidor pide al menos [minClientVersion]. */
        data class UpdateApp(val minClientVersion: String?, val serverVersion: String?) : Verdict

        /** «Actualiza el servidor del restaurante»: es anterior a 0.12.0 o le falta el PIN. */
        data class UpdateServer(val serverVersion: String) : Verdict
    }

    fun check(
        appVersion: String,
        serverVersion: String,
        capabilities: List<String>,
        minAndroidVersion: String?,
    ): Verdict {
        val server = SemanticVersion.parse(serverVersion)
        if (
            server == null || server < MIN_SERVER_VERSION || CAPABILITY_WAITER_PIN !in capabilities
        ) {
            return Verdict.UpdateServer(serverVersion)
        }
        val app = SemanticVersion.parse(appVersion)
        val minimum = minAndroidVersion?.let(SemanticVersion::parse)
        if (app == null || (minimum != null && app < minimum)) {
            return Verdict.UpdateApp(minAndroidVersion, serverVersion)
        }
        return Verdict.Compatible
    }
}
