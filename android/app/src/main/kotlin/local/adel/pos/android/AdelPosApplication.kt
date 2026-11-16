package local.adel.pos.android

import android.app.Application

/**
 * Proceso de la aplicación: crea el contenedor de dependencias una sola vez, al primer uso
 * (inyección manual, sin Hilt).
 */
class AdelPosApplication : Application() {
    /**
     * Ajustes del contenedor. Solo las pruebas instrumentadas (`AdelTestRunner`) los cambian, al
     * arrancar y antes de que nada use el contenedor.
     */
    var containerSettings: AppContainer.Settings = AppContainer.Settings()

    val container: AppContainer by lazy { AppContainer(this, containerSettings) }
}
