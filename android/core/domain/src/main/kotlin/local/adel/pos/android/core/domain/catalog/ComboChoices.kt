package local.adel.pos.android.core.domain.catalog

/**
 * Componentes de un combo (puerto de `customizationRules.ts` y `ComboComponentBlock` del
 * escritorio). Una ranura sin selección explícita conserva su producto y su cantidad originales.
 * Cambiar el producto de una ranura empieza con la configuración original del nuevo producto. Go
 * valida productos permitidos, cantidades y cargos al recibir.
 */
object ComboChoices {
    /** Máximo de niveles de combos dentro de combos que acepta Go. */
    const val MAXIMUM_DEPTH = 8

    /** Selección vigente de una ranura: la explícita o la original. */
    fun selection(value: Customization, slot: ComboSlot): ComponentSelection =
        value.components.firstOrNull { it.slotId == slot.id }
            ?: ComponentSelection(
                slotId = slot.id,
                productId = slot.defaultProductId,
                variantId = null,
                quantity = slot.defaultQuantity,
                customization = Customization.ORIGINAL,
            )

    /** Sustituye la selección de la ranura (en su lugar si ya era explícita). */
    fun withComponent(value: Customization, selection: ComponentSelection): Customization {
        val index = value.components.indexOfFirst { it.slotId == selection.slotId }
        val components =
            if (index >= 0) {
                value.components.toMutableList().also { it[index] = selection }
            } else {
                value.components + selection
            }
        return value.copy(components = components)
    }

    /** Toque en un producto de la ranura: lo pone con su configuración original. */
    fun chooseProduct(value: Customization, slot: ComboSlot, productId: String): Customization {
        val current = selection(value, slot)
        if (current.productId == productId || productId !in slot.productIds) {
            return value
        }
        return withComponent(
            value,
            current.copy(
                productId = productId,
                variantId = null,
                customization = Customization.ORIGINAL,
            ),
        )
    }

    /** Botones −/+ de la cantidad de una ranura, dentro de sus límites. */
    fun withQuantity(value: Customization, slot: ComboSlot, quantity: Int): Customization {
        if (quantity !in slot.minQuantity..slot.maxQuantity) {
            return value
        }
        return withComponent(value, selection(value, slot).copy(quantity = quantity))
    }

    fun withVariant(value: Customization, slot: ComboSlot, variantId: String?): Customization =
        withComponent(value, selection(value, slot).copy(variantId = variantId))

    /** Cambia la personalización del producto que ocupa la ranura. */
    fun withCustomization(
        value: Customization,
        slot: ComboSlot,
        customization: Customization,
    ): Customization =
        withComponent(value, selection(value, slot).copy(customization = customization))

    /** Si la cantidad de la ranura se puede cambiar (mínimo y máximo distintos). */
    fun quantityAdjustable(slot: ComboSlot): Boolean = slot.minQuantity != slot.maxQuantity
}
