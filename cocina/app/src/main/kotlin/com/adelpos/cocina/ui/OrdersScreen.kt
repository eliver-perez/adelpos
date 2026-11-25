package com.adelpos.cocina.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import com.adelpos.cocina.R
import com.adelpos.cocina.access.DisplayConnection
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme

/**
 * La pantalla de pedidos de la cocina. Conectada y sin pedidos activos en sus estaciones dice «No
 * hay pedidos»; mientras se conecta, «Conectando con la computadora principal…»; si la licencia, el
 * cupo o la hora no lo permiten, el texto de la licencia. Las tarjetas de pedido (§4 de la etapa
 * K1) se dibujan aquí.
 */
@Composable
fun OrdersScreen(display: DisplayConnection, modifier: Modifier = Modifier) {
    val title: String
    val detail: String?
    val tag: String
    when (display) {
        DisplayConnection.Connected -> {
            title = stringResource(R.string.orders_empty)
            detail = stringResource(R.string.orders_empty_detail)
            tag = TAG_ORDERS_EMPTY
        }
        DisplayConnection.Connecting -> {
            title = stringResource(R.string.orders_connecting)
            detail = null
            tag = TAG_ORDERS_CONNECTING
        }
        is DisplayConnection.Refused -> {
            title = display.text
            detail = null
            tag = TAG_ORDERS_REFUSED
        }
    }
    Box(
        modifier = modifier.fillMaxSize().padding(AdelDimens.ScreenPadding).testTag(TAG_ORDERS),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier.testTag(tag),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.headlineMedium,
                color = AdelTheme.colors.text,
                textAlign = TextAlign.Center,
            )
            if (detail != null) {
                Text(
                    text = detail,
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
            }
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_ORDERS = "kitchen-orders"
const val TAG_ORDERS_EMPTY = "kitchen-orders-empty"
const val TAG_ORDERS_CONNECTING = "kitchen-orders-connecting"
const val TAG_ORDERS_REFUSED = "kitchen-orders-refused"
