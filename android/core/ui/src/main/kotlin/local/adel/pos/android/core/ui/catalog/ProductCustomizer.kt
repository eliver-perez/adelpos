package local.adel.pos.android.core.ui.catalog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.domain.catalog.AvailabilityStatus
import local.adel.pos.android.core.domain.catalog.Catalog
import local.adel.pos.android.core.domain.catalog.CatalogImages
import local.adel.pos.android.core.domain.catalog.CustomizationCheck
import local.adel.pos.android.core.domain.catalog.CustomizationProblem
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.ui.R
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.components.SecondaryButton
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.core.ui.theme.AdelTheme

/**
 * Personalizador táctil de una partida: presentación, complementos, porciones con −/+
 * (Sin/Normal/Extra), combos, indicaciones rápidas y observaciones, la cantidad y «Agregar».
 *
 * Produce exactamente el `LineRequest` del contrato ([LineSelection]): quien lo usa lo guarda en su
 * ViewModel ([onLineChange]) y lo agrega a la cuenta con [onAdd]. En el dispositivo solo se revisa
 * lo que declara el catálogo ([CustomizationCheck]: mínimos, máximos y obligatorios); los precios
 * son de referencia y el importe lo calcula Go al recibir la partida.
 */
@Composable
fun ProductCustomizer(
    catalog: Catalog,
    line: LineSelection,
    onLineChange: (LineSelection) -> Unit,
    onAdd: (LineSelection) -> Unit,
    onCancel: () -> Unit,
    images: CatalogImages?,
    modifier: Modifier = Modifier,
) {
    val product = catalog.product(line.productId) ?: return
    val problems = CustomizationCheck.problems(catalog, line)
    Column(modifier = modifier.fillMaxSize().testTag(TAG_CUSTOMIZER)) {
        Column(
            modifier =
                Modifier.weight(1f)
                    .verticalScroll(rememberScrollState())
                    .padding(AdelDimens.ScreenPadding),
            verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                ProductImage(
                    imageId = product.imageId,
                    name = product.name,
                    images = images,
                    modifier = Modifier.size(72.dp).clip(RoundedCornerShape(12.dp)),
                )
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = product.name,
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.testTag(TAG_CUSTOMIZER_TITLE),
                    )
                    val price =
                        product.variants.firstOrNull { it.id == line.variantId }?.referencePrice
                            ?: product.referencePrice
                    if (price != null) {
                        Text(
                            text =
                                stringResource(R.string.product_reference_price, price.display()),
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                }
            }
            if (product.description.isNotBlank()) {
                Text(
                    text = product.description,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = stringResource(R.string.product_reference_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            HorizontalDivider()
            CustomizationLevel(
                catalog = catalog,
                product = product,
                variantId = line.variantId,
                value = line.customization,
                onVariantChange = { variantId -> onLineChange(line.copy(variantId = variantId)) },
                onValueChange = { customization ->
                    onLineChange(line.copy(customization = customization))
                },
                depth = 0,
            )
        }
        HorizontalDivider()
        CustomizerFooter(
            line = line,
            problems = problems,
            onLineChange = onLineChange,
            onAdd = { onAdd(line) },
            onCancel = onCancel,
        )
    }
}

@Composable
private fun CustomizerFooter(
    line: LineSelection,
    problems: List<CustomizationProblem>,
    onLineChange: (LineSelection) -> Unit,
    onAdd: () -> Unit,
    onCancel: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(AdelDimens.ScreenPadding),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // Lo que falta para poder agregar (como mucho tres, para no tapar los botones).
        if (problems.isNotEmpty()) {
            Column(
                modifier =
                    Modifier.testTag(TAG_CUSTOMIZER_PROBLEMS).semantics(mergeDescendants = true) {
                        liveRegion = LiveRegionMode.Polite
                    },
            ) {
                for (problem in problems.take(3)) {
                    Text(
                        text = problemText(problem),
                        style = MaterialTheme.typography.bodyMedium,
                        color = AdelTheme.colors.red,
                    )
                }
            }
        }
        // Cantidad en su propio renglón y los botones abajo: caben igual en un teléfono (411 dp)
        // que
        // en el panel de la tableta.
        val quantityLabel = stringResource(R.string.customizer_quantity)
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            Text(
                text = quantityLabel,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
            )
            Stepper(
                value = line.quantity.toString(),
                onDecrease = { onLineChange(line.copy(quantity = line.quantity - 1)) },
                onIncrease = { onLineChange(line.copy(quantity = line.quantity + 1)) },
                canDecrease = line.quantity > 1,
                canIncrease = line.quantity < LineSelection.MAXIMUM_LINE_QUANTITY,
                decreaseLabel = stringResource(R.string.customizer_less, quantityLabel.lowercase()),
                increaseLabel = stringResource(R.string.customizer_more, quantityLabel.lowercase()),
                testTag = TAG_CUSTOMIZER_QUANTITY,
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            SecondaryButton(
                text = stringResource(R.string.customizer_cancel),
                onClick = onCancel,
                modifier = Modifier.testTag(TAG_CUSTOMIZER_CANCEL),
            )
            PrimaryButton(
                text =
                    if (line.quantity == 1) {
                        stringResource(R.string.customizer_add)
                    } else {
                        stringResource(R.string.customizer_add_quantity, line.quantity)
                    },
                onClick = onAdd,
                enabled = problems.isEmpty(),
                modifier = Modifier.weight(1f).testTag(TAG_CUSTOMIZER_ADD),
            )
        }
    }
}

/** Texto en español de lo que impide agregar la partida, con la ranura del combo si aplica. */
@Composable
fun problemText(problem: CustomizationProblem): String {
    val text =
        when (problem) {
            is CustomizationProblem.ProductNotSelectable ->
                if (
                    problem.status == AvailabilityStatus.SOLD_OUT ||
                        problem.status == AvailabilityStatus.SOLD_OUT_INDEFINITE
                ) {
                    stringResource(R.string.problem_sold_out, problem.productName)
                } else {
                    stringResource(
                        R.string.problem_unavailable,
                        problem.productName,
                        problem.reason,
                    )
                }
            is CustomizationProblem.VariantRequired ->
                stringResource(R.string.problem_variant_required, problem.productName)
            is CustomizationProblem.GroupBelowMinimum ->
                stringResource(R.string.problem_group_minimum, problem.groupName, problem.minimum)
            is CustomizationProblem.GroupAboveMaximum ->
                stringResource(R.string.problem_group_maximum, problem.groupName, problem.maximum)
            is CustomizationProblem.OptionAboveMaximum ->
                stringResource(R.string.problem_option_maximum, problem.optionName, problem.maximum)
            is CustomizationProblem.OptionNotSelectable ->
                stringResource(R.string.problem_option_unavailable, problem.optionName)
            is CustomizationProblem.ComponentOutOfRange ->
                stringResource(
                    R.string.problem_component_range,
                    problem.slotName,
                    problem.minimum,
                    problem.maximum,
                )
            is CustomizationProblem.NotesTooLong -> stringResource(R.string.problem_notes_too_long)
        }
    val where = problem.where
    // La ranura ya está en el texto de ComponentOutOfRange.
    return if (where == null || problem is CustomizationProblem.ComponentOutOfRange) {
        text
    } else {
        stringResource(R.string.problem_where, where, text)
    }
}

const val TAG_CUSTOMIZER = "customizer"
const val TAG_CUSTOMIZER_TITLE = "customizer-title"
const val TAG_CUSTOMIZER_PROBLEMS = "customizer-problems"
const val TAG_CUSTOMIZER_QUANTITY = "customizer-quantity"
const val TAG_CUSTOMIZER_ADD = "customizer-add"
const val TAG_CUSTOMIZER_CANCEL = "customizer-cancel"
