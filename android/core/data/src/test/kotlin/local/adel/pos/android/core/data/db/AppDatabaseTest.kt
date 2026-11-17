package local.adel.pos.android.core.data.db

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import local.adel.pos.android.core.data.preferences.DevicePreferencesRepository
import local.adel.pos.android.core.domain.preferences.TablesView
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Room (versión vigente) en la JVM con Robolectric (SDK 36): preferencias del dispositivo y
 * persistencia al reabrir.
 */
@RunWith(AndroidJUnit4::class)
class AppDatabaseTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    @After
    fun deleteDatabase() {
        context.deleteDatabase(AppDatabase.FILE_NAME)
    }

    @Test
    fun laVistaDeMesasEmpiezaEnListaYSeGuarda() = runBlocking {
        val database =
            Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java)
                .allowMainThreadQueries()
                .build()
        try {
            val repository =
                DevicePreferencesRepository(database.devicePreferences(), clock = { 1_000L })
            assertEquals(TablesView.LIST, repository.tablesView.first())
            repository.setTablesView(TablesView.MAP)
            assertEquals(TablesView.MAP, repository.tablesView.first())
            repository.setTablesView(TablesView.LIST)
            assertEquals(TablesView.LIST, repository.tablesView.first())
        } finally {
            database.close()
        }
    }

    @Test
    fun lasPreferenciasSobrevivenAlCerrarYReabrirLaBase() = runBlocking {
        val first = AppDatabase.open(context)
        DevicePreferencesRepository(first.devicePreferences()).setTablesView(TablesView.MAP)
        first.close()

        val second = AppDatabase.open(context)
        try {
            assertEquals("map", second.devicePreferences().get("tables_view"))
            second.devicePreferences().remove("tables_view")
            assertNull(second.devicePreferences().get("tables_view"))
            assertEquals(2, second.openHelper.readableDatabase.version)
        } finally {
            second.close()
        }
    }
}
