package local.adel.pos.android.core.domain.waiter

import local.adel.pos.android.core.domain.waiter.WaiterFixtures.table
import org.junit.Assert.assertEquals
import org.junit.Test

/** Nombres cortos del mapa de Mesas en el teléfono (observación F4 de la Fase F). */
class TableNamesTest {
    @Test
    fun seQuitanLasPalabrasQueCompartenTodasLasMesasDelArea() {
        val names =
            TableNames.compact(
                listOf(table("1", "Mesa F1"), table("12", "Mesa F12"), table("3", "Mesa  F3")),
            )
        assertEquals(mapOf("1" to "F1", "12" to "F12", "3" to "F3"), names)
    }

    @Test
    fun sinPalabrasComunesONombresDeUnaPalabraSeConservaElNombre() {
        assertEquals(
            mapOf("a" to "Mesa 1", "b" to "Barra 1"),
            TableNames.compact(listOf(table("a", "Mesa 1"), table("b", "Barra 1"))),
        )
        assertEquals(
            mapOf("a" to "Terraza", "b" to "Terraza 2"),
            TableNames.compact(listOf(table("a", "Terraza"), table("b", "Terraza 2"))),
        )
        assertEquals(emptyMap<String, String>(), TableNames.compact(emptyList()))
    }
}
