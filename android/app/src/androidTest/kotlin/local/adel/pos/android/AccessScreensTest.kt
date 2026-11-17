package local.adel.pos.android

import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertContentDescriptionEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.test.espresso.Espresso
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.RepairReason
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.ui.components.TAG_PIN_KEY_PREFIX
import local.adel.pos.android.core.ui.components.TAG_PIN_PAD
import local.adel.pos.android.core.ui.components.TAG_PIN_SUBMIT
import local.adel.pos.android.core.ui.components.TAG_WAITER_MENU
import local.adel.pos.android.core.ui.components.TAG_WAITER_SWITCH
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.AccessActions
import local.adel.pos.android.feature.access.AccessContent
import local.adel.pos.android.feature.access.findserver.TAG_ADDRESS
import local.adel.pos.android.feature.access.findserver.TAG_FOUND_SERVER_PREFIX
import local.adel.pos.android.feature.access.findserver.TAG_PAIRING_CODE
import local.adel.pos.android.feature.access.findserver.TAG_PAIRING_NAME
import local.adel.pos.android.feature.access.findserver.TAG_REQUEST_ADDRESS
import local.adel.pos.android.feature.access.pin.TAG_ENTRY_BACK
import local.adel.pos.android.feature.access.pin.TAG_ENTRY_MESSAGE
import local.adel.pos.android.feature.access.pin.TAG_ENTRY_OFFLINE
import local.adel.pos.android.feature.access.pin.TAG_PIN_PANEL
import local.adel.pos.android.feature.access.pin.TAG_WAITER_LIST
import local.adel.pos.android.feature.access.pin.TAG_WAITER_PREFIX
import local.adel.pos.android.feature.access.status.TAG_REPAIR_ACTION
import local.adel.pos.android.feature.access.status.TAG_REPAIR_CONFIRM
import local.adel.pos.android.feature.access.status.TAG_REPAIR_ERASES
import local.adel.pos.android.feature.access.status.TAG_REPAIR_PENDING
import local.adel.pos.android.feature.waiter.tables.TAG_TABLES_SEARCH
import local.adel.pos.android.navigation.AdelPosNavigation
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Pantallas de la entrada con una entrada falsa: la lista de meseros y el teclado del PIN en
 * teléfono (uno a la vez) y en tableta (lado a lado), en ambos temas; PIN incorrecto y correcto;
 * cambio rápido de mesero; «Buscar el servidor» con un servidor encontrado y el código grande; y la
 * confirmación de «Emparejar de nuevo».
 */
@RunWith(AndroidJUnit4::class)
class AccessScreensTest {
    @get:Rule val compose = createComposeRule()

    private fun type(pin: String) {
        for (digit in pin) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
    }

    private fun waitFor(tag: String) {
        compose.waitUntil(timeoutMillis = 10_000) {
            compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
        }
    }

    @Test
    fun entradaEnElTelefonoUnaPantallaALaVezEnAmbosTemas() {
        val fake = FakeAccess(online = false)
        var dark by mutableStateOf(false)
        compose.setContent {
            AdelTheme(darkTheme = dark) {
                AccessContent(
                    status = DeviceStatus.Ready("Fonda", "Teléfono 1", 120, online = false),
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    layout = PaneLayout.SINGLE,
                )
            }
        }
        for (theme in listOf(false, true)) {
            dark = theme
            waitFor("${TAG_WAITER_PREFIX}ana")
            compose.onNodeWithTag(TAG_ENTRY_OFFLINE).assertIsDisplayed()
            // Carla está bloqueada: su botón lo dice.
            compose.onNodeWithText("Bloqueado hasta", substring = true).assertExists()
            compose.onNodeWithTag("${TAG_WAITER_PREFIX}ana").performClick()
            compose.onNodeWithTag(TAG_PIN_PAD).assertIsDisplayed()
            compose.onAllNodes(hasTestTag(TAG_WAITER_LIST)).fetchSemanticsNodes().let {
                assertEquals("en el teléfono, la lista se oculta", 0, it.size)
            }
            type("1111")
            waitFor(TAG_ENTRY_MESSAGE)
            compose.onNodeWithText("PIN incorrecto. Inténtalo de nuevo.").assertIsDisplayed()
            compose.onNodeWithTag(TAG_ENTRY_BACK).performClick()
            waitFor(TAG_WAITER_LIST)
        }
    }

    @Test
    fun entradaEnLaTabletaListaYTecladoLadoALado() {
        val fake = FakeAccess()
        compose.setContent {
            AdelTheme(darkTheme = true) {
                AccessContent(
                    status = DeviceStatus.Ready("Fonda", "Tableta 1", 120, online = true),
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    layout = PaneLayout.DOUBLE,
                )
            }
        }
        waitFor("${TAG_WAITER_PREFIX}beto")
        compose.onNodeWithTag(TAG_PIN_PANEL).assertIsDisplayed()
        compose.onNodeWithTag("${TAG_WAITER_PREFIX}beto").performClick()
        compose.onNodeWithTag(TAG_WAITER_LIST).assertIsDisplayed()
        compose.onNodeWithText("PIN de Beto").assertIsDisplayed()
        type("4829")
        compose.waitUntil(timeoutMillis = 10_000) { fake.mutableActive.value?.userId == "beto" }
    }

    @Test
    fun cambioRapidoDeMeseroDesdeElMenu() {
        val fake = FakeAccess()
        compose.setContent {
            val status by fake.status.collectAsState()
            val waiter by fake.activeWaiter.collectAsState()
            AdelTheme {
                AdelPosNavigation(
                    connection = ConnectionSummary(LinkState.ONLINE, 0),
                    status = status,
                    activeWaiter = waiter,
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    onSwitchWaiter = fake::leave,
                )
            }
        }
        waitFor("${TAG_WAITER_PREFIX}ana")
        compose.onNodeWithTag("${TAG_WAITER_PREFIX}ana").performClick()
        type("4829")
        waitFor(TAG_TABLES_SEARCH)
        compose.onNodeWithText("Ana", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        waitFor("${TAG_WAITER_PREFIX}dora")
        compose.onNodeWithTag("${TAG_WAITER_PREFIX}dora").performClick()
        type("4829")
        waitFor(TAG_TABLES_SEARCH)
        compose.onNodeWithText("Dora Elena", useUnmergedTree = true).assertIsDisplayed()
    }

    @Test
    fun buscarElServidorYElCodigoEnGrande() {
        val fake = FakeAccess()
        compose.setContent {
            AdelTheme {
                AccessContent(
                    status = DeviceStatus.Unpaired(),
                    dependencies = fake.dependencies(),
                    actions = NoActions,
                    layout = PaneLayout.SINGLE,
                )
            }
        }
        waitFor("${TAG_FOUND_SERVER_PREFIX}11111111-2222-4333-8444-555555555555")
        compose.onNodeWithText("Fonda Doña Ñeli — Mac de caja").assertIsDisplayed()
        compose.onNodeWithTag(TAG_ADDRESS).performTextReplacement("192.168.1.20")
        // El teclado se cierra antes de tocar el botón: mientras se abre, la pantalla se
        // reacomoda y un toque puede no llegar a serlo.
        Espresso.closeSoftKeyboard()
        compose.waitForIdle()
        compose.onNodeWithTag(TAG_REQUEST_ADDRESS).performScrollTo().performClick()
        waitFor(TAG_PAIRING_CODE)
        compose.onNodeWithTag(TAG_PAIRING_CODE).assertTextContains("166 101")
        compose
            .onNodeWithTag(TAG_PAIRING_CODE)
            .assertContentDescriptionEquals("Código de emparejamiento 1 6 6 1 0 1")
        compose.onNodeWithText("Computadora principal de Fonda Doña Ñeli").assertIsDisplayed()
        // «Tableta 1» o «Teléfono 1», según el dispositivo.
        compose.onNodeWithTag(TAG_PAIRING_NAME).assertTextContains("se llamará", substring = true)
        compose.onNode(hasText("El código vence en", substring = true)).assertIsDisplayed()
    }

    @Test
    fun emparejarDeNuevoPideConfirmacionDiceQueSeBorraYMuestraLosPendientes() {
        var repairs = 0
        val pending = UnsentOrder("Ana", "Mesa 4: 2 hamburguesas", System.currentTimeMillis())
        val actions =
            object : AccessActions by NoActions {
                override fun repair() {
                    repairs += 1
                }

                override suspend fun pendingOrders(): List<UnsentOrder> = listOf(pending)
            }
        compose.setContent {
            AdelTheme {
                AccessContent(
                    status = DeviceStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED),
                    dependencies = FakeAccess().dependencies(),
                    actions = actions,
                    layout = PaneLayout.SINGLE,
                )
            }
        }
        compose
            .onNodeWithText("necesita emparejarse de nuevo", substring = true)
            .assertIsDisplayed()
        compose.onNodeWithTag(TAG_REPAIR_ACTION).performClick()
        assertEquals("primero se confirma", 0, repairs)
        compose
            .onNodeWithTag(TAG_REPAIR_ERASES)
            .assertTextContains("el catálogo con sus imágenes", substring = true)
        waitFor(TAG_REPAIR_PENDING)
        compose
            .onNode(hasText("Mesa 4: 2 hamburguesas", substring = true), useUnmergedTree = true)
            .assertExists()
        compose.onNodeWithTag(TAG_REPAIR_CONFIRM).performClick()
        assertEquals(1, repairs)
    }
}
