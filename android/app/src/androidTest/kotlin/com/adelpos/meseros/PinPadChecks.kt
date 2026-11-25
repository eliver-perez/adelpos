package com.adelpos.meseros

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.ComposeTestRule
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.components.TAG_PIN_DELETE
import com.adelpos.meseros.core.ui.components.TAG_PIN_KEY_PREFIX
import com.adelpos.meseros.core.ui.components.TAG_PIN_SUBMIT
import org.junit.Assert.assertTrue

/**
 * Etapa A3 (§4): el teclado del PIN se ve **completo** (las teclas 0 a 9, «Borrar» e «Ingresar»)
 * dentro de [containerTag] (el panel del PIN o el diálogo «Vuelve a escribir tu PIN»), sin
 * deslizar, cada tecla mide al menos el mínimo táctil y su nombre cabe en un renglón. Antes, en el
 * teléfono horizontal solo se veían las teclas 1 a 6.
 */
object PinPadChecks {
    private val KEYS =
        ("1234567890".map { "$TAG_PIN_KEY_PREFIX$it" }) + TAG_PIN_DELETE + TAG_PIN_SUBMIT

    fun assertWholePad(compose: ComposeTestRule, containerTag: String, where: String) {
        compose.waitForIdle()
        val container =
            compose
                .onAllNodes(hasTestTag(containerTag), useUnmergedTree = true)
                .onFirst()
                .fetchSemanticsNode()
        val bounds = container.boundsInRoot
        val minimum = with(container.layoutInfo.density) { AdelDimens.MinTouchTarget.toPx() }
        // Medio dp de tolerancia por el redondeo a píxeles.
        val tolerance = with(container.layoutInfo.density) { 0.5.dp.toPx() }
        for (key in KEYS) {
            val node =
                compose
                    .onAllNodes(hasTestTag(key), useUnmergedTree = true)
                    .onFirst()
                    .fetchSemanticsNode()
            val box = node.boundsInRoot
            assertTrue(
                "$key completa a la vista en $where ($box dentro de $bounds)",
                box.top >= bounds.top - tolerance &&
                    box.bottom <= bounds.bottom + tolerance &&
                    box.left >= bounds.left - tolerance &&
                    box.right <= bounds.right + tolerance,
            )
            assertTrue(
                "$key de al menos 48 dp en $where (${box.height} px)",
                box.height >= minimum - tolerance,
            )
            // El nombre de la tecla en un renglón y completo («Ingresar», «Borrar», el dígito).
            val labels =
                compose
                    .onAllNodes(
                        hasAnyAncestor(hasTestTag(key)) and
                            SemanticsMatcher.keyIsDefined(SemanticsProperties.Text),
                        useUnmergedTree = true,
                    )
                    .fetchSemanticsNodes()
            assertTrue("$key con su nombre en $where", labels.isNotEmpty())
            for (label in labels) {
                val layouts = mutableListOf<TextLayoutResult>()
                label.config[SemanticsActions.GetTextLayoutResult].action?.invoke(layouts)
                val layout = layouts.single()
                assertTrue(
                    "$key: «${layout.layoutInput.text}» completo en un renglón en $where",
                    layout.lineCount == 1 &&
                        layout.getLineEnd(0) == layout.layoutInput.text.length &&
                        !layout.multiParagraph.didExceedMaxLines,
                )
            }
        }
    }
}
