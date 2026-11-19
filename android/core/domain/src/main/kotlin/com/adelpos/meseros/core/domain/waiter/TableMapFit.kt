package com.adelpos.meseros.core.domain.waiter

import kotlin.math.ceil

/**
 * Escala automática del mapa de Mesas (etapa A3, §4.7), la misma regla del escritorio (`fitTables`
 * de `desktop/src/components/restaurant/TableMap.tsx`, etapa 9D, §3.2): una sola escala (dp por
 * unidad del lienzo lógico de Go) para ver **todas las mesas del área** en el espacio disponible,
 * calculada sobre el rectángulo que ocupan las mesas (no sobre el lienzo completo de 1000 × 560:
 * así el mapa llena el espacio en la tableta vertical y en el teléfono horizontal). Ninguna mesa
 * queda con su lado menor por debajo de [MINIMUM_TABLE_SIDE]: si con ese tamaño no caben, el mapa
 * se desplaza. La escala no pasa de [MAXIMUM_SCALE], para que pocas mesas no se vuelvan enormes.
 * Solo presentación: las posiciones y medidas de Go no cambian.
 */
object TableMapFit {
    /** Lado menor mínimo de una mesa (dp; en el escritorio, 64 px). Más que los 48 dp táctiles. */
    const val MINIMUM_TABLE_SIDE = 64f

    const val MAXIMUM_SCALE = 2f

    /** Margen alrededor del conjunto de mesas (dp). */
    const val MARGIN = 16f

    // Medio dp de tolerancia: el redondeo del punto flotante no debe crear desplazamientos de nada.
    private const val TOLERANCE = 0.5f

    /**
     * La escala y el lienzo del área.
     *
     * @param originX esquina superior izquierda del conjunto de mesas, en unidades de Go.
     * @param offsetX margen antes de la primera mesa (centra el conjunto si sobra espacio).
     * @param width medidas del lienzo (al menos las del espacio visible).
     * @param overflow las mesas no caben con el tamaño mínimo: el mapa se desplaza.
     */
    data class Fit(
        val scale: Float,
        val originX: Int,
        val originY: Int,
        val offsetX: Float,
        val offsetY: Float,
        val width: Float,
        val height: Float,
        val overflow: Boolean,
    ) {
        /** Dónde se dibuja una mesa en el lienzo (dp). */
        fun left(table: DiningTable): Float = offsetX + (table.x - originX) * scale

        fun top(table: DiningTable): Float = offsetY + (table.y - originY) * scale
    }

    /** null si no hay mesas o el espacio todavía no se midió. */
    fun of(tables: List<DiningTable>, viewportWidth: Float, viewportHeight: Float): Fit? {
        if (tables.isEmpty() || viewportWidth <= 0f || viewportHeight <= 0f) {
            return null
        }
        val originX = tables.minOf { it.x }
        val originY = tables.minOf { it.y }
        val spanX = maxOf(1, tables.maxOf { it.x + it.width } - originX)
        val spanY = maxOf(1, tables.maxOf { it.y + it.height } - originY)
        val availableWidth = maxOf(1f, viewportWidth - MARGIN * 2)
        val availableHeight = maxOf(1f, viewportHeight - MARGIN * 2)
        val fit = minOf(availableWidth / spanX, availableHeight / spanY)
        val smallest = maxOf(1, tables.minOf { minOf(it.width, it.height) })
        val minimum = MINIMUM_TABLE_SIDE / smallest
        val scale = maxOf(minimum, minOf(fit, MAXIMUM_SCALE))
        val contentWidth = spanX * scale + MARGIN * 2
        val contentHeight = spanY * scale + MARGIN * 2
        val overflowX = contentWidth > viewportWidth + TOLERANCE
        val overflowY = contentHeight > viewportHeight + TOLERANCE
        return Fit(
            scale = scale,
            originX = originX,
            originY = originY,
            offsetX = MARGIN + maxOf(0f, (viewportWidth - contentWidth) / 2),
            offsetY = MARGIN + maxOf(0f, (viewportHeight - contentHeight) / 2),
            width = if (overflowX) ceil(contentWidth) else viewportWidth,
            height = if (overflowY) ceil(contentHeight) else viewportHeight,
            overflow = overflowX || overflowY,
        )
    }
}
