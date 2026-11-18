package com.adelpos.meseros.core.domain.catalog

/**
 * Algo que impide agregar la partida tal como está. Solo se revisa lo que Go ya declara en el
 * catálogo (mínimos, máximos y obligatorios de cada grupo y ranura, presentación obligatoria, el
 * límite de las observaciones y lo que Go marcó como no disponible); el servidor vuelve a validar
 * todo, también los precios, al recibir el pedido. [where] es la ranura del combo donde está el
 * problema (nula en el producto principal).
 */
sealed interface CustomizationProblem {
    val where: String?

    /** El producto no se puede elegir: agotado o no disponible según la última descarga. */
    data class ProductNotSelectable(
        val productName: String,
        val status: AvailabilityStatus,
        val reason: String,
        override val where: String?,
    ) : CustomizationProblem

    /** Sin precio base: hay que elegir una presentación. */
    data class VariantRequired(val productName: String, override val where: String?) :
        CustomizationProblem

    /** El grupo pide al menos [minimum] porciones. */
    data class GroupBelowMinimum(
        val groupName: String,
        val minimum: String,
        override val where: String?,
    ) : CustomizationProblem

    /** El grupo admite hasta [maximum] porciones. */
    data class GroupAboveMaximum(
        val groupName: String,
        val maximum: String,
        override val where: String?,
    ) : CustomizationProblem

    /** La opción admite hasta [maximum] porciones. */
    data class OptionAboveMaximum(
        val optionName: String,
        val maximum: String,
        override val where: String?,
    ) : CustomizationProblem

    /** Una opción elegida ya no se puede elegir (agotada, inactiva o no disponible). */
    data class OptionNotSelectable(val optionName: String, override val where: String?) :
        CustomizationProblem

    /** La cantidad de la ranura está fuera de sus límites. */
    data class ComponentOutOfRange(
        val slotName: String,
        val minimum: Int,
        val maximum: Int,
        override val where: String?,
    ) : CustomizationProblem

    /** Las observaciones superan el límite de Go (500 bytes). */
    data class NotesTooLong(override val where: String?) : CustomizationProblem
}

/** Revisión en el dispositivo de una partida antes de agregarla (ver [CustomizationProblem]). */
object CustomizationCheck {
    /** Problemas de la partida; vacía si se puede agregar. */
    fun problems(catalog: Catalog, line: LineSelection): List<CustomizationProblem> {
        val product = catalog.product(line.productId) ?: return emptyList()
        val result = mutableListOf<CustomizationProblem>()
        if (!product.selectable) {
            result.add(
                CustomizationProblem.ProductNotSelectable(
                    productName = product.name,
                    status = product.availability.status,
                    reason = product.availability.reason,
                    where = null,
                ),
            )
        }
        checkLevel(catalog, product, line.variantId, line.customization, null, 0, result)
        return result
    }

    private fun checkLevel(
        catalog: Catalog,
        product: CatalogProduct,
        variantId: String?,
        value: Customization,
        where: String?,
        depth: Int,
        result: MutableList<CustomizationProblem>,
    ) {
        if (depth > ComboChoices.MAXIMUM_DEPTH) {
            return
        }
        if (product.requiresVariant && variantId == null) {
            result.add(CustomizationProblem.VariantRequired(product.name, where))
        }
        if (!KitchenNotes.fits(value.notes)) {
            result.add(CustomizationProblem.NotesTooLong(where))
        }
        for (binding in product.groups) {
            val group = catalog.group(binding.groupId) ?: continue
            checkGroup(value, binding, group, where, result)
        }
        for (slot in product.slots) {
            checkSlot(catalog, value, slot, depth, result)
        }
    }

    private fun checkGroup(
        value: Customization,
        binding: GroupBinding,
        group: OptionGroup,
        where: String?,
        result: MutableList<CustomizationProblem>,
    ) {
        val limits = GroupChoices.limits(binding, group)
        val total = GroupChoices.total(value, binding)
        if (total < limits.minimum) {
            result.add(
                CustomizationProblem.GroupBelowMinimum(
                    groupName = group.name,
                    minimum = PortionQuantity.fromThousandths(limits.minimum),
                    where = where,
                ),
            )
        }
        val maximum = limits.maximum
        if (maximum != null && total > maximum) {
            result.add(
                CustomizationProblem.GroupAboveMaximum(
                    groupName = group.name,
                    maximum = PortionQuantity.fromThousandths(maximum),
                    where = where,
                ),
            )
        }
        for (chosen in GroupChoices.selection(value, binding)) {
            val option = group.options.firstOrNull { it.id == chosen.optionId } ?: continue
            val quantity = PortionQuantity.toThousandths(chosen.quantity)
            val optionMaximum = option.maxQuantity?.let { PortionQuantity.toThousandths(it) }
            if (optionMaximum != null && quantity > optionMaximum) {
                result.add(
                    CustomizationProblem.OptionAboveMaximum(
                        optionName = option.displayName,
                        maximum = PortionQuantity.fromThousandths(optionMaximum),
                        where = where,
                    ),
                )
            }
            // Una opción elegida que Go marcó como no disponible (también una incluida de origen
            // que se agotó) impide el pedido: Go la rechazaría igual.
            if (quantity > 0 && !option.selectable) {
                result.add(CustomizationProblem.OptionNotSelectable(option.displayName, where))
            }
        }
    }

    private fun checkSlot(
        catalog: Catalog,
        value: Customization,
        slot: ComboSlot,
        depth: Int,
        result: MutableList<CustomizationProblem>,
    ) {
        val selection = ComboChoices.selection(value, slot)
        if (selection.quantity !in slot.minQuantity..slot.maxQuantity) {
            result.add(
                CustomizationProblem.ComponentOutOfRange(
                    slotName = slot.name,
                    minimum = slot.minQuantity,
                    maximum = slot.maxQuantity,
                    where = slot.name,
                ),
            )
        }
        if (selection.quantity <= 0) {
            return
        }
        val component = catalog.product(selection.productId) ?: return
        val option = slot.option(selection.productId)
        // La disponibilidad del componente la da la opción de la ranura (Go la calcula en la
        // descarga) o, si no la trae, el propio producto.
        val unavailable =
            if (option != null) {
                !option.availableNow || option.soldOut
            } else {
                !component.availability.availableNow || component.availability.soldOut
            }
        if (unavailable) {
            result.add(
                CustomizationProblem.ProductNotSelectable(
                    productName = component.name,
                    status = component.availability.status,
                    reason = component.availability.reason,
                    where = slot.name,
                ),
            )
        }
        checkLevel(
            catalog,
            component,
            selection.variantId,
            selection.customization,
            slot.name,
            depth + 1,
            result,
        )
    }
}
