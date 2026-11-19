package com.adelpos.meseros.navigation

import androidx.activity.compose.LocalOnBackPressedDispatcherOwner
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavBackStackEntry
import androidx.navigation.NavDestination.Companion.hasRoute
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.adelpos.meseros.AdelPosApplication
import com.adelpos.meseros.R
import com.adelpos.meseros.ShellViewModel
import com.adelpos.meseros.core.capture.CaptureRoute
import com.adelpos.meseros.core.capture.pending.PendingOrdersRoute
import com.adelpos.meseros.core.capture.pending.ReviewRoute
import com.adelpos.meseros.core.capture.pending.pendingTitle
import com.adelpos.meseros.core.capture.pending.reviewTitle
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.connection.ConnectionSummary
import com.adelpos.meseros.core.domain.waiter.DeviceHome
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.ui.components.ActiveWaiterMenu
import com.adelpos.meseros.core.ui.components.AdelTopBar
import com.adelpos.meseros.core.ui.components.LocalTopBarBack
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.TAG_TOP_BAR_BACK
import com.adelpos.meseros.core.ui.components.TopBarBack
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.layout.NavigationPlacement
import com.adelpos.meseros.core.ui.layout.PaneLayout
import com.adelpos.meseros.core.ui.layout.TAG_PANE_BACK
import com.adelpos.meseros.core.ui.layout.currentNavigationPlacement
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
import com.adelpos.meseros.feature.takeaway.TakeawayRoute
import com.adelpos.meseros.feature.takeaway.takeawayTitle
import com.adelpos.meseros.feature.waiter.tables.TablesRoute
import com.adelpos.meseros.feature.waiter.tables.tablesTitle
import com.adelpos.meseros.ready.ReadyAlertsController
import com.adelpos.meseros.ready.ReadyAlertsNavigationEffects
import com.adelpos.meseros.ready.ReadyAlertsOffNotice
import com.adelpos.meseros.ready.ReadyAlertsOverlay
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.Serializable

/** Destinos del modo Mesero (rutas con tipo de navigation-compose). */
@Serializable data object TablesDestination

/** «Para llevar» (etapa A2): la lista de pedidos para llevar. */
@Serializable data object TakeawayDestination

/**
 * Captura de una cuenta (o de una cuenta nueva de la mesa) o, con [takeaway], de un pedido para
 * llevar del mesero.
 */
@Serializable data class CaptureDestination(val captureId: String, val takeaway: Boolean = false)

/** «Pendientes» del mesero. */
@Serializable data object PendingDestination

/** «Requiere tu revisión» de un pedido de la cola (desde Para llevar, con [takeaway]). */
@Serializable data class ReviewDestination(val operationId: String, val takeaway: Boolean = false)

/** Las dos secciones de la navegación principal (§3.1 de la A2). */
enum class MainSection {
    TABLES,
    TAKEAWAY,
}

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
            // Etapa A2, Fase D: permisos, ayuda y aviso dentro de la aplicación.
            ReadyAlertsOverlay(readyAlerts())
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
    val navigationPlacement = currentNavigationPlacement()
    val waiter = activeWaiter?.takeIf { status is DeviceStatus.Ready }
    val readyAlerts = readyAlerts()
    // Un aviso tocado en «Selecciona tu usuario»: se le pide el PIN a su mesero (etapa A2, §4.4).
    val pendingOpen by readyAlerts.pendingOpen.collectAsStateWithLifecycle()
    // Cada mesero empieza su propia navegación: lo que capturó otro no se ve aquí.
    val navController = key(waiter?.userId) { rememberNavController() }
    val backStackEntry by navController.currentBackStackEntryAsState()
    val destination = backStackEntry?.destination
    // Etapa A3 (§4.2): la captura dice dónde se está («Mesa 4», «Para llevar #12»); la informa
    // CaptureRoute al leer su captura.
    var captureTitle by remember(waiter?.userId) { mutableStateOf<String?>(null) }
    val inCapture = destination?.hasRoute<CaptureDestination>() == true
    val captureFromTakeaway =
        inCapture && backStackEntry?.toRoute<CaptureDestination>()?.takeaway == true
    val title =
        when {
            waiter == null -> accessTitle(status)
            inCapture -> captureTitle ?: if (captureFromTakeaway) takeawayTitle() else tablesTitle()
            destination?.hasRoute<PendingDestination>() == true -> pendingTitle()
            destination?.hasRoute<ReviewDestination>() == true -> reviewTitle()
            destination?.hasRoute<TakeawayDestination>() == true -> takeawayTitle()
            else -> tablesTitle()
        }
    // Etapa A3 (§4.1): la flecha de regreso en toda pantalla que no sea la principal (captura,
    // pendientes, revisión, Para llevar encima de Mesas) y con un panel «a un toque» abierto en el
    // teléfono (la mesa o la cuenta). Llama al Atrás del sistema: pasa por los mismos BackHandler
    // (primero el del panel; al salir de una captura con productos sin enviar, el que lo avisa).
    val topBarBack = remember { TopBarBack() }
    val notices = remember { SnackbarHostState() }
    // Un aviso es de la pantalla a la que se vuelve (al salir de una captura con productos sin
    // enviar): se muestra al llegar a ella y se quita al pasar a otra, para que nunca tape los
    // botones de abajo de la siguiente (en el teléfono, «Ver la cuenta» y «Enviar a cocina»).
    var arrivalNotice by remember { mutableStateOf<String?>(null) }
    val showNotice: (String) -> Unit = { text -> arrivalNotice = text }
    LaunchedEffect(backStackEntry?.id) {
        val text = arrivalNotice ?: return@LaunchedEffect
        arrivalNotice = null
        // Al cambiar de pantalla esta espera se cancela y el aviso se quita con ella.
        notices.showSnackbar(text)
    }
    val backDispatcher = LocalOnBackPressedDispatcherOwner.current?.onBackPressedDispatcher
    val notMain =
        inCapture ||
            destination?.hasRoute<PendingDestination>() == true ||
            destination?.hasRoute<ReviewDestination>() == true ||
            destination?.hasRoute<TakeawayDestination>() == true &&
                navController.previousBackStackEntry != null
    val onBack: (() -> Unit)? =
        if (waiter != null && backDispatcher != null && (notMain || topBarBack.panelOpen)) {
            { backDispatcher.onBackPressed() }
        } else {
            null
        }
    // §3.1 de la A2: «Mesas» y «Para llevar» siempre a la vista, si Go le da al mesero activo
    // permiso de ver los pedidos para llevar.
    val takeawayVisible = waiter != null && takeawayVisible()
    val section = backStackEntry?.let(::sectionOf) ?: MainSection.TABLES
    if (waiter != null && !takeawayVisible && section == MainSection.TAKEAWAY) {
        // Perdió el permiso: vuelve a Mesas (lo capturado sigue guardado).
        LaunchedEffect(waiter.userId) {
            navController.popBackStack(TablesDestination, inclusive = false)
        }
    }
    Scaffold(
        modifier = Modifier.fillMaxSize().testTag(TAG_APP_ROOT),
        topBar = {
            AdelTopBar(
                title = title,
                connection = connection,
                onBack = onBack,
                // Con un panel abierto en el teléfono la flecha hace lo que hacía su «Volver a …»
                // (las pruebas de punta a punta lo buscan por esa etiqueta).
                backTag = if (topBarBack.panelOpen) TAG_PANE_BACK else TAG_TOP_BAR_BACK,
                // Etapa A3 (§4.5): el logotipo junto al título en la pantalla de entrada.
                logo = waiter == null && status is DeviceStatus.Ready,
                actions = {
                    if (waiter != null) {
                        ActiveWaiterMenu(
                            name = waiter.profile.name,
                            initial = waiter.profile.initial,
                            colorHex = waiter.profile.color,
                            onSwitchWaiter = onSwitchWaiter,
                            offline = !waiter.hasServerSession,
                            onAlertsHelp = { readyAlerts.helpRequested.value = true },
                        )
                    }
                },
            )
        },
        bottomBar = {
            // En vertical, abajo; con poca altura o en horizontal, el riel lateral (a un lado del
            // contenido, más abajo), para no dejar la captura sin espacio (Fase E de la A2).
            if (takeawayVisible && navigationPlacement == NavigationPlacement.BOTTOM_BAR) {
                MainNavigationBar(
                    selected = section,
                    onSelect = { chosen -> selectSection(navController, chosen) },
                )
            }
        },
        snackbarHost = { SnackbarHost(notices, modifier = Modifier.testTag(TAG_APP_NOTICE)) },
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets.safeDrawing,
    ) { padding ->
        Box(modifier = Modifier.padding(padding).consumeWindowInsets(padding)) {
            if (waiter != null) {
                Row {
                    if (takeawayVisible && navigationPlacement == NavigationPlacement.SIDE_RAIL) {
                        MainNavigationRail(
                            selected = section,
                            onSelect = { chosen -> selectSection(navController, chosen) },
                        )
                    }
                    Column(modifier = Modifier.weight(1f)) {
                        // Etapa A2, Fase D: sin notificaciones, un aviso discreto arriba de Mesas.
                        if (destination?.hasRoute<TablesDestination>() == true) {
                            ReadyAlertsOffNotice(readyAlerts)
                        }
                        Box(modifier = Modifier.weight(1f)) {
                            CompositionLocalProvider(LocalTopBarBack provides topBarBack) {
                                key(waiter.userId) {
                                    WaiterNavigation(
                                        navController = navController,
                                        paneLayout = paneLayout,
                                        onCaptureTitle = { captureTitle = it },
                                        onNotice = showNotice,
                                    )
                                }
                            }
                        }
                    }
                }
            } else {
                AccessContent(
                    status = status,
                    dependencies = dependencies,
                    actions = actions,
                    layout = paneLayout,
                    requestedWaiter = pendingOpen?.waiterUserId,
                )
            }
        }
    }
}

/** El aviso de platillos listos del proceso (etapa A2, Fase D). */
@Composable
private fun readyAlerts(): ReadyAlertsController =
    (LocalContext.current.applicationContext as AdelPosApplication).container.readyAlerts

/** Si Go le da al mesero activo permiso de ver los pedidos para llevar (`takeaway.read`). */
@Composable
private fun takeawayVisible(): Boolean {
    val application = LocalContext.current.applicationContext as AdelPosApplication
    val permissions by
        application.container.waiterDependencies.restaurant.permissions
            .collectAsStateWithLifecycle()
    return permissions.grants(WaiterAction.VIEW_TAKEAWAY)
}

/** La sección de la navegación principal a la que pertenece una pantalla. */
private fun sectionOf(entry: NavBackStackEntry): MainSection {
    val destination = entry.destination
    val takeaway =
        when {
            destination.hasRoute<TakeawayDestination>() -> true
            destination.hasRoute<CaptureDestination>() ->
                entry.toRoute<CaptureDestination>().takeaway
            destination.hasRoute<ReviewDestination>() -> entry.toRoute<ReviewDestination>().takeaway
            else -> false
        }
    return if (takeaway) MainSection.TAKEAWAY else MainSection.TABLES
}

/**
 * Cambiar de sección: Mesas es el inicio; Para llevar va encima de Mesas. Lo capturado no se pierde
 * al cambiar (está en el dispositivo y se ve en su lista).
 */
private fun selectSection(navController: NavHostController, section: MainSection) {
    when (section) {
        MainSection.TABLES -> navController.popBackStack(TablesDestination, inclusive = false)
        MainSection.TAKEAWAY ->
            navController.navigate(TakeawayDestination) {
                popUpTo<TablesDestination> { inclusive = false }
                launchSingleTop = true
            }
    }
}

/**
 * La navegación principal con poca altura o en horizontal: los mismos dos destinos, íconos y textos
 * de [MainNavigationBar] en un riel lateral (`NavigationRail` de Material 3), centrados. Va dentro
 * del contenido, que ya respeta los bordes del sistema (sin márgenes propios).
 */
@Composable
private fun MainNavigationRail(selected: MainSection, onSelect: (MainSection) -> Unit) {
    Box(modifier = Modifier.fillMaxHeight().testTag(TAG_MAIN_NAVIGATION)) {
        NavigationRail(
            windowInsets = WindowInsets(0, 0, 0, 0),
            modifier = Modifier.fillMaxHeight().testTag(TAG_NAVIGATION_RAIL),
        ) {
            Spacer(modifier = Modifier.weight(1f))
            NavigationRailItem(
                selected = selected == MainSection.TABLES,
                onClick = { onSelect(MainSection.TABLES) },
                icon = { Icon(AdelIcons.Table, contentDescription = null) },
                label = { Text(stringResource(R.string.nav_tables)) },
                modifier = Modifier.testTag(TAG_NAV_TABLES),
            )
            NavigationRailItem(
                selected = selected == MainSection.TAKEAWAY,
                onClick = { onSelect(MainSection.TAKEAWAY) },
                icon = { Icon(AdelIcons.Bag, contentDescription = null) },
                label = { Text(stringResource(R.string.nav_takeaway)) },
                modifier = Modifier.testTag(TAG_NAV_TAKEAWAY),
            )
            Spacer(modifier = Modifier.weight(1f))
        }
    }
}

/** La navegación principal en vertical: «Mesas» y «Para llevar» (§3.1 de la A2). */
@Composable
private fun MainNavigationBar(selected: MainSection, onSelect: (MainSection) -> Unit) {
    NavigationBar(modifier = Modifier.testTag(TAG_MAIN_NAVIGATION)) {
        NavigationBarItem(
            selected = selected == MainSection.TABLES,
            onClick = { onSelect(MainSection.TABLES) },
            icon = { Icon(AdelIcons.Table, contentDescription = null) },
            label = { Text(stringResource(R.string.nav_tables)) },
            modifier = Modifier.testTag(TAG_NAV_TABLES),
        )
        NavigationBarItem(
            selected = selected == MainSection.TAKEAWAY,
            onClick = { onSelect(MainSection.TAKEAWAY) },
            icon = { Icon(AdelIcons.Bag, contentDescription = null) },
            label = { Text(stringResource(R.string.nav_takeaway)) },
            modifier = Modifier.testTag(TAG_NAV_TAKEAWAY),
        )
    }
}

/**
 * El modo Mesero (Fase E): Mesas → la captura de una cuenta; «Pendientes» y «Requiere tu revisión».
 * Después de «Enviar a cocina» se vuelve a Mesas (el pedido sigue en la cola con su estado). Desde
 * la A2, «Para llevar» → la misma captura (compartida en `core:capture`), y al terminar se vuelve a
 * Para llevar.
 */
@Composable
private fun WaiterNavigation(
    navController: NavHostController,
    paneLayout: PaneLayout,
    onCaptureTitle: (String) -> Unit,
    onNotice: (String) -> Unit,
) {
    val application = LocalContext.current.applicationContext as AdelPosApplication
    val waiter = application.container.waiterDependencies
    val takeaway = application.container.takeawayDependencies
    val openCapture: (String) -> Unit = { captureId ->
        navController.navigate(CaptureDestination(captureId)) { launchSingleTop = true }
    }
    val openTakeawayCapture: (String) -> Unit = { captureId ->
        navController.navigate(CaptureDestination(captureId, takeaway = true)) {
            launchSingleTop = true
        }
    }
    val openReview: (String) -> Unit = { operationId ->
        navController.navigate(ReviewDestination(operationId)) { launchSingleTop = true }
    }
    val openTakeawayReview: (String) -> Unit = { operationId ->
        navController.navigate(ReviewDestination(operationId, takeaway = true)) {
            launchSingleTop = true
        }
    }
    val backToTakeaway: () -> Unit = {
        if (!navController.popBackStack(TakeawayDestination, inclusive = false)) {
            navController.popBackStack(TablesDestination, inclusive = false)
            selectSection(navController, MainSection.TAKEAWAY)
        }
    }
    // Etapa A2, Fase D: la cuenta a la vista (aviso dentro de la aplicación) y abrir la cuenta de
    // un aviso tocado.
    val current by navController.currentBackStackEntryAsState()
    ReadyAlertsNavigationEffects(
        controller = application.container.readyAlerts,
        capture = waiter.capture,
        viewingCaptureId =
            current
                ?.takeIf { it.destination.hasRoute<CaptureDestination>() }
                ?.toRoute<CaptureDestination>()
                ?.captureId,
        onOpenCapture = { captureId, fromTakeaway ->
            if (fromTakeaway) {
                openTakeawayCapture(captureId)
            } else {
                openCapture(captureId)
            }
        },
        onOpenTakeawayList = { selectSection(navController, MainSection.TAKEAWAY) },
    )
    // Etapa A3 (§4.12): al entrar el mesero, la pantalla inicial que guarda Terminales para este
    // dispositivo (Mesas o Para llevar; cualquier otro valor es Mesas, A3-D7). Lo guardado vale
    // enseguida (también sin conexión) y, unos segundos, lo que Go diga al entrar con conexión (la
    // pantalla y los permisos se leen al entrar). Solo si el mesero sigue en Mesas sin haber
    // tocado nada, y una sola vez por entrada (no al girar la pantalla).
    var homeApplied by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(homeApplied) {
        if (homeApplied) {
            return@LaunchedEffect
        }
        val setup = waiter.capture.precheck.setup
        val permissions = waiter.restaurant.permissions
        val takeaway =
            withTimeoutOrNull(HOME_WAIT_MILLIS) {
                combine(setup, permissions) { read, granted ->
                        DeviceHome.startsInTakeaway(read.home, granted)
                    }
                    .first { it }
            }
        homeApplied = true
        val untouched =
            navController.currentDestination?.hasRoute<TablesDestination>() == true &&
                navController.previousBackStackEntry == null
        if (takeaway == true && untouched) {
            selectSection(navController, MainSection.TAKEAWAY)
        }
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
        composable<TakeawayDestination> {
            TakeawayRoute(
                dependencies = takeaway,
                onOpenCapture = openTakeawayCapture,
                onOpenReview = openTakeawayReview,
            )
        }
        composable<CaptureDestination> { entry ->
            val fromTakeaway = entry.toRoute<CaptureDestination>().takeaway
            CaptureRoute(
                layout = paneLayout,
                dependencies = waiter.capture,
                onDone = {
                    if (fromTakeaway) {
                        backToTakeaway()
                    } else {
                        navController.popBackStack(TablesDestination, inclusive = false)
                    }
                },
                onTitleChange = onCaptureTitle,
                onLeaveUnsent = { notice ->
                    onNotice(notice)
                    navController.popBackStack()
                },
            )
        }
        composable<PendingDestination> {
            PendingOrdersRoute(
                dependencies = waiter.capture,
                onOpenCapture = openCapture,
                onOpenReview = openReview,
            )
        }
        composable<ReviewDestination> { entry ->
            val fromTakeaway = entry.toRoute<ReviewDestination>().takeaway
            ReviewRoute(
                dependencies = waiter.capture,
                onDone = { navController.popBackStack() },
                onOpenCapture = { captureId ->
                    navController.popBackStack()
                    if (fromTakeaway) {
                        openTakeawayCapture(captureId)
                    } else {
                        openCapture(captureId)
                    }
                },
            )
        }
    }
}

/** Cuánto se espera al entrar a que Go diga la pantalla inicial y los permisos del mesero. */
private const val HOME_WAIT_MILLIS = 5_000L

const val TAG_APP_ROOT = "app-root"
const val TAG_APP_NOTICE = "app-notice"
const val TAG_MAIN_NAVIGATION = "main-navigation"
const val TAG_NAVIGATION_RAIL = "main-navigation-rail"
const val TAG_NAV_TABLES = "nav-tables"
const val TAG_NAV_TAKEAWAY = "nav-takeaway"
