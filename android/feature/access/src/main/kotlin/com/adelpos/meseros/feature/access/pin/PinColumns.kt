package com.adelpos.meseros.feature.access.pin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import com.adelpos.meseros.core.ui.components.AdelPinPadDefaults
import com.adelpos.meseros.core.ui.theme.AdelDimens

/**
 * El PIN con poca altura (etapa A3, §4: el teléfono en horizontal, ≈ 411 dp de alto): en una sola
 * columna el teclado no cabía y solo se veían las teclas 1 a 6. En dos columnas: a la izquierda
 * [info] (quién entra, los puntos, los avisos y sus botones; se desliza si no cabe) y a la derecha
 * [pad], el teclado completo con las teclas tan altas como quepan
 * ([AdelPinPadDefaults.keyHeightToFit], nunca menos que el mínimo táctil; si ni así cabe, se
 * desliza). La entrada y «Vuelve a escribir tu PIN» la usan igual.
 */
@Composable
internal fun PinColumns(
    modifier: Modifier,
    info: @Composable ColumnScope.() -> Unit,
    pad: @Composable (keyHeight: Dp) -> Unit,
) {
    Row(modifier = modifier, horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
        Column(
            modifier = Modifier.weight(1f).fillMaxHeight().verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
            content = info,
        )
        BoxWithConstraints(
            modifier = Modifier.weight(1f).fillMaxHeight(),
            contentAlignment = Alignment.Center,
        ) {
            val keyHeight = AdelPinPadDefaults.keyHeightToFit(maxHeight)
            Box(modifier = Modifier.verticalScroll(rememberScrollState())) { pad(keyHeight) }
        }
    }
}
