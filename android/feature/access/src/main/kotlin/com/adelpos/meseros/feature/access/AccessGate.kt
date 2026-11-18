package com.adelpos.meseros.feature.access

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.UnsentOrder
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.feature.access.findserver.FindServerRoute
import com.adelpos.meseros.feature.access.pin.PinEntryRoute
import com.adelpos.meseros.feature.access.status.LoadingScreen
import com.adelpos.meseros.feature.access.status.LocalNetworkPermissionGate
import com.adelpos.meseros.feature.access.status.RepairScreen
import com.adelpos.meseros.feature.access.status.RetiredScreen
import com.adelpos.meseros.feature.access.status.UpdateAppScreen
import com.adelpos.meseros.feature.access.status.UpdateServerScreen

/** Lo que la puerta de entrada pide hacer (lo implementa [AccessViewModel]). */
interface AccessActions {
    fun recheck()

    fun repair()

    /** Pedidos sin enviar que pasarán a la bitácora si se empareja de nuevo. */
    suspend fun pendingOrders(): List<UnsentOrder>

    fun acknowledgeRetirement()

    fun clearDeviceLog()
}

/** [AccessViewModel] como [AccessActions]. */
fun AccessViewModel.asActions(): AccessActions {
    val viewModel = this
    return object : AccessActions {
        override fun recheck() = viewModel.recheck()

        override fun repair() = viewModel.repair()

        override suspend fun pendingOrders(): List<UnsentOrder> = viewModel.pendingOrders()

        override fun acknowledgeRetirement() = viewModel.acknowledgeRetirement()

        override fun clearDeviceLog() = viewModel.clearDeviceLog()
    }
}

/**
 * La pantalla de entrada que corresponde al estado del dispositivo, cuando todavía no hay un mesero
 * activo: buscar el servidor y emparejar, entrar con PIN, emparejar de nuevo, actualizar o el aviso
 * de dispositivo retirado. Con un mesero activo, la aplicación muestra el modo Mesero.
 * [requestedWaiter] (etapa A2): el mesero de un aviso de platillos listos que se tocó; la entrada
 * con PIN lo deja elegido.
 */
@Composable
fun AccessContent(
    status: DeviceStatus,
    dependencies: AccessDependencies,
    actions: AccessActions,
    layout: PaneLayout,
    modifier: Modifier = Modifier,
    requestedWaiter: String? = null,
) {
    when (status) {
        DeviceStatus.Starting -> LoadingScreen(modifier)
        is DeviceStatus.Unpaired ->
            LocalNetworkPermissionGate {
                FindServerRoute(
                    dependencies = dependencies,
                    deviceLog = status.deviceLog,
                    onClearDeviceLog = actions::clearDeviceLog,
                )
            }
        is DeviceStatus.Ready ->
            PinEntryRoute(
                dependencies = dependencies,
                layout = layout,
                requestedWaiter = requestedWaiter,
            )
        is DeviceStatus.NeedsRepair ->
            RepairScreen(
                reason = status.reason,
                onRepair = actions::repair,
                loadPendingOrders = actions::pendingOrders,
                onCheckAgain = actions::recheck,
                modifier = modifier,
            )
        is DeviceStatus.UpdateApp ->
            UpdateAppScreen(
                appVersion = status.appVersion,
                minClientVersion = status.minClientVersion,
                onCheckAgain = actions::recheck,
                modifier = modifier,
                loadPendingOrders = actions::pendingOrders,
            )
        is DeviceStatus.UpdateServer ->
            UpdateServerScreen(
                serverVersion = status.serverVersion,
                onCheckAgain = actions::recheck,
                modifier = modifier,
            )
        is DeviceStatus.Retired ->
            RetiredScreen(
                unsentOrders = status.unsentOrders,
                onPairAgain = actions::acknowledgeRetirement,
                onClearDeviceLog = actions::clearDeviceLog,
                modifier = modifier,
            )
    }
}

/** Título de la barra superior para cada estado de la entrada. */
@Composable
fun accessTitle(status: DeviceStatus): String =
    stringResource(
        when (status) {
            DeviceStatus.Starting -> R.string.access_title_loading
            is DeviceStatus.Unpaired -> R.string.access_title_find_server
            is DeviceStatus.Ready -> R.string.access_title_entry
            is DeviceStatus.NeedsRepair -> R.string.access_title_repair
            is DeviceStatus.UpdateApp,
            is DeviceStatus.UpdateServer -> R.string.access_title_update
            is DeviceStatus.Retired -> R.string.access_title_retired
        },
    )

/** Aviso de que no se puede cambiar de mesero con pedidos sin enviar (§7). */
@Composable
fun SwitchBlockedDialog(onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        title = { Text(stringResource(R.string.switch_blocked_title)) },
        text = { Text(stringResource(R.string.switch_blocked_text)) },
        confirmButton = {
            TextButton(onClick = onDismiss, modifier = Modifier.testTag(TAG_SWITCH_BLOCKED_OK)) {
                Text(stringResource(R.string.common_understood))
            }
        },
    )
}

const val TAG_SWITCH_BLOCKED_OK = "switch-blocked-ok"
