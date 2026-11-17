package local.adel.pos.android.navigation

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import kotlinx.serialization.Serializable
import local.adel.pos.android.AdelPosApplication
import local.adel.pos.android.ShellViewModel
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.ui.components.AdelTopBar
import local.adel.pos.android.core.ui.layout.currentPaneLayout
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.AccessScreen
import local.adel.pos.android.feature.access.accessTitle
import local.adel.pos.android.feature.waiter.TablesRoute
import local.adel.pos.android.feature.waiter.catalog.CatalogSampleRoute
import local.adel.pos.android.feature.waiter.catalog.catalogSampleTitle
import local.adel.pos.android.feature.waiter.tablesTitle

/** Destinos de la navegación (rutas con tipo de navigation-compose). */
@Serializable data object AccessDestination

@Serializable data object TablesDestination

/** Pantalla de muestra del catálogo y el personalizador (Fase D). */
@Serializable data object CatalogDestination

/**
 * Raíz de la interfaz: tema, barra superior con el indicador de conexión y navegación entre
 * pantallas.
 */
@Composable
fun AdelPosApp(
    darkTheme: Boolean = isSystemInDarkTheme(),
    shellViewModel: ShellViewModel = viewModel(factory = ShellViewModel.Factory),
) {
    val connection by shellViewModel.connection.collectAsStateWithLifecycle()
    AdelTheme(darkTheme = darkTheme) { AdelPosNavigation(connection) }
}

/**
 * Navegación con la barra superior común. Atrás (botón, gesto o atrás predictivo) regresa por la
 * pila de navigation- compose; dentro de una pantalla de un panel, el BackHandler de AdaptivePanes
 * cierra primero el panel abierto.
 */
@Composable
fun AdelPosNavigation(connection: ConnectionSummary) {
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val destination = backStackEntry?.destination
    val title =
        when {
            destination?.hasRoute<TablesDestination>() == true -> tablesTitle()
            destination?.hasRoute<CatalogDestination>() == true -> catalogSampleTitle()
            else -> accessTitle()
        }
    val paneLayout = currentPaneLayout()
    Scaffold(
        modifier = Modifier.fillMaxSize().testTag(TAG_APP_ROOT),
        topBar = { AdelTopBar(title = title, connection = connection) },
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = AccessDestination,
            modifier = Modifier.padding(padding).consumeWindowInsets(padding),
        ) {
            composable<AccessDestination> {
                AccessScreen(
                    onEnter = {
                        navController.navigate(TablesDestination) { launchSingleTop = true }
                    },
                )
            }
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
}

const val TAG_APP_ROOT = "app-root"
