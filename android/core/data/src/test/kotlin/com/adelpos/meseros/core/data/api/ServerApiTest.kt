package com.adelpos.meseros.core.data.api

import com.adelpos.meseros.core.data.security.PinnedTrustManager
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.error.AppResult
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Cliente de la API v1 contra un MockWebServer con TLS: huella fijada aceptada y rechazada,
 * cabeceras del cliente y correspondencia de los errores de Go con los de la aplicación.
 */
class ServerApiTest {
    // Certificado autofirmado sin nombres alternativos, como el de Go (identity/server-cert.pem).
    private val serverCertificate = HeldCertificate.Builder().commonName("AD-EL POS prueba").build()
    private val identity = ClientIdentity(version = "0.1.0")
    private lateinit var server: MockWebServer

    @Before
    fun start() {
        server = MockWebServer()
        val handshake = HandshakeCertificates.Builder().heldCertificate(serverCertificate).build()
        server.useHttps(handshake.sslSocketFactory())
        server.start()
    }

    @After
    fun stop() {
        server.close()
    }

    private fun api(
        pin: ByteArray = PinnedTrustManager.spkiFingerprint(serverCertificate.certificate),
    ): ServerApi = ServerApi(server.url("/"), PinnedHttpClient.create(pin, identity))

    private fun respond(
        status: Int,
        body: String,
        contentType: String = "application/json; charset=utf-8",
    ) {
        server.enqueue(
            MockResponse.Builder()
                .code(status)
                .addHeader("Content-Type", contentType)
                .body(body)
                .build(),
        )
    }

    private fun failureOf(result: AppResult<*>): AppError {
        assertTrue("Se esperaba una falla: $result", result is AppResult.Failure)
        return (result as AppResult.Failure).error
    }

    @Test
    fun aceptaLaHuellaFijadaYEnviaLasCabecerasDelCliente() = runBlocking {
        respond(200, VERSION_BODY)
        val result = api().getVersion()
        val version = (result as AppResult.Success).value
        assertEquals("0.11.0", version.serverVersion)
        assertEquals(listOf("desktop", "android"), version.clientKinds)
        assertTrue(version.capabilities.contains("offline-orders-v1"))
        // Un servidor anterior a 0.12.0 no trae la mínima por tipo de cliente.
        assertEquals(emptyMap<String, String>(), version.minClientVersions)
        val request = server.takeRequest()
        assertEquals("/api/v1/version", request.url.encodedPath)
        assertEquals("0.1.0", request.headers["X-Client-Version"])
        assertEquals("android", request.headers["X-Client-Kind"])
    }

    @Test
    fun leeLaMinimaPorTipoDeClienteYCamposNuevos() = runBlocking {
        respond(
            200,
            VERSION_BODY.replace(
                "\"client_kinds\"",
                "\"min_client_versions\":{\"desktop\":\"0.10.0\",\"android\":\"0.1.0\"},\"futuro\":1,\"client_kinds\"",
            ),
        )
        val version = (api().getVersion() as AppResult.Success).value
        assertEquals("0.1.0", version.minClientVersions["android"])
    }

    @Test
    fun rechazaUnCertificadoConOtraHuellaSinEnviarLaPeticion() = runBlocking {
        respond(200, VERSION_BODY)
        val other = HeldCertificate.Builder().commonName("AD-EL POS prueba").build()
        val result = api(pin = PinnedTrustManager.spkiFingerprint(other.certificate)).getVersion()
        assertEquals(AppError.ServerIdentityChanged, failureOf(result))
        assertEquals(0, server.requestCount)
    }

    @Test
    fun actualizarEstaAplicacionCon426() = runBlocking {
        respond(
            426,
            """
            |{"error":{"code":"CLIENT_UPDATE_REQUIRED","message":"Actualiza esta terminal.",
            |"request_id":"r-1","details":{"client_version":"0.1.0","min_client_version":"0.2.0",
            |"server_version":"0.12.0"}}}
            """
                .trimMargin(),
        )
        val error = failureOf(api().getVersion())
        assertEquals(
            AppError.UpdateRequired(
                code = "CLIENT_UPDATE_REQUIRED",
                minClientVersion = "0.2.0",
                serverVersion = "0.12.0",
                serverMessage = "Actualiza esta terminal.",
                requestId = "r-1",
            ),
            error,
        )
    }

    @Test
    fun versionOTipoQueElServidorNoReconoceTambienPidenActualizar() = runBlocking {
        respond(
            400,
            """{"error":{"code":"CLIENT_KIND_INVALID","message":"Actualiza esta terminal."}}""",
        )
        val error = failureOf(api().getVersion())
        assertTrue(error is AppError.UpdateRequired)
        assertEquals("CLIENT_KIND_INVALID", error.code)
    }

    @Test
    fun dispositivoRetiradoYSinCredencial() = runBlocking {
        respond(
            401,
            """{"error":{"code":"DEVICE_REVOKED","message":"Terminal revocada.","request_id":"r-2"}}""",
        )
        respond(401, """{"error":{"code":"DEVICE_REQUIRED","message":"Empareja esta terminal."}}""")
        assertEquals(
            AppError.DeviceRevoked("Terminal revocada.", "r-2"),
            failureOf(api().getVersion()),
        )
        assertEquals(
            AppError.DeviceNotRecognized("DEVICE_REQUIRED", "Empareja esta terminal.", null),
            failureOf(api().getVersion()),
        )
    }

    @Test
    fun sesionTerminadaYPermisoNegado() = runBlocking {
        respond(401, """{"error":{"code":"SESSION_EXPIRED","message":"Tu sesión terminó."}}""")
        respond(403, """{"error":{"code":"PERMISSION_DENIED","message":"No tienes permiso."}}""")
        assertEquals(
            AppError.SessionEnded("SESSION_EXPIRED", "Tu sesión terminó.", null),
            failureOf(api().getVersion()),
        )
        assertEquals(
            AppError.PermissionDenied("No tienes permiso.", null),
            failureOf(api().getVersion()),
        )
    }

    @Test
    fun otrosRechazosConservanSuCodigo() = runBlocking {
        respond(
            409,
            """{"error":{"code":"OFFLINE_ORDER_ALREADY_ACCEPTED","message":"Ya entró.","details":{"result":{}}}}""",
        )
        respond(429, """{"error":{"code":"LOGIN_THROTTLED","message":"Espera 15 minutos."}}""")
        assertEquals(
            AppError.Rejected(409, "OFFLINE_ORDER_ALREADY_ACCEPTED", "Ya entró.", null),
            failureOf(api().getVersion()),
        )
        assertEquals(
            AppError.TryLater("LOGIN_THROTTLED", "Espera 15 minutos.", null),
            failureOf(api().getVersion()),
        )
    }

    @Test
    fun losDetallesDelRechazoQuedanDisponibles() {
        val problem =
            ApiErrors.problemOf(
                409,
                """
                |{"error":{"code":"OFFLINE_ORDER_ALREADY_ACCEPTED","message":"Ya entró.","request_id":"r-3",
                |"details":{"result":{"account_id":"a-1"}}}}
                """
                    .trimMargin(),
            )
        assertEquals("OFFLINE_ORDER_ALREADY_ACCEPTED", problem.code)
        assertEquals("r-3", problem.requestId)
        assertTrue(problem.details!!.containsKey("result"))
    }

    @Test
    fun fallasDelServidorYRespuestasSinLaFormaDeGo() = runBlocking {
        respond(503, "Servicio no disponible", contentType = "text/plain")
        respond(404, "<html>no</html>", contentType = "text/html")
        respond(200, """{"product":"AD-EL POS"}""")
        assertEquals(AppError.ServerProblem(503, null, null, null), failureOf(api().getVersion()))
        assertEquals(AppError.InvalidResponse(404), failureOf(api().getVersion()))
        assertEquals(AppError.InvalidResponse(200), failureOf(api().getVersion()))
    }

    @Test
    fun sinServidorEsInalcanzable() = runBlocking {
        val client = api()
        server.close()
        assertEquals(AppError.Unreachable, failureOf(client.getVersion()))
    }

    private companion object {
        // Respuesta real de GET /api/v1/version del servidor 0.11.0 (contrato VersionInfo).
        const val VERSION_BODY =
            """{"product":"AD-EL POS","server_version":"0.11.0","api_version":"v1","schema_version":24,
            "supported_api_versions":["v1"],"capabilities":["authenticated-sse-v1","client-version-check",
            "openapi-v1","catalog-sync-v1","catalog-image-content","catalog-sold-out","offline-orders-v1",
            "state-recovery-v1"],"events_available":true,"test_mode":true,"min_client_version":"0.10.0",
            "client_kinds":["desktop","android"],"openapi_path":"/api/v1/openapi.json"}"""
    }
}
