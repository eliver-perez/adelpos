package local.adel.pos.android.core.domain.money

/** Moneda de un importe en pantalla: pesos con «$» y dólares con «US$», como el escritorio. */
enum class Currency(val symbol: String) {
    MXN("$"),
    USD("US$"),
}

/**
 * Formato único de importes en pantalla, el mismo del escritorio (desktop/src/services/money.ts) y
 * de Go (backend/internal/moneytext), con la misma tabla de casos en las pruebas de los tres. Opera
 * solo sobre el texto: nunca convierte el importe a número, así que una cadena con más cifras de
 * las que cabe en un Double se muestra igual, sin redondeo.
 * - separador de miles «,» y punto decimal;
 * - al menos dos decimales; si Go entrega más, se conservan sin redondear (solo se quitan los ceros
 *   finales que sobran después del segundo decimal);
 * - «$» para pesos y «US$» para dólares;
 * - el signo negativo va antes del símbolo («−$1,234.56») con el menos tipográfico «−» (U+2212); la
 *   impresión de Go usa el guion ASCII;
 * - un valor vacío o que no es un decimal válido se muestra como «—».
 */
object MoneyFormat {
    const val EMPTY = "—"
    private const val SCREEN_MINUS = "−"

    // Rechaza signos «+», espacios, separadores de miles, notación científica y puntos sin dígitos
    // a ambos lados.
    private val decimalPattern = Regex("^(-?)(\\d+)(?:\\.(\\d+))?$")
    private val leadingZeros = Regex("^0+(?=\\d)")
    private val onlyZeros = Regex("^0+$")
    private val thousands = Regex("\\B(?=(\\d{3})+(?!\\d))")

    private data class ParsedAmount(
        val negative: Boolean,
        val integer: String,
        val fraction: String,
    )

    /** Si el texto es una cadena decimal como las que entrega Go. */
    fun isDecimal(text: String): Boolean = decimalPattern.matches(text)

    /** Importe con símbolo, miles y dos decimales: «$1,234.56», «−US$20.00», «—». */
    fun format(value: String?, currency: Currency = Currency.MXN): String {
        val amount = parse(value) ?: return EMPTY
        val sign = if (amount.negative) SCREEN_MINUS else ""
        return compose(amount, currency, amount.fraction, sign)
    }

    /** Importe con signo explícito para ajustes y diferencias: «+$50.00», «−$50.00» y «$0.00». */
    fun formatSigned(value: String?, currency: Currency = Currency.MXN): String {
        val amount = parse(value) ?: return EMPTY
        val sign =
            when {
                amount.negative -> SCREEN_MINUS
                isZero(amount.integer, amount.fraction) -> ""
                else -> "+"
            }
        return compose(amount, currency, amount.fraction, sign)
    }

    /**
     * Importe que se resta (descuentos, cortesías) cuando Go lo entrega en positivo: «−$50.00».
     * Invertir el signo es solo presentación sobre el texto, sin aritmética; el cero se muestra sin
     * signo.
     */
    fun formatDeduction(value: String?, currency: Currency = Currency.MXN): String {
        val amount = parse(value) ?: return EMPTY
        val sign =
            when {
                isZero(amount.integer, amount.fraction) -> ""
                amount.negative -> "+"
                else -> SCREEN_MINUS
            }
        return compose(amount, currency, amount.fraction, sign)
    }

    /**
     * Valor de un billete o una moneda: sin decimales si es entero («$1,000») y con dos si no
     * («$0.50»).
     */
    fun formatDenomination(value: String?, currency: Currency = Currency.MXN): String {
        val amount = parse(value) ?: return EMPTY
        val fraction = if (onlyZeros.matches(amount.fraction)) "" else amount.fraction
        val sign = if (amount.negative) SCREEN_MINUS else ""
        return compose(amount, currency, fraction, sign)
    }

    // Descompone el texto decimal sin interpretarlo como número.
    private fun parse(value: String?): ParsedAmount? {
        if (value == null) {
            return null
        }
        val match = decimalPattern.matchEntire(value) ?: return null
        val integer = leadingZeros.replace(match.groupValues[2], "")
        val fraction = normalizeFraction(match.groupValues[3])
        val negative = match.groupValues[1] == "-" && !isZero(integer, fraction)
        return ParsedAmount(negative, integer, fraction)
    }

    // Completa a dos decimales; si hay más, solo quita los ceros finales a partir del tercero.
    private fun normalizeFraction(fraction: String): String {
        if (fraction.length < 2) {
            return fraction.padEnd(2, '0')
        }
        var end = fraction.length
        while (end > 2 && fraction[end - 1] == '0') {
            end -= 1
        }
        return fraction.substring(0, end)
    }

    private fun isZero(integer: String, fraction: String): Boolean =
        onlyZeros.matches(integer) && onlyZeros.matches(fraction)

    private fun compose(
        amount: ParsedAmount,
        currency: Currency,
        fraction: String,
        sign: String,
    ): String {
        val decimals = if (fraction.isEmpty()) "" else ".$fraction"
        val grouped = thousands.replace(amount.integer, ",")
        return "$sign${currency.symbol}$grouped$decimals"
    }
}
