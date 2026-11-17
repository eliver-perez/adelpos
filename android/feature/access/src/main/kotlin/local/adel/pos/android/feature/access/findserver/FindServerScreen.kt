package local.adel.pos.android.feature.access.findserver

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.delay
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.pairing.FoundServerChoice
import local.adel.pos.android.core.domain.pairing.PairingProblem
import local.adel.pos.android.core.domain.pairing.PairingProtocol
import local.adel.pos.android.core.domain.pairing.PairingState
import local.adel.pos.android.core.domain.pairing.PairingView
import local.adel.pos.android.core.ui.components.PrimaryButton
import local.adel.pos.android.core.ui.components.SecondaryButton
import local.adel.pos.android.core.ui.icons.AdelIcons
import local.adel.pos.android.core.ui.theme.AdelDimens
import local.adel.pos.android.core.ui.theme.AdelTheme
import local.adel.pos.android.feature.access.AccessDependencies
import local.adel.pos.android.feature.access.R
import local.adel.pos.android.feature.access.deviceWords
import local.adel.pos.android.feature.access.pairingProblemText
import local.adel.pos.android.feature.access.status.DeviceLogList
import local.adel.pos.android.feature.access.status.UpdateServerScreen

/**
 * «Buscar el servidor» (§4), como `FindServerPage` del escritorio: arriba, las computadoras
 * principales encontradas en la red (solo su nombre, nunca la dirección); abajo, «Escribir la
 * dirección» para las redes que no permiten encontrarla. Con un código pedido, la pantalla del
 * código en grande. [deviceLog] son los pedidos que quedaron en la bitácora.
 */
@Composable
fun FindServerRoute(
    dependencies: AccessDependencies,
    deviceLog: List<UnsentOrder>,
    onClearDeviceLog: () -> Unit,
    viewModel: FindServerViewModel =
        viewModel(factory = FindServerViewModel.factory(dependencies.pairing)),
) {
    val words = deviceWords()
    val defaultName =
        stringResource(
            if (words.isTablet) R.string.device_default_tablet_name
            else R.string.device_default_phone_name,
        )
    LaunchedEffect(defaultName) { viewModel.proposeName(defaultName) }
    val code = viewModel.code
    val problem = viewModel.problem
    if (code == null && problem is PairingProblem.UpdateServer) {
        // La computadora principal es anterior a la que pide esta aplicación: el mismo aviso que
        // ya emparejado, con «Volver a comprobar» y la opción de elegir otra.
        UpdateServerScreen(
            serverVersion = problem.serverVersion,
            onCheckAgain = viewModel::checkAgain,
            onChooseAnother = viewModel::startOver,
        )
    } else if (code != null) {
        PairingCodePanel(
            view = code,
            problem = viewModel.problem?.let { pairingProblemText(it) },
            wallClock = dependencies.wallClock,
            onRename = viewModel::rename,
            onCancel = viewModel::cancel,
            onStartOver = viewModel::startOver,
        )
    } else {
        FindServerScreen(
            deviceName = viewModel.deviceName,
            onDeviceNameChange = viewModel::onNameChange,
            address = viewModel.address,
            onAddressChange = viewModel::onAddressChange,
            search = viewModel.search,
            busy = viewModel.busy,
            problem = viewModel.problem?.let { pairingProblemText(it) },
            onSearchAgain = viewModel::searchAgain,
            onRequestFound = viewModel::requestFound,
            onRequestAddress = viewModel::requestAddress,
            deviceLog = deviceLog,
            onClearDeviceLog = onClearDeviceLog,
        )
    }
}

@Composable
fun FindServerScreen(
    deviceName: String,
    onDeviceNameChange: (String) -> Unit,
    address: String,
    onAddressChange: (String) -> Unit,
    search: SearchUi,
    busy: Boolean,
    problem: String?,
    onSearchAgain: () -> Unit,
    onRequestFound: (FoundServerChoice) -> Unit,
    onRequestAddress: () -> Unit,
    deviceLog: List<UnsentOrder>,
    onClearDeviceLog: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val words = deviceWords()
    val nameReady = deviceName.isNotBlank() && !busy
    CenteredColumn(modifier = modifier.testTag(TAG_FIND_SERVER)) {
        Text(
            text =
                stringResource(
                    if (words.isTablet) R.string.find_server_status
                    else R.string.find_server_status_phone,
                    words.thisDeviceCapital,
                ),
            style = MaterialTheme.typography.bodyLarge,
        )
        OutlinedTextField(
            value = deviceName,
            onValueChange = onDeviceNameChange,
            label = { Text(stringResource(R.string.find_server_name_label, words.thisDevice)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().testTag(TAG_DEVICE_NAME),
        )
        if (problem != null) {
            ProblemNotice(problem)
        }
        HorizontalDivider()
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            Text(
                text = stringResource(R.string.find_server_found_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
            )
            SecondaryButton(
                text = stringResource(R.string.find_server_search_again),
                onClick = onSearchAgain,
                enabled = search != SearchUi.Searching && !busy,
                icon = AdelIcons.Refresh,
                modifier = Modifier.testTag(TAG_SEARCH_AGAIN),
            )
        }
        when (search) {
            SearchUi.Searching ->
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                ) {
                    CircularProgressIndicator(modifier = Modifier.size(24.dp))
                    Text(
                        text = stringResource(R.string.find_server_searching),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            SearchUi.Unavailable ->
                Text(
                    text = stringResource(R.string.find_server_unavailable, words.thisDevice),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            is SearchUi.Done ->
                if (search.servers.isEmpty()) {
                    Text(
                        text = stringResource(R.string.find_server_none, words.thisDevice),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag(TAG_SEARCH_NONE),
                    )
                } else {
                    for (server in search.servers) {
                        FoundServerButton(server, enabled = nameReady) { onRequestFound(server) }
                    }
                }
        }
        HorizontalDivider()
        Text(
            text = stringResource(R.string.find_server_address_title),
            style = MaterialTheme.typography.titleMedium,
        )
        Text(
            text = stringResource(R.string.find_server_address_help),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        OutlinedTextField(
            value = address,
            onValueChange = onAddressChange,
            label = { Text(stringResource(R.string.find_server_address_label)) },
            placeholder = { Text(stringResource(R.string.find_server_address_placeholder)) },
            singleLine = true,
            keyboardOptions =
                KeyboardOptions(
                    keyboardType = KeyboardType.Uri,
                    autoCorrectEnabled = false,
                    imeAction = ImeAction.Go,
                ),
            keyboardActions =
                KeyboardActions(
                    onGo = {
                        if (nameReady) {
                            onRequestAddress()
                        }
                    },
                ),
            modifier = Modifier.fillMaxWidth().testTag(TAG_ADDRESS),
        )
        PrimaryButton(
            text = stringResource(R.string.find_server_request_code),
            onClick = onRequestAddress,
            enabled = nameReady && address.isNotBlank(),
            modifier = Modifier.fillMaxWidth().testTag(TAG_REQUEST_ADDRESS),
        )
        if (deviceLog.isNotEmpty()) {
            HorizontalDivider()
            Text(
                text = stringResource(R.string.find_server_log_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.find_server_log_help),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            DeviceLogList(deviceLog, onClearDeviceLog)
        }
    }
}

@Composable
private fun FoundServerButton(server: FoundServerChoice, enabled: Boolean, onClick: () -> Unit) {
    val description = stringResource(R.string.find_server_request_code_to, server.name)
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier =
            Modifier.fillMaxWidth()
                .heightIn(min = AdelDimens.PrimaryActionHeight)
                .semantics { contentDescription = description }
                .testTag("$TAG_FOUND_SERVER_PREFIX${server.serverId}"),
    ) {
        Icon(AdelIcons.Server, contentDescription = null, modifier = Modifier.size(22.dp))
        Text(
            text = server.name,
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.weight(1f).padding(horizontal = 12.dp),
        )
        Text(stringResource(R.string.find_server_request_code))
    }
}

/**
 * El código de 6 dígitos en grande, en dos grupos, con el nombre del restaurante, la cuenta
 * regresiva, el nombre editable del dispositivo y el estado de la solicitud.
 */
@Composable
fun PairingCodePanel(
    view: PairingView,
    problem: String?,
    wallClock: () -> Long,
    onRename: (String) -> Unit,
    onCancel: () -> Unit,
    onStartOver: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val words = deviceWords()
    val waiting = view.state == PairingState.PENDING
    var now by remember { mutableLongStateOf(wallClock()) }
    LaunchedEffect(waiting) {
        while (waiting) {
            now = wallClock()
            delay(1_000)
        }
    }
    var editing by rememberSaveable { mutableStateOf(false) }
    var name by rememberSaveable(view.deviceName) { mutableStateOf(view.deviceName) }
    CenteredColumn(modifier = modifier.testTag(TAG_PAIRING_PANEL)) {
        if (view.serverName.isNotBlank()) {
            Text(
                text = stringResource(R.string.pairing_server_of, view.serverName),
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (waiting) {
            Text(
                text = stringResource(R.string.pairing_instructions),
                style = MaterialTheme.typography.bodyLarge,
            )
            val spoken = view.code.toList().joinToString(" ")
            val description = stringResource(R.string.pairing_code_description, spoken)
            Text(
                text = PairingProtocol.grouped(view.code),
                fontSize = 64.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace,
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.primary,
                modifier =
                    Modifier.fillMaxWidth().testTag(TAG_PAIRING_CODE).semantics {
                        contentDescription = description
                    },
            )
            Text(
                text = stringResource(R.string.pairing_expires_in, remaining(view, now)),
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth(),
            )
        }
        if (editing && waiting) {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.find_server_name_label, words.thisDevice)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag(TAG_RENAME_FIELD),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(AdelDimens.Gap)) {
                PrimaryButton(
                    text = stringResource(R.string.pairing_save_name),
                    onClick = {
                        onRename(name)
                        editing = false
                    },
                    enabled = name.isNotBlank(),
                )
                SecondaryButton(
                    text = stringResource(R.string.common_cancel),
                    onClick = {
                        name = view.deviceName
                        editing = false
                    },
                )
            }
        } else {
            Text(
                text =
                    stringResource(
                        R.string.pairing_will_be_named,
                        words.thisDeviceCapital,
                        view.deviceName,
                    ),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag(TAG_PAIRING_NAME),
            )
            if (waiting) {
                SecondaryButton(
                    text = stringResource(R.string.pairing_change_name),
                    onClick = { editing = true },
                    icon = AdelIcons.Edit,
                )
            }
        }
        Text(
            text = pairingStateText(view.state),
            style = MaterialTheme.typography.titleMedium,
            modifier =
                Modifier.testTag(TAG_PAIRING_STATE).semantics {
                    liveRegion = LiveRegionMode.Polite
                },
        )
        if (problem != null) {
            ProblemNotice(problem)
        }
        if (waiting) {
            SecondaryButton(
                text = stringResource(R.string.common_cancel),
                onClick = onCancel,
                modifier = Modifier.testTag(TAG_PAIRING_CANCEL),
            )
        } else if (view.state != PairingState.PAIRED) {
            PrimaryButton(
                text = stringResource(R.string.pairing_request_another),
                onClick = onStartOver,
                modifier = Modifier.fillMaxWidth().testTag(TAG_PAIRING_START_OVER),
            )
        }
    }
}

@Composable
private fun pairingStateText(state: PairingState): String {
    val words = deviceWords()
    return when (state) {
        PairingState.PENDING -> stringResource(R.string.pairing_waiting)
        PairingState.PAIRED -> stringResource(R.string.pairing_paired)
        PairingState.REJECTED -> stringResource(R.string.pairing_rejected, words.thisDevice)
        PairingState.EXPIRED -> stringResource(R.string.pairing_expired)
        PairingState.CANCELLED -> stringResource(R.string.pairing_cancelled)
        PairingState.CLAIMED -> stringResource(R.string.pairing_claimed, words.thisDeviceCapital)
    }
}

// Minutos y segundos que faltan para que venza el código («4:59»).
private fun remaining(view: PairingView, now: Long): String {
    val seconds = ((view.expiresAtEpochMillis - now) / 1_000).coerceAtLeast(0)
    return "${seconds / 60}:${(seconds % 60).toString().padStart(2, '0')}"
}

/** Columna con desplazamiento y un ancho cómodo de lectura, centrada en la tableta. */
@Composable
internal fun CenteredColumn(
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Box(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()),
        contentAlignment = Alignment.TopCenter,
    ) {
        Column(
            modifier =
                modifier.widthIn(max = 640.dp).fillMaxWidth().padding(AdelDimens.ScreenPadding),
            verticalArrangement = Arrangement.spacedBy(AdelDimens.Gap),
        ) {
            content()
        }
    }
}

/** Aviso de un problema, en rojo y anunciado a los lectores de pantalla. */
@Composable
internal fun ProblemNotice(text: String, modifier: Modifier = Modifier) {
    Surface(
        color = AdelTheme.colors.redSoft,
        contentColor = AdelTheme.colors.red,
        border = BorderStroke(1.dp, AdelTheme.colors.dangerBorder),
        shape = MaterialTheme.shapes.medium,
        modifier =
            modifier.fillMaxWidth().testTag(TAG_PROBLEM).semantics {
                liveRegion = LiveRegionMode.Assertive
            },
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.Top,
        ) {
            Icon(AdelIcons.Alert, contentDescription = null, modifier = Modifier.size(20.dp))
            Text(text = text, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** Etiquetas para las pruebas. */
const val TAG_FIND_SERVER = "find-server"
const val TAG_DEVICE_NAME = "find-server-device-name"
const val TAG_SEARCH_AGAIN = "find-server-search-again"
const val TAG_SEARCH_NONE = "find-server-none"
const val TAG_FOUND_SERVER_PREFIX = "find-server-found-"
const val TAG_ADDRESS = "find-server-address"
const val TAG_REQUEST_ADDRESS = "find-server-request-address"
const val TAG_PAIRING_PANEL = "pairing-panel"
const val TAG_PAIRING_CODE = "pairing-code"
const val TAG_PAIRING_NAME = "pairing-name"
const val TAG_PAIRING_STATE = "pairing-state"
const val TAG_PAIRING_CANCEL = "pairing-cancel"
const val TAG_PAIRING_START_OVER = "pairing-start-over"
const val TAG_RENAME_FIELD = "pairing-rename"
const val TAG_PROBLEM = "access-problem"
