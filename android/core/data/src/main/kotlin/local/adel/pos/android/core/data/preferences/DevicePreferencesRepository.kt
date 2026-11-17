package local.adel.pos.android.core.data.preferences

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import local.adel.pos.android.core.data.db.DevicePreference
import local.adel.pos.android.core.data.db.DevicePreferenceDao
import local.adel.pos.android.core.domain.preferences.TablesView
import local.adel.pos.android.core.domain.preferences.TablesViewPreference

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
