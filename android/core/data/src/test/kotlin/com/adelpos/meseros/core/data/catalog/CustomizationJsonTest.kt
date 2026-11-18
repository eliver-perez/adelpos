package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.domain.catalog.ComboChoices
import com.adelpos.meseros.core.domain.catalog.ComboSlot
import com.adelpos.meseros.core.domain.catalog.ComponentSelection
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.GroupBinding
import com.adelpos.meseros.core.domain.catalog.GroupChoices
import com.adelpos.meseros.core.domain.catalog.GroupOption
import com.adelpos.meseros.core.domain.catalog.GroupPricingMode
import com.adelpos.meseros.core.domain.catalog.GroupSelection
import com.adelpos.meseros.core.domain.catalog.IngredientChoices
import com.adelpos.meseros.core.domain.catalog.IngredientRule
import com.adelpos.meseros.core.domain.catalog.KitchenNotes
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.catalog.OptionGroup
import com.adelpos.meseros.core.domain.catalog.PortionChange
import com.adelpos.meseros.core.domain.catalog.QuickNoteChoices
import com.adelpos.meseros.core.domain.catalog.QuickNoteEffect
import com.adelpos.meseros.core.domain.catalog.QuickNoteOption
import com.adelpos.meseros.core.domain.catalog.QuickNoteSelection
import com.adelpos.meseros.core.domain.money.Money
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * El personalizador produce exactamente `customization` / `LineRequest` del contrato v1 (la forma
 * que guardan `saveDraft` y `submitOfflineOrder`), y lee un borrador que devuelve Go.
 */
class CustomizationJsonTest {
    private val onion =
        IngredientRule(
            ingredientId = "ing-cebolla",
            name = "Cebolla",
            includedQuantity = "1",
            removable = true,
            addable = true,
            maxQuantity = "2",
            extraPrice = Money("5.00"),
        )
    private val bacon =
        IngredientRule(
            ingredientId = "ing-tocino",
            name = "Tocino",
            includedQuantity = "0",
            removable = false,
            addable = true,
            maxQuantity = "2",
            extraPrice = Money("15.00"),
        )
    private val sauces =
        OptionGroup(
            id = "grp-salsas",
            name = "Salsas",
            description = "",
            active = true,
            pricingMode = GroupPricingMode.QUANTITY,
            minQuantity = "0",
            maxQuantity = "3",
            options =
                listOf(
                    GroupOption(
                        id = "opt-chipotle",
                        complementId = "cmp-chipotle",
                        name = "Chipotle",
                        portionId = null,
                        portionName = null,
                        portionFactor = null,
                        referenceValue = Money("0.00"),
                        extraPrice = Money("4.00"),
                        firstTierPrice = null,
                        maxQuantity = null,
                        active = true,
                        availableNow = true,
                        soldOut = false,
                    ),
                ),
        )
    private val saucesBinding = GroupBinding("grp-salsas", null, null, emptyList())
    private val drinkSlot =
        ComboSlot(
            id = "slot-bebida",
            name = "Bebida",
            minQuantity = 1,
            maxQuantity = 1,
            includedQuantity = 1,
            defaultQuantity = 1,
            defaultProductId = "prd-refresco",
            options = emptyList(),
        )

    private fun encode(line: LineSelection): String =
        CustomizationJson.encodeToString(LineRequestDto.serializer(), line.toLineRequest())

    @Test
    fun sinPersonalizarEsLaFormaOriginalDelEscritorio() {
        assertEquals(
            """{"product_id":"prd-hamburguesa","variant_id":null,"quantity":2,""" +
                """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}""",
            encode(LineSelection("prd-hamburguesa", null, 2, Customization.ORIGINAL)),
        )
    }

    @Test
    fun porcionesComplementosComboNotasYObservaciones() {
        var value = IngredientChoices.decrease(Customization.ORIGINAL, onion)
        value = IngredientChoices.increase(value, bacon)
        repeat(2) {
            val change = GroupChoices.addPortion(value, saucesBinding, sauces, sauces.options[0])
            value = (change as PortionChange.Applied).value
        }
        val wellDone =
            QuickNoteOption(
                id = "nota-cocida",
                text = "Bien cocida",
                effect = QuickNoteEffect.NONE,
                ingredientId = null,
                quantity = null,
                original = false,
            )
        value = QuickNoteChoices.toggle(value, wellDone, listOf(wellDone))
        value = KitchenNotes.withObservation(value, "salsa aparte")!!
        var drink = Customization.ORIGINAL
        drink = KitchenNotes.withObservation(drink, "sin hielo")!!
        value = ComboChoices.withVariant(value, drinkSlot, "var-grande")
        value = ComboChoices.withCustomization(value, drinkSlot, drink)

        assertEquals(
            "{" +
                """"product_id":"prd-combo","variant_id":null,"quantity":1,""" +
                """"customization":{""" +
                """"ingredients":[{"ingredient_id":"ing-cebolla","quantity":"0"},""" +
                """{"ingredient_id":"ing-tocino","quantity":"1"}],""" +
                """"groups":[{"group_id":"grp-salsas","options":[{"option_id":"opt-chipotle","quantity":"2"}]}],""" +
                """"components":[{"slot_id":"slot-bebida","product_id":"prd-refresco","variant_id":"var-grande",""" +
                """"quantity":1,"customization":{"ingredients":[],"groups":[],"components":[],"notes":"sin hielo"}}],""" +
                """"notes":"Bien cocida, salsa aparte",""" +
                """"quick_notes":[{"note_id":"nota-cocida","text":"Bien cocida"}]""" +
                "}}",
            encode(LineSelection("prd-combo", null, 1, value)),
        )
    }

    @Test
    fun grupoSinOpcionesEsUnaSeleccionExplicitaVacia() {
        val value = GroupChoices.clear(Customization.ORIGINAL, "grp-salsas")
        assertEquals(
            """{"ingredients":[],"groups":[{"group_id":"grp-salsas","options":[]}],"components":[],"notes":""}""",
            CustomizationJson.encodeToString(CustomizationDto.serializer(), value.toDto()),
        )
    }

    @Test
    fun leeUnBorradorDeGoConListasNulas() {
        val text =
            """{"ingredients":null,"groups":[{"group_id":"grp-salsas","options":null}],""" +
                """"components":[{"slot_id":"slot-bebida","product_id":"prd-agua","variant_id":null,"quantity":1,""" +
                """"customization":{"ingredients":[],"groups":[],"components":[],"notes":""}}],""" +
                """"notes":"","quick_notes":[{"note_id":"nota-cocida","text":"Bien cocida"}],"campo_nuevo":1}"""
        val value =
            CustomizationJson.decodeFromString(CustomizationDto.serializer(), text).toDomain()
        assertEquals(
            Customization(
                ingredients = emptyList(),
                groups = listOf(GroupSelection("grp-salsas", emptyList())),
                components =
                    listOf(
                        ComponentSelection(
                            "slot-bebida",
                            "prd-agua",
                            null,
                            1,
                            Customization.ORIGINAL,
                        ),
                    ),
                notes = "",
                quickNotes = listOf(QuickNoteSelection("nota-cocida", "Bien cocida")),
            ),
            value,
        )
        // Ida y vuelta: lo que se lee se vuelve a escribir igual.
        assertEquals(value, value.toDto().toDomain())
    }
}
