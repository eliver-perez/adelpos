package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.ui.components.TAG_TOP_BAR_BACK
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.TAG_PANES_DOUBLE
import com.adelpos.meseros.core.ui.layout.TAG_PANES_SINGLE
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.core.ui.layout.paneLayoutFor
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_BAR
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_VIEW
import org.junit.After
import org.junit.Assert.assertTrue
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
    fun unoODosPanelesSegunElDispositivoYRotacionSinPerderLaVista() {
        assertPanes()
        // Etapa A3 (§4.7): una sola fila con «Lista | Mapa», las áreas y «Pendientes»; sin la
        // búsqueda de mesa. La vista elegida sobrevive a la rotación.
        compose.onNodeWithTag(TAG_TABLES_BAR).assertIsDisplayed()
        compose.onNodeWithTag(TAG_OPEN_PENDING).assertIsDisplayed()
        compose.onNodeWithTag("$TAG_TABLES_VIEW-map").performClick()
        compose.waitUntil(timeoutMillis = 10_000) { selected("$TAG_TABLES_VIEW-map") }
        rotate(quarterTurn = true)
        assertPanes()
        compose.waitUntil(timeoutMillis = 10_000) { selected("$TAG_TABLES_VIEW-map") }
        rotate(quarterTurn = false)
        assertPanes()
        assertTrue(selected("$TAG_TABLES_VIEW-map"))
        // De vuelta a la lista (la vista se guarda en el dispositivo para las demás pruebas).
        compose.onNodeWithTag("$TAG_TABLES_VIEW-list").performClick()
        compose.waitUntil(timeoutMillis = 10_000) { selected("$TAG_TABLES_VIEW-list") }
    }

    @Test
    fun laFlechaDeRegresoSaleDePendientesTambienDespuesDeGirar() {
        // Etapa A3 (§4.1, §4.6 y §4.7): sin «Ver la mesa» (tocar la mesa abre su panel); en Mesas
        // no hay flecha; en «Pendientes», sí, también al girar, y regresa a Mesas.
        assertTrue(absent(TAG_PANE_OPEN_SECONDARY))
        assertTrue(absent(TAG_TOP_BAR_BACK))
        compose.onNodeWithTag(TAG_OPEN_PENDING).performClick()
        compose.waitUntil(timeoutMillis = 10_000) { !absent(TAG_PENDING) }
        compose.onNodeWithTag(TAG_TOP_BAR_BACK).assertIsDisplayed()
        rotate(quarterTurn = true)
        compose.waitUntil(timeoutMillis = 10_000) { !absent(TAG_TOP_BAR_BACK) }
        compose.onNodeWithTag(TAG_PENDING).assertIsDisplayed()
        compose.onNodeWithTag(TAG_TOP_BAR_BACK).performClick()
        compose.waitUntil(timeoutMillis = 10_000) { !absent(TAG_TABLES_SCREEN) }
        assertTrue(absent(TAG_TOP_BAR_BACK))
        rotate(quarterTurn = false)
    }

    private fun absent(tag: String): Boolean =
        compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isEmpty()

    private fun selected(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag))
            .fetchSemanticsNodes()
            .firstOrNull()
            ?.config
            ?.getOrNull(SemanticsProperties.Selected) == true
}
