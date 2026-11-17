package local.adel.pos.android.core.ui.catalog

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.ui.icons.CatalogIcons
import local.adel.pos.android.core.ui.theme.AdelDimens

/**
 * Botones −/+ de 48 dp con el valor en medio («Sin», «Normal», «Extra», «2»). [decreaseLabel] e
 * [increaseLabel] son lo que dicen los lectores de pantalla («Menos cebolla»).
 */
@Composable
fun Stepper(
    value: String,
    onDecrease: () -> Unit,
    onIncrease: () -> Unit,
    canDecrease: Boolean,
    canIncrease: Boolean,
    decreaseLabel: String,
    increaseLabel: String,
    modifier: Modifier = Modifier,
    testTag: String = TAG_STEPPER,
) {
    Row(
        modifier = modifier.testTag(testTag),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        FilledTonalIconButton(
            onClick = onDecrease,
            enabled = canDecrease,
            modifier = Modifier.size(AdelDimens.MinTouchTarget).testTag("$testTag-less"),
        ) {
            Icon(CatalogIcons.Minus, contentDescription = decreaseLabel)
        }
        Text(
            text = value,
            style = MaterialTheme.typography.titleMedium,
            textAlign = TextAlign.Center,
            modifier =
                Modifier.widthIn(min = 72.dp).testTag("$testTag-value").semantics {
                    liveRegion = LiveRegionMode.Polite
                },
        )
        FilledTonalIconButton(
            onClick = onIncrease,
            enabled = canIncrease,
            modifier = Modifier.size(AdelDimens.MinTouchTarget).testTag("$testTag-more"),
        ) {
            Icon(CatalogIcons.Plus, contentDescription = increaseLabel)
        }
    }
}

const val TAG_STEPPER = "stepper"
