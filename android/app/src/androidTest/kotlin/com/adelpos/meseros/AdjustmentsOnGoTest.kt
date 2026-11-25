package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import android.graphics.Bitmap
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsDisplayed
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
import androidx.compose.ui.text.TextLayoutResult
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.model.error.AppError
import com.adelpos.core.ui.access.findserver.TAG_ADDRESS
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.findserver.TAG_PAIRING_CODE
import com.adelpos.core.ui.components.TAG_TOP_BAR
import com.adelpos.core.ui.components.TAG_TOP_BAR_BACK
import com.adelpos.core.ui.components.TAG_TOP_BAR_LOGO
import com.adelpos.meseros.core.capture.TAG_CAPTURE
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_PRECHECK_PRINT
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_CAPTURE_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_RETRY_NOW
import com.adelpos.meseros.core.ui.catalog.TAG_CATALOG_SEARCH
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_WITHOUT_IMAGE
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.components.TAG_WAITER_MENU
import com.adelpos.meseros.core.ui.components.TAG_WAITER_SWITCH
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_BACK
import com.adelpos.meseros.feature.access.pin.TAG_PIN_PANEL
import com.adelpos.meseros.feature.access.pin.TAG_REENTER
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_CARD_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_TITLE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_DISCARD_EMPTY_CONFIRM
import com.adelpos.meseros.feature.waiter.tables.TAG_DISCARD_EMPTY_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_DISCARD_RESULT
import com.adelpos.meseros.feature.waiter.tables.TAG_MAP_CANVAS
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREA_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_BAR
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_MAP
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_VIEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import com.adelpos.meseros.navigation.TAG_APP_NOTICE
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
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3, §4 (los ajustes de la aplicación), con la aplicación completa contra el **Go temporal**
 * del arnés, nunca el servicio real, en vertical y en horizontal con la rotación real
 * (`UiAutomation.setRotation`) y capturas de pantalla de cada paso (el guion las copia a
 * `<salida>/screens`). Se corre con `scripts/android-e2e.sh --suite adjustments` en el teléfono y
 * en la tableta:
 * - `a1` restaurante (dos áreas con mesas, catálogo sin imágenes, una cuenta vacía en la Mesa A1),
 *   emparejar y entrar.
 * - `a2` Mesas compacta (§4.7): una sola fila con «Lista | Mapa», las áreas y «Pendientes»; sin
 *   búsqueda; las áreas como pestañas; el mapa llena el espacio; tocar la mesa abre su panel (sin
 *   «Ver la mesa») y la flecha lo cierra (§4.1); las cuentas en dos renglones (§4.8).
 * - `a3` desde el panel (§4.9 y §4.10): «Imprimir precuenta» llega a Go (sin impresora asignada, Go
 *   lo dice) y «Descartar cuenta vacía» con su confirmación descarta la cuenta en Go.
 * - `a4` la captura (§4.1–§4.4, §4.6, §4.11): el título dice la mesa, el buscador es de una línea
 *   «Buscar producto», las tarjetas sin imagen son compactas, «Ver la cuenta · 1 por enviar»; al
 *   salir con la flecha, lo capturado se queda guardado y se dice; «Pendientes» con flecha y sin
 *   «Reintentar ya».
 * - `a5` la entrada (§4.5): «Ingresar» con el logotipo, «Selecciona tu usuario», «Elegir otro
 *   usuario».
 * - `a6` la pantalla inicial Para llevar (§4.12): Terminales la cambia y el mesero entra ahí.
 * - `a7` el teclado del PIN completo (§4, teléfono horizontal) en la entrada y en «Vuelve a
 *   escribir tu PIN», en vertical y horizontal, entrando con el PIN de Ana.
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice` y `mainToken` (los arma el guion).
 */
@RunWith(AndroidJUnit4::class)
class AdjustmentsOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "adjustments-on-go.json")

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
    fun a1_prepararEmparejarYEntrar() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantWaiterPermissions()
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createAreasAndTables())
        ids.putAll(createCatalog())
        ids["empty"] = openEmptyAccount(ids.getValue("tableA1"))
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        // Una hora de bloqueo por inactividad: los pasos no se interrumpen.
        patchDevice(home = "tables")
        enterAna()
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("water")) != null
        }
    }

    @Test
    fun a2_mesasCompactaEnLasDosOrientaciones() {
        val ids = loadIds()
        launch()
        enterAna()
        for (landscape in orientations()) {
            rotate(landscape)
            val name = orientationName(landscape)
            waitFor(TAG_TABLES_BAR)
            // §4.7: una sola fila, sin la búsqueda de mesa.
            assertTrue(!present("tables-search"))
            assertInRow(TAG_TABLES_BAR, "$TAG_TABLES_VIEW-list")
            assertInRow(TAG_TABLES_BAR, "${TAG_TABLES_AREA_PREFIX}${ids.getValue("areaA")}")
            assertInRow(TAG_TABLES_BAR, TAG_OPEN_PENDING)
            // Las áreas como pestañas: la lista muestra solo la elegida.
            tap("${TAG_TABLES_AREA_PREFIX}${ids.getValue("areaB")}")
            waitFor("$TAG_TABLE_PREFIX${ids.getValue("tableB1")}")
            assertTrue(!present("$TAG_TABLE_PREFIX${ids.getValue("tableA1")}"))
            tap("${TAG_TABLES_AREA_PREFIX}${ids.getValue("areaA")}")
            waitFor("$TAG_TABLE_PREFIX${ids.getValue("tableA1")}")
            screenshot("a2-mesas-lista-$name")
            // El mapa llena el espacio disponible (antes, diminuto en la tableta vertical y sin
            // altura en el teléfono horizontal).
            tap("$TAG_TABLES_VIEW-map")
            waitFor(TAG_MAP_CANVAS)
            val map = bounds(TAG_TABLES_MAP)
            val canvas = bounds(TAG_MAP_CANVAS)
            log("mapa $name: ${map.width}×${map.height}, lienzo ${canvas.width}×${canvas.height}")
            assertTrue("el mapa tiene alto en $name", map.height > 100f)
            assertTrue("el lienzo llena el mapa en $name", canvas.height >= map.height * 0.9f)
            screenshot("a2-mesas-mapa-$name")
            tap("$TAG_TABLES_VIEW-list")
            waitFor(TAG_TABLES_LIST)
            // Tocar la mesa abre su panel (en el teléfono, sin «Ver la mesa»).
            assertTrue(!present(TAG_PANE_OPEN_SECONDARY))
            tap("$TAG_TABLE_PREFIX${ids.getValue("tableA1")}")
            waitFor(TAG_ACCOUNT_PANE)
            val empty = ids.getValue("empty")
            waitFor("$TAG_ACCOUNT_CARD_PREFIX$empty")
            // §4.8: dos renglones, el título en uno solo.
            assertEquals(
                "título de la cuenta en un renglón ($name)",
                1,
                // El encabezado se puede tocar y junta sus textos: el título, en el árbol sin
                // juntar.
                lines(
                    compose.onNodeWithTag(
                        "$TAG_ACCOUNT_TITLE_PREFIX$empty",
                        useUnmergedTree = true,
                    ),
                ),
            )
            screenshot("a2-panel-de-la-mesa-$name")
            closeTablePanel()
        }
    }

    @Test
    fun a3_precuentaYDescartarCuentaVaciaDesdeElPanel() {
        val ids = loadIds()
        val empty = ids.getValue("empty")
        launch()
        enterAna()
        tap("$TAG_TABLE_PREFIX${ids.getValue("tableA1")}")
        waitFor("$TAG_ACCOUNT_CARD_PREFIX$empty")
        // §4.9: «Imprimir precuenta» en la cuenta del panel llega a Go con la sesión del mesero;
        // el dispositivo no tiene impresora asignada y Go lo dice (PRINTER_NOT_CONFIGURED).
        val card = hasTestTag(TAG_PRECHECK_PRINT)
        reveal(TAG_PRECHECK_PRINT)
        compose.onAllNodes(card).onFirst().performClick()
        waitForText("no tiene impresora asignada", timeout = 30_000)
        screenshot("a3-precuenta-desde-el-panel")
        // §4.10: «Descartar cuenta vacía» con su confirmación; Go la descarta.
        tap("$TAG_DISCARD_EMPTY_PREFIX$empty")
        waitFor(TAG_DISCARD_EMPTY_CONFIRM)
        screenshot("a3-descartar-cuenta-vacia")
        compose.onNodeWithTag(TAG_DISCARD_EMPTY_CONFIRM).performClick()
        waitFor(TAG_DISCARD_RESULT)
        waitUntil("la cuenta descartada en Go") { accountState(empty) != "open" }
        waitUntil("la cuenta ya no está en el panel") { !present("$TAG_ACCOUNT_CARD_PREFIX$empty") }
        screenshot("a3-cuenta-descartada")
    }

    @Test
    fun a4_capturaConTituloBuscadorFlechaYPendientes() {
        val ids = loadIds()
        launch()
        enterAna()
        for (landscape in orientations()) {
            rotate(landscape)
            val name = orientationName(landscape)
            tap("$TAG_TABLE_PREFIX${ids.getValue("tableA2")}")
            tap(TAG_NEW_ACCOUNT)
            waitFor(TAG_CAPTURE)
            // §4.2: la barra dice dónde se está.
            waitForTitle("Mesa A2")
            // §4.3: «Buscar producto» en una sola línea.
            val label = compose.onNode(hasText("Buscar producto"), useUnmergedTree = true)
            assertEquals(1, lines(label))
            assertTrue(present(TAG_CATALOG_SEARCH))
            // §4.11: sin imagen, tarjeta compacta.
            waitFor(productTag(ids.getValue("water")))
            assertTrue(present(TAG_PRODUCT_WITHOUT_IMAGE))
            screenshot("a4-captura-catalogo-$name")
            addProduct(ids.getValue("water"))
            // §4.4: en el teléfono, «Ver la cuenta · 1 por enviar»; en la tableta la cuenta está
            // siempre a la vista.
            if (present(TAG_PANE_OPEN_SECONDARY)) {
                waitForText("Ver la cuenta · 1 por enviar")
                screenshot("a4-ver-la-cuenta-$name")
            } else {
                waitFor(TAG_ORDER_PANEL)
                screenshot("a4-captura-con-la-cuenta-$name")
            }
            // §4.1: la flecha de la barra regresa (en el teléfono, primero cierra la cuenta); lo
            // capturado queda guardado y se dice.
            backToCatalog()
            tap(TAG_TOP_BAR_BACK)
            waitUntil("Mesas") { present(TAG_TABLES_SCREEN) || present(TAG_ACCOUNT_PANE) }
            waitForText("quedó guardado sin enviar a cocina")
            compose.onNodeWithTag(TAG_APP_NOTICE).assertIsDisplayed()
            screenshot("a4-aviso-al-salir-$name")
            closeTablePanel()
            // §4.6: «Pendientes» con su título corto, la flecha y sin «Reintentar ya» vacío.
            tap(TAG_OPEN_PENDING)
            waitFor(TAG_PENDING)
            waitForTitle("Pendientes")
            // El aviso era de Mesas: al pasar a otra pantalla se quita (no tapa sus botones).
            waitUntil("el aviso se quita al pasar a Pendientes") {
                compose
                    .onAllNodes(hasText("quedó guardado", substring = true))
                    .fetchSemanticsNodes()
                    .isEmpty()
            }
            assertTrue(present(TAG_TOP_BAR_BACK))
            assertTrue("sin nada por enviar no hay «Reintentar ya»", !present(TAG_RETRY_NOW))
            val captureId = waitForCapture()
            waitFor("$TAG_PENDING_CAPTURE_PREFIX$captureId")
            screenshot("a4-pendientes-$name")
            // Se envía para dejar la mesa como estaba antes de la otra orientación.
            tap("$TAG_PENDING_CAPTURE_PREFIX$captureId")
            waitFor(TAG_CAPTURE)
            showOrder()
            tap(TAG_SEND_TO_KITCHEN)
            waitUntil("Mesas") { present(TAG_TABLES_SCREEN) || present(TAG_ACCOUNT_PANE) }
            closeTablePanel()
        }
    }

    @Test
    fun a5_entradaConLosTextosNuevos() {
        val ana = loadIds().getValue("ana")
        launch()
        enterAna()
        tap(TAG_WAITER_MENU)
        tap(TAG_WAITER_SWITCH)
        for (landscape in orientations()) {
            rotate(landscape)
            val name = orientationName(landscape)
            waitFor(TAG_ENTRY)
            waitForTitle("Ingresar")
            assertTrue(present(TAG_TOP_BAR_LOGO))
            waitForText("Selecciona tu usuario y escribe tu PIN.")
            screenshot("a5-entrada-$name")
            tap("$TAG_WAITER_PREFIX$ana")
            waitForText("Ingresar")
            if (present(TAG_ENTRY_BACK)) {
                waitForText("Elegir otro usuario")
                screenshot("a5-pin-$name")
                tap(TAG_ENTRY_BACK)
            }
        }
    }

    @Test
    fun a6_pantallaInicialParaLlevar() {
        launch()
        patchDevice(home = "takeaway")
        try {
            enterAna(waitForTables = false)
            // §4.12: entra en Para llevar (la pantalla inicial de Terminales), no en Mesas.
            waitFor(TAG_TAKEAWAY)
            waitForTitle("Para llevar")
            screenshot("a6-inicio-para-llevar")
        } finally {
            patchDevice(home = "tables")
        }
    }

    @Test
    fun a7_tecladoDelPinCompletoEnVerticalYHorizontal() {
        val ana = loadIds().getValue("ana")
        launch()
        for (landscape in orientations()) {
            rotate(landscape)
            val name = orientationName(landscape)
            // La entrada: las doce teclas a la vista, sin deslizar.
            waitFor("$TAG_WAITER_PREFIX$ana")
            tap("$TAG_WAITER_PREFIX$ana")
            waitFor(TAG_PIN_SUBMIT)
            PinPadChecks.assertWholePad(compose, TAG_PIN_PANEL, "la entrada en $name")
            screenshot("a7-pin-$name")
            for (digit in ANA_PIN) {
                compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
            }
            compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
            waitFor(TAG_TABLES_SCREEN)
            // «Vuelve a escribir tu PIN» encima de Mesas (la sesión de Go se dio por terminada).
            container.deviceAccess.report(
                AppError.SessionEnded("SESSION_EXPIRED", null, null),
                ana,
            )
            waitFor(TAG_REENTER)
            PinPadChecks.assertWholePad(compose, TAG_REENTER, "el diálogo en $name")
            screenshot("a7-vuelve-a-escribir-$name")
            for (digit in ANA_PIN) {
                compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
            }
            compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
            waitUntil("el diálogo cerrado") { !present(TAG_REENTER) }
            // De vuelta a la entrada para la otra orientación.
            tap(TAG_WAITER_MENU)
            tap(TAG_WAITER_SWITCH)
        }
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

    private fun enterAna(waitForTables: Boolean = true) {
        val ana = loadIds().getValue("ana")
        waitFor("$TAG_WAITER_PREFIX$ana")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        for (digit in ANA_PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        if (waitForTables) {
            waitFor(TAG_TABLES_SCREEN)
        }
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

    /** En el teléfono, el panel de la mesa abierto se cierra con la flecha de la barra. */
    private fun closeTablePanel() {
        if (present(TAG_ACCOUNT_PANE) && present(TAG_PANE_BACK)) {
            compose.onNodeWithTag(TAG_PANE_BACK).performClick()
            waitFor(TAG_TABLES_SCREEN)
        }
    }

    private fun waitForCapture(): String {
        val ana = loadIds().getValue("ana")
        var id: String? = null
        waitUntil("la captura guardada") {
            id = runBlocking { container.database.waiter().capturesOf(ana).lastOrNull()?.id }
            id != null
        }
        return id!!
    }

    private fun waitForTitle(title: String) {
        compose.waitUntil(timeoutMillis = 20_000) {
            compose
                .onAllNodes(
                    hasText(title) and hasAnyAncestor(hasTestTag(TAG_TOP_BAR)),
                    useUnmergedTree = true,
                )
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }

    private fun assertInRow(row: String, tag: String) {
        val outer = bounds(row)
        val inner = bounds(tag)
        assertTrue(
            "$tag en la fila $row ($inner dentro de $outer)",
            inner.top >= outer.top - 1f && inner.bottom <= outer.bottom + 1f,
        )
    }

    private fun bounds(tag: String) =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .onFirst()
            .fetchSemanticsNode()
            .boundsInRoot

    private fun lines(node: SemanticsNodeInteraction): Int {
        val layouts = mutableListOf<TextLayoutResult>()
        node
            .fetchSemanticsNode()
            .config[SemanticsActions.GetTextLayoutResult]
            .action
            ?.invoke(
                layouts,
            )
        return layouts.single().lineCount
    }

    // --- Orientación y capturas ---

    /** Vertical y horizontal, empezando por la natural del dispositivo. */
    private fun orientations(): List<Boolean> {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        Thread.sleep(SETTLE_MILLIS)
        val naturalLandscape = orientationNow() == Configuration.ORIENTATION_LANDSCAPE
        return listOf(naturalLandscape, !naturalLandscape)
    }

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

    private fun orientationName(landscape: Boolean) = if (landscape) "horizontal" else "vertical"

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
        val areaA = area("Salón A3")
        val areaB = area("Terraza A3")
        return mapOf(
            "areaA" to areaA,
            "areaB" to areaB,
            "tableA1" to table(areaA, "Mesa A1", 60, 60),
            "tableA2" to table(areaA, "Mesa A2", 320, 60),
            "tableA3" to table(areaA, "Mesa A3", 580, 300),
            "tableB1" to table(areaB, "Terraza B1", 100, 100),
        )
    }

    /** Productos sin imagen (todos los de la prueba): la tarjeta compacta (§4.11). */
    private fun createCatalog(): Map<String, String> {
        val category =
            main.call(
                "POST",
                "catalog/categories",
                """{"name":"Bebidas A3","active":true,"color":"#3591d4"}""",
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
            "water" to product("Agua de horchata A3", "A3-AGU", "35"),
            "coffee" to product("Café de olla A3", "A3-CAF", "30"),
        )
    }

    /** Una cuenta abierta sin productos en la mesa (Go la marca `discardable`). */
    private fun openEmptyAccount(tableId: String): String {
        val answer =
            main.call(
                "POST",
                "restaurant/accounts",
                """{"operation_id":"${UUID.randomUUID()}","mode":"dine_in","table_id":"$tableId"}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
        val account = answer.obj.getValue("account").jsonObject
        assertEquals("true", account.getValue("discardable").jsonPrimitive.content)
        return account.text("id")
    }

    private fun accountState(accountId: String): String =
        main
            .call("GET", "restaurant/accounts/$accountId")
            .obj
            .getValue("account")
            .jsonObject
            .text("state")

    /** La pantalla inicial de este dispositivo en Terminales y una hora de bloqueo. */
    private fun patchDevice(home: String) {
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
                    """"home_screen":"$home","idle_lock_seconds":3600}""",
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
        const val ANA_USER = "ana.a3"
        const val ANA_PIN = "4829"
        const val ROLE = "Meseros A1"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L

        /** Las listas perezosas en las que se busca desplazando. */
        val LAZY_LISTS =
            listOf(
                TAG_ACCOUNT_PANE,
                TAG_TABLES_LIST,
                TAG_PENDING,
                TAG_ORDER_PANEL,
                TAG_PRODUCT_GRID,
            )

        /** Lo que usa un mesero en Mesas, la precuenta y Para llevar. */
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
                "takeaway.read",
                "takeaway.manage",
                "takeaway.deliver",
                "payments.tickets.print",
            )
    }
}
