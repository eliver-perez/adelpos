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

/**
 * Poca altura (< 480 dp, el mismo límite de la regla de los paneles): el teléfono en horizontal (≈
 * 411 dp de alto) o una ventana baja en multiventana. Ahí la navegación va a un lado y el catálogo
 * usa su forma compacta, para que la captura siga mostrando los productos (Fase E de la A2).
 */
fun isHeightCompact(heightDp: Int): Boolean =
    heightDp < WindowSizeClass.HEIGHT_DP_MEDIUM_LOWER_BOUND

/** [isHeightCompact] para la ventana actual. */
@Composable
fun currentHeightCompact(): Boolean =
    isHeightCompact(currentWindowAdaptiveInfoV2().windowSizeClass.minHeightDp)

/** Dónde va la navegación principal «Mesas | Para llevar» (etapa A2, §3.1). */
enum class NavigationPlacement {
    /** Barra inferior: en vertical (teléfono y tableta), donde sobra altura. */
    BOTTOM_BAR,

    /** Riel lateral: cuando la altura es lo escaso (teléfono y tableta en horizontal). */
    SIDE_RAIL,
}

/**
 * Regla de la navegación principal, que siempre está a la vista (§3.1) sin quitarle a la captura el
 * espacio que necesita:
 * - **poca altura** (< 480 dp, el mismo límite de la regla de los paneles): riel lateral. En el
 *   teléfono horizontal (≈ 914 × 411 dp) la barra inferior de 80 dp dejaba la cuadrícula de
 *   productos sin altura (Fase E de la A2); el riel toma 80 dp del ancho, que ahí sobra;
 * - **horizontal** (más ancha que alta): también riel. En la tableta horizontal (1280 × 800 dp) lo
 *   escaso para el catálogo y la cuenta es la altura, no el ancho;
 * - **vertical**: barra inferior (en la tableta vertical, 800 dp de ancho, el riel le quitaría a
 *   los dos paneles el ancho que necesitan y la barra solo 80 de 1280 dp de alto).
 */
fun navigationPlacementFor(widthDp: Int, heightDp: Int): NavigationPlacement =
    when {
        isHeightCompact(heightDp) -> NavigationPlacement.SIDE_RAIL
        widthDp > heightDp -> NavigationPlacement.SIDE_RAIL
        else -> NavigationPlacement.BOTTOM_BAR
    }

/** La navegación principal para la ventana actual (cambia al rotar o en multiventana). */
@Composable
fun currentNavigationPlacement(): NavigationPlacement {
    val sizeClass = currentWindowAdaptiveInfoV2().windowSizeClass
    return navigationPlacementFor(sizeClass.minWidthDp, sizeClass.minHeightDp)
}
