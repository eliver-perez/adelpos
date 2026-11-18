package com.adelpos.meseros.feature.waiter.capture

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.LineDescription
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftQuote
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.OfflineOrderPlan
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.SentLine
import com.adelpos.meseros.core.domain.waiter.TableAccountRef
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.catalog.Stepper
import com.adelpos.meseros.core.ui.components.PrimaryButton
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.ui.waiter.OrderStateBadge
import com.adelpos.meseros.core.ui.waiter.PreparationBadge
import com.adelpos.meseros.feature.waiter.R

/** Lo que el mesero puede tocar en la cuenta. */
class OrderPanelActions(
    val onQuantity: (DraftLine, Int) -> Unit,
    val onRemove: (DraftLine) -> Unit,
    val onChange: (DraftLine) -> Unit,
    val onKeepMine: () -> Unit,
    val onDiscardMine: () -> Unit,
    val onJoinAccount: (TableAccountRef) -> Unit,
    val onOpenAnotherAccount: () -> Unit,
    val onRetry: () -> Unit,
    val onSend: () -> Unit,
    val onDiscard: () -> Unit,
    val onSentDialog: (SentDialog?) -> Unit,
)

/**
 * La cuenta, siempre a la vista en la tableta y a un toque en el teléfono (§6.3): lo enviado a
 * cocina con su preparación (del mismo modelo del escritorio y de cocina) y lo por enviar (el
 * borrador, «Borrador» de §7) con −/+, «Cambiar» y «Quitar»; el estado del guardado y, al final,
 * «Enviar a cocina». Los importes son los de Go; sin su cotización, los precios del catálogo son
 * solo de referencia.
 */
@Composable
internal fun OrderPanel(
    capture: Capture,
    bill: AccountBill?,
    quote: DraftQuote?,
    catalog: Catalog,
    permissions: WaiterPermissions,
    online: Boolean,
    sending: Boolean,
    message: String?,
    actions: OrderPanelActions,
) {
    val visible = capture.visible
    val plan = OfflineOrderPlan.of(capture)
    // Precio por unidad de Go para lo guardado (la cotización viene en el orden del borrador).
    val quoted: Map<String, Money> =
        if (quote != null && quote.revision == capture.baseRevision) {
            capture.base.zip(quote.lines).associate { (item, line) -> item.id to line.unitPrice }
        } else {
            emptyMap()
        }
    LazyColumn(
        modifier =
            Modifier.fillMaxSize()
                .padding(horizontal = AdelDimens.ScreenPadding)
                .testTag(TAG_ORDER_PANEL),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        item {
            Column(modifier = Modifier.padding(top = AdelDimens.ScreenPadding)) {
                Text(capture.tableName, style = MaterialTheme.typography.headlineSmall)
                val account = bill?.account
                Text(
                    text =
                        when {
                            account != null ->
                                stringResource(
                                    R.string.capture_account,
                                    account.name,
                                    account.folio,
                                )
                            capture.accountName != null -> capture.accountName!!
                            else -> stringResource(R.string.capture_new_account)
                        },
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(TAG_ORDER_ACCOUNT),
                )
            }
        }
        item { SyncBanner(capture, actions) }
        val sent = bill?.lines.orEmpty()
        if (sent.isNotEmpty()) {
            item { SectionTitle(stringResource(R.string.capture_sent)) }
            items(sent, key = { "enviado-${it.versionId}" }) { line ->
                SentRow(line, catalog, permissions, online, actions)
            }
        }
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                SectionTitle(stringResource(R.string.capture_to_send), Modifier.weight(1f))
                if (plan.items.isNotEmpty()) {
                    OrderStateBadge(OrderState.DRAFT)
                }
            }
        }
        if (visible.isEmpty()) {
            item {
                Text(
                    stringResource(R.string.capture_empty),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        items(visible, key = { "borrador-${it.id}" }) { item ->
            DraftRow(
                item = item,
                catalog = catalog,
                foreign = !capture.isOwn(item.id) && capture.base.any { it.id == item.id },
                price = quoted[item.id] ?: capture.seenPrices[item.id],
                fromGo = quoted.containsKey(item.id),
                canCapture = permissions.allows(WaiterAction.CAPTURE),
                actions = actions,
            )
        }
        item {
            HorizontalDivider()
            val total =
                if (
                    quote != null &&
                        quote.revision == capture.baseRevision &&
                        capture.operations.isEmpty()
                ) {
                    stringResource(R.string.capture_total, quote.combinedTotal.display())
                } else {
                    stringResource(R.string.capture_total_unknown)
                }
            Text(
                total,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(top = 8.dp).testTag(TAG_ORDER_TOTAL),
            )
        }
        if (message != null) {
            item {
                Text(
                    message,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag(TAG_ORDER_MESSAGE),
                )
            }
        }
        item {
            val allowed = permissions.allows(WaiterAction.SEND_TO_KITCHEN)
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                PrimaryButton(
                    text = stringResource(R.string.capture_send),
                    onClick = actions.onSend,
                    enabled = allowed && plan.items.isNotEmpty() && !sending,
                    modifier = Modifier.fillMaxWidth().testTag(TAG_SEND_TO_KITCHEN),
                )
                if (!allowed) {
                    Text(
                        stringResource(R.string.capture_send_no_permission),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (capture.inProgress) {
                    TextButton(
                        onClick = actions.onDiscard,
                        modifier =
                            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                                .testTag(TAG_DISCARD_CAPTURE),
                    ) {
                        Text(stringResource(R.string.capture_discard))
                    }
                }
            }
        }
        item { Column(Modifier.heightIn(min = AdelDimens.ScreenPadding)) {} }
    }
}

@Composable
private fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.titleMedium, modifier = modifier)
}

/** Cómo va el guardado del borrador y, si hace falta, la decisión del mesero. */
@Composable
private fun SyncBanner(capture: Capture, actions: OrderPanelActions) {
    val colors = AdelTheme.colors
    val (text, tone) =
        when (capture.sync) {
            DraftSync.SAVED ->
                if (capture.accountId == null) {
                    return
                } else {
                    stringResource(R.string.sync_saved) to colors.blueSoft
                }
            DraftSync.PENDING -> stringResource(R.string.sync_pending) to colors.amberSoft
            DraftSync.SAVING -> stringResource(R.string.sync_saving) to colors.blueSoft
            DraftSync.UNCERTAIN -> stringResource(R.string.sync_uncertain) to colors.amberSoft
            DraftSync.CONFLICT -> stringResource(R.string.sync_conflict) to colors.redSoft
            DraftSync.TABLE_HAS_ACCOUNT ->
                stringResource(R.string.sync_table_has_account) to colors.amberSoft
            DraftSync.REJECTED -> (capture.syncMessage ?: "") to colors.redSoft
            // El mensaje de Go si lo hubo («La cuenta «Mesa 4» ya se cobró…»); si se supo por el
            // canal de tiempo real, el texto general.
            DraftSync.ACCOUNT_CLOSED ->
                (capture.syncMessage ?: stringResource(R.string.sync_account_closed)) to
                    colors.redSoft
            DraftSync.DISCARDING -> stringResource(R.string.sync_discarding) to colors.amberSoft
        }
    Surface(
        color = tone,
        shape = MaterialTheme.shapes.medium,
        modifier =
            Modifier.fillMaxWidth().testTag("$TAG_SYNC_PREFIX${capture.sync.name.lowercase()}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(text, style = MaterialTheme.typography.bodyMedium)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                when (capture.sync) {
                    DraftSync.CONFLICT -> {
                        SecondaryButton(
                            text = stringResource(R.string.sync_keep_mine),
                            onClick = actions.onKeepMine,
                            modifier = Modifier.testTag(TAG_KEEP_MINE),
                        )
                        SecondaryButton(
                            text = stringResource(R.string.sync_discard_mine),
                            onClick = actions.onDiscardMine,
                            modifier = Modifier.testTag(TAG_DISCARD_MINE),
                        )
                    }
                    DraftSync.TABLE_HAS_ACCOUNT -> {
                        for (account in capture.tableAccounts) {
                            SecondaryButton(
                                text = stringResource(R.string.sync_join_account, account.name),
                                onClick = { actions.onJoinAccount(account) },
                                modifier =
                                    Modifier.testTag("$TAG_JOIN_ACCOUNT_PREFIX${account.id}"),
                            )
                        }
                        SecondaryButton(
                            text = stringResource(R.string.sync_open_another),
                            onClick = actions.onOpenAnotherAccount,
                            modifier = Modifier.testTag(TAG_OPEN_ANOTHER),
                        )
                    }
                    DraftSync.ACCOUNT_CLOSED -> {
                        // Una cuenta combinada: **solo** seguir en la cuenta en que se combinó
                        // (M3; regla del coordinador). Una cuenta nueva dejaría lo movido por
                        // enviar allá y podría enviarse dos veces.
                        for (merged in capture.tableAccounts) {
                            SecondaryButton(
                                text = stringResource(R.string.sync_follow_merged),
                                onClick = { actions.onJoinAccount(merged) },
                                modifier = Modifier.testTag(TAG_FOLLOW_MERGED),
                            )
                        }
                        if (!capture.accountMerged) {
                            SecondaryButton(
                                text = stringResource(R.string.sync_account_closed_action),
                                onClick = actions.onOpenAnotherAccount,
                                modifier = Modifier.testTag(TAG_OPEN_ANOTHER),
                            )
                        }
                    }
                    DraftSync.REJECTED,
                    DraftSync.UNCERTAIN ->
                        SecondaryButton(
                            text = stringResource(R.string.sync_retry),
                            onClick = actions.onRetry,
                            modifier = Modifier.testTag(TAG_SYNC_RETRY),
                        )
                    else -> Unit
                }
            }
        }
    }
}

@Composable
private fun DraftRow(
    item: DraftLine,
    catalog: Catalog,
    foreign: Boolean,
    price: Money?,
    fromGo: Boolean,
    canCapture: Boolean,
    actions: OrderPanelActions,
) {
    val name = catalog.product(item.line.productId)?.name ?: item.line.productId
    val parts = LineDescription.parts(catalog, item.line)
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth().testTag("$TAG_DRAFT_ROW_PREFIX${item.id}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text(name, style = MaterialTheme.typography.titleSmall)
            if (parts.isNotEmpty()) {
                Text(
                    parts.joinToString(" · "),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (foreign) {
                Text(
                    stringResource(R.string.capture_other_terminal),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (price != null) {
                Text(
                    text =
                        if (fromGo) {
                            price.display()
                        } else {
                            stringResource(R.string.capture_reference_price, price.display())
                        },
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Stepper(
                    value = item.line.quantity.toString(),
                    onDecrease = { actions.onQuantity(item, -1) },
                    onIncrease = { actions.onQuantity(item, 1) },
                    canDecrease = canCapture,
                    canIncrease = canCapture,
                    decreaseLabel = "−",
                    increaseLabel = "+",
                    testTag = "$TAG_DRAFT_QUANTITY_PREFIX${item.id}",
                )
                TextButton(
                    onClick = { actions.onChange(item) },
                    enabled = canCapture,
                    modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget),
                ) {
                    Text(stringResource(R.string.capture_change))
                }
                TextButton(
                    onClick = { actions.onRemove(item) },
                    enabled = canCapture,
                    modifier =
                        Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                            .testTag("$TAG_DRAFT_REMOVE_PREFIX${item.id}"),
                ) {
                    Text(stringResource(R.string.capture_remove))
                }
            }
        }
    }
}

@Composable
private fun SentRow(
    line: SentLine,
    catalog: Catalog,
    permissions: WaiterPermissions,
    online: Boolean,
    actions: OrderPanelActions,
) {
    val parts = LineDescription.parts(catalog, line.request)
    Surface(
        shape = MaterialTheme.shapes.medium,
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        modifier = Modifier.fillMaxWidth().testTag("$TAG_SENT_ROW_PREFIX${line.versionId}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "${stringResource(R.string.sent_quantity, line.quantity)} ${line.name}",
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                Text(line.total.display(), style = MaterialTheme.typography.bodyMedium)
            }
            if (parts.isNotEmpty()) {
                Text(
                    parts.joinToString(" · "),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            PreparationBadge(
                if (line.cancelled) {
                    PreparationState.CANCELLED
                } else {
                    line.preparationState
                },
            )
            if (!line.cancelled) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    TextButton(
                        onClick = { actions.onSentDialog(SentDialog.Modify(line)) },
                        enabled = online && permissions.allows(WaiterAction.EDIT_SENT),
                        modifier =
                            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                                .testTag("$TAG_SENT_MODIFY_PREFIX${line.versionId}"),
                    ) {
                        Text(stringResource(R.string.sent_modify))
                    }
                    TextButton(
                        onClick = { actions.onSentDialog(SentDialog.Cancel(line)) },
                        enabled = online && permissions.allows(WaiterAction.CANCEL_SENT),
                        modifier =
                            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                                .testTag("$TAG_SENT_CANCEL_PREFIX${line.versionId}"),
                    ) {
                        Text(stringResource(R.string.sent_cancel_units))
                    }
                }
                val reason =
                    when {
                        !online -> stringResource(R.string.sent_need_connection)
                        !permissions.allows(WaiterAction.EDIT_SENT) &&
                            !permissions.allows(WaiterAction.CANCEL_SENT) ->
                            stringResource(R.string.sent_no_edit_permission)
                        else -> null
                    }
                if (reason != null) {
                    Text(
                        reason,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

const val TAG_ORDER_PANEL = "order-panel"
const val TAG_ORDER_ACCOUNT = "order-account"
const val TAG_ORDER_TOTAL = "order-total"
const val TAG_ORDER_MESSAGE = "order-message"
const val TAG_FOLLOW_MERGED = "follow-merged"
const val TAG_DISCARD_PENDING_OK = "discard-pending-ok"
const val TAG_SEND_TO_KITCHEN = "send-to-kitchen"
const val TAG_DISCARD_CAPTURE = "discard-capture"
const val TAG_SYNC_PREFIX = "sync-"
const val TAG_KEEP_MINE = "keep-mine"
const val TAG_DISCARD_MINE = "discard-mine"
const val TAG_JOIN_ACCOUNT_PREFIX = "join-account-"
const val TAG_OPEN_ANOTHER = "open-another-account"
const val TAG_SYNC_RETRY = "sync-retry"
const val TAG_DRAFT_ROW_PREFIX = "draft-row-"
const val TAG_DRAFT_QUANTITY_PREFIX = "draft-quantity-"
const val TAG_DRAFT_REMOVE_PREFIX = "draft-remove-"
const val TAG_SENT_ROW_PREFIX = "sent-row-"
const val TAG_SENT_MODIFY_PREFIX = "sent-modify-"
const val TAG_SENT_CANCEL_PREFIX = "sent-cancel-"
