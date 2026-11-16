package local.adel.pos.android.core.ui.theme

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
}
