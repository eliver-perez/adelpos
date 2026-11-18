package com.adelpos.meseros.core.data.api

import com.adelpos.meseros.core.domain.money.Money
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

/**
 * El dinero viaja siempre como texto decimal: se lee y se escribe tal cual, y un número JSON se
 * rechaza.
 */
class MoneySerializerTest {
    @Serializable
    private data class Line(
        val name: String,
        @Serializable(with = MoneySerializer::class) val price: Money,
    )

    @Test
    fun leeYEscribeElImporteComoCadena() {
        val line =
            ApiJson.decodeFromString(
                Line.serializer(),
                """{"name":"Hamburguesa","price":"130.00"}""",
            )
        assertEquals(Money("130.00"), line.price)
        assertEquals("$130.00", line.price.display())
        assertEquals(
            """{"name":"Hamburguesa","price":"130.00"}""",
            ApiJson.encodeToString(Line.serializer(), line),
        )
    }

    @Test
    fun conservaTodasLasCifrasSinPasarPorUnDouble() {
        val line =
            ApiJson.decodeFromString(
                Line.serializer(),
                """{"name":"x","price":"9007199254740993.01"}""",
            )
        assertEquals("9007199254740993.01", line.price.decimal)
    }

    @Test
    fun rechazaUnNumeroJson() {
        assertThrows(SerializationException::class.java) {
            ApiJson.decodeFromString(Line.serializer(), """{"name":"x","price":130.0}""")
        }
        assertThrows(SerializationException::class.java) {
            ApiJson.decodeFromString(Line.serializer(), """{"name":"x","price":130}""")
        }
    }

    @Test
    fun rechazaTextoQueNoEsDecimal() {
        assertThrows(SerializationException::class.java) {
            ApiJson.decodeFromString(Line.serializer(), """{"name":"x","price":"$130"}""")
        }
        assertThrows(SerializationException::class.java) {
            ApiJson.decodeFromString(Line.serializer(), """{"name":"x","price":"1.3e2"}""")
        }
    }
}
