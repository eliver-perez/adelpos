package local.adel.pos.android.feature.waiter.tables

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import local.adel.pos.android.core.domain.preferences.TablesView
import local.adel.pos.android.core.domain.table.TableState
import local.adel.pos.android.core.domain.waiter.AccountSummary
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.DiningTable
import local.adel.pos.android.core.domain.waiter.ElapsedTime
import local.adel.pos.android.core.domain.waiter.OfflineOrderPlan
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.RestaurantStatus
import local.adel.pos.android.core.domain.waiter.RestaurantView
import local.adel.pos.android.core.domain.waiter.TableSearch
import local.adel.pos.android.core.domain.waiter.WaiterAction
import local.adel.pos.android.core.domain.waiter.WaiterPermissions
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.components.TableStateBadge
import local.adel.pos.android.core.ui.components.tableStateColors
import local.adel.pos.android.core.ui.layout.AdaptivePanes
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.core.ui.waiter.ElapsedLabel
import local.adel.pos.android.core.ui.waiter.OrderStateBadge
import local.adel.pos.android.core.ui.waiter.PreparationBadge
import local.adel.pos.android.feature.waiter.R
import local.adel.pos.android.feature.waiter.WaiterDependencies

/** «Mesas» con su ViewModel. */
@Composable
fun TablesRoute(
    layout: PaneLayout,
    dependencies: WaiterDependencies,
    onOpenCapture: (String) -> Unit,
    onOpenPending: () -> Unit,
    onOpenOrder: (PendingOrder) -> Unit,
    viewModel: TablesViewModel = viewModel(factory = TablesViewModel.factory(dependencies)),
) {
    val view by viewModel.view.collectAsState()
    val permissions by viewModel.permissions.collectAsState()
    val status by viewModel.status.collectAsState()
    val accounts by viewModel.openAccounts.collectAsState()
    val tablesView by viewModel.tablesView.collectAsState()
    val captures by viewModel.captures.collectAsState()
    val orders by viewModel.orders.collectAsState()
    val now by viewModel.now.collectAsState()
    val selected = viewModel.selectedTableId?.let(view::table)
    TablesScreen(
        layout = layout,
        state =
            TablesState(
                view = view,
                permissions = permissions,
                status = status,
                tablesView = tablesView,
                search = viewModel.search,
                now = now,
                mapAreaId = viewModel.mapAreaId,
            ),
        selected = selected,
        selectedAccounts = selected?.let { viewModel.accountsOf(it.id, accounts) }.orEmpty(),
        captures = captures.filter { it.tableId == selected?.id },
        orders = orders.filter { it.destination.tableId == selected?.id },
        showingTable = viewModel.showingAccount,
        opening = viewModel.opening,
        actions =
            TablesActions(
                onSearchChange = viewModel::onSearchChange,
                onTablesViewChange = viewModel::onTablesViewChange,
                onMapAreaSelect = viewModel::onMapAreaSelect,
                onTableSelect = viewModel::onTableSelect,
                onShowingTableChange = viewModel::onShowingAccountChange,
                onRefresh = viewModel::refresh,
                onOpenPending = onOpenPending,
                onOpenAccount = { table, account ->
                    viewModel.openCapture(table, account, onOpenCapture)
                },
                onOpenCapture = onOpenCapture,
                onOpenOrder = onOpenOrder,
            ),
    )
}

/** Lo que muestra la lista o el mapa. */
data class TablesState(
    val view: RestaurantView,
    val permissions: WaiterPermissions,
    val status: RestaurantStatus,
    val tablesView: TablesView,
    val search: String,
    val now: Long,
    val mapAreaId: String?,
)

/** Lo que el mesero puede tocar en «Mesas». */
class TablesActions(
    val onSearchChange: (String) -> Unit,
    val onTablesViewChange: (TablesView) -> Unit,
    val onMapAreaSelect: (String) -> Unit,
    val onTableSelect: (DiningTable) -> Unit,
    val onShowingTableChange: (Boolean) -> Unit,
    val onRefresh: () -> Unit,
    val onOpenPending: () -> Unit,
    val onOpenAccount: (DiningTable, AccountSummary?) -> Unit,
    val onOpenCapture: (String) -> Unit,
    val onOpenOrder: (PendingOrder) -> Unit,
)

/**
 * Mesas y la mesa elegida en uno o dos paneles: en la tableta lado a lado; en el teléfono, la mesa
 * a un toque.
 */
@Composable
fun TablesScreen(
    layout: PaneLayout,
    state: TablesState,
    selected: DiningTable?,
    selectedAccounts: List<AccountSummary>,
    captures: List<Capture>,
    orders: List<PendingOrder>,
    showingTable: Boolean,
    opening: Boolean,
    actions: TablesActions,
    modifier: Modifier = Modifier,
) {
    AdaptivePanes(
        layout = layout,
        showSecondary = showingTable,
        onShowSecondaryChange = actions.onShowingTableChange,
        primaryTitle = stringResource(R.string.tables_title),
        openSecondaryLabel = stringResource(R.string.tables_open_table),
        primary = { TablesPane(state, actions) },
        secondary = {
            TablePanel(
                table = selected,
                accounts = selectedAccounts,
                captures = captures,
                orders = orders,
                permissions = state.permissions,
                opening = opening,
                actions = actions,
            )
        },
        modifier = modifier,
    )
}

@Composable
private fun TablesPane(state: TablesState, actions: TablesActions) {
    Column(
        modifier = Modifier.fillMaxSize().padding(horizontal = AdelDimens.ScreenPadding),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        Spacer(Modifier.heightIn(min = 4.dp))
        OutlinedTextField(
            value = state.search,
            onValueChange = actions.onSearchChange,
            label = { Text(stringResource(R.string.tables_search)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            modifier = Modifier.fillMaxWidth().testTag(TAG_TABLES_SEARCH),
        )
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            ViewSwitch(state.tablesView, actions.onTablesViewChange)
            Spacer(Modifier.weight(1f))
            TextButton(
                onClick = actions.onOpenPending,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget).testTag(TAG_OPEN_PENDING),
            ) {
                Text(stringResource(R.string.tables_pending))
            }
        }
        StatusLine(state.status, actions.onRefresh)
        if (!state.permissions.allows(WaiterAction.VIEW_TABLES)) {
            Message(stringResource(R.string.tables_no_permission))
            return@Column
        }
        val matches = state.view.tables.filter { TableSearch.matches(it, state.search) }
        when {
            state.view.tables.isEmpty() -> Message(stringResource(R.string.tables_empty))
            matches.isEmpty() -> Message(stringResource(R.string.tables_no_results, state.search))
            state.tablesView == TablesView.MAP ->
                TablesMap(
                    view = state.view,
                    matches = matches.map(DiningTable::id).toSet(),
                    areaId = state.mapAreaId,
                    onAreaSelect = actions.onMapAreaSelect,
                    onTableSelect = actions.onTableSelect,
                    modifier = Modifier.weight(1f),
                )
            else -> TablesList(state, matches, actions.onTableSelect, Modifier.weight(1f))
        }
    }
}

@Composable
private fun ViewSwitch(current: TablesView, onChange: (TablesView) -> Unit) {
    SingleChoiceSegmentedButtonRow(modifier = Modifier.testTag(TAG_TABLES_VIEW)) {
        val options = listOf(TablesView.LIST, TablesView.MAP)
        options.forEachIndexed { index, option ->
            SegmentedButton(
                selected = current == option,
                onClick = { onChange(option) },
                shape = SegmentedButtonDefaults.itemShape(index, options.size),
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag("$TAG_TABLES_VIEW-${option.storedName}"),
            ) {
                Text(
                    stringResource(
                        if (option == TablesView.LIST) {
                            R.string.tables_view_list
                        } else {
                            R.string.tables_view_map
                        },
                    ),
                )
            }
        }
    }
}

@Composable
private fun StatusLine(status: RestaurantStatus, onRefresh: () -> Unit) {
    val text =
        when {
            status.loading -> stringResource(R.string.tables_loading)
            !status.live -> stringResource(R.string.tables_offline)
            else -> null
        }
    if (text == null) {
        return
    }
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = text,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f).testTag(TAG_TABLES_STATUS),
        )
        if (!status.loading) {
            TextButton(
                onClick = onRefresh,
                modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget),
            ) {
                Text(stringResource(R.string.tables_refresh))
            }
        }
    }
}

@Composable
private fun Message(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyLarge,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.testTag(TAG_TABLES_MESSAGE),
    )
}

/** Vista de lista (predeterminada): por área, en su orden. */
@Composable
private fun TablesList(
    state: TablesState,
    matches: List<DiningTable>,
    onTableSelect: (DiningTable) -> Unit,
    modifier: Modifier,
) {
    val byArea = matches.groupBy(DiningTable::areaId)
    LazyColumn(
        modifier = modifier.fillMaxWidth().testTag(TAG_TABLES_LIST),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        for (area in state.view.visibleAreas) {
            val tables =
                state.view.tablesIn(area.id).filter { table ->
                    byArea[area.id].orEmpty().contains(table)
                }
            if (tables.isEmpty()) {
                continue
            }
            item(key = "area-${area.id}") {
                Text(
                    text = area.name,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            items(tables, key = { it.id }) { table ->
                TableRow(
                    table = table,
                    seconds =
                        ElapsedTime.secondsNow(
                            table.occupiedSeconds,
                            state.view.readAtElapsedMillis,
                            state.now,
                        ),
                    longOpenHours = state.view.settings?.longOpenHours,
                    onClick = { onTableSelect(table) },
                )
            }
        }
        item(key = "fin") { Spacer(Modifier.heightIn(min = AdelDimens.ScreenPadding)) }
    }
}

@Composable
private fun TableRow(
    table: DiningTable,
    seconds: Long,
    longOpenHours: Int?,
    onClick: () -> Unit,
) {
    val colors = tableStateColors(table.state)
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, colors.border),
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .clickable(onClick = onClick)
                .testTag("$TAG_TABLE_PREFIX${table.id}"),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(table.name, style = MaterialTheme.typography.titleMedium)
                val details = buildList {
                    if (table.accountCount > 1) {
                        add(
                            pluralStringResource(
                                R.plurals.tables_accounts,
                                table.accountCount,
                                table.accountCount,
                            ),
                        )
                    }
                    if (table.state == TableState.PARTIAL_PAYMENT) {
                        add(stringResource(R.string.tables_balance, table.balance.display()))
                    }
                    if (table.state != TableState.OUT_OF_SERVICE) {
                        add(stringResource(R.string.tables_seats, table.seats))
                    }
                }
                if (details.isNotEmpty()) {
                    Text(
                        details.joinToString(" · "),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            if (table.accountCount > 0) {
                ElapsedLabel(
                    seconds = seconds,
                    longOpen = table.longOpen,
                    longOpenHours = longOpenHours,
                )
            }
            TableStateBadge(table.state)
        }
    }
}

/**
 * Panel de la mesa elegida (§6.2): lo del mesero en ella, las cuentas abiertas para elegir y «Nueva
 * cuenta» (creación diferida: nace con el primer producto). Lo que el mesero no puede hacer aparece
 * deshabilitado con su motivo.
 */
@Composable
private fun TablePanel(
    table: DiningTable?,
    accounts: List<AccountSummary>,
    captures: List<Capture>,
    orders: List<PendingOrder>,
    permissions: WaiterPermissions,
    opening: Boolean,
    actions: TablesActions,
) {
    LazyColumn(
        modifier =
            Modifier.fillMaxSize().padding(AdelDimens.ScreenPadding).testTag(TAG_ACCOUNT_PANE),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        if (table == null) {
            item { Message(stringResource(R.string.table_panel_choose)) }
            return@LazyColumn
        }
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    table.name,
                    style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.weight(1f).testTag(TAG_TABLE_PANEL_TITLE),
                )
                TableStateBadge(table.state)
            }
        }
        if (captures.isNotEmpty() || orders.isNotEmpty()) {
            item {
                Text(
                    stringResource(R.string.table_panel_my_work),
                    style = MaterialTheme.typography.titleMedium,
                )
            }
            items(captures, key = { "captura-${it.id}" }) { capture ->
                val label = capture.accountName ?: stringResource(R.string.table_panel_capture_new)
                val count = OfflineOrderPlan.of(capture).items.sumOf { it.line.quantity }
                WorkRow(
                    title = stringResource(R.string.table_panel_capture, label),
                    detail = pluralStringResource(R.plurals.table_panel_products, count, count),
                    badge = { OrderStateBadge(OrderState.DRAFT) },
                    onClick = { actions.onOpenCapture(capture.id) },
                    tag = "$TAG_TABLE_CAPTURE_PREFIX${capture.id}",
                )
            }
            items(orders, key = { "pedido-${it.operationId}" }) { order ->
                val count = order.lines.sumOf { it.item.line.quantity }
                WorkRow(
                    title =
                        order.destination.accountName
                            ?: stringResource(R.string.table_panel_capture_new),
                    detail = pluralStringResource(R.plurals.table_panel_products, count, count),
                    badge = { OrderStateBadge(order.state) },
                    onClick = { actions.onOpenOrder(order) },
                    tag = "$TAG_TABLE_ORDER_PREFIX${order.operationId}",
                )
            }
        }
        item {
            Text(
                stringResource(R.string.table_panel_accounts),
                style = MaterialTheme.typography.titleMedium,
            )
        }
        if (accounts.isEmpty()) {
            item { Message(stringResource(R.string.table_panel_no_accounts)) }
        }
        items(accounts, key = { "cuenta-${it.id}" }) { account ->
            WorkRow(
                title = stringResource(R.string.table_panel_account, account.name, account.folio),
                detail = account.totals.total.display(),
                badge = { PreparationBadge(account.preparationState) },
                onClick = { actions.onOpenAccount(table, account) },
                tag = "$TAG_ACCOUNT_PREFIX${account.id}",
                enabled = !opening && permissions.allows(WaiterAction.VIEW_ACCOUNTS),
            )
        }
        item {
            val outOfService = table.state == TableState.OUT_OF_SERVICE
            val canOpen = permissions.allows(WaiterAction.OPEN_ACCOUNT)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                PrimaryButton(
                    text = stringResource(R.string.table_panel_new_account),
                    onClick = { actions.onOpenAccount(table, null) },
                    enabled = canOpen && !outOfService && !opening,
                    modifier = Modifier.fillMaxWidth().testTag(TAG_NEW_ACCOUNT),
                )
                Text(
                    text =
                        when {
                            outOfService -> stringResource(R.string.table_panel_out_of_service)
                            !canOpen -> stringResource(R.string.table_panel_no_open_permission)
                            else -> stringResource(R.string.table_panel_new_account_note)
                        },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(TAG_NEW_ACCOUNT_NOTE),
                )
            }
        }
    }
}

@Composable
private fun WorkRow(
    title: String,
    detail: String,
    badge: @Composable () -> Unit,
    onClick: () -> Unit,
    tag: String,
    enabled: Boolean = true,
) {
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .clickable(enabled = enabled, onClick = onClick)
                .testTag(tag),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleSmall)
                Text(
                    detail,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.width(8.dp))
            badge()
        }
    }
}

/** Título de la pantalla para la barra superior. */
@Composable fun tablesTitle(): String = stringResource(R.string.tables_title)

const val TAG_TABLES_SEARCH = "tables-search"
const val TAG_ACCOUNT_PANE = "account-pane"
const val TAG_TABLES_VIEW = "tables-view"
const val TAG_TABLES_LIST = "tables-list"
const val TAG_TABLES_STATUS = "tables-status"
const val TAG_TABLES_MESSAGE = "tables-message"
const val TAG_OPEN_PENDING = "open-pending"
const val TAG_TABLE_PREFIX = "table-"
const val TAG_TABLE_PANEL_TITLE = "table-panel-title"
const val TAG_ACCOUNT_PREFIX = "account-"
const val TAG_TABLE_CAPTURE_PREFIX = "table-capture-"
const val TAG_TABLE_ORDER_PREFIX = "table-order-"
const val TAG_NEW_ACCOUNT = "new-account"
const val TAG_NEW_ACCOUNT_NOTE = "new-account-note"
