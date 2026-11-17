package local.adel.pos.android.feature.waiter.catalog

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
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.CatalogQuery
import local.adel.pos.android.core.domain.catalog.CatalogSource
import local.adel.pos.android.core.domain.catalog.CatalogSyncStatus
import local.adel.pos.android.core.domain.catalog.Customization
import local.adel.pos.android.core.domain.catalog.LineSelection

/**
 * Pantalla de muestra «Catálogo» de la Fase D: los componentes del catálogo y el personalizador
 * sobre el catálogo guardado, sin cuenta ni envío (eso es de la Fase E, que usará los mismos
 * componentes). Lo que se agrega queda en una lista de muestra para ver la selección que se
 * enviaría a Go.
 *
 * La búsqueda, la categoría y el producto que se personaliza se guardan en [SavedStateHandle]
 * (sobreviven a la rotación y al cierre del proceso); la personalización en curso vive en el
 * ViewModel (sobrevive a la rotación).
 */
class CatalogSampleViewModel(
    private val source: CatalogSource,
    private val savedState: SavedStateHandle,
) : ViewModel() {
    private val query =
        MutableStateFlow(
            CatalogQuery(
                text = savedState[KEY_SEARCH] ?: "",
                categoryId = savedState[KEY_CATEGORY],
            ),
        )

    val catalog: StateFlow<Catalog> = source.catalog

    val status: StateFlow<CatalogSyncStatus> = source.status

    /** Productos que se ven: los de la categoría elegida o los resultados de la búsqueda. */
    val products: StateFlow<List<CatalogProduct>> =
        combine(source.catalog, query) { catalog, current ->
                source.search(current.copy(categoryId = categoryFor(catalog, current.categoryId)))
            }
            .stateIn(
                viewModelScope,
                SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
                emptyList(),
            )

    /** Categoría elegida (o la primera del catálogo si no se ha elegido ninguna). */
    val selectedCategoryId: StateFlow<String?> =
        combine(source.catalog, query) { catalog, current ->
                categoryFor(catalog, current.categoryId)
            }
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), null)

    var searchText: String by mutableStateOf(query.value.text)
        private set

    /** Partida que se está personalizando, o null. */
    var editing: LineSelection? by mutableStateOf(restoreEditing())
        private set

    /** Lo agregado en esta pantalla de muestra. */
    var added: List<LineSelection> by mutableStateOf(emptyList())
        private set

    /** En el teléfono: si se ve el panel de la partida (personalizador o lista de muestra). */
    var showingDetail: Boolean by mutableStateOf(savedState[KEY_SHOWING_DETAIL] ?: false)
        private set

    fun onSearchTextChange(text: String) {
        searchText = text
        savedState[KEY_SEARCH] = text
        query.value = query.value.copy(text = text)
    }

    fun onCategorySelect(categoryId: String) {
        savedState[KEY_CATEGORY] = categoryId
        // Elegir una categoría deja la búsqueda, como en el escritorio.
        onSearchTextChange("")
        query.value = query.value.copy(categoryId = categoryId)
    }

    /** Toque en un producto disponible: abre el personalizador con su configuración original. */
    fun onProductSelect(product: CatalogProduct) {
        if (!product.selectable) {
            return
        }
        val line = LineSelection(product.id, null, 1, Customization.ORIGINAL)
        editing = line
        savedState[KEY_EDITING_PRODUCT] = product.id
        onShowingDetailChange(true)
    }

    fun onLineChange(line: LineSelection) {
        editing = line
    }

    fun onAdd(line: LineSelection) {
        added = added + line
        closeEditor()
    }

    fun onCancel() {
        closeEditor()
    }

    fun onShowingDetailChange(showing: Boolean) {
        showingDetail = showing
        savedState[KEY_SHOWING_DETAIL] = showing
    }

    private fun closeEditor() {
        editing = null
        savedState[KEY_EDITING_PRODUCT] = null
        onShowingDetailChange(false)
    }

    private fun restoreEditing(): LineSelection? {
        val productId: String = savedState[KEY_EDITING_PRODUCT] ?: return null
        return LineSelection(productId, null, 1, Customization.ORIGINAL)
    }

    private fun categoryFor(catalog: Catalog, categoryId: String?): String? {
        if (categoryId != null && catalog.categories.any { it.id == categoryId }) {
            return categoryId
        }
        return catalog.categories.firstOrNull()?.id
    }

    companion object {
        const val KEY_SEARCH = "catalog.search"
        const val KEY_CATEGORY = "catalog.category"
        const val KEY_EDITING_PRODUCT = "catalog.editingProduct"
        const val KEY_SHOWING_DETAIL = "catalog.showingDetail"

        private const val STOP_TIMEOUT_MILLIS = 5_000L

        /** Fábrica con el catálogo del AppContainer (inyección manual). */
        fun factory(source: CatalogSource): ViewModelProvider.Factory = viewModelFactory {
            initializer { CatalogSampleViewModel(source, createSavedStateHandle()) }
        }
    }
}
