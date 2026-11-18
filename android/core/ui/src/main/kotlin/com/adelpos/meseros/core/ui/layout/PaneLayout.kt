package com.adelpos.meseros.core.ui.layout

import androidx.compose.material3.adaptive.currentWindowAdaptiveInfoV2
import androidx.compose.runtime.Composable
import androidx.window.core.layout.WindowSizeClass

/** Cuántos paneles caben en la ventana. */
enum class PaneLayout {
    /** Teléfono (vertical u horizontal): un panel a la vez, con lo demás «a un toque». */
    SINGLE,

    /** Tableta o pantalla ancha: dos paneles lado a lado (por ejemplo, catálogo y cuenta). */
    DOUBLE,
}

/**
 * Regla de los dos paneles (§0-B y §2): ancho de tableta (≥ 600 dp, clase «media» o mayor) **y**
 * alto suficiente (≥ 480 dp). Así la tableta usa dos paneles en ambas orientaciones (la Pixel
 * Tablet vertical mide 800 dp de ancho) y el teléfono horizontal, que es ancho pero muy bajo (≈ 914
 * × 411 dp), se queda con un panel.
 */
fun paneLayoutFor(widthDp: Int, heightDp: Int): PaneLayout =
    if (
        widthDp >= WindowSizeClass.WIDTH_DP_MEDIUM_LOWER_BOUND &&
            heightDp >= WindowSizeClass.HEIGHT_DP_MEDIUM_LOWER_BOUND
    ) {
        PaneLayout.DOUBLE
    } else {
        PaneLayout.SINGLE
    }

/** Disposición para la ventana actual (cambia al rotar, al redimensionar o en multiventana). */
@Composable
fun currentPaneLayout(): PaneLayout {
    val sizeClass = currentWindowAdaptiveInfoV2().windowSizeClass
    return paneLayoutFor(sizeClass.minWidthDp, sizeClass.minHeightDp)
}
