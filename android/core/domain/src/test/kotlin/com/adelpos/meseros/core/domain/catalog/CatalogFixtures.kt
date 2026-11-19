package com.adelpos.meseros.core.domain.catalog

import com.adelpos.meseros.core.domain.money.Money

/**
 * Catálogo de ejemplo para las pruebas del personalizador: una hamburguesa con ingredientes,
 * guarnición de elección única, salsas de varias porciones e indicaciones rápidas con efecto, un
 * refresco con presentaciones obligatorias y un combo con dos ranuras.
 */
object CatalogFixtures {
    const val BURGERS = "c0000000-0000-4000-8000-000000000001"
    const val DRINKS = "c0000000-0000-4000-8000-000000000002"

    const val BURGER = "p0000000-0000-4000-8000-000000000001"
    const val SODA = "p0000000-0000-4000-8000-000000000002"
    const val WATER = "p0000000-0000-4000-8000-000000000003"
    const val FRIES = "p0000000-0000-4000-8000-000000000004"
    const val COMBO = "p0000000-0000-4000-8000-000000000005"
    const val SOLD_OUT_SHAKE = "p0000000-0000-4000-8000-000000000006"

    const val SODA_SMALL = "v0000000-0000-4000-8000-000000000001"
    const val SODA_LARGE = "v0000000-0000-4000-8000-000000000002"

    const val ONION = "i0000000-0000-4000-8000-000000000001"
    const val BACON = "i0000000-0000-4000-8000-000000000002"
    const val CHEESE = "i0000000-0000-4000-8000-000000000003"
    const val PICKLE = "i0000000-0000-4000-8000-000000000004"

    const val SIDE_GROUP = "g0000000-0000-4000-8000-000000000001"
    const val SAUCE_GROUP = "g0000000-0000-4000-8000-000000000002"
    const val SIDE_FRIES = "o0000000-0000-4000-8000-000000000001"
    const val SIDE_SALAD = "o0000000-0000-4000-8000-000000000002"
    const val SIDE_RINGS = "o0000000-0000-4000-8000-000000000003"
    const val SAUCE_KETCHUP = "o0000000-0000-4000-8000-000000000004"
    const val SAUCE_MUSTARD = "o0000000-0000-4000-8000-000000000005"
    const val SAUCE_CHIPOTLE = "o0000000-0000-4000-8000-000000000006"

    const val SLOT_DRINK = "s0000000-0000-4000-8000-000000000001"
    const val SLOT_FRIES = "s0000000-0000-4000-8000-000000000002"

    const val NOTE_NO_ONION = "n0000000-0000-4000-8000-000000000001"
    const val NOTE_EXTRA_ONION = "n0000000-0000-4000-8000-000000000002"
    const val NOTE_WITH_BACON = "n0000000-0000-4000-8000-000000000003"
    const val NOTE_WELL_DONE = "n0000000-0000-4000-8000-000000000004"
    const val NOTE_NO_PICKLE = "n0000000-0000-4000-8000-000000000005"
    const val NOTE_NORMAL_ONION = "n0000000-0000-4000-8000-000000000006"

    val onion =
        IngredientRule(
            ingredientId = ONION,
            name = "Cebolla",
            includedQuantity = "1",
            removable = true,
            addable = true,
            maxQuantity = "2",
            extraPrice = Money("5.00"),
        )
    val bacon =
        IngredientRule(
            ingredientId = BACON,
            name = "Tocino",
            includedQuantity = "0",
            removable = false,
            addable = true,
            maxQuantity = "2",
            extraPrice = Money("15.00"),
        )
    val cheese =
        IngredientRule(
            ingredientId = CHEESE,
            name = "Queso",
            includedQuantity = "1",
            removable = false,
            addable = false,
            maxQuantity = "1",
            extraPrice = Money("0.00"),
        )

    val sideGroup =
        OptionGroup(
            id = SIDE_GROUP,
            name = "Guarnición",
            description = "",
            active = true,
            pricingMode = GroupPricingMode.VALUE,
            minQuantity = "1",
            maxQuantity = "1",
            options =
                listOf(
                    option(SIDE_FRIES, "Papas"),
                    option(SIDE_SALAD, "Ensalada", extra = "10.00"),
                    option(SIDE_RINGS, "Aros de cebolla", soldOut = true),
                ),
        )

    val sauceGroup =
        OptionGroup(
            id = SAUCE_GROUP,
            name = "Salsas",
            description = "",
            active = true,
            pricingMode = GroupPricingMode.QUANTITY,
            minQuantity = "0",
            maxQuantity = "3",
            options =
                listOf(
                    option(SAUCE_KETCHUP, "Catsup", maxQuantity = "2"),
                    option(SAUCE_MUSTARD, "Mostaza"),
                    option(
                        SAUCE_CHIPOTLE,
                        "Chipotle",
                        portionId = "r0000000-0000-4000-8000-000000000001",
                        portionName = "Doble",
                    ),
                ),
        )

    val burger =
        product(
            id = BURGER,
            name = "Hamburguesa clásica",
            categoryId = BURGERS,
            price = "120.00",
            ingredients = listOf(onion, bacon, cheese),
            groups =
                listOf(
                    GroupBinding(
                        groupId = SIDE_GROUP,
                        minQuantity = null,
                        maxQuantity = null,
                        included = listOf(OptionQuantity(SIDE_FRIES, "1")),
                    ),
                    GroupBinding(
                        groupId = SAUCE_GROUP,
                        minQuantity = null,
                        maxQuantity = null,
                        included = emptyList(),
                    ),
                ),
        )

    val soda =
        product(
            id = SODA,
            name = "Refresco",
            categoryId = DRINKS,
            price = null,
            variants =
                listOf(
                    ProductVariant(SODA_SMALL, "Chico", "R-CH", true, Money("25.00"), null),
                    ProductVariant(SODA_LARGE, "Grande", "R-GR", true, Money("35.00"), null),
                ),
        )

    val water = product(id = WATER, name = "Agua de jamaica", categoryId = DRINKS, price = "30.00")

    val fries =
        product(id = FRIES, name = "Papas a la francesa", categoryId = BURGERS, price = "45.00")

    val combo =
        product(
            id = COMBO,
            name = "Combo hamburguesa",
            categoryId = BURGERS,
            price = "160.00",
            kind = ProductKind.COMBO,
            slots =
                listOf(
                    ComboSlot(
                        id = SLOT_DRINK,
                        name = "Bebida del combo",
                        minQuantity = 1,
                        maxQuantity = 1,
                        includedQuantity = 1,
                        defaultQuantity = 1,
                        defaultProductId = SODA,
                        options =
                            listOf(
                                ComboOption(SODA, Money("0.00"), true, false),
                                ComboOption(WATER, Money("5.00"), true, false),
                            ),
                    ),
                    ComboSlot(
                        id = SLOT_FRIES,
                        name = "Papas del combo",
                        minQuantity = 0,
                        maxQuantity = 2,
                        includedQuantity = 1,
                        defaultQuantity = 1,
                        defaultProductId = FRIES,
                        options = listOf(ComboOption(FRIES, Money("0.00"), true, false)),
                    ),
                ),
        )

    val soldOutShake =
        product(
            id = SOLD_OUT_SHAKE,
            name = "Malteada de fresa",
            categoryId = DRINKS,
            price = "60.00",
            availability =
                ProductAvailability(
                    availableNow = false,
                    reason = "Agotado hasta nuevo aviso.",
                    soldOut = true,
                    soldOutIndefinite = true,
                    soldOutUntil = null,
                ),
        )

    val quickNotes =
        listOf(
            note(NOTE_NO_ONION, "Sin cebolla", QuickNoteEffect.OMIT, ONION, 1),
            note(NOTE_EXTRA_ONION, "Extra cebolla", QuickNoteEffect.EXTRA, ONION, 2),
            note(NOTE_NORMAL_ONION, "Con cebolla", QuickNoteEffect.NORMAL, ONION, 3),
            note(NOTE_WITH_BACON, "Con tocino", QuickNoteEffect.NORMAL, BACON, 4),
            note(NOTE_WELL_DONE, "Bien cocida", QuickNoteEffect.NONE, null, 5),
            note(NOTE_NO_PICKLE, "Sin pepinillos", QuickNoteEffect.OMIT, PICKLE, 6),
        )

    val catalog =
        Catalog(
            version =
                CatalogVersion(
                    version = 10,
                    epoch = "e0000000-0000-4000-8000-000000000001",
                    generatedAt = "2026-11-16T12:00:00Z",
                    timezone = "America/Monterrey",
                    taxRate = 16,
                    priceListName = "General",
                    syncedAtMillis = 1,
                ),
            categories =
                listOf(
                    CatalogCategory(DRINKS, "Bebidas", "", true, 2, null, true),
                    CatalogCategory(BURGERS, "Hamburguesas", "", true, 1, null, true),
                ),
            products = listOf(soda, burger, water, fries, combo, soldOutShake),
            groups = listOf(sideGroup, sauceGroup),
            quickNotes = quickNotes,
        )

    fun option(
        id: String,
        name: String,
        extra: String = "0.00",
        maxQuantity: String? = null,
        soldOut: Boolean = false,
        portionId: String? = null,
        portionName: String? = null,
    ) =
        GroupOption(
            id = id,
            complementId = "x$id".take(36),
            name = name,
            portionId = portionId,
            portionName = portionName,
            portionFactor = if (portionId != null) "2" else null,
            referenceValue = Money("0.00"),
            extraPrice = Money(extra),
            firstTierPrice = null,
            maxQuantity = maxQuantity,
            active = true,
            availableNow = !soldOut,
            soldOut = soldOut,
        )

    fun product(
        id: String,
        name: String,
        categoryId: String,
        price: String?,
        kind: ProductKind = ProductKind.PRODUCT,
        variants: List<ProductVariant> = emptyList(),
        ingredients: List<IngredientRule> = emptyList(),
        groups: List<GroupBinding> = emptyList(),
        slots: List<ComboSlot> = emptyList(),
        availability: ProductAvailability = ProductAvailability.AVAILABLE,
    ) =
        CatalogProduct(
            id = id,
            name = name,
            description = "",
            active = true,
            categoryId = categoryId,
            code = name.take(3).uppercase(),
            barcode = null,
            kind = kind,
            referencePrice = price?.let { Money(it) },
            pricingError = if (price == null) "Sin precio en la lista." else null,
            imageId = null,
            sortOrder = 0,
            variants = variants,
            ingredients = ingredients,
            groups = groups,
            slots = slots,
            availability = availability,
        )

    private fun note(
        id: String,
        text: String,
        effect: QuickNoteEffect,
        ingredientId: String?,
        order: Int,
    ) =
        QuickNote(
            id = id,
            categoryId = BURGERS,
            text = text,
            effect = effect,
            ingredientId = ingredientId,
            active = true,
            sortOrder = order,
        )
}
