package com.adelpos.meseros.core.domain.catalog

import com.adelpos.core.model.error.AppError
import java.io.File
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow

/**
 * Estado de la descarga del catálogo para mostrar en la captura («Actualizando el catálogo…», «No
 * se pudo actualizar»). [lastError] es el error de la última descarga fallida (se borra con la
 * siguiente correcta).
 */
data class CatalogSyncStatus(
    val running: Boolean = false,
    val lastSuccessMillis: Long? = null,
    val lastError: AppError? = null,
    val waitingForSession: Boolean = false,
)

/**
 * Catálogo guardado en el dispositivo para las pantallas (captura del mesero y, en la A2, Para
 * llevar). Lo implementa `core:data` sobre su base propia (`catalog.db`); las pantallas nunca
 * hablan con el servidor para leer el catálogo, así que todo funciona sin conexión.
 */
interface CatalogSource {
    /** El catálogo guardado; emite uno nuevo después de cada descarga que cambió algo. */
    val catalog: StateFlow<Catalog>

    val status: StateFlow<CatalogSyncStatus>

    /** Pide una actualización (una sola a la vez: si ya hay una en curso, se agenda otra). */
    fun requestRefresh()

    /**
     * Productos que coinciden con [query] en la base local, sin distinguir acentos ni mayúsculas,
     * en el orden de la captura.
     */
    suspend fun search(query: CatalogQuery): List<CatalogProduct>
}

/**
 * Imágenes del catálogo en la caché propia del dispositivo (descargadas aparte, con su `ETag`). Las
 * pantallas muestran un marcador mientras la imagen no está.
 */
interface CatalogImages {
    /** El archivo local de la imagen si ya está en la caché (no descarga nada). */
    fun cached(imageId: String): File?

    /**
     * El archivo local de la imagen; si falta, la descarga (con conexión). Null si no se pudo: la
     * pantalla sigue con el marcador.
     */
    suspend fun obtain(imageId: String): File?

    /** Identificadores de imágenes que acaban de quedar en la caché. */
    val arrivals: Flow<String>
}
