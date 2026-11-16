package local.adel.pos.android.core.ui.components

import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.domain.connection.ConnectionSummary
import local.adel.pos.android.core.ui.theme.AdelTheme

/**
 * Barra superior común a todas las pantallas: título a la izquierda y el indicador de conexión a la
 * derecha, siempre visible. Respeta la barra de estado del sistema (pantalla de borde a borde).
 * [actions] va antes del indicador (por ejemplo, el menú del mesero activo).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AdelTopBar(
    title: String,
    connection: ConnectionSummary,
    modifier: Modifier = Modifier,
    actions: @Composable RowScope.() -> Unit = {},
) {
    TopAppBar(
        modifier = modifier.testTag(TAG_TOP_BAR),
        title = {
            Text(
                text = title,
                style = MaterialTheme.typography.titleLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
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

const val TAG_TOP_BAR = "top-bar"
