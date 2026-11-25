package com.adelpos.meseros

import android.os.SystemClock
import com.adelpos.core.model.access.UnsentOrder
import com.adelpos.core.model.connection.FixedServerLink
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.pairing.FoundServerChoice
import com.adelpos.core.model.pairing.PairingFlow
import com.adelpos.core.model.pairing.PairingProblem
import com.adelpos.core.model.pairing.PairingState
import com.adelpos.core.model.pairing.PairingStep
import com.adelpos.core.model.pairing.PairingTarget
import com.adelpos.core.model.pairing.PairingView
import com.adelpos.core.model.pairing.ServerSearch
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.DeviceAccess
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.EntryResult
import com.adelpos.meseros.core.domain.access.IdleLock
import com.adelpos.meseros.core.domain.access.NoWaiterWork
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.access.WaiterRoster
import com.adelpos.meseros.feature.access.AccessActions
import com.adelpos.meseros.feature.access.AccessDependencies
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * Entrada falsa para las pruebas de interfaz con estados forzados: la lista de meseros, el PIN
 * correcto («4829») y un servidor encontrado en la red. Nunca habla con un servidor. Con [online]
 * en false, la lista es la de los meseros conocidos y el estado único de conexión ([link], etapa
 * A3) empieza en «Sin conexión»; la prueba lo puede cambiar.
 */
class FakeAccess(
    var waiters: List<WaiterProfile> = SAMPLE_WAITERS,
    var online: Boolean = true,
) : DeviceAccess {
    val link = FixedServerLink(if (online) LinkState.ONLINE else LinkState.OFFLINE)
    val mutableStatus =
        MutableStateFlow<DeviceStatus>(DeviceStatus.Ready("Fonda Doña Ñeli", "Tableta 1", 3_600))
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

    /** Pedidos sin enviar que mostraría «Emparejar de nuevo». */
    var pending: List<UnsentOrder> = emptyList()

    override suspend fun pendingOrders(): List<UnsentOrder> = pending

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
            serverLink = link,
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

    override suspend fun pendingOrders(): List<UnsentOrder> = emptyList()

    override fun acknowledgeRetirement() = Unit

    override fun clearDeviceLog() = Unit
}
