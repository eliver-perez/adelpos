package com.adelpos.core.ui.icons

import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.theme.IconMask

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

    /** La flecha de regreso de la barra superior (etapa A3, `back` de Icon.tsx). */
    val Back: ImageVector by lazy { icon("back", "M20 12H4m5-5-5 5 5 5") }

    /** Abrir un menú (el del mesero activo). */
    val ChevronDown: ImageVector by lazy { icon("chevron-down", "m5 9 7 7 7-7") }

    /** Buscar otra vez o volver a comprobar. */
    val Refresh: ImageVector by lazy {
        icon(
            "refresh",
            "M20 7v5h-5M4 17v-5h5M5.6 7a7 7 0 0 1 11.5-2L20 8M4 16l2.9 3A7 7 0 0 0 18.4 17",
        )
    }

    /** Cambiar de mesero. */
    val Users: ImageVector by lazy {
        icon(
            "users",
            circle(9f, 8f, 3.5f),
            "M2.5 20a6.5 6.5 0 0 1 13 0M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14.2a6.5 6.5 0 0 1 3.5 5.8",
        )
    }

    /** Computadora principal (servidor del restaurante). */
    val Server: ImageVector by lazy {
        icon(
            "server",
            roundedRect(4f, 3f, 16f, 7f, 2f),
            roundedRect(4f, 14f, 16f, 7f, 2f),
            "M8 6.5h.01M8 17.5h.01",
        )
    }

    /** Aviso. */
    val Alert: ImageVector by lazy {
        icon("alert", "M12 3.5 2.5 20h19L12 3.5Z", "M12 10v4.5M12 17h.01")
    }

    /** Cambiar el nombre. */
    val Edit: ImageVector by lazy { icon("edit", "M4 20h4L19 9l-4-4L4 16v4Z", "m13.5 6.5 4 4") }

    /** Mesas (navegación principal, etapa A2). */
    val Table: ImageVector by lazy {
        icon("table", "M3 9h18M5 9l-1 11M19 9l1 11M8 9v6h8V9", "M7 5h10")
    }

    /** Para llevar (navegación principal, etapa A2). */
    val Bag: ImageVector by lazy { icon("bag", "M5 8h14l-1 13H6L5 8Z", "M9 8V6a3 3 0 0 1 6 0v2") }

    fun icon(name: String, vararg paths: String): ImageVector {
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
    fun circle(cx: Float, cy: Float, r: Float): String =
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
