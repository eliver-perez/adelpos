package local.adel.pos.android

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import local.adel.pos.android.navigation.AdelPosApp

/**
 * Única actividad. De borde a borde en todas las versiones (obligatorio desde Android 15 con
 * targetSdk 35+; en Android 8 a 14 se activa igual para que se vea lo mismo) y la interfaz respeta
 * las barras del sistema con sus insets.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent { AdelPosApp() }
    }
}
