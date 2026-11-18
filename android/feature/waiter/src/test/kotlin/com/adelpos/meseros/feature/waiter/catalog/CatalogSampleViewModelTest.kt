package com.adelpos.meseros.feature.waiter.catalog

import androidx.lifecycle.SavedStateHandle
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogCategory
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogQuery
import com.adelpos.meseros.core.domain.catalog.CatalogSearch
import com.adelpos.meseros.core.domain.catalog.CatalogSource
import com.adelpos.meseros.core.domain.catalog.CatalogSyncStatus
import com.adelpos.meseros.core.domain.catalog.CatalogVersion
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.ProductAvailability
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.money.Money
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** ViewModel de la pantalla de muestra «Catálogo»: búsqueda, categoría y personalizador. */
@OptIn(ExperimentalCoroutinesApi::class)
class CatalogSampleViewModelTest {
    private fun product(id: String, name: String, category: String, soldOut: Boolean = false) =
        CatalogProduct(
            id = id,
            name = name,
            description = "",
            active = true,
            categoryId = category,
            code = id,
            barcode = null,
            kind = ProductKind.PRODUCT,
            referencePrice = Money("10.00"),
            pricingError = null,
            imageId = null,
            sortOrder = 0,
            variants = emptyList(),
            ingredients = emptyList(),
            groups = emptyList(),
            slots = emptyList(),
            availability =
                if (soldOut) {
                    ProductAvailability.AVAILABLE.copy(availableNow = false, soldOut = true)
                } else {
                    ProductAvailability.AVAILABLE
                },
        )

    private val catalog =
        Catalog(
            version = CatalogVersion(3, "e", "", "America/Monterrey", 16, "General", 1),
            categories =
                listOf(
                    CatalogCategory("c1", "Tacos", "", true, 1, null, true),
                    CatalogCategory("c2", "Bebidas", "", true, 2, null, true),
                ),
            products =
                listOf(
                    product("p1", "Taco de pastor", "c1"),
                    product("p2", "Café", "c2"),
                    product("p3", "Agua", "c2", soldOut = true),
                ),
            groups = emptyList(),
            quickNotes = emptyList(),
        )

    private class FakeSource(initial: Catalog) : CatalogSource {
        override val catalog = MutableStateFlow(initial)
        override val status: StateFlow<CatalogSyncStatus> = MutableStateFlow(CatalogSyncStatus())
        var refreshes = 0

        override fun requestRefresh() {
            refreshes += 1
        }

        override suspend fun search(query: CatalogQuery): List<CatalogProduct> {
            val current = catalog.value
            if (!query.searching) {
                return current.productsIn(query.categoryId ?: return current.products)
            }
            val tokens = CatalogSearch.tokens(query.text)
            return current.products.filter {
                CatalogSearch.matches(CatalogSearch.searchKey(it.name, it.code, null), tokens)
            }
        }
    }

    @Before
    fun setUp() {
        Dispatchers.setMain(UnconfinedTestDispatcher())
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun categoriaBusquedaYPersonalizador() =
        runTest(UnconfinedTestDispatcher()) {
            val saved = SavedStateHandle()
            val model = CatalogSampleViewModel(FakeSource(catalog), saved)
            backgroundScope.launch { model.products.collect {} }
            backgroundScope.launch { model.selectedCategoryId.collect {} }
            // Sin elegir, la primera categoría.
            assertEquals("c1", model.selectedCategoryId.value)
            assertEquals(listOf("Taco de pastor"), model.products.value.map { it.name })

            model.onCategorySelect("c2")
            assertEquals(listOf("Agua", "Café"), model.products.value.map { it.name })
            model.onSearchTextChange("cafe")
            assertEquals(listOf("Café"), model.products.value.map { it.name })
            assertEquals("cafe", saved[CatalogSampleViewModel.KEY_SEARCH])

            // Un agotado no se puede elegir.
            model.onProductSelect(catalog.product("p3")!!)
            assertNull(model.editing)
            model.onProductSelect(catalog.product("p2")!!)
            assertEquals("p2", model.editing?.productId)
            assertTrue(model.showingDetail)
            model.onLineChange(model.editing!!.copy(quantity = 3))
            model.onAdd(model.editing!!)
            assertEquals(listOf(3), model.added.map { it.quantity })
            assertNull(model.editing)
            assertEquals(Customization.ORIGINAL, model.added.single().customization)

            // Lo que se estaba personalizando vuelve tras cerrar el proceso (con su configuración
            // original).
            model.onProductSelect(catalog.product("p1")!!)
            val restored = CatalogSampleViewModel(FakeSource(catalog), saved)
            assertEquals("p1", restored.editing?.productId)
            assertEquals("cafe", restored.searchText)
        }
}
