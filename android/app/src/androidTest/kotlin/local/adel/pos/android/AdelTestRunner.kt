package local.adel.pos.android

import android.app.Application
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnitRunner
import java.util.concurrent.atomic.AtomicInteger
import local.adel.pos.android.core.data.waiter.LostResponses
import local.adel.pos.android.core.data.waiter.WaiterApi

/**
 * Ejecutor de las pruebas instrumentadas de la aplicación. Antes de que nada use el contenedor:
 * - la búsqueda NSD usa el tipo `_adelpostest._tcp`, para que ninguna prueba encuentre (ni pruebe
 *   con su huella) el servidor real del restaurante que se anuncia en la red de la Mac;
 * - con el argumento de instrumentación `clientVersion`, la aplicación se presenta al servidor con
 *   esa versión (prueba de «Actualiza esta aplicación», 426);
 * - con `lostOfflineResponses` (un número), esa cantidad de respuestas de `submitOfflineOrder` se
 *   descarta **después** de que Go la procesó: la «respuesta perdida» de §9 (Fase E).
 */
class AdelTestRunner : AndroidJUnitRunner() {
    override fun callApplicationOnCreate(app: Application) {
        val arguments = InstrumentationRegistry.getArguments()
        (app as AdelPosApplication).containerSettings =
            AppContainer.Settings(
                clientVersion = arguments.getString("clientVersion") ?: BuildConfig.VERSION_NAME,
                discoveryServiceType = TEST_SERVICE_TYPE,
                lostResponses = lostOfflineResponses(arguments.getString("lostOfflineResponses")),
            )
        super.callApplicationOnCreate(app)
    }

    // Descarta las primeras [count] respuestas del pedido sin conexión (después de que Go las
    // procesó); sin el argumento no descarta nada.
    private fun lostOfflineResponses(count: String?): LostResponses? {
        val remaining = AtomicInteger(count?.toIntOrNull() ?: return null)
        return LostResponses { method, path ->
            method == "POST" &&
                path == WaiterApi.OFFLINE_ORDERS_PATH &&
                remaining.getAndDecrement() > 0
        }
    }

    companion object {
        const val TEST_SERVICE_TYPE = "_adelpostest._tcp"
    }
}
