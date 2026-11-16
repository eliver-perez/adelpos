package local.adel.pos.android.feature.access

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.theme.AdelDimens

/**
 * Pantalla de entrada (marcador de la Fase A). La Fase B la reemplaza por la lista de meseros con
 * su PIN y la Fase C agrega el emparejamiento antes de ella.
 */
@Composable
fun AccessScreen(onEnter: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier =
            modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(AdelDimens.ScreenPadding)
                .testTag(TAG_ACCESS_SCREEN),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap, Alignment.CenterVertically),
    ) {
        Text(
            text = stringResource(R.string.access_heading),
            style = MaterialTheme.typography.headlineMedium,
            textAlign = TextAlign.Center,
        )
        Text(
            text = stringResource(R.string.access_placeholder),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 480.dp),
        )
        PrimaryButton(
            text = stringResource(R.string.access_enter),
            onClick = onEnter,
            modifier = Modifier.widthIn(max = 480.dp).fillMaxWidth().testTag(TAG_ACCESS_ENTER),
        )
    }
}

/** Título de la pantalla para la barra superior. */
@Composable fun accessTitle(): String = stringResource(R.string.access_title)

const val TAG_ACCESS_SCREEN = "access-screen"
const val TAG_ACCESS_ENTER = "access-enter"
