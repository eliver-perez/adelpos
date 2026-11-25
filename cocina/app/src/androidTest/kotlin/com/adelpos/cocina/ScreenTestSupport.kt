package com.adelpos.cocina

import android.graphics.Bitmap
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.cocina.board.Board
import com.adelpos.cocina.board.BoardActions
import com.adelpos.cocina.board.BoardScreen
import com.adelpos.cocina.board.BoardState
import com.adelpos.cocina.board.Dish
import com.adelpos.cocina.board.DishLine
import com.adelpos.cocina.board.DishPart
import com.adelpos.cocina.board.DishState
import com.adelpos.cocina.board.DisplaySettings
import com.adelpos.cocina.board.OrderCard
import com.adelpos.cocina.board.RecentState
import com.adelpos.cocina.board.TaskRef
import com.adelpos.cocina.ui.CocinaScreen
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.pairing.PairingFlow
import com.adelpos.core.model.pairing.PairingProblem
import com.adelpos.core.model.pairing.PairingStep
import com.adelpos.core.model.pairing.PairingTarget
import com.adelpos.core.model.pairing.ServerSearch
import com.adelpos.core.testing.SystemDialogs
import java.io.File

/** Emparejamiento que no encuentra nada (las pantallas no llaman a la red en estas pruebas). */
object NoPairing : PairingFlow {
    override suspend fun search(): ServerSearch = ServerSearch.Found(emptyList())

    override suspend fun start(target: PairingTarget, deviceName: String): PairingStep =
        PairingStep.Failed(PairingProblem.NoSession)

    override suspend fun poll(): PairingStep = PairingStep.Failed(PairingProblem.NoSession)

    override suspend fun rename(deviceName: String): PairingStep =
        PairingStep.Failed(PairingProblem.NoSession)

    override suspend fun cancel() = Unit
}

/** Las acciones de la pantalla de pedidos, anotadas (sin Go). */
class RecordedActions : BoardActions {
    val calls = mutableListOf<String>()
    val recovered = mutableListOf<List<TaskRef>>()

    override fun markDish(cardId: String, dishId: String) {
        calls += "dish:$dishId"
    }

    override fun markOrder(cardId: String) {
        calls += "order:$cardId"
    }

    override fun openRecent() {
        calls += "recent"
    }

    override fun closeRecent() {
        calls += "close"
    }

    override fun recover(tasks: List<TaskRef>) {
        calls += "recover"
        recovered += tasks
    }

    override fun takeSeat() {
        calls += "take-seat"
    }

    override fun dismissNotice() {
        calls += "dismiss"
    }
}

/**
 * La pantalla de la aplicación con un estado que la prueba cambia (el contenido se pone una sola
 * vez) y el reloj monotónico simulado ([clockMillis]).
 */
class ScreenHost(private val compose: ComposeContentTestRule) {
    val actions = RecordedActions()
    var clockMillis = 0L

    private class Shown(
        val status: KitchenStatus,
        val link: LinkState,
        val board: BoardState,
        val recent: RecentState,
    )

    private var shown: MutableState<Shown>? = null

    fun show(
        status: KitchenStatus = READY,
        board: BoardState = BoardState(),
        recent: RecentState = RecentState.Hidden,
        link: LinkState = LinkState.ONLINE,
    ) {
        val next = Shown(status, link, board, recent)
        val current = shown
        if (current != null) {
            compose.runOnIdle { current.value = next }
        } else {
            val state = mutableStateOf(next)
            shown = state
            compose.setContent {
                val now = state.value
                CocinaScreen(
                    status = now.status,
                    link = now.link,
                    board = now.board,
                    recent = now.recent,
                    pairing = NoPairing,
                    actions = actions,
                    onCheckAgain = {},
                    onRepair = {},
                    clock = { clockMillis },
                )
            }
        }
        compose.waitForIdle()
    }

    companion object {
        val READY = KitchenStatus.Ready(serverName = "Restaurante", deviceName = "Cocina 1")
    }
}

/** Una captura de la pantalla en `files/screens` de la aplicación (el guion la copia). */
fun screenshot(name: String) {
    val instrumentation = InstrumentationRegistry.getInstrumentation()
    SystemDialogs.dismissNotResponding(instrumentation.uiAutomation)
    SystemDialogs.dismissFullScreenNotice(instrumentation.uiAutomation)
    Thread.sleep(SETTLE_MILLIS)
    val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: return
    val folder = File(instrumentation.targetContext.getExternalFilesDir(null), "screens")
    folder.mkdirs()
    File(folder, "$name.png").outputStream().use { output ->
        bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)
    }
}

private const val SETTLE_MILLIS = 400L

/** Platillos y tarjetas de prueba: los casos de la §4. */
object Fixtures {
    fun dish(
        id: String,
        name: String,
        quantity: Int = 1,
        state: DishState = DishState.PENDING,
        isNew: Boolean = false,
        byOrder: Boolean = false,
        notes: String = "",
        parts: List<DishPart> = emptyList(),
    ) =
        Dish(
            id = id,
            name = name,
            quantity = quantity,
            state = state,
            preparing = false,
            isNew = isNew,
            pendingChange = false,
            byOrder = byOrder,
            notes = notes,
            parts = parts,
            tasks = listOf(TaskRef("$id-tarea", "$id-version")),
        )

    fun card(
        id: String,
        title: String,
        subtitle: String = "",
        waiter: String = "Ana López",
        elapsedSeconds: Long = 0,
        dishes: List<Dish>,
        closedByCancellation: Boolean = false,
    ) =
        OrderCard(
            id = id,
            accountId = "cuenta-$id",
            title = title,
            subtitle = subtitle,
            waiterName = waiter,
            // 24/11/2026 19:08 en la Ciudad de México.
            sentAtMillis = 1_795_568_880_000L,
            elapsedSeconds = elapsedSeconds,
            closedByCancellation = closedByCancellation,
            dishes = dishes,
        )

    /**
     * Mesa con modificadores, nota, un agregado («NUEVO») y un cancelado; para llevar; por pedido.
     */
    fun mixed(): List<OrderCard> =
        listOf(
            card(
                "mesa",
                title = "Mesa 4",
                elapsedSeconds = 120,
                dishes =
                    listOf(
                        dish(
                            "tacos",
                            "Tacos al pastor",
                            quantity = 3,
                            notes = "Bien dorados",
                            parts =
                                listOf(
                                    DishPart(
                                        name = null,
                                        lines =
                                            listOf(
                                                DishLine(0, "SIN Cebolla"),
                                                DishLine(0, "Salsas:"),
                                                DishLine(1, "1 x Verde"),
                                            ),
                                    ),
                                ),
                        ),
                        dish("agua", "Agua de horchata", isNew = true),
                        dish("malteada", "Malteada", state = DishState.CANCELLED),
                    ),
            ),
            card(
                "llevar",
                title = "Para llevar",
                subtitle = "Luisa Martínez",
                waiter = "Beto",
                elapsedSeconds = 60,
                dishes =
                    listOf(
                        dish("hamburguesa", "Hamburguesa", quantity = 2),
                        dish("papas", "Papas a la francesa", state = DishState.READY),
                    ),
            ),
            card(
                "barra",
                title = "Mesa 7",
                elapsedSeconds = 30,
                dishes = listOf(dish("limonada", "Limonada", quantity = 2, byOrder = true)),
            ),
        )

    fun board(
        cards: List<OrderCard>,
        settings: DisplaySettings = DisplaySettings.DEFAULT,
        readAtMonotonic: Long = 0,
        license: com.adelpos.core.model.license.LicenseSummary? = null,
    ) =
        BoardState(
            screen = BoardScreen.Orders,
            board =
                Board(
                    cursor = 1,
                    serverTimeMillis = 1_795_569_000_000L,
                    readAtMonotonic = readAtMonotonic,
                    settings = settings,
                    license = license,
                    timezone = "America/Mexico_City",
                    cards = cards,
                    receiptsPending = emptyList(),
                ),
            settings = settings,
        )
}
