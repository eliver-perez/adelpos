package com.adelpos.cocina

import android.content.res.Configuration
import android.graphics.Bitmap
import android.os.Build
import android.view.View
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.cocina.access.DisplayConnection
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.cocina.ui.CocinaScreen
import com.adelpos.cocina.ui.TAG_ORDERS_CONNECTING
import com.adelpos.cocina.ui.TAG_ORDERS_EMPTY
import com.adelpos.core.model.access.RepairReason
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.pairing.PairingFlow
import com.adelpos.core.model.pairing.PairingProblem
import com.adelpos.core.model.pairing.PairingStep
import com.adelpos.core.model.pairing.PairingTarget
import com.adelpos.core.model.pairing.ServerSearch
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.status.TAG_REPAIR
import com.adelpos.core.ui.access.status.TAG_UPDATE_SERVER
import com.adelpos.core.ui.components.TAG_CONNECTION_STATE
import com.adelpos.core.ui.theme.AdelTheme
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.rules.TestRule
import org.junit.runner.RunWith

/**
 * Las pantallas de la aplicación de cocina de la Fase B de la etapa K1, en la tableta en
 * horizontal: la pantalla de pedidos vacía («No hay pedidos») con la barra y su indicador, las
 * pantallas compartidas de la entrada (buscar el servidor, emparejar de nuevo, actualizar el
 * servidor) y la aplicación real al abrirla sin emparejar: en horizontal, a pantalla completa y en
 * «Buscar el servidor» (con el tipo NSD de pruebas de `CocinaTestRunner`). Guarda una captura de
 * cada pantalla en `files/screens` de la aplicación.
 */
@RunWith(AndroidJUnit4::class)
class KitchenScreensTest {
    private val compose = createComposeRule()

    @get:Rule val rules: TestRule = RuleChain.outerRule(compose)

    // Emparejamiento que no encuentra nada (las pantallas no llaman a la red en estas pruebas).
    private val noPairing =
        object : PairingFlow {
            override suspend fun search(): ServerSearch = ServerSearch.Found(emptyList())

            override suspend fun start(target: PairingTarget, deviceName: String): PairingStep =
                PairingStep.Failed(PairingProblem.NoSession)

            override suspend fun poll(): PairingStep = PairingStep.Failed(PairingProblem.NoSession)

            override suspend fun rename(deviceName: String): PairingStep =
                PairingStep.Failed(PairingProblem.NoSession)

            override suspend fun cancel() = Unit
        }

    // Lo que se muestra: el contenido se pone una sola vez y cada paso solo cambia este estado.
    private class Shown(
        val status: KitchenStatus,
        val link: LinkState,
        val display: DisplayConnection,
    )

    private var shown: MutableState<Shown>? = null

    private fun show(
        status: KitchenStatus,
        link: LinkState = LinkState.ONLINE,
        display: DisplayConnection = DisplayConnection.Connected,
    ) {
        val next = Shown(status, link, display)
        val current = shown
        if (current != null) {
            compose.runOnIdle { current.value = next }
        } else {
            val state = mutableStateOf(next)
            shown = state
            compose.setContent {
                val now = state.value
                AdelTheme(darkTheme = true) {
                    CocinaScreen(
                        status = now.status,
                        link = now.link,
                        display = now.display,
                        pairing = noPairing,
                        onCheckAgain = {},
                        onRepair = {},
                    )
                }
            }
        }
        compose.waitForIdle()
    }

    @Test
    fun pantallaDePedidosVaciaConLaBarraYSuIndicador() {
        show(KitchenStatus.Ready(serverName = "Restaurante", deviceName = "Cocina 1"))
        compose.onNodeWithTag(TAG_ORDERS_EMPTY).assertIsDisplayed()
        compose.onNodeWithText("No hay pedidos").assertIsDisplayed()
        compose.onNodeWithText("Pedidos").assertIsDisplayed()
        compose.onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("En línea", useUnmergedTree = true).assertIsDisplayed()
        screenshot("b1-pedidos-vacia")
        show(
            KitchenStatus.Ready(serverName = "Restaurante", deviceName = "Cocina 1"),
            display = DisplayConnection.Connecting,
        )
        compose.onNodeWithTag(TAG_ORDERS_CONNECTING).assertIsDisplayed()
        compose.onNodeWithText("Conectando con la computadora principal…").assertIsDisplayed()
        screenshot("b1-pedidos-conectando")
    }

    @Test
    fun lasPantallasCompartidasDeLaEntrada() {
        show(KitchenStatus.Unpaired)
        compose.onNodeWithTag(TAG_FIND_SERVER).assertIsDisplayed()
        compose.onNodeWithText("Buscar el servidor").assertIsDisplayed()
        screenshot("b2-buscar-el-servidor")
        show(KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED))
        compose.onNodeWithTag(TAG_REPAIR).assertIsDisplayed()
        screenshot("b3-emparejar-de-nuevo")
        show(KitchenStatus.UpdateServer(serverVersion = "0.15.0"), link = LinkState.OFFLINE)
        compose.onNodeWithTag(TAG_UPDATE_SERVER).assertIsDisplayed()
        compose.onNodeWithText("Sin conexión", useUnmergedTree = true).assertIsDisplayed()
        screenshot("b4-actualizar-el-servidor")
    }

    private fun screenshot(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: return
        val folder = File(instrumentation.targetContext.getExternalFilesDir(null), "screens")
        folder.mkdirs()
        File(folder, "$name.png").outputStream().use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
    }
}

/**
 * La aplicación real al abrirla sin emparejar (instalación nueva): en horizontal, a pantalla
 * completa (sin barras del sistema) y en «Buscar el servidor».
 */
@RunWith(AndroidJUnit4::class)
class KitchenAppStartTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    @Test
    fun abreEnHorizontalAPantallaCompletaYBuscaElServidor() {
        compose.waitUntil(timeoutMillis = 15_000) {
            compose.onAllNodes(hasTestTag(TAG_FIND_SERVER)).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag(TAG_FIND_SERVER).assertIsDisplayed()
        val activity = compose.activity
        assertEquals(
            Configuration.ORIENTATION_LANDSCAPE,
            activity.resources.configuration.orientation,
        )
        // Sin la barra de estado: desde Android 11 lo dicen los insets; antes, la marca de pantalla
        // completa que pone WindowInsetsControllerCompat.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val insets = ViewCompat.getRootWindowInsets(activity.window.decorView)
            assertFalse(insets?.isVisible(WindowInsetsCompat.Type.statusBars()) ?: true)
        } else {
            @Suppress("DEPRECATION") val flags = activity.window.decorView.systemUiVisibility
            @Suppress("DEPRECATION") assertTrue(flags and View.SYSTEM_UI_FLAG_FULLSCREEN != 0)
        }
        screenshot("b0-inicio-sin-emparejar")
    }

    private fun screenshot(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: return
        val folder = File(instrumentation.targetContext.getExternalFilesDir(null), "screens")
        folder.mkdirs()
        File(folder, "$name.png").outputStream().use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
    }
}
