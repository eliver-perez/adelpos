package com.adelpos.meseros

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.capture.TAG_CAPTURE
import com.adelpos.meseros.core.data.waiter.db.WaiterPermissionsEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.ui.components.TAG_CONNECTION_INDICATOR
import com.adelpos.meseros.core.ui.components.TAG_PIN_DELETE
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.components.TAG_TOP_BAR
import com.adelpos.meseros.core.ui.components.TAG_WAITER_MENU
import com.adelpos.meseros.core.ui.components.TAG_WAITER_SWITCH
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_OFFLINE
import com.adelpos.meseros.feature.access.pin.TAG_REENTER
import com.adelpos.meseros.feature.access.pin.TAG_REENTER_MESSAGE
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_EMPTY
import com.adelpos.meseros.feature.takeaway.TAG_TAKEAWAY_NEW
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SCREEN
import com.adelpos.meseros.navigation.TAG_MAIN_NAVIGATION
import com.adelpos.meseros.navigation.TAG_NAV_TABLES
import com.adelpos.meseros.navigation.TAG_NAV_TAKEAWAY
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La aplicación completa: sin servidor emparejado empieza en «Buscar el servidor»; emparejada y sin
 * respuesta del servidor, un mesero que ya entró entra sin conexión con su PIN y llega a Mesas; el
 * menú del mesero hace el cambio rápido de mesero. La barra y el indicador están en todas. Una
 * sesión terminada pide «Vuelve a escribir tu PIN» sin salir de Mesas, y los toques dentro de ese
 * diálogo cuentan para el bloqueo por inactividad (revisión R3, M3 y B4).
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
        assertTitle("Ingresar")
        // El servidor de prueba no responde: «Sin conexión» llega con la tolerancia del estado
        // único (etapa A3: 3 fallas seguidas y 20 s), no con la primera falla.
        compose.waitUntil(timeoutMillis = 60_000) {
            compose.onAllNodes(hasTestTag(TAG_ENTRY_OFFLINE)).fetchSemanticsNodes().isNotEmpty()
        }
        TestDevice.enter(compose)
        assertTitle("Mesas")

        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        assertTitle("Ingresar")
        compose.onNodeWithTag(TAG_ENTRY).assertIsDisplayed()
    }

    private fun waitForTag(tag: String) {
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
        }
    }

    @Test
    fun unaSesionTerminadaPideVolverAEscribirElPinSinSalirDeMesas() {
        TestDevice.seedOfflineWaiter()
        scenario = ActivityScenario.launch(MainActivity::class.java)
        TestDevice.enter(compose)
        assertTitle("Mesas")
        val container = TestDevice.app.container
        container.deviceAccess.report(
            AppError.SessionEnded("SESSION_EXPIRED", null, null),
            TestDevice.WAITER_ID,
        )
        waitForTag(TAG_REENTER)
        compose.onNode(hasText("Vuelve a escribir tu PIN", substring = true)).assertIsDisplayed()
        // La pantalla del mesero sigue debajo, con su barra.
        compose.onNode(hasText("Mesas").and(hasAnyAncestor(hasTestTag(TAG_TOP_BAR)))).assertExists()
        compose.onNodeWithTag(TAG_TABLES_SCREEN).assertExists()

        // Los toques en el diálogo (otra ventana) reinician el plazo del bloqueo por inactividad.
        val timeout = 3_600_000L
        Thread.sleep(1_500)
        val before = container.idleLock.remainingMillis(timeout)
        compose.onNodeWithTag("${TAG_PIN_KEY_PREFIX}1").performClick()
        val after = container.idleLock.remainingMillis(timeout)
        assertTrue("el toque reinició el plazo ($before → $after)", after > before + 1_000)
        compose.onNodeWithTag(TAG_PIN_DELETE).performClick()

        // PIN incorrecto: el aviso aparece en el diálogo y el mesero sigue siendo el mismo.
        for (digit in "1111") {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitForTag(TAG_REENTER_MESSAGE)
        assertTrue(container.deviceAccess.activeWaiter.value!!.mustReenterPin)
        for (digit in TestDevice.PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(TAG_REENTER)).fetchSemanticsNodes().isEmpty()
        }
        assertFalse(container.deviceAccess.activeWaiter.value!!.mustReenterPin)
        assertTitle("Mesas")
    }

    /**
     * Etapa A2, §3.1: con `takeaway.read` en los permisos de Go del mesero, la navegación principal
     * muestra «Mesas» y «Para llevar» (en el teléfono y en la tableta); «Nuevo pedido» entra
     * directo al catálogo y Atrás vuelve a Para llevar sin dejar nada. Sin el permiso, no aparece.
     */
    @Test
    fun mesasYParaLlevarEnLaNavegacionPrincipal() {
        TestDevice.seedOfflineWaiter()
        val waiter = TestDevice.app.container.database.waiter()
        runBlocking {
            waiter.savePermissions(
                WaiterPermissionsEntity(
                    userId = TestDevice.WAITER_ID,
                    permissions =
                        WaiterStorage.encodeStrings(
                            listOf(
                                "restaurant.tables.read",
                                "restaurant.areas.read",
                                "accounts.read",
                                "accounts.open",
                                "orders.capture",
                                "takeaway.read",
                                "takeaway.manage",
                            ),
                        ),
                    readAt = 1L,
                ),
            )
        }
        try {
            scenario = ActivityScenario.launch(MainActivity::class.java)
            TestDevice.enter(compose)
            assertTitle("Mesas")
            waitForTag(TAG_MAIN_NAVIGATION)
            compose.onNodeWithTag(TAG_NAV_TAKEAWAY).performClick()
            assertTitle("Para llevar")
            waitForTag(TAG_TAKEAWAY_NEW)
            compose.onNodeWithTag(TAG_TAKEAWAY_NEW).performClick()
            // Etapa A3 (§4.2): la barra dice dónde se está.
            assertTitle("Nuevo pedido para llevar")
            waitForTag(TAG_CAPTURE)
            // La navegación principal sigue a la vista durante la captura.
            compose.onNodeWithTag(TAG_MAIN_NAVIGATION).assertIsDisplayed()
            Espresso.pressBack()
            assertTitle("Para llevar")
            // Un «Nuevo pedido» sin productos no deja nada en el dispositivo.
            compose.waitUntil(timeoutMillis = 10_000) {
                runBlocking { waiter.allCaptures().isEmpty() }
            }
            compose.onNodeWithTag(TAG_TAKEAWAY_EMPTY).assertIsDisplayed()
            compose.onNodeWithTag(TAG_NAV_TABLES).performClick()
            assertTitle("Mesas")
            compose.onNodeWithTag(TAG_TABLES_SCREEN).assertIsDisplayed()

            // Sin el permiso de ver los pedidos para llevar, no aparece.
            compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
            compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
            assertTitle("Ingresar")
            runBlocking { waiter.deleteAllPermissions() }
            TestDevice.enter(compose)
            assertTitle("Mesas")
            compose.waitForIdle()
            assertTrue(
                compose.onAllNodes(hasTestTag(TAG_MAIN_NAVIGATION)).fetchSemanticsNodes().isEmpty(),
            )
        } finally {
            runBlocking { waiter.deleteAllPermissions() }
        }
    }
}
