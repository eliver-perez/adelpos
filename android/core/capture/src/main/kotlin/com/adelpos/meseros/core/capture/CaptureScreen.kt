package com.adelpos.meseros.core.capture

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.core.ui.components.LocalUserActivity
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogImages
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.waiter.CaptureTitle
import com.adelpos.meseros.core.domain.waiter.OfflineOrderPlan
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.ui.catalog.CatalogBrowser
import com.adelpos.meseros.core.ui.catalog.ProductCustomizer
import com.adelpos.meseros.core.ui.components.SystemBackHandler
import com.adelpos.meseros.core.ui.layout.AdaptivePanes
import com.adelpos.meseros.core.ui.layout.PaneLayout

/**
 * Captura de una cuenta o de un pedido para llevar con su ViewModel; [onDone] vuelve a la lista de
 * donde se entró (Mesas o Para llevar) con el pedido enviado o descartado.
 *
 * Etapa A3: [onTitleChange] recibe el título de la barra («Mesa 4», «Para llevar #12», §4.2). Al
 * salir (flecha o Atrás) con productos propios sin enviar, lo capturado se queda guardado con su
 * mesero (regla de la A1) y [onLeaveUnsent] recibe la frase que lo dice para mostrarla en la lista
 * (§4.1); quien llama regresa.
 */
@Composable
fun CaptureRoute(
    layout: PaneLayout,
    dependencies: CaptureDependencies,
    onDone: () -> Unit,
    onTitleChange: (String) -> Unit = {},
    onLeaveUnsent: (String) -> Unit = {},
    viewModel: CaptureViewModel = viewModel(factory = CaptureViewModel.factory(dependencies)),
) {
    val capture by viewModel.capture.collectAsState()
    val bill by viewModel.bill.collectAsState()
    val quote by viewModel.quote.collectAsState()
    val permissions by viewModel.permissions.collectAsState()
    val catalog by viewModel.catalog.collectAsState()
    val products by viewModel.products.collectAsState()
    val category by viewModel.selectedCategoryId.collectAsState()
    val customer by viewModel.customer.collectAsState()
    val precheckSetup by viewModel.precheckSetup.collectAsState()
    val precheckState by viewModel.precheckState.collectAsState()
    val canPrintNow by viewModel.canPrintNow.collectAsState()
    var confirmingDiscard by remember { mutableStateOf(false) }
    var editingCustomer by rememberSaveable { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }
    var discardPending by remember { mutableStateOf(false) }
    val tooMany = stringResource(R.string.capture_too_many)
    val nothing = stringResource(R.string.capture_nothing)
    val stillSaving = stringResource(R.string.capture_still_saving)
    LaunchedEffect(viewModel.notice) {
        when (viewModel.notice) {
            CaptureNotice.Sent -> onDone()
            CaptureNotice.TooManyLines -> message = tooMany
            CaptureNotice.NothingToSend -> message = nothing
            CaptureNotice.StillSaving -> message = stillSaving
            CaptureNotice.DiscardPending -> discardPending = true
            null -> Unit
        }
        viewModel.onNoticeShown()
    }
    if (discardPending) {
        // Revisión R6, A2: se dice la verdad antes de volver a Mesas.
        AlertDialog(
            onDismissRequest = onDone,
            title = { Text(stringResource(R.string.discard_pending_title)) },
            text = { Text(stringResource(R.string.discard_pending_text)) },
            confirmButton = {
                TextButton(onClick = onDone, modifier = Modifier.testTag(TAG_DISCARD_PENDING_OK)) {
                    Text(stringResource(R.string.discard_pending_ok))
                }
            },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        )
        return
    }
    val current = capture ?: return
    val ownCount = OfflineOrderPlan.of(current).items.sumOf { it.line.quantity }
    val title = captureTitle(CaptureTitle.of(current, bill))
    LaunchedEffect(title) { onTitleChange(title) }
    // Antes que el de AdaptivePanes: con la cuenta abierta en el teléfono, Atrás la cierra primero.
    val leftNotice =
        if (current.takeaway) {
            stringResource(R.string.capture_left_unsent_takeaway, title)
        } else {
            stringResource(R.string.capture_left_unsent, title)
        }
    SystemBackHandler(enabled = ownCount > 0) { onLeaveUnsent(leftNotice) }
    AdaptivePanes(
        layout = layout,
        showSecondary = viewModel.showingOrder,
        onShowSecondaryChange = viewModel::onShowingOrderChange,
        primaryTitle = stringResource(R.string.capture_catalog),
        openSecondaryLabel =
            if (ownCount > 0) {
                pluralStringResource(R.plurals.capture_open_order_unsent, ownCount, ownCount)
            } else {
                stringResource(R.string.capture_open_order)
            },
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
                        onEditCustomer = { editingCustomer = true },
                    ),
                customer = customer,
                // La precuenta (etapa A2, §5): con la cuenta ya en Go. Etapa A3 (§4.9): sin el
                // permiso de Go se ve el botón con su motivo, no se oculta.
                precheck =
                    if (current.accountId != null) {
                        PrecheckSection(
                            setup = precheckSetup,
                            state = precheckState,
                            available = canPrintNow,
                            onPrint = viewModel::onPrintPrecheck,
                            onRetry = viewModel::onRetryPrecheck,
                            allowed = permissions.allows(WaiterAction.PRINT_PRECHECK),
                        )
                    } else {
                        null
                    },
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
    if (editingCustomer) {
        CustomerDialog(
            initial = customer.customer,
            onSave = { changed ->
                editingCustomer = false
                viewModel.onCustomerChange(changed)
            },
            onDismiss = { editingCustomer = false },
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

/** El título de la barra superior (etapa A3, §4.2): dónde se está. */
@Composable
fun captureTitle(title: CaptureTitle): String =
    when (title) {
        is CaptureTitle.Table -> title.name
        is CaptureTitle.Takeaway -> stringResource(R.string.capture_title_takeaway, title.folio)
        is CaptureTitle.NamedTakeaway -> stringResource(R.string.takeaway_label_named, title.name)
        CaptureTitle.NewTakeaway -> stringResource(R.string.capture_title_takeaway_new)
    }

const val TAG_CAPTURE = "capture"
const val TAG_DISCARD_CONFIRM = "capture-discard-confirm"
const val TAG_CUSTOMIZER_NEW = "capture-customizer-new"
const val TAG_CUSTOMIZER_UPDATE = "capture-customizer-update"
