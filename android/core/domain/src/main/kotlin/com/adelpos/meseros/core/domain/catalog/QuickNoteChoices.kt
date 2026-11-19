package com.adelpos.meseros.core.domain.catalog

/**
 * Indicación rápida ofrecida al personalizar un producto.
 *
 * @property quantity cantidad del ingrediente que fija el efecto (nula sin efecto).
 * @property original el efecto deja el ingrediente en lo incluido («Con cebolla» en un producto que
 *   ya la lleva): se aplica como la configuración original, sin cantidad explícita.
 */
data class QuickNoteOption(
    val id: String,
    val text: String,
    val effect: QuickNoteEffect,
    val ingredientId: String?,
    val quantity: String?,
    val original: Boolean,
)

/** Cantidad que fija el efecto de una indicación y si equivale a la configuración original. */
data class QuickNoteTarget(val quantity: String, val original: Boolean)

/**
 * Indicaciones rápidas en la captura (etapa 8; puerto de `quickNoteRules.ts` del escritorio). Una
 * indicación con efecto no es un segundo mecanismo: al tocarla se aplica la misma selección del
 * personalizador (`ingredients`) y se guarda su identificador en `quick_notes`; su texto sigue en
 * `notes`. Go comprueba que ambas cosas coincidan y calcula el precio solo con los ingredientes.
 */
object QuickNoteChoices {
    /**
     * Cantidad que fija un efecto en la regla del producto, o null si no aplica. Misma regla que Go
     * (`catalog.QuickNoteTarget`) y que [IngredientChoices]:
     * - omit: el producto incluye el ingrediente y es removible → 0;
     * - normal: lo incluye → lo incluido; si no, agregable con máximo de al menos 1 → 1;
     * - extra: agregable con máximo mayor que lo incluido → mín(incluido + 1, máximo).
     */
    fun target(effect: QuickNoteEffect, rule: IngredientRule): QuickNoteTarget? {
        val included = PortionQuantity.toThousandths(rule.includedQuantity)
        val maximum = PortionQuantity.toThousandths(rule.maxQuantity)
        val unit = PortionQuantity.UNIT
        return when (effect) {
            QuickNoteEffect.OMIT ->
                if (included > 0 && rule.removable) {
                    QuickNoteTarget(quantity = "0", original = false)
                } else {
                    null
                }
            QuickNoteEffect.NORMAL ->
                when {
                    included > 0 ->
                        QuickNoteTarget(
                            quantity = PortionQuantity.fromThousandths(included),
                            original = true,
                        )
                    rule.addable && maximum >= unit ->
                        QuickNoteTarget(quantity = "1", original = false)
                    else -> null
                }
            QuickNoteEffect.EXTRA ->
                if (rule.addable && maximum > included) {
                    QuickNoteTarget(
                        quantity = PortionQuantity.fromThousandths(minOf(included + unit, maximum)),
                        original = false,
                    )
                } else {
                    null
                }
            QuickNoteEffect.NONE -> null
        }
    }

    /**
     * Indicaciones de la categoría que se ofrecen para un producto: todas las solo de instrucción y
     * las con efecto cuyo ingrediente está en las reglas del producto y les aplica.
     */
    fun applicable(notes: List<QuickNote>, rules: List<IngredientRule>): List<QuickNoteOption> {
        val options = mutableListOf<QuickNoteOption>()
        for (note in notes) {
            if (note.effect == QuickNoteEffect.NONE) {
                options.add(
                    QuickNoteOption(
                        id = note.id,
                        text = note.text,
                        effect = note.effect,
                        ingredientId = null,
                        quantity = null,
                        original = false,
                    ),
                )
                continue
            }
            val rule = rules.firstOrNull { it.ingredientId == note.ingredientId } ?: continue
            val target = target(note.effect, rule) ?: continue
            options.add(
                QuickNoteOption(
                    id = note.id,
                    text = note.text,
                    effect = note.effect,
                    ingredientId = rule.ingredientId,
                    quantity = target.quantity,
                    original = target.original,
                ),
            )
        }
        return options
    }

    /**
     * Una indicación solo de instrucción está elegida si su texto está en las observaciones; una
     * con efecto, solo si su identificador está elegido.
     */
    fun isSelected(value: Customization, option: QuickNoteOption): Boolean {
        if (option.effect == QuickNoteEffect.NONE) {
            return option.text in KitchenNotes.split(value.notes)
        }
        return value.quickNotes.any { it.noteId == option.id }
    }

    /**
     * Marca o desmarca una indicación. Marcar una con efecto quita la otra con efecto sobre el
     * mismo ingrediente (un solo efecto por ingrediente) y aplica la selección del personalizador.
     */
    fun toggle(
        value: Customization,
        option: QuickNoteOption,
        options: List<QuickNoteOption>,
    ): Customization =
        if (isSelected(value, option)) {
            withoutQuickNote(value, option)
        } else {
            withQuickNote(value, option, options)
        }

    fun withQuickNote(
        value: Customization,
        option: QuickNoteOption,
        options: List<QuickNoteOption>,
    ): Customization {
        var next = value
        val ingredientId = option.ingredientId
        if (option.effect != QuickNoteEffect.NONE && ingredientId != null) {
            for (other in options) {
                val sameIngredient = other.ingredientId == ingredientId
                if (
                    other.id != option.id &&
                        other.effect != QuickNoteEffect.NONE &&
                        sameIngredient &&
                        isSelected(next, other)
                ) {
                    next = withoutEntry(next, other.id, other.text)
                }
            }
            next =
                if (option.original) {
                    IngredientChoices.withoutChange(next, ingredientId)
                } else {
                    IngredientChoices.withQuantity(next, ingredientId, option.quantity ?: "0")
                }
        }
        val current = KitchenNotes.split(next.notes)
        if (option.text !in current) {
            next = next.copy(notes = KitchenNotes.join(current + option.text))
        }
        val chosen = next.quickNotes.filter { it.noteId != option.id }
        return next.copy(quickNotes = chosen + QuickNoteSelection(option.id, option.text))
    }

    /**
     * Desmarca una indicación: se quitan su identificador y su texto y, si tiene efecto, el
     * ingrediente vuelve a su configuración original.
     */
    fun withoutQuickNote(value: Customization, option: QuickNoteOption): Customization {
        val next = withoutEntry(value, option.id, option.text)
        val ingredientId = option.ingredientId
        if (option.effect != QuickNoteEffect.NONE && ingredientId != null) {
            return IngredientChoices.withoutChange(next, ingredientId)
        }
        return next
    }

    /**
     * Cambiar en el personalizador el ingrediente de una indicación con efecto la quita (con su
     * texto): la indicación ya no describiría la selección.
     */
    fun reconcile(
        value: Customization,
        options: List<QuickNoteOption>,
        rules: List<IngredientRule>,
    ): Customization {
        var next = value
        for (option in options) {
            val ingredientId = option.ingredientId
            if (
                option.effect == QuickNoteEffect.NONE ||
                    ingredientId == null ||
                    !isSelected(next, option)
            ) {
                continue
            }
            val rule = rules.firstOrNull { it.ingredientId == ingredientId } ?: continue
            val current = PortionQuantity.toThousandths(IngredientChoices.quantity(next, rule))
            if (current != PortionQuantity.toThousandths(option.quantity)) {
                next = withoutEntry(next, option.id, option.text)
            }
        }
        return next
    }

    /**
     * Quita un texto de las observaciones y cualquier indicación elegida con ese texto o
     * identificador, sin tocar ingredientes.
     */
    internal fun withoutEntry(value: Customization, noteId: String, text: String): Customization {
        val notes = KitchenNotes.join(KitchenNotes.split(value.notes).filter { it != text })
        val chosen = value.quickNotes.filter { it.noteId != noteId && it.text != text }
        return value.copy(notes = notes, quickNotes = chosen)
    }
}
