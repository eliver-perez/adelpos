package local.adel.pos.android.core.ui.theme

import androidx.compose.ui.unit.dp

/**
 * Medidas táctiles comunes (§2): botones de 48 dp como mínimo y 56 dp en las acciones principales.
 */
object AdelDimens {
    /** Alto y ancho mínimos de cualquier control que se toca. */
    val MinTouchTarget = 48.dp

    /** Alto de las acciones principales («Entrar», «Enviar a cocina»). */
    val PrimaryActionHeight = 56.dp

    /** Margen de las pantallas. */
    val ScreenPadding = 16.dp

    /** Separación entre elementos. */
    val Gap = 12.dp
}
