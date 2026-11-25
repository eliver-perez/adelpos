package com.adelpos.cocina

import android.app.Application
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnitRunner
import com.adelpos.core.model.server.ServerAnnouncements

/**
 * Ejecutor de las pruebas instrumentadas de la aplicación de cocina (como `AdelTestRunner` de la de
 * meseros). Antes de que nada use el contenedor:
 * - la búsqueda NSD usa el tipo de pruebas `_adelpostest._tcp`, para que ninguna prueba encuentre
 *   (ni pruebe con su huella) el servidor real del restaurante que se anuncia en la red de la Mac;
 * - con el argumento de instrumentación `clientVersion`, la aplicación se presenta al servidor con
 *   esa versión (prueba de «Actualiza esta aplicación», 426).
 */
class CocinaTestRunner : AndroidJUnitRunner() {
    override fun callApplicationOnCreate(app: Application) {
        val arguments = InstrumentationRegistry.getArguments()
        (app as CocinaApplication).containerSettings =
            CocinaContainer.Settings(
                clientVersion = arguments.getString("clientVersion") ?: BuildConfig.VERSION_NAME,
                discoveryServiceType = ServerAnnouncements.TEST_SERVICE_TYPE,
            )
        super.callApplicationOnCreate(app)
    }
}
