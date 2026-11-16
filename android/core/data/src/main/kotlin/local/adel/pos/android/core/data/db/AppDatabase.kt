package local.adel.pos.android.core.data.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

/**
 * Base local del dispositivo (Room). El esquema de cada versión se exporta a `core/data/schemas/` y
 * se versiona.
 * - Versión 1 (Fase A): preferencias del dispositivo.
 * - Versión 2 (fases B y C): servidor emparejado, meseros conocidos (verificador del PIN y
 *   credencial de envío cifrados con el Keystore) y bitácora del dispositivo.
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
        ],
    version = 2,
    exportSchema = true,
)
abstract class AppDatabase : RoomDatabase() {
    abstract fun devicePreferences(): DevicePreferenceDao

    abstract fun access(): AccessDao

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

        /** Migraciones de todas las versiones, en orden. */
        val MIGRATIONS: Array<Migration> = arrayOf(MIGRATION_1_2)

        fun open(context: Context): AppDatabase =
            Room.databaseBuilder(context.applicationContext, AppDatabase::class.java, FILE_NAME)
                .addMigrations(*MIGRATIONS)
                .build()
    }
}
