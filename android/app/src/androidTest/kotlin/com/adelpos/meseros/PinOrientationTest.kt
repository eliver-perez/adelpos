package com.adelpos.meseros

import android.app.UiAutomation
import android.content.res.Configuration
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.feature.access.pin.TAG_PIN_PANEL
import com.adelpos.meseros.feature.access.pin.TAG_REENTER
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3 (§4): el teclado del PIN completo en vertical y en horizontal, con la rotación real del
 * dispositivo (`UiAutomation.setRotation`; la actividad se vuelve a crear). En el teléfono
 * horizontal antes solo se veían las teclas 1 a 6: el panel del PIN de la entrada y el diálogo
 * «Vuelve a escribir tu PIN» van ahí en dos columnas. Se entra sin conexión con el mesero conocido
 * de [TestDevice] (su PIN tiene 8 y 9, teclas que antes quedaban fuera de la vista).
 */
@RunWith(AndroidJUnit4::class)
class PinOrientationTest {
    @get:Rule val compose = createEmptyComposeRule()

    private lateinit var scenario: ActivityScenario<MainActivity>

    private val automation: UiAutomation = InstrumentationRegistry.getInstrumentation().uiAutomation

    // Orientación con la rotación natural: vertical en el teléfono, horizontal en la tableta.
    private var naturalOrientation = Configuration.ORIENTATION_UNDEFINED

    @Before
    fun launch() {
        TestDevice.seedOfflineWaiter()
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        scenario = ActivityScenario.launch(MainActivity::class.java)
        compose.waitUntil(timeoutMillis = 10_000) { orientation() != 0 }
        compose.waitForIdle()
        naturalOrientation = orientation()
    }

    @After
    fun restoreRotation() {
        automation.setRotation(UiAutomation.ROTATION_FREEZE_0)
        automation.setRotation(UiAutomation.ROTATION_UNFREEZE)
        scenario.close()
        TestDevice.reset()
    }

    @Test
    fun elTecladoDelPinSeVeCompletoEnVerticalYHorizontal() {
        // La entrada: en la orientación natural, girada y de vuelta.
        for (quarterTurn in listOf(false, true, false)) {
            rotate(quarterTurn)
            showPinPanel()
            PinPadChecks.assertWholePad(compose, TAG_PIN_PANEL, where(quarterTurn, "la entrada"))
        }
        // Girado (en el teléfono, horizontal) se escribe el PIN completo y se entra.
        rotate(quarterTurn = true)
        showPinPanel()
        typePin()
        waitForTag(TAG_TABLES_SCREEN)

        // «Vuelve a escribir tu PIN» encima de Mesas, también completo en las dos orientaciones.
        TestDevice.app.container.deviceAccess.report(
            AppError.SessionEnded("SESSION_EXPIRED", null, null),
            TestDevice.WAITER_ID,
        )
        for (quarterTurn in listOf(true, false)) {
            rotate(quarterTurn)
            waitForTag(TAG_REENTER)
            PinPadChecks.assertWholePad(compose, TAG_REENTER, where(quarterTurn, "el diálogo"))
        }
        rotate(quarterTurn = true)
        waitForTag(TAG_REENTER)
        typePin()
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(TAG_REENTER)).fetchSemanticsNodes().isEmpty()
        }
        assertFalse(TestDevice.app.container.deviceAccess.activeWaiter.value!!.mustReenterPin)
    }

    private fun where(quarterTurn: Boolean, screen: String): String {
        val landscape = orientation() == Configuration.ORIENTATION_LANDSCAPE
        val name = if (landscape) "horizontal" else "vertical"
        return "$screen en $name${if (quarterTurn) " (girado)" else ""}"
    }

    /** El panel del PIN del mesero (tras una rotación la elección puede no seguir). */
    private fun showPinPanel() {
        val waiter = "$TAG_WAITER_PREFIX${TestDevice.WAITER_ID}"
        compose.waitUntil(timeoutMillis = 30_000) { present(waiter) || present(TAG_PIN_SUBMIT) }
        if (!present(TAG_PIN_SUBMIT)) {
            compose.onNodeWithTag(waiter).performClick()
        }
        waitForTag(TAG_PIN_SUBMIT)
    }

    private fun typePin() {
        for (digit in TestDevice.PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
    }

    private fun orientation(): Int {
        var current = 0
        scenario.onActivity { activity -> current = activity.resources.configuration.orientation }
        return current
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
        compose.waitUntil(timeoutMillis = 10_000) { orientation() == expected }
        compose.waitForIdle()
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun waitForTag(tag: String) {
        compose.waitUntil(timeoutMillis = 30_000) { present(tag) }
    }
}
