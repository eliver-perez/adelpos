package local.adel.pos.android

import android.app.Application

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
    }
}
