package com.adelpos.meseros

import android.app.UiAutomation
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
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
import com.adelpos.meseros.core.capture.TAG_CANCEL_CONFIRM
import com.adelpos.meseros.core.capture.TAG_DISCARD_CAPTURE
import com.adelpos.meseros.core.capture.TAG_DISCARD_CONFIRM
import com.adelpos.meseros.core.capture.TAG_DISCARD_PENDING_OK
import com.adelpos.meseros.core.capture.TAG_DRAFT_ROW_PREFIX
import com.adelpos.meseros.core.capture.TAG_MODIFY_CONFIRM
import com.adelpos.meseros.core.capture.TAG_MODIFY_NOTES
import com.adelpos.meseros.core.capture.TAG_MODIFY_PREVIEW
import com.adelpos.meseros.core.capture.TAG_MODIFY_QUOTE
import com.adelpos.meseros.core.capture.TAG_OPEN_ANOTHER
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SENT_CANCEL_PREFIX
import com.adelpos.meseros.core.capture.TAG_SENT_MODIFY_PREFIX
import com.adelpos.meseros.core.capture.TAG_SENT_REASON
import com.adelpos.meseros.core.capture.TAG_SENT_ROW_PREFIX
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_ORDER_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_REVIEW_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_ACCEPT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_CHANGE_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_REMOVE_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_SEND_TO_MERGED
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.core.ui.waiter.TAG_ORDER_STATE_PREFIX
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import java.io.File
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
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Modo Mesero contra el **Go temporal** del arnés (§9), nunca el servicio real. La prueba hace de
 * computadora principal por la escucha local ([MainComputer]) y la aplicación usa la escucha de red
 * con la huella fijada al emparejar. Cada paso corre en su propia instrumentación, en orden, con la
 * aplicación instalada una sola vez (`scripts/android-e2e.sh --suite waiter`): el proceso se
 * reinicia en cada paso, así que también se comprueba que capturas y pedidos sobreviven al cierre.
 * **La red la corta y la devuelve el guion entre pasos** (modo avión en Android 11+; `adb root` y
 * `svc wifi/data` en Android 8–10).
 *
 * - `w1` prepara el restaurante (rol de meseros con sus permisos, catálogo, área y mesas), empareja
 *   con el código y entra con el PIN.
 * - `w2` captura **en línea** en una cuenta nueva (creación diferida), gira la pantalla sin perder
 *   lo capturado y «Enviar a cocina» → «Recibido por el restaurante»; después otra terminal (la
 *   principal) guarda un renglón y lo confirma, y la captura lo ve al momento.
 * - `w3` (sin red) entra sin conexión y captura en otra mesa: el pedido queda «Por enviar».
 * - `w4` (con red y `lostOfflineResponses=1`) sin volver a entrar: WorkManager envía con la
 *   credencial de envío; la primera respuesta se pierde y el reintento con la misma clave **no
 *   duplica** el pedido.
 * - `w5` precio cambiado y producto agotado mientras se capturaba → «Requiere tu revisión» → quitar
 *   lo agotado, aceptar y enviar con el precio nuevo.
 * - `w6` la mesa se cobra mientras el mesero tiene la cuenta abierta: Go rechaza el guardado
 *   (`account_not_open`, «…ya se cobró») y lo capturado pasa a una cuenta nueva de la mesa.
 * - `w7a` (con red) borrador guardado en Go; `w7b` (sin red) se agrega más y se envía sin conexión;
 *   `w7c` (con red) el pedido sustituye los renglones del borrador (A1-D6): la cuenta queda con
 *   todo confirmado y sin borrador duplicado.
 *
 * - Revisión R6 (A1-D19): `w9` el guardado del borrador pierde su respuesta y se toca «Enviar a
 *   cocina»: se resuelve con su clave y entra una vez; `w10` la confirmación en línea entra y sus
 *   respuestas se pierden: el pedido sin conexión con la misma clave queda «Recibido» sin duplicar;
 *   `w11` descartar con conexión quita de Go lo guardado (y la cuenta vacía que nació con la
 *   captura); `w12a`–`w12c` descartar sin conexión queda pendiente y se termina al volver;
 *   `w13a`–`w13c` la cuenta se combina mientras el pedido espera sin conexión y el mesero lo manda
 *   a la cuenta en que se combinó.
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice`, `mainToken` (los arma el guion) y, en `w4`,
 * `w9` y `w10`, `lostOfflineResponses`, `lostDraftResponses` y `lostConfirmResponses` (los lee
 * [AdelTestRunner]). Sin ellos se omite.
 */
@RunWith(AndroidJUnit4::class)
class WaiterModeOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "waiter-on-go.json")

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
    fun w1_prepararEmparejarYEntrar() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantWaiterPermissions()
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createCatalog())
        ids.putAll(createTables())
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        enterAna()
        waitFor("$TAG_TABLE_PREFIX${ids.getValue("table1")}")
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("burger")) != null
        }
    }

    @Test
    fun w2_capturaEnLineaRotacionYEnvioACocina() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table1"))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table1"))
        val itemId = capture.visible.single().id
        // Rotación: lo capturado sigue a la vista (y guardado en Room y en Go).
        log("girar")
        rotate(UiAutomation.ROTATION_FREEZE_90)
        showOrder()
        waitFor("$TAG_DRAFT_ROW_PREFIX$itemId")
        log("girada; volver")
        rotate(UiAutomation.ROTATION_FREEZE_0)
        showOrder()
        waitFor("$TAG_DRAFT_ROW_PREFIX$itemId")
        log("enviar a cocina")
        tap(TAG_SEND_TO_KITCHEN)
        log("enviado")
        waitForTables()
        val order = waitForOrder(ids.getValue("table1")) { it.state == OrderState.RECEIVED }
        val accountId = order.receivedAccountId!!
        // En Go: una partida confirmada en la cuenta nueva de la mesa.
        val bill = main.call("GET", "restaurant/accounts/$accountId").obj
        assertEquals(1, bill.getValue("lines").jsonArray.size)
        assertEquals(0, bill.getValue("draft").jsonArray.size)
        openPending()
        waitFor("$TAG_ORDER_STATE_PREFIX${OrderState.RECEIVED.storedName}")
        saveIds(ids + ("account1" to accountId))

        // Otra terminal (la principal) guarda un renglón en la cuenta y lo confirma: la captura lo
        // ve al momento por el canal de tiempo real.
        compose.onNodeWithTag(TAG_PENDING).assertExists()
        pressBack()
        openTable(ids.getValue("table1"))
        waitFor("account-$accountId")
        tap("account-$accountId")
        showOrder()
        waitFor("$TAG_SENT_ROW_PREFIX${sentVersion(accountId, 0)}")
        val foreign = UUID.randomUUID().toString()
        val revision = main.call("GET", "restaurant/accounts/$accountId").obj.revision()
        val saved =
            main.call(
                "PUT",
                "restaurant/accounts/$accountId/draft",
                """{"operation_id":"${UUID.randomUUID()}","revision":$revision,""" +
                    """"items":[${draftItem(foreign, ids.getValue("tacos"), 1)}]}""",
            )
        assertEquals(saved.body.toString(), 200, saved.status)
        waitFor("$TAG_DRAFT_ROW_PREFIX$foreign")
        confirmFromMain(accountId)
        waitFor("$TAG_SENT_ROW_PREFIX${sentVersion(accountId, 1)}")
    }

    @Test
    fun w3_capturaSinConexion() {
        val ids = loadIds()
        launch()
        // Sin red: solo entra quien ya entró con conexión aquí (verificador local).
        enterAna()
        newAccountOn(ids.getValue("table2"))
        addProduct(ids.getValue("tacos"))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}pending")
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table2")) { it.state == OrderState.QUEUED }
        assertEquals(2, order.lines.single().item.line.quantity)
        assertTrue("Con la versión del catálogo", order.catalogVersion > 0)
        openPending()
        waitFor("$TAG_ORDER_STATE_PREFIX${OrderState.QUEUED.storedName}")
        waitForText("1 pedido por enviar")
    }

    @Test
    fun w4_alVolverLaRedSeEnviaSinEntrarYSinDuplicar() {
        val ids = loadIds()
        // Proceso nuevo y sin entrar con el PIN: el pedido sigue en la cola y WorkManager lo envía
        // con la credencial de envío de Ana. La primera respuesta se pierde (lostOfflineResponses).
        launch()
        val order =
            waitForOrder(ids.getValue("table2"), timeout = 120_000) {
                it.state == OrderState.RECEIVED
            }
        assertTrue("Hubo al menos un reintento", order.attempts >= 1)
        val accounts = openAccountsOn(ids.getValue("table2"))
        assertEquals("Una sola cuenta en la mesa", 1, accounts.size)
        val bill = main.call("GET", "restaurant/accounts/${accounts.single()}").obj
        val lines = bill.getValue("lines").jsonArray
        assertEquals("El pedido entró una sola vez", 1, lines.size)
        assertEquals(2, lines.single().jsonObject.getValue("quantity").jsonPrimitive.int)
    }

    @Test
    fun w5_precioCambiadoYAgotadoConLaConfirmacionDelMesero() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table3"))
        addProduct(ids.getValue("burger"))
        addProduct(ids.getValue("shake"))
        showOrder()
        waitUntil("el borrador guardado con su cotización") {
            val capture = currentCapture(ids.getValue("table3"))
            capture.accountId != null &&
                capture.operations.isEmpty() &&
                capture.seenPrices.size == 2
        }
        // Mientras tanto, en la principal: la hamburguesa sube y la malteada se agota.
        changePrice(ids.getValue("burger"), "130")
        soldOut(ids.getValue("shake"))
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val review = waitForOrder(ids.getValue("table3")) { it.state == OrderState.NEEDS_REVIEW }
        val shakeItem =
            review.lines.first { it.item.line.productId == ids.getValue("shake") }.item.id
        openPending()
        openReview(review.operationId)
        waitFor("${TAG_REVIEW_CHANGE_PREFIX}price_changed")
        waitFor("${TAG_REVIEW_CHANGE_PREFIX}product_sold_out")
        waitForText("$120.00 → $130.00")
        tap("$TAG_REVIEW_REMOVE_PREFIX$shakeItem")
        waitFor(TAG_REVIEW_ACCEPT)
        tap(TAG_REVIEW_ACCEPT)
        val received = waitForOrder(ids.getValue("table3")) { it.state == OrderState.RECEIVED }
        val bill = main.call("GET", "restaurant/accounts/${received.receivedAccountId}").obj
        val lines = bill.getValue("lines").jsonArray.map { it.jsonObject }
        assertEquals(1, lines.size)
        assertEquals(
            "130.00",
            lines
                .single()
                .getValue("quote_line")
                .jsonObject
                .getValue("unit_price")
                .jsonPrimitive
                .content,
        )
        // El borrador ya no tiene lo que el pedido sustituyó (tampoco la malteada que se quitó).
        assertEquals(0, bill.getValue("draft").jsonArray.size)
    }

    @Test
    fun w6_mesaCobradaMientrasTanto() {
        val ids = loadIds()
        val accountId = ids.getValue("account1")
        launch()
        enterAna()
        openTable(ids.getValue("table1"))
        waitFor("account-$accountId")
        tap("account-$accountId")
        showOrder()
        waitUntil("la cuenta leída de Go") {
            currentCapture(ids.getValue("table1")).baseRevision > 0
        }
        // Mientras el mesero tiene la cuenta abierta, la principal la cobra (Go solo cierra una
        // cuenta de salón liquidada sin borrador pendiente).
        log("cobrar")
        payAccount(accountId)
        backToCatalog()
        addProduct(ids.getValue("tacos"))
        showOrder()
        // La cuenta ya no está abierta (por el canal de tiempo real, o porque Go rechaza el
        // guardado con «…ya se cobró»): se ofrece una cuenta nueva en la mesa.
        waitFor("${TAG_SYNC_PREFIX}account_closed")
        tap(TAG_OPEN_ANOTHER)
        waitFor("${TAG_SYNC_PREFIX}saved")
        val moved = currentCapture(ids.getValue("table1"))
        assertTrue(moved.accountId != null && moved.accountId != accountId)
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val received =
            waitForOrder(ids.getValue("table1")) {
                it.operationId == moved.id && it.state == OrderState.RECEIVED
            }
        assertEquals(moved.accountId, received.receivedAccountId)
        val bill = main.call("GET", "restaurant/accounts/${received.receivedAccountId}").obj
        assertEquals(
            "open",
            bill.getValue("account").jsonObject.getValue("state").jsonPrimitive.content,
        )
        assertEquals(1, bill.getValue("lines").jsonArray.size)
        val paid = main.call("GET", "restaurant/accounts/$accountId").obj
        assertEquals(
            "closed",
            paid.getValue("account").jsonObject.getValue("state").jsonPrimitive.content,
        )
    }

    @Test
    fun w8_cancelarYModificarLoEnviado() {
        val ids = loadIds()
        val accountId = ids.getValue("account4")
        launch()
        enterAna()
        openTable(ids.getValue("table4"))
        waitFor("account-$accountId")
        tap("account-$accountId")
        showOrder()
        val lines =
            main.call("GET", "restaurant/accounts/$accountId").obj.getValue("lines").jsonArray
        val byProduct = lines.associate {
            val line = it.jsonObject
            line.getValue("request").jsonObject.text("product_id") to line.text("version_id")
        }
        val tacos = byProduct.getValue(ids.getValue("tacos"))
        val burger = byProduct.getValue(ids.getValue("burger"))
        // Cancelar la unidad de tacos, con motivo (sin supervisor).
        waitFor("$TAG_SENT_CANCEL_PREFIX$tacos")
        tap("$TAG_SENT_CANCEL_PREFIX$tacos")
        compose
            .onNodeWithTag(TAG_SENT_REASON)
            .performTextReplacement("El cliente cambió de opinión")
        compose.onNodeWithTag(TAG_CANCEL_CONFIRM).performClick()
        waitUntil("la cancelación en Go") { lineState(accountId, tacos) == "cancelled" }
        // Etapa M1 (§5.7): al cerrar el diálogo el teclado no se queda abierto (Android 8).
        waitForKeyboardClosed("cancelar")
        // Modificar la hamburguesa: observaciones nuevas, calculadas primero con Go.
        waitFor("$TAG_SENT_MODIFY_PREFIX$burger")
        tap("$TAG_SENT_MODIFY_PREFIX$burger")
        compose.onNodeWithTag(TAG_MODIFY_NOTES).performTextReplacement("sin cebolla")
        compose.onNodeWithTag(TAG_MODIFY_PREVIEW).performClick()
        waitFor(TAG_MODIFY_QUOTE)
        compose.onNodeWithTag(TAG_SENT_REASON).performTextReplacement("Lo pidió el cliente")
        compose.onNodeWithTag(TAG_MODIFY_CONFIRM).performClick()
        waitUntil("la modificación en Go") {
            val current =
                main.call("GET", "restaurant/accounts/$accountId").obj.getValue("lines").jsonArray
            current.any { line ->
                val request = line.jsonObject.getValue("request").jsonObject
                request.text("product_id") == ids.getValue("burger") &&
                    request.getValue("customization").jsonObject.text("notes") == "sin cebolla"
            }
        }
        waitForKeyboardClosed("modificar")
    }

    /**
     * Etapa M1 (§5.7): después de cerrar el diálogo, el teclado se cierra. Si no, deja la pantalla
     * (`w8-teclado-<qué>.png` en la carpeta de capturas) y lo que dice Android, para saber qué
     * pasó.
     */
    private fun waitForKeyboardClosed(what: String) {
        // Primero se espera a que el diálogo se cierre (lo cierra la respuesta de Go).
        val start = System.currentTimeMillis()
        waitUntil("el diálogo cerrado después de $what", 60_000) { !present(TAG_SENT_REASON) }
        Log.i(
            "AdelOnGo",
            "w8 · el diálogo de $what se cerró a los ${System.currentTimeMillis() - start} ms",
        )
        try {
            waitUntil("el teclado cerrado después de $what", 10_000) { !keyboardOpen() }
        } catch (failure: Throwable) {
            val bitmap = automation.takeScreenshot()
            if (bitmap != null) {
                val folder =
                    java.io.File(TestDevice.app.getExternalFilesDir(null), "screens").apply {
                        mkdirs()
                    }
                java.io.FileOutputStream(java.io.File(folder, "w8-teclado-$what.png")).use {
                    bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
                }
            }
            scenario?.onActivity { activity ->
                val input =
                    activity.getSystemService(
                        android.view.inputmethod.InputMethodManager::class.java,
                    )
                val insets =
                    androidx.core.view.ViewCompat.getRootWindowInsets(activity.window.decorView)
                Log.i(
                    "AdelOnGo",
                    "w8 · teclado después de $what: activo ${input?.isActive}, " +
                        "aceptando ${input?.isAcceptingText}, " +
                        "${insets?.getInsets(androidx.core.view.WindowInsetsCompat.Type.ime())}, " +
                        "foco ${activity.currentFocus}",
                )
            }
            throw failure
        }
    }

    /** El teclado de Android está abierto sobre la actividad (sus `WindowInsets`). */
    private fun keyboardOpen(): Boolean {
        var open = false
        scenario?.onActivity { activity ->
            open =
                androidx.core.view.ViewCompat.getRootWindowInsets(activity.window.decorView)
                    ?.isVisible(androidx.core.view.WindowInsetsCompat.Type.ime()) == true
        }
        return open
    }

    private fun lineState(accountId: String, versionId: String): String? {
        val lines =
            main.call("GET", "restaurant/accounts/$accountId").obj.getValue("lines").jsonArray
        return lines
            .map { it.jsonObject }
            .firstOrNull { it.text("version_id") == versionId }
            ?.text("state")
    }

    @Test
    fun w7a_borradorGuardadoEnGo() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table4"))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table4"))
        val bill = main.call("GET", "restaurant/accounts/${capture.accountId}").obj
        assertEquals(1, bill.getValue("draft").jsonArray.size)
        saveIds(ids + ("account4" to capture.accountId!!))
    }

    @Test
    fun w7b_sinConexionSeAgregaYSeEnvia() {
        val ids = loadIds()
        launch()
        enterAna()
        // La captura de la mesa 4 sigue en el dispositivo (Room) con su renglón ya guardado en Go.
        openTable(ids.getValue("table4"))
        val capture = currentCapture(ids.getValue("table4"))
        waitFor("table-capture-${capture.id}")
        tap("table-capture-${capture.id}")
        addProduct(ids.getValue("tacos"))
        showOrder()
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table4")) { it.state == OrderState.QUEUED }
        // Sustituye el renglón que ya estaba en el borrador de Go, tal como lo vio.
        assertEquals(1, order.replaces.size)
        assertEquals(2, order.lines.size)
    }

    @Test
    fun w7c_elPedidoSustituyeElBorradorSinDuplicar() {
        val ids = loadIds()
        launch()
        waitForOrder(ids.getValue("table4"), timeout = 120_000) { it.state == OrderState.RECEIVED }
        val bill = main.call("GET", "restaurant/accounts/${ids.getValue("account4")}").obj
        assertEquals(2, bill.getValue("lines").jsonArray.size)
        assertEquals(
            "Sin renglones duplicados en el borrador",
            0,
            bill.getValue("draft").jsonArray.size,
        )
    }

    // --- Revisión R6 (A1-D19) ---

    @Test
    fun w9_guardadoSinRespuestaAntesDeEnviar() {
        val ids = loadIds()
        // Una cuenta de la mesa 5 abierta en la principal, con algo ya enviado (la malteada está
        // agotada desde w5).
        val accountId = openFromMain(ids.getValue("table5"), ids.getValue("tacos"))
        confirmFromMain(accountId)
        launch()
        enterAna()
        openTable(ids.getValue("table5"))
        tap("account-$accountId")
        showOrder()
        waitUntil("la cuenta leída de Go") {
            currentCapture(ids.getValue("table5")).baseRevision > 0
        }
        // La respuesta del guardado se pierde (Go sí lo guardó) y el mesero envía enseguida.
        addProduct(ids.getValue("burger"))
        showOrder()
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        waitForOrder(ids.getValue("table5"), timeout = 120_000) { it.state == OrderState.RECEIVED }
        val bill = main.call("GET", "restaurant/accounts/$accountId").obj
        val products =
            bill.getValue("lines").jsonArray.map {
                it.jsonObject.getValue("request").jsonObject.text("product_id")
            }
        assertEquals(1, products.count { it == ids.getValue("burger") })
        assertEquals(2, products.size)
        assertEquals("Sin borrador repetido", 0, bill.getValue("draft").jsonArray.size)
    }

    @Test
    fun w10_confirmacionAplicadaSinRespuestaNoDuplica() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table6"))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table6"))
        // Go confirma y las dos respuestas se pierden: sale como pedido sin conexión con la misma
        // clave y Go lo reconoce como ya aceptado.
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val received =
            waitForOrder(ids.getValue("table6"), timeout = 120_000) {
                it.operationId == capture.id && it.state == OrderState.RECEIVED
            }
        assertEquals(capture.accountId, received.receivedAccountId)
        val bill = main.call("GET", "restaurant/accounts/${capture.accountId}").obj
        assertEquals("Entró una sola vez", 1, bill.getValue("lines").jsonArray.size)
        assertEquals(0, bill.getValue("draft").jsonArray.size)
    }

    @Test
    fun w11_descartarConConexionQuitaDeLaCuenta() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table7"))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table7"))
        assertTrue(capture.createdAccount)
        tap(TAG_DISCARD_CAPTURE)
        tap(TAG_DISCARD_CONFIRM)
        waitForTables()
        assertEquals("voided", accountState(capture.accountId!!))
        assertTrue(captureGone(capture.id))
    }

    @Test
    fun w12a_capturaGuardadaParaDescartar() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table8"))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table8"))
        saveIds(ids + ("account8" to capture.accountId!!) + ("capture8" to capture.id))
    }

    @Test
    fun w12b_descartarSinConexionQuedaPendiente() {
        val ids = loadIds()
        launch()
        enterAna()
        openTable(ids.getValue("table8"))
        tap("table-capture-${ids.getValue("capture8")}")
        showOrder()
        tap(TAG_DISCARD_CAPTURE)
        tap(TAG_DISCARD_CONFIRM)
        // Se dice la verdad: lo guardado en la cuenta se quita en cuanto haya conexión.
        tap(TAG_DISCARD_PENDING_OK)
        waitForTables()
        val pending = runBlocking { container.database.waiter().capture(ids.getValue("capture8")) }
        assertEquals(DraftSync.DISCARDING.name, pending!!.sync)
    }

    @Test
    fun w12c_alVolverSeQuitaDeLaCuenta() {
        val ids = loadIds()
        launch()
        enterAna()
        waitUntil("el descarte terminado", timeout = 120_000) {
            captureGone(ids.getValue("capture8"))
        }
        assertEquals("voided", accountState(ids.getValue("account8")))
    }

    @Test
    fun w13a_capturaEnLaCuentaQueSeCombinara() {
        val ids = loadIds()
        launch()
        enterAna()
        newAccountOn(ids.getValue("table9"))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ids.getValue("table9"))
        // El mesero sale de la captura y, sin que él lo sepa, la principal combina la cuenta en
        // otra de la mesa 10 (su borrador se mueve con sus identificadores). Se hace aquí y no en
        // w13c: al arrancar el proceso, el trabajador envía la cola antes de que la prueba empiece.
        backToTables()
        val target = openFromMain(ids.getValue("table10"), ids.getValue("tacos"))
        confirmFromMain(target)
        mergeFromMain(capture.accountId!!, target)
        saveIds(ids + ("account9" to capture.accountId!!) + ("target9" to target))
    }

    @Test
    fun w13b_sinConexionSeEnviaALaCuenta() {
        val ids = loadIds()
        launch()
        enterAna()
        openTable(ids.getValue("table9"))
        val capture = currentCapture(ids.getValue("table9"))
        tap("table-capture-${capture.id}")
        addProduct(ids.getValue("tacos"))
        showOrder()
        tap(TAG_SEND_TO_KITCHEN)
        waitForTables()
        val order = waitForOrder(ids.getValue("table9")) { it.state == OrderState.QUEUED }
        assertEquals(ids.getValue("account9"), order.destination.accountId)
        assertEquals(1, order.replaces.size)
    }

    @Test
    fun w13c_cuentaCombinadaSeEnviaALaCuentaEnQueSeCombino() {
        val ids = loadIds()
        val target = ids.getValue("target9")
        launch()
        enterAna()
        val review =
            waitForOrder(ids.getValue("table9"), timeout = 120_000) {
                it.state == OrderState.NEEDS_REVIEW
            }
        openPending()
        openReview(review.operationId)
        tap(TAG_REVIEW_SEND_TO_MERGED)
        val received =
            waitForOrder(ids.getValue("table9"), timeout = 120_000) {
                it.operationId == review.operationId && it.state == OrderState.RECEIVED
            }
        assertEquals(target, received.receivedAccountId)
        val bill = main.call("GET", "restaurant/accounts/$target").obj
        val products =
            bill.getValue("lines").jsonArray.map {
                it.jsonObject.getValue("request").jsonObject.text("product_id")
            }
        // Lo de la principal (tacos) y el pedido del mesero (hamburguesa y tacos), una vez cada
        // uno.
        assertEquals(1, products.count { it == ids.getValue("burger") })
        assertEquals(2, products.count { it == ids.getValue("tacos") })
        assertEquals(3, products.size)
        assertEquals("Sin borrador repetido", 0, bill.getValue("draft").jsonArray.size)
    }

    private fun accountState(accountId: String): String =
        main
            .call("GET", "restaurant/accounts/$accountId")
            .obj
            .getValue("account")
            .jsonObject
            .text("state")

    private fun captureGone(captureId: String): Boolean = runBlocking {
        container.database.waiter().capture(captureId) == null
    }

    // --- Pantallas ---

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    /**
     * Espera a que exista un nodo con [tag]. Lo de una lista perezosa solo existe cuando está
     * compuesto (en una pantalla chica, el botón «Enviar a cocina» puede quedar abajo): cada
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
        compose.waitUntil(timeoutMillis = timeout) {
            compose
                .onAllNodes(hasText(text, substring = true), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }

    private fun waitUntil(what: String, timeout: Long = 40_000, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (!condition()) {
            check(System.currentTimeMillis() < deadline) { "No llegó: $what" }
            Thread.sleep(POLL_MILLIS)
        }
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

    private fun newAccountOn(tableId: String) {
        showTables()
        waitFor("$TAG_TABLE_PREFIX$tableId")
        tap("$TAG_TABLE_PREFIX$tableId")
        waitFor(TAG_NEW_ACCOUNT)
        tap(TAG_NEW_ACCOUNT)
        waitFor(productTag(loadIds().getValue("burger")))
    }

    private fun addProduct(productId: String) {
        backToCatalog()
        waitFor(productTag(productId))
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
    }

    /** El botón Atrás del sistema (la actividad vigente, también después de girar). */
    private fun pressBack() {
        // Un diálogo del sistema «… no responde» (emulador cargado) se quedaría con la tecla.
        SystemDialogs.dismissNotResponding(automation)
        InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        Thread.sleep(SETTLE_MILLIS)
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    /**
     * Gira la pantalla y espera a que la actividad nueva tenga la orientación pedida (la actividad
     * se vuelve a crear; lo capturado debe seguir).
     */
    private fun rotate(rotation: Int) {
        val before = TestDevice.app.resources.configuration.orientation
        automation.setRotation(rotation)
        val deadline = System.currentTimeMillis() + ROTATION_TIMEOUT_MILLIS
        while (System.currentTimeMillis() < deadline) {
            val now = TestDevice.app.resources.configuration.orientation
            if (now != before) {
                break
            }
            Thread.sleep(POLL_MILLIS)
        }
        Thread.sleep(SETTLE_MILLIS)
    }

    /**
     * En el teléfono, Mesas muestra la lista o la mesa elegida (a un toque): vuelve a la lista si
     * está abierta la mesa.
     */
    /** Sale de la captura con Atrás hasta Mesas (la captura con algo guardado se conserva). */
    private fun backToTables() {
        repeat(3) {
            if (!present(TAG_TABLES_SCREEN) && !present(TAG_ACCOUNT_PANE)) {
                pressBack()
            }
        }
        waitForTables()
    }

    private fun showTables() {
        waitForTables()
        // Un panel con la mesa abierta tiene «Volver a Mesas».
        if (present(TAG_ACCOUNT_PANE) && present(TAG_PANE_BACK)) {
            pressBack()
        }
    }

    /** Mesas: la lista o, en el teléfono, la mesa elegida a un toque. */
    private fun waitForTables() {
        waitUntil("Mesas") { present(TAG_TABLES_SCREEN) || present(TAG_ACCOUNT_PANE) }
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

    /**
     * Toca el nodo con [tag], desplazando hasta él si está en una lista (un botón de un diálogo no
     * tiene contenedor que desplazar).
     */
    private fun tap(tag: String) {
        waitFor(tag)
        // Compuesto pero fuera de la vista (la lista perezosa lo precarga): performScrollTo no
        // desplaza una lista perezosa, así que se le pide a la lista que lo traiga.
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

    private fun openTable(tableId: String) {
        showTables()
        waitFor("$TAG_TABLE_PREFIX$tableId")
        tap("$TAG_TABLE_PREFIX$tableId")
    }

    /** En el teléfono, la cuenta está a un toque; en la tableta, siempre a la vista. */
    private fun showOrder() {
        // Se espera a la cuenta o a su botón «Ver la cuenta» antes de decidir: si la captura
        // todavía
        // no terminó de dibujarse, el botón no está y no se tocaba nada (falla vista en `w6`).
        waitUntil("la cuenta o «Ver la cuenta»") {
            present(TAG_ORDER_PANEL) || present(TAG_PANE_OPEN_SECONDARY)
        }
        if (!present(TAG_ORDER_PANEL)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    /** En el teléfono, de la cuenta al catálogo («Volver a Catálogo»). */
    private fun backToCatalog() {
        if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
            pressBack()
        }
    }

    // El renglón del pedido que requiere revisión la abre al tocarlo.
    private fun openReview(operationId: String) {
        waitFor("$TAG_PENDING_REVIEW_PREFIX$operationId")
        tap("$TAG_PENDING_ORDER_PREFIX$operationId")
        waitFor(TAG_REVIEW)
    }

    private fun openPending() {
        showTables()
        waitFor(TAG_OPEN_PENDING)
        compose.onNodeWithTag(TAG_OPEN_PENDING).performClick()
        waitFor(TAG_PENDING)
    }

    // --- Lo guardado en el dispositivo ---

    /** La captura más reciente de Ana en la mesa (guardada en Room). */
    private fun currentCapture(tableId: String) = runBlocking {
        val ana = loadIds().getValue("ana")
        container.database
            .waiter()
            .capturesOf(ana)
            .map(WaiterStorage::toCapture)
            .filter { it.tableId == tableId }
            .maxBy { it.createdAtMillis }
    }

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
        Log.i(TAG, "pedido de la mesa: ${found!!.state} intentos ${found!!.attempts}")
        return found!!
    }

    // --- La computadora principal ---

    private fun grantWaiterPermissions() {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.getValue("name").jsonPrimitive.content == ROLE }
        val permissions = WAITER_PERMISSIONS.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.getValue("id").jsonPrimitive.content}",
                """{"name":"$ROLE","permissions":[$permissions]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun createCatalog(): Map<String, String> {
        val category =
            main.call(
                "POST",
                "catalog/categories",
                """{"name":"Comida E","active":true,"color":"#d64545"}""",
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
            "burger" to product("Hamburguesa", "E-HAM", "120"),
            "shake" to product("Malteada de fresa", "E-MAL", "65"),
            "tacos" to product("Tacos al pastor", "E-TAC", "90"),
        )
    }

    private fun createTables(): Map<String, String> {
        val area =
            main.call(
                "POST",
                "restaurant/areas",
                """{"operation_id":"${UUID.randomUUID()}","name":"Salón E","active":true}""",
            )
        assertEquals(area.body.toString(), 200, area.status)
        val areaId = area.obj.text("id")
        val ids = mutableMapOf<String, String>()
        for (number in 1..TABLES) {
            // Dos filas de cinco dentro del lienzo de 1000 × 560.
            val x = 20 + ((number - 1) % 5) * 180
            val y = 40 + ((number - 1) / 5) * 150
            val table =
                main.call(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$areaId",""" +
                        """"name":"Mesa E$number","shape":"rectangle","x":$x,""" +
                        """"y":$y,"width":100,"height":72,"seats":4,"active":true}""",
                )
            assertEquals(table.body.toString(), 200, table.status)
            ids["table$number"] = table.obj.text("id")
        }
        return ids
    }

    private fun changePrice(productId: String, price: String) {
        val current = main.call("GET", "catalog/products/$productId").obj
        val changed = JsonObject(current + ("base_price" to JsonPrimitive(price)))
        val answer = main.call("PATCH", "catalog/products/$productId", changed.toString())
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun soldOut(productId: String) {
        val answer =
            main.call(
                "POST",
                "catalog/products/$productId/sold-out",
                """{"operation_id":"${UUID.randomUUID()}","sold_out":true,"indefinite":true}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    /** «Enviar a cocina» desde la principal (vista previa con su huella y confirmación). */
    private fun confirmFromMain(accountId: String) {
        val preview = main.call("GET", "restaurant/accounts/$accountId/preview").obj
        val answer =
            main.call(
                "POST",
                "restaurant/accounts/$accountId/confirm",
                """{"operation_id":"${UUID.randomUUID()}","revision":${preview.getValue("revision")},""" +
                    """"expected_fingerprint":"${preview.text("fingerprint")}"}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    /**
     * Cobra la cuenta completa en efectivo desde la principal: abre el turno de su caja (si no lo
     * está) y registra el pago total con el saldo vigente.
     */
    private fun payAccount(accountId: String) {
        val methods = main.call("GET", "payments/methods").body.jsonArray.map { it.jsonObject }
        val cash = methods.first { (it["code"] as? JsonPrimitive)?.content == "cash" }.text("id")
        main.call(
            "POST",
            "payments/shifts",
            """{"operation_id":"${UUID.randomUUID()}","opening_float":"0"}""",
        )
        val balance = main.call("GET", "payments/accounts/$accountId").obj.text("balance")
        val paid =
            main.call(
                "POST",
                "payments/accounts/$accountId/payments",
                """{"operation_id":"${UUID.randomUUID()}","method_id":"$cash","mode":"total",""" +
                    """"received":"$balance","expected_balance":"$balance"}""",
            )
        assertEquals(paid.body.toString(), 200, paid.status)
        val state = main.call("GET", "restaurant/accounts/$accountId").obj
        assertEquals(
            "closed",
            state.getValue("account").jsonObject.getValue("state").jsonPrimitive.content,
        )
    }

    /** Abre una cuenta en la mesa desde la principal, con un producto en su borrador. */
    private fun openFromMain(tableId: String, productId: String): String {
        val answer =
            main.call(
                "POST",
                "restaurant/accounts",
                """{"operation_id":"${UUID.randomUUID()}","mode":"dine_in",""" +
                    """"table_id":"$tableId","seen_account_ids":${seenOn(tableId)},""" +
                    """"items":[${draftItem(UUID.randomUUID().toString(), productId, 1)}]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
        return answer.obj.getValue("account").jsonObject.text("id")
    }

    private fun seenOn(tableId: String): String =
        openAccountsOn(tableId).joinToString(",", "[", "]") { "\"$it\"" }

    /** Combina [sourceId] en [targetId] desde la principal (el borrador se mueve con sus ids). */
    private fun mergeFromMain(sourceId: String, targetId: String) {
        val source = main.call("GET", "restaurant/accounts/$sourceId").obj.revision()
        val target = main.call("GET", "restaurant/accounts/$targetId").obj.revision()
        val answer =
            main.call(
                "POST",
                "restaurant/accounts/$sourceId/merge",
                """{"operation_id":"${UUID.randomUUID()}","revision":$source,""" +
                    """"target_account_id":"$targetId","target_revision":$target}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun openAccountsOn(tableId: String): List<String> {
        val listed = main.call("GET", "restaurant/accounts?table_id=$tableId&state=open").obj
        return listed.getValue("items").jsonArray.map { it.jsonObject.text("id") }
    }

    private fun sentVersion(accountId: String, index: Int): String {
        val lines =
            main.call("GET", "restaurant/accounts/$accountId").obj.getValue("lines").jsonArray
        return lines[index].jsonObject.text("version_id")
    }

    private fun draftItem(id: String, productId: String, quantity: Int): String =
        """{"id":"$id","product_id":"$productId","variant_id":null,"quantity":$quantity,""" +
            """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}"""

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    private fun JsonObject.revision(): Int =
        getValue("account").jsonObject.getValue("revision").jsonPrimitive.int

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
        const val ANA_USER = "ana.e1"
        const val ANA_PIN = "4829"
        const val ROLE = "Meseros A1"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val ROTATION_TIMEOUT_MILLIS = 10_000L
        const val REVEAL_EVERY_MILLIS = 1_000L

        /** Mesas del restaurante de prueba (una o dos por paso). */
        const val TABLES = 10

        /** Las listas perezosas del modo Mesero en las que se busca desplazando. */
        val LAZY_LISTS =
            listOf(
                TAG_ORDER_PANEL,
                TAG_ACCOUNT_PANE,
                TAG_TABLES_LIST,
                TAG_PENDING,
                TAG_REVIEW,
                TAG_PRODUCT_GRID,
            )

        /** La lista de permisos de mesero de Go (A1-D17), la misma que da la sesión con PIN. */
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
            )
    }
}
