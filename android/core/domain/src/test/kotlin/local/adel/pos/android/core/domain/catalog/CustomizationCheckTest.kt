package local.adel.pos.android.core.domain.catalog

import local.adel.pos.android.core.domain.catalog.CustomizationProblem.ComponentOutOfRange
import local.adel.pos.android.core.domain.catalog.CustomizationProblem.GroupBelowMinimum
import local.adel.pos.android.core.domain.catalog.CustomizationProblem.NotesTooLong
import local.adel.pos.android.core.domain.catalog.CustomizationProblem.OptionNotSelectable
import local.adel.pos.android.core.domain.catalog.CustomizationProblem.ProductNotSelectable
import local.adel.pos.android.core.domain.catalog.CustomizationProblem.VariantRequired
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Lo que el dispositivo revisa antes de agregar una partida: solo lo que declara el catálogo. */
class CustomizationCheckTest {
    private val catalog = CatalogFixtures.catalog
    private val original = Customization.ORIGINAL

    private fun line(
        productId: String,
        customization: Customization = original,
        variant: String? = null,
    ) = LineSelection(productId, variant, 1, customization)

    @Test
    fun laConfiguracionOriginalEsValida() {
        assertTrue(CustomizationCheck.problems(catalog, line(CatalogFixtures.BURGER)).isEmpty())
        // El refresco del combo no tiene precio base: pide presentación dentro de su ranura.
        assertEquals(
            listOf(VariantRequired("Refresco", "Bebida del combo")),
            CustomizationCheck.problems(catalog, line(CatalogFixtures.COMBO)),
        )
    }

    @Test
    fun presentacionObligatoriaSinPrecioBase() {
        assertEquals(
            listOf(VariantRequired("Refresco", null)),
            CustomizationCheck.problems(catalog, line(CatalogFixtures.SODA)),
        )
        assertTrue(
            CustomizationCheck.problems(
                    catalog,
                    line(CatalogFixtures.SODA, variant = CatalogFixtures.SODA_SMALL),
                )
                .isEmpty(),
        )
    }

    @Test
    fun grupoObligatorioVacioYOpcionAgotada() {
        val empty = GroupChoices.clear(original, CatalogFixtures.SIDE_GROUP)
        assertEquals(
            listOf(GroupBelowMinimum("Guarnición", "1", null)),
            CustomizationCheck.problems(catalog, line(CatalogFixtures.BURGER, empty)),
        )
        val rings =
            GroupChoices.withSelection(
                original,
                CatalogFixtures.SIDE_GROUP,
                listOf(OptionQuantity(CatalogFixtures.SIDE_RINGS, "1")),
            )
        assertEquals(
            listOf(OptionNotSelectable("Aros de cebolla", null)),
            CustomizationCheck.problems(catalog, line(CatalogFixtures.BURGER, rings)),
        )
    }

    @Test
    fun productoAgotadoNoSePuedeAgregar() {
        val problems = CustomizationCheck.problems(catalog, line(CatalogFixtures.SOLD_OUT_SHAKE))
        assertEquals(
            listOf(
                ProductNotSelectable(
                    "Malteada de fresa",
                    AvailabilityStatus.SOLD_OUT_INDEFINITE,
                    "Agotado hasta nuevo aviso.",
                    null,
                ),
            ),
            problems,
        )
    }

    @Test
    fun ranurasDelComboYObservaciones() {
        val drink = CatalogFixtures.combo.slots[0]
        val fries = CatalogFixtures.combo.slots[1]
        var value = ComboChoices.withVariant(original, drink, CatalogFixtures.SODA_LARGE)
        assertTrue(
            CustomizationCheck.problems(catalog, line(CatalogFixtures.COMBO, value)).isEmpty(),
        )
        // Una cantidad fuera de límites (llegada de otra versión) se informa con su ranura.
        value =
            ComboChoices.withComponent(
                value,
                ComboChoices.selection(value, fries).copy(quantity = 5),
            )
        value = value.copy(notes = "x".repeat(KitchenNotes.MAXIMUM_BYTES + 1))
        assertEquals(
            listOf(
                NotesTooLong(null),
                ComponentOutOfRange("Papas del combo", 0, 2, "Papas del combo"),
            ),
            CustomizationCheck.problems(catalog, line(CatalogFixtures.COMBO, value)),
        )
    }
}
