package com.adelpos.core.model.server

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
 * Lo que una aplicación exige al servidor del restaurante (§4 de la A1, «Versiones»): una versión
 * mínima y las capacidades que usa. Cada aplicación declara las suyas (etapa K1): la de meseros en
 * `WaiterServerRequirements` (`android/`) y la de cocina en la suya.
 */
data class ServerRequirements(
    val minServerVersion: SemanticVersion,
    val capabilities: List<String>,
)

/**
 * Si el servidor del restaurante sirve para esta aplicación (§4, «Versiones»): debe cumplir sus
 * [ServerRequirements] (versión y capacidades) y la aplicación debe ser al menos la mínima que el
 * servidor anuncia para su tipo de cliente (`min_client_versions`). El servidor también responde
 * 426 a una versión vieja en cualquier ruta; esto solo lo dice antes, al comprobar `GET version`.
 */
object ServerCompatibility {
    /** Resultado de la comprobación. */
    sealed interface Verdict {
        data object Compatible : Verdict

        /** «Actualiza esta aplicación»: el servidor pide al menos [minClientVersion]. */
        data class UpdateApp(val minClientVersion: String?, val serverVersion: String?) : Verdict

        /**
         * «Actualiza el servidor del restaurante»: es anterior a la versión mínima o le falta una
         * capacidad que la aplicación usa.
         */
        data class UpdateServer(val serverVersion: String) : Verdict
    }

    fun check(
        appVersion: String,
        serverVersion: String,
        capabilities: List<String>,
        minClientVersion: String?,
        requirements: ServerRequirements,
    ): Verdict {
        val server = SemanticVersion.parse(serverVersion)
        val missing = requirements.capabilities.any { it !in capabilities }
        if (server == null || server < requirements.minServerVersion || missing) {
            return Verdict.UpdateServer(serverVersion)
        }
        val app = SemanticVersion.parse(appVersion)
        val minimum = minClientVersion?.let(SemanticVersion::parse)
        if (app == null || (minimum != null && app < minimum)) {
            return Verdict.UpdateApp(minClientVersion, serverVersion)
        }
        return Verdict.Compatible
    }
}
