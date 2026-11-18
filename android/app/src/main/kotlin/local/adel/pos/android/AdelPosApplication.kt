package local.adel.pos.android

import android.app.Application
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.work.Configuration

/**
 * Proceso de la aplicación: crea el contenedor de dependencias una sola vez, al primer uso
 * (inyección manual, sin Hilt).
 */
class AdelPosApplication : Application(), Configuration.Provider {
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
        // Bloqueo por inactividad de todo el proceso (revisión R3, A2 y B4).
        container.startIdleLock()
        // Fase E: la cola sin conexión (recuperación tras reiniciar y envío periódico de respaldo)
        // y el modo Mesero.
        container.waiterMode.start()
        container.orderScheduler.schedulePeriodic()
    }

    /**
     * WorkManager se inicia a pedido con la fábrica del trabajador de la cola (inyección manual):
     * el inicializador automático está quitado en el manifiesto.
     */
    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder().setWorkerFactory(container.orderWorkerFactory).build()
}
