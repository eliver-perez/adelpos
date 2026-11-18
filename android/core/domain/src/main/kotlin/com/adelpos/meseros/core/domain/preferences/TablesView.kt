package com.adelpos.meseros.core.domain.preferences

import kotlinx.coroutines.flow.Flow

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

/**
 * Dónde se recuerda la vista de mesas de este dispositivo (Room en `core:data`); las pantallas solo
 * conocen esta interfaz.
 */
interface TablesViewPreference {
    val tablesView: Flow<TablesView>

    suspend fun setTablesView(view: TablesView)
}
