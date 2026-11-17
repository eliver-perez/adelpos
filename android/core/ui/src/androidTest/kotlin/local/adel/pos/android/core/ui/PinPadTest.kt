package local.adel.pos.android.core.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertContentDescriptionEquals
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.ui.components.ActiveWaiterMenu
import local.adel.pos.android.core.ui.components.PinPad
import local.adel.pos.android.core.ui.components.TAG_PIN_DELETE
import local.adel.pos.android.core.ui.components.TAG_PIN_DOTS
import local.adel.pos.android.core.ui.components.TAG_PIN_KEY_PREFIX
import local.adel.pos.android.core.ui.components.TAG_PIN_SUBMIT
import local.adel.pos.android.core.ui.components.TAG_WAITER_MENU
import local.adel.pos.android.core.ui.components.TAG_WAITER_SWITCH
import local.adel.pos.android.core.ui.theme.AdelTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Teclado del PIN y menú del mesero activo (componentes de `core:ui`), en ambos temas: teclas
 * grandes, «Entrar» desde 4 dígitos y hasta 6, «Borrar», y el cambio rápido de mesero.
 */
@RunWith(AndroidJUnit4::class)
class PinPadTest {
    @get:Rule val compose = createComposeRule()

    private var pin by mutableStateOf("")
    private var dark by mutableStateOf(false)
    private var submitted = ""

    private fun show() {
        compose.setContent {
            AdelTheme(darkTheme = dark) {
                PinPad(
                    length = pin.length,
                    onDigit = { digit -> pin += digit },
                    onDelete = { pin = pin.dropLast(1) },
                    onSubmit = { submitted = pin },
                    submitLabel = "Entrar",
                )
            }
        }
    }

    private fun type(digits: String) {
        for (digit in digits) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
    }

    @Test
    fun entrarDesdeCuatroDigitosYComoMaximoSeis() {
        show()
        for (theme in listOf(false, true)) {
            dark = theme
            pin = ""
            compose.onNodeWithTag(TAG_PIN_SUBMIT).assertIsNotEnabled()
            compose.onNodeWithTag(TAG_PIN_DELETE).assertIsNotEnabled()
            type("482")
            compose.onNodeWithTag(TAG_PIN_SUBMIT).assertIsNotEnabled()
            type("9")
            compose.onNodeWithTag(TAG_PIN_SUBMIT).assertIsEnabled()
            compose.onNodeWithTag(TAG_PIN_DOTS).assertContentDescriptionEquals("4 dígitos escritos")
            type("1234")
            assertEquals("no pasa de 6 dígitos", "482912", pin)
            compose.onNodeWithTag("${TAG_PIN_KEY_PREFIX}5").assertIsNotEnabled()
            compose.onNodeWithTag(TAG_PIN_DELETE).performClick()
            assertEquals("48291", pin)
            compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
            assertEquals("48291", submitted)
        }
    }

    @Test
    fun lasTeclasSonGrandes() {
        show()
        for (digit in "0123456789") {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").assertHeightIsAtLeast(72.dp)
        }
        compose.onNodeWithTag(TAG_PIN_DELETE).assertHeightIsAtLeast(72.dp)
        compose.onNodeWithTag(TAG_PIN_SUBMIT).assertHeightIsAtLeast(72.dp)
    }

    @Test
    fun elMenuDelMeseroCambiaDeMesero() {
        var switched = 0
        compose.setContent {
            AdelTheme(darkTheme = true) {
                ActiveWaiterMenu(
                    name = "Ana",
                    initial = "A",
                    colorHex = "#3591d4",
                    onSwitchWaiter = { switched += 1 },
                    offline = true,
                )
            }
        }
        compose.onNodeWithTag(TAG_WAITER_MENU).assertHeightIsAtLeast(48.dp).performClick()
        compose.onNodeWithText("Entraste sin conexión").assertIsDisplayed()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        assertEquals(1, switched)
    }
}
