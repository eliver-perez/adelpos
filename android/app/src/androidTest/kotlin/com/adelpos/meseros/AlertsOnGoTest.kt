package com.adelpos.meseros

import android.Manifest
import android.app.ActivityManager
import android.app.NotificationManager
import android.app.UiAutomation
import android.content.Intent
import android.os.Build
import android.os.ParcelFileDescriptor
import android.service.notification.StatusBarNotification
import android.util.Log
import android.view.KeyEvent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.capture.TAG_ORDER_PANEL
import com.adelpos.meseros.core.capture.TAG_SEND_TO_KITCHEN
import com.adelpos.meseros.core.capture.TAG_SYNC_PREFIX
import com.adelpos.meseros.core.data.api.ClientIdentity
import com.adelpos.meseros.core.data.api.PinnedHttpClient
import com.adelpos.meseros.core.data.ready.ReadyChannelState
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.ready.ReadyChannels
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.TAG_PRODUCT_GRID
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_PAD
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.core.ui.components.TAG_WAITER_MENU
import com.adelpos.meseros.core.ui.components.TAG_WAITER_SWITCH
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.TAG_PANE_OPEN_SECONDARY
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.access.status.TAG_RETIRED
import com.adelpos.meseros.feature.waiter.tables.TAG_ACCOUNT_PANE
import com.adelpos.meseros.feature.waiter.tables.TAG_NEW_ACCOUNT
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_LIST
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SEARCH
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLE_PREFIX
import com.adelpos.meseros.ready.ReadyAlertNotifications
import com.adelpos.meseros.ready.ReadyAlertsService
import com.adelpos.meseros.ready.TAG_ALERTS_OFF
import com.adelpos.meseros.ready.TAG_IN_APP
import com.adelpos.meseros.ready.TAG_PERMISSION
import com.adelpos.meseros.ready.TAG_PERMISSION_LATER
import java.io.File
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Aviso de platillos listos (etapa A2, §4) contra el **Go temporal** del arnés, nunca el servicio
 * real: la prueba hace de computadora principal y de cocina por la escucha local ([MainComputer]:
 * abre cuentas a nombre del mesero, las envía a cocina y marca «Listo» con `kitchen/states`, como
 * `FieldOnGoTest`) y empareja un **segundo dispositivo** `android` por la API para comprobar a
 * quién llega. Un paso por instrumentación, en orden, con la aplicación instalada una sola vez
 * (`scripts/android-e2e.sh --suite alerts`, que pasa `readyAlerts=on`):
 *
 * - `r1` prepara el restaurante (Ana y Beto, cocina en pantalla, mesas), empareja y entra con
 *   conexión: el servicio arranca con «AD-EL POS conectado» y, antes, la explicación de los
 *   permisos (con `alertPermissions=ask`).
 * - `r2` Ana captura y envía a cocina; con la **aplicación cerrada** (tarea quitada de recientes),
 *   la **pantalla apagada** y Doze forzado (`force-idle`), cocina marca «Listo» y el aviso llega.
 * - `r3` no llega a otro mesero ni a otro dispositivo (Beto en el segundo dispositivo lee
 *   `ready-alerts` con su credencial) y transferir la cuenta cambia el destinatario.
 * - `r4` **el guion corta la red** del dispositivo, cocina marca «Listo» durante el corte (desde la
 *   Mac) y devuelve la red: el aviso llega **una sola vez**.
 * - `r5` agrupado por cuenta (una notificación con el acumulado) y el resumen con dos cuentas.
 * - `r6` en «¿Quién eres?» llega el aviso; al tocarlo pide el PIN de Ana y abre la cuenta; con la
 *   cuenta a la vista, el siguiente «Listo» es un aviso dentro de la aplicación.
 * - `r7` sonido y vibración apagados desde Terminales (canal «listos_silencio») y «Avisar platillos
 *   listos» apagado detiene el servicio.
 * - `r8` deja el servicio corriendo con una cuenta enviada a cocina: **el guion** abre la
 *   aplicación, mata el proceso, espera el reinicio pegajoso, marca «Listo» desde la Mac y busca el
 *   aviso con `dumpsys notification --noredact`.
 * - `r10` (antes de `r9`) en «¿Quién eres?», sin mesero activo, con la pantalla encendida (más que
 *   el periodo de 5 minutos del catálogo) y apagada, la aplicación **no** usa la sesión del último
 *   mesero: Go no renueva su `last_seen_at` (A2-D12: así, de noche, Go responde `active: false`).
 * - `r9` al retirar el dispositivo el servicio se detiene y se borran sus datos.
 *
 * Argumentos: `local`, `address`, `net`, `pin`, `mainDevice` y `mainToken` (los arma el guion). Sin
 * ellos se omite.
 */
@RunWith(AndroidJUnit4::class)
class AlertsOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val automation: UiAutomation = instrumentation.uiAutomation
    private lateinit var main: MainComputer
    private var scenario: ActivityScenario<MainActivity>? = null
    private val app = TestDevice.app
    private val container
        get() = app.container

    private val controller
        get() = container.readyAlerts

    private val manager = app.getSystemService(NotificationManager::class.java)
    private val stateFile = File(app.filesDir, "alerts-on-go.json")

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    @Before
    fun mainComputer() {
        main = MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
        assumeTrue("El aviso va encendido en esta suite (readyAlerts=on)", readyAlertsOn())
        manager.cancelAll()
    }

    @After
    fun close() {
        scenario?.close()
    }

    // --- Pasos ---

    @Test
    fun r1_prepararEmparejarYEntrarArrancaElServicio() {
        val waiters =
            main.ensureWaiters(
                mapOf(ANA_USER to ("Ana" to ANA_PIN), BETO_USER to ("Beto" to BETO_PIN)),
            )
        grantWaiterPermissions()
        val ids =
            mutableMapOf("ana" to waiters.getValue(ANA_USER), "beto" to waiters.getValue(BETO_USER))
        ids.putAll(createCatalogWithKitchen())
        ids.putAll(createTables())
        saveIds(ids)
        TestDevice.reset()
        launch()
        pairByAddress()
        enter(ids.getValue("ana"), ANA_PIN)
        // §4.3: la primera vez que el servicio arranca se explican los permisos antes del diálogo
        // de Android (notificaciones en Android 13+, batería en todas). «Ahora no» deja seguir.
        if (arguments.getString("alertPermissions") == "ask") {
            val expected =
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    2
                } else {
                    1
                }
            repeat(expected) {
                waitFor(TAG_PERMISSION, timeout = 30_000)
                compose.onNodeWithTag(TAG_PERMISSION_LATER).performClick()
                compose.waitForIdle()
                Thread.sleep(SETTLE_MILLIS)
            }
            assertFalse("no se vuelve a preguntar", present(TAG_PERMISSION))
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                // RD6: sin notificaciones, un aviso discreto arriba de Mesas; en la siguiente
                // entrada con conexión se vuelve a ofrecer el permiso (solo ese, una vez).
                waitFor(TAG_ALERTS_OFF)
                compose.onNodeWithTag(TAG_WAITER_MENU).performClick()
                compose.onNodeWithTag(TAG_WAITER_SWITCH).performClick()
                enter(ids.getValue("ana"), ANA_PIN)
                waitFor(TAG_PERMISSION, timeout = 30_000)
                compose.onNodeWithTag(TAG_PERMISSION_LATER).performClick()
                compose.waitForIdle()
                Thread.sleep(SETTLE_MILLIS)
                assertFalse("una sola vez por entrada", present(TAG_PERMISSION))
            }
        }
        // El permiso se concede aquí (no con el diálogo de Android): como lo haría la respuesta
        // del diálogo, se vuelve a publicar la notificación permanente.
        grantNotifications()
        controller.refreshOngoing()
        waitForService()
        waitUntil("el canal de avisos conectado") {
            controller.connection.value == ReadyChannelState.Connected
        }
        val status = runBlocking { controller.ledger.status() }
        assertTrue("Go dice que hay a quién avisar: $status", status?.active == true)
        val deviceId = container.deviceAccess.deviceCredentials()!!.deviceId
        saveIds(ids + ("device" to deviceId))
        log("r1: servicio activo en el dispositivo $deviceId")
    }

    @Test
    fun r2_llegaConLaPantallaApagadaYLaAplicacionCerrada() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        // Ana captura en la mesa R4 y envía a cocina (la cola de la A1, con el servicio activo).
        newAccountOn(ids.getValue("table4"))
        addProduct(ids.getValue("filete"))
        addProduct(ids.getValue("filete"))
        showOrder()
        waitFor("${TAG_SYNC_PREFIX}saved")
        tap(TAG_SEND_TO_KITCHEN)
        val sent = waitForOrder(ids.getValue("ana")) { it.state == OrderState.RECEIVED }
        val accountId = sent.receivedAccountId!!
        // La aplicación se cierra: la tarea sale de recientes (el servicio sigue).
        closeApp()
        waitForService()
        try {
            screenOffAndIdle()
            setKitchenState(kitchenTasks(accountId), "ready")
            val alert = waitForAlert(accountId, timeout = 60_000)
            assertEquals("Mesa R4 · 2 platillos listos", title(alert))
            assertEquals("2 × Filete R", text(alert))
            val idle = shell("dumpsys deviceidle get deep").trim()
            log("r2: aviso con la pantalla apagada; Doze profundo: $idle")
            assertEquals("IDLE", idle)
            assertEquals(ReadyChannels.SOUND_AND_VIBRATION, alert.notification.channelId)
        } finally {
            restoreScreenAndDoze()
        }
        saveIds(ids + ("r2account" to accountId))
    }

    @Test
    fun r3_noLlegaAOtroMeseroNiAOtroDispositivoYTransferirCambiaElDestinatario() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        // El segundo dispositivo: emparejado por la API, con Beto que entra con su PIN.
        val server = GoTestServer()
        val client =
            PinnedHttpClient.create(server.pin, ClientIdentity(version = BuildConfig.VERSION_NAME))
        val other = server.pairAndroid(client, server.mainComputer())
        pinLogin(client, server.network, other, ids.getValue("beto"), BETO_PIN)

        val forAna =
            openAccountFromMain(ids.getValue("table5"), ids.getValue("ana"), ids.getValue("filete"))
        val forBeto =
            openAccountFromMain(ids.getValue("table6"), ids.getValue("beto"), ids.getValue("tacos"))
        setKitchenState(kitchenTasks(forAna), "ready")
        setKitchenState(kitchenTasks(forBeto), "ready")

        val alert = waitForAlert(forAna)
        assertEquals("Mesa R5 · 1 platillo listo", title(alert))
        // Lo de Beto no llega a este dispositivo (Beto nunca entró aquí)…
        Thread.sleep(QUIET_MILLIS)
        assertTrue("el aviso de Beto no llega aquí", alertsOf(forBeto).isEmpty())
        // …y lo de Ana no llega al dispositivo de Beto.
        val otherAlerts = readyAlerts(client, server.network, other)
        assertEquals(listOf(forBeto), otherAlerts.map { it.text("account_id") })

        // Transferir: la cuenta nueva de Ana pasa a Beto antes del «Listo».
        val transferred =
            openAccountFromMain(ids.getValue("table7"), ids.getValue("ana"), ids.getValue("filete"))
        assign(transferred, ids.getValue("beto"))
        setKitchenState(kitchenTasks(transferred), "ready")
        waitUntil("el aviso llega al dispositivo de Beto", timeout = 30_000) {
            readyAlerts(client, server.network, other).any {
                it.text("account_id") == transferred &&
                    it.text("waiter_user_id") == ids.getValue("beto")
            }
        }
        Thread.sleep(QUIET_MILLIS)
        assertTrue("ya no llega a Ana", alertsOf(transferred).isEmpty())
        log("r3: a quién llega cada aviso, comprobado con el segundo dispositivo ${other.deviceId}")
    }

    @Test
    fun r4_corteDeRedDuranteElListoLlegaUnaSolaVez() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        val accountId =
            openAccountFromMain(ids.getValue("table8"), ids.getValue("ana"), ids.getValue("tacos"))
        waitUntil("el canal de avisos conectado") {
            controller.connection.value == ReadyChannelState.Connected
        }
        shell("rm -f $CUT_DONE_FILE")
        // El guion espera esta línea para cortar la red, marcar «Listo» desde la Mac y devolverla.
        log("r4 listo para cortar cuenta=$accountId")
        // En Android 8–10, `svc … disable` no siempre corta la conexión abierta y el canal la da
        // por
        // muerta tras 45 s sin un byte (el guion mantiene el corte hasta que se ve): hasta 90 s.
        waitUntil("el canal nota el corte", timeout = 90_000) {
            controller.connection.value is ReadyChannelState.Waiting
        }
        log("r4 corte visto por el canal: ${controller.connection.value}")
        waitUntil("el guion marcó «Listo» y devolvió la red", timeout = 120_000) {
            shell("ls $CUT_DONE_FILE").contains(CUT_DONE_FILE)
        }
        val alert = waitForAlert(accountId, timeout = 90_000)
        assertEquals("Mesa R8 · 1 platillo listo", title(alert))
        val posted = alert.postTime
        // Una sola vez: ni otra notificación ni otro aviso guardado, aunque el canal se reconecte.
        Thread.sleep(QUIET_MILLIS)
        assertEquals(1, alertsOf(accountId).size)
        assertEquals(posted, alertsOf(accountId).single().postTime)
        val shown = alertsOf(accountId).single().notification.extras.getStringArray(SHOWN_KEY)
        assertEquals(1, shown?.size)
        shell("rm -f $CUT_DONE_FILE")
    }

    @Test
    fun r5_agrupadoPorCuentaYResumenConVariasCuentas() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        val accountId =
            openAccountFromMain(
                ids.getValue("table2"),
                ids.getValue("ana"),
                ids.getValue("filete"),
                ids.getValue("tacos"),
            )
        val tasks = kitchenTasksByProduct(accountId)
        setKitchenState(listOf(tasks.getValue("Filete R")), "ready")
        waitUntil("el primer aviso de la mesa R2") {
            alertsOf(accountId).singleOrNull()?.let(::title) == "Mesa R2 · 1 platillo listo"
        }
        setKitchenState(listOf(tasks.getValue("Tacos R")), "ready")
        waitUntil("el acumulado de la mesa R2 en la misma notificación") {
            alertsOf(accountId).singleOrNull()?.let(::title) == "Mesa R2 · 2 platillos listos"
        }
        assertEquals("1 × Filete R, 1 × Tacos R", text(alertsOf(accountId).single()))
        val second =
            openAccountFromMain(ids.getValue("table3"), ids.getValue("ana"), ids.getValue("filete"))
        setKitchenState(kitchenTasks(second), "ready")
        waitForAlert(second)
        waitUntil("el resumen del grupo") { summary() != null }
        assertEquals(1, alertsOf(accountId).size)
    }

    @Test
    fun r6_enQuienEresAlTocarloPideElPinYAbreLaCuenta() {
        val ids = loadIds()
        grantNotifications()
        val accountId =
            openAccountFromMain(
                ids.getValue("table1"),
                ids.getValue("ana"),
                ids.getValue("filete"),
                ids.getValue("tacos"),
            )
        val tasks = kitchenTasksByProduct(accountId)
        // La aplicación abierta como desde el lanzador (en su propia tarea, como en el dispositivo:
        // el
        // toque en el aviso vuelve a esa misma actividad) en «¿Quién eres?»: el servicio arranca
        // con
        // lo que dijo Go.
        startFromLauncher()
        waitFor(TAG_ENTRY)
        waitForService()
        setKitchenState(listOf(tasks.getValue("Filete R")), "ready")
        val alert = waitForAlert(accountId)
        assertEquals("Mesa R1 · 1 platillo listo", title(alert))
        // Tocar el aviso: pide el PIN de Ana (ya elegida) y después abre la cuenta.
        alert.notification.contentIntent.send()
        waitFor(TAG_PIN_PAD)
        typePin(ANA_PIN)
        // compose.waitUntil (y no un sondeo con Thread.sleep): en una prueba de Compose el reloj de
        // los efectos (sus `delay`) solo avanza mientras la prueba espera con Compose.
        compose.waitUntil(timeoutMillis = 40_000) { controller.viewingAccount.value == accountId }
        assertTrue(present(TAG_ORDER_PANEL) || present(TAG_PRODUCT_GRID))
        // Con la cuenta a la vista, el siguiente «Listo» es un aviso dentro de la aplicación.
        setKitchenState(listOf(tasks.getValue("Tacos R")), "ready")
        waitFor(TAG_IN_APP, timeout = 30_000)
        assertTrue(
            "sin notificación del sistema para la cuenta a la vista",
            alertsOf(accountId).isEmpty(),
        )
        instrumentation.runOnMainSync {
            app.getSystemService(ActivityManager::class.java).appTasks.forEach {
                it.finishAndRemoveTask()
            }
        }
    }

    @Test
    fun r7_sonidoYVibracionApagadosDesdeTerminalesYAvisoApagado() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        configureAlerts(ids.getValue("device"), enabled = true, sound = false, vibration = false)
        waitUntil("el canal en silencio") {
            manager.notificationChannels.map { it.id }.filter { it in ReadyChannels.READY } ==
                listOf(ReadyChannels.SILENT)
        }
        val channel = manager.getNotificationChannel(ReadyChannels.SILENT)
        assertNull(channel.sound)
        assertFalse(channel.shouldVibrate())
        assertEquals("Platillos listos", channel.name.toString())
        val accountId =
            openAccountFromMain(ids.getValue("table9"), ids.getValue("ana"), ids.getValue("filete"))
        setKitchenState(kitchenTasks(accountId), "ready")
        assertEquals(ReadyChannels.SILENT, waitForAlert(accountId).notification.channelId)

        // «Avisar platillos listos» apagado: Go dice active: false y el servicio se detiene.
        configureAlerts(ids.getValue("device"), enabled = false, sound = false, vibration = false)
        waitUntil("el servicio se detuvo", timeout = 30_000) { !serviceRunning() }
        waitUntil("sin notificación permanente") { ongoing() == null }
        configureAlerts(ids.getValue("device"), enabled = true, sound = true, vibration = true)
    }

    @Test
    fun r8a_servicioConUnaCuentaEnCocinaParaElReinicioPegajoso() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        val accountId =
            openAccountFromMain(
                ids.getValue("table10"),
                ids.getValue("ana"),
                ids.getValue("filete"),
                ids.getValue("filete"),
            )
        waitUntil("el canal de avisos conectado") {
            controller.connection.value == ReadyChannelState.Connected
        }
        // El guion toma esta cuenta para marcarla «Listo» después del reinicio pegajoso.
        log("r8 cuenta=$accountId")
    }

    @Test
    fun r10_enQuienEresNoSeUsaLaSesionDelUltimoMesero() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        // Con Ana en Mesas, la aplicación usa su sesión (canal de eventos, mesas, catálogo).
        Thread.sleep(QUIET_MILLIS)
        tap(TAG_WAITER_MENU)
        tap(TAG_WAITER_SWITCH)
        waitFor("$TAG_WAITER_PREFIX${ids.getValue("ana")}")
        assertNull(container.deviceAccess.activeWaiter.value)
        // Lo que iba en camino termina; desde aquí no debe haber ninguna petición con su sesión.
        Thread.sleep(QUIET_MILLIS)
        val deviceId = container.deviceAccess.deviceCredentials()!!.deviceId
        val before = lastSeen(deviceId)
        log("r10: última actividad de la sesión de Ana al pasar a «¿Quién eres?»: $before")
        Thread.sleep(WHO_SCREEN_ON_MILLIS)
        val screenOn = lastSeen(deviceId)
        shell("input keyevent ${KeyEvent.KEYCODE_SLEEP}")
        try {
            Thread.sleep(WHO_SCREEN_OFF_MILLIS)
        } finally {
            shell("input keyevent ${KeyEvent.KEYCODE_WAKEUP}")
            shell("wm dismiss-keyguard")
        }
        val screenOff = lastSeen(deviceId)
        assertEquals("con la pantalla encendida no se usó la sesión de Ana", before, screenOn)
        assertEquals("con la pantalla apagada no se usó la sesión de Ana", before, screenOff)
        // El aviso sigue con la credencial del dispositivo (no es una sesión de mesero).
        assertTrue(serviceRunning())
        log(
            "r10: sin uso de la sesión de Ana en ${WHO_SCREEN_ON_MILLIS + WHO_SCREEN_OFF_MILLIS} ms",
        )
    }

    /** `last_seen_at` de la sesión vigente más reciente de Ana en este dispositivo (Go). */
    private fun lastSeen(deviceId: String): String {
        val session =
            ok("GET", "sessions")
                .body
                .jsonArray
                .map { it.jsonObject }
                .filter {
                    it.text("username") == ANA_USER &&
                        it.text("device_id") == deviceId &&
                        it["revoked_at"] is JsonNull
                }
                .maxBy { it.text("created_at") }
        return session.text("last_seen_at")
    }

    @Test
    fun r9_alRetirarElDispositivoSeDetieneElServicio() {
        val ids = loadIds()
        grantNotifications()
        launch()
        enter(ids.getValue("ana"), ANA_PIN)
        waitForService()
        assertTrue(main.revokeAndroidDevices() >= 1)
        waitUntil("el servicio se detuvo", timeout = 60_000) { !serviceRunning() }
        waitUntil("sin notificación permanente") { ongoing() == null }
        waitFor(TAG_RETIRED, timeout = 60_000)
        assertNull("se borró el cursor del aviso", runBlocking { controller.ledger.status() })
        assertTrue(runBlocking { controller.ledger.seenIds() }.isEmpty())
    }

    // --- Dispositivo ---

    private fun readyAlertsOn(): Boolean = arguments.getString("readyAlerts") == "on"

    private fun grantNotifications() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            automation.grantRuntimePermission(
                app.packageName,
                Manifest.permission.POST_NOTIFICATIONS,
            )
        }
    }

    private fun waitForService() {
        waitUntil("el servicio de avisos en primer plano", timeout = 30_000) {
            serviceRunning() && ongoing() != null
        }
    }

    @Suppress("DEPRECATION")
    private fun serviceRunning(): Boolean =
        app.getSystemService(ActivityManager::class.java).getRunningServices(Int.MAX_VALUE).any {
            it.service.className == ReadyAlertsService::class.java.name &&
                it.started &&
                it.foreground
        }

    private fun ongoing(): StatusBarNotification? =
        manager.activeNotifications.firstOrNull { it.id == ReadyAlertNotifications.ONGOING_ID }

    private fun summary(): StatusBarNotification? =
        manager.activeNotifications.firstOrNull { it.id == ReadyAlertNotifications.SUMMARY_ID }

    private fun alertsOf(accountId: String): List<StatusBarNotification> =
        manager.activeNotifications.filter {
            it.id == ReadyAlertNotifications.ALERT_ID && it.tag == accountId
        }

    private fun waitForAlert(accountId: String, timeout: Long = 40_000): StatusBarNotification {
        waitUntil("el aviso de la cuenta $accountId", timeout) { alertsOf(accountId).isNotEmpty() }
        return alertsOf(accountId).single()
    }

    private fun title(notification: StatusBarNotification): String? =
        notification.notification.extras.getString("android.title")

    private fun text(notification: StatusBarNotification): String? =
        notification.notification.extras.getCharSequence("android.text")?.toString()

    /** Quita la tarea de recientes (como deslizarla): la actividad se va, el proceso sigue. */
    private fun closeApp() {
        scenario = null
        instrumentation.runOnMainSync {
            app.getSystemService(ActivityManager::class.java).appTasks.forEach {
                it.finishAndRemoveTask()
            }
        }
        waitUntil("la aplicación cerrada") { !container.appForeground.inForeground.value }
    }

    /**
     * Pantalla apagada, sin cargador y Doze profundo forzado (se restaura en
     * [restoreScreenAndDoze]).
     */
    private fun screenOffAndIdle() {
        shell("input keyevent ${KeyEvent.KEYCODE_SLEEP}")
        shell("dumpsys battery unplug")
        shell("dumpsys deviceidle force-idle")
        log("r2: ${shell("dumpsys deviceidle get deep").trim()} con la pantalla apagada")
    }

    private fun restoreScreenAndDoze() {
        shell("dumpsys deviceidle unforce")
        shell("dumpsys battery reset")
        shell("input keyevent ${KeyEvent.KEYCODE_WAKEUP}")
        shell("wm dismiss-keyguard")
    }

    private fun shell(command: String): String {
        val descriptor = automation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(descriptor).use {
            it.readBytes().decodeToString()
        }
    }

    // --- Pantallas ---

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    /**
     * Abre la aplicación como el lanzador, en la tarea de la aplicación (ActivityScenario la abre
     * en una tarea propia, y el toque en una notificación crearía otra actividad en otra tarea).
     */
    private fun startFromLauncher() {
        val intent =
            requireNotNull(app.packageManager.getLaunchIntentForPackage(app.packageName))
                .addFlags(
                    Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED,
                )
        app.startActivity(intent)
        waitUntil("la aplicación al frente") { container.appForeground.inForeground.value }
    }

    private fun pairByAddress() {
        waitFor(TAG_FIND_SERVER)
        compose.onNodeWithTag(TAG_ADDRESS).performTextReplacement(argument("address"))
        compose.onNodeWithTag(TAG_ADDRESS).performImeAction()
        waitFor(TAG_PAIRING_CODE)
        val shown =
            compose
                .onNodeWithTag(TAG_PAIRING_CODE)
                .fetchSemanticsNode()
                .config[SemanticsProperties.Text]
                .joinToString("") { it.text }
        val approved = main.approvePairing(shown.replace(" ", ""))
        assertEquals(approved.body.toString(), 200, approved.status)
        waitFor(TAG_ENTRY)
    }

    private fun typePin(pin: String) {
        for (digit in pin) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
    }

    private fun enter(userId: String, pin: String) {
        waitFor("$TAG_WAITER_PREFIX$userId")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$userId").performClick()
        typePin(pin)
        waitFor(TAG_TABLES_SEARCH)
    }

    private fun newAccountOn(tableId: String) {
        tap("$TAG_TABLE_PREFIX$tableId")
        tap(TAG_NEW_ACCOUNT)
        waitFor(TAG_PRODUCT_GRID)
    }

    private fun addProduct(productId: String) {
        if (present(TAG_ORDER_PANEL) && present(TAG_PANE_BACK)) {
            pressBack()
        }
        tap(productTag(productId))
        waitFor(TAG_CUSTOMIZER_ADD)
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        log("agregado $productId")
    }

    private fun showOrder() {
        if (!present(TAG_ORDER_PANEL) && present(TAG_PANE_OPEN_SECONDARY)) {
            compose.onNodeWithTag(TAG_PANE_OPEN_SECONDARY).performClick()
        }
        waitFor(TAG_ORDER_PANEL)
    }

    private fun pressBack() {
        SystemDialogs.dismissNotResponding(automation)
        instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        Thread.sleep(SETTLE_MILLIS)
    }

    private fun waitFor(tag: String, timeout: Long = 40_000) {
        var nextReveal = 0L
        try {
            compose.waitUntil(timeoutMillis = timeout) {
                present(tag) ||
                    (System.currentTimeMillis() >= nextReveal &&
                        run {
                            nextReveal = System.currentTimeMillis() + REVEAL_EVERY_MILLIS
                            reveal(tag)
                        })
            }
        } catch (failure: Throwable) {
            runCatching { compose.onRoot(useUnmergedTree = true).printToLog(TAG) }
            throw failure
        }
    }

    private fun waitUntil(what: String, timeout: Long = 40_000, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (!condition()) {
            check(System.currentTimeMillis() < deadline) { "No llegó: $what" }
            Thread.sleep(POLL_MILLIS)
        }
    }

    private fun present(tag: String): Boolean = runCatching {
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()
    }
        .getOrDefault(false)

    private fun reveal(tag: String): Boolean {
        for (list in LAZY_LISTS) {
            if (present(list)) {
                runCatching {
                    compose
                        .onAllNodes(hasTestTag(list), useUnmergedTree = true)
                        .onFirst()
                        .performScrollToNode(hasTestTag(tag))
                }
                if (present(tag)) {
                    return true
                }
            }
        }
        return false
    }

    private fun tap(tag: String) {
        waitFor(tag)
        val node = compose.onNodeWithTag(tag)
        if (runCatching { node.assertIsDisplayed() }.isFailure) {
            reveal(tag)
        }
        runCatching { node.performScrollTo() }
        node.performClick()
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private fun waitForOrder(userId: String, condition: (PendingOrder) -> Boolean): PendingOrder {
        var found: PendingOrder? = null
        waitUntil("el pedido de la mesa", 60_000) {
            found = runBlocking {
                container.database
                    .waiter()
                    .ordersOf(userId)
                    .map(WaiterStorage::toOrder)
                    .lastOrNull(condition)
            }
            found != null
        }
        return found!!
    }

    // --- La computadora principal (y la cocina) ---

    private fun ok(method: String, path: String, body: String? = null): MainComputer.Answer {
        val answer = main.call(method, path, body)
        assertEquals("$method $path: ${answer.body}", 200, answer.status)
        return answer
    }

    private fun grantWaiterPermissions() {
        val roles = ok("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.text("name") == ROLE }
        val permissions = WAITER_PERMISSIONS.joinToString(",") { "\"$it\"" }
        ok("PATCH", "roles/${role.text("id")}", """{"name":"$ROLE","permissions":[$permissions]}""")
    }

    /** «Filete R» y «Tacos R» en una categoría con su cocina en pantalla (KDS). */
    private fun createCatalogWithKitchen(): Map<String, String> {
        val category =
            ok(
                    "POST",
                    "catalog/categories",
                    """{"name":"Comida R","active":true,"color":"#d64545"}""",
                )
                .obj
                .text("id")
        fun product(name: String, code: String, price: String): String =
            ok(
                    "POST",
                    "catalog/products",
                    """{"name":"$name","active":true,"category_id":"$category",""" +
                        """"code":"$code","kind":"product","base_price":"$price"}""",
                )
                .obj
                .text("id")
        val station =
            ok(
                    "POST",
                    "kitchen/stations",
                    """{"operation_id":"${UUID.randomUUID()}","name":"Cocina R","active":true,""" +
                        """"reception":"kds","target_minutes":12,"warning_minutes":15,""" +
                        """"overdue_minutes":25,"grouping":"inherit","tracking":"inherit"}""",
                )
                .obj
                .text("id")
        ok(
            "POST",
            "kitchen/routes",
            """{"operation_id":"${UUID.randomUUID()}","category_id":"$category",""" +
                """"mode":"prepare","station_ids":["$station"]}""",
        )
        return mapOf(
            "filete" to product("Filete R", "R-FIL", "250"),
            "tacos" to product("Tacos R", "R-TAC", "90"),
        )
    }

    private fun createTables(): Map<String, String> {
        val area =
            ok(
                    "POST",
                    "restaurant/areas",
                    """{"operation_id":"${UUID.randomUUID()}","name":"Salón R","active":true}""",
                )
                .obj
                .text("id")
        val ids = mutableMapOf<String, String>()
        for (number in 1..TABLES) {
            // Dos filas de cinco: el mapa de Go mide 1000 × 560.
            val x = 20 + (number - 1) % 5 * 150
            val y = 20 + (number - 1) / 5 * 120
            val table =
                ok(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$area",""" +
                        """"name":"Mesa R$number","shape":"rectangle","x":$x,"y":$y,""" +
                        """"width":100,"height":72,"seats":4,"active":true}""",
                )
            ids["table$number"] = table.obj.text("id")
        }
        return ids
    }

    /**
     * La principal abre una cuenta en la mesa **a nombre de** [waiterId] (`assigned_user_id`, con
     * `accounts.transfer` del administrador) con [products] y la envía a cocina.
     */
    private fun openAccountFromMain(
        tableId: String,
        waiterId: String,
        vararg products: String,
    ): String {
        val items =
            products.joinToString(",") { productId ->
                """{"id":"${UUID.randomUUID()}","product_id":"$productId","variant_id":null,""" +
                    """"quantity":1,"customization":{"ingredients":[],"groups":[],""" +
                    """"components":[],"notes":""}}"""
            }
        val opened =
            ok(
                "POST",
                "restaurant/accounts",
                """{"operation_id":"${UUID.randomUUID()}","mode":"dine_in","table_id":"$tableId",""" +
                    """"assigned_user_id":"$waiterId","seen_account_ids":[],"items":[$items]}""",
            )
        val accountId = opened.obj.getValue("account").jsonObject.text("id")
        val preview = ok("GET", "restaurant/accounts/$accountId/preview").obj
        ok(
            "POST",
            "restaurant/accounts/$accountId/confirm",
            """{"operation_id":"${UUID.randomUUID()}","revision":${preview.getValue("revision")},""" +
                """"expected_fingerprint":"${preview.text("fingerprint")}"}""",
        )
        return accountId
    }

    private fun assign(accountId: String, waiterId: String) {
        val revision =
            ok("GET", "restaurant/accounts/$accountId")
                .obj
                .getValue("account")
                .jsonObject
                .getValue("revision")
                .jsonPrimitive
                .int
        ok(
            "POST",
            "restaurant/accounts/$accountId/assign",
            """{"operation_id":"${UUID.randomUUID()}","revision":$revision,""" +
                """"assigned_user_id":"$waiterId"}""",
        )
    }

    private fun kitchenTaskObjects(accountId: String): List<JsonObject> =
        ok("GET", "kitchen/board?state=all&account_id=$accountId")
            .obj
            .getValue("tasks")
            .jsonArray
            .map { it.jsonObject }
            .filter { it.text("state") != "cancelled" }

    private fun kitchenTasks(accountId: String): List<Pair<String, Int>> =
        kitchenTaskObjects(accountId).map {
            it.text("id") to it.getValue("revision").jsonPrimitive.int
        }

    /** Las tareas de cocina de la cuenta por nombre de producto (una unidad de cada uno). */
    private fun kitchenTasksByProduct(accountId: String): Map<String, Pair<String, Int>> =
        kitchenTaskObjects(accountId).associate { task ->
            val name = task.getValue("instructions").jsonObject.text("name")
            name to (task.text("id") to task.getValue("revision").jsonPrimitive.int)
        }

    private fun setKitchenState(tasks: List<Pair<String, Int>>, state: String) {
        assertTrue("Cocina tiene las tareas: $tasks", tasks.isNotEmpty())
        val references =
            tasks.joinToString(",") { (id, revision) -> """{"id":"$id","revision":$revision}""" }
        ok(
            "POST",
            "kitchen/states",
            """{"operation_id":"${UUID.randomUUID()}","state":"$state","tasks":[$references]}""",
        )
    }

    /** «Avisar platillos listos», «Sonido» y «Vibración» del dispositivo (Terminales › Editar). */
    private fun configureAlerts(
        deviceId: String,
        enabled: Boolean,
        sound: Boolean,
        vibration: Boolean,
    ) {
        val device =
            ok("GET", "devices")
                .body
                .jsonArray
                .map { it.jsonObject }
                .single { it.text("id") == deviceId }
        val register = device["register_id"]?.takeUnless { it is JsonNull }?.jsonPrimitive?.content
        val registerField = if (register == null) "null" else "\"$register\""
        ok(
            "PATCH",
            "devices/$deviceId",
            """{"name":"${device.text("name")}","register_id":$registerField,""" +
                """"keyboard_mode":"${device.text("keyboard_mode")}",""" +
                """"ready_alerts_enabled":$enabled,"ready_alerts_sound":$sound,""" +
                """"ready_alerts_vibration":$vibration}""",
        )
    }

    // --- El segundo dispositivo (por la API, con su propia credencial) ---

    private fun pinLogin(
        client: okhttp3.OkHttpClient,
        network: String,
        device: GoTestServer.Device,
        userId: String,
        pin: String,
    ) {
        val request =
            Request.Builder()
                .url("$network/api/v1/auth/pin-login")
                .header("X-Device-ID", device.deviceId)
                .header("X-Device-Token", device.deviceToken)
                .post("""{"user_id":"$userId","pin":"$pin"}""".toRequestBody(JSON))
                .build()
        client.newCall(request).execute().use { response ->
            assertEquals(response.body.string(), 200, response.code)
        }
    }

    /** `GET ready-alerts?after=0` con **solo** la credencial del segundo dispositivo. */
    private fun readyAlerts(
        client: okhttp3.OkHttpClient,
        network: String,
        device: GoTestServer.Device,
    ): List<JsonObject> {
        val request =
            Request.Builder()
                .url("$network/api/v1/ready-alerts?after=0")
                .header("X-Device-ID", device.deviceId)
                .header("X-Device-Token", device.deviceToken)
                .get()
                .build()
        client.newCall(request).execute().use { response ->
            val body = response.body.string()
            assertEquals(body, 200, response.code)
            val alerts = Json.parseToJsonElement(body).jsonObject.getValue("alerts") as JsonArray
            for (alert in alerts) {
                assertFalse("sin precios: $alert", alert.toString().contains("price"))
            }
            return alerts.map { it.jsonObject }
        }
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    // --- Estado entre pasos ---

    private fun saveIds(ids: Map<String, String>) {
        stateFile.writeText(JsonObject(ids.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun loadIds(): Map<String, String> {
        assumeTrue("Falta el estado de r1", stateFile.isFile)
        return Json.parseToJsonElement(stateFile.readText()).jsonObject.mapValues {
            it.value.jsonPrimitive.content
        }
    }

    private companion object {
        const val TAG = "AdelOnGo"
        const val ANA_USER = "ana.r"
        const val ANA_PIN = "4829"
        const val BETO_USER = "beto.r"
        const val BETO_PIN = "5713"
        const val ROLE = "Meseros A1"
        const val TABLES = 10
        const val POLL_MILLIS = 250L
        const val SETTLE_MILLIS = 1_500L
        const val REVEAL_EVERY_MILLIS = 1_000L
        const val QUIET_MILLIS = 5_000L

        /** `r10`: más que el periodo del catálogo (5 minutos) con la pantalla encendida. */
        const val WHO_SCREEN_ON_MILLIS = 330_000L
        const val WHO_SCREEN_OFF_MILLIS = 120_000L

        /** Lo crea el guion (como shell) cuando ya marcó «Listo» y devolvió la red en `r4`. */
        const val CUT_DONE_FILE = "/data/local/tmp/adelpos-r4-listo"

        /** Los avisos (sus `key`) que contiene la notificación de una cuenta (sus extras). */
        const val SHOWN_KEY = "adelpos.ready.shown.keys"

        val JSON = "application/json; charset=utf-8".toMediaType()

        val LAZY_LISTS =
            listOf(TAG_TABLES_LIST, TAG_ACCOUNT_PANE, TAG_ORDER_PANEL, TAG_PRODUCT_GRID)

        /** Los permisos de mesero de Go (A1-D17); el aviso no pide ninguno nuevo. */
        val WAITER_PERMISSIONS =
            listOf(
                "waiter_devices.use",
                "restaurant.tables.read",
                "restaurant.areas.read",
                "accounts.read",
                "accounts.open",
                "orders.capture",
                "orders.confirm",
                "orders.edit",
                "orders.cancel",
                "catalog.read",
                "catalog.prices.read",
                "events.read",
            )
    }
}
