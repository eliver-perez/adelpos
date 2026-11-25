package com.adelpos.cocina.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.adelpos.cocina.R
import com.adelpos.cocina.board.BoardActions
import com.adelpos.cocina.board.BoardRules
import com.adelpos.cocina.board.DishState
import com.adelpos.cocina.board.RecentOrder
import com.adelpos.cocina.board.RecentState
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme

/**
 * «Recientes» (§4): lo marcado listo en los últimos 30 minutos en las estaciones de la pantalla,
 * del más reciente al más antiguo, con «Recuperar» para regresar a la pantalla un platillo o el
 * pedido marcado por error (Go no vuelve a avisar al mesero, K1-D8). Si Go dice que ya no se puede,
 * su mensaje se muestra arriba y la lista se vuelve a leer.
 */
@Composable
fun RecentDialog(recent: RecentState, timezone: String, actions: BoardActions) {
    Dialog(
        onDismissRequest = actions::closeRecent,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier = Modifier.fillMaxSize().padding(32.dp).testTag(TAG_RECENT),
            shape = MaterialTheme.shapes.large,
            color = AdelTheme.colors.background,
        ) {
            Column(
                modifier = Modifier.padding(AdelDimens.ScreenPadding),
                verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = stringResource(R.string.recent_title),
                            style = MaterialTheme.typography.headlineSmall,
                            fontWeight = FontWeight.Bold,
                            color = AdelTheme.colors.text,
                        )
                        Text(
                            text = stringResource(R.string.recent_help),
                            style = MaterialTheme.typography.bodyLarge,
                            color = AdelTheme.colors.muted,
                        )
                    }
                    SecondaryButton(
                        text = stringResource(R.string.recent_close),
                        onClick = actions::closeRecent,
                        modifier = Modifier.testTag(TAG_RECENT_CLOSE),
                    )
                }
                when (recent) {
                    RecentState.Hidden -> Unit
                    RecentState.Loading ->
                        Box(
                            modifier = Modifier.fillMaxWidth().padding(32.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            CircularProgressIndicator()
                        }
                    is RecentState.Shown -> Shown(recent, timezone, actions)
                }
            }
        }
    }
}

@Composable
private fun Shown(recent: RecentState.Shown, timezone: String, actions: BoardActions) {
    val message = recent.message
    if (message != null) {
        Text(
            text = message,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
            color = AdelTheme.colors.red,
            modifier = Modifier.testTag(TAG_RECENT_MESSAGE),
        )
    }
    if (recent.orders.isEmpty()) {
        Text(
            text = stringResource(R.string.recent_empty),
            style = MaterialTheme.typography.titleMedium,
            color = AdelTheme.colors.text,
            modifier = Modifier.testTag(TAG_RECENT_EMPTY),
        )
        return
    }
    LazyColumn(verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
        items(recent.orders, key = { it.accountId + (it.markedAtMillis ?: 0L) }) { order ->
            RecentOrderView(order = order, timezone = timezone, actions = actions)
            HorizontalDivider(color = AdelTheme.colors.border)
        }
    }
}

@Composable
private fun RecentOrderView(order: RecentOrder, timezone: String, actions: BoardActions) {
    val colors = AdelTheme.colors
    Column(
        modifier = Modifier.fillMaxWidth().testTag("$TAG_RECENT_ORDER_PREFIX${order.accountId}"),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text =
                        listOf(order.title, order.subtitle)
                            .filter { it.isNotBlank() }
                            .joinToString(" · "),
                    style = MaterialTheme.typography.titleLarge,
                    fontWeight = FontWeight.Bold,
                    color = colors.text,
                )
                val marked = order.markedAtMillis?.let { clockText(it, timezone) }
                val details =
                    listOfNotNull(
                        order.waiterName.takeIf { it.isNotBlank() },
                        marked?.let { stringResource(R.string.recent_marked_at, it) },
                    )
                if (details.isNotEmpty()) {
                    Text(
                        text = details.joinToString(" · "),
                        style = MaterialTheme.typography.bodyLarge,
                        color = colors.muted,
                    )
                }
            }
            val all = BoardRules.recoverableTasks(order.dishes)
            if (order.dishes.count { it.state == DishState.READY } > 1) {
                SecondaryButton(
                    text = stringResource(R.string.recent_recover_order),
                    onClick = { actions.recover(all) },
                    enabled = all.isNotEmpty(),
                    modifier =
                        Modifier.testTag("$TAG_RECENT_RECOVER_ORDER_PREFIX${order.accountId}"),
                )
            }
        }
        for (dish in order.dishes) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = stringResource(R.string.dish_line, dish.quantity, dish.name),
                    style = MaterialTheme.typography.titleMedium,
                    color = colors.text,
                    modifier = Modifier.weight(1f),
                )
                val tasks = BoardRules.recoverableTasks(listOf(dish))
                SecondaryButton(
                    text = stringResource(R.string.recent_recover),
                    onClick = { actions.recover(tasks) },
                    enabled = tasks.isNotEmpty(),
                    modifier = Modifier.testTag("$TAG_RECENT_RECOVER_PREFIX${dish.id}"),
                )
            }
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_RECENT = "kitchen-recent"
const val TAG_RECENT_CLOSE = "kitchen-recent-close"
const val TAG_RECENT_EMPTY = "kitchen-recent-empty"
const val TAG_RECENT_MESSAGE = "kitchen-recent-message"
const val TAG_RECENT_ORDER_PREFIX = "kitchen-recent-order-"
const val TAG_RECENT_RECOVER_PREFIX = "kitchen-recent-recover-"
const val TAG_RECENT_RECOVER_ORDER_PREFIX = "kitchen-recent-recover-order-"
