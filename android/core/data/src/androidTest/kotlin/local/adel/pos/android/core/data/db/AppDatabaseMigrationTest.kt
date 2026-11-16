package local.adel.pos.android.core.data.db

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Migraciones de la base local con los esquemas exportados (`core/data/schemas/`): cada versión
 * anterior se crea con datos, se migra a la vigente y Room valida que el resultado sea exactamente
 * el esquema exportado.
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
                "INSERT INTO device_preferences (`key`, value, updated_at_millis) " +
                    "VALUES ('tables_view', 'map', 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 1, true).close()
    }

    @Test
    fun de1a2ConservaLasPreferenciasYCreaLasTablasDeEntrada() {
        helper.createDatabase(testDatabase, 1).use { database ->
            database.execSQL(
                "INSERT INTO device_preferences (`key`, value, updated_at_millis) " +
                    "VALUES ('tables_view', 'map', 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 2, true, AppDatabase.MIGRATION_1_2).use {
            database ->
            database
                .query("SELECT value FROM device_preferences WHERE `key` = 'tables_view'")
                .use { cursor ->
                    cursor.moveToFirst()
                    assertEquals("map", cursor.getString(0))
                }
            for (table in listOf("paired_server", "known_waiters", "device_log")) {
                database.query("SELECT count(*) FROM $table").use { cursor ->
                    cursor.moveToFirst()
                    assertEquals(table, 0, cursor.getInt(0))
                }
            }
        }
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
            val access = database.access()
            assertNull(access.pairedServer())
            access.savePairedServer(
                PairedServerEntity(
                    serverId = "11111111-2222-4333-8444-555555555555",
                    serverName = "Fonda",
                    baseUrl = "https://192.168.1.20:8443",
                    fingerprintHex = "00".repeat(32),
                    deviceId = "0b5d2f6c-6b1a-4c4b-9b70-3f0f8e1c2a10",
                    deviceName = "Tableta 1",
                    sealedDeviceToken = byteArrayOf(1, 2, 3),
                    idleLockSeconds = 120,
                    pairedAtMillis = 1,
                ),
            )
            assertArrayEquals(byteArrayOf(1, 2, 3), access.pairedServer()?.sealedDeviceToken)
            assertEquals(2, database.openHelper.readableDatabase.version)
        } finally {
            database.close()
        }
    }
}
