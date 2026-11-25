package com.adelpos.cocina

import android.content.res.Configuration
import android.os.Build
import android.view.View
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.cocina.board.BoardScreen
import com.adelpos.cocina.board.BoardState
import com.adelpos.cocina.ui.TAG_ORDERS_CONNECTING
import com.adelpos.cocina.ui.TAG_ORDERS_EMPTY
import com.adelpos.cocina.ui.TAG_ORDERS_NOT_ACTIVATED
import com.adelpos.cocina.ui.TAG_ORDERS_NO_SEAT
import com.adelpos.cocina.ui.TAG_ORDERS_SEAT_RELEASED
import com.adelpos.cocina.ui.TAG_TAKE_SEAT
import com.adelpos.core.model.access.RepairReason
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.status.TAG_REPAIR
import com.adelpos.core.ui.access.status.TAG_UPDATE_SERVER
import com.adelpos.core.ui.components.TAG_CONNECTION_STATE
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Las pantallas de la aplicación de cocina que no son tarjetas, en la tableta en horizontal: la
 * pantalla de pedidos vacía y conectando, el cupo de cocina y la licencia (§6: sin cupo, cupo
 * liberado con «Usar esta pantalla», sin activar) y las pantallas compartidas de la entrada. Guarda
 * una captura de cada una en `files/screens` de la aplicación.
 */
@RunWith(AndroidJUnit4::class)
class KitchenScreensTest {
    @get:Rule val compose = createComposeRule()

    private val host = ScreenHost(compose)

    @Test
    fun pantallaDePedidosVaciaConLaBarraYSuIndicador() {
        host.show(board = Fixtures.board(cards = emptyList()))
        compose.onNodeWithTag(TAG_ORDERS_EMPTY).assertIsDisplayed()
        compose.onNodeWithText("No hay pedidos").assertIsDisplayed()
        compose.onNodeWithText("Pedidos").assertIsDisplayed()
        compose.onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("En línea", useUnmergedTree = true).assertIsDisplayed()
        screenshot("b1-pedidos-vacia")
        host.show(board = BoardState(screen = BoardScreen.Connecting))
        compose.onNodeWithTag(TAG_ORDERS_CONNECTING).assertIsDisplayed()
        compose.onNodeWithText("Conectando con la computadora principal…").assertIsDisplayed()
        screenshot("b1-pedidos-conectando")
    }

    @Test
    fun elCupoYLaLicenciaEnLaTableta() {
        host.show(board = BoardState(screen = BoardScreen.NoSeat(inUse = 2, limit = 2)))
        compose.onNodeWithTag(TAG_ORDERS_NO_SEAT).assertIsDisplayed()
        compose
            .onNodeWithText(
                "No hay cupo de pantalla de cocina libre (2 de 2 en uso). Revoca otra pantalla " +
                    "en Terminales o amplía tu licencia.",
            )
            .assertIsDisplayed()
        screenshot("c7-sin-cupo")
        host.show(board = BoardState(screen = BoardScreen.SeatReleased))
        compose.onNodeWithTag(TAG_ORDERS_SEAT_RELEASED).assertIsDisplayed()
        compose.onNodeWithText("Esta pantalla dejó su cupo de cocina.").assertIsDisplayed()
        screenshot("c7-cupo-liberado")
        compose.onNodeWithTag(TAG_TAKE_SEAT).performClick()
        compose.waitForIdle()
        assertEquals(listOf("take-seat"), host.actions.calls)
        host.show(board = BoardState(screen = BoardScreen.NotActivated))
        compose.onNodeWithTag(TAG_ORDERS_NOT_ACTIVATED).assertIsDisplayed()
        screenshot("c7-sin-activar")
    }

    @Test
    fun lasPantallasCompartidasDeLaEntrada() {
        host.show(status = KitchenStatus.Unpaired)
        compose.onNodeWithTag(TAG_FIND_SERVER).assertIsDisplayed()
        compose.onNodeWithText("Buscar el servidor").assertIsDisplayed()
        screenshot("b2-buscar-el-servidor")
        host.show(status = KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED))
        compose.onNodeWithTag(TAG_REPAIR).assertIsDisplayed()
        screenshot("b3-emparejar-de-nuevo")
        host.show(
            status = KitchenStatus.UpdateServer(serverVersion = "0.15.0"),
            link = LinkState.OFFLINE,
        )
        compose.onNodeWithTag(TAG_UPDATE_SERVER).assertIsDisplayed()
        compose.onNodeWithText("Sin conexión", useUnmergedTree = true).assertIsDisplayed()
        screenshot("b4-actualizar-el-servidor")
    }
}

/**
 * La aplicación real al abrirla sin emparejar (instalación nueva): en horizontal, a pantalla
 * completa (sin barras del sistema) y en «Buscar el servidor».
 */
@RunWith(AndroidJUnit4::class)
class KitchenAppStartTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    @Test
    fun abreEnHorizontalAPantallaCompletaYBuscaElServidor() {
        compose.waitUntil(timeoutMillis = 15_000) {
            compose.onAllNodes(hasTestTag(TAG_FIND_SERVER)).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag(TAG_FIND_SERVER).assertIsDisplayed()
        val activity = compose.activity
        assertEquals(
            Configuration.ORIENTATION_LANDSCAPE,
            activity.resources.configuration.orientation,
        )
        // Sin la barra de estado: desde Android 11 lo dicen los insets; antes, la marca de pantalla
        // completa que pone WindowInsetsControllerCompat.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val insets = ViewCompat.getRootWindowInsets(activity.window.decorView)
            assertFalse(insets?.isVisible(WindowInsetsCompat.Type.statusBars()) ?: true)
        } else {
            @Suppress("DEPRECATION") val flags = activity.window.decorView.systemUiVisibility
            @Suppress("DEPRECATION") assertTrue(flags and View.SYSTEM_UI_FLAG_FULLSCREEN != 0)
        }
        screenshot("b0-inicio-sin-emparejar")
    }
}
