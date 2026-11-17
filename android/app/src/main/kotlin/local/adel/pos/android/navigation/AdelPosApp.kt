package local.adel.pos.android.navigation

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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
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
import kotlinx.serialization.Serializable
import local.adel.pos.android.AdelPosApplication
import local.adel.pos.android.ShellViewModel
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.ui.components.ActiveWaiterMenu
import local.adel.pos.android.core.ui.components.AdelTopBar
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.layout.currentPaneLayout
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.AccessActions
import local.adel.pos.android.feature.access.AccessContent
import local.adel.pos.android.feature.access.AccessDependencies
import local.adel.pos.android.feature.access.AccessViewModel
import local.adel.pos.android.feature.access.SwitchBlockedDialog
import local.adel.pos.android.feature.access.accessTitle
import local.adel.pos.android.feature.access.asActions
import local.adel.pos.android.feature.waiter.TablesRoute
import local.adel.pos.android.feature.waiter.catalog.CatalogSampleRoute
import local.adel.pos.android.feature.waiter.catalog.catalogSampleTitle
import local.adel.pos.android.feature.waiter.tablesTitle

/** Destinos del modo Mesero (rutas con tipo de navigation-compose). */
@Serializable data object TablesDestination

/** Pantalla de muestra del catálogo y el personalizador (Fase D). */
@Serializable data object CatalogDestination

/**
 * Raíz de la interfaz: tema, barra superior con el indicador de conexión y, según el estado del
 * dispositivo, la entrada (buscar el servidor, emparejar, PIN…) o el modo Mesero del mesero activo.
 * Con la aplicación a la vista se vuelve a comprobar el servidor cada minuto.
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
    AdelTheme(darkTheme = darkTheme) {
        AdelPosNavigation(
            connection = connection,
            status = status,
            activeWaiter = activeWaiter,
            dependencies = dependencies,
            actions = gate.asActions(),
            onSwitchWaiter = gate::switchWaiter,
        )
        if (gate.switchBlocked) {
            SwitchBlockedDialog(onDismiss = gate::dismissSwitchBlocked)
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
    val onCatalog = backStackEntry?.destination?.hasRoute<CatalogDestination>() == true
    val title =
        when {
            waiter == null -> accessTitle(status)
            onCatalog -> catalogSampleTitle()
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

@Composable
private fun WaiterNavigation(navController: NavHostController, paneLayout: PaneLayout) {
    NavHost(navController = navController, startDestination = TablesDestination) {
        composable<TablesDestination> {
            TablesRoute(
                layout = paneLayout,
                onOpenCatalog = {
                    navController.navigate(CatalogDestination) { launchSingleTop = true }
                },
            )
        }
        composable<CatalogDestination> {
            val application = LocalContext.current.applicationContext as AdelPosApplication
            val catalogRealtime = application.container.catalogRealtime
            CatalogSampleRoute(
                layout = paneLayout,
                source = catalogRealtime.catalog,
                images = catalogRealtime.images,
            )
        }
    }
}

const val TAG_APP_ROOT = "app-root"
