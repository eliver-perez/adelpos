package local.adel.pos.android.feature.access.pin

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.createSavedStateHandle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import kotlinx.coroutines.launch
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.WaiterProfile
import local.adel.pos.android.core.domain.access.WaiterRoster
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.ui.components.AdelPinPadDefaults

/** Mensaje de la pantalla de entrada después de escribir el PIN. */
sealed interface EntryMessage {
    data object WrongPin : EntryMessage

    /** Bloqueado hasta [untilEpochMillis] (null: hasta que un administrador lo desbloquee). */
    data class Locked(val untilEpochMillis: Long?) : EntryMessage

    data object NotAllowed : EntryMessage

    data object UnknownOffline : EntryMessage

    data object UpdateRequired : EntryMessage

    data object Unreachable : EntryMessage

    /** Otro error; [serverMessage] es el texto de Go, si lo hay. */
    data class Failed(val serverMessage: String?) : EntryMessage
}

/**
 * Pantalla de entrada (§3.2): la lista de meseros y el teclado del PIN del que se eligió. El mesero
 * elegido se guarda en [SavedStateHandle] (sobrevive a la rotación); **el PIN solo vive en
 * memoria**: nunca se escribe en el estado que Android guarda a disco al cerrar la aplicación. Cada
 * intento borra lo escrito.
 */
class PinEntryViewModel(
    private val access: DeviceAccess,
    private val savedState: SavedStateHandle,
) : ViewModel() {
    var roster: WaiterRoster? by mutableStateOf(null)
        private set

    var loading by mutableStateOf(false)
        private set

    var selectedId: String? by mutableStateOf(savedState[KEY_SELECTED])
        private set

    var pinLength by mutableStateOf(0)
        private set

    var busy by mutableStateOf(false)
        private set

    var message: EntryMessage? by mutableStateOf(null)
        private set

    // El PIN que se está escribiendo: solo en memoria.
    private val pin = StringBuilder()

    /** El mesero elegido (de la lista vigente). */
    val selected: WaiterProfile?
        get() = roster?.waiters?.firstOrNull { it.userId == selectedId }

    init {
        refresh()
    }

    /** Vuelve a pedir la lista (con conexión, la de Go; sin ella, los meseros conocidos). */
    fun refresh() {
        if (loading) {
            return
        }
        loading = true
        viewModelScope.launch {
            roster = access.roster()
            loading = false
        }
    }

    fun select(waiter: WaiterProfile) {
        selectedId = waiter.userId
        savedState[KEY_SELECTED] = waiter.userId
        clearPin()
        message = null
    }

    /** «Elegir otro nombre» o Atrás: de vuelta a la lista. */
    fun back() {
        selectedId = null
        savedState[KEY_SELECTED] = null
        clearPin()
        message = null
    }

    fun onDigit(digit: Char) {
        if (busy || pin.length >= AdelPinPadDefaults.MAX_LENGTH || digit !in '0'..'9') {
            return
        }
        pin.append(digit)
        pinLength = pin.length
        message = null
    }

    fun onDelete() {
        if (busy || pin.isEmpty()) {
            return
        }
        pin.deleteCharAt(pin.length - 1)
        pinLength = pin.length
    }

    /** «Entrar»: con conexión lo comprueba Go; sin ella, el verificador local. */
    fun submit() {
        val userId = selectedId ?: return
        if (busy || pin.length < AdelPinPadDefaults.MIN_LENGTH) {
            return
        }
        val typed = pin.toString()
        clearPin()
        busy = true
        viewModelScope.launch {
            val result = access.enter(userId, typed)
            busy = false
            message = messageFor(result)
            if (result is EntryResult.Entered) {
                selectedId = null
                savedState[KEY_SELECTED] = null
            } else if (result is EntryResult.Locked || result is EntryResult.UnknownOffline) {
                // La lista muestra el bloqueo o deja de mostrar a quien ya no puede entrar.
                refresh()
            }
        }
    }

    private fun clearPin() {
        pin.setLength(0)
        pinLength = 0
    }

    override fun onCleared() {
        clearPin()
    }

    private fun messageFor(result: EntryResult): EntryMessage? =
        when (result) {
            is EntryResult.Entered -> null
            EntryResult.WrongPin -> EntryMessage.WrongPin
            is EntryResult.Locked -> EntryMessage.Locked(result.untilEpochMillis)
            EntryResult.NotAllowed -> EntryMessage.NotAllowed
            EntryResult.UnknownOffline -> EntryMessage.UnknownOffline
            is EntryResult.Failed ->
                when (result.error) {
                    is AppError.UpdateRequired -> EntryMessage.UpdateRequired
                    AppError.Unreachable -> EntryMessage.Unreachable
                    else -> EntryMessage.Failed(result.error.serverMessage)
                }
        }

    companion object {
        private const val KEY_SELECTED = "pinEntry.selected"

        fun factory(access: DeviceAccess): ViewModelProvider.Factory = viewModelFactory {
            initializer { PinEntryViewModel(access, createSavedStateHandle()) }
        }
    }
}
