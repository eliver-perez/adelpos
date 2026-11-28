package com.adelpos.meseros.feature.waiter.tables

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.border
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
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.RestaurantView
import com.adelpos.meseros.core.domain.waiter.TableGroup
import com.adelpos.meseros.core.domain.waiter.TableMapFit
import com.adelpos.meseros.core.domain.waiter.TableNames
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.TableUnion
import com.adelpos.meseros.core.domain.waiter.UnionOutline
import com.adelpos.meseros.core.ui.components.tableStateColors
import com.adelpos.meseros.core.ui.components.tableStateIcon
import com.adelpos.meseros.core.ui.components.tableStateLabel
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
    onGroupJoin: ((TableGroup) -> Unit)? = null,
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
                    // Etapa M1 (contrato §10), detrás de las mesas: el marco tenue de cada grupo
                    // con su nombre (su atajo) y el contorno sólido de cada unión con su nombre y
                    // su capacidad. Las mesas se siguen tocando: tocar una mesa unida abre la
                    // unión.
                    val unions = view.unionsIn(current.id)
                    for (group in view.groupsIn(current.id)) {
                        if (unions.any { it.groupId == group.id }) {
                            continue
                        }
                        val box =
                            UnionOutline.of(
                                fit,
                                group.tableIds.mapNotNull(view::table),
                                GROUP_MARGIN,
                            ) ?: continue
                        GroupFrame(group, box, onGroupJoin)
                    }
                    for (union in unions) {
                        val box =
                            UnionOutline.of(
                                fit,
                                union.tableIds.mapNotNull(view::table),
                                UNION_MARGIN,
                            ) ?: continue
                        UnionFrame(union, box)
                    }
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

/** El contorno sólido de una unión (etapa M1) con su nombre y su capacidad arriba. */
@Composable
private fun UnionFrame(union: TableUnion, box: UnionOutline.Box) {
    val top = (box.top - LABEL_BAND).coerceAtLeast(0f)
    Box(
        modifier =
            Modifier.offset(box.left.dp, top.dp)
                .size(box.width.dp, (box.top + box.height - top).dp)
                .border(2.dp, MaterialTheme.colorScheme.primary, RoundedCornerShape(12.dp))
                .semantics { contentDescription = "${union.name}, ${union.seats}" }
                .testTag("$TAG_MAP_UNION_PREFIX${union.id}"),
    ) {
        Text(
            text = stringResource(R.string.join_preview, union.name, union.seats),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.primary,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
        )
    }
}

/**
 * El marco tenue de un grupo predefinido (etapa M1, §3) con su nombre; tocar el nombre es el atajo
 * que une todas sus secciones (si se puede unir aquí).
 */
@Composable
private fun GroupFrame(
    group: TableGroup,
    box: UnionOutline.Box,
    onGroupJoin: ((TableGroup) -> Unit)?,
) {
    val top = (box.top - LABEL_BAND).coerceAtLeast(0f)
    Box(
        modifier =
            Modifier.offset(box.left.dp, top.dp)
                .size(box.width.dp, (box.top + box.height - top).dp)
                .border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(12.dp))
                .testTag("$TAG_MAP_GROUP_PREFIX${group.id}"),
    ) {
        Text(
            text =
                if (onGroupJoin != null) {
                    stringResource(R.string.table_panel_join_group, group.name)
                } else {
                    group.name
                },
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier =
                Modifier.then(
                        if (onGroupJoin != null) {
                            Modifier.clickable { onGroupJoin(group) }
                        } else {
                            Modifier
                        },
                    )
                    .padding(horizontal = 8.dp, vertical = 2.dp)
                    .testTag("$TAG_MAP_GROUP_JOIN_PREFIX${group.id}"),
        )
    }
}

/** Margen (dp) del contorno de una unión y del marco de un grupo alrededor de sus mesas. */
private const val UNION_MARGIN = 6f
private const val GROUP_MARGIN = 10f

/** Espacio (dp) arriba del contorno para su nombre. */
private const val LABEL_BAND = 20f

/** Ancho (dp) por debajo del cual una mesa del mapa muestra su nombre corto. */
private const val COMPACT_BELOW_DP = 80f

const val TAG_TABLES_MAP = "tables-map"
const val TAG_MAP_CANVAS = "tables-map-canvas"
const val TAG_MAP_TABLE_PREFIX = "map-table-"
const val TAG_MAP_UNION_PREFIX = "map-union-"
const val TAG_MAP_GROUP_PREFIX = "map-group-"
const val TAG_MAP_GROUP_JOIN_PREFIX = "map-group-join-"
