package com.adelpos.meseros.core.ui.components

import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput

/**
 * Qué hacer con cada toque del mesero (reiniciar el plazo del bloqueo por inactividad, §3.2). Lo da
 * la raíz de la aplicación; sin raíz (vistas previas, pruebas de una pantalla) no hace nada.
 */
val LocalUserActivity = staticCompositionLocalOf<() -> Unit> { {} }

/**
 * Avisa [onActivity] con cada toque dentro de este elemento, antes que sus hijos y sin quitarles el
 * toque. Es para las **ventanas propias** de Compose (`AlertDialog`, `DropdownMenu`, hojas): sus
 * toques no pasan por `Activity.onUserInteraction`, así que sin esto un mesero que usa un diálogo
 * más que el plazo se bloquearía en medio (revisión R3, B4). Uso:
 * `Modifier.reportsUserActivity(LocalUserActivity.current)`.
 */
fun Modifier.reportsUserActivity(onActivity: () -> Unit): Modifier =
    this.pointerInput(onActivity) {
        awaitPointerEventScope {
            while (true) {
                awaitPointerEvent(PointerEventPass.Initial)
                onActivity()
            }
        }
    }
