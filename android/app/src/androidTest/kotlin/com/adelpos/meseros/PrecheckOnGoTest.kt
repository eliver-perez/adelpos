package com.adelpos.meseros

import android.app.UiAutomation
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
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
import com.adelpos.meseros.core.capture.TAG_PRECHECK_MESSAGE_PREFIX
import com.adelpos.meseros.core.capture.TAG_PRECHECK_OFFLINE
import com.adelpos.meseros.core.capture.TAG_PRECHECK_PRINT
import com.adelpos.meseros.core.capture.TAG_PRECHECK_RETRY
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
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
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_ENTRY_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_NEW
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SEARCH
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import com.adelpos.meseros.navigation.TAG_NAV_TABLES
import com.adelpos.meseros.navigation.TAG_NAV_TAKEAWAY
import java.io.File
import java.util.Base64
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
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
 * Imprimir la precuenta desde el dispositivo (etapa A2, Fase C, §5 y §6) contra el **Go temporal**
 * del arnés, nunca el servicio real. La prueba hace de computadora principal por la escucha local
 * ([MainComputer]: asigna la impresora en Terminales, cambia el nombre visible en Administración ›
 * Documentos y lee los documentos de la cuenta) y del **agente de impresión** de una terminal de
 * prueba «Caja 2» con su impresora USB (`print-agent/jobs/claim` con la credencial de esa terminal:
 * recibe los bytes ESC/POS ya preparados por Go y los revisa). Un paso por instrumentación, en
 * orden, con la aplicación instalada una sola vez (`scripts/android-e2e.sh --suite precheck`); **la
 * red la corta y la devuelve el guion** (en `p6`).
 *
 * - `p1` prepara el restaurante (rol de meseros **sin** `payments.tickets.print`, catálogo, mesa,
 *   la terminal «Caja 2» con su impresora USB y una impresora de red que no responde), empareja,
 *   entra y envía una cuenta a cocina: sin el permiso no hay botón de imprimir. Al final da el
 *   permiso (la guía: «Imprimir tickets y precuentas»).
 * - `p2` sin impresora asignada: «Imprimir precuenta» → «Este dispositivo no tiene impresora
 *   asignada» y Go no encola nada (A2-D3: nunca otra impresora).
 * - `p3` la impresora USB de «Caja 2», que está desconectada: «La Caja 2 está desconectada»; al
 *   volver, su agente recibe los bytes ESC/POS con «PRECUENTA», «Atendió: <mesero>», el folio, la
 *   mesa y las partidas, y el dispositivo dice «Enviada a Caja 2».
 * - `p4` el nombre visible: «Ticket de venta» y «Cuenta» en el botón y en el impreso.
 * - `p5` un pedido para llevar: el mismo botón y el impreso con «Para llevar» y su folio.
 * - `p6` (SIN red) el botón no se puede tocar y lo dice; nada queda para después.
 * - `p7` la impresora de red no responde: «La impresora no responde» y «Reintentar» sin motivo la
 *   misma precuenta; ninguna precuenta salió por `p6`.
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice` y `mainToken` (los arma el guion). Sin ellos
 * se omite.
 */
@RunWith(AndroidJUnit4::class)
class PrecheckOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "precheck-on-go.json")
    private val plain =
        OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .build()

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
    fun p1_prepararEnviarACocinaYSinPermisoNoHayBoton() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        setWaiterPermissions(WAITER_PERMISSIONS)
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createCatalogAndTable())
        ids.putAll(createAgentTerminalAndPrinters())
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        enterAna()
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("burger")) != null
        }
        ids["device"] = androidDevice().text("id")
        // Una cuenta de la mesa con dos partidas enviadas a cocina.
        newAccountOn(ids.getValue("table"))
        addProduct(ids.getValue("burger"))
        addProduct(ids.getValue("fries"))
        addProduct(ids.getValue("fries"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        val order = waitForOrder { it.state == OrderState.RECEIVED }
        val accountId = order.receivedAccountId!!
        ids["account"] = accountId
        ids["folio"] = account(accountId).getValue("folio").jsonPrimitive.content
        saveIds(ids)
        // Sin `payments.tickets.print` en los permisos de Go: la cuenta no ofrece imprimir.
        openAccount(ids.getValue("table"), accountId)
        compose.waitForIdle()
        Thread.sleep(SETTLE_MILLIS)
        assertFalse(present(TAG_PRECHECK_PRINT))
        assertFalse(reveal(TAG_PRECHECK_PRINT))
        // El permiso de la guía para el rol Mesero (se lee al volver a entrar).
        setWaiterPermissions(WAITER_PERMISSIONS + PRINT_PERMISSION)
    }

    @Test
    fun p2_sinImpresoraAsignadaNoSeImprime() {
        val ids = loadIds()
        val before = documents(ids.getValue("account")).size
        launch()
        enterAna()
        openAccount(ids.getValue("table"), ids.getValue("account"))
        // Ya se sabe (getIdentity) que no tiene: se avisa antes de tocar, pero Go manda.
        showPrecheck()
        waitForText("Este dispositivo todavía no tiene impresora asignada.")
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir precuenta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}no-printer")
        waitForText("Este dispositivo no tiene impresora asignada.")
        waitForText(
            "Pídele al administrador que la elija en Administración › Sistema › Terminales.",
        )
        // A2-D3: no se encoló nada (aunque exista la impresora de tickets general de Cobros).
        assertEquals(before, documents(ids.getValue("account")).size)
    }

    @Test
    fun p3_terminalDesconectadaYLosBytesEscPos() {
        val ids = loadIds()
        assignPrinter(ids.getValue("device"), ids.getValue("usb"))
        launch()
        enterAna()
        openAccount(ids.getValue("table"), ids.getValue("account"))
        // A dónde va, según getIdentity (`terminal.precheck_printer`).
        showPrecheck()
        waitForText("Se imprime en $AGENT_TERMINAL.")
        // La terminal «Caja 2» nunca ha consultado: Go ya lo sabe al encolar.
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir precuenta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}terminal-offline")
        waitForText("La $AGENT_TERMINAL está desconectada.")
        assertFalse(present(TAG_PRECHECK_RETRY))
        val waiting = documents(ids.getValue("account")).last()
        assertEquals("precheck", waiting.text("kind"))
        assertEquals("pending", waiting.text("job_state"))
        // La terminal vuelve: su agente recibe la precuenta (la misma plantilla de la caja).
        claimAndCheck(
            ids,
            expected =
                listOf(
                    "PRECUENTA",
                    "Atendió:",
                    ANA_DISPLAY_NAME,
                    "Folio",
                    ids.getValue("folio"),
                    TABLE_NAME,
                    BURGER_NAME,
                    FRIES_NAME,
                ),
        )
        // El dispositivo lo sigue: la impresora (la cola de la terminal) ya la aceptó.
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent", timeout = 15_000)
        waitForText("Enviada a $AGENT_TERMINAL")
        assertEquals("submitted", documents(ids.getValue("account")).last().text("job_state"))
    }

    @Test
    fun p4_elNombreVisibleEnElBotonYEnElImpreso() {
        val ids = loadIds()
        setPrecheckName("sales_ticket")
        launch()
        enterAna()
        openAccount(ids.getValue("table"), ids.getValue("account"))
        // La terminal está conectada (su agente acaba de consultar).
        agentPoll()
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir ticket de venta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent")
        waitForText("Enviado a $AGENT_TERMINAL")
        claimAndCheck(
            ids,
            expected = listOf("TICKET DE VENTA", "Atendió:", ANA_DISPLAY_NAME, BURGER_NAME),
            absent = listOf("PRECUENTA"),
        )

        // «Cuenta»: el botón cambia al volver a abrir la cuenta (se lee al abrirla).
        setPrecheckName("bill")
        backToTables()
        openAccount(ids.getValue("table"), ids.getValue("account"))
        agentPoll()
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir cuenta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent")
        waitForText("Enviada a $AGENT_TERMINAL")
        claimAndCheck(
            ids,
            expected = listOf("CUENTA", "Atendió:", ANA_DISPLAY_NAME),
            absent = listOf("PRECUENTA", "TICKET DE VENTA"),
        )
        setPrecheckName("precheck")
    }

    @Test
    fun p5_unPedidoParaLlevarConSuFolio() {
        val ids = loadIds()
        launch()
        enterAna()
        openTakeaway()
        tap(TAG_TAKEAWAY_NEW)
        waitFor(productTag(ids.getValue("burger")))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        waitFor(TAG_TAKEAWAY)
        val order = waitForOrder { it.destination.takeaway && it.state == OrderState.RECEIVED }
        val accountId = order.receivedAccountId!!
        val folio = account(accountId).getValue("folio").jsonPrimitive.content
        // El mismo botón en el pedido para llevar (la captura compartida de core:capture).
        tap("$TAG_TAKEAWAY_ENTRY_PREFIX$accountId")
        showOrder()
        agentPoll()
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir precuenta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent")
        waitForText("Enviada a $AGENT_TERMINAL")
        claimAndCheck(
            ids,
            expected = listOf("PRECUENTA", "Para llevar", "Folio", folio, BURGER_NAME),
        )
        saveIds(ids + ("takeaway" to accountId))
        // Lo que había de la mesa antes del paso sin conexión.
        saveIds(
            loadIds() + ("documentsBeforeOffline" to "${documents(ids.getValue("account")).size}"),
        )
    }

    @Test
    fun p6_sinConexionElBotonLoDice() {
        val ids = loadIds()
        launch()
        // Sin red: el mesero entra con su PIN en el dispositivo (sin sesión de Go).
        enterAna()
        openAccount(ids.getValue("table"), ids.getValue("account"))
        showPrecheck()
        waitForText("Para imprimir necesitas conexión con la computadora principal.")
        compose
            .onNodeWithTag(TAG_PRECHECK_PRINT)
            .assertIsNotEnabled()
            .assertTextEquals("Imprimir precuenta")
        assertTrue(present(TAG_PRECHECK_OFFLINE))
        // Nada queda en la cola: imprimir nunca entra en ella sin conexión.
        val ana = ids.getValue("ana")
        val queued = runBlocking { container.database.waiter().ordersOf(ana) }
        assertTrue(queued.none { it.state != OrderState.RECEIVED.storedName })
    }

    @Test
    fun p7_laImpresoraNoRespondeYReintentarSinMotivo() {
        val ids = loadIds()
        val accountId = ids.getValue("account")
        // Ninguna precuenta salió por el paso sin conexión.
        assertEquals(ids.getValue("documentsBeforeOffline").toInt(), documents(accountId).size)
        assignPrinter(ids.getValue("device"), ids.getValue("network"))
        launch()
        enterAna()
        openAccount(ids.getValue("table"), accountId)
        showPrecheck()
        waitForText("Se imprime en $NETWORK_PRINTER.")
        tapWhenShown(TAG_PRECHECK_PRINT, "Imprimir precuenta")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent")
        waitForText("Enviada a $NETWORK_PRINTER")
        // Nada escucha en ese puerto de la Mac: el trabajador de Go no la alcanza.
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}not-responding", timeout = 20_000)
        waitForText("La impresora no responde.")
        waitUntil("el trabajo fallido en Go") {
            documents(accountId).last().text("job_state") in setOf("error", "intervention")
        }
        val ticket = documents(accountId).last().text("ticket_id")
        // El trabajador de Go la reintenta solo dos veces más (2 s y 4 s después): se espera a que
        // termine para que «Reintentar» sea del mesero.
        Thread.sleep(AUTOMATIC_RETRIES_MILLIS)
        // «Reintentar» sin motivo: la misma precuenta, en la impresora del dispositivo.
        tap(TAG_PRECHECK_RETRY)
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}sent")
        waitFor("${TAG_PRECHECK_MESSAGE_PREFIX}not-responding", timeout = 20_000)
        val after = documents(accountId)
        assertEquals(ids.getValue("documentsBeforeOffline").toInt() + 1, after.size)
        assertEquals(ticket, after.last().text("ticket_id"))
        assertEquals(NETWORK_PRINTER, after.last().text("printer_name"))
    }

    // --- Pantallas ---

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
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
        waitFor(TAG_TABLES_SEARCH)
    }

    private fun openTakeaway() {
        waitFor(TAG_NAV_TAKEAWAY)
        compose.onNodeWithTag(TAG_NAV_TAKEAWAY).performClick()
        waitFor(TAG_TAKEAWAY)
    }

    private fun newAccountOn(tableId: String) {
        showTables()
        tap("$TAG_TABLE_PREFIX$tableId")
        tap(TAG_NEW_ACCOUNT)
        waitFor(TAG_PRODUCT_GRID)
    }

    private fun openAccount(tableId: String, accountId: String) {
        showTables()
        tap("$TAG_TABLE_PREFIX$tableId")
        tap("$TAG_ACCOUNT_PREFIX$accountId")
        waitFor(TAG_CAPTURE)
        showOrder()
    }

    private fun addProduct(productId: String) {
        backToCatalog()
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
    }

    /** Trae a la vista la sección de la precuenta (al final de la cuenta). */
    private fun showPrecheck() {
        waitFor(TAG_PRECHECK_PRINT)
        reveal(TAG_PRECHECK_PRINT)
    }

    /** Espera el botón de imprimir con [label] (el nombre visible) y lo toca. */
    private fun tapWhenShown(tag: String, label: String) {
        waitFor(tag)
        reveal(tag)
        compose.waitUntil(timeoutMillis = 20_000) {
            runCatching { compose.onNodeWithTag(tag).assertTextEquals(label) }.isSuccess
        }
        tap(tag)
    }

    /** En el teléfono, la cuenta está a un toque; en la tableta, siempre a la vista. */
    private fun showOrder() {
        if (!present(TAG_ORDER_PANEL) && present(TAG_PANE_OPEN_SECONDARY)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    /** En el teléfono, de la cuenta al catálogo; el primer Atrás puede cerrar solo el teclado. */
    private fun backToCatalog() {
        repeat(3) {
            if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
                pressBack()
            }
        }
    }

    /** Sale de la captura (o de la mesa en el teléfono) hasta la lista de Mesas. */
    private fun backToTables() {
        repeat(4) {
            if (!present(TAG_TABLES_SEARCH) || present(TAG_CAPTURE)) {
                pressBack()
            }
        }
        if (!present(TAG_TABLES_SEARCH) && present(TAG_NAV_TABLES)) {
            compose.onNodeWithTag(TAG_NAV_TABLES).performClick()
        }
        waitFor(TAG_TABLES_SEARCH)
    }

    /** Mesas: en el teléfono con la mesa abierta, primero «Volver a Mesas». */
    private fun showTables() {
        waitUntil("Mesas") { present(TAG_TABLES_SEARCH) || present(TAG_ACCOUNT_PANE) }
        if (present(TAG_ACCOUNT_PANE) && present(TAG_PANE_BACK)) {
            pressBack()
        }
    }

    private fun pressBack() {
        // Un diálogo del sistema «… no responde» (emulador cargado) se quedaría con la tecla.
        SystemDialogs.dismissNotResponding(automation)
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        Thread.sleep(SETTLE_MILLIS)
    }

    /**
     * Espera a que exista un nodo con [tag]; lo de una lista perezosa solo existe compuesto: cada
     * segundo se desplazan hasta él las listas a la vista.
     */
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

    /** Toca el nodo con [tag], desplazando hasta él si está en una lista. */
    private fun tap(tag: String) {
        waitFor(tag)
        if (!displayed(tag)) {
            reveal(tag)
        }
        val node = compose.onNodeWithTag(tag)
        runCatching { node.performScrollTo() }
        node.performClick()
    }

    private fun displayed(tag: String): Boolean = runCatching {
        compose.onNodeWithTag(tag).assertIsDisplayed()
    }
        .isSuccess

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    // --- Lo guardado en el dispositivo ---

    private fun waitForOrder(
        timeout: Long = 60_000,
        condition: (PendingOrder) -> Boolean,
    ): PendingOrder {
        val ana = loadIds().getValue("ana")
        var found: PendingOrder? = null
        waitUntil("el pedido", timeout) {
            found = runBlocking {
                container.database.waiter().ordersOf(ana).map(WaiterStorage::toOrder).lastOrNull {
                    condition(it)
                }
            }
            found != null
        }
        return found!!
    }

    // --- La computadora principal ---

    private fun setWaiterPermissions(permissions: List<String>) {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.text("name") == ROLE }
        val list = permissions.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.text("id")}",
                """{"name":"$ROLE","permissions":[$list]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    /** Catálogo de dos productos y una mesa. */
    private fun createCatalogAndTable(): Map<String, String> {
        val category =
            main.call(
                "POST",
                "catalog/categories",
                """{"name":"Comida P","active":true,"color":"#d64545"}""",
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
        val area =
            main.call(
                "POST",
                "restaurant/areas",
                """{"operation_id":"${UUID.randomUUID()}","name":"Salón P","active":true}""",
            )
        assertEquals(area.body.toString(), 200, area.status)
        val table =
            main.call(
                "POST",
                "restaurant/tables",
                """{"operation_id":"${UUID.randomUUID()}","area_id":"${area.obj.text("id")}",""" +
                    """"name":"$TABLE_NAME","shape":"rectangle","x":20,"y":40,"width":100,""" +
                    """"height":72,"seats":4,"active":true}""",
            )
        assertEquals(table.body.toString(), 200, table.status)
        return mapOf(
            "burger" to product(BURGER_NAME, "P-HAM", "120"),
            "fries" to product(FRIES_NAME, "P-PAP", "45"),
            "table" to table.obj.text("id"),
        )
    }

    /**
     * La terminal de prueba «Caja 2» (inscrita por la escucha local con un código de la principal,
     * como una caja de la misma computadora) informa su impresora USB, que se registra como
     * «Impresora USB de una terminal»; y una impresora de red en un puerto de la Mac donde nada
     * escucha (no responde).
     */
    private fun createAgentTerminalAndPrinters(): Map<String, String> {
        val invitation = main.call("POST", "devices/invitations")
        assertEquals(invitation.body.toString(), 200, invitation.status)
        val enrolled =
            raw(
                "POST",
                "devices/enroll",
                """{"code":"${invitation.obj.text("code")}","name":"$AGENT_TERMINAL"}""",
                emptyMap(),
            )
        assertEquals(enrolled.toString(), 200, enrolled.first)
        val agent = enrolled.second.jsonObject
        val agentId = agent.text("device_id")
        val agentToken = agent.text("device_token")
        val reported =
            raw(
                "PUT",
                "print-agent/printers",
                """{"printers":[{"queue":"$AGENT_QUEUE","model":"POS-80 Series",""" +
                    """"connection":"usb","printer_type":36876}]}""",
                agentHeaders(agentId, agentToken),
            )
        assertEquals(reported.toString(), 200, reported.first)
        fun printer(body: String): String {
            val answer =
                main.call(
                    "POST",
                    "kitchen/printers",
                    """{"operation_id":"${UUID.randomUUID()}",$body}""",
                )
            assertEquals(answer.body.toString(), 200, answer.status)
            return answer.obj.text("id")
        }
        return mapOf(
            "agentId" to agentId,
            "agentToken" to agentToken,
            "usb" to
                printer(
                    """"name":"$USB_PRINTER","transport":"agent","destination":"$AGENT_QUEUE",""" +
                        """"paper_width":80,"copies":1,"active":true,"agent_device_id":"$agentId"""",
                ),
            "network" to
                printer(
                    """"name":"$NETWORK_PRINTER","transport":"escpos_tcp",""" +
                        """"destination":"127.0.0.1","port":$NETWORK_PORT,"paper_width":80,""" +
                        """"copies":1,"active":true""",
                ),
        )
    }

    /** El teléfono o tableta emparejado (Terminales). */
    private fun androidDevice(): JsonObject =
        main
            .call("GET", "devices")
            .body
            .jsonArray
            .map { it.jsonObject }
            .single { it.text("kind") == "android" && it["revoked_at"] is JsonNull }

    /** Administración › Sistema › Terminales › Editar › «Impresora de precuentas». */
    private fun assignPrinter(deviceId: String, printerId: String?) {
        val device = androidDevice()
        assertEquals(deviceId, device.text("id"))
        val printer = printerId?.let { "\"$it\"" } ?: "null"
        val answer =
            main.call(
                "PATCH",
                "devices/$deviceId",
                """{"name":${JsonPrimitive(device.text("name"))},"register_id":null,""" +
                    """"keyboard_mode":"${device.text("keyboard_mode")}",""" +
                    """"precheck_printer_id":$printer}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
        assertEquals(
            printerId,
            (answer.obj["precheck_printer_id"] as? JsonPrimitive)?.contentOrNull,
        )
    }

    /** Administración › Documentos: el nombre visible (Precuenta, Ticket de venta o Cuenta). */
    private fun setPrecheckName(key: String) {
        val identity = main.call("GET", "documents/identity").obj
        val body =
            JsonObject(
                mapOf(
                    "operation_id" to JsonPrimitive(UUID.randomUUID().toString()),
                    "revision" to identity.getValue("revision"),
                    "additional_info" to identity.getValue("additional_info"),
                    "header_text" to identity.getValue("header_text"),
                    "footer_text" to identity.getValue("footer_text"),
                    "presentation" to identity.getValue("presentation"),
                    "precheck_name" to JsonPrimitive(key),
                ),
            )
        val answer = main.call("PUT", "documents/identity", body.toString())
        assertEquals(answer.body.toString(), 200, answer.status)
        assertEquals(key, answer.obj.text("precheck_name"))
    }

    /** La cuenta en Go (`Account`). */
    private fun account(accountId: String): JsonObject =
        main.call("GET", "restaurant/accounts/$accountId").obj.getValue("account").jsonObject

    /** Precuentas y tickets de la cuenta con el estado de su último trabajo (Cobros). */
    private fun documents(accountId: String): List<JsonObject> {
        val answer = main.call("GET", "payments/accounts/$accountId")
        assertEquals(answer.body.toString(), 200, answer.status)
        val documents = answer.obj["documents"] as? JsonArray ?: return emptyList()
        // En el orden de Go (por creación).
        return documents.map { it.jsonObject }
    }

    // --- El agente de impresión de «Caja 2» ---

    private fun agentHeaders(deviceId: String, token: String) =
        mapOf("X-Device-ID" to deviceId, "X-Device-Token" to token)

    /** El agente consulta sin trabajo pendiente: la terminal queda conectada (30 s). */
    private fun agentPoll() {
        val ids = loadIds()
        val answer =
            raw(
                "POST",
                "print-agent/jobs/claim",
                """{"claim_id":"${UUID.randomUUID()}"}""",
                agentHeaders(ids.getValue("agentId"), ids.getValue("agentToken")),
            )
        assertEquals(answer.toString(), 200, answer.first)
        assertTrue("sin trabajos pendientes", answer.second.jsonObject["job"] is JsonNull)
    }

    /**
     * Reclama el trabajo como el agente de «Caja 2», revisa sus bytes ESC/POS (CP850, la página de
     * códigos del controlador de Go) y lo confirma como enviado a la cola de su computadora.
     */
    private fun claimAndCheck(
        ids: Map<String, String>,
        expected: List<String>,
        absent: List<String> = emptyList(),
    ) {
        val headers = agentHeaders(ids.getValue("agentId"), ids.getValue("agentToken"))
        val claimed =
            raw(
                "POST",
                "print-agent/jobs/claim",
                """{"claim_id":"${UUID.randomUUID()}"}""",
                headers,
            )
        assertEquals(claimed.toString(), 200, claimed.first)
        val job = claimed.second.jsonObject["job"] as? JsonObject ?: error("Sin trabajo: $claimed")
        assertEquals(AGENT_QUEUE, job.text("queue"))
        val data = Base64.getDecoder().decode(job.text("data"))
        assertEquals(job.getValue("size").jsonPrimitive.int, data.size)
        log("precuenta impresa (${data.size} bytes): ${printable(data)}")
        for (text in expected) {
            assertTrue("el impreso dice «$text»: ${printable(data)}", data.contains(cp850(text)))
        }
        for (text in absent) {
            assertFalse("el impreso no dice «$text»", data.contains(cp850(text)))
        }
        val result =
            raw(
                "POST",
                "print-agent/jobs/${job.text("id")}/result",
                """{"attempt_id":"${job.text("attempt_id")}","outcome":"submitted",""" +
                    """"reference":"$AGENT_QUEUE-1"}""",
                headers,
            )
        assertEquals(result.toString(), 200, result.first)
    }

    /** Petición a la escucha local sin la sesión de la principal (inscripción y agente). */
    private fun raw(
        method: String,
        path: String,
        body: String,
        headers: Map<String, String>,
    ): Pair<Int, JsonElement> {
        val builder =
            Request.Builder()
                .url("${argument("local")}/api/v1/$path")
                .method(method, body.toRequestBody(JSON))
        headers.forEach { (name, value) -> builder.header(name, value) }
        plain.newCall(builder.build()).execute().use { response ->
            val text = response.body.string()
            val parsed = if (text.isBlank()) JsonNull else Json.parseToJsonElement(text)
            return response.code to parsed
        }
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
        const val ANA_USER = "ana.p1"
        const val ANA_PIN = "4829"

        /** `display_name` que le pone [MainComputer] («Atendió:» del impreso). */
        const val ANA_DISPLAY_NAME = "Ana A1"
        const val ROLE = "Meseros A1"
        const val TABLE_NAME = "Mesa P1"
        const val BURGER_NAME = "Hamburguesa P"
        const val FRIES_NAME = "Papas P"
        const val AGENT_TERMINAL = "Caja 2"
        const val AGENT_QUEUE = "POS_80_P"
        const val USB_PRINTER = "USB Caja 2"
        const val NETWORK_PRINTER = "Red P"

        /** Un puerto de impresora (9100–9109) donde nada escucha en la Mac: «no responde». */
        const val NETWORK_PORT = 9106

        /** Los dos reintentos automáticos del trabajador de Go (2 s y 4 s) con margen. */
        const val AUTOMATIC_RETRIES_MILLIS = 9_000L
        const val PRINT_PERMISSION = "payments.tickets.print"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L
        val JSON = "application/json; charset=utf-8".toMediaType()

        /** Las listas perezosas en las que se busca desplazando. */
        val LAZY_LISTS = listOf(TAG_ORDER_PANEL, TAG_TAKEAWAY, TAG_TABLES_LIST, TAG_PRODUCT_GRID)

        /**
         * Los permisos de mesero de la guía con Para llevar, **sin** imprimir (el paso 1 lo agrega
         * al final, como el administrador en Personal › Roles).
         */
        val WAITER_PERMISSIONS =
            listOf(
                "waiter_devices.use",
                "restaurant.tables.read",
                "restaurant.areas.read",
                "accounts.read",
                "accounts.open",
                "orders.capture",
                "orders.confirm",
                "orders.edit",
                "orders.cancel",
                "catalog.read",
                "catalog.prices.read",
                "events.read",
                "takeaway.read",
                "takeaway.manage",
            )

        // Página de códigos 850 (la del controlador ESC/POS de Go) para lo que revisa la prueba.
        private val CP850 =
            mapOf(
                'á' to 0xA0,
                'é' to 0x82,
                'í' to 0xA1,
                'ó' to 0xA2,
                'ú' to 0xA3,
                'ñ' to 0xA4,
                'Á' to 0xB5,
                'É' to 0x90,
                'Í' to 0xD6,
                'Ó' to 0xE0,
                'Ú' to 0xE9,
                'Ñ' to 0xA5,
            )

        fun cp850(text: String): ByteArray =
            ByteArray(text.length) { index ->
                val character = text[index]
                val code = CP850[character] ?: character.code
                check(code < 0x100) { "Sin CP850 para «$character»" }
                code.toByte()
            }

        fun ByteArray.contains(part: ByteArray): Boolean {
            if (part.isEmpty()) {
                return true
            }
            for (start in 0..size - part.size) {
                var same = true
                for (offset in part.indices) {
                    if (this[start + offset] != part[offset]) {
                        same = false
                        break
                    }
                }
                if (same) {
                    return true
                }
            }
            return false
        }

        /** Los bytes como texto para el registro (los de control como «·»). */
        fun printable(data: ByteArray): String =
            data.joinToString("") { byte ->
                val code = byte.toInt() and 0xFF
                when {
                    code in 0x20..0x7E -> code.toChar().toString()
                    code == 0x0A -> "⏎"
                    else -> CP850.entries.firstOrNull { it.value == code }?.key?.toString() ?: "·"
                }
            }
    }
}
