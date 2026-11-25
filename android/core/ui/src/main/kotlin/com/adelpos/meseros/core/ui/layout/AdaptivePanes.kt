package com.adelpos.meseros.core.ui.layout

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.components.SecondaryButton
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.meseros.core.ui.R
import com.adelpos.meseros.core.ui.components.LocalTopBarBack

/**
 * Andamio de uno o dos paneles, compartido por el modo Mesero y el modo Para llevar de la A2.
 * - [PaneLayout.DOUBLE]: [primary] a la izquierda y [secondary] a la derecha, siempre visibles.
 * - [PaneLayout.SINGLE]: un panel a la vez. Con [showSecondary] en falso se ve [primary] y, abajo,
 *   el botón [openSecondaryLabel] que abre el otro panel «a un toque»; con verdadero se ve
 *   [secondary] con «Volver a …» arriba, y el gesto o botón Atrás del sistema (también el atrás
 *   predictivo de Android 16) regresa a [primary].
 *
 * El estado [showSecondary] lo guarda quien llama (en su ViewModel con SavedStateHandle) para
 * sobrevivir a la rotación y a que el sistema cierre la aplicación en segundo plano.
 *
 * Etapa A3 (§4.1): si la raíz provee la flecha de la barra superior ([LocalTopBarBack]), el panel
 * abierto se registra ahí y no dibuja su propio «Volver a …» (una sola flecha). Con
 * [openSecondaryLabel] en null no hay botón para abrirlo: quien llama lo abre al tocar algo (Mesas:
 * tocar la mesa abre su panel, §4.7).
 */
@Composable
fun AdaptivePanes(
    layout: PaneLayout,
    showSecondary: Boolean,
    onShowSecondaryChange: (Boolean) -> Unit,
    primaryTitle: String,
    openSecondaryLabel: String?,
    primary: @Composable () -> Unit,
    secondary: @Composable () -> Unit,
    modifier: Modifier = Modifier,
) {
    when (layout) {
        PaneLayout.DOUBLE -> {
            Row(modifier = modifier.fillMaxSize().testTag(TAG_PANES_DOUBLE)) {
                Box(modifier = Modifier.weight(0.58f).fillMaxHeight().testTag(TAG_PANE_PRIMARY)) {
                    primary()
                }
                VerticalDivider()
                Surface(
                    modifier = Modifier.weight(0.42f).fillMaxHeight().testTag(TAG_PANE_SECONDARY),
                ) {
                    secondary()
                }
            }
        }
        PaneLayout.SINGLE -> {
            if (showSecondary) {
                BackHandler { onShowSecondaryChange(false) }
                val topBarBack = LocalTopBarBack.current
                if (topBarBack != null) {
                    DisposableEffect(topBarBack) {
                        topBarBack.panelOpened()
                        onDispose { topBarBack.panelClosed() }
                    }
                }
                Column(modifier = modifier.fillMaxSize().testTag(TAG_PANES_SINGLE)) {
                    if (topBarBack == null) {
                        TextButton(
                            onClick = { onShowSecondaryChange(false) },
                            modifier =
                                Modifier.padding(horizontal = 8.dp)
                                    .heightIn(min = AdelDimens.MinTouchTarget)
                                    .testTag(TAG_PANE_BACK),
                        ) {
                            Icon(
                                AdelIcons.ChevronLeft,
                                contentDescription = null,
                                modifier = Modifier.size(22.dp),
                            )
                            Spacer(Modifier.width(4.dp))
                            Text(stringResource(R.string.panes_back, primaryTitle))
                        }
                        HorizontalDivider()
                    }
                    Box(modifier = Modifier.weight(1f).fillMaxWidth().testTag(TAG_PANE_SECONDARY)) {
                        secondary()
                    }
                }
            } else {
                Column(modifier = modifier.fillMaxSize().testTag(TAG_PANES_SINGLE)) {
                    Box(modifier = Modifier.weight(1f).fillMaxWidth().testTag(TAG_PANE_PRIMARY)) {
                        primary()
                    }
                    if (openSecondaryLabel != null) {
                        SecondaryButton(
                            text = openSecondaryLabel,
                            onClick = { onShowSecondaryChange(true) },
                            modifier =
                                Modifier.fillMaxWidth()
                                    .padding(AdelDimens.ScreenPadding)
                                    .testTag(TAG_PANE_OPEN_SECONDARY),
                        )
                    }
                }
            }
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_PANES_DOUBLE = "panes-double"
const val TAG_PANES_SINGLE = "panes-single"
const val TAG_PANE_PRIMARY = "pane-primary"
const val TAG_PANE_SECONDARY = "pane-secondary"
const val TAG_PANE_OPEN_SECONDARY = "pane-open-secondary"
const val TAG_PANE_BACK = "pane-back"
