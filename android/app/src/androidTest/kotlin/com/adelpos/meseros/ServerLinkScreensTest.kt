package com.adelpos.meseros

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.connection.ConnectionSummary
import com.adelpos.meseros.core.domain.connection.FixedServerLink
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.components.AdelTopBar
import com.adelpos.meseros.core.ui.components.TAG_CONNECTION_STATE
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.feature.access.AccessContent
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_OFFLINE
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_STATUS
import com.adelpos.meseros.feature.takeaway.TakeawayActions
import com.adelpos.meseros.feature.takeaway.TakeawayClock
import com.adelpos.meseros.feature.takeaway.TakeawayScreen
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_STATUS
import com.adelpos.meseros.feature.waiter.tables.TablesActions
import com.adelpos.meseros.feature.waiter.tables.TablesScreen
import com.adelpos.meseros.feature.waiter.tables.TablesState
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3, §2 y §6: la barra y los avisos de cada pantalla leen **el mismo** estado de conexión
 * (`ServerLink`) y nunca se contradicen. Con un estado forzado (`FakeAccess` y su
 * [FixedServerLink]), cada cambio se ve igual en la barra y en «Selecciona tu usuario», Mesas y
 * Para llevar; una lectura que falló con el estado «En línea» no dice «Sin conexión».
 */
@RunWith(AndroidJUnit4::class)
class ServerLinkScreensTest {
    @get:Rule val compose = createComposeRule()

    private val link = FixedServerLink(LinkState.ONLINE)

    // La barra con el mismo estado, como ShellViewModel en la aplicación.
    @Composable
    private fun WithBar(title: String, content: @Composable () -> Unit) {
        val state by link.state.collectAsState()
        AdelTheme(darkTheme = false) {
            Column {
                AdelTopBar(title = title, connection = ConnectionSummary(state, 0))
                content()
            }
        }
    }

    private fun bar(): String =
        compose
            .onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true)
            .fetchSemanticsNode()
            .config[SemanticsProperties.Text]
            .joinToString("") { it.text }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun waitForBar(text: String) {
        compose.waitUntil(timeoutMillis = 5_000) {
            compose
                .onAllNodes(hasText(text), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
    }

    @Test
    fun quienEresYLaBarraSiempreDicenLoMismo() {
        val fake = FakeAccess()
        // El estado único de la entrada falsa es el mismo que lee la barra.
        val shared = fake.link
        compose.setContent {
            val state by shared.state.collectAsState()
            AdelTheme(darkTheme = false) {
                Column {
                    AdelTopBar(title = "Ingresar", connection = ConnectionSummary(state, 0))
                    AccessContent(
                        status = DeviceStatus.Ready("Fonda", "Tableta 1", 120),
                        dependencies = fake.dependencies(),
                        actions = NoActions,
                        layout = PaneLayout.DOUBLE,
                    )
                }
            }
        }
        compose.waitUntil(timeoutMillis = 10_000) { present("${TAG_WAITER_PREFIX}ana") }
        for (state in listOf(LinkState.OFFLINE, LinkState.ONLINE, LinkState.OFFLINE)) {
            shared.set(state)
            val offline = state == LinkState.OFFLINE
            waitForBar(if (offline) "Sin conexión" else "En línea")
            compose.waitUntil(timeoutMillis = 5_000) { present(TAG_ENTRY_OFFLINE) == offline }
            assertEquals(
                "la barra y «Selecciona tu usuario» dicen lo mismo",
                offline,
                bar() == "Sin conexión" && present(TAG_ENTRY_OFFLINE),
            )
        }
    }

    @Test
    fun mesasYLaBarraSiempreDicenLoMismo() {
        // La última lectura falló (live = false), pero eso solo no es «Sin conexión».
        val stale = RestaurantStatus(loading = false, live = false, lastError = null)
        compose.setContent {
            val state by link.state.collectAsState()
            WithBar("Mesas") {
                TablesScreen(
                    layout = PaneLayout.SINGLE,
                    state = tablesState(stale, offline = state == LinkState.OFFLINE),
                    selected = null,
                    selectedAccounts = emptyList(),
                    captures = emptyList(),
                    orders = emptyList(),
                    showingTable = false,
                    opening = false,
                    actions = tablesActions(),
                )
            }
        }
        waitForBar("En línea")
        assertEquals("con «En línea» no dice «Sin conexión»", false, present(TAG_TABLES_STATUS))
        link.set(LinkState.OFFLINE)
        waitForBar("Sin conexión")
        compose
            .onNodeWithTag(TAG_TABLES_STATUS)
            .assertTextEquals("Sin conexión: se ven las mesas de la última vez que hubo conexión.")
        link.set(LinkState.ONLINE)
        waitForBar("En línea")
        compose.waitUntil(timeoutMillis = 5_000) { !present(TAG_TABLES_STATUS) }
    }

    @Test
    fun paraLlevarYLaBarraSiempreDicenLoMismo() {
        val stale = RestaurantStatus(loading = false, live = false, lastError = null)
        compose.setContent {
            val state by link.state.collectAsState()
            WithBar("Para llevar") {
                TakeawayScreen(
                    entries = emptyList(),
                    permissions = WaiterPermissions(setOf("takeaway.read")),
                    status = stale,
                    offline = state == LinkState.OFFLINE,
                    readAt = TakeawayRead(wallMillis = 1_700_000_000_000L, elapsedMillis = 0),
                    clock = TakeawayClock(elapsedMillis = 60_000, wallMillis = 1_700_000_060_000L),
                    online = state == LinkState.ONLINE,
                    delivering = null,
                    message = null,
                    actions = TakeawayActions({}, {}, {}, {}, {}),
                )
            }
        }
        waitForBar("En línea")
        compose.onNodeWithTag(TAG_TAKEAWAY_STATUS).assertTextEquals("")
        link.set(LinkState.OFFLINE)
        waitForBar("Sin conexión")
        compose.waitUntil(timeoutMillis = 5_000) {
            compose
                .onAllNodes(hasText("Sin conexión: es lo último que se vio", substring = true))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        link.set(LinkState.ONLINE)
        waitForBar("En línea")
        compose.onNodeWithTag(TAG_TAKEAWAY_STATUS).assertTextEquals("")
    }

    private fun tablesState(status: RestaurantStatus, offline: Boolean) =
        TablesState(
            view =
                RestaurantView(
                    areas = listOf(DiningArea("a1", "Salón", 1, true)),
                    tables = emptyList(),
                    settings = null,
                    readAtElapsedMillis = 0,
                ),
            permissions = WaiterPermissions(setOf("restaurant.tables.read")),
            status = status,
            offline = offline,
            tablesView = TablesView.LIST,
            now = 0,
            areaId = null,
        )

    private fun tablesActions() =
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
        )
}
