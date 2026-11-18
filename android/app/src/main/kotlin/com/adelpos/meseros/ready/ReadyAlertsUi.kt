package com.adelpos.meseros.ready

import android.Manifest
import android.annotation.SuppressLint
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.PowerManager
import android.os.SystemClock
import android.provider.Settings
import android.util.Log
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.core.content.ContextCompat
import androidx.core.net.toUri
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.adelpos.meseros.R
import com.adelpos.meseros.core.capture.CaptureDependencies
import com.adelpos.meseros.core.domain.access.ActiveWaiter
import com.adelpos.meseros.core.domain.ready.ReadyNotice
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.RestaurantSource
import com.adelpos.meseros.core.domain.waiter.WaiterAction
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.ui.theme.AdelDimens
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/*
 * Lo que el aviso de platillos listos muestra dentro de la aplicación (etapa A2, Fase D): la
 * explicación de los permisos antes del diálogo de Android, la ayuda «Avisos de platillos listos»,
 * el aviso dentro de la aplicación y abrir la cuenta al tocar una notificación.
 */

/**
 * Encima de toda la interfaz (en la raíz, `AdelPosApp`): los permisos la primera vez que el
 * servicio arranca, la ayuda cuando se pide y el aviso de la cuenta que el mesero está viendo.
 */
@Composable
fun ReadyAlertsOverlay(controller: ReadyAlertsController) {
    ReadyAlertsPermissionGate(controller)
    val help by controller.helpRequested.collectAsStateWithLifecycle()
    if (help) {
        ReadyAlertsHelp(
            controller = controller,
            onClose = { controller.helpRequested.value = false },
        )
    }
    ReadyInAppNotice(controller)
}

/** Qué permiso falta explicar y pedir. */
private enum class PermissionStep {
    NOTIFICATIONS,
    BATTERY,
}

/**
 * §4.3: la primera vez que el servicio va a arrancar (corre), una explicación en español y después
 * el diálogo de Android: notificaciones (Android 13+) y la exclusión de la optimización de batería.
 * Cada una se pide **una sola vez**; si se niegan, la aplicación sigue y la ayuda dice qué falta.
 */
@Composable
private fun ReadyAlertsPermissionGate(controller: ReadyAlertsController) {
    if (!controller.askPermissions) {
        return
    }
    val running by controller.running.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var step by remember { mutableStateOf<PermissionStep?>(null) }
    var round by remember { mutableIntStateOf(0) }
    LaunchedEffect(running, round) {
        step =
            if (running) {
                nextPermissionStep(context, controller)
            } else {
                null
            }
    }
    val notificationsLauncher =
        rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
            if (granted) {
                controller.refreshOngoing()
            }
            round++
        }
    val batteryLauncher =
        rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {
            round++
        }
    when (step) {
        PermissionStep.NOTIFICATIONS ->
            PermissionDialog(
                text = stringResource(R.string.ready_permission_notifications_text),
                onContinue = {
                    step = null
                    scope.launch {
                        controller.markPermissionAsked(
                            ReadyAlertsController.PERMISSION_NOTIFICATIONS,
                        )
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                            notificationsLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
                        } else {
                            round++
                        }
                    }
                },
                onLater = {
                    step = null
                    scope.launch {
                        controller.markPermissionAsked(
                            ReadyAlertsController.PERMISSION_NOTIFICATIONS,
                        )
                        round++
                    }
                },
            )
        PermissionStep.BATTERY ->
            PermissionDialog(
                text = stringResource(R.string.ready_permission_battery_text),
                onContinue = {
                    step = null
                    scope.launch {
                        controller.markPermissionAsked(ReadyAlertsController.PERMISSION_BATTERY)
                        try {
                            batteryLauncher.launch(batteryExemptionRequest(context))
                        } catch (_: ActivityNotFoundException) {
                            round++
                        }
                    }
                },
                onLater = {
                    step = null
                    scope.launch {
                        controller.markPermissionAsked(ReadyAlertsController.PERMISSION_BATTERY)
                        round++
                    }
                },
            )
        null -> Unit
    }
}

private suspend fun nextPermissionStep(
    context: Context,
    controller: ReadyAlertsController,
): PermissionStep? {
    if (
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) !=
                PackageManager.PERMISSION_GRANTED &&
            !controller.permissionAsked(ReadyAlertsController.PERMISSION_NOTIFICATIONS)
    ) {
        return PermissionStep.NOTIFICATIONS
    }
    if (
        !ignoresBatteryOptimizations(context) &&
            !controller.permissionAsked(ReadyAlertsController.PERMISSION_BATTERY)
    ) {
        return PermissionStep.BATTERY
    }
    return null
}

@Composable
private fun PermissionDialog(text: String, onContinue: () -> Unit, onLater: () -> Unit) {
    AlertDialog(
        onDismissRequest = onLater,
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current).testTag(TAG_PERMISSION),
        title = { Text(stringResource(R.string.ready_permission_title)) },
        text = { Text(text) },
        confirmButton = {
            TextButton(
                onClick = onContinue,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag(TAG_PERMISSION_CONTINUE),
            ) {
                Text(stringResource(R.string.ready_permission_continue))
            }
        },
        dismissButton = {
            TextButton(
                onClick = onLater,
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag(TAG_PERMISSION_LATER),
            ) {
                Text(stringResource(R.string.ready_permission_later))
            }
        },
    )
}

/** Si AD-EL POS ya está fuera de la optimización de batería. */
fun ignoresBatteryOptimizations(context: Context): Boolean {
    val power = context.getSystemService(PowerManager::class.java) ?: return false
    return power.isIgnoringBatteryOptimizations(context.packageName)
}

/**
 * El diálogo de Android para quitar la optimización de batería a AD-EL POS. Google Play solo lo
 * admite cuando Doze rompe la función principal y no se puede usar FCM: aquí la red es la del
 * restaurante (sin servidores de Google, la wifi puede no tener internet) y la aplicación no se
 * distribuye por Play (avance de la etapa, §0.5.4). Sirve de exención para volver a arrancar el
 * servicio desde segundo plano y atenúa los gestores de batería de algunos fabricantes.
 */
@SuppressLint("BatteryLife")
private fun batteryExemptionRequest(context: Context): Intent =
    Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
        .setData("package:${context.packageName}".toUri())

/** Ajustes de notificaciones de la aplicación (Android 8+). */
private fun notificationSettings(context: Context): Intent =
    Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
        .putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

/**
 * La pantalla de ayuda «Avisos de platillos listos» (§4.3): qué falta (notificaciones, batería),
 * con el botón a Ajustes, qué significa la notificación permanente y que «Forzar detención» o
 * «Detener» del Administrador de tareas paran los avisos hasta abrir la aplicación (y si eso pasó
 * la última vez). Se abre desde «AD-EL POS conectado» o desde el menú del mesero.
 */
@Composable
fun ReadyAlertsHelp(controller: ReadyAlertsController, onClose: () -> Unit) {
    val context = LocalContext.current
    val running by controller.running.collectAsStateWithLifecycle()
    val stoppedByUser by controller.stoppedByUser.collectAsStateWithLifecycle()
    var notificationsVisible by remember { mutableStateOf(true) }
    var batteryFree by remember { mutableStateOf(true) }
    var disabledInTerminals by remember { mutableStateOf(false) }
    var refreshes by remember { mutableIntStateOf(0) }
    // Al volver de los Ajustes de Android se vuelve a revisar.
    LifecycleResumeEffect(Unit) {
        refreshes++
        onPauseOrDispose {}
    }
    LaunchedEffect(refreshes, running) {
        notificationsVisible = controller.notifications.alertsVisible()
        if (notificationsVisible) {
            // Pudo volver de los Ajustes con el permiso recién dado.
            controller.refreshOngoing()
        }
        batteryFree = ignoresBatteryOptimizations(context)
        disabledInTerminals = controller.status()?.settings?.enabled == false
    }
    val batteryLauncher =
        rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {
            refreshes++
        }
    Dialog(
        onDismissRequest = onClose,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier =
                Modifier.fillMaxSize()
                    .reportsUserActivity(LocalUserActivity.current)
                    .testTag(TAG_HELP),
            color = MaterialTheme.colorScheme.background,
        ) {
            Column(
                modifier =
                    Modifier.fillMaxSize()
                        .windowInsetsPadding(WindowInsets.safeDrawing)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 24.dp, vertical = 16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        text = stringResource(R.string.ready_help_title),
                        style = MaterialTheme.typography.headlineSmall,
                        modifier = Modifier.weight(1f),
                    )
                    TextButton(
                        onClick = onClose,
                        modifier =
                            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                                .testTag(TAG_HELP_CLOSE),
                    ) {
                        Text(stringResource(R.string.ready_help_close))
                    }
                }
                HelpText(stringResource(R.string.ready_help_intro))
                HelpSection(stringResource(R.string.ready_help_status_title)) {
                    HelpText(
                        stringResource(
                            when {
                                disabledInTerminals -> R.string.ready_help_disabled
                                running -> R.string.ready_help_running
                                else -> R.string.ready_help_stopped
                            },
                        ),
                        tag = TAG_HELP_STATE,
                    )
                    HelpText(
                        stringResource(
                            if (notificationsVisible) {
                                R.string.ready_help_notifications_ok
                            } else {
                                R.string.ready_help_notifications_off
                            },
                        ),
                        tag = TAG_HELP_NOTIFICATIONS,
                    )
                    if (!notificationsVisible) {
                        SecondaryButton(
                            text = stringResource(R.string.ready_help_notifications_settings),
                            onClick = { openSafely(context, notificationSettings(context)) },
                            modifier = Modifier.testTag(TAG_HELP_NOTIFICATIONS_SETTINGS),
                        )
                    }
                    HelpText(
                        stringResource(
                            if (batteryFree) {
                                R.string.ready_help_battery_ok
                            } else {
                                R.string.ready_help_battery_off
                            },
                        ),
                        tag = TAG_HELP_BATTERY,
                    )
                    if (!batteryFree) {
                        SecondaryButton(
                            text = stringResource(R.string.ready_help_battery_settings),
                            onClick = {
                                try {
                                    batteryLauncher.launch(batteryExemptionRequest(context))
                                } catch (_: ActivityNotFoundException) {
                                    openSafely(
                                        context,
                                        Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
                                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                                    )
                                }
                            },
                            modifier = Modifier.testTag(TAG_HELP_BATTERY_SETTINGS),
                        )
                    }
                }
                HelpSection(stringResource(R.string.ready_help_ongoing_title)) {
                    HelpText(stringResource(R.string.ready_help_ongoing_text))
                }
                HelpSection(stringResource(R.string.ready_help_stop_title)) {
                    HelpText(stringResource(R.string.ready_help_stop_text))
                    if (stoppedByUser) {
                        HelpText(
                            stringResource(R.string.ready_help_stop_detected),
                            tag = TAG_HELP_STOPPED_BY_USER,
                        )
                    }
                }
                HelpSection(stringResource(R.string.ready_help_sound_title)) {
                    HelpText(stringResource(R.string.ready_help_sound_text))
                }
                HelpText(stringResource(R.string.ready_help_kitchen_text))
            }
        }
    }
}

@Composable
private fun HelpSection(title: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
        )
        content()
    }
}

@Composable
private fun HelpText(text: String, tag: String? = null) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyLarge,
        color = MaterialTheme.colorScheme.onBackground,
        modifier =
            if (tag == null) {
                Modifier
            } else {
                Modifier.testTag(tag)
            },
    )
}

private fun openSafely(context: Context, intent: Intent) {
    try {
        context.startActivity(intent)
    } catch (_: ActivityNotFoundException) {
        // Algún fabricante no tiene esa pantalla de Ajustes: no hay a dónde ir.
    }
}

/**
 * El aviso dentro de la aplicación (§4.4): si el mesero ya está viendo la cuenta, en lugar de la
 * notificación del sistema, arriba de la pantalla unos segundos («Mesa 4 · 2 platillos listos», «2
 * × Filete»).
 */
@Composable
private fun ReadyInAppNotice(controller: ReadyAlertsController) {
    val context = LocalContext.current
    val words = remember(context) { ReadyAlertNotifications.wordsFrom(context) }
    var shown by remember { mutableStateOf<ReadyNotice?>(null) }
    LaunchedEffect(controller) { controller.inAppNotices.collect { notice -> shown = notice } }
    val notice = shown ?: return
    LaunchedEffect(notice) {
        delay(IN_APP_MILLIS)
        if (shown == notice) {
            shown = null
        }
    }
    Box(
        modifier = Modifier.fillMaxSize().windowInsetsPadding(WindowInsets.safeDrawing),
        contentAlignment = Alignment.TopCenter,
    ) {
        Surface(
            color = MaterialTheme.colorScheme.inverseSurface,
            contentColor = MaterialTheme.colorScheme.inverseOnSurface,
            shape = MaterialTheme.shapes.medium,
            tonalElevation = 6.dp,
            shadowElevation = 6.dp,
            modifier = Modifier.padding(16.dp).widthIn(max = 560.dp).testTag(TAG_IN_APP),
        ) {
            Row(
                modifier = Modifier.padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f, fill = false)) {
                    Text(
                        text = words.title(notice),
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Text(text = words.text(notice), style = MaterialTheme.typography.bodyMedium)
                }
                TextButton(
                    onClick = { shown = null },
                    modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget),
                ) {
                    Text(
                        text = stringResource(R.string.ready_in_app_close),
                        color = MaterialTheme.colorScheme.inversePrimary,
                    )
                }
            }
        }
    }
}

/**
 * Dentro del modo Mesero: dice qué cuenta está a la vista (para el aviso dentro de la aplicación) y
 * abre la cuenta de un aviso tocado. Si entró otro mesero, solo la abre si él puede verla (la
 * cuenta está en sus mesas o en su Para llevar, según Go); un pedido para llevar sin permiso de
 * capturar abre la lista.
 *
 * @param viewingCaptureId la captura de la pantalla actual (null en otra pantalla).
 */
@Composable
fun ReadyAlertsNavigationEffects(
    controller: ReadyAlertsController,
    capture: CaptureDependencies,
    viewingCaptureId: String?,
    onOpenCapture: (captureId: String, takeaway: Boolean) -> Unit,
    onOpenTakeawayList: () -> Unit,
) {
    LaunchedEffect(viewingCaptureId) {
        if (viewingCaptureId == null) {
            controller.setViewing(null)
            return@LaunchedEffect
        }
        capture.captures
            .capture(viewingCaptureId)
            .map { it?.accountId }
            .distinctUntilChanged()
            .collect { controller.setViewing(it) }
    }
    DisposableEffect(controller) { onDispose { controller.setViewing(null) } }
    val pending by controller.pendingOpen.collectAsStateWithLifecycle()
    val waiter by capture.activeWaiter.collectAsStateWithLifecycle()
    LaunchedEffect(pending, waiter?.userId, waiter?.mustReenterPin) {
        val request = pending ?: return@LaunchedEffect
        val current = waiter?.takeIf { !it.mustReenterPin } ?: return@LaunchedEffect
        // Se consume al terminar: consumirlo antes cambiaría la llave de este efecto y lo
        // cancelaría
        // a la mitad (antes de abrir la cuenta). Si se cancela por otro motivo (cambió el mesero),
        // el efecto nuevo lo vuelve a intentar.
        if (!request.expired(SystemClock.elapsedRealtime())) {
            openAccount(capture, current, request, onOpenCapture, onOpenTakeawayList)
        }
        controller.consumeOpen(request)
    }
}

private suspend fun openAccount(
    capture: CaptureDependencies,
    waiter: ActiveWaiter,
    request: ReadyOpenRequest,
    onOpenCapture: (String, Boolean) -> Unit,
    onOpenTakeawayList: () -> Unit,
) {
    val account = findAccount(capture.restaurant, request.accountId)
    if (account == null) {
        // No está entre las cuentas que este mesero puede ver (o ya se cerró): no se abre nada.
        Log.i(TAG, "la cuenta ${request.accountId} del aviso no está a la vista de este mesero")
        return
    }
    val permissions = capture.restaurant.permissions.value
    if (account.mode == OrderMode.TAKEAWAY) {
        if (permissions.allows(WaiterAction.MANAGE_TAKEAWAY)) {
            onOpenCapture(capture.captures.openTakeaway(waiter.userId, account).id, true)
        } else {
            onOpenTakeawayList()
        }
        return
    }
    val tableId = account.tableId ?: return
    val seen = capture.restaurant.openAccounts.value.filter { it.tableId == tableId }.map { it.id }
    val opened =
        capture.captures.open(
            userId = waiter.userId,
            tableId = tableId,
            tableName = account.tableName.orEmpty(),
            account = account,
            seenAccountIds = seen,
        )
    onOpenCapture(opened.id, false)
    Log.i(TAG, "abierta la cuenta ${account.id} del aviso")
}

/**
 * La cuenta entre las abiertas que ve el mesero activo (la lectura de Go con su sesión, o lo último
 * guardado). Si todavía no está, se vuelve a leer y se espera unos segundos.
 */
private suspend fun findAccount(restaurant: RestaurantSource, accountId: String): AccountSummary? {
    restaurant.openAccounts.value
        .firstOrNull { it.id == accountId }
        ?.let {
            return it
        }
    restaurant.refresh()
    return withTimeoutOrNull(FIND_ACCOUNT_MILLIS) {
        var found: AccountSummary? = null
        while (found == null) {
            found = restaurant.openAccounts.value.firstOrNull { it.id == accountId }
            if (found == null) {
                delay(FIND_POLL_MILLIS)
            }
        }
        found
    }
}

private const val TAG = "AdelAvisos"
private const val IN_APP_MILLIS = 6_000L
private const val FIND_ACCOUNT_MILLIS = 10_000L
private const val FIND_POLL_MILLIS = 250L

const val TAG_PERMISSION = "ready-permission"
const val TAG_PERMISSION_CONTINUE = "ready-permission-continue"
const val TAG_PERMISSION_LATER = "ready-permission-later"
const val TAG_HELP = "ready-help"
const val TAG_HELP_CLOSE = "ready-help-close"
const val TAG_HELP_STATE = "ready-help-state"
const val TAG_HELP_NOTIFICATIONS = "ready-help-notifications"
const val TAG_HELP_NOTIFICATIONS_SETTINGS = "ready-help-notifications-settings"
const val TAG_HELP_BATTERY = "ready-help-battery"
const val TAG_HELP_BATTERY_SETTINGS = "ready-help-battery-settings"
const val TAG_HELP_STOPPED_BY_USER = "ready-help-stopped-by-user"
const val TAG_IN_APP = "ready-in-app"
