package com.adelpos.meseros

import android.graphics.Bitmap
import android.util.Log
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.testing.MainComputer
import com.adelpos.core.testing.SystemDialogs
import com.adelpos.core.ui.access.findserver.TAG_ADDRESS
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.core.capture.TAG_OPEN_ANOTHER
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_CARD_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_UNION_PREFIX
import java.io.File
import java.io.FileOutputStream
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa M1 (Fase C, M1-D9): **la 0.4.1 frente a una unión**. Se corre con `scripts/android-e2e.sh
 * --suite legacy` en una tableta:
 * - `v1` con la **0.4.1** instalada (su APK de pruebas lleva este paso; se compila de una copia de
 *   `main` con esta clase agregada solo para la prueba): la principal abre una cuenta en la Mesa 21
 *   y la une con la 22 por la API; la tableta abre la Mesa 22, captura y manda un pedido. Se anota
 *   en el registro (`AdelOnGo`, «M1 · 0.4.1») lo que de verdad pasa: la 0.4.1 no conoce la unión,
 *   Go le responde `TABLE_HAS_ACCOUNT` con la cuenta de la unión, el mesero abre otra y nada se
 *   pierde.
 * - `v2` con la **0.5.0** instalada encima (sin volver a emparejar): la unión es una sola entrada
 *   «Mesa 21 + 22» con las dos cuentas.
 */
@RunWith(AndroidJUnit4::class)
class LegacyUnionOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "legacy-union-on-go.json")

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    @Before
    fun mainComputer() {
        main = MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
    }

    @After
    fun close() {
        scenario?.close()
    }

    @Test
    fun v1_laVersionAnteriorFrenteAUnaUnion() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantPermissions()
        val ids = mutableMapOf("ana" to ana)
        val area = post("restaurant/areas", """{"name":"Salón L","active":true}""").text("id")
        fun table(name: String, x: Int) =
            post(
                    "restaurant/tables",
                    """{"area_id":"$area","name":"$name","shape":"rectangle","x":$x,"y":40,""" +
                        """"width":100,"height":72,"seats":4,"active":true}""",
                )
                .text("id")
        ids["table21"] = table("Mesa 21", 40)
        ids["table22"] = table("Mesa 22", 170)
        val category =
            main.call("POST", "catalog/categories", """{"name":"Platos L","active":true}""")
        val burger =
            main
                .call(
                    "POST",
                    "catalog/products",
                    """{"name":"Hamburguesa L","active":true,"category_id":"${category.obj.text("id")}",""" +
                        """"code":"L-HAM","kind":"product","base_price":"120"}""",
                )
                .obj
                .text("id")
        ids["burger"] = burger
        // La principal abre una cuenta en la Mesa 21 y la une con la 22 (otra terminal).
        val opened =
            post(
                "restaurant/accounts",
                """{"mode":"dine_in","table_id":"${ids.getValue("table21")}","seen_account_ids":[],""" +
                    """"items":[{"id":"${UUID.randomUUID()}","product_id":"$burger","variant_id":null,""" +
                    """"quantity":1,"customization":{}}]}""",
            )
        ids["account21"] = opened.getValue("account").jsonObject.text("id")
        val union =
            post(
                "restaurant/table-unions",
                """{"start_table_id":"${ids.getValue("table21")}","table_ids":["${ids.getValue("table22")}"]}""",
            )
        ids["union"] = union.getValue("union").jsonObject.text("id")
        log(
            "M1 · 0.4.1 · unión ${union.getValue("union").jsonObject.text("name")} creada por la API",
        )
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        enterAna()
        waitUntil("el catálogo") {
            container.catalogRealtime.catalog.catalog.value.product(burger) != null
        }
        // La 0.4.1 ve las dos mesas por separado (Ocupada): abre la 22 y captura.
        tap("$TAG_TABLE_PREFIX${ids.getValue("table22")}")
        waitFor(TAG_ACCOUNT_PANE)
        screenshot("legacy-041-mesa-22")
        tap(TAG_NEW_ACCOUNT)
        tap(productTag(burger))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        showOrder()
        // Lo que de verdad pasa: el guardado de la cuenta nueva.
        var sync: String? = null
        waitUntil("la respuesta de Go al guardar", 60_000) {
            sync = currentSync(ana, ids.getValue("table22"))
            sync != null && sync != "PENDING" && sync != "SAVING"
        }
        log("M1 · 0.4.1 · la captura de la Mesa 22 quedó: $sync")
        screenshot("legacy-041-captura")
        if (sync == "TABLE_HAS_ACCOUNT") {
            // Go le dice que la mesa ya tiene cuenta (la de la unión): el mesero abre otra.
            tap(TAG_OPEN_ANOTHER)
            waitFor("${TAG_SYNC_PREFIX}saved", 40_000)
        }
        tap(TAG_SEND_TO_KITCHEN)
        var received: String? = null
        waitUntil("el pedido recibido", 90_000) {
            received = runBlocking {
                container.database
                    .waiter()
                    .ordersOf(ana)
                    .map(WaiterStorage::toOrder)
                    .lastOrNull {
                        it.destination.tableId == ids.getValue("table22") &&
                            it.state == OrderState.RECEIVED
                    }
                    ?.receivedAccountId
            }
            received != null
        }
        val account =
            main.call("GET", "restaurant/accounts/$received").obj.getValue("account").jsonObject
        log(
            "M1 · 0.4.1 · el pedido entró en la cuenta ${account.text("folio")} de la mesa " +
                "${account.text("table_name")}, unión: ${account["table_union"]}",
        )
        saveIds(ids + ("account22" to received!!))
    }

    @Test
    fun v2_alActualizarALa050LaUnionEsUnaSolaEntrada() {
        val ids = loadIds()
        launch()
        enterAna()
        val unionId = ids.getValue("union")
        waitFor("$TAG_UNION_PREFIX$unionId")
        assertTrue(
            "la Mesa 22 no sale por separado",
            compose
                .onAllNodes(hasTestTag("$TAG_TABLE_PREFIX${ids.getValue("table22")}"))
                .fetchSemanticsNodes()
                .isEmpty(),
        )
        tap("$TAG_UNION_PREFIX$unionId")
        waitFor("$TAG_ACCOUNT_CARD_PREFIX${ids.getValue("account21")}")
        waitFor("$TAG_ACCOUNT_CARD_PREFIX${ids.getValue("account22")}")
        screenshot("legacy-050-union")
        log("M1 · 0.5.0 · la unión con sus dos cuentas, sin volver a emparejar")
    }

    // --- Ayudantes ---

    private fun currentSync(ana: String, tableId: String): String? = runBlocking {
        container.database
            .waiter()
            .capturesOf(ana)
            .map(WaiterStorage::toCapture)
            .lastOrNull { it.tableId == tableId }
            ?.sync
            ?.name
    }

    private fun post(path: String, body: String): JsonObject {
        val withId = """{"operation_id":"${UUID.randomUUID()}",""" + body.removePrefix("{")
        val answer = main.call("POST", path, withId)
        assertEquals(answer.body.toString(), 200, answer.status)
        return answer.obj
    }

    private fun grantPermissions() {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.getValue("name").jsonPrimitive.content == ROLE }
        val list = PERMISSIONS.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.text("id")}",
                """{"name":"$ROLE","permissions":[$list]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun pairByAddress() {
        waitFor(TAG_FIND_SERVER)
        compose.onNodeWithTag(TAG_ADDRESS).performTextReplacement(argument("address"))
        compose.onNodeWithTag(TAG_ADDRESS).performImeAction()
        waitFor(TAG_PAIRING_CODE)
        val shown =
            compose
                .onNodeWithTag(TAG_PAIRING_CODE)
                .fetchSemanticsNode()
                .config[SemanticsProperties.Text]
                .joinToString("") { it.text }
        val approved = main.approvePairing(shown.replace(" ", ""))
        assertEquals(approved.body.toString(), 200, approved.status)
        waitFor(TAG_ENTRY)
    }

    private fun enterAna() {
        val ana = loadIds().getValue("ana")
        waitFor("$TAG_WAITER_PREFIX$ana")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        for (digit in ANA_PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitFor(TAG_TABLES_SCREEN)
    }

    private fun showOrder() {
        waitUntil("la cuenta o «Ver la cuenta»") {
            present(TAG_ORDER_PANEL) || present(TAG_PANE_OPEN_SECONDARY)
        }
        if (!present(TAG_ORDER_PANEL)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun tap(tag: String) {
        waitFor(tag)
        for (list in listOf(TAG_ACCOUNT_PANE, TAG_TABLES_LIST, TAG_ORDER_PANEL, TAG_PRODUCT_GRID)) {
            if (present(list)) {
                runCatching {
                    compose
                        .onAllNodes(hasTestTag(list), true)
                        .onFirst()
                        .performScrollToNode(hasTestTag(tag))
                }
            }
        }
        val node = compose.onAllNodes(hasTestTag(tag)).onFirst()
        runCatching { node.performScrollTo() }
        node.performClick()
        compose.waitForIdle()
    }

    private fun waitFor(tag: String, timeout: Long = 40_000) {
        compose.waitUntil(timeoutMillis = timeout) { present(tag) }
    }

    private fun waitUntil(what: String, timeout: Long = 40_000, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (!condition()) {
            check(System.currentTimeMillis() < deadline) { "No llegó: $what" }
            Thread.sleep(250)
        }
    }

    private fun screenshot(name: String) {
        SystemDialogs.dismissNotResponding(automation)
        compose.waitForIdle()
        Thread.sleep(1_500)
        val bitmap = automation.takeScreenshot() ?: return
        val folder = File(TestDevice.app.getExternalFilesDir(null), "screens").apply { mkdirs() }
        FileOutputStream(File(folder, "$name.png")).use {
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
        }
        log("captura $name")
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    private fun saveIds(ids: Map<String, String>) {
        stateFile.writeText(JsonObject(ids.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun loadIds(): Map<String, String> =
        Json.parseToJsonElement(stateFile.readText()).jsonObject.mapValues {
            it.value.jsonPrimitive.content
        }

    private companion object {
        const val TAG = "AdelOnGo"
        const val ANA_USER = "ana.legacy"
        const val ANA_PIN = "4829"
        const val ROLE = "Meseros A1"
        val PERMISSIONS =
            listOf(
                "waiter_devices.use",
                "restaurant.tables.read",
                "restaurant.areas.read",
                "accounts.read",
                "accounts.open",
                "orders.capture",
                "orders.confirm",
                "catalog.read",
                "catalog.prices.read",
                "events.read",
            )
    }
}
