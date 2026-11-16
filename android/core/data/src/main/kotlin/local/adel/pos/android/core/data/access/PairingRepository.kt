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
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.Headers
import okhttp3.HttpUrl.Companion.toHttpUrl

/**
 * Búsqueda del servidor y emparejamiento con el protocolo exacto de la 9G (§4; `pairing.rs` del
 * escritorio y `backend/internal/administration/pairing.go`):
 * 1. La sesión fija el **primer certificado** que ve ([FirstSeenTrustManager]); si el servidor vino
 *    de la búsqueda, su huella debe empezar como la anunciada (`fp`) y su `server_id` ser el
 *    anunciado.
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
        if (current.claimed == null) {
            when (
                val status =
                    current.api.get(
                        "pairing/requests/${current.requestId}",
                        PairingStatusDto.serializer(),
                        current.headers(),
                    )
            ) {
                is ApiOutcome.Success -> {
                    status.value.name?.takeIf { it.isNotBlank() }?.let { current.deviceName = it }
                    if (status.value.state != "approved") {
                        val state =
                            PairingState.fromWireName(status.value.state)
                                ?: return@withLock failed(
                                    PairingProblem.Server(AppError.InvalidResponse(200)),
                                )
                        if (state.finished) {
                            session = null
                        }
                        return@withLock PairingStep.Showing(current.view(state))
                    }
                }
                else -> return@withLock failed(PairingProblem.Server(status.error()))
            }
            when (
                val claim =
                    current.api.post(
                        "pairing/requests/${current.requestId}/claim",
                        EmptyBody(),
                        EmptyBody.serializer(),
                        ClaimedCredentialDto.serializer(),
                        current.headers(),
                    )
            ) {
                is ApiOutcome.Success -> {
                    val credential = claim.value
                    if (
                        !UUID_PATTERN.matches(credential.deviceId) ||
                            credential.deviceToken.length != TOKEN_LENGTH ||
                            credential.serverId != current.serverId
                    ) {
                        session = null
                        return@withLock failed(PairingProblem.InvalidCredential)
                    }
                    current.claimed =
                        Claimed(
                            credential.deviceId,
                            credential.deviceToken,
                            credential.name.ifBlank { current.deviceName },
                        )
                }
                else -> {
                    session = null
                    return@withLock failed(PairingProblem.Server(claim.error()))
                }
            }
        }
        val claimed = current.claimed ?: return@withLock failed(PairingProblem.NoSession)
        val sealedToken =
            try {
                sealer.seal(
                    claimed.deviceToken.toByteArray(Charsets.UTF_8),
                    AccessRepository.DEVICE_PURPOSE,
                )
            } catch (_: SecretUnavailableException) {
                // Se conserva lo reclamado: la siguiente consulta vuelve a intentar guardarlo.
                return@withLock failed(PairingProblem.StorageFailed)
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
        PairingStep.Showing(current.view(PairingState.PAIRED))
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
        if (announced != null) {
            // La primera conexión de la sesión fija el certificado; su huella debe empezar como la
            // anunciada antes de pedir nada.
            val version = api.get("version", VersionInfo.serializer())
            if (version !is ApiOutcome.Success) {
                return failed(PairingProblem.Server(version.error()))
            }
            val seen = trust.fingerprint ?: return failed(PairingProblem.AnnouncementMismatch)
            if (ServerAnnouncements.shortFingerprint(seen) != announced.fingerprintPrefix) {
                return failed(PairingProblem.AnnouncementMismatch)
            }
        }
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
                else -> return failed(PairingProblem.Server(outcome.error()))
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
            return failed(PairingProblem.Server(revealed.error()))
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
