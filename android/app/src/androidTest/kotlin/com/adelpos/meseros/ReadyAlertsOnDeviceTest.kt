package com.adelpos.meseros

import android.Manifest
import android.app.ActivityManager
import android.app.NotificationManager
import android.content.Intent
import android.os.Build
import android.service.notification.StatusBarNotification
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.core.content.ContextCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyChannels
import com.adelpos.meseros.core.domain.ready.ReadyItem
import com.adelpos.meseros.core.domain.ready.ReadyNotice
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.ready.InAppAlerts
import com.adelpos.meseros.ready.ReadyAlertNotifications
import com.adelpos.meseros.ready.ReadyAlertsHelp
import com.adelpos.meseros.ready.ReadyAlertsService
import com.adelpos.meseros.ready.TAG_HELP
import com.adelpos.meseros.ready.TAG_HELP_BATTERY
import com.adelpos.meseros.ready.TAG_HELP_CLOSE
import com.adelpos.meseros.ready.TAG_HELP_NOTIFICATIONS
import com.adelpos.meseros.ready.TAG_HELP_STATE
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El aviso de platillos listos en el dispositivo, sin servidor (etapa A2, Fase D): el servicio en
 * primer plano arranca con su notificación permanente y se detiene (también al borrar los datos del
 * dispositivo), un solo canal visible «Platillos listos» según el sonido y la vibración, una
 * notificación por cuenta con el acumulado y el resumen, sin repetir un aviso a la vista, el aviso
 * dentro de la aplicación y la pantalla de ayuda. Con el dispositivo emparejado a una dirección
 * donde no responde nadie (`TestDevice`, nunca el servicio real).
 */
@RunWith(AndroidJUnit4::class)
class ReadyAlertsOnDeviceTest {
    @get:Rule val compose = createComposeRule()

    private val app = TestDevice.app
    private val manager = app.getSystemService(NotificationManager::class.java)
    private val controller
        get() = app.container.readyAlerts

    @Before
    fun prepare() {
        // Sin el permiso (Android 13+) Android no muestra ni registra las notificaciones.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            InstrumentationRegistry.getInstrumentation()
                .uiAutomation
                .grantRuntimePermission(app.packageName, Manifest.permission.POST_NOTIFICATIONS)
        }
        TestDevice.seedOfflineWaiter()
        manager.cancelAll()
    }

    @After
    fun cleanUp() {
        controller.stopService()
        waitUntil("el servicio se detuvo") { !serviceRunning() }
        manager.cancelAll()
        TestDevice.reset()
    }

    @Test
    fun elServicioArrancaConSuNotificacionPermanenteYSeDetiene() {
        ContextCompat.startForegroundService(app, Intent(app, ReadyAlertsService::class.java))
        waitUntil("el servicio en primer plano") { serviceRunning() && foregroundService() }
        waitUntil("la notificación «AD-EL POS conectado»") { ongoing() != null }
        val ongoing = ongoing()!!
        assertEquals(ReadyChannels.CONNECTED, ongoing.notification.channelId)
        assertEquals("AD-EL POS conectado", ongoing.notification.extras.getString("android.title"))
        assertTrue(ongoing.isOngoing)
        assertTrue(controller.running.value)
        // RD5: sin el canal abierto no dice «Recibiendo avisos».
        assertNotEquals(
            "Recibiendo avisos de platillos listos",
            ongoing.notification.extras.getCharSequence("android.text")?.toString(),
        )
        // Sin servidor que responda, sigue en primer plano reintentando y lo dice cuando el estado
        // único de conexión (el de la barra) dice «Sin conexión»: tras la tolerancia de la etapa
        // A3 (3 fallas seguidas y 20 s, con la espera creciente del canal: unos 30 s).
        waitUntil("el texto de sin conexión", timeout = 60_000) {
            ongoing()?.notification?.extras?.getCharSequence("android.text")?.toString() ==
                "Sin conexión con la computadora principal. Reintentando…"
        }
        // Un segundo arranque no abre otro canal.
        ContextCompat.startForegroundService(app, Intent(app, ReadyAlertsService::class.java))
        Thread.sleep(SETTLE_MILLIS)
        assertTrue(serviceRunning())

        controller.stopService()
        waitUntil("el servicio se detuvo") { !serviceRunning() }
        waitUntil("sin notificación permanente") { ongoing() == null }
        waitUntil("el controlador sabe que se detuvo") { !controller.running.value }
    }

    @Test
    fun borrarLosDatosDelDispositivoDetieneElServicioYQuitaLosAvisos() = runBlocking {
        ContextCompat.startForegroundService(app, Intent(app, ReadyAlertsService::class.java))
        waitUntil("el servicio en primer plano") { serviceRunning() && ongoing() != null }
        controller.notifications.apply(ReadyAlertSettings.DEFAULT)
        controller.notifications.present(listOf(alert(1)))
        waitUntil("el aviso a la vista") { alertsOf("cuenta-4").isNotEmpty() }

        // «Emparejar de nuevo» (lo mismo que el retiro del dispositivo).
        app.container.deviceAccess.forgetServer()

        waitUntil("el servicio se detuvo") { !serviceRunning() }
        waitUntil("sin avisos ni notificación permanente") {
            alertsOf("cuenta-4").isEmpty() && ongoing() == null
        }
        assertNull(controller.ledger.status())
    }

    @Test
    fun unSoloCanalPlatillosListosSegunSonidoYVibracion() {
        val notifications = controller.notifications
        notifications.ensureConnectedChannel()
        for (settings in
            listOf(
                ReadyAlertSettings(true, true, true),
                ReadyAlertSettings(true, false, true),
                ReadyAlertSettings(true, true, false),
                ReadyAlertSettings(true, false, false),
            )) {
            notifications.ensureReadyChannel(settings)
            val ready = manager.notificationChannels.filter { it.id in ReadyChannels.READY }
            assertEquals("un solo canal visible con $settings", 1, ready.size)
            val channel = ready.single()
            assertEquals(ReadyChannels.readyChannel(settings), channel.id)
            assertEquals("Platillos listos", channel.name.toString())
            assertEquals(NotificationManager.IMPORTANCE_HIGH, channel.importance)
            assertEquals(settings.vibration, channel.shouldVibrate())
            assertEquals(settings.sound, channel.sound != null)
        }
        val connected = manager.getNotificationChannel(ReadyChannels.CONNECTED)
        assertEquals(NotificationManager.IMPORTANCE_LOW, connected.importance)
        assertEquals("AD-EL POS conectado", connected.name.toString())
    }

    @Test
    fun unaNotificacionPorCuentaConElAcumuladoYSinRepetir() = runBlocking {
        val notifications = ReadyAlertNotifications(app, NotViewing)
        notifications.apply(ReadyAlertSettings(true, false, false))
        notifications.present(listOf(alert(1)))
        waitUntil("el aviso de la Mesa 4") { alertsOf("cuenta-4").isNotEmpty() }
        val first = alertsOf("cuenta-4").single()
        assertEquals(
            "Mesa 4 · 2 platillos listos",
            first.notification.extras.getString("android.title"),
        )
        assertEquals(
            "2 × Filete · Grande",
            first.notification.extras.getCharSequence("android.text").toString(),
        )
        assertEquals(ReadyChannels.SILENT, first.notification.channelId)

        // Otro «Listo» de la misma cuenta: se reemplaza con el acumulado.
        notifications.present(
            listOf(
                alert(2, items = listOf(ReadyItem("Filete · Grande", 1), ReadyItem("Tacos", 2))),
            ),
        )
        waitUntil("el acumulado") {
            alertsOf("cuenta-4").singleOrNull()?.notification?.extras?.getString("android.title") ==
                "Mesa 4 · 5 platillos listos"
        }
        assertEquals(
            "3 × Filete · Grande, 2 × Tacos",
            alertsOf("cuenta-4")
                .single()
                .notification
                .extras
                .getCharSequence("android.text")
                .toString(),
        )
        val posted = alertsOf("cuenta-4").single().postTime

        // El mismo aviso otra vez (el proceso murió antes de guardarlo): no se vuelve a publicar.
        Thread.sleep(20)
        notifications.present(listOf(alert(2, items = listOf(ReadyItem("Filete · Grande", 1)))))
        Thread.sleep(SETTLE_MILLIS)
        assertEquals(posted, alertsOf("cuenta-4").single().postTime)

        // Otra cuenta (para llevar): su propia notificación y el resumen del grupo.
        notifications.present(
            listOf(
                alert(
                    3,
                    account = "cuenta-12",
                    mode = OrderMode.TAKEAWAY,
                    table = null,
                    folio = 12,
                    customer = "Juan",
                    items = listOf(ReadyItem("Hamburguesa", 1)),
                ),
            ),
        )
        waitUntil("el resumen") { summary() != null }
        assertEquals(
            "Para llevar #12 (Juan) · 1 platillo listo",
            alertsOf("cuenta-12").single().notification.extras.getString("android.title"),
        )
        assertEquals(
            ReadyAlertNotifications.GROUP,
            alertsOf("cuenta-12").single().notification.group,
        )
        // Sin precios en ningún texto.
        for (notification in manager.activeNotifications) {
            val extras = notification.notification.extras
            val text =
                "${extras.getString("android.title")} ${extras.getCharSequence("android.text")}"
            assertFalse(text, text.contains("$"))
        }
        notifications.cancelAlerts()
        waitUntil("sin avisos") { manager.activeNotifications.isEmpty() }
    }

    @Test
    fun elResumenNoEsperaAQueAndroidListeLoRecienPublicado() = runBlocking {
        // Etapa A3 (Fase E, `alerts r5` en Android 13): `notify` es asíncrono y, justo después de
        // publicar, la lista de Android muchas veces todavía no tiene la notificación. Aquí la
        // lista que lee la aplicación se queda atrasada a propósito hasta que la prueba la pone al
        // día; antes, con una sola cuenta en ella, se quitaba el resumen en lugar de publicarlo.
        var listed: List<StatusBarNotification> = emptyList()
        val notifications = ReadyAlertNotifications(app, NotViewing, listed = { listed })
        notifications.apply(ReadyAlertSettings(true, false, false))
        notifications.present(listOf(alert(1)))
        waitUntil("el aviso de la Mesa 4") { alertsOf("cuenta-4").isNotEmpty() }
        listed = manager.activeNotifications.toList()
        // Otra cuenta: la lista atrasada todavía no la tiene.
        notifications.present(
            listOf(alert(3, account = "cuenta-12", table = "Mesa 12", folio = 12)),
        )
        waitUntil("el resumen con las dos cuentas") { summary() != null }
        // Quitar una de las dos (el mesero abrió la Mesa 12): el aviso de la otra se queda. Quitar
        // el resumen del grupo haría que Android quitara también el de la Mesa 4.
        waitUntil("la Mesa 12 en la lista de Android") { alertsOf("cuenta-12").isNotEmpty() }
        listed = manager.activeNotifications.toList()
        notifications.cancelAccount("cuenta-12")
        waitUntil("sin la Mesa 12") { alertsOf("cuenta-12").isEmpty() }
        Thread.sleep(SETTLE_MILLIS)
        assertEquals("el aviso de la Mesa 4 se queda", 1, alertsOf("cuenta-4").size)
        // Sin ninguna cuenta a la vista, el resumen se quita (aunque la lista atrasada las tenga).
        listed = manager.activeNotifications.toList()
        notifications.cancelAccount("cuenta-4")
        waitUntil("sin resumen") { summary() == null }
        notifications.cancelAlerts()
        waitUntil("sin avisos") {
            manager.activeNotifications.none { it.id != ReadyAlertNotifications.ONGOING_ID }
        }
    }

    @Test
    fun siElMeseroYaVeLaCuentaElAvisoVaDentroDeLaAplicacion() = runBlocking {
        val shown = mutableListOf<ReadyNotice>()
        val viewing =
            object : InAppAlerts {
                override fun isViewing(accountId: String): Boolean = accountId == "cuenta-4"

                override fun show(notice: ReadyNotice) {
                    shown.add(notice)
                }
            }
        val notifications = ReadyAlertNotifications(app, viewing)
        notifications.apply(ReadyAlertSettings.DEFAULT)
        notifications.present(listOf(alert(1), alert(2, account = "cuenta-7", table = "Mesa 7")))
        waitUntil("el aviso de la Mesa 7") { alertsOf("cuenta-7").isNotEmpty() }
        assertEquals(listOf("cuenta-4"), shown.map { it.accountId })
        assertTrue("la cuenta a la vista no tiene notificación", alertsOf("cuenta-4").isEmpty())
        notifications.cancelAlerts()
    }

    @Test
    fun laAyudaDiceQueFaltaYQueSignificaLaNotificacionPermanente() {
        var closed = false
        compose.setContent {
            AdelTheme(darkTheme = false) {
                ReadyAlertsHelp(controller = controller, onClose = { closed = true })
            }
        }
        compose.onNodeWithTag(TAG_HELP).assertIsDisplayed()
        compose.onNodeWithText("Avisos de platillos listos").assertIsDisplayed()
        compose.waitUntil(timeoutMillis = 5_000) {
            compose.onAllNodes(hasTestTag(TAG_HELP_STATE)).fetchSemanticsNodes().isNotEmpty()
        }
        compose
            .onNodeWithText(
                "Los avisos no están activos ahora: se activan cuando un mesero entra con su PIN " +
                    "con conexión.",
            )
            .assertExists()
        compose.onNodeWithTag(TAG_HELP_NOTIFICATIONS).assertExists()
        compose.onNodeWithTag(TAG_HELP_BATTERY).assertExists()
        compose.onNodeWithText("«AD-EL POS conectado»").assertExists()
        compose
            .onNodeWithText(
                "«Forzar detención» en los Ajustes de Android o «Detener» en el Administrador " +
                    "de tareas paran los avisos hasta que se vuelve a abrir AD-EL POS.",
            )
            .assertExists()
        compose.onNodeWithTag(TAG_HELP_CLOSE).performClick()
        compose.waitUntil(timeoutMillis = 5_000) { closed }
    }

    private fun alert(
        id: Long,
        account: String = "cuenta-4",
        mode: OrderMode = OrderMode.DINE_IN,
        table: String? = "Mesa 4",
        folio: Long = 31,
        customer: String = "",
        items: List<ReadyItem> = listOf(ReadyItem("Filete · Grande", 2)),
    ) =
        ReadyAlert(
            id = id,
            key = "k-$account-$id",
            accountId = account,
            mode = mode,
            tableName = table,
            folio = folio,
            customerName = customer,
            waiterUserId = TestDevice.WAITER_ID,
            waiterShortName = "Mesero",
            items = items,
            readyAt = "2026-11-18T20:41:0$id.12Z",
        )

    private fun ongoing() =
        manager.activeNotifications.firstOrNull { it.id == ReadyAlertNotifications.ONGOING_ID }

    private fun summary() =
        manager.activeNotifications.firstOrNull { it.id == ReadyAlertNotifications.SUMMARY_ID }

    private fun alertsOf(account: String) =
        manager.activeNotifications.filter {
            it.id == ReadyAlertNotifications.ALERT_ID && it.tag == account
        }

    @Suppress("DEPRECATION")
    private fun runningService(): ActivityManager.RunningServiceInfo? =
        app.getSystemService(ActivityManager::class.java)
            .getRunningServices(Int.MAX_VALUE)
            .firstOrNull { it.service.className == ReadyAlertsService::class.java.name }

    private fun serviceRunning(): Boolean = runningService()?.started == true

    private fun foregroundService(): Boolean = runningService()?.foreground == true

    private fun waitUntil(what: String, timeout: Long = 15_000, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (!condition()) {
            check(System.currentTimeMillis() < deadline) { "No llegó: $what" }
            Thread.sleep(POLL_MILLIS)
        }
    }

    private object NotViewing : InAppAlerts {
        override fun isViewing(accountId: String): Boolean = false

        override fun show(notice: ReadyNotice) = Unit
    }

    private companion object {
        const val POLL_MILLIS = 100L
        const val SETTLE_MILLIS = 1_000L
    }
}
