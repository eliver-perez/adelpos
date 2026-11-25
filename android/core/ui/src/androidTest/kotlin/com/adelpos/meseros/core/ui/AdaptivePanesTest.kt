package com.adelpos.meseros.core.ui

import androidx.activity.ComponentActivity
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.ui.layout.AdaptivePanes
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.TAG_PANES_DOUBLE
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Andamio de uno o dos paneles con la disposición forzada (independiente del tamaño del
 * dispositivo).
 */
@RunWith(AndroidJUnit4::class)
class AdaptivePanesTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    private var showSecondary by mutableStateOf(false)

    private fun show(layout: PaneLayout) {
        compose.setContent {
            AdelTheme {
                AdaptivePanes(
                    layout = layout,
                    showSecondary = showSecondary,
                    onShowSecondaryChange = { showSecondary = it },
                    primaryTitle = "Mesas",
                    openSecondaryLabel = "Ver cuenta",
                    primary = { Text("Lista de mesas") },
                    secondary = { Text("Detalle de la cuenta") },
                )
            }
        }
    }

    @Test
    fun dosPanelesMuestranAmbosASuVez() {
        show(PaneLayout.DOUBLE)
        compose.onNodeWithTag(TAG_PANES_DOUBLE).assertIsDisplayed()
        compose.onNodeWithText("Lista de mesas").assertIsDisplayed()
        compose.onNodeWithText("Detalle de la cuenta").assertIsDisplayed()
        compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).assertDoesNotExist()
    }

    @Test
    fun unPanelConElOtroAUnToqueYAtrasDelSistema() {
        show(PaneLayout.SINGLE)
        compose.onNodeWithText("Lista de mesas").assertIsDisplayed()
        compose.onNodeWithText("Detalle de la cuenta").assertDoesNotExist()

        compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        compose.onNodeWithText("Detalle de la cuenta").assertIsDisplayed()
        compose.onNodeWithText("Lista de mesas").assertDoesNotExist()

        // Atrás del sistema (el mismo despachador que usa el gesto y el atrás predictivo) vuelve al
        // primer panel.
        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.onNodeWithText("Lista de mesas").assertIsDisplayed()

        compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        compose.onNodeWithTag(TAG_PANE_BACK).performClick()
        compose.onNodeWithText("Lista de mesas").assertIsDisplayed()
    }
}
