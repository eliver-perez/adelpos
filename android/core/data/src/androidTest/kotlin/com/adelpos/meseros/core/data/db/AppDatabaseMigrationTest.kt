package com.adelpos.meseros.core.data.db

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
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
    fun de2a3ConservaLosFallosYCierraLaEntradaDeQuienEstabaBloqueado() {
        helper.createDatabase(testDatabase, 2).use { database ->
            // Ana llevaba 2 fallos sin bloqueo; Beto estaba bloqueado (por reloj) hasta una hora.
            for ((id, failures, lockedUntil) in
                listOf(Triple("ana", 2, "NULL"), Triple("beto", 5, "1700000300000"))) {
                database.execSQL(
                    "INSERT INTO known_waiters (user_id, server_id, name, initial, color, " +
                        "pin_version, sealed_verifier, sealed_offline_credential, " +
                        "last_online_entry_millis, local_failures, local_window_start_millis, " +
                        "local_locked_until_millis) VALUES ('$id', 's1', '$id', 'A', NULL, 1, " +
                        "X'01', NULL, 1, $failures, 1, $lockedUntil)",
                )
            }
        }
        helper.runMigrationsAndValidate(testDatabase, 3, true, AppDatabase.MIGRATION_2_3).use {
            database ->
            database
                .query(
                    "SELECT user_id, offline_failures, offline_closed, sealed_verifier " +
                        "FROM known_waiters ORDER BY user_id",
                )
                .use { cursor ->
                    cursor.moveToFirst()
                    assertEquals("ana", cursor.getString(0))
                    assertEquals(2, cursor.getInt(1))
                    assertEquals(0, cursor.getInt(2))
                    assertArrayEquals(byteArrayOf(1), cursor.getBlob(3))
                    cursor.moveToNext()
                    assertEquals("beto", cursor.getString(0))
                    assertEquals(5, cursor.getInt(1))
                    assertEquals("el bloqueo por reloj pasa a entrada cerrada", 1, cursor.getInt(2))
                }
        }
    }

    @Test
    fun de3a4ConservaLoAnteriorYCreaLasTablasDelModoMesero() {
        helper.createDatabase(testDatabase, 3).use { database ->
            database.execSQL(
                "INSERT INTO known_waiters (user_id, server_id, name, initial, color, " +
                    "pin_version, sealed_verifier, sealed_offline_credential, " +
                    "last_online_entry_millis, offline_failures, offline_closed) " +
                    "VALUES ('ana', 's1', 'Ana', 'A', NULL, 1, X'01', X'02', 1, 0, 0)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 4, true, AppDatabase.MIGRATION_3_4).use {
            database ->
            database.query("SELECT sealed_offline_credential FROM known_waiters").use { cursor ->
                cursor.moveToFirst()
                assertArrayEquals(byteArrayOf(2), cursor.getBlob(0))
            }
            for (table in
                listOf("captures", "pending_orders", "restaurant_snapshot", "waiter_permissions")) {
                database.query("SELECT count(*) FROM $table").use { cursor ->
                    cursor.moveToFirst()
                    assertEquals(table, 0, cursor.getInt(0))
                }
            }
            // La cola guarda su pedido con la clave de operación como llave.
            database.execSQL(
                "INSERT INTO pending_orders (operation_id, waiter_user_id, waiter_name, state, " +
                    "destination, lines, replaces, catalog_version, catalog_epoch, captured_at, " +
                    "confirmation, review, attempts, last_error, credential_rejected, " +
                    "online_confirm, received_account_id, received_at, updated_at) VALUES " +
                    "('op-1', 'ana', 'Ana', 'queued', '{}', '[]', '[]', 41, NULL, 1, NULL, NULL, " +
                    "0, NULL, 0, NULL, NULL, NULL, 1)",
            )
            database.query("SELECT state FROM pending_orders WHERE operation_id = 'op-1'").use {
                cursor ->
                cursor.moveToFirst()
                assertEquals("queued", cursor.getString(0))
            }
        }
    }

    @Test
    fun de4a5ConservaLaColaYAgregaLasColumnasDeLaRevisionR6() {
        helper.createDatabase(testDatabase, 4).use { database ->
            database.execSQL(
                "INSERT INTO pending_orders (operation_id, waiter_user_id, waiter_name, state, " +
                    "destination, lines, replaces, catalog_version, catalog_epoch, captured_at, " +
                    "confirmation, review, attempts, last_error, credential_rejected, " +
                    "online_confirm, received_account_id, received_at, updated_at) VALUES " +
                    "('op-1', 'ana', 'Ana', 'queued', '{}', '[]', '[]', 41, NULL, 1, NULL, NULL, " +
                    "0, NULL, 0, NULL, NULL, NULL, 1)",
            )
            database.execSQL(
                "INSERT INTO captures (id, waiter_user_id, table_id, table_name, account_id, " +
                    "account_name, seen_account_ids, base, base_revision, operations, " +
                    "own_item_ids, seen_prices, sync, sync_message, table_accounts, " +
                    "pending_operation_id, in_flight_count, created_at, updated_at) VALUES " +
                    "('c-1', 'ana', 'mesa', 'Mesa 4', NULL, NULL, '[]', '[]', 0, '[]', '[]', " +
                    "'{}', 'SAVED', NULL, '[]', NULL, 0, 1, 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 5, true, AppDatabase.MIGRATION_4_5).use {
            database ->
            // El pedido sin enviar sigue igual y las columnas nuevas toman su valor por omisión.
            database
                .query(
                    "SELECT state, unresolved_save, clock_corrected FROM pending_orders " +
                        "WHERE operation_id = 'op-1'",
                )
                .use { cursor ->
                    cursor.moveToFirst()
                    assertEquals("queued", cursor.getString(0))
                    assertTrue(cursor.isNull(1))
                    assertEquals(0, cursor.getInt(2))
                }
            database.query("SELECT created_account FROM captures WHERE id = 'c-1'").use { cursor ->
                cursor.moveToFirst()
                assertEquals(0, cursor.getInt(0))
            }
        }
    }

    /**
     * 5 → 6 (etapa A2, Para llevar): `captures` se copia a la tabla nueva (con `table_id` opcional)
     * sin perder nada; las capturas que había quedan como de salón y sin cliente, la cola no cambia
     * y aparece la tabla de ediciones del cliente, vacía. Después se puede guardar una captura para
     * llevar (sin mesa).
     */
    @Test
    fun de5a6ConservaLasCapturasYLaColaYAdmitePedidosParaLlevar() {
        helper.createDatabase(testDatabase, 5).use { database ->
            database.execSQL(
                "INSERT INTO pending_orders (operation_id, waiter_user_id, waiter_name, state, " +
                    "destination, lines, replaces, catalog_version, catalog_epoch, captured_at, " +
                    "confirmation, review, attempts, last_error, credential_rejected, " +
                    "online_confirm, received_account_id, received_at, updated_at, " +
                    "unresolved_save, clock_corrected) VALUES " +
                    "('op-1', 'ana', 'Ana', 'queued', '{\"table_id\":\"mesa\"}', '[]', '[]', " +
                    "41, NULL, 1, NULL, NULL, 0, NULL, 0, NULL, NULL, NULL, 1, NULL, 1)",
            )
            database.execSQL(
                "INSERT INTO captures (id, waiter_user_id, table_id, table_name, account_id, " +
                    "account_name, seen_account_ids, base, base_revision, operations, " +
                    "own_item_ids, seen_prices, sync, sync_message, table_accounts, " +
                    "pending_operation_id, in_flight_count, created_at, updated_at, " +
                    "created_account) VALUES " +
                    "('c-1', 'ana', 'mesa', 'Mesa 4', 'cuenta', 'Mesa 4', '[\"vista\"]', " +
                    "'[]', 3, '[{\"kind\":\"remove\",\"id\":\"r1\"}]', '[\"r1\"]', " +
                    "'{}', 'PENDING', 'mensaje', '[]', 'op-guardado', 1, 7, 8, 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 6, true, AppDatabase.MIGRATION_5_6).use {
            database ->
            database
                .query(
                    "SELECT table_id, table_name, account_id, seen_account_ids, base_revision, " +
                        "operations, own_item_ids, sync, sync_message, pending_operation_id, " +
                        "in_flight_count, created_at, updated_at, created_account, mode, " +
                        "customer_name, customer_phone FROM captures WHERE id = 'c-1'",
                )
                .use { cursor ->
                    assertTrue(cursor.moveToFirst())
                    assertEquals("mesa", cursor.getString(0))
                    assertEquals("Mesa 4", cursor.getString(1))
                    assertEquals("cuenta", cursor.getString(2))
                    assertEquals("[\"vista\"]", cursor.getString(3))
                    assertEquals(3, cursor.getInt(4))
                    assertEquals("[{\"kind\":\"remove\",\"id\":\"r1\"}]", cursor.getString(5))
                    assertEquals("[\"r1\"]", cursor.getString(6))
                    assertEquals("PENDING", cursor.getString(7))
                    assertEquals("mensaje", cursor.getString(8))
                    assertEquals("op-guardado", cursor.getString(9))
                    assertEquals(1, cursor.getInt(10))
                    assertEquals(7L, cursor.getLong(11))
                    assertEquals(8L, cursor.getLong(12))
                    assertEquals(1, cursor.getInt(13))
                    assertEquals("dine_in", cursor.getString(14))
                    assertEquals("", cursor.getString(15))
                    assertEquals("", cursor.getString(16))
                }
            database
                .query(
                    "SELECT state, clock_corrected FROM pending_orders WHERE operation_id = 'op-1'",
                )
                .use { cursor ->
                    cursor.moveToFirst()
                    assertEquals("queued", cursor.getString(0))
                    assertEquals(1, cursor.getInt(1))
                }
            database.query("SELECT count(*) FROM takeaway_customer_edits").use { cursor ->
                cursor.moveToFirst()
                assertEquals(0, cursor.getInt(0))
            }
            // Una captura para llevar, sin mesa, ya se puede guardar.
            database.execSQL(
                "INSERT INTO captures (id, waiter_user_id, table_id, table_name, account_id, " +
                    "account_name, seen_account_ids, base, base_revision, operations, " +
                    "own_item_ids, seen_prices, sync, sync_message, table_accounts, " +
                    "pending_operation_id, in_flight_count, created_at, updated_at, mode, " +
                    "customer_name, customer_phone) VALUES " +
                    "('c-2', 'ana', NULL, '', NULL, NULL, '[]', '[]', 0, '[]', '[]', '{}', " +
                    "'SAVED', NULL, '[]', NULL, 0, 9, 9, 'takeaway', 'Juan', '555')",
            )
            database.query("SELECT count(*) FROM captures WHERE table_id IS NULL").use { cursor ->
                cursor.moveToFirst()
                assertEquals(1, cursor.getInt(0))
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
            assertEquals(6, database.openHelper.readableDatabase.version)
        } finally {
            database.close()
        }
    }
}
