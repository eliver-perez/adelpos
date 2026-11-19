package com.adelpos.meseros.core.data.catalog

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.catalog.CatalogTestSupport.error
import com.adelpos.meseros.core.data.catalog.CatalogTestSupport.fixture
import com.adelpos.meseros.core.data.catalog.CatalogTestSupport.json
import com.adelpos.meseros.core.data.catalog.db.CatalogDatabase
import com.adelpos.meseros.core.data.catalog.db.ImageFileEntity
import com.adelpos.meseros.core.data.session.FixedServerAccess
import com.adelpos.meseros.core.domain.catalog.AvailabilityStatus
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogQuery
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.catalog.QuickNoteEffect
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.realtime.AccessInterruption
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Descarga del catálogo contra MockWebServer con los JSON de ejemplo del contrato
 * (`src/test/resources/catalog`), guardada en catalog.db (Room en Robolectric, SDK 36).
 */
@RunWith(AndroidJUnit4::class)
class CatalogSyncTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var mock: MockWebServer
    private lateinit var database: CatalogDatabase
    private lateinit var access: FixedServerAccess
    private lateinit var store: CatalogStore
    private lateinit var repository: CatalogRepository
    private lateinit var scope: CoroutineScope
    private val interruptions = mutableListOf<AccessInterruption>()

    @Before
    fun setUp() {
        mock = MockWebServer()
        mock.start()
        database = Room.inMemoryDatabaseBuilder(context, CatalogDatabase::class.java).build()
        access = FixedServerAccess(CatalogTestSupport.server(mock))
        store = CatalogStore(database)
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        val synchronizer =
            CatalogSynchronizer(
                access = access,
                api = CatalogApi(),
                store = store,
                interruptions = { interruption, _ -> interruptions.add(interruption) },
                clock = { 1_000L },
            )
        repository = CatalogRepository(store, synchronizer, images = null, scope = scope)
    }

    @After
    fun tearDown() {
        scope.cancel()
        database.close()
        mock.close()
    }

    @Test
    fun borrarLosDatosDelDispositivoVaciaElCatalogoYLaSiguienteDescargaEsCompleta() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        repository.refreshNow()
        mock.takeRequest()
        assertTrue(repository.catalog.value.products.isNotEmpty())

        // Retirar el dispositivo o emparejarlo de nuevo (DeviceDataWipe).
        repository.eraseAll()
        assertTrue(repository.catalog.value.products.isEmpty())
        assertNull(repository.catalog.value.version)
        assertNull(store.state())
        assertEquals(0, store.productCount())
        assertEquals(emptySet<String>(), store.referencedImageIds())

        mock.enqueue(json(200, fixture("full.json")))
        repository.refreshNow()
        assertNull(
            "sin versión guardada se descarga todo otra vez",
            mock.takeRequest().url.queryParameter("since"),
        )
    }

    @Test
    fun primeraDescargaCompletaConPreciosDeReferenciaYAgotados() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        val outcome = repository.refreshNow()
        assertEquals(
            CatalogSyncOutcome.Saved(full = true, reset = false, version = 41, records = 13),
            outcome,
        )

        val request = mock.takeRequest()
        assertEquals("/api/v1/catalog/sync", request.url.encodedPath)
        assertNull("La primera descarga es completa", request.url.queryParameter("since"))
        assertEquals(CatalogTestSupport.DEVICE_ID, request.headers["X-Device-ID"])
        assertEquals(CatalogTestSupport.DEVICE_TOKEN, request.headers["X-Device-Token"])
        assertEquals("Bearer ${CatalogTestSupport.SESSION}", request.headers["Authorization"])

        val catalog = repository.catalog.value
        assertEquals(41L, catalog.version?.version)
        assertEquals("General", catalog.version?.priceListName)
        assertEquals(listOf("Hamburguesas", "Bebidas"), catalog.categories.map { it.name })
        assertEquals(
            listOf("Hamburguesa clásica", "Combo clásico", "Refresco", "Café de olla"),
            catalog.products.map { it.name },
        )
        val burger = catalog.products.first { it.code == "HAM-01" }
        assertEquals(Money("120.00"), burger.referencePrice)
        assertEquals(CatalogTestSupport.IMAGE_ID, burger.imageId)
        assertEquals("Cebolla", burger.ingredients.single().name)
        assertEquals("1", burger.groups.single().included.single().quantity)

        // Sin precio base: se elige una presentación (cada una con su precio de referencia).
        val soda = catalog.products.first { it.code == "REF" }
        assertTrue(soda.requiresVariant)
        assertEquals(
            listOf(Money("25.00"), Money("35.00")),
            soda.variants.map { it.referencePrice },
        )
        assertEquals("El producto no tiene precio en la lista predeterminada.", soda.pricingError)

        val coffee = catalog.products.first { it.code == "CAF" }
        assertEquals(AvailabilityStatus.SOLD_OUT_INDEFINITE, coffee.availability.status)
        assertFalse(coffee.selectable)

        val combo = catalog.products.first { it.kind == ProductKind.COMBO }
        val slot = combo.slots.single()
        assertEquals(listOf(soda.id, coffee.id), slot.productIds)
        assertTrue(slot.option(coffee.id)!!.soldOut)

        // Las opciones toman su nombre del documento o, si no lo trae, del complemento.
        val side = catalog.group("f1000000-0000-4000-8000-000000000001")!!
        assertEquals(listOf("Papas", "Ensalada · Media"), side.options.map { it.displayName })
        assertEquals(Money("8.00"), side.options[1].firstTierPrice)

        val notes = catalog.quickNotesFor(burger)
        assertEquals(listOf(QuickNoteEffect.OMIT, QuickNoteEffect.NONE), notes.map { it.effect })
    }

    @Test
    fun laSegundaDescargaPideSoloLosCambiosConVersionYEpoca() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        mock.enqueue(json(200, fixture("delta.json")))
        repository.refreshNow()
        val outcome = repository.refreshNow()
        assertEquals(
            CatalogSyncOutcome.Saved(full = false, reset = false, version = 45, records = 6),
            outcome,
        )

        mock.takeRequest()
        val delta = mock.takeRequest()
        assertEquals("41", delta.url.queryParameter("since"))
        assertEquals("6f1f5f3e-1c2b-4d3e-9a8b-7c6d5e4f3a21", delta.url.queryParameter("epoch"))

        val catalog = repository.catalog.value
        assertEquals(45L, catalog.version?.version)
        // Precio cambiado y agotado hasta el día siguiente.
        val burger = catalog.products.first { it.code == "HAM-01" }
        assertEquals(Money("130.00"), burger.referencePrice)
        assertEquals(AvailabilityStatus.SOLD_OUT, burger.availability.status)
        assertEquals("2026-11-17T11:00:00Z", burger.availability.soldOutUntil)
        assertFalse(burger.selectable)
        // Producto nuevo, archivado que se quita y combo borrado (deleted).
        assertEquals(
            listOf("Hamburguesa clásica", "Refresco", "Jamaica"),
            catalog.products.map { it.name },
        )
        assertNull(catalog.product("b1000000-0000-4000-8000-000000000003"))
        // La indicación borrada ya no se ofrece.
        assertEquals(listOf("Sin cebolla"), catalog.quickNotesFor(burger).map { it.text })
    }

    @Test
    fun conResetOEpocaDistintaSeReemplazaTodoYSeRevalidanLasImagenes() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        mock.enqueue(json(200, fixture("reset.json")))
        repository.refreshNow()
        database
            .catalog()
            .saveImageFile(
                ImageFileEntity(CatalogTestSupport.IMAGE_ID, "\"etiqueta\"", 75, 1, false),
            )

        val outcome = repository.refreshNow()
        assertEquals(
            CatalogSyncOutcome.Saved(full = true, reset = true, version = 7, records = 2),
            outcome,
        )
        val catalog = repository.catalog.value
        assertEquals("0b0b0b0b-1111-4222-8333-444444444444", catalog.version?.epoch)
        assertEquals(8, catalog.version?.taxRate)
        assertEquals(listOf("Postres"), catalog.categories.map { it.name })
        assertEquals(listOf("Pastel de elote"), catalog.products.map { it.name })
        assertTrue(catalog.group("f1000000-0000-4000-8000-000000000001") == null)
        // La copia de la imagen se conserva, pero se volverá a preguntar con su ETag.
        assertTrue(database.catalog().imageFile(CatalogTestSupport.IMAGE_ID)!!.needsCheck)

        // La descarga siguiente ya usa la época y la versión nuevas.
        mock.enqueue(
            json(200, fixture("reset.json").replace("\"reset\": true", "\"reset\": false")),
        )
        repository.refreshNow()
        mock.takeRequest()
        mock.takeRequest()
        val next = mock.takeRequest()
        assertEquals("7", next.url.queryParameter("since"))
        assertEquals("0b0b0b0b-1111-4222-8333-444444444444", next.url.queryParameter("epoch"))
    }

    @Test
    fun unaDescargaCompletaInvalidaNoDejaElCatalogoAMedias() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        repository.refreshNow()
        // Un identificador repetido hace fallar la inserción a mitad del reemplazo.
        val full = fixture("full.json")
        val duplicated =
            full.replace(
                "\"id\": \"b1000000-0000-4000-8000-000000000003\"",
                "\"id\": \"b1000000-0000-4000-8000-000000000001\"",
            )
        mock.enqueue(json(200, duplicated))
        val outcome = repository.refreshNow()
        assertEquals(CatalogSyncOutcome.Failed(AppError.InvalidResponse(200)), outcome)
        assertEquals(41L, store.state()?.version)
        assertEquals(4, store.productCount())
        assertEquals(AppError.InvalidResponse(200), repository.status.value.lastError)
    }

    @Test
    fun busquedaSinAcentosNiMayusculasSobreLaBaseLocal() = runBlocking {
        mock.enqueue(json(200, fixture("full.json")))
        repository.refreshNow()
        assertEquals(listOf("Café de olla"), names(repository.search(CatalogQuery("CAFE"))))
        assertEquals(
            listOf("Hamburguesa clásica"),
            names(repository.search(CatalogQuery("clasica ham"))),
        )
        assertEquals(listOf("Refresco"), names(repository.search(CatalogQuery("7501055"))))
        assertEquals(emptyList<String>(), names(repository.search(CatalogQuery("100%"))))
        assertEquals(emptyList<String>(), names(repository.search(CatalogQuery("a_b"))))
        // La búsqueda ignora la categoría elegida; sin texto, los productos de la categoría.
        assertEquals(
            listOf("Refresco", "Café de olla"),
            names(repository.search(CatalogQuery("", "c1000000-0000-4000-8000-000000000002"))),
        )
        assertEquals(
            listOf("Hamburguesa clásica"),
            names(
                repository.search(
                    CatalogQuery("hamburguesa", "c1000000-0000-4000-8000-000000000002"),
                ),
            ),
        )
    }

    @Test
    fun sinSesionNoSeDescargaNada() = runBlocking {
        access.set(null)
        assertEquals(CatalogSyncOutcome.NoSession, repository.refreshNow())
        assertEquals(0, mock.requestCount)
        assertTrue(repository.status.value.waitingForSession)
    }

    @Test
    fun sesionTerminadaSeAvisaAlAccesoYSinPermisoNo() = runBlocking {
        mock.enqueue(error(401, "SESSION_EXPIRED"))
        mock.enqueue(error(403, "PERMISSION_DENIED"))
        val expired = repository.refreshNow()
        assertTrue(expired is CatalogSyncOutcome.Failed)
        assertEquals(listOf(AccessInterruption.SessionEnded("SESSION_EXPIRED")), interruptions)
        val denied = repository.refreshNow() as CatalogSyncOutcome.Failed
        assertTrue(denied.error is AppError.PermissionDenied)
        assertEquals(1, interruptions.size)
    }

    @Test
    fun variasPeticionesDuranteUnaDescargaHacenSoloUnaMas() = runBlocking {
        mock.enqueue(
            MockResponse.Builder()
                .code(200)
                .addHeader("Content-Type", "application/json")
                .body(fixture("full.json"))
                .headersDelay(400, TimeUnit.MILLISECONDS)
                .build(),
        )
        mock.enqueue(json(200, fixture("full.json").replace("\"full\": true", "\"full\": false")))
        repository.start()
        repository.requestRefresh()
        // Mientras la primera espera la respuesta, llegan más peticiones.
        delay(100)
        repeat(5) { repository.requestRefresh() }
        withTimeout(5_000) {
            while (mock.requestCount < 2 || repository.status.value.running) {
                delay(20)
            }
        }
        delay(300)
        assertEquals(2, mock.requestCount)
        assertEquals(41L, repository.catalog.value.version?.version)
    }

    private fun names(products: List<CatalogProduct>) = products.map { it.name }
}
