package com.adelpos.core.ui.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Contraste de los pares principales en ambos temas (los mismos criterios del escritorio,
 * styles.test.ts): texto ≥ 4.5:1 y bordes o puntos de estado ≥ 3:1. Protege los tokens copiados de
 * styles.css de un cambio accidental.
 */
class PaletteContrastTest {
    private fun contrast(foreground: Color, background: Color): Double {
        val lighter = maxOf(foreground.luminance(), background.luminance()) + 0.05
        val darker = minOf(foreground.luminance(), background.luminance()) + 0.05
        return lighter / darker
    }

    private fun assertText(name: String, foreground: Color, background: Color) {
        val ratio = contrast(foreground, background)
        assertTrue("$name: $ratio < 4.5", ratio >= 4.5)
    }

    private fun assertMark(name: String, foreground: Color, background: Color) {
        val ratio = contrast(foreground, background)
        assertTrue("$name: $ratio < 3", ratio >= 3.0)
    }

    @Test
    fun laMarcaEsLaDelEscritorio() {
        assertEquals(Color(0xFF1F5FA8), BrandBlue)
        assertEquals(Color(0xFF173D6E), BrandBluePressed)
        assertEquals(BrandBlue, LightAdelColors.blue)
    }

    @Test
    fun paresPrincipalesEnAmbosTemas() {
        for (colors in listOf(LightAdelColors, DarkAdelColors)) {
            val theme = if (colors.isDark) "oscuro" else "claro"
            assertText("$theme texto/fondo", colors.text, colors.background)
            assertText("$theme texto/panel", colors.text, colors.panel)
            assertText("$theme secundario/panel", colors.muted, colors.panel)
            assertText("$theme sobre el azul", colors.onAccent, colors.blue)
            assertText("$theme azul/panel", colors.blue, colors.panel)
            assertText("$theme ámbar/panel", colors.amber, colors.panel)
            assertText("$theme ámbar/suave", colors.amber, colors.amberSoft)
            assertText("$theme rojo/panel", colors.red, colors.panel)
            assertMark("$theme punto en línea", colors.statusOnline, colors.panel)
            assertMark("$theme punto sin conexión", colors.statusOffline, colors.panel)
            for (table in
                listOf(
                    colors.tableFree,
                    colors.tableOccupied,
                    colors.tableAwaitingPayment,
                    colors.tablePartialPayment,
                    colors.tableOutOfService,
                )) {
                assertText("$theme estado de mesa $table", table.content, table.background)
            }
        }
    }

    /**
     * Distintivo de los meseros con cada color de la paleta de Go (`catalog.Palette`, leída de
     * `backend/internal/catalog/palette.go`): la inicial (texto grande y en negritas) contrasta al
     * menos 3:1 con el círculo, y sin color se usa el azul suave con texto ≥ 4.5:1, en ambos temas.
     */
    @Test
    fun laInicialDelDistintivoSeLeeConCadaColorDeGo() {
        val repositoryRoot =
            java.io.File(
                System.getProperty("adelpos.repositoryRoot")
                    ?: error("Falta adelpos.repositoryRoot"),
            )
        val palette = repositoryRoot.resolve("backend/internal/catalog/palette.go").readText()
        val hexes =
            Regex("Hex: \"(#[0-9a-f]{6})\"").findAll(palette).map { it.groupValues[1] }.toList()
        assertEquals("la paleta de Go tiene 16 colores", 16, hexes.size)
        for (colors in listOf(LightAdelColors, DarkAdelColors)) {
            for (hex in hexes) {
                val badge = waiterBadgeColors(hex, colors)
                assertMark("distintivo $hex", badge.content, badge.background)
            }
            val none = waiterBadgeColors(null, colors)
            assertText("distintivo sin color", none.content, none.background)
            assertEquals(none, waiterBadgeColors("azul", colors))
            assertEquals(none, waiterBadgeColors("#12345", colors))
        }
        assertEquals(Color(0xFF3591D4), waiterBadgeColors("#3591D4", LightAdelColors).background)
    }
}
