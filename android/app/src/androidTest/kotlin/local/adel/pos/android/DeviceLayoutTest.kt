package local.adel.pos.android

import android.app.UiAutomation
import android.content.res.Configuration
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.layout.TAG_PANES_DOUBLE
import local.adel.pos.android.core.ui.layout.TAG_PANES_SINGLE
import local.adel.pos.android.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import local.adel.pos.android.core.ui.layout.paneLayoutFor
import local.adel.pos.android.feature.access.TAG_ACCESS_ENTER
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
 * cuando el mesero gira la tableta.
 */
@RunWith(AndroidJUnit4::class)
class DeviceLayoutTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation

    // Orientación con la rotación natural: vertical en el teléfono, horizontal en la tableta.
    private var naturalOrientation = Configuration.ORIENTATION_UNDEFINED

    @Before
    fun naturalRotation() {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        compose.waitUntil(timeoutMillis = 10_000) {
            compose.activity.resources.configuration.orientation != 0
        }
        compose.waitForIdle()
        naturalOrientation = compose.activity.resources.configuration.orientation
    }

    @After
    fun restoreRotation() {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        automation.setRotation(UiAutomation.ROTATION_UNFREEZE)
    }

    private fun currentLayout(): PaneLayout {
        val configuration = compose.activity.resources.configuration
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
        compose.waitUntil(timeoutMillis = 10_000) {
            compose.activity.resources.configuration.orientation == expected
        }
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
        compose.onNodeWithTag(TAG_ACCESS_ENTER).performClick()
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
        compose.onNodeWithTag(TAG_ACCESS_ENTER).performClick()
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
