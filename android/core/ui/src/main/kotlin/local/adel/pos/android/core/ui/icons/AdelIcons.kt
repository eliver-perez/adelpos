package local.adel.pos.android.core.ui.icons

import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.ui.theme.IconMask

/**
 * Íconos propios de AD-EL POS, los mismos trazos SVG de `desktop/src/components/Icon.tsx` (24×24,
 * trazo de 1.6, sin relleno, extremos y uniones redondeados), sin la biblioteca de íconos extendida
 * de Material. Los círculos y rectángulos del SVG se escriben como trazos equivalentes. Son
 * decorativos: el texto del control o del estado los acompaña siempre.
 */
object AdelIcons {
    /** Libre (estado de mesa). */
    val Check: ImageVector by lazy { icon("check", "m5 12 4 4L19 6") }

    /** Ocupada (estado de mesa). */
    val Clock: ImageVector by lazy { icon("clock", circle(12f, 12f, 9f), "M12 7v5l3 2") }

    /** Esperando pago (estado de mesa). */
    val Receipt: ImageVector by lazy {
        icon("receipt", "M6 3h12v18l-2-1.5-2 1.5-2-1.5-2 1.5-2-1.5L6 21V3Z", "M9 8h6M9 12h6M9 16h3")
    }

    /** Pago parcial (estado de mesa). */
    val Cash: ImageVector by lazy {
        icon(
            "cash",
            roundedRect(2.5f, 6f, 19f, 12f, 2f),
            circle(12f, 12f, 2.6f),
            "M6 9.5v5M18 9.5v5",
        )
    }

    /** Fuera de servicio (estado de mesa). */
    val Lock: ImageVector by lazy {
        icon("lock", roundedRect(5f, 11f, 14f, 10f, 2f), "M8 11V7a4 4 0 0 1 8 0v4")
    }

    /** Pedidos por enviar. */
    val Send: ImageVector by lazy { icon("send", "M21 3 10 14", "M21 3 14 21l-4-7-7-4 18-7Z") }

    /** Volver al panel anterior. */
    val ChevronLeft: ImageVector by lazy { icon("chevron-left", "m15 5-7 7 7 7") }

    private fun icon(name: String, vararg paths: String): ImageVector {
        val builder =
            ImageVector.Builder(
                name = name,
                defaultWidth = 24.dp,
                defaultHeight = 24.dp,
                viewportWidth = 24f,
                viewportHeight = 24f,
            )
        for (path in paths) {
            builder.addPath(
                pathData = addPathNodes(path),
                fill = null,
                stroke = SolidColor(IconMask),
                strokeLineWidth = 1.6f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
        return builder.build()
    }

    // <circle cx cy r> como dos arcos.
    private fun circle(cx: Float, cy: Float, r: Float): String =
        "M${cx - r} ${cy}a$r $r 0 1 0 ${2 * r} 0a$r $r 0 1 0 ${-2 * r} 0Z"

    // <rect x y width height rx> como trazo con esquinas redondeadas.
    private fun roundedRect(x: Float, y: Float, width: Float, height: Float, rx: Float): String {
        val right = x + width
        val bottom = y + height
        return "M${x + rx} ${y}H${right - rx}A$rx $rx 0 0 1 $right ${y + rx}V${bottom - rx}" +
            "A$rx $rx 0 0 1 ${right - rx} ${bottom}H${x + rx}A$rx $rx 0 0 1 $x ${bottom - rx}V${y + rx}" +
            "A$rx $rx 0 0 1 ${x + rx} ${y}Z"
    }
}
