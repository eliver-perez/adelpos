package com.adelpos.meseros.core.domain.catalog

/** Límites efectivos de un grupo en un producto, en milésimas ([maximum] nulo = sin máximo). */
data class GroupLimits(val minimum: Int, val maximum: Int?) {
    /** Con máximo de una porción el grupo es una elección única: un toque sustituye. */
    val singleChoice: Boolean
        get() = maximum == PortionQuantity.UNIT

    val required: Boolean
        get() = minimum > 0
}

/** Resultado de intentar sumar una porción a una opción de un grupo. */
sealed interface PortionChange {
    data class Applied(val value: Customization) : PortionChange

    /** El grupo ya tiene su máximo de porciones: hay que quitar una de otra opción. */
    data class GroupFull(val maximum: String) : PortionChange

    /** La opción ya tiene su máximo de porciones. */
    data class OptionFull(val maximum: String) : PortionChange

    /** La opción no se puede elegir (inactiva, agotada o no disponible según Go). */
    data object Unavailable : PortionChange
}

/**
 * Grupos de complementos y guarniciones (puerto de `customizationRules.ts` y de `GroupBlock` del
 * escritorio). Un grupo omitido conserva su configuración original (lo incluido del producto); una
 * selección explícita, aunque esté vacía, la sustituye. Con máximo de una porción es una elección
 * única; con varias, cada toque suma una porción y «−» resta una. Go valida límites y precios al
 * recibir.
 */
object GroupChoices {
    /** Límites del producto si los tiene; si no, los del grupo reutilizable. */
    fun limits(binding: GroupBinding, group: OptionGroup): GroupLimits {
        val minimum = PortionQuantity.toThousandths(binding.minQuantity ?: group.minQuantity)
        val maximumText = binding.maxQuantity ?: group.maxQuantity
        val maximum = maximumText?.let { PortionQuantity.toThousandths(it) }
        return GroupLimits(minimum, maximum)
    }

    /** Selección vigente del grupo: la explícita o lo incluido de origen. */
    fun selection(value: Customization, binding: GroupBinding): List<OptionQuantity> =
        value.groups.firstOrNull { it.groupId == binding.groupId }?.options ?: binding.included

    /** Porciones elegidas de una opción, en milésimas. */
    fun quantityOf(value: Customization, binding: GroupBinding, optionId: String): Int =
        PortionQuantity.toThousandths(
            selection(value, binding).firstOrNull { it.optionId == optionId }?.quantity,
        )

    /** Total de porciones elegidas en el grupo, en milésimas. */
    fun total(value: Customization, binding: GroupBinding): Int =
        selection(value, binding).sumOf { PortionQuantity.toThousandths(it.quantity) }

    /** Total de porciones incluidas de origen, en milésimas. */
    fun includedTotal(binding: GroupBinding): Int =
        binding.included.sumOf { PortionQuantity.toThousandths(it.quantity) }

    /** Sustituye la selección del grupo (en su lugar si ya era explícita). */
    fun withSelection(
        value: Customization,
        groupId: String,
        options: List<OptionQuantity>,
    ): Customization {
        val selection = GroupSelection(groupId, options)
        val index = value.groups.indexOfFirst { it.groupId == groupId }
        val groups =
            if (index >= 0) {
                value.groups.toMutableList().also { it[index] = selection }
            } else {
                value.groups + selection
            }
        return value.copy(groups = groups)
    }

    /**
     * Toque en una opción de un grupo de elección única: la elige sola. Tocar la elegida la quita
     * solo si el grupo es opcional (una elección obligatoria no se deja vacía con un toque).
     */
    fun chooseSingle(
        value: Customization,
        binding: GroupBinding,
        group: OptionGroup,
        option: GroupOption,
    ): Customization {
        val limits = limits(binding, group)
        if (quantityOf(value, binding, option.id) > 0) {
            if (limits.required) {
                return value
            }
            return withSelection(value, binding.groupId, emptyList())
        }
        if (!option.selectable) {
            return value
        }
        return withSelection(
            value,
            binding.groupId,
            listOf(
                OptionQuantity(option.id, PortionQuantity.fromThousandths(PortionQuantity.UNIT)),
            ),
        )
    }

    /**
     * Botón «+» o toque en la tarjeta de una opción de un grupo de varias porciones: suma una
     * porción si cabe en el máximo del grupo y en el de la opción; si no, dice por qué.
     */
    fun addPortion(
        value: Customization,
        binding: GroupBinding,
        group: OptionGroup,
        option: GroupOption,
    ): PortionChange {
        if (!option.selectable) {
            return PortionChange.Unavailable
        }
        val limits = limits(binding, group)
        val current = quantityOf(value, binding, option.id)
        val maximum = limits.maximum
        if (maximum != null && total(value, binding) + PortionQuantity.UNIT > maximum) {
            return PortionChange.GroupFull(PortionQuantity.fromThousandths(maximum))
        }
        val optionMaximum = option.maxQuantity?.let { PortionQuantity.toThousandths(it) }
        if (optionMaximum != null && current + PortionQuantity.UNIT > optionMaximum) {
            return PortionChange.OptionFull(PortionQuantity.fromThousandths(optionMaximum))
        }
        return PortionChange.Applied(
            withOptionQuantity(value, binding, option.id, current + PortionQuantity.UNIT),
        )
    }

    /** Botón «−»: resta una porción; en cero la opción sale de la selección. */
    fun removePortion(
        value: Customization,
        binding: GroupBinding,
        optionId: String,
    ): Customization {
        val current = quantityOf(value, binding, optionId)
        if (current <= 0) {
            return value
        }
        return withOptionQuantity(value, binding, optionId, current - PortionQuantity.UNIT)
    }

    /** «Sin …»: selección explícita vacía (solo tiene sentido en un grupo opcional). */
    fun clear(value: Customization, groupId: String): Customization =
        withSelection(value, groupId, emptyList())

    /**
     * Cambia la cantidad de una opción y conserva las demás de la selección vigente (explícita u
     * original). Una cantidad de cero o menos quita la opción.
     */
    private fun withOptionQuantity(
        value: Customization,
        binding: GroupBinding,
        optionId: String,
        thousandths: Int,
    ): Customization {
        val current = selection(value, binding)
        val index = current.indexOfFirst { it.optionId == optionId }
        val next = current.toMutableList()
        if (thousandths <= 0) {
            if (index >= 0) {
                next.removeAt(index)
            }
        } else {
            val entry = OptionQuantity(optionId, PortionQuantity.fromThousandths(thousandths))
            if (index >= 0) {
                next[index] = entry
            } else {
                next.add(entry)
            }
        }
        return withSelection(value, binding.groupId, next)
    }
}
