package com.adelpos.meseros.feature.access.pin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.core.ui.access.findserver.ProblemNotice
import com.adelpos.core.ui.components.LocalUserActivity
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.DeviceAccess
import com.adelpos.meseros.core.ui.components.AdelPinPadDefaults
import com.adelpos.meseros.core.ui.components.PinPad
import com.adelpos.meseros.core.ui.components.PinPadDots
import com.adelpos.meseros.core.ui.components.WaiterBadge
import com.adelpos.meseros.core.ui.layout.currentHeightCompact
import com.adelpos.meseros.feature.access.R

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
    // Etapa A3 (§4): con poca altura (teléfono horizontal), en dos columnas y más ancho, para que
    // el teclado completo quepa a la vista.
    val compact = currentHeightCompact()
    Dialog(
        onDismissRequest = {},
        properties =
            DialogProperties(
                dismissOnBackPress = false,
                dismissOnClickOutside = false,
                usePlatformDefaultWidth = !compact,
            ),
    ) {
        Surface(
            shape = MaterialTheme.shapes.extraLarge,
            color = MaterialTheme.colorScheme.surfaceContainerHigh,
            modifier =
                Modifier.then(
                        if (compact) {
                            Modifier.padding(horizontal = 24.dp, vertical = 8.dp)
                        } else {
                            Modifier
                        },
                    )
                    .widthIn(max = if (compact) COMPACT_WIDTH else WIDTH)
                    .reportsUserActivity(LocalUserActivity.current)
                    .testTag(TAG_REENTER),
        ) {
            val pad: @Composable (Dp, Boolean) -> Unit = { keyHeight, dots ->
                PinPad(
                    length = pinLength,
                    onDigit = onDigit,
                    onDelete = onDelete,
                    onSubmit = onSubmit,
                    submitLabel = stringResource(R.string.entry_enter),
                    enabled = !busy,
                    showDots = dots,
                    keyHeight = keyHeight,
                    gap = if (dots) AdelPinPadDefaults.Gap else AdelPinPadDefaults.CompactGap,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            val info: @Composable ColumnScope.() -> Unit = {
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
                    Checking()
                }
            }
            val switch: @Composable () -> Unit = {
                SecondaryButton(
                    text = stringResource(R.string.reenter_switch),
                    onClick = onSwitchWaiter,
                    icon = AdelIcons.Users,
                    modifier = Modifier.testTag(TAG_REENTER_SWITCH),
                )
            }
            if (compact) {
                PinColumns(
                    modifier = Modifier.padding(AdelDimens.ScreenPadding),
                    info = {
                        info()
                        PinPadDots(length = pinLength)
                        switch()
                    },
                    pad = { keyHeight -> pad(keyHeight, false) },
                )
            } else {
                Column(
                    modifier =
                        Modifier.verticalScroll(rememberScrollState())
                            .padding(AdelDimens.ScreenPadding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
                ) {
                    info()
                    pad(AdelPinPadDefaults.KeyHeight, true)
                    switch()
                }
            }
        }
    }
}

// Ancho del diálogo: el de siempre y, con poca altura, el de las dos columnas.
private val WIDTH = 480.dp
private val COMPACT_WIDTH = 760.dp

/** Etiquetas para las pruebas. */
const val TAG_REENTER = "reenter-pin"
const val TAG_REENTER_MESSAGE = "reenter-pin-message"
const val TAG_REENTER_SWITCH = "reenter-pin-switch"
