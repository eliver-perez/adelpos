package local.adel.pos.android.feature.access

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.access.WaiterWorkGate
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.pairing.PairingFlow

/**
 * Lo que necesita la entrada al dispositivo. Lo arma `AppContainer` (inyección manual) y las
 * pruebas lo arman con implementaciones falsas.
 *
 * @param idleLock el mismo que avisa de cada toque de la pantalla (`MainActivity`).
 * @param wallClock hora del dispositivo, para mostrar «Intenta de nuevo a las 14:35».
 */
class AccessDependencies(
    val access: DeviceAccess,
    val pairing: PairingFlow,
    val work: WaiterWorkGate,
    val idleLock: IdleLock,
    val linkState: Flow<LinkState>,
    val wallClock: () -> Long = System::currentTimeMillis,
)

/**
 * Puerta de entrada de la aplicación: decide qué pantalla se ve según el estado del dispositivo
 * ([DeviceAccess.status]) y el mesero activo, vigila el bloqueo por inactividad (§3.2), vuelve a
 * comprobar el servidor al recuperar la red y cada minuto con la aplicación a la vista, y hace el
 * cambio rápido de mesero.
 */
class AccessViewModel(private val dependencies: AccessDependencies) : ViewModel() {
    private val access = dependencies.access

    val status: StateFlow<DeviceStatus> = access.status
    val activeWaiter: StateFlow<ActiveWaiter?> = access.activeWaiter

    /** Aviso de que no se pudo cambiar de mesero porque tiene pedidos sin enviar (§7). */
    var switchBlocked by mutableStateOf(false)
        private set

    private var periodicCheck: Job? = null

    init {
        viewModelScope.launch { access.start() }
        // Al recuperar la red: versiones, lista de meseros e invalidación de verificadores.
        viewModelScope.launch {
            dependencies.linkState
                .distinctUntilChanged()
                .drop(1)
                .filter { it == LinkState.ONLINE }
                .collect { access.recheck() }
        }
        // Bloqueo por inactividad del mesero activo: al cambiar de mesero se vigila al nuevo.
        viewModelScope.launch {
            access.activeWaiter.collectLatest { waiter ->
                if (waiter != null) {
                    dependencies.idleLock.watch(
                        userId = waiter.userId,
                        timeoutSeconds = ::idleLockSeconds,
                    ) {
                        access.leave()
                    }
                }
            }
        }
    }

    private fun idleLockSeconds(): Int =
        (access.status.value as? DeviceStatus.Ready)?.idleLockSeconds ?: IdleLock.DEFAULT_SECONDS

    /** La aplicación pasó a primer plano: comprueba ya y después cada minuto. */
    fun onForeground() {
        periodicCheck?.cancel()
        periodicCheck = viewModelScope.launch {
            while (true) {
                access.recheck()
                delay(PERIODIC_CHECK_MILLIS)
            }
        }
    }

    /** La aplicación pasó a segundo plano: deja de comprobar (batería). */
    fun onBackground() {
        periodicCheck?.cancel()
        periodicCheck = null
    }

    /**
     * «Cambiar de mesero»: vuelve a la lista sin cerrar la sesión del mesero en Go. No se permite
     * con pedidos sin enviar de ese mesero (§7).
     */
    fun switchWaiter() {
        val waiter = activeWaiter.value ?: return
        viewModelScope.launch {
            if (dependencies.work.workOf(waiter.userId).blocksSwitch) {
                switchBlocked = true
            } else {
                access.leave()
            }
        }
    }

    fun dismissSwitchBlocked() {
        switchBlocked = false
    }

    /** «Volver a comprobar» (actualizar la aplicación o el servidor). */
    fun recheck() {
        viewModelScope.launch { access.recheck() }
    }

    /** «Emparejar de nuevo» (confirmado): deja de usar el servidor emparejado. */
    fun repair() {
        viewModelScope.launch { access.forgetServer() }
    }

    /** El mesero leyó el aviso de dispositivo retirado. */
    fun acknowledgeRetirement() {
        access.acknowledgeRetirement()
    }

    /** «Borrar la bitácora» (confirmado). */
    fun clearDeviceLog() {
        viewModelScope.launch { access.clearDeviceLog() }
    }

    companion object {
        private const val PERIODIC_CHECK_MILLIS = 60_000L

        fun factory(dependencies: AccessDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { AccessViewModel(dependencies) }
            }
    }
}
