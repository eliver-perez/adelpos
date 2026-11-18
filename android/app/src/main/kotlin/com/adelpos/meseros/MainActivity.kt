package com.adelpos.meseros

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.adelpos.meseros.navigation.AdelPosApp

/**
 * Única actividad. De borde a borde en todas las versiones (obligatorio desde Android 15 con
 * targetSdk 35+; en Android 8 a 14 se activa igual para que se vea lo mismo) y la interfaz respeta
 * las barras del sistema con sus insets.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        // Un toque en un aviso de platillos listos o en «AD-EL POS conectado» (etapa A2); al
        // volver a crear la actividad (rotación) ya se atendió.
        if (savedInstanceState == null) {
            (application as AdelPosApplication).container.readyAlerts.handleIntent(intent)
        }
        setContent { AdelPosApp() }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        (application as AdelPosApplication).container.readyAlerts.handleIntent(intent)
    }

    // Cada toque o tecla reinicia el plazo del bloqueo por inactividad (§3.2).
    override fun onUserInteraction() {
        super.onUserInteraction()
        (application as AdelPosApplication).container.idleLock.touch()
    }
}
