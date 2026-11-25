package com.adelpos.meseros

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertContentDescriptionEquals
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.core.model.connection.ConnectionSummary
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.components.AdelTopBar
import com.adelpos.core.ui.components.TAG_TOP_BAR_BACK
import com.adelpos.core.ui.components.TAG_TOP_BAR_LOGO
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.capture.TAG_PRECHECK_NOT_ALLOWED
import com.adelpos.meseros.core.capture.TAG_PRECHECK_PRINT
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.ProductAvailability
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.Totals
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.catalog.CatalogSearchField
import com.adelpos.meseros.core.ui.catalog.ProductCard
import com.adelpos.meseros.core.ui.catalog.TAG_CATALOG_SEARCH
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_WITHOUT_IMAGE
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.feature.access.AccessContent
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_BACK
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.AccountCard
import com.adelpos.meseros.feature.waiter.tables.AccountTools
import com.adelpos.meseros.feature.waiter.tables.DiscardEmptyDialog
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_TITLE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_DISCARD_EMPTY_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_DISCARD_EMPTY_TEXT
import com.adelpos.meseros.feature.waiter.tables.TAG_MAP_CANVAS
import com.adelpos.meseros.feature.waiter.tables.TAG_OPEN_PENDING
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREAS
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_AREA_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_BAR
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_MAP
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_VIEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TablesActions
import com.adelpos.meseros.feature.waiter.tables.TablesScreen
import com.adelpos.meseros.feature.waiter.tables.TablesState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3, §4, con estados forzados: la tarjeta de cuenta del panel en dos renglones en todos los
 * anchos (320–840 dp, §4.8) con «Imprimir precuenta» (§4.9) y «Descartar cuenta vacía» (§4.10), la
 * flecha y el logotipo de la barra (§4.1, §4.5), el buscador de una línea (§4.3), la tarjeta de
 * producto sin imagen (§4.11), la entrada con sus textos (§4.5) y Mesas compacta con el mapa que
 * llena el espacio (§4.7). La rotación real con la aplicación completa la prueba
 * `AdjustmentsOnGoTest` (`--suite adjustments`).
 */
@RunWith(AndroidJUnit4::class)
class StageA3ScreensTest {
    @get:Rule val compose = createComposeRule()

    private lateinit var density: Density

    private fun lines(node: SemanticsNodeInteraction): Int {
        val layouts = mutableListOf<TextLayoutResult>()
        node
            .fetchSemanticsNode()
            .config[SemanticsActions.GetTextLayoutResult]
            .action
            ?.invoke(
                layouts,
            )
        return layouts.single().lineCount
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
            // La etiqueta más larga del modelo: la que antes partía la tarjeta letra por letra.
            preparationState = PreparationState.CONFIRMED_PENDING_DISPATCH,
            confirmedQuantity = 2,
            draftQuantity = 0,
            totals = Totals(Money("755.00"), Money("0.00"), Money("755.00")),
            assignedName = null,
            elapsedSeconds = 600,
            longOpen = false,
            discardable = true,
        )

    @Test
    fun laTarjetaDeCuentaVaEnDosRenglonesEnTodosLosAnchos() {
        var width by mutableIntStateOf(320)
        var permissions by mutableStateOf(WaiterPermissions.UNKNOWN)
        compose.setContent {
            density = LocalDensity.current
            AdelTheme {
                Box(modifier = Modifier.width(width.dp)) {
                    AccountCard(
                        account = account,
                        permissions = permissions,
                        tools = AccountTools.NONE,
                        enabled = true,
                        onOpen = {},
                        onPrintPrecheck = {},
                        onRetryPrecheck = {},
                        onDiscardEmpty = {},
                    )
                }
            }
        }
        for (each in listOf(320, 360, 400, 600, 840)) {
            width = each
            compose.waitForIdle()
            // El encabezado se puede tocar (abre la cuenta) y junta sus textos: se busca el título
            // en el árbol sin juntar.
            val title =
                compose.onNodeWithTag(
                    "$TAG_ACCOUNT_TITLE_PREFIX${account.id}",
                    useUnmergedTree = true,
                )
            assertEquals("el título en un renglón a $each dp", 1, lines(title))
            // La etiqueta corta abajo, también en un solo renglón.
            val badge = compose.onNode(hasText("Pendiente de liberación"), useUnmergedTree = true)
            assertEquals("la etiqueta en un renglón a $each dp", 1, lines(badge))
            val titleBounds = title.fetchSemanticsNode().boundsInRoot
            val badgeBounds = badge.fetchSemanticsNode().boundsInRoot
            assertTrue("la etiqueta abajo del título", badgeBounds.top >= titleBounds.bottom)
            // Siempre se ven «Imprimir precuenta» y, con discardable de Go, «Descartar cuenta
            // vacía»; sin conexión, deshabilitados con su motivo (no ocultos).
            compose.onNodeWithTag(TAG_PRECHECK_PRINT).assertIsDisplayed().assertIsNotEnabled()
            compose.onNodeWithTag("$TAG_DISCARD_EMPTY_PREFIX${account.id}").assertIsNotEnabled()
            compose
                .onNodeWithText("Para imprimir necesitas conexión", substring = true)
                .assertExists()
        }
        // Sin el permiso de Go: el botón con su explicación.
        permissions = WaiterPermissions(setOf("accounts.read"))
        compose.waitForIdle()
        compose.onNodeWithTag(TAG_PRECHECK_NOT_ALLOWED, useUnmergedTree = true).assertExists()
        compose
            .onNodeWithText("Tu usuario no tiene permiso para descartar cuentas", substring = true)
            .assertExists()
    }

    @Test
    fun descartarDiceLoQueSeBorraYEsperaLaCapturaDeEsteDispositivo() {
        // Con una captura de esta cuenta en el dispositivo, el botón espera con su motivo.
        compose.setContent {
            AdelTheme {
                AccountCard(
                    account = account,
                    permissions = WaiterPermissions.UNKNOWN,
                    tools =
                        AccountTools(
                            precheckSetup = PrecheckSetup.UNKNOWN,
                            printingAccountId = null,
                            printing = PrecheckPrintState.IDLE,
                            canWriteNow = true,
                            discardingAccountId = null,
                            discardResult = null,
                        ),
                    enabled = true,
                    onOpen = {},
                    onPrintPrecheck = {},
                    onRetryPrecheck = {},
                    onDiscardEmpty = {},
                    workHere = true,
                )
                // La confirmación dice cuántos productos sin enviar se borran (draft_quantity).
                DiscardEmptyDialog(
                    account = account.copy(draftQuantity = 2),
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }
        compose.onNodeWithTag("$TAG_DISCARD_EMPTY_PREFIX${account.id}").assertIsNotEnabled()
        compose.onNodeWithText("Tienes una captura de esta cuenta", substring = true).assertExists()
        compose
            .onNodeWithTag(TAG_DISCARD_EMPTY_TEXT)
            .assertTextContains("Se borrarán 2 productos sin enviar.", substring = true)
            .assertTextContains("No es una cancelación", substring = true)
    }

    @Test
    fun laBarraTieneFlechaDeRegresoYLogotipo() {
        var backs = 0
        compose.setContent {
            AdelTheme(darkTheme = true) {
                AdelTopBar(
                    title = "Ingresar",
                    connection = ConnectionSummary(LinkState.ONLINE, 0),
                    onBack = { backs++ },
                    logo = true,
                )
            }
        }
        compose
            .onNodeWithTag(TAG_TOP_BAR_BACK)
            .assertContentDescriptionEquals("Volver")
            .assertHeightIsAtLeast(48.dp)
            .performClick()
        assertEquals(1, backs)
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO, useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("Ingresar").assertIsDisplayed()
    }

    @Test
    fun elBuscadorDeProductosEsDeUnaLinea() {
        compose.setContent {
            density = LocalDensity.current
            AdelTheme {
                Box(modifier = Modifier.width(220.dp)) {
                    CatalogSearchField(text = "", onTextChange = {})
                }
            }
        }
        val label = compose.onNode(hasText("Buscar producto"), useUnmergedTree = true)
        assertEquals(1, lines(label))
        val field = compose.onNodeWithTag(TAG_CATALOG_SEARCH).fetchSemanticsNode()
        assertTrue("una línea de alto", field.size.height <= with(density) { 72.dp.toPx() })
    }

    @Test
    fun laTarjetaDeProductoSinImagenEsCompacta() {
        val withoutImage = product("p1", imageId = null)
        val withImage = product("p2", imageId = "img-2")
        compose.setContent {
            density = LocalDensity.current
            AdelTheme {
                androidx.compose.foundation.layout.Row {
                    Box(modifier = Modifier.width(160.dp)) {
                        ProductCard(product = withoutImage, images = null, onClick = {})
                    }
                    Box(modifier = Modifier.width(160.dp)) {
                        ProductCard(product = withImage, images = null, onClick = {})
                    }
                }
            }
        }
        compose.onNodeWithTag(TAG_PRODUCT_WITHOUT_IMAGE, useUnmergedTree = true).assertIsDisplayed()
        val compact = compose.onNodeWithTag(productTag("p1")).fetchSemanticsNode().size.height
        val full = compose.onNodeWithTag(productTag("p2")).fetchSemanticsNode().size.height
        assertTrue("sin imagen es más baja ($compact < $full)", compact < full)
        compose.onNodeWithTag(productTag("p1")).assertHeightIsAtLeast(56.dp)
    }

    @Test
    fun laEntradaDiceSeleccionaTuUsuarioEIngresar() {
        val fake = FakeAccess()
        compose.setContent {
            AdelTheme {
                AccessContent(
                    status = DeviceStatus.Ready("Fonda", "Teléfono 1", 120),
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    layout = PaneLayout.SINGLE,
                )
            }
        }
        compose.waitUntil(timeoutMillis = 10_000) {
            compose
                .onAllNodes(hasTestTag("${TAG_WAITER_PREFIX}ana"))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        compose.onNodeWithText("Selecciona tu usuario").assertIsDisplayed()
        compose.onNodeWithText("Selecciona tu usuario y escribe tu PIN.").assertIsDisplayed()
        compose.onNodeWithTag("${TAG_WAITER_PREFIX}ana").performClick()
        compose.onNodeWithText("Ingresar").assertExists()
        compose.onNodeWithTag(TAG_ENTRY_BACK).assertExists()
        compose.onNodeWithText("Elegir otro usuario").assertExists()
    }

    @Test
    fun mesasCompactaEnUnaFilaYElMapaLlenaElEspacio() {
        var view by mutableStateOf(TablesView.LIST)
        var area by mutableStateOf<String?>(null)
        compose.setContent {
            AdelTheme {
                // La mitad de una tableta vertical: donde el mapa se veía diminuto.
                Box(modifier = Modifier.width(400.dp).height(900.dp)) {
                    TablesScreen(
                        layout = PaneLayout.SINGLE,
                        state =
                            TablesState(
                                view = restaurant(),
                                permissions = WaiterPermissions.UNKNOWN,
                                status = RestaurantStatus(false, true, null),
                                offline = false,
                                tablesView = view,
                                now = 0,
                                areaId = area,
                            ),
                        selected = null,
                        selectedAccounts = emptyList(),
                        captures = emptyList(),
                        orders = emptyList(),
                        showingTable = false,
                        opening = false,
                        actions =
                            TablesActions(
                                onTablesViewChange = { view = it },
                                onAreaSelect = { area = it },
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
        // Una sola fila: «Lista | Mapa», las áreas y «Pendientes», a la misma altura.
        val bar = compose.onNodeWithTag(TAG_TABLES_BAR).fetchSemanticsNode().boundsInRoot
        for (tag in
            listOf("$TAG_TABLES_VIEW-list", "${TAG_TABLES_AREA_PREFIX}salon", TAG_OPEN_PENDING)) {
            val bounds = compose.onNodeWithTag(tag).fetchSemanticsNode().boundsInRoot
            assertTrue(
                "$tag en la fila",
                bounds.top >= bar.top - 1 && bounds.bottom <= bar.bottom + 1,
            )
        }
        // Las áreas como pestañas: la lista muestra solo la elegida.
        compose.onNodeWithTag("${TAG_TABLE_PREFIX}t1").assertExists()
        // Las pestañas se desplazan si no caben: se lleva la de Terraza a la vista antes de
        // tocarla.
        compose.onNodeWithTag("${TAG_TABLES_AREA_PREFIX}terraza").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("${TAG_TABLE_PREFIX}t3").assertExists()
        assertTrue(
            compose.onAllNodes(hasTestTag("${TAG_TABLE_PREFIX}t1")).fetchSemanticsNodes().isEmpty(),
        )
        // El mapa llena el espacio (antes, el lienzo completo de 1000 × 560 escalado al ancho).
        compose.onNodeWithTag("$TAG_TABLES_VIEW-map").performClick()
        compose.waitForIdle()
        val map = compose.onNodeWithTag(TAG_TABLES_MAP).fetchSemanticsNode().boundsInRoot
        val canvas = compose.onNodeWithTag(TAG_MAP_CANVAS).fetchSemanticsNode().boundsInRoot
        assertTrue("el lienzo llena el alto del mapa", canvas.height >= map.height * 0.9f)
        val table = compose.onNodeWithTag("map-table-t3").fetchSemanticsNode().boundsInRoot
        val old = map.width / 1000f * 100f
        assertTrue("la mesa más grande que antes (${table.width} > $old)", table.width > old * 1.5f)
        // La pestaña elegida se trae a la vista completa aunque la fila esté deslizada a otra.
        area = "salon"
        compose.waitForIdle()
        val tabs = compose.onNodeWithTag(TAG_TABLES_AREAS).fetchSemanticsNode().boundsInRoot
        val salon =
            compose
                .onNodeWithTag("${TAG_TABLES_AREA_PREFIX}salon")
                .fetchSemanticsNode()
                .boundsInRoot
        assertTrue(
            "la pestaña elegida completa ($salon dentro de $tabs)",
            salon.left >= tabs.left - 1 && salon.right <= tabs.right + 1,
        )
    }

    private fun restaurant() =
        RestaurantView(
            areas =
                listOf(
                    DiningArea("salon", "Salón", 1, true),
                    DiningArea("terraza", "Terraza", 2, true),
                ),
            tables =
                listOf(
                    table("t1", "salon", "Mesa 1", 40, 40),
                    table("t2", "salon", "Mesa 2", 300, 40),
                    table("t3", "terraza", "Terraza 1", 100, 80),
                    table("t4", "terraza", "Terraza 2", 400, 300),
                ),
            settings = null,
            readAtElapsedMillis = 0,
        )

    private fun table(id: String, area: String, name: String, x: Int, y: Int) =
        DiningTable(
            id = id,
            areaId = area,
            name = name,
            shape = TableShape.RECTANGLE,
            x = x,
            y = y,
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

    private fun product(id: String, imageId: String?) =
        CatalogProduct(
            id = id,
            name = "Agua de horchata grande",
            description = "",
            active = true,
            categoryId = "bebidas",
            code = id,
            barcode = null,
            kind = ProductKind.PRODUCT,
            referencePrice = Money("35.00"),
            pricingError = null,
            imageId = imageId,
            sortOrder = 0,
            variants = emptyList(),
            ingredients = emptyList(),
            groups = emptyList(),
            slots = emptyList(),
            availability = ProductAvailability(true, "", false, false, null),
        )
}
