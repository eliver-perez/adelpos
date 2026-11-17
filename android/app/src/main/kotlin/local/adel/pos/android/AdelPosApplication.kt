package local.adel.pos.android

import android.app.Application
import androidx.lifecycle.ProcessLifecycleOwner

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

    override fun onCreate() {
        super.onCreate()
        // Fase D: el catálogo guardado, su actualización y el canal de tiempo real (abierto solo
        // con la aplicación en primer plano y un mesero con sesión). AdelTestRunner ya dejó sus
        // ajustes antes de este punto.
        ProcessLifecycleOwner.get().lifecycle.addObserver(container.appForeground)
        container.catalogRealtime.start()
    }
}
