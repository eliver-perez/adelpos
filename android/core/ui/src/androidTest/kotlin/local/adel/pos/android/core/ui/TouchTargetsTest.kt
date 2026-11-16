package local.adel.pos.android.core.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.components.SecondaryButton
import local.adel.pos.android.core.ui.icons.AdelIcons
import local.adel.pos.android.core.ui.theme.AdelTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Botones táctiles (§2): 56 dp las acciones principales y 48 dp como mínimo las demás, aun con
 * texto corto.
 */
@RunWith(AndroidJUnit4::class)
class TouchTargetsTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun principales56YSecundarios48() {
        compose.setContent {
            AdelTheme {
                Column {
                    PrimaryButton(
                        text = "Enviar a cocina",
                        onClick = {},
                        modifier = Modifier.testTag("primary"),
                    )
                    PrimaryButton(
                        text = "Sí",
                        onClick = {},
                        modifier = Modifier.testTag("short-primary"),
                    )
                    SecondaryButton(
                        text = "Ver cuenta",
                        onClick = {},
                        icon = AdelIcons.Receipt,
                        modifier = Modifier.testTag("secondary"),
                    )
                    SecondaryButton(
                        text = "No",
                        onClick = {},
                        modifier = Modifier.testTag("short-secondary"),
                    )
                }
            }
        }
        compose.onNodeWithTag("primary").assertHeightIsAtLeast(56.dp)
        compose
            .onNodeWithTag("short-primary")
            .assertHeightIsAtLeast(56.dp)
            .assertWidthIsAtLeast(48.dp)
        compose.onNodeWithTag("secondary").assertHeightIsAtLeast(48.dp)
        compose
            .onNodeWithTag("short-secondary")
            .assertHeightIsAtLeast(48.dp)
            .assertWidthIsAtLeast(48.dp)
    }
}
