package local.adel.pos.android

import android.app.UiAutomation
import android.content.res.Configuration
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.layout.TAG_PANES_DOUBLE
import local.adel.pos.android.core.ui.layout.TAG_PANES_SINGLE
import local.adel.pos.android.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import local.adel.pos.android.core.ui.layout.paneLayoutFor
import local.adel.pos.android.feature.waiter.TAG_ACCOUNT_PANE
import local.adel.pos.android.feature.waiter.TAG_TABLES_SEARCH
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La pantalla Mesas en el dispositivo real: uno o dos paneles según su tamaño y **rotación sin
 * perder lo capturado** (la búsqueda escrita y, en el teléfono, el panel de la cuenta abierto). La
 * rotación es la del dispositivo (no se simula): la actividad se destruye y se vuelve a crear como
 * cuando el mesero gira la tableta. Se llega a Mesas entrando con el PIN de un mesero conocido, sin
 * conexión ([TestDevice]); el mesero sigue dentro después de cada rotación.
 */
@RunWith(AndroidJUnit4::class)
class DeviceLayoutTest {
    @get:Rule val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<MainActivity>

    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation

    // Orientación con la rotación natural: vertical en el teléfono, horizontal en la tableta.
    private var naturalOrientation = Configuration.ORIENTATION_UNDEFINED

    @Before
    fun naturalRotation() {
        TestDevice.seedOfflineWaiter()
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        scenario = ActivityScenario.launch(MainActivity::class.java)
        compose.waitUntil(timeoutMillis = 10_000) { configuration().orientation != 0 }
        compose.waitForIdle()
        naturalOrientation = configuration().orientation
        TestDevice.enter(compose)
    }

    @After
    fun restoreRotation() {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        automation.setRotation(UiAutomation.ROTATION_UNFREEZE)
        scenario.close()
        TestDevice.reset()
    }

    // Configuración de la actividad vigente (cambia de objeto con cada rotación).
    private fun configuration(): Configuration {
        var current: Configuration? = null
        scenario.onActivity { activity ->
            current = Configuration(activity.resources.configuration)
        }
        return requireNotNull(current)
    }

    private fun currentLayout(): PaneLayout {
        val configuration = configuration()
        return paneLayoutFor(configuration.screenWidthDp, configuration.screenHeightDp)
    }

    private fun rotate(quarterTurn: Boolean) {
        val expected =
            when {
                !quarterTurn -> naturalOrientation
                naturalOrientation == Configuration.ORIENTATION_PORTRAIT ->
                    Configuration.ORIENTATION_LANDSCAPE
                else -> Configuration.ORIENTATION_PORTRAIT
            }
        automation.setRotation(
            if (quarterTurn) UiAutomation.ROTATION_FREEZE_90 else UiAutomation.ROTATION_FREEZE_0,
        )
        compose.waitUntil(timeoutMillis = 10_000) { configuration().orientation == expected }
        compose.waitForIdle()
    }

    private fun assertPanes() {
        when (currentLayout()) {
            PaneLayout.DOUBLE -> {
                compose.onNodeWithTag(TAG_PANES_DOUBLE).assertIsDisplayed()
                compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertIsDisplayed()
            }
            PaneLayout.SINGLE -> compose.onNodeWithTag(TAG_PANES_SINGLE).assertIsDisplayed()
        }
    }

    @Test
    fun unoODosPanelesSegunElDispositivoYRotacionSinPerderLoCapturado() {
        assertPanes()
        compose.onNodeWithTag(TAG_TABLES_SEARCH).performTextInput("Terraza 4")

        rotate(quarterTurn = true)
        assertPanes()
        compose.onNodeWithTag(TAG_TABLES_SEARCH).assertTextContains("Terraza 4")

        rotate(quarterTurn = false)
        assertPanes()
        compose.onNodeWithTag(TAG_TABLES_SEARCH).assertTextContains("Terraza 4")
    }

    @Test
    fun enUnPanelLaCuentaAbiertaSigueAbiertaAlRotar() {
        if (currentLayout() == PaneLayout.DOUBLE) {
            // Tableta: la cuenta siempre está a la vista; no hay panel que abrir.
            compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertIsDisplayed()
            return
        }
        compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertIsDisplayed()
        rotate(quarterTurn = true)
        // Teléfono horizontal: sigue en un panel (muy bajo para dos) y con la cuenta abierta.
        assertEquals(PaneLayout.SINGLE, currentLayout())
        compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertIsDisplayed()
        rotate(quarterTurn = false)
        compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertIsDisplayed()
    }
}
