package local.adel.pos.android.feature.waiter.tables

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.domain.waiter.DiningTable
import local.adel.pos.android.core.domain.waiter.RestaurantView
import local.adel.pos.android.core.domain.waiter.TableCanvas
import local.adel.pos.android.core.domain.waiter.TableShape
import local.adel.pos.android.core.ui.components.tableStateColors
import local.adel.pos.android.core.ui.components.tableStateIcon
import local.adel.pos.android.core.ui.components.tableStateLabel
import local.adel.pos.android.core.ui.theme.AdelDimens

/**
 * Vista de mapa (§6.1): cada área en su lienzo lógico de 1000 × 560 (las posiciones y tamaños de
 * Go) escalado al espacio disponible sin deformarlo; cada mesa con su forma, el color y el ícono de
 * su estado. Las mesas que no coinciden con la búsqueda se atenúan.
 */
@Composable
internal fun TablesMap(
    view: RestaurantView,
    matches: Set<String>,
    areaId: String?,
    onAreaSelect: (String) -> Unit,
    onTableSelect: (DiningTable) -> Unit,
    modifier: Modifier = Modifier,
) {
    val areas = view.visibleAreas
    val current = areas.firstOrNull { it.id == areaId } ?: areas.firstOrNull() ?: return
    Column(modifier = modifier.fillMaxWidth().testTag(TAG_TABLES_MAP)) {
        Row(
            modifier = Modifier.horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (area in areas) {
                FilterChip(
                    selected = area.id == current.id,
                    onClick = { onAreaSelect(area.id) },
                    label = { Text(area.name) },
                    modifier =
                        Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                            .testTag("$TAG_MAP_AREA_PREFIX${area.id}"),
                )
            }
        }
        BoxWithConstraints(
            modifier = Modifier.fillMaxWidth().weight(1f).padding(vertical = 8.dp),
        ) {
            val scale = TableCanvas.scaleToFit(maxWidth.value, maxHeight.value)
            Surface(
                shape = RoundedCornerShape(12.dp),
                color = MaterialTheme.colorScheme.surfaceContainerLow,
                border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
                modifier =
                    Modifier.size((TableCanvas.WIDTH * scale).dp, (TableCanvas.HEIGHT * scale).dp),
            ) {
                Box(modifier = Modifier.fillMaxSize()) {
                    for (table in view.tablesIn(current.id)) {
                        MapTable(
                            table = table,
                            scale = scale,
                            dimmed = table.id !in matches,
                            onClick = { onTableSelect(table) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun MapTable(table: DiningTable, scale: Float, dimmed: Boolean, onClick: () -> Unit) {
    val colors = tableStateColors(table.state)
    val shape =
        if (table.shape == TableShape.ROUND) {
            CircleShape
        } else {
            RoundedCornerShape(8.dp)
        }
    val label = tableStateLabel(table.state)
    Surface(
        shape = shape,
        color = colors.background,
        contentColor = colors.content,
        border = BorderStroke(1.5.dp, colors.border),
        modifier =
            Modifier.offset((table.x * scale).dp, (table.y * scale).dp)
                .size((table.width * scale).dp, (table.height * scale).dp)
                .alpha(if (dimmed) DIMMED_ALPHA else 1f)
                .clickable(onClick = onClick)
                .semantics { contentDescription = "${table.name}, $label" }
                .testTag("$TAG_MAP_TABLE_PREFIX${table.id}"),
    ) {
        Column(
            modifier = Modifier.fillMaxSize().padding(2.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = table.name,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                textAlign = TextAlign.Center,
            )
            Icon(
                tableStateIcon(table.state),
                contentDescription = null,
                modifier = Modifier.size(16.dp),
            )
        }
    }
}

private const val DIMMED_ALPHA = 0.35f

const val TAG_TABLES_MAP = "tables-map"
const val TAG_MAP_AREA_PREFIX = "map-area-"
const val TAG_MAP_TABLE_PREFIX = "map-table-"
