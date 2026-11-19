package com.adelpos.meseros

import androidx.activity.compose.setContent
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
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.capture.CustomerDialog
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_NAME
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_PHONE
import com.adelpos.meseros.core.capture.TAG_CUSTOMER_SAVE
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.OrderDestination
import com.adelpos.meseros.core.domain.waiter.OrderLine
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayEntry
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.Totals
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.ui.waiter.TAG_ORDER_STATE_PREFIX
import com.adelpos.meseros.core.ui.waiter.TAG_TAKEAWAY_PAYMENT_PREFIX
import com.adelpos.meseros.core.ui.waiter.TAG_TAKEAWAY_PREPARATION_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_COLLECT_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_DELIVER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_EDIT_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_ENTRY_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_NEW
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_READ_ONLY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_STATUS
import com.adelpos.meseros.feature.takeaway.TakeawayActions
import com.adelpos.meseros.feature.takeaway.TakeawayClock
import com.adelpos.meseros.feature.takeaway.TakeawayScreen
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Las pantallas de Para llevar (etapa A2, §3.3) con datos fijos, en tema claro y oscuro: la lista
 * con folio, nombre, preparación y pago como los dice Go; «Entregado» solo pagado y con permiso, y
 * «Cobrar en caja antes de entregar» sin pagar (A2-D2); lo que todavía no nace en Go con su estado;
 * sin conexión, lo último que se vio con su hora; y «Nombre y teléfono» con los límites de Go.
 */
@RunWith(AndroidJUnit4::class)
class TakeawayScreensTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private fun show(dark: Boolean, content: @Composable () -> Unit) {
        compose.activity.runOnUiThread {
            compose.activity.setContent { AdelTheme(darkTheme = dark) { content() } }
        }
        compose.waitForIdle()
    }

    private val all =
        WaiterPermissions(
            setOf(
                "takeaway.read",
                "takeaway.manage",
                "takeaway.deliver",
                "accounts.open",
                "orders.capture",
            ),
        )

    private val live = RestaurantStatus(loading = false, live = true, lastError = null)
    private val clock = TakeawayClock(elapsedMillis = 60_000L, wallMillis = 1_700_000_060_000L)
    private val read = TakeawayRead(wallMillis = 1_700_000_000_000L, elapsedMillis = 0L)

    private fun account(
        id: String,
        folio: Int,
        preparation: PreparationState,
        payment: String,
        customer: TakeawayCustomer,
    ) =
        AccountSummary(
            id = id,
            folio = folio,
            name = "Pedido para llevar",
            tableId = null,
            tableName = null,
            state = AccountSummary.STATE_OPEN,
            revision = 2,
            preparationState = preparation,
            confirmedQuantity = 1,
            draftQuantity = 0,
            totals = Totals.ZERO,
            assignedName = "Ana",
            elapsedSeconds = 600,
            longOpen = false,
            mode = OrderMode.TAKEAWAY,
            customer = customer,
            paymentState = payment,
            deliveryState = "not_delivered",
        )

    private fun inGo(account: AccountSummary) =
        TakeawayEntry.InGo(
            account = account,
            customer = account.customer,
            customerPending = false,
            captureId = null,
            queued = null,
        )

    private fun queued(): TakeawayEntry.Queued {
        val line = DraftLine("r1", LineSelection("p1", null, 2, Customization.ORIGINAL))
        val order =
            PendingOrder(
                operationId = "op-1",
                waiterUserId = "ana",
                waiterName = "Ana",
                state = OrderState.QUEUED,
                destination =
                    OrderDestination(
                        tableId = null,
                        tableName = "",
                        accountId = null,
                        accountName = null,
                        seenAccountIds = emptyList(),
                        mode = OrderMode.TAKEAWAY,
                        customer = TakeawayCustomer("Luisa", ""),
                    ),
                lines = listOf(OrderLine(line, "Hamburguesa", Money("120.00"))),
                replaces = emptyList(),
                catalogVersion = 41,
                catalogEpoch = null,
                capturedAtMillis = 1_700_000_000_000L,
                confirmation = null,
                review = null,
                attempts = 0,
                lastError = null,
                credentialRejected = false,
                onlineConfirm = null,
                receivedAccountId = null,
                receivedAtMillis = null,
                updatedAtMillis = 1_700_000_000_000L,
            )
        return TakeawayEntry.Queued(order, TakeawayCustomer("Luisa", ""), customerPending = true)
    }

    private val ready =
        account("listo", 12, PreparationState.READY, "paid", TakeawayCustomer("Juan", "555"))
    private val unpaid =
        account("sin-pagar", 13, PreparationState.RELEASED, "not_recorded", TakeawayCustomer.NONE)
    private val cooking =
        account("cocinando", 14, PreparationState.PREPARING, "partial", TakeawayCustomer.NONE)

    private fun actions(
        delivered: MutableList<String> = mutableListOf(),
        edited: MutableList<String> = mutableListOf(),
    ) =
        TakeawayActions(
            onNewOrder = {},
            onOpen = {},
            onEditCustomer = { edited.add(it.key) },
            onDeliver = { delivered.add(it.key) },
            onRefresh = {},
        )

    @Test
    fun listaConPreparacionPagoYEntregaEnAmbosTemas() {
        for (dark in listOf(false, true)) {
            val delivered = mutableListOf<String>()
            val edited = mutableListOf<String>()
            show(dark) {
                TakeawayScreen(
                    entries = listOf(queued(), inGo(ready), inGo(unpaid), inGo(cooking)),
                    permissions = all,
                    status = live,
                    readAt = read,
                    clock = clock,
                    online = true,
                    delivering = null,
                    message = null,
                    actions = actions(delivered, edited),
                )
            }
            compose.onNodeWithTag(TAG_TAKEAWAY_NEW).assertIsEnabled()
            // Lo que todavía no nace en Go, con su estado de la cola y el nombre.
            compose.onNodeWithTag("${TAG_TAKEAWAY_ENTRY_PREFIX}op-1").assertExists()
            compose.onNodeWithText("Pedido nuevo (sin folio todavía)").assertExists()
            compose
                .onNodeWithTag("${TAG_ORDER_STATE_PREFIX}queued", useUnmergedTree = true)
                .assertExists()
            compose.onNodeWithText("Luisa").assertExists()
            compose.onNodeWithText("2 × Hamburguesa").assertExists()

            // Listo y pagado: «Entregado».
            scrollTo("${TAG_TAKEAWAY_ENTRY_PREFIX}listo")
            compose.onNodeWithText("Folio 12").assertExists()
            compose.onNodeWithText("Juan · 555").assertExists()
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_PREPARATION_PREFIX}ready", useUnmergedTree = true)
                .assertExists()
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_PAYMENT_PREFIX}paid", useUnmergedTree = true)
                .assertExists()
            // 600 s de Go más el minuto que pasó desde la lectura (todos los de la prueba llevan lo
            // mismo).
            assertTrue(compose.onAllNodes(hasText("11 min")).fetchSemanticsNodes().isNotEmpty())
            compose.onNodeWithTag("${TAG_TAKEAWAY_DELIVER_PREFIX}listo").performClick()
            assertEquals(listOf("listo"), delivered)

            // Sin pagar: nunca «Entregado», sí «Cobrar en caja antes de entregar».
            scrollTo("${TAG_TAKEAWAY_ENTRY_PREFIX}sin-pagar")
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_COLLECT_PREFIX}sin-pagar", useUnmergedTree = true)
                .assertExists()
            assertTrue(missing("${TAG_TAKEAWAY_DELIVER_PREFIX}sin-pagar"))
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_PREPARATION_PREFIX}pending", useUnmergedTree = true)
                .assertExists()
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_PAYMENT_PREFIX}unpaid", useUnmergedTree = true)
                .assertExists()

            // Pago parcial (como lo dice Go) y en preparación: tampoco se entrega.
            scrollTo("${TAG_TAKEAWAY_ENTRY_PREFIX}cocinando")
            compose
                .onNodeWithTag(
                    "${TAG_TAKEAWAY_PREPARATION_PREFIX}preparing",
                    useUnmergedTree = true,
                )
                .assertExists()
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_PAYMENT_PREFIX}partial", useUnmergedTree = true)
                .assertExists()
            compose
                .onNodeWithTag("${TAG_TAKEAWAY_COLLECT_PREFIX}cocinando", useUnmergedTree = true)
                .assertExists()
            compose.onNodeWithTag("${TAG_TAKEAWAY_EDIT_PREFIX}cocinando").performClick()
            assertEquals(listOf("cocinando"), edited)
        }
    }

    @Test
    fun sinPermisoDeEntregarSoloElEstadoYSinManageSoloLaLista() {
        show(dark = false) {
            TakeawayScreen(
                entries = listOf(inGo(ready), inGo(unpaid)),
                permissions = WaiterPermissions(setOf("takeaway.read")),
                status = live,
                readAt = read,
                clock = clock,
                online = true,
                delivering = null,
                message = null,
                actions = actions(),
            )
        }
        compose.onNodeWithTag(TAG_TAKEAWAY_READ_ONLY).assertExists()
        assertTrue(missing(TAG_TAKEAWAY_NEW))
        compose
            .onNodeWithTag("${TAG_TAKEAWAY_PAYMENT_PREFIX}paid", useUnmergedTree = true)
            .assertExists()
        assertTrue(missing("${TAG_TAKEAWAY_DELIVER_PREFIX}listo"))
        assertTrue(missing("${TAG_TAKEAWAY_COLLECT_PREFIX}sin-pagar"))
        assertTrue(missing("${TAG_TAKEAWAY_EDIT_PREFIX}listo"))
    }

    @Test
    fun sinConexionLoUltimoQueSeVioConSuHora() {
        show(dark = true) {
            TakeawayScreen(
                entries = listOf(inGo(ready)),
                permissions = all,
                status = RestaurantStatus(loading = false, live = false, lastError = null),
                readAt = read,
                clock = clock,
                online = false,
                delivering = null,
                message = null,
                actions = actions(),
            )
        }
        val time =
            DateTimeFormatter.ofPattern("HH:mm")
                .format(Instant.ofEpochMilli(read.wallMillis).atZone(ZoneId.systemDefault()))
        compose.onNodeWithTag(TAG_TAKEAWAY_STATUS).assertExists().also {
            compose
                .onNodeWithText("Sin conexión: es lo último que se vio, a las $time.")
                .assertExists()
        }
        // «Entregado» exige conexión: no va a la cola.
        compose.onNodeWithTag("${TAG_TAKEAWAY_DELIVER_PREFIX}listo").assertIsNotEnabled()
        compose
            .onNodeWithText(
                "Para marcarlo entregado necesitas conexión con la computadora principal.",
            )
            .assertExists()
    }

    /**
     * A2-D1: opcionales, sin formato, con los límites de Go (100 y 40 bytes del texto recortado).
     */
    @Test
    fun nombreYTelefonoConLosLimitesDeGo() {
        var saved: TakeawayCustomer? = null
        show(dark = false) {
            CustomerDialog(
                initial = TakeawayCustomer("Juan", ""),
                onSave = { saved = it },
                onDismiss = {},
            )
        }
        compose.onNodeWithTag(TAG_CUSTOMER_NAME).performTextReplacement("ñ".repeat(50))
        // Una «ñ» más no cabe (Go cuenta bytes: 102 > 100): se queda como estaba.
        compose.onNodeWithTag(TAG_CUSTOMER_NAME).performTextInput("ñ")
        compose.onNodeWithTag(TAG_CUSTOMER_PHONE).performTextInput("(55) 1234-5678 ext. 2 ")
        compose.onNodeWithTag(TAG_CUSTOMER_SAVE).performClick()
        assertEquals(TakeawayCustomer("ñ".repeat(50), "(55) 1234-5678 ext. 2"), saved)
    }

    private fun scrollTo(tag: String) {
        compose.onNodeWithTag(TAG_TAKEAWAY).performScrollToNode(hasTestTag(tag))
    }

    // Lo de un renglón tocable se fusiona con él: se busca en el árbol sin fusionar.
    private fun missing(tag: String): Boolean =
        compose.onAllNodes(hasTestTag(tag), useUnmergedTree = true).fetchSemanticsNodes().isEmpty()
}
