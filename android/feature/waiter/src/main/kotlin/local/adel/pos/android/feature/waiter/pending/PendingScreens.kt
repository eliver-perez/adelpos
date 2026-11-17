package local.adel.pos.android.feature.waiter.pending

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.ChangeKind
import local.adel.pos.android.core.domain.waiter.DiscardResult
import local.adel.pos.android.core.domain.waiter.DraftSync
import local.adel.pos.android.core.domain.waiter.OfflineOrderPlan
import local.adel.pos.android.core.domain.waiter.OrderReviewRules
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.OthersPending
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.ReviewReason
import local.adel.pos.android.core.ui.components.LocalUserActivity
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.components.SecondaryButton
import local.adel.pos.android.core.ui.components.reportsUserActivity
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.core.ui.waiter.OrderStateBadge
import local.adel.pos.android.feature.waiter.R
import local.adel.pos.android.feature.waiter.WaiterDependencies

/** «Pedidos pendientes» con su ViewModel. */
@Composable
fun PendingOrdersRoute(
    dependencies: WaiterDependencies,
    onOpenCapture: (String) -> Unit,
    onOpenReview: (String) -> Unit,
    viewModel: PendingOrdersViewModel =
        viewModel(factory = PendingOrdersViewModel.factory(dependencies)),
) {
    val orders by viewModel.orders.collectAsState()
    val captures by viewModel.captures.collectAsState()
    val others by viewModel.others.collectAsState()
    PendingOrdersScreen(
        orders = orders,
        captures = captures.filter(Capture::inProgress),
        others = others,
        onRetryNow = viewModel::retryNow,
        onOpenCapture = onOpenCapture,
        onOpenReview = onOpenReview,
    )
}

/**
 * Lo que falta enviar, con su estado (los cinco de §7, siempre distintos) y «Reintentar ya». Un
 * pedido «Requiere tu revisión» se abre para revisarlo; una captura, para seguir capturando.
 */
@Composable
fun PendingOrdersScreen(
    orders: List<PendingOrder>,
    captures: List<Capture>,
    others: List<OthersPending>,
    onRetryNow: () -> Unit,
    onOpenCapture: (String) -> Unit,
    onOpenReview: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier =
            modifier
                .fillMaxSize()
                .padding(horizontal = AdelDimens.ScreenPadding)
                .testTag(TAG_PENDING),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        item {
            PrimaryButton(
                text = stringResource(R.string.pending_retry_now),
                onClick = onRetryNow,
                enabled = orders.any { it.state == OrderState.QUEUED } || others.isNotEmpty(),
                modifier =
                    Modifier.fillMaxWidth()
                        .padding(top = AdelDimens.ScreenPadding)
                        .testTag(TAG_RETRY_NOW),
            )
        }
        if (orders.isEmpty() && captures.isEmpty()) {
            item {
                Text(
                    stringResource(R.string.pending_empty),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(TAG_PENDING_EMPTY),
                )
            }
        }
        if (orders.isNotEmpty()) {
            item {
                Text(
                    stringResource(R.string.pending_orders),
                    style = MaterialTheme.typography.titleMedium,
                )
            }
            items(orders.sortedByDescending { it.capturedAtMillis }, key = { it.operationId }) {
                order ->
                OrderRow(order, onOpenReview)
            }
        }
        if (captures.isNotEmpty()) {
            item {
                Text(
                    stringResource(R.string.pending_drafts),
                    style = MaterialTheme.typography.titleMedium,
                )
            }
            items(captures, key = { "captura-${it.id}" }) { capture ->
                CaptureRow(capture, onOpenCapture)
            }
        }
        if (others.isNotEmpty()) {
            item {
                Text(
                    stringResource(R.string.pending_others_title),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.testTag(TAG_PENDING_OTHERS),
                )
            }
            items(others, key = { "otros-${it.waiterName}" }) { other -> OthersRow(other) }
        }
    }
}

@Composable
private fun OrderRow(order: PendingOrder, onOpenReview: (String) -> Unit) {
    val review = order.state == OrderState.NEEDS_REVIEW
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier =
            Modifier.fillMaxWidth()
                .clickable(enabled = review) { onOpenReview(order.operationId) }
                .testTag("$TAG_PENDING_ORDER_PREFIX${order.operationId}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    order.destination.tableName,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                OrderStateBadge(order.state)
            }
            Text(
                order.lines.joinToString(", ") { "${it.item.line.quantity} × ${it.productName}" },
                style = MaterialTheme.typography.bodyMedium,
            )
            val time =
                when (val received = order.receivedAtMillis) {
                    null ->
                        stringResource(R.string.pending_captured_at, clock(order.capturedAtMillis))
                    else -> stringResource(R.string.pending_received_at, clock(received))
                }
            Text(
                time,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (order.credentialRejected) {
                Text(
                    stringResource(R.string.pending_credential),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier =
                        Modifier.testTag("$TAG_PENDING_CREDENTIAL_PREFIX${order.operationId}"),
                )
            }
            if (review) {
                SecondaryButton(
                    text = stringResource(R.string.pending_review),
                    onClick = { onOpenReview(order.operationId) },
                    modifier = Modifier.testTag("$TAG_PENDING_REVIEW_PREFIX${order.operationId}"),
                )
            }
        }
    }
}

/**
 * Lo de otro mesero en este dispositivo: cuántos pedidos tiene sin enviar y por qué no salen (su
 * clave de envío ya no vale o esperan su revisión; revisión R6, B3), nunca cuáles.
 */
@Composable
private fun OthersRow(other: OthersPending) {
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth().testTag("$TAG_PENDING_OTHER_PREFIX${other.waiterName}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                pluralStringResource(
                    R.plurals.pending_others_count,
                    other.unsent,
                    other.waiterName,
                    other.unsent,
                ),
                style = MaterialTheme.typography.bodyLarge,
            )
            if (other.waitingForPin > 0) {
                Text(
                    stringResource(R.string.pending_others_pin),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            if (other.needingReview > 0) {
                Text(
                    stringResource(R.string.pending_others_review),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun CaptureRow(capture: Capture, onOpenCapture: (String) -> Unit) {
    val count = OfflineOrderPlan.of(capture).items.sumOf { it.line.quantity }
    // Un descarte pendiente ya no se captura: solo se dice que falta quitarlo de la cuenta (A2).
    val discarding = capture.sync == DraftSync.DISCARDING
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .clickable(enabled = !discarding) { onOpenCapture(capture.id) }
                .testTag("$TAG_PENDING_CAPTURE_PREFIX${capture.id}"),
    ) {
        Row(modifier = Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text(capture.tableName, style = MaterialTheme.typography.titleMedium)
                Text(
                    if (discarding) {
                        stringResource(R.string.pending_discarding)
                    } else {
                        pluralStringResource(R.plurals.table_panel_products, count, count)
                    },
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            OrderStateBadge(OrderState.DRAFT)
        }
    }
}

/** «Requiere tu revisión» con su ViewModel. */
@Composable
fun ReviewRoute(
    dependencies: WaiterDependencies,
    onDone: () -> Unit,
    onOpenCapture: (String) -> Unit,
    viewModel: ReviewViewModel = viewModel(factory = ReviewViewModel.factory(dependencies)),
) {
    val order by viewModel.order.collectAsState()
    var confirmingDiscard by remember { mutableStateOf(false) }
    var discardPending by remember { mutableStateOf(false) }
    if (discardPending) {
        // Revisión R6, A2: lo guardado en la cuenta se quita en cuanto haya conexión.
        AlertDialog(
            onDismissRequest = onDone,
            title = { Text(stringResource(R.string.discard_pending_title)) },
            text = { Text(stringResource(R.string.discard_pending_text)) },
            confirmButton = {
                TextButton(onClick = onDone, modifier = Modifier.testTag(TAG_REVIEW_DISCARD_OK)) {
                    Text(stringResource(R.string.discard_pending_ok))
                }
            },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        )
        return
    }
    val current = order
    if (current == null || current.state != OrderState.NEEDS_REVIEW) {
        Column(modifier = Modifier.fillMaxSize().padding(AdelDimens.ScreenPadding)) {
            if (current != null) {
                OrderStateBadge(current.state)
            }
            Text(
                stringResource(R.string.review_gone),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag(TAG_REVIEW_GONE),
            )
        }
        return
    }
    ReviewScreen(
        order = current,
        working = viewModel.working,
        onAccept = { viewModel.accept(onDone) },
        onResend = { viewModel.resend(onDone) },
        onRemoveLine = viewModel::removeLine,
        onNewAccount = { viewModel.moveToNewAccount(onDone) },
        mergedTarget = viewModel.mergedTarget,
        onSendToAccount = { accountId -> viewModel.sendToAccount(accountId, onDone) },
        onBackToCapture = { viewModel.backToCapture(onOpenCapture) },
        onDiscard = { confirmingDiscard = true },
    )
    if (confirmingDiscard) {
        AlertDialog(
            onDismissRequest = { confirmingDiscard = false },
            title = { Text(stringResource(R.string.review_discard_title)) },
            text = { Text(stringResource(R.string.review_discard_text)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingDiscard = false
                        viewModel.discard { result ->
                            when (result) {
                                DiscardResult.DONE -> onDone()
                                DiscardResult.PENDING -> discardPending = true
                            }
                        }
                    },
                    modifier = Modifier.testTag(TAG_REVIEW_DISCARD_CONFIRM),
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
}

/**
 * La lista clara de §6.4 con los mensajes de Go («Hamburguesa: $120.00 → $130.00», «Malteada de
 * fresa: agotado. Quítalo del pedido.», «La cuenta «Mesa 4» ya se cobró…»): quitar lo que no se
 * puede enviar, abrir una cuenta nueva si la cuenta ya no está abierta (o mandarlo a la cuenta en
 * que se combinó), corregir en la captura o descartar el pedido; «Aceptar los cambios y enviar»
 * solo con cambios que el mesero puede aceptar. Lo que Go dice que ya está en cocina se marca «Ya
 * se envió» y el botón principal nunca invita a enviarlo otra vez: «Enviar solo lo demás» o, si
 * todo ya se había enviado, «Confirmar sin volver a enviar» (revisión R6, A1).
 */
@Composable
fun ReviewScreen(
    order: PendingOrder,
    working: Boolean,
    onAccept: () -> Unit,
    onResend: () -> Unit,
    onRemoveLine: (String) -> Unit,
    onNewAccount: () -> Unit,
    onBackToCapture: () -> Unit,
    onDiscard: () -> Unit,
    modifier: Modifier = Modifier,
    mergedTarget: String? = null,
    onSendToAccount: (String) -> Unit = {},
) {
    val review = requireNotNull(order.review)
    val removable = OrderReviewRules.removableItemIds(order)
    val alreadySent = review.alreadySentItemIds.isNotEmpty()
    LazyColumn(
        modifier =
            modifier
                .fillMaxSize()
                .padding(horizontal = AdelDimens.ScreenPadding)
                .testTag(TAG_REVIEW),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        item {
            Row(
                modifier = Modifier.padding(top = AdelDimens.ScreenPadding),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    order.destination.tableName,
                    style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.weight(1f),
                )
                OrderStateBadge(order.state)
            }
        }
        item {
            Text(stringResource(R.string.review_intro), style = MaterialTheme.typography.bodyMedium)
        }
        item {
            Text(
                review.message,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag(TAG_REVIEW_MESSAGE),
            )
        }
        items(review.changes, key = { "${it.wireKind}-${it.itemId}-${it.productId}" }) { change ->
            Surface(
                shape = MaterialTheme.shapes.medium,
                border =
                    BorderStroke(
                        1.dp,
                        if (change.acceptable) {
                            MaterialTheme.colorScheme.outlineVariant
                        } else {
                            MaterialTheme.colorScheme.error
                        },
                    ),
                modifier =
                    Modifier.fillMaxWidth().testTag("$TAG_REVIEW_CHANGE_PREFIX${change.wireKind}"),
            ) {
                Column(
                    modifier = Modifier.padding(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    if (change.kind == ChangeKind.DRAFT_ITEM_SENT) {
                        Text(
                            stringResource(R.string.review_already_sent),
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.testTag("$TAG_REVIEW_ALREADY_SENT_PREFIX${change.itemId}"),
                        )
                    }
                    Text(change.message, style = MaterialTheme.typography.bodyLarge)
                    val target = change.accountId
                    if (change.kind == ChangeKind.ITEMS_IN_ACCOUNT && target != null) {
                        SecondaryButton(
                            text = stringResource(R.string.review_send_to_account),
                            onClick = { onSendToAccount(target) },
                            enabled = !working,
                            modifier = Modifier.testTag("$TAG_REVIEW_SEND_TO_PREFIX$target"),
                        )
                    }
                    val itemId = change.itemId
                    if (itemId != null && itemId in removable) {
                        SecondaryButton(
                            text = stringResource(R.string.review_remove),
                            onClick = { onRemoveLine(itemId) },
                            enabled = !working,
                            modifier = Modifier.testTag("$TAG_REVIEW_REMOVE_PREFIX$itemId"),
                        )
                    }
                }
            }
        }
        item {
            Text(
                stringResource(R.string.review_products),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                order.lines.joinToString(", ") { "${it.item.line.quantity} × ${it.productName}" },
                style = MaterialTheme.typography.bodyMedium,
            )
        }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                when {
                    review.canAccept ->
                        PrimaryButton(
                            text =
                                when {
                                    OrderReviewRules.everythingAlreadySent(order) ->
                                        stringResource(R.string.review_confirm_all_sent)
                                    alreadySent -> stringResource(R.string.review_send_rest)
                                    else -> stringResource(R.string.review_accept)
                                },
                            onClick = onAccept,
                            enabled = !working,
                            modifier = Modifier.fillMaxWidth().testTag(TAG_REVIEW_ACCEPT),
                        )
                    review.changes.isEmpty() && review.reason == ReviewReason.CHANGES ->
                        PrimaryButton(
                            text = stringResource(R.string.review_send),
                            onClick = onResend,
                            enabled = !working,
                            modifier = Modifier.fillMaxWidth().testTag(TAG_REVIEW_SEND),
                        )
                    else -> Unit
                }
                if (mergedTarget != null) {
                    SecondaryButton(
                        text = stringResource(R.string.review_send_to_merged),
                        onClick = { onSendToAccount(mergedTarget) },
                        enabled = !working,
                        modifier = Modifier.fillMaxWidth().testTag(TAG_REVIEW_SEND_TO_MERGED),
                    )
                }
                if (OrderReviewRules.offersNewAccount(review)) {
                    SecondaryButton(
                        text = stringResource(R.string.review_new_account),
                        onClick = onNewAccount,
                        enabled = !working,
                        modifier = Modifier.fillMaxWidth().testTag(TAG_REVIEW_NEW_ACCOUNT),
                    )
                }
                SecondaryButton(
                    text = stringResource(R.string.review_back_to_capture),
                    onClick = onBackToCapture,
                    enabled = !working,
                    modifier = Modifier.fillMaxWidth().testTag(TAG_REVIEW_BACK_TO_CAPTURE),
                )
                TextButton(
                    onClick = onDiscard,
                    enabled = !working,
                    modifier =
                        Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                            .testTag(TAG_REVIEW_DISCARD),
                ) {
                    Text(stringResource(R.string.review_discard))
                }
            }
        }
    }
}

private val CLOCK: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm")

/** Hora local del dispositivo de un momento («14:35»). */
private fun clock(millis: Long): String =
    CLOCK.format(Instant.ofEpochMilli(millis).atZone(ZoneId.systemDefault()))

/** Títulos de las pantallas para la barra superior. */
@Composable fun pendingTitle(): String = stringResource(R.string.pending_title)

@Composable fun reviewTitle(): String = stringResource(R.string.review_title)

const val TAG_PENDING = "pending"
const val TAG_PENDING_EMPTY = "pending-empty"
const val TAG_PENDING_OTHERS = "pending-others"
const val TAG_PENDING_OTHER_PREFIX = "pending-other-"
const val TAG_RETRY_NOW = "retry-now"
const val TAG_PENDING_ORDER_PREFIX = "pending-order-"
const val TAG_PENDING_CAPTURE_PREFIX = "pending-capture-"
const val TAG_PENDING_REVIEW_PREFIX = "pending-review-"
const val TAG_PENDING_CREDENTIAL_PREFIX = "pending-credential-"
const val TAG_REVIEW = "review"
const val TAG_REVIEW_MESSAGE = "review-message"
const val TAG_REVIEW_CHANGE_PREFIX = "review-change-"
const val TAG_REVIEW_REMOVE_PREFIX = "review-remove-"
const val TAG_REVIEW_ACCEPT = "review-accept"
const val TAG_REVIEW_SEND = "review-send"
const val TAG_REVIEW_NEW_ACCOUNT = "review-new-account"
const val TAG_REVIEW_BACK_TO_CAPTURE = "review-back-to-capture"
const val TAG_REVIEW_DISCARD = "review-discard"
const val TAG_REVIEW_DISCARD_CONFIRM = "review-discard-confirm"
const val TAG_REVIEW_GONE = "review-gone"
const val TAG_REVIEW_DISCARD_OK = "review-discard-ok"
const val TAG_REVIEW_ALREADY_SENT_PREFIX = "review-already-sent-"
const val TAG_REVIEW_SEND_TO_PREFIX = "review-send-to-"
const val TAG_REVIEW_SEND_TO_MERGED = "review-send-to-merged"
