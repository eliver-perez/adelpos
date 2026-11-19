package com.adelpos.meseros.core.data.catalog.db

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Upsert

/** Fila mínima para buscar productos: su identificador y su clave normalizada. */
data class ProductSearchRow(val id: String, @ColumnInfo(name = "search_key") val searchKey: String)

/**
 * Acceso a catalog.db. Las inserciones de la descarga completa usan ABORT: un identificador
 * repetido hace fallar la transacción entera y el catálogo anterior queda intacto. La descarga de
 * cambios usa upsert. Las listas de identificadores se pasan en bloques (`CatalogStore`) para no
 * pasar del límite de variables de SQLite en Android 8.
 */
@Dao
interface CatalogDao {
    @Query("SELECT * FROM catalog_state WHERE id = 1") suspend fun state(): CatalogStateEntity?

    @Upsert suspend fun saveState(state: CatalogStateEntity)

    // Descarga completa: inserción estricta.

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertCategories(items: List<CategoryEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertProducts(items: List<ProductEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertGroups(items: List<OptionGroupEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertIngredients(items: List<IngredientEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertComplements(items: List<ComplementEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertPortions(items: List<PortionEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertQuickNotes(items: List<QuickNoteEntity>)

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertImages(items: List<ImageRefEntity>)

    // Descarga de cambios: alta o reemplazo de cada registro que cambió.

    @Upsert suspend fun upsertCategories(items: List<CategoryEntity>)

    @Upsert suspend fun upsertProducts(items: List<ProductEntity>)

    @Upsert suspend fun upsertGroups(items: List<OptionGroupEntity>)

    @Upsert suspend fun upsertIngredients(items: List<IngredientEntity>)

    @Upsert suspend fun upsertComplements(items: List<ComplementEntity>)

    @Upsert suspend fun upsertPortions(items: List<PortionEntity>)

    @Upsert suspend fun upsertQuickNotes(items: List<QuickNoteEntity>)

    @Upsert suspend fun upsertImages(items: List<ImageRefEntity>)

    // Bajas (deleted) y productos archivados.

    @Query("DELETE FROM categories WHERE id IN (:ids)")
    suspend fun deleteCategories(ids: List<String>)

    @Query("DELETE FROM products WHERE id IN (:ids)") suspend fun deleteProducts(ids: List<String>)

    @Query("DELETE FROM option_groups WHERE id IN (:ids)")
    suspend fun deleteGroups(ids: List<String>)

    @Query("DELETE FROM ingredients WHERE id IN (:ids)")
    suspend fun deleteIngredients(ids: List<String>)

    @Query("DELETE FROM complements WHERE id IN (:ids)")
    suspend fun deleteComplements(ids: List<String>)

    @Query("DELETE FROM portions WHERE id IN (:ids)") suspend fun deletePortions(ids: List<String>)

    @Query("DELETE FROM quick_notes WHERE id IN (:ids)")
    suspend fun deleteQuickNotes(ids: List<String>)

    @Query("DELETE FROM images WHERE id IN (:ids)") suspend fun deleteImages(ids: List<String>)

    // Reemplazo completo.

    @Query("DELETE FROM categories") suspend fun clearCategories()

    @Query("DELETE FROM products") suspend fun clearProducts()

    @Query("DELETE FROM option_groups") suspend fun clearGroups()

    @Query("DELETE FROM ingredients") suspend fun clearIngredients()

    @Query("DELETE FROM complements") suspend fun clearComplements()

    @Query("DELETE FROM portions") suspend fun clearPortions()

    @Query("DELETE FROM quick_notes") suspend fun clearQuickNotes()

    @Query("DELETE FROM images") suspend fun clearImages()

    // Lectura del catálogo completo para la captura.

    @Query("SELECT * FROM categories") suspend fun categories(): List<CategoryEntity>

    @Query("SELECT * FROM products") suspend fun products(): List<ProductEntity>

    @Query("SELECT * FROM option_groups") suspend fun groups(): List<OptionGroupEntity>

    @Query("SELECT * FROM ingredients") suspend fun ingredients(): List<IngredientEntity>

    @Query("SELECT * FROM complements") suspend fun complements(): List<ComplementEntity>

    @Query("SELECT * FROM quick_notes") suspend fun quickNotes(): List<QuickNoteEntity>

    @Query("SELECT * FROM images") suspend fun images(): List<ImageRefEntity>

    @Query("SELECT * FROM images WHERE id = :id") suspend fun image(id: String): ImageRefEntity?

    @Query("SELECT COUNT(*) FROM products") suspend fun productCount(): Int

    /**
     * Productos activos cuya clave contiene [pattern] (un `LIKE` con `\` como escape), en el orden
     * de la captura; [categoryId] nulo = todo el catálogo.
     */
    @Query(
        "SELECT id, search_key FROM products " +
            "WHERE active = 1 " +
            "AND (:categoryId IS NULL OR category_id = :categoryId) " +
            "AND search_key LIKE :pattern ESCAPE '\\' " +
            "ORDER BY sort_order, name",
    )
    suspend fun searchProducts(pattern: String, categoryId: String?): List<ProductSearchRow>

    // Caché de imágenes.

    @Query("SELECT * FROM image_files WHERE id = :id")
    suspend fun imageFile(id: String): ImageFileEntity?

    @Query("SELECT * FROM image_files ORDER BY last_used_millis")
    suspend fun imageFilesOldestFirst(): List<ImageFileEntity>

    @Upsert suspend fun saveImageFile(file: ImageFileEntity)

    @Query("UPDATE image_files SET last_used_millis = :millis WHERE id = :id")
    suspend fun touchImageFile(id: String, millis: Long)

    /** Después de un `reset`: cada imagen guardada se vuelve a preguntar con su `ETag`. */
    @Query("UPDATE image_files SET needs_check = 1") suspend fun markImageFilesForCheck()

    @Query("DELETE FROM image_files WHERE id IN (:ids)")
    suspend fun deleteImageFiles(ids: List<String>)

    // Borrar los datos de este dispositivo (retirar o emparejar de nuevo).

    @Query("DELETE FROM catalog_state") suspend fun clearState()

    @Query("DELETE FROM image_files") suspend fun clearImageFiles()
}
