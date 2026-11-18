package com.adelpos.meseros.ready

import android.app.ActivityManager
import android.app.ApplicationExitInfo
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.SystemClock
import android.util.Log
import androidx.core.content.ContextCompat
import com.adelpos.meseros.core.data.access.AccessRepository
import com.adelpos.meseros.core.data.preferences.DevicePreferencesRepository
import com.adelpos.meseros.core.data.ready.DeviceReadyAlertsServer
import com.adelpos.meseros.core.data.ready.ReadyAlertsChannel
import com.adelpos.meseros.core.data.ready.ReadyAlertsClient
import com.adelpos.meseros.core.data.ready.ReadyChannelState
import com.adelpos.meseros.core.data.ready.ReadyRead
import com.adelpos.meseros.core.data.ready.ReadyStopReason
import com.adelpos.meseros.core.data.ready.RoomReadyAlertsLedger
import com.adelpos.meseros.core.data.ready.db.ReadyAlertsDao
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.StartDecision
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.Trigger
import com.adelpos.meseros.core.domain.ready.ReadyAlertsProcessor
import com.adelpos.meseros.core.domain.ready.ReadyAlertsStatus
import com.adelpos.meseros.core.domain.ready.ReadyNotice
import com.adelpos.meseros.core.domain.waiter.OrderMode
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

/** El mesero tocó un aviso: abrir esa cuenta (después de pedir su PIN si hace falta). */
data class ReadyOpenRequest(
    val accountId: String,
    val mode: OrderMode,
    val waiterUserId: String,
    val requestedAtElapsedMillis: Long,
) {
    /** Un toque viejo ya no abre nada (nadie entró en ese rato). */
    fun expired(nowElapsedMillis: Long): Boolean =
        nowElapsedMillis - requestedAtElapsedMillis > VALID_MILLIS

    companion object {
        const val VALID_MILLIS: Long = 3 * 60_000L
    }
}

/**
 * El aviso de platillos listos en el proceso (etapa A2, Fase D), **una instancia por proceso**
 * (`AppContainer.readyAlerts`). Dueño del único canal de avisos ([channel]) y de cuándo corre el
 * servicio en primer plano ([ReadyAlertsService], que solo lo sostiene):
 *
 * - **Arranque** ([request]): al entrar un mesero con conexión, al abrir la aplicación con el
 *   dispositivo listo y desde `BOOT_COMPLETED`/`MY_PACKAGE_REPLACED` ([ReadyAlertsBootReceiver]),
 *   según [ReadyAlertsLifecycle.decide] con lo guardado en Room. Si hay que preguntar antes, un
 *   `GET ready-alerts` sin mover el cursor ([probe]): sin destinatarios en el día no arranca y no
 *   hay notificación permanente.
 * - **Parada**: cuando [ReadyAlertsChannel.run] termina (`active: false`, fin del día sin
 *   respuesta, rechazo de la credencial), al dejar de estar listo el dispositivo (retirado,
 *   «Emparejar de nuevo», actualizar) y con «borrar los datos de este dispositivo»
 *   ([eraseDeviceData], registrado en `DeviceDataWipe`).
 * - El canal de eventos de la A1 no cambia y el servicio no lo abre: son dos canales distintos, uno
 *   de cada tipo por proceso. El servicio tampoco envía pedidos: al reconectar pide un envío de la
 *   cola ([retryOrders]), como los disparadores de la A1.
 *
 * @param enabled en las pruebas instrumentadas de la A1 el aviso queda apagado (su notificación y
 *   su canal no deben cambiar esas pruebas); en la aplicación, siempre encendido.
 * @param askPermissions si se explican y piden los permisos (notificaciones y batería) al arrancar
 *   por primera vez; las pruebas lo apagan y los conceden ellas.
 */
class ReadyAlertsController(
    context: Context,
    private val access: AccessRepository,
    dao: ReadyAlertsDao,
    private val preferences: DevicePreferencesRepository,
    private val foreground: StateFlow<Boolean>,
    private val linkState: Flow<LinkState>,
    private val retryOrders: () -> Unit,
    private val enabled: Boolean,
    val askPermissions: Boolean,
) : InAppAlerts {
    private val appContext: Context = context.applicationContext
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val startMutex = Mutex()
    private var started = false

    /** El registro en Room: cursor, lo último de Go y los avisos ya mostrados. */
    val ledger: RoomReadyAlertsLedger = RoomReadyAlertsLedger(dao)

    private val server = DeviceReadyAlertsServer(access)
    private val client = ReadyAlertsClient()

    val notifications: ReadyAlertNotifications = ReadyAlertNotifications(appContext, this)

    private val processor = ReadyAlertsProcessor(ledger, notifications)

    /** **El único** canal de avisos del proceso. */
    val channel: ReadyAlertsChannel =
        ReadyAlertsChannel(
            server = server,
            ledger = ledger,
            processor = processor,
            client = client,
            onReconnected = retryOrders,
        )

    private val runningState = MutableStateFlow(false)
    private val inAppFlow = MutableSharedFlow<ReadyNotice>(extraBufferCapacity = 8)
    // El canal en curso: lo arranca el hilo principal (el servicio) y lo cancela el borrado del
    // dispositivo (otro hilo).
    @Volatile private var loop: Job? = null

    // El canal terminó y el servicio se está deteniendo: un pedido de arranque en ese momento se
    // atiende en cuanto se destruya (no se pierde).
    @Volatile private var stopping = false
    @Volatile private var restartAfterStop: Trigger? = null

    /** El servicio en primer plano corre (o se acaba de pedir). */
    val running: StateFlow<Boolean> = runningState.asStateFlow()

    /** Cómo va el canal (notificación permanente y ayuda). */
    val connection: StateFlow<ReadyChannelState> = channel.state

    /** Avisos de la cuenta que el mesero está viendo: se muestran dentro de la aplicación. */
    val inAppNotices: SharedFlow<ReadyNotice> = inAppFlow.asSharedFlow()

    /** La cuenta de la captura a la vista (la pone la navegación); null en otra pantalla. */
    val viewingAccount = MutableStateFlow<String?>(null)

    /** El mesero tocó un aviso: la navegación abre la cuenta (y pide el PIN si hace falta). */
    val pendingOpen = MutableStateFlow<ReadyOpenRequest?>(null)

    /**
     * La cuenta del aviso tocado ya no está abierta (cobrada, cancelada o combinada sin una cuenta
     * vigente a la vista): la pantalla dice «Esta cuenta ya no está abierta».
     */
    val openedClosedAccount = MutableStateFlow(false)

    /** Se pidió la ayuda «Avisos de platillos listos» (desde la notificación o el menú). */
    val helpRequested = MutableStateFlow(false)

    /**
     * La salida anterior del proceso fue «Forzar detención» o «Detener» del Administrador de tareas
     * (`ApplicationExitInfo.REASON_USER_REQUESTED`, Android 11+): la ayuda lo dice.
     */
    val stoppedByUser = MutableStateFlow(false)

    /** Lo último que dijo Go (para la ayuda): null si nunca se preguntó. */
    suspend fun status(): ReadyAlertsStatus? = ledger.status()

    /** Vigila las entradas, el primer plano, el estado del dispositivo y la red (una sola vez). */
    fun start() {
        if (started) {
            return
        }
        started = true
        notifications.ensureConnectedChannel()
        stoppedByUser.value = lastExitWasUserStop()
        scope.launch {
            // Un mesero que entra con conexión: Go ya tiene su sesión con PIN de hoy.
            access.activeWaiter
                .map { waiter ->
                    waiter?.takeIf { it.hasServerSession && !it.mustReenterPin }?.userId
                }
                .distinctUntilChanged()
                .filter { it != null }
                .collect { request(Trigger.ONLINE_ENTRY) }
        }
        scope.launch {
            combine(foreground, access.status) { visible, status ->
                    visible && status is DeviceStatus.Ready
                }
                .distinctUntilChanged()
                .filter { it }
                .collect { request(Trigger.OPENED) }
        }
        scope.launch {
            // Retirado, sin emparejar, por emparejar de nuevo o por actualizar: sin avisos.
            access.status
                .map { status -> status !is DeviceStatus.Ready && status !is DeviceStatus.Starting }
                .distinctUntilChanged()
                .filter { it }
                .collect { stopService() }
        }
        scope.launch {
            // Al volver la red, el canal no espera a que venza su espera.
            linkState
                .map { it == LinkState.ONLINE }
                .distinctUntilChanged()
                .drop(1)
                .filter { it }
                .collect { channel.reconnectNow() }
        }
    }

    /** Pide arrancar el servicio por [trigger] (sin esperar). */
    fun request(trigger: Trigger) {
        if (!enabled) {
            return
        }
        scope.launch { requestNow(trigger) }
    }

    /**
     * Decide y, si corresponde, arranca el servicio; espera a que termine la decisión (lo usa el
     * receptor de arranque dentro de su `goAsync`).
     */
    suspend fun requestNow(trigger: Trigger) {
        if (!enabled) {
            return
        }
        startMutex.withLock {
            if (runningState.value) {
                if (stopping) {
                    restartAfterStop = trigger
                } else {
                    // Ya corre: un mesero que entró con conexión o la aplicación abierta dicen que
                    // hay red; el canal no espera a que venza su espera.
                    channel.reconnectNow()
                }
                return
            }
            server.prepare()
            if (server.ready != true) {
                return
            }
            when (
                ReadyAlertsLifecycle.decide(trigger, ledger.status(), System.currentTimeMillis())
            ) {
                StartDecision.START -> startService(trigger)
                StartDecision.ASK_FIRST ->
                    if (probe()) {
                        startService(trigger)
                    }
                StartDecision.NONE -> Unit
            }
        }
    }

    /**
     * La consulta previa: `GET ready-alerts` con la credencial del dispositivo. Guarda lo que dijo
     * Go sin mover el cursor (si está activo) y dice si hay a quién avisar.
     */
    private suspend fun probe(): Boolean {
        val connection = server.connection() ?: return false
        val cursor = ledger.status()?.cursor ?: 0L
        return when (val read = client.read(connection, cursor)) {
            is ReadyRead.Success -> {
                processor.onProbe(read.batch)
                read.batch.active
            }
            is ReadyRead.Failed -> {
                val action = ReadyAlertsLifecycle.onError(read.error)
                if (action == ReadyAlertsLifecycle.ErrorAction.STOP_AND_REPORT) {
                    server.report(read.error)
                }
                false
            }
        }
    }

    /**
     * `startForegroundService`: siempre desde primer plano o con una exención (§0.5.2). Si Android
     * no lo permite (`ForegroundServiceStartNotAllowedException`, Android 12+; o
     * `SecurityException` por el tipo), no se arranca y se vuelve a intentar con el siguiente
     * disparador.
     */
    private fun startService(trigger: Trigger) {
        try {
            ContextCompat.startForegroundService(appContext, serviceIntent())
            runningState.value = true
            Log.i(TAG, "servicio de avisos pedido ($trigger)")
        } catch (refused: IllegalStateException) {
            Log.w(TAG, "Android no dejó arrancar el servicio de avisos ($trigger): $refused")
        } catch (refused: SecurityException) {
            Log.w(TAG, "Android no dejó arrancar el servicio de avisos ($trigger): $refused")
        }
    }

    /** El servicio ya está en primer plano: corre el canal (una sola vez por proceso). */
    internal fun onServiceStarted() {
        runningState.value = true
        if (loop?.isActive == true) {
            return
        }
        stopping = false
        loop = scope.launch {
            val reason = channel.run()
            Log.i(TAG, "el canal de avisos terminó: $reason")
            onChannelEnded(reason)
        }
    }

    private fun onChannelEnded(reason: ReadyStopReason) {
        stopping = true
        if (reason == ReadyStopReason.INACTIVE) {
            Log.i(TAG, "sin a quién avisar: se detiene el servicio")
        }
        stopService()
    }

    /** Android destruyó el servicio (por [stopService] o por el sistema). */
    internal fun onServiceDestroyed() {
        loop?.cancel()
        loop = null
        runningState.value = false
        stopping = false
        val pending = restartAfterStop
        restartAfterStop = null
        if (pending != null) {
            request(pending)
        }
    }

    /**
     * Vuelve a publicar la notificación permanente si el servicio corre: la publicada sin el
     * permiso de notificaciones (Android 13+) no aparece sola cuando se concede después.
     */
    fun refreshOngoing() {
        if (!runningState.value || loop?.isActive != true) {
            return
        }
        val state = channel.state.value
        val connected = state !is ReadyChannelState.Waiting
        val manager = appContext.getSystemService(NotificationManager::class.java) ?: return
        manager.notify(ReadyAlertNotifications.ONGOING_ID, notifications.ongoing(connected))
    }

    /** Detiene el servicio (si corre); su notificación permanente se va con él. */
    fun stopService() {
        appContext.stopService(serviceIntent())
    }

    /**
     * «Borrar los datos de este dispositivo» (retirarlo o emparejarlo de nuevo): detiene el canal y
     * el servicio, borra el cursor y los avisos ya mostrados, y quita los avisos a la vista.
     */
    suspend fun eraseDeviceData() {
        restartAfterStop = null
        val running = loop
        if (running != null) {
            withTimeoutOrNull(ERASE_WAIT_MILLIS) { running.cancelAndJoin() }
        }
        stopService()
        ledger.erase()
        notifications.cancelAlerts()
        pendingOpen.value = null
    }

    /** Llega de [android.app.Activity.onCreate] o `onNewIntent`: un toque en una notificación. */
    fun handleIntent(intent: Intent?) {
        when (intent?.action) {
            ReadyAlertsIntents.ACTION_OPEN_ALERT -> {
                val account = intent.getStringExtra(ReadyAlertsIntents.EXTRA_ACCOUNT_ID) ?: return
                Log.i(TAG, "se tocó el aviso de la cuenta $account")
                pendingOpen.value =
                    ReadyOpenRequest(
                        accountId = account,
                        mode =
                            OrderMode.fromWireName(
                                intent.getStringExtra(ReadyAlertsIntents.EXTRA_MODE),
                            ),
                        waiterUserId =
                            intent.getStringExtra(ReadyAlertsIntents.EXTRA_WAITER_ID).orEmpty(),
                        requestedAtElapsedMillis = SystemClock.elapsedRealtime(),
                    )
            }
            ReadyAlertsIntents.ACTION_OPEN_HELP -> helpRequested.value = true
        }
    }

    /** Ya se abrió la cuenta del aviso (o no se pudo): no se vuelve a intentar. */
    fun consumeOpen(request: ReadyOpenRequest) {
        pendingOpen.compareAndSet(request, null)
    }

    /** La captura a la vista: si es de una cuenta con aviso, su notificación ya no hace falta. */
    fun setViewing(accountId: String?) {
        viewingAccount.value = accountId
        if (accountId != null) {
            notifications.cancelAccount(accountId)
        }
    }

    override fun isViewing(accountId: String): Boolean =
        foreground.value && viewingAccount.value == accountId

    override fun show(notice: ReadyNotice) {
        inAppFlow.tryEmit(notice)
    }

    /** Si ya se explicó y pidió [permission] (una sola vez). */
    suspend fun permissionAsked(permission: String): Boolean =
        preferences.alertPermissionAsked(permission)

    suspend fun markPermissionAsked(permission: String) {
        preferences.markAlertPermissionAsked(permission)
    }

    private fun serviceIntent(): Intent = Intent(appContext, ReadyAlertsService::class.java)

    /**
     * Android 11+ guarda por qué terminó el proceso anterior; «Forzar detención» y «Detener» del
     * Administrador de tareas (Android 13+) son `REASON_USER_REQUESTED`.
     */
    private fun lastExitWasUserStop(): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) {
            return false
        }
        val manager = appContext.getSystemService(ActivityManager::class.java) ?: return false
        val last =
            try {
                manager.getHistoricalProcessExitReasons(appContext.packageName, 0, 1).firstOrNull()
            } catch (_: RuntimeException) {
                null
            }
        return last?.reason == ApplicationExitInfo.REASON_USER_REQUESTED
    }

    companion object {
        private const val TAG = "AdelAvisos"
        private const val ERASE_WAIT_MILLIS = 5_000L

        /** Las «preguntas» de permisos que se hacen una sola vez. */
        const val PERMISSION_NOTIFICATIONS = "notifications"
        const val PERMISSION_BATTERY = "battery"
    }
}
