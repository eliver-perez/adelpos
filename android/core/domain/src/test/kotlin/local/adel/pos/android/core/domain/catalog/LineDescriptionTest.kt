package local.adel.pos.android.core.domain.catalog

import org.junit.Assert.assertEquals
import org.junit.Test

class LineDescriptionTest {
    private val catalog = CatalogFixtures.catalog
    private val original = Customization.ORIGINAL

    @Test
    fun soloDescribeLoQueCambio() {
        assertEquals(
            emptyList<String>(),
            LineDescription.parts(
                catalog,
                LineSelection(CatalogFixtures.BURGER, null, 1, original),
            ),
        )
        val sideBinding = CatalogFixtures.burger.groups[0]
        var value = IngredientChoices.increase(original, CatalogFixtures.bacon)
        value =
            GroupChoices.chooseSingle(
                value,
                sideBinding,
                CatalogFixtures.sideGroup,
                CatalogFixtures.sideGroup.options[1],
            )
        val options =
            QuickNoteChoices.applicable(
                catalog.quickNotesFor(CatalogFixtures.burger),
                CatalogFixtures.burger.ingredients,
            )
        value = QuickNoteChoices.toggle(value, options.first { it.text == "Sin cebolla" }, options)
        value = KitchenNotes.withObservation(value, "salsa aparte")!!
        // «Sin cebolla» aparece una sola vez (la indicación ya lo dice).
        assertEquals(
            listOf("Extra tocino", "Ensalada", "Sin cebolla, salsa aparte"),
            LineDescription.parts(catalog, LineSelection(CatalogFixtures.BURGER, null, 1, value)),
        )
    }

    @Test
    fun presentacionYComponentesDelCombo() {
        val drink = CatalogFixtures.combo.slots[0]
        val fries = CatalogFixtures.combo.slots[1]
        var value = ComboChoices.chooseProduct(original, drink, CatalogFixtures.WATER)
        value = ComboChoices.withQuantity(value, fries, 0)
        assertEquals(
            listOf("Agua de jamaica", "Sin papas del combo"),
            LineDescription.parts(catalog, LineSelection(CatalogFixtures.COMBO, null, 1, value)),
        )
        assertEquals(
            listOf("Grande"),
            LineDescription.parts(
                catalog,
                LineSelection(CatalogFixtures.SODA, CatalogFixtures.SODA_LARGE, 1, original),
            ),
        )
    }
}
