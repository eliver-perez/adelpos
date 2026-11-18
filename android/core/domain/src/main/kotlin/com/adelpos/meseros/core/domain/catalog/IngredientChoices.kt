package com.adelpos.meseros.core.domain.catalog

/** Porción de un ingrediente que se muestra en el personalizador con los botones −/+. */
enum class IngredientLevel {
    /** «Sin»: cantidad 0. */
    WITHOUT,

    /** «Normal»: la cantidad incluida en el producto. */
    NORMAL,

    /** «Extra»: más de lo incluido (una porción más, hasta el máximo). */
    EXTRA,
}

/**
 * Ingredientes de un producto con «Sin / Normal / Extra» y los botones −/+ (porciones del sistema
 * anterior). Solo construyen la selección: los mismos niveles que los botones «Sin» y «Extra» del
 * escritorio (`CustomizationBlocks.tsx`) y que `catalog.QuickNoteTarget` de Go:
 * - «Sin» = 0, solo si el producto lo incluye y es removible;
 * - «Normal» = lo incluido (sin cantidad explícita: la configuración original);
 * - «Extra» = mín(incluido + 1, máximo), solo si es agregable y su máximo supera lo incluido.
 *
 * Go vuelve a validar cantidades y cobra el extra al recibir el pedido.
 */
object IngredientChoices {
    /** Ingredientes que el mesero puede cambiar (removibles o agregables). */
    fun editable(product: CatalogProduct): List<IngredientRule> =
        product.ingredients.filter { levels(it).size > 1 }

    /** Niveles posibles de un ingrediente en orden (de menos a más). */
    fun levels(rule: IngredientRule): List<IngredientLevel> {
        val included = PortionQuantity.toThousandths(rule.includedQuantity)
        val maximum = PortionQuantity.toThousandths(rule.maxQuantity)
        val result = mutableListOf<IngredientLevel>()
        if (rule.removable && included > 0) {
            result.add(IngredientLevel.WITHOUT)
        }
        result.add(IngredientLevel.NORMAL)
        if (rule.addable && maximum > included) {
            result.add(IngredientLevel.EXTRA)
        }
        return result
    }

    /** Cantidad que pide el mesero: la explícita o la incluida. */
    fun quantity(value: Customization, rule: IngredientRule): String =
        value.ingredients.firstOrNull { it.ingredientId == rule.ingredientId }?.quantity
            ?: rule.includedQuantity

    /** Nivel que corresponde a la cantidad pedida (para el texto «Sin», «Normal» o «Extra»). */
    fun level(value: Customization, rule: IngredientRule): IngredientLevel {
        val requested = PortionQuantity.toThousandths(quantity(value, rule))
        val included = PortionQuantity.toThousandths(rule.includedQuantity)
        return when {
            requested == 0 && included > 0 -> IngredientLevel.WITHOUT
            requested > included -> IngredientLevel.EXTRA
            else -> IngredientLevel.NORMAL
        }
    }

    /** Pone el ingrediente en [level]. «Normal» quita la cantidad explícita. */
    fun withLevel(
        value: Customization,
        rule: IngredientRule,
        level: IngredientLevel,
    ): Customization {
        if (level !in levels(rule)) {
            return value
        }
        return when (level) {
            IngredientLevel.NORMAL -> withoutChange(value, rule.ingredientId)
            IngredientLevel.WITHOUT -> withQuantity(value, rule.ingredientId, "0")
            IngredientLevel.EXTRA -> withQuantity(value, rule.ingredientId, extraQuantity(rule))
        }
    }

    /** Botón «+»: el nivel siguiente, si existe. */
    fun increase(value: Customization, rule: IngredientRule): Customization = step(value, rule, +1)

    /** Botón «−»: el nivel anterior, si existe. */
    fun decrease(value: Customization, rule: IngredientRule): Customization = step(value, rule, -1)

    fun canIncrease(value: Customization, rule: IngredientRule): Boolean {
        val options = levels(rule)
        return options.indexOf(level(value, rule)) < options.lastIndex
    }

    fun canDecrease(value: Customization, rule: IngredientRule): Boolean =
        levels(rule).indexOf(level(value, rule)) > 0

    /** Cantidad de «Extra»: mín(incluido + 1 porción, máximo), como el botón del escritorio. */
    fun extraQuantity(rule: IngredientRule): String {
        val included = PortionQuantity.toThousandths(rule.includedQuantity)
        val maximum = PortionQuantity.toThousandths(rule.maxQuantity)
        return PortionQuantity.fromThousandths(minOf(included + PortionQuantity.UNIT, maximum))
    }

    fun withQuantity(value: Customization, ingredientId: String, quantity: String): Customization {
        val selection = IngredientSelection(ingredientId, quantity)
        val index = value.ingredients.indexOfFirst { it.ingredientId == ingredientId }
        val ingredients =
            if (index >= 0) {
                value.ingredients.toMutableList().also { it[index] = selection }
            } else {
                value.ingredients + selection
            }
        return value.copy(ingredients = ingredients)
    }

    /** Sin cantidad explícita: el ingrediente vuelve a lo incluido. */
    fun withoutChange(value: Customization, ingredientId: String): Customization =
        value.copy(ingredients = value.ingredients.filter { it.ingredientId != ingredientId })

    private fun step(value: Customization, rule: IngredientRule, direction: Int): Customization {
        val options = levels(rule)
        val target = options.indexOf(level(value, rule)) + direction
        if (target !in options.indices) {
            return value
        }
        return withLevel(value, rule, options[target])
    }
}
