package com.adelpos.meseros.core.data.preferences

import com.adelpos.meseros.core.data.db.DevicePreference
import com.adelpos.meseros.core.data.db.DevicePreferenceDao
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.preferences.TablesViewPreference
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** Preferencias de este dispositivo guardadas en Room, con sus valores predeterminados. */
class DevicePreferencesRepository(
    private val dao: DevicePreferenceDao,
    private val clock: () -> Long = System::currentTimeMillis,
) : TablesViewPreference {
    /** Vista de mesas de este dispositivo; la lista mientras no se elija otra. */
    override val tablesView: Flow<TablesView> =
        dao.observe(KEY_TABLES_VIEW).map(TablesView::fromStoredName)

    override suspend fun setTablesView(view: TablesView) {
        dao.put(DevicePreference(KEY_TABLES_VIEW, view.storedName, clock()))
    }

    private companion object {
        const val KEY_TABLES_VIEW = "tables_view"
    }
}
