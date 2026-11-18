package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.domain.catalog.ComponentSelection
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.GroupSelection
import com.adelpos.meseros.core.domain.catalog.IngredientSelection
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.catalog.OptionQuantity
import com.adelpos.meseros.core.domain.catalog.QuickNoteSelection
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/*
 * `Customization` y `LineRequest` del contrato v1, tal como los esperan `openAccount`, `saveDraft`,
 * `previewDraft` y `submitOfflineOrder` (`backend/internal/catalog/quote_models.go`). La Fase E
 * arma con ellos `DraftItem` y `OfflineItem` (agregando el id del renglón y el precio que vio).
 *
 * Forma al escribir (la misma del escritorio): `ingredients`, `groups`, `components` y `notes`
 * siempre presentes (listas vacías, no null), `variant_id` presente (null sin presentación) y
 * `quick_notes` solo si hay alguna (Go la omite vacía para que un borrador anterior a la etapa 8
 * siga igual). Al leer (un borrador que devuelve Go) se aceptan listas null u omitidas.
 */

@Serializable
data class CustomizationDto(
    val ingredients: List<IngredientSelectionDto>? = null,
    val groups: List<GroupSelectionDto>? = null,
    val components: List<ComponentSelectionDto>? = null,
    val notes: String? = null,
    @SerialName("quick_notes") val quickNotes: List<QuickNoteSelectionDto>? = null,
)

@Serializable
data class IngredientSelectionDto(
    @SerialName("ingredient_id") val ingredientId: String,
    val quantity: String,
)

@Serializable
data class GroupSelectionDto(
    @SerialName("group_id") val groupId: String,
    val options: List<OptionQuantityDto>? = null,
)

@Serializable
data class ComponentSelectionDto(
    @SerialName("slot_id") val slotId: String,
    @SerialName("product_id") val productId: String,
    @SerialName("variant_id") val variantId: String?,
    val quantity: Int,
    val customization: CustomizationDto? = null,
)

@Serializable
data class QuickNoteSelectionDto(@SerialName("note_id") val noteId: String, val text: String)

/** `LineRequest`: producto, presentación, cantidad y personalización de una partida. */
@Serializable
data class LineRequestDto(
    @SerialName("product_id") val productId: String,
    @SerialName("variant_id") val variantId: String?,
    val quantity: Int,
    val customization: CustomizationDto,
)

/**
 * JSON de las escrituras con personalización: escribe los null (`"variant_id": null`) y omite lo
 * que vale lo predeterminado (`quick_notes` vacía, que [toDto] deja en null). Ignora campos
 * desconocidos al leer, como [com.adelpos.meseros.core.data.api.ApiJson].
 */
val CustomizationJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = true
    encodeDefaults = false
}

/** La personalización del dominio en la forma del contrato. */
fun Customization.toDto(): CustomizationDto =
    CustomizationDto(
        ingredients = ingredients.map { IngredientSelectionDto(it.ingredientId, it.quantity) },
        groups =
            groups.map { group ->
                GroupSelectionDto(
                    groupId = group.groupId,
                    options = group.options.map { OptionQuantityDto(it.optionId, it.quantity) },
                )
            },
        components =
            components.map { component ->
                ComponentSelectionDto(
                    slotId = component.slotId,
                    productId = component.productId,
                    variantId = component.variantId,
                    quantity = component.quantity,
                    customization = component.customization.toDto(),
                )
            },
        notes = notes,
        quickNotes =
            if (quickNotes.isEmpty()) {
                null
            } else {
                quickNotes.map { QuickNoteSelectionDto(it.noteId, it.text) }
            },
    )

/** Una personalización que devolvió Go (un borrador guardado) en el dominio. */
fun CustomizationDto.toDomain(): Customization =
    Customization(
        ingredients =
            ingredients.orEmpty().map { IngredientSelection(it.ingredientId, it.quantity) },
        groups =
            groups.orEmpty().map { group ->
                GroupSelection(
                    groupId = group.groupId,
                    options =
                        group.options.orEmpty().map { OptionQuantity(it.optionId, it.quantity) },
                )
            },
        components =
            components.orEmpty().map { component ->
                ComponentSelection(
                    slotId = component.slotId,
                    productId = component.productId,
                    variantId = component.variantId,
                    quantity = component.quantity,
                    customization = component.customization?.toDomain() ?: Customization.ORIGINAL,
                )
            },
        notes = notes.orEmpty(),
        quickNotes = quickNotes.orEmpty().map { QuickNoteSelection(it.noteId, it.text) },
    )

/** La partida del personalizador como `LineRequest`. */
fun LineSelection.toLineRequest(): LineRequestDto =
    LineRequestDto(
        productId = productId,
        variantId = variantId,
        quantity = quantity,
        customization = customization.toDto(),
    )
