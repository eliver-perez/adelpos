package local.adel.pos.android.core.domain.catalog

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CatalogSearchTest {
    @Test
    fun normalizaSinAcentosNiMayusculas() {
        assertEquals("cafe de olla", CatalogSearch.normalize("  Café   DE Olla "))
        assertEquals("jamon serrano", CatalogSearch.normalize("JAMÓN serrano"))
        assertEquals("pina colada", CatalogSearch.normalize("Piña colada"))
        assertEquals("pinguino", CatalogSearch.normalize("Pingüino"))
    }

    @Test
    fun cadaPalabraDebeAparecerEnCualquierOrden() {
        val key = CatalogSearch.searchKey("Hamburguesa clásica", "HAM-01", "7501234567890")
        assertTrue(CatalogSearch.matches(key, CatalogSearch.tokens("clasica hambur")))
        assertTrue(CatalogSearch.matches(key, CatalogSearch.tokens("ham-01")))
        assertTrue(CatalogSearch.matches(key, CatalogSearch.tokens("7501234")))
        assertFalse(CatalogSearch.matches(key, CatalogSearch.tokens("clasica pollo")))
    }

    @Test
    fun sinTextoNoHayBusqueda() {
        assertEquals(emptyList<String>(), CatalogSearch.tokens("   "))
        assertFalse(CatalogQuery("  ", CatalogFixtures.BURGERS).searching)
        assertTrue(CatalogQuery("ref").searching)
    }

    @Test
    fun elCatalogoOrdenaPorCategoriaYOcultaInactivos() {
        val catalog = CatalogFixtures.catalog
        assertEquals(listOf("Hamburguesas", "Bebidas"), catalog.categories.map { it.name })
        assertEquals(
            listOf(
                "Combo hamburguesa",
                "Hamburguesa clásica",
                "Papas a la francesa",
                "Agua de jamaica",
                "Malteada de fresa",
                "Refresco",
            ),
            catalog.products.map { it.name },
        )
        assertEquals(
            listOf(
                "Sin cebolla",
                "Extra cebolla",
                "Con cebolla",
                "Con tocino",
                "Bien cocida",
                "Sin pepinillos",
            ),
            catalog.quickNotesFor(CatalogFixtures.burger).map { it.text },
        )
        assertTrue(catalog.quickNotesFor(CatalogFixtures.soda).isEmpty())
        assertTrue(Catalog.EMPTY.isEmpty)
    }

    @Test
    fun laMarcaDeDisponibilidadSaleDeLoQueDijoGo() {
        assertEquals(
            AvailabilityStatus.SOLD_OUT_INDEFINITE,
            CatalogFixtures.soldOutShake.availability.status,
        )
        assertFalse(CatalogFixtures.soldOutShake.selectable)
        val soldOutToday =
            CatalogFixtures.soldOutShake.availability.copy(
                soldOutIndefinite = false,
                soldOutUntil = "2026-11-17T11:00:00Z",
            )
        assertEquals(AvailabilityStatus.SOLD_OUT, soldOutToday.status)
        val paused = ProductAvailability.AVAILABLE.copy(availableNow = false, reason = "Pausado.")
        assertEquals(AvailabilityStatus.UNAVAILABLE, paused.status)
        assertTrue(CatalogFixtures.soda.selectable)
        assertTrue(CatalogFixtures.soda.requiresVariant)
        assertFalse(CatalogFixtures.burger.requiresVariant)
    }
}
