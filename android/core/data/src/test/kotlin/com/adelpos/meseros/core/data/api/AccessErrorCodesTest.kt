package com.adelpos.meseros.core.data.api

import com.adelpos.meseros.core.data.security.FirstSeenTrustManager
import com.adelpos.meseros.core.data.security.PinMismatchException
import com.adelpos.meseros.core.data.security.PinnedTrustManager
import com.adelpos.meseros.core.domain.error.AppError
import java.security.cert.X509Certificate
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Correspondencia de los códigos de error nuevos de las fases B y C (PIN, credencial de envío y
 * emparejamiento) con los errores de la aplicación, la confianza del primer certificado de la
 * sesión de emparejamiento y la marca «la petición ya salió» que decide si se puede repetir en otra
 * dirección.
 */
class AccessErrorCodesTest {
    private fun problem(status: Int, code: String, details: String? = null): AppError {
        val extra = details?.let { ""","details":$it""" }.orEmpty()
        return ApiErrors.toAppError(
            ApiErrors.problemOf(
                status,
                """{"error":{"code":"$code","message":"Mensaje.","request_id":"r"$extra}}""",
            ),
        )
    }

    @Test
    fun codigosDelPinYDeLaCredencialDeEnvio() {
        assertEquals(
            AppError.Rejected(401, "PIN_INVALID", "Mensaje.", "r"),
            problem(401, "PIN_INVALID"),
        )
        assertEquals(
            AppError.TryLater("PIN_LOCKED", "Mensaje.", "r"),
            problem(429, "PIN_LOCKED", """{"locked_until":"2026-11-16T14:35:00Z"}"""),
        )
        assertEquals(AppError.PermissionDenied("Mensaje.", "r"), problem(403, "PERMISSION_DENIED"))
        assertEquals(
            AppError.Rejected(403, "PIN_DEVICE_NOT_ALLOWED", "Mensaje.", "r"),
            problem(403, "PIN_DEVICE_NOT_ALLOWED"),
        )
        assertEquals(
            AppError.Rejected(401, "OFFLINE_CREDENTIAL_INVALID", "Mensaje.", "r"),
            problem(401, "OFFLINE_CREDENTIAL_INVALID"),
        )
        assertEquals(
            AppError.Rejected(401, "OFFLINE_CREDENTIAL_NOT_ALLOWED", "Mensaje.", "r"),
            problem(401, "OFFLINE_CREDENTIAL_NOT_ALLOWED"),
        )
        assertEquals(AppError.DeviceRevoked("Mensaje.", "r"), problem(401, "DEVICE_REVOKED"))
        assertEquals(
            AppError.DeviceNotRecognized("DEVICE_REQUIRED", "Mensaje.", "r"),
            problem(401, "DEVICE_REQUIRED"),
        )
        assertEquals(
            AppError.SessionEnded("SESSION_REVOKED", "Mensaje.", "r"),
            problem(401, "SESSION_REVOKED"),
        )
    }

    @Test
    fun codigosDelEmparejamiento() {
        assertEquals(
            AppError.TryLater("PAIRING_RATE_LIMITED", "Mensaje.", "r"),
            problem(429, "PAIRING_RATE_LIMITED"),
        )
        assertEquals(
            AppError.TryLater("PAIRING_LIMIT_REACHED", "Mensaje.", "r"),
            problem(429, "PAIRING_LIMIT_REACHED"),
        )
        for (code in listOf("PAIRING_EXPIRED", "PAIRING_REJECTED", "PAIRING_ALREADY_CLAIMED")) {
            assertEquals(AppError.Rejected(409, code, "Mensaje.", "r"), problem(409, code))
        }
        assertEquals(
            AppError.ServerProblem(503, "PAIRING_UNAVAILABLE", "Mensaje.", "r"),
            problem(503, "PAIRING_UNAVAILABLE"),
        )
    }

    private val first = HeldCertificate.Builder().commonName("A").build()
    private val second = HeldCertificate.Builder().commonName("B").build()

    private fun chain(certificate: HeldCertificate): Array<X509Certificate> =
        arrayOf(certificate.certificate)

    @Test
    fun laSesionDeEmparejamientoFijaElPrimerCertificado() {
        val trust = FirstSeenTrustManager()
        assertNull(trust.fingerprint)
        trust.checkServerTrusted(chain(first), "ECDHE_ECDSA")
        assertArrayEquals(PinnedTrustManager.spkiFingerprint(first.certificate), trust.fingerprint)
        trust.checkServerTrusted(chain(first), "ECDHE_ECDSA")
        // Un intermediario con otro certificado a mitad de la sesión se rechaza.
        assertThrows(PinMismatchException::class.java) {
            trust.checkServerTrusted(chain(second), "ECDHE_ECDSA")
        }
        assertThrows(PinMismatchException::class.java) {
            trust.checkServerTrusted(emptyArray(), "ECDHE_ECDSA")
        }
        assertArrayEquals(PinnedTrustManager.spkiFingerprint(first.certificate), trust.fingerprint)
    }

    @Test
    fun laPeticionRechazadaPorLaHuellaNoSalioYLaRespondidaSi() = runBlocking {
        val server = MockWebServer()
        server.useHttps(
            HandshakeCertificates.Builder().heldCertificate(first).build().sslSocketFactory(),
        )
        server.enqueue(MockResponse.Builder().code(200).body("""{"status":"alive"}""").build())
        server.start()
        try {
            val identity = ClientIdentity("0.1.0")
            val wrong =
                ServerApi(
                        server.url("/"),
                        PinnedHttpClient.create(
                            PinnedTrustManager.spkiFingerprint(second.certificate),
                            identity,
                        ),
                    )
                    .get("health/live", IgnoredBody.serializer())
            assertEquals(
                ApiOutcome.NoAnswer(AppError.ServerIdentityChanged, requestSent = false),
                wrong,
            )
            val right =
                ServerApi(
                        server.url("/"),
                        PinnedHttpClient.create(
                            PinnedTrustManager.spkiFingerprint(first.certificate),
                            identity,
                        ),
                    )
                    .get("health/live", IgnoredBody.serializer())
            assertTrue(right is ApiOutcome.Success)
            assertEquals(1, server.requestCount)
        } finally {
            server.close()
        }
        // Sin servidor que escuche: no se pudo conectar y nada salió.
        val closed =
            ServerApi(
                    server.url("/"),
                    PinnedHttpClient.create(ByteArray(32), ClientIdentity("0.1.0")),
                )
                .get("health/live", IgnoredBody.serializer())
        assertTrue(closed is ApiOutcome.NoAnswer)
        assertFalse((closed as ApiOutcome.NoAnswer).requestSent)
        assertEquals(AppError.Unreachable, closed.error)
    }
}
