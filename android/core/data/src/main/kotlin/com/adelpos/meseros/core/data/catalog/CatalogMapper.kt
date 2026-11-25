package com.adelpos.meseros.core.data.catalog

import com.adelpos.core.network.api.ApiJson
import com.adelpos.meseros.core.data.catalog.db.CatalogStateEntity
import com.adelpos.meseros.core.data.catalog.db.CategoryEntity
import com.adelpos.meseros.core.data.catalog.db.ComplementEntity
import com.adelpos.meseros.core.data.catalog.db.IngredientEntity
import com.adelpos.meseros.core.data.catalog.db.OptionGroupEntity
import com.adelpos.meseros.core.data.catalog.db.ProductEntity
import com.adelpos.meseros.core.data.catalog.db.QuickNoteEntity
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogCategory
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogVersion
import com.adelpos.meseros.core.domain.catalog.ComboOption
import com.adelpos.meseros.core.domain.catalog.ComboSlot
import com.adelpos.meseros.core.domain.catalog.GroupBinding
import com.adelpos.meseros.core.domain.catalog.GroupOption
import com.adelpos.meseros.core.domain.catalog.GroupPricingMode
import com.adelpos.meseros.core.domain.catalog.IngredientRule
import com.adelpos.meseros.core.domain.catalog.OptionGroup
import com.adelpos.meseros.core.domain.catalog.OptionQuantity
import com.adelpos.meseros.core.domain.catalog.ProductAvailability
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.catalog.ProductVariant
import com.adelpos.meseros.core.domain.catalog.QuickNote
import com.adelpos.meseros.core.domain.catalog.QuickNoteEffect
import kotlinx.serialization.json.Json

/**
 * Convierte las filas de catalog.db (con el documento de Go de cada registro) en el [Catalog] del
 * dominio. Los nombres que el documento no trae (opciones de grupos o reglas de ingredientes de una
 * versión anterior del servidor) se completan con los registros del catálogo.
 */
class CatalogMapper(private val json: Json = ApiJson) {
    fun toCatalog(
        state: CatalogStateEntity?,
        categories: List<CategoryEntity>,
        products: List<ProductEntity>,
        groups: List<OptionGroupEntity>,
        ingredients: List<IngredientEntity>,
        complements: List<ComplementEntity>,
        quickNotes: List<QuickNoteEntity>,
    ): Catalog {
        if (state == null) {
            return Catalog.EMPTY
        }
        val ingredientNames = ingredients.associate { it.id to it.name }
        val complementNames = complements.associate { it.id to it.name }
        return Catalog(
            version = version(state),
            categories = categories.map { category(it) },
            products = products.map { product(it, ingredientNames) },
            groups = groups.map { group(it, complementNames) },
            quickNotes = quickNotes.map { quickNote(it) },
        )
    }

    fun version(state: CatalogStateEntity): CatalogVersion =
        CatalogVersion(
            version = state.version,
            epoch = state.epoch,
            generatedAt = state.generatedAt,
            timezone = state.timezone,
            taxRate = state.taxRate,
            priceListName = state.priceListName,
            syncedAtMillis = state.syncedAtMillis,
        )

    fun category(entity: CategoryEntity): CatalogCategory {
        val dto = json.decodeFromString(CategoryDto.serializer(), entity.document)
        return CatalogCategory(
            id = dto.id,
            name = dto.name,
            description = dto.description,
            active = dto.active,
            sortOrder = dto.sortOrder,
            imageId = dto.imageId,
            availableNow = dto.availableNow,
        )
    }

    fun product(entity: ProductEntity, ingredientNames: Map<String, String>): CatalogProduct {
        val dto = json.decodeFromString(ProductDto.serializer(), entity.document)
        return CatalogProduct(
            id = dto.id,
            name = dto.name,
            description = dto.description,
            active = dto.active,
            categoryId = dto.categoryId,
            code = dto.code,
            barcode = dto.barcode,
            kind =
                if (dto.kind == "combo") {
                    ProductKind.COMBO
                } else {
                    ProductKind.PRODUCT
                },
            referencePrice = dto.price,
            pricingError = dto.pricingError,
            imageId = dto.imageId,
            sortOrder = dto.sortOrder,
            variants =
                dto.variants.map { variant ->
                    ProductVariant(
                        id = variant.id,
                        name = variant.name,
                        code = variant.code,
                        active = variant.active,
                        referencePrice = variant.effectivePrice,
                        pricingError = variant.pricingError,
                    )
                },
            ingredients =
                dto.ingredients.map { rule ->
                    IngredientRule(
                        ingredientId = rule.ingredientId,
                        name = rule.name ?: ingredientNames[rule.ingredientId] ?: "",
                        includedQuantity = rule.includedQuantity,
                        removable = rule.removable,
                        addable = rule.addable,
                        maxQuantity = rule.maxQuantity,
                        extraPrice = rule.extraPrice,
                    )
                },
            groups =
                dto.groups.map { binding ->
                    GroupBinding(
                        groupId = binding.groupId,
                        minQuantity = binding.minQuantity,
                        maxQuantity = binding.maxQuantity,
                        included =
                            binding.included.map { OptionQuantity(it.optionId, it.quantity) },
                    )
                },
            slots = dto.slots.map { slot(it) },
            availability =
                ProductAvailability(
                    availableNow = dto.availableNow,
                    reason = dto.availabilityReason,
                    soldOut = dto.soldOut,
                    soldOutIndefinite = dto.soldOutIndefinite,
                    soldOutUntil = dto.soldOutUntil,
                ),
        )
    }

    private fun slot(dto: ComboSlotDto): ComboSlot =
        ComboSlot(
            id = dto.id,
            name = dto.name,
            minQuantity = dto.minQuantity,
            maxQuantity = dto.maxQuantity,
            includedQuantity = dto.includedQuantity,
            defaultQuantity = dto.defaultQuantity,
            defaultProductId = dto.defaultProductId,
            options =
                dto.options.map { option ->
                    ComboOption(
                        productId = option.productId,
                        substitutionPrice = option.substitutionPrice,
                        availableNow = option.availableNow,
                        soldOut = option.soldOut,
                    )
                },
        )

    fun group(entity: OptionGroupEntity, complementNames: Map<String, String>): OptionGroup {
        val dto = json.decodeFromString(OptionGroupDto.serializer(), entity.document)
        return OptionGroup(
            id = dto.id,
            name = dto.name,
            description = dto.description,
            active = dto.active,
            pricingMode =
                if (dto.pricingMode == "quantity") {
                    GroupPricingMode.QUANTITY
                } else {
                    GroupPricingMode.VALUE
                },
            minQuantity = dto.minQuantity,
            maxQuantity = dto.maxQuantity,
            options =
                dto.options.map { option ->
                    GroupOption(
                        id = option.id,
                        complementId = option.complementId,
                        name = option.name ?: complementNames[option.complementId] ?: "",
                        portionId = option.portionId,
                        portionName = option.portionName,
                        portionFactor = option.portionFactor,
                        referenceValue = option.referenceValue,
                        extraPrice = option.extraPrice,
                        firstTierPrice = option.tiers?.firstOrNull()?.unitPrice,
                        maxQuantity = option.maxQuantity,
                        active = option.active,
                        availableNow = option.availableNow,
                        soldOut = option.soldOut,
                    )
                },
        )
    }

    fun quickNote(entity: QuickNoteEntity): QuickNote {
        val dto = json.decodeFromString(QuickNoteDto.serializer(), entity.document)
        val effect =
            when (dto.effect) {
                "omit" -> QuickNoteEffect.OMIT
                "normal" -> QuickNoteEffect.NORMAL
                "extra" -> QuickNoteEffect.EXTRA
                else -> QuickNoteEffect.NONE
            }
        return QuickNote(
            id = dto.id,
            categoryId = dto.categoryId,
            text = dto.text,
            effect = effect,
            ingredientId = dto.ingredientId,
            active = dto.active,
            sortOrder = dto.sortOrder,
        )
    }
}
