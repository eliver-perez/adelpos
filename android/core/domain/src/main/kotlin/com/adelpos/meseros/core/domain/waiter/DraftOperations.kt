package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.catalog.LineSelection

/**
 * Cambio local sobre el borrador de una cuenta (puerto de `draftOperations.ts` del escritorio).
 * Cada toque se registra como una operación y el borrador visible se obtiene aplicándolas, en
 * orden, sobre el último borrador que Go aceptó. Así, si otra terminal cambia la cuenta, los mismos
 * toques pueden volver a aplicarse sobre el borrador actual («Conservar mis cambios») sin
 * sobrescribir lo que hizo la otra terminal.
 */
sealed interface DraftOperation {
    /** Agrega un renglón nuevo (con su identificador ya generado). */
    data class Add(val item: DraftLine) : DraftOperation

    /**
     * Suma [delta] unidades al renglón [id]. [fallback] es el renglón que se vuelve a crear si, al
     * aplicarse, el original ya no existe (por ejemplo, porque se envió a cocina mientras se tocaba
     * «+»).
     */
    data class Quantity(val id: String, val delta: Int, val fallback: DraftLine) : DraftOperation

    /** Sustituye el contenido del renglón [id] (personalización o cantidad elegidas de nuevo). */
    data class Replace(val id: String, val item: DraftLine) : DraftOperation

    data class Remove(val id: String) : DraftOperation
}

/** Aplicación de las operaciones locales y las reglas para agregar productos. */
object DraftOperations {
    fun apply(base: List<DraftLine>, operations: List<DraftOperation>): List<DraftLine> =
        operations.fold(base, ::applyOne)

    private fun applyOne(items: List<DraftLine>, operation: DraftOperation): List<DraftLine> =
        when (operation) {
            is DraftOperation.Add -> applyAdd(items, operation)
            is DraftOperation.Quantity -> applyQuantity(items, operation)
            is DraftOperation.Replace -> applyReplace(items, operation)
            is DraftOperation.Remove -> items.filter { it.id != operation.id }
        }

    private fun applyAdd(items: List<DraftLine>, operation: DraftOperation.Add): List<DraftLine> {
        if (items.any { it.id == operation.item.id }) {
            return items
        }
        return items + operation.item
    }

    private fun applyQuantity(
        items: List<DraftLine>,
        operation: DraftOperation.Quantity,
    ): List<DraftLine> {
        val target = items.firstOrNull { it.id == operation.id }
        if (target == null) {
            // El renglón ya no está: solo un «+» vuelve a crearlo (con su respaldo y solo las
            // unidades que se sumaron).
            val alreadyThere = items.any { it.id == operation.fallback.id }
            if (operation.delta <= 0 || alreadyThere) {
                return items
            }
            return items + operation.fallback.withQuantity(operation.delta)
        }
        val quantity = target.line.quantity + operation.delta
        if (quantity <= 0) {
            return items.filter { it.id != operation.id }
        }
        val limited = minOf(quantity, LineSelection.MAXIMUM_LINE_QUANTITY)
        return items.map { item ->
            if (item.id == operation.id) {
                item.withQuantity(limited)
            } else {
                item
            }
        }
    }

    private fun applyReplace(
        items: List<DraftLine>,
        operation: DraftOperation.Replace,
    ): List<DraftLine> {
        val replacement = operation.item.copy(id = operation.id)
        if (items.none { it.id == operation.id }) {
            return items + replacement
        }
        return items.map { item ->
            if (item.id == operation.id) {
                replacement
            } else {
                item
            }
        }
    }

    /**
     * Agregar un producto: si ya hay un renglón sencillo idéntico (mismo producto y presentación,
     * sin personalización ni indicaciones) se suma a su cantidad; si no, se crea otro renglón. Un
     * renglón personalizado nunca se combina con otro (regla del escritorio).
     */
    fun addLine(
        current: List<DraftLine>,
        line: LineSelection,
        newId: () -> String,
    ): DraftOperation {
        if (line.customization.isOriginal) {
            val existing = current.firstOrNull { item ->
                item.line.customization.isOriginal &&
                    item.line.productId == line.productId &&
                    item.line.variantId == line.variantId
            }
            if (existing != null) {
                return DraftOperation.Quantity(
                    id = existing.id,
                    delta = line.quantity,
                    fallback = existing.copy(id = newId()),
                )
            }
        }
        return DraftOperation.Add(DraftLine(newId(), line))
    }

    /** «+» o «−» sobre un renglón. */
    fun changeQuantity(item: DraftLine, delta: Int, newId: () -> String): DraftOperation =
        DraftOperation.Quantity(id = item.id, delta = delta, fallback = item.copy(id = newId()))
}
