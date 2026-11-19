package com.adelpos.meseros.core.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf

/**
 * La flecha de regreso de la barra superior (etapa A3, §4.1): además de las pantallas que no son la
 * principal (captura, pendientes, revisión), la muestra un panel «a un toque» abierto en el
 * teléfono (la mesa o la cuenta de `AdaptivePanes`). La raíz la provee con [LocalTopBarBack]; el
 * panel se registra mientras está abierto y entonces no dibuja su propio «Volver a …» (una sola
 * flecha, sin doble Atrás). La flecha llama al Atrás del sistema, así pasa por los mismos
 * `BackHandler` (el del panel primero).
 */
class TopBarBack {
    private var openPanels by mutableIntStateOf(0)

    /** Hay un panel «a un toque» abierto: la flecha lo cierra. */
    val panelOpen: Boolean
        get() = openPanels > 0

    fun panelOpened() {
        openPanels += 1
    }

    fun panelClosed() {
        openPanels = (openPanels - 1).coerceAtLeast(0)
    }
}

/** La flecha de la barra superior, si la raíz la provee (null: el panel dibuja su «Volver a …»). */
val LocalTopBarBack = staticCompositionLocalOf<TopBarBack?> { null }

/**
 * El Atrás del sistema (`BackHandler` de activity-compose, también el atrás predictivo) para los
 * módulos que no dependen de activity-compose (la captura de `core:capture`, etapa A3, §4.1).
 */
@Composable
fun SystemBackHandler(enabled: Boolean = true, onBack: () -> Unit) {
    BackHandler(enabled = enabled, onBack = onBack)
}
