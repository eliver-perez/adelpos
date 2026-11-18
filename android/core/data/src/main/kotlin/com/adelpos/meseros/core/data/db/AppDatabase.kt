package com.adelpos.meseros.core.data.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase
import com.adelpos.meseros.core.data.waiter.db.CaptureEntity
import com.adelpos.meseros.core.data.waiter.db.PendingOrderEntity
import com.adelpos.meseros.core.data.waiter.db.RestaurantSnapshotEntity
import com.adelpos.meseros.core.data.waiter.db.WAITER_COLUMNS_V5
import com.adelpos.meseros.core.data.waiter.db.WAITER_TABLES_V4
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterPermissionsEntity

/**
 * Base local del dispositivo (Room). El esquema de cada versión se exporta a `core/data/schemas/` y
 * se versiona.
 * - Versión 1 (Fase A): preferencias del dispositivo.
 * - Versión 2 (fases B y C): servidor emparejado, meseros conocidos (verificador del PIN y
 *   credencial de envío cifrados con el Keystore) y bitácora del dispositivo.
 * - Versión 3 (revisión R3, M1 y M2): los intentos sin conexión de cada mesero dejan de depender
 *   del reloj (`offline_failures` acumulados y `offline_closed`).
 * - Versión 4 (Fase E): modo Mesero: capturas en curso por mesero (`captures`), la cola de pedidos
 *   (`pending_orders`), lo último leído de Go para trabajar sin conexión (`restaurant_snapshot`) y
 *   los permisos de cada mesero (`waiter_permissions`).
 *
 * Reglas para las fases siguientes: cada cambio sube [version][Database.version] en uno, agrega su
 * `Migration` a [MIGRATIONS] y una prueba con `MigrationTestHelper` (`AppDatabaseMigrationTest`).
 * Nunca `fallbackToDestructiveMigration`: la base guarda pedidos sin enviar que no se pueden
 * perder.
 */
@Database(
    entities =
        [
            DevicePreference::class,
            PairedServerEntity::class,
            KnownWaiterEntity::class,
            DeviceLogEntry::class,
            CaptureEntity::class,
            PendingOrderEntity::class,
            RestaurantSnapshotEntity::class,
            WaiterPermissionsEntity::class,
        ],
    version = 5,
    exportSchema = true,
)
abstract class AppDatabase : RoomDatabase() {
    abstract fun devicePreferences(): DevicePreferenceDao

    abstract fun access(): AccessDao

    /** Capturas por mesero, cola de pedidos y lo último leído de Go (Fase E). */
    abstract fun waiter(): WaiterDao

    companion object {
        const val FILE_NAME = "adel-pos.db"

        /**
         * 1 → 2: tablas nuevas del emparejamiento, los meseros conocidos y la bitácora; las
         * preferencias no cambian. El SQL es el que genera Room para la versión 2
         * (`schemas/…/2.json`).
         */
        val MIGRATION_1_2: Migration =
            object : Migration(1, 2) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    db.execSQL(
                        "CREATE TABLE IF NOT EXISTS `paired_server` (`slot` INTEGER NOT NULL, " +
                            "`server_id` TEXT NOT NULL, `server_name` TEXT NOT NULL, " +
                            "`base_url` TEXT NOT NULL, `fingerprint_hex` TEXT NOT NULL, " +
                            "`device_id` TEXT NOT NULL, `device_name` TEXT NOT NULL, " +
                            "`sealed_device_token` BLOB NOT NULL, " +
                            "`idle_lock_seconds` INTEGER NOT NULL, " +
                            "`paired_at_millis` INTEGER NOT NULL, PRIMARY KEY(`slot`))",
                    )
                    db.execSQL(
                        "CREATE TABLE IF NOT EXISTS `known_waiters` (`user_id` TEXT NOT NULL, " +
                            "`server_id` TEXT NOT NULL, `name` TEXT NOT NULL, " +
                            "`initial` TEXT NOT NULL, `color` TEXT, " +
                            "`pin_version` INTEGER NOT NULL, `sealed_verifier` BLOB, " +
                            "`sealed_offline_credential` BLOB, " +
                            "`last_online_entry_millis` INTEGER NOT NULL, " +
                            "`local_failures` INTEGER NOT NULL, " +
                            "`local_window_start_millis` INTEGER NOT NULL, " +
                            "`local_locked_until_millis` INTEGER, PRIMARY KEY(`user_id`))",
                    )
                    db.execSQL(
                        "CREATE INDEX IF NOT EXISTS `index_known_waiters_server_id` " +
                            "ON `known_waiters` (`server_id`)",
                    )
                    db.execSQL(
                        "CREATE TABLE IF NOT EXISTS `device_log` " +
                            "(`id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, " +
                            "`created_at_millis` INTEGER NOT NULL, `kind` TEXT NOT NULL, " +
                            "`waiter_name` TEXT, `description` TEXT NOT NULL, " +
                            "`captured_at_millis` INTEGER)",
                    )
                }
            }

        /**
         * 2 → 3: `known_waiters` cambia sus columnas de intentos (sin reloj): se copia la tabla con
         * los fallos que llevaba y la entrada sin conexión cerrada si estaba bloqueada. SQLite de
         * Android 8 no puede quitar columnas: se crea la tabla nueva, se copian las filas y se
         * cambia el nombre, como indica Room. El SQL es el de `schemas/…/3.json`.
         */
        val MIGRATION_2_3: Migration =
            object : Migration(2, 3) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    db.execSQL(
                        "CREATE TABLE IF NOT EXISTS `known_waiters_v3` " +
                            "(`user_id` TEXT NOT NULL, `server_id` TEXT NOT NULL, " +
                            "`name` TEXT NOT NULL, `initial` TEXT NOT NULL, `color` TEXT, " +
                            "`pin_version` INTEGER NOT NULL, `sealed_verifier` BLOB, " +
                            "`sealed_offline_credential` BLOB, " +
                            "`last_online_entry_millis` INTEGER NOT NULL, " +
                            "`offline_failures` INTEGER NOT NULL, " +
                            "`offline_closed` INTEGER NOT NULL, PRIMARY KEY(`user_id`))",
                    )
                    db.execSQL(
                        "INSERT INTO `known_waiters_v3` (`user_id`, `server_id`, `name`, " +
                            "`initial`, `color`, `pin_version`, `sealed_verifier`, " +
                            "`sealed_offline_credential`, `last_online_entry_millis`, " +
                            "`offline_failures`, `offline_closed`) " +
                            "SELECT `user_id`, `server_id`, `name`, `initial`, `color`, " +
                            "`pin_version`, `sealed_verifier`, `sealed_offline_credential`, " +
                            "`last_online_entry_millis`, `local_failures`, " +
                            "CASE WHEN `local_locked_until_millis` IS NULL THEN 0 ELSE 1 END " +
                            "FROM `known_waiters`",
                    )
                    db.execSQL("DROP TABLE `known_waiters`")
                    db.execSQL("ALTER TABLE `known_waiters_v3` RENAME TO `known_waiters`")
                    db.execSQL(
                        "CREATE INDEX IF NOT EXISTS `index_known_waiters_server_id` " +
                            "ON `known_waiters` (`server_id`)",
                    )
                }
            }

        /**
         * 3 → 4 (Fase E): tablas nuevas del modo Mesero (capturas, cola de pedidos, lo último leído
         * de Go y los permisos de cada mesero); las demás no cambian. El SQL es el que genera Room
         * para la versión 4 (`schemas/…/4.json`).
         */
        val MIGRATION_3_4: Migration =
            object : Migration(3, 4) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    for (statement in WAITER_TABLES_V4) {
                        db.execSQL(statement)
                    }
                }
            }

        /**
         * 4 → 5 (revisión R6 en el dispositivo): columnas nuevas de la cola y las capturas (la
         * cuenta que nació con la captura, el guardado sin respuesta de un pedido y si su hora ya
         * es la de Go); los datos que había no cambian. El SQL es el de `schemas/…/5.json`.
         */
        val MIGRATION_4_5: Migration =
            object : Migration(4, 5) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    for (statement in WAITER_COLUMNS_V5) {
                        db.execSQL(statement)
                    }
                }
            }

        /** Migraciones de todas las versiones, en orden. */
        val MIGRATIONS: Array<Migration> =
            arrayOf(MIGRATION_1_2, MIGRATION_2_3, MIGRATION_3_4, MIGRATION_4_5)

        fun open(context: Context): AppDatabase =
            Room.databaseBuilder(context.applicationContext, AppDatabase::class.java, FILE_NAME)
                .addMigrations(*MIGRATIONS)
                .build()
    }
}
