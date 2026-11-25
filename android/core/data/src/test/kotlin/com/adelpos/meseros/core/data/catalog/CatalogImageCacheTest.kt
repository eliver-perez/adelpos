package com.adelpos.meseros.core.data.catalog

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.core.network.session.FixedServerAccess
import com.adelpos.meseros.core.data.catalog.CatalogTestSupport.IMAGE_BYTES
import com.adelpos.meseros.core.data.catalog.CatalogTestSupport.IMAGE_ID
import com.adelpos.meseros.core.data.catalog.db.CatalogDatabase
import com.adelpos.meseros.core.data.catalog.db.ImageRefEntity
import java.io.File
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okio.Buffer
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Caché propia de imágenes: descarga aparte, 304 con `If-None-Match`, comprobación del checksum y
 * limpieza de las que ya no usa el catálogo o pasan del límite.
 */
@RunWith(AndroidJUnit4::class)
class CatalogImageCacheTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val directory = File(context.cacheDir, "prueba-imagenes")
    private val otherId = "d1000000-0000-4000-8000-000000000002"
    private val otherBytes = ByteArray(100) { 3 }
    private lateinit var mock: MockWebServer
    private lateinit var database: CatalogDatabase
    private lateinit var access: FixedServerAccess
    private var now = 1_000L

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
        database = Room.inMemoryDatabaseBuilder(context, CatalogDatabase::class.java).build()
        access = FixedServerAccess(CatalogTestSupport.server(mock))
        directory.deleteRecursively()
        runBlocking {
            database
                .catalog()
                .upsertImages(
                    listOf(
                        reference(IMAGE_ID, IMAGE_BYTES),
                        reference(otherId, otherBytes),
                    ),
                )
        }
    }

    @After
    fun tearDown() {
        database.close()
        mock.close()
        directory.deleteRecursively()
    }

    private fun reference(id: String, bytes: ByteArray) =
        ImageRefEntity(id, "image/png", 4, 3, bytes.size, CatalogImageCache.sha256(bytes))

    private fun cache(maximumBytes: Long = CatalogImageCache.DEFAULT_MAXIMUM_BYTES) =
        CatalogImageCache(
            directory = directory,
            dao = database.catalog(),
            api = CatalogApi(),
            access = access,
            clock = { now },
            maximumBytes = maximumBytes,
        )

    private fun image(bytes: ByteArray): MockResponse =
        MockResponse.Builder()
            .code(200)
            .addHeader("Content-Type", "image/png")
            .addHeader("ETag", "\"${CatalogImageCache.sha256(bytes)}\"")
            .body(Buffer().write(bytes))
            .build()

    @Test
    fun descargaAparteYDespuesUsaLaCopiaSinPedirla() = runBlocking {
        val cache = cache()
        assertNull(cache.cached(IMAGE_ID))
        mock.enqueue(image(IMAGE_BYTES))
        val file = cache.obtain(IMAGE_ID)
        assertNotNull(file)
        assertArrayEquals(IMAGE_BYTES, file!!.readBytes())
        val request = mock.takeRequest()
        assertEquals("/api/v1/catalog/images/$IMAGE_ID/content", request.url.encodedPath)
        assertNull(request.headers["If-None-Match"])
        assertEquals("Bearer ${CatalogTestSupport.SESSION}", request.headers["Authorization"])

        // La segunda vez no hay petición: el contenido de una imagen nunca cambia.
        assertEquals(file, cache.obtain(IMAGE_ID))
        assertEquals(file, cache.cached(IMAGE_ID))
        assertEquals(1, mock.requestCount)
    }

    @Test
    fun despuesDeUnResetSeRevalidaConSuEtiquetaYGoResponde304() = runBlocking {
        val cache = cache()
        mock.enqueue(image(IMAGE_BYTES))
        cache.obtain(IMAGE_ID)
        mock.takeRequest()
        database.catalog().markImageFilesForCheck()

        mock.enqueue(MockResponse.Builder().code(304).build())
        val file = cache.obtain(IMAGE_ID)
        val revalidation = mock.takeRequest()
        assertEquals(
            "\"${CatalogImageCache.sha256(IMAGE_BYTES)}\"",
            revalidation.headers["If-None-Match"],
        )
        assertArrayEquals(IMAGE_BYTES, file!!.readBytes())
        assertFalse(database.catalog().imageFile(IMAGE_ID)!!.needsCheck)
    }

    @Test
    fun borrarLosDatosDelDispositivoBorraLasImagenesYSuRegistro() = runBlocking {
        val cache = cache()
        mock.enqueue(image(IMAGE_BYTES))
        assertNotNull(cache.obtain(IMAGE_ID))
        cache.eraseAll()
        assertNull(cache.cached(IMAGE_ID))
        assertFalse(directory.exists())
        assertNull(database.catalog().imageFile(IMAGE_ID))
    }

    @Test
    fun unContenidoQueNoCorrespondeAlChecksumNoSeGuarda() = runBlocking {
        val cache = cache()
        mock.enqueue(image(byteArrayOf(1, 2, 3)))
        assertNull(cache.obtain(IMAGE_ID))
        assertNull(cache.cached(IMAGE_ID))
        assertNull(database.catalog().imageFile(IMAGE_ID))
    }

    @Test
    fun sinSesionNoDescargaYUnIdInvalidoNuncaSaleDeLaCarpeta() = runBlocking {
        val cache = cache()
        access.set(null)
        assertNull(cache.obtain(IMAGE_ID))
        assertNull(cache.obtain("../../databases/catalog.db"))
        assertEquals(0, mock.requestCount)
    }

    @Test
    fun limpiezaDeLasQueYaNoSeUsanYDeLasMenosUsadasAlPasarDelLimite() = runBlocking {
        val cache = cache(maximumBytes = 120)
        mock.enqueue(image(IMAGE_BYTES))
        mock.enqueue(image(otherBytes))
        cache.obtain(IMAGE_ID)
        now = 2_000L
        cache.obtain(otherId)
        // 75 + 100 bytes pasan del límite de 120: se borra la menos usada recientemente.
        cache.prune(setOf(IMAGE_ID, otherId))
        assertNull(cache.cached(IMAGE_ID))
        assertNotNull(cache.cached(otherId))
        // Una imagen que el catálogo ya no usa se borra.
        cache.prune(emptySet())
        assertNull(cache.cached(otherId))
        assertTrue(database.catalog().imageFilesOldestFirst().isEmpty())
    }

    @Test
    fun laDescargaAnticipadaSeDetieneEnElLimite() = runBlocking {
        val cache = cache(maximumBytes = 50)
        mock.enqueue(image(IMAGE_BYTES))
        mock.enqueue(image(otherBytes))
        cache.prefetch(listOf(IMAGE_ID, otherId))
        assertNotNull(cache.cached(IMAGE_ID))
        assertNull(cache.cached(otherId))
        assertEquals(1, mock.requestCount)
    }
}
