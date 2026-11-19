package com.adelpos.meseros.core.data.catalog.db

import androidx.room.ColumnInfo
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/*
 * Tablas de catalog.db. Cada registro del catálogo guarda su documento JSON tal como lo entregó Go
 * (`document`) más las columnas que la aplicación consulta (orden, categoría, clave de búsqueda).
 * Así la base no tiene que reproducir la estructura completa del catálogo (presentaciones, reglas
 * de ingredientes, ranuras de combos…) y un campo nuevo del contrato v1 queda guardado aunque esta
 * versión todavía no lo use.
 */

/**
 * Versión y época del catálogo guardado (una sola fila). La descarga de cambios las envía a Go
 * (`since`, `epoch`); con otra época Go entrega todo y se reemplaza la base completa.
 */
@Entity(tableName = "catalog_state")
data class CatalogStateEntity(
    @PrimaryKey val id: Int = SINGLE_ROW,
    val version: Long,
    val epoch: String,
    @ColumnInfo(name = "generated_at") val generatedAt: String,
    val timezone: String,
    @ColumnInfo(name = "tax_rate") val taxRate: Int,
    @ColumnInfo(name = "price_list_id") val priceListId: String,
    @ColumnInfo(name = "price_list_name") val priceListName: String,
    @ColumnInfo(name = "synced_at_millis") val syncedAtMillis: Long,
) {
    companion object {
        const val SINGLE_ROW = 1
    }
}

@Entity(tableName = "categories")
data class CategoryEntity(
    @PrimaryKey val id: String,
    val name: String,
    @ColumnInfo(name = "sort_order") val sortOrder: Int,
    val active: Boolean,
    val document: String,
)

/**
 * Producto con su clave de búsqueda ya normalizada (sin acentos ni mayúsculas, ver
 * `CatalogSearch`).
 */
@Entity(tableName = "products", indices = [Index("category_id")])
data class ProductEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "category_id") val categoryId: String,
    val name: String,
    @ColumnInfo(name = "sort_order") val sortOrder: Int,
    val active: Boolean,
    @ColumnInfo(name = "search_key") val searchKey: String,
    @ColumnInfo(name = "image_id") val imageId: String?,
    val document: String,
)

@Entity(tableName = "option_groups")
data class OptionGroupEntity(@PrimaryKey val id: String, val document: String)

@Entity(tableName = "ingredients")
data class IngredientEntity(@PrimaryKey val id: String, val name: String, val document: String)

@Entity(tableName = "complements")
data class ComplementEntity(@PrimaryKey val id: String, val name: String, val document: String)

@Entity(tableName = "portions")
data class PortionEntity(@PrimaryKey val id: String, val name: String, val document: String)

@Entity(tableName = "quick_notes", indices = [Index("category_id")])
data class QuickNoteEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "category_id") val categoryId: String,
    @ColumnInfo(name = "sort_order") val sortOrder: Int,
    val document: String,
)

/** Referencia de una imagen del catálogo (su contenido se descarga aparte). */
@Entity(tableName = "images")
data class ImageRefEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "media_type") val mediaType: String,
    val width: Int,
    val height: Int,
    val size: Int,
    val checksum: String,
)

/**
 * Imagen descargada en la caché propia (`cacheDir/catalog-images/<id>`): su `ETag`, tamaño, último
 * uso (para liberar las menos usadas al pasar del límite) y si hay que volver a preguntarla con
 * `If-None-Match` (después de un `reset` del catálogo). No se borra al reemplazar el catálogo: la
 * imagen se revalida y Go responde 304 si no cambió.
 */
@Entity(tableName = "image_files")
data class ImageFileEntity(
    @PrimaryKey val id: String,
    val etag: String,
    @ColumnInfo(name = "size_bytes") val sizeBytes: Long,
    @ColumnInfo(name = "last_used_millis") val lastUsedMillis: Long,
    @ColumnInfo(name = "needs_check") val needsCheck: Boolean,
)
