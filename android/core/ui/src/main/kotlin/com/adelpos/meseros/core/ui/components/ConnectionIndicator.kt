package com.adelpos.meseros.core.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.connection.ConnectionSummary
import com.adelpos.meseros.core.domain.connection.LinkState
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.theme.AdelTheme

/**
 * Indicador de conexión, siempre visible en la barra superior (§2): «En línea» (punto azul) o «Sin
 * conexión» (punto rojo) y, si hay pedidos que faltan por llegar al servidor, «N pedidos por
 * enviar» en ámbar. Los cambios se anuncian a los lectores de pantalla.
 */
@Composable
fun ConnectionIndicator(summary: ConnectionSummary, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    val online = summary.link == LinkState.ONLINE
    Row(
        modifier =
            modifier.testTag(TAG_CONNECTION_INDICATOR).semantics(mergeDescendants = true) {
                liveRegion = LiveRegionMode.Polite
            },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Box(
            modifier =
                Modifier.size(10.dp)
                    .background(
                        color = if (online) colors.statusOnline else colors.statusOffline,
                        shape = CircleShape,
                    ),
        )
        Text(
            text =
                stringResource(
                    if (online) R.string.connection_online else R.string.connection_offline,
                ),
            style = MaterialTheme.typography.labelLarge,
            color = if (online) colors.text else colors.red,
            maxLines = 1,
            modifier = Modifier.testTag(TAG_CONNECTION_STATE),
        )
        if (summary.pendingOrders > 0) {
            PendingOrdersChip(summary.pendingOrders)
        }
    }
}

@Composable
private fun PendingOrdersChip(count: Int) {
    val colors = AdelTheme.colors
    Row(
        modifier =
            Modifier.heightIn(min = 32.dp)
                .background(colors.amberSoft, RoundedCornerShape(16.dp))
                .padding(horizontal = 10.dp)
                .testTag(TAG_PENDING_ORDERS),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(
            imageVector = AdelIcons.Send,
            contentDescription = null,
            tint = colors.amber,
            modifier = Modifier.size(18.dp),
        )
        Text(
            text = pluralStringResource(R.plurals.connection_pending_orders, count, count),
            style = MaterialTheme.typography.labelLarge,
            color = colors.amber,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** Etiquetas para las pruebas. */
const val TAG_CONNECTION_INDICATOR = "connection-indicator"
const val TAG_CONNECTION_STATE = "connection-state"
const val TAG_PENDING_ORDERS = "connection-pending-orders"
