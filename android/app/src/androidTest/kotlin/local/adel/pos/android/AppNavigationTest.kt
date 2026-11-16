package local.adel.pos.android

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.ui.components.TAG_CONNECTION_INDICATOR
import local.adel.pos.android.core.ui.components.TAG_TOP_BAR
import local.adel.pos.android.core.ui.components.TAG_WAITER_MENU
import local.adel.pos.android.core.ui.components.TAG_WAITER_SWITCH
import local.adel.pos.android.feature.access.findserver.TAG_FIND_SERVER
import local.adel.pos.android.feature.access.pin.TAG_ENTRY
import local.adel.pos.android.feature.access.pin.TAG_ENTRY_OFFLINE
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La aplicación completa: sin servidor emparejado empieza en «Buscar el servidor»; emparejada y sin
 * respuesta del servidor, un mesero que ya entró entra sin conexión con su PIN y llega a Mesas; el
 * menú del mesero hace el cambio rápido de mesero. La barra y el indicador están en todas.
 */
@RunWith(AndroidJUnit4::class)
class AppNavigationTest {
    @get:Rule val compose = createEmptyComposeRule()

    private var scenario: ActivityScenario<MainActivity>? = null

    @After
    fun close() {
        scenario?.close()
        TestDevice.reset()
    }

    private fun assertTitle(title: String) {
        compose.waitUntil(timeoutMillis = 30_000) {
            compose
                .onAllNodes(hasText(title).and(hasAnyAncestor(hasTestTag(TAG_TOP_BAR))))
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        compose.onNodeWithTag(TAG_CONNECTION_INDICATOR).assertIsDisplayed()
    }

    @Test
    fun sinServidorEmparejadoEmpiezaEnBuscarElServidor() {
        TestDevice.reset()
        scenario = ActivityScenario.launch(MainActivity::class.java)
        assertTitle("Buscar el servidor")
        compose.onNodeWithTag(TAG_FIND_SERVER).assertIsDisplayed()
    }

    @Test
    fun entrarSinConexionLlevaAMesasYElMenuCambiaDeMesero() {
        TestDevice.seedOfflineWaiter()
        scenario = ActivityScenario.launch(MainActivity::class.java)
        assertTitle("Entrar")
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(TAG_ENTRY_OFFLINE)).fetchSemanticsNodes().isNotEmpty()
        }
        TestDevice.enter(compose)
        assertTitle("Mesas")

        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        assertTitle("Entrar")
        compose.onNodeWithTag(TAG_ENTRY).assertIsDisplayed()
    }
}
