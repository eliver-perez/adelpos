package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import android.graphics.Bitmap
import android.os.SystemClock
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.testing.MainComputer
import com.adelpos.core.testing.SystemDialogs
import com.adelpos.core.ui.access.findserver.TAG_ADDRESS
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.core.ui.waiter.TAG_ACCOUNT_MINE
import com.adelpos.meseros.core.ui.waiter.TAG_ORDER_STATE_PREFIX
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_CARD_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_TITLE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREA_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_ORDER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import java.io.File
import java.io.FileOutputStream
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa M1 (Fase C) con la aplicación completa contra el **Go temporal** del arnés, nunca el
 * servicio real. Se corre con `scripts/android-e2e.sh --suite unions` en las tabletas:
 * - `u1` restaurante (el área «Salón M1» con las mesas 5 a 10 y «Terraza M1»; hamburguesa con ruta
 *   a cocina y refresco sin ruta), emparejar y entrar.
 * - `u2` §5.5 y §5.6: una sola tarjeta por cuenta con «Tu cuenta» y «Recibido por el restaurante»,
 *   sin «Lo tuyo en esta mesa»; y **cuánto tarda** el panel en dejar de decir «$0.00 · Borrador»
 *   después de «Enviar a cocina» (tres medidas, en el registro `AdelOnGo` con «M1 · retraso»).
 * - `u3` (SIN red) un pedido sin conexión en la Mesa 10 es su propia tarjeta «Por enviar».
 * - `u4` (con red) el pedido sale y su tarjeta pasa a ser la de la cuenta, con «Tu cuenta».
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice` y `mainToken` (los arma el guion).
 */
@RunWith(AndroidJUnit4::class)
class UnionsOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "unions-on-go.json")

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
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        automation.setRotation(UiAutomation.ROTATION_UNFREEZE)
        scenario?.close()
    }

    // --- Pasos ---

    @Test
    fun u1_prepararEmparejarYEntrar() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantWaiterPermissions(WAITER_PERMISSIONS)
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createAreasAndTables())
        ids.putAll(createCatalog())
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        patchDevice()
        enterAna()
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("burger")) != null
        }
    }

    @Test
    fun u2_unaTarjetaPorCuentaYElRetrasoDespuesDeEnviar() {
        val ids = loadIds()
        launch()
        enterAna()
        portrait()
        val delays = mutableListOf<Long>()
        val stale = mutableListOf<Long>()
        for ((index, table) in listOf("table5", "table7", "tableT1").withIndex()) {
            if (table == "tableT1") {
                tap("$TAG_TABLES_AREA_PREFIX${ids.getValue("areaTerrace")}")
            }
            newAccountOn(ids.getValue(table))
            addProduct(ids.getValue("burger"))
            showOrder()
            waitFor("${TAG_SYNC_PREFIX}saved")
            val accountId = waitForAccountOf(ids.getValue(table))
            // Como el mesero de verdad: entre el primer producto y «Enviar a cocina» pasa un rato,
            // y
            // Mesas ya leyó la cuenta con solo el borrador («$0.00 · Borrador»).
            waitUntil("Mesas ya vio la cuenta con el borrador") {
                container.waiterMode.restaurant.openAccounts.value.any {
                    it.id == accountId && it.totals.total.toString() == "0.00"
                }
            }
            Thread.sleep(SETTLE_MILLIS)
            val measure = measureSend(accountId, "$120.00")
            delays += measure.first
            stale += measure.second
            // §5.5: una sola tarjeta para la cuenta, con «Tu cuenta» y el envío recibido.
            val card = "$TAG_ACCOUNT_CARD_PREFIX$accountId"
            waitFor(card)
            waitUntil("«Recibido por el restaurante» en la tarjeta") {
                inside(card, "$TAG_ORDER_STATE_PREFIX${OrderState.RECEIVED.storedName}")
            }
            assertTrue("«Tu cuenta» en la tarjeta de Ana", inside(card, TAG_ACCOUNT_MINE))
            assertFalse("sin «Lo tuyo en esta mesa»", hasText("Lo tuyo en esta mesa"))
            val received = waitForOrder(ids.getValue(table)) { it.state == OrderState.RECEIVED }
            assertFalse(
                "el pedido recibido no tiene su propia tarjeta",
                present("$TAG_TABLE_ORDER_PREFIX${received.operationId}"),
            )
            if (index == 0) {
                screenshot("u2-tarjeta-unica-vertical")
                landscape()
                waitFor(card)
                screenshot("u2-tarjeta-unica-horizontal")
                portrait()
            }
            saveIds(loadIds() + ("account-$table" to accountId))
        }
        log(
            "M1 · retraso · resumen: hasta el total de Go ${delays} ms; con «\$0.00» a la vista $stale ms",
        )
    }

    @Test
    fun u3_sinConexionElPedidoEsSuPropiaTarjeta() {
        val ids = loadIds()
        launch()
        enterAna(offline = true)
        portrait()
        newAccountOn(ids.getValue("table10"))
        addProduct(ids.getValue("burger"))
        showOrder()
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table10")) { it.state == OrderState.QUEUED }
        openTable(ids.getValue("table10"))
        val tag = "$TAG_TABLE_ORDER_PREFIX${order.operationId}"
        waitFor(tag)
        assertTrue(
            "la tarjeta dice «Por enviar»",
            inside(tag, "$TAG_ORDER_STATE_PREFIX${OrderState.QUEUED.storedName}"),
        )
        screenshot("u3-por-enviar-vertical")
        saveIds(ids + ("order10" to order.operationId))
    }

    @Test
    fun u4_alVolverLaRedLaTarjetaEsLaDeLaCuenta() {
        val ids = loadIds()
        launch()
        enterAna()
        portrait()
        val received = waitForOrder(ids.getValue("table10")) { it.state == OrderState.RECEIVED }
        openTable(ids.getValue("table10"))
        val card = "$TAG_ACCOUNT_CARD_PREFIX${received.receivedAccountId}"
        waitFor(card)
        waitUntil("«Recibido por el restaurante» en la tarjeta de la cuenta") {
            inside(card, "$TAG_ORDER_STATE_PREFIX${OrderState.RECEIVED.storedName}")
        }
        assertTrue(inside(card, TAG_ACCOUNT_MINE))
        assertFalse(present("$TAG_TABLE_ORDER_PREFIX${ids.getValue("order10")}"))
    }

    // --- §5.6: el retraso después de «Enviar a cocina» ---

    /**
     * Toca «Enviar a cocina» y mira la tarjeta de la cuenta cada cuadro (unos 16 ms) hasta que dice
     * el total de Go ([total]). Devuelve cuánto tardó y cuánto tiempo, en ese lapso, la tarjeta
     * estuvo a la vista con «$0.00» (lo que vio el propietario como «$0.00 · Borrador»). Cada
     * cambio queda en el registro con su marca de tiempo.
     */
    private fun measureSend(accountId: String, total: String): Pair<Long, Long> {
        val title = "$TAG_ACCOUNT_TITLE_PREFIX$accountId"
        val start = SystemClock.elapsedRealtime()
        compose.onNodeWithTag(TAG_SEND_TO_KITCHEN).performClick()
        var last: String? = null
        var staleSince: Long? = null
        var staleTotal = 0L
        var receivedAt: Long? = null
        val deadline = start + MEASURE_TIMEOUT_MILLIS
        while (true) {
            val now = SystemClock.elapsedRealtime()
            check(now < deadline) { "La tarjeta no llegó al total de Go: $last" }
            val text = textOf(title)
            if (text != last) {
                log("M1 · retraso · +${now - start} ms: ${text ?: "(sin tarjeta)"}")
                last = text
            }
            if (receivedAt == null && orderReceivedIn(accountId)) {
                receivedAt = now
                log("M1 · retraso · +${now - start} ms: la respuesta de Go llegó (Recibido)")
            }
            val showsZero = text != null && text.contains("$0.00")
            if (showsZero && staleSince == null) {
                staleSince = now
            }
            if (!showsZero && staleSince != null) {
                staleTotal += now - staleSince
                staleSince = null
            }
            if (text != null && text.contains(total)) {
                val delay = now - start
                log(
                    "M1 · retraso · total de Go a los $delay ms; «\$0.00» a la vista $staleTotal ms",
                )
                return delay to staleTotal
            }
            Thread.sleep(FRAME_MILLIS)
        }
    }

    private fun orderReceivedIn(accountId: String): Boolean = runBlocking {
        val ana = loadIds().getValue("ana")
        container.database.waiter().ordersOf(ana).map(WaiterStorage::toOrder).any {
            it.state == OrderState.RECEIVED && it.receivedAccountId == accountId
        }
    }

    private fun textOf(tag: String): String? =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .firstOrNull()
            ?.config
            ?.getOrElseNullable(SemanticsProperties.Text) { null }
            ?.joinToString("") { it.text }

    // --- Pantallas ---

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

    private fun enterAna(offline: Boolean = false) {
        val ana = loadIds().getValue("ana")
        waitFor("$TAG_WAITER_PREFIX$ana", timeout = if (offline) 90_000 else 40_000)
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        for (digit in ANA_PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitFor(TAG_TABLES_SCREEN, timeout = if (offline) 90_000 else 40_000)
    }

    private fun newAccountOn(tableId: String) {
        openTable(tableId)
        tap(TAG_NEW_ACCOUNT)
        waitFor(productTag(loadIds().getValue("burger")))
    }

    private fun openTable(tableId: String) {
        waitForTables()
        if (present(TAG_ACCOUNT_PANE) && present(TAG_PANE_BACK)) {
            pressBack()
        }
        tap("$TAG_TABLE_PREFIX$tableId")
        waitFor(TAG_ACCOUNT_PANE)
    }

    private fun addProduct(productId: String) {
        repeat(3) {
            if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
                pressBack()
            }
        }
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
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

    private fun waitForTables() {
        waitUntil("Mesas") { present(TAG_TABLES_SCREEN) || present(TAG_ACCOUNT_PANE) }
    }

    private fun waitForAccountOf(tableId: String): String {
        val ana = loadIds().getValue("ana")
        var account: String? = null
        waitUntil("la cuenta de la captura") {
            account = runBlocking {
                container.database
                    .waiter()
                    .capturesOf(ana)
                    .map(WaiterStorage::toCapture)
                    .lastOrNull { it.tableId == tableId }
                    ?.accountId
            }
            account != null
        }
        return account!!
    }

    private fun waitForOrder(
        tableId: String,
        timeout: Long = 90_000,
        condition: (PendingOrder) -> Boolean,
    ): PendingOrder {
        val ana = loadIds().getValue("ana")
        var found: PendingOrder? = null
        waitUntil("el pedido de la mesa", timeout) {
            found = runBlocking {
                container.database.waiter().ordersOf(ana).map(WaiterStorage::toOrder).lastOrNull {
                    it.destination.tableId == tableId && condition(it)
                }
            }
            found != null
        }
        return found!!
    }

    // --- Ayudantes ---

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    private fun pressBack() {
        SystemDialogs.dismissNotResponding(automation)
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        Thread.sleep(SETTLE_MILLIS)
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    /** Hay un nodo con [inner] dentro del nodo con [outer]. */
    private fun inside(outer: String, inner: String): Boolean =
        compose
            .onAllNodes(hasTestTag(inner) and hasAnyAncestor(hasTestTag(outer)), true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun hasText(text: String): Boolean =
        compose
            .onAllNodes(hasText(text, substring = true), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun reveal(tag: String): Boolean {
        for (list in LAZY_LISTS) {
            if (present(list)) {
                runCatching {
                    compose
                        .onAllNodes(hasTestTag(list), useUnmergedTree = true)
                        .onFirst()
                        .performScrollToNode(hasTestTag(tag))
                }
                if (present(tag)) {
                    return true
                }
            }
        }
        return false
    }

    private fun tap(tag: String) {
        waitFor(tag)
        reveal(tag)
        val node = compose.onAllNodes(hasTestTag(tag)).onFirst()
        runCatching { node.performScrollTo() }
        node.performClick()
        compose.waitForIdle()
    }

    private fun waitFor(tag: String, timeout: Long = 40_000) {
        var nextReveal = 0L
        try {
            compose.waitUntil(timeoutMillis = timeout) {
                present(tag) ||
                    (System.currentTimeMillis() >= nextReveal &&
                        run {
                            nextReveal = System.currentTimeMillis() + REVEAL_EVERY_MILLIS
                            reveal(tag)
                        })
            }
        } catch (failure: Throwable) {
            compose.onRoot(useUnmergedTree = true).printToLog(TAG)
            throw failure
        }
    }

    private fun waitUntil(what: String, timeout: Long = 40_000, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (!condition()) {
            check(System.currentTimeMillis() < deadline) { "No llegó: $what" }
            Thread.sleep(POLL_MILLIS)
        }
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private fun portrait() = rotate(landscape = false)

    private fun landscape() = rotate(landscape = true)

    private fun rotate(landscape: Boolean) {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        Thread.sleep(SETTLE_MILLIS)
        val naturalLandscape = orientationNow() == Configuration.ORIENTATION_LANDSCAPE
        if (landscape != naturalLandscape) {
            automation.setRotation(UiAutomation.ROTATION_FREEZE_90)
        }
        waitUntil("la orientación pedida") {
            (orientationNow() == Configuration.ORIENTATION_LANDSCAPE) == landscape
        }
        Thread.sleep(SETTLE_MILLIS)
        compose.waitForIdle()
    }

    private fun orientationNow(): Int = TestDevice.app.resources.configuration.orientation

    /** Captura de pantalla en la carpeta de la aplicación (el guion la copia a la Mac). */
    private fun screenshot(name: String) {
        SystemDialogs.dismissNotResponding(automation)
        compose.waitForIdle()
        Thread.sleep(SETTLE_MILLIS)
        val bitmap = automation.takeScreenshot() ?: return
        val folder = File(TestDevice.app.getExternalFilesDir(null), "screens").apply { mkdirs() }
        FileOutputStream(File(folder, "$name.png")).use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
        log("captura $name")
    }

    // --- La computadora principal ---

    private fun grantWaiterPermissions(permissions: List<String>) {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.getValue("name").jsonPrimitive.content == ROLE }
        val list = permissions.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.text("id")}",
                """{"name":"$ROLE","permissions":[$list]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun createAreasAndTables(): Map<String, String> {
        fun area(name: String): String {
            val answer =
                main.call(
                    "POST",
                    "restaurant/areas",
                    """{"operation_id":"${UUID.randomUUID()}","name":"$name","active":true}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        fun table(areaId: String, name: String, x: Int, y: Int): String {
            val answer =
                main.call(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$areaId",""" +
                        """"name":"$name","shape":"rectangle","x":$x,"y":$y,"width":100,""" +
                        """"height":72,"seats":4,"active":true}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        val salon = area("Salón M1")
        val terrace = area("Terraza M1")
        val ids =
            mutableMapOf(
                "areaSalon" to salon,
                "areaTerrace" to terrace,
                "tableT1" to table(terrace, "Terraza 1", 60, 60),
            )
        // Mesas 5 a 10 en un renglón (la 9 y la 10, en el segundo): las uniones las juntan.
        for (number in 5..10) {
            val index = number - 5
            ids["table$number"] =
                table(salon, "Mesa $number", 40 + (index % 4) * 130, 40 + (index / 4) * 110)
        }
        return ids
    }

    /** Hamburguesa con ruta a Cocina y refresco sin ruta («No necesitan preparación», §5.2). */
    private fun createCatalog(): Map<String, String> {
        fun category(name: String): String {
            val answer =
                main.call(
                    "POST",
                    "catalog/categories",
                    """{"name":"$name","active":true,"color":"#3591d4"}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        fun product(categoryId: String, name: String, code: String, price: String): String {
            val answer =
                main.call(
                    "POST",
                    "catalog/products",
                    """{"name":"$name","active":true,"category_id":"$categoryId",""" +
                        """"code":"$code","kind":"product","base_price":"$price"}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        val burgers = category("Hamburguesas M1")
        val drinks = category("Refrescos M1")
        val station =
            main.call(
                "POST",
                "kitchen/stations",
                """{"operation_id":"${UUID.randomUUID()}","name":"Cocina M1","active":true,""" +
                    """"reception":"kds","printer_id":null,"grouping":"inherit",""" +
                    """"tracking":"inherit","description":""}""",
            )
        assertEquals(station.body.toString(), 200, station.status)
        val route =
            main.call(
                "POST",
                "kitchen/routes",
                """{"operation_id":"${UUID.randomUUID()}","category_id":"$burgers",""" +
                    """"product_id":null,"mode":"prepare","station_ids":["${station.obj.text("id")}"]}""",
            )
        assertEquals(route.body.toString(), 200, route.status)
        return mapOf(
            "burger" to product(burgers, "Hamburguesa M1", "M1-HAM", "120"),
            "soda" to product(drinks, "Coca Cola M1", "M1-COC", "35"),
        )
    }

    /** Una hora de bloqueo por inactividad y Mesas como pantalla inicial. */
    private fun patchDevice() {
        val device =
            main
                .call("GET", "devices")
                .body
                .jsonArray
                .map { it.jsonObject }
                .last { it.text("kind") == "android" && it["revoked_at"] is JsonNull }
        val answer =
            main.call(
                "PATCH",
                "devices/${device.text("id")}",
                """{"name":"${device.text("name")}","keyboard_mode":"auto",""" +
                    """"home_screen":"tables","idle_lock_seconds":3600}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    // --- Estado entre pasos ---

    private fun saveIds(ids: Map<String, String>) {
        stateFile.writeText(JsonObject(ids.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun loadIds(): Map<String, String> =
        Json.parseToJsonElement(stateFile.readText()).jsonObject.mapValues {
            it.value.jsonPrimitive.content
        }

    private companion object {
        const val TAG = "AdelOnGo"
        const val ANA_USER = "ana.m1"
        const val ANA_PIN = "4829"
        const val ROLE = "Meseros A1"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L
        const val FRAME_MILLIS = 16L
        const val MEASURE_TIMEOUT_MILLIS = 20_000L

        /** Las listas perezosas en las que se busca desplazando. */
        val LAZY_LISTS =
            listOf(
                TAG_ACCOUNT_PANE,
                TAG_TABLES_LIST,
                TAG_PENDING,
                TAG_ORDER_PANEL,
                TAG_PRODUCT_GRID,
            )

        /** Lo que usa un mesero en Mesas (sin «Unir mesas»: la Fase A todavía no lo tiene). */
        val WAITER_PERMISSIONS =
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
                "payments.tickets.print",
            )
    }
}
