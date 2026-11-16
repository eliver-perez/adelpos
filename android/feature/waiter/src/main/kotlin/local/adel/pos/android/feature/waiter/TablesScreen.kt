package local.adel.pos.android.feature.waiter

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import local.adel.pos.android.core.domain.table.TableState
import local.adel.pos.android.core.ui.components.TableStateBadge
import local.adel.pos.android.core.ui.layout.AdaptivePanes
import local.adel.pos.android.core.ui.layout.PaneLayout
import local.adel.pos.android.core.ui.theme.AdelDimens

/**
 * Pantalla Mesas con su ViewModel (marcador de la Fase A; la Fase E trae áreas, mesas y cuentas).
 */
@Composable
fun TablesRoute(
    layout: PaneLayout,
    viewModel: TablesViewModel = viewModel(factory = TablesViewModel.Factory),
) {
    TablesScreen(
        layout = layout,
        search = viewModel.search,
        onSearchChange = viewModel::onSearchChange,
        showingAccount = viewModel.showingAccount,
        onShowingAccountChange = viewModel::onShowingAccountChange,
    )
}

/**
 * Mesas y cuenta en uno o dos paneles: en la tableta lado a lado; en el teléfono, las mesas con la
 * cuenta a un toque.
 */
@Composable
fun TablesScreen(
    layout: PaneLayout,
    search: String,
    onSearchChange: (String) -> Unit,
    showingAccount: Boolean,
    onShowingAccountChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
) {
    val title = stringResource(R.string.tables_title)
    AdaptivePanes(
        layout = layout,
        showSecondary = showingAccount,
        onShowSecondaryChange = onShowingAccountChange,
        primaryTitle = title,
        openSecondaryLabel = stringResource(R.string.account_open),
        primary = { TablesPane(search, onSearchChange) },
        secondary = { AccountPane() },
        modifier = modifier,
    )
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun TablesPane(search: String, onSearchChange: (String) -> Unit) {
    Column(
        modifier =
            Modifier.fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(AdelDimens.ScreenPadding),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        OutlinedTextField(
            value = search,
            onValueChange = onSearchChange,
            label = { Text(stringResource(R.string.tables_search)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            modifier = Modifier.fillMaxWidth().testTag(TAG_TABLES_SEARCH),
        )
        Text(
            stringResource(R.string.tables_states_heading),
            style = MaterialTheme.typography.titleMedium,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            for (state in TableState.entries) {
                TableStateBadge(state)
            }
        }
        Text(
            text = stringResource(R.string.tables_placeholder),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun AccountPane() {
    Column(
        modifier =
            Modifier.fillMaxSize().padding(AdelDimens.ScreenPadding).testTag(TAG_ACCOUNT_PANE),
        verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
    ) {
        Text(stringResource(R.string.account_title), style = MaterialTheme.typography.titleLarge)
        Text(
            text = stringResource(R.string.account_placeholder),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** Título de la pantalla para la barra superior. */
@Composable fun tablesTitle(): String = stringResource(R.string.tables_title)

const val TAG_TABLES_SEARCH = "tables-search"
const val TAG_ACCOUNT_PANE = "account-pane"
