package com.adelpos.cocina

import android.graphics.Bitmap
import android.util.Log
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.cocina.ui.TAG_ORDERS
import com.adelpos.cocina.ui.TAG_ORDERS_EMPTY
import com.adelpos.core.testing.MainComputer
import com.adelpos.core.testing.SystemDialogs
import com.adelpos.core.ui.access.findserver.TAG_ADDRESS
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.findserver.TAG_PAIRING_CODE
import java.io.File
import java.io.FileOutputStream
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La aplicación de cocina contra el Go temporal del arnés (etapa K1, suite `kitchen` de
 * `scripts/android-e2e.sh`, un paso por instrumentación con la aplicación instalada una sola vez):
 * - `k1`: empareja la tableta como **pantalla de cocina** escribiendo la dirección (la prueba hace
 *   de computadora principal por la escucha local y aprueba el código); Go la registra con el tipo
 *   `kitchen`.
 * - `k2`: con un proceso nuevo, la pantalla emparejada se conecta sola (`kitchen-display/connect`,
 *   su cupo de cocina) y muestra la pantalla de pedidos vacía, «No hay pedidos».
 *
 * Argumentos del guion: `local`, `address`, `pin`, `mainDevice` y `mainToken`. Sin ellos, se omite.
 * Nunca contra el servicio real de la Mac. Capturas en `files/screens` (el guion las copia).
 */
@RunWith(AndroidJUnit4::class)
class KitchenOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val arguments = InstrumentationRegistry.getArguments()
    private var scenario: ActivityScenario<MainActivity>? = null

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    private val main by lazy {
        MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
    }

    @After
    fun close() {
        scenario?.close()
    }

    @Test
    fun k1_emparejarComoPantallaDeCocina() {
        launch()
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
        screenshot("k1-codigo")
        val approved = main.approvePairing(shown.replace(" ", ""))
        assertEquals(approved.body.toString(), 200, approved.status)
        waitFor(TAG_ORDERS)
        // Go la registró como pantalla de cocina (K1-D3), con la versión de esta aplicación.
        val devices = main.call("GET", "devices").body.jsonArray.map { it.jsonObject }
        val kitchen = devices.filter { it.getValue("kind").jsonPrimitive.content == "kitchen" }
        assertEquals("Una pantalla de cocina emparejada: $devices", 1, kitchen.size)
        log("k1 emparejada como pantalla de cocina: ${kitchen.single()}")
    }

    @Test
    fun k2_conectarYPantallaDePedidosVacia() {
        // Sin el Go temporal del guion se omite, como k1.
        argument("address")
        launch()
        // Emparejada: abre directo en la pantalla de pedidos, se conecta sola y no hay pedidos.
        waitFor(TAG_ORDERS)
        waitFor(TAG_ORDERS_EMPTY)
        screenshot("k2-pedidos-vacia")
        log("k2 conectada: «No hay pedidos»")
    }

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun waitFor(tag: String, timeout: Long = 60_000) {
        try {
            compose.waitUntil(timeoutMillis = timeout) { present(tag) }
        } catch (failure: Throwable) {
            compose.onRoot(useUnmergedTree = true).printToLog(TAG)
            throw failure
        }
    }

    private fun screenshot(name: String) {
        SystemDialogs.dismissNotResponding(instrumentation.uiAutomation)
        compose.waitForIdle()
        Thread.sleep(SETTLE_MILLIS)
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: return
        val folder =
            File(instrumentation.targetContext.getExternalFilesDir(null), "screens").apply {
                mkdirs()
            }
        FileOutputStream(File(folder, "$name.png")).use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
        log("captura $name")
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private companion object {
        // La etiqueta que copia el guion de cada paso (android-e2e.sh, run_step).
        const val TAG = "AdelOnGo"
        const val SETTLE_MILLIS = 500L
    }
}
