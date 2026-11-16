package local.adel.pos.android.core.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.domain.table.TableState
import local.adel.pos.android.core.ui.R
import local.adel.pos.android.core.ui.icons.AdelIcons
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.core.ui.theme.TableStateColors

/**
 * Estado de una mesa, que se reconoce por su color, su texto y su ícono, como en el escritorio
 * (`tableStateIcons.ts`): Libre con palomita, Ocupada con reloj, Esperando pago con recibo, Pago
 * parcial con efectivo y Fuera de servicio con candado. El estado lo calcula Go; aquí solo se
 * muestra.
 */
@Composable
fun TableStateBadge(state: TableState, modifier: Modifier = Modifier) {
    val colors = tableStateColors(state)
    Surface(
        modifier = modifier.testTag("$TAG_TABLE_STATE_PREFIX${state.wireName}"),
        shape = RoundedCornerShape(8.dp),
        color = colors.background,
        contentColor = colors.content,
        border = BorderStroke(width = 1.5.dp, color = colors.border),
    ) {
        Row(
            modifier = Modifier.heightIn(min = 36.dp).padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(
                imageVector = tableStateIcon(state),
                contentDescription = null,
                modifier =
                    Modifier.size(20.dp)
                        .testTag("$TAG_TABLE_STATE_ICON_PREFIX${tableStateIconName(state)}"),
            )
            Text(text = tableStateLabel(state), style = MaterialTheme.typography.labelLarge)
        }
    }
}

/** Texto del estado («Esperando pago»). */
@Composable
fun tableStateLabel(state: TableState): String =
    stringResource(
        when (state) {
            TableState.FREE -> R.string.table_state_free
            TableState.OCCUPIED -> R.string.table_state_occupied
            TableState.AWAITING_PAYMENT -> R.string.table_state_awaiting_payment
            TableState.PARTIAL_PAYMENT -> R.string.table_state_partial_payment
            TableState.OUT_OF_SERVICE -> R.string.table_state_out_of_service
        },
    )

/** Ícono del estado, el mismo del escritorio. */
fun tableStateIcon(state: TableState): ImageVector =
    when (state) {
        TableState.FREE -> AdelIcons.Check
        TableState.OCCUPIED -> AdelIcons.Clock
        TableState.AWAITING_PAYMENT -> AdelIcons.Receipt
        TableState.PARTIAL_PAYMENT -> AdelIcons.Cash
        TableState.OUT_OF_SERVICE -> AdelIcons.Lock
    }

/** Nombre del ícono del estado, como `data-icon` del escritorio (para las pruebas). */
fun tableStateIconName(state: TableState): String =
    when (state) {
        TableState.FREE -> "check"
        TableState.OCCUPIED -> "clock"
        TableState.AWAITING_PAYMENT -> "receipt"
        TableState.PARTIAL_PAYMENT -> "cash"
        TableState.OUT_OF_SERVICE -> "lock"
    }

/** Colores del estado según el tema vigente. */
@Composable
fun tableStateColors(state: TableState): TableStateColors {
    val colors = AdelTheme.colors
    return when (state) {
        TableState.FREE -> colors.tableFree
        TableState.OCCUPIED -> colors.tableOccupied
        TableState.AWAITING_PAYMENT -> colors.tableAwaitingPayment
        TableState.PARTIAL_PAYMENT -> colors.tablePartialPayment
        TableState.OUT_OF_SERVICE -> colors.tableOutOfService
    }
}

const val TAG_TABLE_STATE_PREFIX = "table-state-"
const val TAG_TABLE_STATE_ICON_PREFIX = "table-state-icon-"
