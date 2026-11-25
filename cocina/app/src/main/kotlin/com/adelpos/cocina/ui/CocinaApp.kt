package com.adelpos.cocina.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.adelpos.cocina.CocinaContainer
import com.adelpos.cocina.R
import com.adelpos.cocina.access.DisplayConnection
import com.adelpos.cocina.access.KitchenStatus
import com.adelpos.core.model.connection.ConnectionSummary
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.pairing.PairingFlow
import com.adelpos.core.ui.R as CoreR
import com.adelpos.core.ui.access.findserver.FindServerRoute
import com.adelpos.core.ui.access.status.LoadingScreen
import com.adelpos.core.ui.access.status.LocalNetworkPermissionGate
import com.adelpos.core.ui.access.status.RepairScreen
import com.adelpos.core.ui.access.status.UpdateAppScreen
import com.adelpos.core.ui.access.status.UpdateServerScreen
import com.adelpos.core.ui.components.AdelTopBar
import com.adelpos.core.ui.theme.AdelTheme
import kotlinx.coroutines.delay

/** Cada cuánto se vuelve a comprobar el servidor con la aplicación a la vista (como meseros). */
private const val RECHECK_MILLIS = 5 * 60 * 1000L

/**
 * Raíz de la aplicación de cocina. §4 de la etapa K1: tema oscuro por omisión (el claro se elige
 * por pantalla en Terminales, en la pantalla de pedidos).
 */
@Composable
fun CocinaApp(
    container: CocinaContainer,
    viewModel: CocinaViewModel = viewModel(factory = CocinaViewModel.factory(container)),
) {
    val status by viewModel.status.collectAsStateWithLifecycle()
    val link by viewModel.link.collectAsStateWithLifecycle()
    val display by viewModel.display.collectAsStateWithLifecycle()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                delay(RECHECK_MILLIS)
                viewModel.checkAgain()
            }
        }
    }
    AdelTheme(darkTheme = true) {
        CocinaScreen(
            status = status,
            link = link,
            display = display,
            pairing = container.pairing,
            onCheckAgain = viewModel::checkAgain,
            onRepair = viewModel::repair,
        )
    }
}

/**
 * La pantalla que corresponde a [status], con la barra común y su indicador de conexión siempre a
 * la vista. Las pantallas de la entrada son las compartidas con la aplicación de meseros.
 */
@Composable
fun CocinaScreen(
    status: KitchenStatus,
    link: LinkState,
    display: DisplayConnection,
    pairing: PairingFlow,
    onCheckAgain: () -> Unit,
    onRepair: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Scaffold(
        modifier = modifier,
        topBar = {
            AdelTopBar(
                title = title(status),
                connection = ConnectionSummary(link, pendingOrders = 0),
                logo = status !is KitchenStatus.Ready,
            )
        },
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { padding ->
        Box(modifier = Modifier.padding(padding).fillMaxSize()) {
            when (status) {
                KitchenStatus.Starting -> LoadingScreen()
                KitchenStatus.Unpaired ->
                    LocalNetworkPermissionGate {
                        FindServerRoute(
                            pairing = pairing,
                            deviceLog = emptyList(),
                            onClearDeviceLog = {},
                            defaultDeviceName = stringResource(R.string.device_default_name),
                        )
                    }
                is KitchenStatus.Ready -> OrdersScreen(display = display)
                is KitchenStatus.NeedsRepair ->
                    RepairScreen(
                        reason = status.reason,
                        onRepair = onRepair,
                        onCheckAgain = onCheckAgain,
                    )
                is KitchenStatus.UpdateApp ->
                    UpdateAppScreen(
                        appVersion = status.appVersion,
                        minClientVersion = status.minClientVersion,
                        onCheckAgain = onCheckAgain,
                    )
                is KitchenStatus.UpdateServer ->
                    UpdateServerScreen(
                        serverVersion = status.serverVersion,
                        onCheckAgain = onCheckAgain,
                    )
            }
        }
    }
}

@Composable
private fun title(status: KitchenStatus): String =
    stringResource(
        when (status) {
            KitchenStatus.Starting -> R.string.app_name
            KitchenStatus.Unpaired -> CoreR.string.access_title_find_server
            is KitchenStatus.Ready -> R.string.orders_title
            is KitchenStatus.NeedsRepair -> CoreR.string.access_title_repair
            is KitchenStatus.UpdateApp,
            is KitchenStatus.UpdateServer -> CoreR.string.access_title_update
        },
    )
