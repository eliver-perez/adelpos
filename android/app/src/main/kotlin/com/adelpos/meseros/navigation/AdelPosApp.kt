package com.adelpos.meseros.navigation

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.adelpos.meseros.AdelPosApplication
import com.adelpos.meseros.ShellViewModel
import com.adelpos.meseros.core.capture.CaptureRoute
import com.adelpos.meseros.core.capture.captureTitle
import com.adelpos.meseros.core.capture.pending.PendingOrdersRoute
import com.adelpos.meseros.core.capture.pending.ReviewRoute
import com.adelpos.meseros.core.capture.pending.pendingTitle
import com.adelpos.meseros.core.capture.pending.reviewTitle
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.connection.ConnectionSummary
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.ui.components.ActiveWaiterMenu
import com.adelpos.meseros.core.ui.components.AdelTopBar
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.currentPaneLayout
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.feature.access.AccessActions
import com.adelpos.meseros.feature.access.AccessContent
import com.adelpos.meseros.feature.access.AccessDependencies
import com.adelpos.meseros.feature.access.AccessViewModel
import com.adelpos.meseros.feature.access.SwitchBlockedDialog
import com.adelpos.meseros.feature.access.accessTitle
import com.adelpos.meseros.feature.access.asActions
import com.adelpos.meseros.feature.access.pin.ReenterPinRoute
import com.adelpos.meseros.feature.waiter.tables.TablesRoute
import com.adelpos.meseros.feature.waiter.tables.tablesTitle
import kotlinx.serialization.Serializable

/** Destinos del modo Mesero (rutas con tipo de navigation-compose). */
@Serializable data object TablesDestination

/** Captura de una cuenta (o de una cuenta nueva de la mesa) del mesero. */
@Serializable data class CaptureDestination(val captureId: String)

/** «Pedidos pendientes» del mesero. */
@Serializable data object PendingDestination

/** «Requiere tu revisión» de un pedido de la cola. */
@Serializable data class ReviewDestination(val operationId: String)

/**
 * Raíz de la interfaz: tema, barra superior con el indicador de conexión y, según el estado del
 * dispositivo, la entrada (buscar el servidor, emparejar, PIN…) o el modo Mesero del mesero activo.
 * Si el acceso del mesero activo dejó de valer, «Vuelve a escribir tu PIN» encima de su pantalla
 * (revisión R3, M3). Con la aplicación a la vista se vuelve a comprobar el servidor cada minuto.
 * Los toques dentro de los diálogos también cuentan para el bloqueo por inactividad
 * ([LocalUserActivity]; revisión R3, B4).
 */
@Composable
fun AdelPosApp(
    darkTheme: Boolean = isSystemInDarkTheme(),
    shellViewModel: ShellViewModel = viewModel(factory = ShellViewModel.Factory),
    dependencies: AccessDependencies =
        (LocalContext.current.applicationContext as AdelPosApplication)
            .container
            .accessDependencies,
    gate: AccessViewModel = viewModel(factory = AccessViewModel.factory(dependencies)),
) {
    val connection by shellViewModel.connection.collectAsStateWithLifecycle()
    val status by gate.status.collectAsStateWithLifecycle()
    val activeWaiter by gate.activeWaiter.collectAsStateWithLifecycle()
    LifecycleStartEffect(gate) {
        gate.onForeground()
        onStopOrDispose { gate.onBackground() }
    }
    val idleLock = dependencies.idleLock
    val onUserActivity: () -> Unit = remember(idleLock) { { idleLock.touch() } }
    CompositionLocalProvider(LocalUserActivity provides onUserActivity) {
        AdelTheme(darkTheme = darkTheme) {
            AdelPosNavigation(
                connection = connection,
                status = status,
                activeWaiter = activeWaiter,
                dependencies = dependencies,
                actions = gate.asActions(),
                onSwitchWaiter = gate::switchWaiter,
            )
            val reentering = activeWaiter?.takeIf { it.mustReenterPin }
            if (reentering != null && status is DeviceStatus.Ready) {
                ReenterPinRoute(
                    access = dependencies.access,
                    waiter = reentering,
                    onSwitchWaiter = gate::switchWaiter,
                )
            }
            // Después del de volver a escribir el PIN, para quedar encima de él.
            if (gate.switchBlocked) {
                SwitchBlockedDialog(onDismiss = gate::dismissSwitchBlocked)
            }
        }
    }
}

/**
 * La barra superior común y el contenido. Con un mesero activo (y el dispositivo listo), el modo
 * Mesero con su menú «Cambiar de mesero» en la barra; si no, la pantalla de entrada que toque.
 * Atrás (botón, gesto o atrás predictivo) regresa por la pila de navigation-compose; dentro de una
 * pantalla de un panel, el BackHandler de AdaptivePanes cierra primero el panel abierto.
 */
@Composable
fun AdelPosNavigation(
    connection: ConnectionSummary,
    status: DeviceStatus,
    activeWaiter: ActiveWaiter?,
    dependencies: AccessDependencies,
    actions: AccessActions,
    onSwitchWaiter: () -> Unit,
) {
    val paneLayout = currentPaneLayout()
    val waiter = activeWaiter?.takeIf { status is DeviceStatus.Ready }
    // Cada mesero empieza su propia navegación: lo que capturó otro no se ve aquí.
    val navController = key(waiter?.userId) { rememberNavController() }
    val backStackEntry by navController.currentBackStackEntryAsState()
    val destination = backStackEntry?.destination
    val title =
        when {
            waiter == null -> accessTitle(status)
            destination?.hasRoute<CaptureDestination>() == true -> captureTitle()
            destination?.hasRoute<PendingDestination>() == true -> pendingTitle()
            destination?.hasRoute<ReviewDestination>() == true -> reviewTitle()
            else -> tablesTitle()
        }
    Scaffold(
        modifier = Modifier.fillMaxSize().testTag(TAG_APP_ROOT),
        topBar = {
            AdelTopBar(
                title = title,
                connection = connection,
                actions = {
                    if (waiter != null) {
                        ActiveWaiterMenu(
                            name = waiter.profile.name,
                            initial = waiter.profile.initial,
                            colorHex = waiter.profile.color,
                            onSwitchWaiter = onSwitchWaiter,
                            offline = !waiter.hasServerSession,
                        )
                    }
                },
            )
        },
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { padding ->
        Box(modifier = Modifier.padding(padding).consumeWindowInsets(padding)) {
            if (waiter != null) {
                key(waiter.userId) { WaiterNavigation(navController, paneLayout) }
            } else {
                AccessContent(
                    status = status,
                    dependencies = dependencies,
                    actions = actions,
                    layout = paneLayout,
                )
            }
        }
    }
}

/**
 * El modo Mesero (Fase E): Mesas → la captura de una cuenta; «Pedidos pendientes» y «Requiere tu
 * revisión». Después de «Enviar a cocina» se vuelve a Mesas (el pedido sigue en la cola con su
 * estado).
 */
@Composable
private fun WaiterNavigation(navController: NavHostController, paneLayout: PaneLayout) {
    val application = LocalContext.current.applicationContext as AdelPosApplication
    val waiter = application.container.waiterDependencies
    val openCapture: (String) -> Unit = { captureId ->
        navController.navigate(CaptureDestination(captureId)) { launchSingleTop = true }
    }
    val openReview: (String) -> Unit = { operationId ->
        navController.navigate(ReviewDestination(operationId)) { launchSingleTop = true }
    }
    NavHost(navController = navController, startDestination = TablesDestination) {
        composable<TablesDestination> {
            TablesRoute(
                layout = paneLayout,
                dependencies = waiter,
                onOpenCapture = openCapture,
                onOpenPending = {
                    navController.navigate(PendingDestination) { launchSingleTop = true }
                },
                onOpenOrder = { order ->
                    if (order.state == OrderState.NEEDS_REVIEW) {
                        openReview(order.operationId)
                    } else {
                        navController.navigate(PendingDestination) { launchSingleTop = true }
                    }
                },
            )
        }
        composable<CaptureDestination> {
            CaptureRoute(
                layout = paneLayout,
                dependencies = waiter.capture,
                onDone = { navController.popBackStack(TablesDestination, inclusive = false) },
            )
        }
        composable<PendingDestination> {
            PendingOrdersRoute(
                dependencies = waiter.capture,
                onOpenCapture = openCapture,
                onOpenReview = openReview,
            )
        }
        composable<ReviewDestination> {
            ReviewRoute(
                dependencies = waiter.capture,
                onDone = { navController.popBackStack() },
                onOpenCapture = { captureId ->
                    navController.popBackStack()
                    openCapture(captureId)
                },
            )
        }
    }
}

const val TAG_APP_ROOT = "app-root"
