package com.adelpos.meseros.core.ui.waiter

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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.domain.waiter.ElapsedTime
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.TakeawayPayment
import com.adelpos.meseros.core.domain.waiter.TakeawayPreparation
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.icons.CatalogIcons

/** Colores de una etiqueta de estado: fondo suave, borde y el tono del ícono. */
private data class BadgeTone(val background: Color, val border: Color, val accent: Color)

/**
 * Estado de un pedido (§7) con color, texto e ícono, **siempre distintos** entre los cinco:
 * Borrador (gris, lápiz), Por enviar (ámbar, reloj), Enviando (azul, enviar), Recibido por el
 * restaurante (verde azulado, palomita; solo con la confirmación de Go) y Requiere tu revisión
 * (rojo, alerta).
 */
@Composable
fun OrderStateBadge(state: OrderState, modifier: Modifier = Modifier) {
    StateBadge(
        text = orderStateLabel(state),
        icon = orderStateIcon(state),
        tone = orderStateTone(state),
        modifier = modifier.testTag("$TAG_ORDER_STATE_PREFIX${state.storedName}"),
    )
}

@Composable
fun orderStateLabel(state: OrderState): String =
    stringResource(
        when (state) {
            OrderState.DRAFT -> R.string.order_state_draft
            OrderState.QUEUED -> R.string.order_state_queued
            OrderState.SENDING -> R.string.order_state_sending
            OrderState.RECEIVED -> R.string.order_state_received
            OrderState.NEEDS_REVIEW -> R.string.order_state_needs_review
        },
    )

fun orderStateIcon(state: OrderState): ImageVector =
    when (state) {
        OrderState.DRAFT -> AdelIcons.Edit
        OrderState.QUEUED -> AdelIcons.Clock
        OrderState.SENDING -> AdelIcons.Send
        OrderState.RECEIVED -> AdelIcons.Check
        OrderState.NEEDS_REVIEW -> AdelIcons.Alert
    }

@Composable
private fun orderStateTone(state: OrderState): BadgeTone {
    val colors = AdelTheme.colors
    return when (state) {
        OrderState.DRAFT -> BadgeTone(colors.graySoft, colors.gray, colors.gray)
        OrderState.QUEUED -> BadgeTone(colors.amberSoft, colors.amber, colors.amber)
        OrderState.SENDING -> BadgeTone(colors.blueSoft, colors.blue, colors.blue)
        OrderState.RECEIVED -> BadgeTone(colors.tealSoft, colors.teal, colors.teal)
        OrderState.NEEDS_REVIEW -> BadgeTone(colors.redSoft, colors.red, colors.red)
    }
}

/**
 * Preparación de un producto enviado o de una cuenta (`preparation_state` de Go), con los textos
 * del escritorio y de cocina.
 */
@Composable
fun PreparationBadge(state: PreparationState?, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    val tone =
        when (state) {
            PreparationState.READY -> BadgeTone(colors.tealSoft, colors.teal, colors.teal)
            PreparationState.PREPARING,
            PreparationState.RELEASED,
            PreparationState.SCHEDULED -> BadgeTone(colors.blueSoft, colors.blue, colors.blue)
            PreparationState.PENDING_RELEASE,
            PreparationState.CHANGE_PENDING,
            PreparationState.CONFIRMED_PENDING_DISPATCH ->
                BadgeTone(colors.amberSoft, colors.amber, colors.amber)
            else -> BadgeTone(colors.graySoft, colors.gray, colors.gray)
        }
    val icon =
        when (state) {
            PreparationState.READY -> AdelIcons.Check
            PreparationState.CANCELLED -> CatalogIcons.Close
            PreparationState.PENDING_RELEASE,
            PreparationState.CHANGE_PENDING -> AdelIcons.Alert
            else -> AdelIcons.Clock
        }
    StateBadge(
        text = preparationLabel(state),
        icon = icon,
        tone = tone,
        modifier = modifier.testTag("$TAG_PREPARATION_PREFIX${state?.wireName ?: "unknown"}"),
    )
}

@Composable
fun preparationLabel(state: PreparationState?): String =
    stringResource(
        when (state) {
            PreparationState.WITHOUT_ITEMS -> R.string.preparation_without_items
            PreparationState.DRAFT -> R.string.preparation_draft
            PreparationState.CONFIRMED_PENDING_DISPATCH ->
                R.string.preparation_confirmed_pending_dispatch
            PreparationState.PENDING_RELEASE -> R.string.preparation_pending_release
            PreparationState.SCHEDULED -> R.string.preparation_scheduled
            PreparationState.RELEASED -> R.string.preparation_released
            PreparationState.PREPARING -> R.string.preparation_preparing
            PreparationState.READY -> R.string.preparation_ready
            PreparationState.CHANGE_PENDING -> R.string.preparation_change_pending
            PreparationState.CANCELLED -> R.string.preparation_cancelled
            null -> R.string.preparation_unknown
        },
    )

/**
 * Preparación de un pedido para llevar en la lista (§3.3: pendiente, en preparación, listo), con la
 * correspondencia de [com.adelpos.meseros.core.domain.waiter.TakeawayStates].
 */
@Composable
fun TakeawayPreparationBadge(state: TakeawayPreparation, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    val (tone, icon, text) =
        when (state) {
            TakeawayPreparation.NOT_SENT ->
                Triple(
                    BadgeTone(colors.graySoft, colors.gray, colors.gray),
                    AdelIcons.Edit,
                    R.string.takeaway_preparation_not_sent,
                )
            TakeawayPreparation.PENDING ->
                Triple(
                    BadgeTone(colors.amberSoft, colors.amber, colors.amber),
                    AdelIcons.Clock,
                    R.string.takeaway_preparation_pending,
                )
            TakeawayPreparation.PREPARING ->
                Triple(
                    BadgeTone(colors.blueSoft, colors.blue, colors.blue),
                    AdelIcons.Clock,
                    R.string.takeaway_preparation_preparing,
                )
            TakeawayPreparation.READY ->
                Triple(
                    BadgeTone(colors.tealSoft, colors.teal, colors.teal),
                    AdelIcons.Check,
                    R.string.takeaway_preparation_ready,
                )
            TakeawayPreparation.CANCELLED ->
                Triple(
                    BadgeTone(colors.graySoft, colors.gray, colors.gray),
                    CatalogIcons.Close,
                    R.string.takeaway_preparation_cancelled,
                )
            TakeawayPreparation.UNKNOWN ->
                Triple(
                    BadgeTone(colors.graySoft, colors.gray, colors.gray),
                    AdelIcons.Clock,
                    R.string.takeaway_preparation_unknown,
                )
        }
    StateBadge(
        text = stringResource(text),
        icon = icon,
        tone = tone,
        modifier = modifier.testTag("$TAG_TAKEAWAY_PREPARATION_PREFIX${state.name.lowercase()}"),
    )
}

/**
 * Pago de un pedido para llevar como lo dice Go (sin pagar, pago parcial, pagado) o, ya entregado,
 * «Entregado». La aplicación nunca cobra.
 */
@Composable
fun TakeawayPaymentBadge(
    payment: TakeawayPayment,
    delivered: Boolean,
    modifier: Modifier = Modifier,
) {
    val colors = AdelTheme.colors
    val (tone, icon, text) =
        when {
            delivered ->
                Triple(
                    BadgeTone(colors.tealSoft, colors.teal, colors.teal),
                    AdelIcons.Check,
                    R.string.takeaway_delivered,
                )
            payment == TakeawayPayment.PAID ->
                Triple(
                    BadgeTone(colors.tealSoft, colors.teal, colors.teal),
                    AdelIcons.Cash,
                    R.string.takeaway_payment_paid,
                )
            payment == TakeawayPayment.PARTIAL ->
                Triple(
                    BadgeTone(colors.amberSoft, colors.amber, colors.amber),
                    AdelIcons.Cash,
                    R.string.takeaway_payment_partial,
                )
            payment == TakeawayPayment.UNPAID ->
                Triple(
                    BadgeTone(colors.amberSoft, colors.amber, colors.amber),
                    AdelIcons.Receipt,
                    R.string.takeaway_payment_unpaid,
                )
            else ->
                Triple(
                    BadgeTone(colors.graySoft, colors.gray, colors.gray),
                    AdelIcons.Receipt,
                    R.string.takeaway_payment_unknown,
                )
        }
    val tag = if (delivered) "delivered" else payment.name.lowercase()
    StateBadge(
        text = stringResource(text),
        icon = icon,
        tone = tone,
        modifier = modifier.testTag("$TAG_TAKEAWAY_PAYMENT_PREFIX$tag"),
    )
}

/**
 * Tiempo de una mesa con el formato del escritorio («45 min»); si Go la marcó como abierta mucho
 * tiempo (`long_open`), en una etiqueta ámbar con el triángulo de advertencia, distinta de los
 * colores de estado.
 */
@Composable
fun ElapsedLabel(
    seconds: Long,
    longOpen: Boolean,
    longOpenHours: Int?,
    modifier: Modifier = Modifier,
) {
    val colors = AdelTheme.colors
    val text = ElapsedTime.format(seconds)
    if (!longOpen) {
        Row(
            modifier = modifier,
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(AdelIcons.Clock, contentDescription = null, modifier = Modifier.size(16.dp))
            // Etapa A3 (§4.8): una etiqueta nunca se parte; si no cabe, se acorta.
            Text(
                text = text,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        return
    }
    val warning =
        if (longOpenHours != null) {
            stringResource(R.string.elapsed_long_open_hours, longOpenHours)
        } else {
            stringResource(R.string.elapsed_long_open)
        }
    Surface(
        modifier = modifier.testTag(TAG_LONG_OPEN),
        shape = RoundedCornerShape(999.dp),
        color = colors.amberSoft,
        border = BorderStroke(1.dp, colors.warningMark),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Icon(
                AdelIcons.Alert,
                contentDescription = warning,
                tint = colors.warningMark,
                modifier = Modifier.size(16.dp),
            )
            // Etapa A3 (§4.8): una etiqueta nunca se parte; si no cabe, se acorta.
            Text(
                text = text,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun StateBadge(text: String, icon: ImageVector, tone: BadgeTone, modifier: Modifier) {
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(8.dp),
        color = tone.background,
        contentColor = AdelTheme.colors.text,
        border = BorderStroke(width = 1.5.dp, color = tone.border),
    ) {
        Row(
            modifier = Modifier.heightIn(min = 32.dp).padding(horizontal = 8.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = tone.accent,
                modifier = Modifier.size(18.dp),
            )
            // Etapa A3 (§4.8): una etiqueta nunca se parte; si no cabe, se acorta.
            Text(
                text = text,
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

const val TAG_ORDER_STATE_PREFIX = "order-state-"
const val TAG_PREPARATION_PREFIX = "preparation-"
const val TAG_LONG_OPEN = "long-open"
const val TAG_TAKEAWAY_PREPARATION_PREFIX = "takeaway-preparation-"
const val TAG_TAKEAWAY_PAYMENT_PREFIX = "takeaway-payment-"
