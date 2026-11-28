package com.adelpos.meseros.core.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.adelpos.core.ui.components.LocalUserActivity
import com.adelpos.core.ui.components.TAG_PENDING_ORDERS
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.core.ui.theme.waiterBadgeColors
import com.adelpos.meseros.core.ui.R

/**
 * Distintivo de un mesero: un círculo con su inicial en el color que eligió el administrador (dato
 * de Go, `color` de `auth/pin-users`), con la inicial en el tono que mejor contrasta en ambos temas
 * ([waiterBadgeColors]). Decorativo: el nombre del mesero siempre lo acompaña.
 */
@Composable
fun WaiterBadge(
    initial: String,
    colorHex: String?,
    modifier: Modifier = Modifier,
    size: Dp = 48.dp,
) {
    val colors = waiterBadgeColors(colorHex, AdelTheme.colors)
    Box(
        modifier = modifier.size(size).background(colors.background, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = initial.take(2),
            color = colors.content,
            fontWeight = FontWeight.Bold,
            fontSize = (size.value * 0.45f).sp,
            maxLines = 1,
        )
    }
}

/**
 * Menú del mesero activo en la barra superior, siempre visible (§3.2, «Cambio rápido de usuario»):
 * su distintivo y su nombre; al tocarlo, «Cambiar de mesero» regresa a la lista de meseros. Cada
 * mesero conserva su propia sesión en el dispositivo. Con [onAlertsHelp] (etapa A2), también
 * «Avisos de platillos listos», la ayuda del aviso.
 */
@Composable
fun ActiveWaiterMenu(
    name: String,
    initial: String,
    colorHex: String?,
    onSwitchWaiter: () -> Unit,
    modifier: Modifier = Modifier,
    offline: Boolean = false,
    onAlertsHelp: (() -> Unit)? = null,
    compact: Boolean = false,
) {
    var open by remember { mutableStateOf(false) }
    Box(modifier = modifier) {
        TextButton(
            onClick = { open = true },
            modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget).testTag(TAG_WAITER_MENU),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                WaiterBadge(initial = initial, colorHex = colorHex, size = 32.dp)
                // Etapa M1 (§5.6): en el teléfono vertical solo la inicial (el nombre lo dicen la
                // descripción y el menú); así caben el título y «N pedidos por enviar».
                if (compact) {
                    Spacer(Modifier.semantics { contentDescription = name })
                } else {
                    Text(
                        text = name,
                        style = MaterialTheme.typography.titleSmall,
                        color = AdelTheme.colors.text,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Icon(
                    imageVector = AdelIcons.ChevronDown,
                    contentDescription = null,
                    tint = AdelTheme.colors.text,
                    modifier = Modifier.size(18.dp),
                )
            }
        }
        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        ) {
            if (offline) {
                Text(
                    text = stringResource(R.string.waiter_menu_offline),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                )
            }
            DropdownMenuItem(
                text = { Text(stringResource(R.string.waiter_menu_switch)) },
                leadingIcon = { Icon(AdelIcons.Users, contentDescription = null) },
                onClick = {
                    open = false
                    onSwitchWaiter()
                },
                modifier =
                    Modifier.heightIn(min = AdelDimens.MinTouchTarget).testTag(TAG_WAITER_SWITCH),
            )
            // Etapa A2: la ayuda «Avisos de platillos listos».
            if (onAlertsHelp != null) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.waiter_menu_alerts)) },
                    leadingIcon = { Icon(AdelIcons.Alert, contentDescription = null) },
                    onClick = {
                        open = false
                        onAlertsHelp()
                    },
                    modifier =
                        Modifier.heightIn(min = AdelDimens.MinTouchTarget)
                            .testTag(TAG_WAITER_ALERTS_HELP),
                )
            }
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_WAITER_MENU = "waiter-menu"
const val TAG_WAITER_SWITCH = "waiter-menu-switch"
const val TAG_WAITER_ALERTS_HELP = "waiter-menu-alerts-help"

/**
 * Etapa M1 (§5.6): «N pedidos por enviar» de la barra en el teléfono vertical, en dos renglones si
 * hace falta. En la barra común el texto va en uno solo y, junto al título, al mesero y a «En
 * línea», se cortaba («2 pedidos por en…»). La raíz lo pone en lugar del de la barra común cuando
 * la barra no tiene espacio (`currentTopBarHasRoom`); el texto es el mismo.
 */
@Composable
fun PendingOrdersCompact(count: Int, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    Row(
        modifier =
            modifier
                .heightIn(min = 32.dp)
                .background(colors.amberSoft, RoundedCornerShape(16.dp))
                .padding(horizontal = 8.dp, vertical = 2.dp)
                .testTag(TAG_PENDING_ORDERS),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Icon(
            imageVector = AdelIcons.Send,
            contentDescription = null,
            tint = colors.amber,
            modifier = Modifier.size(16.dp),
        )
        Text(
            text =
                pluralStringResource(
                    com.adelpos.core.ui.R.plurals.connection_pending_orders,
                    count,
                    count,
                ),
            style = MaterialTheme.typography.labelMedium,
            color = colors.amber,
            maxLines = 2,
            modifier = Modifier.widthIn(max = PENDING_COMPACT_TEXT_WIDTH),
        )
    }
}

/** Ancho del texto de [PendingOrdersCompact]: «2 pedidos» arriba y «por enviar» abajo. */
private val PENDING_COMPACT_TEXT_WIDTH = 76.dp
