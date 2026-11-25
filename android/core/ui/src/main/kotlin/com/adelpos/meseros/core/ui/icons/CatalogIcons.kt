package com.adelpos.meseros.core.ui.icons

import androidx.compose.ui.graphics.vector.ImageVector
import com.adelpos.core.ui.icons.AdelIcons

/**
 * Íconos del catálogo y del personalizador, con los trazos de `desktop/src/components/Icon.tsx`
 * (`search`, `close`, `plus`) y el mismo estilo de [AdelIcons]. Decorativos: el control siempre
 * tiene su texto o su descripción.
 */
object CatalogIcons {
    val Search: ImageVector by lazy {
        AdelIcons.icon("search", AdelIcons.circle(10.5f, 10.5f, 6.5f), "m15.5 15.5 5 5")
    }

    val Close: ImageVector by lazy { AdelIcons.icon("close", "M6 6l12 12M18 6 6 18") }

    val Plus: ImageVector by lazy { AdelIcons.icon("plus", "M12 5v14M5 12h14") }

    /** «−» del escritorio (el trazo horizontal de `plus`). */
    val Minus: ImageVector by lazy { AdelIcons.icon("minus", "M5 12h14") }
}
