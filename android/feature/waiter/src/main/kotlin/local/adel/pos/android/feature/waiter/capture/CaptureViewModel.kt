package local.adel.pos.android.feature.waiter.capture

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
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.CatalogQuery
import local.adel.pos.android.core.domain.catalog.Customization
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.error.AppResult
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.waiter.AccountBill
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.DiscardResult
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.DraftOperations
import local.adel.pos.android.core.domain.waiter.DraftQuote
import local.adel.pos.android.core.domain.waiter.SendRequest
import local.adel.pos.android.core.domain.waiter.SentChangeResult
import local.adel.pos.android.core.domain.waiter.SentLine
import local.adel.pos.android.core.domain.waiter.TableAccountRef
import local.adel.pos.android.core.domain.waiter.WaiterPermissions
import local.adel.pos.android.feature.waiter.WaiterDependencies

/** Aviso de una sola vez para la pantalla de captura. */
sealed interface CaptureNotice {
    /** El pedido ya está en la cola (Enviando o Por enviar): volver a Mesas. */
    data object Sent : CaptureNotice

    data object TooManyLines : CaptureNotice

    data object NothingToSend : CaptureNotice

    /** El guardado anterior todavía no tiene respuesta: el pedido no se armó (revisión R6, C1). */
    data object StillSaving : CaptureNotice

    /**
     * Descartada, pero lo guardado en la cuenta se quita en cuanto haya conexión (revisión R6, A2):
     * se avisa antes de volver a Mesas.
     */
    data object DiscardPending : CaptureNotice
}

/** El diálogo abierto sobre lo enviado (modificar o cancelar unidades). */
sealed interface SentDialog {
    data class Cancel(val line: SentLine) : SentDialog

    data class Modify(val line: SentLine) : SentDialog
}

/**
 * Captura de una cuenta (§6.3): el catálogo con la búsqueda y el personalizador, la cuenta con lo
 * enviado (con su preparación) y lo por enviar, y «Enviar a cocina». Cada toque se guarda en Room
 * por mesero ([local.adel.pos.android.core.domain.waiter.CaptureService]); la búsqueda, la
 * categoría, el producto que se personaliza y el panel abierto en el teléfono, en
 * [SavedStateHandle] (sobreviven a la rotación y al cierre del proceso).
 */
class CaptureViewModel(
    private val dependencies: WaiterDependencies,
    private val savedState: SavedStateHandle,
) : ViewModel() {
    val captureId: String = requireNotNull(savedState[KEY_CAPTURE_ID]) { "Falta la captura" }

    private val userId: String? = dependencies.activeWaiter.value?.userId

    val capture: StateFlow<Capture?> =
        dependencies.captures
            .capture(captureId)
            .stateIn(viewModelScope, SharingStarted.Eagerly, null)

    val quote: StateFlow<DraftQuote?> =
        dependencies.captures
            .quote(captureId)
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), null)

    private val mutableBill = MutableStateFlow<AccountBill?>(null)

    /** La cuenta de Go (lo enviado, con su preparación); null sin cuenta o sin conexión. */
    val bill: StateFlow<AccountBill?> = mutableBill.asStateFlow()

    val permissions: StateFlow<WaiterPermissions> = dependencies.restaurant.permissions

    val catalog: StateFlow<Catalog> = dependencies.catalog.catalog

    private val query =
        MutableStateFlow(
            CatalogQuery(
                text = savedState[KEY_SEARCH] ?: "",
                categoryId = savedState[KEY_CATEGORY],
            ),
        )

    val products: StateFlow<List<CatalogProduct>> =
        combine(dependencies.catalog.catalog, query) { catalog, current ->
                dependencies.catalog.search(
                    current.copy(categoryId = categoryFor(catalog, current.categoryId)),
                )
            }
            .stateIn(
                viewModelScope,
                SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS),
                emptyList(),
            )

    val selectedCategoryId: StateFlow<String?> =
        combine(dependencies.catalog.catalog, query) { catalog, current ->
                categoryFor(catalog, current.categoryId)
            }
            .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), null)

    var searchText: String by mutableStateOf(query.value.text)
        private set

    /** Partida en el personalizador (nueva, o un renglón por enviar que se cambia). */
    var editing: LineSelection? by mutableStateOf(restoreEditing())
        private set

    /** El renglón por enviar que se está cambiando, o null si es uno nuevo. */
    var editingItemId: String? by mutableStateOf(savedState[KEY_EDITING_ITEM])
        private set

    var showingOrder: Boolean by mutableStateOf(savedState[KEY_SHOWING_ORDER] ?: false)
        private set

    var notice: CaptureNotice? by mutableStateOf(null)
        private set

    var sending: Boolean by mutableStateOf(false)
        private set

    var sentDialog: SentDialog? by mutableStateOf(null)
        private set

    /** Cotización de una modificación de lo enviado (con su huella) y el error de Go, si hubo. */
    var editQuote: DraftQuote? by mutableStateOf(null)
        private set

    var sentError: String? by mutableStateOf(null)
        private set

    var sentWorking: Boolean by mutableStateOf(false)
        private set

    /** El mesero tiene sesión de Go: lo enviado solo se modifica o cancela con conexión. */
    val online: Boolean
        get() = dependencies.activeWaiter.value?.hasServerSession == true

    init {
        viewModelScope.launch {
            capture
                .map { it?.accountId }
                .distinctUntilChanged()
                .collect { accountId -> if (accountId != null) loadBill(accountId) }
        }
        viewModelScope.launch {
            // Cambios de otras terminales y de cocina, al momento (canal de tiempo real).
            dependencies.restaurant.accountChanges
                .filter { it == capture.value?.accountId }
                .collect { loadBill(it) }
        }
        restoreChangedLine()
    }

    /**
     * Si Android cerró la aplicación mientras se cambiaba una partida por enviar («Cambiar»), el
     * personalizador vuelve con **esa partida tal como está guardada** en la captura (Room), nunca
     * con la configuración original del producto: «Guardar el cambio» la reemplazaría en silencio
     * (cantidad 1 y sin su personalización). Si la partida ya no existe, se cierra el
     * personalizador.
     */
    private fun restoreChangedLine() {
        val itemId = editingItemId ?: return
        if (editing != null) {
            return
        }
        viewModelScope.launch {
            val saved = capture.filterNotNull().first().visible.firstOrNull { it.id == itemId }
            if (editingItemId != itemId || editing != null) {
                return@launch
            }
            if (saved == null) {
                closeEditor()
            } else {
                editing = saved.line
            }
        }
    }

    private suspend fun loadBill(accountId: String) {
        val result = dependencies.restaurant.bill(accountId)
        if (result is AppResult.Success) {
            mutableBill.value = result.value
            dependencies.captures.observe(captureId, result.value)
        }
    }

    // --- Catálogo ---

    fun onSearchTextChange(text: String) {
        searchText = text
        savedState[KEY_SEARCH] = text
        query.value = query.value.copy(text = text)
    }

    fun onCategorySelect(categoryId: String) {
        savedState[KEY_CATEGORY] = categoryId
        onSearchTextChange("")
        query.value = query.value.copy(categoryId = categoryId)
    }

    fun onProductSelect(product: CatalogProduct) {
        if (!product.selectable) {
            return
        }
        openEditor(LineSelection(product.id, null, 1, Customization.ORIGINAL), itemId = null)
    }

    fun onLineChange(line: LineSelection) {
        editing = line
    }

    /** «Agregar» o «Guardar el cambio» del personalizador. */
    fun onAdd(line: LineSelection) {
        val current = capture.value ?: return
        val itemId = editingItemId
        val operation =
            if (itemId == null) {
                DraftOperations.addLine(current.visible, line, ::newId)
            } else {
                DraftOperation.Replace(itemId, DraftLine(itemId, line))
            }
        stage(operation, referencePrice(line))
        closeEditor()
    }

    fun onCancelEditor() {
        closeEditor()
    }

    // --- Lo por enviar ---

    fun onQuantity(item: DraftLine, delta: Int) {
        stage(DraftOperations.changeQuantity(item, delta, ::newId), null)
    }

    fun onRemove(item: DraftLine) {
        stage(DraftOperation.Remove(item.id), null)
    }

    fun onChange(item: DraftLine) {
        openEditor(item.line, itemId = item.id)
    }

    private fun stage(operation: DraftOperation, seenPrice: Money?) {
        viewModelScope.launch { dependencies.captures.stage(captureId, operation, seenPrice) }
    }

    fun onKeepMine() {
        viewModelScope.launch { dependencies.captures.keepMine(captureId) }
    }

    fun onDiscardMine() {
        viewModelScope.launch { dependencies.captures.discardMine(captureId) }
    }

    fun onJoinAccount(account: TableAccountRef) {
        viewModelScope.launch { dependencies.captures.joinAccount(captureId, account) }
    }

    fun onOpenAnotherAccount() {
        viewModelScope.launch { dependencies.captures.openAnotherAccount(captureId) }
    }

    fun onRetrySave() {
        viewModelScope.launch { dependencies.captures.retrySave(captureId) }
    }

    /** «Enviar a cocina», explícito (§0-B). */
    fun onSendToKitchen() {
        val waiter = dependencies.activeWaiter.value ?: return
        if (sending) {
            return
        }
        sending = true
        viewModelScope.launch {
            try {
                notice =
                    when (dependencies.captures.sendToKitchen(captureId, waiter.profile.name)) {
                        is SendRequest.Queued -> CaptureNotice.Sent
                        SendRequest.NothingToSend -> CaptureNotice.NothingToSend
                        SendRequest.TooManyLines -> CaptureNotice.TooManyLines
                        SendRequest.StillSaving -> CaptureNotice.StillSaving
                    }
            } finally {
                sending = false
            }
        }
    }

    fun onDiscardCapture() {
        viewModelScope.launch {
            notice =
                when (dependencies.captures.discard(captureId)) {
                    DiscardResult.DONE -> CaptureNotice.Sent
                    DiscardResult.PENDING -> CaptureNotice.DiscardPending
                }
        }
    }

    fun onNoticeShown() {
        notice = null
    }

    fun onShowingOrderChange(showing: Boolean) {
        showingOrder = showing
        savedState[KEY_SHOWING_ORDER] = showing
    }

    // --- Lo enviado ---

    fun onSentDialog(dialog: SentDialog?) {
        sentDialog = dialog
        editQuote = null
        sentError = null
    }

    fun onCancelUnits(line: SentLine, quantity: Int, reason: String) {
        val bill = bill.value ?: return
        val waiter = userId ?: return
        runSent {
            dependencies.sentLines.cancel(
                userId = waiter,
                accountId = bill.account.id,
                revision = bill.account.revision,
                versionId = line.versionId,
                quantity = quantity,
                reason = reason,
            )
        }
    }

    fun onPreviewEdit(line: SentLine, request: LineSelection) {
        val bill = bill.value ?: return
        val waiter = userId ?: return
        sentWorking = true
        sentError = null
        viewModelScope.launch {
            try {
                when (
                    val result =
                        dependencies.sentLines.previewEdit(
                            userId = waiter,
                            accountId = bill.account.id,
                            revision = bill.account.revision,
                            versionId = line.versionId,
                            line = request,
                        )
                ) {
                    is AppResult.Success -> editQuote = result.value
                    is AppResult.Failure -> sentError = messageOf(result.error)
                }
            } finally {
                sentWorking = false
            }
        }
    }

    fun onEditUnits(line: SentLine, request: LineSelection, reason: String) {
        val bill = bill.value ?: return
        val quote = editQuote ?: return
        val waiter = userId ?: return
        runSent {
            dependencies.sentLines.edit(
                userId = waiter,
                accountId = bill.account.id,
                revision = bill.account.revision,
                versionId = line.versionId,
                line = request,
                fingerprint = quote.fingerprint,
                reason = reason,
            )
        }
    }

    private fun runSent(block: suspend () -> SentChangeResult) {
        sentWorking = true
        sentError = null
        viewModelScope.launch {
            try {
                when (val result = block()) {
                    is SentChangeResult.Done -> {
                        mutableBill.value = result.bill
                        dependencies.captures.observe(captureId, result.bill)
                        sentDialog = null
                        editQuote = null
                    }
                    is SentChangeResult.Failed -> sentError = messageOf(result.error)
                }
            } finally {
                sentWorking = false
            }
        }
    }

    /** El mensaje de Go, tal cual; sin respuesta, el texto de la pantalla (null). */
    private fun messageOf(error: AppError): String? = error.serverMessage

    override fun onCleared() {
        // Salir sin productos no deja registro.
        dependencies.captures.closeIfIdle(captureId)
    }

    // --- Utilidades ---

    private fun openEditor(line: LineSelection, itemId: String?) {
        editing = line
        editingItemId = itemId
        savedState[KEY_EDITING_PRODUCT] = line.productId
        savedState[KEY_EDITING_ITEM] = itemId
        if (itemId == null) {
            onShowingOrderChange(false)
        }
    }

    private fun closeEditor() {
        editing = null
        editingItemId = null
        savedState[KEY_EDITING_PRODUCT] = null
        savedState[KEY_EDITING_ITEM] = null
    }

    private fun restoreEditing(): LineSelection? {
        val productId: String = savedState[KEY_EDITING_PRODUCT] ?: return null
        if (savedState.get<String>(KEY_EDITING_ITEM) != null) {
            // «Cambiar» una partida: se toma de la captura guardada (restoreChangedLine).
            return null
        }
        // Una partida nueva vuelve con el producto y su configuración original: lo elegido en el
        // personalizador antes de que Android cerrara la aplicación no se guarda.
        return LineSelection(productId, null, 1, Customization.ORIGINAL)
    }

    /**
     * Precio de referencia que vio el mesero (el de la presentación o el del producto). Nunca se
     * calcula uno: si Go cobra otro, lo avisa y el mesero lo acepta antes de que el pedido entre.
     */
    private fun referencePrice(line: LineSelection): Money? {
        val product = catalog.value.product(line.productId) ?: return null
        val variant = product.variants.firstOrNull { it.id == line.variantId }
        return variant?.referencePrice ?: product.referencePrice
    }

    private fun categoryFor(catalog: Catalog, categoryId: String?): String? {
        if (categoryId != null && catalog.categories.any { it.id == categoryId }) {
            return categoryId
        }
        return catalog.categories.firstOrNull()?.id
    }

    private fun newId(): String = UUID.randomUUID().toString()

    companion object {
        const val KEY_CAPTURE_ID = "captureId"
        const val KEY_SEARCH = "capture.search"
        const val KEY_CATEGORY = "capture.category"
        const val KEY_EDITING_PRODUCT = "capture.editingProduct"
        const val KEY_EDITING_ITEM = "capture.editingItem"
        const val KEY_SHOWING_ORDER = "capture.showingOrder"

        private const val STOP_TIMEOUT_MILLIS = 5_000L

        fun factory(dependencies: WaiterDependencies): ViewModelProvider.Factory =
            viewModelFactory {
                initializer { CaptureViewModel(dependencies, createSavedStateHandle()) }
            }
    }
}
