package com.adelpos.meseros.core.ui.layout

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Regla de uno o dos paneles con las medidas reales de los emuladores de la etapa (avance §1.6).
 */
class PaneLayoutTest {
    @Test
    fun telefonoVerticalUnPanel() {
        // Pixel 8: ≈ 411 × 914 dp.
        assertEquals(PaneLayout.SINGLE, paneLayoutFor(widthDp = 411, heightDp = 914))
    }

    @Test
    fun telefonoHorizontalUnPanelPorqueEsMuyBajo() {
        // Ancho expandido pero alto compacto.
        assertEquals(PaneLayout.SINGLE, paneLayoutFor(widthDp = 914, heightDp = 411))
    }

    @Test
    fun tabletaDosPanelesEnAmbasOrientaciones() {
        // Pixel Tablet y la tableta de Android 8: 1280 × 800 dp.
        assertEquals(PaneLayout.DOUBLE, paneLayoutFor(widthDp = 1280, heightDp = 800))
        assertEquals(PaneLayout.DOUBLE, paneLayoutFor(widthDp = 800, heightDp = 1280))
    }

    @Test
    fun limitesDeLaRegla() {
        assertEquals(PaneLayout.DOUBLE, paneLayoutFor(widthDp = 600, heightDp = 480))
        assertEquals(PaneLayout.SINGLE, paneLayoutFor(widthDp = 599, heightDp = 1000))
        assertEquals(PaneLayout.SINGLE, paneLayoutFor(widthDp = 1000, heightDp = 479))
    }
}
