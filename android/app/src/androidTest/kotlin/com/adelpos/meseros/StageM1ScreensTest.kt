package com.adelpos.meseros

import android.graphics.Bitmap
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.model.connection.ConnectionSummary
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.components.AdelTopBar
import com.adelpos.core.ui.components.TAG_PENDING_ORDERS
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.Totals
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.components.ActiveWaiterMenu
import com.adelpos.meseros.core.ui.components.PendingOrdersCompact
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.feature.access.pin.ReenterPinDialog
import com.adelpos.meseros.feature.access.pin.TAG_REENTER
import com.adelpos.meseros.feature.waiter.tables.AccountCard
import com.adelpos.meseros.feature.waiter.tables.AccountTools
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREAS
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREA_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_BAR
import com.adelpos.meseros.feature.waiter.tables.TablesActions
import com.adelpos.meseros.feature.waiter.tables.TablesScreen
import com.adelpos.meseros.feature.waiter.tables.TablesState
import java.io.File
import java.io.FileOutputStream
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa M1, §5.6, con estados forzados y anchos fijos (el teléfono lo cubre la Fase F con
 * `Pixel_8`; aquí, los anchos del teléfono dentro de la tableta):
 * - la etiqueta más larga de la cocina («Enviado · sin confirmar en cocina») completa en la tarjeta
 *   del panel de la tableta vertical (antes «…en coci…»);
 * - las pestañas de las áreas nunca cortadas en la orilla (antes «Terraz…»);
 * - en la barra del teléfono vertical, «N pedidos por enviar» completo junto al mesero y «En línea»
 *   (antes «2 pedidos por en…»);
 * - «Vuelve a escribir tu PIN» en el tema claro, con su captura (el contraste está medido en el
 *   avance).
 */
@RunWith(AndroidJUnit4::class)
class StageM1ScreensTest {
    @get:Rule val compose = createComposeRule()

    /** Ningún renglón del texto termina en puntos suspensivos. */
    private fun complete(node: SemanticsNodeInteraction): Boolean {
        val layouts = mutableListOf<TextLayoutResult>()
        node
            .fetchSemanticsNode()
            .config[SemanticsActions.GetTextLayoutResult]
            .action
            ?.invoke(layouts)
        val layout = layouts.single()
        return (0 until layout.lineCount).none { layout.isLineEllipsized(it) }
    }

    private val account =
        AccountSummary(
            id = "c6",
            folio = 12,
            name = "Mesa 6",
            tableId = "t6",
            tableName = "Mesa 6",
            state = AccountSummary.STATE_OPEN,
            revision = 3,
            preparationState = PreparationState.PENDING_RELEASE,
            confirmedQuantity = 2,
            draftQuantity = 0,
            totals = Totals(Money("755.00"), Money("0.00"), Money("755.00")),
            assignedName = null,
            elapsedSeconds = 600,
            longOpen = false,
            assignedUserId = "ana",
        )

    @Test
    fun laEtiquetaMasLargaSeLeeCompletaEnLaTarjeta() {
        var width by mutableIntStateOf(280)
        compose.setContent {
            AdelTheme {
                Box(modifier = Modifier.width(width.dp)) {
                    AccountCard(
                        account = account,
                        permissions = WaiterPermissions.UNKNOWN,
                        tools = AccountTools.NONE,
                        enabled = true,
                        onOpen = {},
                        onPrintPrecheck = {},
                        onRetryPrecheck = {},
                        onDiscardEmpty = {},
                        mine = true,
                    )
                }
            }
        }
        // 304 dp es el panel de la tableta vertical sin sus márgenes; 280, un teléfono angosto.
        for (each in listOf(280, 304, 360, 600)) {
            width = each
            compose.waitForIdle()
            val badge =
                compose.onNode(hasText("Enviado · sin confirmar en cocina"), useUnmergedTree = true)
            assertTrue("la etiqueta completa a $each dp", complete(badge))
        }
    }

    @Test
    fun lasPestanasDeLasAreasNuncaQuedanCortadas() {
        var width by mutableIntStateOf(432)
        compose.setContent {
            AdelTheme {
                Box(modifier = Modifier.width(width.dp).height(600.dp)) {
                    TablesScreen(
                        layout = PaneLayout.SINGLE,
                        state =
                            TablesState(
                                view = restaurant(),
                                permissions = WaiterPermissions.UNKNOWN,
                                status = RestaurantStatus(false, true, null),
                                offline = false,
                                tablesView = TablesView.LIST,
                                now = 0,
                                areaId = null,
                            ),
                        selected = null,
                        selectedAccounts = emptyList(),
                        captures = emptyList(),
                        orders = emptyList(),
                        showingTable = false,
                        opening = false,
                        actions =
                            TablesActions(
                                onTablesViewChange = {},
                                onAreaSelect = {},
                                onTableSelect = {},
                                onShowingTableChange = {},
                                onRefresh = {},
                                onOpenPending = {},
                                onOpenAccount = { _, _ -> },
                                onOpenCapture = {},
                                onOpenOrder = {},
                            ),
                    )
                }
            }
        }
        // 432 dp: el panel izquierdo de la tableta vertical; 360: un teléfono; 840: horizontal.
        for (each in listOf(432, 360, 840)) {
            width = each
            compose.waitForIdle()
            val row = compose.onNodeWithTag(TAG_TABLES_AREAS).fetchSemanticsNode().boundsInRoot
            for (area in listOf("salon", "terraza")) {
                val tab =
                    compose
                        .onNodeWithTag("$TAG_TABLES_AREA_PREFIX$area")
                        .fetchSemanticsNode()
                        .boundsInRoot
                assertTrue(
                    "la pestaña $area completa a $each dp ($tab dentro de $row)",
                    tab.left >= row.left - 1f && tab.right <= row.right + 1f,
                )
            }
            val bar = compose.onNodeWithTag(TAG_TABLES_BAR).fetchSemanticsNode().boundsInRoot
            assertTrue("las pestañas dentro de la fila de arriba", row.bottom <= bar.bottom + 1f)
        }
    }

    @Test
    fun enLaBarraDelTelefonoLosPedidosPorEnviarSeLeenCompletos() {
        var width by mutableIntStateOf(360)
        compose.setContent {
            AdelTheme {
                Box(modifier = Modifier.width(width.dp)) {
                    // Como la raíz en el teléfono vertical: el mesero solo con su inicial y «N
                    // pedidos por enviar» de meseros (la barra común recibe 0).
                    AdelTopBar(
                        title = "Pendientes",
                        connection = ConnectionSummary(LinkState.ONLINE, pendingOrders = 0),
                        onBack = {},
                        actions = {
                            ActiveWaiterMenu(
                                name = "Ana",
                                initial = "A",
                                colorHex = null,
                                onSwitchWaiter = {},
                                compact = true,
                            )
                            PendingOrdersCompact(2)
                        },
                    )
                }
            }
        }
        for (each in listOf(360, 411)) {
            width = each
            compose.waitForIdle()
            val text = compose.onNode(hasText("2 pedidos por enviar"), useUnmergedTree = true)
            assertTrue("«2 pedidos por enviar» completo a $each dp", complete(text))
            assertFalse(
                "una sola insignia de pedidos",
                compose.onAllNodesWithTag(TAG_PENDING_ORDERS).fetchSemanticsNodes().size != 1,
            )
            compose.onNode(hasText("En línea"), useUnmergedTree = true).assertExists()
        }
    }

    @Test
    fun vuelveAEscribirTuPinEnElTemaClaro() {
        compose.setContent {
            AdelTheme(darkTheme = false) {
                ReenterPinDialog(
                    waiter =
                        ActiveWaiter(
                            WaiterProfile("ana", "Ana", "A", null, 1, null),
                            hasServerSession = false,
                            mustReenterPin = true,
                        ),
                    pinLength = 2,
                    busy = false,
                    message = null,
                    onDigit = {},
                    onDelete = {},
                    onSubmit = {},
                    onSwitchWaiter = {},
                )
            }
        }
        compose.onNodeWithTag(TAG_REENTER).assertExists()
        compose.waitForIdle()
        // La pantalla completa (captureToImage no toma diálogos antes de Android 9).
        val shot = InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()
        if (shot != null) {
            save(shot, "m1-pin-claro")
        }
    }

    private fun save(bitmap: Bitmap, name: String) {
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val folder = File(app.getExternalFilesDir(null), "screens").apply { mkdirs() }
        FileOutputStream(File(folder, "$name.png")).use { output ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
        }
    }

    private fun restaurant() =
        RestaurantView(
            areas =
                listOf(
                    DiningArea("salon", "Salón M1", 1, true),
                    DiningArea("terraza", "Terraza M1", 2, true),
                ),
            tables = listOf(table("t1", "salon", "Mesa 5"), table("t2", "terraza", "Terraza 1")),
            settings = null,
            readAtElapsedMillis = 0,
        )

    private fun table(id: String, area: String, name: String) =
        DiningTable(
            id = id,
            areaId = area,
            name = name,
            shape = TableShape.RECTANGLE,
            x = 40,
            y = 40,
            width = 100,
            height = 72,
            seats = 4,
            active = true,
            outOfService = false,
            accountCount = 0,
            state = TableState.FREE,
            occupiedSeconds = 0,
            balance = Money("0.00"),
            longOpen = false,
        )
}
