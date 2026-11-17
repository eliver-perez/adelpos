package local.adel.pos.android.feature.access

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.access.WaiterProfile
import local.adel.pos.android.core.domain.access.WaiterRoster
import local.adel.pos.android.core.domain.access.WaiterWork
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.pairing.PairingFlow
import local.adel.pos.android.core.domain.pairing.PairingProblem
import local.adel.pos.android.core.domain.pairing.PairingState
import local.adel.pos.android.core.domain.pairing.PairingStep
import local.adel.pos.android.core.domain.pairing.PairingTarget
import local.adel.pos.android.core.domain.pairing.PairingView
import local.adel.pos.android.core.domain.pairing.ServerSearch
import local.adel.pos.android.feature.access.findserver.FindServerViewModel
import local.adel.pos.android.feature.access.pin.EntryMessage
import local.adel.pos.android.feature.access.pin.PinEntryViewModel
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * ViewModels de la entrada en la JVM con un acceso falso: el PIN nunca se guarda en el estado que
 * Android escribe a disco, los mensajes de cada resultado, el bloqueo por inactividad que regresa a
 * la entrada (y no con trabajo pendiente), el cambio de mesero bloqueado con pedidos sin enviar, la
 * comprobación al recuperar la red y la espera del código cada 2 s.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class AccessViewModelsTest {
    private val dispatcher = StandardTestDispatcher()

    // Los ViewModel de cada prueba viven en este almacén; al limpiarlo se cancelan sus corrutinas
    // (la vigilancia del bloqueo por inactividad no termina sola con trabajo pendiente).
    private val store = ViewModelStore()
    private val ana = WaiterProfile("ana", "Ana", "A", "#3591d4", 1, null)

    @Before
    fun main() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun reset() {
        store.clear()
        Dispatchers.resetMain()
    }

    private inline fun <reified T : ViewModel> owned(key: String, noinline create: () -> T): T =
        ViewModelProvider.create(store, viewModelFactory { initializer { create() } })[
                key, T::class]

    private class FakeAccess : DeviceAccess {
        val mutableStatus =
            MutableStateFlow<DeviceStatus>(DeviceStatus.Ready("Fonda", "Tableta 1", 60, true))
        val mutableActive = MutableStateFlow<ActiveWaiter?>(null)
        var next: EntryResult = EntryResult.WrongPin
        val pins = mutableListOf<String>()
        var rechecks = 0
        var rosterCalls = 0

        override val status: StateFlow<DeviceStatus> = mutableStatus
        override val activeWaiter: StateFlow<ActiveWaiter?> = mutableActive

        override suspend fun start() = Unit

        override suspend fun recheck() {
            rechecks += 1
        }

        override suspend fun roster(): WaiterRoster {
            rosterCalls += 1
            return WaiterRoster(listOf(WaiterProfile("ana", "Ana", "A", null, 1, null)), true)
        }

        override suspend fun enter(userId: String, pin: String): EntryResult {
            pins.add(pin)
            val result = next
            if (result is EntryResult.Entered) {
                mutableActive.value = result.waiter
            }
            return result
        }

        override fun leave() {
            mutableActive.value = null
        }

        override suspend fun forgetServer() = Unit

        override suspend fun clearDeviceLog() = Unit

        override fun acknowledgeRetirement() = Unit

        override fun report(error: AppError, userId: String?) = Unit
    }

    @Test
    fun elPinSoloViveEnMemoriaYCadaResultadoTieneSuMensaje() =
        runTest(dispatcher) {
            val access = FakeAccess()
            val saved = SavedStateHandle()
            val viewModel = owned("pin") { PinEntryViewModel(access, saved) }
            advanceUntilIdle()
            viewModel.select(ana)
            "4829".forEach(viewModel::onDigit)
            assertEquals(4, viewModel.pinLength)
            // En el estado guardado solo está el mesero elegido, nunca los dígitos.
            assertEquals(setOf("pinEntry.selected"), saved.keys())
            assertFalse(saved.keys().any { saved.get<Any>(it).toString().contains("4829") })
            viewModel.submit()
            advanceUntilIdle()
            assertEquals(listOf("4829"), access.pins)
            assertEquals(EntryMessage.WrongPin, viewModel.message)
            assertEquals("cada intento borra lo escrito", 0, viewModel.pinLength)

            access.next = EntryResult.Locked(1_000L)
            "482".forEach(viewModel::onDigit)
            viewModel.submit()
            advanceUntilIdle()
            assertEquals("con menos de 4 dígitos no se envía", 1, access.pins.size)
            "9".forEach(viewModel::onDigit)
            viewModel.submit()
            advanceUntilIdle()
            assertEquals(EntryMessage.Locked(1_000L), viewModel.message)

            for ((result, message) in
                listOf(
                    EntryResult.NotAllowed to EntryMessage.NotAllowed,
                    EntryResult.UnknownOffline to EntryMessage.UnknownOffline,
                    EntryResult.Failed(AppError.Unreachable) to EntryMessage.Unreachable,
                    EntryResult.Failed(
                        AppError.UpdateRequired(
                            "CLIENT_UPDATE_REQUIRED",
                            "0.2.0",
                            null,
                            null,
                            null,
                        ),
                    ) to EntryMessage.UpdateRequired,
                    EntryResult.Failed(AppError.ServerProblem(500, null, "Falla de Go.", null)) to
                        EntryMessage.Failed("Falla de Go."),
                )) {
                access.next = result
                "123456".forEach(viewModel::onDigit)
                viewModel.onDigit('7')
                assertEquals("como máximo 6 dígitos", 6, viewModel.pinLength)
                viewModel.submit()
                advanceUntilIdle()
                assertEquals(message, viewModel.message)
            }
            access.next = EntryResult.Entered(ActiveWaiter(ana, hasServerSession = true))
            "4829".forEach(viewModel::onDigit)
            viewModel.onDelete()
            viewModel.onDigit('9')
            viewModel.submit()
            advanceUntilIdle()
            assertEquals("4829", access.pins.last())
            assertNull(viewModel.message)
            assertNull("al entrar se olvida el elegido", viewModel.selectedId)
            store.clear()
        }

    private fun gate(
        access: FakeAccess,
        work: WaiterWork,
        link: MutableStateFlow<LinkState>,
        scope: TestScope,
    ) =
        owned("gate-${System.identityHashCode(access)}") {
            AccessViewModel(
                AccessDependencies(
                    access = access,
                    pairing = NoPairing,
                    work = { work },
                    idleLock =
                        IdleLock(clock = { scope.testScheduler.currentTime }, work = { work }),
                    linkState = link,
                ),
            )
        }

    @Test
    fun elBloqueoPorInactividadRegresaALaEntradaSalvoConTrabajo() =
        runTest(dispatcher) {
            val access = FakeAccess()
            val link = MutableStateFlow(LinkState.ONLINE)
            gate(access, WaiterWork.NONE, link, this)
            access.mutableActive.value = ActiveWaiter(ana, hasServerSession = true)
            runCurrent()
            advanceTimeBy(59_000)
            runCurrent()
            assertTrue(access.mutableActive.value != null)
            advanceTimeBy(1_001)
            runCurrent()
            assertNull("a los 60 s sin uso vuelve a la entrada", access.mutableActive.value)

            val busy = FakeAccess()
            gate(busy, WaiterWork(captureInProgress = true, unsentOrders = 0), link, this)
            busy.mutableActive.value = ActiveWaiter(ana, hasServerSession = true)
            runCurrent()
            advanceTimeBy(10 * 60_000)
            runCurrent()
            assertTrue("con una captura en curso no se bloquea", busy.mutableActive.value != null)
            store.clear()
        }

    @Test
    fun noSeCambiaDeMeseroConPedidosSinEnviarYSeCompruebaAlVolverLaRed() =
        runTest(dispatcher) {
            val access = FakeAccess()
            val link = MutableStateFlow(LinkState.ONLINE)
            val viewModel =
                gate(access, WaiterWork(captureInProgress = false, unsentOrders = 2), link, this)
            access.mutableActive.value = ActiveWaiter(ana, hasServerSession = true)
            runCurrent()
            viewModel.switchWaiter()
            runCurrent()
            assertTrue(viewModel.switchBlocked)
            assertTrue(access.mutableActive.value != null)
            viewModel.dismissSwitchBlocked()
            assertFalse(viewModel.switchBlocked)

            val free = FakeAccess()
            val freeGate =
                gate(free, WaiterWork(captureInProgress = true, unsentOrders = 0), link, this)
            free.mutableActive.value = ActiveWaiter(ana, hasServerSession = true)
            runCurrent()
            freeGate.switchWaiter()
            runCurrent()
            assertNull(
                "una captura en curso se queda con su mesero y sí se cambia",
                free.mutableActive.value,
            )

            val before = access.rechecks
            link.value = LinkState.OFFLINE
            runCurrent()
            link.value = LinkState.ONLINE
            runCurrent()
            assertEquals(before + 1, access.rechecks)
            store.clear()
        }

    private object NoPairing : PairingFlow {
        override suspend fun search() = ServerSearch.Found(emptyList())

        override suspend fun start(target: PairingTarget, deviceName: String): PairingStep =
            PairingStep.Failed(PairingProblem.NoSession)

        override suspend fun poll(): PairingStep = PairingStep.Failed(PairingProblem.NoSession)

        override suspend fun rename(deviceName: String): PairingStep =
            PairingStep.Failed(PairingProblem.NoSession)

        override suspend fun cancel() = Unit
    }

    @Test
    fun elCodigoSeConsultaCadaDosSegundosHastaEmparejar() =
        runTest(dispatcher) {
            var polls = 0
            val pending = PairingView(PairingState.PENDING, "166101", "Tableta 1", "Fonda", 0)
            val flow =
                object : PairingFlow {
                    override suspend fun search() = ServerSearch.Found(emptyList())

                    override suspend fun start(
                        target: PairingTarget,
                        deviceName: String,
                    ): PairingStep {
                        assertEquals(PairingTarget.Address("192.168.1.20"), target)
                        assertEquals("Tableta 1", deviceName)
                        return PairingStep.Showing(pending)
                    }

                    override suspend fun poll(): PairingStep {
                        polls += 1
                        return PairingStep.Showing(
                            if (polls < 3) pending else pending.copy(state = PairingState.PAIRED),
                        )
                    }

                    override suspend fun rename(deviceName: String) = PairingStep.Showing(pending)

                    override suspend fun cancel() = Unit
                }
            val saved = SavedStateHandle()
            val viewModel = owned("find") { FindServerViewModel(flow, saved) }
            viewModel.proposeName("Tableta 1")
            viewModel.onAddressChange("192.168.1.20")
            assertEquals("192.168.1.20", saved["findServer.address"])
            viewModel.requestAddress()
            runCurrent()
            assertEquals("166101", viewModel.code?.code)
            advanceTimeBy(FindServerViewModel.POLL_MILLIS + 1)
            assertEquals(1, polls)
            advanceTimeBy(2 * FindServerViewModel.POLL_MILLIS)
            assertEquals(3, polls)
            assertEquals(PairingState.PAIRED, viewModel.code?.state)
            advanceTimeBy(10 * FindServerViewModel.POLL_MILLIS)
            assertEquals("ya emparejado deja de consultar", 3, polls)
            store.clear()
        }
}
