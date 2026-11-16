package local.adel.pos.android.core.domain.money

import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Formato de importes con la tabla de casos que comparten Go (backend/internal/moneytext) y el
 * escritorio (desktop/src/services/money.test.ts). Go imprime el negativo con el guion ASCII; la
 * pantalla usa el menos tipográfico U+2212.
 */
class MoneyFormatTest {
    private data class MoneyCase(val currency: Currency, val amount: String, val printed: String)

    private val cases: JsonObject by lazy {
        val path =
            System.getProperty("adelpos.moneyCases")
                ?: error("Falta la propiedad adelpos.moneyCases")
        Json.parseToJsonElement(File(path).readText()).jsonObject
    }

    private fun casesOf(group: String): List<MoneyCase> =
        cases.getValue(group).jsonArray.map { element ->
            val entry = element.jsonObject
            val currency =
                if (entry.getValue("currency").jsonPrimitive.content == "USD") {
                    Currency.USD
                } else {
                    Currency.MXN
                }
            MoneyCase(
                currency = currency,
                amount = entry.getValue("amount").jsonPrimitive.content,
                printed = entry.getValue("printed").jsonPrimitive.content,
            )
        }

    private fun onScreen(printed: String): String = printed.replaceFirst("-", "−")

    @Test
    fun formatoConLaTablaCompartida() {
        val format = casesOf("format")
        assertTrue("La tabla debe tener casos", format.size > 10)
        for (entry in format) {
            assertEquals(
                entry.toString(),
                onScreen(entry.printed),
                MoneyFormat.format(entry.amount, entry.currency),
            )
        }
    }

    @Test
    fun conSignoConLaTablaCompartida() {
        for (entry in casesOf("signed")) {
            assertEquals(
                entry.toString(),
                onScreen(entry.printed),
                MoneyFormat.formatSigned(entry.amount, entry.currency),
            )
        }
    }

    @Test
    fun denominacionConLaTablaCompartida() {
        for (entry in casesOf("denomination")) {
            assertEquals(
                entry.toString(),
                onScreen(entry.printed),
                MoneyFormat.formatDenomination(entry.amount, entry.currency),
            )
        }
    }

    @Test
    fun pesosPorOmisionYRayaSinValor() {
        assertEquals("$1,500.00", MoneyFormat.format("1500"))
        assertEquals("—", MoneyFormat.format(null))
        assertEquals("—", MoneyFormat.format(""))
        assertEquals("—", MoneyFormat.format("1e3"))
        assertEquals("—", MoneyFormat.format("+5"))
        assertEquals("—", MoneyFormat.format("1,000.00"))
        assertEquals("—", MoneyFormat.formatSigned(null))
        assertEquals("—", MoneyFormat.formatDenomination(null))
    }

    @Test
    fun deduccionAnteponeElMenos() {
        assertEquals("−$50.00", MoneyFormat.formatDeduction("50"))
        assertEquals("−$1,250.50", MoneyFormat.formatDeduction("1250.5"))
        assertEquals("$0.00", MoneyFormat.formatDeduction("0.00"))
        assertEquals("+$15.00", MoneyFormat.formatDeduction("-15"))
        assertEquals("−US$12.00", MoneyFormat.formatDeduction("12", Currency.USD))
        assertEquals("—", MoneyFormat.formatDeduction(""))
    }

    @Test
    fun nuncaPasaPorUnNumero() {
        // 9007199254740993 no cabe exacto en un Double (se volvería …992).
        assertEquals("$9,007,199,254,740,993.01", MoneyFormat.format("9007199254740993.01"))
        assertEquals("$0.30000000000000004", MoneyFormat.format("0.30000000000000004"))
        assertEquals(
            "$123,456,789,012,345,678,901,234,567,890.12",
            MoneyFormat.format("123456789012345678901234567890.12"),
        )
    }

    @Test
    fun elImporteSoloAceptaCadenasDecimales() {
        assertEquals("130.00", Money("130.00").decimal)
        assertEquals("$130.00", Money("130.00").display())
        assertEquals("−US$7.00", Money("-7").display(Currency.USD))
        assertTrue(MoneyFormat.isDecimal("-0.5"))
        assertFalse(MoneyFormat.isDecimal("130,00"))
        assertFalse(MoneyFormat.isDecimal(".5"))
        assertThrows(IllegalArgumentException::class.java) { Money("1.3e2") }
        assertThrows(IllegalArgumentException::class.java) { Money("") }
    }
}
