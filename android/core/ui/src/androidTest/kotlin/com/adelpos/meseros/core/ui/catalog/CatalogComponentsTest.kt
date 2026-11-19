package com.adelpos.meseros.core.ui.catalog

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.GroupSelection
import com.adelpos.meseros.core.domain.catalog.IngredientSelection
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.catalog.OptionQuantity
import com.adelpos.meseros.core.domain.catalog.QuickNoteSelection
import com.adelpos.meseros.core.ui.theme.AdelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Componentes del catálogo para la captura en el dispositivo (teléfono y tableta según el
 * emulador): cuadrícula con «Agotado» que no se puede elegir, búsqueda, categorías y el
 * personalizador con −/+, grupos, presentaciones, indicaciones y observaciones, en ambos temas.
 */
@RunWith(AndroidJUnit4::class)
class CatalogComponentsTest {
    @get:Rule val compose = createComposeRule()

    private val catalog = CatalogUiFixtures.catalog

    @Test
    fun laCuadriculaMarcaLosAgotadosYNoDejaElegirlos() {
        var chosen: CatalogProduct? = null
        var search = ""
        var category: String? = null
        compose.setContent {
            AdelTheme {
                CatalogBrowser(
                    categories = catalog.categories,
                    selectedCategoryId = CatalogUiFixtures.DRINKS,
                    searchText = "",
                    products = catalog.productsIn(CatalogUiFixtures.DRINKS),
                    images = null,
                    onSearchTextChange = { search = it },
                    onCategorySelect = { category = it },
                    onProductSelect = { chosen = it },
                )
            }
        }
        compose
            .onNodeWithTag(productTag(CatalogUiFixtures.SHAKE))
            .assertIsNotEnabled()
            .assertTextContains("Agotado hasta nuevo aviso", substring = true)
        compose
            .onNodeWithTag(productTag(CatalogUiFixtures.COFFEE))
            .assertIsNotEnabled()
            .assertTextContains("Agotado", substring = true)
            .performClick()
        assertNull("Un agotado no se puede elegir", chosen)

        // Precio de referencia o «Elige presentación» si no tiene precio base.
        compose
            .onNodeWithTag(productTag(CatalogUiFixtures.SODA))
            .assertIsEnabled()
            .assertTextContains("Elige presentación", substring = true)
            .assertHeightIsAtLeast(56.dp)
            .performClick()
        assertEquals(CatalogUiFixtures.SODA, chosen?.id)
        // Sin imagen, tarjeta compacta de nombre y precio, sin la letra grande (etapa A3, §4.11).
        compose.onNodeWithTag(productTag(CatalogUiFixtures.SODA)).assertIsDisplayed()
        compose
            .onAllNodes(hasTestTag(TAG_PRODUCT_WITHOUT_IMAGE), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .let { assertTrue("las tarjetas sin imagen son compactas", it.isNotEmpty()) }
        compose
            .onAllNodes(hasTestTag(TAG_PRODUCT_IMAGE_PLACEHOLDER), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .let { assertTrue("sin la letra grande", it.isEmpty()) }

        compose.onNodeWithTag(categoryTag(CatalogUiFixtures.DRINKS)).assertIsSelected()
        compose
            .onNodeWithTag(categoryTag(CatalogUiFixtures.BURGERS))
            .assertHeightIsAtLeast(48.dp)
            .performClick()
        assertEquals(CatalogUiFixtures.BURGERS, category)
        compose.onNodeWithTag(TAG_CATALOG_SEARCH).performTextInput("cafe")
        assertEquals("cafe", search)
    }

    /**
     * Con poca altura (el teléfono en horizontal; Fase E de la A2), la búsqueda y las categorías
     * van en una fila y las tarjetas son bajas: en 220 dp de alto se ve una tarjeta completa, con
     * su nombre y su precio, y se puede elegir.
     */
    @Test
    fun conPocaAlturaLasTarjetasCabenConSuNombre() {
        var chosen: CatalogProduct? = null
        val first = catalog.productsIn(CatalogUiFixtures.BURGERS).first()
        compose.setContent {
            AdelTheme {
                Box(modifier = Modifier.width(800.dp).height(220.dp)) {
                    CatalogBrowser(
                        categories = catalog.categories,
                        selectedCategoryId = CatalogUiFixtures.BURGERS,
                        searchText = "",
                        products = catalog.productsIn(CatalogUiFixtures.BURGERS),
                        images = null,
                        onSearchTextChange = {},
                        onCategorySelect = {},
                        onProductSelect = { chosen = it },
                        compactHeight = true,
                    )
                }
            }
        }
        val search = compose.onNodeWithTag(TAG_CATALOG_SEARCH).fetchSemanticsNode().boundsInRoot
        val categories = compose.onNodeWithTag(TAG_CATEGORY_BAR).fetchSemanticsNode().boundsInRoot
        assertTrue("la búsqueda y las categorías en una fila", categories.left >= search.right)
        // La primera fila completa, con el nombre a la vista (la siguiente, desplazando).
        compose
            .onNodeWithTag(productTag(first.id))
            .assertIsDisplayed()
            .assertTextContains(first.name, substring = true)
            .assertHeightIsAtLeast(48.dp)
            .performClick()
        val card = compose.onNodeWithTag(productTag(first.id)).fetchSemanticsNode()
        val grid = compose.onNodeWithTag(TAG_PRODUCT_GRID).fetchSemanticsNode().boundsInRoot
        assertTrue("la tarjeta completa a la vista", card.boundsInRoot.bottom <= grid.bottom)
        assertEquals(first.id, chosen?.id)
    }

    @Test
    fun busquedaSinResultadosYBorrarLaBusqueda() {
        compose.setContent {
            var text by remember { mutableStateOf("pozole") }
            AdelTheme {
                CatalogBrowser(
                    categories = catalog.categories,
                    selectedCategoryId = CatalogUiFixtures.BURGERS,
                    searchText = text,
                    products = emptyList(),
                    images = null,
                    onSearchTextChange = { text = it },
                    onCategorySelect = {},
                    onProductSelect = {},
                    emptyMessage = if (text.isBlank()) null else "No hay productos con «$text».",
                )
            }
        }
        compose.onNodeWithTag(TAG_CATALOG_EMPTY).assertTextEquals("No hay productos con «pozole».")
        compose.onNodeWithTag(TAG_CATALOG_SEARCH_CLEAR).performClick()
        compose.onNodeWithTag(TAG_PRODUCT_GRID).assertIsDisplayed()
    }

    @Test
    fun personalizadorConPorcionesGruposIndicacionesYObservaciones() {
        var added: LineSelection? = null
        compose.setContent {
            var line by remember {
                mutableStateOf(
                    LineSelection(CatalogUiFixtures.BURGER, null, 1, Customization.ORIGINAL),
                )
            }
            AdelTheme {
                ProductCustomizer(
                    catalog = catalog,
                    line = line,
                    onLineChange = { line = it },
                    onAdd = { added = it },
                    onCancel = {},
                    images = null,
                )
            }
        }
        compose.onNodeWithTag(TAG_CUSTOMIZER_TITLE).assertTextEquals("Hamburguesa clásica")
        val onion = ingredientTag(CatalogUiFixtures.ONION)
        compose.onNodeWithTag("$onion-value", useUnmergedTree = true).assertTextEquals("Normal")
        compose.onNodeWithTag("$onion-less").assertWidthIsAtLeast(48.dp).performClick()
        compose.onNodeWithTag("$onion-value", useUnmergedTree = true).assertTextEquals("Sin")
        compose.onNodeWithTag("$onion-less").assertIsNotEnabled()
        compose.onNodeWithTag("$onion-more").performClick()
        compose.onNodeWithTag("$onion-more").performClick()
        compose.onNodeWithTag("$onion-value", useUnmergedTree = true).assertTextEquals("Extra")

        // Guarnición de elección única y salsas con −/+ hasta su máximo (con aviso).
        compose.onNodeWithTag(optionTag(CatalogUiFixtures.SALAD)).performScrollTo().performClick()
        val ketchup = optionTag(CatalogUiFixtures.KETCHUP)
        compose.onNodeWithTag("$ketchup-more").performScrollTo().performClick()
        compose.onNodeWithTag("$ketchup-more").performClick()
        compose.onNodeWithTag(optionTag("o-mostaza") + "-more").performScrollTo().performClick()
        compose
            .onNodeWithTag(TAG_GROUP_WARNING)
            .assertTextContains("Salsas admite hasta 2 porciones", substring = true)

        // «Sin cebolla» (indicación con efecto) deja la cebolla en 0 y su texto en las notas.
        compose
            .onNodeWithTag(quickNoteTag(CatalogUiFixtures.NO_ONION))
            .performScrollTo()
            .performClick()
        compose.onNodeWithTag("$onion-value", useUnmergedTree = true).assertTextEquals("Sin")

        // Observación con el teclado del sistema.
        compose
            .onNodeWithTag(TAG_OBSERVATION_FIELD)
            .performScrollTo()
            .performTextInput("salsa aparte")
        compose.onNodeWithTag(TAG_OBSERVATION_ADD).performScrollTo().performClick()
        compose.onNodeWithText("salsa aparte").assertIsDisplayed()

        compose.onNodeWithTag("$TAG_CUSTOMIZER_QUANTITY-more").performClick()
        compose
            .onNodeWithTag(TAG_CUSTOMIZER_ADD)
            .assertHeightIsAtLeast(56.dp)
            .assertTextEquals("Agregar 2")
            .performClick()

        assertEquals(
            LineSelection(
                productId = CatalogUiFixtures.BURGER,
                variantId = null,
                quantity = 2,
                customization =
                    Customization(
                        ingredients = listOf(IngredientSelection(CatalogUiFixtures.ONION, "0")),
                        groups =
                            listOf(
                                GroupSelection(
                                    CatalogUiFixtures.SIDES,
                                    listOf(OptionQuantity(CatalogUiFixtures.SALAD, "1")),
                                ),
                                GroupSelection(
                                    CatalogUiFixtures.SAUCES,
                                    listOf(OptionQuantity(CatalogUiFixtures.KETCHUP, "2")),
                                ),
                            ),
                        notes = "Sin cebolla, salsa aparte",
                        quickNotes =
                            listOf(QuickNoteSelection(CatalogUiFixtures.NO_ONION, "Sin cebolla")),
                    ),
            ),
            added,
        )
    }

    @Test
    fun presentacionObligatoriaYComboConComponenteAgotadoEnTemaOscuro() {
        var line by
            mutableStateOf(LineSelection(CatalogUiFixtures.SODA, null, 1, Customization.ORIGINAL))
        compose.setContent {
            AdelTheme(darkTheme = true) {
                ProductCustomizer(
                    catalog = catalog,
                    line = line,
                    onLineChange = { line = it },
                    onAdd = {},
                    onCancel = {},
                    images = null,
                )
            }
        }
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).assertIsNotEnabled()
        compose
            .onNodeWithTag(TAG_CUSTOMIZER_PROBLEMS)
            .assertTextContains("Elige la presentación de Refresco.", substring = true)
        compose
            .onNodeWithTag(variantTag(CatalogUiFixtures.SODA, CatalogUiFixtures.SODA_LARGE))
            .performClick()
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).assertIsEnabled()
        assertEquals(CatalogUiFixtures.SODA_LARGE, line.variantId)

        // El combo trae de origen un café que se agotó: no se puede agregar y la alternativa
        // agotada tampoco se elige.
        compose.runOnIdle {
            line = LineSelection(CatalogUiFixtures.COMBO, null, 1, Customization.ORIGINAL)
        }
        compose.onNodeWithTag(TAG_CUSTOMIZER_ADD).assertIsNotEnabled()
        compose
            .onNodeWithTag(TAG_CUSTOMIZER_PROBLEMS)
            .assertTextContains("Bebida del combo · Café de olla está agotado.", substring = true)
        compose
            .onNodeWithTag(slotProductTag(CatalogUiFixtures.SLOT_DRINK, CatalogUiFixtures.SHAKE))
            .assertIsNotEnabled()
        assertTrue(line.customization.isOriginal)
    }
}
