package com.adelpos.cocina.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.adelpos.cocina.R
import com.adelpos.cocina.settings.SettingsActions
import com.adelpos.cocina.settings.SettingsScreen
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.ui.components.PrimaryButton
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme

/** Lo que muestran los ajustes abiertos (§5.4): solo esto. */
data class SettingsInfo(val serverAddress: String, val link: LinkState, val appVersion: String)

/**
 * Los ajustes de la tableta (§5.4): primero el usuario y la contraseña de un administrador; ya
 * abiertos, solo la dirección de la computadora principal, el estado de la conexión, la versión y
 * «Volver a emparejar» (con confirmación). Cada toque o tecla cuenta como uso para el cierre
 * automático.
 */
@Composable
fun SettingsDialog(screen: SettingsScreen, info: SettingsInfo, actions: SettingsActions) {
    if (screen == SettingsScreen.Closed) {
        return
    }
    Dialog(
        onDismissRequest = actions::close,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier =
                Modifier.widthIn(max = 640.dp)
                    .fillMaxWidth()
                    .padding(32.dp)
                    .reportsUserActivity(actions::touch)
                    .testTag(TAG_SETTINGS),
            shape = MaterialTheme.shapes.large,
            color = AdelTheme.colors.background,
        ) {
            Column(
                modifier = Modifier.padding(AdelDimens.ScreenPadding),
                verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
            ) {
                Text(
                    text = stringResource(R.string.settings_title),
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                    color = AdelTheme.colors.text,
                )
                when (screen) {
                    SettingsScreen.Closed -> Unit
                    is SettingsScreen.Password -> PasswordStep(screen, actions)
                    is SettingsScreen.Open -> OpenStep(screen, info, actions)
                    is SettingsScreen.Unpaired -> UnpairedStep(screen, actions)
                }
            }
        }
    }
}

@Composable
private fun PasswordStep(screen: SettingsScreen.Password, actions: SettingsActions) {
    // Lo escrito vive solo en esta pantalla: al cerrar los ajustes se olvida.
    var username by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    Text(
        text = stringResource(R.string.settings_password_help),
        style = MaterialTheme.typography.bodyLarge,
        color = AdelTheme.colors.muted,
    )
    OutlinedTextField(
        value = username,
        onValueChange = {
            username = it
            actions.touch()
        },
        label = { Text(stringResource(R.string.settings_username)) },
        singleLine = true,
        enabled = !screen.busy,
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
        modifier = Modifier.fillMaxWidth().testTag(TAG_SETTINGS_USERNAME),
    )
    OutlinedTextField(
        value = password,
        onValueChange = {
            password = it
            actions.touch()
        },
        label = { Text(stringResource(R.string.settings_password)) },
        singleLine = true,
        enabled = !screen.busy,
        visualTransformation = PasswordVisualTransformation(),
        keyboardOptions =
            KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { actions.submit(username, password) }),
        modifier = Modifier.fillMaxWidth().testTag(TAG_SETTINGS_PASSWORD),
    )
    ErrorText(screen.error)
    Row(
        horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        PrimaryButton(
            text = stringResource(R.string.settings_open),
            onClick = { actions.submit(username, password) },
            enabled = !screen.busy,
            modifier = Modifier.testTag(TAG_SETTINGS_SUBMIT),
        )
        SecondaryButton(
            text = stringResource(R.string.settings_cancel),
            onClick = actions::close,
            modifier = Modifier.testTag(TAG_SETTINGS_CLOSE),
        )
        if (screen.busy) {
            CircularProgressIndicator()
        }
    }
}

@Composable
private fun OpenStep(screen: SettingsScreen.Open, info: SettingsInfo, actions: SettingsActions) {
    InfoRow(
        label = stringResource(R.string.settings_server),
        value = info.serverAddress,
        tag = TAG_SETTINGS_SERVER,
    )
    InfoRow(
        label = stringResource(R.string.settings_connection),
        value =
            stringResource(
                if (info.link == LinkState.ONLINE) {
                    R.string.settings_online
                } else {
                    R.string.settings_offline
                },
            ),
        tag = TAG_SETTINGS_CONNECTION,
    )
    InfoRow(
        label = stringResource(R.string.settings_version),
        value = info.appVersion,
        tag = TAG_SETTINGS_VERSION,
    )
    HorizontalDivider(color = AdelTheme.colors.border)
    if (screen.confirmUnpair) {
        Text(
            text = stringResource(R.string.settings_unpair_question),
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
            color = AdelTheme.colors.text,
            modifier = Modifier.testTag(TAG_SETTINGS_UNPAIR_QUESTION),
        )
        ErrorText(screen.error)
        Row(
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            PrimaryButton(
                text = stringResource(R.string.settings_unpair_confirm),
                onClick = actions::confirmUnpair,
                enabled = !screen.busy,
                modifier = Modifier.testTag(TAG_SETTINGS_UNPAIR_CONFIRM),
            )
            SecondaryButton(
                text = stringResource(R.string.settings_cancel),
                onClick = actions::cancelUnpair,
                enabled = !screen.busy,
            )
            if (screen.busy) {
                CircularProgressIndicator()
            }
        }
    } else {
        ErrorText(screen.error)
        Row(horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
            SecondaryButton(
                text = stringResource(R.string.settings_unpair),
                onClick = actions::askUnpair,
                modifier = Modifier.testTag(TAG_SETTINGS_UNPAIR),
            )
            PrimaryButton(
                text = stringResource(R.string.settings_close),
                onClick = actions::close,
                modifier = Modifier.testTag(TAG_SETTINGS_CLOSE),
            )
        }
    }
}

@Composable
private fun UnpairedStep(screen: SettingsScreen.Unpaired, actions: SettingsActions) {
    Text(
        text = screen.message.orEmpty(),
        style = MaterialTheme.typography.titleMedium,
        color = AdelTheme.colors.text,
        modifier = Modifier.testTag(TAG_SETTINGS_UNPAIRED),
    )
    PrimaryButton(
        text = stringResource(R.string.settings_understood),
        onClick = actions::close,
        modifier = Modifier.testTag(TAG_SETTINGS_CLOSE),
    )
}

@Composable
private fun InfoRow(label: String, value: String, tag: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = AdelTheme.colors.muted,
        )
        Text(
            text = value,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
            color = AdelTheme.colors.text,
            modifier = Modifier.testTag(tag),
        )
    }
}

@Composable
private fun ErrorText(error: String?) {
    if (error != null) {
        Text(
            text = error,
            style = MaterialTheme.typography.bodyLarge,
            color = AdelTheme.colors.red,
            modifier = Modifier.testTag(TAG_SETTINGS_ERROR),
        )
    }
}

const val TAG_SETTINGS = "kitchen-settings"
const val TAG_SETTINGS_USERNAME = "kitchen-settings-username"
const val TAG_SETTINGS_PASSWORD = "kitchen-settings-password"
const val TAG_SETTINGS_SUBMIT = "kitchen-settings-submit"
const val TAG_SETTINGS_CLOSE = "kitchen-settings-close"
const val TAG_SETTINGS_ERROR = "kitchen-settings-error"
const val TAG_SETTINGS_SERVER = "kitchen-settings-server"
const val TAG_SETTINGS_CONNECTION = "kitchen-settings-connection"
const val TAG_SETTINGS_VERSION = "kitchen-settings-version"
const val TAG_SETTINGS_UNPAIR = "kitchen-settings-unpair"
const val TAG_SETTINGS_UNPAIR_QUESTION = "kitchen-settings-unpair-question"
const val TAG_SETTINGS_UNPAIR_CONFIRM = "kitchen-settings-unpair-confirm"
const val TAG_SETTINGS_UNPAIRED = "kitchen-settings-unpaired"
