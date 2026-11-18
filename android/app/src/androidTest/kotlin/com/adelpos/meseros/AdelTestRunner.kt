package com.adelpos.meseros

import android.app.Application
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnitRunner
import com.adelpos.meseros.core.data.waiter.LostResponses
import com.adelpos.meseros.core.data.waiter.WaiterApi
import java.util.concurrent.atomic.AtomicInteger

/**
 * Ejecutor de las pruebas instrumentadas de la aplicación. Antes de que nada use el contenedor:
 * - la búsqueda NSD usa el tipo `_adelpostest._tcp`, para que ninguna prueba encuentre (ni pruebe
 *   con su huella) el servidor real del restaurante que se anuncia en la red de la Mac;
 * - con el argumento de instrumentación `clientVersion`, la aplicación se presenta al servidor con
 *   esa versión (prueba de «Actualiza esta aplicación», 426);
 * - con `lostOfflineResponses` (un número), esa cantidad de respuestas de `submitOfflineOrder` se
 *   descarta **después** de que Go la procesó: la «respuesta perdida» de §9 (Fase E); igual con
 *   `lostDraftResponses` (guardar el borrador) y `lostConfirmResponses` («Enviar a cocina» en
 *   línea) para la revisión R6;
 * - el aviso de platillos listos (etapa A2, Fase D) queda **apagado** salvo con `readyAlerts=on`
 *   (la suite `alerts` y las pruebas del aviso): su servicio y sus notificaciones no deben cambiar
 *   las pruebas de la A1. Sus permisos no se explican ni se piden salvo con `alertPermissions=ask`
 *   (las pruebas los conceden ellas).
 */
class AdelTestRunner : AndroidJUnitRunner() {
    override fun callApplicationOnCreate(app: Application) {
        val arguments = InstrumentationRegistry.getArguments()
        (app as AdelPosApplication).containerSettings =
            AppContainer.Settings(
                clientVersion = arguments.getString("clientVersion") ?: BuildConfig.VERSION_NAME,
                discoveryServiceType = TEST_SERVICE_TYPE,
                lostResponses =
                    lostResponses(
                        offline = arguments.getString("lostOfflineResponses"),
                        drafts = arguments.getString("lostDraftResponses"),
                        confirms = arguments.getString("lostConfirmResponses"),
                    ),
                readyAlerts = arguments.getString("readyAlerts") == "on",
                askAlertPermissions = arguments.getString("alertPermissions") == "ask",
            )
        super.callApplicationOnCreate(app)
    }

    // Descarta las primeras respuestas de cada ruta indicada (después de que Go las procesó); sin
    // ningún argumento no descarta nada.
    private fun lostResponses(
        offline: String?,
        drafts: String?,
        confirms: String?,
    ): LostResponses? {
        val offlineLeft = AtomicInteger(offline?.toIntOrNull() ?: 0)
        val draftsLeft = AtomicInteger(drafts?.toIntOrNull() ?: 0)
        val confirmsLeft = AtomicInteger(confirms?.toIntOrNull() ?: 0)
        if (offlineLeft.get() == 0 && draftsLeft.get() == 0 && confirmsLeft.get() == 0) {
            return null
        }
        return LostResponses { method, path ->
            val counter =
                when {
                    method == "POST" && path == WaiterApi.OFFLINE_ORDERS_PATH -> offlineLeft
                    method == "PUT" && DRAFT_PATH.matches(path) -> draftsLeft
                    method == "POST" && CONFIRM_PATH.matches(path) -> confirmsLeft
                    else -> null
                }
            counter != null && counter.getAndDecrement() > 0
        }
    }

    companion object {
        const val TEST_SERVICE_TYPE = "_adelpostest._tcp"

        private val DRAFT_PATH = Regex("restaurant/accounts/[^/]+/draft")
        private val CONFIRM_PATH = Regex("restaurant/accounts/[^/]+/confirm")
    }
}
