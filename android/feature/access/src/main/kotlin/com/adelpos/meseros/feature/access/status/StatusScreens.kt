package com.adelpos.meseros.feature.access.status

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.access.RepairReason
import com.adelpos.meseros.core.domain.access.UnsentOrder
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.PrimaryButton
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.feature.access.R
import com.adelpos.meseros.feature.access.clockTime
import com.adelpos.meseros.feature.access.deviceWords
import com.adelpos.meseros.feature.access.findserver.CenteredColumn
import com.adelpos.meseros.feature.access.findserver.ProblemNotice

/** Mientras se lee lo guardado. */
@Composable
fun LoadingScreen(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier.fillMaxSize().testTag(TAG_LOADING),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            CircularProgressIndicator()
            Text(stringResource(R.string.access_loading))
        }
    }
}

/**
 * «Esta tableta necesita emparejarse de nuevo» (§4, como `RepairTerminal` del escritorio): el
 * servidor cambió de identidad (nunca se conecta a otro certificado), ya no reconoce el dispositivo
 * o la autorización guardada ya no se puede leer. «Emparejar de nuevo» pide confirmación, dice qué
 * se borra de este dispositivo y muestra antes los pedidos sin enviar, que pasan a la bitácora
 * (revisión R3, M4).
 */
@Composable
fun RepairScreen(
    reason: RepairReason,
    onRepair: () -> Unit,
    onCheckAgain: () -> Unit,
    modifier: Modifier = Modifier,
    loadPendingOrders: suspend () -> List<UnsentOrder> = { emptyList() },
) {
    val words = deviceWords()
    var confirming by rememberSaveable { mutableStateOf(false) }
    var pending by remember { mutableStateOf(emptyList<UnsentOrder>()) }
    LaunchedEffect(confirming) { pending = if (confirming) loadPendingOrders() else emptyList() }
    CenteredColumn(modifier = modifier.testTag(TAG_REPAIR)) {
        Text(
            text = stringResource(R.string.repair_title, words.thisDeviceCapital),
            style = MaterialTheme.typography.headlineSmall,
        )
        val explanation =
            when (reason) {
                RepairReason.IDENTITY_CHANGED ->
                    stringResource(
                        if (words.isTablet) R.string.repair_identity_changed
                        else R.string.repair_identity_changed_phone,
                        words.thisDevice,
                    )
                RepairReason.NOT_RECOGNIZED ->
                    stringResource(R.string.repair_not_recognized, words.thisDevice)
                RepairReason.CREDENTIAL_LOST ->
                    stringResource(R.string.repair_credential_lost, words.thisDeviceCapital)
            }
        ProblemNotice(explanation)
        PrimaryButton(
            text = stringResource(R.string.repair_action),
            onClick = { confirming = true },
            modifier = Modifier.fillMaxWidth().testTag(TAG_REPAIR_ACTION),
        )
        // Un cambio de identidad puede ser pasajero (otro equipo tomó la dirección un momento):
        // se puede volver a comprobar sin perder el emparejamiento.
        if (reason == RepairReason.IDENTITY_CHANGED) {
            SecondaryButton(
                text = stringResource(R.string.repair_check_again),
                onClick = onCheckAgain,
                icon = AdelIcons.Refresh,
                modifier = Modifier.fillMaxWidth().testTag(TAG_CHECK_AGAIN),
            )
        }
    }
    if (confirming) {
        AlertDialog(
            onDismissRequest = { confirming = false },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
            title = { Text(stringResource(R.string.repair_confirm_title)) },
            text = {
                Column(
                    modifier = Modifier.verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text =
                            stringResource(R.string.repair_confirm_text, words.thisDeviceCapital),
                        modifier = Modifier.testTag(TAG_REPAIR_ERASES),
                    )
                    if (pending.isNotEmpty()) {
                        Text(
                            text = stringResource(R.string.repair_confirm_pending),
                            color = AdelTheme.colors.red,
                        )
                        Column(modifier = Modifier.testTag(TAG_REPAIR_PENDING)) {
                            for (order in pending) {
                                Text(orderLine(order))
                            }
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirming = false
                        onRepair()
                    },
                    modifier = Modifier.testTag(TAG_REPAIR_CONFIRM),
                ) {
                    Text(stringResource(R.string.repair_action))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirming = false }) {
                    Text(stringResource(R.string.common_back))
                }
            },
        )
    }
}

/**
 * «Actualiza esta aplicación» (426 o la mínima de Android del servidor) con «Volver a comprobar».
 * Si hay pedidos sin enviar se muestran con su aviso (Fase E): siguen en el dispositivo y salen al
 * instalar la versión nueva encima, pero **desinstalar** la aplicación los perdería.
 */
@Composable
fun UpdateAppScreen(
    appVersion: String,
    minClientVersion: String?,
    onCheckAgain: () -> Unit,
    modifier: Modifier = Modifier,
    loadPendingOrders: suspend () -> List<UnsentOrder> = { emptyList() },
) {
    val words = deviceWords()
    var pending by remember { mutableStateOf(emptyList<UnsentOrder>()) }
    LaunchedEffect(Unit) { pending = loadPendingOrders() }
    CenteredColumn(modifier = modifier.testTag(TAG_UPDATE_APP)) {
        Text(
            text = stringResource(R.string.update_app_title),
            style = MaterialTheme.typography.headlineSmall,
        )
        Text(
            text = stringResource(R.string.update_app_text, words.thisDevice),
            style = MaterialTheme.typography.bodyLarge,
        )
        if (pending.isNotEmpty()) {
            Text(
                text = stringResource(R.string.update_app_pending, words.thisDevice),
                color = AdelTheme.colors.red,
            )
            Column(modifier = Modifier.testTag(TAG_UPDATE_APP_PENDING)) {
                for (order in pending) {
                    Text(orderLine(order))
                }
            }
        }
        PrimaryButton(
            text = stringResource(R.string.update_check_again),
            onClick = onCheckAgain,
            icon = AdelIcons.Refresh,
            modifier = Modifier.fillMaxWidth().testTag(TAG_CHECK_AGAIN),
        )
        SupportSection(
            listOfNotNull(
                stringResource(R.string.support_app_version, appVersion),
                minClientVersion?.let { stringResource(R.string.support_min_version, it) },
            ),
        )
    }
}

/**
 * «Actualiza el servidor del restaurante» (anterior a 0.13.0 o sin la entrada con PIN o Para
 * llevar).
 */
@Composable
fun UpdateServerScreen(
    serverVersion: String?,
    onCheckAgain: () -> Unit,
    modifier: Modifier = Modifier,
    onChooseAnother: (() -> Unit)? = null,
) {
    CenteredColumn(modifier = modifier.testTag(TAG_UPDATE_SERVER)) {
        Text(
            text = stringResource(R.string.update_server_title),
            style = MaterialTheme.typography.headlineSmall,
        )
        Text(
            text = stringResource(R.string.update_server_text),
            style = MaterialTheme.typography.bodyLarge,
        )
        PrimaryButton(
            text = stringResource(R.string.update_check_again),
            onClick = onCheckAgain,
            icon = AdelIcons.Refresh,
            modifier = Modifier.fillMaxWidth().testTag(TAG_CHECK_AGAIN),
        )
        // Al emparejar: puede que haya otra computadora principal, ya actualizada.
        if (onChooseAnother != null) {
            SecondaryButton(
                text = stringResource(R.string.update_server_choose_another),
                onClick = onChooseAnother,
                modifier = Modifier.fillMaxWidth().testTag(TAG_CHOOSE_ANOTHER),
            )
        }
        SupportSection(
            listOfNotNull(
                serverVersion?.let { stringResource(R.string.support_server_version, it) },
            ),
        )
    }
}

// Los datos técnicos solo aquí, aparte y en letra pequeña.
@Composable
private fun SupportSection(lines: List<String>) {
    if (lines.isEmpty()) {
        return
    }
    Column(
        modifier = Modifier.padding(top = 16.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = stringResource(R.string.support_title),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        for (line in lines) {
            Text(
                text = line,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * «Este dispositivo fue retirado del restaurante» (§3.2): ya se borraron sus datos y su
 * autorización. Si tenía pedidos sin enviar, los muestra con el aviso; quedan en la bitácora hasta
 * que alguien la borra.
 */
@Composable
fun RetiredScreen(
    unsentOrders: List<UnsentOrder>,
    onPairAgain: () -> Unit,
    onClearDeviceLog: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val words = deviceWords()
    CenteredColumn(modifier = modifier.testTag(TAG_RETIRED)) {
        Text(
            text = stringResource(R.string.retired_title),
            style = MaterialTheme.typography.headlineSmall,
        )
        Text(
            text = stringResource(R.string.retired_text, words.thisDevice),
            style = MaterialTheme.typography.bodyLarge,
        )
        if (unsentOrders.isNotEmpty()) {
            ProblemNotice(stringResource(R.string.retired_unsent))
            DeviceLogList(unsentOrders, onClearDeviceLog)
            Text(
                text = stringResource(R.string.retired_log_kept),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        PrimaryButton(
            text = stringResource(R.string.retired_pair_again),
            onClick = onPairAgain,
            modifier = Modifier.fillMaxWidth().testTag(TAG_RETIRED_PAIR_AGAIN),
        )
    }
}

/**
 * Pedidos de la bitácora (quién, cuándo y qué) y «Borrar la bitácora» con confirmación: nunca
 * llegaron a la cocina.
 */
@Composable
fun DeviceLogList(orders: List<UnsentOrder>, onClear: () -> Unit) {
    var confirming by rememberSaveable { mutableStateOf(false) }
    Column(
        modifier = Modifier.fillMaxWidth().testTag(TAG_DEVICE_LOG),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        for (order in orders) {
            Text(
                text = orderLine(order),
                style = MaterialTheme.typography.bodyLarge,
                color = AdelTheme.colors.text,
            )
        }
        SecondaryButton(
            text = stringResource(R.string.log_clear),
            onClick = { confirming = true },
            modifier = Modifier.testTag(TAG_DEVICE_LOG_CLEAR),
        )
    }
    if (confirming) {
        AlertDialog(
            onDismissRequest = { confirming = false },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
            title = { Text(stringResource(R.string.log_clear_confirm_title)) },
            text = { Text(stringResource(R.string.log_clear_confirm_text)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirming = false
                        onClear()
                    },
                ) {
                    Text(stringResource(R.string.log_clear))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirming = false }) {
                    Text(stringResource(R.string.common_back))
                }
            },
        )
    }
}

// Un pedido sin enviar: a qué hora, de quién y qué.
@Composable
private fun orderLine(order: UnsentOrder): String =
    stringResource(
        R.string.log_order_line,
        clockTime(order.capturedAtEpochMillis),
        order.waiterName,
        order.description,
    )

/** Etiquetas para las pruebas. */
const val TAG_LOADING = "access-loading"
const val TAG_REPAIR = "access-repair"
const val TAG_REPAIR_ACTION = "access-repair-action"
const val TAG_REPAIR_CONFIRM = "access-repair-confirm"
const val TAG_REPAIR_ERASES = "access-repair-erases"
const val TAG_REPAIR_PENDING = "access-repair-pending"
const val TAG_UPDATE_APP_PENDING = "access-update-app-pending"
const val TAG_CHECK_AGAIN = "access-check-again"
const val TAG_UPDATE_APP = "access-update-app"
const val TAG_UPDATE_SERVER = "access-update-server"
const val TAG_CHOOSE_ANOTHER = "access-choose-another"
const val TAG_RETIRED = "access-retired"
const val TAG_RETIRED_PAIR_AGAIN = "access-retired-pair-again"
const val TAG_DEVICE_LOG = "access-device-log"
const val TAG_DEVICE_LOG_CLEAR = "access-device-log-clear"
