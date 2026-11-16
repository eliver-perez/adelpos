package local.adel.pos.android.core.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

// Tipografía del sistema un poco más grande que la de Material para leerse a distancia en una
// tableta sobre la mesa o en la mano del mesero: cuerpo de 16–18 sp y botones de 16 sp. Respeta el
// tamaño de letra del sistema (sp).
private val Default = Typography()

internal val AdelTypography =
    Typography(
        headlineMedium = Default.headlineMedium.copy(fontWeight = FontWeight.SemiBold),
        headlineSmall = Default.headlineSmall.copy(fontWeight = FontWeight.SemiBold),
        titleLarge =
            Default.titleLarge.copy(
                fontSize = 24.sp,
                lineHeight = 30.sp,
                fontWeight = FontWeight.SemiBold,
            ),
        titleMedium =
            Default.titleMedium.copy(
                fontSize = 18.sp,
                lineHeight = 24.sp,
                fontWeight = FontWeight.SemiBold,
            ),
        bodyLarge = Default.bodyLarge.copy(fontSize = 18.sp, lineHeight = 26.sp),
        bodyMedium = Default.bodyMedium.copy(fontSize = 16.sp, lineHeight = 22.sp),
        bodySmall = Default.bodySmall.copy(fontSize = 14.sp, lineHeight = 20.sp),
        labelLarge =
            Default.labelLarge.copy(
                fontSize = 16.sp,
                lineHeight = 22.sp,
                fontWeight = FontWeight.SemiBold,
            ),
        labelMedium = Default.labelMedium.copy(fontSize = 14.sp, lineHeight = 20.sp),
    )

/** Estilo de los importes: en el color normal del texto y en negritas, como el escritorio. */
val AmountTextStyle: TextStyle
    get() = AdelTypography.bodyLarge.copy(fontWeight = FontWeight.Bold)
