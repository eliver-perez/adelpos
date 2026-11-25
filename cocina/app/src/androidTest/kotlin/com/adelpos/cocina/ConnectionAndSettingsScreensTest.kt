package com.adelpos.cocina

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.cocina.settings.SettingsScreen
import com.adelpos.cocina.settings.SettingsTexts
import com.adelpos.cocina.ui.FRESH_STATE
import com.adelpos.cocina.ui.TAG_CARD_PREFIX
import com.adelpos.cocina.ui.TAG_CARD_READY_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_PREFIX
import com.adelpos.cocina.ui.TAG_OFFLINE
import com.adelpos.cocina.ui.TAG_SETTINGS
import com.adelpos.cocina.ui.TAG_SETTINGS_CONNECTION
import com.adelpos.cocina.ui.TAG_SETTINGS_ERROR
import com.adelpos.cocina.ui.TAG_SETTINGS_PASSWORD
import com.adelpos.cocina.ui.TAG_SETTINGS_SERVER
import com.adelpos.cocina.ui.TAG_SETTINGS_SUBMIT
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIRED
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR_CONFIRM
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR_QUESTION
import com.adelpos.cocina.ui.TAG_SETTINGS_USERNAME
import com.adelpos.cocina.ui.TAG_SETTINGS_VERSION
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.components.TAG_TOP_BAR_LOGO
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La Fase D en la pantalla, con estados preparados y en horizontal: «Sin conexión» (aviso grande,
 * nada se puede marcar), la animación de la tarjeta nueva y los ajustes de la tableta (pulsación
 * larga en el logotipo, contraseña, lo que muestran y «Volver a emparejar»). Guarda capturas en
 * `files/screens`.
 */
@RunWith(AndroidJUnit4::class)
class ConnectionAndSettingsScreensTest {
    @get:Rule val compose = createComposeRule()

    private val host = ScreenHost(compose)

    @Test
    fun sinConexionAvisoGrandeYNadaSePuedeMarcar() {
        val board = Fixtures.board(Fixtures.mixed()).copy(offline = true)
        host.show(board = board, link = LinkState.OFFLINE)
        compose.onNodeWithTag(TAG_OFFLINE).assertIsDisplayed()
        compose
            .onNodeWithText("Sin conexión con la computadora principal. Reconectando…")
            .assertIsDisplayed()
        // Los pedidos se siguen viendo, pero los botones están desactivados y tocar no marca.
        compose.onNodeWithText("3 × Tacos al pastor").assertIsDisplayed()
        compose.onNodeWithTag("${TAG_CARD_READY_PREFIX}mesa").assertIsNotEnabled()
        compose.onNodeWithTag("${TAG_DISH_PREFIX}tacos").performClick()
        compose.waitForIdle()
        assertTrue(host.actions.calls.isEmpty())
        screenshot("d1-sin-conexion")
        // Al volver, el aviso se va solo.
        host.show(board = board.copy(offline = false))
        assertTrue(
            compose.onAllNodes(hasTestTag(TAG_OFFLINE)).fetchSemanticsNodes().isEmpty(),
        )
    }

    @Test
    fun laTarjetaNuevaSeAnimaUnMomento() {
        compose.mainClock.autoAdvance = false
        val board = Fixtures.board(Fixtures.mixed()).copy(freshCards = mapOf("llevar" to 1L))
        host.show(board = board)
        // A media animación: el borde azul y el estado «nueva».
        compose.mainClock.advanceTimeBy(300)
        assertEquals(FRESH_STATE, stateOf("${TAG_CARD_PREFIX}llevar"))
        assertNull(stateOf("${TAG_CARD_PREFIX}mesa"))
        screenshot("d2-tarjeta-nueva")
        // Tres latidos en menos de 2 s y se detiene.
        compose.mainClock.advanceTimeBy(2_500)
        assertNull(stateOf("${TAG_CARD_PREFIX}llevar"))
        compose.mainClock.autoAdvance = true
    }

    @Test
    fun losAjustesSeAbrenConUnaPulsacionLargaEnElLogotipo() {
        host.show(board = Fixtures.board(Fixtures.mixed()))
        // Un toque normal no hace nada; la pulsación larga sí.
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO).performClick()
        compose.waitForIdle()
        assertTrue(host.settingsActions.calls.isEmpty())
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO).performTouchInput { longClick() }
        compose.waitForIdle()
        assertEquals(listOf("open"), host.settingsActions.calls)
        // Sin emparejar, el logotipo no abre nada.
        host.show(status = KitchenStatus.Unpaired)
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO).performTouchInput { longClick() }
        compose.waitForIdle()
        assertEquals(listOf("open"), host.settingsActions.calls)
    }

    @Test
    fun laContrasenaLoQueMuestranYVolverAEmparejar() {
        val board = Fixtures.board(Fixtures.mixed())
        host.show(board = board, settings = SettingsScreen.Password())
        compose.onNodeWithTag(TAG_SETTINGS).assertIsDisplayed()
        compose.onNodeWithTag(TAG_SETTINGS_USERNAME).performTextReplacement("admin")
        compose.onNodeWithTag(TAG_SETTINGS_PASSWORD).performTextReplacement("secreta")
        // La contraseña no se ve: el campo es de contraseña.
        assertTrue(
            compose
                .onNodeWithTag(TAG_SETTINGS_PASSWORD)
                .fetchSemanticsNode()
                .config
                .contains(SemanticsProperties.Password),
        )
        screenshot("d3-contrasena")
        compose.onNodeWithTag(TAG_SETTINGS_SUBMIT).performClick()
        compose.waitForIdle()
        assertEquals(listOf("submit admin/secreta"), host.settingsActions.calls)
        host.show(
            board = board,
            settings = SettingsScreen.Password(error = SettingsTexts.NOT_ALLOWED),
        )
        compose.onNodeWithTag(TAG_SETTINGS_ERROR).assertIsDisplayed()
        screenshot("d3-contrasena-sin-permiso")
        host.show(board = board, settings = SettingsScreen.Open())
        compose.onNodeWithTag(TAG_SETTINGS_SERVER).assertIsDisplayed()
        compose.onNodeWithText("192.168.1.20:8443").assertIsDisplayed()
        compose.onNodeWithTag(TAG_SETTINGS_CONNECTION).assertTextEquals("En línea")
        compose.onNodeWithTag(TAG_SETTINGS_VERSION).assertIsDisplayed()
        screenshot("d4-ajustes")
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIR).performClick()
        compose.waitForIdle()
        assertEquals("ask-unpair", host.settingsActions.calls.last())
        host.show(board = board, settings = SettingsScreen.Open(confirmUnpair = true))
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIR_QUESTION).assertIsDisplayed()
        screenshot("d5-volver-a-emparejar")
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIR_CONFIRM).performClick()
        compose.waitForIdle()
        assertEquals("confirm-unpair", host.settingsActions.calls.last())
        host.show(
            status = KitchenStatus.Unpaired,
            settings = SettingsScreen.Unpaired(SettingsTexts.UNPAIRED_NO_ANSWER),
        )
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIRED).assertIsDisplayed()
        screenshot("d6-revocala-en-terminales")
    }

    private fun stateOf(tag: String): String? =
        compose
            .onNodeWithTag(tag)
            .fetchSemanticsNode()
            .config
            .getOrNull(SemanticsProperties.StateDescription)
}
