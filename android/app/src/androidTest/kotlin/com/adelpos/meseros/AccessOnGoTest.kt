package com.adelpos.meseros

import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.data.api.ApiErrors
import com.adelpos.meseros.core.data.api.ApiOutcome
import com.adelpos.meseros.core.data.api.ClientIdentity
import com.adelpos.meseros.core.data.api.PinUsersResponse
import com.adelpos.meseros.core.data.api.PinnedHttpClient
import com.adelpos.meseros.core.data.api.ServerApi
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.EntryResult
import com.adelpos.meseros.core.domain.access.RepairReason
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.text.Hex
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.components.TAG_WAITER_MENU
import com.adelpos.meseros.core.ui.components.TAG_WAITER_SWITCH
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_BACK
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_EMPTY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_MESSAGE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_OFFLINE
import com.adelpos.meseros.feature.access.pin.TAG_REENTER
import com.adelpos.meseros.feature.access.pin.TAG_REENTER_MESSAGE
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.access.status.TAG_CHECK_AGAIN
import com.adelpos.meseros.feature.access.status.TAG_REPAIR
import com.adelpos.meseros.feature.access.status.TAG_REPAIR_ACTION
import com.adelpos.meseros.feature.access.status.TAG_REPAIR_CONFIRM
import com.adelpos.meseros.feature.access.status.TAG_RETIRED
import com.adelpos.meseros.feature.access.status.TAG_RETIRED_PAIR_AGAIN
import com.adelpos.meseros.feature.access.status.TAG_UPDATE_APP
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SEARCH
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Fases C y B contra el **Go temporal** del arnés (`iso.sh server-up`), nunca el servicio real. La
 * prueba hace de computadora principal por la escucha local ([MainComputer]) y la aplicación usa la
 * escucha de red HTTPS con el certificado que fija al emparejar.
 *
 * Cada paso corre en su propia instrumentación (`am instrument -e class …#paso`), en orden, con la
 * aplicación instalada una sola vez: los datos de la aplicación pasan de un paso al siguiente y el
 * proceso se reinicia en cada uno (como cuando Android cierra la aplicación). Entre pasos, el guion
 * del anfitrión corta la red en Android 8, reinicia el Go temporal con otra identidad en la misma
 * dirección o corre con otra versión de cliente. Sin los argumentos se omite.
 *
 * Argumentos (los arma `scripts/android-e2e.sh`): `local` (`http://10.0.2.2:<puerto local>`),
 * `address` (`10.0.2.2:<puerto de red>`), `pin` (huella SPKI del certificado del Go temporal),
 * `serverId` (opcional), `mainDevice` y `mainToken` (la terminal principal del asistente),
 * `hostNetworkCut` (`true` si el anfitrión cortó la red) y `clientVersion` (lo lee
 * [AdelTestRunner]).
 */
@RunWith(AndroidJUnit4::class)
class AccessOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    @Before
    fun mainComputer() {
        main = MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
    }

    @After
    fun close() {
        scenario?.close()
    }

    private fun log(message: String) = Log.i(TAG, message)

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    private fun waitFor(tag: String, timeout: Long = 40_000) {
        try {
            compose.waitUntil(timeoutMillis = timeout) {
                compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
            }
        } catch (failure: Throwable) {
            // Lo que había en pantalla, para el registro de la corrida.
            compose.onRoot(useUnmergedTree = true).printToLog(TAG)
            throw failure
        }
    }

    private fun waitForText(text: String, timeout: Long = 40_000) {
        compose.waitUntil(timeoutMillis = timeout) {
            compose.onAllNodes(hasText(text, substring = true)).fetchSemanticsNodes().isNotEmpty()
        }
    }

    private fun typePin(pin: String) {
        for (digit in pin) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
    }

    private fun userIdOf(name: String): String = runBlocking {
        container.deviceAccess.roster().waiters.first { it.name == name }.userId
    }

    private fun shell(command: String): String {
        val output =
            InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        ParcelFileDescriptor.AutoCloseInputStream(output).use {
            return it.readBytes().toString(Charsets.UTF_8)
        }
    }

    /** Emparejar por la dirección escrita y aprobar el código como la computadora principal. */
    private fun pairByAddress() {
        waitFor(TAG_FIND_SERVER)
        compose.onNodeWithTag(TAG_ADDRESS).performTextReplacement(argument("address"))
        // Se pide con «Ir» del teclado: tocar el botón mientras el teclado termina de abrirse
        // (la pantalla se reacomoda) a veces no llega a ser un toque. El botón lo prueba
        // AccessScreensTest.
        compose.onNodeWithTag(TAG_ADDRESS).performImeAction()
        waitFor(TAG_PAIRING_CODE)
        val shown =
            compose
                .onNodeWithTag(TAG_PAIRING_CODE)
                .fetchSemanticsNode()
                .config[SemanticsProperties.Text]
                .joinToString("") { it.text }
        val code = shown.replace(" ", "")
        log("código en pantalla: $shown")
        assertEquals(6, code.length)
        // Un código equivocado no aprueba (y cuenta un intento).
        val wrong = ((code.toInt() + 1) % 1_000_000).toString().padStart(6, '0')
        assertEquals("PAIRING_CODE_INVALID", main.approvePairing(wrong).errorCode)
        val approved = main.approvePairing(code)
        assertEquals(approved.body.toString(), 200, approved.status)
        waitFor(TAG_ENTRY)
        // La huella fijada es la del certificado del Go temporal; la credencial va cifrada.
        val stored = runBlocking { container.database.access().pairedServer() }
        assertNotNull(stored)
        assertEquals(argument("pin"), stored!!.fingerprintHex)
        // El servidor emparejado es el que se indicó (si se indicó su server_id).
        val serverId = arguments.getString("serverId")
        if (!serverId.isNullOrBlank()) {
            assertEquals(serverId, stored.serverId)
        }
        assertFalse(
            String(stored.sealedDeviceToken, Charsets.ISO_8859_1).all { it.isLetterOrDigit() },
        )
    }

    @Test
    fun paso1_emparejarConElCodigo() {
        main.ensureWaiters(mapOf("ana.a1" to ("Ana" to ANA_PIN), "beto.a1" to ("Beto" to BETO_PIN)))
        TestDevice.reset()
        launch()
        pairByAddress()
        waitFor("$TAG_WAITER_PREFIX${userIdOf("Ana")}")
        waitFor("$TAG_WAITER_PREFIX${userIdOf("Beto")}")
        val status = container.deviceAccess.status.value
        assertTrue("$status", status is DeviceStatus.Ready && status.online)
    }

    // GET restaurant/state con las cabeceras del mesero [owner] (revisión R3, M5): Go responde a
    // nombre de ese mesero.
    private fun stateUserOf(owner: String): String? = runBlocking {
        val api = container.deviceAccess.serverApi()!!
        val headers = container.deviceAccess.requestHeaders(owner)!!
        val outcome = api.get("restaurant/state", JsonObject.serializer(), headers)
        assertTrue("$outcome", outcome is ApiOutcome.Success)
        val user = (outcome as ApiOutcome.Success).value["user"]?.jsonObject
        user?.get("id")?.jsonPrimitive?.content
    }

    @Test
    fun paso2_pinIncorrectoBloqueoALosCincoYEntrada() {
        launch()
        val beto = userIdOf("Beto")
        waitFor("$TAG_WAITER_PREFIX$beto")
        // Beto entra una vez con conexión (queda su verificador) y cambia de mesero.
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$beto").performClick()
        typePin(BETO_PIN)
        waitFor(TAG_TABLES_SEARCH)
        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        waitFor("$TAG_WAITER_PREFIX$beto")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$beto").performClick()
        repeat(4) { attempt ->
            typePin("1111")
            waitForText("PIN incorrecto")
            log("intento ${attempt + 1}: PIN incorrecto")
        }
        typePin("1111")
        waitForText("Intenta de nuevo a las")
        compose.onNodeWithTag(TAG_ENTRY_MESSAGE).assertIsDisplayed()
        // Bloqueado: también con el PIN correcto.
        typePin(BETO_PIN)
        waitForText("Intenta de nuevo a las")
        assertNull(container.deviceAccess.activeWaiter.value)
        // El bloqueo de Go cierra su entrada sin conexión (revisión R3, M2): se prueba en el paso
        // 3.
        assertTrue(
            "el bloqueo de Go cierra la entrada sin conexión",
            runBlocking { container.database.access().knownWaiter(beto) }!!.offlineClosed,
        )

        // En el teléfono el teclado ocupa la pantalla: se vuelve a la lista para elegir a Ana.
        compose.onNodeWithTag(TAG_ENTRY_BACK).performClick()
        val ana = userIdOf("Ana")
        waitFor("$TAG_WAITER_PREFIX$ana")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        typePin(ANA_PIN)
        waitFor(TAG_TABLES_SEARCH)
        val active = container.deviceAccess.activeWaiter.value
        assertEquals(ana, active?.userId)
        assertTrue(active!!.hasServerSession)
        assertNotNull(container.deviceAccess.sessionToken(ana))
        assertNotNull(runBlocking { container.deviceAccess.offlineCredential(ana) })
        assertNotNull(runBlocking { container.database.access().knownWaiter(ana) }?.sealedVerifier)
        // Las cabeceras son del mesero dueño de la operación, aunque el activo sea otro (M5).
        assertEquals(ana, stateUserOf(ana))
        assertEquals(beto, stateUserOf(beto))
        // Cambio rápido de mesero desde el menú visible.
        compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
        compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
        waitFor(TAG_ENTRY)
        assertNull(container.deviceAccess.activeWaiter.value)
    }

    @Test
    fun paso3_entradaSinConexion() {
        val byHost = arguments.getString("hostNetworkCut") == "true"
        if (!byHost) {
            assumeTrue("Sin modo avión por shell antes de Android 11", Build.VERSION.SDK_INT >= 30)
            shell("cmd connectivity airplane-mode enable")
        }
        try {
            compose.waitUntil(timeoutMillis = 20_000) {
                container.connectionMonitor.current() == LinkState.OFFLINE
            }
            launch()
            waitFor(TAG_ENTRY_OFFLINE)
            val known = runBlocking { container.database.access().knownWaiters() }
            assertEquals(setOf("Ana", "Beto"), known.map { it.name }.toSet())
            val ana = known.single { it.name == "Ana" }.userId
            val beto = known.single { it.name == "Beto" }.userId
            // Beto tiene el PIN bloqueado en Go (paso 2): sin conexión tampoco entra, ni con el PIN
            // correcto, y la lista lo dice (revisión R3, M2).
            waitForText("Sin conexión no puede entrar")
            assertEquals(
                EntryResult.OfflineClosed,
                runBlocking { container.deviceAccess.enter(beto, BETO_PIN) },
            )
            // Alguien que nunca entró en este dispositivo no entra sin conexión.
            val stranger = "bbbbbbbb-0000-4000-8000-000000000000"
            assertEquals(
                EntryResult.UnknownOffline,
                runBlocking { container.deviceAccess.enter(stranger, BETO_PIN) },
            )
            compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
            typePin("9999")
            waitForText("PIN incorrecto")
            typePin(ANA_PIN)
            waitFor(TAG_TABLES_SEARCH)
            val active = container.deviceAccess.activeWaiter.value
            assertEquals(ana, active?.userId)
            assertFalse("sin conexión no hay sesión de Go", active!!.hasServerSession)
            assertNull(container.deviceAccess.sessionToken(ana))
            compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
            compose.onNodeWithText("Entraste sin conexión").assertIsDisplayed()
        } finally {
            if (!byHost) {
                shell("cmd connectivity airplane-mode disable")
            }
        }
    }

    @Test
    fun paso4_pinCambiadoPideVolverAEscribirloSinSalirDeMesas() {
        compose.waitUntil(timeoutMillis = 60_000) {
            container.connectionMonitor.current() == LinkState.ONLINE
        }
        // Ana entró con conexión en el paso 2: su verificador está guardado.
        val ana = runBlocking {
            container.database.access().knownWaiters()
        }
            .single { it.name == "Ana" }
            .userId
        assertNotNull(runBlocking { container.database.access().knownWaiter(ana) }?.sealedVerifier)
        launch()
        waitFor("$TAG_WAITER_PREFIX$ana")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$ana").performClick()
        typePin(ANA_PIN)
        waitFor(TAG_TABLES_SEARCH)
        // En la principal le cambian el PIN mientras trabaja: al volver a comprobar (otra
        // pin_version) su acceso deja de valer, pero Ana no sale de Mesas (revisión R3, M3).
        main.changePin("ana.a1", ANA_NEW_PIN)
        runBlocking { container.deviceAccess.recheck() }
        waitFor(TAG_REENTER)
        waitForText("Vuelve a escribir tu PIN")
        compose.onNodeWithTag(TAG_TABLES_SEARCH).assertExists()
        assertEquals(ana, container.deviceAccess.activeWaiter.value?.userId)
        assertNull(runBlocking { container.database.access().knownWaiter(ana) })
        assertNull(runBlocking { container.deviceAccess.offlineCredential(ana) })
        assertNull(container.deviceAccess.sessionToken(ana))
        // El PIN viejo ya no sirve; el nuevo sí, y renueva el verificador con la versión nueva.
        typePin(ANA_PIN)
        waitFor(TAG_REENTER_MESSAGE)
        waitForText("PIN incorrecto")
        typePin(ANA_NEW_PIN)
        compose.waitUntil(timeoutMillis = 40_000) {
            compose.onAllNodes(hasTestTag(TAG_REENTER)).fetchSemanticsNodes().isEmpty()
        }
        compose.onNodeWithTag(TAG_TABLES_SEARCH).assertExists()
        val active = container.deviceAccess.activeWaiter.value!!
        assertTrue(active.hasServerSession)
        assertFalse(active.mustReenterPin)
        assertEquals(2, runBlocking { container.database.access().knownWaiter(ana) }?.pinVersion)
        assertEquals(ana, stateUserOf(ana))
    }

    @Test
    fun paso5_versionViejaPideActualizarLaAplicacion() {
        val version = argument("clientVersion")
        assertEquals(version, container.clientIdentity.version)
        launch()
        waitFor(TAG_UPDATE_APP)
        compose.onNodeWithText("Actualiza esta aplicación").assertIsDisplayed()
        compose.onNodeWithText("Versión de esta aplicación: $version").assertIsDisplayed()
        compose.onNodeWithTag(TAG_CHECK_AGAIN).performClick()
        waitFor(TAG_UPDATE_APP)
        // Cualquier ruta de la aplicación responde 426 a esa versión.
        val stored = runBlocking { container.database.access().pairedServer() }!!
        val api =
            ServerApi(
                stored.baseUrl.toHttpUrl(),
                PinnedHttpClient.create(Hex.decode(stored.fingerprintHex), ClientIdentity(version)),
            )
        val outcome = runBlocking { api.get("auth/pin-users", PinUsersResponse.serializer()) }
        assertTrue("$outcome", outcome is ApiOutcome.Rejected)
        val problem = (outcome as ApiOutcome.Rejected).problem
        assertEquals(426, problem.status)
        assertTrue(ApiErrors.toAppError(problem) is AppError.UpdateRequired)
    }

    @Test
    fun paso6_certificadoCambiadoNoConectaYPideEmparejarDeNuevo() {
        launch()
        waitFor(TAG_REPAIR)
        assertEquals(
            DeviceStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED),
            container.deviceAccess.status.value,
        )
        compose
            .onNodeWithText("necesita emparejarse de nuevo", substring = true)
            .assertIsDisplayed()
        assertNull(container.deviceAccess.activeWaiter.value)
        compose.onNodeWithTag(TAG_REPAIR_ACTION).performScrollTo().performClick()
        compose.onNodeWithTag(TAG_REPAIR_CONFIRM).performClick()
        // De vuelta a buscar el servidor y emparejar con el nuevo (su huella es otra).
        pairByAddress()
        waitFor(TAG_ENTRY_EMPTY)
    }

    @Test
    fun paso7_dispositivoRetiradoBorraTodo() {
        launch()
        waitFor(TAG_ENTRY)
        assertEquals(1, main.revokeAndroidDevices())
        runBlocking { container.deviceAccess.recheck() }
        waitFor(TAG_RETIRED)
        compose.onNodeWithText("Este dispositivo fue retirado del restaurante").assertIsDisplayed()
        val dao = container.database.access()
        assertNull(runBlocking { dao.pairedServer() })
        assertEquals(0, runBlocking { dao.knownWaiters() }.size)
        compose.onNodeWithTag(TAG_RETIRED_PAIR_AGAIN).performScrollTo().performClick()
        waitFor(TAG_FIND_SERVER)
    }

    private companion object {
        const val TAG = "AdelOnGo"
        const val ANA_PIN = "4829"
        const val ANA_NEW_PIN = "7305"
        const val BETO_PIN = "6152"
    }
}
