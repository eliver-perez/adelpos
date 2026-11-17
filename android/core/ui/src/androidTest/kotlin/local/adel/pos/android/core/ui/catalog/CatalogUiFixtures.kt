package local.adel.pos.android.core.ui.catalog

import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogCategory
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.CatalogVersion
import local.adel.pos.android.core.domain.catalog.ComboOption
import local.adel.pos.android.core.domain.catalog.ComboSlot
import local.adel.pos.android.core.domain.catalog.GroupBinding
import local.adel.pos.android.core.domain.catalog.GroupOption
import local.adel.pos.android.core.domain.catalog.GroupPricingMode
import local.adel.pos.android.core.domain.catalog.IngredientRule
import local.adel.pos.android.core.domain.catalog.OptionGroup
import local.adel.pos.android.core.domain.catalog.OptionQuantity
import local.adel.pos.android.core.domain.catalog.ProductAvailability
import local.adel.pos.android.core.domain.catalog.ProductKind
import local.adel.pos.android.core.domain.catalog.ProductVariant
import local.adel.pos.android.core.domain.catalog.QuickNote
import local.adel.pos.android.core.domain.catalog.QuickNoteEffect
import local.adel.pos.android.core.domain.money.Money

/**
 * Catálogo pequeño para las pruebas de los componentes (hamburguesa, refresco, combo y agotados).
 */
object CatalogUiFixtures {
    const val BURGERS = "c1"
    const val DRINKS = "c2"
    const val BURGER = "p-hamburguesa"
    const val SODA = "p-refresco"
    const val COMBO = "p-combo"
    const val SHAKE = "p-malteada"
    const val COFFEE = "p-cafe"
    const val SODA_LARGE = "v-grande"
    const val ONION = "i-cebolla"
    const val SIDES = "g-guarnicion"
    const val SAUCES = "g-salsas"
    const val SALAD = "o-ensalada"
    const val KETCHUP = "o-catsup"
    const val NO_ONION = "n-sin-cebolla"
    const val SLOT_DRINK = "s-bebida"

    private fun option(id: String, name: String, extra: String = "0.00", max: String? = null) =
        GroupOption(
            id = id,
            complementId = "x-$id",
            name = name,
            portionId = null,
            portionName = null,
            portionFactor = null,
            referenceValue = Money("0.00"),
            extraPrice = Money(extra),
            firstTierPrice = null,
            maxQuantity = max,
            active = true,
            availableNow = true,
            soldOut = false,
        )

    private fun product(
        id: String,
        name: String,
        category: String,
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
            categoryId = category,
            code = id,
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

    val burger =
        product(
            id = BURGER,
            name = "Hamburguesa clásica",
            category = BURGERS,
            price = "120.00",
            ingredients =
                listOf(
                    IngredientRule(ONION, "Cebolla", "1", true, true, "2", Money("5.00")),
                ),
            groups =
                listOf(
                    GroupBinding(SIDES, null, null, listOf(OptionQuantity("o-papas", "1"))),
                    GroupBinding(SAUCES, null, null, emptyList()),
                ),
        )

    val soda =
        product(
            id = SODA,
            name = "Refresco",
            category = DRINKS,
            price = null,
            variants =
                listOf(
                    ProductVariant("v-chico", "Chico", "R-CH", true, Money("25.00"), null),
                    ProductVariant(SODA_LARGE, "Grande", "R-GR", true, Money("35.00"), null),
                ),
        )

    val combo =
        product(
            id = COMBO,
            name = "Combo hamburguesa",
            category = BURGERS,
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
                        defaultProductId = COFFEE,
                        options =
                            listOf(
                                ComboOption(COFFEE, Money("0.00"), false, true),
                                ComboOption(SHAKE, Money("10.00"), false, true),
                            ),
                    ),
                ),
        )

    val shake =
        product(
            id = SHAKE,
            name = "Malteada de fresa",
            category = DRINKS,
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

    val coffee =
        product(
            id = COFFEE,
            name = "Café de olla",
            category = DRINKS,
            price = "30.00",
            availability =
                ProductAvailability(
                    availableNow = false,
                    reason = "Agotado.",
                    soldOut = true,
                    soldOutIndefinite = false,
                    soldOutUntil = "2026-11-17T11:00:00Z",
                ),
        )

    val catalog =
        Catalog(
            version = CatalogVersion(10, "e", "", "America/Monterrey", 16, "General", 1),
            categories =
                listOf(
                    CatalogCategory(BURGERS, "Hamburguesas", "", true, 1, null, true),
                    CatalogCategory(DRINKS, "Bebidas", "", true, 2, null, true),
                ),
            products = listOf(burger, soda, combo, shake, coffee),
            groups =
                listOf(
                    OptionGroup(
                        id = SIDES,
                        name = "Guarnición",
                        description = "",
                        active = true,
                        pricingMode = GroupPricingMode.VALUE,
                        minQuantity = "1",
                        maxQuantity = "1",
                        options =
                            listOf(option("o-papas", "Papas"), option(SALAD, "Ensalada", "10.00")),
                    ),
                    OptionGroup(
                        id = SAUCES,
                        name = "Salsas",
                        description = "",
                        active = true,
                        pricingMode = GroupPricingMode.QUANTITY,
                        minQuantity = "0",
                        maxQuantity = "2",
                        options =
                            listOf(
                                option(KETCHUP, "Catsup", max = "2"),
                                option("o-mostaza", "Mostaza"),
                            ),
                    ),
                ),
            quickNotes =
                listOf(
                    QuickNote(
                        NO_ONION,
                        BURGERS,
                        "Sin cebolla",
                        QuickNoteEffect.OMIT,
                        ONION,
                        true,
                        1,
                    ),
                    QuickNote(
                        "n-cocida",
                        BURGERS,
                        "Bien cocida",
                        QuickNoteEffect.NONE,
                        null,
                        true,
                        2,
                    ),
                ),
        )
}
