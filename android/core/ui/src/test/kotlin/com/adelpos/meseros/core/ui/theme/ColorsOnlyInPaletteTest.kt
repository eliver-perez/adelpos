package com.adelpos.meseros.core.ui.theme

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Los colores solo se escriben en AdelPalette.kt (como los tokens de styles.css en el escritorio):
 * ningún otro archivo de la aplicación usa `Color(0x…)`, colores con nombre de Compose
 * (`Color.Red`…), `Color.parse`/`toColorInt` ni colores hexadecimales en los recursos XML.
 * `Color.Transparent` y `Color.Unspecified` se permiten.
 */
class ColorsOnlyInPaletteTest {
    private val androidRoot =
        File(System.getProperty("adelpos.androidRoot") ?: error("Falta adelpos.androidRoot"))

    private val kotlinColor =
        Regex(
            "Color\\(\\s*0x|Color\\(\\s*(red|green|blue)\\s*=|Color\\.(Black|White|Red|Green|Blue|Yellow|Cyan|Magenta|" +
                "Gray|LightGray|DarkGray)\\b|parseColor|toColorInt|Color\\.rgb\\(|Color\\.argb\\(",
        )
    private val xmlColor = Regex("#[0-9a-fA-F]{3,8}\\b|@android:color/(?!transparent)")

    private fun sources(extension: String): List<File> =
        androidRoot
            .walkTopDown()
            .onEnter { it.name != "build" && !it.name.startsWith(".") }
            .filter {
                it.isFile &&
                    it.extension == extension &&
                    it.path.contains("${File.separator}src${File.separator}main${File.separator}")
            }
            .toList()

    @Test
    fun soloLaPaletaEscribeColoresEnKotlin() {
        val kotlinFiles = sources("kt")
        assertTrue("No se encontraron fuentes en $androidRoot", kotlinFiles.size > 10)
        val offenders =
            kotlinFiles
                .filter { it.name != "AdelPalette.kt" }
                .flatMap { file ->
                    file.readLines().mapIndexedNotNull { index, line ->
                        if (kotlinColor.containsMatchIn(line))
                            "${file.relativeTo(androidRoot)}:${index + 1}: $line"
                        else null
                    }
                }
        assertEquals(
            "Colores fuera de AdelPalette.kt:\n" + offenders.joinToString("\n"),
            emptyList<String>(),
            offenders,
        )
    }

    @Test
    fun losRecursosNoEscribenColores() {
        val offenders =
            sources("xml").flatMap { file ->
                file.readLines().mapIndexedNotNull { index, line ->
                    if (xmlColor.containsMatchIn(line))
                        "${file.relativeTo(androidRoot)}:${index + 1}: $line"
                    else null
                }
            }
        assertEquals(
            "Colores en recursos XML:\n" + offenders.joinToString("\n"),
            emptyList<String>(),
            offenders,
        )
    }
}
