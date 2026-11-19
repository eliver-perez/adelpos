package com.adelpos.meseros.core.domain.money

/**
 * Importe tal como lo entrega el servidor: una cadena decimal ya calculada ("130.00", IVA
 * incluido).
 *
 * La aplicación nunca convierte un importe a número ni hace cuentas con él: Go es la única
 * autoridad en precios e impuestos (los precios del catálogo local son solo referencia). Un importe
 * se muestra con [MoneyFormat] y, cuando hace falta, se devuelve al servidor tal cual (por ejemplo,
 * el precio que vio el mesero en un pedido sin conexión).
 */
@JvmInline
value class Money(val decimal: String) {
    init {
        require(MoneyFormat.isDecimal(decimal)) { "El importe debe ser una cadena decimal" }
    }

    /** Importe para la pantalla: «$1,234.56». */
    fun display(currency: Currency = Currency.MXN): String = MoneyFormat.format(decimal, currency)

    override fun toString(): String = decimal
}
