package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import android.graphics.Bitmap
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isRoot
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
import com.adelpos.meseros.core.capture.TAG_CAPTURE
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_NAME
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_PHONE
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_SAVE
import com.adelpos.meseros.core.capture.TAG_ORDER_CUSTOMER_EDIT
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
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
import com.adelpos.meseros.core.ui.waiter.TAG_TAKEAWAY_PAYMENT_PREFIX
import com.adelpos.meseros.core.ui.waiter.TAG_TAKEAWAY_PREPARATION_PREFIX
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_COLLECT_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_DELIVER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_EDIT_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_EMPTY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_ENTRY_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_NEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.navigation.TAG_MAIN_NAVIGATION
import com.adelpos.meseros.navigation.TAG_NAVIGATION_RAIL
import com.adelpos.meseros.navigation.TAG_NAV_TAKEAWAY
import java.io.File
import java.io.FileOutputStream
import java.util.UUID
import kotlinx.coroutines.launch
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
 * Para llevar (etapa A2, §3) contra el **Go temporal** del arnés, nunca el servicio real: la prueba
 * hace de computadora principal por la escucha local ([MainComputer]): de cocina (estación y ruta,
 * «En preparación» y «Listo» por la API de cocina, como `FieldOnGoTest`) y de caja (cobra por la
 * API de pagos). Un paso por instrumentación, en orden, con la aplicación instalada una sola vez
 * (`scripts/android-e2e.sh --suite takeaway`); **la red la corta y la devuelve el guion** (en
 * `t4`).
 *
 * - `t1` prepara el restaurante (rol de meseros con `takeaway.*`, catálogo con su cocina), empareja
 *   y entra: la navegación principal muestra «Mesas» y «Para llevar».
 * - `t2` «Nuevo pedido» sin productos (aunque se escriba el nombre) no deja nada ni en Go ni en el
 *   dispositivo.
 * - `t3` captura y envío **en línea**: el pedido nace con el primer producto con su nombre y
 *   teléfono (`openAccount` para llevar) y «Enviar a cocina» lo deja «Recibido».
 * - `t4` (SIN red) la lista muestra lo último que se vio, con su hora; un pedido nuevo nace al
 *   enviarse («Por enviar»), y el nombre y el teléfono se editan sin conexión (de un pedido que ya
 *   está en Go y del que sigue en la cola).
 * - `t5` (con red) al entrar con conexión sale el pedido (`destination.takeaway`) y después las
 *   ediciones del cliente, la última de cada una.
 * - `t6` nombre y teléfono después, en línea, desde la lista.
 * - `t7` la lista al momento con la preparación de cocina («En preparación», «Listo») y el pago de
 *   la caja («Pagado»).
 * - `t8` sin pagar, «Cobrar en caja antes de entregar» y Go lo rechaza (`BALANCE_PENDING`); pagado,
 *   «Entregado», y un pedido entregado y pagado sale de la lista.
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice` y `mainToken` (los arma el guion). Sin ellos
 * se omite.
 */
@RunWith(AndroidJUnit4::class)
class TakeawayOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "takeaway-on-go.json")

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
    fun t1_prepararEmparejarYEntrar() {
        val ana = main.ensureWaiters(mapOf(ANA_USER to ("Ana" to ANA_PIN))).getValue(ANA_USER)
        grantWaiterPermissions()
        val ids = mutableMapOf("ana" to ana)
        ids.putAll(createCatalogWithKitchen())
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        enterAna()
        waitUntil("el catálogo descargado") {
            container.catalogRealtime.catalog.catalog.value.product(ids.getValue("burger")) != null
        }
        // §3.1: con los permisos de Go para llevar, la navegación principal tiene las dos.
        waitFor(TAG_MAIN_NAVIGATION)
        openTakeaway()
        waitFor(TAG_TAKEAWAY_NEW)
        assertEquals(0, openTakeawayInGo().size)
    }

    @Test
    fun t2_nuevoPedidoSinProductosNoDejaNada() {
        launch()
        enterAna()
        openTakeaway()
        tap(TAG_TAKEAWAY_NEW)
        waitFor(productTag(loadIds().getValue("burger")))
        // El nombre se puede escribir antes del primer producto…
        editCustomerInCapture(name = "Nadie", phone = "")
        waitUntil("el nombre en la captura") {
            takeawayCaptures().singleOrNull()?.customer?.name == "Nadie"
        }
        // …y salir sin productos no deja nada.
        leaveCapture()
        waitUntil("sin capturas en el dispositivo") { takeawayCaptures().isEmpty() }
        waitFor(TAG_TAKEAWAY_EMPTY)
        assertEquals(0, openTakeawayInGo().size)
        assertTrue(runBlocking { container.database.waiter().customerEdits().isEmpty() })
    }

    @Test
    fun t3_capturaYEnvioEnLinea() {
        val ids = loadIds()
        launch()
        enterAna()
        openTakeaway()
        tap(TAG_TAKEAWAY_NEW)
        waitFor(productTag(ids.getValue("burger")))
        editCustomerInCapture(name = "Juan", phone = "5551234")
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        // Nació en Go con el primer producto, para llevar, con su nombre y teléfono.
        val capture = takeawayCaptures().single()
        val accountId = capture.accountId!!
        val account = account(accountId)
        assertEquals("takeaway", account.text("mode"))
        assertEquals("Para llevar", account.text("origin"))
        assertEquals("Juan", account.text("customer_name"))
        assertEquals("5551234", account.text("phone"))
        tap(TAG_SEND_TO_KITCHEN)
        waitFor(TAG_TAKEAWAY)
        val order = waitForOrder { it.state == OrderState.RECEIVED }
        assertEquals(accountId, order.receivedAccountId)
        val bill = main.call("GET", "restaurant/accounts/$accountId").obj
        assertEquals(1, bill.getValue("lines").jsonArray.size)
        assertEquals(0, bill.getValue("draft").jsonArray.size)
        // En la lista, con su folio y el cliente.
        waitFor("$TAG_TAKEAWAY_ENTRY_PREFIX$accountId")
        waitForText("Juan · 5551234")
        saveIds(ids + ("online" to accountId))
    }

    @Test
    fun t4_sinConexionNaceAlEnviarseYElClienteSeEditaSinConexion() {
        val ids = loadIds()
        val online = ids.getValue("online")
        launch()
        enterAna()
        openTakeaway()
        // Lo último que se vio, con su hora (§3.3).
        waitForText("Sin conexión: es lo último que se vio")
        waitFor("$TAG_TAKEAWAY_ENTRY_PREFIX$online")

        // Nombre y teléfono de un pedido que ya está en Go, sin conexión: se guarda aquí.
        tap("$TAG_TAKEAWAY_EDIT_PREFIX$online")
        fillCustomer(name = "Juan Pérez", phone = "5551234")
        waitUntil("la edición guardada en el dispositivo") {
            customerEdits().any { it.accountId == online && it.customerName == "Juan Pérez" }
        }

        // Un pedido nuevo sin conexión: nace al enviarse.
        tap(TAG_TAKEAWAY_NEW)
        waitFor(productTag(ids.getValue("tacos")))
        editCustomerInCapture(name = "Luisa", phone = "")
        addProduct(ids.getValue("tacos"))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}pending")
        tap(TAG_SEND_TO_KITCHEN)
        waitFor(TAG_TAKEAWAY)
        val order = waitForOrder { it.state == OrderState.QUEUED }
        assertTrue(order.destination.takeaway)
        assertEquals("Luisa", order.destination.customer.name)
        assertEquals(2, order.lines.single().item.line.quantity)
        waitFor("$TAG_TAKEAWAY_ENTRY_PREFIX${order.operationId}")
        waitFor("$TAG_ORDER_STATE_PREFIX${OrderState.QUEUED.storedName}")

        // Su nombre, editado mientras sigue en la cola.
        tap("$TAG_TAKEAWAY_EDIT_PREFIX${order.operationId}")
        fillCustomer(name = "Luisa M.", phone = "5559876")
        waitUntil("la edición del pedido en la cola") {
            customerEdits().any { it.key == order.operationId && it.customerName == "Luisa M." }
        }
        waitForText("Luisa M. · 5559876")
        saveIds(ids + ("queued" to order.operationId))
    }

    @Test
    fun t5_alVolverLaRedSaleElPedidoYDespuesElCliente() {
        val ids = loadIds()
        val online = ids.getValue("online")
        launch()
        enterAna()
        val order =
            waitForOrder(timeout = 90_000) {
                it.operationId == ids.getValue("queued") && it.state == OrderState.RECEIVED
            }
        val accountId = order.receivedAccountId!!
        // Las ediciones salen con la sesión de Ana en cuanto entra con conexión (la última gana).
        waitUntil("las ediciones enviadas", 90_000) { customerEdits().isEmpty() }
        val created = account(accountId)
        assertEquals("takeaway", created.text("mode"))
        assertEquals("Para llevar", created.text("origin"))
        assertEquals("Luisa M.", created.text("customer_name"))
        assertEquals("5559876", created.text("phone"))
        assertEquals("Juan Pérez", account(online).text("customer_name"))
        val lines = main.call("GET", "restaurant/accounts/$accountId").obj.getValue("lines")
        assertEquals(1, lines.jsonArray.size)
        openTakeaway()
        waitFor("$TAG_TAKEAWAY_ENTRY_PREFIX$accountId")
        waitForText("Luisa M. · 5559876")
        waitForText("Juan Pérez · 5551234")
        saveIds(ids + ("offline" to accountId))
    }

    @Test
    fun t6_nombreYTelefonoDespuesEnLinea() {
        val ids = loadIds()
        val offline = ids.getValue("offline")
        launch()
        enterAna()
        openTakeaway()
        // Etapa M1 (§5.7): lo que pasa en el dispositivo queda en el registro con su hora (el
        // canal de eventos, las lecturas de Go y lo que dice la lista del pedido), para saber por
        // qué en Android 8 la lista no se ponía al día.
        val watch = watchTakeaway(offline)
        try {
            tap("$TAG_TAKEAWAY_EDIT_PREFIX$offline")
            fillCustomer(name = "Luisa Martínez", phone = "")
            waitUntil("el nombre en Go") {
                account(offline).text("customer_name") == "Luisa Martínez" &&
                    account(offline).text("phone") == "" &&
                    customerEdits().isEmpty()
            }
            log("t6 · el nombre ya está en Go y no queda la edición en el dispositivo")
            try {
                waitForText("Luisa Martínez")
            } catch (failure: Throwable) {
                // Lo que se ve en ese momento: la pantalla (¿el teclado?), el método de entrada y
                // cada ventana con sus nodos.
                screenshot("t6-falla")
                val input =
                    TestDevice.app.getSystemService(
                        android.view.inputmethod.InputMethodManager::class.java,
                    )
                log(
                    "t6 · método de entrada activo: ${input?.isActive}, aceptando texto: ${input?.isAcceptingText}",
                )
                scenario?.onActivity { activity ->
                    val insets =
                        androidx.core.view.ViewCompat.getRootWindowInsets(activity.window.decorView)
                    log(
                        "t6 · teclado según Android: " +
                            "${insets?.isVisible(androidx.core.view.WindowInsetsCompat.Type.ime())} " +
                            "${insets?.getInsets(androidx.core.view.WindowInsetsCompat.Type.ime())}",
                    )
                }
                compose
                    .onAllNodes(isRoot(), useUnmergedTree = true)
                    .fetchSemanticsNodes()
                    .forEach { root -> log("t6 · ventana ${root.id} ${root.boundsInRoot}") }
                val roots = compose.onAllNodes(isRoot(), useUnmergedTree = true)
                for (index in roots.fetchSemanticsNodes().indices) {
                    runCatching { roots[index].printToLog(TAG, maxDepth = Int.MAX_VALUE) }
                }
                throw failure
            }
        } finally {
            watch.cancel()
        }
    }

    /** Registra en `AdelOnGo` el canal de eventos y la lectura de [accountId] mientras corre t6. */
    private fun watchTakeaway(accountId: String): kotlinx.coroutines.Job {
        val realtime = container.catalogRealtime.realtime
        val restaurant = container.waiterMode.restaurant
        val scope = kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.Dispatchers.Default)
        val start = android.os.SystemClock.elapsedRealtime()
        fun at() = "+${android.os.SystemClock.elapsedRealtime() - start} ms"
        val job = kotlinx.coroutines.SupervisorJob()
        scope.launch(job) { realtime.state.collect { log("t6 · ${at()} canal: $it") } }
        scope.launch(job) {
            realtime.signals.collect { signal ->
                when (signal) {
                    is com.adelpos.core.model.realtime.RealtimeSignal.Changes ->
                        signal.events.forEach { event ->
                            log(
                                "t6 · ${at()} evento ${event.scope} ${event.kind} ${event.aggregateId}",
                            )
                        }
                    else -> log("t6 · ${at()} señal $signal")
                }
            }
        }
        scope.launch(job) {
            restaurant.accountsReadAt.collect { log("t6 · ${at()} lectura de Go: $it") }
        }
        scope.launch(job) {
            restaurant.allOpenAccounts.collect { accounts ->
                val name = accounts.firstOrNull { it.id == accountId }?.customer?.name
                log("t6 · ${at()} la lectura dice: $name")
            }
        }
        scope.launch(job) {
            container.database.waiter().observeCustomerEdits().collect { edits ->
                log("t6 · ${at()} ediciones por enviar: ${edits.map { it.customerName }}")
            }
        }
        return job
    }

    @Test
    fun t7_listaConPreparacionYPagoAlMomento() {
        val ids = loadIds()
        val online = ids.getValue("online")
        launch()
        enterAna()
        openTakeaway()
        waitFor("$TAG_TAKEAWAY_ENTRY_PREFIX$online")
        waitFor("${TAG_TAKEAWAY_PREPARATION_PREFIX}pending")
        // La cocina (la principal) marca «En preparación» y «Listo»: la lista lo ve sin tocar nada.
        setKitchenState(kitchenTasks(online), "preparing")
        waitFor("${TAG_TAKEAWAY_PREPARATION_PREFIX}preparing")
        setKitchenState(kitchenTasks(online), "ready")
        waitFor("${TAG_TAKEAWAY_PREPARATION_PREFIX}ready")
        // La caja cobra: «Pagado» y ya se ofrece «Entregado».
        waitFor("$TAG_TAKEAWAY_COLLECT_PREFIX$online")
        payAccount(online)
        waitFor("${TAG_TAKEAWAY_PAYMENT_PREFIX}paid")
        waitFor("$TAG_TAKEAWAY_DELIVER_PREFIX$online")
        assertEquals("paid", account(online).text("payment_state"))
    }

    @Test
    fun t8_entregaBloqueadaSinPagoYPermitidaPagado() {
        val ids = loadIds()
        val online = ids.getValue("online")
        val offline = ids.getValue("offline")
        launch()
        enterAna()
        openTakeaway()
        // Sin pagar: nunca «Entregado»; «Cobrar en caja antes de entregar». Go también lo impide.
        waitFor("$TAG_TAKEAWAY_COLLECT_PREFIX$offline")
        assertTrue(!present("$TAG_TAKEAWAY_DELIVER_PREFIX$offline"))
        val refused = runBlocking {
            container.waiterMode.takeaway.deliver(ids.getValue("ana"), offline)
        }
        assertEquals(DeliveryResult.CollectFirst, refused)
        assertEquals("not_delivered", account(offline).text("delivery_state"))

        // Pagado: «Entregado», y entregado y pagado sale de la lista (Go lo cierra).
        tap("$TAG_TAKEAWAY_DELIVER_PREFIX$online")
        waitForText("Pedido entregado.")
        waitUntil("cerrado en Go") { account(online).text("state") == "closed" }
        waitUntil("fuera de la lista") { !present("$TAG_TAKEAWAY_ENTRY_PREFIX$online") }
        payAccount(offline)
        waitFor("$TAG_TAKEAWAY_DELIVER_PREFIX$offline")
        tap("$TAG_TAKEAWAY_DELIVER_PREFIX$offline")
        waitUntil("cerrado en Go") { account(offline).text("state") == "closed" }
        waitFor(TAG_TAKEAWAY_EMPTY)
        assertEquals(0, openTakeawayInGo().size)
    }

    /**
     * Hallazgo de la Fase E: en el teléfono horizontal la barra inferior dejaba la captura sin
     * espacio. Con poca altura o en horizontal la navegación va en un riel lateral y la captura
     * muestra los productos; en vertical, la barra inferior. Se guardan capturas de pantalla de las
     * dos orientaciones (el guion las copia a `<salida>/screens`).
     */
    @Test
    fun t9_navegacionLateralEnHorizontalYProductosALaVista() {
        val ids = loadIds()
        // La rotación natural: vertical en el teléfono, horizontal en la tableta.
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        Thread.sleep(SETTLE_MILLIS)
        val natural = orientationNow()
        launch()
        enterAna()
        for (landscape in listOf(true, false)) {
            val quarterTurn = landscape != (natural == Configuration.ORIENTATION_LANDSCAPE)
            automation.setRotation(
                if (quarterTurn) UiAutomation.ROTATION_FREEZE_90
                else UiAutomation.ROTATION_FREEZE_0,
            )
            waitUntil("la orientación pedida") {
                (orientationNow() == Configuration.ORIENTATION_LANDSCAPE) == landscape
            }
            Thread.sleep(SETTLE_MILLIS)
            val name = if (landscape) "horizontal" else "vertical"
            openTakeaway()
            tap(TAG_TAKEAWAY_NEW)
            val burger = productTag(ids.getValue("burger"))
            waitFor(burger)
            // El producto se ve de verdad (no solo existe en la jerarquía).
            compose.onNodeWithTag(burger).assertIsDisplayed()
            assertEquals(
                "riel lateral en $name",
                landscape,
                present(TAG_NAVIGATION_RAIL),
            )
            compose.onNodeWithTag(TAG_MAIN_NAVIGATION).assertIsDisplayed()
            screenshot("t9-captura-para-llevar-$name")
            leaveCapture()
        }
    }

    // --- Pantallas ---

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
        waitFor(TAG_TABLES_SCREEN)
    }

    private fun openTakeaway() {
        waitFor(TAG_NAV_TAKEAWAY)
        compose.onNodeWithTag(TAG_NAV_TAKEAWAY).performClick()
        waitFor(TAG_TAKEAWAY)
    }

    private fun addProduct(productId: String) {
        backToCatalog()
        waitFor(productTag(productId))
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
    }

    /** «Nombre y teléfono» desde la captura (en el teléfono, con la cuenta a la vista). */
    private fun editCustomerInCapture(name: String, phone: String) {
        showOrder()
        tap(TAG_ORDER_CUSTOMER_EDIT)
        fillCustomer(name, phone)
    }

    private fun fillCustomer(name: String, phone: String) {
        waitFor(TAG_CUSTOMER_NAME)
        compose.onNodeWithTag(TAG_CUSTOMER_NAME).performTextReplacement(name)
        compose.onNodeWithTag(TAG_CUSTOMER_PHONE).performTextReplacement(phone)
        compose.onNodeWithTag(TAG_CUSTOMER_SAVE).performClick()
        compose.waitUntil(timeoutMillis = 10_000) { !present(TAG_CUSTOMER_NAME) }
    }

    /** Sale de la captura con Atrás hasta la lista de Para llevar. */
    private fun leaveCapture() {
        repeat(3) {
            if (present(TAG_CAPTURE)) {
                pressBack()
            }
        }
        waitFor(TAG_TAKEAWAY)
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

    /**
     * En el teléfono, de la cuenta al catálogo («Volver a Catálogo»). Después de escribir el
     * nombre, el primer Atrás puede cerrar solo el teclado: se repite mientras siga la cuenta a la
     * vista.
     */
    private fun backToCatalog() {
        repeat(3) {
            if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
                pressBack()
            }
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

    private fun takeawayCaptures(): List<Capture> = runBlocking {
        container.database
            .waiter()
            .capturesOf(loadIds().getValue("ana"))
            .map(WaiterStorage::toCapture)
            .filter { it.takeaway }
    }

    private fun customerEdits() = runBlocking { container.database.waiter().customerEdits() }

    private fun waitForOrder(
        timeout: Long = 60_000,
        condition: (PendingOrder) -> Boolean,
    ): PendingOrder {
        val ana = loadIds().getValue("ana")
        var found: PendingOrder? = null
        waitUntil("el pedido para llevar", timeout) {
            found = runBlocking {
                container.database.waiter().ordersOf(ana).map(WaiterStorage::toOrder).lastOrNull {
                    it.destination.takeaway && condition(it)
                }
            }
            found != null
        }
        Log.i(TAG, "pedido para llevar: ${found!!.state} intentos ${found!!.attempts}")
        return found!!
    }

    // --- La computadora principal (caja y cocina) ---

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

    /** Catálogo con su cocina en pantalla: lo enviado crea tareas de cocina. */
    private fun createCatalogWithKitchen(): Map<String, String> {
        val category =
            main.call(
                "POST",
                "catalog/categories",
                """{"name":"Comida T","active":true,"color":"#d64545"}""",
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
        val station =
            main.call(
                "POST",
                "kitchen/stations",
                """{"operation_id":"${UUID.randomUUID()}","name":"Cocina T","active":true,""" +
                    """"reception":"kds","target_minutes":12,"warning_minutes":15,""" +
                    """"overdue_minutes":25,"grouping":"inherit","tracking":"inherit"}""",
            )
        assertEquals(station.body.toString(), 200, station.status)
        val route =
            main.call(
                "POST",
                "kitchen/routes",
                """{"operation_id":"${UUID.randomUUID()}","category_id":"$categoryId",""" +
                    """"mode":"prepare","station_ids":["${station.obj.text("id")}"]}""",
            )
        assertEquals(route.body.toString(), 200, route.status)
        return mapOf(
            "burger" to product("Hamburguesa T", "T-HAM", "120"),
            "tacos" to product("Tacos T", "T-TAC", "90"),
        )
    }

    /** La cuenta para llevar en Go (`Account`). */
    private fun account(accountId: String): JsonObject =
        main.call("GET", "restaurant/accounts/$accountId").obj.getValue("account").jsonObject

    private fun openTakeawayInGo(): List<JsonObject> =
        main
            .call("GET", "restaurant/accounts?mode=takeaway&state=open")
            .obj
            .getValue("items")
            .jsonArray
            .map { it.jsonObject }

    private fun kitchenTasks(accountId: String): List<Pair<String, Int>> =
        main
            .call("GET", "kitchen/board?state=all&account_id=$accountId")
            .obj
            .getValue("tasks")
            .jsonArray
            .map { it.jsonObject }
            .filter { it.text("state") != "cancelled" }
            .map { it.text("id") to it.getValue("revision").jsonPrimitive.int }

    private fun setKitchenState(tasks: List<Pair<String, Int>>, state: String) {
        assertTrue("Cocina tiene las tareas del pedido: $tasks", tasks.isNotEmpty())
        val references =
            tasks.joinToString(",") { (id, revision) -> """{"id":"$id","revision":$revision}""" }
        val answer =
            main.call(
                "POST",
                "kitchen/states",
                """{"operation_id":"${UUID.randomUUID()}","state":"$state","tasks":[$references]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    /**
     * La caja cobra el pedido completo en efectivo (abre su turno si hace falta). Un pedido para
     * llevar pagado sigue abierto hasta que se entrega.
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
        val state = account(accountId)
        assertEquals("open", state.text("state"))
        assertEquals("paid", state.text("payment_state"))
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
        const val ANA_USER = "ana.t1"
        const val ANA_PIN = "4829"
        const val ROLE = "Meseros A1"
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L

        /** Las listas perezosas en las que se busca desplazando. */
        val LAZY_LISTS = listOf(TAG_TAKEAWAY, TAG_ORDER_PANEL, TAG_PRODUCT_GRID)

        /**
         * La lista de permisos de mesero de Go con Para llevar (A1-D17 y la parte B de la A2): lo
         * de la A1 más `takeaway.read`, `takeaway.manage` y `takeaway.deliver`.
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
                "takeaway.deliver",
            )
    }
}
