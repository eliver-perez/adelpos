package local.adel.pos.android.feature.access.pin

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import kotlinx.coroutines.launch
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.ui.components.AdelPinPadDefaults

/**
 * «Vuelve a escribir tu PIN» (revisión R3, M3): el acceso del mesero activo [userId] dejó de valer
 * (su sesión terminó o la cerró Go, le cambiaron el PIN o su credencial de envío ya no vale). Sigue
 * siendo el mesero activo, con su captura y sus pedidos sin enviar; al escribir bien su PIN vuelve
 * a entrar **como él mismo** (con conexión, con una sesión nueva; sin conexión, con su verificador
 * si todavía lo tiene). Como en la entrada, **el PIN solo vive en memoria** y cada intento borra lo
 * escrito.
 */
class ReenterPinViewModel(private val access: DeviceAccess, private val userId: String) :
    ViewModel() {
    var pinLength by mutableStateOf(0)
        private set

    var busy by mutableStateOf(false)
        private set

    var message: EntryMessage? by mutableStateOf(null)
        private set

    private val pin = StringBuilder()

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

    /** «Entrar»: el mismo camino que la entrada (Go con conexión; el verificador sin ella). */
    fun submit() {
        if (busy || pin.length < AdelPinPadDefaults.MIN_LENGTH) {
            return
        }
        val typed = pin.toString()
        clearPin()
        busy = true
        viewModelScope.launch {
            val result = access.enter(userId, typed)
            busy = false
            message = if (result is EntryResult.Entered) null else entryMessageFor(result)
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
        fun factory(access: DeviceAccess, userId: String): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { ReenterPinViewModel(access, userId) }
            }
    }
}
