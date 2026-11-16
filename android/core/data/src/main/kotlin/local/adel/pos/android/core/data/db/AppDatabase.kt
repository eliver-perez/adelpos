package local.adel.pos.android.core.data.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration

/**
 * Base local del dispositivo (Room). El esquema de cada versión se exporta a `core/data/schemas/` y
 * se versiona.
 *
 * Reglas para las fases siguientes: cada cambio sube [version][Database.version] en uno, agrega su
 * `Migration` a [MIGRATIONS] y una prueba con `MigrationTestHelper` (`AppDatabaseMigrationTest`).
 * Nunca `fallbackToDestructiveMigration`: la base guarda pedidos sin enviar que no se pueden
 * perder.
 */
@Database(entities = [DevicePreference::class], version = 1, exportSchema = true)
abstract class AppDatabase : RoomDatabase() {
    abstract fun devicePreferences(): DevicePreferenceDao

    companion object {
        const val FILE_NAME = "adel-pos.db"

        /**
         * Migraciones de todas las versiones, en orden. Vacía mientras solo existe la versión 1.
         */
        val MIGRATIONS: Array<Migration> = emptyArray()

        fun open(context: Context): AppDatabase =
            Room.databaseBuilder(context.applicationContext, AppDatabase::class.java, FILE_NAME)
                .addMigrations(*MIGRATIONS)
                .build()
    }
}
