package com.adelpos.meseros.ready

import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.ServiceCompat
import com.adelpos.meseros.AdelPosApplication
import com.adelpos.meseros.core.data.ready.ReadyChannelState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * El servicio en primer plano del aviso de platillos listos (A2-D8): tipo **`specialUse`** (sin
 * límite de horas; `dataSync` vence a las 6 h en Android 15+ y no puede arrancar tras reiniciar),
 * con la notificación permanente «AD-EL POS conectado». Solo **sostiene** el proceso y su
 * notificación: el canal de avisos, cuándo arrancar y cuándo detenerse viven en
 * [ReadyAlertsController] (uno por proceso).
 *
 * - `onStartCommand` llama a `startForeground` **antes que nada** (Android exige hacerlo pocos
 *   segundos después de `startForegroundService`), también cuando Android lo reinicia solo
 *   (`START_STICKY`, con `intent == null`, tras morir el proceso): el canal se reanuda desde el
 *   cursor guardado en Room.
 * - `onTaskRemoved` (quitar la aplicación de recientes) no lo detiene: el proceso y los avisos
 *   siguen (comprobado en la Fase 0).
 * - El tipo se pasa a `startForeground` desde Android 10 (API 29); antes no existe.
 */
class ReadyAlertsService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var foreground = false

    private val controller: ReadyAlertsController
        get() = (application as AdelPosApplication).container.readyAlerts

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        controller.notifications.ensureConnectedChannel()
        scope.launch {
            // La notificación permanente dice si hay conexión con la computadora principal.
            controller.connection
                .map { it == ReadyChannelState.Connected || it == ReadyChannelState.Connecting }
                .distinctUntilChanged()
                .collect { connected -> if (foreground) promote(connected) }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!promote(connected = true)) {
            // Android no lo dejó pasar a primer plano: sin la notificación no se sigue.
            stopSelf()
            return START_NOT_STICKY
        }
        if (intent == null) {
            Log.i(TAG, "reinicio pegajoso del servicio de avisos")
        }
        controller.onServiceStarted()
        return START_STICKY
    }

    /** Pasa (o sigue) en primer plano con la notificación permanente. */
    private fun promote(connected: Boolean): Boolean {
        val notification = controller.notifications.ongoing(connected)
        val type =
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            } else {
                0
            }
        return try {
            ServiceCompat.startForeground(
                this,
                ReadyAlertNotifications.ONGOING_ID,
                notification,
                type,
            )
            foreground = true
            true
        } catch (refused: IllegalStateException) {
            // ForegroundServiceStartNotAllowedException (Android 12+) es un IllegalStateException.
            Log.w(TAG, "startForeground rechazado: $refused")
            false
        } catch (refused: SecurityException) {
            Log.w(TAG, "startForeground rechazado: $refused")
            false
        }
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        Log.i(TAG, "la aplicación se quitó de recientes; los avisos siguen")
        super.onTaskRemoved(rootIntent)
    }

    override fun onDestroy() {
        scope.cancel()
        if (foreground) {
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
            foreground = false
        }
        controller.onServiceDestroyed()
        super.onDestroy()
    }

    private companion object {
        const val TAG = "AdelAvisos"
    }
}
