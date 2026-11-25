package com.adelpos.core.ui.access

import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.LicenseMessages
import com.adelpos.core.model.pairing.PairingProblem
import com.adelpos.core.model.server.ServerAddress
import com.adelpos.core.ui.R
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * Cómo se nombra el dispositivo en los textos: «esta tableta» o «este teléfono» (por el ancho menor
 * de la pantalla, como Android distingue una tableta).
 */
class DeviceWords(val isTablet: Boolean, val thisDevice: String, val thisDeviceCapital: String)

@Composable
fun deviceWords(): DeviceWords {
    val isTablet = LocalConfiguration.current.smallestScreenWidthDp >= TABLET_MIN_WIDTH_DP
    return if (isTablet) {
        DeviceWords(
            isTablet = true,
            thisDevice = stringResource(R.string.device_this_tablet),
            thisDeviceCapital = stringResource(R.string.device_this_tablet_capital),
        )
    } else {
        DeviceWords(
            isTablet = false,
            thisDevice = stringResource(R.string.device_this_phone),
            thisDeviceCapital = stringResource(R.string.device_this_phone_capital),
        )
    }
}

private const val TABLET_MIN_WIDTH_DP = 600

/** Hora local «14:35» de un instante (para «Intenta de nuevo a las 14:35»). */
fun clockTime(epochMillis: Long, zone: ZoneId = ZoneId.systemDefault()): String =
    DateTimeFormatter.ofPattern("HH:mm").format(Instant.ofEpochMilli(epochMillis).atZone(zone))

/**
 * Texto de un problema del emparejamiento. Un rechazo de Go con su mensaje en español se muestra
 * tal cual (Go lo escribe para mostrarse); lo demás, con los textos de la aplicación.
 * [requiredServerVersion] es la versión mínima del servidor que pide la aplicación.
 */
@Composable
fun pairingProblemText(problem: PairingProblem, requiredServerVersion: String): String {
    val device = deviceWords()
    return when (problem) {
        is PairingProblem.InvalidAddress ->
            when (problem.problem) {
                ServerAddress.Problem.MALFORMED ->
                    stringResource(R.string.pairing_problem_address_malformed)
                ServerAddress.Problem.NOT_LOCAL ->
                    stringResource(R.string.pairing_problem_address_not_local)
            }
        PairingProblem.InvalidName ->
            stringResource(R.string.pairing_problem_name, device.thisDevice)
        PairingProblem.NoLongerFound -> stringResource(R.string.pairing_problem_no_longer_found)
        PairingProblem.AnnouncementMismatch -> stringResource(R.string.pairing_problem_mismatch)
        PairingProblem.NoSession -> stringResource(R.string.pairing_problem_no_session)
        PairingProblem.InvalidCredential -> stringResource(R.string.pairing_problem_credential)
        PairingProblem.StorageFailed ->
            stringResource(R.string.pairing_problem_storage, device.thisDevice)
        is PairingProblem.UpdateServer ->
            stringResource(R.string.update_server_text, requiredServerVersion)
        is PairingProblem.Server -> serverErrorText(problem.error)
    }
}

@Composable
private fun serverErrorText(error: AppError): String =
    when (error) {
        AppError.Unreachable -> stringResource(R.string.pairing_problem_unreachable)
        AppError.ServerIdentityChanged -> stringResource(R.string.pairing_problem_identity)
        is AppError.UpdateRequired -> stringResource(R.string.entry_update_required)
        // Etapa L1: emparejar es administración; sin licencia activada, Go no lo permite (L1-D6).
        is AppError.LicenseRefused -> LicenseMessages.forPairing(error.refusal)
        else -> error.serverMessage ?: stringResource(R.string.pairing_problem_server)
    }
