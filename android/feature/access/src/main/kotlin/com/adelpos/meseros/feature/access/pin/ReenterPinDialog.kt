package com.adelpos.meseros.feature.access.pin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.DeviceAccess
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.PinPad
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.components.WaiterBadge
import com.adelpos.meseros.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.feature.access.R
import com.adelpos.meseros.feature.access.findserver.ProblemNotice

/**
 * «Vuelve a escribir tu PIN» con su ViewModel (uno por mesero), sobre la pantalla del mesero activo
 * cuyo acceso dejó de valer ([ActiveWaiter.mustReenterPin]; revisión R3, M3). No se cierra con
 * Atrás ni tocando fuera: lo que estaba capturando sigue debajo, intacto. «Cambiar de mesero» sigue
 * las reglas de siempre (no se puede con pedidos sin enviar).
 */
@Composable
fun ReenterPinRoute(
    access: DeviceAccess,
    waiter: ActiveWaiter,
    onSwitchWaiter: () -> Unit,
    viewModel: ReenterPinViewModel =
        viewModel(
            key = "reenter-pin-${waiter.userId}",
            factory = ReenterPinViewModel.factory(access, waiter.userId),
        ),
) {
    ReenterPinDialog(
        waiter = waiter,
        pinLength = viewModel.pinLength,
        busy = viewModel.busy,
        message = viewModel.message,
        onDigit = viewModel::onDigit,
        onDelete = viewModel::onDelete,
        onSubmit = viewModel::submit,
        onSwitchWaiter = onSwitchWaiter,
    )
}

/** El diálogo de [ReenterPinRoute], sin estado (pruebas de la pantalla). */
@Composable
fun ReenterPinDialog(
    waiter: ActiveWaiter,
    pinLength: Int,
    busy: Boolean,
    message: EntryMessage?,
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    onSubmit: () -> Unit,
    onSwitchWaiter: () -> Unit,
) {
    Dialog(
        onDismissRequest = {},
        properties = DialogProperties(dismissOnBackPress = false, dismissOnClickOutside = false),
    ) {
        Surface(
            shape = MaterialTheme.shapes.extraLarge,
            color = MaterialTheme.colorScheme.surfaceContainerHigh,
            modifier =
                Modifier.widthIn(max = 480.dp)
                    .reportsUserActivity(LocalUserActivity.current)
                    .testTag(TAG_REENTER),
        ) {
            Column(
                modifier =
                    Modifier.verticalScroll(rememberScrollState())
                        .padding(AdelDimens.ScreenPadding),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    WaiterBadge(
                        initial = waiter.profile.initial,
                        colorHex = waiter.profile.color,
                        size = 48.dp,
                    )
                    Text(
                        text = stringResource(R.string.reenter_title, waiter.profile.name),
                        style = MaterialTheme.typography.headlineSmall,
                    )
                }
                Text(
                    text = stringResource(R.string.reenter_text),
                    style = MaterialTheme.typography.bodyLarge,
                    textAlign = TextAlign.Center,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (message != null) {
                    Box(modifier = Modifier.testTag(TAG_REENTER_MESSAGE)) {
                        ProblemNotice(entryMessageText(message))
                    }
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
                    text = stringResource(R.string.reenter_switch),
                    onClick = onSwitchWaiter,
                    icon = AdelIcons.Users,
                    modifier = Modifier.testTag(TAG_REENTER_SWITCH),
                )
            }
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_REENTER = "reenter-pin"
const val TAG_REENTER_MESSAGE = "reenter-pin-message"
const val TAG_REENTER_SWITCH = "reenter-pin-switch"
