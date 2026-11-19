package com.adelpos.meseros.feature.waiter.tables

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TableMapFit
import com.adelpos.meseros.core.domain.waiter.TableNames
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.ui.components.tableStateColors
import com.adelpos.meseros.core.ui.components.tableStateIcon
import com.adelpos.meseros.core.ui.components.tableStateLabel
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.feature.waiter.R

/**
 * Vista de mapa (§6.1) del área elegida: cada mesa con su forma, el color y el ícono de su estado,
 * en las posiciones de Go. Etapa A3 (§4.7): el mapa **llena todo el espacio** con la escala
 * ajustada a las mesas del área ([TableMapFit], la regla del escritorio) y se desplaza en los dos
 * ejes si no caben con el tamaño mínimo. Las áreas se eligen en la fila de arriba de Mesas.
 */
@Composable
internal fun TablesMap(
    view: RestaurantView,
    areaId: String?,
    onTableSelect: (DiningTable) -> Unit,
    modifier: Modifier = Modifier,
) {
    val areas = view.visibleAreas
    val current = areas.firstOrNull { it.id == areaId } ?: areas.firstOrNull() ?: return
    val tables = view.tablesIn(current.id)
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = modifier.fillMaxSize().testTag(TAG_TABLES_MAP),
    ) {
        BoxWithConstraints(modifier = Modifier.fillMaxSize()) {
            val fit = TableMapFit.of(tables, maxWidth.value, maxHeight.value)
            if (fit == null) {
                Text(
                    text = stringResource(R.string.tables_area_empty),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(AdelDimens.ScreenPadding),
                )
                return@BoxWithConstraints
            }
            Box(
                modifier =
                    Modifier.fillMaxSize()
                        .horizontalScroll(rememberScrollState())
                        .verticalScroll(rememberScrollState()),
            ) {
                Box(modifier = Modifier.size(fit.width.dp, fit.height.dp).testTag(TAG_MAP_CANVAS)) {
                    val compact = TableNames.compact(tables)
                    for (table in tables) {
                        MapTable(
                            table = table,
                            // Una mesa angosta (muchas mesas en el teléfono) muestra su nombre
                            // corto.
                            name =
                                if (table.width * fit.scale < COMPACT_BELOW_DP) {
                                    compact[table.id] ?: table.name
                                } else {
                                    table.name
                                },
                            left = fit.left(table),
                            top = fit.top(table),
                            scale = fit.scale,
                            onClick = { onTableSelect(table) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun MapTable(
    table: DiningTable,
    name: String,
    left: Float,
    top: Float,
    scale: Float,
    onClick: () -> Unit,
) {
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
            Modifier.offset(left.dp, top.dp)
                .size((table.width * scale).dp, (table.height * scale).dp)
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
                text = name,
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

/** Ancho (dp) por debajo del cual una mesa del mapa muestra su nombre corto. */
private const val COMPACT_BELOW_DP = 80f

const val TAG_TABLES_MAP = "tables-map"
const val TAG_MAP_CANVAS = "tables-map-canvas"
const val TAG_MAP_TABLE_PREFIX = "map-table-"
