package com.adelpos.cocina.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.staggeredgrid.LazyVerticalStaggeredGrid
import androidx.compose.foundation.lazy.staggeredgrid.StaggeredGridCells
import androidx.compose.foundation.lazy.staggeredgrid.items
import androidx.compose.foundation.lazy.staggeredgrid.rememberLazyStaggeredGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import com.adelpos.cocina.R
import com.adelpos.cocina.board.BoardActions
import com.adelpos.cocina.board.BoardRules
import com.adelpos.cocina.board.BoardScreen
import com.adelpos.cocina.board.BoardState
import com.adelpos.cocina.board.TextSize
import com.adelpos.core.model.error.LicenseMessages
import com.adelpos.core.model.license.LicenseStateName
import com.adelpos.core.ui.components.PrimaryButton
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme
import kotlinx.coroutines.delay

/** La letra grande de la pantalla (Terminales, §4): todo un cuarto más grande. */
private const val LARGE_TEXT_SCALE = 1.25f

/**
 * La pantalla de pedidos de la cocina (§4 de la etapa K1): las tarjetas en columnas, del pedido más
 * antiguo al más nuevo, con su tiempo de espera en vivo; o lo que dice Go en su lugar (conectando,
 * sin cupo, cupo liberado, sin licencia activada). Arriba, los avisos de la licencia y de la hora.
 * [clock] es el reloj monotónico de la tableta.
 */
@Composable
fun OrdersScreen(
    state: BoardState,
    actions: BoardActions,
    clock: () -> Long,
    modifier: Modifier = Modifier,
) {
    val density = LocalDensity.current
    val scaled =
        if (state.settings.textSize == TextSize.LARGE) {
            Density(density.density, density.fontScale * LARGE_TEXT_SCALE)
        } else {
            density
        }
    CompositionLocalProvider(LocalDensity provides scaled) {
        Box(modifier = modifier.fillMaxSize().testTag(TAG_ORDERS)) {
            when (val screen = state.screen) {
                BoardScreen.Connecting ->
                    CenteredMessage(
                        title = stringResource(R.string.orders_connecting),
                        tag = TAG_ORDERS_CONNECTING,
                    )
                is BoardScreen.NoSeat ->
                    CenteredMessage(
                        title = LicenseMessages.kitchenDisplayNoSeat(screen.inUse, screen.limit),
                        tag = TAG_ORDERS_NO_SEAT,
                    )
                BoardScreen.SeatReleased ->
                    CenteredMessage(
                        title = LicenseMessages.KITCHEN_DISPLAY_SEAT_RELEASED,
                        detail = stringResource(R.string.seat_released_detail),
                        tag = TAG_ORDERS_SEAT_RELEASED,
                        action = {
                            PrimaryButton(
                                text = stringResource(R.string.seat_take),
                                onClick = actions::takeSeat,
                                modifier = Modifier.testTag(TAG_TAKE_SEAT),
                            )
                        },
                    )
                BoardScreen.NotActivated ->
                    CenteredMessage(
                        title = LicenseMessages.KITCHEN_DISPLAY_NOT_ACTIVATED,
                        tag = TAG_ORDERS_NOT_ACTIVATED,
                    )
                BoardScreen.Orders -> Board(state = state, actions = actions, clock = clock)
            }
        }
    }
}

@Composable
private fun Board(state: BoardState, actions: BoardActions, clock: () -> Long) {
    val board = state.board ?: return
    val cards = board.cards.filter { it.id !in state.markingCards }
    // La espera avanza en vivo con el reloj monotónico desde el `server_time` de la lectura.
    val now by
        produceState(clock()) {
            while (true) {
                delay(TICK_MILLIS)
                value = clock()
            }
        }
    Column(modifier = Modifier.fillMaxSize()) {
        Notices(state = state, onDismiss = actions::dismissNotice)
        if (cards.isEmpty()) {
            CenteredMessage(
                title = stringResource(R.string.orders_empty),
                detail = stringResource(R.string.orders_empty_detail),
                tag = TAG_ORDERS_EMPTY,
            )
            return@Column
        }
        val gridState = rememberLazyStaggeredGridState()
        val visible by remember { derivedStateOf { gridState.layoutInfo.visibleItemsInfo.size } }
        val hidden = BoardRules.hiddenCount(cards.size, visible)
        Box(modifier = Modifier.fillMaxSize()) {
            LazyVerticalStaggeredGrid(
                columns = StaggeredGridCells.Adaptive(CARD_MIN_WIDTH),
                state = gridState,
                modifier = Modifier.fillMaxSize().testTag(TAG_ORDERS_GRID),
                contentPadding = PaddingValues(AdelDimens.ScreenPadding),
                horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
                verticalItemSpacing = AdelDimens.Gap,
            ) {
                items(cards, key = { it.id }) { card ->
                    OrderCardView(
                        card = card,
                        settings = state.settings,
                        waitSeconds = BoardRules.waitSeconds(card, board.readAtMonotonic, now),
                        markingDishes = state.markingDishes,
                        markingAllowed = state.markingAllowed,
                        timezone = board.timezone,
                        onDish = { dishId -> actions.markDish(card.id, dishId) },
                        onOrder = { actions.markOrder(card.id) },
                    )
                }
            }
            if (hidden > 0) {
                Text(
                    text = pluralStringResource(R.plurals.orders_hidden, hidden, hidden),
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    color = AdelTheme.colors.onAccent,
                    modifier =
                        Modifier.align(Alignment.BottomCenter)
                            .padding(AdelDimens.ScreenPadding)
                            .background(AdelTheme.colors.blue, RoundedCornerShape(24.dp))
                            .padding(horizontal = 20.dp, vertical = 10.dp)
                            .testTag(TAG_ORDERS_HIDDEN),
                )
            }
        }
    }
}

// Los avisos de arriba: la hora incorrecta (no se puede marcar), la licencia vencida o revocada
// (se sigue trabajando) y el aviso pasajero de una respuesta de Go.
@Composable
private fun Notices(state: BoardState, onDismiss: () -> Unit) {
    val colors = AdelTheme.colors
    if (!state.markingAllowed) {
        NoticeBar(
            text = LicenseMessages.KITCHEN_DISPLAY_CLOCK_INVALID,
            background = colors.redSoft,
            content = colors.text,
            tag = TAG_NOTICE_CLOCK,
        )
    }
    when (BoardRules.licenseNotice(state.board?.license)) {
        LicenseStateName.EXPIRED ->
            NoticeBar(
                text = LicenseMessages.KITCHEN_DISPLAY_EXPIRED,
                background = colors.amberSoft,
                content = colors.text,
                tag = TAG_NOTICE_LICENSE,
            )
        LicenseStateName.REVOKED ->
            NoticeBar(
                text = LicenseMessages.KITCHEN_DISPLAY_REVOKED,
                background = colors.amberSoft,
                content = colors.text,
                tag = TAG_NOTICE_LICENSE,
            )
        else -> Unit
    }
    val notice = state.notice
    if (notice != null) {
        NoticeBar(
            text = notice,
            background = colors.amberSoft,
            content = colors.text,
            tag = TAG_NOTICE_MESSAGE,
            action = {
                SecondaryButton(text = stringResource(R.string.notice_close), onClick = onDismiss)
            },
        )
    }
}

@Composable
private fun NoticeBar(
    text: String,
    background: Color,
    content: Color,
    tag: String,
    action: (@Composable () -> Unit)? = null,
) {
    Column(
        modifier =
            Modifier.fillMaxWidth()
                .background(background)
                .padding(horizontal = AdelDimens.ScreenPadding, vertical = 12.dp)
                .testTag(tag),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.SemiBold,
            color = content,
        )
        action?.invoke()
    }
}

@Composable
private fun CenteredMessage(
    title: String,
    tag: String,
    detail: String? = null,
    action: (@Composable () -> Unit)? = null,
) {
    Box(
        modifier = Modifier.fillMaxSize().padding(AdelDimens.ScreenPadding),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier.widthIn(max = 720.dp).testTag(tag),
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
            action?.invoke()
        }
    }
}

/** Ancho mínimo de una columna de tarjetas: en la tableta horizontal caben tres o cuatro. */
private val CARD_MIN_WIDTH = 300.dp

private const val TICK_MILLIS = 1_000L

/** Etiquetas para las pruebas. */
const val TAG_ORDERS = "kitchen-orders"
const val TAG_ORDERS_EMPTY = "kitchen-orders-empty"
const val TAG_ORDERS_CONNECTING = "kitchen-orders-connecting"
const val TAG_ORDERS_NO_SEAT = "kitchen-orders-no-seat"
const val TAG_ORDERS_SEAT_RELEASED = "kitchen-orders-seat-released"
const val TAG_ORDERS_NOT_ACTIVATED = "kitchen-orders-not-activated"
const val TAG_ORDERS_GRID = "kitchen-orders-grid"
const val TAG_ORDERS_HIDDEN = "kitchen-orders-hidden"
const val TAG_TAKE_SEAT = "kitchen-take-seat"
const val TAG_NOTICE_CLOCK = "kitchen-notice-clock"
const val TAG_NOTICE_LICENSE = "kitchen-notice-license"
const val TAG_NOTICE_MESSAGE = "kitchen-notice-message"
