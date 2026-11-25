package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.ui.components.TAG_TOP_BAR
import com.adelpos.meseros.core.capture.TAG_CAPTURE
import com.adelpos.meseros.core.data.waiter.db.WaiterPermissionsEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.ui.layout.NavigationPlacement
import com.adelpos.meseros.core.ui.layout.navigationPlacementFor
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_NEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.navigation.TAG_MAIN_NAVIGATION
import com.adelpos.meseros.navigation.TAG_NAVIGATION_RAIL
import com.adelpos.meseros.navigation.TAG_NAV_TABLES
import com.adelpos.meseros.navigation.TAG_NAV_TAKEAWAY
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La navegación principal «Mesas | Para llevar» (§3.1 de la A2) en el dispositivo real, en sus dos
 * orientaciones: barra inferior en vertical y riel lateral con poca altura o en horizontal
 * (`navigationPlacementFor`; hallazgo de la Fase E: en el teléfono horizontal la barra dejaba la
 * captura sin espacio). En el teléfono se prueban vertical y horizontal; en la tableta, horizontal
 * (su rotación natural) y vertical. Siempre a la vista, también en la captura, con zonas táctiles
 * de 48 dp.
 */
@RunWith(AndroidJUnit4::class)
class MainNavigationLayoutTest {
    @get:Rule val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<MainActivity>

    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation

    // Orientación con la rotación natural: vertical en el teléfono, horizontal en la tableta.
    private var naturalOrientation = Configuration.ORIENTATION_UNDEFINED

    private val waiter
        get() = TestDevice.app.container.database.waiter()

    @Before
    fun withTakeaway() {
        TestDevice.seedOfflineWaiter()
        runBlocking {
            waiter.savePermissions(
                WaiterPermissionsEntity(
                    userId = TestDevice.WAITER_ID,
                    permissions =
                        WaiterStorage.encodeStrings(
                            listOf(
                                "restaurant.tables.read",
                                "restaurant.areas.read",
                                "accounts.read",
                                "accounts.open",
                                "orders.capture",
                                "takeaway.read",
                                "takeaway.manage",
                            ),
                        ),
                    readAt = 1L,
                ),
            )
        }
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        scenario = ActivityScenario.launch(MainActivity::class.java)
        compose.waitUntil(timeoutMillis = 10_000) { configuration().orientation != 0 }
        compose.waitForIdle()
        naturalOrientation = configuration().orientation
        TestDevice.enter(compose)
        waitForTag(TAG_MAIN_NAVIGATION)
    }

    @After
    fun restore() {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        automation.setRotation(UiAutomation.ROTATION_UNFREEZE)
        scenario.close()
        runBlocking { waiter.deleteAllPermissions() }
        TestDevice.reset()
    }

    @Test
    fun barraEnVerticalYRielConPocaAlturaOEnHorizontal() {
        for (quarterTurn in listOf(false, true, false)) {
            rotate(quarterTurn)
            val placement = currentPlacement()
            assertPlacement(placement)
            // Cambiar de sección funciona igual con la barra y con el riel.
            compose.onNodeWithTag(TAG_NAV_TAKEAWAY).performClick()
            waitForTitle("Para llevar")
            waitForTag(TAG_TAKEAWAY_NEW)
            compose.onNodeWithTag(TAG_TAKEAWAY_NEW).performClick()
            waitForTitle("Nuevo pedido para llevar")
            waitForTag(TAG_CAPTURE)
            // También en la captura (siempre a la vista, §3.1) y en el mismo lugar.
            assertPlacement(placement)
            compose.onNodeWithTag(TAG_NAV_TABLES).performClick()
            waitForTitle("Mesas")
            compose.onNodeWithTag(TAG_TABLES_SCREEN).assertIsDisplayed()
        }
        // Un «Nuevo pedido» sin productos no deja nada, también al cambiar de sección.
        compose.waitUntil(timeoutMillis = 10_000) { runBlocking { waiter.allCaptures().isEmpty() } }
    }

    private fun assertPlacement(placement: NavigationPlacement) {
        compose.onNodeWithTag(TAG_MAIN_NAVIGATION).assertIsDisplayed()
        val rail = present(TAG_NAVIGATION_RAIL)
        assertEquals(
            "riel lateral en ${configuration().screenWidthDp}×${configuration().screenHeightDp} dp",
            placement == NavigationPlacement.SIDE_RAIL,
            rail,
        )
        val navigation =
            compose.onNodeWithTag(TAG_MAIN_NAVIGATION).fetchSemanticsNode().boundsInRoot
        if (rail) {
            // A un lado: más alto que ancho.
            assertTrue("el riel va a un lado", navigation.height > navigation.width)
        } else {
            // Abajo: más ancha que alta.
            assertTrue("la barra va abajo", navigation.width > navigation.height)
        }
        for (tag in listOf(TAG_NAV_TABLES, TAG_NAV_TAKEAWAY)) {
            compose
                .onNodeWithTag(tag)
                .assertIsDisplayed()
                .assertHeightIsAtLeast(48.dp)
                .assertWidthIsAtLeast(48.dp)
        }
    }

    // Configuración de la actividad vigente (cambia de objeto con cada rotación).
    private fun configuration(): Configuration {
        var current: Configuration? = null
        scenario.onActivity { activity ->
            current = Configuration(activity.resources.configuration)
        }
        return requireNotNull(current)
    }

    private fun currentPlacement(): NavigationPlacement {
        val configuration = configuration()
        return navigationPlacementFor(configuration.screenWidthDp, configuration.screenHeightDp)
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

    private fun present(tag: String): Boolean =
        compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()

    private fun waitForTag(tag: String) {
        compose.waitUntil(timeoutMillis = 30_000) { present(tag) }
    }

    private fun waitForTitle(title: String) {
        compose.waitUntil(timeoutMillis = 30_000) {
            compose
                .onAllNodes(hasText(title).and(hasAnyAncestor(hasTestTag(TAG_TOP_BAR))))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }
}
