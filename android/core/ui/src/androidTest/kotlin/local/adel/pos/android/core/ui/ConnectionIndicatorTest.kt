package local.adel.pos.android.core.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.ui.components.AdelTopBar
import local.adel.pos.android.core.ui.components.TAG_CONNECTION_STATE
import local.adel.pos.android.core.ui.components.TAG_PENDING_ORDERS
import local.adel.pos.android.core.ui.theme.AdelTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Indicador de conexión de la barra superior: en línea, sin conexión y contador de pedidos por
 * enviar.
 */
@RunWith(AndroidJUnit4::class)
class ConnectionIndicatorTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun enLineaSinConexionYPendientes() {
        var summary by mutableStateOf(ConnectionSummary(LinkState.ONLINE, 0))
        compose.setContent {
            AdelTheme(darkTheme = false) { AdelTopBar(title = "Mesas", connection = summary) }
        }

        compose.onNodeWithText("Mesas").assertIsDisplayed()
        compose
            .onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true)
            .assertTextEquals("En línea")
        compose.onNodeWithTag(TAG_PENDING_ORDERS, useUnmergedTree = true).assertDoesNotExist()

        summary = ConnectionSummary(LinkState.OFFLINE, 0)
        compose
            .onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true)
            .assertTextEquals("Sin conexión")
        compose.onNodeWithTag(TAG_PENDING_ORDERS, useUnmergedTree = true).assertDoesNotExist()

        summary = ConnectionSummary(LinkState.OFFLINE, 1)
        compose.onNodeWithText("1 pedido por enviar", useUnmergedTree = true).assertIsDisplayed()

        summary = ConnectionSummary(LinkState.OFFLINE, 3)
        compose.onNodeWithText("3 pedidos por enviar", useUnmergedTree = true).assertIsDisplayed()

        // Los pendientes también se ven en línea (por ejemplo, mientras se envían).
        summary = ConnectionSummary(LinkState.ONLINE, 12)
        compose
            .onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true)
            .assertTextEquals("En línea")
        compose.onNodeWithText("12 pedidos por enviar", useUnmergedTree = true).assertIsDisplayed()
    }
}
