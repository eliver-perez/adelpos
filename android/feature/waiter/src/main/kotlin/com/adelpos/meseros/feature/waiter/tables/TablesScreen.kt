package com.adelpos.meseros.feature.waiter.tables

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.components.LocalUserActivity
import com.adelpos.core.ui.components.PrimaryButton
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.capture.PrecheckPrintBar
import com.adelpos.meseros.core.capture.PrecheckSection
import com.adelpos.meseros.core.capture.R as CaptureR
import com.adelpos.meseros.core.domain.preferences.TablesView
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.Capture
import com.adelpos.meseros.core.domain.waiter.CardDelivery
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DiscardAccountResult
import com.adelpos.meseros.core.domain.waiter.DraftSync
import com.adelpos.meseros.core.domain.waiter.ElapsedTime
import com.adelpos.meseros.core.domain.waiter.OfflineOrderPlan
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.RestaurantStatus
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TableCard
import com.adelpos.meseros.core.domain.waiter.TableCards
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.core.ui.components.TableStateBadge
import com.adelpos.meseros.core.ui.components.tableStateColors
import com.adelpos.meseros.core.ui.layout.AdaptivePanes
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.waiter.CaptureUnsentBadge
import com.adelpos.meseros.core.ui.waiter.ElapsedLabel
import com.adelpos.meseros.core.ui.waiter.MineBadge
import com.adelpos.meseros.core.ui.waiter.OrderStateBadge
import com.adelpos.meseros.core.ui.waiter.PreparationBadge
import com.adelpos.meseros.feature.waiter.R
import com.adelpos.meseros.feature.waiter.WaiterDependencies

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
    val link by viewModel.link.collectAsState()
    val accounts by viewModel.openAccounts.collectAsState()
    val tablesView by viewModel.tablesView.collectAsState()
    val captures by viewModel.captures.collectAsState()
    val orders by viewModel.orders.collectAsState()
    val now by viewModel.now.collectAsState()
    val precheckSetup by viewModel.precheckSetup.collectAsState()
    val precheckState by viewModel.precheckState.collectAsState()
    val canWriteNow by viewModel.canWriteNow.collectAsState()
    var confirmingDiscard by remember { mutableStateOf<AccountSummary?>(null) }
    val selected = viewModel.selectedTableId?.let(view::table)
    TablesScreen(
        layout = layout,
        state =
            TablesState(
                view = view,
                permissions = permissions,
                status = status,
                offline = link == LinkState.OFFLINE,
                tablesView = tablesView,
                now = now,
                areaId = viewModel.areaId,
            ),
        selected = selected,
        selectedAccounts = selected?.let { viewModel.accountsOf(it.id, accounts) }.orEmpty(),
        captures = captures.filter { it.tableId == selected?.id },
        orders = orders.filter { it.destination.tableId == selected?.id },
        showingTable = viewModel.showingAccount,
        opening = viewModel.opening,
        userId = viewModel.userId,
        tools =
            AccountTools(
                precheckSetup = precheckSetup,
                printingAccountId = viewModel.printingAccountId,
                printing = precheckState,
                canWriteNow = canWriteNow,
                discardingAccountId = viewModel.discardingAccountId,
                discardResult = viewModel.discardResult,
            ),
        actions =
            TablesActions(
                onTablesViewChange = viewModel::onTablesViewChange,
                onAreaSelect = viewModel::onAreaSelect,
                onTableSelect = viewModel::onTableSelect,
                onShowingTableChange = viewModel::onShowingAccountChange,
                onRefresh = viewModel::refresh,
                onOpenPending = onOpenPending,
                onOpenAccount = { table, account ->
                    viewModel.openCapture(table, account, onOpenCapture)
                },
                onOpenCapture = onOpenCapture,
                onOpenOrder = onOpenOrder,
                onPrintPrecheck = viewModel::onPrintPrecheck,
                onRetryPrecheck = viewModel::onRetryPrecheck,
                onDiscardEmpty = { account -> confirmingDiscard = account },
            ),
    )
    val discarding = confirmingDiscard
    if (discarding != null) {
        DiscardEmptyDialog(
            account = discarding,
            onConfirm = {
                confirmingDiscard = null
                viewModel.onDiscardEmpty(discarding)
            },
            onDismiss = { confirmingDiscard = null },
        )
    }
}

/**
 * Lo que muestra la lista o el mapa. [offline] es el estado único de conexión (etapa A3, A3-D1), el
 * mismo de la barra: «Sin conexión» se dice solo con él, nunca por una lectura que falló. [areaId]
 * es el área elegida en la fila de arriba (§4.7: la lista y el mapa muestran esa área).
 */
data class TablesState(
    val view: RestaurantView,
    val permissions: WaiterPermissions,
    val status: RestaurantStatus,
    val offline: Boolean,
    val tablesView: TablesView,
    val now: Long,
    val areaId: String?,
) {
    /** El área que se ve: la elegida o, si ya no existe, la primera. */
    val area: DiningArea?
        get() = view.visibleAreas.firstOrNull { it.id == areaId } ?: view.visibleAreas.firstOrNull()
}

/**
 * Lo de cada cuenta del panel que no viene de la cuenta (etapa A3): la precuenta (§4.9, una
 * impresión siguiéndose a la vez, la de [printingAccountId]) y «Descartar cuenta vacía» (§4.10).
 * [canWriteNow]: sesión de Go y conexión (el estado único); ninguna de las dos entra en la cola.
 */
class AccountTools(
    val precheckSetup: PrecheckSetup,
    val printingAccountId: String?,
    val printing: PrecheckPrintState,
    val canWriteNow: Boolean,
    val discardingAccountId: String?,
    val discardResult: DiscardAccountResult?,
) {
    companion object {
        val NONE =
            AccountTools(
                precheckSetup = PrecheckSetup.UNKNOWN,
                printingAccountId = null,
                printing = PrecheckPrintState.IDLE,
                canWriteNow = false,
                discardingAccountId = null,
                discardResult = null,
            )
    }
}

/** Lo que el mesero puede tocar en «Mesas». */
class TablesActions(
    val onTablesViewChange: (TablesView) -> Unit,
    val onAreaSelect: (String) -> Unit,
    val onTableSelect: (DiningTable) -> Unit,
    val onShowingTableChange: (Boolean) -> Unit,
    val onRefresh: () -> Unit,
    val onOpenPending: () -> Unit,
    val onOpenAccount: (DiningTable, AccountSummary?) -> Unit,
    val onOpenCapture: (String) -> Unit,
    val onOpenOrder: (PendingOrder) -> Unit,
    val onPrintPrecheck: (AccountSummary) -> Unit = {},
    val onRetryPrecheck: (AccountSummary) -> Unit = {},
    val onDiscardEmpty: (AccountSummary) -> Unit = {},
)

/**
 * Mesas y la mesa elegida en uno o dos paneles: en la tableta lado a lado; en el teléfono, tocar la
 * mesa abre su panel (etapa A3, §4.7: sin «Ver la mesa») y la flecha de la barra regresa.
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
    tools: AccountTools = AccountTools.NONE,
    userId: String? = null,
) {
    AdaptivePanes(
        layout = layout,
        showSecondary = showingTable,
        onShowSecondaryChange = actions.onShowingTableChange,
        primaryTitle = stringResource(R.string.tables_title),
        openSecondaryLabel = null,
        primary = { TablesPane(state, actions) },
        secondary = {
            TablePanel(
                table = selected,
                accounts = selectedAccounts,
                captures = captures,
                orders = orders,
                permissions = state.permissions,
                opening = opening,
                tools = tools,
                actions = actions,
                userId = userId,
            )
        },
        modifier = modifier,
    )
}

@Composable
private fun TablesPane(state: TablesState, actions: TablesActions) {
    Column(
        modifier =
            Modifier.fillMaxSize()
                .padding(horizontal = AdelDimens.ScreenPadding)
                .testTag(TAG_TABLES_SCREEN),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // Etapa A3 (§4.7): una sola fila con «Lista | Mapa», las áreas como pestañas (se desplazan
        // si no caben) y «Pendientes»; sin búsqueda de mesa.
        Row(
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag(TAG_TABLES_BAR),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            ViewSwitch(state.tablesView, actions.onTablesViewChange)
            AreaTabs(
                areas = state.view.visibleAreas,
                selected = state.area,
                onSelect = actions.onAreaSelect,
                modifier = Modifier.weight(1f),
            )
            TextButton(
                onClick = actions.onOpenPending,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget).testTag(TAG_OPEN_PENDING),
            ) {
                Text(stringResource(R.string.tables_pending), maxLines = 1)
            }
        }
        StatusLine(state.status, state.offline, actions.onRefresh)
        if (!state.permissions.allows(WaiterAction.VIEW_TABLES)) {
            Message(stringResource(R.string.tables_no_permission))
            return@Column
        }
        val area = state.area
        when {
            state.view.tables.isEmpty() || area == null ->
                Message(stringResource(R.string.tables_empty))
            state.tablesView == TablesView.MAP ->
                TablesMap(
                    view = state.view,
                    areaId = area.id,
                    onTableSelect = actions.onTableSelect,
                    modifier = Modifier.weight(1f).padding(bottom = 8.dp),
                )
            else -> TablesList(state, area, actions.onTableSelect, Modifier.weight(1f))
        }
    }
}

/**
 * «Lista | Mapa» compacto (etapa A3, §4.7). El `SegmentedButton` de Material deja siempre lugar
 * para un ícono (26 dp por opción, aunque no tenga): en el teléfono vertical no dejaba lugar a las
 * pestañas de las áreas en la misma fila. Mismo aspecto (borde redondo y la opción elegida con
 * fondo) y la misma semántica de opción elegida.
 */
@Composable
private fun ViewSwitch(current: TablesView, onChange: (TablesView) -> Unit) {
    val shape = RoundedCornerShape(percent = 50)
    val outline = MaterialTheme.colorScheme.outline
    Row(
        modifier =
            Modifier.height(AdelDimens.MinTouchTarget)
                .clip(shape)
                .border(1.dp, outline, shape)
                .selectableGroup()
                .testTag(TAG_TABLES_VIEW),
    ) {
        val options = listOf(TablesView.LIST, TablesView.MAP)
        options.forEachIndexed { index, option ->
            if (index > 0) {
                VerticalDivider(color = outline)
            }
            val selected = current == option
            Box(
                contentAlignment = Alignment.Center,
                modifier =
                    Modifier.fillMaxHeight()
                        .then(
                            if (selected) {
                                Modifier.background(MaterialTheme.colorScheme.secondaryContainer)
                            } else {
                                Modifier
                            },
                        )
                        .selectable(
                            selected = selected,
                            onClick = { onChange(option) },
                            role = Role.RadioButton,
                        )
                        .padding(horizontal = 16.dp)
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
                    style = MaterialTheme.typography.labelLarge,
                    color =
                        if (selected) {
                            MaterialTheme.colorScheme.onSecondaryContainer
                        } else {
                            MaterialTheme.colorScheme.onSurface
                        },
                    maxLines = 1,
                )
            }
        }
    }
}

/** Las áreas como pestañas (etapa A3, §4.7): la elegida marcada; se desplazan si no caben. */
@Composable
private fun AreaTabs(
    areas: List<DiningArea>,
    selected: DiningArea?,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier.horizontalScroll(rememberScrollState()).testTag(TAG_TABLES_AREAS),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        for (area in areas) {
            val isSelected = area.id == selected?.id
            // La pestaña elegida siempre se ve completa: en el teléfono vertical caben pocas y la
            // fila se desliza; al elegir una (o al volver a la pantalla) se trae a la vista.
            val requester = remember(area.id) { BringIntoViewRequester() }
            LaunchedEffect(isSelected) {
                if (isSelected) {
                    requester.bringIntoView()
                }
            }
            FilterChip(
                selected = isSelected,
                onClick = { onSelect(area.id) },
                label = { Text(area.name, maxLines = 1) },
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .bringIntoViewRequester(requester)
                        .testTag("$TAG_TABLES_AREA_PREFIX${area.id}"),
            )
        }
    }
}

@Composable
private fun StatusLine(status: RestaurantStatus, offline: Boolean, onRefresh: () -> Unit) {
    // Etapa A3: una lectura que falló con el estado «En línea» se repite sola (sin aviso); solo
    // el estado único dice «Sin conexión».
    val text =
        when {
            status.loading -> stringResource(R.string.tables_loading)
            offline -> stringResource(R.string.tables_offline)
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

/** Vista de lista (predeterminada): las mesas del área elegida, en su orden. */
@Composable
private fun TablesList(
    state: TablesState,
    area: DiningArea,
    onTableSelect: (DiningTable) -> Unit,
    modifier: Modifier,
) {
    val tables = state.view.tablesIn(area.id)
    if (tables.isEmpty()) {
        Message(stringResource(R.string.tables_area_empty))
        return
    }
    LazyColumn(
        modifier = modifier.fillMaxWidth().testTag(TAG_TABLES_LIST),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
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
                Text(
                    table.name,
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
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
 * Panel de la mesa elegida (§6.2): sus cuentas abiertas para elegir y «Nueva cuenta» (creación
 * diferida: nace con el primer producto). Lo que el mesero no puede hacer aparece deshabilitado con
 * su motivo. Etapa A3: cada cuenta en dos renglones (§4.8), con «Imprimir precuenta» (§4.9) y, si
 * Go la marca vacía, «Descartar cuenta vacía» (§4.10). Etapa M1 (§5.5, M1-D16): **una sola tarjeta
 * por cuenta** ([TableCards]) con lo del mesero dentro y «Tu cuenta» en la suya; lo que todavía no
 * tiene cuenta (una captura o un pedido de la cola) es su propia tarjeta con «Por enviar». Ya no
 * hay «Lo tuyo en esta mesa».
 */
@Composable
private fun TablePanel(
    table: DiningTable?,
    accounts: List<AccountSummary>,
    captures: List<Capture>,
    orders: List<PendingOrder>,
    permissions: WaiterPermissions,
    opening: Boolean,
    tools: AccountTools,
    actions: TablesActions,
    userId: String?,
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
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    table.name,
                    style = MaterialTheme.typography.headlineSmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).testTag(TAG_TABLE_PANEL_TITLE),
                )
                TableStateBadge(table.state)
            }
        }
        val cards =
            TableCards.of(
                tableId = table.id,
                accounts = accounts,
                captures = captures,
                orders = orders,
                userId = userId,
            )
        item {
            Text(
                stringResource(R.string.table_panel_accounts),
                style = MaterialTheme.typography.titleMedium,
            )
        }
        if (cards.isEmpty()) {
            item { Message(stringResource(R.string.table_panel_no_accounts)) }
        }
        tools.discardResult?.let { result -> item { DiscardResultText(result, tools.canWriteNow) } }
        items(cards, key = { it.key }) { card ->
            when (card) {
                is TableCard.CaptureCard -> CaptureWorkCard(card.capture, actions)
                is TableCard.OrderCard -> OrderWorkCard(card.order, actions)
                is TableCard.AccountCard -> {
                    val account = card.account
                    // Lo de este dispositivo en esa cuenta (una captura o un pedido de la cola) se
                    // resuelve desde la captura o Pendientes: descartar la cuenta en Go lo dejaría
                    // sin cuenta.
                    val workHere =
                        captures.any { it.accountId == account.id } ||
                            orders.any { it.destination.accountId == account.id }
                    AccountCard(
                        account = account,
                        permissions = permissions,
                        tools = tools,
                        workHere = workHere,
                        enabled = !opening && permissions.allows(WaiterAction.VIEW_ACCOUNTS),
                        onOpen = { actions.onOpenAccount(table, account) },
                        onPrintPrecheck = { actions.onPrintPrecheck(account) },
                        onRetryPrecheck = { actions.onRetryPrecheck(account) },
                        onDiscardEmpty = { actions.onDiscardEmpty(account) },
                        mine = card.mine,
                        delivery = card.delivery,
                        showsKitchen = card.showsKitchen,
                        onOpenCapture = actions.onOpenCapture,
                        onOpenOrder = actions.onOpenOrder,
                    )
                }
            }
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

/**
 * Una captura de una cuenta nueva (o de una cuenta que ya no está abierta) con productos sin enviar
 * (§5.5): su propia tarjeta con «Por enviar», para que no se pierda de vista. Un descarte pendiente
 * ya no se captura (revisión R6, A2).
 */
@Composable
private fun CaptureWorkCard(capture: Capture, actions: TablesActions) {
    val label = capture.accountName ?: stringResource(R.string.table_panel_capture_new)
    val count = OfflineOrderPlan.of(capture).items.sumOf { it.line.quantity }
    val discarding = capture.sync == DraftSync.DISCARDING
    val detail =
        if (discarding) {
            stringResource(CaptureR.string.pending_discarding)
        } else {
            pluralStringResource(R.plurals.table_panel_products, count, count)
        }
    WorkRow(
        title = "$label · $detail",
        badge = { CaptureUnsentBadge() },
        onClick = { actions.onOpenCapture(capture.id) },
        tag = "$TAG_TABLE_CAPTURE_PREFIX${capture.id}",
        enabled = !discarding,
    )
}

/** Un pedido de la cola que todavía no tiene una cuenta abierta («Por enviar», «Enviando»…). */
@Composable
private fun OrderWorkCard(order: PendingOrder, actions: TablesActions) {
    val count = order.lines.sumOf { it.item.line.quantity }
    val name = order.destination.accountName ?: stringResource(R.string.table_panel_capture_new)
    WorkRow(
        title = name + " · " + pluralStringResource(R.plurals.table_panel_products, count, count),
        badge = { OrderStateBadge(order.state) },
        onClick = { actions.onOpenOrder(order) },
        tag = "$TAG_TABLE_ORDER_PREFIX${order.operationId}",
    )
}

/**
 * Una cuenta del panel de la mesa (etapa A3, §4.8): **dos renglones**, arriba «Mesa 6 · folio 12 ·
 * $755.00» en una sola línea y abajo la etiqueta corta de preparación. Antes la etiqueta, sin peso,
 * se medía primero con todo el ancho y en un panel angosto dejaba la columna con peso en ~0 dp: el
 * texto se partía letra por letra. Debajo, «Imprimir precuenta» siempre a la vista (sin permiso o
 * sin conexión, deshabilitado con su motivo, §4.9) y, si Go la marca vacía (`discardable`),
 * «Descartar cuenta vacía» (§4.10).
 */
@Composable
fun AccountCard(
    account: AccountSummary,
    permissions: WaiterPermissions,
    tools: AccountTools,
    enabled: Boolean,
    onOpen: () -> Unit,
    onPrintPrecheck: () -> Unit,
    onRetryPrecheck: () -> Unit,
    onDiscardEmpty: () -> Unit,
    modifier: Modifier = Modifier,
    workHere: Boolean = false,
    mine: Boolean = false,
    delivery: CardDelivery? = null,
    showsKitchen: Boolean = true,
    onOpenCapture: (String) -> Unit = {},
    onOpenOrder: (PendingOrder) -> Unit = {},
) {
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = modifier.fillMaxWidth().testTag("$TAG_ACCOUNT_CARD_PREFIX${account.id}"),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            // Tocar la cuenta (los dos renglones) la abre; los botones de abajo hacen lo suyo.
            Column(
                modifier =
                    Modifier.fillMaxWidth()
                        .heightIn(min = AdelDimens.PrimaryActionHeight)
                        .clickable(enabled = enabled, onClick = onOpen)
                        .testTag("$TAG_ACCOUNT_PREFIX${account.id}")
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                // Etapa M1 (§5.5): «Tu cuenta» arriba a la derecha, junto al título.
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text =
                            stringResource(
                                R.string.table_panel_account_line,
                                account.name,
                                account.folio,
                                account.totals.total.display(),
                            ),
                        style = MaterialTheme.typography.titleSmall,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier =
                            Modifier.weight(1f).testTag("$TAG_ACCOUNT_TITLE_PREFIX${account.id}"),
                    )
                    if (mine) {
                        MineBadge()
                    }
                }
                // El envío de lo del mesero y la cocina de la cuenta (§5.5); sin etiqueta de
                // cocina si nada necesita preparación (§5.2). Si no caben, bajan de renglón.
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    DeliveryBadge(delivery, onOpenCapture, onOpenOrder)
                    if (showsKitchen) {
                        PreparationBadge(account.preparationState)
                    }
                }
            }
            Column(
                modifier = Modifier.padding(start = 12.dp, end = 12.dp, bottom = 12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                val printing = tools.printingAccountId == account.id
                PrecheckPrintBar(
                    section =
                        PrecheckSection(
                            setup = tools.precheckSetup,
                            state = if (printing) tools.printing else PrecheckPrintState.IDLE,
                            available = tools.canWriteNow,
                            allowed = permissions.allows(WaiterAction.PRINT_PRECHECK),
                            onPrint = onPrintPrecheck,
                            onRetry = onRetryPrecheck,
                        ),
                    framed = false,
                )
                if (account.discardable) {
                    DiscardEmptyButton(
                        allowed = permissions.allows(WaiterAction.DISCARD_EMPTY_ACCOUNT),
                        workHere = workHere,
                        online = tools.canWriteNow,
                        working = tools.discardingAccountId == account.id,
                        onClick = onDiscardEmpty,
                        tag = "$TAG_DISCARD_EMPTY_PREFIX${account.id}",
                    )
                }
            }
        }
    }
}

/**
 * Lo del mesero en la cuenta (§5.5): un pedido de la cola sin recibir (al tocarlo, su detalle o su
 * revisión), una captura con productos sin enviar (al tocarla, la captura) o «Recibido por el
 * restaurante».
 */
@Composable
private fun DeliveryBadge(
    delivery: CardDelivery?,
    onOpenCapture: (String) -> Unit,
    onOpenOrder: (PendingOrder) -> Unit,
) {
    when (delivery) {
        null -> Unit
        is CardDelivery.Received -> OrderStateBadge(OrderState.RECEIVED)
        is CardDelivery.Order ->
            Box(
                contentAlignment = Alignment.CenterStart,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .clickable { onOpenOrder(delivery.order) }
                        .testTag("$TAG_TABLE_ORDER_PREFIX${delivery.order.operationId}"),
            ) {
                OrderStateBadge(delivery.order.state)
            }
        is CardDelivery.CaptureWork -> {
            val discarding = delivery.capture.sync == DraftSync.DISCARDING
            Box(
                contentAlignment = Alignment.CenterStart,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .clickable(enabled = !discarding) { onOpenCapture(delivery.capture.id) }
                        .testTag("$TAG_TABLE_CAPTURE_PREFIX${delivery.capture.id}"),
            ) {
                if (discarding) {
                    Text(
                        stringResource(CaptureR.string.pending_discarding),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    CaptureUnsentBadge()
                }
            }
        }
    }
}

/** «Descartar cuenta vacía» con su motivo si no se puede ahora (sin permiso o sin conexión). */
@Composable
private fun DiscardEmptyButton(
    allowed: Boolean,
    workHere: Boolean,
    online: Boolean,
    working: Boolean,
    onClick: () -> Unit,
    tag: String,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        SecondaryButton(
            text = stringResource(R.string.table_panel_discard),
            onClick = onClick,
            enabled = allowed && !workHere && online && !working,
            modifier = Modifier.fillMaxWidth().testTag(tag),
        )
        val reason =
            when {
                !allowed -> stringResource(R.string.table_panel_discard_no_permission)
                workHere -> stringResource(R.string.table_panel_discard_work_here)
                !online -> stringResource(R.string.table_panel_discard_offline)
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

/** Lo que pasó con el último «Descartar cuenta vacía». */
@Composable
private fun DiscardResultText(result: DiscardAccountResult, online: Boolean) {
    val text =
        when (result) {
            DiscardAccountResult.Discarded -> stringResource(R.string.table_panel_discarded)
            DiscardAccountResult.Changed -> stringResource(R.string.table_panel_discard_changed)
            // «Necesitas conexión» solo con la barra en «Sin conexión» (etapa A3, §2).
            DiscardAccountResult.NoConnection ->
                if (online) {
                    stringResource(R.string.table_panel_discard_failed)
                } else {
                    stringResource(R.string.table_panel_discard_offline)
                }
            is DiscardAccountResult.Failed ->
                result.message?.takeIf { it.isNotBlank() }
                    ?: stringResource(R.string.table_panel_discard_failed)
        }
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium,
        modifier = Modifier.testTag(TAG_DISCARD_RESULT),
    )
}

/**
 * La confirmación de «Descartar cuenta vacía» (como en el escritorio): dice que no hay nada enviado
 * a cocina ni pagos, cuántos productos sin enviar se borran (`draft_quantity` de Go) y que no es
 * una cancelación.
 */
@Composable
fun DiscardEmptyDialog(
    account: AccountSummary,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.table_panel_discard_title)) },
        text = {
            val unsent =
                if (account.draftQuantity > 0) {
                    " " +
                        pluralStringResource(
                            R.plurals.table_panel_discard_unsent,
                            account.draftQuantity,
                            account.draftQuantity,
                        )
                } else {
                    ""
                }
            Text(
                stringResource(R.string.table_panel_discard_text, account.name, account.folio) +
                    unsent +
                    " " +
                    stringResource(R.string.table_panel_discard_effect),
                modifier = Modifier.testTag(TAG_DISCARD_EMPTY_TEXT),
            )
        },
        confirmButton = {
            TextButton(
                onClick = onConfirm,
                modifier = Modifier.testTag(TAG_DISCARD_EMPTY_CONFIRM),
            ) {
                Text(stringResource(R.string.table_panel_discard_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.table_panel_discard_cancel))
            }
        },
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
    )
}

/**
 * Una captura o un pedido de la cola en el panel (etapa A3, §4.8): dos renglones, el título en una
 * línea y la etiqueta abajo (nunca un texto partido letra por letra en un panel angosto).
 */
@Composable
private fun WorkRow(
    title: String,
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
        Column(
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text(
                title,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            badge()
        }
    }
}

/** Título de la pantalla para la barra superior. */
@Composable fun tablesTitle(): String = stringResource(R.string.tables_title)

const val TAG_TABLES_SCREEN = "tables-screen"
const val TAG_TABLES_BAR = "tables-bar"
const val TAG_TABLES_AREAS = "tables-areas"
const val TAG_TABLES_AREA_PREFIX = "tables-area-"
const val TAG_ACCOUNT_PANE = "account-pane"
const val TAG_TABLES_VIEW = "tables-view"
const val TAG_TABLES_LIST = "tables-list"
const val TAG_TABLES_STATUS = "tables-status"
const val TAG_TABLES_MESSAGE = "tables-message"
const val TAG_OPEN_PENDING = "open-pending"
const val TAG_TABLE_PREFIX = "table-"
const val TAG_TABLE_PANEL_TITLE = "table-panel-title"
const val TAG_ACCOUNT_PREFIX = "account-"
const val TAG_ACCOUNT_TITLE_PREFIX = "account-title-"
const val TAG_ACCOUNT_CARD_PREFIX = "account-card-"
const val TAG_DISCARD_EMPTY_PREFIX = "discard-empty-"
const val TAG_DISCARD_EMPTY_TEXT = "discard-empty-text"
const val TAG_DISCARD_EMPTY_CONFIRM = "discard-empty-confirm"
const val TAG_DISCARD_RESULT = "discard-result"
const val TAG_TABLE_CAPTURE_PREFIX = "table-capture-"
const val TAG_TABLE_ORDER_PREFIX = "table-order-"
const val TAG_NEW_ACCOUNT = "new-account"
const val TAG_NEW_ACCOUNT_NOTE = "new-account-note"
