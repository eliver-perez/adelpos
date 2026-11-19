package com.adelpos.meseros.core.domain.catalog

/**
 * Lo que se eligió en una partida, para mostrarlo debajo de su nombre en la cuenta o el pedido
 * mientras Go no devuelve su desglose (como `describeSelection` del escritorio): presentación,
 * ingredientes cambiados («Sin cebolla», «Extra tocino»), lo elegido en cada grupo, los componentes
 * del combo y las observaciones. Solo describe lo que cambió respecto de la configuración original
 * y nunca muestra precios.
 */
object LineDescription {
    fun parts(catalog: Catalog, line: LineSelection): List<String> {
        val product = catalog.product(line.productId) ?: return emptyList()
        return levelParts(catalog, product, line.variantId, line.customization, 0)
    }

    private fun levelParts(
        catalog: Catalog,
        product: CatalogProduct,
        variantId: String?,
        value: Customization,
        depth: Int,
    ): List<String> {
        if (depth > ComboChoices.MAXIMUM_DEPTH) {
            return emptyList()
        }
        val parts = mutableListOf<String>()
        product.variants.firstOrNull { it.id == variantId }?.let { parts.add(it.name) }
        parts.addAll(ingredientParts(catalog, product, value))
        parts.addAll(groupParts(catalog, product, value))
        for (slot in product.slots) {
            parts.addAll(componentParts(catalog, slot, value, depth))
        }
        val notes = value.notes.trim()
        if (notes.isNotEmpty()) {
            parts.add(notes)
        }
        return parts
    }

    // Un ingrediente que ya describe una indicación con efecto elegida (su texto está en las
    // observaciones) no se repite: «Sin cebolla» aparece una sola vez, como en la comanda.
    private fun ingredientParts(
        catalog: Catalog,
        product: CatalogProduct,
        value: Customization,
    ): List<String> {
        val parts = mutableListOf<String>()
        val covered = coveredIngredients(catalog, product, value)
        for (rule in product.ingredients) {
            if (rule.ingredientId in covered) {
                continue
            }
            when (IngredientChoices.level(value, rule)) {
                IngredientLevel.WITHOUT -> parts.add("Sin ${rule.name.lowercase()}")
                IngredientLevel.EXTRA -> parts.add("Extra ${rule.name.lowercase()}")
                IngredientLevel.NORMAL -> Unit
            }
        }
        return parts
    }

    private fun coveredIngredients(
        catalog: Catalog,
        product: CatalogProduct,
        value: Customization,
    ): Set<String> {
        val notes = KitchenNotes.split(value.notes)
        val chosen = value.quickNotes.map { it.noteId }.toSet()
        return catalog
            .quickNotesFor(product)
            .filter { note ->
                note.effect != QuickNoteEffect.NONE && note.id in chosen && note.text in notes
            }
            .mapNotNull { it.ingredientId }
            .toSet()
    }

    private fun groupParts(
        catalog: Catalog,
        product: CatalogProduct,
        value: Customization,
    ): List<String> {
        val parts = mutableListOf<String>()
        for (binding in product.groups) {
            val group = catalog.group(binding.groupId) ?: continue
            val explicit = value.groups.firstOrNull { it.groupId == binding.groupId } ?: continue
            val chosen = explicit.options.filter { PortionQuantity.toThousandths(it.quantity) > 0 }
            if (chosen.isEmpty()) {
                parts.add("Sin ${group.name.lowercase()}")
                continue
            }
            for (selection in chosen) {
                val option = group.options.firstOrNull { it.id == selection.optionId } ?: continue
                val quantity = PortionQuantity.display(selection.quantity)
                if (quantity == "1") {
                    parts.add(option.displayName)
                } else {
                    parts.add("$quantity × ${option.displayName}")
                }
            }
        }
        return parts
    }

    private fun componentParts(
        catalog: Catalog,
        slot: ComboSlot,
        value: Customization,
        depth: Int,
    ): List<String> {
        val explicit = value.components.firstOrNull { it.slotId == slot.id } ?: return emptyList()
        val component = catalog.product(explicit.productId) ?: return emptyList()
        if (explicit.quantity <= 0) {
            return listOf("Sin ${slot.name.lowercase()}")
        }
        val nested =
            levelParts(catalog, component, explicit.variantId, explicit.customization, depth + 1)
        val label =
            if (explicit.quantity == 1) {
                component.name
            } else {
                "${explicit.quantity} × ${component.name}"
            }
        return if (nested.isEmpty()) {
            listOf(label)
        } else {
            listOf("$label (${nested.joinToString(", ")})")
        }
    }
}
