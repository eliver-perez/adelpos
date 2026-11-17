package local.adel.pos.android

import android.os.SystemClock
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.flowOf
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.access.NoWaiterWork
import local.adel.pos.android.core.domain.access.WaiterProfile
import local.adel.pos.android.core.domain.access.WaiterRoster
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.pairing.FoundServerChoice
import local.adel.pos.android.core.domain.pairing.PairingFlow
import local.adel.pos.android.core.domain.pairing.PairingProblem
import local.adel.pos.android.core.domain.pairing.PairingState
import local.adel.pos.android.core.domain.pairing.PairingStep
import local.adel.pos.android.core.domain.pairing.PairingTarget
import local.adel.pos.android.core.domain.pairing.PairingView
import local.adel.pos.android.core.domain.pairing.ServerSearch
import local.adel.pos.android.feature.access.AccessActions
import local.adel.pos.android.feature.access.AccessDependencies

/**
 * Entrada falsa para las pruebas de interfaz con estados forzados: la lista de meseros, el PIN
 * correcto («4829») y un servidor encontrado en la red. Nunca habla con un servidor.
 */
class FakeAccess(
    var waiters: List<WaiterProfile> = SAMPLE_WAITERS,
    var online: Boolean = true,
) : DeviceAccess {
    val mutableStatus =
        MutableStateFlow<DeviceStatus>(
            DeviceStatus.Ready("Fonda Doña Ñeli", "Tableta 1", 3_600, online = true),
        )
    val mutableActive = MutableStateFlow<ActiveWaiter?>(null)
    override val status: StateFlow<DeviceStatus> = mutableStatus
    override val activeWaiter: StateFlow<ActiveWaiter?> = mutableActive

    override suspend fun start() = Unit

    override suspend fun recheck() = Unit

    override suspend fun roster(): WaiterRoster = WaiterRoster(waiters, online)

    override suspend fun enter(userId: String, pin: String): EntryResult {
        val waiter = waiters.first { it.userId == userId }
        if (pin != "4829") {
            return EntryResult.WrongPin
        }
        val active = ActiveWaiter(waiter, hasServerSession = online)
        mutableActive.value = active
        return EntryResult.Entered(active)
    }

    override fun leave() {
        mutableActive.value = null
    }

    override suspend fun forgetServer() = Unit

    override suspend fun clearDeviceLog() = Unit

    override fun acknowledgeRetirement() = Unit

    override fun report(error: AppError, userId: String?) = Unit

    /** Lo que necesitan las pantallas de entrada, con esta entrada falsa. */
    fun dependencies(): AccessDependencies =
        AccessDependencies(
            access = this,
            pairing = FakePairing,
            work = NoWaiterWork,
            idleLock = IdleLock(SystemClock::elapsedRealtime, NoWaiterWork),
            linkState = flowOf(LinkState.ONLINE),
        )

    companion object {
        val SAMPLE_WAITERS =
            listOf(
                WaiterProfile("ana", "Ana", "A", "#3591d4", 1, null),
                WaiterProfile("beto", "Beto", "B", "#a68f14", 1, null),
                WaiterProfile("carla", "Carla", "C", null, 1, Long.MAX_VALUE),
                WaiterProfile("dora", "Dora Elena", "D", "#77818c", 1, null),
            )
    }
}

/** Búsqueda y emparejamiento falsos: un servidor encontrado y un código que espera. */
object FakePairing : PairingFlow {
    override suspend fun search(): ServerSearch =
        ServerSearch.Found(
            listOf(
                FoundServerChoice(
                    "11111111-2222-4333-8444-555555555555",
                    "Fonda Doña Ñeli — Mac de caja",
                ),
            ),
        )

    private var waiting: PairingView? = null

    override suspend fun start(target: PairingTarget, deviceName: String): PairingStep {
        val view =
            PairingView(
                PairingState.PENDING,
                "166101",
                deviceName,
                "Fonda Doña Ñeli",
                System.currentTimeMillis() + 5 * 60_000,
            )
        waiting = view
        return PairingStep.Showing(view)
    }

    // Sigue esperando la aprobación: el código se queda en pantalla.
    override suspend fun poll(): PairingStep =
        waiting?.let { PairingStep.Showing(it) } ?: PairingStep.Failed(PairingProblem.NoSession)

    override suspend fun rename(deviceName: String): PairingStep =
        PairingStep.Failed(PairingProblem.NoSession)

    override suspend fun cancel() = Unit
}

/** Acciones de la puerta que no hacen nada (estados forzados). */
object NoActions : AccessActions {
    override fun recheck() = Unit

    override fun repair() = Unit

    override fun acknowledgeRetirement() = Unit

    override fun clearDeviceLog() = Unit
}
