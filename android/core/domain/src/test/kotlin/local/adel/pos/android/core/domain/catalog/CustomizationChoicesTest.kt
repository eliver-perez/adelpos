package local.adel.pos.android.core.domain.catalog

import local.adel.pos.android.core.domain.catalog.CatalogFixtures.BACON
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.NOTE_EXTRA_ONION
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.NOTE_NO_ONION
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.NOTE_WITH_BACON
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.ONION
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SAUCE_CHIPOTLE
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SAUCE_GROUP
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SAUCE_KETCHUP
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SIDE_FRIES
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SIDE_GROUP
import local.adel.pos.android.core.domain.catalog.CatalogFixtures.SIDE_SALAD
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** Reglas del personalizador: la selección que se enviará a Go en `customization`. */
class CustomizationChoicesTest {
    private val original = Customization.ORIGINAL
    private val sideBinding = CatalogFixtures.burger.groups[0]
    private val sauceBinding = CatalogFixtures.burger.groups[1]

    @Test
    fun cantidadesDePorcionesEnMilesimas() {
        assertEquals(1500, PortionQuantity.toThousandths("1.5"))
        assertEquals(250, PortionQuantity.toThousandths("0.250"))
        assertEquals(0, PortionQuantity.toThousandths("abc"))
        assertEquals(0, PortionQuantity.toThousandths(null))
        assertEquals("1.5", PortionQuantity.fromThousandths(1500))
        assertEquals("2", PortionQuantity.fromThousandths(2000))
        assertEquals("0.25", PortionQuantity.fromThousandths(250))
        assertEquals("0", PortionQuantity.fromThousandths(-5))
        assertEquals("1", PortionQuantity.display("1.000"))
    }

    @Test
    fun ingredientesSinNormalYExtraConMenosYMas() {
        assertEquals(
            listOf(IngredientLevel.WITHOUT, IngredientLevel.NORMAL, IngredientLevel.EXTRA),
            IngredientChoices.levels(CatalogFixtures.onion),
        )
        // Tocino: no lo incluye (no hay «Sin») y es agregable hasta 2; queso: ni quitar ni
        // agregar, así que no se ofrece.
        assertEquals(
            listOf(IngredientLevel.NORMAL, IngredientLevel.EXTRA),
            IngredientChoices.levels(CatalogFixtures.bacon),
        )
        assertEquals(
            listOf(CatalogFixtures.onion, CatalogFixtures.bacon),
            IngredientChoices.editable(CatalogFixtures.burger),
        )

        val without = IngredientChoices.decrease(original, CatalogFixtures.onion)
        assertEquals(listOf(IngredientSelection(ONION, "0")), without.ingredients)
        assertEquals(
            IngredientLevel.WITHOUT,
            IngredientChoices.level(without, CatalogFixtures.onion),
        )
        assertFalse(IngredientChoices.canDecrease(without, CatalogFixtures.onion))
        assertSame(without, IngredientChoices.decrease(without, CatalogFixtures.onion))

        // «+» desde «Sin» vuelve a «Normal»: sin cantidad explícita (configuración original).
        val normal = IngredientChoices.increase(without, CatalogFixtures.onion)
        assertEquals(original, normal)

        val extra = IngredientChoices.increase(normal, CatalogFixtures.onion)
        assertEquals(listOf(IngredientSelection(ONION, "2")), extra.ingredients)
        assertEquals(IngredientLevel.EXTRA, IngredientChoices.level(extra, CatalogFixtures.onion))
        assertFalse(IngredientChoices.canIncrease(extra, CatalogFixtures.onion))

        val bacon = IngredientChoices.increase(original, CatalogFixtures.bacon)
        assertEquals(listOf(IngredientSelection(BACON, "1")), bacon.ingredients)
    }

    @Test
    fun grupoDeEleccionUnicaSustituyeConUnToque() {
        val group = CatalogFixtures.sideGroup
        val limits = GroupChoices.limits(sideBinding, group)
        assertTrue(limits.singleChoice)
        assertTrue(limits.required)
        // Sin tocar nada vale lo incluido de origen (papas).
        assertEquals(
            listOf(OptionQuantity(SIDE_FRIES, "1")),
            GroupChoices.selection(original, sideBinding),
        )

        val salad = GroupChoices.chooseSingle(original, sideBinding, group, group.options[1])
        assertEquals(
            listOf(GroupSelection(SIDE_GROUP, listOf(OptionQuantity(SIDE_SALAD, "1")))),
            salad.groups,
        )
        // Tocar la elegida en un grupo obligatorio no lo deja vacío.
        assertSame(salad, GroupChoices.chooseSingle(salad, sideBinding, group, group.options[1]))
        // Una opción agotada no se elige.
        assertSame(salad, GroupChoices.chooseSingle(salad, sideBinding, group, group.options[2]))
    }

    @Test
    fun grupoDeVariasPorcionesConMenosYMasYAvisos() {
        val group = CatalogFixtures.sauceGroup
        val ketchup = group.options[0]
        val mustard = group.options[1]
        val chipotle = group.options[2]
        assertEquals("Chipotle · Doble", chipotle.displayName)

        var value =
            (GroupChoices.addPortion(original, sauceBinding, group, ketchup)
                    as PortionChange.Applied)
                .value
        value =
            (GroupChoices.addPortion(value, sauceBinding, group, ketchup) as PortionChange.Applied)
                .value
        assertEquals(
            PortionChange.OptionFull("2"),
            GroupChoices.addPortion(value, sauceBinding, group, ketchup),
        )
        value =
            (GroupChoices.addPortion(value, sauceBinding, group, chipotle) as PortionChange.Applied)
                .value
        assertEquals(
            PortionChange.GroupFull("3"),
            GroupChoices.addPortion(value, sauceBinding, group, mustard),
        )
        assertEquals(
            listOf(
                GroupSelection(
                    SAUCE_GROUP,
                    listOf(OptionQuantity(SAUCE_KETCHUP, "2"), OptionQuantity(SAUCE_CHIPOTLE, "1")),
                ),
            ),
            value.groups,
        )
        assertEquals(3000, GroupChoices.total(value, sauceBinding))

        value = GroupChoices.removePortion(value, sauceBinding, SAUCE_KETCHUP)
        value = GroupChoices.removePortion(value, sauceBinding, SAUCE_CHIPOTLE)
        assertEquals(
            listOf(GroupSelection(SAUCE_GROUP, listOf(OptionQuantity(SAUCE_KETCHUP, "1")))),
            value.groups,
        )
        // «Sin salsas»: selección explícita vacía.
        assertEquals(
            listOf(GroupSelection(SAUCE_GROUP, emptyList())),
            GroupChoices.clear(value, SAUCE_GROUP).groups,
        )
    }

    @Test
    fun combosConProductoCantidadYPersonalizacionPropia() {
        val drink = CatalogFixtures.combo.slots[0]
        val fries = CatalogFixtures.combo.slots[1]
        assertEquals(CatalogFixtures.SODA, ComboChoices.selection(original, drink).productId)
        assertFalse(ComboChoices.quantityAdjustable(drink))
        assertTrue(ComboChoices.quantityAdjustable(fries))

        var value = ComboChoices.chooseProduct(original, drink, CatalogFixtures.WATER)
        value = ComboChoices.withQuantity(value, fries, 2)
        assertSame(value, ComboChoices.withQuantity(value, fries, 3))
        val burgerLike = IngredientChoices.decrease(original, CatalogFixtures.onion)
        value = ComboChoices.withCustomization(value, fries, burgerLike)
        assertEquals(
            listOf(
                ComponentSelection(drink.id, CatalogFixtures.WATER, null, 1, original),
                ComponentSelection(fries.id, CatalogFixtures.FRIES, null, 2, burgerLike),
            ),
            value.components,
        )
        // Volver al refresco empieza con su configuración original y conserva el lugar.
        val back = ComboChoices.chooseProduct(value, drink, CatalogFixtures.SODA)
        assertEquals(
            ComponentSelection(drink.id, CatalogFixtures.SODA, null, 1, original),
            back.components[0],
        )
        // Un producto que no es de la ranura se ignora.
        assertSame(back, ComboChoices.chooseProduct(back, drink, CatalogFixtures.BURGER))
        val sized = ComboChoices.withVariant(back, drink, CatalogFixtures.SODA_LARGE)
        assertEquals(CatalogFixtures.SODA_LARGE, sized.components[0].variantId)
    }

    @Test
    fun indicacionesRapidasConEfectoAplicanElIngrediente() {
        val notes = CatalogFixtures.catalog.quickNotesFor(CatalogFixtures.burger)
        val options = QuickNoteChoices.applicable(notes, CatalogFixtures.burger.ingredients)
        // «Sin pepinillos» no aplica: la hamburguesa no lleva pepinillos.
        assertEquals(
            listOf("Sin cebolla", "Extra cebolla", "Con cebolla", "Con tocino", "Bien cocida"),
            options.map { it.text },
        )
        val noOnion = options.first { it.id == NOTE_NO_ONION }
        val extraOnion = options.first { it.id == NOTE_EXTRA_ONION }
        val normalOnion = options.first { it.text == "Con cebolla" }
        val withBacon = options.first { it.id == NOTE_WITH_BACON }
        val wellDone = options.first { it.text == "Bien cocida" }
        assertEquals("0", noOnion.quantity)
        assertEquals("2", extraOnion.quantity)
        assertTrue(normalOnion.original)
        assertEquals("1", withBacon.quantity)

        var value = QuickNoteChoices.toggle(original, noOnion, options)
        assertEquals(listOf(IngredientSelection(ONION, "0")), value.ingredients)
        assertEquals("Sin cebolla", value.notes)
        assertEquals(listOf(QuickNoteSelection(NOTE_NO_ONION, "Sin cebolla")), value.quickNotes)

        // Excluyentes sobre el mismo ingrediente: «Extra cebolla» quita «Sin cebolla».
        value = QuickNoteChoices.toggle(value, extraOnion, options)
        assertEquals(listOf(IngredientSelection(ONION, "2")), value.ingredients)
        assertEquals("Extra cebolla", value.notes)
        assertEquals(
            listOf(QuickNoteSelection(NOTE_EXTRA_ONION, "Extra cebolla")),
            value.quickNotes,
        )

        // Una solo de instrucción agrega su texto a las observaciones (y su identificador, como el
        // escritorio); no toca ingredientes.
        value = QuickNoteChoices.toggle(value, wellDone, options)
        assertEquals("Extra cebolla, Bien cocida", value.notes)
        assertTrue(QuickNoteChoices.isSelected(value, wellDone))

        // Cambiar la cebolla en el personalizador quita la indicación que ya no la describe.
        val changed = IngredientChoices.decrease(value, CatalogFixtures.onion)
        val reconciled =
            QuickNoteChoices.reconcile(changed, options, CatalogFixtures.burger.ingredients)
        assertEquals("Bien cocida", reconciled.notes)
        assertFalse(QuickNoteChoices.isSelected(reconciled, extraOnion))
        assertTrue(reconciled.ingredients.isEmpty())

        // Desmarcar devuelve el ingrediente a su configuración original.
        val unmarked = QuickNoteChoices.toggle(value, extraOnion, options)
        assertTrue(unmarked.ingredients.isEmpty())
        assertEquals("Bien cocida", unmarked.notes)

        // «Con cebolla» en un producto que ya la lleva no fija cantidad explícita.
        val withNormal = QuickNoteChoices.toggle(original, normalOnion, options)
        assertTrue(withNormal.ingredients.isEmpty())
        assertEquals(
            listOf(QuickNoteSelection(normalOnion.id, "Con cebolla")),
            withNormal.quickNotes,
        )
    }

    @Test
    fun observacionesLibresConElLimiteDeGoEnBytes() {
        val value = KitchenNotes.withObservation(original, "  salsa aparte ")!!
        assertEquals("salsa aparte", value.notes)
        assertSame(value, KitchenNotes.withObservation(value, "salsa aparte"))
        assertNull(KitchenNotes.withObservation(value, "   "))
        val both = KitchenNotes.withObservation(value, "sin sal")!!
        assertEquals(listOf("salsa aparte", "sin sal"), KitchenNotes.split(both.notes))
        assertEquals("sin sal", KitchenNotes.withoutText(both, "salsa aparte").notes)

        // 250 letras con acento son 500 bytes: caben; una más ya no.
        val accented = "á".repeat(250)
        assertTrue(KitchenNotes.fits(accented))
        assertFalse(KitchenNotes.fits(accented + "a"))
        assertNull(
            KitchenNotes.withObservation(KitchenNotes.withObservation(original, accented)!!, "b"),
        )
    }
}
