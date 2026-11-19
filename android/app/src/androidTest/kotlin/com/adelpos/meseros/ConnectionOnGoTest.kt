package com.adelpos.meseros

import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.util.Log
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.data.ready.ReadyOngoing
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.ui.components.TAG_CONNECTION_STATE
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import com.adelpos.meseros.feature.access.findserver.TAG_ADDRESS
import com.adelpos.meseros.feature.access.findserver.TAG_FIND_SERVER
import com.adelpos.meseros.feature.access.findserver.TAG_PAIRING_CODE
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY
import com.adelpos.meseros.feature.access.pin.TAG_ENTRY_OFFLINE
import com.adelpos.meseros.feature.access.pin.TAG_WAITER_PREFIX
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_SEARCH
import com.adelpos.meseros.feature.waiter.tables.TAG_TABLES_STATUS
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa A3, §2 y §6 («una hora en el emulador sin uso sin avisos falsos, con la Mac sin carga»):
 * contra el **Go temporal** del arnés, nunca el servicio real. Empareja como [AccessOnGoTest],
 * entra un mesero y deja la tableta abierta y sin usar durante `minutes` minutos (10 por omisión;
 * la Fase E corre 60) con el **bloqueo por inactividad de fábrica**: Mesas, a los 2 minutos «¿Quién
 * eres?» y, 45 s después, el mesero vuelve a entrar, una y otra vez (el guion de la Fase 0, §0.3,
 * corrida C, que reprodujo la captura del propietario).
 *
 * Cada segundo registra (etiqueta `AdelConexion`) cada cambio de la barra, del aviso de «¿Quién
 * eres?», del de Mesas, del texto de la notificación del canal de avisos y del estado único de
 * conexión; al final exige **cero**: ningún segundo con «Sin conexión» a la vista, ninguna petición
 * sin respuesta y ninguna lectura repetida por una conexión muerta (lo cuenta el estado único,
 * `ServerLinkMonitor`), y cada vuelta del mesero con sesión de Go.
 *
 * Se corre con `scripts/android-e2e.sh --suite connection` (fuera de `all`, porque dura):
 * argumentos de siempre (`local`, `address`, `pin`, `mainDevice`, `mainToken`), `minutes` y el
 * aviso de platillos listos encendido (`readyAlerts=on`), como en la tableta del restaurante.
 */
@RunWith(AndroidJUnit4::class)
class ConnectionOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private var scenario: ActivityScenario<MainActivity>? = null
    private val container
        get() = TestDevice.app.container

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    @After
    fun close() {
        scenario?.close()
    }

    @Test
    fun c1_sinUsoNingunAvisoFalsoDeSinConexion() {
        val main = MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
        val minutes = arguments.getString("minutes")?.toLongOrNull() ?: DEFAULT_MINUTES
        val ana = main.ensureWaiters(mapOf(WAITER to (WAITER_NAME to PIN))).getValue(WAITER)
        grantPermissions(main)
        createTables(main)
        // La pantalla del emulador encendida todo el tiempo (el emulador es de solo lectura: no
        // queda guardado).
        shell("svc power stayon true")
        shell("settings put system screen_off_timeout 2147483647")
        TestDevice.reset()
        scenario = ActivityScenario.launch(MainActivity::class.java)
        pairByAddress(main)
        enter(ana)
        log("inicio: $minutes min sin uso, bloqueo por inactividad de fábrica")
        val result = watch(ana, minutes * 60_000)
        log(
            "resumen: ${result.offlineSeconds} de ${result.samples} s con «Sin conexión» a la " +
                "vista; sin respuesta ${result.noAnswers}, lecturas repetidas ${result.repeats}, " +
                "respuestas de Go ${result.answers}; bloqueos ${result.locks}, entradas " +
                "${result.entries} (sin sesión ${result.entriesWithoutSession})",
        )
        assertTrue("se esperaba al menos un bloqueo por inactividad", result.locks > 0)
        assertEquals("segundos con «Sin conexión» a la vista", 0, result.offlineSeconds)
        assertEquals("peticiones sin respuesta de Go", 0L, result.noAnswers)
        assertEquals("lecturas repetidas por una conexión muerta", 0L, result.repeats)
        assertEquals("entradas sin sesión de Go", 0, result.entriesWithoutSession)
    }

    private class Result(
        val samples: Int,
        val offlineSeconds: Int,
        val noAnswers: Long,
        val repeats: Long,
        val answers: Long,
        val locks: Int,
        val entries: Int,
        val entriesWithoutSession: Int,
    )

    // Cada segundo: lo que se ve y el estado único; el mesero vuelve a entrar 45 s después de cada
    // bloqueo por inactividad.
    private fun watch(waiter: String, millis: Long): Result {
        val link = container.serverLink
        val noAnswersBefore = link.noAnswers
        val repeatsBefore = link.repeats
        val answersBefore = link.answers
        val end = SystemClock.elapsedRealtime() + millis
        var last = ""
        var samples = 0
        var offlineSeconds = 0
        var locks = 0
        var entries = 0
        var withoutSession = 0
        var lockedAt = 0L
        while (SystemClock.elapsedRealtime() < end) {
            val bar = text(TAG_CONNECTION_STATE)
            val entryNotice = present(TAG_ENTRY_OFFLINE)
            val tablesNotice = text(TAG_TABLES_STATUS)?.takeIf { it.startsWith("Sin conexión") }
            val ongoing = container.readyAlerts.ongoing.value
            val state = link.state.value
            val offlineShown =
                bar == "Sin conexión" ||
                    entryNotice ||
                    tablesNotice != null ||
                    (container.readyAlerts.running.value && ongoing == ReadyOngoing.WAITING)
            samples += 1
            if (offlineShown || state == LinkState.OFFLINE) {
                offlineSeconds += 1
            }
            val now =
                "barra=$bar avisoEntrada=$entryNotice avisoMesas=$tablesNotice " +
                    "notificacion=$ongoing estado=$state sinRespuesta=${link.noAnswers} " +
                    "repetidas=${link.repeats}"
            if (now != last) {
                log(now)
                last = now
            }
            if (present(TAG_ENTRY)) {
                if (lockedAt == 0L) {
                    lockedAt = SystemClock.elapsedRealtime()
                    locks += 1
                    log("bloqueo por inactividad #$locks: «¿Quién eres?»")
                } else if (SystemClock.elapsedRealtime() - lockedAt > REENTER_AFTER_MILLIS) {
                    enter(waiter)
                    entries += 1
                    val session = container.deviceAccess.activeWaiter.value?.hasServerSession
                    log("el mesero vuelve a entrar (#$entries): sesión de Go=$session")
                    if (session != true) {
                        withoutSession += 1
                    }
                    lockedAt = 0L
                }
            }
            Thread.sleep(SAMPLE_MILLIS)
            // El reloj de Compose de la prueba no avanza solo: sin esto, los LaunchedEffect con
            // espera (la lista de «¿Quién eres?» cada 60 s) no correrían en tiempo real.
            compose.mainClock.advanceTimeBy(SAMPLE_MILLIS)
        }
        return Result(
            samples = samples,
            offlineSeconds = offlineSeconds,
            noAnswers = link.noAnswers - noAnswersBefore,
            repeats = link.repeats - repeatsBefore,
            answers = link.answers - answersBefore,
            locks = locks,
            entries = entries,
            entriesWithoutSession = withoutSession,
        )
    }

    private fun pairByAddress(main: MainComputer) {
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
        val stored = runBlocking { container.database.access().pairedServer() }
        assertEquals(argument("pin"), stored?.fingerprintHex)
    }

    private fun enter(waiter: String) {
        waitFor("$TAG_WAITER_PREFIX$waiter")
        compose.onNodeWithTag("$TAG_WAITER_PREFIX$waiter").performClick()
        for (digit in PIN) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        waitFor(TAG_TABLES_SEARCH)
    }

    // Lo que necesita un mesero en Mesas con su canal de eventos (events.read) y la precuenta.
    private fun grantPermissions(main: MainComputer) {
        val roles = main.call("GET", "roles").body.jsonArray.map { it.jsonObject }
        val role = roles.first { it.getValue("name").jsonPrimitive.content == ROLE }
        val permissions = PERMISSIONS.joinToString(",") { "\"$it\"" }
        val answer =
            main.call(
                "PATCH",
                "roles/${role.getValue("id").jsonPrimitive.content}",
                """{"name":"${ROLE}","permissions":[$permissions]}""",
            )
        assertEquals(answer.body.toString(), 200, answer.status)
    }

    private fun createTables(main: MainComputer) {
        val area =
            main.call(
                "POST",
                "restaurant/areas",
                """{"operation_id":"${UUID.randomUUID()}","name":"Salón A3","active":true}""",
            )
        assertEquals(area.body.toString(), 200, area.status)
        val areaId = area.obj.getValue("id").jsonPrimitive.content
        for (number in 1..6) {
            val x = 20 + ((number - 1) % 3) * 250
            val y = 40 + ((number - 1) / 3) * 200
            val table =
                main.call(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$areaId",""" +
                        """"name":"Mesa $number","shape":"rectangle","x":$x,""" +
                        """"y":$y,"width":100,"height":72,"seats":4,"active":true}""",
                )
            assertEquals(table.body.toString(), 200, table.status)
        }
    }

    private fun text(tag: String): String? =
        runCatching<String?> {
                val node =
                    compose
                        .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
                        .fetchSemanticsNodes()
                        .firstOrNull()
                if (node == null || !node.config.contains(SemanticsProperties.Text)) {
                    null
                } else {
                    node.config[SemanticsProperties.Text].joinToString("") { it.text }
                }
            }
            .getOrNull()

    private fun present(tag: String): Boolean = runCatching {
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()
    }
        .getOrDefault(false)

    private fun waitFor(tag: String, timeout: Long = 60_000) {
        try {
            compose.waitUntil(timeoutMillis = timeout) { present(tag) }
        } catch (failure: Throwable) {
            compose.onRoot(useUnmergedTree = true).printToLog(TAG)
            throw failure
        }
    }

    private fun shell(command: String): String {
        val output =
            InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        ParcelFileDescriptor.AutoCloseInputStream(output).use {
            return it.readBytes().toString(Charsets.UTF_8)
        }
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private companion object {
        const val TAG = "AdelConexion"

        /** El rol de los meseros de prueba que crea [MainComputer.ensureWaiters]. */
        const val ROLE = "Meseros A1"
        const val WAITER = "ana.a3"
        const val WAITER_NAME = "Ana"
        const val PIN = "4829"
        const val DEFAULT_MINUTES = 10L
        const val SAMPLE_MILLIS = 1_000L

        /** Como la corrida C de la Fase 0: el mesero vuelve 45 s después del bloqueo. */
        const val REENTER_AFTER_MILLIS = 45_000L

        val PERMISSIONS =
            listOf(
                "waiter_devices.use",
                "restaurant.tables.read",
                "restaurant.areas.read",
                "accounts.read",
                "accounts.open",
                "orders.capture",
                "orders.confirm",
                "catalog.read",
                "catalog.prices.read",
                "events.read",
                "takeaway.read",
                "takeaway.manage",
                "payments.tickets.print",
            )
    }
}
