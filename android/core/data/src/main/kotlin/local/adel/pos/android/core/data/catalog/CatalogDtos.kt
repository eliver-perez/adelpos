package local.adel.pos.android.core.data.catalog

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import local.adel.pos.android.core.data.api.MoneySerializer
import local.adel.pos.android.core.domain.money.Money

/*
 * JSON de `GET catalog/sync` (`CatalogSync` del contrato v1, `backend/internal/catalog/sync.go`).
 * La respuesta se lee en dos pasos: primero el sobre con cada registro como JsonObject (para
 * guardar el documento tal como llegó y no perder campos que esta versión todavía no usa) y después
 * cada registro con su DTO, solo para los campos que la aplicación necesita. Los campos opcionales
 * del contrato llevan un valor por omisión; los importes son cadenas decimales (MoneySerializer).
 */

/** Sobre de la descarga: versión, época y los registros sin interpretar. */
@Serializable
data class CatalogSyncEnvelope(
    @SerialName("catalog_version") val catalogVersion: Long,
    @SerialName("catalog_epoch") val catalogEpoch: String,
    val since: Long = 0,
    val full: Boolean,
    val reset: Boolean = false,
    @SerialName("generated_at") val generatedAt: String = "",
    val timezone: String = "",
    @SerialName("tax_rate") val taxRate: Int = 0,
    @SerialName("price_list") val priceList: PriceListDto,
    val categories: List<JsonObject> = emptyList(),
    val products: List<JsonObject> = emptyList(),
    val ingredients: List<JsonObject> = emptyList(),
    val complements: List<JsonObject> = emptyList(),
    val portions: List<JsonObject> = emptyList(),
    val groups: List<JsonObject> = emptyList(),
    @SerialName("quick_notes") val quickNotes: List<JsonObject> = emptyList(),
    val images: List<ImageRefDto> = emptyList(),
    val deleted: List<DeletedDto> = emptyList(),
)

@Serializable data class PriceListDto(val id: String, val name: String, val revision: Long = 0)

@Serializable data class DeletedDto(val entity: String, val id: String)

@Serializable
data class ImageRefDto(
    val id: String,
    @SerialName("media_type") val mediaType: String,
    val width: Int,
    val height: Int,
    val size: Int,
    val checksum: String,
)

@Serializable
data class CategoryDto(
    val id: String,
    val name: String,
    val description: String = "",
    val active: Boolean,
    @SerialName("sort_order") val sortOrder: Int = 0,
    @SerialName("image_id") val imageId: String? = null,
    @SerialName("available_now") val availableNow: Boolean = true,
)

@Serializable
data class ProductDto(
    val id: String,
    val name: String,
    val description: String = "",
    val active: Boolean,
    @SerialName("category_id") val categoryId: String,
    val code: String = "",
    val barcode: String? = null,
    val kind: String = "product",
    @SerialName("image_id") val imageId: String? = null,
    @SerialName("sort_order") val sortOrder: Int = 0,
    val archived: Boolean = false,
    val variants: List<VariantDto> = emptyList(),
    val ingredients: List<ProductIngredientDto> = emptyList(),
    val groups: List<ProductGroupDto> = emptyList(),
    val slots: List<ComboSlotDto> = emptyList(),
    @SerialName("available_now") val availableNow: Boolean = true,
    @SerialName("availability_reason") val availabilityReason: String = "",
    @SerialName("sold_out") val soldOut: Boolean = false,
    @SerialName("sold_out_indefinite") val soldOutIndefinite: Boolean = false,
    @SerialName("sold_out_until") val soldOutUntil: String? = null,
    @Serializable(with = MoneySerializer::class) val price: Money? = null,
    @SerialName("pricing_error") val pricingError: String? = null,
)

@Serializable
data class VariantDto(
    val id: String,
    val name: String,
    val code: String = "",
    val active: Boolean,
    @SerialName("effective_price")
    @Serializable(with = MoneySerializer::class)
    val effectivePrice: Money? = null,
    @SerialName("pricing_error") val pricingError: String? = null,
)

@Serializable
data class ProductIngredientDto(
    @SerialName("ingredient_id") val ingredientId: String,
    @SerialName("included_quantity") val includedQuantity: String,
    val removable: Boolean,
    val addable: Boolean,
    @SerialName("max_quantity") val maxQuantity: String,
    @SerialName("extra_price") @Serializable(with = MoneySerializer::class) val extraPrice: Money,
    val name: String? = null,
)

@Serializable
data class OptionQuantityDto(
    @SerialName("option_id") val optionId: String,
    val quantity: String,
)

@Serializable
data class ProductGroupDto(
    @SerialName("group_id") val groupId: String,
    @SerialName("min_quantity") val minQuantity: String? = null,
    @SerialName("max_quantity") val maxQuantity: String? = null,
    val included: List<OptionQuantityDto> = emptyList(),
)

@Serializable
data class ComboSlotDto(
    val id: String,
    val name: String,
    @SerialName("min_quantity") val minQuantity: Int,
    @SerialName("max_quantity") val maxQuantity: Int,
    @SerialName("included_quantity") val includedQuantity: Int = 0,
    @SerialName("default_quantity") val defaultQuantity: Int = 0,
    @SerialName("default_product_id") val defaultProductId: String,
    val options: List<ComboOptionDto> = emptyList(),
)

@Serializable
data class ComboOptionDto(
    @SerialName("product_id") val productId: String,
    @SerialName("substitution_price")
    @Serializable(with = MoneySerializer::class)
    val substitutionPrice: Money,
    @SerialName("available_now") val availableNow: Boolean = true,
    @SerialName("sold_out") val soldOut: Boolean = false,
)

@Serializable
data class TierDto(
    val from: Int,
    @SerialName("unit_price") @Serializable(with = MoneySerializer::class) val unitPrice: Money,
)

@Serializable
data class OptionGroupDto(
    val id: String,
    val name: String,
    val description: String = "",
    val active: Boolean,
    @SerialName("pricing_mode") val pricingMode: String = "value",
    @SerialName("min_quantity") val minQuantity: String = "0",
    @SerialName("max_quantity") val maxQuantity: String? = null,
    val options: List<GroupOptionDto> = emptyList(),
)

@Serializable
data class GroupOptionDto(
    val id: String,
    @SerialName("complement_id") val complementId: String,
    @SerialName("portion_id") val portionId: String? = null,
    @SerialName("reference_value")
    @Serializable(with = MoneySerializer::class)
    val referenceValue: Money,
    @SerialName("extra_price") @Serializable(with = MoneySerializer::class) val extraPrice: Money,
    @SerialName("max_quantity") val maxQuantity: String? = null,
    val active: Boolean,
    val tiers: List<TierDto>? = null,
    val name: String? = null,
    @SerialName("portion_name") val portionName: String? = null,
    @SerialName("portion_factor") val portionFactor: String? = null,
    @SerialName("available_now") val availableNow: Boolean = true,
    @SerialName("sold_out") val soldOut: Boolean = false,
)

@Serializable
data class NamedRecordDto(val id: String, val name: String, val active: Boolean = true)

@Serializable
data class QuickNoteDto(
    val id: String,
    @SerialName("category_id") val categoryId: String,
    val text: String,
    val effect: String? = null,
    @SerialName("ingredient_id") val ingredientId: String? = null,
    val active: Boolean,
    @SerialName("sort_order") val sortOrder: Int = 0,
)
