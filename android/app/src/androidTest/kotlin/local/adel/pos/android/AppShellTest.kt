package local.adel.pos.android

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.TAG_ACCESS_ENTER
import local.adel.pos.android.navigation.AdelPosNavigation
import local.adel.pos.android.navigation.TAG_APP_ROOT
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La interfaz completa con estados forzados: tema oscuro en todas las pantallas, indicador de
 * conexión y pendientes en la barra común y objetivos táctiles de 48 dp en todo lo que se toca (56
 * dp en «Entrar»).
 */
@RunWith(AndroidJUnit4::class)
class AppShellTest {
    @get:Rule val compose = createComposeRule()

    private var dark by mutableStateOf(true)
    private var connection by mutableStateOf(ConnectionSummary(LinkState.OFFLINE, 2))
    private lateinit var density: Density
    private var darkBackground = androidx.compose.ui.graphics.Color.Unspecified

    private fun show() {
        compose.setContent {
            density = LocalDensity.current
            AdelTheme(darkTheme = dark) {
                darkBackground = androidx.compose.material3.MaterialTheme.colorScheme.background
                AdelPosNavigation(connection)
            }
        }
    }

    @Test
    fun temaOscuroEIndicadorEnTodasLasPantallas() {
        show()
        compose.onNodeWithText("Sin conexión", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("2 pedidos por enviar", useUnmergedTree = true).assertIsDisplayed()
        assertDarkBackground()

        compose.onNodeWithTag(TAG_ACCESS_ENTER).performClick()
        compose.onNodeWithText("Sin conexión", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("2 pedidos por enviar", useUnmergedTree = true).assertIsDisplayed()
        assertDarkBackground()

        connection = ConnectionSummary(LinkState.ONLINE, 0)
        compose.onNodeWithText("En línea", useUnmergedTree = true).assertIsDisplayed()
    }

    @Test
    fun todoLoQueSeTocaMideAlMenos48Dp() {
        show()
        compose.onNodeWithTag(TAG_ACCESS_ENTER).assertHeightIsAtLeast(56.dp)
        assertTouchTargets()
        compose.onNodeWithTag(TAG_ACCESS_ENTER).performClick()
        compose.waitForIdle()
        assertTouchTargets()
    }

    private fun assertDarkBackground() {
        compose.waitForIdle()
        // Un punto del fondo, abajo a la izquierda, lejos de textos y botones.
        val image = compose.onNodeWithTag(TAG_APP_ROOT).captureToImage().toPixelMap()
        val pixel = image[4, image.height - 4]
        assertEquals(darkBackground, pixel)
    }

    private fun assertTouchTargets() {
        val minimum = with(density) { 48.dp.toPx() }
        val clickable = SemanticsMatcher.keyIsDefined(SemanticsActions.OnClick)
        val nodes = compose.onAllNodes(clickable).fetchSemanticsNodes()
        assertTrue("Debe haber controles", nodes.isNotEmpty())
        for (node in nodes) {
            val bounds: Rect = node.touchBoundsInRoot
            assertTrue(
                "Control de ${bounds.width} × ${bounds.height} px (< 48 dp): ${node.config}",
                bounds.width >= minimum - 1 && bounds.height >= minimum - 1,
            )
        }
    }
}
