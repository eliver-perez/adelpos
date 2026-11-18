package local.adel.pos.android

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.RepairReason
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.ui.components.TAG_WAITER_MENU
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.findserver.TAG_FIND_SERVER
import local.adel.pos.android.feature.access.pin.TAG_ENTRY
import local.adel.pos.android.feature.access.status.TAG_REPAIR
import local.adel.pos.android.feature.access.status.TAG_RETIRED
import local.adel.pos.android.feature.access.status.TAG_UPDATE_APP
import local.adel.pos.android.feature.access.status.TAG_UPDATE_SERVER
import local.adel.pos.android.feature.waiter.tables.TAG_TABLES_SEARCH
import local.adel.pos.android.navigation.AdelPosNavigation
import local.adel.pos.android.navigation.TAG_APP_ROOT
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La interfaz completa con estados forzados y una entrada falsa: cada pantalla de la entrada
 * (buscar el servidor, PIN, emparejar de nuevo, actualizar, retirado) y Mesas con el menú del
 * mesero, en tema oscuro, con el indicador de conexión y pendientes en la barra común y objetivos
 * táctiles de 48 dp en todo lo que se toca.
 */
@RunWith(AndroidJUnit4::class)
class AppShellTest {
    @get:Rule val compose = createComposeRule()

    private val fake = FakeAccess()
    private var status: DeviceStatus by mutableStateOf(DeviceStatus.Unpaired())
    private var waiter: ActiveWaiter? by mutableStateOf(null)
    private var connection by mutableStateOf(ConnectionSummary(LinkState.OFFLINE, 2))
    private lateinit var density: Density
    private var darkBackground = Color.Unspecified

    private fun show() {
        compose.setContent {
            density = LocalDensity.current
            AdelTheme(darkTheme = true) {
                darkBackground = MaterialTheme.colorScheme.background
                AdelPosNavigation(
                    connection = connection,
                    status = status,
                    activeWaiter = waiter,
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    onSwitchWaiter = {},
                )
            }
        }
    }

    private fun screens(): List<Pair<String, () -> Unit>> =
        listOf(
            TAG_FIND_SERVER to
                {
                    status = DeviceStatus.Unpaired(listOf(UnsentOrder("Ana", "Mesa 4: 2 tacos", 0)))
                },
            TAG_ENTRY to { status = DeviceStatus.Ready("Fonda", "Tableta 1", 120, online = true) },
            TAG_REPAIR to { status = DeviceStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED) },
            TAG_UPDATE_APP to { status = DeviceStatus.UpdateApp("0.0.9", "0.1.0") },
            TAG_UPDATE_SERVER to { status = DeviceStatus.UpdateServer("0.11.0") },
            TAG_RETIRED to
                {
                    status = DeviceStatus.Retired(listOf(UnsentOrder("Ana", "Mesa 4: 2 tacos", 0)))
                },
            TAG_TABLES_SEARCH to
                {
                    status = DeviceStatus.Ready("Fonda", "Tableta 1", 120, online = true)
                    waiter = ActiveWaiter(FakeAccess.SAMPLE_WAITERS[0], hasServerSession = false)
                },
        )

    @Test
    fun temaOscuroEIndicadorEnTodasLasPantallas() {
        show()
        for ((tag, apply) in screens()) {
            apply()
            compose.waitUntil(timeoutMillis = 10_000) {
                compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
            }
            compose.onNodeWithText("Sin conexión", useUnmergedTree = true).assertIsDisplayed()
            compose
                .onNodeWithText("2 pedidos por enviar", useUnmergedTree = true)
                .assertIsDisplayed()
            assertDarkBackground()
        }
        compose.onNodeWithTag(TAG_WAITER_MENU).assertIsDisplayed()
        connection = ConnectionSummary(LinkState.ONLINE, 0)
        compose.onNodeWithText("En línea", useUnmergedTree = true).assertIsDisplayed()
    }

    @Test
    fun todoLoQueSeTocaMideAlMenos48Dp() {
        show()
        for ((tag, apply) in screens()) {
            apply()
            compose.waitUntil(timeoutMillis = 10_000) {
                compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
            }
            compose.waitForIdle()
            assertTouchTargets(tag)
        }
    }

    private fun assertDarkBackground() {
        compose.waitForIdle()
        // Un punto del fondo, abajo a la izquierda, lejos de textos y botones.
        val image = compose.onNodeWithTag(TAG_APP_ROOT).captureToImage().toPixelMap()
        val pixel = image[4, image.height - 4]
        assertEquals(darkBackground, pixel)
    }

    private fun assertTouchTargets(screen: String) {
        val minimum = with(density) { 48.dp.toPx() }
        val clickable = SemanticsMatcher.keyIsDefined(SemanticsActions.OnClick)
        val nodes = compose.onAllNodes(clickable).fetchSemanticsNodes()
        assertTrue("Debe haber controles en $screen", nodes.isNotEmpty())
        for (node in nodes) {
            // El área táctil (que Material agranda hasta 48 dp) o, si el control quedó fuera de la
            // vista por el desplazamiento (área recortada a 0), su tamaño sin recortar.
            val bounds: Rect = node.touchBoundsInRoot
            val width = maxOf(bounds.width, node.size.width.toFloat())
            val height = maxOf(bounds.height, node.size.height.toFloat())
            assertTrue(
                "$screen: control de $width × $height px (< 48 dp): ${node.config}",
                width >= minimum - 1 && height >= minimum - 1,
            )
        }
    }
}
