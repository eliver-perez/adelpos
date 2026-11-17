package local.adel.pos.android.core.data.catalog.db

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration

/**
 * Base propia del catálogo (`catalog.db`), separada de `AppDatabase`: es una **copia del catálogo
 * de Go** que se puede reemplazar completa (`reset` o época distinta) sin tocar la base de la
 * aplicación, que guarda la cola de pedidos y el acceso. Tiene su propio ciclo de versiones y su
 * esquema exportado en `core/data/schemas/…CatalogDatabase/`.
 *
 * Reglas para los cambios de esquema: subir [version][Database.version], agregar la `Migration` a
 * [MIGRATIONS] y un caso en `CatalogDatabaseMigrationTest`. Aunque el contenido se puede volver a
 * descargar, no se usa `fallbackToDestructiveMigration`: una migración explícita conserva el
 * catálogo para capturar sin conexión mientras no hay red.
 */
@Database(
    entities =
        [
            CatalogStateEntity::class,
            CategoryEntity::class,
            ProductEntity::class,
            OptionGroupEntity::class,
            IngredientEntity::class,
            ComplementEntity::class,
            PortionEntity::class,
            QuickNoteEntity::class,
            ImageRefEntity::class,
            ImageFileEntity::class,
        ],
    version = 1,
    exportSchema = true,
)
abstract class CatalogDatabase : RoomDatabase() {
    abstract fun catalog(): CatalogDao

    companion object {
        const val FILE_NAME = "catalog.db"

        /**
         * Migraciones de todas las versiones, en orden. Vacía mientras solo existe la versión 1.
         */
        val MIGRATIONS: Array<Migration> = emptyArray()

        fun open(context: Context): CatalogDatabase =
            Room.databaseBuilder(context.applicationContext, CatalogDatabase::class.java, FILE_NAME)
                .addMigrations(*MIGRATIONS)
                .build()
    }
}
