package com.adelpos.meseros

import androidx.activity.compose.setContent
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogCategory
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogQuery
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.catalog.CatalogSyncStatus
import com.adelpos.meseros.core.domain.catalog.CatalogVersion
import com.adelpos.meseros.core.domain.catalog.ProductAvailability
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER
import com.adelpos.meseros.core.ui.catalog.TAG_CUSTOMIZER_ADD
import com.adelpos.meseros.core.ui.catalog.productTag
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.currentPaneLayout
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.feature.waiter.catalog.CatalogSampleScreen
import com.adelpos.meseros.feature.waiter.catalog.CatalogSampleViewModel
import com.adelpos.meseros.feature.waiter.catalog.TAG_SAMPLE_LINE
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Pantalla de muestra «Catálogo» en el dispositivo: en la tableta la cuadrícula y el personalizador
 * van lado a lado y en el teléfono el personalizador se abre al elegir un producto; lo agregado
 * queda en la lista de muestra. (Se llega desde Mesas con un mesero dentro; esa navegación la
 * cubren las pruebas de extremo a extremo de la entrada.)
 */
@RunWith(AndroidJUnit4::class)
class CatalogSampleScreenTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private class FixedCatalog(value: Catalog) : CatalogSource {
        override val catalog: StateFlow<Catalog> = MutableStateFlow(value)
        override val status: StateFlow<CatalogSyncStatus> = MutableStateFlow(CatalogSyncStatus())

        override fun requestRefresh() = Unit

        override suspend fun search(query: CatalogQuery): List<CatalogProduct> =
            catalog.value.productsIn(query.categoryId ?: return catalog.value.products)
    }

    private val tacos =
        CatalogProduct(
            id = "p-taco",
            name = "Taco de pastor",
            description = "",
            active = true,
            categoryId = "c-tacos",
            code = "TAC",
            barcode = null,
            kind = ProductKind.PRODUCT,
            referencePrice = Money("22.00"),
            pricingError = null,
            imageId = null,
            sortOrder = 1,
            variants = emptyList(),
            ingredients = emptyList(),
            groups = emptyList(),
            slots = emptyList(),
            availability = ProductAvailability.AVAILABLE,
        )

    private val catalog =
        Catalog(
            version = CatalogVersion(1, "e", "", "America/Monterrey", 16, "General", 1),
            categories = listOf(CatalogCategory("c-tacos", "Tacos", "", true, 1, null, true)),
            products = listOf(tacos),
            groups = emptyList(),
            quickNotes = emptyList(),
        )

    @Test
    fun elegirPersonalizarYAgregarEnUnoODosPaneles() {
        val model = CatalogSampleViewModel(FixedCatalog(catalog), SavedStateHandle())
        var layout = PaneLayout.SINGLE
        compose.activity.runOnUiThread {
            compose.activity.setContent {
                layout = currentPaneLayout()
                val current by model.catalog.collectAsState()
                val products by model.products.collectAsState()
                val selected by model.selectedCategoryId.collectAsState()
                AdelTheme {
                    CatalogSampleScreen(
                        layout = layout,
                        catalog = current,
                        status = CatalogSyncStatus(),
                        products = products,
                        selectedCategoryId = selected,
                        searchText = model.searchText,
                        editing = model.editing,
                        added = model.added,
                        showingDetail = model.showingDetail,
                        images = null,
                        onSearchTextChange = model::onSearchTextChange,
                        onCategorySelect = model::onCategorySelect,
                        onProductSelect = model::onProductSelect,
                        onLineChange = model::onLineChange,
                        onAdd = model::onAdd,
                        onCancel = model::onCancel,
                        onShowingDetailChange = model::onShowingDetailChange,
                    )
                }
            }
        }
        compose.onNodeWithTag(productTag("p-taco")).performClick()
        compose.onNodeWithTag(TAG_CUSTOMIZER).assertIsDisplayed()
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).performClick()
        if (layout == PaneLayout.SINGLE) {
            // En el teléfono se vuelve al catálogo; lo agregado está a un toque.
            compose.onNodeWithTag(productTag("p-taco")).assertIsDisplayed()
            compose.runOnIdle { model.onShowingDetailChange(true) }
        }
        compose
            .onNodeWithTag(TAG_SAMPLE_LINE)
            .assertTextContains("1 × Taco de pastor", substring = true)
    }
}
