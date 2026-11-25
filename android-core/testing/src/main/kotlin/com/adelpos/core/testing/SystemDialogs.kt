package com.adelpos.core.testing

import android.accessibilityservice.AccessibilityServiceInfo
import android.app.UiAutomation
import android.util.Log
import android.view.accessibility.AccessibilityNodeInfo

/**
 * Diálogos del sistema que estorban a las pruebas en un emulador cargado (Fase F): «Pixel Launcher
 * isn't responding» (ANR del lanzador) se queda con el foco de la pantalla y la tecla Atrás que
 * inyecta la instrumentación ya no llega a la aplicación (no puede inyectar en ventanas ajenas).
 * Antes de Atrás se cierra con «Wait»/«Esperar» (no cierra ninguna aplicación). Solo actúa si la
 * ventana activa es ese diálogo.
 */
object SystemDialogs {
    private val NOT_RESPONDING = listOf("isn't responding", "no responde")
    private val WAIT = listOf("Wait", "Esperar")

    /** Cierra el diálogo «… no responde» si está a la vista; true si cerró alguno. */
    fun dismissNotResponding(automation: UiAutomation): Boolean {
        val root = automation.rootInActiveWindow ?: return false
        val notResponding = NOT_RESPONDING.any {
            root.findAccessibilityNodeInfosByText(it).isNotEmpty()
        }
        if (!notResponding) {
            return false
        }
        for (label in WAIT) {
            val button = root.findAccessibilityNodeInfosByText(label).firstOrNull() ?: continue
            if (clickable(button)?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) {
                Log.i("AdelOnGo", "diálogo del sistema «no responde» cerrado con «$label»")
                Thread.sleep(500)
                return true
            }
        }
        return false
    }

    private val FULL_SCREEN = listOf("Viewing full screen", "pantalla completa")
    private val GOT_IT = listOf("Got it", "GOT IT", "Entendido", "ENTENDIDO")

    /**
     * Cierra el aviso del sistema «Viewing full screen» con «Got it»/«Entendido»: Android lo
     * muestra la primera vez que una aplicación a pantalla completa (la de cocina, etapa K1) se
     * abre y tapa las capturas. Es una ventana del sistema que no toma el foco: se busca entre
     * todas las ventanas. Solo actúa si el aviso está a la vista; true si lo cerró.
     */
    fun dismissFullScreenNotice(automation: UiAutomation): Boolean {
        val info = automation.serviceInfo
        if (
            info != null &&
                info.flags and AccessibilityServiceInfo.FLAG_RETRIEVE_INTERACTIVE_WINDOWS == 0
        ) {
            info.flags = info.flags or AccessibilityServiceInfo.FLAG_RETRIEVE_INTERACTIVE_WINDOWS
            automation.serviceInfo = info
        }
        val roots =
            listOfNotNull(automation.rootInActiveWindow) + automation.windows.mapNotNull { it.root }
        for (root in roots) {
            if (FULL_SCREEN.none { root.findAccessibilityNodeInfosByText(it).isNotEmpty() }) {
                continue
            }
            for (label in GOT_IT) {
                val button = root.findAccessibilityNodeInfosByText(label).firstOrNull() ?: continue
                if (clickable(button)?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) {
                    Log.i("AdelOnGo", "aviso del sistema de pantalla completa cerrado con «$label»")
                    Thread.sleep(500)
                    return true
                }
            }
        }
        return false
    }

    private fun clickable(node: AccessibilityNodeInfo): AccessibilityNodeInfo? {
        var current: AccessibilityNodeInfo? = node
        while (current != null && !current.isClickable) {
            current = current.parent
        }
        return current
    }
}
