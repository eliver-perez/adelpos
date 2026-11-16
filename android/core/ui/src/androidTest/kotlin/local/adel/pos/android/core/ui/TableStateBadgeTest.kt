package local.adel.pos.android.core.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.domain.table.TableState
import local.adel.pos.android.core.ui.components.TAG_TABLE_STATE_ICON_PREFIX
import local.adel.pos.android.core.ui.components.TAG_TABLE_STATE_PREFIX
import local.adel.pos.android.core.ui.components.TableStateBadge
import local.adel.pos.android.core.ui.theme.AdelTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Cada estado de mesa se reconoce por su texto y su ícono (los del escritorio), además de su color.
 */
@RunWith(AndroidJUnit4::class)
class TableStateBadgeTest {
    @get:Rule val compose = createComposeRule()

    private val expected =
        mapOf(
            TableState.FREE to ("Libre" to "check"),
            TableState.OCCUPIED to ("Ocupada" to "clock"),
            TableState.AWAITING_PAYMENT to ("Esperando pago" to "receipt"),
            TableState.PARTIAL_PAYMENT to ("Pago parcial" to "cash"),
            TableState.OUT_OF_SERVICE to ("Fuera de servicio" to "lock"),
        )

    @Test
    fun textoEIconoDeCadaEstadoEnAmbosTemas() {
        var dark by mutableStateOf(false)
        compose.setContent {
            AdelTheme(darkTheme = dark) {
                Column { TableState.entries.forEach { TableStateBadge(it) } }
            }
        }
        for (theme in listOf(false, true)) {
            dark = theme
            compose.waitForIdle()
            for ((state, labelAndIcon) in expected) {
                val (label, icon) = labelAndIcon
                val badge = hasTestTag("$TAG_TABLE_STATE_PREFIX${state.wireName}")
                compose
                    .onNodeWithTag("$TAG_TABLE_STATE_PREFIX${state.wireName}")
                    .assertIsDisplayed()
                compose
                    .onNode(
                        hasTestTag("$TAG_TABLE_STATE_ICON_PREFIX$icon").and(hasAnyAncestor(badge)),
                        useUnmergedTree = true,
                    )
                    .assertIsDisplayed()
                compose.onNodeWithText(label, useUnmergedTree = true).assertIsDisplayed()
            }
        }
    }
}
