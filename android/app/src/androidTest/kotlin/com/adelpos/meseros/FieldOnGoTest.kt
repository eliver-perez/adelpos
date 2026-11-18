package com.adelpos.meseros

import android.app.Activity
import android.app.Application
import android.content.Intent
import android.graphics.Bitmap
import android.os.Build
import android.os.Bundle
import android.os.ParcelFileDescriptor
import android.os.Process
import android.os.SystemClock
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasClickAction
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
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.access.EntryResult
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_CANCEL
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_QUANTITY
import com.adelpos.meseros.core.ui.catalog.TAG_OBSERVATION_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_OBSERVATION_ENTRY
import com.adelpos.meseros.core.ui.catalog.TAG_OBSERVATION_FIELD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.components.TAG_WAITER_MENU
import com.adelpos.meseros.core.ui.components.TAG_WAITER_SWITCH
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.core.ui.waiter.TAG_PREPARATION_PREFIX
import com.adelpos.meseros.feature.access.TAG_SWITCH_BLOCKED_OK
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_BACK
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_OFFLINE
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.capture.TAG_DRAFT_ROW_PREFIX
import com.adelpos.meseros.feature.waiter.capture.TAG_FOLLOW_MERGED
import com.adelpos.meseros.feature.waiter.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.feature.waiter.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.feature.waiter.capture.TAG_SENT_ROW_PREFIX
import com.adelpos.meseros.feature.waiter.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.feature.waiter.pending.TAG_PENDING
import com.adelpos.meseros.feature.waiter.pending.TAG_PENDING_CAPTURE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SEARCH
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_VIEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_CAPTURE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import java.io.File
import java.io.FileOutputStream
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Lo que las fases anteriores dejaron sin verificar, contra el **Go temporal** del arnés (§9 y Fase
 * F), nunca el servicio real. La prueba hace de computadora principal por la escucha local
 * ([MainComputer]) y la aplicación usa la escucha de red con la huella fijada al emparejar. Cada
 * paso corre en su propia instrumentación, en orden, con la aplicación instalada una sola vez
 * (`scripts/android-e2e.sh --suite field`); **la red la corta y la devuelve el guion** entre pasos.
 *
 * - `f1` restaurante con cocina en pantalla (estación y ruta de la categoría), 40 mesas y unos 300
 *   productos; dos meseros (Ana y Beto); captura del tema oscuro de «Buscar el servidor»; emparejar
 *   con el código y entrar con el PIN.
 * - `f2` rendimiento (registro `AdelOnGo`): arranque en frío hasta la lista de meseros, entrada
 *   hasta Mesas con 40 mesas, abrir la captura con el catálogo, «Enviar a cocina» hasta «Recibido
 *   por el restaurante» y la memoria del proceso (`dumpsys meminfo`).
 * - `f3` cocina pasa lo enviado a «En preparación» y a «Listo»: el mesero lo ve al momento por
 *   `restaurant.account.preparation_changed`, sin permisos de cocina.
 * - `f4` dos meseros en el mismo dispositivo: Ana deja una captura en curso y cambia a Beto (se
 *   permite); Beto no la ve, captura y envía; Ana vuelve y la encuentra; Go anota a cada uno.
 * - `f5a` (sin red) un pedido de Beto «Por enviar» impide cambiar de mesero; `f5b` (con red) sale
 *   con la credencial de envío de Beto aunque Ana sea la activa.
 * - `f6` con la cuenta abierta en la captura, la principal **transfiere** lo enviado a otra cuenta
 *   y después la **combina**: la aplicación lo nota (lo enviado desaparece; «la cuenta ya no está
 *   abierta») y no pierde lo capturado. Deja en el registro lo que queda en la cuenta destino
 *   (hallazgo M3 de la revisión R6).
 * - `f7a` bloqueo largo del PIN (A1-D16; el guion deja en la base temporal dos bloqueos previos de
 *   la serie): el quinto fallo deja el PIN de Beto «hasta que lo desbloquee un administrador»;
 *   `f7b` (sin red) su entrada sin conexión también queda cerrada; `f7c` la principal lo desbloquea
 *   y Beto entra.
 * - `f8` Android destruye la actividad y sus ViewModel con la captura y el personalizador abiertos
 *   (se recrea la actividad vaciando su almacén de ViewModel: el mismo camino de restauración que
 *   tras la muerte del proceso para el estado de pantalla): lo capturado sigue; «Cambiar» vuelve
 *   con la partida guardada; una partida nueva a medio personalizar vuelve con su configuración
 *   original (limitación conocida, queda en el registro).
 * - `f9` Atrás en Mesas (en Android 8–11 cierra la actividad) y, al volver después del plazo, el
 *   bloqueo por inactividad; con una captura en curso no se bloquea.
 * - `f10` tema oscuro: capturas de las pantallas principales (las copia el guion) y todo lo que se
 *   toca mide al menos 48 dp.
 *
 * Argumentos: `local`, `address`, `pin`, `mainDevice`, `mainToken` (los arma el guion). Sin ellos
 * se omite.
 */
@RunWith(AndroidJUnit4::class)
class FieldOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private val stateFile = File(TestDevice.app.filesDir, "field-on-go.json")

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
        runCatching { scenario?.close() }
    }

    // --- Pasos ---

    @Test
    fun f1_prepararRestauranteConCocinaEmparejarYEntrar() {
        val users =
            main.ensureWaiters(
                mapOf(ANA_USER to ("Ana" to ANA_PIN), BETO_USER to ("Beto" to BETO_PIN)),
            )
        grantWaiterPermissions()
        val ids =
            mutableMapOf("ana" to users.getValue(ANA_USER), "beto" to users.getValue(BETO_USER))
        ids.putAll(createCatalog())
        ids.putAll(createKitchen(ids.getValue("category")))
        ids.putAll(createTables())
        saveIds(ids)
        TestDevice.reset()
        // «Buscar el servidor» en el tema oscuro (para la revisión de f10).
        setNight(true)
        try {
            launch()
            waitFor(TAG_FIND_SERVER)
            screenshot("01-buscar-servidor")
            assertTouchTargets("buscar-servidor")
        } finally {
            setNight(false)
        }
        waitFor(TAG_FIND_SERVER)
        pairByAddress()
        enter(ids.getValue("ana"), ANA_PIN)
        waitFor("$TAG_TABLE_PREFIX${table(ids, 1)}")
        waitUntil("el catálogo descargado", timeout = 90_000) {
            container.catalogRealtime.catalog.catalog.value.products.size >= FILLER_PRODUCTS
        }
    }

    @Test
    fun f2_rendimiento() {
        val ids = loadIds()
        // Arranque en frío: el proceso lo acaba de crear la instrumentación (incluye el ejecutor de
        // pruebas, que es poco) hasta que se ve la lista de meseros.
        val launched = SystemClock.elapsedRealtime()
        launch()
        waitFor("$TAG_WAITER_PREFIX${ids.getValue("ana")}")
        val now = SystemClock.elapsedRealtime()
        val coldMillis = now - Process.getStartElapsedRealtime()
        val activityMillis = now - launched
        perf("arranque en frío (proceso → lista de meseros): $coldMillis ms")
        perf("abrir la actividad → lista de meseros: $activityMillis ms")
        perf("sistema: ${displayedLine()}")

        // Entrada con conexión hasta ver Mesas con las 40 mesas (la primera y la última de la
        // lista).
        compose.onNodeWithTag("$TAG_WAITER_PREFIX${ids.getValue("ana")}").performClick()
        typeDigits(ANA_PIN)
        val entering = SystemClock.elapsedRealtime()
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitFor("$TAG_TABLE_PREFIX${table(ids, 1)}", reveal = false)
        perf("PIN → Mesas con la primera mesa: ${SystemClock.elapsedRealtime() - entering} ms")
        waitFor("$TAG_TABLE_PREFIX${table(ids, TABLES)}")
        perf("PIN → la mesa $TABLES a la vista: ${SystemClock.elapsedRealtime() - entering} ms")

        // Captura con el catálogo de ~300 productos.
        tap("$TAG_TABLE_PREFIX${table(ids, 1)}")
        waitFor(TAG_NEW_ACCOUNT)
        val opening = SystemClock.elapsedRealtime()
        tap(TAG_NEW_ACCOUNT)
        waitFor(productTag(ids.getValue("burger")), reveal = false)
        perf("«Nueva cuenta» → catálogo a la vista: ${SystemClock.elapsedRealtime() - opening} ms")
        addProduct(ids.getValue("burger"))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val sending = SystemClock.elapsedRealtime()
        tap(TAG_SEND_TO_KITCHEN)
        val order =
            waitForOrder(ids.getValue("ana"), table(ids, 1)) { it.state == OrderState.RECEIVED }
        perf(
            "«Enviar a cocina» → Recibido por el restaurante: ${SystemClock.elapsedRealtime() - sending} ms",
        )
        perf("memoria: ${memory()}")
        saveIds(ids + ("account1" to order.receivedAccountId!!))
    }

    @Test
    fun f3_preparacionYListoAlMomento() {
        val ids = loadIds()
        val accountId = ids.getValue("account1")
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        openAccount(table(ids, 1), accountId)
        val versions = sentVersions(accountId)
        assertEquals(2, versions.size)
        for (version in versions) {
            waitFor("$TAG_SENT_ROW_PREFIX$version")
        }
        val tasks = kitchenTasks(accountId)
        assertTrue("Cocina tiene las tareas de lo enviado: $tasks", tasks.size >= 2)
        // «En preparación» desde cocina (la principal): el mesero lo ve sin tocar nada.
        val preparing = SystemClock.elapsedRealtime()
        setKitchenState(tasks, "preparing")
        for (version in versions) {
            waitForPreparation(version, "preparing")
        }
        perf(
            "cocina «En preparación» → visto por el mesero: ${SystemClock.elapsedRealtime() - preparing} ms",
        )
        val ready = SystemClock.elapsedRealtime()
        setKitchenState(kitchenTasks(accountId), "ready")
        for (version in versions) {
            waitForPreparation(version, "ready")
        }
        perf("cocina «Listo» → visto por el mesero: ${SystemClock.elapsedRealtime() - ready} ms")
    }

    @Test
    fun f4_dosMeserosConCapturaEnCurso() {
        val ids = loadIds()
        val ana = ids.getValue("ana")
        val beto = ids.getValue("beto")
        launch()
        enter(ana, ANA_PIN)
        newAccountOn(table(ids, 5))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val anaCapture = currentCapture(ana, table(ids, 5))
        val anaItem = anaCapture.visible.single().id

        // Una captura en curso no impide cambiar de mesero: se queda guardada con Ana.
        switchWaiter()
        enter(beto, BETO_PIN)
        assertEquals(beto, container.deviceAccess.activeWaiter.value?.userId)
        openTable(table(ids, 5))
        waitFor(TAG_ACCOUNT_PANE)
        assertFalse(
            "Beto no ve la captura de Ana",
            present("$TAG_TABLE_CAPTURE_PREFIX${anaCapture.id}"),
        )
        showTables()
        openPending()
        assertFalse(present("$TAG_PENDING_CAPTURE_PREFIX${anaCapture.id}"))
        pressBack()

        // Beto captura y envía con su sesión.
        newAccountOn(table(ids, 6))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        val betoOrder = waitForOrder(beto, table(ids, 6)) { it.state == OrderState.RECEIVED }

        // Ana vuelve (Beto no tiene pedidos sin enviar) y su captura sigue ahí.
        switchWaiter()
        enter(ana, ANA_PIN)
        openTable(table(ids, 5))
        tap("$TAG_TABLE_CAPTURE_PREFIX${anaCapture.id}")
        showOrder()
        waitFor("$TAG_DRAFT_ROW_PREFIX$anaItem")
        tap(TAG_SEND_TO_KITCHEN)
        val anaOrder = waitForOrder(ana, table(ids, 5)) { it.state == OrderState.RECEIVED }

        // Go anota a cada mesero en su pedido.
        assertEquals(listOf("Ana A1"), confirmedBy(anaOrder.receivedAccountId!!))
        assertEquals(listOf("Beto A1"), confirmedBy(betoOrder.receivedAccountId!!))
    }

    @Test
    fun f5a_sinConexionLosPedidosDeBetoImpidenCambiar() {
        val ids = loadIds()
        val beto = ids.getValue("beto")
        launch()
        // Sin red: Beto entra con su verificador local (ya entró con conexión en f4).
        waitFor(TAG_ENTRY_OFFLINE)
        enter(beto, BETO_PIN)
        newAccountOn(table(ids, 7))
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}pending")
        tap(TAG_SEND_TO_KITCHEN)
        waitForOrder(beto, table(ids, 7)) { it.state == OrderState.QUEUED }
        // Con un pedido suyo «Por enviar» no se puede cambiar de mesero.
        showTables()
        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        waitFor(TAG_SWITCH_BLOCKED_OK)
        compose.onNodeWithTag(TAG_SWITCH_BLOCKED_OK).performClick()
        assertEquals(beto, container.deviceAccess.activeWaiter.value?.userId)
        assertFalse("Sigue en Mesas, no en la lista de meseros", present(TAG_ENTRY))
    }

    @Test
    fun f5b_cadaPedidoSaleConLaCredencialDeSuMesero() {
        val ids = loadIds()
        launch()
        // Ana entra con conexión; el pedido de Beto sale con la credencial de envío de Beto.
        enter(ids.getValue("ana"), ANA_PIN)
        val order =
            waitForOrder(ids.getValue("beto"), table(ids, 7), timeout = 120_000) {
                it.state == OrderState.RECEIVED
            }
        assertEquals(ids.getValue("ana"), container.deviceAccess.activeWaiter.value?.userId)
        assertEquals(listOf("Beto A1"), confirmedBy(order.receivedAccountId!!))
    }

    @Test
    fun f6_transferirYCombinarConLaCuentaAbierta() {
        val ids = loadIds()
        val ana = ids.getValue("ana")
        launch()
        enter(ana, ANA_PIN)
        newAccountOn(table(ids, 8))
        addProduct(ids.getValue("burger"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        val sent = waitForOrder(ana, table(ids, 8)) { it.state == OrderState.RECEIVED }
        val accountId = sent.receivedAccountId!!
        val burgerVersion = sentVersions(accountId).single()

        // Ana vuelve a la cuenta y captura algo más (guardado en el borrador de Go).
        openAccount(table(ids, 8), accountId)
        waitFor("$TAG_SENT_ROW_PREFIX$burgerVersion")
        addProduct(ids.getValue("shake"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val capture = currentCapture(ana, table(ids, 8))
        val shakeItem = capture.visible.single().id

        // Mientras tanto, en la principal: otra cuenta en la mesa 9 y se le transfiere lo enviado.
        val target = openAccountFromMain(table(ids, 9), ids.getValue("tacos"))
        transfer(accountId, target, burgerVersion)
        waitUntil("lo enviado sale de la cuenta de Ana") {
            !present("$TAG_SENT_ROW_PREFIX$burgerVersion")
        }
        assertTrue("Lo capturado sigue", present("$TAG_DRAFT_ROW_PREFIX$shakeItem"))

        // Después la combina en esa otra cuenta: la cuenta de Ana ya no está abierta.
        merge(accountId, target)
        waitFor("${TAG_SYNC_PREFIX}account_closed")
        waitFor("$TAG_DRAFT_ROW_PREFIX$shakeItem")
        val closed = currentCapture(ana, table(ids, 8))
        assertEquals(1, closed.visible.size)
        // M3 (revisión R6): Go movió el borrador a la cuenta destino con sus mismos ids.
        val targetDraft = bill(target).getValue("draft").jsonArray.map { it.jsonObject.text("id") }
        perf(
            "f6: borrador de la cuenta destino tras combinar: $targetDraft (renglón de Ana: $shakeItem)",
        )

        // M3 (revisión R6, A1-D19): la captura ofrece seguir en la cuenta en que se combinó; lo
        // capturado se envía ahí y no queda nada de Ana por enviar en la cuenta destino.
        tap(TAG_FOLLOW_MERGED)
        waitUntil("la captura en la cuenta destino") {
            val following = currentCapture(ana, table(ids, 8))
            following.accountId == target &&
                following.operations.isEmpty() &&
                following.base.any { it.id == shakeItem }
        }
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        val received =
            waitForOrder(ana, table(ids, 8)) {
                it.operationId == closed.id && it.state == OrderState.RECEIVED
            }
        assertEquals(target, received.receivedAccountId)
        val sentShakes =
            bill(target).getValue("lines").jsonArray.count {
                it.jsonObject.getValue("request").jsonObject.text("product_id") ==
                    ids.getValue("shake")
            }
        assertEquals("La malteada entró una vez", 1, sentShakes)
        val stillThere =
            bill(target).getValue("draft").jsonArray.count {
                it.jsonObject.text("product_id") == ids.getValue("shake")
            }
        perf(
            "f6 (M3): renglones de la malteada que siguen por enviar en la cuenta destino: $stillThere",
        )
        assertEquals("Nada de Ana queda por enviar en la cuenta destino", 0, stillThere)
    }

    @Test
    fun f7a_bloqueoLargoDelPin() {
        val ids = loadIds()
        val beto = ids.getValue("beto")
        launch()
        waitFor("$TAG_WAITER_PREFIX$beto")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$beto").performClick()
        // El guion dejó dos bloqueos previos en la serie: el quinto fallo es el tercer bloqueo.
        repeat(4) {
            typePin(WRONG_PIN)
            waitForText("PIN incorrecto")
        }
        typePin(WRONG_PIN)
        waitForText("Pide a un administrador que lo desbloquee")
        // Ni con el PIN correcto.
        typePin(BETO_PIN)
        waitForText("Pide a un administrador que lo desbloquee")
        assertNull(container.deviceAccess.activeWaiter.value)
        val known = runBlocking { container.database.access().knownWaiter(beto) }!!
        assertTrue("El bloqueo de Go cierra la entrada sin conexión", known.offlineClosed)
        val user =
            main
                .call("GET", "users")
                .body
                .jsonArray
                .map { it.jsonObject }
                .single { it.text("id") == beto }
        assertTrue(user.getValue("pin_requires_unlock").jsonPrimitive.boolean)
        compose.onNodeWithTag(TAG_ENTRY_BACK).performClick()
        waitForText("PIN bloqueado")
    }

    @Test
    fun f7b_bloqueoLargoCierraLaEntradaSinConexion() {
        val ids = loadIds()
        launch()
        waitFor(TAG_ENTRY_OFFLINE)
        waitForText("Sin conexión no puede entrar")
        assertEquals(
            EntryResult.OfflineClosed,
            runBlocking { container.deviceAccess.enter(ids.getValue("beto"), BETO_PIN) },
        )
    }

    @Test
    fun f7c_unAdministradorLoDesbloquea() {
        val ids = loadIds()
        val beto = ids.getValue("beto")
        val unlocked = main.call("POST", "users/$beto/pin/unlock")
        assertEquals(unlocked.body.toString(), 200, unlocked.status)
        launch()
        enter(beto, BETO_PIN)
        assertEquals(beto, container.deviceAccess.activeWaiter.value?.userId)
        val known = runBlocking { container.database.access().knownWaiter(beto) }!!
        assertFalse(
            "Entrar con conexión vuelve a abrir la entrada sin conexión",
            known.offlineClosed,
        )
    }

    @Test
    fun f8_muerteDeLaActividadEnCapturaYPersonalizador() {
        val ids = loadIds()
        val ana = ids.getValue("ana")
        launch()
        enter(ana, ANA_PIN)
        newAccountOn(table(ids, 10))
        // Una partida ya capturada (la captura no está vacía: su ViewModel no la cierra).
        addProduct(ids.getValue("tacos"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val first = currentCapture(ana, table(ids, 10)).visible.single()

        // Personalizador de una partida nueva: 3 hamburguesas «sin cebolla», y Android destruye la
        // actividad con sus ViewModel.
        backToCatalog()
        tap(productTag(ids.getValue("burger")))
        customize(quantity = 3, observation = "sin cebolla")
        recreateLosingViewModels()
        waitFor(TAG_CUSTOMIZER_ADD)
        val restoredNew = customizerState()
        perf("f8: personalizador de una partida nueva tras perder la actividad: $restoredNew")
        val keptNew = restoredNew == CustomizerState("3", listOf("sin cebolla"))
        perf("f8: ¿conserva lo elegido en la partida nueva? $keptNew")
        // Lo ya capturado sigue (Room).
        assertEquals(first.line, currentCapture(ana, table(ids, 10)).visible.first().line)
        click(compose.onNodeWithTag(TAG_CUSTOMIZER_CANCEL))

        // «Cambiar» una partida ya capturada (2 tacos «bien dorados»), con el personalizador
        // abierto al perder la actividad: al volver debe mostrar esa partida, no la
        // configuración original del producto.
        showOrder()
        changeLine(first.id)
        customize(quantity = 2, observation = "bien dorados")
        click(compose.onNodeWithTag(TAG_CUSTOMIZER_ADD))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        val changed = currentCapture(ana, table(ids, 10)).visible.single { it.id == first.id }
        assertEquals(2, changed.line.quantity)
        changeLine(first.id)
        waitFor(TAG_CUSTOMIZER_ADD)
        recreateLosingViewModels()
        waitFor(TAG_CUSTOMIZER_ADD)
        val restoredChange = customizerState()
        perf("f8: «Cambiar» tras perder la actividad: $restoredChange")
        assertEquals(
            "«Cambiar» vuelve con la configuración de la partida",
            CustomizerState(quantity = "2", observations = listOf("bien dorados")),
            restoredChange,
        )
        click(compose.onNodeWithTag(TAG_CUSTOMIZER_CANCEL))
        showOrder()
        waitFor("$TAG_DRAFT_ROW_PREFIX${first.id}")
        val after = currentCapture(ana, table(ids, 10)).visible.single { it.id == first.id }
        assertEquals(changed.line, after.line)
    }

    @Test
    fun f9_atrasYBloqueoPorInactividadAlVolver() {
        val ids = loadIds()
        // Beto no tiene capturas ni pedidos pendientes de los pasos anteriores (Ana podría
        // tenerlos).
        val beto = ids.getValue("beto")
        setIdleLock(IDLE_SECONDS)
        try {
            startFromLauncher()
            // La entrada con conexión trae el plazo nuevo del dispositivo.
            enter(beto, BETO_PIN)
            showTables()
            // Atrás en Mesas cierra la actividad (el proceso sigue): al volver, el plazo ya venció.
            pressBack()
            waitUntil("la aplicación en segundo plano") { resumedActivity() == null }
            perf(
                "f9: Atrás en Mesas (API ${Build.VERSION.SDK_INT}): actividades vivas ${liveActivities()}",
            )
            Thread.sleep((IDLE_SECONDS + 6) * 1_000L)
            startFromLauncher()
            waitFor(TAG_ENTRY)
            assertNull("Bloqueado al volver", container.deviceAccess.activeWaiter.value)

            // Con una captura en curso no se bloquea.
            enter(beto, BETO_PIN)
            newAccountOn(table(ids, 11))
            addProduct(ids.getValue("tacos"))
            pressBack()
            showTables()
            pressBack()
            waitUntil("la aplicación en segundo plano") { resumedActivity() == null }
            Thread.sleep((IDLE_SECONDS + 6) * 1_000L)
            startFromLauncher()
            waitUntil("de vuelta") { resumedActivity() != null }
            Thread.sleep(SETTLE_MILLIS)
            assertEquals(beto, container.deviceAccess.activeWaiter.value?.userId)
            assertFalse("No se bloquea con una captura en curso", present(TAG_ENTRY))
        } finally {
            // El resto de la suite sin el plazo corto (lo toma la siguiente entrada con conexión).
            setIdleLock(3_600)
        }
    }

    @Test
    fun f10_temaOscuroYBotones() {
        val ids = loadIds()
        val ana = ids.getValue("ana")
        val problems = mutableListOf<String>()
        setNight(true)
        try {
            launch()
            waitFor("$TAG_WAITER_PREFIX$ana")
            shot("02-meseros", problems)
            compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
            waitFor("${TAG_PIN_KEY_PREFIX}1")
            shot("03-pin", problems)
            typePin(ANA_PIN)
            waitFor(TAG_TABLES_SEARCH)
            shot("04-mesas-lista", problems)
            tap("$TAG_TABLES_VIEW-map")
            Thread.sleep(SETTLE_MILLIS)
            shot("05-mesas-mapa", problems)
            tap("$TAG_TABLES_VIEW-list")
            openTable(table(ids, 1))
            waitFor(TAG_ACCOUNT_PANE)
            shot("06-mesa", problems)
            tap("account-${ids.getValue("account1")}")
            waitFor(TAG_PRODUCT_GRID)
            shot("07-captura", problems)
            tap(productTag(ids.getValue("burger")))
            waitFor(TAG_CUSTOMIZER_ADD)
            shot("08-personalizador", problems)
            click(compose.onNodeWithTag(TAG_CUSTOMIZER_CANCEL))
            showOrder()
            shot("09-cuenta", problems)
            pressBack()
            showTables()
            openPending()
            shot("10-pedidos-pendientes", problems)
            pressBack()
            showTables()
            compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
            Thread.sleep(SETTLE_MILLIS)
            shot("11-menu-del-mesero", problems)
        } finally {
            setNight(false)
        }
        assertTrue("Botones de menos de 48 dp:\n${problems.joinToString("\n")}", problems.isEmpty())
    }

    // --- Pantallas ---

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    /** Abre la aplicación como el lanzador (o desde Recientes): retoma la tarea si existe. */
    private fun startFromLauncher() {
        val context = TestDevice.app
        val intent =
            requireNotNull(context.packageManager.getLaunchIntentForPackage(context.packageName))
                .addFlags(
                    Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED,
                )
        context.startActivity(intent)
        waitUntil("la actividad al frente") { resumedActivity() != null }
    }

    private fun resumedActivity(): Activity? {
        var found: Activity? = null
        instrumentation.runOnMainSync {
            found =
                ActivityLifecycleMonitorRegistry.getInstance()
                    .getActivitiesInStage(Stage.RESUMED)
                    .firstOrNull()
        }
        return found
    }

    /** Si el despachador de Atrás de la actividad tiene alguna acción habilitada. */
    private fun backEnabled(): Boolean? {
        val activity = resumedActivity() as? androidx.activity.ComponentActivity ?: return null
        var enabled = false
        instrumentation.runOnMainSync {
            enabled = activity.onBackPressedDispatcher.hasEnabledCallbacks()
        }
        return enabled
    }

    private fun liveActivities(): Int {
        var count = 0
        instrumentation.runOnMainSync {
            val monitor = ActivityLifecycleMonitorRegistry.getInstance()
            count =
                listOf(Stage.CREATED, Stage.STARTED, Stage.RESUMED, Stage.PAUSED, Stage.STOPPED)
                    .sumOf { monitor.getActivitiesInStage(it).size }
        }
        return count
    }

    /**
     * Android destruye la actividad y sus ViewModel, y la vuelve a crear con su estado guardado: el
     * mismo camino de restauración que tras la muerte del proceso para lo de pantalla
     * ([androidx.lifecycle.SavedStateHandle] y la pila de navegación). Se recrea la actividad y, al
     * destruirse (después de guardar su estado), se vacía el almacén de ViewModel que conservaría.
     */
    private fun recreateLosingViewModels() {
        compose.waitForIdle()
        val old = resumedActivity() as androidx.activity.ComponentActivity
        val callbacks =
            object : Application.ActivityLifecycleCallbacks {
                override fun onActivityDestroyed(activity: Activity) {
                    if (activity === old) {
                        old.viewModelStore.clear()
                    }
                }

                override fun onActivityCreated(activity: Activity, state: Bundle?) {}

                override fun onActivityStarted(activity: Activity) {}

                override fun onActivityResumed(activity: Activity) {}

                override fun onActivityPaused(activity: Activity) {}

                override fun onActivityStopped(activity: Activity) {}

                override fun onActivitySaveInstanceState(activity: Activity, state: Bundle) {}
            }
        TestDevice.app.registerActivityLifecycleCallbacks(callbacks)
        try {
            instrumentation.runOnMainSync { old.recreate() }
            waitUntil("la actividad nueva") {
                val now = resumedActivity()
                now != null && now !== old
            }
        } finally {
            TestDevice.app.unregisterActivityLifecycleCallbacks(callbacks)
        }
        log("actividad creada de nuevo sin sus ViewModel")
    }

    private fun waitFor(tag: String, timeout: Long = 40_000, reveal: Boolean = true) {
        var nextReveal = 0L
        try {
            compose.waitUntil(timeoutMillis = timeout) {
                present(tag) ||
                    (reveal &&
                        System.currentTimeMillis() >= nextReveal &&
                        run {
                            nextReveal = System.currentTimeMillis() + REVEAL_EVERY_MILLIS
                            reveal(tag)
                        })
            }
        } catch (failure: Throwable) {
            runCatching { compose.onRoot(useUnmergedTree = true).printToLog(TAG) }
            throw failure
        }
    }

    /** Si aparece el nodo [tag] antes de [timeout] (sin fallar). */
    private fun waitedFor(tag: String, timeout: Long): Boolean = runCatching {
        compose.waitUntil(timeoutMillis = timeout) { present(tag) }
    }
        .isSuccess

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

    private fun present(tag: String): Boolean = present(hasTestTag(tag))

    private fun present(matcher: SemanticsMatcher): Boolean =
        compose.onAllNodes(matcher, useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty()

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
        // En un teléfono, el nodo puede existir (la lista perezosa ya lo compuso) y estar fuera de
        // la vista: performScrollTo no desplaza una lista perezosa, así que se le pide a la lista
        // que lo traiga (regresión de la Fase G: «Enviar a cocina» debajo del total en f6).
        if (!displayed(tag)) {
            reveal(tag)
        }
        click(compose.onNodeWithTag(tag))
    }

    private fun displayed(tag: String): Boolean = runCatching {
        compose.onNodeWithTag(tag).assertIsDisplayed()
    }
        .isSuccess

    /** Toca el nodo; si está dentro de algo desplazable, primero lo trae a la vista. */
    private fun click(node: SemanticsNodeInteraction) {
        runCatching { node.performScrollTo() }
        node.performClick()
    }

    private fun pressBack() {
        log("Atrás")
        SystemDialogs.dismissNotResponding(instrumentation.uiAutomation)
        instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        Thread.sleep(SETTLE_MILLIS)
        compose.waitForIdle()
    }

    private fun pairByAddress() {
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

    private fun typeDigits(pin: String) {
        for (digit in pin) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
    }

    private fun typePin(pin: String) {
        typeDigits(pin)
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
    }

    private fun enter(userId: String, pin: String) {
        waitFor("$TAG_WAITER_PREFIX$userId")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$userId").performClick()
        typePin(pin)
        waitFor(TAG_TABLES_SEARCH)
    }

    /** «Cambiar de mesero» desde el menú visible (lleva a la lista de meseros). */
    private fun switchWaiter() {
        if (!present(TAG_WAITER_MENU)) {
            showTables()
        }
        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        waitFor(TAG_ENTRY)
    }

    private fun waitForTables() {
        try {
            waitUntil("Mesas") { present(TAG_TABLES_SEARCH) || present(TAG_ACCOUNT_PANE) }
        } catch (failure: Throwable) {
            printTree()
            throw failure
        }
    }

    private fun printTree() {
        runCatching { compose.onRoot(useUnmergedTree = true).printToLog(TAG) }
            .onFailure { log("sin pantalla: ${it.message}") }
    }

    /** Vuelve a Mesas desde la captura o desde la mesa abierta en el teléfono. */
    private fun showTables() {
        repeat(4) {
            compose.waitForIdle()
            val list = present(TAG_TABLES_SEARCH)
            val tableOnPhone = present(TAG_ACCOUNT_PANE) && present(TAG_PANE_BACK)
            val capture = present(TAG_ORDER_PANEL) || present(TAG_PRODUCT_GRID)
            val pending = present(TAG_PENDING)
            log(
                "Mesas: lista=$list mesa=$tableOnPhone captura=$capture pendientes=$pending " +
                    "atrás=${backEnabled()} al frente=${resumedActivity() != null}",
            )
            if (list && !tableOnPhone) {
                return
            }
            if (!list && !present(TAG_ACCOUNT_PANE) && !capture && !pending) {
                printTree()
                return
            }
            pressBack()
        }
        waitForTables()
    }

    private fun openTable(tableId: String) {
        showTables()
        tap("$TAG_TABLE_PREFIX$tableId")
    }

    private fun openAccount(tableId: String, accountId: String) {
        openTable(tableId)
        tap("account-$accountId")
        showOrder()
    }

    private fun newAccountOn(tableId: String) {
        openTable(tableId)
        tap(TAG_NEW_ACCOUNT)
        waitFor(TAG_PRODUCT_GRID)
    }

    private fun addProduct(productId: String) {
        backToCatalog()
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        click(compose.onNodeWithTag(TAG_CUSTOMIZER_ADD))
        log("agregado $productId")
    }

    /** En el teléfono, la cuenta está a un toque; en la tableta, siempre a la vista. */
    private fun showOrder() {
        if (!present(TAG_ORDER_PANEL) && present(TAG_PANE_OPEN_SECONDARY)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    private fun backToCatalog() {
        if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
            pressBack()
        }
    }

    private fun openPending() {
        showTables()
        tap(TAG_OPEN_PENDING)
        waitFor(TAG_PENDING)
    }

    /** Cantidad y una observación en el personalizador abierto. */
    private fun customize(quantity: Int, observation: String) {
        waitFor(TAG_CUSTOMIZER_ADD)
        repeat(quantity - 1) { click(compose.onNodeWithTag("$TAG_CUSTOMIZER_QUANTITY-more")) }
        val field = compose.onNodeWithTag(TAG_OBSERVATION_FIELD)
        runCatching { field.performScrollTo() }
        field.performTextReplacement(observation)
        // La acción del teclado agrega la observación (como el botón); si no llegó, el botón.
        field.performImeAction()
        if (!waitedFor(TAG_OBSERVATION_ENTRY, OBSERVATION_WAIT_MILLIS)) {
            click(compose.onNodeWithTag(TAG_OBSERVATION_ADD))
        }
        waitFor(TAG_OBSERVATION_ENTRY)
        assertEquals(CustomizerState(quantity.toString(), listOf(observation)), customizerState())
    }

    /** Lo que muestra el personalizador: la cantidad y las observaciones agregadas. */
    private data class CustomizerState(val quantity: String, val observations: List<String>)

    private fun customizerState(): CustomizerState {
        val quantity =
            compose
                .onAllNodes(hasTestTag("$TAG_CUSTOMIZER_QUANTITY-value"), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .single()
                .let(::textOf)
        val observations =
            compose
                .onAllNodes(hasTestTag(TAG_OBSERVATION_ENTRY), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .map(::textOf)
        return CustomizerState(quantity, observations)
    }

    private fun textOf(node: SemanticsNode): String =
        node.config.getOrNull(SemanticsProperties.Text)?.joinToString("") { it.text }
            ?: node.children.joinToString("") { textOf(it) }

    /** «Cambiar» de una partida por enviar (el botón no tiene etiqueta propia: su texto). */
    private fun changeLine(itemId: String) {
        showOrder()
        waitFor("$TAG_DRAFT_ROW_PREFIX$itemId")
        click(
            compose
                .onAllNodes(
                    hasText("Cambiar") and
                        hasClickAction() and
                        hasAnyAncestor(hasTestTag("$TAG_DRAFT_ROW_PREFIX$itemId")),
                )
                .onFirst(),
        )
    }

    private fun waitForPreparation(versionId: String, state: String) {
        waitFor("$TAG_SENT_ROW_PREFIX$versionId")
        waitUntil("«$state» en la partida $versionId") {
            present(
                hasTestTag("$TAG_PREPARATION_PREFIX$state") and
                    hasAnyAncestor(hasTestTag("$TAG_SENT_ROW_PREFIX$versionId")),
            )
        }
    }

    // --- Capturas y medidas ---

    private fun setNight(on: Boolean) {
        shell("cmd uimode night ${if (on) "yes" else "no"}")
        Thread.sleep(SETTLE_MILLIS)
    }

    private fun shot(name: String, problems: MutableList<String>) {
        screenshot(name)
        problems += touchTargetProblems(name)
    }

    /** Captura de pantalla en la carpeta de la aplicación (el guion la copia a la Mac). */
    private fun screenshot(name: String) {
        // Un diálogo del sistema «… no responde» (emulador cargado) taparía la pantalla.
        SystemDialogs.dismissNotResponding(instrumentation.uiAutomation)
        compose.waitForIdle()
        Thread.sleep(SETTLE_MILLIS)
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: return
        val folder = File(TestDevice.app.getExternalFilesDir(null), "screens").apply { mkdirs() }
        FileOutputStream(File(folder, "$name.png")).use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
        log("captura $name")
    }

    private fun assertTouchTargets(screen: String) {
        val problems = touchTargetProblems(screen)
        assertTrue("Botones de menos de 48 dp:\n${problems.joinToString("\n")}", problems.isEmpty())
    }

    /** Todo lo que se toca en la pantalla [screen] mide al menos 48 × 48 dp (zona táctil). */
    private fun touchTargetProblems(screen: String): List<String> {
        val density = TestDevice.app.resources.displayMetrics.density
        return compose.onAllNodes(hasClickAction()).fetchSemanticsNodes().mapNotNull { node ->
            // Lo que quedó fuera de la vista (desplazado) no se revisa en esta pantalla; lo
            // recortado por el borde de una lista se mide sin el recorte (su tamaño completo).
            if (node.boundsInRoot.isEmpty) {
                return@mapNotNull null
            }
            val bounds = node.touchBoundsInRoot
            val width = maxOf(bounds.width, node.size.width.toFloat()) / density
            val height = maxOf(bounds.height, node.size.height.toFloat()) / density
            if (
                width + TOUCH_TOLERANCE_DP >= MIN_TOUCH_DP &&
                    height + TOUCH_TOLERANCE_DP >= MIN_TOUCH_DP
            ) {
                null
            } else {
                val name =
                    node.config.getOrNull(SemanticsProperties.TestTag)
                        ?: textOf(node).ifBlank {
                            node.config
                                .getOrNull(SemanticsProperties.ContentDescription)
                                ?.joinToString() ?: "?"
                        }
                "$screen: «$name» ${"%.0f".format(width)} × ${"%.0f".format(height)} dp"
            }
        }
    }

    private fun perf(message: String) {
        Log.i(TAG, "Fase F · $message")
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private fun shell(command: String): String {
        val descriptor = instrumentation.uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(descriptor).use {
            it.readBytes().decodeToString()
        }
    }

    /** La línea «Displayed …» del sistema para la actividad (tiempo de la primera pantalla). */
    private fun displayedLine(): String =
        shell("logcat -d -s ActivityTaskManager:I ActivityManager:I")
            .lines()
            .lastOrNull { it.contains("Displayed com.adelpos.meseros") }
            ?.substringAfter("Displayed ")
            ?.trim() ?: "sin línea Displayed"

    /** PSS total del proceso (incluye el código de la prueba, que comparte el proceso). */
    private fun memory(): String {
        val text = shell("dumpsys meminfo ${TestDevice.app.packageName}")
        val total = text.lines().firstOrNull { it.trim().startsWith("TOTAL") }?.trim()
        val heap = text.lines().firstOrNull { it.trim().startsWith("Java Heap") }?.trim()
        val native = text.lines().firstOrNull { it.trim().startsWith("Native Heap") }?.trim()
        return "$total | $heap | $native"
    }

    // --- Lo guardado en el dispositivo ---

    private fun currentCapture(userId: String, tableId: String): Capture = runBlocking {
        container.database
            .waiter()
            .capturesOf(userId)
            .map(WaiterStorage::toCapture)
            .filter { it.tableId == tableId }
            .maxBy { it.createdAtMillis }
    }

    private fun waitForOrder(
        userId: String,
        tableId: String,
        timeout: Long = 60_000,
        condition: (PendingOrder) -> Boolean,
    ): PendingOrder {
        var found: PendingOrder? = null
        waitUntil("el pedido de la mesa", timeout) {
            found = runBlocking {
                container.database
                    .waiter()
                    .ordersOf(userId)
                    .map(WaiterStorage::toOrder)
                    .lastOrNull { it.destination.tableId == tableId && condition(it) }
            }
            found != null
        }
        log("pedido de la mesa: ${found!!.state} intentos ${found!!.attempts}")
        return found!!
    }

    // --- La computadora principal ---

    private fun ok(method: String, path: String, body: String? = null): MainComputer.Answer {
        val answer = main.call(method, path, body)
        assertEquals("$method $path: ${answer.body}", 200, answer.status)
        return answer
    }

    private fun grantWaiterPermissions() {
        val roles = ok("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.text("name") == ROLE }
        val permissions = WAITER_PERMISSIONS.joinToString(",") { "\"$it\"" }
        ok("PATCH", "roles/${role.text("id")}", """{"name":"$ROLE","permissions":[$permissions]}""")
    }

    private fun createCatalog(): Map<String, String> {
        fun category(name: String, color: String): String =
            ok("POST", "catalog/categories", """{"name":"$name","active":true,"color":"$color"}""")
                .obj
                .text("id")
        val food = category("Comida F", "#d64545")
        val filler = category("Relleno F", "#719a26")
        fun product(category: String, name: String, code: String, price: String): String =
            ok(
                    "POST",
                    "catalog/products",
                    """{"name":"$name","active":true,"category_id":"$category",""" +
                        """"code":"$code","kind":"product","base_price":"$price"}""",
                )
                .obj
                .text("id")
        val ids =
            mutableMapOf(
                "category" to food,
                "burger" to product(food, "Hamburguesa F", "F-HAM", "120"),
                "tacos" to product(food, "Tacos F", "F-TAC", "90"),
                "shake" to product(food, "Malteada F", "F-MAL", "65"),
            )
        for (number in 1..FILLER_PRODUCTS) {
            product(filler, "Producto F $number", "F-REL-$number", "15")
        }
        return ids
    }

    /** Cocina en pantalla (sin impresora) para la categoría «Comida F»: lo enviado crea tareas. */
    private fun createKitchen(categoryId: String): Map<String, String> {
        val station =
            ok(
                    "POST",
                    "kitchen/stations",
                    """{"operation_id":"${UUID.randomUUID()}","name":"Cocina F","active":true,""" +
                        """"reception":"kds","target_minutes":12,"warning_minutes":15,""" +
                        """"overdue_minutes":25,"grouping":"inherit","tracking":"inherit"}""",
                )
                .obj
                .text("id")
        ok(
            "POST",
            "kitchen/routes",
            """{"operation_id":"${UUID.randomUUID()}","category_id":"$categoryId",""" +
                """"mode":"prepare","station_ids":["$station"]}""",
        )
        return mapOf("station" to station)
    }

    private fun createTables(): Map<String, String> {
        val area =
            ok(
                    "POST",
                    "restaurant/areas",
                    """{"operation_id":"${UUID.randomUUID()}","name":"Salón F","active":true}""",
                )
                .obj
                .text("id")
        val ids = mutableMapOf<String, String>()
        for (number in 1..TABLES) {
            val column = (number - 1) % 8
            val row = (number - 1) / 8
            val table =
                ok(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$area",""" +
                        """"name":"Mesa F$number","shape":"rectangle","x":${20 + column * 120},""" +
                        """"y":${20 + row * 105},"width":100,"height":72,"seats":4,"active":true}""",
                )
            ids["table$number"] = table.obj.text("id")
        }
        return ids
    }

    private fun bill(accountId: String): JsonObject =
        ok("GET", "restaurant/accounts/$accountId").obj

    private fun sentVersions(accountId: String): List<String> =
        bill(accountId).getValue("lines").jsonArray.map { it.jsonObject.text("version_id") }

    /** Quién envió a cocina cada pedido de la cuenta (historial de Go). */
    private fun confirmedBy(accountId: String): List<String> =
        ok("GET", "restaurant/accounts/$accountId/history")
            .obj
            .getValue("items")
            .jsonArray
            .map { it.jsonObject }
            .filter { it.text("kind") == "order.confirmed" }
            .map { it.text("author_name") }

    /** Las tareas de cocina de la cuenta (id y revisión), como las ve la pantalla de Cocina. */
    private fun kitchenTasks(accountId: String): List<Pair<String, Int>> =
        ok("GET", "kitchen/board?state=all&account_id=$accountId")
            .obj
            .getValue("tasks")
            .jsonArray
            .map { it.jsonObject }
            .filter { it.text("state") != "cancelled" }
            .map { it.text("id") to it.getValue("revision").jsonPrimitive.int }

    private fun setKitchenState(tasks: List<Pair<String, Int>>, state: String) {
        val references =
            tasks.joinToString(",") { (id, revision) -> """{"id":"$id","revision":$revision}""" }
        ok(
            "POST",
            "kitchen/states",
            """{"operation_id":"${UUID.randomUUID()}","state":"$state","tasks":[$references]}""",
        )
    }

    /** La principal abre una cuenta en la mesa con un producto en su borrador. */
    private fun openAccountFromMain(tableId: String, productId: String): String {
        val item =
            """{"id":"${UUID.randomUUID()}","product_id":"$productId","variant_id":null,""" +
                """"quantity":1,"customization":{"ingredients":[],"groups":[],"components":[],""" +
                """"notes":""}}"""
        val opened =
            ok(
                "POST",
                "restaurant/accounts",
                """{"operation_id":"${UUID.randomUUID()}","mode":"dine_in","table_id":"$tableId",""" +
                    """"seen_account_ids":[],"items":[$item]}""",
            )
        return opened.obj.getValue("account").jsonObject.text("id")
    }

    private fun revision(accountId: String): Int =
        bill(accountId).getValue("account").jsonObject.getValue("revision").jsonPrimitive.int

    private fun transfer(source: String, target: String, versionId: String) {
        ok(
            "POST",
            "restaurant/accounts/$source/transfer",
            """{"operation_id":"${UUID.randomUUID()}","revision":${revision(source)},""" +
                """"target_account_id":"$target","target_revision":${revision(target)},""" +
                """"selections":[{"version_id":"$versionId","quantity":1}]}""",
        )
    }

    private fun merge(source: String, target: String) {
        ok(
            "POST",
            "restaurant/accounts/$source/merge",
            """{"operation_id":"${UUID.randomUUID()}","revision":${revision(source)},""" +
                """"target_account_id":"$target","target_revision":${revision(target)}}""",
        )
    }

    /** Bloqueo por inactividad de este dispositivo (Administración › Terminales). */
    private fun setIdleLock(seconds: Int) {
        val device =
            ok("GET", "devices")
                .body
                .jsonArray
                .map { it.jsonObject }
                .single { it.text("kind") == "android" && it["revoked_at"] is JsonNull }
        val register = device["register_id"]?.takeUnless { it is JsonNull }?.jsonPrimitive?.content
        val registerField = if (register == null) "null" else "\"$register\""
        ok(
            "PATCH",
            "devices/${device.text("id")}",
            """{"name":"${device.text("name")}","register_id":$registerField,""" +
                """"keyboard_mode":"${device.text("keyboard_mode")}","idle_lock_seconds":$seconds}""",
        )
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    /**
     * La mesa [number] de f1. Con el argumento `tableOffset` se usan otras (para repetir un paso en
     * el mismo servidor sin chocar con las cuentas que dejó la corrida anterior).
     */
    private fun table(ids: Map<String, String>, number: Int): String {
        val offset = arguments.getString("tableOffset")?.toIntOrNull() ?: 0
        return ids.getValue("table${number + offset}")
    }

    // --- Estado entre pasos ---

    private fun saveIds(ids: Map<String, String>) {
        stateFile.writeText(JsonObject(ids.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun loadIds(): Map<String, String> {
        assumeTrue("Falta el estado de f1", stateFile.isFile)
        return Json.parseToJsonElement(stateFile.readText()).jsonObject.mapValues {
            it.value.jsonPrimitive.content
        }
    }

    private companion object {
        const val TAG = "AdelOnGo"
        const val ANA_USER = "ana.f"
        const val ANA_PIN = "4829"
        const val BETO_USER = "beto.f"
        const val BETO_PIN = "5713"
        const val WRONG_PIN = "9036"
        const val ROLE = "Meseros A1"
        const val TABLES = 40
        const val FILLER_PRODUCTS = 300
        const val IDLE_SECONDS = 30
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L
        const val OBSERVATION_WAIT_MILLIS = 5_000L
        const val MIN_TOUCH_DP = 48f
        const val TOUCH_TOLERANCE_DP = 0.5f

        val LAZY_LISTS =
            listOf(
                TAG_ORDER_PANEL,
                TAG_ACCOUNT_PANE,
                TAG_TABLES_LIST,
                TAG_PENDING,
                TAG_PRODUCT_GRID,
            )

        /** La lista de permisos de mesero de Go (A1-D17). */
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
