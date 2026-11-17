package local.adel.pos.android

import android.app.Application
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnitRunner

/**
 * Ejecutor de las pruebas instrumentadas de la aplicación. Antes de que nada use el contenedor:
 * - la búsqueda NSD usa el tipo `_adelpostest._tcp`, para que ninguna prueba encuentre (ni pruebe
 *   con su huella) el servidor real del restaurante que se anuncia en la red de la Mac;
 * - con el argumento de instrumentación `clientVersion`, la aplicación se presenta al servidor con
 *   esa versión (prueba de «Actualiza esta aplicación», 426).
 */
class AdelTestRunner : AndroidJUnitRunner() {
    override fun callApplicationOnCreate(app: Application) {
        val arguments = InstrumentationRegistry.getArguments()
        (app as AdelPosApplication).containerSettings =
            AppContainer.Settings(
                clientVersion = arguments.getString("clientVersion") ?: BuildConfig.VERSION_NAME,
                discoveryServiceType = TEST_SERVICE_TYPE,
            )
        super.callApplicationOnCreate(app)
    }

    companion object {
        const val TEST_SERVICE_TYPE = "_adelpostest._tcp"
    }
}
