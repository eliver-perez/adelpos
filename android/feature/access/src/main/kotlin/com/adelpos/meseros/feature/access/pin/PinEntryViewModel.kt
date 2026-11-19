package com.adelpos.meseros.feature.access.pin

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
import com.adelpos.meseros.core.domain.access.DeviceAccess
import com.adelpos.meseros.core.domain.access.EntryResult
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.access.WaiterRoster
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.ui.components.AdelPinPadDefaults
import kotlinx.coroutines.launch

/** Mensaje de la pantalla de entrada después de escribir el PIN. */
sealed interface EntryMessage {
    data object WrongPin : EntryMessage

    /**
     * Bloqueado en Go hasta [untilEpochMillis]; con [requiresUnlock] (o sin hora), hasta que un
     * administrador lo desbloquee (A1-D16).
     */
    data class Locked(val untilEpochMillis: Long?, val requiresUnlock: Boolean = false) :
        EntryMessage

    /** Entrada sin conexión cerrada para ese mesero: tiene que entrar con conexión. */
    data object OfflineClosed : EntryMessage

    /** Go está ocupado verificando otras entradas: intentarlo de nuevo en unos segundos. */
    data object Busy : EntryMessage

    /** Un problema de configuración que resuelve soporte ([code] de Go, para dictarlo). */
    data class SupportProblem(val code: String) : EntryMessage

    data object NotAllowed : EntryMessage

    data object UnknownOffline : EntryMessage

    data object UpdateRequired : EntryMessage

    data object Unreachable : EntryMessage

    /** Otro error; [serverMessage] es el texto de Go, si lo hay. */
    data class Failed(val serverMessage: String?) : EntryMessage
}

// 400 de Go cuando el tipo de cliente no coincide con el del dispositivo (no debería pasar).
private const val CLIENT_KIND_MISMATCH = "CLIENT_KIND_MISMATCH"

/** El mensaje que corresponde a un intento de entrada (null si entró). */
internal fun entryMessageFor(result: EntryResult): EntryMessage? =
    when (result) {
        is EntryResult.Entered -> null
        EntryResult.WrongPin -> EntryMessage.WrongPin
        is EntryResult.Locked -> EntryMessage.Locked(result.untilEpochMillis, result.requiresUnlock)
        EntryResult.OfflineClosed -> EntryMessage.OfflineClosed
        EntryResult.Busy -> EntryMessage.Busy
        EntryResult.NotAllowed -> EntryMessage.NotAllowed
        EntryResult.UnknownOffline -> EntryMessage.UnknownOffline
        is EntryResult.Failed ->
            when (result.error) {
                is AppError.UpdateRequired -> EntryMessage.UpdateRequired
                AppError.Unreachable -> EntryMessage.Unreachable
                else ->
                    if (result.error.code == CLIENT_KIND_MISMATCH) {
                        EntryMessage.SupportProblem(CLIENT_KIND_MISMATCH)
                    } else {
                        EntryMessage.Failed(result.error.serverMessage)
                    }
            }
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
            message = entryMessageFor(result)
            if (result is EntryResult.Entered) {
                selectedId = null
                savedState[KEY_SELECTED] = null
            } else if (
                result is EntryResult.Locked ||
                    result is EntryResult.UnknownOffline ||
                    result is EntryResult.OfflineClosed
            ) {
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

    companion object {
        private const val KEY_SELECTED = "pinEntry.selected"

        fun factory(access: DeviceAccess): ViewModelProvider.Factory = viewModelFactory {
            initializer { PinEntryViewModel(access, createSavedStateHandle()) }
        }
    }
}
