package com.adelpos.meseros

import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.capture.pending.PendingOrdersScreen
import com.adelpos.meseros.core.capture.pending.ReviewScreen
import com.adelpos.meseros.core.capture.pending.TAG_PENDING
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_CREDENTIAL_PREFIX
import com.adelpos.meseros.core.capture.pending.TAG_PENDING_OTHERS
import com.adelpos.meseros.core.capture.pending.TAG_RETRY_NOW
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_ACCEPT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_NEW_ACCOUNT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_REMOVE_PREFIX
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.ChangeKind
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OrderChange
import com.adelpos.meseros.core.domain.waiter.OrderDestination
import com.adelpos.meseros.core.domain.waiter.OrderLine
import com.adelpos.meseros.core.domain.waiter.OrderReview
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.OthersPending
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.TAG_PANES_DOUBLE
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.ui.waiter.OrderStateBadge
import com.adelpos.meseros.core.ui.waiter.TAG_LONG_OPEN
import com.adelpos.meseros.core.ui.waiter.TAG_ORDER_STATE_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT_NOTE
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
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
 * Las pantallas del modo Mesero con datos fijos, en tema claro y oscuro: los cinco estados de un
 * pedido siempre distintos, «Pedidos pendientes» con «Reintentar ya», «Requiere tu revisión» que
 * solo se acepta con cambios aceptables, y Mesas con la mesa en uno o dos paneles.
 */
@RunWith(AndroidJUnit4::class)
class WaiterScreensTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private fun show(dark: Boolean, content: @Composable () -> Unit) {
        compose.activity.runOnUiThread {
            compose.activity.setContent { AdelTheme(darkTheme = dark) { content() } }
        }
        compose.waitForIdle()
    }

    @Test
    fun losCincoEstadosSonDistintosEnAmbosTemas() {
        for (dark in listOf(false, true)) {
            show(dark) { Column { OrderState.entries.forEach { OrderStateBadge(it) } } }
            val texts =
                OrderState.entries.map { state ->
                    val node =
                        compose
                            .onNodeWithTag("$TAG_ORDER_STATE_PREFIX${state.storedName}")
                            .fetchSemanticsNode()
                    node.config.toString()
                }
            assertEquals(5, texts.toSet().size)
            compose.onNodeWithText("Recibido por el restaurante").assertExists()
            compose.onNodeWithText("Requiere tu revisión").assertExists()
            compose.onNodeWithText("Por enviar").assertExists()
            compose.onNodeWithText("Enviando").assertExists()
            compose.onNodeWithText("Borrador").assertExists()
        }
    }

    @Test
    fun pedidosPendientesConSuEstadoYReintentarYa() {
        var retried = 0
        val queued = order("op-1", OrderState.QUEUED).copy(credentialRejected = true)
        show(dark = true) {
            PendingOrdersScreen(
                orders =
                    listOf(
                        queued,
                        order("op-2", OrderState.SENDING),
                        order("op-3", OrderState.RECEIVED).copy(receivedAtMillis = 2_000),
                        order("op-4", OrderState.NEEDS_REVIEW)
                            .copy(review = review(acceptable = true)),
                    ),
                captures = listOf(capture()),
                others =
                    listOf(OthersPending("Beto", unsent = 2, waitingForPin = 1, needingReview = 0)),
                onRetryNow = { retried++ },
                onOpenCapture = {},
                onOpenReview = {},
            )
        }
        for (state in OrderState.entries) {
            compose
                .onNodeWithTag("$TAG_ORDER_STATE_PREFIX${state.storedName}", useUnmergedTree = true)
                .assertExists()
        }
        compose
            .onNodeWithTag("${TAG_PENDING_CREDENTIAL_PREFIX}op-1", useUnmergedTree = true)
            .assertExists()
        // Al final de la lista (perezosa): se desplaza hasta el aviso de los demás meseros.
        compose.onNodeWithTag(TAG_PENDING).performScrollToNode(hasTestTag(TAG_PENDING_OTHERS))
        compose.onNodeWithTag(TAG_PENDING_OTHERS).assertExists()
        // De los demás: cuántos y por qué no salen (revisión R6, B3), nunca cuáles.
        compose
            .onNodeWithTag(TAG_PENDING)
            .performScrollToNode(hasText("Beto: 2 pedidos por enviar"))
        compose.onNodeWithText("Beto: 2 pedidos por enviar").assertExists()
        compose.onNodeWithText("Su clave para enviar ya no vale", substring = true).assertExists()
        compose.onNodeWithTag(TAG_PENDING).performScrollToNode(hasTestTag(TAG_RETRY_NOW))
        compose.onNodeWithTag(TAG_RETRY_NOW).assertIsEnabled().performClick()
        assertEquals(1, retried)
        // Sin nada «Por enviar», no hay qué reintentar.
        show(dark = false) {
            PendingOrdersScreen(
                orders = listOf(order("op-3", OrderState.RECEIVED)),
                captures = emptyList(),
                others = emptyList(),
                onRetryNow = { retried++ },
                onOpenCapture = {},
                onOpenReview = {},
            )
        }
        compose.onNodeWithTag(TAG_RETRY_NOW).assertIsNotEnabled()
    }

    @Test
    fun laRevisionSoloSeAceptaConCambiosAceptables() {
        var removed: String? = null
        show(dark = false) {
            ReviewScreen(
                order =
                    order("op-1", OrderState.NEEDS_REVIEW)
                        .copy(review = review(acceptable = false)),
                working = false,
                onAccept = {},
                onResend = {},
                onRemoveLine = { removed = it },
                onNewAccount = {},
                onBackToCapture = {},
                onDiscard = {},
            )
        }
        compose.onNodeWithText("Malteada de fresa: agotado. Quítalo del pedido.").assertExists()
        compose.onNodeWithTag(TAG_REVIEW_ACCEPT).assertDoesNotExist()
        compose.onNodeWithTag("${TAG_REVIEW_REMOVE_PREFIX}r1").performClick()
        assertEquals("r1", removed)

        show(dark = true) {
            ReviewScreen(
                order =
                    order("op-1", OrderState.NEEDS_REVIEW).copy(review = review(acceptable = true)),
                working = false,
                onAccept = {},
                onResend = {},
                onRemoveLine = {},
                onNewAccount = {},
                onBackToCapture = {},
                onDiscard = {},
            )
        }
        compose.onNodeWithText("Hamburguesa: $120.00 → $130.00").assertExists()
        compose.onNodeWithTag(TAG_REVIEW_ACCEPT).assertIsEnabled()
        compose.onNodeWithTag(TAG_REVIEW_NEW_ACCOUNT).assertDoesNotExist()

        val closed =
            OrderReview(
                reason = ReviewReason.ACCOUNT_NOT_OPEN,
                message = "La cuenta «Mesa 4» ya se cobró. No admite nuevas operaciones.",
                changes = emptyList(),
                catalogVersion = null,
            )
        show(dark = false) {
            ReviewScreen(
                order = order("op-1", OrderState.NEEDS_REVIEW).copy(review = closed),
                working = false,
                onAccept = {},
                onResend = {},
                onRemoveLine = {},
                onNewAccount = {},
                onBackToCapture = {},
                onDiscard = {},
            )
        }
        compose
            .onNodeWithText("La cuenta «Mesa 4» ya se cobró. No admite nuevas operaciones.")
            .assertExists()
        compose.onNodeWithTag(TAG_REVIEW_NEW_ACCOUNT).assertExists()
    }

    @Test
    fun mesasConLaMesaEnUnoODosPanelesYSinPermisoParaAbrir() {
        for ((layout, dark) in listOf(PaneLayout.DOUBLE to false, PaneLayout.SINGLE to true)) {
            var showing = false
            show(dark) {
                TablesScreen(
                    layout = layout,
                    state = tablesState(),
                    selected = table(),
                    selectedAccounts = emptyList(),
                    captures = emptyList(),
                    orders = emptyList(),
                    showingTable = showing,
                    opening = false,
                    actions = actions { showing = it },
                )
            }
            compose.onNodeWithTag(TAG_TABLES_LIST).assertExists()
            compose.onNodeWithTag("${TAG_TABLE_PREFIX}t1").assertExists()
            compose.onNodeWithTag(TAG_LONG_OPEN, useUnmergedTree = true).assertExists()
            if (layout == PaneLayout.DOUBLE) {
                compose.onNodeWithTag(TAG_PANES_DOUBLE).assertExists()
                compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertExists()
                // Sin accounts.open, «Nueva cuenta» deshabilitada con su motivo.
                compose.onNodeWithTag(TAG_NEW_ACCOUNT).assertIsNotEnabled()
                compose.onNodeWithText("Sin permiso para abrir cuentas.").assertExists()
                compose.onNodeWithTag(TAG_NEW_ACCOUNT_NOTE).assertExists()
            } else {
                compose.onNodeWithTag(TAG_ACCOUNT_PANE).assertDoesNotExist()
                compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
                assertTrue(showing)
            }
        }
    }

    // --- Datos fijos ---

    private fun line(id: String, productId: String) =
        DraftLine(id, LineSelection(productId, null, 1, Customization.ORIGINAL))

    private fun order(operationId: String, state: OrderState) =
        PendingOrder(
            operationId = operationId,
            waiterUserId = "ana",
            waiterName = "Ana",
            state = state,
            destination = OrderDestination("t1", "Mesa 4", null, null, emptyList()),
            lines =
                listOf(
                    OrderLine(line("r1", "malteada"), "Malteada de fresa", Money("65.00")),
                    OrderLine(line("r2", "hamburguesa"), "Hamburguesa", Money("120.00")),
                ),
            replaces = emptyList(),
            catalogVersion = 41,
            catalogEpoch = null,
            capturedAtMillis = 1_000,
            confirmation = null,
            review = null,
            attempts = 0,
            lastError = null,
            credentialRejected = false,
            onlineConfirm = null,
            receivedAccountId = null,
            receivedAtMillis = null,
            updatedAtMillis = 1_000,
        )

    private fun review(acceptable: Boolean): OrderReview {
        val price =
            OrderChange(
                kind = ChangeKind.PRICE_CHANGED,
                wireKind = "price_changed",
                itemId = "r2",
                productId = "hamburguesa",
                productName = "Hamburguesa",
                before = "120.00",
                now = "130.00",
                message = "Hamburguesa: $120.00 → $130.00",
                acceptable = true,
            )
        val soldOut =
            OrderChange(
                kind = ChangeKind.PRODUCT_SOLD_OUT,
                wireKind = "product_sold_out",
                itemId = "r1",
                productId = "malteada",
                productName = "Malteada de fresa",
                before = "65.00",
                now = null,
                message = "Malteada de fresa: agotado. Quítalo del pedido.",
                acceptable = false,
            )
        val changes =
            if (acceptable) {
                listOf(price)
            } else {
                listOf(price, soldOut)
            }
        return OrderReview(ReviewReason.CHANGES, "Revisa los cambios", changes, 42)
    }

    private fun capture() =
        Capture(
            id = "captura",
            waiterUserId = "ana",
            tableId = "t1",
            tableName = "Mesa 4",
            accountId = null,
            accountName = null,
            seenAccountIds = emptyList(),
            base = emptyList(),
            baseRevision = 0,
            operations = listOf(DraftOperation.Add(line("n1", "hamburguesa"))),
            ownItemIds = setOf("n1"),
            seenPrices = emptyMap(),
            sync = DraftSync.PENDING,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = 1_000,
        )

    private fun table() =
        DiningTable(
            id = "t1",
            areaId = "a1",
            name = "Mesa 4",
            shape = TableShape.ROUND,
            x = 10,
            y = 10,
            width = 90,
            height = 90,
            seats = 4,
            active = true,
            outOfService = false,
            accountCount = 1,
            state = TableState.OCCUPIED,
            occupiedSeconds = 5 * 3600,
            balance = Money("0.00"),
            longOpen = true,
        )

    private fun tablesState() =
        TablesState(
            view =
                RestaurantView(
                    areas = listOf(DiningArea("a1", "Salón", 1, true)),
                    tables = listOf(table()),
                    settings = null,
                    readAtElapsedMillis = 0,
                ),
            // Puede ver mesas y cuentas, pero no abrirlas.
            permissions =
                WaiterPermissions(
                    setOf("restaurant.tables.read", "restaurant.areas.read", "accounts.read"),
                ),
            status = RestaurantStatus(loading = false, live = true, lastError = null),
            offline = false,
            tablesView = TablesView.LIST,
            search = "",
            now = 0,
            mapAreaId = null,
        )

    private fun actions(onShowing: (Boolean) -> Unit) =
        TablesActions(
            onSearchChange = {},
            onTablesViewChange = {},
            onMapAreaSelect = {},
            onTableSelect = {},
            onShowingTableChange = onShowing,
            onRefresh = {},
            onOpenPending = {},
            onOpenAccount = { _, _ -> },
            onOpenCapture = {},
            onOpenOrder = {},
        )
}
