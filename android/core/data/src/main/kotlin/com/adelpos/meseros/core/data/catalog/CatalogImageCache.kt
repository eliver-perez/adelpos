package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.data.catalog.db.CatalogDao
import com.adelpos.meseros.core.data.catalog.db.ImageFileEntity
import com.adelpos.meseros.core.data.session.ServerAccess
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.error.AppResult
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/**
 * Caché propia de las imágenes del catálogo, en la carpeta de caché de la aplicación
 * (`cacheDir/catalog-images/<id>`; Android la puede vaciar si le falta espacio y se vuelven a
 * descargar). Reglas:
 * - Se descargan aparte de la descarga del catálogo, de una en una, con `If-None-Match` cuando ya
 *   hay una copia (Go responde 304 si no cambió; el contenido de un id nunca cambia, así que solo
 *   se revalida después de un `reset`).
 * - Se comprueba que el contenido corresponda a su `checksum` (SHA-256 de Go) antes de guardarlo.
 * - Límite de [maximumBytes]: al pasarlo se borran las menos usadas; las que ya no usa el catálogo
 *   se borran en cada [prune].
 */
class CatalogImageCache(
    private val directory: File,
    private val dao: CatalogDao,
    private val api: CatalogApi,
    private val access: ServerAccess,
    private val clock: () -> Long = System::currentTimeMillis,
    private val maximumBytes: Long = DEFAULT_MAXIMUM_BYTES,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) : CatalogImages {
    private val downloads = Mutex()
    private val arrived = MutableSharedFlow<String>(extraBufferCapacity = 64)

    override val arrivals: Flow<String> = arrived.asSharedFlow()

    override fun cached(imageId: String): File? {
        if (!validId(imageId)) {
            return null
        }
        val file = fileOf(imageId)
        return if (file.isFile) file else null
    }

    override suspend fun obtain(imageId: String): File? {
        if (!validId(imageId)) {
            return null
        }
        return downloads.withLock { obtainLocked(imageId) }
    }

    /**
     * Descarga, de una en una, las imágenes del catálogo que faltan o que hay que revalidar,
     * mientras quepan en el límite. Sin conexión o sin sesión se detiene: se vuelve a intentar en
     * la siguiente descarga del catálogo o cuando una pantalla pide la imagen.
     */
    suspend fun prefetch(imageIds: Collection<String>) {
        for (imageId in imageIds) {
            if (totalBytes() >= maximumBytes) {
                return
            }
            val entry = dao.imageFile(imageId)
            if (entry != null && !entry.needsCheck && fileOf(imageId).isFile) {
                continue
            }
            if (obtain(imageId) == null && access.current.value == null) {
                return
            }
        }
    }

    /**
     * Borra las imágenes que el catálogo ya no usa y, si se pasa del límite, las menos usadas
     * recientemente.
     */
    suspend fun prune(referenced: Set<String>) {
        downloads.withLock {
            val entries = dao.imageFilesOldestFirst()
            val unused = entries.filter { it.id !in referenced }
            remove(unused.map { it.id })
            var total = entries.filter { it.id in referenced }.sumOf { it.sizeBytes }
            val removable = mutableListOf<String>()
            for (entry in entries.filter { it.id in referenced }) {
                if (total <= maximumBytes) {
                    break
                }
                removable.add(entry.id)
                total -= entry.sizeBytes
            }
            remove(removable)
            // Archivos que quedaron sin registro (por ejemplo, un proceso que terminó a la mitad).
            withContext(ioDispatcher) {
                val known = dao.imageFilesOldestFirst().map { it.id }.toSet()
                directory.listFiles()?.forEach { file ->
                    if (file.name !in known) {
                        file.delete()
                    }
                }
            }
        }
    }

    /**
     * Borra todas las imágenes guardadas (archivos y su registro): «borrar los datos de este
     * dispositivo». Espera a la descarga en curso para que no deje un archivo después.
     */
    suspend fun eraseAll() {
        downloads.withLock {
            withContext(ioDispatcher) { directory.deleteRecursively() }
            dao.clearImageFiles()
        }
    }

    private suspend fun obtainLocked(imageId: String): File? {
        val file = fileOf(imageId)
        // Una fila sin su archivo (Android vació la caché) cuenta como imagen que falta.
        val entry = dao.imageFile(imageId)?.takeIf { file.isFile }
        if (entry != null && !entry.needsCheck) {
            dao.touchImageFile(imageId, clock())
            return file
        }
        // Si la revalidación no se puede hacer ahora, la copia guardada sigue sirviendo.
        val fallback = if (entry != null) file else null
        val server = access.current.value ?: return fallback
        val reference = dao.image(imageId)
        val result = api.image(server, imageId, entry?.etag)
        if (result !is AppResult.Success) {
            return fallback
        }
        when (val download = result.value) {
            ImageDownload.NotModified -> {
                if (entry == null) {
                    return null
                }
                dao.saveImageFile(entry.copy(lastUsedMillis = clock(), needsCheck = false))
                return file
            }
            is ImageDownload.Content -> {
                // Solo se guarda un contenido que corresponde a la imagen del catálogo.
                if (reference != null && sha256(download.bytes) != reference.checksum) {
                    return fallback
                }
                if (!write(file, download.bytes)) {
                    return fallback
                }
                dao.saveImageFile(
                    ImageFileEntity(
                        id = imageId,
                        etag = download.etag ?: quoted(reference?.checksum),
                        sizeBytes = download.bytes.size.toLong(),
                        lastUsedMillis = clock(),
                        needsCheck = false,
                    ),
                )
                arrived.tryEmit(imageId)
                return file
            }
        }
    }

    private suspend fun write(file: File, bytes: ByteArray): Boolean =
        withContext(ioDispatcher) {
            try {
                directory.mkdirs()
                val partial = File(directory, "${file.name}.partial")
                partial.writeBytes(bytes)
                partial.renameTo(file)
            } catch (_: IOException) {
                false
            }
        }

    private suspend fun remove(ids: List<String>) {
        if (ids.isEmpty()) {
            return
        }
        withContext(ioDispatcher) {
            for (id in ids) {
                fileOf(id).delete()
            }
        }
        for (chunk in ids.chunked(CHUNK_SIZE)) {
            dao.deleteImageFiles(chunk)
        }
    }

    private suspend fun totalBytes(): Long = dao.imageFilesOldestFirst().sumOf { it.sizeBytes }

    private fun fileOf(imageId: String): File = File(directory, imageId)

    companion object {
        /** Límite de la caché: 48 MiB (cientos de fotos de producto de 50–200 KB). */
        const val DEFAULT_MAXIMUM_BYTES: Long = 48L * 1024 * 1024

        /** Nombre de la carpeta dentro de `cacheDir`. */
        const val DIRECTORY_NAME = "catalog-images"

        private const val CHUNK_SIZE = 500

        // El id es un UUID de Go: también es el nombre del archivo, así que se valida para que
        // nunca salga de la carpeta de la caché.
        private val uuid =
            Regex(
                "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
            )

        fun validId(imageId: String): Boolean = uuid.matches(imageId)

        fun sha256(bytes: ByteArray): String =
            MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { byte ->
                "%02x".format(byte)
            }

        private fun quoted(checksum: String?): String = "\"${checksum.orEmpty()}\""
    }
}
