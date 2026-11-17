package local.adel.pos.android.core.ui.catalog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogProduct
import local.adel.pos.android.core.domain.catalog.ComboChoices
import local.adel.pos.android.core.domain.catalog.ComboSlot
import local.adel.pos.android.core.domain.catalog.Customization
import local.adel.pos.android.core.domain.catalog.GroupBinding
import local.adel.pos.android.core.domain.catalog.GroupChoices
import local.adel.pos.android.core.domain.catalog.GroupLimits
import local.adel.pos.android.core.domain.catalog.GroupOption
import local.adel.pos.android.core.domain.catalog.GroupPricingMode
import local.adel.pos.android.core.domain.catalog.IngredientChoices
import local.adel.pos.android.core.domain.catalog.IngredientLevel
import local.adel.pos.android.core.domain.catalog.KitchenNotes
import local.adel.pos.android.core.domain.catalog.OptionGroup
import local.adel.pos.android.core.domain.catalog.PortionChange
import local.adel.pos.android.core.domain.catalog.PortionQuantity
import local.adel.pos.android.core.domain.catalog.QuickNoteChoices
import local.adel.pos.android.core.domain.catalog.QuickNoteOption
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.money.MoneyFormat
import local.adel.pos.android.core.ui.R
import local.adel.pos.android.core.ui.components.SecondaryButton
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.core.ui.theme.AdelTheme

/*
 * Bloques del personalizador táctil (puerto de `TouchCustomizer` y `CustomizationBlocks` del
 * escritorio). Cada bloque solo construye la selección con las reglas de `core:domain`; los precios
 * que se ven son los de referencia que entregó Go, sin sumar nada.
 */

/**
 * Un nivel del personalizador: el producto principal o el producto de una ranura de combo (que
 * puede ser otro combo, hasta [ComboChoices.MAXIMUM_DEPTH] niveles).
 */
@Composable
internal fun CustomizationLevel(
    catalog: Catalog,
    product: CatalogProduct,
    variantId: String?,
    value: Customization,
    onVariantChange: (String?) -> Unit,
    onValueChange: (Customization) -> Unit,
    depth: Int,
) {
    if (depth > ComboChoices.MAXIMUM_DEPTH) {
        return
    }
    Column(verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
        VariantBlock(product, variantId, onVariantChange)
        for (binding in product.groups) {
            val group = catalog.group(binding.groupId) ?: continue
            GroupBlock(binding, group, value, onValueChange)
        }
        val quickNotes =
            QuickNoteChoices.applicable(catalog.quickNotesFor(product), product.ingredients)
        IngredientBlock(
            product = product,
            value = value,
            // Cambiar un ingrediente quita la indicación con efecto que ya no lo describe.
            onValueChange = { next ->
                onValueChange(QuickNoteChoices.reconcile(next, quickNotes, product.ingredients))
            },
        )
        for (slot in product.slots) {
            ComboSlotBlock(catalog, slot, value, onValueChange, depth)
        }
        KitchenNotesBlock(product, value, onValueChange, quickNotes)
    }
}

@Composable
private fun BlockHeader(title: String, rule: String?, counter: String? = null) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.weight(1f).semantics { heading() },
        )
        if (counter != null) {
            Text(counter, style = MaterialTheme.typography.labelLarge)
        }
        if (rule != null) {
            Text(
                text = rule,
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun VariantBlock(
    product: CatalogProduct,
    variantId: String?,
    onVariantChange: (String?) -> Unit,
) {
    val variants = product.activeVariants
    if (variants.isEmpty()) {
        return
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        BlockHeader(
            title = stringResource(R.string.customizer_variant),
            rule =
                if (product.requiresVariant) stringResource(R.string.customizer_required_one)
                else stringResource(R.string.customizer_optional),
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (!product.requiresVariant) {
                ChoiceChip(
                    label = stringResource(R.string.customizer_variant_base),
                    detail = product.referencePrice?.display(),
                    selected = variantId == null,
                    enabled = true,
                    onClick = { onVariantChange(null) },
                    testTag = variantTag(product.id, null),
                )
            }
            for (variant in variants) {
                ChoiceChip(
                    label = variant.name,
                    detail = variant.referencePrice?.display(),
                    selected = variantId == variant.id,
                    enabled = true,
                    onClick = { onVariantChange(variant.id) },
                    testTag = variantTag(product.id, variant.id),
                )
            }
        }
    }
}

/** Aviso de un grupo al intentar pasar de un máximo. */
private sealed interface PortionWarning {
    data class GroupFull(val maximum: String) : PortionWarning

    data class OptionFull(val optionName: String, val maximum: String) : PortionWarning
}

/**
 * Grupo de complementos. Con máximo de una porción, un toque elige (y sustituye); con varias, cada
 * opción tiene −/+ y el encabezado dice cuántas van de las incluidas. Al intentar pasar de un
 * máximo aparece un aviso en el propio grupo en lugar de desactivar el botón sin explicación.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun GroupBlock(
    binding: GroupBinding,
    group: OptionGroup,
    value: Customization,
    onValueChange: (Customization) -> Unit,
) {
    var warning by remember(group.id) { mutableStateOf<PortionWarning?>(null) }
    val limits = GroupChoices.limits(binding, group)
    val total = GroupChoices.total(value, binding)
    val included = GroupChoices.includedTotal(binding)
    val counter =
        if (included > 0) {
            stringResource(
                R.string.customizer_group_counter,
                PortionQuantity.fromThousandths(total),
                PortionQuantity.fromThousandths(included),
            )
        } else {
            stringResource(R.string.customizer_group_chosen, PortionQuantity.fromThousandths(total))
        }
    Column(
        modifier = Modifier.testTag(groupTag(group.id)),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        BlockHeader(title = group.name, rule = groupRule(limits), counter = counter)
        if (limits.singleChoice) {
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                for (option in group.options) {
                    val selected = GroupChoices.quantityOf(value, binding, option.id) > 0
                    ChoiceChip(
                        label = option.displayName,
                        detail = optionDetail(group, option) ?: unavailableText(option),
                        selected = selected,
                        enabled = selected || option.selectable,
                        onClick = {
                            onValueChange(GroupChoices.chooseSingle(value, binding, group, option))
                        },
                        testTag = optionTag(option.id),
                    )
                }
            }
        } else {
            for (option in group.options) {
                PortionRow(
                    group = group,
                    option = option,
                    quantity = GroupChoices.quantityOf(value, binding, option.id),
                    onLess = {
                        warning = null
                        onValueChange(GroupChoices.removePortion(value, binding, option.id))
                    },
                    onMore = {
                        when (val change = GroupChoices.addPortion(value, binding, group, option)) {
                            is PortionChange.Applied -> {
                                warning = null
                                onValueChange(change.value)
                            }
                            is PortionChange.GroupFull ->
                                warning = PortionWarning.GroupFull(change.maximum)
                            is PortionChange.OptionFull ->
                                warning =
                                    PortionWarning.OptionFull(option.displayName, change.maximum)
                            PortionChange.Unavailable -> Unit
                        }
                    },
                )
            }
            if (!limits.required && total > 0) {
                TextButton(
                    onClick = {
                        warning = null
                        onValueChange(GroupChoices.clear(value, group.id))
                    },
                    modifier =
                        Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                            .testTag(groupTag(group.id) + "-sin"),
                ) {
                    Text(stringResource(R.string.customizer_group_clear, group.name.lowercase()))
                }
            }
        }
        val message =
            when (val current = warning) {
                is PortionWarning.GroupFull ->
                    stringResource(
                        R.string.customizer_group_full,
                        group.name,
                        portionsText(current.maximum),
                    )
                is PortionWarning.OptionFull ->
                    stringResource(
                        R.string.customizer_option_full,
                        current.optionName,
                        portionsText(current.maximum),
                    )
                null -> null
            }
        if (message != null) {
            Text(
                text = message,
                style = MaterialTheme.typography.bodyMedium,
                color = AdelTheme.colors.red,
                modifier = Modifier.testTag(TAG_GROUP_WARNING),
            )
        }
        HorizontalDivider()
    }
}

/** «1 porción», «3 porciones», «1.5 porciones». */
@Composable
private fun portionsText(quantity: String): String =
    if (quantity == "1") {
        stringResource(R.string.customizer_one_portion)
    } else {
        stringResource(R.string.customizer_portions, quantity)
    }

@Composable
private fun PortionRow(
    group: OptionGroup,
    option: GroupOption,
    quantity: Int,
    onLess: () -> Unit,
    onMore: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().heightIn(min = AdelDimens.MinTouchTarget),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(option.displayName, style = MaterialTheme.typography.bodyLarge)
            val detail = optionDetail(group, option) ?: unavailableText(option)
            if (detail != null) {
                Text(
                    detail,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Stepper(
            value = PortionQuantity.fromThousandths(quantity),
            onDecrease = onLess,
            onIncrease = onMore,
            canDecrease = quantity > 0,
            canIncrease = option.selectable,
            decreaseLabel = stringResource(R.string.customizer_portion_less, option.displayName),
            increaseLabel = stringResource(R.string.customizer_portion_more, option.displayName),
            testTag = optionTag(option.id),
        )
    }
}

/** Ingredientes con «Sin / Normal / Extra» y −/+ (las porciones del sistema anterior). */
@Composable
private fun IngredientBlock(
    product: CatalogProduct,
    value: Customization,
    onValueChange: (Customization) -> Unit,
) {
    val editable = IngredientChoices.editable(product)
    if (editable.isEmpty()) {
        return
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        BlockHeader(
            title = stringResource(R.string.customizer_ingredients),
            rule = stringResource(R.string.customizer_optional),
        )
        for (rule in editable) {
            Row(
                modifier =
                    Modifier.fillMaxWidth()
                        .heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag(ingredientTag(rule.ingredientId)),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(rule.name, style = MaterialTheme.typography.bodyLarge)
                    if (
                        IngredientLevel.EXTRA in IngredientChoices.levels(rule) &&
                            positiveAmount(rule.extraPrice)
                    ) {
                        Text(
                            text =
                                stringResource(
                                    R.string.customizer_ingredient_extra_price,
                                    MoneyFormat.formatSigned(rule.extraPrice.decimal),
                                ),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Stepper(
                    value = levelText(IngredientChoices.level(value, rule)),
                    onDecrease = { onValueChange(IngredientChoices.decrease(value, rule)) },
                    onIncrease = { onValueChange(IngredientChoices.increase(value, rule)) },
                    canDecrease = IngredientChoices.canDecrease(value, rule),
                    canIncrease = IngredientChoices.canIncrease(value, rule),
                    decreaseLabel = stringResource(R.string.customizer_less, rule.name.lowercase()),
                    increaseLabel = stringResource(R.string.customizer_more, rule.name.lowercase()),
                    testTag = ingredientTag(rule.ingredientId),
                )
            }
        }
        HorizontalDivider()
    }
}

/**
 * Ranura de un combo: el producto que la ocupa (el original y sus alternativas), su cantidad si se
 * puede variar y, al tocar «Personalizar …», la personalización de ese producto.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ComboSlotBlock(
    catalog: Catalog,
    slot: ComboSlot,
    value: Customization,
    onValueChange: (Customization) -> Unit,
    depth: Int,
) {
    val selection = ComboChoices.selection(value, slot)
    var expanded by rememberSaveable(slot.id) { mutableStateOf(false) }
    val rule =
        if (ComboChoices.quantityAdjustable(slot)) {
            stringResource(
                R.string.customizer_slot_range,
                slot.includedQuantity,
                slot.minQuantity,
                slot.maxQuantity,
            )
        } else {
            stringResource(R.string.customizer_slot_included, slot.includedQuantity)
        }
    Column(
        modifier = Modifier.testTag(slotTag(slot.id)),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        BlockHeader(title = slot.name, rule = rule)
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (productId in slot.productIds) {
                val candidate = catalog.product(productId) ?: continue
                val option = slot.option(productId)
                val selected = selection.productId == productId
                val available =
                    if (option != null) option.availableNow && !option.soldOut
                    else candidate.selectable
                val substitution = option?.substitutionPrice?.takeIf { positiveAmount(it) }
                ChoiceChip(
                    label = candidate.name,
                    detail =
                        substitution?.let { MoneyFormat.formatSigned(it.decimal) }
                            ?: if (!available) stringResource(R.string.product_sold_out) else null,
                    selected = selected,
                    enabled = selected || available,
                    onClick = { onValueChange(ComboChoices.chooseProduct(value, slot, productId)) },
                    testTag = slotProductTag(slot.id, productId),
                )
            }
        }
        if (ComboChoices.quantityAdjustable(slot)) {
            Stepper(
                value = selection.quantity.toString(),
                onDecrease = {
                    onValueChange(ComboChoices.withQuantity(value, slot, selection.quantity - 1))
                },
                onIncrease = {
                    onValueChange(ComboChoices.withQuantity(value, slot, selection.quantity + 1))
                },
                canDecrease = selection.quantity > slot.minQuantity,
                canIncrease = selection.quantity < slot.maxQuantity,
                decreaseLabel = stringResource(R.string.customizer_less, slot.name.lowercase()),
                increaseLabel = stringResource(R.string.customizer_more, slot.name.lowercase()),
                testTag = slotTag(slot.id) + "-cantidad",
            )
        }
        val component = catalog.product(selection.productId)
        if (component != null && selection.quantity > 0) {
            TextButton(
                onClick = { expanded = !expanded },
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                        .testTag(slotTag(slot.id) + "-personalizar"),
            ) {
                Text(
                    if (expanded) stringResource(R.string.customizer_slot_hide, component.name)
                    else stringResource(R.string.customizer_slot_customize, component.name),
                )
            }
            if (expanded) {
                Column(modifier = Modifier.padding(start = AdelDimens.ScreenPadding)) {
                    CustomizationLevel(
                        catalog = catalog,
                        product = component,
                        variantId = selection.variantId,
                        value = selection.customization,
                        onVariantChange = { variantId ->
                            onValueChange(ComboChoices.withVariant(value, slot, variantId))
                        },
                        onValueChange = { nested ->
                            onValueChange(ComboChoices.withCustomization(value, slot, nested))
                        },
                        depth = depth + 1,
                    )
                }
            }
        }
        HorizontalDivider()
    }
}

/**
 * Indicaciones para cocina: las rápidas de la categoría (se marcan y desmarcan en su lugar), las
 * demás que ya tiene la partida y las observaciones que escribe el mesero con el teclado del
 * sistema. Todo va en las observaciones de la partida, sin cargo.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun KitchenNotesBlock(
    product: CatalogProduct,
    value: Customization,
    onValueChange: (Customization) -> Unit,
    quickNotes: List<QuickNoteOption>,
) {
    var draft by rememberSaveable(product.id) { mutableStateOf("") }
    var tooLong by remember(product.id) { mutableStateOf(false) }
    val current = KitchenNotes.split(value.notes)
    // Lo escrito (u otra indicación) que no es una indicación rápida elegida.
    val others = current.filter { entry ->
        quickNotes.none { note -> note.text == entry && QuickNoteChoices.isSelected(value, note) }
    }
    Column(
        modifier = Modifier.testTag(TAG_KITCHEN_NOTES),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        BlockHeader(
            title = stringResource(R.string.customizer_kitchen_notes),
            rule = stringResource(R.string.customizer_kitchen_notes_rule),
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (note in quickNotes) {
                val selected = QuickNoteChoices.isSelected(value, note)
                ChoiceChip(
                    label = note.text,
                    detail = null,
                    selected = selected,
                    enabled = selected || KitchenNotes.canAdd(value, note.text),
                    onClick = { onValueChange(QuickNoteChoices.toggle(value, note, quickNotes)) },
                    testTag = quickNoteTag(note.id),
                )
            }
            for (entry in others) {
                ChoiceChip(
                    label = entry,
                    detail = null,
                    selected = true,
                    enabled = true,
                    onClick = { onValueChange(KitchenNotes.withoutText(value, entry)) },
                    testTag = TAG_OBSERVATION_ENTRY,
                    description = stringResource(R.string.customizer_remove_note, entry),
                )
            }
        }
        val addObservation = {
            val next = KitchenNotes.withObservation(value, draft)
            if (next != null) {
                tooLong = false
                draft = ""
                onValueChange(next)
            } else if (draft.isNotBlank()) {
                tooLong = true
            }
        }
        OutlinedTextField(
            value = draft,
            onValueChange = {
                draft = it
                tooLong = false
            },
            label = { Text(stringResource(R.string.customizer_observation)) },
            placeholder = { Text(stringResource(R.string.customizer_observation_hint)) },
            singleLine = true,
            isError = tooLong,
            supportingText =
                if (tooLong) {
                    { Text(stringResource(R.string.customizer_observation_too_long)) }
                } else {
                    null
                },
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = { addObservation() }),
            modifier = Modifier.fillMaxWidth().testTag(TAG_OBSERVATION_FIELD),
        )
        SecondaryButton(
            text = stringResource(R.string.customizer_observation_add),
            onClick = addObservation,
            enabled = draft.isNotBlank(),
            modifier = Modifier.testTag(TAG_OBSERVATION_ADD),
        )
    }
}

/** Botón de elección (chip) de 48 dp con su texto y, abajo, un detalle (precio o estado). */
@Composable
private fun ChoiceChip(
    label: String,
    detail: String?,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
    testTag: String,
    description: String? = null,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        enabled = enabled,
        label = {
            Column(modifier = Modifier.padding(vertical = 6.dp)) {
                Text(label, style = MaterialTheme.typography.labelLarge)
                if (detail != null) {
                    Text(detail, style = MaterialTheme.typography.labelMedium)
                }
            }
        },
        modifier =
            Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                .testTag(testTag)
                .then(
                    if (description != null) {
                        Modifier.semantics { contentDescription = description }
                    } else {
                        Modifier
                    },
                ),
    )
}

@Composable
private fun groupRule(limits: GroupLimits): String {
    val unit = PortionQuantity.UNIT
    val minimum = PortionQuantity.fromThousandths(limits.minimum)
    val maximum = limits.maximum?.let { PortionQuantity.fromThousandths(it) }
    return when {
        limits.minimum == unit && limits.maximum == unit ->
            stringResource(R.string.customizer_required_one)
        limits.required && maximum == null ->
            stringResource(R.string.customizer_required_at_least, minimum)
        limits.required -> stringResource(R.string.customizer_required_range, minimum, maximum!!)
        limits.maximum == unit -> stringResource(R.string.customizer_optional_one)
        maximum == null -> stringResource(R.string.customizer_optional)
        else -> stringResource(R.string.customizer_optional_up_to, maximum)
    }
}

/**
 * Precio que se muestra en una opción, tal como lo configura el catálogo (como `optionPrices` del
 * escritorio): en grupos por equivalencia, el valor de referencia y su extra; en grupos por
 * cantidad, el extra o el precio del primer tramo. No se suma nada: el cargo real lo calcula Go.
 */
private fun optionDetail(group: OptionGroup, option: GroupOption): String? {
    val prices = mutableListOf<String>()
    if (group.pricingMode == GroupPricingMode.VALUE && positiveAmount(option.referenceValue)) {
        prices.add(option.referenceValue.display())
    }
    val tier = option.firstTierPrice
    if (positiveAmount(option.extraPrice)) {
        prices.add(MoneyFormat.formatSigned(option.extraPrice.decimal))
    } else if (
        group.pricingMode == GroupPricingMode.QUANTITY && tier != null && positiveAmount(tier)
    ) {
        prices.add(MoneyFormat.formatSigned(tier.decimal))
    }
    return prices.takeIf { it.isNotEmpty() }?.joinToString(" · ")
}

@Composable
private fun unavailableText(option: GroupOption): String? =
    when {
        option.soldOut -> stringResource(R.string.customizer_option_sold_out)
        !option.selectable -> stringResource(R.string.customizer_option_unavailable)
        else -> null
    }

@Composable
private fun levelText(level: IngredientLevel): String =
    when (level) {
        IngredientLevel.WITHOUT -> stringResource(R.string.customizer_ingredient_without)
        IngredientLevel.NORMAL -> stringResource(R.string.customizer_ingredient_normal)
        IngredientLevel.EXTRA -> stringResource(R.string.customizer_ingredient_extra)
    }

/**
 * Si un importe de Go es mayor que cero, mirando solo el texto (como `positiveAmount` del
 * escritorio): decide si se muestra un cargo, sin aritmética con dinero.
 */
internal fun positiveAmount(amount: Money): Boolean {
    val text = amount.decimal.trim()
    if (text.startsWith("-")) {
        return false
    }
    return text.any { it in '1'..'9' }
}

fun variantTag(productId: String, variantId: String?): String =
    "variant-$productId-${variantId ?: "base"}"

fun groupTag(groupId: String): String = "group-$groupId"

fun optionTag(optionId: String): String = "option-$optionId"

fun ingredientTag(ingredientId: String): String = "ingredient-$ingredientId"

fun slotTag(slotId: String): String = "slot-$slotId"

fun slotProductTag(slotId: String, productId: String): String = "slot-$slotId-$productId"

fun quickNoteTag(noteId: String): String = "quick-note-$noteId"

const val TAG_GROUP_WARNING = "group-warning"
const val TAG_KITCHEN_NOTES = "kitchen-notes"
const val TAG_OBSERVATION_FIELD = "observation-field"
const val TAG_OBSERVATION_ADD = "observation-add"
const val TAG_OBSERVATION_ENTRY = "observation-entry"
