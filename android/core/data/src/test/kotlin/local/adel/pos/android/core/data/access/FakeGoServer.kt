package local.adel.pos.android.core.data.access

import java.util.concurrent.ConcurrentLinkedQueue
import local.adel.pos.android.core.data.security.PinnedTrustManager
import local.adel.pos.android.core.domain.pairing.PairingProtocol
import local.adel.pos.android.core.domain.text.Hex
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate

/**
 * Servidor Go simulado con TLS para las pruebas de la JVM: las rutas de versión, meseros, entrada
 * con PIN y emparejamiento con las formas y los códigos de Go (`pin_login.go`, `pairing.go`). Nunca
 * es el servidor real.
 */
class FakeGoServer(
    val certificate: HeldCertificate =
        HeldCertificate.Builder().commonName("AD-EL POS prueba").build(),
) {
    val server = MockWebServer()
    val fingerprint: ByteArray = PinnedTrustManager.spkiFingerprint(certificate.certificate)
    val requests = ConcurrentLinkedQueue<RecordedRequest>()

    var serverVersion = "0.12.0"
    var capabilities = listOf("offline-orders-v1", "client-versions-by-kind", "waiter-pin-v1")
    var minAndroid = "0.1.0"
    var deviceId = "0b5d2f6c-6b1a-4c4b-9b70-3f0f8e1c2a10"
    var deviceToken = "t".repeat(43)
    var idleLockSeconds = 120

    /** Meseros con PIN: id → (nombre, PIN, pin_version). */
    val waiters = linkedMapOf<String, Triple<String, String, Int>>()
    /** PIN bloqueado (de todos los meseros) hasta esta hora RFC 3339. */
    var lockedUntil: String? = null

    /** PIN bloqueado (de todos) hasta que lo desbloquee un administrador (A1-D16). */
    var requiresUnlock = false
    var revoked = false
    var forbidden = emptySet<String>()
    var pairingState = "pending"

    /** Rechazo de la solicitud de código (por ejemplo 429 `PAIRING_RATE_LIMITED`). */
    var createRejection: Pair<Int, String>? = null
    val serverId = "11111111-2222-4333-8444-555555555555"
    val requestId = "00000000-0000-4000-8000-000000000001"
    val serverNonce = ByteArray(32) { (64 + it).toByte() }
    var revealedNonce: ByteArray? = null
    var claims = 0
    var logins = 0

    init {
        val handshake = HandshakeCertificates.Builder().heldCertificate(certificate).build()
        server.useHttps(handshake.sslSocketFactory())
        server.dispatcher =
            object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    requests.add(request)
                    return route(request)
                }
            }
        server.start()
    }

    /** Dirección como la guarda el dispositivo: `https://127.0.0.1:puerto`. */
    val base: String
        get() = "https://${server.hostName}:${server.port}"

    /** Código que calcula Go con su huella, si el dispositivo ya reveló su número. */
    fun expectedCode(): String? = revealedNonce?.let {
        PairingProtocol.code(fingerprint, it, serverNonce, requestId)
    }

    fun close() = server.close()

    private fun json(status: Int, body: String) =
        MockResponse.Builder()
            .code(status)
            .addHeader("Content-Type", "application/json; charset=utf-8")
            .body(body)
            .build()

    private fun fail(status: Int, code: String, details: String? = null): MockResponse {
        val extra = details?.let { ""","details":$it""" }.orEmpty()
        return json(
            status,
            """{"error":{"code":"$code","message":"Mensaje de Go.","request_id":"r-1"$extra}}""",
        )
    }

    private fun deviceOk(request: RecordedRequest): MockResponse? {
        if (revoked) {
            return fail(401, "DEVICE_REVOKED")
        }
        if (
            request.headers["X-Device-ID"] != deviceId ||
                request.headers["X-Device-Token"] != deviceToken
        ) {
            return fail(401, "DEVICE_REQUIRED")
        }
        return null
    }

    private fun route(request: RecordedRequest): MockResponse {
        val path = request.url.encodedPath.removePrefix("/api/v1/")
        val body = request.body?.utf8().orEmpty()
        return when {
            path == "version" -> json(200, versionBody())
            path == "health/live" -> json(200, """{"status":"alive"}""")
            path == "auth/pin-users" -> deviceOk(request) ?: json(200, pinUsersBody())
            path == "auth/pin-login" -> deviceOk(request) ?: pinLogin(body)
            path == "pairing/requests" && createRejection != null ->
                fail(createRejection!!.first, createRejection!!.second)
            path == "pairing/requests" ->
                json(
                    200,
                    """{"request_id":"$requestId","server_nonce":"${Hex.encode(serverNonce)}",
                    |"pairing_secret":"${"s".repeat(43)}","expires_at":"2099-01-01T00:05:00.5-06:00",
                    |"server_id":"$serverId","server_name":"Fonda Doña Ñeli","name":"Tableta 1"}"""
                        .trimMargin(),
                )
            path.endsWith("/reveal") -> {
                revealedNonce =
                    Hex.decode(
                        Regex("\"terminal_nonce\":\"([0-9a-f]{64})\"").find(body)!!.groupValues[1],
                    )
                json(200, """{"state":"pending","expires_at":"2099-01-01T00:05:00Z"}""")
            }
            path.endsWith("/claim") -> {
                claims += 1
                if (claims > 1) {
                    fail(409, "PAIRING_ALREADY_CLAIMED")
                } else {
                    json(
                        200,
                        """{"device_id":"$deviceId","device_token":"$deviceToken","server_id":"$serverId",
                        |"server_name":"Fonda Doña Ñeli","name":"Tableta de la terraza"}"""
                            .trimMargin(),
                    )
                }
            }
            path.endsWith("/name") -> json(200, """{"state":"pending","name":"Tableta 2"}""")
            path.endsWith("/cancel") -> json(200, """{"state":"cancelled"}""")
            path == "pairing/requests/$requestId" ->
                json(
                    200,
                    """{"state":"$pairingState","expires_at":"2099-01-01T00:05:00Z","name":"Tableta 1"}""",
                )
            else -> fail(404, "NOT_FOUND")
        }
    }

    private fun versionBody() =
        """{"product":"AD-EL POS","server_version":"$serverVersion","api_version":"v1","schema_version":25,
        |"supported_api_versions":["v1"],"capabilities":[${capabilities.joinToString(",") { "\"$it\"" }}],
        |"events_available":true,"test_mode":true,"min_client_version":"0.10.0",
        |"min_client_versions":{"desktop":"0.10.0","android":"$minAndroid"},
        |"client_kinds":["desktop","android"]}"""
            .trimMargin()

    private fun pinUsersBody(): String {
        val users =
            waiters.entries.joinToString(",") { (id, value) ->
                val locked = lockedUntil?.takeUnless { requiresUnlock }?.let { "\"$it\"" } ?: "null"
                """{"id":"$id","name":"${value.first}","initial":"${value.first.take(1)}","color":"#3591d4",
                |"pin_version":${value.third},"locked_until":$locked,"requires_unlock":$requiresUnlock}"""
                    .trimMargin()
            }
        return """{"device":{"id":"$deviceId","name":"Tableta de la terraza","idle_lock_seconds":$idleLockSeconds},
        |"users":[$users]}"""
            .trimMargin()
    }

    private fun pinLogin(body: String): MockResponse {
        val userId = Regex("\"user_id\":\"([^\"]+)\"").find(body)?.groupValues?.get(1)
        val pin = Regex("\"pin\":\"([^\"]+)\"").find(body)?.groupValues?.get(1)
        val waiter = waiters[userId] ?: return fail(401, "PIN_INVALID")
        if (requiresUnlock) {
            return fail(429, "PIN_LOCKED", """{"locked_until":null,"requires_unlock":true}""")
        }
        if (lockedUntil != null) {
            return fail(429, "PIN_LOCKED", """{"locked_until":"$lockedUntil"}""")
        }
        if (waiter.second != pin) {
            return fail(401, "PIN_INVALID")
        }
        if (userId in forbidden) {
            return fail(403, "PERMISSION_DENIED")
        }
        logins += 1
        return json(
            200,
            """{"session_token":"${"a".repeat(42)}$logins","pin_version":${waiter.third},
            |"offline_credential":"${"c".repeat(42)}$logins"}"""
                .trimMargin(),
        )
    }
}
