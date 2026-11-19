package com.adelpos.meseros.ready

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.adelpos.meseros.AdelPosApplication
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Tras reiniciar el equipo (`BOOT_COMPLETED`, después del primer desbloqueo) o actualizar la
 * aplicación (`MY_PACKAGE_REPLACED`), vuelve a arrancar el servicio de avisos **solo si lo último
 * que dijo Go fue `active: true` y no ha terminado el día comercial** (guardado en Room). Ambos son
 * exenciones para arrancar un servicio en primer plano desde segundo plano (Android 12+), y
 * `specialUse` sí puede arrancar desde `BOOT_COMPLETED` (`dataSync` no; comprobado en la Fase 0).
 */
class ReadyAlertsBootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action !in HANDLED) {
            return
        }
        val controller = (context.applicationContext as AdelPosApplication).container.readyAlerts
        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.Default).launch {
            try {
                // goAsync da unos 10 s; la decisión solo lee Room y lo guardado del emparejamiento.
                withTimeoutOrNull(DECISION_MILLIS) {
                    controller.requestNow(ReadyAlertsLifecycle.Trigger.BOOT)
                }
            } finally {
                pending.finish()
            }
        }
    }

    private companion object {
        const val DECISION_MILLIS = 8_000L
        val HANDLED = setOf(Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED)
    }
}
