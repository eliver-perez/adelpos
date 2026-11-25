package com.adelpos.cocina

import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import com.adelpos.cocina.ui.CocinaApp

/**
 * Única actividad, en horizontal (manifiesto) y a pantalla completa (§4 de la etapa K1): las barras
 * del sistema se esconden y vuelven un momento al deslizar desde el borde. De borde a borde en
 * todas las versiones, como la aplicación de meseros. La pantalla se queda encendida (§5.3) y, ya
 * emparejada, abre directo en la pantalla de pedidos (lo guardado en Room).
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        hideSystemBars()
        // §5.3: la pantalla nunca se apaga mientras la aplicación está abierta (solo esta ventana;
        // no hace falta ningún permiso).
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        val container = (application as CocinaApplication).container
        setContent { CocinaApp(container) }
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        // Un diálogo del sistema (permisos, el teclado) puede volver a mostrar las barras.
        if (hasFocus) {
            hideSystemBars()
        }
    }

    private fun hideSystemBars() {
        val controller = WindowCompat.getInsetsController(window, window.decorView)
        controller.systemBarsBehavior =
            WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        controller.hide(WindowInsetsCompat.Type.systemBars())
    }
}
