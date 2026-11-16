package local.adel.pos.android.core.domain.preferences

/** Cómo ve el mesero las mesas en este dispositivo (§6.1): lista (predeterminada) o mapa. */
enum class TablesView(val storedName: String) {
    LIST("list"),
    MAP("map");

    companion object {
        val DEFAULT = LIST

        /** Valor guardado; si no se reconoce, el predeterminado. */
        fun fromStoredName(name: String?): TablesView =
            entries.firstOrNull { it.storedName == name } ?: DEFAULT
    }
}
