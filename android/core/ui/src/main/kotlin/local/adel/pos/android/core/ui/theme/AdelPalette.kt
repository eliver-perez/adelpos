package local.adel.pos.android.core.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance

// ÚNICO archivo con colores escritos de la aplicación Android, como los bloques :root y
// :root[data-theme='dark'] de desktop/src/styles.css en el escritorio (etapa 9D). Los valores son
// los mismos tokens del escritorio y con el mismo significado: azul = botones principales,
// selección, mesa Ocupada y «En línea»; ámbar = Esperando pago y advertencias; uva = Pago parcial;
// gris = Libre y Fuera de servicio (con candado); turquesa = información; rojo = acciones
// destructivas, errores y «Sin conexión». Un color nuevo es un token aquí, en ambos temas; las
// pantallas usan MaterialTheme.colorScheme o AdelTheme.colors, nunca un color propio
// (ColorsOnlyInPaletteTest lo comprueba).

/** Azul de la marca AD-EL POS; igual en ambos temas. */
internal val BrandBlue = Color(0xFF1F5FA8)

/** Azul de la marca presionado o activo. */
internal val BrandBluePressed = Color(0xFF173D6E)

/** Azul claro del logotipo. */
internal val BrandBlueLight = Color(0xFF4090F0)

/**
 * Máscara de los íconos vectoriales: el trazo se dibuja con este color y
 * [androidx.compose.material3.Icon] lo tiñe con el color del contenido o el que se le indique.
 * Nunca se ve tal cual.
 */
internal val IconMask = Color(0xFF000000)

/** Colores de un estado de mesa: relleno, borde y texto (--table-*-bg, -border y -text). */
@Immutable data class TableStateColors(val background: Color, val border: Color, val content: Color)

/** Tokens de AD-EL POS de un tema (claro u oscuro), con los nombres de styles.css. */
@Immutable
data class AdelColors(
    val isDark: Boolean,
    // Neutros.
    val background: Color,
    val panel: Color,
    val text: Color,
    val muted: Color,
    val border: Color,
    val subtle: Color,
    val banner: Color,
    // Paleta de estado (§0-B de la 9D): cada tono con su versión suave.
    val blue: Color,
    val blueStrong: Color,
    val blueSoft: Color,
    val blueBorder: Color,
    val amber: Color,
    val amberSoft: Color,
    val grape: Color,
    val grapeSoft: Color,
    val gray: Color,
    val graySoft: Color,
    val teal: Color,
    val tealSoft: Color,
    val red: Color,
    val redSoft: Color,
    // Usos.
    val onAccent: Color,
    val warningMark: Color,
    val dangerBorder: Color,
    val dangerContrast: Color,
    val backdrop: Color,
    // Estados de mesa: Libre blanco con borde gris, Ocupada azul claro, Esperando pago ámbar, Pago
    // parcial uva y Fuera de servicio gris tenue rayado (nunca rojo).
    val tableFree: TableStateColors,
    val tableOccupied: TableStateColors,
    val tableAwaitingPayment: TableStateColors,
    val tablePartialPayment: TableStateColors,
    val tableOutOfService: TableStateColors,
    val tableOutOfServiceStripe: Color,
) {
    /** Indicador «En línea» (--status-online). */
    val statusOnline: Color
        get() = blue

    /** Indicador «Sin conexión» (--danger). */
    val statusOffline: Color
        get() = red
}

/** Tema claro: bloque :root de styles.css. */
internal val LightAdelColors =
    AdelColors(
        isDark = false,
        background = Color(0xFFF6F7F9),
        panel = Color(0xFFFFFFFF),
        text = Color(0xFF202B3E),
        muted = Color(0xFF5F6D80),
        border = Color(0xFFE5E9EF),
        subtle = Color(0xFFFAFBFD),
        banner = Color(0xFFEEF2F7),
        blue = BrandBlue,
        blueStrong = BrandBluePressed,
        blueSoft = Color(0xFFE7EEF7),
        blueBorder = Color(0xFFA9C2E2),
        amber = Color(0xFF9A5B0C),
        amberSoft = Color(0xFFFBF3E6),
        grape = Color(0xFF7B3FA6),
        grapeSoft = Color(0xFFF3EAF9),
        gray = Color(0xFF5A6878),
        graySoft = Color(0xFFEEF1F5),
        teal = Color(0xFF0B737A),
        tealSoft = Color(0xFFE5F5F5),
        red = Color(0xFFB42318),
        redSoft = Color(0xFFFDECEA),
        onAccent = Color(0xFFFFFFFF),
        warningMark = Color(0xFFC27C0E),
        dangerBorder = Color(0xFFEDA39B),
        dangerContrast = Color(0xFFFFFFFF),
        // --backdrop: rgb(15 23 32 / 45%).
        backdrop = Color(0x730F1720),
        tableFree =
            TableStateColors(
                background = Color(0xFFFFFFFF),
                border = Color(0xFF8691A0),
                content = Color(0xFF202B3E),
            ),
        tableOccupied =
            TableStateColors(
                background = Color(0xFFDCE8F6),
                border = Color(0xFF3F7FCB),
                content = BrandBluePressed,
            ),
        tableAwaitingPayment =
            TableStateColors(
                background = Color(0xFFF3C76F),
                border = Color(0xFFA86A12),
                content = Color(0xFF4A2F05),
            ),
        tablePartialPayment =
            TableStateColors(
                background = Color(0xFFC9A6E8),
                border = Color(0xFF7B3FA6),
                content = Color(0xFF2E1048),
            ),
        tableOutOfService =
            TableStateColors(
                background = Color(0xFFE4E7EB),
                border = Color(0xFF98A3B0),
                content = Color(0xFF4F5B69),
            ),
        tableOutOfServiceStripe = Color(0xFFCFD5DC),
    )

/**
 * Tema oscuro: bloque :root[data-theme='dark'] de styles.css (lo que no redefine se hereda del
 * claro).
 */
internal val DarkAdelColors =
    AdelColors(
        isDark = true,
        background = Color(0xFF121922),
        panel = Color(0xFF19232E),
        text = Color(0xFFE4EBF2),
        muted = Color(0xFF9BAABD),
        border = Color(0xFF2A3543),
        subtle = Color(0xFF16202A),
        banner = Color(0xFF1E2A38),
        // El azul del logotipo aclarado para llegar a AA sobre el panel, con texto oscuro encima.
        blue = Color(0xFF529BF2),
        blueStrong = Color(0xFF8DBDF6),
        blueSoft = Color(0xFF1B2C42),
        blueBorder = Color(0xFF2F5684),
        amber = Color(0xFFE6B15F),
        amberSoft = Color(0xFF3A2F1C),
        grape = Color(0xFFC7A2EC),
        grapeSoft = Color(0xFF33264A),
        gray = Color(0xFFA9B6C6),
        graySoft = Color(0xFF243140),
        teal = Color(0xFF4FC6CC),
        tealSoft = Color(0xFF163A3D),
        red = Color(0xFFF08C80),
        redSoft = Color(0xFF3A2422),
        onAccent = Color(0xFF0B1626),
        warningMark = Color(0xFFE6B15F),
        dangerBorder = Color(0xFF6B3530),
        dangerContrast = Color(0xFF1A0F0E),
        backdrop = Color(0x730F1720),
        tableFree =
            TableStateColors(
                background = Color(0xFF19232E),
                border = Color(0xFF5F6E84),
                content = Color(0xFFE4EBF2),
            ),
        tableOccupied =
            TableStateColors(
                background = Color(0xFF1D3A5F),
                border = Color(0xFF529BF2),
                content = Color(0xFFDCEAFB),
            ),
        tableAwaitingPayment =
            TableStateColors(
                background = Color(0xFF7D5C12),
                border = Color(0xFFE6B15F),
                content = Color(0xFFFFF0CC),
            ),
        tablePartialPayment =
            TableStateColors(
                background = Color(0xFF4B2C6E),
                border = Color(0xFFC7A2EC),
                content = Color(0xFFF5EBFF),
            ),
        tableOutOfService =
            TableStateColors(
                background = Color(0xFF1D252F),
                border = Color(0xFF56637A),
                content = Color(0xFF9BAABD),
            ),
        tableOutOfServiceStripe = Color(0xFF27313D),
    )

/** Colores del distintivo de un mesero: el círculo y la inicial encima. */
@Immutable data class BadgeColors(val background: Color, val content: Color)

/**
 * Distintivo de un mesero con el color que eligió el administrador en la paleta de Go (`GET
 * users/badge-colors`, la misma de las categorías: `catalog.Palette`), que llega como dato
 * («#3591d4»). Es el único lugar donde un color se construye a partir de un dato. La inicial va en
 * el tono (blanco o el texto oscuro del tema claro) que más contraste da con ese color, para que se
 * lea en ambos temas; sin color (o con uno que no es hexadecimal), el azul suave de la marca.
 */
fun waiterBadgeColors(hex: String?, colors: AdelColors): BadgeColors {
    val parsed = hex?.let(::colorFromHex) ?: return BadgeColors(colors.blueSoft, colors.blueStrong)
    val light = Color(0xFFFFFFFF)
    val dark = LightAdelColors.text
    val content = if (contrastRatio(light, parsed) >= contrastRatio(dark, parsed)) light else dark
    return BadgeColors(parsed, content)
}

/** Contraste WCAG entre dos colores (1 a 21). */
fun contrastRatio(first: Color, second: Color): Double {
    val lighter = maxOf(first.luminance(), second.luminance()) + 0.05
    val darker = minOf(first.luminance(), second.luminance()) + 0.05
    return lighter / darker
}

// «#rrggbb» en minúsculas o mayúsculas; null si no tiene esa forma.
private fun colorFromHex(hex: String): Color? {
    if (!Regex("^#[0-9a-fA-F]{6}$").matches(hex)) {
        return null
    }
    val value = hex.substring(1).toLong(16)
    return Color(0xFF000000 or value)
}
