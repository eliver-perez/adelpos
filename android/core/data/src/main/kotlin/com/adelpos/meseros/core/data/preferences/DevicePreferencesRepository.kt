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

    /**
     * Si ya se explicó y se pidió [permission] para el aviso de platillos listos (etapa A2, §4.3:
     * notificaciones y exclusión de la optimización de batería): se pide una sola vez; después, la
     * pantalla de ayuda dice qué falta.
     */
    suspend fun alertPermissionAsked(permission: String): Boolean =
        dao.get(KEY_ALERT_ASKED_PREFIX + permission) != null

    suspend fun markAlertPermissionAsked(permission: String) {
        dao.put(DevicePreference(KEY_ALERT_ASKED_PREFIX + permission, "asked", clock()))
    }

    private companion object {
        const val KEY_TABLES_VIEW = "tables_view"
        const val KEY_ALERT_ASKED_PREFIX = "ready_alerts.asked."
    }
}
