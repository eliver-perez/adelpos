package com.adelpos.meseros.core.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf

private val LocalAdelColors = staticCompositionLocalOf { LightAdelColors }

/**
 * Tema de AD-EL POS para Android (Material 3), claro u oscuro. Por omisión sigue el tema del
 * sistema. Los roles de Material se llenan con los tokens del escritorio para que ningún componente
 * use la paleta de muestra de Material.
 */
@Composable
fun AdelTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors = if (darkTheme) DarkAdelColors else LightAdelColors
    CompositionLocalProvider(LocalAdelColors provides colors) {
        MaterialTheme(
            colorScheme = materialColors(colors),
            typography = AdelTypography,
            content = content,
        )
    }
}

/**
 * Acceso a los tokens de AD-EL POS que Material no tiene (estados de mesa, ámbar, uva, indicador de
 * conexión).
 */
object AdelTheme {
    val colors: AdelColors
        @Composable @ReadOnlyComposable get() = LocalAdelColors.current
}

// Correspondencia de los tokens con los roles de Material 3. Azul de la marca para lo principal,
// gris para lo secundario, turquesa para lo informativo y rojo para errores; superficies con los
// neutros del escritorio.
internal fun materialColors(colors: AdelColors): ColorScheme =
    if (colors.isDark) {
        darkColorScheme(
            primary = colors.blue,
            onPrimary = colors.onAccent,
            primaryContainer = colors.blueSoft,
            onPrimaryContainer = colors.blueStrong,
            inversePrimary = BrandBlue,
            secondary = colors.gray,
            onSecondary = colors.background,
            secondaryContainer = colors.graySoft,
            onSecondaryContainer = colors.text,
            tertiary = colors.teal,
            onTertiary = colors.background,
            tertiaryContainer = colors.tealSoft,
            onTertiaryContainer = colors.teal,
            background = colors.background,
            onBackground = colors.text,
            surface = colors.panel,
            onSurface = colors.text,
            surfaceVariant = colors.graySoft,
            onSurfaceVariant = colors.muted,
            surfaceTint = colors.blue,
            inverseSurface = colors.text,
            inverseOnSurface = colors.panel,
            error = colors.red,
            onError = colors.dangerContrast,
            errorContainer = colors.redSoft,
            onErrorContainer = colors.red,
            outline = colors.tableFree.border,
            outlineVariant = colors.border,
            scrim = colors.backdrop,
            surfaceBright = colors.banner,
            surfaceDim = colors.background,
            surfaceContainerLowest = colors.background,
            surfaceContainerLow = colors.subtle,
            surfaceContainer = colors.panel,
            surfaceContainerHigh = colors.banner,
            surfaceContainerHighest = colors.graySoft,
        )
    } else {
        lightColorScheme(
            primary = colors.blue,
            onPrimary = colors.onAccent,
            primaryContainer = colors.blueSoft,
            onPrimaryContainer = colors.blueStrong,
            inversePrimary = BrandBlueLight,
            secondary = colors.gray,
            onSecondary = colors.panel,
            secondaryContainer = colors.graySoft,
            onSecondaryContainer = colors.text,
            tertiary = colors.teal,
            onTertiary = colors.panel,
            tertiaryContainer = colors.tealSoft,
            onTertiaryContainer = colors.teal,
            background = colors.background,
            onBackground = colors.text,
            surface = colors.panel,
            onSurface = colors.text,
            surfaceVariant = colors.graySoft,
            onSurfaceVariant = colors.muted,
            surfaceTint = colors.blue,
            inverseSurface = colors.text,
            inverseOnSurface = colors.panel,
            error = colors.red,
            onError = colors.dangerContrast,
            errorContainer = colors.redSoft,
            onErrorContainer = colors.red,
            outline = colors.tableFree.border,
            outlineVariant = colors.border,
            scrim = colors.backdrop,
            surfaceBright = colors.panel,
            surfaceDim = colors.background,
            surfaceContainerLowest = colors.panel,
            surfaceContainerLow = colors.subtle,
            surfaceContainer = colors.background,
            surfaceContainerHigh = colors.banner,
            surfaceContainerHighest = colors.graySoft,
        )
    }
