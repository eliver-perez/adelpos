package local.adel.pos.android.feature.waiter.capture

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.viewmodel.compose.viewModel
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogImages
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.waiter.OfflineOrderPlan
import local.adel.pos.android.core.ui.catalog.CatalogBrowser
import local.adel.pos.android.core.ui.catalog.ProductCustomizer
import local.adel.pos.android.core.ui.components.LocalUserActivity
import local.adel.pos.android.core.ui.components.reportsUserActivity
import local.adel.pos.android.core.ui.layout.AdaptivePanes
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.feature.waiter.R
import local.adel.pos.android.feature.waiter.WaiterDependencies

/**
 * Captura de una cuenta con su ViewModel; [onDone] vuelve a Mesas (pedido enviado o descartado).
 */
@Composable
fun CaptureRoute(
    layout: PaneLayout,
    dependencies: WaiterDependencies,
    onDone: () -> Unit,
    viewModel: CaptureViewModel = viewModel(factory = CaptureViewModel.factory(dependencies)),
) {
    val capture by viewModel.capture.collectAsState()
    val bill by viewModel.bill.collectAsState()
    val quote by viewModel.quote.collectAsState()
    val permissions by viewModel.permissions.collectAsState()
    val catalog by viewModel.catalog.collectAsState()
    val products by viewModel.products.collectAsState()
    val category by viewModel.selectedCategoryId.collectAsState()
    var confirmingDiscard by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }
    val tooMany = stringResource(R.string.capture_too_many)
    val nothing = stringResource(R.string.capture_nothing)
    LaunchedEffect(viewModel.notice) {
        when (viewModel.notice) {
            CaptureNotice.Sent -> onDone()
            CaptureNotice.TooManyLines -> message = tooMany
            CaptureNotice.NothingToSend -> message = nothing
            null -> Unit
        }
        viewModel.onNoticeShown()
    }
    val current = capture ?: return
    val ownCount = OfflineOrderPlan.of(current).items.sumOf { it.line.quantity }
    AdaptivePanes(
        layout = layout,
        showSecondary = viewModel.showingOrder,
        onShowSecondaryChange = viewModel::onShowingOrderChange,
        primaryTitle = stringResource(R.string.capture_catalog),
        openSecondaryLabel = stringResource(R.string.capture_open_order, ownCount),
        primary = {
            CatalogPane(
                catalog = catalog,
                products = products,
                selectedCategoryId = category,
                searchText = viewModel.searchText,
                editing = viewModel.editing,
                updating = viewModel.editingItemId != null,
                images = dependencies.images,
                onSearchTextChange = viewModel::onSearchTextChange,
                onCategorySelect = viewModel::onCategorySelect,
                onProductSelect = viewModel::onProductSelect,
                onLineChange = viewModel::onLineChange,
                onAdd = viewModel::onAdd,
                onCancel = viewModel::onCancelEditor,
            )
        },
        secondary = {
            OrderPanel(
                capture = current,
                bill = bill,
                quote = quote,
                catalog = catalog,
                permissions = permissions,
                online = viewModel.online,
                sending = viewModel.sending,
                message = message,
                actions =
                    OrderPanelActions(
                        onQuantity = viewModel::onQuantity,
                        onRemove = viewModel::onRemove,
                        onChange = { item ->
                            viewModel.onChange(item)
                            viewModel.onShowingOrderChange(false)
                        },
                        onKeepMine = viewModel::onKeepMine,
                        onDiscardMine = viewModel::onDiscardMine,
                        onJoinAccount = viewModel::onJoinAccount,
                        onOpenAnotherAccount = viewModel::onOpenAnotherAccount,
                        onRetry = viewModel::onRetrySave,
                        onSend = {
                            message = null
                            viewModel.onSendToKitchen()
                        },
                        onDiscard = { confirmingDiscard = true },
                        onSentDialog = viewModel::onSentDialog,
                    ),
            )
        },
        modifier = Modifier.testTag(TAG_CAPTURE),
    )
    if (confirmingDiscard) {
        AlertDialog(
            onDismissRequest = { confirmingDiscard = false },
            title = { Text(stringResource(R.string.capture_discard_title)) },
            text = { Text(stringResource(R.string.capture_discard_text)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingDiscard = false
                        viewModel.onDiscardCapture()
                    },
                    modifier = Modifier.testTag(TAG_DISCARD_CONFIRM),
                ) {
                    Text(stringResource(R.string.capture_discard_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmingDiscard = false }) {
                    Text(stringResource(R.string.capture_cancel))
                }
            },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        )
    }
    SentLineDialogs(
        dialog = viewModel.sentDialog,
        quote = viewModel.editQuote,
        error = viewModel.sentError,
        working = viewModel.sentWorking,
        canReduce = permissions.canReduceSent,
        onDismiss = { viewModel.onSentDialog(null) },
        onCancelUnits = viewModel::onCancelUnits,
        onPreviewEdit = viewModel::onPreviewEdit,
        onEditUnits = viewModel::onEditUnits,
    )
}

/**
 * El catálogo (categorías siempre a la mano y búsqueda) o, mientras se personaliza una partida, el
 * personalizador táctil (Fase D).
 */
@Composable
private fun CatalogPane(
    catalog: Catalog,
    products: List<CatalogProduct>,
    selectedCategoryId: String?,
    searchText: String,
    editing: LineSelection?,
    updating: Boolean,
    images: CatalogImages?,
    onSearchTextChange: (String) -> Unit,
    onCategorySelect: (String) -> Unit,
    onProductSelect: (CatalogProduct) -> Unit,
    onLineChange: (LineSelection) -> Unit,
    onAdd: (LineSelection) -> Unit,
    onCancel: () -> Unit,
) {
    if (editing != null && catalog.product(editing.productId) != null) {
        ProductCustomizer(
            catalog = catalog,
            line = editing,
            onLineChange = onLineChange,
            onAdd = onAdd,
            onCancel = onCancel,
            images = images,
            modifier =
                Modifier.testTag(if (updating) TAG_CUSTOMIZER_UPDATE else TAG_CUSTOMIZER_NEW),
        )
        return
    }
    CatalogBrowser(
        categories = catalog.categories,
        selectedCategoryId = selectedCategoryId,
        searchText = searchText,
        products = products,
        images = images,
        onSearchTextChange = onSearchTextChange,
        onCategorySelect = onCategorySelect,
        onProductSelect = onProductSelect,
    )
}

/** Título de la pantalla para la barra superior. */
@Composable fun captureTitle(): String = stringResource(R.string.capture_screen_title)

const val TAG_CAPTURE = "capture"
const val TAG_DISCARD_CONFIRM = "capture-discard-confirm"
const val TAG_CUSTOMIZER_NEW = "capture-customizer-new"
const val TAG_CUSTOMIZER_UPDATE = "capture-customizer-update"
