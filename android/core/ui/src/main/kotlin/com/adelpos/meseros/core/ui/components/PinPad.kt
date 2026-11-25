package com.adelpos.meseros.core.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.R

/**
 * Teclado numérico grande para el PIN (§3.2), compartido con la etapa A2: puntos que dicen cuántos
 * dígitos se escribieron (nunca cuáles), teclas de 0 a 9 de [AdelPinPadDefaults.KeyHeight] de alto,
 * «Borrar» y la acción principal ([submitLabel], «Ingresar») que se habilita desde [minLength]
 * dígitos. Quien lo usa guarda el PIN solo en memoria: nunca en el estado que el sistema escribe a
 * disco al cerrar la aplicación.
 *
 * Etapa A3 (§4, teléfono horizontal): con poca altura quien lo usa pone los puntos aparte
 * ([showDots] en falso y [PinPadDots] a un lado) y elige [keyHeight] con
 * [AdelPinPadDefaults.keyHeightToFit] para que las cuatro filas quepan completas.
 */
@Composable
fun PinPad(
    length: Int,
    onDigit: (Char) -> Unit,
    onDelete: () -> Unit,
    onSubmit: () -> Unit,
    submitLabel: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    minLength: Int = AdelPinPadDefaults.MIN_LENGTH,
    maxLength: Int = AdelPinPadDefaults.MAX_LENGTH,
    showDots: Boolean = true,
    keyHeight: Dp = AdelPinPadDefaults.KeyHeight,
    gap: Dp = AdelPinPadDefaults.Gap,
) {
    Column(
        modifier = modifier.widthIn(max = AdelPinPadDefaults.MaxWidth).testTag(TAG_PIN_PAD),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(gap),
    ) {
        if (showDots) {
            PinPadDots(length = length, maxLength = maxLength)
        }
        val rows = listOf("123", "456", "789")
        for (row in rows) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(gap),
            ) {
                for (digit in row) {
                    DigitKey(digit, enabled && length < maxLength, keyHeight, onDigit)
                }
            }
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(gap),
        ) {
            OutlinedButton(
                onClick = onDelete,
                enabled = enabled && length > 0,
                shape = RoundedCornerShape(12.dp),
                contentPadding = KEY_PADDING,
                modifier = Modifier.weight(1f).heightIn(min = keyHeight).testTag(TAG_PIN_DELETE),
            ) {
                Text(
                    stringResource(R.string.pin_pad_delete),
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                )
            }
            DigitKey('0', enabled && length < maxLength, keyHeight, onDigit)
            Button(
                onClick = onSubmit,
                enabled = enabled && length >= minLength,
                shape = RoundedCornerShape(12.dp),
                contentPadding = KEY_PADDING,
                modifier = Modifier.weight(1f).heightIn(min = keyHeight).testTag(TAG_PIN_SUBMIT),
            ) {
                Text(submitLabel, style = MaterialTheme.typography.titleMedium, maxLines = 1)
            }
        }
    }
}

/** Medidas del teclado del PIN. */
object AdelPinPadDefaults {
    const val MIN_LENGTH = 4
    const val MAX_LENGTH = 6

    /** Alto de cada tecla: mayor que el mínimo táctil, para teclear rápido sin mirar. */
    val KeyHeight = 72.dp

    /** Ancho máximo del teclado: en la tableta no se estira a toda la pantalla. */
    val MaxWidth = 420.dp

    /** Espacio entre teclas y filas. */
    val Gap = 12.dp

    /** Espacio entre teclas con poca altura (teléfono horizontal). */
    val CompactGap = 8.dp

    /**
     * El alto de cada tecla para que las cuatro filas (sin los puntos) quepan en [available] con
     * [gap] entre ellas: nunca más que [KeyHeight] ni menos que el mínimo táctil
     * ([AdelDimens.MinTouchTarget]). Si ni así caben, quien lo usa desplaza el teclado.
     */
    fun keyHeightToFit(available: Dp, gap: Dp = CompactGap): Dp =
        ((available - gap * (ROWS - 1)) / ROWS).coerceIn(AdelDimens.MinTouchTarget, KeyHeight)

    private const val ROWS = 4
}

@Composable
private fun RowScope.DigitKey(
    digit: Char,
    enabled: Boolean,
    keyHeight: Dp,
    onDigit: (Char) -> Unit,
) {
    FilledTonalButton(
        onClick = { onDigit(digit) },
        enabled = enabled,
        shape = RoundedCornerShape(12.dp),
        contentPadding = KEY_PADDING,
        modifier =
            Modifier.weight(1f).heightIn(min = keyHeight).testTag("$TAG_PIN_KEY_PREFIX$digit"),
    ) {
        Text(
            text = digit.toString(),
            fontSize = 28.sp,
            fontWeight = FontWeight.SemiBold,
        )
    }
}

/**
 * Puntos del PIN: llenos los escritos y vacíos los que faltan para el máximo. Dentro de [PinPad] o,
 * con poca altura, a un lado del teclado.
 */
@Composable
fun PinPadDots(
    length: Int,
    modifier: Modifier = Modifier,
    maxLength: Int = AdelPinPadDefaults.MAX_LENGTH,
) {
    val description = pluralStringResource(R.plurals.pin_pad_digits, length, length)
    Row(
        modifier =
            modifier.heightIn(min = 32.dp).testTag(TAG_PIN_DOTS).semantics {
                contentDescription = description
            },
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val color = MaterialTheme.colorScheme.onSurface
        for (index in 0 until maxLength) {
            val filled = index < length
            Box(
                modifier =
                    Modifier.size(18.dp)
                        .then(
                            if (filled) {
                                Modifier.background(color, CircleShape)
                            } else {
                                Modifier.border(
                                    2.dp,
                                    MaterialTheme.colorScheme.outline,
                                    CircleShape,
                                )
                            },
                        ),
            )
        }
    }
}

// Margen interior de las teclas: el de Material (24 dp a cada lado) partía «Ingresar» y «Borrar» en
// dos renglones en las teclas angostas (el diálogo «Vuelve a escribir tu PIN» en el teléfono).
private val KEY_PADDING = PaddingValues(horizontal = 8.dp)

/** Etiquetas para las pruebas. */
const val TAG_PIN_PAD = "pin-pad"
const val TAG_PIN_DOTS = "pin-dots"
const val TAG_PIN_KEY_PREFIX = "pin-key-"
const val TAG_PIN_DELETE = "pin-key-delete"
const val TAG_PIN_SUBMIT = "pin-key-submit"
