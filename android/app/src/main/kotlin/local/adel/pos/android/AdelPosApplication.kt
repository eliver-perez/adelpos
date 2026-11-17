package local.adel.pos.android

import android.app.Application
import androidx.lifecycle.ProcessLifecycleOwner

/**
 * Proceso de la aplicación: crea el contenedor de dependencias una sola vez (inyección manual, sin
 * Hilt).
 */
class AdelPosApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
        // Fase D: el catálogo guardado, su actualización y el canal de tiempo real (abierto solo
        // con la aplicación en primer plano).
        ProcessLifecycleOwner.get().lifecycle.addObserver(container.appForeground)
        container.catalogRealtime.start()
    }
}
