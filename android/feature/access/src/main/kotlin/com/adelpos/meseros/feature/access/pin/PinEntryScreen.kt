package com.adelpos.meseros.feature.access.pin

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.meseros.core.domain.access.WaiterProfile
import com.adelpos.meseros.core.domain.access.WaiterRoster
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.ui.components.PinPad
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.components.WaiterBadge
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.feature.access.AccessDependencies
import com.adelpos.meseros.feature.access.R
import com.adelpos.meseros.feature.access.clockTime
import com.adelpos.meseros.feature.access.deviceWords
import com.adelpos.meseros.feature.access.findserver.ProblemNotice
import kotlinx.coroutines.delay

/**
 * Entrada con PIN con su ViewModel: pide la lista al mostrarse, cada [refreshMillis] mientras está
 * a la vista (aparecen los meseros a los que se les acaba de asignar un PIN) y cada vez que cambia
 * el estado único de conexión; mientras Go no ha dado su lista y el estado sigue «En línea», la
 * vuelve a pedir a los [retryMillis]. El aviso «Sin conexión» sale del mismo estado que la barra
 * (etapa A3, A3-D1). Con [requestedWaiter], ese mesero queda elegido para escribir su PIN.
 */
@Composable
fun PinEntryRoute(
    dependencies: AccessDependencies,
    layout: PaneLayout,
    viewModel: PinEntryViewModel =
        viewModel(factory = PinEntryViewModel.factory(dependencies.access)),
    refreshMillis: Long = REFRESH_MILLIS,
    retryMillis: Long = RETRY_MILLIS,
    requestedWaiter: String? = null,
) {
    val link by dependencies.serverLink.state.collectAsState()
    val offline = link == LinkState.OFFLINE
    LaunchedEffect(viewModel, link) {
        while (true) {
            viewModel.refresh()
            delay(refreshMillis)
        }
    }
    val waitingForGo = !offline && viewModel.roster?.online == false
    LaunchedEffect(viewModel, waitingForGo) {
        if (waitingForGo) {
            delay(retryMillis)
            viewModel.refresh()
        }
    }
    // Etapa A2: se tocó un aviso de platillos listos en «¿Quién eres?»: se le pide el PIN a su
    // mesero (si está en la lista y nadie eligió otro).
    val roster = viewModel.roster
    LaunchedEffect(requestedWaiter, roster) {
        val requested = roster?.waiters?.firstOrNull { it.userId == requestedWaiter }
        if (requested != null && viewModel.selectedId == null) {
            viewModel.select(requested)
        }
    }
    PinEntryScreen(
        layout = layout,
        roster = viewModel.roster,
        offline = offline,
        loading = viewModel.loading,
        selected = viewModel.selected,
        pinLength = viewModel.pinLength,
        busy = viewModel.busy,
        message = viewModel.message,
        now = dependencies.wallClock(),
        onRefresh = viewModel::refresh,
        onSelect = viewModel::select,
        onBack = viewModel::back,
        onDigit = viewModel::onDigit,
        onDelete = viewModel::onDelete,
        onSubmit = viewModel::submit,
    )
}

/**
 * La lista de «¿Quién eres?» con la pantalla a la vista (A3-D1: la revisión se espacia con el
 * dispositivo sin uso; cada respuesta cuenta también como la revisión de la conexión).
 */
private const val REFRESH_MILLIS = 60_000L

/** Volver a pedir la lista de Go mientras no llega y el estado sigue «En línea». */
private const val RETRY_MILLIS = 5_000L

/**
 * Lista de meseros en botones grandes y el teclado del PIN del elegido. En la tableta, lado a lado;
 * en el teléfono, primero la lista y después el teclado (Atrás regresa a la lista). Con [offline]
 * (el estado único de conexión, el mismo de la barra), el aviso de que solo entran los meseros que
 * ya entraron antes aquí.
 */
@Composable
fun PinEntryScreen(
    layout: PaneLayout,
    roster: WaiterRoster?,
    offline: Boolean,
    loading: Boolean,
    selected: WaiterProfile?,
    pinLength: Int,
    busy: Boolean,
    message: EntryMessage?,
    now: Long,
    onRefresh: () -> Unit,
    onSelect: (WaiterProfile) -> Unit,
    onBack: () -> Unit,
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    onSubmit: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val list: @Composable () -> Unit = {
        WaiterList(
            roster = roster,
            offline = offline,
            loading = loading,
            selected = selected,
            now = now,
            onRefresh = onRefresh,
            onSelect = onSelect,
        )
    }
    val pad: @Composable () -> Unit = {
        PinPanel(
            selected = selected,
            pinLength = pinLength,
            busy = busy,
            message = message,
            onBack = onBack,
            onDigit = onDigit,
            onDelete = onDelete,
            onSubmit = onSubmit,
        )
    }
    when (layout) {
        PaneLayout.DOUBLE ->
            Row(modifier = modifier.fillMaxSize().testTag(TAG_ENTRY)) {
                Box(modifier = Modifier.weight(0.55f).fillMaxHeight()) { list() }
                VerticalDivider()
                Box(modifier = Modifier.weight(0.45f).fillMaxHeight()) { pad() }
            }
        PaneLayout.SINGLE ->
            Box(modifier = modifier.fillMaxSize().testTag(TAG_ENTRY)) {
                if (selected == null) {
                    list()
                } else {
                    BackHandler(onBack = onBack)
                    pad()
                }
            }
    }
}

@Composable
private fun WaiterList(
    roster: WaiterRoster?,
    offline: Boolean,
    loading: Boolean,
    selected: WaiterProfile?,
    now: Long,
    onRefresh: () -> Unit,
    onSelect: (WaiterProfile) -> Unit,
) {
    val words = deviceWords()
    LazyVerticalGrid(
        columns = GridCells.Adaptive(minSize = 200.dp),
        contentPadding = PaddingValues(AdelDimens.ScreenPadding),
        horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        modifier = Modifier.fillMaxSize().testTag(TAG_WAITER_LIST),
    ) {
        item(span = { GridItemSpan(maxLineSpan) }) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    stringResource(R.string.entry_heading),
                    style = MaterialTheme.typography.headlineSmall,
                )
                Text(
                    text = stringResource(R.string.entry_help),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (offline) {
                    Text(
                        text = stringResource(R.string.entry_offline, words.thisDevice),
                        color = AdelTheme.colors.amber,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.testTag(TAG_ENTRY_OFFLINE),
                    )
                }
            }
        }
        // Sin la lista de Go y sin «Sin conexión» todavía se está preguntando: nada que contradiga
        // a la barra.
        val waitingForGo = roster != null && !roster.online && !offline
        if (roster == null || (roster.waiters.isEmpty() && waitingForGo)) {
            item(span = { GridItemSpan(maxLineSpan) }) {
                CircularProgressIndicator(modifier = Modifier.padding(16.dp))
            }
        } else if (roster.waiters.isEmpty()) {
            item(span = { GridItemSpan(maxLineSpan) }) {
                Column(verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
                    Text(
                        text =
                            if (offline)
                                stringResource(R.string.entry_empty_offline, words.thisDevice)
                            else stringResource(R.string.entry_empty_online),
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.testTag(TAG_ENTRY_EMPTY),
                    )
                    SecondaryButton(
                        text = stringResource(R.string.entry_refresh),
                        onClick = onRefresh,
                        enabled = !loading,
                        icon = AdelIcons.Refresh,
                        modifier = Modifier.testTag(TAG_ENTRY_REFRESH),
                    )
                }
            }
        } else {
            items(roster.waiters, key = WaiterProfile::userId) { waiter ->
                WaiterButton(
                    waiter = waiter,
                    selected = waiter.userId == selected?.userId,
                    now = now,
                    onClick = { onSelect(waiter) },
                )
            }
        }
    }
}

/**
 * Botón grande de un mesero: su distintivo, su nombre y, si aplica, hasta cuándo está bloqueado.
 */
@Composable
private fun WaiterButton(
    waiter: WaiterProfile,
    selected: Boolean,
    now: Long,
    onClick: () -> Unit,
) {
    val lockedUntil = waiter.lockedUntilEpochMillis?.takeIf { it > now }
    // Lo que se ve bajo el nombre: el bloqueo de Go (hasta una hora o hasta que lo desbloquee un
    // administrador) o la entrada sin conexión cerrada.
    val notice =
        when {
            waiter.requiresUnlock -> stringResource(R.string.entry_locked_admin_short)
            lockedUntil != null ->
                stringResource(R.string.entry_locked_until, clockTime(lockedUntil))
            waiter.offlineEntryClosed -> stringResource(R.string.entry_offline_closed_short)
            else -> null
        }
    val colors =
        if (selected) {
            ButtonDefaults.outlinedButtonColors(
                containerColor = MaterialTheme.colorScheme.primaryContainer,
            )
        } else {
            ButtonDefaults.outlinedButtonColors()
        }
    OutlinedButton(
        onClick = onClick,
        colors = colors,
        shape = RoundedCornerShape(12.dp),
        contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = WAITER_BUTTON_HEIGHT)
                .testTag("$TAG_WAITER_PREFIX${waiter.userId}"),
    ) {
        WaiterBadge(initial = waiter.initial, colorHex = waiter.color)
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
            Text(
                text = waiter.name,
                style = MaterialTheme.typography.titleLarge,
                color = MaterialTheme.colorScheme.onSurface,
            )
            if (notice != null) {
                Text(
                    text = notice,
                    style = MaterialTheme.typography.bodyMedium,
                    color = AdelTheme.colors.red,
                )
            }
        }
    }
}

@Composable
private fun PinPanel(
    selected: WaiterProfile?,
    pinLength: Int,
    busy: Boolean,
    message: EntryMessage?,
    onBack: () -> Unit,
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    onSubmit: () -> Unit,
) {
    Column(
        modifier =
            Modifier.fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(AdelDimens.ScreenPadding)
                .testTag(TAG_PIN_PANEL),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        if (message != null) {
            Box(modifier = Modifier.testTag(TAG_ENTRY_MESSAGE)) {
                ProblemNotice(entryMessageText(message))
            }
        }
        if (selected == null) {
            Text(
                text = stringResource(R.string.entry_help),
                style = MaterialTheme.typography.titleMedium,
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            WaiterBadge(initial = selected.initial, colorHex = selected.color, size = 56.dp)
            Text(
                text = stringResource(R.string.entry_pin_for, selected.name),
                style = MaterialTheme.typography.headlineSmall,
            )
        }
        if (busy) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            ) {
                CircularProgressIndicator(modifier = Modifier.padding(4.dp))
                Text(stringResource(R.string.entry_checking))
            }
        }
        PinPad(
            length = pinLength,
            onDigit = onDigit,
            onDelete = onDelete,
            onSubmit = onSubmit,
            submitLabel = stringResource(R.string.entry_enter),
            enabled = !busy,
            modifier = Modifier.fillMaxWidth(),
        )
        SecondaryButton(
            text = stringResource(R.string.entry_other_waiter),
            onClick = onBack,
            icon = AdelIcons.ChevronLeft,
            modifier = Modifier.testTag(TAG_ENTRY_BACK),
        )
    }
}

/** El texto de un mensaje de entrada (también lo usa «Vuelve a escribir tu PIN»). */
@Composable
internal fun entryMessageText(message: EntryMessage): String {
    val words = deviceWords()
    val lockedUntil = (message as? EntryMessage.Locked)?.untilEpochMillis
    return when (message) {
        EntryMessage.WrongPin -> stringResource(R.string.entry_wrong_pin)
        is EntryMessage.Locked ->
            if (message.requiresUnlock || lockedUntil == null) {
                stringResource(R.string.entry_locked_admin)
            } else {
                stringResource(R.string.entry_locked, clockTime(lockedUntil))
            }
        EntryMessage.OfflineClosed ->
            stringResource(R.string.entry_offline_closed, words.thisDevice)
        EntryMessage.Busy -> stringResource(R.string.entry_busy)
        is EntryMessage.SupportProblem ->
            stringResource(R.string.entry_support_problem, words.thisDeviceCapital, message.code)
        EntryMessage.NotAllowed -> stringResource(R.string.entry_not_allowed)
        EntryMessage.UnknownOffline ->
            stringResource(R.string.entry_unknown_offline, words.thisDevice)
        EntryMessage.UpdateRequired -> stringResource(R.string.entry_update_required)
        EntryMessage.Unreachable -> stringResource(R.string.entry_unreachable)
        is EntryMessage.Failed -> message.serverMessage ?: stringResource(R.string.entry_failed)
    }
}

private val WAITER_BUTTON_HEIGHT = 72.dp

/** Etiquetas para las pruebas. */
const val TAG_ENTRY = "entry"
const val TAG_WAITER_LIST = "entry-waiters"
const val TAG_WAITER_PREFIX = "entry-waiter-"
const val TAG_ENTRY_OFFLINE = "entry-offline"
const val TAG_ENTRY_EMPTY = "entry-empty"
const val TAG_ENTRY_REFRESH = "entry-refresh"
const val TAG_PIN_PANEL = "entry-pin-panel"
const val TAG_ENTRY_MESSAGE = "entry-message"
const val TAG_ENTRY_BACK = "entry-back"
