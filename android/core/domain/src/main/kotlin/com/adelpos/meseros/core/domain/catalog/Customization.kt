package com.adelpos.meseros.core.domain.catalog

/**
 * Personalización de una partida, con la misma forma que `Customization` del contrato v1
 * (`backend/internal/catalog/quote_models.go`): la que esperan `saveDraft`, `openAccount`,
 * `previewDraft` y `submitOfflineOrder`. La aplicación solo construye la selección; Go la valida y
 * calcula el precio al recibirla.
 *
 * - [ingredients]: cantidades **explícitas** de ingredientes (un ingrediente que no aparece
 *   conserva su cantidad incluida).
 * - [groups]: selecciones **explícitas** de grupos (un grupo que no aparece conserva su
 *   configuración original; una selección vacía la sustituye por «nada»).
 * - [components]: componentes de combo elegidos (una ranura que no aparece conserva su producto y
 *   cantidad originales).
 * - [notes]: observaciones para cocina (texto libre y textos de las indicaciones, separados por
 *   comas), hasta 500 bytes.
 * - [quickNotes]: indicaciones rápidas elegidas (identificador y texto que vio el mesero).
 */
data class Customization(
    val ingredients: List<IngredientSelection> = emptyList(),
    val groups: List<GroupSelection> = emptyList(),
    val components: List<ComponentSelection> = emptyList(),
    val notes: String = "",
    val quickNotes: List<QuickNoteSelection> = emptyList(),
) {
    /** Sin ningún cambio respecto de la configuración original del producto. */
    val isOriginal: Boolean
        get() =
            ingredients.isEmpty() &&
                groups.isEmpty() &&
                components.isEmpty() &&
                notes.isBlank() &&
                quickNotes.isEmpty()

    companion object {
        /** La configuración original del producto (nada explícito). */
        val ORIGINAL = Customization()
    }
}

/** Cantidad explícita de un ingrediente del producto. */
data class IngredientSelection(val ingredientId: String, val quantity: String)

/** Selección explícita de un grupo: las opciones con su cantidad (puede estar vacía). */
data class GroupSelection(val groupId: String, val options: List<OptionQuantity>)

/**
 * Componente de un combo: el producto que ocupa la ranura, su presentación, cuántas unidades y su
 * propia personalización (un combo puede tener combos, hasta ocho niveles).
 */
data class ComponentSelection(
    val slotId: String,
    val productId: String,
    val variantId: String?,
    val quantity: Int,
    val customization: Customization,
)

/** Indicación rápida elegida: su identificador y el texto que vio el mesero. */
data class QuickNoteSelection(val noteId: String, val text: String)

/**
 * Lo que produce el personalizador para una partida: producto, presentación, cantidad y
 * personalización. Es exactamente `LineRequest` del contrato (y, con un identificador, `DraftItem`
 * u `OfflineItem`). Sin precios: el importe lo calcula Go.
 */
data class LineSelection(
    val productId: String,
    val variantId: String?,
    val quantity: Int,
    val customization: Customization,
) {
    init {
        require(quantity in 1..MAXIMUM_LINE_QUANTITY) { "La cantidad debe estar entre 1 y 1000" }
    }

    companion object {
        /** Máximo de unidades de una partida que acepta Go (`LineRequest.quantity`). */
        const val MAXIMUM_LINE_QUANTITY = 1000
    }
}
