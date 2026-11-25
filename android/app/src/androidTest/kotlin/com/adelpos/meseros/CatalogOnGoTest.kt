package com.adelpos.meseros

import android.content.Context
import android.graphics.Bitmap
import android.graphics.Color
import android.os.SystemClock
import android.util.Base64
import android.util.Log
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.room.Room
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.model.realtime.RealtimeKinds
import com.adelpos.core.model.realtime.RealtimeScopes
import com.adelpos.core.model.realtime.RealtimeSignal
import com.adelpos.core.model.realtime.RebuildReason
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.ClientKinds
import com.adelpos.core.network.api.PinnedHttpClient
import com.adelpos.core.network.realtime.RealtimeClient
import com.adelpos.core.network.session.AuthorizedServer
import com.adelpos.core.network.session.FixedServerAccess
import com.adelpos.core.testing.GoTestServer
import com.adelpos.core.testing.MainComputer
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.data.catalog.CatalogApi
import com.adelpos.meseros.core.data.catalog.CatalogImageCache
import com.adelpos.meseros.core.data.catalog.CatalogRepository
import com.adelpos.meseros.core.data.catalog.CatalogStore
import com.adelpos.meseros.core.data.catalog.CatalogSyncOutcome
import com.adelpos.meseros.core.data.catalog.CatalogSynchronizer
import com.adelpos.meseros.core.data.catalog.db.CatalogDatabase
import com.adelpos.meseros.core.data.realtime.RecoveryStateCursorSeed
import com.adelpos.meseros.core.domain.catalog.AvailabilityStatus
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogQuery
import com.adelpos.meseros.core.ui.catalog.CatalogBrowser
import com.adelpos.meseros.core.ui.catalog.productTag
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Catálogo en el dispositivo contra el Go temporal (§9: «catálogo completo y por cambios, `reset` y
 * producto agotado»), en tres etapas que el guion del arnés corre por separado con `-e stage`:
 * - `a`: emparejar y entrar por la API; la principal crea el catálogo (con unos 300 productos para
 *   medir); descarga completa y búsqueda (con tiempos); un cambio de precio, un producto nuevo y
 *   uno archivado llegan por el evento `catalog.changed` del canal y por la descarga de cambios; un
 *   producto agotado desde la principal se ve «Agotado» y no se puede elegir; la imagen responde
 *   304 al revalidarla.
 * - `b`: la principal quita el producto de prueba («Filete de prueba», código `FILETE`, la firma de
 *   los datos de prueba de Go) con «Quitar datos de prueba» (`maintenance/test-data`, su API: lo
 *   borra de verdad porque no tiene operaciones) y la baja llega en `deleted` por la descarga de
 *   cambios.
 * - (el guion cambia la época del catálogo en la base temporal, como una restauración: Go no tiene
 *   API para eso)
 * - `c`: Go responde `reset: true` y el catálogo se reemplaza completo; las imágenes se revalidan.
 *
 * Lo corre `scripts/android-e2e.sh --suite catalog`. Sin los argumentos de instrumentación (`pin`,
 * `net`, `local`, `stage` y la terminal principal `mainDevice`/`mainToken` o `provisioning`) se
 * omite. El estado entre etapas (credenciales e identificadores) queda en un archivo de la
 * aplicación.
 */
@RunWith(AndroidJUnit4::class)
class CatalogOnGoTest {
    @get:Rule val compose = createComposeRule()

    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext
    private val stateFile = File(context.filesDir, "catalog-on-go.json")
    private val imageCodes = CopyOnWriteArrayList<Int>()

    private class Harness(
        val database: CatalogDatabase,
        val repository: CatalogRepository,
        val images: CatalogImageCache,
        val access: FixedServerAccess,
        val scope: CoroutineScope,
    ) {
        fun close() {
            scope.cancel()
            database.close()
        }
    }

    private fun stage(server: GoTestServer, expected: String) {
        assumeTrue("Etapa $expected", server.argument("stage") == expected)
    }

    private fun deviceClient(server: GoTestServer): OkHttpClient =
        PinnedHttpClient.create(
                server.pin,
                ClientIdentity(version = "0.1.0", kind = ClientKinds.ANDROID),
            )
            .newBuilder()
            // Registra el estado de cada petición de imagen (200 o 304) para comprobar la caché.
            .addNetworkInterceptor { chain ->
                val response = chain.proceed(chain.request())
                if (chain.request().url.encodedPath.contains("/catalog/images/")) {
                    imageCodes.add(response.code)
                }
                response
            }
            .build()

    private fun harness(server: GoTestServer, device: GoTestServer.Device): Harness {
        val authorized =
            AuthorizedServer(
                baseUrl = server.network.toHttpUrl(),
                client = deviceClient(server),
                credentials =
                    AuthorizedServer.credentials(
                        device.deviceId,
                        device.deviceToken,
                        device.session,
                    ),
            )
        val access = FixedServerAccess(authorized)
        val database = Room.databaseBuilder(context, CatalogDatabase::class.java, DATABASE).build()
        val store = CatalogStore(database)
        val api = CatalogApi()
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        val images =
            CatalogImageCache(
                directory = File(context.cacheDir, "catalog-images-go-test"),
                dao = database.catalog(),
                api = api,
                access = access,
            )
        val repository =
            CatalogRepository(
                store = store,
                synchronizer = CatalogSynchronizer(access, api, store, interruptions = { _, _ -> }),
                images = null,
                scope = scope,
            )
        return Harness(database, repository, images, access, scope)
    }

    @Test
    fun a_catalogoCompletoCambiosAgotadoEImagen() = runBlocking {
        val server = GoTestServer()
        stage(server, "a")
        context.deleteDatabase(DATABASE)
        File(context.cacheDir, "catalog-images-go-test").deleteRecursively()

        val main = server.mainComputer()
        val device = server.pairAndroid(deviceClient(server), main)
        val ids = createCatalog(server, main)

        val harness = harness(server, device)
        try {
            // Descarga completa (con la medida de tiempo para la tableta económica).
            val started = SystemClock.elapsedRealtime()
            val full = harness.repository.refreshNow()
            val fullMillis = SystemClock.elapsedRealtime() - started
            assertTrue("Descarga completa: $full", full is CatalogSyncOutcome.Saved && full.full)
            val catalog = harness.repository.catalog.value
            assertEquals(FILLER_PRODUCTS + 3, catalog.products.size)
            val burger = catalog.product(ids.getValue("burger"))!!
            assertEquals("120.00", burger.referencePrice?.decimal)
            assertEquals(ids.getValue("image"), burger.imageId)

            val searchStarted = SystemClock.elapsedRealtimeNanos()
            repeat(SEARCH_ROUNDS) { harness.repository.search(CatalogQuery("cafe de olla")) }
            val searchMicros =
                (SystemClock.elapsedRealtimeNanos() - searchStarted) / SEARCH_ROUNDS / 1_000
            val found = harness.repository.search(CatalogQuery("CAFE de OLLA"))
            assertEquals(listOf("Café de olla"), found.map { it.name })
            assertEquals(
                FILLER_PRODUCTS,
                harness.repository.search(CatalogQuery("relleno")).size,
            )
            Log.i(TAG, "Descarga completa de ${catalog.products.size} productos: $fullMillis ms")
            Log.i(TAG, "Búsqueda sin acentos (promedio de $SEARCH_ROUNDS): $searchMicros µs")

            // Tiempo real: el canal (único) recibe catalog.changed y la descarga trae solo cambios.
            val realtime =
                RealtimeClient(
                    access = harness.access,
                    active = MutableStateFlow(true),
                    interruptions = { _, _ -> },
                    cursorSeed = RecoveryStateCursorSeed(),
                )
            val signals = Channel<RealtimeSignal>(Channel.UNLIMITED)
            harness.scope.launch { realtime.signals.collect { signals.send(it) } }
            realtime.start(harness.scope)
            val first = withTimeout(15_000) { signals.receive() }
            assertEquals(RebuildReason.FRESH_START, (first as RealtimeSignal.Rebuild).reason)

            changePrice(server, main, ids.getValue("burger"), "130")
            val jamaica =
                createProduct(server, main, ids.getValue("drinks"), "Jamaica", "JAM", "28")
            archive(server, main, ids.getValue("coffee"))
            awaitCatalogEvent(signals, RealtimeKinds.CATALOG_CHANGED)
            val delta = harness.repository.refreshNow()
            assertTrue("Cambios: $delta", delta is CatalogSyncOutcome.Saved && !delta.full)
            val changed = harness.repository.catalog.value
            assertEquals("130.00", changed.product(ids.getValue("burger"))?.referencePrice?.decimal)
            assertNotNull(changed.product(jamaica))
            assertNull(changed.product(ids.getValue("coffee")))

            // «Agotado» desde la principal: llega por el canal y por la descarga de cambios.
            soldOut(server, main, ids.getValue("burger"), indefinite = false)
            soldOut(server, main, jamaica, indefinite = true)
            awaitCatalogEvent(signals, RealtimeKinds.PRODUCT_SOLD_OUT)
            harness.repository.refreshNow()
            val soldOut = harness.repository.catalog.value
            val soldBurger = soldOut.product(ids.getValue("burger"))!!
            assertEquals(AvailabilityStatus.SOLD_OUT, soldBurger.availability.status)
            assertFalse(soldBurger.selectable)
            assertEquals(
                AvailabilityStatus.SOLD_OUT_INDEFINITE,
                soldOut.product(jamaica)!!.availability.status,
            )
            assertSoldOutOnScreen(soldOut.productsIn(ids.getValue("burgers")), soldBurger.id)

            // Imagen: descarga aparte y 304 al revalidarla con su ETag.
            assertNotNull(harness.images.obtain(ids.getValue("image")))
            harness.database.catalog().markImageFilesForCheck()
            assertNotNull(harness.images.obtain(ids.getValue("image")))
            assertEquals(listOf(200, 304), imageCodes.toList())

            saveState(main, device, ids + ("jamaica" to jamaica), fullMillis, searchMicros)
        } finally {
            harness.close()
        }
    }

    @Test
    fun b_unaBajaLlegaPorCambios() = runBlocking {
        val server = GoTestServer()
        stage(server, "b")
        val saved = loadState()
        removeTestProduct(server, mainComputer(server, saved), saved.text("temporary"))
        val harness = harness(server, device(saved))
        try {
            // La base guarda el catálogo de la etapa a: se piden solo los cambios.
            val outcome = harness.repository.refreshNow()
            assertTrue("Cambios: $outcome", outcome is CatalogSyncOutcome.Saved && !outcome.full)
            val catalog = harness.repository.catalog.value
            assertNull(catalog.product(saved.text("temporary")))
            assertNotNull(catalog.product(saved.text("burger")))
        } finally {
            harness.close()
        }
    }

    @Test
    fun c_epocaDistintaReemplazaTodo() = runBlocking {
        val server = GoTestServer()
        stage(server, "c")
        val saved = loadState()
        val harness = harness(server, device(saved))
        try {
            val previous = harness.database.catalog().state()!!
            val outcome = harness.repository.refreshNow()
            assertTrue(
                "Reemplazo: $outcome",
                outcome is CatalogSyncOutcome.Saved && outcome.full && outcome.reset,
            )
            val state = harness.database.catalog().state()!!
            assertNotEquals(previous.epoch, state.epoch)
            assertEquals(FILLER_PRODUCTS + 2, harness.repository.catalog.value.products.size)
            // Después del reset la imagen guardada se revalida con su ETag: 304.
            assertTrue(harness.database.catalog().imageFile(saved.text("image"))!!.needsCheck)
            assertNotNull(harness.images.obtain(saved.text("image")))
            assertEquals(listOf(304), imageCodes.toList())
        } finally {
            harness.close()
        }
    }

    private fun assertSoldOutOnScreen(products: List<CatalogProduct>, productId: String) {
        var chosen: CatalogProduct? = null
        compose.setContent {
            AdelTheme {
                CatalogBrowser(
                    categories = emptyList(),
                    selectedCategoryId = null,
                    searchText = "",
                    products = products,
                    images = null,
                    onSearchTextChange = {},
                    onCategorySelect = {},
                    onProductSelect = { chosen = it },
                )
            }
        }
        compose
            .onNodeWithTag(productTag(productId))
            .assertTextContains("Agotado", substring = true)
            .assertIsNotEnabled()
            .performClick()
        assertNull("Un producto agotado no se puede elegir", chosen)
    }

    private suspend fun awaitCatalogEvent(signals: Channel<RealtimeSignal>, kind: String) {
        withTimeout(15_000) {
            while (true) {
                val signal = signals.receive()
                if (
                    signal is RealtimeSignal.Changes &&
                        signal.events.any { it.scope == RealtimeScopes.CATALOG && it.kind == kind }
                ) {
                    return@withTimeout
                }
            }
        }
    }

    /** La principal crea categorías, una imagen y los productos (con ~300 de relleno). */
    private fun createCatalog(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
    ): Map<String, String> {
        val burgers = createCategory(server, main, "Hamburguesas", "#d64545")
        val drinks = createCategory(server, main, "Bebidas", "#dd6b26")
        val filler = createCategory(server, main, "Relleno", "#719a26")
        val image = uploadImage(server, main)
        val burger =
            createProduct(server, main, burgers, "Hamburguesa clásica", "HAM-01", "120", image)
        val coffee = createProduct(server, main, drinks, "Café de olla", "CAF", "30")
        // Con la firma exacta de un producto de prueba de Go (maintenance/signatures.go): «Quitar
        // datos de prueba» lo borra en la etapa b.
        val temporary = createProduct(server, main, drinks, "Filete de prueba", "FILETE", "10")
        for (number in 1..FILLER_PRODUCTS) {
            createProduct(server, main, filler, "Producto de relleno $number", "REL-$number", "15")
        }
        return mapOf(
            "burgers" to burgers,
            "drinks" to drinks,
            "image" to image,
            "burger" to burger,
            "coffee" to coffee,
            "temporary" to temporary,
        )
    }

    private fun createCategory(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
        name: String,
        color: String,
    ): String {
        val answer =
            server.main(
                "POST",
                "catalog/categories",
                """{"name":"$name","active":true,"color":"$color"}""",
                main,
            )
        assertEquals(answer.body.toString(), 200, answer.status)
        return answer.text("id")
    }

    private fun createProduct(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
        category: String,
        name: String,
        code: String,
        price: String,
        image: String? = null,
    ): String {
        val imageField =
            if (image != null) {
                ""","image_id":"$image""""
            } else {
                ""
            }
        val answer =
            server.main(
                "POST",
                "catalog/products",
                """{"name":"$name","active":true,"category_id":"$category","code":"$code",""" +
                    """"kind":"product","base_price":"$price"$imageField}""",
                main,
            )
        assertEquals(answer.body.toString(), 200, answer.status)
        return answer.text("id")
    }

    private fun uploadImage(server: GoTestServer, main: GoTestServer.MainComputer): String {
        val bitmap = Bitmap.createBitmap(4, 3, Bitmap.Config.ARGB_8888)
        bitmap.setPixel(1, 1, Color.RED)
        val bytes = ByteArrayOutputStream()
        bitmap.compress(Bitmap.CompressFormat.PNG, 100, bytes)
        val encoded = Base64.encodeToString(bytes.toByteArray(), Base64.NO_WRAP)
        val answer =
            server.main("POST", "catalog/images", """{"content_base64":"$encoded"}""", main)
        assertEquals(answer.body.toString(), 200, answer.status)
        return answer.text("id")
    }

    private fun changePrice(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
        productId: String,
        price: String,
    ) {
        val current = server.main("GET", "catalog/products/$productId", null, main)
        assertEquals(200, current.status)
        val changed = JsonObject(current.body + ("base_price" to JsonPrimitive(price)))
        val answer = server.main("PATCH", "catalog/products/$productId", changed.toString(), main)
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun archive(server: GoTestServer, main: GoTestServer.MainComputer, productId: String) {
        val current = server.main("GET", "catalog/products/$productId", null, main)
        val revision = current.text("revision")
        val answer =
            server.main(
                "PATCH",
                "catalog/products/$productId/state",
                """{"revision":$revision,"active":false,"archived":true}""",
                main,
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun soldOut(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
        productId: String,
        indefinite: Boolean,
    ) {
        val answer =
            server.main(
                "POST",
                "catalog/products/$productId/sold-out",
                """{"operation_id":"${GoTestServer.operationId()}","sold_out":true,""" +
                    """"indefinite":$indefinite}""",
                main,
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun saveState(
        main: GoTestServer.MainComputer,
        device: GoTestServer.Device,
        ids: Map<String, String>,
        fullMillis: Long,
        searchMicros: Long,
    ) {
        val state = buildJsonObject {
            put("main_device_id", main.deviceId)
            put("main_device_token", main.deviceToken)
            put("device_id", device.deviceId)
            put("device_token", device.deviceToken)
            put("session", device.session)
            put("full_millis", fullMillis)
            put("search_micros", searchMicros)
            for ((name, id) in ids) {
                put(name, id)
            }
        }
        stateFile.writeText(state.toString())
    }

    private fun loadState(): JsonObject {
        assumeTrue("Falta el estado de la etapa a", stateFile.isFile)
        return Json.parseToJsonElement(stateFile.readText()).jsonObject
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    /** La principal de la etapa a, con una sesión nueva. */
    private fun mainComputer(server: GoTestServer, saved: JsonObject): GoTestServer.MainComputer {
        val deviceId = saved.text("main_device_id")
        val deviceToken = saved.text("main_device_token")
        return GoTestServer.MainComputer(
            deviceId,
            deviceToken,
            server.adminSession(deviceId, deviceToken),
        )
    }

    /**
     * «Quitar datos de prueba» desde la principal: la vista previa debe tener solo el producto de
     * prueba [productId] y se aplica ese mismo plan (Go borra el producto y anota la baja).
     */
    private fun removeTestProduct(
        server: GoTestServer,
        main: GoTestServer.MainComputer,
        productId: String,
    ) {
        val preview = server.main("GET", "maintenance/test-data", null, main)
        assertEquals(preview.body.toString(), 200, preview.status)
        val remove = preview.body.getValue("remove").jsonArray.map { it.jsonObject }
        assertEquals(listOf(productId), remove.map { it.text("id") })
        val removed =
            server.main(
                "POST",
                "maintenance/test-data/remove",
                """{"operation_id":"${GoTestServer.operationId()}",""" +
                    """"plan_id":"${preview.text("plan_id")}"}""",
                main,
            )
        assertEquals(removed.body.toString(), 200, removed.status)
    }

    private fun device(saved: JsonObject): GoTestServer.Device =
        GoTestServer.Device(
            saved.text("device_id"),
            saved.text("device_token"),
            saved.text("session"),
        )

    private companion object {
        const val TAG = "CatalogOnGoTest"
        const val DATABASE = "catalog-go-test.db"
        const val FILLER_PRODUCTS = 300
        const val SEARCH_ROUNDS = 20
    }
}
