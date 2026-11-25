package com.adelpos.cocina.board

import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.realtime.RealtimeEvent
import com.adelpos.core.model.realtime.RealtimeSignal
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ApiProblem
import com.adelpos.core.network.api.IgnoredBody
import com.adelpos.core.network.connection.ServerLinkMonitor
import com.adelpos.core.network.session.AuthorizedServer
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El ciclo de la pantalla de pedidos con un Go falso (K1-D25 y el contrato de la Fase A):
 * `connect`, `orders` con su cursor, una sola lectura aunque lleguen varios eventos, el acuse de
 * recepción, las marcas, «Recuperar» y la pantalla de cada error (sin cupo, cupo liberado, sin
 * conectar, sin activar, hora incorrecta, revocada).
 */
@OptIn(ExperimentalCoroutinesApi::class)
class KitchenBoardTest {
    private class FakeAccess : BoardAccess {
        override val status = MutableStateFlow<KitchenStatus>(KitchenStatus.Unpaired)
        val reported = mutableListOf<AppError>()

        override fun report(error: AppError) {
            reported += error
        }

        override fun channelServer(): AuthorizedServer? = null
    }

    private class FakeService : KitchenDisplayService {
        val calls = mutableListOf<String>()
        var connectAnswers = ArrayDeque<ApiOutcome<ConnectResponseDto>>()
        var ordersAnswers = ArrayDeque<ApiOutcome<OrdersResponseDto>>()
        var markAnswer: ApiOutcome<MarkResultDto> = ApiOutcome.Success(MarkResultDto(updated = 1))
        val markAnswers = ArrayDeque<ApiOutcome<MarkResultDto>>()
        var recoverAnswer: ApiOutcome<RecoverResultDto> = ApiOutcome.Success(RecoverResultDto(1))
        var readDelayMillis = 0L
        val marked = mutableListOf<List<TaskRef>>()
        val receipts = mutableListOf<List<ReceiptDto>>()
        var lastTakeSeat = false

        override suspend fun connect(takeSeat: Boolean): ApiOutcome<ConnectResponseDto> {
            calls += if (takeSeat) "connect(take_seat)" else "connect"
            lastTakeSeat = takeSeat
            return connectAnswers.removeFirstOrNull() ?: ApiOutcome.Success(ConnectResponseDto())
        }

        override suspend fun orders(): ApiOutcome<OrdersResponseDto> {
            calls += "orders"
            delay(readDelayMillis)
            return ordersAnswers.removeFirstOrNull() ?: ApiOutcome.Success(response())
        }

        override suspend fun markItems(tasks: List<TaskRef>): ApiOutcome<MarkResultDto> {
            calls += "items/ready"
            marked += tasks
            return markAnswers.removeFirstOrNull() ?: markAnswer
        }

        override suspend fun markOrder(tasks: List<TaskRef>): ApiOutcome<MarkResultDto> {
            calls += "orders/ready"
            marked += tasks
            return markAnswers.removeFirstOrNull() ?: markAnswer
        }

        override suspend fun recent(): ApiOutcome<RecentResponseDto> {
            calls += "recent"
            return ApiOutcome.Success(RecentResponseDto())
        }

        override suspend fun recover(tasks: List<TaskRef>): ApiOutcome<RecoverResultDto> {
            calls += "recover"
            return recoverAnswer
        }

        override suspend fun receipts(receipts: List<ReceiptDto>): ApiOutcome<IgnoredBody> {
            calls += "receipts"
            this.receipts += receipts
            return ApiOutcome.Success(JsonObject(emptyMap()))
        }
    }

    private val access = FakeAccess()
    private val service = FakeService()
    private val signals = MutableSharedFlow<RealtimeSignal>(extraBufferCapacity = 16)
    private var monotonic = 0L
    private val link = MutableStateFlow(LinkState.ONLINE)
    private val played = mutableListOf<Pair<Set<AlertSound>, String>>()

    private fun TestScope.board(linkState: StateFlow<LinkState> = link): KitchenBoard {
        val board =
            KitchenBoard(
                access = access,
                service = service,
                signals = signals,
                monotonic = { monotonic },
                link = linkState,
                alerts = { sounds, volume -> played += sounds to volume },
            )
        board.start(TestScope(UnconfinedTestDispatcher(testScheduler)))
        return board
    }

    @Test
    fun conectaLeeLosPedidosYConfirmaLasComandas() = runTest {
        service.ordersAnswers +=
            ApiOutcome.Success(
                response(
                    orders = listOf(card("t1")),
                    receipts = listOf(ReceiptDto("comanda-1", "cocina")),
                ),
            )
        val board = board()
        assertEquals(BoardScreen.Connecting, board.state.value.screen)
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertEquals(listOf("connect", "orders", "receipts"), service.calls)
        assertEquals(BoardScreen.Orders, board.state.value.screen)
        assertEquals(listOf("t1"), board.state.value.board?.cards?.map { it.id })
        assertEquals(listOf(ReceiptDto("comanda-1", "cocina")), service.receipts.single())
        // Sin emparejar otra vez, todo se olvida.
        access.status.value = KitchenStatus.Unpaired
        advanceUntilIdle()
        assertEquals(BoardState(), board.state.value)
    }

    @Test
    fun variosEventosSeguidosSonUnaSolaLecturaMas() = runTest {
        service.readDelayMillis = 1_000
        board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        runCurrent()
        // Mientras la primera lectura está en curso llegan cinco eventos.
        repeat(5) { signals.tryEmit(RealtimeSignal.Changes(it.toLong(), listOf(event()))) }
        advanceUntilIdle()
        assertEquals(2, service.calls.count { it == "orders" })
        // Un mensaje sin eventos no lee; un reset sí.
        signals.tryEmit(RealtimeSignal.Changes(9, emptyList()))
        advanceUntilIdle()
        assertEquals(2, service.calls.count { it == "orders" })
        signals.tryEmit(
            RealtimeSignal.Rebuild(10, com.adelpos.core.model.realtime.RebuildReason.SERVER_RESET),
        )
        advanceUntilIdle()
        assertEquals(3, service.calls.count { it == "orders" })
    }

    @Test
    fun sinCupoMuestraLosNumerosYLoVuelveAPedirCada30Segundos() = runTest {
        service.connectAnswers += rejected(409, "LICENSE_SEAT_UNAVAILABLE", seat = true)
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        runCurrent()
        assertEquals(BoardScreen.NoSeat(inUse = 2, limit = 2), board.state.value.screen)
        assertEquals(listOf("connect"), service.calls)
        advanceTimeBy(29_000)
        assertEquals(1, service.calls.count { it == "connect" })
        advanceTimeBy(2_000)
        assertEquals(2, service.calls.count { it == "connect" })
        assertEquals(BoardScreen.Orders, board.state.value.screen)
    }

    @Test
    fun cupoLiberadoEsperaAUsarEstaPantalla() = runTest {
        service.connectAnswers += rejected(409, "KITCHEN_DISPLAY_SEAT_RELEASED")
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertEquals(BoardScreen.SeatReleased, board.state.value.screen)
        assertEquals(listOf("connect"), service.calls)
        board.takeSeat()
        advanceUntilIdle()
        assertEquals(listOf("connect", "connect(take_seat)", "orders"), service.calls)
        assertEquals(BoardScreen.Orders, board.state.value.screen)
    }

    @Test
    fun sinConectarVuelveAConnectYSinActivarNoMuestraPedidos() = runTest {
        service.ordersAnswers += rejected(409, "KITCHEN_DISPLAY_NOT_CONNECTED")
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertEquals(listOf("connect", "orders", "connect", "orders"), service.calls)
        assertEquals(BoardScreen.Orders, board.state.value.screen)
        // Sin activar: la pantalla no muestra pedidos y se vuelve a intentar con el siguiente
        // evento (license.changed).
        service.ordersAnswers += rejected(409, "LICENSE_NOT_ACTIVATED")
        service.connectAnswers += rejected(409, "LICENSE_NOT_ACTIVATED")
        signals.tryEmit(RealtimeSignal.Changes(1, listOf(event())))
        advanceUntilIdle()
        assertEquals(BoardScreen.NotActivated, board.state.value.screen)
        signals.tryEmit(RealtimeSignal.Changes(2, listOf(event("license.changed"))))
        advanceUntilIdle()
        assertEquals(BoardScreen.NotActivated, board.state.value.screen)
        signals.tryEmit(RealtimeSignal.Changes(3, listOf(event("license.changed"))))
        advanceUntilIdle()
        assertEquals(BoardScreen.Orders, board.state.value.screen)
    }

    @Test
    fun marcarUnPlatilloYElPedidoCompleto() = runTest {
        // La lectura inicial y la de después de marcar el platillo traen las dos tarjetas.
        repeat(2) {
            service.ordersAnswers +=
                ApiOutcome.Success(response(orders = listOf(card("t1"), card("t2"))))
        }
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        board.markDish("t1", "t1-a")
        advanceUntilIdle()
        assertEquals(listOf(TaskRef("t1-a-1", "v1")), service.marked.single())
        assertTrue(
            "se vuelve a leer después de marcar",
            service.calls.takeLast(2) == listOf("items/ready", "orders"),
        )
        // La lectura de Go manda: lo marcado ya no se ve «en camino».
        assertTrue(board.state.value.markingDishes.isEmpty())
        // El botón del pedido manda todas las tareas de la tarjeta.
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t2"))))
        board.markOrder("t1")
        advanceUntilIdle()
        assertEquals(listOf("t1-a-1", "t1-b-1"), service.marked.last().map { it.id })
        assertEquals(listOf("t2"), board.state.value.board?.cards?.map { it.id })
    }

    @Test
    fun unaLecturaQueYaIbaEnCaminoNoHaceParpadearLoMarcado() = runTest {
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        // Una lectura lenta sale antes de marcar y trae el platillo todavía pendiente.
        service.readDelayMillis = 1_000
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        signals.tryEmit(RealtimeSignal.Changes(2, listOf(event())))
        runCurrent()
        board.markDish("t1", "t1-a")
        runCurrent()
        assertEquals(setOf("t1-a"), board.state.value.markingDishes)
        // Llega la lectura vieja: el platillo se sigue viendo listo.
        service.ordersAnswers += ApiOutcome.Success(response(orders = emptyList()))
        advanceTimeBy(1_001)
        runCurrent()
        assertEquals(setOf("t1-a"), board.state.value.markingDishes)
        // La lectura empezada después de marcar manda (Go ya cerró la tarjeta).
        advanceUntilIdle()
        assertTrue(board.state.value.markingDishes.isEmpty())
        assertEquals(emptyList<String>(), board.state.value.board?.cards?.map { it.id })
    }

    // --- Fase D: conexión tolerante (§5.1) ---

    @Test
    fun sinConexionAvisoGrandeNoSeMarcaYAlVolverSeLeeTodo() = runTest {
        repeat(3) {
            service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        }
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertFalse(board.state.value.offline)
        link.value = LinkState.OFFLINE
        runCurrent()
        assertTrue(board.state.value.offline)
        assertFalse("no se puede marcar", board.state.value.markingAllowed)
        assertFalse("tampoco es la hora", board.state.value.clockBlocked)
        board.markDish("t1", "t1-a")
        board.markOrder("t1")
        advanceUntilIdle()
        assertTrue(service.marked.isEmpty())
        assertEquals(
            "los pedidos se siguen viendo",
            listOf("t1"),
            board.state.value.board?.cards?.map { it.id },
        )
        val readsBefore = service.calls.count { it == "orders" }
        link.value = LinkState.ONLINE
        advanceUntilIdle()
        assertFalse(board.state.value.offline)
        assertTrue(board.state.value.markingAllowed)
        assertEquals(
            "al volver, el estado completo",
            readsBefore + 1,
            service.calls.count { it == "orders" },
        )
    }

    @Test
    fun unaMarcaSinRespuestaSeRepiteEnSilencioConSuPalomita() = runTest {
        repeat(3) {
            service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        }
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        service.markAnswers += ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = true)
        service.markAnswers += ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = true)
        // La tercera vez Go ya la tenía (la primera sí le llegó): skipped, que es un éxito.
        service.markAnswers +=
            ApiOutcome.Success(
                MarkResultDto(updated = 0, skipped = listOf(SkippedDto("t1-a-1", "already_ready"))),
            )
        board.markDish("t1", "t1-a")
        runCurrent()
        assertEquals(1, service.marked.size)
        assertEquals(setOf("t1-a"), board.state.value.markingDishes)
        assertFalse("sin ningún aviso", board.state.value.offline)
        assertEquals(null, board.state.value.notice)
        advanceTimeBy(2_001)
        assertEquals(2, service.marked.size)
        assertEquals(setOf("t1-a"), board.state.value.markingDishes)
        advanceTimeBy(2_001)
        assertEquals(3, service.marked.size)
        advanceUntilIdle()
        assertEquals(3, service.marked.size)
        // La lectura de después manda (aquí Go ya cerró la tarjeta).
        assertTrue(board.state.value.markingDishes.isEmpty())
    }

    @Test
    fun conSinConexionLaMarcaEsperaYSeRepiteAlVolver() = runTest {
        repeat(3) {
            service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        }
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        service.markAnswers += ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = false)
        board.markDish("t1", "t1-a")
        runCurrent()
        link.value = LinkState.OFFLINE
        advanceTimeBy(60_000)
        assertEquals("esperando la conexión", 1, service.marked.size)
        assertEquals(setOf("t1-a"), board.state.value.markingDishes)
        link.value = LinkState.ONLINE
        advanceUntilIdle()
        assertEquals(2, service.marked.size)
        assertTrue(board.state.value.markingDishes.isEmpty())
    }

    @Test
    fun siGoDiceOtraCosaLaMarcaPendienteSeQuita() = runTest {
        repeat(3) {
            service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        }
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        service.markAnswers += ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = true)
        service.markAnswers += rejected(400, "KITCHEN_VALIDATION")
        board.markDish("t1", "t1-a")
        advanceUntilIdle()
        assertEquals(2, service.marked.size)
        assertTrue(board.state.value.markingDishes.isEmpty())
    }

    @Test
    fun laToleranciaDeLaCocinaConUnRelojSimulado() = runTest {
        // El estado único de conexión real (el de la barra), con el margen sin red de la cocina.
        var now = 0L
        val monitor =
            ServerLinkMonitor(
                network = emptyFlow(),
                initialNetwork = LinkState.ONLINE,
                foreground = MutableStateFlow(false),
                channelOpen = flowOf(true),
                probing = { _, _ -> true },
                scope = backgroundScope,
                clock = { now },
                networkLossGraceMillis = 10_000,
            )
        val board = board(monitor.state)
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        // Un corte de 1 a 5 s: una falla y luego responde. Nada visible.
        for (seconds in 1..5) {
            monitor.noAnswer()
            now += seconds * 1_000L
            runCurrent()
            assertFalse("corte de $seconds s", board.state.value.offline)
            monitor.answered()
        }
        // Sin red de Android 9 s (dentro del margen): nada; 11 s: «Sin conexión».
        monitor.onNetwork(LinkState.OFFLINE)
        now += 9_000
        monitor.onNetwork(LinkState.ONLINE)
        runCurrent()
        assertFalse(board.state.value.offline)
        monitor.onNetwork(LinkState.OFFLINE)
        now += 11_000
        monitor.onNetwork(LinkState.OFFLINE)
        runCurrent()
        assertTrue(board.state.value.offline)
        monitor.onNetwork(LinkState.ONLINE)
        monitor.answered()
        runCurrent()
        assertFalse(board.state.value.offline)
        // Tres fallas o más durante más de 20 s: «Sin conexión».
        repeat(4) {
            monitor.noAnswer()
            now += 6_000
        }
        monitor.noAnswer()
        runCurrent()
        assertTrue(board.state.value.offline)
        monitor.answered()
        runCurrent()
        assertFalse(board.state.value.offline)
    }

    // --- Fase D: alertas (§5.2) ---

    @Test
    fun suenaLoNuevoUnaVezYNoLaPrimeraLectura() = runTest {
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        service.ordersAnswers +=
            ApiOutcome.Success(response(orders = listOf(card("t1"), card("t2"))))
        service.ordersAnswers +=
            ApiOutcome.Success(response(orders = listOf(card("t1"), card("t2"))))
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertTrue("la primera lectura no suena", played.isEmpty())
        assertTrue(board.state.value.freshCards.isEmpty())
        signals.tryEmit(RealtimeSignal.Changes(1, listOf(event())))
        advanceUntilIdle()
        assertEquals(listOf(setOf(AlertSound.NEW_ORDER) to "medium"), played)
        assertEquals(setOf("t2"), board.state.value.freshCards.keys)
        // La misma lectura otra vez no vuelve a sonar.
        signals.tryEmit(RealtimeSignal.Changes(2, listOf(event())))
        advanceUntilIdle()
        assertEquals(1, played.size)
    }

    @Test
    fun conElSonidoApagadoSoloSeAnima() = runTest {
        val quiet = DisplayConfigDto(soundEnabled = false)
        service.ordersAnswers +=
            ApiOutcome.Success(response(orders = listOf(card("t1"))).copy(display = quiet))
        service.ordersAnswers +=
            ApiOutcome.Success(
                response(orders = listOf(card("t1"), card("t2"))).copy(display = quiet),
            )
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        signals.tryEmit(RealtimeSignal.Changes(1, listOf(event())))
        advanceUntilIdle()
        assertTrue(played.isEmpty())
        assertEquals(setOf("t2"), board.state.value.freshCards.keys)
    }

    @Test
    fun alVolverDeSinConexionSuenaUnaSolaVez() = runTest {
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        // Mientras no había conexión llegaron tres pedidos.
        service.ordersAnswers +=
            ApiOutcome.Success(
                response(orders = listOf(card("t1"), card("t2"), card("t3"), card("t4"))),
            )
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        link.value = LinkState.OFFLINE
        runCurrent()
        link.value = LinkState.ONLINE
        advanceUntilIdle()
        assertEquals(listOf(setOf(AlertSound.NEW_ORDER) to "medium"), played)
        assertEquals(setOf("t2", "t3", "t4"), board.state.value.freshCards.keys)
    }

    @Test
    fun loRecuperadoAquiNoSuena() = runTest {
        service.ordersAnswers += ApiOutcome.Success(response(orders = emptyList()))
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        board.openRecent()
        board.recover(listOf(TaskRef("t1-a-1", "v1"), TaskRef("t1-b-1", "v1")))
        advanceUntilIdle()
        assertEquals(listOf("t1"), board.state.value.board?.cards?.map { it.id })
        assertTrue(played.isEmpty())
    }

    @Test
    fun conLaHoraIncorrectaNoSePuedeMarcar() = runTest {
        service.markAnswer = rejected(409, "SYSTEM_CLOCK_INVALID")
        service.ordersAnswers += ApiOutcome.Success(response(orders = listOf(card("t1"))))
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertTrue(board.state.value.markingAllowed)
        board.markDish("t1", "t1-a")
        advanceUntilIdle()
        assertFalse(board.state.value.markingAllowed)
        assertEquals(listOf("t1"), board.state.value.board?.cards?.map { it.id })
        // Mientras tanto, tocar no manda nada.
        val before = service.calls.size
        board.markDish("t1", "t1-b")
        advanceUntilIdle()
        assertEquals(before, service.calls.size)
    }

    @Test
    fun recuperarYUnPlatilloQueYaNoSeRecupera() = runTest {
        val board = board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        board.openRecent()
        advanceUntilIdle()
        assertEquals(RecentState.Shown(emptyList(), null), board.recent.value)
        board.recover(listOf(TaskRef("x", "v")))
        advanceUntilIdle()
        // Se vuelven a leer los pedidos y «Recientes» (en cualquier orden).
        assertEquals(setOf("recover", "orders", "recent"), service.calls.takeLast(3).toSet())
        service.recoverAnswer =
            rejected(
                409,
                "KITCHEN_NOT_RECOVERABLE",
                message = "Ese platillo ya no se puede regresar a la pantalla.",
            )
        board.recover(listOf(TaskRef("y", "v")))
        advanceUntilIdle()
        assertEquals(
            RecentState.Shown(emptyList(), "Ese platillo ya no se puede regresar a la pantalla."),
            board.recent.value,
        )
        board.closeRecent()
        assertEquals(RecentState.Hidden, board.recent.value)
    }

    @Test
    fun revocadaVaALaEntrada() = runTest {
        service.ordersAnswers += rejected(401, "DEVICE_REVOKED")
        board()
        access.status.value = KitchenStatus.Ready("Restaurante", "Cocina 1")
        advanceUntilIdle()
        assertTrue(access.reported.single() is AppError.DeviceRevoked)
    }

    private fun <T> rejected(
        status: Int,
        code: String,
        seat: Boolean = false,
        message: String? = null,
    ): ApiOutcome<T> {
        val details =
            if (seat) {
                JsonObject(
                    mapOf(
                        "seat" to JsonPrimitive("kitchen_displays"),
                        "in_use" to JsonPrimitive(2),
                        "limit" to JsonPrimitive(2),
                    ),
                )
            } else {
                null
            }
        return ApiOutcome.Rejected(ApiProblem(status, code, message, "req", details))
    }

    private fun event(kind: String = "kitchen.ticket.created") =
        RealtimeEvent(1, "e", 1, kind, "kitchen", null, null, null)

    private fun card(id: String) =
        OrderCardDto(
            id = id,
            title = "Mesa $id",
            sentAt = "2026-11-24T19:00:00Z",
            items =
                listOf(
                    OrderItemDto(
                        id = "$id-a",
                        name = "Tacos",
                        tasks = listOf(TaskRefDto("$id-a-1", "v1")),
                    ),
                    OrderItemDto(
                        id = "$id-b",
                        name = "Agua",
                        tasks = listOf(TaskRefDto("$id-b-1", "v1")),
                    ),
                ),
        )
}

private fun response(
    orders: List<OrderCardDto> = emptyList(),
    receipts: List<ReceiptDto> = emptyList(),
) =
    OrdersResponseDto(
        cursor = 10,
        serverTime = "2026-11-24T19:20:00Z",
        orders = orders,
        receiptsPending = receipts,
    )
