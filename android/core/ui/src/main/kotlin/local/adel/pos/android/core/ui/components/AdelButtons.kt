package local.adel.pos.android.core.ui.components

import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.ui.theme.AdelDimens

/**
 * Acción principal de una pantalla o panel («Entrar», «Enviar a cocina»): relleno azul y 56 dp de
 * alto. Un solo botón principal por panel, como en el escritorio.
 */
@Composable
fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    icon: ImageVector? = null,
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.heightIn(min = AdelDimens.PrimaryActionHeight),
    ) {
        ButtonContent(text, icon)
    }
}

/** Acción secundaria: contorno y 48 dp de alto como mínimo. */
@Composable
fun SecondaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    icon: ImageVector? = null,
) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.heightIn(min = AdelDimens.MinTouchTarget),
    ) {
        ButtonContent(text, icon)
    }
}

// Ícono a la izquierda (decorativo) y el texto, como IconLabel del escritorio.
@Composable
private fun ButtonContent(text: String, icon: ImageVector?) {
    if (icon != null) {
        Icon(imageVector = icon, contentDescription = null, modifier = Modifier.size(22.dp))
        Spacer(Modifier.width(8.dp))
    }
    Text(text)
}
