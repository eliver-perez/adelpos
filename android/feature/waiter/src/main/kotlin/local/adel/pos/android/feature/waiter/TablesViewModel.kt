package local.adel.pos.android.feature.waiter

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.createSavedStateHandle
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory

/**
 * Estado de la pantalla Mesas. Lo que el mesero captura (la búsqueda) y el panel abierto en el
 * teléfono se guardan en [SavedStateHandle]: sobreviven a la rotación (el ViewModel sigue vivo) y
 * también a que Android cierre el proceso en segundo plano (se restauran desde el estado guardado).
 * El texto vive además en un estado de Compose para que el campo se actualice en el mismo cuadro en
 * que se escribe.
 */
class TablesViewModel(private val savedState: SavedStateHandle) : ViewModel() {
    var search: String by mutableStateOf(savedState[KEY_SEARCH] ?: "")
        private set

    var showingAccount: Boolean by mutableStateOf(savedState[KEY_SHOWING_ACCOUNT] ?: false)
        private set

    fun onSearchChange(text: String) {
        search = text
        savedState[KEY_SEARCH] = text
    }

    fun onShowingAccountChange(showing: Boolean) {
        showingAccount = showing
        savedState[KEY_SHOWING_ACCOUNT] = showing
    }

    companion object {
        const val KEY_SEARCH = "tables.search"
        const val KEY_SHOWING_ACCOUNT = "tables.showingAccount"

        /**
         * Fábrica sin inyección automática (sin Hilt): la pantalla aún no necesita nada del
         * AppContainer.
         */
        val Factory = viewModelFactory { initializer { TablesViewModel(createSavedStateHandle()) } }
    }
}
