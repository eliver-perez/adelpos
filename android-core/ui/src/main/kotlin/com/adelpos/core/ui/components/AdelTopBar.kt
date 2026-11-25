package com.adelpos.core.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.adelpos.core.model.connection.ConnectionSummary
import com.adelpos.core.ui.R
import com.adelpos.core.ui.icons.AdelIcons
import com.adelpos.core.ui.theme.AdelDimens
import com.adelpos.core.ui.theme.AdelTheme

/**
 * Barra superior común a todas las pantallas: título a la izquierda y el indicador de conexión a la
 * derecha, siempre visible. Respeta la barra de estado del sistema (pantalla de borde a borde).
 * [actions] va antes del indicador (por ejemplo, el menú del mesero activo).
 *
 * Etapa A3 (§4.1 y §4.5): con [onBack], la flecha de regreso arriba a la izquierda (48 dp, además
 * del Atrás de Android; [backTag] la nombra para las pruebas); con [logo], el logotipo de AD-EL POS
 * junto al título (las pantallas de entrada), en su versión clara u oscura según el tema.
 *
 * Etapa K1 (§5.4): con [onLogoLongPress], una pulsación larga en el logotipo abre algo escondido
 * (los ajustes de la pantalla de cocina). Un toque normal no hace nada.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AdelTopBar(
    title: String,
    connection: ConnectionSummary,
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
    backTag: String = TAG_TOP_BAR_BACK,
    logo: Boolean = false,
    onLogoLongPress: (() -> Unit)? = null,
    actions: @Composable RowScope.() -> Unit = {},
) {
    TopAppBar(
        modifier = modifier.testTag(TAG_TOP_BAR),
        navigationIcon = {
            if (onBack != null) {
                // TopAppBar deja el botón en 40 dp: se fija en el mínimo táctil (regla 12).
                IconButton(
                    onClick = onBack,
                    modifier = Modifier.size(AdelDimens.MinTouchTarget).testTag(backTag),
                ) {
                    Icon(AdelIcons.Back, contentDescription = stringResource(R.string.top_bar_back))
                }
            }
        },
        title = {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (logo) {
                    Image(
                        painter =
                            painterResource(
                                if (AdelTheme.colors.isDark) {
                                    R.drawable.adel_pos_logo_dark
                                } else {
                                    R.drawable.adel_pos_logo_light
                                },
                            ),
                        contentDescription = stringResource(R.string.top_bar_logo),
                        modifier =
                            Modifier.height(LOGO_HEIGHT)
                                .longPress(
                                    onLogoLongPress,
                                    stringResource(R.string.top_bar_logo_long_press),
                                )
                                .testTag(TAG_TOP_BAR_LOGO),
                    )
                }
                Text(
                    text = title,
                    style = MaterialTheme.typography.titleLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        },
        actions = {
            actions()
            ConnectionIndicator(connection, Modifier.padding(start = 8.dp, end = 16.dp))
        },
        colors =
            TopAppBarDefaults.topAppBarColors(
                containerColor = AdelTheme.colors.panel,
                titleContentColor = AdelTheme.colors.text,
            ),
    )
}

// La pulsación larga del logotipo, también para los lectores de pantalla (su acción de mantener
// presionado, con [label]). Sin acción, el logotipo no reacciona.
private fun Modifier.longPress(action: (() -> Unit)?, label: String): Modifier {
    if (action == null) {
        return this
    }
    return this.pointerInput(action) { detectTapGestures(onLongPress = { action() }) }
        .semantics {
            onLongClick(label = label) {
                action()
                true
            }
        }
}

// El logotipo mide 3 × 1: a 28 dp de alto ocupa unos 84 dp y deja lugar al título y al indicador
// también en un teléfono angosto.
private val LOGO_HEIGHT = 28.dp

const val TAG_TOP_BAR = "top-bar"
const val TAG_TOP_BAR_BACK = "top-bar-back"
const val TAG_TOP_BAR_LOGO = "top-bar-logo"
