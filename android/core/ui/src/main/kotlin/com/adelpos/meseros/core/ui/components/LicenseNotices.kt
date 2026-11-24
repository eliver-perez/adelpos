package com.adelpos.meseros.core.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.license.ClockNotice
import com.adelpos.meseros.core.domain.license.GraceNotice
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.icons.AdelIcons
import com.adelpos.meseros.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.theme.AdelTheme

/**
 * Etapa L1 (§6.2 de la especificación): el **aviso fijo** de la hora incorrecta de la computadora
 * principal, debajo de la barra en todas las pantallas mientras Go diga que la hora está bloqueada.
 * Dice lo mismo que el rechazo de una operación, para que nadie lo confunda con la licencia, la
 * conexión o los permisos, y, si Go las dio, las dos fechas.
 */
@Composable
fun LicenseClockBanner(notice: ClockNotice, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    Surface(
        color = colors.redSoft,
        contentColor = colors.text,
        modifier =
            modifier.fillMaxWidth().testTag(TAG_LICENSE_CLOCK_BANNER).semantics(
                mergeDescendants = true,
            ) {
                liveRegion = LiveRegionMode.Polite
            },
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                AdelIcons.Alert,
                contentDescription = null,
                tint = colors.red,
                modifier = Modifier.size(24.dp),
            )
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    notice.text,
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.testTag(TAG_LICENSE_CLOCK_TEXT),
                )
                notice.detail?.let { detail ->
                    Text(
                        detail,
                        style = MaterialTheme.typography.bodySmall,
                        color = colors.muted,
                        modifier = Modifier.testTag(TAG_LICENSE_CLOCK_DETAIL),
                    )
                }
            }
        }
    }
}

/**
 * Etapa L1 (§8): el **aviso discreto** de la barra con la licencia en tolerancia: cuántos días
 * quedan antes de que se bloquee la captura. Con poco ancho (el teléfono en vertical) solo el
 * ícono, para no quitarle lugar al título; al tocarlo, el texto completo. La zona táctil es de 48
 * dp aunque se vea más pequeño (regla 12).
 */
@Composable
fun LicenseGraceBadge(notice: GraceNotice, showText: Boolean, modifier: Modifier = Modifier) {
    val colors = AdelTheme.colors
    var open by rememberSaveable { mutableStateOf(false) }
    Box(
        modifier =
            modifier
                .sizeIn(minWidth = AdelDimens.MinTouchTarget, minHeight = AdelDimens.MinTouchTarget)
                .clickable(role = Role.Button) { open = true }
                .semantics(mergeDescendants = true) { contentDescription = notice.short }
                .testTag(TAG_LICENSE_GRACE_BADGE),
        contentAlignment = Alignment.Center,
    ) {
        Row(
            modifier =
                Modifier.heightIn(min = 32.dp)
                    .background(colors.amberSoft, RoundedCornerShape(16.dp))
                    .padding(horizontal = if (showText) 10.dp else 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(
                AdelIcons.Clock,
                contentDescription = null,
                tint = colors.amber,
                modifier = Modifier.size(20.dp),
            )
            if (showText) {
                Text(
                    notice.short,
                    style = MaterialTheme.typography.labelLarge,
                    color = colors.text,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.testTag(TAG_LICENSE_GRACE_TEXT),
                )
            }
        }
    }
    if (open) {
        AlertDialog(
            onDismissRequest = { open = false },
            title = { Text(stringResource(R.string.license_grace_title)) },
            text = { Text(notice.long, modifier = Modifier.testTag(TAG_LICENSE_GRACE_DETAIL)) },
            confirmButton = {
                TextButton(
                    onClick = { open = false },
                    modifier = Modifier.heightIn(min = AdelDimens.MinTouchTarget),
                ) {
                    Text(stringResource(R.string.license_grace_ok))
                }
            },
            modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
        )
    }
}

const val TAG_LICENSE_CLOCK_BANNER = "license-clock-banner"
const val TAG_LICENSE_CLOCK_TEXT = "license-clock-text"
const val TAG_LICENSE_CLOCK_DETAIL = "license-clock-detail"
const val TAG_LICENSE_GRACE_BADGE = "license-grace-badge"
const val TAG_LICENSE_GRACE_TEXT = "license-grace-text"
const val TAG_LICENSE_GRACE_DETAIL = "license-grace-detail"
