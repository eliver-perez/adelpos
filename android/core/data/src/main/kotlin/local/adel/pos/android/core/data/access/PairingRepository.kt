package local.adel.pos.android.core.data.access

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ClaimedCredentialDto
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.api.CreatePairingRequest
import local.adel.pos.android.core.data.api.CreatedPairing
import local.adel.pos.android.core.data.api.EmptyBody
import local.adel.pos.android.core.data.api.IgnoredBody
import local.adel.pos.android.core.data.api.PairingStatusDto
import local.adel.pos.android.core.data.api.PinnedHttpClient
import local.adel.pos.android.core.data.api.RenamePairingRequest
import local.adel.pos.android.core.data.api.RevealPairingNonce
import local.adel.pos.android.core.data.api.ServerApi
import local.adel.pos.android.core.data.api.VersionInfo
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.discovery.NsdServerBrowser
import local.adel.pos.android.core.data.security.FirstSeenTrustManager
import local.adel.pos.android.core.domain.access.IdleLock
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.pairing.FoundServerChoice
import local.adel.pos.android.core.domain.pairing.PairingFlow
import local.adel.pos.android.core.domain.pairing.PairingProblem
import local.adel.pos.android.core.domain.pairing.PairingProtocol
import local.adel.pos.android.core.domain.pairing.PairingState
import local.adel.pos.android.core.domain.pairing.PairingStep
import local.adel.pos.android.core.domain.pairing.PairingTarget
import local.adel.pos.android.core.domain.pairing.PairingView
import local.adel.pos.android.core.domain.pairing.ServerSearch
import local.adel.pos.android.core.domain.security.SecretSealer
import local.adel.pos.android.core.domain.security.SecretUnavailableException
import local.adel.pos.android.core.domain.server.FoundServer
import local.adel.pos.android.core.domain.server.ServerAddress
import local.adel.pos.android.core.domain.server.ServerAnnouncements
import local.adel.pos.android.core.domain.server.ServerCompatibility
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.Headers
import okhttp3.HttpUrl.Companion.toHttpUrl

/**
 * Búsqueda del servidor y emparejamiento con el protocolo exacto de la 9G (§4; `pairing.rs` del
 * escritorio y `backend/internal/administration/pairing.go`):
 * 1. La sesión fija el **primer certificado** que ve ([FirstSeenTrustManager]) al pedir `GET
 *    version`; si el servidor vino de la búsqueda, su huella debe empezar como la anunciada (`fp`)
 *    y su `server_id` ser el anunciado. Con la versión se comprueba, antes de pedir el código
 *    (también con la dirección escrita), que el servidor sirva para esta aplicación
 *    ([ServerCompatibility.check]): uno anterior a 0.12.0 o sin `waiter-pin-v1` lleva a «Actualiza
 *    el servidor del restaurante» ([PairingProblem.UpdateServer]).
 * 2. Solicitud con el compromiso SHA-256(nT), número del servidor nS y revelación de nT.
 * 3. Código de 6 dígitos con la huella fijada; el administrador lo escribe en la computadora
 *    principal.
 * 4. Aprobado, se reclama la credencial **una sola vez**, se cifra con el Android Keystore y se
 *    guarda con el servidor emparejado ([AccessRepository.onPaired]). Si guardar falla, la
 *    credencial reclamada se conserva en la sesión y la siguiente consulta lo vuelve a intentar sin
 *    reclamar otra vez.
 *
 * El secreto de la solicitud y los números del código nunca salen de aquí.
 *
 * @param browse búsqueda en la red ([NsdServerBrowser.browse]).
 * @param osLabel sistema que se informa al servidor («Android 13»).
 * @param normalizeAddress validación de la dirección escrita ([ServerAddress.normalize]; las
 *   pruebas de la JVM la sustituyen porque su servidor simulado escucha en loopback).
 */
class PairingRepository(
    private val browse: suspend () -> NsdServerBrowser.Browse,
    private val identity: ClientIdentity,
    private val osLabel: String,
    private val sealer: SecretSealer,
    private val access: AccessRepository,
    private val wallClock: () -> Long = System::currentTimeMillis,
    private val normalizeAddress: (String) -> ServerAddress.Result = ServerAddress::normalize,
) : PairingFlow {
    private val mutex = Mutex()
    private var lastSearch: List<FoundServer> = emptyList()
    private var session: Session? = null

    private class Claimed(val deviceId: String, val deviceToken: String, val deviceName: String)

    private class Session(
        val api: ServerApi,
        val base: String,
        val fingerprint: ByteArray,
        val requestId: String,
        val secret: String,
        val serverId: String,
        val serverName: String,
        val code: String,
        var deviceName: String,
        val expiresAtEpochMillis: Long,
        var claimed: Claimed? = null,
    ) {
        fun headers(): Headers = Headers.headersOf(PAIRING_SECRET_HEADER, secret)

        fun view(state: PairingState) =
            PairingView(state, code, deviceName, serverName, expiresAtEpochMillis)
    }

    override suspend fun search(): ServerSearch =
        when (val result = browse()) {
            NsdServerBrowser.Browse.Unavailable -> ServerSearch.Unavailable
            is NsdServerBrowser.Browse.Found -> {
                lastSearch = result.servers
                ServerSearch.Found(result.servers.map { FoundServerChoice(it.serverId, it.name) })
            }
        }

    override suspend fun start(target: PairingTarget, deviceName: String): PairingStep =
        mutex.withLock {
            val name = cleanName(deviceName) ?: return@withLock failed(PairingProblem.InvalidName)
            val attempts: List<Pair<String, FoundServer?>> =
                when (target) {
                    is PairingTarget.Address ->
                        when (val address = normalizeAddress(target.text)) {
                            is ServerAddress.Result.Valid -> listOf(address.base to null)
                            is ServerAddress.Result.Invalid ->
                                return@withLock failed(
                                    PairingProblem.InvalidAddress(address.problem),
                                )
                        }
                    is PairingTarget.Found -> {
                        val found =
                            lastSearch.firstOrNull { it.serverId == target.serverId }
                                ?: return@withLock failed(PairingProblem.NoLongerFound)
                        found.bases().map { it to found }
                    }
                }
            var failure: PairingStep = failed(PairingProblem.Server(AppError.Unreachable))
            // Cada dirección anunciada, hasta que una responda con la identidad anunciada.
            for ((base, announced) in attempts) {
                when (val step = request(base, name, announced)) {
                    is PairingStep.Showing -> return@withLock step
                    is PairingStep.Failed -> failure = step
                }
            }
            failure
        }

    override suspend fun poll(): PairingStep = mutex.withLock {
        val current = session ?: return@withLock failed(PairingProblem.NoSession)
        // Mientras no se tiene la credencial: se consulta el estado y, aprobada, se reclama.
        if (current.claimed == null) {
            val notApprovedYet = checkApproval(current)
            if (notApprovedYet != null) {
                return@withLock notApprovedYet
            }
            val claimFailure = claimCredential(current)
            if (claimFailure != null) {
                return@withLock claimFailure
            }
        }
        // Con la credencial: se guarda cifrada junto con el servidor emparejado.
        saveClaimed(current)
    }

    /**
     * Estado de la solicitud en el servidor. null si ya se aprobó (sigue reclamar la credencial);
     * si no, lo que muestra la pantalla (en espera, rechazada, vencida…) o el problema.
     */
    private suspend fun checkApproval(current: Session): PairingStep? {
        val outcome =
            current.api.get(
                "pairing/requests/${current.requestId}",
                PairingStatusDto.serializer(),
                current.headers(),
            )
        if (outcome !is ApiOutcome.Success) {
            return failed(PairingProblem.Server(outcome.error()))
        }
        val status = outcome.value
        // El administrador pudo corregir el nombre al aprobar.
        val name = status.name
        if (!name.isNullOrBlank()) {
            current.deviceName = name
        }
        if (status.state == "approved") {
            return null
        }
        val state =
            PairingState.fromWireName(status.state)
                ?: return failed(PairingProblem.Server(AppError.InvalidResponse(200)))
        if (state.finished) {
            session = null
        }
        return PairingStep.Showing(current.view(state))
    }

    /**
     * Reclama la credencial del dispositivo, una sola vez. null si se recibió y es válida (queda en
     * la sesión hasta guardarla); si no, el problema, y la sesión termina.
     */
    private suspend fun claimCredential(current: Session): PairingStep.Failed? {
        val outcome =
            current.api.post(
                "pairing/requests/${current.requestId}/claim",
                EmptyBody(),
                EmptyBody.serializer(),
                ClaimedCredentialDto.serializer(),
                current.headers(),
            )
        if (outcome !is ApiOutcome.Success) {
            session = null
            return failed(PairingProblem.Server(outcome.error()))
        }
        val credential = outcome.value
        val valid =
            UUID_PATTERN.matches(credential.deviceId) &&
                credential.deviceToken.length == TOKEN_LENGTH &&
                credential.serverId == current.serverId
        if (!valid) {
            session = null
            return failed(PairingProblem.InvalidCredential)
        }
        current.claimed =
            Claimed(
                deviceId = credential.deviceId,
                deviceToken = credential.deviceToken,
                deviceName = credential.name.ifBlank { current.deviceName },
            )
        return null
    }

    /**
     * Cifra la credencial reclamada con el Android Keystore y guarda el servidor emparejado (su
     * `server_id`, la huella fijada en la sesión, la dirección y los nombres). Si el Keystore
     * falla, lo reclamado se conserva y la siguiente consulta vuelve a intentarlo sin reclamar otra
     * vez.
     */
    private suspend fun saveClaimed(current: Session): PairingStep {
        val claimed = current.claimed ?: return failed(PairingProblem.NoSession)
        val sealedToken =
            try {
                sealer.seal(
                    claimed.deviceToken.toByteArray(Charsets.UTF_8),
                    AccessRepository.DEVICE_PURPOSE,
                )
            } catch (_: SecretUnavailableException) {
                return failed(PairingProblem.StorageFailed)
            }
        access.onPaired(
            PairedServerEntity(
                serverId = current.serverId,
                serverName = current.serverName,
                baseUrl = current.base,
                fingerprintHex = Hex.encode(current.fingerprint),
                deviceId = claimed.deviceId,
                deviceName = claimed.deviceName,
                sealedDeviceToken = sealedToken,
                idleLockSeconds = IdleLock.DEFAULT_SECONDS,
                pairedAtMillis = wallClock(),
            ),
        )
        session = null
        current.deviceName = claimed.deviceName
        return PairingStep.Showing(current.view(PairingState.PAIRED))
    }

    override suspend fun rename(deviceName: String): PairingStep = mutex.withLock {
        val current = session ?: return@withLock failed(PairingProblem.NoSession)
        val name = cleanName(deviceName) ?: return@withLock failed(PairingProblem.InvalidName)
        when (
            val renamed =
                current.api.post(
                    "pairing/requests/${current.requestId}/name",
                    RenamePairingRequest(name),
                    RenamePairingRequest.serializer(),
                    PairingStatusDto.serializer(),
                    current.headers(),
                )
        ) {
            is ApiOutcome.Success -> {
                current.deviceName = renamed.value.name ?: name
                PairingStep.Showing(current.view(PairingState.PENDING))
            }
            else -> failed(PairingProblem.Server(renamed.error()))
        }
    }

    override suspend fun cancel() {
        val current =
            mutex.withLock {
                val value = session
                session = null
                value
            } ?: return
        // Si el servidor no responde, la solicitud vence sola a los 5 minutos.
        current.api.post(
            "pairing/requests/${current.requestId}/cancel",
            EmptyBody(),
            EmptyBody.serializer(),
            IgnoredBody.serializer(),
            current.headers(),
        )
    }

    private suspend fun request(base: String, name: String, announced: FoundServer?): PairingStep {
        val trust = FirstSeenTrustManager()
        val api = ServerApi(base.toHttpUrl(), PinnedHttpClient.createForPairing(trust, identity))
        // La primera conexión de la sesión fija el certificado y trae la versión del servidor.
        val version = api.get("version", VersionInfo.serializer())
        if (version !is ApiOutcome.Success) {
            // Sin la versión no se sabe qué mínima pide el servidor: un 426 aquí es de uno viejo.
            return failed(pairingProblem(version.error(), informsMinimum = false, null))
        }
        val info = version.value
        if (announced != null) {
            // Su huella debe empezar como la anunciada antes de pedir nada.
            val seen = trust.fingerprint ?: return failed(PairingProblem.AnnouncementMismatch)
            if (ServerAnnouncements.shortFingerprint(seen) != announced.fingerprintPrefix) {
                return failed(PairingProblem.AnnouncementMismatch)
            }
        }
        // Antes de pedir el código: un servidor que no sirve para esta aplicación respondería 426
        // a la solicitud, y el aviso sería el equivocado («actualiza esta aplicación»).
        val minimum = info.minClientVersions[ClientIdentity.KIND]
        val verdict =
            ServerCompatibility.check(
                appVersion = identity.version,
                serverVersion = info.serverVersion,
                capabilities = info.capabilities,
                minAndroidVersion = minimum,
            )
        when (verdict) {
            is ServerCompatibility.Verdict.UpdateServer ->
                return failed(PairingProblem.UpdateServer(verdict.serverVersion))
            is ServerCompatibility.Verdict.UpdateApp ->
                return failed(
                    PairingProblem.Server(
                        AppError.UpdateRequired(
                            code = "CLIENT_UPDATE_REQUIRED",
                            minClientVersion = verdict.minClientVersion,
                            serverVersion = verdict.serverVersion,
                            serverMessage = null,
                            requestId = null,
                        ),
                    ),
                )
            ServerCompatibility.Verdict.Compatible -> Unit
        }
        val informsMinimum = minimum != null
        val nonce = PairingProtocol.newNonce()
        val created =
            when (
                val outcome =
                    api.post(
                        "pairing/requests",
                        CreatePairingRequest(
                            name = name,
                            deviceKind = ClientIdentity.KIND,
                            os = osLabel,
                            appVersion = identity.version,
                            commitment = PairingProtocol.commitment(nonce),
                        ),
                        CreatePairingRequest.serializer(),
                        CreatedPairing.serializer(),
                    )
            ) {
                is ApiOutcome.Success -> outcome.value
                else ->
                    return failed(
                        pairingProblem(outcome.error(), informsMinimum, info.serverVersion),
                    )
            }
        val fingerprint =
            trust.fingerprint ?: return failed(PairingProblem.Server(AppError.InvalidResponse(200)))
        val serverNonce = created.serverNonce.takeIf(NONCE_PATTERN::matches)?.let(Hex::decode)
        val expires = AccessRepository.epochMillis(created.expiresAt)
        if (
            serverNonce == null ||
                expires == null ||
                created.pairingSecret.length != TOKEN_LENGTH ||
                !UUID_PATTERN.matches(created.requestId) ||
                !UUID_PATTERN.matches(created.serverId)
        ) {
            return failed(PairingProblem.Server(AppError.InvalidResponse(200)))
        }
        if (announced != null && announced.serverId != created.serverId) {
            return failed(PairingProblem.AnnouncementMismatch)
        }
        val headers = Headers.headersOf(PAIRING_SECRET_HEADER, created.pairingSecret)
        // El número propio se revela solo después de recibir el del servidor (compromiso previo).
        val revealed =
            api.post(
                "pairing/requests/${created.requestId}/reveal",
                RevealPairingNonce(Hex.encode(nonce)),
                RevealPairingNonce.serializer(),
                PairingStatusDto.serializer(),
                headers,
            )
        if (revealed !is ApiOutcome.Success) {
            return failed(pairingProblem(revealed.error(), informsMinimum, info.serverVersion))
        }
        val code = PairingProtocol.code(fingerprint, nonce, serverNonce, created.requestId)
        val started =
            Session(
                api = api,
                base = base,
                fingerprint = fingerprint,
                requestId = created.requestId,
                secret = created.pairingSecret,
                serverId = created.serverId,
                serverName = created.serverName.filterNot(Char::isISOControl).take(MAX_SERVER_NAME),
                code = code,
                deviceName = created.name.ifBlank { name },
                expiresAtEpochMillis = expires,
            )
        session = started
        return PairingStep.Showing(started.view(PairingState.PENDING))
    }

    private fun failed(problem: PairingProblem) = PairingStep.Failed(problem)

    /**
     * El problema de un error al pedir el código. «Actualiza esta aplicación» (426 o tipo o versión
     * de cliente no reconocidos) solo es cierto si el servidor informa la mínima de Android
     * (`min_client_versions.android`, desde la 0.12.0); si no la informa, es un servidor anterior
     * que no conoce esta aplicación: «Actualiza el servidor del restaurante».
     */
    private fun pairingProblem(
        error: AppError,
        informsMinimum: Boolean,
        serverVersion: String?,
    ): PairingProblem =
        if (error is AppError.UpdateRequired && !informsMinimum) {
            PairingProblem.UpdateServer(serverVersion ?: error.serverVersion)
        } else {
            PairingProblem.Server(error)
        }

    private fun ApiOutcome<*>.error(): AppError =
        when (this) {
            is ApiOutcome.Success -> AppError.InvalidResponse(200)
            is ApiOutcome.Rejected -> ApiErrors.toAppError(problem)
            is ApiOutcome.NoAnswer -> error
            is ApiOutcome.Malformed -> AppError.InvalidResponse(status)
        }

    companion object {
        const val PAIRING_SECRET_HEADER = "X-Pairing-Secret"
        private const val TOKEN_LENGTH = 43
        private const val MAX_NAME = 80
        private const val MAX_SERVER_NAME = 100
        private val UUID_PATTERN =
            Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
        private val NONCE_PATTERN = Regex("^[0-9a-f]{64}$")

        /**
         * Nombre del dispositivo como lo acepta Go (`cleanLabel`): sin espacios a los lados, de 1 a
         * 80 caracteres y sin caracteres de control. null si no sirve.
         */
        fun cleanName(name: String): String? {
            val trimmed = name.trim()
            if (
                trimmed.isEmpty() ||
                    trimmed.codePointCount(0, trimmed.length) > MAX_NAME ||
                    trimmed.any(Char::isISOControl)
            ) {
                return null
            }
            return trimmed
        }
    }
}
