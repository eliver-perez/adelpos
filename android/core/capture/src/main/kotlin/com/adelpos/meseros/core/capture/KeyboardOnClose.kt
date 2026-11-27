package com.adelpos.meseros.core.capture

import android.view.inputmethod.InputMethodManager
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.LocalView

/**
 * Etapa M1 (§5.7): en Android 8, al cerrar un diálogo con un campo de texto, Android pasa el
 * teclado a la ventana de la actividad y lo deja abierto, tapando lo que sigue (en Para llevar, el
 * pedido editado quedaba fuera de la vista). Se llama en la composición de la pantalla, **antes**
 * del diálogo: al salir el diálogo de la composición se esconde el teclado desde la ventana de la
 * actividad, un momento después (cuando Android ya quitó la ventana del diálogo). En Android 11+ el
 * teclado ya se cerraba solo; esconderlo otra vez no hace nada.
 */
@Composable
internal fun HideKeyboardWhenClosed() {
    val host = LocalView.current
    DisposableEffect(host) {
        onDispose {
            // Dos intentos: Android quita la ventana del diálogo y pasa el teclado a la actividad
            // un momento después (más tarde con el emulador cargado).
            for (delay in KEYBOARD_HIDE_DELAYS_MILLIS) {
                host.postDelayed(
                    {
                        host.context
                            .getSystemService(InputMethodManager::class.java)
                            ?.hideSoftInputFromWindow(host.windowToken, 0)
                    },
                    delay,
                )
            }
        }
    }
}

/**
 * El cierre de un diálogo con texto: suelta el foco y esconde el teclado **mientras el diálogo
 * todavía lo tiene** (en Android 8 es lo que de verdad lo cierra) y después hace [action].
 */
@Composable
internal fun rememberKeyboardCloser(): (() -> Unit) -> Unit {
    val keyboard = LocalSoftwareKeyboardController.current
    val focus = LocalFocusManager.current
    return { action ->
        focus.clearFocus(force = true)
        keyboard?.hide()
        action()
    }
}

/** Esperas (ms) para esconder el teclado después de cerrar el diálogo. */
private val KEYBOARD_HIDE_DELAYS_MILLIS = listOf(150L, 600L)
