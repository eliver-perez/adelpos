package local.adel.pos.android.feature.waiter.catalog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.lifecycle.viewmodel.compose.viewModel
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogImages
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.CatalogSource
import local.adel.pos.android.core.domain.catalog.CatalogSyncStatus
import local.adel.pos.android.core.domain.catalog.LineDescription
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.ui.catalog.CatalogBrowser
import local.adel.pos.android.core.ui.catalog.ProductCustomizer
import local.adel.pos.android.core.ui.layout.AdaptivePanes
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.feature.waiter.R

/** Pantalla de muestra «Catálogo» con su ViewModel (el catálogo llega del AppContainer). */
@Composable
fun CatalogSampleRoute(
    layout: PaneLayout,
    source: CatalogSource,
    images: CatalogImages?,
    viewModel: CatalogSampleViewModel = viewModel(factory = CatalogSampleViewModel.factory(source)),
) {
    val catalog by viewModel.catalog.collectAsState()
    val products by viewModel.products.collectAsState()
    val status by viewModel.status.collectAsState()
    val selectedCategory by viewModel.selectedCategoryId.collectAsState()
    CatalogSampleScreen(
        layout = layout,
        catalog = catalog,
        status = status,
        products = products,
        selectedCategoryId = selectedCategory,
        searchText = viewModel.searchText,
        editing = viewModel.editing,
        added = viewModel.added,
        showingDetail = viewModel.showingDetail,
        images = images,
        onSearchTextChange = viewModel::onSearchTextChange,
        onCategorySelect = viewModel::onCategorySelect,
        onProductSelect = viewModel::onProductSelect,
        onLineChange = viewModel::onLineChange,
        onAdd = viewModel::onAdd,
        onCancel = viewModel::onCancel,
        onShowingDetailChange = viewModel::onShowingDetailChange,
    )
}

/**
 * Catálogo y partida en uno o dos paneles: en la tableta, la cuadrícula y el personalizador lado a
 * lado; en el teléfono, la cuadrícula y el personalizador a un toque (se abre al elegir un
 * producto).
 */
@Composable
fun CatalogSampleScreen(
    layout: PaneLayout,
    catalog: Catalog,
    status: CatalogSyncStatus,
    products: List<CatalogProduct>,
    selectedCategoryId: String?,
    searchText: String,
    editing: LineSelection?,
    added: List<LineSelection>,
    showingDetail: Boolean,
    images: CatalogImages?,
    onSearchTextChange: (String) -> Unit,
    onCategorySelect: (String) -> Unit,
    onProductSelect: (CatalogProduct) -> Unit,
    onLineChange: (LineSelection) -> Unit,
    onAdd: (LineSelection) -> Unit,
    onCancel: () -> Unit,
    onShowingDetailChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    val emptyMessage =
        when {
            catalog.isEmpty -> stringResource(R.string.catalog_sample_not_downloaded)
            searchText.isNotBlank() ->
                stringResource(R.string.catalog_sample_no_results, searchText)
            else -> stringResource(R.string.catalog_sample_empty_category)
        }
    AdaptivePanes(
        layout = layout,
        showSecondary = showingDetail,
        onShowSecondaryChange = onShowingDetailChange,
        primaryTitle = stringResource(R.string.catalog_sample_title),
        openSecondaryLabel = stringResource(R.string.catalog_sample_open_lines),
        primary = {
            CatalogBrowser(
                categories = catalog.categories,
                selectedCategoryId = selectedCategoryId,
                searchText = searchText,
                products = products,
                images = images,
                onSearchTextChange = onSearchTextChange,
                onCategorySelect = onCategorySelect,
                onProductSelect = onProductSelect,
                emptyMessage = emptyMessage,
            )
        },
        secondary = {
            if (editing != null) {
                ProductCustomizer(
                    catalog = catalog,
                    line = editing,
                    onLineChange = onLineChange,
                    onAdd = onAdd,
                    onCancel = onCancel,
                    images = images,
                )
            } else {
                SampleLines(catalog, added, status)
            }
        },
        modifier = modifier,
    )
}

/** Lo agregado en la muestra, descrito como se verá en la cuenta (sin precios finales). */
@Composable
private fun SampleLines(catalog: Catalog, added: List<LineSelection>, status: CatalogSyncStatus) {
    Column(
        modifier =
            Modifier.fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(AdelDimens.ScreenPadding)
                .testTag(TAG_SAMPLE_LINES),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        Text(
            stringResource(R.string.catalog_sample_lines_title),
            style = MaterialTheme.typography.titleLarge,
        )
        if (added.isEmpty()) {
            Text(
                text = stringResource(R.string.catalog_sample_lines_empty),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        for (line in added) {
            val product = catalog.product(line.productId) ?: continue
            // Nombre y descripción se leen juntos (lectores de pantalla y pruebas).
            Column(
                modifier = Modifier.testTag(TAG_SAMPLE_LINE).semantics(mergeDescendants = true) {},
            ) {
                Text(
                    text =
                        stringResource(R.string.catalog_sample_line, line.quantity, product.name),
                    style = MaterialTheme.typography.titleMedium,
                )
                val parts = LineDescription.parts(catalog, line)
                if (parts.isNotEmpty()) {
                    Text(
                        text = parts.joinToString(", "),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            HorizontalDivider()
        }
        Text(
            text = statusText(catalog, status),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.testTag(TAG_SAMPLE_STATUS),
        )
    }
}

@Composable
private fun statusText(catalog: Catalog, status: CatalogSyncStatus): String =
    when {
        status.running -> stringResource(R.string.catalog_sample_status_running)
        status.lastError != null -> stringResource(R.string.catalog_sample_status_failed)
        status.waitingForSession && catalog.isEmpty ->
            stringResource(R.string.catalog_sample_status_no_session)
        catalog.isEmpty -> stringResource(R.string.catalog_sample_not_downloaded)
        else ->
            pluralStringResource(
                R.plurals.catalog_sample_status_ready,
                catalog.products.size,
                catalog.products.size,
            )
    }

/** Título de la pantalla para la barra superior. */
@Composable fun catalogSampleTitle(): String = stringResource(R.string.catalog_sample_title)

const val TAG_SAMPLE_LINES = "catalog-sample-lines"
const val TAG_SAMPLE_LINE = "catalog-sample-line"
const val TAG_SAMPLE_STATUS = "catalog-sample-status"
