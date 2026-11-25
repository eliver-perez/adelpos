package com.adelpos.meseros.core.ui.catalog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.core.ui.theme.AmountTextStyle
import com.adelpos.meseros.core.domain.catalog.AvailabilityStatus
import com.adelpos.meseros.core.domain.catalog.CatalogCategory
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.icons.CatalogIcons
import com.adelpos.meseros.core.ui.layout.currentHeightCompact

/**
 * Catálogo para capturar (modo Mesero y, en la A2, Para llevar): búsqueda arriba, categorías
 * siempre a la mano y la cuadrícula de productos con su precio de referencia, imagen y marca
 * «Agotado». Sin estado propio: quien la usa guarda la búsqueda y la categoría (en su ViewModel,
 * para que sobrevivan a la rotación).
 *
 * @param searchText texto de la búsqueda; con texto, [products] son los resultados de todo el
 *   catálogo y ninguna categoría aparece elegida.
 * @param emptyMessage texto cuando no hay productos que mostrar (catálogo sin descargar, categoría
 *   vacía o búsqueda sin resultados).
 * @param compactHeight con poca altura (el teléfono en horizontal, [currentHeightCompact]): la
 *   búsqueda y las categorías van en una sola fila y las tarjetas son bajas (imagen a la izquierda,
 *   nombre y precio a la derecha), para que la cuadrícula muestre productos con su nombre (Fase E
 *   de la A2: con la forma normal no cabía ninguna tarjeta completa).
 */
@Composable
fun CatalogBrowser(
    categories: List<CatalogCategory>,
    selectedCategoryId: String?,
    searchText: String,
    products: List<CatalogProduct>,
    images: CatalogImages?,
    onSearchTextChange: (String) -> Unit,
    onCategorySelect: (String) -> Unit,
    onProductSelect: (CatalogProduct) -> Unit,
    modifier: Modifier = Modifier,
    emptyMessage: String? = null,
    compactHeight: Boolean = currentHeightCompact(),
) {
    val searching = searchText.isNotBlank()
    Column(
        modifier = modifier.fillMaxSize().testTag(TAG_CATALOG_BROWSER),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (compactHeight) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = AdelDimens.ScreenPadding),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                CatalogSearchField(
                    text = searchText,
                    onTextChange = onSearchTextChange,
                    modifier = Modifier.weight(1f).padding(start = AdelDimens.ScreenPadding),
                )
                CategoryBar(
                    categories = categories,
                    selectedId = if (searching) null else selectedCategoryId,
                    onSelect = onCategorySelect,
                    modifier = Modifier.weight(1f),
                )
            }
        } else {
            CatalogSearchField(
                text = searchText,
                onTextChange = onSearchTextChange,
                modifier =
                    Modifier.fillMaxWidth()
                        .padding(horizontal = AdelDimens.ScreenPadding)
                        .padding(top = AdelDimens.ScreenPadding),
            )
            CategoryBar(
                categories = categories,
                selectedId = if (searching) null else selectedCategoryId,
                onSelect = onCategorySelect,
            )
        }
        if (products.isEmpty() && emptyMessage != null) {
            Text(
                text = emptyMessage,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(AdelDimens.ScreenPadding).testTag(TAG_CATALOG_EMPTY),
            )
        } else {
            ProductGrid(
                products = products,
                images = images,
                onProductSelect = onProductSelect,
                modifier = Modifier.weight(1f),
                compact = compactHeight,
            )
        }
    }
}

/**
 * Búsqueda de productos por nombre o código, con el teclado del sistema. Etapa A3 (§4.3): una sola
 * línea con «Buscar producto» en todas las formas; si la etiqueta se partiera, el campo crecería y
 * le quitaría altura a la cuadrícula.
 */
@Composable
fun CatalogSearchField(
    text: String,
    onTextChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    OutlinedTextField(
        value = text,
        onValueChange = onTextChange,
        label = {
            Text(
                stringResource(R.string.catalog_search_label),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        },
        leadingIcon = { Icon(CatalogIcons.Search, contentDescription = null) },
        trailingIcon = {
            if (text.isNotEmpty()) {
                IconButton(
                    onClick = { onTextChange("") },
                    modifier = Modifier.testTag(TAG_CATALOG_SEARCH_CLEAR),
                ) {
                    Icon(
                        CatalogIcons.Close,
                        contentDescription = stringResource(R.string.catalog_search_clear),
                    )
                }
            }
        },
        singleLine = true,
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
        modifier = modifier.testTag(TAG_CATALOG_SEARCH),
    )
}

/** Categorías siempre a la mano: una fila que se desplaza, con la elegida marcada. */
@Composable
fun CategoryBar(
    categories: List<CatalogCategory>,
    selectedId: String?,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val label = stringResource(R.string.catalog_categories)
    LazyRow(
        modifier =
            modifier.fillMaxWidth().testTag(TAG_CATEGORY_BAR).semantics {
                contentDescription = label
            },
        contentPadding = PaddingValues(horizontal = AdelDimens.ScreenPadding),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(categories, key = { it.id }) { category ->
            FilterChip(
                selected = category.id == selectedId,
                onClick = { onSelect(category.id) },
                label = {
                    Text(category.name, style = MaterialTheme.typography.labelLarge, maxLines = 1)
                },
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag(categoryTag(category.id)),
            )
        }
    }
}

/**
 * Cuadrícula de productos: tantas columnas como quepan (teléfono 2, tableta 4–6); con [compact]
 * (poca altura), tarjetas bajas y más anchas.
 */
@Composable
fun ProductGrid(
    products: List<CatalogProduct>,
    images: CatalogImages?,
    onProductSelect: (CatalogProduct) -> Unit,
    modifier: Modifier = Modifier,
    compact: Boolean = false,
) {
    LazyVerticalGrid(
        columns = GridCells.Adaptive(minSize = if (compact) 240.dp else 152.dp),
        modifier = modifier.fillMaxWidth().testTag(TAG_PRODUCT_GRID),
        contentPadding = PaddingValues(AdelDimens.ScreenPadding),
        horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        items(products, key = { it.id }) { product ->
            ProductCard(
                product = product,
                images = images,
                onClick = { onProductSelect(product) },
                compact = compact,
            )
        }
    }
}

/**
 * Tarjeta de un producto: imagen (o marcador), nombre, precio de referencia y su marca. Un producto
 * agotado o no disponible se ve atenuado con «Agotado», «Agotado hasta nuevo aviso» o el motivo de
 * Go, y no se puede elegir. Con [compact] (poca altura), la imagen va a la izquierda, pequeña, y el
 * texto a la derecha: la tarjeta mide lo de una fila de texto, no lo de la imagen.
 *
 * Etapa A3 (§4.11): un producto **sin imagen** es una tarjeta compacta de nombre y precio, sin el
 * marcador con la letra grande, en el teléfono y en la tableta (la imagen solo ocupa lugar cuando
 * hay una que mostrar).
 */
@Composable
fun ProductCard(
    product: CatalogProduct,
    images: CatalogImages?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    compact: Boolean = false,
) {
    val price = priceText(product)
    val status = statusText(product)
    val description =
        stringResource(R.string.product_card_description, product.name, status ?: price)
    Card(
        onClick = onClick,
        enabled = product.selectable,
        colors =
            CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                disabledContainerColor = MaterialTheme.colorScheme.surfaceContainerLow,
            ),
        modifier =
            modifier
                .fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .testTag(productTag(product.id))
                .semantics(mergeDescendants = true) { contentDescription = description },
    ) {
        if (product.imageId == null) {
            ProductText(product, price, status, Modifier.testTag(TAG_PRODUCT_WITHOUT_IMAGE))
        } else if (compact) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                ProductImage(
                    imageId = product.imageId,
                    name = product.name,
                    images = images,
                    modifier = Modifier.size(COMPACT_IMAGE_SIZE),
                )
                ProductText(product, price, status, Modifier.weight(1f))
            }
        } else {
            ProductImage(
                imageId = product.imageId,
                name = product.name,
                images = images,
                modifier = Modifier.fillMaxWidth().aspectRatio(4f / 3f),
            )
            ProductText(product, price, status, Modifier)
        }
    }
}

/** Nombre, precio de referencia y marca de disponibilidad (o «Combo») de una tarjeta. */
@Composable
private fun ProductText(
    product: CatalogProduct,
    price: String,
    status: String?,
    modifier: Modifier,
) {
    Column(
        modifier = modifier.padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = product.name,
            style = MaterialTheme.typography.titleMedium,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Text(text = price, style = AmountTextStyle)
        if (status != null) {
            Text(
                text = status,
                style = MaterialTheme.typography.labelLarge,
                color = AdelTheme.colors.red,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.testTag(TAG_PRODUCT_STATUS),
            )
        } else if (product.kind == ProductKind.COMBO) {
            Text(
                text = stringResource(R.string.product_combo),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Imagen de la tarjeta baja (poca altura). */
private val COMPACT_IMAGE_SIZE = 72.dp

/** Precio de referencia de Go, «Elige presentación» o «Sin precio». */
@Composable
private fun priceText(product: CatalogProduct): String {
    val price = product.referencePrice
    return when {
        price != null -> price.display()
        product.requiresVariant -> stringResource(R.string.product_choose_variant)
        else -> stringResource(R.string.product_no_price)
    }
}

/** Marca de disponibilidad, o null si se puede elegir. */
@Composable
private fun statusText(product: CatalogProduct): String? =
    when (product.availability.status) {
        AvailabilityStatus.SOLD_OUT -> stringResource(R.string.product_sold_out)
        AvailabilityStatus.SOLD_OUT_INDEFINITE ->
            stringResource(R.string.product_sold_out_indefinite)
        AvailabilityStatus.UNAVAILABLE ->
            product.availability.reason.ifBlank { stringResource(R.string.product_unavailable) }
        // Disponible pero sin precio que Go pudiera determinar: tampoco se puede elegir.
        AvailabilityStatus.AVAILABLE ->
            if (product.selectable) {
                null
            } else {
                stringResource(R.string.product_unavailable)
            }
    }

fun productTag(productId: String): String = "product-$productId"

fun categoryTag(categoryId: String): String = "category-$categoryId"

const val TAG_CATALOG_BROWSER = "catalog-browser"
const val TAG_CATALOG_SEARCH = "catalog-search"
const val TAG_CATALOG_SEARCH_CLEAR = "catalog-search-clear"
const val TAG_CATEGORY_BAR = "category-bar"
const val TAG_PRODUCT_GRID = "product-grid"
const val TAG_PRODUCT_STATUS = "product-status"
const val TAG_PRODUCT_WITHOUT_IMAGE = "product-without-image"
const val TAG_CATALOG_EMPTY = "catalog-empty"
