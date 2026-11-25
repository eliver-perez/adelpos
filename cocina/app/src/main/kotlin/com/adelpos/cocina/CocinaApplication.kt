package com.adelpos.cocina

import android.app.Application
import androidx.lifecycle.ProcessLifecycleOwner
import com.adelpos.cocina.access.KitchenStatus
import kotlinx.coroutines.launch

/**
 * Proceso de la aplicación de cocina: crea el contenedor de dependencias una sola vez, al primer
 * uso (inyección manual, sin Hilt).
 */
class CocinaApplication : Application() {
    /**
     * Ajustes del contenedor. Solo las pruebas instrumentadas (`CocinaTestRunner`) los cambian, al
     * arrancar y antes de que nada use el contenedor.
     */
    var containerSettings: CocinaContainer.Settings = CocinaContainer.Settings()

    val container: CocinaContainer by lazy { CocinaContainer(this, containerSettings) }

    override fun onCreate() {
        super.onCreate()
        // El primer plano del proceso y el estado único de conexión con su revisión según el uso
        // (etapa A3). CocinaTestRunner ya dejó sus ajustes antes de este punto.
        ProcessLifecycleOwner.get().lifecycle.addObserver(container.appForeground)
        container.serverLink.start()
        // La pantalla de pedidos y su canal de eventos (con la aplicación a la vista).
        container.realtime.start(container.appScope)
        container.board.start(container.appScope)
        // Los ajustes abiertos se cierran (y olvidan la contraseña) si la pantalla deja de estar
        // lista.
        container.appScope.launch {
            container.access.status.collect { status ->
                if (status !is KitchenStatus.Ready) {
                    container.kitchenSettings.onNotReady()
                }
            }
        }
    }
}
