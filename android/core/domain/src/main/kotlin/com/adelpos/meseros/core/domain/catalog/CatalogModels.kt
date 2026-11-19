package com.adelpos.meseros.core.domain.catalog

import com.adelpos.meseros.core.domain.money.Money

/**
 * Modelos del catálogo guardado en el dispositivo, tal como los entrega Go en `GET catalog/sync`.
 *
 * Reglas que valen para todo el paquete:
 * - Los precios son **referencia**: los calculó Go con la lista predeterminada al momento de la
 *   descarga. El importe de un pedido siempre lo vuelve a calcular el servidor.
 * - La disponibilidad («Agotado», «Pausar», horarios) también es la del momento de la descarga; el
 *   servidor decide al recibir cada pedido.
 * - Las cantidades de porciones e ingredientes son cadenas decimales ([PortionQuantity]).
 */
enum class ProductKind {
    PRODUCT,
    COMBO,
}

/** Categoría del catálogo («Bebidas», «Hamburguesas»). */
data class CatalogCategory(
    val id: String,
    val name: String,
    val description: String,
    val active: Boolean,
    val sortOrder: Int,
    val imageId: String?,
    val availableNow: Boolean,
)

/**
 * Disponibilidad de un producto calculada por Go al momento de la descarga.
 *
 * @property availableNow si se podía elegir en ese momento (activo, sin pausa, dentro de su horario
 *   y sin «Agotado»).
 * @property reason motivo en español cuando no está disponible (lo escribe Go).
 * @property soldOut «Agotado» marcado desde la captura; con [soldOutIndefinite], «hasta nuevo
 *   aviso» (si no, hasta el inicio del día comercial siguiente, [soldOutUntil]).
 */
data class ProductAvailability(
    val availableNow: Boolean,
    val reason: String,
    val soldOut: Boolean,
    val soldOutIndefinite: Boolean,
    val soldOutUntil: String?,
) {
    /** Qué marca se muestra en la tarjeta del producto. */
    val status: AvailabilityStatus
        get() =
            when {
                soldOut && soldOutIndefinite -> AvailabilityStatus.SOLD_OUT_INDEFINITE
                soldOut -> AvailabilityStatus.SOLD_OUT
                !availableNow -> AvailabilityStatus.UNAVAILABLE
                else -> AvailabilityStatus.AVAILABLE
            }

    companion object {
        val AVAILABLE =
            ProductAvailability(
                availableNow = true,
                reason = "",
                soldOut = false,
                soldOutIndefinite = false,
                soldOutUntil = null,
            )
    }
}

/** Marca de disponibilidad de un producto en la captura. */
enum class AvailabilityStatus {
    AVAILABLE,

    /** «Agotado» (hasta el inicio del día comercial siguiente). */
    SOLD_OUT,

    /** «Agotado hasta nuevo aviso». */
    SOLD_OUT_INDEFINITE,

    /** Pausado, fuera de horario o inactivo: el motivo lo da Go. */
    UNAVAILABLE,
}

/** Presentación de un producto («Chica», «Grande»), con su precio de referencia. */
data class ProductVariant(
    val id: String,
    val name: String,
    val code: String,
    val active: Boolean,
    val referencePrice: Money?,
    val pricingError: String?,
)

/**
 * Regla de un ingrediente dentro de un producto: cuánto lleva ([includedQuantity]), si se puede
 * quitar o agregar y hasta cuánto ([maxQuantity]), y el cargo de referencia por porción extra.
 */
data class IngredientRule(
    val ingredientId: String,
    val name: String,
    val includedQuantity: String,
    val removable: Boolean,
    val addable: Boolean,
    val maxQuantity: String,
    val extraPrice: Money,
)

/** Cantidad elegida de una opción de un grupo (también lo incluido de origen). */
data class OptionQuantity(val optionId: String, val quantity: String)

/**
 * Grupo de complementos que ofrece un producto. Los límites del producto, si existen, sustituyen
 * los del grupo reutilizable; [included] es la configuración original del producto.
 */
data class GroupBinding(
    val groupId: String,
    val minQuantity: String?,
    val maxQuantity: String?,
    val included: List<OptionQuantity>,
)

/** Cómo cobra Go un grupo: por equivalencia de valor o por cantidad (tramos). */
enum class GroupPricingMode {
    VALUE,
    QUANTITY,
}

/** Grupo de complementos reutilizable («Guarnición», «Salsas»). */
data class OptionGroup(
    val id: String,
    val name: String,
    val description: String,
    val active: Boolean,
    val pricingMode: GroupPricingMode,
    val minQuantity: String,
    val maxQuantity: String?,
    val options: List<GroupOption>,
)

/**
 * Opción de un grupo: un complemento, quizá con una porción propia («Arroz · Media»).
 *
 * @property availableNow y [soldOut] los calcula Go en la descarga (un complemento que es un
 *   producto agotado no se puede elegir).
 * @property firstTierPrice precio de referencia de la primera porción adicional en los grupos por
 *   cantidad (solo para mostrar).
 */
data class GroupOption(
    val id: String,
    val complementId: String,
    val name: String,
    val portionId: String?,
    val portionName: String?,
    val portionFactor: String?,
    val referenceValue: Money,
    val extraPrice: Money,
    val firstTierPrice: Money?,
    val maxQuantity: String?,
    val active: Boolean,
    val availableNow: Boolean,
    val soldOut: Boolean,
) {
    /** Nombre para el botón: con la porción solo cuando la opción tiene una porción propia. */
    val displayName: String
        get() =
            if (portionId != null && !portionName.isNullOrBlank()) {
                "$name · $portionName"
            } else {
                name
            }

    /** Si el mesero la puede elegir según lo que dijo Go en la descarga. */
    val selectable: Boolean
        get() = active && availableNow && !soldOut
}

/** Ranura de un combo («Bebida del combo») con los productos que la pueden ocupar. */
data class ComboSlot(
    val id: String,
    val name: String,
    val minQuantity: Int,
    val maxQuantity: Int,
    val includedQuantity: Int,
    val defaultQuantity: Int,
    val defaultProductId: String,
    val options: List<ComboOption>,
) {
    /**
     * Productos que pueden ocupar la ranura: el original siempre (de forma implícita, etapa 6.1) y
     * después las alternativas configuradas, sin repetir.
     */
    val productIds: List<String>
        get() {
            val identifiers = mutableListOf(defaultProductId)
            for (option in options) {
                if (option.productId !in identifiers) {
                    identifiers.add(option.productId)
                }
            }
            return identifiers
        }

    fun option(productId: String): ComboOption? = options.firstOrNull { it.productId == productId }
}

/** Alternativa de una ranura de combo con su cargo de sustitución de referencia. */
data class ComboOption(
    val productId: String,
    val substitutionPrice: Money,
    val availableNow: Boolean,
    val soldOut: Boolean,
)

/** Efecto de una indicación rápida sobre un ingrediente (etapa 8). */
enum class QuickNoteEffect {
    NONE,
    OMIT,
    NORMAL,
    EXTRA,
}

/** Indicación rápida de una categoría («Sin cebolla», «Bien cocido»). */
data class QuickNote(
    val id: String,
    val categoryId: String,
    val text: String,
    val effect: QuickNoteEffect,
    val ingredientId: String?,
    val active: Boolean,
    val sortOrder: Int,
)

/** Producto del catálogo con todo lo que necesita la captura. */
data class CatalogProduct(
    val id: String,
    val name: String,
    val description: String,
    val active: Boolean,
    val categoryId: String,
    val code: String,
    val barcode: String?,
    val kind: ProductKind,
    val referencePrice: Money?,
    val pricingError: String?,
    val imageId: String?,
    val sortOrder: Int,
    val variants: List<ProductVariant>,
    val ingredients: List<IngredientRule>,
    val groups: List<GroupBinding>,
    val slots: List<ComboSlot>,
    val availability: ProductAvailability,
) {
    val activeVariants: List<ProductVariant>
        get() = variants.filter { it.active }

    /**
     * Si el mesero puede elegirlo con un toque: disponible según Go y con un precio que Go pudo
     * determinar (o presentaciones activas que lo tengan). Misma regla que `productSelectable` del
     * escritorio.
     */
    val selectable: Boolean
        get() {
            if (!availability.availableNow || availability.soldOut) {
                return false
            }
            return referencePrice != null || activeVariants.isNotEmpty()
        }

    /**
     * Sin precio base en la lista pero con presentaciones: hay que elegir una (la base no se puede
     * pedir).
     */
    val requiresVariant: Boolean
        get() = referencePrice == null && activeVariants.isNotEmpty()
}

/** Imagen referida por el catálogo (el contenido se descarga aparte, con su `ETag`). */
data class CatalogImageRef(
    val id: String,
    val mediaType: String,
    val width: Int,
    val height: Int,
    val size: Int,
    val checksum: String,
)

/** Versión y época del catálogo guardado y datos generales de la descarga. */
data class CatalogVersion(
    val version: Long,
    val epoch: String,
    val generatedAt: String,
    val timezone: String,
    val taxRate: Int,
    val priceListName: String,
    val syncedAtMillis: Long,
)

/**
 * Fotografía del catálogo guardado en el dispositivo, con búsquedas por identificador para la
 * captura y el personalizador. Inmutable: cada descarga produce una nueva.
 */
class Catalog(
    val version: CatalogVersion?,
    categories: List<CatalogCategory>,
    products: List<CatalogProduct>,
    groups: List<OptionGroup>,
    quickNotes: List<QuickNote>,
) {
    /** Categorías activas en su orden del catálogo. */
    val categories: List<CatalogCategory> =
        categories.filter { it.active }.sortedWith(compareBy({ it.sortOrder }, { it.name }))

    /** Productos activos en el orden de la captura: categoría, orden propio y nombre. */
    val products: List<CatalogProduct>

    private val productsById: Map<String, CatalogProduct> = products.associateBy { it.id }
    private val groupsById: Map<String, OptionGroup> = groups.associateBy { it.id }
    private val quickNotesByCategory: Map<String, List<QuickNote>> =
        quickNotes
            .filter { it.active }
            .sortedWith(compareBy({ it.sortOrder }, { it.text }))
            .groupBy { it.categoryId }

    init {
        val categoryOrder = this.categories.withIndex().associate { it.value.id to it.index }
        this.products =
            products
                .filter { it.active }
                .sortedWith(
                    compareBy(
                        { categoryOrder[it.categoryId] ?: Int.MAX_VALUE },
                        { it.sortOrder },
                        { it.name },
                    ),
                )
    }

    val isEmpty: Boolean
        get() = version == null

    /** Cualquier producto guardado (también un inactivo que es componente de un combo). */
    fun product(id: String): CatalogProduct? = productsById[id]

    fun group(id: String): OptionGroup? = groupsById[id]

    fun productsIn(categoryId: String): List<CatalogProduct> = products.filter {
        it.categoryId == categoryId
    }

    /** Indicaciones rápidas activas de la categoría del producto, en su orden del catálogo. */
    fun quickNotesFor(product: CatalogProduct): List<QuickNote> =
        quickNotesByCategory[product.categoryId].orEmpty()

    companion object {
        /** Catálogo vacío: todavía no se ha descargado nada. */
        val EMPTY =
            Catalog(
                version = null,
                categories = emptyList(),
                products = emptyList(),
                groups = emptyList(),
                quickNotes = emptyList(),
            )
    }
}
