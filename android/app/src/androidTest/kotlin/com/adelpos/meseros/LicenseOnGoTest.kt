package com.adelpos.meseros

import android.app.UiAutomation
import android.graphics.Bitmap
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
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
import com.adelpos.meseros.core.capture.TAG_CAPTURE
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_ORDER_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_REVIEW_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_ACCEPT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_MESSAGE
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_RESEND
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_BANNER
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_DETAIL
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_TEXT
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_BADGE
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_DETAIL
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_TEXT
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import java.io.File
import java.io.FileOutputStream
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.int
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
 * Etapa L1, §8: los mensajes de la licencia, del cupo y de la hora con la aplicación completa
 * contra el **Go temporal** del arnés, nunca el servicio real ni un servidor de licencias. El guion
 * (`scripts/android-e2e.sh --suite license`) importa entre pasos cada licencia de prueba (firmada
 * con las llaves de `vectors.json` por `backend/cmd/license-test`) y mueve el reloj del servidor
 * con su archivo de desfase (`ADEL_TEST_CLOCK_FILE`), nunca la hora de la Mac ni la del emulador:
 * - `l1` (licencia de 0 comanderos) restaurante, emparejar y entrar; al capturar, la captura dice
 *   «La licencia no incluye comanderos (0 de 0)…» y «Enviar a cocina» deja el pedido en «Requiere
 *   tu revisión» con ese motivo y «Enviar de nuevo».
 * - `l2` (SIN red) un pedido «Por enviar» en otra mesa.
 * - `l3` (con red) la cola lo envía sola, Go lo rechaza por cupo y queda en revisión con su motivo;
 *   no se reintenta solo (los intentos no cambian) y Go no tiene ninguna cuenta en la mesa.
 * - `l4` (licencia con cupos) «Enviar de nuevo» de los dos: entran **una vez**, con su misma clave.
 * - `l5` (reloj del servidor 2 h atrás) el aviso fijo de la hora en Mesas, la captura y Pendientes,
 *   con las dos fechas; el pedido nuevo se rechaza con el texto de la hora.
 * - `l6` (reloj corregido) el aviso se quita solo y el pedido se envía de nuevo.
 * - `l7` (licencia en tolerancia) el aviso discreto «Licencia: quedan 3 días» en la barra; al
 *   tocarlo, el texto completo.
 * - `l8` (licencia vencida) capturar dice «La licencia del sistema no permite capturar pedidos.
 *   Avisa al encargado.».
 *
 * Cada paso deja capturas de pantalla en tema claro (el guion las copia a `<salida>/screens`).
 * Argumentos: `local`, `address`, `pin`, `mainDevice` y `mainToken` (los arma el guion).
 */
@RunWith(AndroidJUnit4::class)
class LicenseOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "license-on-go.json")

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

    // --- Pasos ---

    @Test
    fun l1_sinComanderosElMensajeYLaRevision() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantWaiterPermissions()
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createTables())
        ids.putAll(createCatalog())
        saveIds(ids)
        TestDevice.reset()
        launch()
        // Emparejar es administración: con la licencia activa se puede (sin comanderos, también).
        pairByAddress()
        enterAna()
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("water")) != null
        }
        // En línea, el primer producto abre la cuenta en Go y Go lo rechaza por el cupo: la captura
        // lo dice con el texto de su código (§5 y §8), con cuántos en uso y el límite.
        newAccountOn(ids.getValue("table1"))
        addProduct(ids.getValue("water"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}rejected")
        waitForText(NO_ORDER_TAKERS)
        screenshot("l1-captura-sin-cupo-de-comandero")
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table1")) { it.state == OrderState.NEEDS_REVIEW }
        assertEquals(ReviewReason.NOT_ALLOWED, order.review!!.reason)
        assertEquals(NO_ORDER_TAKERS, order.review!!.message)
        openPending()
        openReview(order.operationId)
        assertEquals(NO_ORDER_TAKERS, textOf(TAG_REVIEW_MESSAGE))
        waitFor(TAG_REVIEW_RESEND)
        assertFalse("no hay cambios que aceptar", present(TAG_REVIEW_ACCEPT))
        screenshot("l1-revision-sin-cupo-con-enviar-de-nuevo")
        assertTrue("Go no abrió ninguna cuenta", openAccountsOn(ids.getValue("table1")).isEmpty())
    }

    @Test
    fun l2_pedidoSinConexionPorEnviar() {
        val ids = loadIds()
        launch()
        // Sin red entra con el verificador local; el pedido se queda «Por enviar».
        enterAna()
        newAccountOn(ids.getValue("table2"))
        addProduct(ids.getValue("coffee"))
        showOrder()
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table2")) { it.state == OrderState.QUEUED }
        log("l2 pedido ${order.operationId} por enviar")
    }

    @Test
    fun l3_alVolverLaRedQuedaEnRevisionSinReintentos() {
        val ids = loadIds()
        // Proceso nuevo y sin entrar: WorkManager lo envía con la credencial de envío de Ana.
        launch()
        val order =
            waitForOrder(ids.getValue("table2"), timeout = 120_000) {
                it.state == OrderState.NEEDS_REVIEW
            }
        assertEquals(ReviewReason.NOT_ALLOWED, order.review!!.reason)
        assertEquals(NO_ORDER_TAKERS, order.review!!.message)
        // No se reintenta solo: ni al volver a primer plano ni con el envío pedido a la cola.
        val attempts = order.attempts
        container.orderQueue.retryNow()
        Thread.sleep(NO_RETRY_WAIT_MILLIS)
        val later = currentOrder(order.operationId)
        assertEquals(OrderState.NEEDS_REVIEW, later.state)
        assertEquals("sin reintentos solos", attempts, later.attempts)
        assertTrue("Go no abrió ninguna cuenta", openAccountsOn(ids.getValue("table2")).isEmpty())
        enterAna()
        openPending()
        waitFor("$TAG_PENDING_REVIEW_PREFIX${order.operationId}")
        screenshot("l3-pendientes-con-dos-pedidos-en-revision")
        openReview(order.operationId)
        assertEquals(NO_ORDER_TAKERS, textOf(TAG_REVIEW_MESSAGE))
        screenshot("l3-revision-del-pedido-sin-conexion")
    }

    @Test
    fun l4_volvioElCupoYSeEnviaDeNuevoConLaMismaClave() {
        val ids = loadIds()
        launch()
        enterAna()
        for (table in listOf("table1", "table2")) {
            val tableId = ids.getValue(table)
            val order = waitForOrder(tableId) { it.state == OrderState.NEEDS_REVIEW }
            openPending()
            openReview(order.operationId)
            tap(TAG_REVIEW_RESEND)
            val received =
                waitForOrder(tableId, timeout = 90_000) { it.state == OrderState.RECEIVED }
            assertEquals("la misma clave", order.operationId, received.operationId)
            val accounts = openAccountsOn(tableId)
            assertEquals("una sola cuenta en la mesa", 1, accounts.size)
            val lines =
                main
                    .call("GET", "restaurant/accounts/${accounts.single()}")
                    .obj
                    .getValue("lines")
                    .jsonArray
            assertEquals("entró una sola vez", 1, lines.size)
            assertEquals(1, lines.single().jsonObject.getValue("quantity").jsonPrimitive.int)
            showTables()
        }
        openPending()
        screenshot("l4-pendientes-recibidos")
    }

    @Test
    fun l5_horaIncorrectaAvisoFijoYRechazo() {
        val ids = loadIds()
        launch()
        enterAna()
        // El vigilante de Go lo nota en segundos y avisa por el canal (license.changed); Mesas
        // vuelve a leer el estado.
        waitFor(TAG_LICENSE_CLOCK_BANNER, timeout = 120_000)
        assertEquals(LicenseMessages.CLOCK_INVALID, textOf(TAG_LICENSE_CLOCK_TEXT))
        assertTrue(
            "con las dos fechas",
            textOf(TAG_LICENSE_CLOCK_DETAIL).startsWith("La computadora principal tiene la hora"),
        )
        screenshot("l5-mesas-con-el-aviso-de-la-hora")
        newAccountOn(ids.getValue("table3"))
        addProduct(ids.getValue("water"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}rejected")
        // El aviso fijo arriba y el rechazo de la captura dicen lo mismo.
        assertTrue(present(TAG_LICENSE_CLOCK_BANNER))
        screenshot("l5-captura-con-la-hora-incorrecta")
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table3")) { it.state == OrderState.NEEDS_REVIEW }
        assertEquals(LicenseMessages.CLOCK_INVALID, order.review!!.message)
        openPending()
        openReview(order.operationId)
        assertTrue(present(TAG_LICENSE_CLOCK_BANNER))
        screenshot("l5-revision-con-la-hora-incorrecta")
    }

    @Test
    fun l6_horaCorregidaSeQuitaElAvisoYSeEnvia() {
        val ids = loadIds()
        launch()
        enterAna()
        waitUntil("el aviso de la hora se quita solo", timeout = 120_000) {
            !present(TAG_LICENSE_CLOCK_BANNER)
        }
        screenshot("l6-mesas-sin-el-aviso")
        val tableId = ids.getValue("table3")
        val order = waitForOrder(tableId) { it.state == OrderState.NEEDS_REVIEW }
        openPending()
        openReview(order.operationId)
        tap(TAG_REVIEW_RESEND)
        val received = waitForOrder(tableId, timeout = 90_000) { it.state == OrderState.RECEIVED }
        assertEquals(order.operationId, received.operationId)
        assertEquals(1, openAccountsOn(tableId).size)
    }

    @Test
    fun l7_toleranciaAvisoDiscretoEnLaBarra() {
        launch()
        enterAna()
        waitFor(TAG_LICENSE_GRACE_BADGE, timeout = 60_000)
        // La tableta tiene lugar: el aviso dice los días que calculó Go.
        if (present(TAG_LICENSE_GRACE_TEXT)) {
            assertEquals("Licencia: quedan 3 días", textOf(TAG_LICENSE_GRACE_TEXT))
        }
        screenshot("l7-mesas-con-la-licencia-en-tolerancia")
        tap(TAG_LICENSE_GRACE_BADGE)
        waitFor(TAG_LICENSE_GRACE_DETAIL)
        assertEquals(LicenseMessages.graceLong(3), textOf(TAG_LICENSE_GRACE_DETAIL))
        screenshot("l7-aviso-completo-de-la-tolerancia")
        pressBack()
    }

    @Test
    fun l8_vencidaNoPermiteCapturar() {
        val ids = loadIds()
        launch()
        enterAna()
        assertFalse("vencida ya no es tolerancia", present(TAG_LICENSE_GRACE_BADGE))
        newAccountOn(ids.getValue("table4"))
        addProduct(ids.getValue("coffee"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}rejected")
        waitForText(LicenseMessages.CAPTURE_NOT_ALLOWED)
        screenshot("l8-captura-con-la-licencia-vencida")
        assertTrue("Go no abrió ninguna cuenta", openAccountsOn(ids.getValue("table4")).isEmpty())
    }

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

    private fun enterAna() {
        val ana = loadIds().getValue("ana")
        waitFor("$TAG_WAITER_PREFIX$ana")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        for (digit in ANA_PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitForTables()
    }

    private fun newAccountOn(tableId: String) {
        showTables()
        tap("$TAG_TABLE_PREFIX$tableId")
        tap(TAG_NEW_ACCOUNT)
        waitFor(TAG_CAPTURE)
    }

    private fun addProduct(productId: String) {
        backToCatalog()
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
    }

    private fun showOrder() {
        // La captura aparece antes que su botón «Ver la cuenta»: se espera a uno de los dos.
        waitUntil("la cuenta o «Ver la cuenta»") {
            present(TAG_ORDER_PANEL) || present(TAG_PANE_OPEN_SECONDARY)
        }
        if (!present(TAG_ORDER_PANEL)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    private fun backToCatalog() {
        repeat(3) {
            if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
                pressBack()
            }
        }
    }

    /** Vuelve a Mesas desde cualquier pantalla del modo Mesero. */
    private fun showTables() {
        repeat(4) {
            if (present(TAG_TABLES_SCREEN) && !present(TAG_ACCOUNT_PANE)) {
                return
            }
            if (present(TAG_TABLES_SCREEN) && !present(TAG_PANE_BACK)) {
                return
            }
            pressBack()
        }
        waitForTables()
    }

    private fun waitForTables() {
        waitFor(TAG_TABLES_SCREEN)
    }

    private fun openPending() {
        showTables()
        tap(TAG_OPEN_PENDING)
        waitFor(TAG_PENDING)
    }

    private fun openReview(operationId: String) {
        waitFor("$TAG_PENDING_REVIEW_PREFIX$operationId")
        tap("$TAG_PENDING_ORDER_PREFIX$operationId")
        waitFor(TAG_REVIEW)
    }

    private fun textOf(tag: String): String =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .onFirst()
            .fetchSemanticsNode()
            .config[SemanticsProperties.Text]
            .joinToString("") { it.text }

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

    private fun waitForText(text: String, timeout: Long = 40_000) {
        try {
            compose.waitUntil(timeoutMillis = timeout) {
                compose
                    .onAllNodes(hasText(text, substring = true), useUnmergedTree = true)
                    .fetchSemanticsNodes()
                    .isNotEmpty()
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

    // --- Lo guardado en el dispositivo ---

    private fun waitForOrder(
        tableId: String,
        timeout: Long = 60_000,
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
        log("pedido de la mesa: ${found!!.state} intentos ${found!!.attempts}")
        return found!!
    }

    private fun currentOrder(operationId: String): PendingOrder = runBlocking {
        WaiterStorage.toOrder(container.database.waiter().order(operationId)!!)
    }

    // --- La computadora principal ---

    private fun grantWaiterPermissions() {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.getValue("name").jsonPrimitive.content == ROLE }
        val permissions = WAITER_PERMISSIONS.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.text("id")}",
                """{"name":"$ROLE","permissions":[$permissions]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun createTables(): Map<String, String> {
        val area =
            main.call(
                "POST",
                "restaurant/areas",
                """{"operation_id":"${UUID.randomUUID()}","name":"Salón L1","active":true}""",
            )
        assertEquals(area.body.toString(), 200, area.status)
        val areaId = area.obj.text("id")
        fun table(name: String, x: Int): String {
            val answer =
                main.call(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$areaId",""" +
                        """"name":"$name","shape":"rectangle","x":$x,"y":60,"width":100,""" +
                        """"height":72,"seats":4,"active":true}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        return mapOf(
            "table1" to table("Mesa L1", 60),
            "table2" to table("Mesa L2", 220),
            "table3" to table("Mesa L3", 380),
            "table4" to table("Mesa L4", 540),
        )
    }

    private fun createCatalog(): Map<String, String> {
        val category =
            main.call(
                "POST",
                "catalog/categories",
                """{"name":"Bebidas L1","active":true,"color":"#3591d4"}""",
            )
        assertEquals(category.body.toString(), 200, category.status)
        val categoryId = category.obj.text("id")
        fun product(name: String, code: String, price: String): String {
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
        return mapOf(
            "water" to product("Agua de jamaica L1", "L1-AGU", "35"),
            "coffee" to product("Café americano L1", "L1-CAF", "30"),
        )
    }

    /** Las cuentas abiertas de la mesa en Go. */
    private fun openAccountsOn(tableId: String): List<String> =
        main
            .call("GET", "restaurant/state")
            .obj
            .getValue("accounts")
            .jsonArray
            .map { it.jsonObject }
            .filter {
                it["table_id"]?.jsonPrimitive?.content == tableId && it.text("state") == "open"
            }
            .map { it.text("id") }

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
        const val ANA_USER = "ana.l1"
        const val ANA_PIN = "5173"
        const val ROLE = "Meseros A1"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L

        /** Lo que se espera sin pedirle a la cola nada más: un reintento solo se vería aquí. */
        const val NO_RETRY_WAIT_MILLIS = 30_000L

        /** §5 con límite 0 (la licencia de prueba de `l1`: 0 comanderos). */
        const val NO_ORDER_TAKERS =
            "La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal " +
                "con el turno de caja abierto."

        /** Las listas perezosas en las que se busca desplazando. */
        val LAZY_LISTS =
            listOf(
                TAG_ACCOUNT_PANE,
                TAG_TABLES_LIST,
                TAG_PENDING,
                TAG_ORDER_PANEL,
                TAG_PRODUCT_GRID,
            )

        /** Lo que usa un mesero en Mesas. */
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
            )
    }
}
