package local.adel.pos.android.core.domain.catalog

/**
 * Cantidades de porciones e ingredientes (nunca dinero). Go las transporta como cadenas decimales
 * con hasta tres decimales («1», «1.5», «0.25»); la captura las convierte a milésimas enteras para
 * comparar, sumar porciones completas y decidir qué botón está elegido sin usar punto flotante
 * (puerto de `desktop/src/pages/restaurant/capture/quantities.ts`). Los importes nunca pasan por
 * aquí.
 */
object PortionQuantity {
    /** Una porción completa en milésimas. */
    const val UNIT: Int = 1000

    private val pattern = Regex("^(\\d{1,7})(?:\\.(\\d{1,3}))?$")

    /**
     * «1.500» → 1500. Un valor no reconocido (o nulo) cuenta como cero: Go rechazaría esa
     * selección.
     */
    fun toThousandths(text: String?): Int {
        val match = pattern.matchEntire(text?.trim().orEmpty()) ?: return 0
        val whole = match.groupValues[1].toInt()
        val fraction = match.groupValues[2].padEnd(3, '0').toInt()
        return whole * UNIT + fraction
    }

    /** 1500 → «1.5»; 2000 → «2»; la forma más corta que Go acepta. Los negativos cuentan como 0. */
    fun fromThousandths(value: Int): String {
        val safe = value.coerceAtLeast(0)
        val whole = safe / UNIT
        val fraction = safe % UNIT
        if (fraction == 0) {
            return whole.toString()
        }
        val digits = fraction.toString().padStart(3, '0').trimEnd('0')
        return "$whole.$digits"
    }

    /** Texto de una cantidad para mostrarla: «1», «0.5». */
    fun display(text: String?): String = fromThousandths(toThousandths(text))
}
