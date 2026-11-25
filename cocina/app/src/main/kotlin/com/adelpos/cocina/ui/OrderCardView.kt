package com.adelpos.cocina.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.adelpos.cocina.R
import com.adelpos.cocina.board.BoardRules
import com.adelpos.cocina.board.Dish
import com.adelpos.cocina.board.DishState
import com.adelpos.cocina.board.DisplaySettings
import com.adelpos.cocina.board.OrderCard
import com.adelpos.cocina.board.WaitLevel
import com.adelpos.core.ui.components.PrimaryButton
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** Transparencia de lo que ya está listo (atenuado, §4). */
private const val READY_ALPHA = 0.45f

/**
 * Una tarjeta de pedido (§4): el encabezado con la mesa (o «Para llevar» y el cliente), el mesero,
 * la hora de envío y la espera en vivo con el color por tiempo (normal, atención, retraso; K1-D17);
 * los platillos con su cantidad y nombre en letra grande y sus cambios y notas debajo; «NUEVO» y
 * «CANCELADO»; y el botón grande del pedido completo.
 */
@Composable
fun OrderCardView(
    card: OrderCard,
    settings: DisplaySettings,
    waitSeconds: Long,
    markingDishes: Set<String>,
    markingAllowed: Boolean,
    timezone: String,
    onDish: (String) -> Unit,
    onOrder: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = AdelTheme.colors
    val level = BoardRules.level(waitSeconds, settings)
    val (headerBackground, accent) =
        when (level) {
            WaitLevel.NORMAL -> colors.panel to colors.border
            WaitLevel.ATTENTION -> colors.amberSoft to colors.amber
            WaitLevel.LATE -> colors.redSoft to colors.red
        }
    Surface(
        modifier = modifier.fillMaxWidth().testTag("$TAG_CARD_PREFIX${card.id}"),
        shape = RoundedCornerShape(12.dp),
        color = colors.background,
        border = BorderStroke(if (level == WaitLevel.NORMAL) 1.dp else 3.dp, accent),
    ) {
        Column {
            Header(
                card = card,
                waitSeconds = waitSeconds,
                level = level,
                background = headerBackground,
                timezone = timezone,
            )
            HorizontalDivider(color = accent)
            Column(
                modifier = Modifier.padding(vertical = 4.dp),
            ) {
                val mixed = BoardRules.mixesTracking(card)
                for (dish in card.dishes) {
                    DishRow(
                        dish = dish,
                        marking = dish.id in markingDishes,
                        tappable = BoardRules.dishTappable(dish, markingAllowed),
                        byOrderHint = mixed && dish.byOrder && dish.state == DishState.PENDING,
                        onClick = { onDish(dish.id) },
                    )
                }
            }
            if (BoardRules.hasByOrderDishes(card) && !BoardRules.mixesTracking(card)) {
                Text(
                    text = stringResource(R.string.card_by_order),
                    style = MaterialTheme.typography.bodyMedium,
                    color = colors.muted,
                    modifier = Modifier.padding(horizontal = 12.dp),
                )
            }
            PrimaryButton(
                text =
                    stringResource(
                        if (BoardRules.onlyDismiss(card)) R.string.card_dismiss
                        else R.string.card_ready,
                    ),
                onClick = onOrder,
                enabled = BoardRules.orderButtonEnabled(card, markingAllowed),
                icon = AdelIcons.Check,
                modifier =
                    Modifier.fillMaxWidth()
                        .padding(12.dp)
                        .testTag("$TAG_CARD_READY_PREFIX${card.id}"),
            )
        }
    }
}

@Composable
private fun Header(
    card: OrderCard,
    waitSeconds: Long,
    level: WaitLevel,
    background: Color,
    timezone: String,
) {
    val colors = AdelTheme.colors
    val waitColor =
        when (level) {
            WaitLevel.NORMAL -> colors.text
            WaitLevel.ATTENTION -> colors.amber
            WaitLevel.LATE -> colors.red
        }
    Column(
        modifier = Modifier.fillMaxWidth().background(background).padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Row(verticalAlignment = Alignment.Top) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = card.title,
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                    color = colors.text,
                )
                if (card.subtitle.isNotBlank()) {
                    Text(
                        text = card.subtitle,
                        style = MaterialTheme.typography.titleMedium,
                        color = colors.text,
                    )
                }
            }
            Text(
                text = BoardRules.waitText(waitSeconds),
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                color = waitColor,
                modifier = Modifier.testTag("$TAG_CARD_WAIT_PREFIX${card.id}"),
            )
        }
        val sent = card.sentAtMillis?.let { clockText(it, timezone) }
        val details =
            listOfNotNull(
                card.waiterName.takeIf { it.isNotBlank() },
                sent?.let { stringResource(R.string.card_sent_at, it) },
            )
        if (details.isNotEmpty()) {
            Text(
                text = details.joinToString(" · "),
                style = MaterialTheme.typography.bodyLarge,
                color = colors.muted,
            )
        }
        if (level != WaitLevel.NORMAL) {
            Text(
                text =
                    stringResource(
                        if (level == WaitLevel.LATE) R.string.card_late
                        else R.string.card_attention,
                    ),
                style = MaterialTheme.typography.labelLarge,
                fontWeight = FontWeight.Bold,
                color = waitColor,
                modifier = Modifier.testTag("$TAG_CARD_LEVEL_PREFIX${card.id}"),
            )
        }
    }
}

@Composable
private fun DishRow(
    dish: Dish,
    marking: Boolean,
    tappable: Boolean,
    byOrderHint: Boolean,
    onClick: () -> Unit,
) {
    val colors = AdelTheme.colors
    val cancelled = dish.state == DishState.CANCELLED
    val ready = dish.state == DishState.READY || marking
    val clickable =
        if (tappable && !marking) {
            Modifier.clickable(role = Role.Button, onClick = onClick)
        } else {
            Modifier
        }
    Column(
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .then(clickable)
                .alpha(if (ready) READY_ALPHA else 1f)
                .padding(horizontal = 12.dp, vertical = 6.dp)
                .testTag("$TAG_DISH_PREFIX${dish.id}"),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (ready) {
                Icon(
                    imageVector = AdelIcons.Check,
                    contentDescription = stringResource(R.string.dish_ready),
                    tint = colors.teal,
                    modifier = Modifier.size(28.dp).testTag("$TAG_DISH_READY_PREFIX${dish.id}"),
                )
            }
            Text(
                text = stringResource(R.string.dish_line, dish.quantity, dish.name),
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold,
                color = if (cancelled) colors.red else colors.text,
                textDecoration = if (cancelled) TextDecoration.LineThrough else null,
                modifier = Modifier.weight(1f, fill = false),
            )
            if (cancelled) {
                Badge(
                    text = stringResource(R.string.dish_cancelled),
                    background = colors.red,
                    tag = "$TAG_DISH_CANCELLED_PREFIX${dish.id}",
                )
            } else if (dish.isNew) {
                Badge(
                    text = stringResource(R.string.dish_new),
                    background = colors.blue,
                    tag = "$TAG_DISH_NEW_PREFIX${dish.id}",
                )
            }
        }
        for (part in dish.parts) {
            if (part.name != null) {
                Text(
                    text = part.name,
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.text,
                    textDecoration = if (cancelled) TextDecoration.LineThrough else null,
                )
            }
            for (line in part.lines) {
                Text(
                    text = line.text,
                    style = MaterialTheme.typography.titleMedium,
                    color = colors.text,
                    textDecoration = if (cancelled) TextDecoration.LineThrough else null,
                    modifier = Modifier.padding(start = (16 * (line.level + 1)).dp),
                )
            }
        }
        if (dish.notes.isNotBlank()) {
            Text(
                text = stringResource(R.string.dish_notes, dish.notes),
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                fontStyle = FontStyle.Italic,
                color = colors.text,
                modifier = Modifier.padding(start = 16.dp),
            )
        }
        if (dish.pendingChange && dish.state == DishState.PENDING) {
            Text(
                text = stringResource(R.string.dish_pending_change),
                style = MaterialTheme.typography.bodyMedium,
                color = colors.amber,
            )
        }
        if (byOrderHint) {
            Text(
                text = stringResource(R.string.dish_by_order),
                style = MaterialTheme.typography.bodyMedium,
                color = colors.muted,
            )
        }
    }
}

@Composable
private fun Badge(text: String, background: Color, tag: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge,
        fontWeight = FontWeight.Bold,
        color = AdelTheme.colors.onAccent,
        modifier =
            Modifier.background(background, RoundedCornerShape(6.dp))
                .padding(horizontal = 8.dp, vertical = 2.dp)
                .testTag(tag),
    )
}

/** La hora «19:08» en la zona del restaurante (la de la tableta si Go no la dio). */
fun clockText(epochMillis: Long, timezone: String): String {
    val zone =
        try {
            if (timezone.isBlank()) ZoneId.systemDefault() else ZoneId.of(timezone)
        } catch (_: java.time.DateTimeException) {
            ZoneId.systemDefault()
        }
    return DateTimeFormatter.ofPattern("HH:mm")
        .format(Instant.ofEpochMilli(epochMillis).atZone(zone))
}

/** Etiquetas para las pruebas (más el identificador de la tarjeta o del platillo). */
const val TAG_CARD_PREFIX = "kitchen-card-"
const val TAG_CARD_READY_PREFIX = "kitchen-card-ready-"
const val TAG_CARD_WAIT_PREFIX = "kitchen-card-wait-"
const val TAG_CARD_LEVEL_PREFIX = "kitchen-card-level-"
const val TAG_DISH_PREFIX = "kitchen-dish-"
const val TAG_DISH_READY_PREFIX = "kitchen-dish-ready-"
const val TAG_DISH_NEW_PREFIX = "kitchen-dish-new-"
const val TAG_DISH_CANCELLED_PREFIX = "kitchen-dish-cancelled-"
