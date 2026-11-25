package com.adelpos.core.ui

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element

/**
 * Regla de textos de la etapa A3 (§1, A3-D11; regla 2 de `android/CLAUDE.md`): ningún texto
 * provisional o de desarrollo llega a la pantalla. Recorre **todos** los módulos (como
 * `ColorsOnlyInPaletteTest`):
 * - los recursos XML de las carpetas `values` de `src/main/res`: cada `<string>` y cada `<item>` de
 *   `<plurals>` y `<string-array>` (los comentarios XML no son interfaz);
 * - las cadenas literales de los `.kt` de `src/main/kotlin` (sin comentarios y sin las líneas de
 *   `require`, `check`, `error(`, `Log.` ni `…Exception(`, que nunca se muestran): son los pocos
 *   textos que arma `core:data` (por ejemplo la bitácora de pedidos sin enviar).
 *
 * Busca las frases de §0.2 del avance sin distinguir mayúsculas. Una excepción se agrega a
 * [allowed] con el archivo, el patrón y su porqué: así no se puede exceptuar nada sin escribir el
 * motivo, y una excepción que ya no encuentra nada hace fallar la prueba (no quedan excepciones
 * viejas). Hoy no hace falta ninguna: los «todavía no» que describían un estado real se
 * reescribieron (A3-D11) y la pantalla de muestra del catálogo se quitó (A3-D10).
 */
class PlainLanguageTest {
    // Etapa K1: los tres proyectos Android (android/, android-core/ y cocina/) siguen la misma
    // regla.
    private val repositoryRoot =
        File(System.getProperty("adelpos.repositoryRoot") ?: error("Falta adelpos.repositoryRoot"))

    private val androidRoots =
        listOf("android", "android-core", "cocina")
            .map(repositoryRoot::resolve)
            .filter(File::isDirectory)

    /**
     * Una excepción escrita: el texto de [file] que coincide con [pattern] se permite por [reason].
     */
    private class Allowed(val file: String, val pattern: Regex, val reason: String)

    private val allowed: List<Allowed> = emptyList()

    /** Un texto que llega a la pantalla, con dónde está. */
    private data class Shown(val file: File, val line: Int, val text: String)

    private fun sources(extension: String, folder: (File) -> Boolean): List<File> =
        androidRoots.flatMap { root ->
            root
                .walkTopDown()
                .onEnter { it.name != "build" && !it.name.startsWith(".") }
                .filter {
                    it.isFile &&
                        it.extension == extension &&
                        it.path.contains(
                            "${File.separator}src${File.separator}main${File.separator}",
                        ) &&
                        folder(it)
                }
                .toList()
        }

    private fun resourceFiles(): List<File> =
        sources("xml") { file -> file.parentFile?.name?.startsWith("values") == true }

    private fun kotlinFiles(): List<File> = sources("kt") { true }

    @Test
    fun ningunTextoDeLaInterfazTieneNotasDeDesarrollo() {
        val resources = resourceFiles()
        val kotlin = kotlinFiles()
        assertTrue("No se encontraron recursos en $androidRoots", resources.size >= 7)
        assertTrue("No se encontraron fuentes en $androidRoots", kotlin.size > 50)
        val shown =
            resources.flatMap { file -> resourceTexts(file.readText()).map { it.at(file) } } +
                kotlin.flatMap { file -> kotlinTexts(file.readText()).map { it.at(file) } }
        assertTrue("Se esperaban cientos de textos", shown.size > 300)
        val used = mutableSetOf<Allowed>()
        val offenders = shown.mapNotNull { text ->
            val phrase = DEVELOPMENT_PHRASES.firstOrNull { it.containsMatchIn(text.text) }
            if (phrase == null) {
                return@mapNotNull null
            }
            val exception = allowed.firstOrNull { rule ->
                text.file.invariantPath().endsWith(rule.file) &&
                    rule.pattern.containsMatchIn(text.text)
            }
            if (exception != null) {
                used.add(exception)
                return@mapNotNull null
            }
            "${text.file.relativeTo(repositoryRoot)}:${text.line}: «${text.text}» (${phrase.pattern})"
        }
        assertEquals(
            "Textos provisionales o de desarrollo en la interfaz (quítalos o reescríbelos; " +
                "una excepción va en «allowed» con su porqué):\n" +
                offenders.joinToString("\n"),
            emptyList<String>(),
            offenders,
        )
        val stale = allowed.filter { it !in used }.map { "${it.file}: ${it.pattern.pattern}" }
        assertEquals("Excepciones que ya no encuentran nada", emptyList<String>(), stale)
    }

    /** El lector de recursos y el de Kotlin encuentran lo que deben y no los comentarios. */
    @Test
    fun laComprobacionLeeLosTextosYNoLosComentarios() {
        val xml =
            """
            <?xml version="1.0" encoding="utf-8"?>
            <!-- Textos de la etapa A2: este comentario no es interfaz. -->
            <resources>
                <string name="a">Todavía no hay mesas.</string>
                <plurals name="b"><item quantity="one">%d mesa (muestra)</item></plurals>
                <string-array name="c"><item>Próximamente</item></string-array>
            </resources>
            """
                .trimIndent()
        val fromXml = resourceTexts(xml).map { it.second }
        assertEquals(listOf("Todavía no hay mesas.", "%d mesa (muestra)", "Próximamente"), fromXml)
        val kotlin =
            """
            // Fase A: este comentario no es interfaz, ni "este texto en un comentario".
            /* Un bloque /* anidado con "comillas" */ sigue siendo comentario. */
            val shown = "Mesa ${'$'}{table.name} · etapa de prueba"
            val raw = ${"\"\"\""}En desarrollo${"\"\"\""}
            val quote = '"'
            require(ok) { "todavía no se validó" }
            val after = "Más adelante"
            """
                .trimIndent()
        val fromKotlin = kotlinTexts(kotlin)
        assertEquals(
            listOf(3 to "Mesa  · etapa de prueba", 4 to "En desarrollo", 7 to "Más adelante"),
            fromKotlin,
        )
        assertTrue(
            fromKotlin.all { (_, text) -> DEVELOPMENT_PHRASES.any { it.containsMatchIn(text) } },
        )
    }

    private fun Pair<Int, String>.at(file: File) = Shown(file, first, second)

    private fun File.invariantPath(): String = path.replace(File.separatorChar, '/')

    private companion object {
        /**
         * Las frases típicas de §1 y §0.2: notas de etapas o fases, «todavía no»/«aún no»,
         * «próximamente», «provisional» y otras promesas de algo que vendrá.
         */
        val DEVELOPMENT_PHRASES: List<Regex> =
            listOf(
                Regex("todav[ií]a no", RegexOption.IGNORE_CASE),
                Regex("\\ba[uú]n no\\b", RegexOption.IGNORE_CASE),
                Regex("\\betapa\\b", RegexOption.IGNORE_CASE),
                Regex("\\b[Ff]ase [0-9A-Z]\\b"),
                Regex("pr[oó]ximamente", RegexOption.IGNORE_CASE),
                Regex("provisional", RegexOption.IGNORE_CASE),
                Regex("por ahora", RegexOption.IGNORE_CASE),
                Regex("m[aá]s adelante", RegexOption.IGNORE_CASE),
                Regex("en desarrollo", RegexOption.IGNORE_CASE),
                Regex("versi[oó]n de prueba", RegexOption.IGNORE_CASE),
                Regex("\\(muestra\\)", RegexOption.IGNORE_CASE),
            )

        // Líneas de Kotlin cuyas cadenas nunca se muestran: validaciones, errores y registro.
        val NOT_SHOWN_LINE =
            Regex(
                "\\b(require|requireNotNull|check|checkNotNull|error)\\s*\\(|\\bLog\\.|Exception\\(",
            )

        /** Los textos de un archivo de recursos (línea aproximada 0: el XML no da líneas). */
        fun resourceTexts(text: String): List<Pair<Int, String>> {
            val factory = DocumentBuilderFactory.newInstance()
            factory.isNamespaceAware = false
            val document = factory.newDocumentBuilder().parse(text.byteInputStream())
            val lines = text.lines()
            val found = mutableListOf<Pair<Int, String>>()
            fun lineOf(name: String?): Int =
                if (name == null) {
                    0
                } else {
                    lines.indexOfFirst { it.contains("name=\"$name\"") } + 1
                }
            val strings = document.getElementsByTagName("string")
            for (index in 0 until strings.length) {
                val element = strings.item(index) as Element
                found.add(lineOf(element.getAttribute("name")) to element.textContent.trim())
            }
            val items = document.getElementsByTagName("item")
            for (index in 0 until items.length) {
                val element = items.item(index) as Element
                val parent = element.parentNode as? Element ?: continue
                if (parent.tagName == "plurals" || parent.tagName == "string-array") {
                    found.add(lineOf(parent.getAttribute("name")) to element.textContent.trim())
                }
            }
            return found
        }

        /**
         * Las cadenas literales de un archivo de Kotlin con su línea: sin comentarios (también los
         * bloques anidados), sin lo que va dentro de `${…}` y sin las líneas de [NOT_SHOWN_LINE].
         */
        fun kotlinTexts(source: String): List<Pair<Int, String>> {
            val lines = source.lines()
            val found = mutableListOf<Pair<Int, String>>()
            var index = 0
            var line = 1
            fun peek(offset: Int = 0): Char? = source.getOrNull(index + offset)
            fun advance() {
                if (source[index] == '\n') {
                    line += 1
                }
                index += 1
            }
            while (index < source.length) {
                when {
                    source.startsWith("//", index) -> {
                        while (index < source.length && source[index] != '\n') {
                            advance()
                        }
                    }
                    source.startsWith("/*", index) -> {
                        var depth = 0
                        do {
                            if (source.startsWith("/*", index)) {
                                depth += 1
                                advance()
                                advance()
                            } else if (source.startsWith("*/", index)) {
                                depth -= 1
                                advance()
                                advance()
                            } else {
                                advance()
                            }
                        } while (depth > 0 && index < source.length)
                    }
                    peek() == '\'' -> {
                        // Un carácter: '"' o '\'' no abre una cadena.
                        advance()
                        while (index < source.length && peek() != '\'') {
                            if (peek() == '\\') {
                                advance()
                            }
                            advance()
                        }
                        if (index < source.length) {
                            advance()
                        }
                    }
                    peek() == '"' -> {
                        val raw = source.startsWith("\"\"\"", index)
                        val startLine = line
                        repeat(if (raw) 3 else 1) { advance() }
                        val text = StringBuilder()
                        while (index < source.length) {
                            if (raw && source.startsWith("\"\"\"", index)) {
                                repeat(3) { advance() }
                                break
                            }
                            if (!raw && peek() == '"') {
                                advance()
                                break
                            }
                            if (!raw && peek() == '\\') {
                                advance()
                                if (index < source.length) {
                                    text.append(source[index])
                                    advance()
                                }
                                continue
                            }
                            if (peek() == '$' && peek(1) == '{') {
                                skipTemplate(source, index).let { end ->
                                    while (index < end) {
                                        advance()
                                    }
                                }
                                continue
                            }
                            text.append(source[index])
                            advance()
                        }
                        val lineText = lines.getOrElse(startLine - 1) { "" }
                        if (text.isNotBlank() && !NOT_SHOWN_LINE.containsMatchIn(lineText)) {
                            found.add(startLine to text.toString())
                        }
                    }
                    else -> advance()
                }
            }
            return found
        }

        // El final de una plantilla `${…}` que empieza en [start] (llaves anidadas incluidas).
        private fun skipTemplate(source: String, start: Int): Int {
            var depth = 0
            var position = start + 1
            while (position < source.length) {
                when (source[position]) {
                    '{' -> depth += 1
                    '}' -> {
                        depth -= 1
                        if (depth == 0) {
                            return position + 1
                        }
                    }
                }
                position += 1
            }
            return position
        }
    }
}
