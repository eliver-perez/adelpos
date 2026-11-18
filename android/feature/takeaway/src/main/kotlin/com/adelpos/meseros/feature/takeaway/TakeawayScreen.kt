package com.adelpos.meseros.feature.takeaway

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
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
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.meseros.core.capture.CustomerDialog
import com.adelpos.meseros.core.capture.customerSummary
import com.adelpos.meseros.core.domain.waiter.DeliveryOffer
import com.adelpos.meseros.core.domain.waiter.ElapsedTime
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.TakeawayDelivery
import com.adelpos.meseros.core.domain.waiter.TakeawayEntry
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.TakeawayStates
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.components.PrimaryButton
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.waiter.ElapsedLabel
import com.adelpos.meseros.core.ui.waiter.OrderStateBadge
import com.adelpos.meseros.core.ui.waiter.TakeawayPaymentBadge
import com.adelpos.meseros.core.ui.waiter.TakeawayPreparationBadge
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * «Para llevar» con su ViewModel: [onOpenCapture] abre la captura compartida (un pedido nuevo o uno
 * de la lista) y [onOpenReview], «Requiere tu revisión».
 */
@Composable
fun TakeawayRoute(
    dependencies: TakeawayDependencies,
    onOpenCapture: (String) -> Unit,
    onOpenReview: (String) -> Unit,
    viewModel: TakeawayViewModel = viewModel(factory = TakeawayViewModel.factory(dependencies)),
) {
    val entries by viewModel.entries.collectAsState()
    val permissions by viewModel.permissions.collectAsState()
    val status by viewModel.status.collectAsState()
    val readAt by viewModel.readAt.collectAsState()
    val clock by viewModel.clock.collectAsState()
    var editingKey by rememberSaveable { mutableStateOf<String?>(null) }
    var message by rememberSaveable { mutableStateOf<String?>(null) }
    val texts =
        mapOf(
            TakeawayNotice.DELIVERED to stringResource(R.string.takeaway_delivered_notice),
            TakeawayNotice.COLLECT_FIRST to stringResource(R.string.takeaway_collect_first),
            TakeawayNotice.ALREADY_DELIVERED to stringResource(R.string.takeaway_already_delivered),
            TakeawayNotice.NO_CONNECTION to stringResource(R.string.takeaway_deliver_offline),
            TakeawayNotice.FAILED to stringResource(R.string.takeaway_deliver_failed),
        )
    LaunchedEffect(viewModel.notice) {
        val notice = viewModel.notice ?: return@LaunchedEffect
        message = viewModel.noticeMessage ?: texts.getValue(notice)
        viewModel.onNoticeShown()
    }
    TakeawayScreen(
        entries = entries,
        permissions = permissions,
        status = status,
        readAt = readAt,
        clock = clock,
        online = viewModel.online,
        delivering = viewModel.delivering,
        message = message,
        actions =
            TakeawayActions(
                onNewOrder = { viewModel.newOrder(onOpenCapture) },
                onOpen = { entry -> viewModel.open(entry, onOpenCapture, onOpenReview) },
                onEditCustomer = { entry -> editingKey = entry.key },
                onDeliver = { entry ->
                    message = null
                    viewModel.deliver(entry)
                },
                onRefresh = {
                    message = null
                    viewModel.refresh()
                },
            ),
    )
    val editing = entries.firstOrNull { it.key == editingKey }
    if (editing != null) {
        CustomerDialog(
            initial = editing.customer,
            onSave = { customer ->
                editingKey = null
                viewModel.editCustomer(editing, customer)
            },
            onDismiss = { editingKey = null },
        )
    }
}

/** Lo que el mesero puede tocar en la lista. */
class TakeawayActions(
    val onNewOrder: () -> Unit,
    val onOpen: (TakeawayEntry) -> Unit,
    val onEditCustomer: (TakeawayEntry) -> Unit,
    val onDeliver: (TakeawayEntry.InGo) -> Unit,
    val onRefresh: () -> Unit,
)

/**
 * La lista de pedidos para llevar (§3.3): «Nuevo pedido» arriba (con `takeaway.manage`), cómo va la
 * lectura (sin conexión, lo último que se vio con su hora) y cada pedido con folio, nombre si hay,
 * tiempo, preparación y pago como los dice Go; lo que todavía no nace en Go, con su estado de la
 * cola. «Entregado» solo con el permiso y el pedido pagado; sin pagar, «Cobrar en caja antes de
 * entregar» (A2-D2).
 */
@Composable
fun TakeawayScreen(
    entries: List<TakeawayEntry>,
    permissions: WaiterPermissions,
    status: RestaurantStatus,
    readAt: TakeawayRead?,
    clock: TakeawayClock,
    online: Boolean,
    delivering: String?,
    message: String?,
    actions: TakeawayActions,
    modifier: Modifier = Modifier,
) {
    val canCapture = permissions.allows(WaiterAction.MANAGE_TAKEAWAY)
    LazyColumn(
        modifier =
            modifier
                .fillMaxSize()
                .padding(horizontal = AdelDimens.ScreenPadding)
                .testTag(TAG_TAKEAWAY),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        item {
            Column(
                modifier = Modifier.padding(top = AdelDimens.ScreenPadding),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (permissions.allows(WaiterAction.OPEN_TAKEAWAY)) {
                    PrimaryButton(
                        text = stringResource(R.string.takeaway_new_order),
                        onClick = actions.onNewOrder,
                        modifier = Modifier.fillMaxWidth().testTag(TAG_TAKEAWAY_NEW),
                    )
                } else {
                    Text(
                        stringResource(R.string.takeaway_read_only),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag(TAG_TAKEAWAY_READ_ONLY),
                    )
                }
                ReadStatus(status, readAt, actions.onRefresh)
                if (message != null) {
                    Text(
                        message,
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.testTag(TAG_TAKEAWAY_MESSAGE),
                    )
                }
            }
        }
        if (entries.isEmpty()) {
            item {
                Text(
                    stringResource(R.string.takeaway_empty),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(TAG_TAKEAWAY_EMPTY),
                )
            }
        }
        items(entries, key = { it.key }) { entry ->
            EntryRow(
                entry = entry,
                permissions = permissions,
                readAt = readAt,
                clock = clock,
                online = online,
                delivering = delivering == entry.key,
                canCapture = canCapture,
                actions = actions,
            )
        }
        item { Column(Modifier.heightIn(min = AdelDimens.ScreenPadding)) {} }
    }
}

/** «Leyendo…» o, sin conexión, lo último que se vio con su hora, y «Actualizar». */
@Composable
private fun ReadStatus(status: RestaurantStatus, readAt: TakeawayRead?, onRefresh: () -> Unit) {
    val text =
        when {
            status.loading -> stringResource(R.string.takeaway_loading)
            !status.live && readAt != null ->
                stringResource(R.string.takeaway_offline_since, clockText(readAt.wallMillis))
            !status.live -> stringResource(R.string.takeaway_offline_never)
            else -> null
        }
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(
            text ?: "",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f).testTag(TAG_TAKEAWAY_STATUS),
        )
        TextButton(
            onClick = onRefresh,
            modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget),
        ) {
            Text(stringResource(R.string.takeaway_refresh))
        }
    }
}

@Composable
private fun EntryRow(
    entry: TakeawayEntry,
    permissions: WaiterPermissions,
    readAt: TakeawayRead?,
    clock: TakeawayClock,
    online: Boolean,
    delivering: Boolean,
    canCapture: Boolean,
    actions: TakeawayActions,
) {
    val clickable =
        when (entry) {
            is TakeawayEntry.InGo -> canCapture
            is TakeawayEntry.Draft -> true
            is TakeawayEntry.Queued -> entry.order.state == OrderState.NEEDS_REVIEW
        }
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .clickable(enabled = clickable) { actions.onOpen(entry) }
                .testTag("$TAG_TAKEAWAY_ENTRY_PREFIX${entry.key}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    title(entry),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                val seconds = elapsedSeconds(entry, readAt, clock)
                if (seconds != null) {
                    ElapsedLabel(
                        seconds = seconds,
                        longOpen = (entry as? TakeawayEntry.InGo)?.account?.longOpen == true,
                        longOpenHours = null,
                    )
                }
            }
            Text(
                customerSummary(entry.customer),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag("$TAG_TAKEAWAY_CUSTOMER_PREFIX${entry.key}"),
            )
            if (entry.customerPending) {
                Text(
                    stringResource(R.string.takeaway_customer_pending),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Detail(entry)
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (entry is TakeawayEntry.InGo) {
                    TakeawayPreparationBadge(entry.preparation)
                    TakeawayPaymentBadge(
                        payment = entry.payment,
                        delivered = entry.delivery == TakeawayDelivery.DELIVERED,
                    )
                }
                localState(entry)?.let { OrderStateBadge(it) }
            }
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (canCapture) {
                    TextButton(
                        onClick = { actions.onEditCustomer(entry) },
                        modifier =
                            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                                .testTag("$TAG_TAKEAWAY_EDIT_PREFIX${entry.key}"),
                    ) {
                        Text(stringResource(R.string.takeaway_edit_customer))
                    }
                }
                if (entry is TakeawayEntry.InGo) {
                    Delivery(entry, permissions, online, delivering, actions)
                }
            }
        }
    }
}

/** Lo que el mesero puede hacer con la entrega (A2-D2): el botón, el aviso o solo el estado. */
@Composable
private fun Delivery(
    entry: TakeawayEntry.InGo,
    permissions: WaiterPermissions,
    online: Boolean,
    delivering: Boolean,
    actions: TakeawayActions,
) {
    when (TakeawayStates.deliveryOffer(permissions, entry.payment, entry.delivery)) {
        DeliveryOffer.DELIVER ->
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                SecondaryButton(
                    text = stringResource(R.string.takeaway_deliver),
                    onClick = { actions.onDeliver(entry) },
                    enabled = online && !delivering,
                    modifier = Modifier.testTag("$TAG_TAKEAWAY_DELIVER_PREFIX${entry.key}"),
                )
                if (!online) {
                    Text(
                        stringResource(R.string.takeaway_deliver_offline),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        DeliveryOffer.COLLECT_FIRST ->
            Text(
                stringResource(R.string.takeaway_collect_first),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier =
                    Modifier.padding(vertical = 12.dp)
                        .testTag("$TAG_TAKEAWAY_COLLECT_PREFIX${entry.key}"),
            )
        DeliveryOffer.STATUS_ONLY,
        DeliveryOffer.DELIVERED -> Unit
    }
}

/** Lo que falta de un pedido que todavía no nace en Go o que tiene algo sin enviar. */
@Composable
private fun Detail(entry: TakeawayEntry) {
    val text =
        when (entry) {
            is TakeawayEntry.Queued -> localLines(entry.order)
            is TakeawayEntry.Draft -> {
                val count = draftCount(entry.capture)
                pluralStringResource(R.plurals.takeaway_draft_products, count, count)
            }
            is TakeawayEntry.InGo ->
                when {
                    entry.queued != null -> stringResource(R.string.takeaway_has_queued)
                    entry.captureId != null -> stringResource(R.string.takeaway_has_draft)
                    else -> null
                }
        } ?: return
    Text(text, style = MaterialTheme.typography.bodyMedium)
}

@Composable
private fun title(entry: TakeawayEntry): String =
    when (entry) {
        is TakeawayEntry.InGo -> stringResource(R.string.takeaway_folio, entry.account.folio)
        is TakeawayEntry.Queued ->
            entry.order.receivedAccountId?.let { stringResource(R.string.takeaway_received) }
                ?: stringResource(R.string.takeaway_not_in_go)
        is TakeawayEntry.Draft -> stringResource(R.string.takeaway_not_in_go)
    }

/**
 * Segundos desde que se abrió: los de Go más lo que pasó desde la lectura, o los del dispositivo.
 */
private fun elapsedSeconds(entry: TakeawayEntry, readAt: TakeawayRead?, clock: TakeawayClock) =
    when (entry) {
        is TakeawayEntry.InGo -> {
            val seconds = entry.account.elapsedSeconds
            if (seconds == null || readAt == null) {
                null
            } else {
                ElapsedTime.secondsNow(seconds, readAt.elapsedMillis, clock.elapsedMillis)
            }
        }
        else ->
            localSinceMillis(entry)?.let { since ->
                maxOf(0L, clock.wallMillis - since) / MILLIS_PER_SECOND
            }
    }

private const val MILLIS_PER_SECOND = 1000L

private val CLOCK: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm")

/** Hora local del dispositivo de un momento («14:35»). */
private fun clockText(millis: Long): String =
    CLOCK.format(Instant.ofEpochMilli(millis).atZone(ZoneId.systemDefault()))

/** Título de la pantalla para la barra superior. */
@Composable fun takeawayTitle(): String = stringResource(R.string.takeaway_title)

const val TAG_TAKEAWAY = "takeaway"
const val TAG_TAKEAWAY_NEW = "takeaway-new"
const val TAG_TAKEAWAY_READ_ONLY = "takeaway-read-only"
const val TAG_TAKEAWAY_STATUS = "takeaway-status"
const val TAG_TAKEAWAY_MESSAGE = "takeaway-message"
const val TAG_TAKEAWAY_EMPTY = "takeaway-empty"
const val TAG_TAKEAWAY_ENTRY_PREFIX = "takeaway-entry-"
const val TAG_TAKEAWAY_CUSTOMER_PREFIX = "takeaway-customer-"
const val TAG_TAKEAWAY_EDIT_PREFIX = "takeaway-edit-"
const val TAG_TAKEAWAY_DELIVER_PREFIX = "takeaway-deliver-"
const val TAG_TAKEAWAY_COLLECT_PREFIX = "takeaway-collect-"
