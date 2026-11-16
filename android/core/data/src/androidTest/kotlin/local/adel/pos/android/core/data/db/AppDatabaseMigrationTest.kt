package local.adel.pos.android.core.data.db

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Migraciones de la base local con los esquemas exportados (`core/data/schemas/`). Con una sola
 * versión comprueba que el esquema 1 que crea Room es el exportado; cada fase que suba la versión
 * agrega aquí su caso (crear la versión anterior con datos, migrar y validar).
 */
@RunWith(AndroidJUnit4::class)
class AppDatabaseMigrationTest {
    private val testDatabase = "migration-test.db"

    @get:Rule
    val helper =
        MigrationTestHelper(InstrumentationRegistry.getInstrumentation(), AppDatabase::class.java)

    @Test
    fun laVersion1CoincideConElEsquemaExportado() {
        helper.createDatabase(testDatabase, 1).use { database ->
            database.execSQL(
                "INSERT INTO device_preferences (`key`, value, updated_at_millis) VALUES ('tables_view', 'map', 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 1, true, *AppDatabase.MIGRATIONS).close()
    }

    @Test
    fun todasLasMigracionesLlevanALaVersionActual() = runBlocking {
        helper.createDatabase(testDatabase, 1).close()
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val database =
            Room.databaseBuilder(context, AppDatabase::class.java, testDatabase)
                .addMigrations(*AppDatabase.MIGRATIONS)
                .build()
        try {
            database.devicePreferences().put(DevicePreference("tables_view", "list", 2))
            assertEquals("list", database.devicePreferences().get("tables_view"))
            assertEquals(1, database.openHelper.readableDatabase.version)
        } finally {
            database.close()
        }
    }
}
