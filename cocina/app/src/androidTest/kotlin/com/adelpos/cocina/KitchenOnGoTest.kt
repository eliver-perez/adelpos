package com.adelpos.cocina

import android.os.SystemClock
import android.util.Log
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.printToLog
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.cocina.board.BoardScreen
import com.adelpos.cocina.board.Dish
import com.adelpos.cocina.board.DisplayTheme
import com.adelpos.cocina.board.OrderCard
import com.adelpos.cocina.board.RecentState
import com.adelpos.cocina.board.TextSize
import com.adelpos.cocina.settings.SettingsScreen
import com.adelpos.cocina.ui.TAG_CARD_READY_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_PREFIX
import com.adelpos.cocina.ui.TAG_DISH_READY_PREFIX
import com.adelpos.cocina.ui.TAG_OFFLINE
import com.adelpos.cocina.ui.TAG_ORDERS
import com.adelpos.cocina.ui.TAG_ORDERS_EMPTY
import com.adelpos.cocina.ui.TAG_RECENT
import com.adelpos.cocina.ui.TAG_RECENT_CLOSE
import com.adelpos.cocina.ui.TAG_RECENT_OPEN
import com.adelpos.cocina.ui.TAG_RECENT_RECOVER_PREFIX
import com.adelpos.cocina.ui.TAG_SETTINGS
import com.adelpos.cocina.ui.TAG_SETTINGS_CLOSE
import com.adelpos.cocina.ui.TAG_SETTINGS_CONNECTION
import com.adelpos.cocina.ui.TAG_SETTINGS_PASSWORD
import com.adelpos.cocina.ui.TAG_SETTINGS_SERVER
import com.adelpos.cocina.ui.TAG_SETTINGS_SUBMIT
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR_CONFIRM
import com.adelpos.cocina.ui.TAG_SETTINGS_UNPAIR_QUESTION
import com.adelpos.cocina.ui.TAG_SETTINGS_USERNAME
import com.adelpos.cocina.ui.TAG_SETTINGS_VERSION
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.testing.MainComputer
import com.adelpos.core.ui.access.findserver.TAG_ADDRESS
import com.adelpos.core.ui.access.findserver.TAG_FIND_SERVER
import com.adelpos.core.ui.access.findserver.TAG_PAIRING_CODE
import com.adelpos.core.ui.components.TAG_TOP_BAR_LOGO
import java.io.File
import java.util.UUID
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La aplicación de cocina contra el Go temporal del arnés (etapa K1, suite `kitchen` de
 * `scripts/android-e2e.sh`, un paso por instrumentación con la aplicación instalada una sola vez;
 * la prueba hace de computadora principal por la escucha local):
 * - `k1`: arma el restaurante (estación «Cocina K» por platillo y «Barra K» por pedido, las dos en
 *   pantalla; productos, uno con un ingrediente que se puede quitar; una mesa y un mesero) y
 *   empareja la tableta como **pantalla de cocina** escribiendo la dirección.
 * - `k2`: con un proceso nuevo, se conecta sola (`kitchen-display/connect`) y no hay pedidos.
 * - `k3`: pedidos reales creados en Go (mesa con un modificador y una nota, para llevar con su
 *   cliente, un agregado después del envío y una cancelación): las tarjetas aparecen sin tocar
 *   nada, con «NUEVO» y «CANCELADO».
 * - `k4`: marcar un platillo, el cierre automático al marcar lo último y el botón del pedido; Go lo
 *   registra (el guion revisa después el aviso al mesero en `ready_notices`).
 * - `k5`: «Recientes» y «Recuperar»: el platillo vuelve a la pantalla y en Go deja de estar listo.
 * - `k6`: el tamaño de letra y el tema cambiados con `PATCH devices/{id}` llegan al momento.
 *
 * Los identificadores que crea `k1` y las cuentas de `k3` quedan en `files/kitchen-on-go.json` de
 * la aplicación para los pasos siguientes. Argumentos del guion: `local`, `address`, `mainDevice` y
 * `mainToken`; sin ellos, se omite. Nunca contra el servicio real de la Mac. Capturas en
 * `files/screens` (el guion las copia).
 */
@RunWith(AndroidJUnit4::class)
class KitchenOnGoTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val arguments = InstrumentationRegistry.getArguments()
    private var scenario: ActivityScenario<MainActivity>? = null
    private val application
        get() = ApplicationProvider.getApplicationContext<CocinaApplication>()

    private val container
        get() = application.container

    private fun argument(name: String): String {
        val value = arguments.getString(name)
        assumeTrue("Sin el argumento $name (Go temporal)", !value.isNullOrBlank())
        return value!!
    }

    private val main by lazy {
        MainComputer(argument("local"), argument("mainDevice"), argument("mainToken"))
    }

    @After
    fun close() {
        scenario?.close()
    }

    @Test
    fun k1_emparejarComoPantallaDeCocina() {
        argument("address")
        // En la prueba de punta a punta (suite `endtoend`) el restaurante ya lo armó la aplicación
        // de
        // meseros: aquí solo se empareja (un salón más cambiaría las mesas que ve el mesero).
        if (arguments.getString("restaurant") != "existing") {
            saveIds(createRestaurant())
        }
        launch()
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
        screenshot("k1-codigo")
        val approved = main.approvePairing(shown.replace(" ", ""))
        assertEquals(approved.body.toString(), 200, approved.status)
        waitFor(TAG_ORDERS)
        // Go la registró como pantalla de cocina (K1-D3).
        val kitchen = kitchenDevices()
        assertEquals("Una pantalla de cocina emparejada: $kitchen", 1, kitchen.size)
        log("k1 emparejada como pantalla de cocina: ${kitchen.single()}")
    }

    @Test
    fun k2_conectarYPantallaDePedidosVacia() {
        argument("address")
        launch()
        // Emparejada: abre directo en la pantalla de pedidos, se conecta sola y no hay pedidos.
        waitFor(TAG_ORDERS)
        waitFor(TAG_ORDERS_EMPTY)
        screenshot("k2-pedidos-vacia")
        log("k2 conectada: «No hay pedidos»")
    }

    @Test
    fun k3_pedidosRealesAparecenSolos() {
        val ids = loadIds()
        launch()
        waitFor(TAG_ORDERS_EMPTY)
        // Sin tocar la tableta: la principal manda un pedido de mesa (tacos sin cebolla y con nota,
        // agua y limonadas de la barra) y uno para llevar.
        val onion = """[{"ingredient_id":"${ids.getValue("cebolla")}","quantity":"0"}]"""
        val table =
            openAccount(
                destination =
                    """"mode":"dine_in","table_id":"${ids.getValue("table")}",""" +
                        """"seen_account_ids":[]""",
                waiter = ids.getValue("waiter"),
                items =
                    listOf(
                        item(ids.getValue("tacos"), 3, notes = "Bien dorados", ingredients = onion),
                        item(ids.getValue("horchata"), 1),
                        item(ids.getValue("limonada"), 2),
                    ),
            )
        val takeaway =
            openAccount(
                destination =
                    """"mode":"takeaway","table_id":null,"origin":"Para llevar",""" +
                        """"customer_name":"$CUSTOMER"""",
                waiter = ids.getValue("waiter"),
                items = listOf(item(ids.getValue("hamburguesa"), 2)),
            )
        saveIds(ids + mapOf("mesa" to table, "llevar" to takeaway))
        waitForText("3 × $TACOS")
        waitForText("SIN Cebolla K")
        waitForText("Nota: Bien dorados")
        waitForText("2 × $LIMONADA")
        // La mesa mezcla estaciones: la limonada (barra, por pedido) dice que se marca con el
        // botón.
        waitForText("Se marca con el botón «Pedido listo».")
        waitForText(CUSTOMER)
        waitForText("2 × $HAMBURGUESA")
        // Lo agregado después del envío llega como «NUEVO»; lo cancelado se queda tachado.
        addAndConfirm(table, listOf(item(ids.getValue("flan"), 1)))
        waitForText("1 × $FLAN")
        waitUntil("el flan con «NUEVO»") { dish(table, FLAN).isNew }
        cancelOne(table, HORCHATA)
        waitForText("CANCELADO")
        screenshot("k3-pedidos-reales")
        log("k3 tarjetas: ${cards().map { "${it.title} ${it.subtitle}".trim() }}")
    }

    @Test
    fun k4_marcarPlatilloCierreAutomaticoYPedido() {
        val ids = loadIds()
        val table = ids.getValue("mesa")
        val takeaway = ids.getValue("llevar")
        log("k4 mesa cuenta=$table")
        log("k4 llevar cuenta=$takeaway")
        launch()
        waitForText("3 × $TACOS")
        // Tocar un platillo lo marca: Go lo registra.
        val tacos = dish(table, TACOS).id
        tap(TAG_DISH_PREFIX + tacos)
        waitUntil("los tacos listos en Go") { taskStates(table, TACOS).all { it == READY } }
        // La confirmación visual: la palomita del platillo, también después de volver a leer.
        waitFor(TAG_DISH_READY_PREFIX + tacos)
        log("k4 tacos marcados: ${dish(table, TACOS).state}")
        screenshot("k4-platillo-marcado")
        // Cierre automático: lo único de la tarjeta para llevar se marca y la tarjeta se va sola.
        tap(TAG_DISH_PREFIX + dish(takeaway, HAMBURGUESA).id)
        waitUntil("la tarjeta para llevar fuera de la pantalla") { card(takeaway) == null }
        assertTrue(taskStates(takeaway, HAMBURGUESA).all { it == READY })
        // El botón grande marca todo lo que queda de la mesa (el flan y las limonadas de la barra,
        // que se marcan por pedido) y la tarjeta desaparece.
        tap(TAG_CARD_READY_PREFIX + requireNotNull(card(table)).id)
        waitUntil("la mesa fuera de la pantalla") { card(table) == null }
        waitUntil("el flan y las limonadas listos en Go") {
            (taskStates(table, FLAN) + taskStates(table, LIMONADA)).all { it == READY }
        }
        waitFor(TAG_ORDERS_EMPTY)
        screenshot("k4-todo-listo")
        log("k4 listo: mesa y para llevar")
    }

    @Test
    fun k5_recientesYRecuperar() {
        val ids = loadIds()
        val takeaway = ids.getValue("llevar")
        log("k5 llevar cuenta=$takeaway")
        launch()
        waitFor(TAG_ORDERS)
        tap(TAG_RECENT_OPEN)
        waitFor(TAG_RECENT)
        waitUntil("«Recientes» con el pedido para llevar") {
            recentDish(takeaway, HAMBURGUESA) != null
        }
        // En «Recientes» el título y el cliente van en una línea.
        waitForText("Para llevar · $CUSTOMER")
        screenshot("k5-recientes")
        val recovered = requireNotNull(recentDish(takeaway, HAMBURGUESA))
        tap(TAG_RECENT_RECOVER_PREFIX + recovered.id)
        waitUntil("la hamburguesa de regreso en Go") {
            taskStates(takeaway, HAMBURGUESA).none { it == READY }
        }
        tap(TAG_RECENT_CLOSE)
        // La tarjeta vuelve a la pantalla.
        waitUntil("la tarjeta para llevar de regreso") { card(takeaway) != null }
        waitForText("2 × $HAMBURGUESA")
        screenshot("k5-recuperado")
        log("k5 recuperada la hamburguesa")
    }

    @Test
    fun k6_letraYTemaDesdeTerminalesAlMomento() {
        loadIds()
        launch()
        waitForText("2 × $HAMBURGUESA")
        val normalHeight = textHeight("2 × $HAMBURGUESA")
        val device = kitchenDevices().single()
        val id = device.text("id")
        val changed =
            main.call(
                "PATCH",
                "devices/$id",
                """{"name":"${device.text("name")}","keyboard_mode":"${device.text("keyboard_mode")}",""" +
                    """"kitchen_display_text_size":"large","kitchen_display_theme":"light"}""",
            )
        assertEquals(changed.body.toString(), 200, changed.status)
        waitUntil("letra grande y tema claro en la tableta") {
            val settings = application.container.board.state.value.settings
            settings.textSize == TextSize.LARGE && settings.theme == DisplayTheme.LIGHT
        }
        // Y se ve: el platillo con letra más grande y el fondo de la pantalla claro.
        waitUntil("la letra grande en la pantalla") {
            textHeight("2 × $HAMBURGUESA") > normalHeight * LARGE_TEXT_MIN_RATIO
        }
        waitUntil("el tema claro en la pantalla") { backgroundIsLight() }
        screenshot("k6-letra-grande-tema-claro")
        log(
            "k6 letra grande y tema claro al momento: alto del platillo $normalHeight → " +
                "${textHeight("2 × $HAMBURGUESA")}",
        )
    }

    // --- Fase D: conexión tolerante, ajustes y «Volver a emparejar» (§5) ---

    @Test
    fun k7_cortesCortosDelGoSinNingunAviso() {
        loadIds()
        launch()
        waitFor(TAG_ORDERS)
        waitUntil("la pantalla de pedidos conectada") {
            container.board.state.value.screen == BoardScreen.Orders
        }
        // El guion pausa el Go temporal 1, 2, 3, 4 y 5 s (`kill -STOP`), con 5 s entre cortes.
        log("k7 listo para los cortes")
        val watch = watchOffline(File(K7_DONE_FILE), timeout = 120_000)
        assertTrue("el guion terminó los cortes", watch.finished)
        assertEquals("ningún aviso en los cortes cortos", emptyList<Long>(), watch.offlineAt)
        log(
            "k7 sin avisos; respuestas de Go ${container.serverLink.answers}, sin respuesta " +
                "${container.serverLink.noAnswers}",
        )
    }

    @Test
    fun k8_unaMarcaDuranteUnCorteSeRepiteYGoLaRegistraUnaVez() {
        val ids = loadIds()
        val takeaway = ids.getValue("llevar")
        launch()
        waitForText("2 × $HAMBURGUESA")
        val dish = dish(takeaway, HAMBURGUESA)
        val tasks = dish.tasks.map { it.id }
        val device = kitchenDevices().single().text("id")
        // El guion pausa el Go 18 s: la marca se queda sin respuesta (el cliente espera 15 s) y se
        // repite sola; todo dentro de la tolerancia, sin aviso.
        log("k8 listo para el corte")
        waitUntil("el Go pausado", timeout = 60_000) { File(K8_PAUSED_FILE).exists() }
        tap(TAG_DISH_PREFIX + dish.id)
        waitFor(TAG_DISH_READY_PREFIX + dish.id)
        screenshot("k8-marcado-durante-el-corte")
        val watch = watchOffline(File(K8_DONE_FILE), timeout = 90_000)
        assertTrue("el guion terminó el corte", watch.finished)
        assertEquals("sin aviso durante el corte", emptyList<Long>(), watch.offlineAt)
        waitUntil("la hamburguesa lista en Go") {
            taskStates(takeaway, HAMBURGUESA).all { it == READY }
        }
        waitUntil("la tarjeta se cerró sola") { card(takeaway) == null }
        // Una sola marca en el historial de cocina de Go para esas tareas desde que se recuperaron
        // en k5 (el historial va del más nuevo al más viejo; la de k4 es anterior).
        val history =
            main
                .call("GET", "kitchen/history")
                .body
                .jsonArray
                .map { it.jsonObject }
                .filter { entry ->
                    tasks.any { task -> entry["before_value"].toString().contains(task) }
                }
                .takeWhile { it.text("kind") != "states.recovered" }
        val marks = history.filter {
            it.text("kind") == "states.changed" &&
                (it["device_id"] as? JsonPrimitive)?.content == device
        }
        assertEquals("Go la registró una sola vez: $marks", 1, marks.size)
        log("k8 una sola marca en Go; sin respuesta ${container.serverLink.noAnswers}")
    }

    @Test
    fun k9_corteLargoAvisaNoDejaMarcarYSeRecuperaSolo() {
        val ids = loadIds()
        val order =
            openAccount(
                destination =
                    """"mode":"takeaway","table_id":null,"origin":"Para llevar",""" +
                        """"customer_name":"Pedro Ruiz"""",
                waiter = ids.getValue("waiter"),
                items = listOf(item(ids.getValue("flan"), 1)),
            )
        saveIds(ids + ("largo" to order))
        launch()
        waitUntil("la tarjeta de Pedro") { card(order) != null }
        log("k9 listo para el corte")
        // El guion pausa el Go 45 s: el aviso grande aparece (tres fallas en más de 20 s).
        waitFor(TAG_OFFLINE, timeout = 60_000)
        val offlineAt = System.currentTimeMillis()
        assertFalse(container.board.state.value.markingAllowed)
        compose
            .onNodeWithTag(TAG_CARD_READY_PREFIX + requireNotNull(card(order)).id)
            .assertIsNotEnabled()
        compose.onNodeWithTag(TAG_DISH_PREFIX + dish(order, FLAN).id).performClick()
        compose.waitForIdle()
        assertTrue("no se marca nada", container.board.state.value.markingDishes.isEmpty())
        screenshot("k9-sin-conexion")
        // Se recupera sola, sin tocar la tableta.
        waitUntil("el aviso se quita solo", timeout = 90_000) {
            !container.board.state.value.offline && !present(TAG_OFFLINE)
        }
        val recoveredAt = System.currentTimeMillis()
        assertTrue(File(K9_DONE_FILE).exists())
        waitUntil("los pedidos al día") { card(order) != null }
        assertTrue("Go no marcó nada", taskStates(order, FLAN).none { it == READY })
        log("k9 aviso durante ${(recoveredAt - offlineAt) / 1000} s y recuperada sola")
    }

    /**
     * Cortes de red del emulador (modo avión en Android 11+; wifi y datos en Android 8) de 1, 3 y 5
     * s y uno de 35 s, con el margen de 10 s sin red de la cocina (K1-D33). Mide cuánto dura la
     * falta de red **para Android** en cada corte (después de quitar el modo avión la wifi tarda en
     * volver) y comprueba que el aviso sigue la regla: nada mientras Android vuelve a tener red
     * antes del margen; el aviso cuando la falta de red pasa del margen, y se quita solo.
     */
    @Test
    fun k10_cortesDeRedCortosSinAvisoYUnoLargoConAviso() {
        loadIds()
        launch()
        waitFor(TAG_ORDERS)
        val episodes = NetworkEpisodes(application)
        try {
            log("k10 listo para los cortes")
            val watch = watchOffline(File(K10_DONE_FILE), timeout = 300_000)
            assertTrue("el guion terminó los cortes", watch.finished)
            val starts = episodes.starts()
            val lost = episodes.durations()
            log(
                "k10 sin red para Android (ms): $lost; aviso a los ${watch.offlineAt}; cortes ${starts}",
            )
            assertEquals("tres cortes cortos y uno largo: $lost", 4, lost.size)
            for (index in 0 until 4) {
                val from = starts[index]
                val until = from + lost[index] + SETTLE_AFTER_NETWORK_MILLIS
                val warned = watch.offlineAt.any { it in from..until }
                when {
                    lost[index] < NETWORK_GRACE_MILLIS - GRACE_SLACK_MILLIS ->
                        assertFalse(
                            "corte ${index + 1}: ${lost[index]} ms sin red, sin aviso",
                            warned,
                        )
                    lost[index] > NETWORK_GRACE_MILLIS + GRACE_SLACK_MILLIS ->
                        assertTrue(
                            "corte ${index + 1}: ${lost[index]} ms sin red, con aviso",
                            warned,
                        )
                }
            }
            assertTrue("el largo pasa del margen", lost[3] > NETWORK_GRACE_MILLIS)
            waitUntil("sin aviso al final", timeout = 60_000) {
                !container.board.state.value.offline
            }
        } finally {
            episodes.close()
        }
    }

    @Test
    fun k11_ajustesConLaContrasenaDelAdministrador() {
        loadIds()
        launch()
        waitFor(TAG_ORDERS)
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO).performTouchInput { longClick() }
        waitFor(TAG_SETTINGS)
        // Una contraseña equivocada: el mensaje de Go en palabras claras.
        compose.onNodeWithTag(TAG_SETTINGS_USERNAME).performTextReplacement(ADMIN)
        compose.onNodeWithTag(TAG_SETTINGS_PASSWORD).performTextReplacement("equivocada")
        compose.onNodeWithTag(TAG_SETTINGS_SUBMIT).performClick()
        waitForText("El usuario o la contraseña no son correctos.")
        assertFalse(container.kitchenSettings.holdsPassword)
        screenshot("k11-contrasena-equivocada")
        compose.onNodeWithTag(TAG_SETTINGS_PASSWORD).performTextReplacement(ADMIN_PASSWORD)
        compose.onNodeWithTag(TAG_SETTINGS_SUBMIT).performClick()
        waitFor(TAG_SETTINGS_SERVER)
        compose.onNodeWithTag(TAG_SETTINGS_SERVER).assertTextEquals(argument("address"))
        compose.onNodeWithTag(TAG_SETTINGS_CONNECTION).assertTextEquals("En línea")
        compose.onNodeWithTag(TAG_SETTINGS_VERSION).assertTextEquals(BuildConfig.VERSION_NAME)
        assertTrue("la contraseña, solo en memoria", container.kitchenSettings.holdsPassword)
        screenshot("k11-ajustes")
        compose.onNodeWithTag(TAG_SETTINGS_CLOSE).performClick()
        waitUntil("cerrados") { container.kitchenSettings.screen.value == SettingsScreen.Closed }
        assertFalse("al cerrar se borra", container.kitchenSettings.holdsPassword)
        log("k11 ajustes abiertos con la contraseña del administrador")
    }

    /**
     * La prueba de punta a punta (§8, suite `endtoend`): mientras la aplicación de meseros espera
     * el aviso, la pantalla de cocina marca listo el pedido que Ana acaba de enviar (`account`, lo
     * pasa el guion). Sin capturas: la otra instrumentación tiene la automatización de la pantalla.
     */
    @Test
    fun k12_marcaListoElPedidoDelMesero() {
        val account = argument("account")
        launch()
        waitFor(TAG_ORDERS)
        waitUntil("el pedido de Ana en la pantalla de cocina", timeout = 90_000) {
            card(account) != null
        }
        val sent = requireNotNull(card(account))
        log("k12 tarjeta ${sent.title}: ${sent.dishes.map { "${it.quantity} × ${it.name}" }}")
        tap(TAG_CARD_READY_PREFIX + sent.id)
        waitUntil("la tarjeta se va") { card(account) == null }
        log("k12 marcado listo cuenta=$account")
    }

    @Test
    fun k13_volverAEmparejarSueltaElCupo() {
        loadIds()
        launch()
        waitFor(TAG_ORDERS)
        val device = kitchenDevices().single().text("id")
        assertEquals("la pantalla ocupa su cupo", 1, kitchenSeatsInUse())
        compose.onNodeWithTag(TAG_TOP_BAR_LOGO).performTouchInput { longClick() }
        waitFor(TAG_SETTINGS)
        compose.onNodeWithTag(TAG_SETTINGS_USERNAME).performTextReplacement(ADMIN)
        compose.onNodeWithTag(TAG_SETTINGS_PASSWORD).performTextReplacement(ADMIN_PASSWORD)
        compose.onNodeWithTag(TAG_SETTINGS_SUBMIT).performClick()
        waitFor(TAG_SETTINGS_UNPAIR)
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIR).performClick()
        waitFor(TAG_SETTINGS_UNPAIR_QUESTION)
        screenshot("k13-volver-a-emparejar")
        compose.onNodeWithTag(TAG_SETTINGS_UNPAIR_CONFIRM).performClick()
        // Vuelve a «Buscar el servidor», sin nada de lo anterior.
        waitFor(TAG_FIND_SERVER)
        assertFalse(container.kitchenSettings.holdsPassword)
        val revoked =
            main
                .call("GET", "devices")
                .body
                .jsonArray
                .map { it.jsonObject }
                .single { it.text("id") == device }
        assertTrue("revocada en Go: $revoked", revoked["revoked_at"] !is JsonNull)
        assertEquals("el cupo quedó libre", 0, kitchenSeatsInUse())
        screenshot("k13-buscar-el-servidor")
        log("k13 revocada ${revoked.text("revoked_at")} y cupo libre")
    }

    // --- La computadora principal (escucha local) ---

    private fun ok(method: String, path: String, body: String? = null): JsonObject {
        val answer = main.call(method, path, body)
        assertEquals("$method $path: ${answer.body}", 200, answer.status)
        return answer.body as? JsonObject ?: JsonObject(emptyMap())
    }

    private fun JsonObject.text(name: String): String = getValue(name).jsonPrimitive.content

    private fun createRestaurant(): Map<String, String> {
        val waiter = main.ensureWaiters(mapOf(WAITER to ("Kari" to "4829"))).getValue(WAITER)
        fun station(name: String, tracking: String): String =
            ok(
                    "POST",
                    "kitchen/stations",
                    """{"operation_id":"${UUID.randomUUID()}","name":"$name","active":true,""" +
                        """"reception":"kds","target_minutes":12,"warning_minutes":15,""" +
                        """"overdue_minutes":25,"grouping":"inherit","tracking":"$tracking"}""",
                )
                .text("id")
        val kitchen = station("Cocina K", "item")
        val bar = station("Barra K", "order")
        fun category(name: String): String =
            ok("POST", "catalog/categories", """{"name":"$name","active":true,"color":"#d64545"}""")
                .text("id")
        val food = category("Comida K")
        val drinks = category("Bebidas K")
        val onion =
            ok(
                    "POST",
                    "catalog/ingredients",
                    """{"name":"Cebolla K","active":true,"unit":"porción"}""",
                )
                .text("id")
        fun product(category: String, name: String, code: String, ingredients: String = "[]") =
            ok(
                    "POST",
                    "catalog/products",
                    """{"name":"$name","active":true,"category_id":"$category",""" +
                        """"code":"$code","kind":"product","base_price":"50",""" +
                        """"ingredients":$ingredients}""",
                )
                .text("id")
        val tacosIngredients =
            """[{"ingredient_id":"$onion","included_quantity":"1","removable":true,""" +
                """"addable":true,"max_quantity":"3","extra_price":"2.50",""" +
                """"recipe_quantity":"0.05"}]"""
        val ids =
            mutableMapOf(
                "waiter" to waiter,
                "cebolla" to onion,
                "tacos" to product(food, TACOS, "K-TAC", tacosIngredients),
                "horchata" to product(food, HORCHATA, "K-HOR"),
                "hamburguesa" to product(food, HAMBURGUESA, "K-HAM"),
                "flan" to product(food, FLAN, "K-FLA"),
                "limonada" to product(drinks, LIMONADA, "K-LIM"),
            )
        fun route(category: String, station: String) =
            ok(
                "POST",
                "kitchen/routes",
                """{"operation_id":"${UUID.randomUUID()}","category_id":"$category",""" +
                    """"mode":"prepare","station_ids":["$station"]}""",
            )
        route(food, kitchen)
        route(drinks, bar)
        val area =
            ok(
                    "POST",
                    "restaurant/areas",
                    """{"operation_id":"${UUID.randomUUID()}","name":"Salón K","active":true}""",
                )
                .text("id")
        ids["table"] =
            ok(
                    "POST",
                    "restaurant/tables",
                    """{"operation_id":"${UUID.randomUUID()}","area_id":"$area","name":"Mesa K1",""" +
                        """"shape":"rectangle","x":20,"y":20,"width":100,"height":72,"seats":4,""" +
                        """"active":true}""",
                )
                .text("id")
        return ids
    }

    private fun item(
        productId: String,
        quantity: Int,
        notes: String = "",
        ingredients: String = "[]",
    ): String =
        """{"id":"${UUID.randomUUID()}","product_id":"$productId","variant_id":null,""" +
            """"quantity":$quantity,"customization":{"ingredients":$ingredients,"groups":[],""" +
            """"components":[],"notes":"$notes"}}"""

    /** Abre la cuenta a nombre del mesero (en [destination]) y la envía a cocina. */
    private fun openAccount(destination: String, waiter: String, items: List<String>): String {
        val opened =
            ok(
                "POST",
                "restaurant/accounts",
                """{"operation_id":"${UUID.randomUUID()}",$destination,""" +
                    """"assigned_user_id":"$waiter","items":[${items.joinToString(",")}]}""",
            )
        val account = opened.getValue("account").jsonObject.text("id")
        confirm(account)
        return account
    }

    private fun bill(accountId: String): JsonObject = ok("GET", "restaurant/accounts/$accountId")

    private fun revision(accountId: String): Int =
        bill(accountId).getValue("account").jsonObject.getValue("revision").jsonPrimitive.int

    private fun confirm(accountId: String) {
        val preview = ok("GET", "restaurant/accounts/$accountId/preview")
        ok(
            "POST",
            "restaurant/accounts/$accountId/confirm",
            """{"operation_id":"${UUID.randomUUID()}","revision":${preview.getValue("revision")},""" +
                """"expected_fingerprint":"${preview.text("fingerprint")}"}""",
        )
    }

    private fun addAndConfirm(accountId: String, items: List<String>) {
        ok(
            "PUT",
            "restaurant/accounts/$accountId/draft",
            """{"operation_id":"${UUID.randomUUID()}","revision":${revision(accountId)},""" +
                """"items":[${items.joinToString(",")}]}""",
        )
        confirm(accountId)
    }

    /**
     * Cancela una unidad de lo enviado de [productName] (la versión de su partida en la cuenta).
     */
    private fun cancelOne(accountId: String, productName: String) {
        val current = bill(accountId)
        val versions = kitchenTasks(accountId, productName).map { it.text("version_id") }.toSet()
        val line =
            current
                .getValue("lines")
                .jsonArray
                .map { it.jsonObject }
                .first { it.text("version_id") in versions }
        val revision = current.getValue("account").jsonObject.getValue("revision").jsonPrimitive.int
        ok(
            "POST",
            "restaurant/accounts/$accountId/cancel",
            """{"operation_id":"${UUID.randomUUID()}","revision":$revision,""" +
                """"version_id":"${line.text("version_id")}","quantity":1,""" +
                """"reason":"El cliente cambió de opinión"}""",
        )
    }

    /** Las tareas de cocina de [productName] en la cuenta, como las ve Cocina en el escritorio. */
    private fun kitchenTasks(accountId: String, productName: String): List<JsonObject> =
        ok("GET", "kitchen/board?state=all&account_id=$accountId")
            .getValue("tasks")
            .jsonArray
            .map { it.jsonObject }
            .filter { it.getValue("instructions").jsonObject.text("name") == productName }

    private fun taskStates(accountId: String, productName: String): List<String> =
        kitchenTasks(accountId, productName)
            .map { it.text("state") }
            .also { assertTrue("Go no tiene tareas de $productName", it.isNotEmpty()) }

    private fun kitchenDevices(): List<JsonObject> =
        main
            .call("GET", "devices")
            .body
            .jsonArray
            .map { it.jsonObject }
            .filter { it.text("kind") == "kitchen" }

    /** Las pantallas de cocina que ocupan su cupo, según `devices/sessions-summary`. */
    private fun kitchenSeatsInUse(): Int =
        ok("GET", "devices/sessions-summary")
            .getValue("seats")
            .jsonObject
            .getValue("kitchen_displays")
            .jsonObject
            .getValue("in_use")
            .jsonPrimitive
            .int

    // --- La tableta ---

    private fun cards(): List<OrderCard> =
        application.container.board.state.value.board?.cards.orEmpty()

    private fun card(accountId: String): OrderCard? =
        cards().firstOrNull { it.accountId == accountId }

    private fun dish(accountId: String, name: String): Dish =
        requireNotNull(card(accountId)) { "Sin la tarjeta de $accountId" }
            .dishes
            .first { it.name == name }

    private fun recentDish(accountId: String, name: String): Dish? =
        (application.container.board.recent.value as? RecentState.Shown)
            ?.orders
            ?.firstOrNull { it.accountId == accountId }
            ?.dishes
            ?.firstOrNull { it.name == name }

    private fun textHeight(text: String): Int =
        compose
            .onAllNodes(hasText(text), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .first()
            .size
            .height

    /** Si el fondo de la pantalla de pedidos se ve claro (a la derecha, donde no hay tarjetas). */
    private fun backgroundIsLight(): Boolean {
        val shot =
            InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()
                ?: return false
        val pixel = shot.getPixel(shot.width - 24, shot.height / 2)
        val luminance =
            0.299 * android.graphics.Color.red(pixel) +
                0.587 * android.graphics.Color.green(pixel) +
                0.114 * android.graphics.Color.blue(pixel)
        log("k6 fondo en la orilla derecha: luminancia ${luminance.toInt()}")
        return luminance > LIGHT_LUMINANCE
    }

    /**
     * Toca el elemento [tag] cuando ya se puede tocar: aparece en el árbol fusionado (recién
     * abierta la pantalla, la barra o una tarjeta pueden estar todavía en el árbol sin fusionar).
     */
    private fun tap(tag: String) {
        waitForNode(TIMEOUT_MILLIS) { compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes() }
        compose.onNodeWithTag(tag).performClick()
    }

    /** Lo que se vio de «Sin conexión» mientras el guion hacía sus cortes. */
    private class OfflineWatch(val finished: Boolean, val offlineAt: List<Long>)

    /**
     * Mira cada 100 ms, hasta que el guion crea [done], si la pantalla muestra «Sin conexión» (en
     * el estado o en la pantalla) y anota cuándo (reloj monotónico), una vez por aparición.
     */
    private fun watchOffline(done: File, timeout: Long): OfflineWatch {
        val deadline = System.currentTimeMillis() + timeout
        val seen = mutableListOf<Long>()
        var showing = false
        while (!done.exists() && System.currentTimeMillis() < deadline) {
            val offline =
                container.board.state.value.offline ||
                    container.serverLink.state.value == LinkState.OFFLINE ||
                    present(TAG_OFFLINE)
            if (offline && !showing) {
                seen += SystemClock.elapsedRealtime()
            }
            showing = offline
            Thread.sleep(WATCH_MILLIS)
        }
        return OfflineWatch(done.exists(), seen)
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun launch() {
        scenario = ActivityScenario.launch(MainActivity::class.java)
    }

    private fun waitFor(tag: String, timeout: Long = TIMEOUT_MILLIS) {
        waitForNode(timeout) {
            compose.onAllNodes(hasTestTag(tag), useUnmergedTree = true).fetchSemanticsNodes()
        }
    }

    private fun waitForText(text: String, timeout: Long = TIMEOUT_MILLIS) {
        waitForNode(timeout) {
            compose.onAllNodes(hasText(text), useUnmergedTree = true).fetchSemanticsNodes()
        }
    }

    private fun waitForNode(timeout: Long, nodes: () -> List<Any>) {
        try {
            compose.waitUntil(timeoutMillis = timeout) { nodes().isNotEmpty() }
        } catch (failure: Throwable) {
            compose.onAllNodes(isRoot(), useUnmergedTree = true).printToLog(TAG)
            throw failure
        }
    }

    private fun waitUntil(what: String, timeout: Long = TIMEOUT_MILLIS, condition: () -> Boolean) {
        val deadline = System.currentTimeMillis() + timeout
        while (System.currentTimeMillis() < deadline) {
            if (condition()) {
                return
            }
            Thread.sleep(POLL_MILLIS)
        }
        compose.onAllNodes(isRoot(), useUnmergedTree = true).printToLog(TAG)
        throw AssertionError("No se cumplió a tiempo: $what")
    }

    private val stateFile: File
        get() = File(application.filesDir, "kitchen-on-go.json")

    private fun saveIds(ids: Map<String, String>) {
        stateFile.writeText(JsonObject(ids.mapValues { JsonPrimitive(it.value) }).toString())
    }

    private fun loadIds(): Map<String, String> {
        argument("address")
        assumeTrue("Sin el restaurante de k1", stateFile.exists())
        return Json.parseToJsonElement(stateFile.readText()).jsonObject.mapValues {
            it.value.jsonPrimitive.content
        }
    }

    private fun log(message: String) {
        Log.i(TAG, message)
    }

    private companion object {
        // La etiqueta que copia el guion de cada paso (android-e2e.sh, run_step).
        const val TAG = "AdelOnGo"
        const val WAITER = "mesero.k"
        const val CUSTOMER = "Luisa Martínez"
        const val TACOS = "Tacos al pastor K"
        const val HORCHATA = "Agua de horchata K"
        const val HAMBURGUESA = "Hamburguesa K"
        const val FLAN = "Flan K"
        const val LIMONADA = "Limonada K"
        const val READY = "ready"
        const val TIMEOUT_MILLIS = 60_000L
        const val POLL_MILLIS = 500L
        // La letra grande es ×1.25 (OrdersScreen); el alto del texto crece al menos ×1.15.
        const val LARGE_TEXT_MIN_RATIO = 1.15
        const val LIGHT_LUMINANCE = 160

        // Fase D: los archivos con los que el guion dice que terminó sus cortes.
        const val K7_DONE_FILE = "/data/local/tmp/adelpos-k7-fin"
        const val K8_PAUSED_FILE = "/data/local/tmp/adelpos-k8-pausado"
        const val K8_DONE_FILE = "/data/local/tmp/adelpos-k8-fin"
        const val K9_DONE_FILE = "/data/local/tmp/adelpos-k9-fin"
        const val K10_DONE_FILE = "/data/local/tmp/adelpos-k10-fin"
        const val WATCH_MILLIS = 100L

        // El margen sin red de la cocina (K1-D33) y la holgura para comparar con lo que mide
        // Android.
        const val NETWORK_GRACE_MILLIS = 10_000L
        const val GRACE_SLACK_MILLIS = 1_000L
        const val SETTLE_AFTER_NETWORK_MILLIS = 3_000L

        // El administrador de prueba del arnés (el mismo de MainComputer).
        const val ADMIN = "admin"
        const val ADMIN_PASSWORD = "admin"
    }
}
