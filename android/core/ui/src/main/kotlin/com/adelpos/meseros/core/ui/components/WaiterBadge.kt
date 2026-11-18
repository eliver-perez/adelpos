package com.adelpos.meseros.core.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.ui.theme.waiterBadgeColors

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
 * mesero conserva su propia sesión en el dispositivo.
 */
@Composable
fun ActiveWaiterMenu(
    name: String,
    initial: String,
    colorHex: String?,
    onSwitchWaiter: () -> Unit,
    modifier: Modifier = Modifier,
    offline: Boolean = false,
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
                Text(
                    text = name,
                    style = MaterialTheme.typography.titleSmall,
                    color = AdelTheme.colors.text,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
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
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_WAITER_MENU = "waiter-menu"
const val TAG_WAITER_SWITCH = "waiter-menu-switch"
