package com.adelpos.meseros.feature.access.findserver

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
import com.adelpos.meseros.core.domain.pairing.FoundServerChoice
import com.adelpos.meseros.core.domain.pairing.PairingFlow
import com.adelpos.meseros.core.domain.pairing.PairingProblem
import com.adelpos.meseros.core.domain.pairing.PairingState
import com.adelpos.meseros.core.domain.pairing.PairingStep
import com.adelpos.meseros.core.domain.pairing.PairingTarget
import com.adelpos.meseros.core.domain.pairing.PairingView
import com.adelpos.meseros.core.domain.pairing.ServerSearch
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** Resultado de la búsqueda en la red tal como lo muestra la pantalla. */
sealed interface SearchUi {
    data object Searching : SearchUi

    data class Done(val servers: List<FoundServerChoice>) : SearchUi

    /**
     * No se pudo buscar (permiso de red local o NSD no disponible): queda escribir la dirección.
     */
    data object Unavailable : SearchUi
}

/**
 * «Buscar el servidor» y la pantalla del código (§4): búsqueda automática al abrir, «Buscar otra
 * vez», dirección escrita a mano, código de 6 dígitos con su espera (consulta cada 2 s, como el
 * escritorio), cambio de nombre, cancelación y «Pedir otro código».
 *
 * Lo que escribe el usuario (nombre del dispositivo y dirección) se guarda en [SavedStateHandle]:
 * sobrevive a la rotación y al cierre del proceso. El código en pantalla no: si Android cierra la
 * aplicación, la solicitud se pierde y se pide otra.
 */
class FindServerViewModel(
    private val pairing: PairingFlow,
    private val savedState: SavedStateHandle,
    private val pollMillis: Long = POLL_MILLIS,
) : ViewModel() {
    var deviceName: String by mutableStateOf(savedState[KEY_NAME] ?: "")
        private set

    var address: String by mutableStateOf(savedState[KEY_ADDRESS] ?: "")
        private set

    var search: SearchUi by mutableStateOf(SearchUi.Searching)
        private set

    var busy by mutableStateOf(false)
        private set

    /** Por qué no se pudo pedir el código o consultar la solicitud. */
    var problem: PairingProblem? by mutableStateOf(null)
        private set

    /** El código en pantalla, o null mientras se busca el servidor. */
    var code: PairingView? by mutableStateOf(null)
        private set

    private var polling: Job? = null

    // Lo último que se pidió, para «Volver a comprobar» después de «Actualiza el servidor».
    private var lastTarget: PairingTarget? = null

    init {
        searchAgain()
    }

    /** Nombre propuesto la primera vez («Tableta 1» o «Teléfono 1», según el dispositivo). */
    fun proposeName(name: String) {
        if (deviceName.isEmpty() && savedState.get<String>(KEY_NAME) == null) {
            onNameChange(name)
        }
    }

    fun onNameChange(name: String) {
        deviceName = name
        savedState[KEY_NAME] = name
    }

    fun onAddressChange(text: String) {
        address = text
        savedState[KEY_ADDRESS] = text
    }

    fun searchAgain() {
        search = SearchUi.Searching
        viewModelScope.launch {
            search =
                when (val result = pairing.search()) {
                    is ServerSearch.Found -> SearchUi.Done(result.servers)
                    ServerSearch.Unavailable -> SearchUi.Unavailable
                }
        }
    }

    /** Pide el código al servidor elegido de la lista. */
    fun requestFound(server: FoundServerChoice) {
        request(PairingTarget.Found(server.serverId))
    }

    /** Pide el código a la dirección escrita. */
    fun requestAddress() {
        request(PairingTarget.Address(address))
    }

    /**
     * «Volver a comprobar» tras «Actualiza el servidor del restaurante»: pide otra vez el código al
     * mismo servidor (ya actualizado, sigue el emparejamiento).
     */
    fun checkAgain() {
        lastTarget?.let(::request)
    }

    private fun request(target: PairingTarget) {
        if (busy) {
            return
        }
        lastTarget = target
        busy = true
        problem = null
        viewModelScope.launch {
            show(pairing.start(target, deviceName))
            busy = false
        }
    }

    fun rename(name: String) {
        viewModelScope.launch {
            when (val step = pairing.rename(name)) {
                is PairingStep.Showing -> {
                    code = step.view
                    onNameChange(step.view.deviceName)
                    problem = null
                }
                is PairingStep.Failed -> problem = step.problem
            }
        }
    }

    /** «Cancelar» mientras espera la aprobación. */
    fun cancel() {
        polling?.cancel()
        code = null
        problem = null
        viewModelScope.launch { pairing.cancel() }
        searchAgain()
    }

    /** «Pedir otro código»: de vuelta a la búsqueda. */
    fun startOver() {
        polling?.cancel()
        code = null
        problem = null
        searchAgain()
    }

    private fun show(step: PairingStep) {
        when (step) {
            is PairingStep.Showing -> {
                code = step.view
                if (step.view.state == PairingState.PENDING) {
                    startPolling()
                }
            }
            is PairingStep.Failed -> problem = step.problem
        }
    }

    // Una consulta a la vez: la siguiente se programa cuando termina la anterior. Un error no
    // detiene la espera (puede ser un corte breve de la red); se muestra y se sigue consultando.
    private fun startPolling() {
        polling?.cancel()
        polling = viewModelScope.launch {
            while (code?.state == PairingState.PENDING) {
                delay(pollMillis)
                when (val step = pairing.poll()) {
                    is PairingStep.Showing -> {
                        code = step.view
                        problem = null
                    }
                    is PairingStep.Failed -> {
                        problem = step.problem
                        if (step.problem == PairingProblem.NoSession) {
                            code = null
                        }
                    }
                }
            }
        }
    }

    companion object {
        const val POLL_MILLIS = 2_000L
        private const val KEY_NAME = "findServer.deviceName"
        private const val KEY_ADDRESS = "findServer.address"

        fun factory(pairing: PairingFlow): ViewModelProvider.Factory = viewModelFactory {
            initializer { FindServerViewModel(pairing, createSavedStateHandle()) }
        }
    }
}
